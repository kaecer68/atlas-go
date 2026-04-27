package backtest

import (
	"encoding/json"
	"net/http"
	"time"

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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (h *Handlers) HandleBacktestRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Start string `json:"start"`
		End   string `json:"end"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Start == "" || req.End == "" {
		writeJSONError(w, http.StatusBadRequest, "start and end dates required")
		return
	}
	startDate, err := time.Parse("2006-01-02", req.Start)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid start date format (YYYY-MM-DD)")
		return
	}
	endDate, err := time.Parse("2006-01-02", req.End)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid end date format (YYYY-MM-DD)")
		return
	}

	if err := h.svc.Start(startDate, endDate); err != nil {
		writeJSONError(w, http.StatusConflict, "backtest already running")
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"running":      true,
		"check_status": "/api/backtest/status",
		"start":        req.Start,
		"end":          req.End,
	})
}

func (h *Handlers) HandleBacktestStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	status := h.svc.GetStatus()
	writeJSON(w, http.StatusOK, status)
}
