package subscription

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// proxyTimeout bounds each go-member upstream call (login/register proxy).
const proxyTimeout = 8 * time.Second

// Handler serves subscription and auth endpoints.
type Handler struct {
	store    *Store
	jwt      *JWTManager
	waitlist *WaitlistStore

	// goMemberAPIBaseURL is the go-member API base (GO_MEMBER_API_BASE_URL).
	// When empty, login/register fall back to the legacy local HS256 path
	// for backward compatibility (see handleLogin/handleRegister).
	goMemberAPIBaseURL string
	// proxyClient is the HTTP client used to reach go-member. Credentials are
	// never logged; only the email is (matching prior behavior).
	proxyClient *http.Client
}

// NewHandler creates a subscription HTTP handler.
func NewHandler(store *Store, jwt *JWTManager) *Handler {
	return &Handler{
		store:       store,
		jwt:         jwt,
		proxyClient: &http.Client{Timeout: proxyTimeout},
	}
}

// WithGoMember enables the go-member thin-proxy login/register path, pointing
// at go-member's API base (e.g. "http://host.docker.internal:8093"). When
// unset (default), login/register keep using the local HS256 store.
func (h *Handler) WithGoMember(apiBaseURL string) *Handler {
	h.goMemberAPIBaseURL = strings.TrimSpace(apiBaseURL)
	return h
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

	// M4b: when go-member is configured, register is a thin proxy — go-member
	// owns the user source of truth and mints/verifies membership. On success
	// it returns 201 {id,email,message} WITHOUT a token (email verification
	// required before login), matching the spec.
	if h.goMemberAPIBaseURL != "" {
		status, payload := h.goMemberPost(r.Context(), "/api/v1/auth/register", req.Email, req.Password)
		if status >= 200 && status < 300 {
			logging.Info("subscription", "user_registered", "email", req.Email)
			// Pass through go-member's {id,email,message} unchanged.
			writeJSON(w, status, payload)
			return
		}
		// 409 email exists and any other upstream error: mirror status, generic message.
		msg := extractMessage(payload, "register failed")
		writeJSON(w, status, map[string]string{"error": msg})
		return
	}

	// Legacy HS256 self-signed path (GoMemberAPIBaseURL unset) — backward
	// compatible local users-table register.
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
	if req.Email == "" || req.Password == "" {
		http.Error(w, `{"error":"email and password required"}`, http.StatusBadRequest)
		return
	}

	// M4b: thin proxy to go-member. The RS256 access token returned by
	// go-member is stored in the cookie and echoed in the JSON body so the
	// frontend auth.js can cache it for the Authorization header. Atlas no
	// longer re-mints an HS256 token or reads the local users table here.
	if h.goMemberAPIBaseURL != "" {
		status, payload := h.goMemberPost(r.Context(), "/api/v1/auth/login", req.Email, req.Password)
		if status == http.StatusOK {
			accessToken, _ := payload["accessToken"].(string)
			if accessToken == "" {
				writeJSON(w, http.StatusBadGateway, map[string]string{"error": "login upstream returned no token"})
				return
			}
			logging.Info("subscription", "user_login", "email", req.Email)
			setAuthCookie(w, accessToken)
			writeJSON(w, http.StatusOK, map[string]any{
				"user":  map[string]string{"email": req.Email},
				"token": accessToken,
			})
			return
		}
		// Error passthrough: preserve go-member status; never leak credentials.
		h.writeGoMemberLoginError(w, status, payload)
		return
	}

	// Legacy HS256 self-signed path (GoMemberAPIBaseURL unset).
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

// goMemberPost sends a json {email,password} POST to a go-member auth path and
// returns the status code plus the decoded JSON payload. Credentials are never
// logged — only the request path. Network/upstream failures surface as
// 502 Bad Gateway with a generic message.
func (h *Handler) goMemberPost(ctx context.Context, path, email, password string) (int, map[string]any) {
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(h.goMemberAPIBaseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		logging.Warn("subscription", "go_member_proxy_error", logging.Err(err), logging.FStr("path", path))
		return http.StatusBadGateway, map[string]any{"error": "login upstream unavailable"}
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.proxyClient.Do(req)
	if err != nil {
		logging.Warn("subscription", "go_member_proxy_error", logging.Err(err), logging.FStr("path", path))
		return http.StatusBadGateway, map[string]any{"error": "login upstream unavailable"}
	}
	defer func() { _ = resp.Body.Close() }()
	var payload map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&payload); err != nil {
		payload = map[string]any{}
	}
	return resp.StatusCode, payload
}

// writeGoMemberLoginError maps go-member login failures to atlas status codes
// (per M4b spec): 401→invalid credentials, 403→banned/not approved,
// 422→email not verified, 429→rate limited. The go-member body message is
// preserved where meaningful (403/422/429); 401 always reports the generic
// "invalid credentials" so it never hints at account existence.
func (h *Handler) writeGoMemberLoginError(w http.ResponseWriter, status int, payload map[string]any) {
	msg := extractMessage(payload, "")
	switch status {
	case http.StatusUnauthorized:
		writeJSON(w, status, map[string]string{"error": "invalid credentials"})
	case http.StatusForbidden:
		if msg == "" {
			msg = "account banned or not approved"
		}
		writeJSON(w, status, map[string]string{"error": msg})
	case http.StatusUnprocessableEntity:
		if msg == "" {
			msg = "email not verified"
		}
		writeJSON(w, status, map[string]string{"error": msg})
	case http.StatusTooManyRequests:
		if msg == "" {
			msg = "rate limited"
		}
		writeJSON(w, status, map[string]string{"error": msg})
	default:
		if msg == "" {
			msg = "login failed"
		}
		writeJSON(w, status, map[string]string{"error": msg})
	}
}

// extractMessage pulls a human-readable message from a go-member error JSON
// payload, preferring "message" then "error". Returns fallback if neither is
// present or non-empty.
func extractMessage(payload map[string]any, fallback string) string {
	if payload == nil {
		return fallback
	}
	for _, key := range []string{"message", "error"} {
		if v, ok := payload[key].(string); ok && v != "" {
			return v
		}
	}
	return fallback
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
