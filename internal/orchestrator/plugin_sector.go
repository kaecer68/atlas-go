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
	conviction := dynamicSignalStrength(quote, signalParamsFromAgent(agent))

	ctrl, ok := domain.ExtractPromptControl(prompt)
	if !ok {
		if strings.Contains(prompt, "close strength confirms leadership") && quote.Last > quote.Open {
			conviction += 10
		}
		if strings.Contains(prompt, "weak volume") {
			conviction -= 15
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

	if ctrl.CloseStrengthBoost > 0 && quote.Last > quote.Open {
		conviction += ctrl.CloseStrengthBoost
	}

	if ctrl.VolumeDowngrade > 0 && quote.Last < quote.Open {
		conviction -= max(10, ctrl.VolumeDowngrade/2)
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

type signalParams struct {
	volumeThreshold int64
	priceBoost      int
	volumeBoost     int
	maxCap          int
}

func defaultSignalParams() signalParams {
	return signalParams{
		volumeThreshold: 5_000_000,
		priceBoost:      5,
		volumeBoost:     5,
		maxCap:          75,
	}
}

func signalParamsFromAgent(agent domain.AgentSpec) signalParams {
	params := defaultSignalParams()
	if agent.ScreeningCriteria.VolumeIntraday != nil && agent.ScreeningCriteria.VolumeIntraday.Min != nil {
		params.volumeThreshold = *agent.ScreeningCriteria.VolumeIntraday.Min
	}
	return params
}

func dynamicSignalStrength(quote domain.Quote, params signalParams) int {
	conviction := 60
	if quote.Last > quote.Open {
		conviction += params.priceBoost
	}
	if quote.Volume > params.volumeThreshold {
		conviction += params.volumeBoost
	}
	if conviction > params.maxCap {
		conviction = params.maxCap
	}
	return conviction
}

type AISupplyChainExecutor struct{}

func (AISupplyChainExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "ai_supply_chain_desk"
}

func (AISupplyChainExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	conviction := dynamicSignalStrength(quote, signalParamsFromAgent(agent))
	if quote.Last < quote.Open {
		conviction -= 5
	}
	if strings.Contains(prompt, "order-flow") && quote.Last > quote.Open {
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
	conviction := dynamicSignalStrength(quote, signalParamsFromAgent(agent))

	ctrl, ok := domain.ExtractPromptControl(prompt)
	if ok {
		if quote.Last < quote.Open {
			conviction -= 3
		}
		if ctrl.CloseStrengthBoost > 0 && quote.Last > quote.Open {
			conviction += ctrl.CloseStrengthBoost
		}
		if ctrl.VolumeBoost > 0 && quote.Last > quote.Open {
			conviction += ctrl.VolumeBoost
		}
		minConviction := 55
		if ctrl.ConvictionFloor > 0 {
			minConviction = ctrl.ConvictionFloor
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
	if strings.Contains(prompt, "sector leadership") {
		conviction += 5
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
