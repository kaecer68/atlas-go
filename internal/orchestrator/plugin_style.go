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

const (
	vyCashFlowBoost    = 10
	vyYieldTrapPenalty = 10

	eqRepeatableBoost   = 9
	eqGuidancePenalty   = 8
	eqGuidanceThreshold = 0.99

	tbDefaultVolumeFloor     = 5_000_000
	tbStrictVolumeFloor      = 7_000_000
	tbRelaxedVolumeFloor     = 0
	tbLowVolumeFloor         = 3_000_000
	tbLowVolumeBoost         = 3
	tbRejectLowVolumeFloor   = 5_000_000
	tbVolumeBoost            = 8
	tbCloseStrengthPenalty   = 1
	tbCloseStrengthThreshold = 0.985
	tbCloseStrengthTolerance = 0.98
	tbSurgeBoost             = 4
	tbSurgePenalty           = 4
	tbOpenRejectionPenalty   = 10
	tbLateBreakoutPenalty    = 8
	tbLateBreakoutThreshold  = 0.998
	tbConfirmationBoost      = 12
	tbConfirmationThreshold  = 0.998
	tbCatchUpBoost           = 6
	tbCatchUpLowerThreshold  = 0.993
	tbCatchUpUpperThreshold  = 0.998
)

type ValueYieldExecutor struct{}

func (ValueYieldExecutor) Supports(agent domain.AgentSpec) bool { return agent.Skill == "value_yield" }

func (ValueYieldExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	cashFlowBoost := vyCashFlowBoost
	yieldTrapPenalty := vyYieldTrapPenalty
	if cfg := config.GetParametersConfig(); cfg != nil {
		vp := cfg.SectorExecutor.ValueYield
		if vp.CashFlowBoost.Value != 0 {
			cashFlowBoost = vp.CashFlowBoost.Value
			yieldTrapPenalty = vp.YieldTrapPenalty.Value
		}
	}
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), defaultConvictionFloor)
	if strings.Contains(prompt, "cash-flow support") && quote.Last >= quote.Open {
		b.add("cash_flow_boost", cashFlowBoost, "cash-flow support keyword + last >= open")
	}
	if strings.Contains(prompt, "yield trap") && quote.Last < quote.Open {
		b.add("yield_trap_penalty", -yieldTrapPenalty, "yield trap keyword + last < open")
	}
	if !b.floorCheck() {
		return domain.Recommendation{}, false
	}
	tp, slp := priceTargets(quote, 1.05, 0.96)
	conv, cb := b.build()
	return domain.Recommendation{Agent: agent.ID, Skill: agent.Skill, Layer: agent.Layer, Symbol: quote.Symbol, Side: domain.SideBuy, Conviction: conv, Reason: "defensive yield lens with valuation discipline", TargetPrice: tp, StopLossPrice: slp, ConvictionBreakdown: cb}, true
}

type EarningsQualityExecutor struct{}

func (EarningsQualityExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "earnings_quality"
}

func (EarningsQualityExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	repeatableBoost := eqRepeatableBoost
	guidancePenalty := eqGuidancePenalty
	guidanceThreshold := eqGuidanceThreshold
	if cfg := config.GetParametersConfig(); cfg != nil {
		ep := cfg.SectorExecutor.EarningsQuality
		if ep.RepeatableBoost.Value != 0 {
			repeatableBoost = ep.RepeatableBoost.Value
			guidancePenalty = ep.GuidancePenalty.Value
			guidanceThreshold = ep.GuidanceThreshold.Value
		}
	}
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), defaultConvictionFloor)
	if strings.Contains(prompt, "repeatable") && quote.Last > quote.Open {
		b.add("repeatable_boost", repeatableBoost, "repeatable keyword + last > open")
	}
	if strings.Contains(prompt, "guidance") && quote.Last < quote.High*guidanceThreshold {
		b.add("guidance_penalty", -guidancePenalty, "guidance keyword + last < high*threshold")
	}
	if !b.floorCheck() {
		return domain.Recommendation{}, false
	}
	tp, slp := priceTargets(quote, 1.06, 0.95)
	conv, cb := b.build()
	return domain.Recommendation{Agent: agent.ID, Skill: agent.Skill, Layer: agent.Layer, Symbol: quote.Symbol, Side: domain.SideBuy, Conviction: conv, Reason: "earnings quality and forward visibility support", TargetPrice: tp, StopLossPrice: slp, ConvictionBreakdown: cb}, true
}

