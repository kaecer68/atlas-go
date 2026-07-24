package strategy_ranker

import (
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/strategy_techniques"
)

// Handler serves the strategy ranking HTTP API.
type Handler struct {
	registry *strategy_techniques.Registry
}

// NewHandler creates a Handler backed by the given strategy registry.
func NewHandler(registry *strategy_techniques.Registry) *Handler {
	return &Handler{registry: registry}
}

// RegisterRoutes attaches /api/strategy-ranker/* routes to mux.
func RegisterRoutes(mux *http.ServeMux, registry *strategy_techniques.Registry) {
	h := NewHandler(registry)
	mux.Handle("GET /api/strategy-ranker/rank", shared.Get(h.HandleRank))
}

// HandleRank returns active strategies ranked and tiered.
func (h *Handler) HandleRank(r *http.Request) (int, any) {
	if h.registry == nil {
		return http.StatusServiceUnavailable, map[string]string{"error": "strategy registry not initialized"}
	}
	frames := h.registry.All()
	reports := make([]*StrategyReport, 0, len(frames))
	for _, f := range frames {
		if f.Status != strategy_techniques.StatusActive {
			continue
		}
		reports = append(reports, &StrategyReport{
			StrategyID:   f.ID,
			StrategyName: f.Name,
			WinRate:      f.HitRate,
			SampleDays:   f.TotalTests,
		})
	}
	if len(reports) == 0 {
		return http.StatusOK, []RankedReport{}
	}
	ranker := New()
	ranked := ranker.RankAndTier(reports)
	return http.StatusOK, ranked
}
