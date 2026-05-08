package backtest

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/kaecer68/atlas-go/internal/autobacktest"
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

	hist := autobacktest.NewHistory(h.LedgerDir)
	snapshots, err := hist.LatestN(days)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
	if snapshots == nil {
		snapshots = []autobacktest.AutoSnapshot{}
	}
	return http.StatusOK, map[string]any{
		"snapshots": snapshots,
		"count":     len(snapshots),
	}
}

func (h *Handlers) HandleBacktestSignals(r *http.Request) (int, any) {
	eng := autobacktest.NewSignalEngine(h.LedgerDir)
	sigs, err := eng.Evaluate()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
	var active []string
	for _, s := range sigs.Active {
		active = append(active, string(s))
	}
	return http.StatusOK, map[string]any{
		"active_signals": active,
		"var_95":         sigs.VaR95,
		"var_99":         sigs.VaR99,
		"sharpe_short":   sigs.SharpeShort,
		"sharpe_long":    sigs.SharpeLong,
		"drawdown_pct":   sigs.DrawdownPct,
	}
}