type TechnicalBreakoutExecutor struct{}

type tbRuntimeParams struct {
	defaultVolumeFloor, strictVolumeFloor, relaxedVolumeFloor, lowVolumeFloor, rejectLowVolumeFloor                                                         int64
	lowVolumeBoost, volumeBoost, closeStrengthPenalty, surgeBoost, surgePenalty, openRejectionPenalty, lateBreakoutPenalty, confirmationBoost, catchUpBoost int
	closeStrengthThreshold, closeStrengthTolerance, lateBreakoutThreshold, confirmationThreshold, catchUpLowerThreshold, catchUpUpperThreshold              float64
}

func loadTBParams() tbRuntimeParams {
	p := tbRuntimeParams{tbDefaultVolumeFloor, tbStrictVolumeFloor, tbRelaxedVolumeFloor, tbLowVolumeFloor, tbRejectLowVolumeFloor, tbLowVolumeBoost, tbVolumeBoost, tbCloseStrengthPenalty, tbSurgeBoost, tbSurgePenalty, tbOpenRejectionPenalty, tbLateBreakoutPenalty, tbConfirmationBoost, tbCatchUpBoost, tbCloseStrengthThreshold, tbCloseStrengthTolerance, tbLateBreakoutThreshold, tbConfirmationThreshold, tbCatchUpLowerThreshold, tbCatchUpUpperThreshold}
	if cfg := config.GetParametersConfig(); cfg != nil {
		tp := cfg.SectorExecutor.TechnicalBreakout
		if tp.DefaultVolumeFloor.Value != 0 {
			p.defaultVolumeFloor = tp.DefaultVolumeFloor.Value
			p.strictVolumeFloor = tp.StrictVolumeFloor.Value
			p.relaxedVolumeFloor = tp.RelaxedVolumeFloor.Value
			p.lowVolumeFloor = tp.LowVolumeFloor.Value
			p.lowVolumeBoost = tp.LowVolumeBoost.Value
			p.rejectLowVolumeFloor = tp.RejectLowVolumeFloor.Value
			p.volumeBoost = tp.VolumeBoost.Value
			p.closeStrengthPenalty = tp.CloseStrengthPenalty.Value
			p.closeStrengthThreshold = tp.CloseStrengthThreshold.Value
			p.closeStrengthTolerance = tp.CloseStrengthTolerance.Value
			p.surgeBoost = tp.SurgeBoost.Value
			p.surgePenalty = tp.SurgePenalty.Value
			p.openRejectionPenalty = tp.OpenRejectionPenalty.Value
			p.lateBreakoutPenalty = tp.LateBreakoutPenalty.Value
			p.lateBreakoutThreshold = tp.LateBreakoutThreshold.Value
			p.confirmationBoost = tp.ConfirmationBoost.Value
			p.confirmationThreshold = tp.ConfirmationThreshold.Value
			p.catchUpBoost = tp.CatchUpBoost.Value
			p.catchUpLowerThreshold = tp.CatchUpLowerThreshold.Value
			p.catchUpUpperThreshold = tp.CatchUpUpperThreshold.Value
		}
	}
	return p
}

func (TechnicalBreakoutExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "technical_breakout"
}

