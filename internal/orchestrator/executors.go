package orchestrator

import (
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
	plugins := NewPluginRegistry()
	quoteBySymbol := make(map[string]domain.Quote, len(quotes))
	for _, quote := range quotes {
		quoteBySymbol[quote.Symbol] = quote
	}

	regime := inferRegime(registry, quoteBySymbol, plugins, overrides)
	raw := collectRecommendations(registry, quoteBySymbol, plugins, overrides)
	final := applyControlLayer(registry, plugins, raw, policy)
	return regime, raw, final
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
	raw := collectRecommendations(registry, quoteBySymbol, plugins, overrides)

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

func collectRecommendations(registry domain.AgentRegistry, quotes map[string]domain.Quote, plugins *PluginRegistry, overrides map[string]string) []domain.Recommendation {
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
			symbols = slices.Collect(symbolIterator(defaultSymbols()))
		}

		for _, symbol := range symbols {
			quote, ok := quotes[symbol]
			if !ok || !quote.IsTradable {
				continue
			}
			rec, ok := plugins.Recommendation(agent, quote, prompt)
			if !ok {
				continue
			}
			recs = append(recs, rec)
		}
	}
	return recs
}

func applyControlLayer(registry domain.AgentRegistry, plugins *PluginRegistry, recs []domain.Recommendation, policy domain.ExecutionPolicy) []domain.Recommendation {
	if !policy.RequireCROPass {
		return recs
	}
	current := recs
	for _, agent := range registry.Agents {
		if !agent.Enabled || agent.Layer != domain.LayerControl {
			continue
		}
		current = plugins.ApplyControl(agent, current, policy)
	}
	return current
}

func DefaultExecutionPolicy() domain.ExecutionPolicy {
	return domain.ExecutionPolicy{
		ConvictionFloor: 50,
		RequireCROPass:  true,
	}
}

func defaultSymbols() []string {
	return []string{
		"2330.TW",
		"2317.TW",
		"2382.TW",
		"2603.TW",
		"2609.TW",
		"2881.TW",
		"2891.TW",
		"0050.TW",
	}
}

func RegistrySymbols(registry domain.AgentRegistry) []string {
	seen := make(map[string]struct{})
	symbols := make([]string, 0)

	for _, symbol := range defaultSymbols() {
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
	return defaultSymbols()
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
