package dashboard

import (
	"net/http"
	"time"
)

// HandleDrawdown returns the latest drawdown simulation result.
func (h *Handlers) HandleDrawdown(r *http.Request) (int, any) {
	result := h.DrawdownProvider.GetLatestDrawdown()
	if result == nil {
		return http.StatusOK, map[string]any{
			"status":    "not_available",
			"message":   "no drawdown simulation available yet",
			"generated": time.Now().Format(time.RFC3339),
		}
	}
	return http.StatusOK, map[string]any{
		"max_drawdown": result.MaxDrawdown,
		"var_95":       result.VaR95,
		"worst_path":   result.WorstPath,
		"generated":    time.Now().Format(time.RFC3339),
	}
}
