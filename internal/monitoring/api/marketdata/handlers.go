package marketdata

import (
	"fmt"
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

// Handlers provides market-data-related API endpoints.
type Handlers struct {
	WorkDir string
}

// NewHandlers creates a new marketdata Handlers.
func NewHandlers(workDir string) *Handlers {
	return &Handlers{WorkDir: workDir}
}

// RegisterRoutes mounts marketdata endpoints on the given mux.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/dashboard/retail-sentiment", h.HandleRetailSentiment)
	mux.HandleFunc("/api/dashboard/capital-phase", h.HandleCapitalPhase)
}

// HandleRetailSentiment handles GET /api/dashboard/retail-sentiment.
func (h *Handlers) HandleRetailSentiment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	provider := newRetailSentimentProvider(h.WorkDir)
	snap, err := provider.FetchSnapshot(r.Context())
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("fetch retail sentiment: %v", err))
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"margin_balance":    snap.MarginBalance,
		"margin_change_pct": snap.MarginChangePct,
		"day_trading_ratio": snap.DayTradingRatio,
		"margin_percentile": snap.MarginPercentile,
		"sentiment_score":   snap.CalculateSentimentScore(),
		"extreme_reading":   snap.ExtremeReading(),
		"timestamp":         snap.Timestamp,
	})
}

// HandleCapitalPhase handles GET /api/dashboard/capital-phase.
func (h *Handlers) HandleCapitalPhase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	config := domain.DefaultCapitalPhaseConfig()
	snap := domain.CapitalSnapshot{
		Phase:           config.CurrentPhase,
		PhaseStartDate:  config.PhaseStartDate,
		DaysInPhase:     int(time.Since(config.PhaseStartDate).Hours() / 24),
		TotalCapital:    0,
		DeployedCapital: 0,
		ReserveCash:     0,
		RollingSharpe:   0,
		MaxDrawdown:     0,
		CanAdvance:      false,
		AdvanceReason:   "no live trading data available",
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"phase":            snap.Phase,
		"phase_start_date": snap.PhaseStartDate,
		"days_in_phase":    snap.DaysInPhase,
		"rolling_sharpe":   snap.RollingSharpe,
		"max_drawdown":     snap.MaxDrawdown,
		"can_advance":      snap.CanAdvance,
		"advance_reason":   snap.AdvanceReason,
		"capital_limit":    config.CapitalLimits[string(snap.Phase)],
		"total_capital":    snap.TotalCapital,
		"deployed_capital": snap.DeployedCapital,
		"reserve_cash":     snap.ReserveCash,
		"is_simulated":     true,
		"note":             "no live trading data — capital phase requires orchestrator.System in live mode",
	})
}
