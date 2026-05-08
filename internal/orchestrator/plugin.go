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
	ProcessRecommendations(regime domain.Regime, recs []domain.Recommendation) []domain.Recommendation
	PostSimulation(quotes []domain.Quote, regime domain.Regime, asOf time.Time)
}

// ControllerAware is implemented by plugins that accept a Phase3Controller.
// This allows Phase3Controller to be injected without type-asserting on concrete plugin types.
type ControllerAware interface {
	SetController(ctrl *Phase3Controller)
}
