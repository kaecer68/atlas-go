package strategy_ranker

import (
	"net/http"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/strategy_techniques"
)

// Handler serves the strategy ranking HTTP API.
type Handler struct {
	registry    *strategy_techniques.Registry
	snapshotDir string
}

// NewHandler creates a Handler backed by the given strategy registry.
// Metrics fields (Sharpe, max drawdown, etc.) will be omitted when no
// historical snapshot directory is configured.
func NewHandler(registry *strategy_techniques.Registry) *Handler {
	return &Handler{registry: registry}
}

// NewHandlerWithMetrics creates a Handler that also computes historical
// performance metrics from dated macro snapshots in snapshotDir.
func NewHandlerWithMetrics(registry *strategy_techniques.Registry, snapshotDir string) *Handler {
	return &Handler{registry: registry, snapshotDir: snapshotDir}
}

// RegisterRoutes attaches /api/strategy-ranker/* routes to mux.
func RegisterRoutes(mux *http.ServeMux, registry *strategy_techniques.Registry) {
	h := NewHandler(registry)
	mux.Handle("GET /api/strategy-ranker/rank", shared.Get(h.HandleRank))
}

// RegisterRoutesWithMetrics attaches /api/strategy-ranker/* routes to mux
// and enables historical performance metrics from dated macro snapshots.
func RegisterRoutesWithMetrics(mux *http.ServeMux, registry *strategy_techniques.Registry, snapshotDir string) {
	h := NewHandlerWithMetrics(registry, snapshotDir)
	mux.Handle("GET /api/strategy-ranker/rank", shared.Get(h.HandleRank))
}

// HandleRank returns active strategies ranked and tiered.
func (h *Handler) HandleRank(r *http.Request) (int, any) {
	if h.registry == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "strategy registry not initialized"}
	}

	var snapshots []marketdata.MacroDataSnapshot
	if h.snapshotDir != "" {
		loaded, _ := strategy_techniques.LoadSnapshotsFromDir(h.snapshotDir)
		snapshots = loaded
	}

	frames := h.registry.All()
	reports := make([]*StrategyReport, 0, len(frames))
	for _, f := range frames {
		if f.Status != strategy_techniques.StatusActive {
			continue
		}
		reports = append(reports, h.buildReport(f, snapshots))
	}
	if len(reports) == 0 {
		return http.StatusOK, []RankedReport{}
	}
	ranker := New()
	ranked := ranker.RankAndTier(reports)
	return http.StatusOK, ranked
}

func (h *Handler) buildReport(frame strategy_techniques.StrategyFrame, snapshots []marketdata.MacroDataSnapshot) *StrategyReport {
	report := &StrategyReport{
		StrategyID:   frame.ID,
		StrategyName: frame.Name,
		WinRate:      frame.HitRate,
		SampleDays:   frame.TotalTests,
	}

	if len(snapshots) == 0 {
		return report
	}

	eval := strategy_techniques.NewConditionEvaluator()
	strategyReturns, taiexReturns, totalTests := eval.EvaluateReturns(frame, snapshots, 1)
	if totalTests > 0 {
		report.SampleDays = totalTests
	}
	if len(strategyReturns) == 0 || len(strategyReturns) != len(taiexReturns) {
		return report
	}

	validated := NewValidator().Validate(frame.ID, frame.Name, strategyReturns, taiexReturns)
	if validated == nil {
		return report
	}

	// Preserve registry-level hit rate / sample count when the return-based
	// evaluation yields fewer valid samples (e.g., missing TAIEX on trigger days).
	validated.WinRate = frame.HitRate
	validated.SampleDays = totalTests
	return validated
}
