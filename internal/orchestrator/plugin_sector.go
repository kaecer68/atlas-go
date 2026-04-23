package orchestrator

import (
	"fmt"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type SemiconductorExecutor struct{}

func (SemiconductorExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "semiconductor_desk"
}

func (SemiconductorExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), 60)

	ctrl, ok := domain.ExtractPromptControl(prompt)
	if !ok {
		if strings.Contains(prompt, "close strength confirms leadership") && quote.Last > quote.Open {
			b.add("prompt_boost", 10, "close strength confirms leadership")
		}
		if strings.Contains(prompt, "weak volume") {
			b.add("prompt_penalty", -15, "weak volume")
		}
		if !b.floorCheck() {
			return domain.Recommendation{}, false
		}
		tp, slp := priceTargets(quote, 1.06, 0.95)
		conv, cb := b.build()
		return domain.Recommendation{
			Agent:               agent.ID,
			Skill:               agent.Skill,
			Layer:               agent.Layer,
			Symbol:              quote.Symbol,
			Side:                domain.SideBuy,
			Conviction:          conv,
			Reason:              "semiconductor leadership and supply-chain role",
			TargetPrice:         tp,
			StopLossPrice:       slp,
			ConvictionBreakdown: cb,
		}, true
	}

	if ctrl.CloseStrengthBoost > 0 && quote.Last > quote.Open {
		b.add("close_strength_boost", ctrl.CloseStrengthBoost, "last > open")
	}

	if ctrl.VolumeDowngrade > 0 && quote.Last < quote.Open {
		b.add("volume_downgrade", -max(10, ctrl.VolumeDowngrade/2), "last < open")
	}

	minConviction := 60
	if ctrl.ConvictionFloor > 0 {
		b.floor = ctrl.ConvictionFloor
		minConviction = ctrl.ConvictionFloor
	}
	if b.final < minConviction {
		b.add("floor", minConviction-b.final, fmt.Sprintf("below floor %d", minConviction))
		b.final = minConviction
		return domain.Recommendation{}, false
	}

	tp, slp := priceTargets(quote, 1.06, 0.95)
	conv, cb := b.build()
	return domain.Recommendation{
		Agent:               agent.ID,
		Skill:               agent.Skill,
		Layer:               agent.Layer,
		Symbol:              quote.Symbol,
		Side:                domain.SideBuy,
		Conviction:          conv,
		Reason:              "semiconductor leadership and supply-chain role",
		TargetPrice:         tp,
		StopLossPrice:       slp,
		ConvictionBreakdown: cb,
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
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), 60)
	if quote.Last < quote.Open {
		b.add("price_penalty", -5, "last < open")
	}
	if strings.Contains(prompt, "order-flow") && quote.Last > quote.Open {
		b.add("order_flow_boost", 8, "order-flow keyword + last > open")
	}
	if strings.Contains(prompt, "downgrade") && quote.Last < quote.High*0.99 {
		b.add("downgrade_penalty", -10, "downgrade keyword + last < high*0.99")
	}
	if !b.floorCheck() {
		return domain.Recommendation{}, false
	}
	tp, slp := priceTargets(quote, 1.08, 0.95)
	conv, cb := b.build()
	return domain.Recommendation{
		Agent:               agent.ID,
		Skill:               agent.Skill,
		Layer:               agent.Layer,
		Symbol:              quote.Symbol,
		Side:                domain.SideBuy,
		Conviction:          conv,
		Reason:              "ai infrastructure order-flow sensitivity",
		TargetPrice:         tp,
		StopLossPrice:       slp,
		ConvictionBreakdown: cb,
	}, true
}

type ETFRotationExecutor struct{}

func (ETFRotationExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "etf_rotation_desk"
}

func (ETFRotationExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), 55)

	ctrl, ok := domain.ExtractPromptControl(prompt)
	if ok {
		if quote.Last < quote.Open {
			b.add("price_penalty", -3, "last < open")
		}
		if ctrl.CloseStrengthBoost > 0 && quote.Last > quote.Open {
			b.add("close_strength_boost", ctrl.CloseStrengthBoost, "last > open")
		}
		if ctrl.VolumeBoost > 0 && quote.Last > quote.Open {
			b.add("volume_boost", ctrl.VolumeBoost, "last > open")
		}
		if ctrl.ConvictionFloor > 0 {
			b.floor = ctrl.ConvictionFloor
		}
		if !b.floorCheck() {
			return domain.Recommendation{}, false
		}
		tp, slp := priceTargets(quote, 1.04, 0.97)
		conv, cb := b.build()
		return domain.Recommendation{
			Agent:               agent.ID,
			Skill:               agent.Skill,
			Layer:               agent.Layer,
			Symbol:              quote.Symbol,
			Side:                domain.SideBuy,
			Conviction:          conv,
			Reason:              "broad ETF fallback under controlled risk",
			TargetPrice:         tp,
			StopLossPrice:       slp,
			ConvictionBreakdown: cb,
		}, true
	}

	if quote.Last < quote.Open {
		b.add("price_penalty", -3, "last < open")
	}
	if strings.Contains(prompt, "rotation") && quote.Last > quote.Open {
		b.add("rotation_boost", 6, "rotation keyword + last > open")
	}
	if strings.Contains(prompt, "sector leadership") {
		b.add("sector_leadership_boost", 5, "sector leadership keyword")
	}
	if !b.floorCheck() {
		return domain.Recommendation{}, false
	}
	tp, slp := priceTargets(quote, 1.04, 0.97)
	conv, cb := b.build()
	return domain.Recommendation{
		Agent:               agent.ID,
		Skill:               agent.Skill,
		Layer:               agent.Layer,
		Symbol:              quote.Symbol,
		Side:                domain.SideBuy,
		Conviction:          conv,
		Reason:              "broad ETF fallback under controlled risk",
		TargetPrice:         tp,
		StopLossPrice:       slp,
		ConvictionBreakdown: cb,
	}, true
}
