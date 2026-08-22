package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/macroflow"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/ml"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// ExecuteWithContext executes registry research using a unified context.
func ExecuteWithContext(ctx ExecutionContext) ResearchResult {
	if ctx.Plugins == nil {
		ctx.Plugins = NewPluginRegistry()
	}

	// When use_ml_scoring is enabled and no ML scorer is already wired,
	// create a default ML scorer with OLS. The caller is responsible for
	// training via MLScorer.Train() before execution — typically done at
	// system initialization with historical replay data.
	if cfg := config.GetParametersConfig(); cfg != nil && cfg.Orchestrator.UseMLScoring.Value {
		if ctx.Plugins.mlScorer == nil {
			ctx.Plugins.WithMLScorer(NewMLScorer(ml.NewOLS()))
		}
	}
	if ctx.Policy == (domain.ExecutionPolicy{}) {
		ctx.Policy = DefaultExecutionPolicy()
	}
	if ctx.Context == nil {
		ctx.Context = context.Background()
	}

	if ctx.Scratchpad != nil {
		ctx.Scratchpad.Record(ReasoningTrace{
			SessionID:  ctx.SessionID,
			Timestamp:  time.Now().UTC(),
			Phase:      PhaseSystem,
			Step:       0,
			Component:  "orchestrator",
			Action:     "execute_start",
			Reasoning:  "Starting registry research execution",
			Data:       map[string]any{"registry_version": ctx.Registry.Version, "quote_count": len(ctx.Quotes)},
			Confidence: -1,
		})
	}

	// Resolve strategy defaults. Each phase can be overridden by setting the
	// corresponding field on ExecutionContext before calling ExecuteWithContext.
	if ctx.MutedAgentFilter == nil {
		ctx.MutedAgentFilter = DefaultMutedAgentFilterStrategy{}
	}
	if ctx.RegimeInference == nil {
		ctx.RegimeInference = DefaultRegimeInferenceStrategy{}
	}
	if ctx.RecommendationCollection == nil {
		ctx.RecommendationCollection = DefaultRecommendationCollectionStrategy{}
	}
	if ctx.MomentumCrashProtection == nil {
		ctx.MomentumCrashProtection = DefaultMomentumCrashProtectionStrategy{}
	}
	if ctx.WeightApplication == nil {
		ctx.WeightApplication = DefaultWeightApplicationStrategy{}
	}
	if ctx.ControlLayer == nil {
		ctx.ControlLayer = DefaultControlLayerStrategy{}
	}

	quoteBySymbol := make(map[string]domain.Quote, len(ctx.Quotes))
	for _, quote := range ctx.Quotes {
		quoteBySymbol[quote.Symbol] = quote
	}

	registry := ctx.MutedAgentFilter.Filter(ctx.Registry, ctx.Plugins)

	// ── A4 P1: Period detection from MacroDataSnapshot ──
	// Runs before recommendation collection so the CharterMode period filter
	// (ctx.PeriodStrategyFilter) and the macroflow risk level can both consume
	// the 7-period classification. No-op when no PeriodDetector is wired
	// (Phase A behavior preserved).
	if ctx.PeriodDetector != nil && ctx.MacroDataSnapshot != nil {
		ind := snapshotToPeriodIndicators(*ctx.MacroDataSnapshot)
		period := ctx.PeriodDetector.DetectPeriod(ind)
		ctx.Period = &period
	}

	regime := ctx.RegimeInference.InferRegime(ctx, registry, quoteBySymbol)
	raw, rejects := ctx.RecommendationCollection.Collect(ctx.Context, registry, quoteBySymbol, regime, ctx.Plugins, ctx.Overrides, ctx.NarrativeEvents, ctx.SessionID, ctx.Scratchpad)

	// ── CharterMode period→strategy gate (Phase C2) ──
	// Drops recommendations whose skill maps to a charter strategy category
	// that is not allowed in the detected period. Runs before momentum-crash
	// protection / weighting so downstream conviction work is not wasted on
	// gated recs. nil filter or nil period → pass-through (Phase A).
	if ctx.PeriodStrategyFilter != nil && ctx.Period != nil {
		raw = ctx.PeriodStrategyFilter(*ctx.Period, raw, registry)
	}

	// ── MacroFlow: 宏觀風險調整（憲章 §2 第〇層→第一層）──
	// Computed post-regime inference so risk-level→adjustment mapping matches
	// the detected regime. ApplyControl consumes the result later in the pipeline.

	var macroFlowResult *macroflow.AdjustmentResult
	if ctx.MacroFlow != nil && ctx.MacroDataSnapshot != nil {
		macroFlowResult = ctx.MacroFlow.ComputeAdjustment(ctx.MacroDataSnapshot, regimeToRiskLevel(regime, ctx.Period))
	}

	if macroFlowResult != nil && ctx.Scratchpad != nil {
		ctx.Scratchpad.Record(ReasoningTrace{
			SessionID: ctx.SessionID,
			Timestamp: time.Now().UTC(),
			Phase:     PhaseMacroFlow,
			Step:      6,
			Component: "macroflow",
			Action:    "macro_flow.applied",
			Reasoning: fmt.Sprintf("macro_flow applied: risk_level=%s defensive=%+.1f%% aggressive=%+.1f%% cash=%+.1f%%",
				macroFlowResult.RiskLevel, macroFlowResult.Adjustment.Defensive,
				macroFlowResult.Adjustment.Aggressive, macroFlowResult.Adjustment.Cash),
			Data: map[string]any{
				"risk_level": string(macroFlowResult.RiskLevel),
				"is_stress":  macroFlowResult.IsStress,
				"defensive":  macroFlowResult.Adjustment.Defensive,
				"aggressive": macroFlowResult.Adjustment.Aggressive,
				"cash":       macroFlowResult.Adjustment.Cash,
				"reasoning":  macroFlowResult.Reasoning,
			},
			Confidence: -1,
			// B5 P1: causal chain layer tracing
			LayerID:       "layer_0", // 全球資金總開關
			LayerParentID: "layer_root",
		})
	}

	// Stage gating: skip momentum crash protection during RISK_OFF regime.
	if regime != domain.RegimeRiskOff {
		raw = ctx.MomentumCrashProtection.Apply(raw, quoteBySymbol, ctx.Policy)
	}

	controlInput, weightData := ctx.WeightApplication.ApplyWeights(raw, ctx.WeightManager, ctx.ConvictionClampingCallback)

	if ctx.ForecastBridge != nil && ctx.DirectionalTradeLayer != nil {
		symbols := make([]string, 0, len(ctx.Quotes))
		for _, q := range ctx.Quotes {
			symbols = append(symbols, q.Symbol)
		}
		if signals, err := ctx.ForecastBridge.PredictAll(symbols); err == nil {
			for _, sig := range signals {
				ctx.DirectionalTradeLayer.ApplySignal(sig)
			}
		}
	}

	final, guardOutcomes := ctx.ControlLayer.ApplyControl(registry, ctx.Plugins, controlInput, ctx.Policy, regime, ctx.Scratchpad, ctx.SessionID, macroFlowResult)
	return ResearchResult{
		MacroFlowAdjustment:  macroFlowResult,
		Regime:               regime,
		RawRecommendations:   raw,
		FinalRecommendations: final,
		GuardOutcomes:        guardOutcomes,
		ScreeningRejects:     rejects,
		DarwinianWeights:     weightData,
		Period:               ctx.Period,
		TraceSessionID:       ctx.SessionID,
	}
}

