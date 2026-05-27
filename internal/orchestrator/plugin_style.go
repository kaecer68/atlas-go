package orchestrator

import (
	"fmt"
	"strings"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

type GrowthMomentumExecutor struct{}

func (GrowthMomentumExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "growth_momentum"
}

func (GrowthMomentumExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), 45)

	ctrl, ok := domain.ExtractPromptControl(prompt)
	if ok {
		if quote.Last < quote.Open {
			b.add("price_penalty", -8, "last < open")
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
		if penalty > 0 {
			b.add("control_penalty", -penalty, "prompt control penalties")
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
		tp, slp := priceTargets(quote, 1.08, 0.95)
		conv, cb := b.build()
		return domain.Recommendation{
			Agent:               agent.ID,
			Skill:               agent.Skill,
			Layer:               agent.Layer,
			Symbol:              quote.Symbol,
			Side:                domain.SideBuy,
			Conviction:          conv,
			Reason:              "price persistence with style overlay",
			TargetPrice:         tp,
			StopLossPrice:       slp,
			ConvictionBreakdown: cb,
		}, true
	}

	if quote.Last < quote.Open {
		b.add("price_penalty", -8, "last < open")
	}
	if strings.Contains(prompt, "require trend confirmation") {
		if quote.Last < quote.Open {
			b.add("trend_confirmation_penalty", -15, "require trend confirmation + last < open")
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
			b.add("downgrade_price_penalty", -pricePenalty, "last < high*0.995")
		}
		if quote.Last < quote.Open {
			b.add("downgrade_open_penalty", -openPenalty, "last < open")
		}
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
		Reason:              "price persistence with style overlay",
		TargetPrice:         tp,
		StopLossPrice:       slp,
		ConvictionBreakdown: cb,
	}, true
}

type ValueYieldExecutor struct{}

func (ValueYieldExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "value_yield"
}

func (ValueYieldExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), defaultConvictionFloor)
	if strings.Contains(prompt, "cash-flow support") && quote.Last >= quote.Open {
		b.add("cash_flow_boost", vyCashFlowBoost, "cash-flow support keyword + last >= open")
	}
	if strings.Contains(prompt, "yield trap") && quote.Last < quote.Open {
		b.add("yield_trap_penalty", -vyYieldTrapPenalty, "yield trap keyword + last < open")
	}
	if !b.floorCheck() {
		return domain.Recommendation{}, false
	}
	tp, slp := priceTargets(quote, 1.05, 0.96)
	conv, cb := b.build()
	return domain.Recommendation{
		Agent:               agent.ID,
		Skill:               agent.Skill,
		Layer:               agent.Layer,
		Symbol:              quote.Symbol,
		Side:                domain.SideBuy,
		Conviction:          conv,
		Reason:              "defensive yield lens with valuation discipline",
		TargetPrice:         tp,
		StopLossPrice:       slp,
		ConvictionBreakdown: cb,
	}, true
}

type EarningsQualityExecutor struct{}

func (EarningsQualityExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "earnings_quality"
}

func (EarningsQualityExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), defaultConvictionFloor)
	if strings.Contains(prompt, "repeatable") && quote.Last > quote.Open {
		b.add("repeatable_boost", eqRepeatableBoost, "repeatable keyword + last > open")
	}
	if strings.Contains(prompt, "guidance") && quote.Last < quote.High*eqGuidanceThreshold {
		b.add("guidance_penalty", -eqGuidancePenalty, "guidance keyword + last < high*0.99")
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
		Reason:              "earnings quality and forward visibility support",
		TargetPrice:         tp,
		StopLossPrice:       slp,
		ConvictionBreakdown: cb,
	}, true
}

type TechnicalBreakoutExecutor struct{}

func (TechnicalBreakoutExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "technical_breakout"
}

