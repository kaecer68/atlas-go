package orchestrator

import (
	"context"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

func ExecuteRegistryResearch(registry domain.AgentRegistry, quotes []domain.Quote, overrides map[string]string) (domain.Regime, []domain.Recommendation) {
	regime, _, final := ExecuteRegistryResearchDetailed(registry, quotes, overrides)
	return regime, final
}

func ExecuteRegistryResearchDetailed(registry domain.AgentRegistry, quotes []domain.Quote, overrides map[string]string) (domain.Regime, []domain.Recommendation, []domain.Recommendation) {
	return ExecuteRegistryResearchDetailedWithPolicy(registry, quotes, overrides, DefaultExecutionPolicy())
}

func ExecuteRegistryResearchDetailedWithPolicy(registry domain.AgentRegistry, quotes []domain.Quote, overrides map[string]string, policy domain.ExecutionPolicy) (domain.Regime, []domain.Recommendation, []domain.Recommendation) {
	regime, raw, final, _ := ExecuteRegistryResearchDetailedWithPolicyAndGuards(registry, quotes, overrides, policy)
	return regime, raw, final
}

func ExecuteRegistryResearchDetailedWithPolicyAndGuards(registry domain.AgentRegistry, quotes []domain.Quote, overrides map[string]string, policy domain.ExecutionPolicy) (domain.Regime, []domain.Recommendation, []domain.Recommendation, []domain.GuardOutcome) {
	regime, raw, final, guardOutcomes, _ := executeRegistryResearchDetailedWithPolicyAndGuards(registry, quotes, overrides, policy, NewPluginRegistry(), "")
	return regime, raw, final, guardOutcomes
}

func ExecuteRegistryResearchDetailedWithPolicyAndGuardsAndPlugins(
	registry domain.AgentRegistry,
	quotes []domain.Quote,
	overrides map[string]string,
	policy domain.ExecutionPolicy,
	plugins *PluginRegistry,
) (domain.Regime, []domain.Recommendation, []domain.Recommendation, []domain.GuardOutcome) {
	regime, raw, final, guardOutcomes, _ := executeRegistryResearchDetailedWithPolicyAndGuards(registry, quotes, overrides, policy, plugins, "")
	return regime, raw, final, guardOutcomes
}

func executeRegistryResearchDetailedWithPolicyAndGuards(
	registry domain.AgentRegistry,
	quotes []domain.Quote,
	overrides map[string]string,
	policy domain.ExecutionPolicy,
	plugins *PluginRegistry,
	sessionID string,
) (domain.Regime, []domain.Recommendation, []domain.Recommendation, []domain.GuardOutcome, []domain.ScreeningReject) {
	quoteBySymbol := make(map[string]domain.Quote, len(quotes))
	for _, quote := range quotes {
		quoteBySymbol[quote.Symbol] = quote
	}

	regime := inferRegime(registry, quoteBySymbol, plugins, overrides)
	raw, rejects := collectRecommendations(registry, quoteBySymbol, plugins, overrides, regime, sessionID)
	if policy.MomentumCrashProtection {
		raw = applyMomentumCrashProtection(raw, quoteBySymbol)
	}
	final, guardOutcomes := applyControlLayerWithOutcomes(registry, plugins, raw, policy)
	return regime, raw, final, guardOutcomes, rejects
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
	quoteBySymbol := make(map[string]domain.Quote, len(quotes))
	for _, quote := range quotes {
		quoteBySymbol[quote.Symbol] = quote
	}

	regime := inferRegime(registry, quoteBySymbol, plugins, overrides)
	raw, rejects := collectRecommendations(registry, quoteBySymbol, plugins, overrides, regime, sessionID)
	if policy.MomentumCrashProtection {
		raw = applyMomentumCrashProtection(raw, quoteBySymbol)
	}

	// Apply Darwinian weights to raw recommendations before control layer.
	weighted := weightManager.ApplyDarwinianWeights(raw)

	// Apply control layer with outcomes to weighted recommendations.
	final, guardOutcomes := applyControlLayerWithOutcomes(registry, plugins, weighted, policy)

	return regime, raw, final, guardOutcomes, rejects
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
	quoteBySymbol := make(map[string]domain.Quote, len(quotes))
	for _, quote := range quotes {
		quoteBySymbol[quote.Symbol] = quote
	}

	// Initialize weight manager if needed
	if weightManager == nil {
		weightManager = portfolio.NewDarwinianWeightManager("configs/darwinian_weights.json")
		weightManager.InitializeFromRegistry(registry)
		_ = weightManager.Load() // Try to load existing weights
	}

	regime := inferRegime(registry, quoteBySymbol, plugins, overrides)
	raw, rejects := collectRecommendations(registry, quoteBySymbol, plugins, overrides, regime, sessionID)
	if policy.MomentumCrashProtection {
		raw = applyMomentumCrashProtection(raw, quoteBySymbol)
	}

	// Apply Darwinian weights to recommendations
	weighted := weightManager.ApplyDarwinianWeights(raw)

	// Apply control layer (CRO) to weighted recommendations
	final := applyControlLayer(registry, plugins, weighted, policy)

	// Get current weight data for reporting
	weightData := weightManager.GetAllAgentWeightData()

	return regime, raw, final, weightData, rejects
}

func inferRegime(registry domain.AgentRegistry, quotes map[string]domain.Quote, plugins *PluginRegistry, overrides map[string]string) domain.Regime {
	score := 0
	for _, agent := range registry.Agents {
		if !agent.Enabled || agent.Layer != domain.LayerContext {
			continue
		}
		prompt := plugins.ResolvePrompt(agent, overrides)
		score += plugins.RegimeScore(agent, quotes, prompt)
	}

	switch {
	case score > 0:
		return domain.RegimeRiskOn
	case score < 0:
		return domain.RegimeRiskOff
	default:
		return domain.RegimeNeutral
	}
}

func collectRecommendations(registry domain.AgentRegistry, quotes map[string]domain.Quote, plugins *PluginRegistry, overrides map[string]string, regime domain.Regime, sessionID string) ([]domain.Recommendation, []domain.ScreeningReject) {
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
			screenRes, err := plugins.ScreenDetailed(context.Background(), agent, symbol, quotes)
			if err != nil || !screenRes.Passed {
				if !screenRes.Passed {
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

	agentWeights := make(map[string]float64)
	for i := range recs {
		breakdown, scores := plugins.CalculateFactorScoresWithBreakdown(recs[i].Symbol, quotes, recs, agentWeights)
		if scores != nil {
			recs[i].FactorScores = domain.FactorScores{
				Momentum:  scores[portfolio.FactorMomentum],
				Value:     scores[portfolio.FactorValue],
				Quality:   scores[portfolio.FactorQuality],
				Agent:     scores[portfolio.FactorAgent],
				Total:     scores["total"],
				Breakdown: breakdown,
			}
		}
	}
	for i := range rejects {
		breakdown, scores := plugins.CalculateFactorScoresWithBreakdown(rejects[i].Symbol, quotes, recs, agentWeights)
		if scores != nil {
			rejects[i].FactorScores = domain.FactorScores{
				Momentum:  scores[portfolio.FactorMomentum],
				Value:     scores[portfolio.FactorValue],
				Quality:   scores[portfolio.FactorQuality],
				Agent:     scores[portfolio.FactorAgent],
				Total:     scores["total"],
				Breakdown: breakdown,
			}
		}
	}

	return recs, rejects
}

const vixMomentumCrashThreshold = 30.0

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
		log.Printf("[WARN] VIX not found in quotes; momentum crash protection disabled")
		return recs
	}
	if vix <= vixMomentumCrashThreshold {
		return recs
	}

	for i := range recs {
		if recs[i].FactorScores.Momentum == 0 {
			continue
		}
		recs[i].FactorScores.Momentum = 0
		if recs[i].FactorScores.Breakdown != nil {
			recs[i].FactorScores.Breakdown.Momentum.Score = 0
		}
		// Recalculate total without momentum, normalizing remaining weights to sum to 1.0
		// Original weights: momentum=0.30, value=0.25, quality=0.25, agent=0.20
		remainingWeight := 0.25 + 0.25 + 0.20
		recs[i].FactorScores.Total = recs[i].FactorScores.Value*(0.25/remainingWeight) +
			recs[i].FactorScores.Quality*(0.25/remainingWeight) +
			recs[i].FactorScores.Agent*(0.20/remainingWeight)
		if recs[i].FactorScores.Breakdown != nil {
			recs[i].FactorScores.Breakdown.Total.Score = recs[i].FactorScores.Total
		}
	}
	return recs
}

func applyControlLayer(registry domain.AgentRegistry, plugins *PluginRegistry, recs []domain.Recommendation, policy domain.ExecutionPolicy) []domain.Recommendation {
	final, _ := applyControlLayerWithOutcomes(registry, plugins, recs, policy)
	return final
}

func applyControlLayerWithOutcomes(registry domain.AgentRegistry, plugins *PluginRegistry, recs []domain.Recommendation, policy domain.ExecutionPolicy) ([]domain.Recommendation, []domain.GuardOutcome) {
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

	out := make([]domain.Recommendation, len(recs))
	for i, rec := range recs {
		agents := symbolAgents[rec.Symbol]
		penalty := 1.0
		if len(agents) >= 4 {
			penalty = 0.6
		} else if len(agents) >= 3 {
			penalty = 0.75
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

	minTrade := 100000.0
	maxStocks := 8
	if availableCash > 0 {
		calculated := int(availableCash / minTrade)
		if calculated < 5 {
			calculated = 5
		}
		if calculated > 12 {
			calculated = 12
		}
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
	return domain.ExecutionPolicy{
		ConvictionFloor:         50,
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
