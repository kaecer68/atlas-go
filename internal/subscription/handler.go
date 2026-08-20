package subscription

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// Handler serves subscription and auth endpoints.
type Handler struct {
	store    *Store
	jwt      *JWTManager
	waitlist *WaitlistStore
}

// NewHandler creates a subscription HTTP handler.
func NewHandler(store *Store, jwt *JWTManager) *Handler {
	return &Handler{store: store, jwt: jwt}
}

func hashPassword(email, password string) string {
	h := sha256.Sum256([]byte(email + ":" + password))
	return hex.EncodeToString(h[:])
}

// RegisterRoutes registers subscription endpoints.
//
// allowGuest=true downgrades missing/invalid tokens to TierFree
// instead of 401 — see cmd/atlas/main.go ATLAS_REQUIRE_USER_AUTH.
func (h *Handler) RegisterRoutes(mux *http.ServeMux, allowGuest bool) {
	mid := NewAuthMiddleware(h.jwt, allowGuest)

	mux.HandleFunc("POST /api/auth/register", h.handleRegister)
	mux.HandleFunc("POST /api/auth/login", h.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", h.handleLogout)
	mux.Handle("GET /api/user/profile", mid.Wrap(http.HandlerFunc(h.handleProfile)))
	mux.Handle("GET /api/user/subscription", mid.Wrap(http.HandlerFunc(h.handleSubscription)))
	if h.waitlist != nil {
		mux.HandleFunc("POST /api/waitlist", h.handleWaitlist)
	}
}

// setAuthCookie writes the HttpOnly token cookie. Shared by /register
// and /login so both endpoints produce identical session state.
func setAuthCookie(w http.ResponseWriter, token string) {
	//nolint:gosec // local dev without HTTPS; Secure flag is environment-dependent
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(24 * time.Hour.Seconds()),
	})
}

func (h *Handler) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	if req.Email == "" || req.Password == "" {
		http.Error(w, `{"error":"email and password required"}`, http.StatusBadRequest)
		return
	}
	hash := hashPassword(req.Email, req.Password)
	user, err := h.store.Register(req.Email, hash)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	token, _ := h.jwt.Generate(user, 24*time.Hour)
	logging.Info("subscription", "user_registered", "email", req.Email)
	setAuthCookie(w, token)
	writeJSON(w, http.StatusCreated, map[string]any{
		"user":  user,
		"token": token,
	})
}

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid body"}`, http.StatusBadRequest)
		return
	}
	hash := hashPassword(req.Email, req.Password)
	user, err := h.store.Authenticate(req.Email, hash)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	token, _ := h.jwt.Generate(user, 24*time.Hour)
	logging.Info("subscription", "user_login", "email", req.Email)
	setAuthCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{
		"user":  user,
		"token": token,
	})
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	//nolint:gosec // local dev without HTTPS; Secure flag is environment-dependent
	c := &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	}
	http.SetCookie(w, c)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (h *Handler) handleProfile(w http.ResponseWriter, r *http.Request) {
	claims := GetClaims(r)
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	// C-02: profile is assembled from the verified JWT claims (go-member
	// access token) rather than the local users table — atlas no longer owns
	// the membership source of truth. Guest claims (free) have no identity.
	tier := Tier(claims.Tier)
	guest := claims.Email == "" && claims.Sub == ""
	trialEnd := time.Time{}
	if claims.MembershipExpiresAt > 0 {
		trialEnd = time.Unix(claims.MembershipExpiresAt, 0)
	}
	profile := ProfileResponse{
		User:          nil,
		Email:         claims.Email,
		Tier:          tier,
		EffectiveTier: tier,
		TrialEnd:      trialEnd,
		Guest:         guest,
	}
	// Best-effort backward compat: a legacy self-signed (HS256) token still
	// carries an email that maps to a local users row; enrich the profile with
	// it so existing demo behavior is preserved. Go-member (JWKS) members have
	// no local row, so the profile stays purely claims-derived.
	if !guest && h.store != nil {
		if u, err := h.store.GetByEmail(claims.Email); err == nil && u != nil {
			profile.User = u
		}
	}
	writeJSON(w, http.StatusOK, profile)
}

func (h *Handler) handleSubscription(w http.ResponseWriter, r *http.Request) {
	claims := GetClaims(r)
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	// C-02: tier + membership expiry are read from the verified JWT claims
	// (go-member access token), not the local users table.
	tier := Tier(claims.Tier)
	guest := claims.Email == "" && claims.Sub == ""
	var trialEnd any
	trialEnd = nil
	if claims.MembershipExpiresAt > 0 {
		trialEnd = time.Unix(claims.MembershipExpiresAt, 0)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tier":      tier,
		"effective": tier,
		"trial_end": trialEnd,
		"guest":     guest,
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data) // write path; client disconnected is non-fatal
}
