package orchestrator

import (
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

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
