package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// TokenAdminHandler serves the management API for MCP tokens.
// It requires an X-Admin-Token header matching the configured admin token.
// When the admin token is empty, all requests are rejected (no default).
type TokenAdminHandler struct {
	store      TokenStore
	adminToken string
}

// NewTokenAdminHandler creates a handler. adminToken == "" rejects all requests.
func NewTokenAdminHandler(store TokenStore, adminToken string) *TokenAdminHandler {
	return &TokenAdminHandler{store: store, adminToken: adminToken}
}

// ServeHTTP dispatches requests.
//
//	POST   /api/admin/mcp/tokens              — register
//	DELETE /api/admin/mcp/tokens/<id>          — revoke
//	POST   /api/admin/mcp/tokens/<id>/rotate   — rotate
//	GET    /api/admin/mcp/tokens               — list (redacted)
func (h *TokenAdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "token store not configured (DATABASE_URL not set)",
		})
		return
	}
	if !h.checkAdmin(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "missing or invalid X-Admin-Token",
		})
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/admin/mcp/tokens")
	path = strings.TrimPrefix(path, "/")

	switch {
	case r.Method == http.MethodPost && path == "":
		h.handleRegister(w, r)
	case r.Method == http.MethodGet && path == "":
		h.handleList(w, r)
	case r.Method == http.MethodDelete && !strings.Contains(path, "/"):
		h.handleRevoke(w, r, path)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/rotate"):
		id := strings.TrimSuffix(path, "/rotate")
		h.handleRotate(w, r, id)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "not found",
		})
	}
}

func (h *TokenAdminHandler) checkAdmin(r *http.Request) bool {
	if h.adminToken == "" {
		return false
	}
	got := r.Header.Get("X-Admin-Token")
	if len(got) != len(h.adminToken) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(h.adminToken)) == 1
}

func (h *TokenAdminHandler) handleRegister(w http.ResponseWriter, r *http.Request) {
	var reg TokenRegistration
	if err := json.NewDecoder(r.Body).Decode(&reg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	if reg.TenantID == "" || reg.AgentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tenant_id and agent_id are required"})
		return
	}

	info, raw, err := h.store.Register(r.Context(), reg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "register: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"token_id":   info.TokenID.String(),
		"token":      raw,
		"tenant_id":  info.TenantID,
		"agent_id":   info.AgentID,
		"scopes":     info.Scopes,
		"created_at": info.CreatedAt,
	})
}

func (h *TokenAdminHandler) handleRevoke(w http.ResponseWriter, r *http.Request, idStr string) {
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid token_id"})
		return
	}
	if err := h.store.Revoke(r.Context(), id); err != nil {
		if errors.Is(err, ErrTokenNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "revoke: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked", "token_id": idStr})
}

func (h *TokenAdminHandler) handleRotate(w http.ResponseWriter, r *http.Request, idStr string) {
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid token_id"})
		return
	}
	info, raw, err := h.store.Rotate(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrTokenNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "token not found"})
			return
		}
		if errors.Is(err, ErrRevoked) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token already revoked"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "rotate: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token_id":  info.TokenID.String(),
		"token":     raw,
		"tenant_id": info.TenantID,
		"agent_id":  info.AgentID,
		"scopes":    info.Scopes,
	})
}

func (h *TokenAdminHandler) handleList(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.store.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list: " + err.Error()})
		return
	}
	if tokens == nil {
		tokens = []TokenInfo{}
	}

	// Redact: return only safe fields, with token_hash shown as sha256:prefix.
	type safeToken struct {
		TokenID         string   `json:"token_id"`
		TokenHash       string   `json:"token_hash"`
		TenantID        string   `json:"tenant_id"`
		AgentID         string   `json:"agent_id"`
		Scopes          []string `json:"scopes"`
		RateLimitPerMin *int     `json:"rate_limit_per_min,omitempty"`
		CreatedAt       string   `json:"created_at"`
		ExpiresAt       *string  `json:"expires_at,omitempty"`
		RevokedAt       *string  `json:"revoked_at,omitempty"`
	}

	out := make([]safeToken, 0, len(tokens))
	for _, t := range tokens {
		st := safeToken{
			TokenID:         t.TokenID.String(),
			TokenHash:       redactHash(t.TokenHash),
			TenantID:        t.TenantID,
			AgentID:         t.AgentID,
			Scopes:          t.Scopes,
			RateLimitPerMin: t.RateLimitPerMin,
			CreatedAt:       t.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
		if st.Scopes == nil {
			st.Scopes = []string{}
		}
		if t.ExpiresAt != nil {
			s := t.ExpiresAt.Format("2006-01-02T15:04:05Z")
			st.ExpiresAt = &s
		}
		if t.RevokedAt != nil {
			s := t.RevokedAt.Format("2006-01-02T15:04:05Z")
			st.RevokedAt = &s
		}
		out = append(out, st)
	}

	writeJSON(w, http.StatusOK, out)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func redactHash(hash string) string {
	if hash == "" {
		return "sha256:"
	}
	prefixLen := min(len(hash), 8)
	return "sha256:" + hash[:prefixLen] + "..."
}

// StartAdminServer starts the admin HTTP server on addr and blocks until
// ctx is cancelled. Performs graceful shutdown.
func StartAdminServer(ctx context.Context, addr string, handler http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		return srv.Shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}
