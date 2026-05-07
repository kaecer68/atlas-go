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
	mux.HandleFunc("/api/backtest/run", h.HandleBacktestRun)
	mux.HandleFunc("/api/backtest/status", h.HandleBacktestStatus)
}

func (h *Handlers) HandleBacktestRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Start string `json:"start"`
		End   string `json:"end"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shared.WriteJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Start == "" || req.End == "" {
		shared.WriteJSONError(w, http.StatusBadRequest, "start and end dates required")
		return
	}
	startDate, err := time.Parse("2006-01-02", req.Start)
	if err != nil {
		shared.WriteJSONError(w, http.StatusBadRequest, "invalid start date format (YYYY-MM-DD)")
		return
	}
	endDate, err := time.Parse("2006-01-02", req.End)
	if err != nil {
		shared.WriteJSONError(w, http.StatusBadRequest, "invalid end date format (YYYY-MM-DD)")
		return
	}

	if err := h.svc.Start(startDate, endDate); err != nil {
		shared.WriteJSONError(w, http.StatusConflict, "backtest already running")
		return
	}

	shared.WriteJSON(w, http.StatusAccepted, map[string]interface{}{
		"running":      true,
		"check_status": "/api/backtest/status",
		"start":        req.Start,
		"end":          req.End,
	})
}

func (h *Handlers) HandleBacktestStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	status := h.svc.GetStatus()
	shared.WriteJSON(w, http.StatusOK, status)
}
