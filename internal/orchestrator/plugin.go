package orchestrator

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// ServiceRegistry provides plugins with access to core system services.
// Each plugin declares its dependencies through the Attach method, and the
// PluginHost injects the registry. This follows the Dependency Inversion Principle —
// plugins depend on this interface, not on *SystemCore.
type ServiceRegistry interface {
	Replay() *replay.Dataset
	GetRegistry() domain.AgentRegistry
	GetPolicy() baseline.Policy
	GetLastOutcomes() []domain.RecommendationOutcome
	Ledger() ledger.OutcomeStore
	EventBus() *eventbus.ChannelEventBus
}

// Plugin defines the lifecycle interface for optional system extensions.
type Plugin interface {
	Name() string
	Attach(registry ServiceRegistry)
	// ProcessRecommendations transforms recommendations during the simulation loop.
	ProcessRecommendations(regime domain.Regime, recs []domain.Recommendation) []domain.Recommendation
	// PostSimulation runs after the daily simulation completes.
	PostSimulation(quotes []domain.Quote, regime domain.Regime, asOf time.Time)
}
