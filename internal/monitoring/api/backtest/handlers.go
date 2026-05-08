package backtest

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

type Handlers struct {
	LedgerDir string
	svc       *service.BacktestService
}

func NewHandlers(svc *service.BacktestService, ledgerDir string) *Handlers {
	return &Handlers{LedgerDir: ledgerDir, svc: svc}
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("POST /api/backtest/run", shared.Post(h.HandleBacktestRun))
	mux.Handle("GET /api/backtest/status", shared.Get(h.HandleBacktestStatus))
	mux.Handle("GET /api/backtest/snapshots", shared.Get(h.HandleBacktestSnapshots))
	mux.Handle("GET /api/backtest/signals", shared.Get(h.HandleBacktestSignals))
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

func (h *Handlers) HandleBacktestSnapshots(r *http.Request) (int, any) {
	vals := r.URL.Query()
	days := 20
	if d := vals.Get("days"); d != "" {
		if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
			days = parsed
		}
	}
	_ = days
	return http.StatusOK, map[string]any{
		"snapshots": []any{},
		"count":     0,
		"note":      "autobacktest package pending implementation",
	}
}

func (h *Handlers) HandleBacktestSignals(r *http.Request) (int, any) {
	return http.StatusOK, map[string]any{
		"active_signals": []string{},
		"var_95":         0,
		"var_99":         0,
		"sharpe_short":   0,
		"sharpe_long":    0,
		"drawdown_pct":   0,
		"note":           "autobacktest package pending implementation",
	}
}
