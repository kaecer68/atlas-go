package orchestrator

import (
	"context"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/forecast"
	"github.com/kaecer68/atlas-go/internal/macroflow"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// RegimeInferenceStrategy infers the market regime from agent context, quotes, and
// narrative events. The default implementation aggregates 4 evidence sources
// (Macro, Technical, Narrative, AgentSignal) into a single RiskOn/RiskOff/Neutral verdict.
type RegimeInferenceStrategy interface {
	InferRegime(ctx ExecutionContext, registry domain.AgentRegistry, quoteBySymbol map[string]domain.Quote) domain.Regime
}

// DefaultRegimeInferenceStrategy delegates to the package-level inferRegime function.
// It is stateless and safe for concurrent use.
type DefaultRegimeInferenceStrategy struct{}

func (DefaultRegimeInferenceStrategy) InferRegime(ctx ExecutionContext, registry domain.AgentRegistry, quoteBySymbol map[string]domain.Quote) domain.Regime {
	return inferRegime(registry, quoteBySymbol, ctx.Plugins, ctx.Overrides, ctx.NarrativeEvents, ctx.Scratchpad, ctx.SessionID)
}

// RecommendationCollectionStrategy walks the enabled Sector/Style/Superinvestor agents,
// runs each through screener + factor quality gate, and emits raw recommendations
// plus screening rejects.
type RecommendationCollectionStrategy interface {
	Collect(ctx context.Context, registry domain.AgentRegistry, quoteBySymbol map[string]domain.Quote, regime domain.Regime, plugins *PluginRegistry, overrides map[string]string, narrativeEvents []narrative.NarrativeEvent, sessionID string, scratchpad *Scratchpad) ([]domain.Recommendation, []domain.ScreeningReject)
}

// DefaultRecommendationCollectionStrategy delegates to the package-level collectRecommendations function.
type DefaultRecommendationCollectionStrategy struct{}

func (DefaultRecommendationCollectionStrategy) Collect(ctx context.Context, registry domain.AgentRegistry, quoteBySymbol map[string]domain.Quote, regime domain.Regime, plugins *PluginRegistry, overrides map[string]string, narrativeEvents []narrative.NarrativeEvent, sessionID string, scratchpad *Scratchpad) ([]domain.Recommendation, []domain.ScreeningReject) {
	return collectRecommendations(ctx, registry, quoteBySymbol, plugins, overrides, regime, narrativeEvents, sessionID, scratchpad)
}

// MomentumCrashProtectionStrategy zeroes the momentum factor when VIX exceeds the
// configured threshold, redistributing the freed weight to Value/Quality/Agent.
// No-op when policy.MomentumCrashProtection is false.
type MomentumCrashProtectionStrategy interface {
	Apply(recs []domain.Recommendation, quotes map[string]domain.Quote, policy domain.ExecutionPolicy) []domain.Recommendation
}

// DefaultMomentumCrashProtectionStrategy delegates to the package-level applyMomentumCrashProtection function.
type DefaultMomentumCrashProtectionStrategy struct{}

func (DefaultMomentumCrashProtectionStrategy) Apply(recs []domain.Recommendation, quotes map[string]domain.Quote, policy domain.ExecutionPolicy) []domain.Recommendation {
	if !policy.MomentumCrashProtection {
		return recs
	}
	return applyMomentumCrashProtection(recs, quotes)
}

// ControlLayerStrategy runs the CRO + CIO + superinvestor control agents against
// the recommendations and returns the filtered set plus per-guard outcomes.
// macroAdjustment is the macroflow adjustment produced upstream (may be nil when
// no MacroFlow strategy was wired); when non-nil, recs are uniformly scaled by
// the net conservative bias before the per-agent guard loop runs.
type ControlLayerStrategy interface {
	ApplyControl(registry domain.AgentRegistry, plugins *PluginRegistry, recs []domain.Recommendation, policy domain.ExecutionPolicy, regime domain.Regime, scratchpad *Scratchpad, sessionID string, macroAdjustment *macroflow.AdjustmentResult) ([]domain.Recommendation, []domain.GuardOutcome)
}

// DefaultControlLayerStrategy delegates to the package-level applyControlLayerWithOutcomes function.
type DefaultControlLayerStrategy struct{}

func (DefaultControlLayerStrategy) ApplyControl(registry domain.AgentRegistry, plugins *PluginRegistry, recs []domain.Recommendation, policy domain.ExecutionPolicy, regime domain.Regime, scratchpad *Scratchpad, sessionID string, macroAdjustment *macroflow.AdjustmentResult) ([]domain.Recommendation, []domain.GuardOutcome) {
	return applyControlLayerWithOutcomes(registry, plugins, recs, policy, regime, scratchpad, sessionID, macroAdjustment)
}

// WeightApplicationStrategy applies Darwinian (Atlas-GIC style) weight multipliers
// in the [0.3, 2.5] range to agent recommendations. No-op when manager is nil.
type WeightApplicationStrategy interface {
	ApplyWeights(raw []domain.Recommendation, manager *portfolio.DarwinianWeightManager, callback func([]portfolio.ConvictionClampingEvent)) ([]domain.Recommendation, []*portfolio.DarwinianAgentWeight)
}

// DefaultWeightApplicationStrategy delegates to DarwinianWeightManager.ApplyDarwinianWeightsWithEvents
// and forwards conviction-clamping events to the optional callback.
type DefaultWeightApplicationStrategy struct{}

func (DefaultWeightApplicationStrategy) ApplyWeights(raw []domain.Recommendation, manager *portfolio.DarwinianWeightManager, callback func([]portfolio.ConvictionClampingEvent)) ([]domain.Recommendation, []*portfolio.DarwinianAgentWeight) {
	if manager == nil {
		return raw, nil
	}
	weighted, events := manager.ApplyDarwinianWeightsWithEvents(raw)
	if len(events) > 0 && callback != nil {
		callback(events)
	}
	return weighted, manager.GetAllAgentWeightData()
}

// MutedAgentFilterStrategy excludes agents currently in muted/degraded health state
// from the registry passed to downstream phases.
type MutedAgentFilterStrategy interface {
	Filter(registry domain.AgentRegistry, plugins *PluginRegistry) domain.AgentRegistry
}

// DefaultMutedAgentFilterStrategy delegates to the package-level filterMutedAgents function.
type DefaultMutedAgentFilterStrategy struct{}

func (DefaultMutedAgentFilterStrategy) Filter(registry domain.AgentRegistry, plugins *PluginRegistry) domain.AgentRegistry {
	return filterMutedAgents(registry, plugins)
}

// MacroFlowStrategy computes macro regime–based factor weight adjustments
// (defensive/aggressive/cash allocation tier deltas) from market data + risk level.
// Runs after WeightApplication and before ControlLayer in the pipeline.
type MacroFlowStrategy interface {
	// ComputeAdjustment returns the macro flow adjustment result given a snapshot
	// and a risk level, or nil when data is unavailable / stale.
	ComputeAdjustment(snapshot *marketdata.MacroDataSnapshot, level macroflow.RiskLevel) *macroflow.AdjustmentResult
}

// DefaultMacroFlowStrategy delegates to macroflow.Engine.Compute.
type DefaultMacroFlowStrategy struct {
	engine *macroflow.Engine
}

func (s DefaultMacroFlowStrategy) ComputeAdjustment(snapshot *marketdata.MacroDataSnapshot, level macroflow.RiskLevel) *macroflow.AdjustmentResult {
	if s.engine == nil {
		return nil
	}
	return s.engine.Compute(snapshot, level)
}

// ForecastBridgeStrategy runs per-symbol forecast predictions and returns trade signals.
// nil → skip forecast bridge entirely in the pipeline.
type ForecastBridgeStrategy interface {
	PredictAll(symbols []string) ([]forecast.TradeSignal, error)
}

// DirectionalTradeWeightProvider returns a weight multiplier for a symbol.
// Defaults to 1.0 when no trade signal exists.
type DirectionalTradeWeightProvider interface {
	WeightFor(symbol string) float64
	ApplySignal(signal forecast.TradeSignal)
	Reset()
}
