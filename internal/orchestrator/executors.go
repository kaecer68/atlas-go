package orchestrator

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
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
	Scratchpad                 *Scratchpad // optional reasoning trace recorder
}

// ResearchResult holds all outputs from executing registry research.
type ResearchResult struct {
	Regime               domain.Regime
	RawRecommendations   []domain.Recommendation
	FinalRecommendations []domain.Recommendation
	GuardOutcomes        []domain.GuardOutcome
	ScreeningRejects     []domain.ScreeningReject
	DarwinianWeights     []*portfolio.DarwinianAgentWeight
}

// ExecuteWithContext executes registry research using a unified context.
func ExecuteWithContext(ctx ExecutionContext) ResearchResult {
	if ctx.Plugins == nil {
		ctx.Plugins = NewPluginRegistry()
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

	quoteBySymbol := make(map[string]domain.Quote, len(ctx.Quotes))
	for _, quote := range ctx.Quotes {
		quoteBySymbol[quote.Symbol] = quote
	}

	registry := filterMutedAgents(ctx.Registry, ctx.Plugins)

	regime := inferRegime(registry, quoteBySymbol, ctx.Plugins, ctx.Overrides, ctx.NarrativeEvents, ctx.Scratchpad, ctx.SessionID)
	raw, rejects := collectRecommendations(ctx.Context, registry, quoteBySymbol, ctx.Plugins, ctx.Overrides, regime, ctx.NarrativeEvents, ctx.SessionID, ctx.Scratchpad)

	if ctx.Policy.MomentumCrashProtection {
		raw = applyMomentumCrashProtection(raw, quoteBySymbol)
	}

	var weightData []*portfolio.DarwinianAgentWeight
	controlInput := raw
	if ctx.WeightManager != nil {
		var convictionEvents []portfolio.ConvictionClampingEvent
		controlInput, convictionEvents = ctx.WeightManager.ApplyDarwinianWeightsWithEvents(raw)
		weightData = ctx.WeightManager.GetAllAgentWeightData()
		if len(convictionEvents) > 0 && ctx.ConvictionClampingCallback != nil {
			ctx.ConvictionClampingCallback(convictionEvents)
		}
	}

	final, guardOutcomes := applyControlLayerWithOutcomes(registry, ctx.Plugins, controlInput, ctx.Policy, ctx.Scratchpad, ctx.SessionID)

	return ResearchResult{
		Regime:               regime,
		RawRecommendations:   raw,
		FinalRecommendations: final,
		GuardOutcomes:        guardOutcomes,
		ScreeningRejects:     rejects,
		DarwinianWeights:     weightData,
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

func filterMutedAgents(registry domain.AgentRegistry, plugins *PluginRegistry) domain.AgentRegistry {
	if plugins == nil || plugins.healthManager == nil {
		return registry
	}
	filtered := make([]domain.AgentSpec, 0, len(registry.Agents))
	for _, agent := range registry.Agents {
		if !plugins.IsAgentHealthy(agent.ID) {
			health := plugins.healthManager.GetHealth(agent.ID)
			score := 0.0
			if health != nil {
				score = health.CompositeScore
			}
			logging.Info("executors", "agent_muted", logging.AgentID(agent.ID), "composite_score", score)
			continue
		}
		filtered = append(filtered, agent)
	}
	return domain.AgentRegistry{
		Version: registry.Version,
		Agents:  filtered,
	}
}

func executeRegistryResearchDetailedWithPolicyAndGuards(
	registry domain.AgentRegistry,
	quotes []domain.Quote,
	overrides map[string]string,
	policy domain.ExecutionPolicy,
	plugins *PluginRegistry,
	sessionID string,
) (domain.Regime, []domain.Recommendation, []domain.Recommendation, []domain.GuardOutcome, []domain.ScreeningReject) {
	result := ExecuteWithContext(ExecutionContext{
		Registry:  registry,
		Quotes:    quotes,
		Overrides: overrides,
		Policy:    policy,
		Plugins:   plugins,
		SessionID: sessionID,
	})
	return result.Regime, result.RawRecommendations, result.FinalRecommendations, result.GuardOutcomes, result.ScreeningRejects
}

func executeRegistryResearchDetailedWithPolicyAndGuardsAndDarwinian(
	registry domain.AgentRegistry,
	quotes []domain.Quote,
	overrides map[string]string,
	policy domain.ExecutionPolicy,
	plugins *PluginRegistry,
	sessionID string,
	weightManager *portfolio.DarwinianWeightManager,
) (domain.Regime, []domain.Recommendation, []domain.Recommendation, []domain.GuardOutcome, []domain.ScreeningReject) {
	result := ExecuteWithContext(ExecutionContext{
		Registry:      registry,
		Quotes:        quotes,
		Overrides:     overrides,
		Policy:        policy,
		Plugins:       plugins,
		SessionID:     sessionID,
		WeightManager: weightManager,
	})
	return result.Regime, result.RawRecommendations, result.FinalRecommendations, result.GuardOutcomes, result.ScreeningRejects
}

// ExecuteRegistryResearchWithDarwinianWeights executes research with Darwinian weight application
// This applies Atlas-GIC style weight multipliers (0.3-2.5) to agent recommendations
func ExecuteRegistryResearchWithDarwinianWeights(
	registry domain.AgentRegistry,
	quotes []domain.Quote,
	overrides map[string]string,
	policy domain.ExecutionPolicy,
	weightManager *portfolio.DarwinianWeightManager,
) (domain.Regime, []domain.Recommendation, []domain.Recommendation, []*portfolio.DarwinianAgentWeight) {
	regime, raw, final, weightData, _ := executeRegistryResearchWithDarwinianWeights(registry, quotes, overrides, policy, weightManager, NewPluginRegistry(), "")
	return regime, raw, final, weightData
}

func executeRegistryResearchWithDarwinianWeights(
	registry domain.AgentRegistry,
	quotes []domain.Quote,
	overrides map[string]string,
	policy domain.ExecutionPolicy,
	weightManager *portfolio.DarwinianWeightManager,
	plugins *PluginRegistry,
	sessionID string,
) (domain.Regime, []domain.Recommendation, []domain.Recommendation, []*portfolio.DarwinianAgentWeight, []domain.ScreeningReject) {
	wm := weightManager
	if wm == nil {
		wm = portfolio.NewDarwinianWeightManager("configs/darwinian_weights.json")
		wm.InitializeFromRegistry(registry)
		_ = wm.Load()
	}

	result := ExecuteWithContext(ExecutionContext{
		Registry:      registry,
		Quotes:        quotes,
		Overrides:     overrides,
		Policy:        policy,
		Plugins:       plugins,
		SessionID:     sessionID,
		WeightManager: wm,
	})
	return result.Regime, result.RawRecommendations, result.FinalRecommendations, result.DarwinianWeights, result.ScreeningRejects
}

func inferRegime(registry domain.AgentRegistry, quotes map[string]domain.Quote, plugins *PluginRegistry, overrides map[string]string, events []narrative.NarrativeEvent, scratchpad *Scratchpad, sessionID string) domain.Regime {
	sources := []RegimeEvidenceSource{
		NewMacroEvidenceSource(),
		NewTechnicalEvidenceSource(),
		NewNarrativeEvidenceSource(),
		NewAgentSignalEvidenceSource(registry, plugins, overrides),
	}

	var totalScore, totalWeight float64
	for _, src := range sources {
		ev := src.Evidence(quotes, events)
		if ev.Confidence > 0 {
			totalScore += ev.Score * ev.Confidence
			totalWeight += ev.Confidence
		}
	}

	if totalWeight == 0 {
		if scratchpad != nil {
			scratchpad.Record(ReasoningTrace{
				SessionID:  sessionID,
				Timestamp:  time.Now().UTC(),
				Phase:      PhaseRegimeDetection,
				Step:       1,
				Component:  "regime_inference",
				Action:     "detect_regime",
				Reasoning:  "No evidence sources produced confidence; defaulting to neutral",
				Data:       map[string]any{"regime": domain.RegimeNeutral, "score": 0.0, "evidence_count": len(sources)},
				Confidence: 0.0,
			})
		}
		return domain.RegimeNeutral
	}

	normalized := totalScore / totalWeight

	var regime domain.Regime
	switch {
	case normalized > 0.15:
		regime = domain.RegimeRiskOn
	case normalized < -0.15:
		regime = domain.RegimeRiskOff
	default:
		regime = domain.RegimeNeutral
	}

	if scratchpad != nil {
		scratchpad.Record(ReasoningTrace{
			SessionID:  sessionID,
			Timestamp:  time.Now().UTC(),
			Phase:      PhaseRegimeDetection,
			Step:       1,
			Component:  "regime_inference",
			Action:     "detect_regime",
			Reasoning:  fmt.Sprintf("Regime detected: %s (normalized score: %.4f)", regime, normalized),
			Data:       map[string]any{"regime": regime, "score": normalized, "evidence_count": len(sources)},
			Confidence: totalWeight,
		})
	}

	return regime
}

func collectRecommendations(ctx context.Context, registry domain.AgentRegistry, quotes map[string]domain.Quote, plugins *PluginRegistry, overrides map[string]string, regime domain.Regime, narrativeEvents []narrative.NarrativeEvent, sessionID string, scratchpad *Scratchpad) ([]domain.Recommendation, []domain.ScreeningReject) {
	recs := make([]domain.Recommendation, 0)
	rejects := make([]domain.ScreeningReject, 0)
	now := time.Now().UTC()
	for _, agent := range registry.Agents {
		if !agent.Enabled {
			continue
		}
		if agent.Layer != domain.LayerSector && agent.Layer != domain.LayerStyle && agent.Layer != domain.LayerSuperinvestor {
			continue
		}

		prompt := plugins.ResolvePrompt(agent, overrides)
		symbols := agent.Universe
		if len(symbols) == 0 {
			symbols = slices.Collect(symbolIterator(DefaultSymbols()))
		}

		for _, symbol := range symbols {
			quote, ok := quotes[symbol]
			if !ok || !quote.IsTradable {
				continue
			}
			screenRes, err := plugins.ScreenDetailed(ctx, agent, symbol, quotes)
			if err != nil || !screenRes.Passed {
				if !screenRes.Passed {
					logging.Debug("screener", "screen_reject",
						logging.Symbol(symbol),
						logging.AgentID(agent.ID),
						logging.FStr("criterion", screenRes.Criterion),
						logging.FStr("reason", screenRes.Reason))
					rejects = append(rejects, domain.ScreeningReject{
						SessionID:      sessionID,
						Symbol:         symbol,
						AgentID:        agent.ID,
						Skill:          agent.Skill,
						Criterion:      screenRes.Criterion,
						CriterionLabel: screenRes.Label,
						Threshold:      screenRes.Threshold,
						ActualValue:    screenRes.Actual,
						RecordedAt:     now,
					})
				}
				continue
			}
			rec, ok := plugins.Recommendation(agent, quote, prompt, regime)
			if !ok {
				continue
			}
			recs = append(recs, rec)
		}
	}

	// Fill SupportingEvents for all recommendations with narrative event IDs.
	for i := range recs {
		eventIDs := make([]string, len(narrativeEvents))
		for j, e := range narrativeEvents {
			eventIDs[j] = e.ID
		}
		recs[i].SupportingEvents = eventIDs
	}

	agentWeights := make(map[string]float64)
	for i := range recs {
		breakdown, scores := plugins.CalculateFactorScoresWithBreakdown(recs[i].Symbol, quotes, recs, agentWeights)
		if scores != nil {
			recs[i].FactorScores = domain.FactorScores{
				Momentum:               scores[portfolio.FactorMomentum],
				Value:                  scores[portfolio.FactorValue],
				Quality:                scores[portfolio.FactorQuality],
				Agent:                  scores[portfolio.FactorAgent],
				InstitutionalSentiment: scores[portfolio.FactorInstSent],
				Liquidity:              scores[portfolio.FactorLiquidity],
				Total:                  scores["total"],
				Breakdown:              breakdown,
			}
		}
	}
	for i := range rejects {
		breakdown, scores := plugins.CalculateFactorScoresWithBreakdown(rejects[i].Symbol, quotes, recs, agentWeights)
		if scores != nil {
			rejects[i].FactorScores = domain.FactorScores{
				Momentum:               scores[portfolio.FactorMomentum],
				Value:                  scores[portfolio.FactorValue],
				Quality:                scores[portfolio.FactorQuality],
				Agent:                  scores[portfolio.FactorAgent],
				InstitutionalSentiment: scores[portfolio.FactorInstSent],
				Liquidity:              scores[portfolio.FactorLiquidity],
				Total:                  scores["total"],
				Breakdown:              breakdown,
			}
		}
	}

	var modulatorSteps []ModulationStep
	if plugins.cycleModulator != nil {
		steps := plugins.cycleModulator.CollectModulationSteps(recs, registry)
		modulatorSteps = append(modulatorSteps, steps...)
	}
	if plugins.narrativeModulator != nil {
		steps := plugins.narrativeModulator.CollectModulationSteps(recs, registry, narrativeEvents)
		modulatorSteps = append(modulatorSteps, steps...)
	}
	for _, ms := range modulatorSteps {
		if ms.RecIndex >= len(recs) {
			continue
		}
		for _, step := range ms.Steps {
			recs[ms.RecIndex].Conviction += step.Delta
			if recs[ms.RecIndex].ConvictionBreakdown != nil {
				recs[ms.RecIndex].ConvictionBreakdown.Steps = append(recs[ms.RecIndex].ConvictionBreakdown.Steps, step)
				recs[ms.RecIndex].ConvictionBreakdown.Final = recs[ms.RecIndex].Conviction
			}
		}
	}

	if scratchpad != nil {
		recData := make([]map[string]any, 0, len(recs))
		for _, rec := range recs {
			recData = append(recData, map[string]any{
				"symbol":     rec.Symbol,
				"agent":      rec.Agent,
				"conviction": rec.Conviction,
			})
		}
		rejSummary := make([]map[string]string, 0, len(rejects))
		rejReasons := make(map[string]int)
		for _, r := range rejects {
			rejSummary = append(rejSummary, map[string]string{
				"symbol":    r.Symbol,
				"agent":     r.AgentID,
				"reason":    r.Criterion,
				"label":     r.CriterionLabel,
				"actual":    r.ActualValue,
				"threshold": r.Threshold,
			})
			rejReasons[r.Criterion]++
		}
		reasoning := fmt.Sprintf("Collected %d recommendations, %d screening rejects", len(recs), len(rejects))
		if len(recs) == 0 && len(rejects) > 0 {
			var topReasons []string
			for k, v := range rejReasons {
				topReasons = append(topReasons, fmt.Sprintf("%d×%s", v, k))
			}
			reasoning += " | All rejected: " + strings.Join(topReasons, ", ")
		}
		if len(recs) == 0 && len(rejects) == 0 {
			reasoning += " | WARNING: no quotes available — check replay data or market provider"
		}
		scratchpad.Record(ReasoningTrace{
			SessionID: sessionID,
			Timestamp: now,
			Phase:     PhaseAgentRecommendation,
			Step:      2,
			Component: "recommendation_collector",
			Action:    "collect_recommendations",
			Reasoning: reasoning,
			Data: map[string]any{
				"recommendation_count": len(recs),
				"reject_count":         len(rejects),
				"quote_count":          len(quotes),
				"recommendations":      recData,
				"rejects":              rejSummary,
			},
			Confidence: avgConvictionScore(recs),
		})
	}

	// Emit WARN trace when all agents muted (zero recommendations)
	if len(recs) == 0 && scratchpad != nil {
		activeAgents := 0
		for _, agent := range registry.Agents {
			if agent.Enabled && (agent.Layer == domain.LayerSector || agent.Layer == domain.LayerStyle || agent.Layer == domain.LayerSuperinvestor) {
				activeAgents++
			}
		}
		scratchpad.Record(ReasoningTrace{
			SessionID: sessionID,
			Timestamp: time.Now().UTC(),
			Phase:     PhaseAgentRecommendation,
			Step:      3,
			Component: "recommendation_collector",
			Action:    "zero_recommendations_warning",
			Reasoning: "All agents muted: no recommendations generated",
			Data: map[string]any{
				"agents_total":  len(registry.Agents),
				"agents_active": activeAgents,
				"regime":        string(regime),
			},
			Confidence: 0.0,
		})
	}

	return recs, rejects
}

func applyMomentumCrashProtection(recs []domain.Recommendation, quotes map[string]domain.Quote) []domain.Recommendation {
	vix := 0.0
	vixFound := false
	for _, q := range quotes {
		if q.Symbol == "VIX" || q.Symbol == "^VIX" {
			vix = q.Last
			vixFound = true
			break
		}
	}
	if !vixFound {
		logging.Warn("executors", "vix_not_found", "event", "momentum_crash_protection_disabled")
		return recs
	}
	cfg := config.GetEngineConfig().Executors
	if vix <= cfg.VIXMomentumCrashThreshold {
		return recs
	}

	params := config.GetParametersConfig().Orchestrator
	for i := range recs {
		if recs[i].FactorScores.Momentum == 0 {
			continue
		}
		recs[i].FactorScores.Momentum = 0
		if recs[i].FactorScores.Breakdown != nil {
			recs[i].FactorScores.Breakdown.Momentum.Score = 0
		}
		remainingWeight := params.FactorWeightValue.Value + params.FactorWeightQuality.Value + params.FactorWeightAgent.Value
		recs[i].FactorScores.Total = recs[i].FactorScores.Value*(params.FactorWeightValue.Value/remainingWeight) +
			recs[i].FactorScores.Quality*(params.FactorWeightQuality.Value/remainingWeight) +
			recs[i].FactorScores.Agent*(params.FactorWeightAgent.Value/remainingWeight)
		if recs[i].FactorScores.Breakdown != nil {
			recs[i].FactorScores.Breakdown.Total.Score = recs[i].FactorScores.Total
		}
	}
	return recs
}

func applyControlLayer(registry domain.AgentRegistry, plugins *PluginRegistry, recs []domain.Recommendation, policy domain.ExecutionPolicy) []domain.Recommendation {
	final, _ := applyControlLayerWithOutcomes(registry, plugins, recs, policy, nil, "")
	return final
}

func applyControlLayerWithOutcomes(registry domain.AgentRegistry, plugins *PluginRegistry, recs []domain.Recommendation, policy domain.ExecutionPolicy, scratchpad *Scratchpad, sessionID string) ([]domain.Recommendation, []domain.GuardOutcome) {
	if !policy.RequireCROPass {
		return recs, []domain.GuardOutcome{{
			GuardID:     "control-bypass",
			GuardSkill:  "control_bypass",
			Severity:    domain.GuardSeveritySoft,
			Passed:      true,
			Reason:      "控制層已略過（未啟用 CRO 檢查）",
			InputCount:  len(recs),
			OutputCount: len(recs),
		}}
	}

	current := recs
	outcomes := make([]domain.GuardOutcome, 0)
	for _, agent := range registry.Agents {
		if !agent.Enabled || agent.Layer != domain.LayerControl {
			continue
		}
		before := len(current)
		next := plugins.ApplyControl(agent, current, policy)
		after := len(next)
		severity := severityForControlAgent(agent)
		blocked := before > 0 && after == 0 && severity == domain.GuardSeverityHard
		reason := "未過濾任何推薦，全部放行"
		if after < before {
			reason = fmt.Sprintf("過濾了 %d 筆推薦，僅保留符合條件的標的", before-after)
		}
		if blocked {
			reason = "強制阻擋全部推薦，當日不進場"
		}
		outcomes = append(outcomes, domain.GuardOutcome{
			GuardID:     agent.ID,
			GuardSkill:  agent.Skill,
			Severity:    severity,
			Passed:      !blocked,
			Reason:      reason,
			InputCount:  before,
			OutputCount: after,
		})
		current = next

		if scratchpad != nil {
			if agent.Skill == "cro_risk" {
				scratchpad.Record(ReasoningTrace{
					SessionID: sessionID,
					Timestamp: time.Now().UTC(),
					Phase:     PhaseControlFilter,
					Step:      3,
					Component: "cro_filter",
					Action:    "apply_cro_filter",
					Reasoning: fmt.Sprintf("CRO filter: %d in -> %d out, conviction floor: %d", before, after, policy.ConvictionFloor),
					Data: map[string]any{
						"input_count":               before,
						"output_count":              after,
						"conviction_floor":          policy.ConvictionFloor,
						"z_score_enabled":           policy.EnableConvictionNormalization,
						"momentum_crash_protection": policy.MomentumCrashProtection,
					},
					Confidence: passRatio(before, after),
				})
			}
			if agent.Skill == "cio_portfolio" {
				symbolAgents := make(map[string][]map[string]any)
				for _, rec := range recs[:before] {
					symbolAgents[rec.Symbol] = append(symbolAgents[rec.Symbol], map[string]any{
						"agent":      rec.Agent,
						"conviction": rec.Conviction,
					})
				}
				symbolData := make([]map[string]any, 0, len(next))
				for _, rec := range next {
					agents := symbolAgents[rec.Symbol]
					symbolData = append(symbolData, map[string]any{
						"symbol":              rec.Symbol,
						"agent_count":         len(agents),
						"weighted_conviction": rec.Conviction,
						"agents":              agents,
					})
				}
				scratchpad.Record(ReasoningTrace{
					SessionID: sessionID,
					Timestamp: time.Now().UTC(),
					Phase:     PhaseControlFilter,
					Step:      4,
					Component: "cio_aggregator",
					Action:    "apply_cio_aggregation",
					Reasoning: fmt.Sprintf("CIO aggregation: %d recommendations -> %d unique symbols", before, len(next)),
					Data: map[string]any{
						"input_count":  before,
						"output_count": len(next),
						"symbols":      symbolData,
					},
					Confidence: passRatio(before, len(next)),
				})
			}
		}
	}

	current = applyCrowdingPenalty(current)
	current = applyAntiCorrelationLayer(current, 0)

	// Align the last guard's OutputCount with the true final recommendation count
	// so that downstream outcome building and UI display are consistent.
	if len(outcomes) > 0 {
		last := &outcomes[len(outcomes)-1]
		last.OutputCount = len(current)
		if last.OutputCount < last.InputCount {
			last.Reason = fmt.Sprintf("過濾了 %d 筆推薦，僅保留符合條件的標的", last.InputCount-last.OutputCount)
		} else {
			last.Reason = "未過濾任何推薦，全部放行"
		}
	}

	return current, outcomes
}

// applyCrowdingPenalty reduces conviction when 3+ agents recommend the same symbol.
func applyCrowdingPenalty(recs []domain.Recommendation) []domain.Recommendation {
	if len(recs) == 0 {
		return recs
	}
	symbolAgents := map[string]map[string]struct{}{}
	for _, rec := range recs {
		if _, ok := symbolAgents[rec.Symbol]; !ok {
			symbolAgents[rec.Symbol] = map[string]struct{}{}
		}
		symbolAgents[rec.Symbol][rec.Agent] = struct{}{}
	}

	cfg := config.GetEngineConfig().Executors
	out := make([]domain.Recommendation, len(recs))
	for i, rec := range recs {
		agents := symbolAgents[rec.Symbol]
		penalty := 1.0
		if len(agents) >= 4 {
			penalty = cfg.CrowdingPenaltyAgents4
		} else if len(agents) >= 3 {
			penalty = cfg.CrowdingPenaltyAgents3
		}
		rec.Conviction = int(float64(rec.Conviction) * penalty)
		out[i] = rec
	}
	return out
}

// applyAntiCorrelationLayer deduplicates by symbol and enforces skill-level diversity.
func applyAntiCorrelationLayer(recs []domain.Recommendation, availableCash float64) []domain.Recommendation {
	if len(recs) == 0 {
		return recs
	}
	bySymbol := map[string]domain.Recommendation{}
	for _, rec := range recs {
		existing, ok := bySymbol[rec.Symbol]
		if !ok || rec.Conviction > existing.Conviction {
			bySymbol[rec.Symbol] = rec
		}
	}

	skillRecs := map[string][]domain.Recommendation{}
	for _, rec := range bySymbol {
		skillRecs[rec.Skill] = append(skillRecs[rec.Skill], rec)
	}
	for skill := range skillRecs {
		slices.SortFunc(skillRecs[skill], func(a, b domain.Recommendation) int {
			if a.Conviction > b.Conviction {
				return -1
			}
			if a.Conviction < b.Conviction {
				return 1
			}
			return 0
		})
	}

	cfg := config.GetEngineConfig().Executors
	minTrade := cfg.MinTradeAmount
	maxStocks := cfg.MaxStocksDefault
	if availableCash > 0 {
		calculated := min(max(int(availableCash/minTrade), cfg.MaxStocksMin), cfg.MaxStocksMax)
		maxStocks = calculated
	}

	out := make([]domain.Recommendation, 0, len(bySymbol))
	for _, recsForSkill := range skillRecs {
		for i, rec := range recsForSkill {
			if i >= 2 {
				continue
			}
			out = append(out, rec)
		}
	}
	for _, recsForSkill := range skillRecs {
		for i, rec := range recsForSkill {
			if i < 2 {
				continue
			}
			if len(out) < maxStocks {
				out = append(out, rec)
			}
		}
	}

	slices.SortFunc(out, func(a, b domain.Recommendation) int {
		if a.Conviction > b.Conviction {
			return -1
		}
		if a.Conviction < b.Conviction {
			return 1
		}
		if a.Symbol < b.Symbol {
			return -1
		}
		if a.Symbol > b.Symbol {
			return 1
		}
		return 0
	})
	return out
}

func severityForControlAgent(agent domain.AgentSpec) domain.GuardSeverity {
	if agent.Skill == "cro_risk" {
		return domain.GuardSeverityHard
	}
	return domain.GuardSeveritySoft
}

func DefaultExecutionPolicy() domain.ExecutionPolicy {
	cfg := config.GetEngineConfig().Executors
	return domain.ExecutionPolicy{
		ConvictionFloor:         cfg.ConvictionFloorDefault,
		RequireCROPass:          true,
		MomentumCrashProtection: true,
	}
}

func DefaultSymbols() []string {
	return []string{
		"2330.TW",
		"2317.TW",
		"2382.TW",
		"2454.TW",
		"2303.TW",
		"2308.TW",
		"3008.TW",
		"3034.TW",
		"3037.TW",
		"6669.TW",
		"2603.TW",
		"2609.TW",
		"2615.TW",
		"2881.TW",
		"2882.TW",
		"2886.TW",
		"2891.TW",
		"2892.TW",
		"1301.TW",
		"1303.TW",
		"1326.TW",
		"0050.TW",
		"0056.TW",
		"00878.TW",
	}
}

func RegistrySymbols(registry domain.AgentRegistry) []string {
	seen := make(map[string]struct{})
	symbols := make([]string, 0)

	for _, symbol := range DefaultSymbols() {
		if _, ok := seen[symbol]; ok {
			continue
		}
		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)
	}

	for _, agent := range registry.Agents {
		if !agent.Enabled {
			continue
		}
		for _, symbol := range agent.Universe {
			if _, ok := seen[symbol]; ok {
				continue
			}
			seen[symbol] = struct{}{}
			symbols = append(symbols, symbol)
		}
	}
	return symbols
}

func SymbolsForSkill(registry domain.AgentRegistry, skill string) []string {
	for _, agent := range registry.Agents {
		if !agent.Enabled || agent.Skill != skill {
			continue
		}
		if len(agent.Universe) > 0 {
			return agent.Universe
		}
		break
	}
	return DefaultSymbols()
}

func symbolIterator(symbols []string) func(func(string) bool) {
	return func(yield func(string) bool) {
		for _, symbol := range symbols {
			if !yield(symbol) {
				return
			}
		}
	}
}

// avgConvictionScore returns the average conviction of recommendations as a
// 0-1 score. Returns 0 when recs is empty.
func avgConvictionScore(recs []domain.Recommendation) float64 {
	if len(recs) == 0 {
		return 0
	}
	var total int
	for _, r := range recs {
		total += r.Conviction
	}
	avg := float64(total) / float64(len(recs))
	if avg > 100 {
		return 1.0
	}
	return avg / 100.0
}

// passRatio returns the ratio of output to input as a 0-1 confidence score.
// Returns 1 when input is 0 (no recommendations to filter).
func passRatio(input, output int) float64 {
	if input <= 0 {
		return 0.0
	}
	ratio := float64(output) / float64(input)
	if ratio > 1.0 {
		return 1.0
	}
	return ratio
}
