package orchestrator

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// CoreServices defines the minimal interface that Plugins need from SystemCore.
// This allows external packages to implement Plugins without depending on *SystemCore.
type CoreServices interface {
	GetReplay() *replay.Dataset
	GetRegistry() domain.AgentRegistry
	GetPolicy() baseline.Policy
	GetLastOutcomes() []domain.RecommendationOutcome
}

// Plugin defines the lifecycle interface for optional system extensions.
// Unifying PRISM, Swarm, JANUS, Spawning, and Phase3 under one contract
// makes cross-package dependencies visible to static analysis.
type Plugin interface {
	Name() string
	// Attach wires the plugin to core dependencies via CoreServices interface.
	// This allows Plugins to be implemented in external packages.
	Attach(core CoreServices)
	// ProcessRecommendations transforms recommendations during the simulation loop.
	ProcessRecommendations(regime domain.Regime, recs []domain.Recommendation) []domain.Recommendation
	// PostSimulation runs after the daily simulation completes.
	PostSimulation(quotes []domain.Quote, regime domain.Regime, asOf time.Time)
}
