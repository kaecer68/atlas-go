package orchestrator

import (
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// Common conviction thresholds used across executors.
const (
	defaultConvictionFloor = 50
)

// priceTargets returns reasonable target and stop-loss prices based on current quote.
// Multipliers vary by strategy volatility profile.
func priceTargets(quote domain.Quote, targetMult, stopLossMult float64) (float64, float64) {
	return quote.Last * targetMult, quote.Last * stopLossMult
}

// Financials desk thresholds.
const (
	finBaseConviction           = 58
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

// Shipping desk thresholds.
const (
	shipBaseConviction     = 55
	shipTacticalBoost      = 10
	shipWeakClosePenalty   = 12
	shipWeakCloseThreshold = 0.992
)

// Value-yield desk thresholds.
const (
	vyBaseConviction   = 52
	vyCashFlowBoost    = 10
	vyYieldTrapPenalty = 10
)

// Earnings quality desk thresholds.
const (
	eqBaseConviction    = 57
	eqRepeatableBoost   = 9
	eqGuidancePenalty   = 8
	eqGuidanceThreshold = 0.99
)

// Technical breakout desk thresholds.
const (
	tbBaseConviction         = 54
	tbDefaultVolumeFloor     = 5_000_000
	tbStrictVolumeFloor      = 7_000_000
	tbRelaxedVolumeFloor     = 0
	tbLowVolumeFloor         = 3_000_000
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
	tbLowVolumeBoost         = 3
	tbRejectLowVolumeFloor   = 5_000_000
)

type FinancialsExecutor struct{}

func (FinancialsExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "financials_desk"
}

func (FinancialsExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	conviction := finConviction(prompt, quote)
	if conviction < 50 {
		return domain.Recommendation{}, false
	}
	tp, slp := priceTargets(quote, 1.05, 0.96)
	return domain.Recommendation{
		Agent:         agent.ID,
		Skill:         agent.Skill,
		Layer:         agent.Layer,
		Symbol:        quote.Symbol,
		Side:          domain.SideBuy,
		Conviction:    conviction,
		Reason:        "financial carry with resilient balance-sheet posture",
		TargetPrice:   tp,
		StopLossPrice: slp,
	}, true
}

func finConviction(prompt string, quote domain.Quote) int {
	conviction := finBaseConviction
	if strings.Contains(prompt, "dividend") && quote.Last >= quote.Open {
		conviction += finDividendBoost
	}
	if strings.Contains(prompt, "balance-sheet") && quote.Low < quote.Open*finPriceToOpenThreshold {
		conviction -= finBalanceSheetPenalty
	}
	if strings.Contains(prompt, "credit quality gate") {
		if quote.Last >= quote.Open {
			conviction += finCreditQualityBoost
		} else {
			conviction -= finCreditQualityPenalty
		}
	}
	if strings.Contains(prompt, "spread sensitivity downgrade") {
		if quote.Last >= quote.High*finPriceToHighThreshold {
			conviction += finSpreadSensitivityBoost
		} else {
			conviction -= finSpreadSensitivityPenalty
		}
	}
	if strings.Contains(prompt, "capital adequacy premium") && quote.Last >= quote.Open && quote.Last >= quote.High*finPriceToHighThreshold {
		conviction += finCapitalAdequacyBoost
	}
	return conviction
}

type ShippingExecutor struct{}

func (ShippingExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "shipping_desk"
}

func (ShippingExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	conviction := shipBaseConviction
	if strings.Contains(prompt, "tactical") && quote.Last > quote.Open {
		conviction += shipTacticalBoost
	}
	if strings.Contains(prompt, "avoid weak closes") && quote.Last < quote.High*shipWeakCloseThreshold {
		conviction -= shipWeakClosePenalty
	}
	if conviction < defaultConvictionFloor {
		return domain.Recommendation{}, false
	}
	tp, slp := priceTargets(quote, 1.07, 0.94)
	return domain.Recommendation{
		Agent:         agent.ID,
		Skill:         agent.Skill,
		Layer:         agent.Layer,
		Symbol:        quote.Symbol,
		Side:          domain.SideBuy,
		Conviction:    conviction,
		Reason:        "shipping beta used as tactical cycle exposure",
		TargetPrice:   tp,
		StopLossPrice: slp,
	}, true
}

type ValueYieldExecutor struct{}

func (ValueYieldExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "value_yield"
}

func (ValueYieldExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	conviction := vyBaseConviction
	if strings.Contains(prompt, "cash-flow support") && quote.Last >= quote.Open {
		conviction += vyCashFlowBoost
	}
	if strings.Contains(prompt, "yield trap") && quote.Last < quote.Open {
		conviction -= vyYieldTrapPenalty
	}
	if conviction < defaultConvictionFloor {
		return domain.Recommendation{}, false
	}
	tp, slp := priceTargets(quote, 1.05, 0.96)
	return domain.Recommendation{
		Agent:         agent.ID,
		Skill:         agent.Skill,
		Layer:         agent.Layer,
		Symbol:        quote.Symbol,
		Side:          domain.SideBuy,
		Conviction:    conviction,
		Reason:        "defensive yield lens with valuation discipline",
		TargetPrice:   tp,
		StopLossPrice: slp,
	}, true
}

type EarningsQualityExecutor struct{}

func (EarningsQualityExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "earnings_quality"
}

func (EarningsQualityExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	conviction := eqBaseConviction
	if strings.Contains(prompt, "repeatable") && quote.Last > quote.Open {
		conviction += eqRepeatableBoost
	}
	if strings.Contains(prompt, "guidance") && quote.Last < quote.High*eqGuidanceThreshold {
		conviction -= eqGuidancePenalty
	}
	if conviction < defaultConvictionFloor {
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
		Reason:        "earnings quality and forward visibility support",
		TargetPrice:   tp,
		StopLossPrice: slp,
	}, true
}

type TechnicalBreakoutExecutor struct{}

func (TechnicalBreakoutExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "technical_breakout"
}

func (TechnicalBreakoutExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime) (domain.Recommendation, bool) {
	volumeFloor := tbVolumeFloor(prompt)
	conviction := tbConviction(prompt, quote, volumeFloor)
	if tbReject(prompt, quote, volumeFloor, conviction) {
		return domain.Recommendation{}, false
	}
	tp, slp := priceTargets(quote, 1.10, 0.94)
	return domain.Recommendation{
		Agent:         agent.ID,
		Skill:         agent.Skill,
		Layer:         agent.Layer,
		Symbol:        quote.Symbol,
		Side:          domain.SideBuy,
		Conviction:    conviction,
		Reason:        "breakout structure confirmed by volume and close",
		TargetPrice:   tp,
		StopLossPrice: slp,
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

func tbConviction(prompt string, quote domain.Quote, volumeFloor int64) int {
	conviction := tbBaseConviction
	if strings.Contains(prompt, "volume") && quote.Volume >= tbDefaultVolumeFloor {
		conviction += tbVolumeBoost
	}
	if strings.Contains(prompt, "close strength") && quote.Last < quote.High*tbCloseStrengthThreshold {
		penalty := tbCloseStrengthPenalty
		if strings.Contains(prompt, "close-strength tolerance") && quote.Last >= quote.High*tbCloseStrengthTolerance {
			penalty = 0
		}
		conviction -= penalty
	}
	if strings.Contains(prompt, "volume surge requirement") {
		if quote.Volume >= volumeFloor {
			conviction += tbSurgeBoost
		} else {
			conviction -= tbSurgePenalty
		}
	}
	if strings.Contains(prompt, "structure-first breakout filter") && quote.Last < quote.Open {
		conviction -= tbOpenRejectionPenalty
	}
	if strings.Contains(prompt, "late-breakout penalty") && quote.Last < quote.High*tbLateBreakoutThreshold {
		conviction -= tbLateBreakoutPenalty
	}
	if strings.Contains(prompt, "breakout confirmation bonus") && quote.Last >= quote.High*tbConfirmationThreshold && quote.Volume >= tbDefaultVolumeFloor {
		conviction += tbConfirmationBoost
	}
	if strings.Contains(prompt, "catch-up momentum") && quote.Last >= quote.High*tbCatchUpLowerThreshold && quote.Last < quote.High*tbCatchUpUpperThreshold && quote.Last >= quote.Open {
		conviction += tbCatchUpBoost
	}
	if strings.Contains(prompt, "volume participation acceptance") && quote.Volume >= tbLowVolumeFloor && quote.Volume < tbDefaultVolumeFloor {
		conviction += tbLowVolumeBoost
	}
	return conviction
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
