package apiswarm

import (
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/swarm"
)

// Handlers exposes swarm simulation state via the dashboard API.
type Handlers struct {
	Svc *service.SwarmService
}

// NewHandlers creates swarm API handlers.
func NewHandlers(svc *service.SwarmService) *Handlers {
	return &Handlers{Svc: svc}
}

// RegisterRoutes registers swarm routes on the given mux.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard/swarm-status", shared.Get(h.HandleStatus))
	mux.Handle("GET /api/dashboard/swarm-consensus", shared.Get(h.HandleConsensus))
	mux.Handle("GET /api/dashboard/swarm-anomalies", shared.Get(h.HandleAnomalies))
	mux.Handle("GET /api/dashboard/swarm-scenarios", shared.Get(h.HandleScenarios))
	mux.Handle("GET /api/dashboard/swarm-strategies", shared.Get(h.HandleStrategies))
}

// HandleStatus returns a summary of the latest swarm simulation.
func (h *Handlers) HandleStatus(r *http.Request) (int, any) {
	status, err := h.Svc.LoadStatus()
	if err != nil {
		return http.StatusNotFound, map[string]string{"error": "no swarm data available"}
	}
	return http.StatusOK, status
}

// HandleConsensus returns per-symbol consensus breakdown.
func (h *Handlers) HandleConsensus(r *http.Request) (int, any) {
	entries, err := h.Svc.LoadConsensus()
	if err != nil {
		return http.StatusNotFound, map[string]string{"error": "no swarm consensus data"}
	}
	if len(entries) == 0 {
		return http.StatusOK, []service.ConsensusEntry{}
	}
	return http.StatusOK, entries
}

// HandleAnomalies returns anomalies from the latest swarm simulation.
func (h *Handlers) HandleAnomalies(r *http.Request) (int, any) {
	anomalies, err := h.Svc.LoadAnomalies()
	if err != nil {
		return http.StatusNotFound, map[string]string{"error": "no swarm anomaly data"}
	}
	if anomalies == nil {
		anomalies = []swarm.Anomaly{}
	}
	return http.StatusOK, anomalies
}

// HandleScenarios returns scenario parameters from the latest swarm simulation.
func (h *Handlers) HandleScenarios(r *http.Request) (int, any) {
	scenarios, err := h.Svc.LoadScenarios()
	if err != nil {
		return http.StatusNotFound, map[string]string{"error": "no swarm scenario data"}
	}
	if scenarios == nil {
		scenarios = []swarm.ScenarioSnapshot{}
	}
	return http.StatusOK, scenarios
}

// HandleStrategies returns top learning strategies from the MetaLearner.
func (h *Handlers) HandleStrategies(r *http.Request) (int, any) {
	strategies, err := h.Svc.LoadRecommendedStrategies()
	if err != nil {
		return http.StatusNotFound, map[string]string{"error": "no strategy data available"}
	}
	if len(strategies) == 0 {
		return http.StatusOK, []service.StrategySummary{}
	}
	return http.StatusOK, strategies
}
