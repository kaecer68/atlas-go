package orchestrator

import (
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type GrowthMomentumExecutor struct{}

func (GrowthMomentumExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "growth_momentum"
}

func (GrowthMomentumExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string) (domain.Recommendation, bool) {
	conviction := 62
	if strings.Contains(prompt, "require trend confirmation") && quote.Last < quote.Open {
		conviction -= 18
	}
	if strings.Contains(prompt, "downgrade conviction") && quote.Last < quote.High*0.995 {
		conviction -= 12
	}
	if strings.Contains(prompt, "reject setups") && quote.Last < quote.Open {
		return domain.Recommendation{}, false
	}
	if strings.Contains(prompt, "illiquid") && quote.Volume < 5000000 {
		return domain.Recommendation{}, false
	}
	if conviction < 45 {
		return domain.Recommendation{}, false
	}
	return domain.Recommendation{
		Agent:      agent.ID,
		Skill:      agent.Skill,
		Symbol:     quote.Symbol,
		Side:       domain.SideBuy,
		Conviction: conviction,
		Reason:     "price persistence with style overlay",
	}, true
}