// regimeToRiskLevel maps a regime to macroflow risk level, using the 7-period
// classification (via PeriodToRiskLevel) when available. Falls back to the
// legacy 3-regime mapping when period is unknown.
func regimeToRiskLevel(regime domain.Regime, period *domain.MarketPeriod) macroflow.RiskLevel {
	if period != nil {
		return portfolio.PeriodToRiskLevel(*period)
	}
	switch regime {
	case domain.RegimeRiskOff:
		return macroflow.RiskRed
	case domain.RegimeRiskOn, domain.RegimeNeutral:
		return macroflow.RiskYellow
	default:
		return macroflow.RiskYellow
	}
}

func ExecuteRegistryResearch(registry domain.AgentRegistry, quotes []domain.Quote, overrides map[string]string) (domain.Regime, []domain.Recommendation) {
	result := ExecuteWithContext(ExecutionContext{
		Registry:  registry,
		Quotes:    quotes,
		Overrides: overrides,
	})
	return result.Regime, result.FinalRecommendations
}

func ExecuteRegistryResearchDetailed(registry domain.AgentRegistry, quotes []domain.Quote, overrides map[string]string) (domain.Regime, []domain.Recommendation, []domain.Recommendation) {
	result := ExecuteWithContext(ExecutionContext{
		Registry:  registry,
		Quotes:    quotes,
		Overrides: overrides,
	})
	return result.Regime, result.RawRecommendations, result.FinalRecommendations
}

