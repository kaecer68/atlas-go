package orchestrator

import (
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type GrowthMomentumExecutor struct{}

func (GrowthMomentumExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "growth_momentum"
}

func (GrowthMomentumExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	conviction := dynamicSignalStrength(quote, signalParamsFromAgent(agent))

	ctrl, ok := domain.ExtractPromptControl(prompt)
	if ok {
		if quote.Last < quote.Open {
			conviction -= 8
		}
		penalty := 0
		if ctrl.RequireTrend {
			if quote.Last < quote.Open {
				penalty += 8
			}
		}
		if ctrl.VolumeDowngrade > 0 {
			if quote.Last < quote.High*0.995 {
				penalty += max(5, ctrl.VolumeDowngrade/2)
			}
			if quote.Last < quote.Open {
				penalty += max(3, ctrl.VolumeDowngrade/3)
			}
		}
		if regime == domain.RegimeNeutral && ctrl.NeutralPenaltyReduction > 0 {
			penalty = max(0, penalty-ctrl.NeutralPenaltyReduction)
		}
		conviction -= penalty
		if ctrl.CloseStrengthBoost > 0 && quote.Last > quote.Open {
			conviction += ctrl.CloseStrengthBoost
		}
		if ctrl.VolumeBoost > 0 && quote.Last > quote.Open {
			conviction += ctrl.VolumeBoost
		}
		minConviction := 45
		if ctrl.ConvictionFloor > 0 {
			minConviction = ctrl.ConvictionFloor
		}
		if conviction < minConviction {
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
			Reason:        "price persistence with style overlay",
			TargetPrice:   tp,
			StopLossPrice: slp,
		}, true
	}

	// Legacy fallback
	if quote.Last < quote.Open {
		conviction -= 8
	}
	if strings.Contains(prompt, "require trend confirmation") {
		if quote.Last < quote.Open {
			conviction -= 15
		}
	}
	if strings.Contains(prompt, "downgrade conviction") {
		pricePenalty := 12
		openPenalty := 8
		if strings.Contains(prompt, "exploratory mode") {
			pricePenalty = 6
			openPenalty = 4
		}
		if quote.Last < quote.High*0.995 {
			conviction -= pricePenalty
		}
		if quote.Last < quote.Open {
			conviction -= openPenalty
		}
	}
	if conviction < 45 {
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
		Reason:        "price persistence with style overlay",
		TargetPrice:   tp,
		StopLossPrice: slp,
	}, true
}
