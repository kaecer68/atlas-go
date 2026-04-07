package orchestrator

import (
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type SemiconductorExecutor struct{}

func (SemiconductorExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "semiconductor_desk"
}

func (SemiconductorExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string) (domain.Recommendation, bool) {
	conviction := 84
	if strings.Contains(prompt, "illiquid") && quote.Volume < 5000000 {
		return domain.Recommendation{}, false
	}
	return domain.Recommendation{
		Agent:      agent.ID,
		Skill:      agent.Skill,
		Symbol:     quote.Symbol,
		Side:       domain.SideBuy,
		Conviction: conviction,
		Reason:     "semiconductor leadership and supply-chain role",
	}, true
}

type AISupplyChainExecutor struct{}

func (AISupplyChainExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "ai_supply_chain_desk"
}

func (AISupplyChainExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string) (domain.Recommendation, bool) {
	conviction := 78
	if quote.Last < quote.Open {
		conviction -= 5
	}
	if strings.Contains(prompt, "order-flow") && quote.Volume > 10000000 {
		conviction += 8
	}
	if strings.Contains(prompt, "downgrade") && quote.Last < quote.High*0.99 {
		conviction -= 10
	}
	if conviction < 60 {
		return domain.Recommendation{}, false
	}
	return domain.Recommendation{
		Agent:      agent.ID,
		Skill:      agent.Skill,
		Symbol:     quote.Symbol,
		Side:       domain.SideBuy,
		Conviction: conviction,
		Reason:     "ai infrastructure order-flow sensitivity",
	}, true
}

type ETFRotationExecutor struct{}

func (ETFRotationExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "etf_rotation_desk"
}

func (ETFRotationExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string) (domain.Recommendation, bool) {
	conviction := 64
	if quote.Last < quote.Open {
		conviction -= 3
	}
	if strings.Contains(prompt, "rotation") && quote.Last > quote.Open {
		conviction += 6
	}
	if strings.Contains(prompt, "sector leadership") && quote.Volume > 8000000 {
		conviction += 5
	}
	if strings.Contains(prompt, "reject") && quote.Last < quote.Low*1.005 {
		return domain.Recommendation{}, false
	}
	if conviction < 55 {
		return domain.Recommendation{}, false
	}
	return domain.Recommendation{
		Agent:      agent.ID,
		Skill:      agent.Skill,
		Symbol:     quote.Symbol,
		Side:       domain.SideBuy,
		Conviction: conviction,
		Reason:     "broad ETF fallback under controlled risk",
	}, true
}
