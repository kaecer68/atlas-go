package orchestrator

import (
	"context"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/macroflow"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// LayerRouter encapsulates layer-based agent routing logic.
// This decouples AgentRegistry layer filtering from executor dispatch,
// allowing routing strategy to be tested and evolved independently.
type LayerRouter interface {
	// GetContextAgents returns agents for regime inference (LayerContext only).
	GetContextAgents(registry domain.AgentRegistry) []domain.AgentSpec
	// GetSectorAgents returns agents for sector allocation (LayerSector only).
	GetSectorAgents(registry domain.AgentRegistry) []domain.AgentSpec
	// GetStyleAgents returns agents for style allocation (LayerStyle only).
	GetStyleAgents(registry domain.AgentRegistry) []domain.AgentSpec
	// GetControlAgents returns agents for control layer (LayerControl only).
	GetControlAgents(registry domain.AgentRegistry) []domain.AgentSpec
	// FilterByLayer returns agents matching the specified layer.
	FilterByLayer(registry domain.AgentRegistry, layer domain.AgentLayer) []domain.AgentSpec
}

// DefaultLayerRouter is the default layer-based router implementation.
type DefaultLayerRouter struct{}

func (DefaultLayerRouter) GetContextAgents(registry domain.AgentRegistry) []domain.AgentSpec {
	return FilterAgentsByLayer(registry.Agents, domain.LayerContext)
}

func (DefaultLayerRouter) GetSectorAgents(registry domain.AgentRegistry) []domain.AgentSpec {
	return FilterAgentsByLayer(registry.Agents, domain.LayerSector)
}

func (DefaultLayerRouter) GetStyleAgents(registry domain.AgentRegistry) []domain.AgentSpec {
	return FilterAgentsByLayer(registry.Agents, domain.LayerStyle)
}

func (DefaultLayerRouter) GetControlAgents(registry domain.AgentRegistry) []domain.AgentSpec {
	return FilterAgentsByLayer(registry.Agents, domain.LayerControl)
}

func (r DefaultLayerRouter) FilterByLayer(registry domain.AgentRegistry, layer domain.AgentLayer) []domain.AgentSpec {
	return FilterAgentsByLayer(registry.Agents, layer)
}

// FilterAgentsByLayer returns enabled agents matching the specified layer.
func FilterAgentsByLayer(agents []domain.AgentSpec, layer domain.AgentLayer) []domain.AgentSpec {
	result := make([]domain.AgentSpec, 0, len(agents))
	for _, a := range agents {
		if a.Enabled && a.Layer == layer {
			result = append(result, a)
		}
	}
	return result
}

// ExecutionContext holds all parameters needed to execute registry research.
type ExecutionContext struct {
	Registry                   domain.AgentRegistry
	Quotes                     []domain.Quote
	Overrides                  map[string]string
	Policy                     domain.ExecutionPolicy
	Plugins                    *PluginRegistry
	SessionID                  string
	WeightManager              *portfolio.DarwinianWeightManager
	Context                    context.Context            // request-level context for cancellation propagation
	NarrativeEvents            []narrative.NarrativeEvent // narrative events for regime evidence fusion
	ConvictionClampingCallback func([]portfolio.ConvictionClampingEvent)
	Scratchpad                 *Scratchpad     // optional reasoning trace recorder
	FactorSnapshot             *FactorSnapshot // pre-computed factor scores for executor consumption
	// Strategy injection points. nil → use the matching Default*Strategy implementation.
	// Embed a custom struct to override any phase without rewriting ExecuteWithContext.
	MutedAgentFilter         MutedAgentFilterStrategy
	RegimeInference          RegimeInferenceStrategy
	RecommendationCollection RecommendationCollectionStrategy
	MomentumCrashProtection  MomentumCrashProtectionStrategy
	WeightApplication        WeightApplicationStrategy
	MacroFlow                MacroFlowStrategy // nil → skip macro flow adjustment
	ControlLayer             ControlLayerStrategy
	ForecastBridge           ForecastBridgeStrategy         // nil → skip forecast bridge step
	DirectionalTradeLayer    DirectionalTradeWeightProvider // nil → signals not recorded
	// Period is the 7-period market classification (A4 P1). When non-nil,
	// regimeToRiskLevel uses PeriodToRiskLevel for finer-grained RiskLevel
	// than the legacy 3-regime mapping.
	Period *domain.MarketPeriod

	// PeriodDetector is the optional 7-period market cycle classifier (A4 P1).
	// When non-nil and MacroDataSnapshot has sufficient indicators, the period
	// is computed before macro flow adjustment and stored in ctx.Period.
	PeriodDetector *portfolio.PeriodDetector

	// MacroDataSnapshot is the latest macro snapshot for macro flow adjustment.
	// When non-nil, the MacroFlowStrategy can use it to compute allocation-tier deltas.
	MacroDataSnapshot *marketdata.MacroDataSnapshot
}

// ResearchResult holds all outputs from executing registry research.
type ResearchResult struct {
	Regime               domain.Regime
	RawRecommendations   []domain.Recommendation
	FinalRecommendations []domain.Recommendation
	GuardOutcomes        []domain.GuardOutcome
	ScreeningRejects     []domain.ScreeningReject
	DarwinianWeights     []*portfolio.DarwinianAgentWeight
	MacroFlowAdjustment  *macroflow.AdjustmentResult
	TraceSessionID       string // links to Scratchpad trace for decision provenance
}
