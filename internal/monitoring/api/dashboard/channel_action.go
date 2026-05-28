package dashboard

import (
	"encoding/json"
	"net/http"
	"strings"
)

// HandleChannelAction handles channel enable/disable toggle and trigger actions.
func (h *Handlers) HandleChannelAction(r *http.Request) (int, any) {
	path := strings.TrimPrefix(r.URL.Path, "/api/dashboard/channels/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return http.StatusBadRequest, map[string]string{"error": "invalid path"}
	}
	channelID := parts[0]
	action := parts[1]

	switch action {
	case "trigger":
		return http.StatusOK, map[string]any{
			"channel_id": channelID,
			"action":     "trigger",
			"status":     "ok",
			"note":       "next poll will reflect fresh status",
		}
	case "toggle":
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return http.StatusBadRequest, map[string]string{"error": "invalid body"}
		}
		h.setChannelEnabled(channelID, req.Enabled)
		return http.StatusOK, map[string]any{
			"channel_id": channelID,
			"enabled":    req.Enabled,
			"status":     "ok",
		}
	default:
		return http.StatusBadRequest, map[string]string{"error": "unknown action"}
	}
}
