package backtest

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

type Handlers struct {
	svc *service.BacktestService
}

func NewHandlers(svc *service.BacktestService) *Handlers {
	return &Handlers{svc: svc}
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("POST /api/backtest/run", shared.Post(h.HandleBacktestRun))
	mux.Handle("GET /api/backtest/status", shared.Get(h.HandleBacktestStatus))
}

func (h *Handlers) HandleBacktestRun(r *http.Request) (int, any) {
	var req struct {
		Start string `json:"start"`
		End   string `json:"end"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid json"}
	}
	if req.Start == "" || req.End == "" {
		return http.StatusBadRequest, map[string]string{"error": "start and end dates required"}
	}
	startDate, err := time.Parse("2006-01-02", req.Start)
	if err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid start date format (YYYY-MM-DD)"}
	}
	endDate, err := time.Parse("2006-01-02", req.End)
	if err != nil {
		return http.StatusBadRequest, map[string]string{"error": "invalid end date format (YYYY-MM-DD)"}
	}

	if err := h.svc.Start(startDate, endDate); err != nil {
		return http.StatusConflict, map[string]string{"error": "backtest already running"}
	}

	return http.StatusAccepted, map[string]interface{}{
		"running":      true,
		"check_status": "/api/backtest/status",
		"start":        req.Start,
		"end":          req.End,
	}
}

func (h *Handlers) HandleBacktestStatus(r *http.Request) (int, any) {
	return http.StatusOK, h.svc.GetStatus()
}
