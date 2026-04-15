package orchestrator

import (
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type SemiconductorExecutor struct{}

func (SemiconductorExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "semiconductor_desk"
}

func (SemiconductorExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	conviction := 84

	// Try structured control block first; fall back to legacy string matching.
	ctrl, ok := domain.ExtractPromptControl(prompt)
	if !ok {
		// Legacy fallback for older prompts without control blocks.
		volumeFloor := int64(1500000)
		if strings.Contains(prompt, "2.0M") {
			volumeFloor = 2000000
		}
		if strings.Contains(prompt, "close strength confirms leadership") && quote.Last > quote.Open && quote.Volume >= volumeFloor {
			conviction += 10
		}
		if strings.Contains(prompt, "weak volume") {
			if quote.Volume < volumeFloor {
				conviction -= 25
			} else if quote.Volume < 3000000 && quote.Last < quote.Open {
				conviction -= 15
			}
		}
		if strings.Contains(prompt, "illiquid") && quote.Volume < volumeFloor {
			return domain.Recommendation{}, false
		}
		if conviction < 60 {
			return domain.Recommendation{}, false
		}
		tp, slp := priceTargets(quote, 1.06, 0.95)
		return domain.Recommendation{
			Agent:         agent.ID,
			Skill:         agent.Skill,
			Layer:         agent.Layer,
			Symbol:        quote.Symbol,
			Side:          domain.SideBuy,
			Conviction:    conviction,
			Reason:        "semiconductor leadership and supply-chain role",
			TargetPrice:   tp,
			StopLossPrice: slp,
		}, true
	}

	// Structured path
	volumeFloor := ctrl.VolumeFloor
	if volumeFloor == 0 {
		volumeFloor = 1500000
	}

	if ctrl.CloseStrengthBoost > 0 && quote.Last > quote.Open && quote.Volume >= volumeFloor {
		conviction += ctrl.CloseStrengthBoost
	}

	if ctrl.VolumeDowngrade > 0 {
		if quote.Volume < volumeFloor {
			conviction -= ctrl.VolumeDowngrade
		} else if quote.Volume < 3000000 && quote.Last < quote.Open {
			conviction -= max(10, ctrl.VolumeDowngrade/2)
		}
	}

	if ctrl.HardRejectVolume > 0 && quote.Volume < ctrl.HardRejectVolume {
		return domain.Recommendation{}, false
	}

	minConviction := 60
	if ctrl.ConvictionFloor > 0 {
		minConviction = ctrl.ConvictionFloor
	}
	if conviction < minConviction {
		return domain.Recommendation{}, false
	}

	tp, slp := priceTargets(quote, 1.06, 0.95)
	return domain.Recommendation{
		Agent:         agent.ID,
		Skill:         agent.Skill,
		Layer:         agent.Layer,
		Symbol:        quote.Symbol,
		Side:          domain.SideBuy,
		Conviction:    conviction,
		Reason:        "semiconductor leadership and supply-chain role",
		TargetPrice:   tp,
		StopLossPrice: slp,
	}, true
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type AISupplyChainExecutor struct{}

func (AISupplyChainExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "ai_supply_chain_desk"
}

func (AISupplyChainExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
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
	tp, slp := priceTargets(quote, 1.08, 0.95)
	return domain.Recommendation{
		Agent:         agent.ID,
		Skill:         agent.Skill,
		Layer:         agent.Layer,
		Symbol:        quote.Symbol,
		Side:          domain.SideBuy,
		Conviction:    conviction,
		Reason:        "ai infrastructure order-flow sensitivity",
		TargetPrice:   tp,
		StopLossPrice: slp,
	}, true
}

type ETFRotationExecutor struct{}

func (ETFRotationExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "etf_rotation_desk"
}

func (ETFRotationExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	conviction := 64

	ctrl, ok := domain.ExtractPromptControl(prompt)
	if ok {
		if quote.Last < quote.Open {
			conviction -= 3
		}
		if ctrl.CloseStrengthBoost > 0 && quote.Last > quote.Open {
			conviction += ctrl.CloseStrengthBoost
		}
		if ctrl.VolumeBoost > 0 && quote.Volume > 8000000 {
			conviction += ctrl.VolumeBoost
		}
		minConviction := 55
		if ctrl.ConvictionFloor > 0 {
			minConviction = ctrl.ConvictionFloor
		}
		if ctrl.HardRejectVolume > 0 && quote.Volume < ctrl.HardRejectVolume {
			return domain.Recommendation{}, false
		}
		if conviction < minConviction {
			return domain.Recommendation{}, false
		}
		tp, slp := priceTargets(quote, 1.04, 0.97)
		return domain.Recommendation{
			Agent:         agent.ID,
			Skill:         agent.Skill,
			Layer:         agent.Layer,
			Symbol:        quote.Symbol,
			Side:          domain.SideBuy,
			Conviction:    conviction,
			Reason:        "broad ETF fallback under controlled risk",
			TargetPrice:   tp,
			StopLossPrice: slp,
		}, true
	}

	// Legacy fallback
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
	tp, slp := priceTargets(quote, 1.04, 0.97)
	return domain.Recommendation{
		Agent:         agent.ID,
		Skill:         agent.Skill,
		Layer:         agent.Layer,
		Symbol:        quote.Symbol,
		Side:          domain.SideBuy,
		Conviction:    conviction,
		Reason:        "broad ETF fallback under controlled risk",
		TargetPrice:   tp,
		StopLossPrice: slp,
	}, true
}
