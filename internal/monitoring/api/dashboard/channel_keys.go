package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/kaecer68/atlas-go/internal/channelsecrets"
)

// ChannelKeysManager is the slice of *channelsecrets.Manager the dashboard
// handlers need (interface keeps the dashboard package decoupled and tests
// light).
type ChannelKeysManager interface {
	Set(ctx context.Context, provider, apiKey, updatedBy string) (channelsecrets.KeyStatus, error)
	Status(ctx context.Context, envObserved map[string]string) ([]channelsecrets.KeyStatus, error)
	EncryptionEnabled() bool
}

// envKeyNames maps provider → the env var whose value was used at startup
// (config merge base). Used by the list endpoint to mark rows sourced from
// .env rather than the DB store.
var envKeyNames = map[string]string{
	"finmind": "FINMIND_API_KEY",
	"fugle":   "FUGLE_API_KEY",
}

// HandleChannelKeysList backs GET /api/admin/channel-keys (issue #1776
// Phase 1). Never returns plaintext keys — masked (last 4) only.
func (h *Handlers) HandleChannelKeysList(r *http.Request) (int, any) {
	if h.ChannelKeys == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "channel key manager not initialized"}
	}
	envObserved := make(map[string]string, len(envKeyNames))
	for provider, envName := range envKeyNames {
		envObserved[provider] = os.Getenv(envName)
	}
	keys, err := h.ChannelKeys.Status(r.Context(), envObserved)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
	return http.StatusOK, map[string]any{
		"keys":               keys,
		"encryption_enabled": h.ChannelKeys.EncryptionEnabled(),
	}
}

// HandleChannelKeyUpdate backs PUT /api/admin/channel-keys/{provider}
// (issue #1776 Phase 1): validate → encrypt → persist → hot reload.
func (h *Handlers) HandleChannelKeyUpdate(r *http.Request) (int, any) {
	if h.ChannelKeys == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "channel key manager not initialized"}
	}
	provider := r.PathValue("provider")
	var req struct {
		APIKey    string `json:"api_key"`
		UpdatedBy string `json:"updated_by"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 64<<10)).Decode(&req); err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid body"}
	}
	if strings.TrimSpace(req.APIKey) == "" {
		return http.StatusBadRequest, map[string]string{"error": "api_key required"}
	}
	if req.UpdatedBy == "" {
		// Default to the authenticated principal when the proxy does not
		// supply one (member admin panel passes the operator identity).
		req.UpdatedBy = "admin"
	}
	st, err := h.ChannelKeys.Set(r.Context(), provider, req.APIKey, req.UpdatedBy)
	switch {
	case errors.Is(err, channelsecrets.ErrUnknownProvider):
		return http.StatusBadRequest, map[string]string{"error": "unknown provider (allowed: finmind, fugle)"}
	case errors.Is(err, channelsecrets.ErrInvalidKey):
		return http.StatusBadRequest, map[string]string{"error": "api_key length invalid (8-512)"}
	case errors.Is(err, channelsecrets.ErrNoMasterKey):
		return http.StatusServiceUnavailable, map[string]string{"error": "ATLAS_SECRET_STORE_KEY not set — refusing to store plaintext keys"}
	case err != nil:
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
	return http.StatusOK, map[string]any{"status": "ok", "key": st}
}
