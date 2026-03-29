package orchestrator

import (
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type FinancialsExecutor struct{}

func (FinancialsExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "financials_desk"
}

func (FinancialsExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string) (domain.Recommendation, bool) {
	conviction := 58
	if strings.Contains(prompt, "dividend") && quote.Last >= quote.Open {
		conviction += 8
	}
	if strings.Contains(prompt, "balance-sheet") && quote.Low < quote.Open*0.985 {
		conviction -= 6
	}
	if conviction < 50 {
		return domain.Recommendation{}, false
	}
	return domain.Recommendation{
		Agent:      agent.ID,
		Skill:      agent.Skill,
		Symbol:     quote.Symbol,
		Side:       domain.SideBuy,
		Conviction: conviction,
		Reason:     "financial carry with resilient balance-sheet posture",
	}, true
}

type ShippingExecutor struct{}

func (ShippingExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "shipping_desk"
}

func (ShippingExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string) (domain.Recommendation, bool) {
	conviction := 55
	if strings.Contains(prompt, "tactical") && quote.Last > quote.Open {
		conviction += 10
	}
	if strings.Contains(prompt, "avoid weak closes") && quote.Last < quote.High*0.992 {
		conviction -= 12
	}
	if conviction < 50 {
		return domain.Recommendation{}, false
	}
	return domain.Recommendation{
		Agent:      agent.ID,
		Skill:      agent.Skill,
		Symbol:     quote.Symbol,
		Side:       domain.SideBuy,
		Conviction: conviction,
		Reason:     "shipping beta used as tactical cycle exposure",
	}, true
}

type ValueYieldExecutor struct{}

func (ValueYieldExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "value_yield"
}

func (ValueYieldExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string) (domain.Recommendation, bool) {
	conviction := 52
	if strings.Contains(prompt, "cash-flow support") && quote.Last >= quote.Open {
		conviction += 10
	}
	if strings.Contains(prompt, "yield trap") && quote.Last < quote.Open {
		conviction -= 10
	}
	if conviction < 50 {
		return domain.Recommendation{}, false
	}
	return domain.Recommendation{
		Agent:      agent.ID,
		Skill:      agent.Skill,
		Symbol:     quote.Symbol,
		Side:       domain.SideBuy,
		Conviction: conviction,
		Reason:     "defensive yield lens with valuation discipline",
	}, true
}

type EarningsQualityExecutor struct{}

func (EarningsQualityExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "earnings_quality"
}

func (EarningsQualityExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string) (domain.Recommendation, bool) {
	conviction := 57
	if strings.Contains(prompt, "repeatable") && quote.Last > quote.Open {
		conviction += 9
	}
	if strings.Contains(prompt, "guidance") && quote.Last < quote.High*0.99 {
		conviction -= 8
	}
	if conviction < 50 {
		return domain.Recommendation{}, false
	}
	return domain.Recommendation{
		Agent:      agent.ID,
		Skill:      agent.Skill,
		Symbol:     quote.Symbol,
		Side:       domain.SideBuy,
		Conviction: conviction,
		Reason:     "earnings quality and forward visibility support",
	}, true
}

type TechnicalBreakoutExecutor struct{}

func (TechnicalBreakoutExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "technical_breakout"
}

func (TechnicalBreakoutExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string) (domain.Recommendation, bool) {
	conviction := 54
	if strings.Contains(prompt, "volume") && quote.Volume >= 5000000 {
		conviction += 8
	}
	if strings.Contains(prompt, "close strength") && quote.Last < quote.High*0.995 {
		conviction -= 14
	}
	if strings.Contains(prompt, "reject low volume") && quote.Volume < 5000000 {
		return domain.Recommendation{}, false
	}
	if conviction < 50 {
		return domain.Recommendation{}, false
	}
	return domain.Recommendation{
		Agent:      agent.ID,
		Skill:      agent.Skill,
		Symbol:     quote.Symbol,
		Side:       domain.SideBuy,
		Conviction: conviction,
		Reason:     "breakout structure confirmed by volume and close",
	}, true
}