func ExecuteRegistryResearchDetailedWithPolicy(registry domain.AgentRegistry, quotes []domain.Quote, overrides map[string]string, policy domain.ExecutionPolicy) (domain.Regime, []domain.Recommendation, []domain.Recommendation) {
	result := ExecuteWithContext(ExecutionContext{
		Registry:  registry,
		Quotes:    quotes,
		Overrides: overrides,
		Policy:    policy,
	})
	return result.Regime, result.RawRecommendations, result.FinalRecommendations
}

func ExecuteRegistryResearchDetailedWithPolicyAndGuards(registry domain.AgentRegistry, quotes []domain.Quote, overrides map[string]string, policy domain.ExecutionPolicy) (domain.Regime, []domain.Recommendation, []domain.Recommendation, []domain.GuardOutcome) {
	result := ExecuteWithContext(ExecutionContext{
		Registry:  registry,
		Quotes:    quotes,
		Overrides: overrides,
		Policy:    policy,
	})
	return result.Regime, result.RawRecommendations, result.FinalRecommendations, result.GuardOutcomes
}

func ExecuteRegistryResearchDetailedWithPolicyAndGuardsAndPlugins(
	registry domain.AgentRegistry,
	quotes []domain.Quote,
	overrides map[string]string,
	policy domain.ExecutionPolicy,
	plugins *PluginRegistry,
) (domain.Regime, []domain.Recommendation, []domain.Recommendation, []domain.GuardOutcome) {
	result := ExecuteWithContext(ExecutionContext{
		Registry:  registry,
		Quotes:    quotes,
		Overrides: overrides,
		Policy:    policy,
		Plugins:   plugins,
	})
	return result.Regime, result.RawRecommendations, result.FinalRecommendations, result.GuardOutcomes
}

// snapshotToPeriodIndicators converts a marketdata.MacroDataSnapshot to the
// portfolio.PeriodIndicators format expected by PeriodDetector.DetectPeriod.
//
// Currently maps 12 of ~30 indicator fields. The 12 mapped fields cover the
// most critical single-day signals. Fields requiring historical time-series
// (MA50/MA20/5-day avg/consecutive days/futures delta/TWD change) are left
// at zero — the PeriodDetector treats zero-valued indicators as "unavailable".
// Full coverage needs a MultiDayPeriodSnapshot (planned as follow-up PR-2).
func snapshotToPeriodIndicators(snapshot marketdata.MacroDataSnapshot) portfolio.PeriodIndicators {
	return portfolio.PeriodIndicators{
		VIX:                    snapshot.VIX.Value,
		DXY:                    snapshot.DXY.Value,
		US10Y:                  snapshot.US10Y.Value,
		SOXPrice:               snapshot.SOXIndex.Value,
		TSMADRPrice:            snapshot.TSMADR.Value,
		TAIEXPrice:             snapshot.TAIEX.Value,
		ForeignSingleDayNet:    snapshot.ForeignInvestorNet.Value,
		ForeignFuturesOI:       snapshot.ForeignFuturesOINet.Value,
		MarginBalance:          snapshot.RetailMarginBalance.Value,
		MarginMaintenanceRatio: snapshot.MarginMaintenanceRatio.Value,
		MarketVolume:           snapshot.MarketVolume.Value,
		DayTradeRatio:          snapshot.DayTradeRatio.Value,
	}
}
