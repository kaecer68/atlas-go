package dashboard

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

// HandleAPIKeyUpdate updates an API key in the environment.
func (h *Handlers) HandleAPIKeyUpdate(r *http.Request) (int, any) {
	var req struct {
		Provider string `json:"provider"`
		APIKey   string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid body"}
	}
	if req.Provider == "" || req.APIKey == "" {
		return http.StatusBadRequest, map[string]string{"error": "provider and api_key required"}
	}

	allowedProviders := map[string]bool{
		"finmind": true,
		"fugle":   true,
		"tej":     true,
		"fubon":   true,
	}
	if !allowedProviders[strings.ToLower(req.Provider)] {
		return http.StatusBadRequest, map[string]string{"error": "invalid provider"}
	}
	if len(req.APIKey) < 8 || len(req.APIKey) > 512 {
		return http.StatusBadRequest, map[string]string{"error": "api_key length invalid"}
	}

	key := strings.ToUpper(req.Provider) + "_API_KEY"
	_ = os.Setenv(key, req.APIKey)

	return http.StatusOK, map[string]any{
		"provider": req.Provider,
		"status":   "ok",
	}
}