func (TechnicalBreakoutExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	p := loadTBParams()
	volumeFloor := tbVolumeFloor(prompt, p)
	conviction, cb := tbConviction(agent, prompt, quote, volumeFloor, p)
	if tbReject(prompt, quote, volumeFloor, conviction, p) {
		return domain.Recommendation{}, false
	}
	tp, slp := priceTargets(quote, 1.10, 0.94)
	return domain.Recommendation{Agent: agent.ID, Skill: agent.Skill, Layer: agent.Layer, Symbol: quote.Symbol, Side: domain.SideBuy, Conviction: conviction, Reason: "breakout structure confirmed by volume and close", TargetPrice: tp, StopLossPrice: slp, ConvictionBreakdown: cb}, true
}

func tbVolumeFloor(prompt string, p tbRuntimeParams) int64 {
	if strings.Contains(prompt, "coverage expansion") {
		return p.relaxedVolumeFloor
	}
	if strings.Contains(prompt, "volume surge requirement") {
		return p.strictVolumeFloor
	}
	return p.defaultVolumeFloor
}

func tbConviction(agent domain.AgentSpec, prompt string, quote domain.Quote, volumeFloor int64, p tbRuntimeParams) (int, *domain.ConvictionBreakdown) {
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), defaultConvictionFloor)
	if strings.Contains(prompt, "volume") && quote.Volume >= p.defaultVolumeFloor {
		b.add("volume_boost", p.volumeBoost, "volume keyword + vol >= floor")
	}
	if strings.Contains(prompt, "close strength") && quote.Last < quote.High*p.closeStrengthThreshold {
		penalty := p.closeStrengthPenalty
		if strings.Contains(prompt, "close-strength tolerance") && quote.Last >= quote.High*p.closeStrengthTolerance {
			penalty = 0
		}
		if penalty != 0 {
			b.add("close_strength_penalty", -penalty, "last < high*threshold")
		}
	}
	if strings.Contains(prompt, "volume surge requirement") {
		if quote.Volume >= volumeFloor {
			b.add("surge_boost", p.surgeBoost, fmt.Sprintf("vol >= %d", volumeFloor))
		} else {
			b.add("surge_penalty", -p.surgePenalty, fmt.Sprintf("vol < %d", volumeFloor))
		}
	}
	if strings.Contains(prompt, "structure-first breakout filter") && quote.Last < quote.Open {
		b.add("open_rejection_penalty", -p.openRejectionPenalty, "last < open")
	}
	if strings.Contains(prompt, "late-breakout penalty") && quote.Last < quote.High*p.lateBreakoutThreshold {
		b.add("late_breakout_penalty", -p.lateBreakoutPenalty, "last < high*threshold")
	}
	if strings.Contains(prompt, "breakout confirmation bonus") && quote.Last >= quote.High*p.confirmationThreshold && quote.Volume >= p.defaultVolumeFloor {
		b.add("confirmation_boost", p.confirmationBoost, "last >= high*threshold + vol >= floor")
	}
	if strings.Contains(prompt, "catch-up momentum") && quote.Last >= quote.High*p.catchUpLowerThreshold && quote.Last < quote.High*p.catchUpUpperThreshold && quote.Last >= quote.Open {
		b.add("catch_up_boost", p.catchUpBoost, "catch-up momentum zone")
	}
	if strings.Contains(prompt, "volume participation acceptance") && quote.Volume >= p.lowVolumeFloor && quote.Volume < p.defaultVolumeFloor {
		b.add("low_volume_boost", p.lowVolumeBoost, "low volume range")
	}
	return b.build()
}

func tbReject(prompt string, quote domain.Quote, volumeFloor int64, conviction int, p tbRuntimeParams) bool {
	if strings.Contains(prompt, "reject low volume") && quote.Volume < p.rejectLowVolumeFloor {
		return true
	}
	if strings.Contains(prompt, "enforce strict breakout confirmation") && quote.Volume < volumeFloor {
		return true
	}
	return conviction < defaultConvictionFloor
}
