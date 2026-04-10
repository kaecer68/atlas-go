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
	if strings.Contains(prompt, "credit quality gate") && quote.Last >= quote.Open {
		conviction += 2
	}
	if strings.Contains(prompt, "credit quality gate") && quote.Last < quote.Open {
		conviction -= 6
	}
	if strings.Contains(prompt, "spread sensitivity downgrade") && quote.Last >= quote.High*0.995 {
		conviction += 2
	}
	if strings.Contains(prompt, "spread sensitivity downgrade") && quote.Last < quote.High*0.995 {
		conviction -= 4
	}
	if strings.Contains(prompt, "capital adequacy premium") && quote.Last >= quote.Open && quote.Last >= quote.High*0.995 {
		conviction += 3
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
	if strings.Contains(prompt, "close strength") && quote.Last < quote.High*0.985 {
		penalty := 1
		if strings.Contains(prompt, "close-strength tolerance") {
			if quote.Last >= quote.High*0.98 {
				penalty = 0
			}
		}
		conviction -= penalty
	}
	volumeFloor := int64(5000000)
	if strings.Contains(prompt, "volume surge requirement") {
		volumeFloor = 7000000
		if quote.Volume >= volumeFloor {
			conviction += 4
		} else {
			conviction -= 4
		}
	}
	if strings.Contains(prompt, "exploratory mode") {
		volumeFloor = 5000000
	}
	if strings.Contains(prompt, "coverage expansion") {
		volumeFloor = 0
	}
	if strings.Contains(prompt, "structure-first breakout filter") && quote.Last < quote.Open {
		conviction -= 10
	}
	if strings.Contains(prompt, "late-breakout penalty") && quote.Last < quote.High*0.998 {
		conviction -= 8
	}
	if strings.Contains(prompt, "breakout confirmation bonus") && quote.Last >= quote.High*0.998 && quote.Volume >= 5000000 {
		conviction += 12
	}
	if strings.Contains(prompt, "catch-up momentum") && quote.Last >= quote.High*0.993 && quote.Last < quote.High*0.998 && quote.Last >= quote.Open {
		conviction += 6
	}
	if strings.Contains(prompt, "volume participation acceptance") && quote.Volume >= 3000000 && quote.Volume < 5000000 {
		conviction += 3
	}
	if strings.Contains(prompt, "reject low volume") && quote.Volume < 5000000 {
		return domain.Recommendation{}, false
	}
	if strings.Contains(prompt, "enforce strict breakout confirmation") && quote.Volume < volumeFloor {
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
