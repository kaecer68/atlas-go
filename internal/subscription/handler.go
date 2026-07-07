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
	store *Store
	jwt   *JWTManager
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
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mid := NewAuthMiddleware(h.jwt)

	mux.HandleFunc("POST /api/auth/register", h.handleRegister)
	mux.HandleFunc("POST /api/auth/login", h.handleLogin)
	mux.Handle("GET /api/user/profile", mid.Wrap(http.HandlerFunc(h.handleProfile)))
	mux.Handle("GET /api/user/subscription", mid.Wrap(http.HandlerFunc(h.handleSubscription)))
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
	//nolint:gosec // local dev without HTTPS; Secure flag is environment-dependent
	c := &http.Cookie{
		Name:     "token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(24 * time.Hour.Seconds()),
	}
	http.SetCookie(w, c)
	writeJSON(w, http.StatusOK, map[string]any{
		"user":  user,
		"token": token,
	})
}

func (h *Handler) handleProfile(w http.ResponseWriter, r *http.Request) {
	claims := GetClaims(r)
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	user, err := h.store.GetByEmail(claims.Email)
	if err != nil || user == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "user lookup failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":           user,
		"email":          user.Email,
		"tier":           user.Tier,
		"effective_tier": user.EffectiveTier(),
		"trial_end":      user.TrialEnd,
	})
}

func (h *Handler) handleSubscription(w http.ResponseWriter, r *http.Request) {
	claims := GetClaims(r)
	if claims == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	user, err := h.store.GetByEmail(claims.Email)
	if err != nil || user == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "user lookup failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tier":      user.Tier,
		"effective": user.EffectiveTier(),
		"trial_end": user.TrialEnd,
	})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data) // write path; client disconnected is non-fatal
}