func (TechnicalBreakoutExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	volumeFloor := tbVolumeFloor(prompt)
	conviction, cb := tbConviction(agent, prompt, quote, volumeFloor)
	if tbReject(prompt, quote, volumeFloor, conviction) {
		return domain.Recommendation{}, false
	}
	tp, slp := priceTargets(quote, 1.10, 0.94)
	return domain.Recommendation{
		Agent:               agent.ID,
		Skill:               agent.Skill,
		Layer:               agent.Layer,
		Symbol:              quote.Symbol,
		Side:                domain.SideBuy,
		Conviction:          conviction,
		Reason:              "breakout structure confirmed by volume and close",
		TargetPrice:         tp,
		StopLossPrice:       slp,
		ConvictionBreakdown: cb,
	}, true
}

func tbVolumeFloor(prompt string) int64 {
	if strings.Contains(prompt, "coverage expansion") {
		return tbRelaxedVolumeFloor
	}
	if strings.Contains(prompt, "volume surge requirement") {
		return tbStrictVolumeFloor
	}
	return tbDefaultVolumeFloor
}

func tbConviction(agent domain.AgentSpec, prompt string, quote domain.Quote, volumeFloor int64) (int, *domain.ConvictionBreakdown) {
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), defaultConvictionFloor)
	if strings.Contains(prompt, "volume") && quote.Volume >= tbDefaultVolumeFloor {
		b.add("volume_boost", tbVolumeBoost, "volume keyword + vol >= 5M")
	}
	if strings.Contains(prompt, "close strength") && quote.Last < quote.High*tbCloseStrengthThreshold {
		penalty := tbCloseStrengthPenalty
		if strings.Contains(prompt, "close-strength tolerance") && quote.Last >= quote.High*tbCloseStrengthTolerance {
			penalty = 0
		}
		if penalty != 0 {
			b.add("close_strength_penalty", -penalty, "last < high*0.985")
		}
	}
	if strings.Contains(prompt, "volume surge requirement") {
		if quote.Volume >= volumeFloor {
			b.add("surge_boost", tbSurgeBoost, fmt.Sprintf("vol >= %d", volumeFloor))
		} else {
			b.add("surge_penalty", -tbSurgePenalty, fmt.Sprintf("vol < %d", volumeFloor))
		}
	}
	if strings.Contains(prompt, "structure-first breakout filter") && quote.Last < quote.Open {
		b.add("open_rejection_penalty", -tbOpenRejectionPenalty, "last < open")
	}
	if strings.Contains(prompt, "late-breakout penalty") && quote.Last < quote.High*tbLateBreakoutThreshold {
		b.add("late_breakout_penalty", -tbLateBreakoutPenalty, "last < high*0.998")
	}
	if strings.Contains(prompt, "breakout confirmation bonus") && quote.Last >= quote.High*tbConfirmationThreshold && quote.Volume >= tbDefaultVolumeFloor {
		b.add("confirmation_boost", tbConfirmationBoost, "last >= high*0.998 + vol >= 5M")
	}
	if strings.Contains(prompt, "catch-up momentum") && quote.Last >= quote.High*tbCatchUpLowerThreshold && quote.Last < quote.High*tbCatchUpUpperThreshold && quote.Last >= quote.Open {
		b.add("catch_up_boost", tbCatchUpBoost, "catch-up momentum zone")
	}
	if strings.Contains(prompt, "volume participation acceptance") && quote.Volume >= tbLowVolumeFloor && quote.Volume < tbDefaultVolumeFloor {
		b.add("low_volume_boost", tbLowVolumeBoost, "3M <= vol < 5M")
	}
	return b.build()
}

func tbReject(prompt string, quote domain.Quote, volumeFloor int64, conviction int) bool {
	if strings.Contains(prompt, "reject low volume") && quote.Volume < tbRejectLowVolumeFloor {
		return true
	}
	if strings.Contains(prompt, "enforce strict breakout confirmation") && quote.Volume < volumeFloor {
		return true
	}
	return conviction < defaultConvictionFloor
}
