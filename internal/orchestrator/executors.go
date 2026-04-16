package orchestrator

import (
	"fmt"
	"slices"

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
	plugins := NewPluginRegistry()
	quoteBySymbol := make(map[string]domain.Quote, len(quotes))
	for _, quote := range quotes {
		quoteBySymbol[quote.Symbol] = quote
	}

	regime := inferRegime(registry, quoteBySymbol, plugins, overrides)
	raw := collectRecommendations(registry, quoteBySymbol, plugins, overrides, regime)
	final, guardOutcomes := applyControlLayerWithOutcomes(registry, plugins, raw, policy)
	return regime, raw, final, guardOutcomes
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
	plugins := NewPluginRegistry()
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
	raw := collectRecommendations(registry, quoteBySymbol, plugins, overrides, regime)

	// Apply Darwinian weights to recommendations
	weighted := weightManager.ApplyDarwinianWeights(raw)

	// Apply control layer (CRO) to weighted recommendations
	final := applyControlLayer(registry, plugins, weighted, policy)

	// Get current weight data for reporting
	weightData := weightManager.GetAllAgentWeightData()

	return regime, raw, final, weightData
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

func collectRecommendations(registry domain.AgentRegistry, quotes map[string]domain.Quote, plugins *PluginRegistry, overrides map[string]string, regime domain.Regime) []domain.Recommendation {
	recs := make([]domain.Recommendation, 0)
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
			rec, ok := plugins.Recommendation(agent, quote, prompt, regime)
			if !ok {
				continue
			}
			recs = append(recs, rec)
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
	current = applyAntiCorrelationLayer(current)

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
func applyAntiCorrelationLayer(recs []domain.Recommendation) []domain.Recommendation {
	if len(recs) == 0 {
		return recs
	}
	// Keep highest-conviction rec per symbol
	bySymbol := map[string]domain.Recommendation{}
	for _, rec := range recs {
		existing, ok := bySymbol[rec.Symbol]
		if !ok || rec.Conviction > existing.Conviction {
			bySymbol[rec.Symbol] = rec
		}
	}

	// Group by skill and enforce max 2 symbols per skill to improve diversification
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

	out := make([]domain.Recommendation, 0, len(bySymbol))
	skillCount := map[string]int{}
	// First pass: add highest conviction from each skill up to limit
	for _, recsForSkill := range skillRecs {
		for i, rec := range recsForSkill {
			if i >= 2 {
				continue
			}
			out = append(out, rec)
			skillCount[rec.Skill]++
		}
	}
	// Second pass: add remaining symbols that didn't make the cut if portfolio still has room
	for _, recsForSkill := range skillRecs {
		for i, rec := range recsForSkill {
			if i < 2 {
				continue
			}
			// Only add if total portfolio under 8 symbols and skill already represented
			if len(out) < 8 {
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
		ConvictionFloor: 50,
		RequireCROPass:  true,
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
