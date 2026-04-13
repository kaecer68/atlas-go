package orchestrator

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// Plugin defines the lifecycle interface for optional system extensions.
// Unifying PRISM, Swarm, JANUS, Spawning, and Phase3 under one contract
// makes cross-package dependencies visible to static analysis.
type Plugin interface {
	Name() string
	// Attach wires the plugin to core dependencies (registry, replay, policy).
	Attach(core *SystemCore)
	// ProcessRecommendations transforms recommendations during the simulation loop.
	ProcessRecommendations(regime domain.Regime, recs []domain.Recommendation) []domain.Recommendation
	// PostSimulation runs after the daily simulation completes.
	PostSimulation(quotes []domain.Quote, regime domain.Regime, asOf time.Time)
}
