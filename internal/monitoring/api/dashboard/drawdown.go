package dashboard

import (
	"net/http"
	"time"
)

// DrawdownResponse is the API response shape for GET /api/dashboard/drawdown.
type DrawdownResponse struct {
	Status      string    `json:"status,omitempty"`
	Message     string    `json:"message,omitempty"`
	Generated   string    `json:"generated"`
	MaxDrawdown float64   `json:"max_drawdown,omitempty"`
	VaR95       float64   `json:"var_95,omitempty"`
	WorstPath   []float64 `json:"worst_path,omitempty"`
}

// HandleDrawdown returns the latest drawdown simulation result.
func (h *Handlers) HandleDrawdown(r *http.Request) (int, any) {
	result := h.DrawdownProvider.GetLatestDrawdown()
	if result == nil {
		return http.StatusOK, DrawdownResponse{
			Status:    "not_available",
			Message:   "風險引擎尚未完成第一輪模擬；drawdown 數據將在 stress_test_daily 排程首次完成後自動產生（通常需 ≥1 個交易日）",
			Generated: time.Now().Format(time.RFC3339),
		}
	}
	return http.StatusOK, DrawdownResponse{
		MaxDrawdown: result.MaxDrawdown,
		VaR95:       result.VaR95,
		WorstPath:   result.WorstPath,
		Generated:   time.Now().Format(time.RFC3339),
	}
}
