package orchestrator

import (
	"fmt"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
)

const defaultConvictionFloor = 50

func priceTargets(quote domain.Quote, targetMult, stopLossMult float64) (float64, float64) {
	return quote.Last * targetMult, quote.Last * stopLossMult
}

const (
	finDividendBoost            = 8
	finBalanceSheetPenalty      = 6
	finCreditQualityBoost       = 2
	finCreditQualityPenalty     = 6
	finSpreadSensitivityBoost   = 2
	finSpreadSensitivityPenalty = 4
	finCapitalAdequacyBoost     = 3
	finPriceToOpenThreshold     = 0.985
	finPriceToHighThreshold     = 0.995
)

const (
	shipTacticalBoost      = 10
	shipWeakClosePenalty   = 12
	shipWeakCloseThreshold = 0.992
)

const (
	vyCashFlowBoost    = 10
	vyYieldTrapPenalty = 10
)

const (
	eqRepeatableBoost   = 9
	eqGuidancePenalty   = 8
	eqGuidanceThreshold = 0.99
)

const (
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

type FinancialsExecutor struct{}

func (FinancialsExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "financials_desk"
}

func (FinancialsExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	conviction, cb := finConviction(agent, prompt, quote)
	if conviction < 50 {
		return domain.Recommendation{}, false
	}
	tp, slp := priceTargets(quote, 1.05, 0.96)
	return domain.Recommendation{
		Agent:               agent.ID,
		Skill:               agent.Skill,
		Layer:               agent.Layer,
		Symbol:              quote.Symbol,
		Side:                domain.SideBuy,
		Conviction:          conviction,
		Reason:              "financial carry with resilient balance-sheet posture",
		TargetPrice:         tp,
		StopLossPrice:       slp,
		ConvictionBreakdown: cb,
	}, true
}

func finConviction(agent domain.AgentSpec, prompt string, quote domain.Quote) (int, *domain.ConvictionBreakdown) {
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), 50)
	if strings.Contains(prompt, "dividend") && quote.Last >= quote.Open {
		b.add("dividend_boost", finDividendBoost, "dividend keyword + last >= open")
	}
	if strings.Contains(prompt, "balance-sheet") && quote.Low < quote.Open*finPriceToOpenThreshold {
		b.add("balance_sheet_penalty", -finBalanceSheetPenalty, "balance-sheet keyword + low < open*0.985")
	}
	if strings.Contains(prompt, "credit quality gate") {
		if quote.Last >= quote.Open {
			b.add("credit_quality_boost", finCreditQualityBoost, "credit quality gate + last >= open")
		} else {
			b.add("credit_quality_penalty", -finCreditQualityPenalty, "credit quality gate + last < open")
		}
	}
	if strings.Contains(prompt, "spread sensitivity downgrade") {
		if quote.Last >= quote.High*finPriceToHighThreshold {
			b.add("spread_sensitivity_boost", finSpreadSensitivityBoost, "last >= high*0.995")
		} else {
			b.add("spread_sensitivity_penalty", -finSpreadSensitivityPenalty, "last < high*0.995")
		}
	}
	if strings.Contains(prompt, "capital adequacy premium") && quote.Last >= quote.Open && quote.Last >= quote.High*finPriceToHighThreshold {
		b.add("capital_adequacy_boost", finCapitalAdequacyBoost, "capital adequacy premium + last >= open & high*0.995")
	}
	return b.build()
}

type ShippingExecutor struct{}

func (ShippingExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "shipping_desk"
}

func (ShippingExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), defaultConvictionFloor)
	if strings.Contains(prompt, "tactical") && quote.Last > quote.Open {
		b.add("tactical_boost", shipTacticalBoost, "tactical keyword + last > open")
	}
	if strings.Contains(prompt, "avoid weak closes") && quote.Last < quote.High*shipWeakCloseThreshold {
		b.add("weak_close_penalty", -shipWeakClosePenalty, "avoid weak closes + last < high*0.992")
	}
	if !b.floorCheck() {
		return domain.Recommendation{}, false
	}
	tp, slp := priceTargets(quote, 1.07, 0.94)
	conv, cb := b.build()
	return domain.Recommendation{
		Agent:               agent.ID,
		Skill:               agent.Skill,
		Layer:               agent.Layer,
		Symbol:              quote.Symbol,
		Side:                domain.SideBuy,
		Conviction:          conv,
		Reason:              "shipping beta used as tactical cycle exposure",
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
