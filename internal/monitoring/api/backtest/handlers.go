package backtest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/kaecer68/atlas-go/internal/autobacktest"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

type Handlers struct {
	LedgerDir string
	svc       *service.BacktestService
	// store is the backend-aware ledger store backing GET /api/backtest/signals.
	// Dashboard wiring injects it (WithStore); nil keeps the directory-based
	// JSONL read for struct-literal construction in tests.
	store ledger.FullStore
	// storeErr records a wiring failure so the signals endpoint fails loudly
	// (500) instead of degrading to an empty JSONL read that answers 200 with
	// all-zero metrics and no error (2026-10 production defect).
	storeErr error
}

func NewHandlers(svc *service.BacktestService, ledgerDir string) *Handlers {
	return &Handlers{LedgerDir: ledgerDir, svc: svc}
}

// WithStore injects the backend-aware store used by the signals endpoint.
func (h *Handlers) WithStore(store ledger.FullStore) *Handlers {
	h.store = store
	h.storeErr = nil
	return h
}

// WithStoreError records that the backend-aware store could not be built, so
// the signals endpoint returns 500 rather than misleading zeros.
func (h *Handlers) WithStoreError(err error) *Handlers {
	h.store = nil
	h.storeErr = err
	return h
}

// signalEngine returns the engine backing GET /api/backtest/signals: the
// injected backend-aware store when wired, the legacy JSONL directory
// otherwise.
func (h *Handlers) signalEngine() (*autobacktest.SignalEngine, error) {
	if h.storeErr != nil {
		return nil, fmt.Errorf("backtest signals store unavailable: %w", h.storeErr)
	}
	if h.store != nil {
		return autobacktest.NewSignalEngineWithStore(h.store), nil
	}
	return autobacktest.NewSignalEngineFromDir(h.LedgerDir)
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("POST /api/backtest/run", shared.AdminPost(h.HandleBacktestRun))
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
	if !startDate.Before(endDate) && !startDate.Equal(endDate) {
		return http.StatusBadRequest, map[string]string{"error": "start date must be before or equal to end date"}
	}

	if err := h.svc.Start(startDate, endDate); err != nil {
		return http.StatusConflict, map[string]string{"error": "backtest already running"}
	}

	return http.StatusAccepted, map[string]any{
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

// BacktestSignalsResponse is the API response shape for GET /api/backtest/signals.
type BacktestSignalsResponse struct {
	ActiveSignals []string `json:"active_signals"`
	VaR95         float64  `json:"var_95"`
	VaR99         float64  `json:"var_99"`
	SharpeShort   float64  `json:"sharpe_short"`
	SharpeLong    float64  `json:"sharpe_long"`
	DrawdownPct   float64  `json:"drawdown_pct"`
}

func (h *Handlers) HandleBacktestSignals(r *http.Request) (int, any) {
	eng, err := h.signalEngine()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
	sigs, err := eng.Evaluate()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
	var active []string
	for _, s := range sigs.Active {
		active = append(active, string(s))
	}
	return http.StatusOK, BacktestSignalsResponse{
		ActiveSignals: active,
		VaR95:         sigs.VaR95,
		VaR99:         sigs.VaR99,
		SharpeShort:   sigs.SharpeShort,
		SharpeLong:    sigs.SharpeLong,
		DrawdownPct:   sigs.DrawdownPct,
	}
}
