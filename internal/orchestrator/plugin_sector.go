package orchestrator

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

type SemiconductorExecutor struct{}

func (SemiconductorExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "semiconductor_desk"
}

func (SemiconductorExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), 60)

	ctrl, ok := domain.ExtractPromptControl(prompt)
	fc := loadFactorConfig()
	if !ok {
		if strings.Contains(prompt, "close strength confirms leadership") && quote.Last > quote.Open {
			b.add("prompt_boost", 10, "close strength confirms leadership")
		}
		if strings.Contains(prompt, "weak volume") {
			b.add("prompt_penalty", -15, "weak volume")
		}
		addMomentumAdjustment(b, fq, quote.Symbol, fc)
		addLiquidityAdjustment(b, fq, quote.Symbol, fc)
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
	addMomentumAdjustment(b, fq, quote.Symbol, fc)
	addLiquidityAdjustment(b, fq, quote.Symbol, fc)
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

func (SemiconductorExecutor) EvaluatePosition(pos domain.Position, quote domain.Quote, agent domain.AgentSpec, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	if !slices.Contains(agent.Universe, pos.Symbol) {
		return domain.Recommendation{}, false
	}
	ss := dynamicSignalStrength(quote, signalParamsFromAgent(agent))
	if quote.Last < quote.Open*0.97 {
		return domain.Recommendation{
			Agent: agent.ID, Skill: agent.Skill, Layer: agent.Layer, Symbol: pos.Symbol,
			Side: domain.SideSell, Conviction: 100 - ss,
			Reason: "momentum decay: significant weakness (Last << Open)",
		}, true
	}
	if shouldReducePosition(quote) {
		return domain.Recommendation{
			Agent: agent.ID, Skill: agent.Skill, Layer: agent.Layer, Symbol: pos.Symbol,
			Side: domain.SideReduce, Conviction: 50,
			Reason: "signal weakening: reduce exposure",
		}, true
	}
	return domain.Recommendation{
		Agent: agent.ID, Skill: agent.Skill, Layer: agent.Layer, Symbol: pos.Symbol,
		Side: domain.SideBuy, Conviction: ss,
		Reason: "position evaluation: maintain holding",
	}, true
}

type AISupplyChainExecutor struct{}

func (AISupplyChainExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "ai_supply_chain_desk"
}

func (AISupplyChainExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
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
	addMomentumAdjustment(b, fq, quote.Symbol, loadFactorConfig())
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

func (AISupplyChainExecutor) EvaluatePosition(pos domain.Position, quote domain.Quote, agent domain.AgentSpec, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	if !slices.Contains(agent.Universe, pos.Symbol) {
		return domain.Recommendation{}, false
	}
	ss := dynamicSignalStrength(quote, signalParamsFromAgent(agent))
	if quote.Last < quote.Open*0.97 {
		return domain.Recommendation{
			Agent: agent.ID, Skill: agent.Skill, Layer: agent.Layer, Symbol: pos.Symbol,
			Side: domain.SideSell, Conviction: 100 - ss,
			Reason: "momentum decay: significant weakness (Last << Open)",
		}, true
	}
	if shouldReducePosition(quote) {
		return domain.Recommendation{
			Agent: agent.ID, Skill: agent.Skill, Layer: agent.Layer, Symbol: pos.Symbol,
			Side: domain.SideReduce, Conviction: 50,
			Reason: "signal weakening: reduce exposure",
		}, true
	}
	return domain.Recommendation{
		Agent: agent.ID, Skill: agent.Skill, Layer: agent.Layer, Symbol: pos.Symbol,
		Side: domain.SideBuy, Conviction: ss,
		Reason: "position evaluation: maintain holding",
	}, true
}

type LEOSatelliteExecutor struct{}

func (LEOSatelliteExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "leo_satellite_desk"
}

func (LEOSatelliteExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	// Load tunable parameters from ParametersConfig with hardcoded fallback.
	leoParams := config.GetParametersConfig()
	convBase := 60
	pricePenalty := -5
	launchBoost := 10
	deploymentBoost := 8
	downgradePenalty := -10
	targetMult := 1.08
	stopLossMult := 0.95
	if leoParams != nil {
		lp := leoParams.SectorExecutor.LEOSatellite
		convBase = lp.ConvictionBase.Value
		pricePenalty = lp.PricePenaltyDelta.Value
		launchBoost = lp.LaunchBoostDelta.Value
		deploymentBoost = lp.DeploymentBoostDelta.Value
		downgradePenalty = lp.DowngradePenaltyDelta.Value
		targetMult = lp.TargetPriceMult.Value
		stopLossMult = lp.StopLossMult.Value
	}

	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), convBase)
	if quote.Last < quote.Open {
		b.add("price_penalty", pricePenalty, "last < open")
	}
	if strings.Contains(prompt, "launch") && quote.Last > quote.Open {
		b.add("launch_boost", launchBoost, "launch keyword + last > open")
	}
	if strings.Contains(prompt, "deployment") && quote.Last > quote.Open {
		b.add("deployment_boost", deploymentBoost, "deployment keyword + last > open")
	}
	if strings.Contains(prompt, "downgrade") && quote.Last < quote.High*0.99 {
		b.add("downgrade_penalty", downgradePenalty, "downgrade keyword + last < high*0.99")
	}
	addMomentumAdjustment(b, fq, quote.Symbol, loadFactorConfig())
	if !b.floorCheck() {
		return domain.Recommendation{}, false
	}
	tp, slp := priceTargets(quote, targetMult, stopLossMult)
	conv, cb := b.build()
	return domain.Recommendation{
		Agent:               agent.ID,
		Skill:               agent.Skill,
		Layer:               agent.Layer,
		Symbol:              quote.Symbol,
		Side:                domain.SideBuy,
		Conviction:          conv,
		Reason:              "leo satellite infrastructure and deployment cycle",
		TargetPrice:         tp,
		StopLossPrice:       slp,
		ConvictionBreakdown: cb,
	}, true
}

type ETFRotationExecutor struct{}

func (ETFRotationExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "etf_rotation_desk"
}

func (ETFRotationExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	etfType := classifyETFType(quote.Symbol)

	// Base conviction varies by macro regime and ETF type
	base := 55
	var reason string

	switch regime {
	case domain.RegimeRiskOff:
		switch etfType {
		case "gold":
			base = 65
			reason = "safe-haven gold ETF in risk-off regime"
		case "dividend", "defensive":
			base = 60
			reason = "defensive dividend ETF in risk-off regime"
		default:
			base = 45
			reason = "equity ETF penalized in risk-off regime"
		}
	case domain.RegimeRiskOn:
		switch etfType {
		case "broad_market":
			base = 68
			reason = "broad market ETF in risk-on regime"
		case "dividend":
			base = 62
			reason = "dividend ETF in risk-on regime"
		case "gold":
			base = 50
			reason = "gold ETF penalized in risk-on regime"
		default:
			base = 58
			reason = "diversified ETF in risk-on regime"
		}
	default:
		if quote.Last > quote.Open {
			reason = "balanced ETF allocation with positive momentum in neutral regime"
		} else {
			reason = "balanced ETF allocation in neutral regime"
		}
	}

	b := newConvictionBuilder(base, 40)

	// Price-based adjustments
	if quote.Last > quote.Open {
		b.add("momentum_up", 5, "last > open")
	} else if quote.Last < quote.Open {
		b.add("momentum_down", -5, "last < open")
	}

	// Volume confirmation
	if quote.Volume > 500000 {
		b.add("volume_confirm", 3, "volume > 500k")
	}

	// Narrative / prompt-based signals
	if strings.Contains(prompt, "risk_off") || strings.Contains(prompt, "defensive") {
		if etfType == "gold" || etfType == "defensive" {
			b.add("narrative_defensive", 8, "risk_off/defensive narrative + defensive ETF")
		} else {
			b.add("narrative_defensive_mismatch", -5, "risk_off/defensive narrative but non-defensive ETF")
		}
	}
	if strings.Contains(prompt, "rotation") && quote.Last > quote.Open {
		b.add("rotation_boost", 6, "rotation keyword + last > open")
	}
	if strings.Contains(prompt, "sector_leadership") {
		b.add("sector_leadership", 5, "sector leadership keyword")
	}
	if strings.Contains(prompt, "JPY_carry") || strings.Contains(prompt, "carry_trade") {
		if etfType == "dividend" || etfType == "defensive" {
			b.add("carry_defensive", 7, "JPY carry unwind → defensive ETF")
		} else {
			b.add("carry_risk", -4, "JPY carry unwind → penalize beta")
		}
	}

	fc := loadFactorConfig()
	addMomentumAdjustment(b, fq, quote.Symbol, fc)
	addLiquidityAdjustment(b, fq, quote.Symbol, fc)

	if !b.floorCheck() {
		return domain.Recommendation{}, false
	}

	tpMult := 1.04
	slpMult := 0.97
	if etfType == "gold" {
		tpMult = 1.06
		slpMult = 0.95
	}

	tp, slp := priceTargets(quote, tpMult, slpMult)
	conv, cb := b.build()

	return domain.Recommendation{
		Agent:               agent.ID,
		Skill:               agent.Skill,
		Layer:               agent.Layer,
		Symbol:              quote.Symbol,
		Side:                domain.SideBuy,
		Conviction:          conv,
		Reason:              reason,
		TargetPrice:         tp,
		StopLossPrice:       slp,
		ConvictionBreakdown: cb,
	}, true
}

func (ETFRotationExecutor) EvaluatePosition(pos domain.Position, quote domain.Quote, agent domain.AgentSpec, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	if !slices.Contains(agent.Universe, pos.Symbol) {
		return domain.Recommendation{}, false
	}
	ss := dynamicSignalStrength(quote, signalParamsFromAgent(agent))
	if quote.Last < quote.Open*0.97 {
		return domain.Recommendation{
			Agent: agent.ID, Skill: agent.Skill, Layer: agent.Layer, Symbol: pos.Symbol,
			Side: domain.SideSell, Conviction: 100 - ss,
			Reason: "momentum decay: significant weakness (Last << Open)",
		}, true
	}
	if shouldReducePosition(quote) {
		return domain.Recommendation{
			Agent: agent.ID, Skill: agent.Skill, Layer: agent.Layer, Symbol: pos.Symbol,
			Side: domain.SideReduce, Conviction: 50,
			Reason: "signal weakening: reduce exposure",
		}, true
	}
	return domain.Recommendation{
		Agent: agent.ID, Skill: agent.Skill, Layer: agent.Layer, Symbol: pos.Symbol,
		Side: domain.SideBuy, Conviction: ss,
		Reason: "position evaluation: maintain holding",
	}, true
}

// classifyETFType maps symbol to ETF category for macro-aware routing logic.
func classifyETFType(symbol string) string {
	switch symbol {
	// Broad market ETFs
	case "0050.TW", "006208.TW", "00692.TW":
		return "broad_market"
	// Dividend ETFs
	case "0056.TW", "00919.TW", "00929.TW", "00940.TW":
		return "dividend"
	// Defensive / low-vol ETFs
	case "00878.TW", "00713.TW":
		return "defensive"
	// Sector equity ETFs
	case "00881.TW", "00891.TW":
		return "equity"
	// Gold ETFs
	case "00635U", "00693U", "00708L", "GLD", "IAU", "SGOL", "BAR":
		return "gold"
	default:
		return "equity"
	}
}

// ─── StrategyMeta implementations ──────────────────────────────

var _ StrategyProvider = SemiconductorExecutor{}

func (SemiconductorExecutor) StrategyMeta() StrategyMeta {
	fc := loadFactorConfig()
	return StrategyMeta{
		ID: "semiconductor", Skill: "semiconductor_desk",
		Description: "Semiconductor supply-chain leadership and capital-expenditure cycle detector",
		Factors:     []string{"momentum", "liquidity"},
		Parameters:  append(momentumParams(fc), liquidityParams(fc)...),
	}
}

func (AISupplyChainExecutor) StrategyMeta() StrategyMeta {
	fc := loadFactorConfig()
	return StrategyMeta{
		ID: "ai_supply_chain", Skill: "ai_supply_chain_desk",
		Description: "AI infrastructure order-flow sensitivity and capex cycle exposure",
		Factors:     []string{"momentum"},
		Parameters:  momentumParams(fc),
	}
}

func (LEOSatelliteExecutor) StrategyMeta() StrategyMeta {
	fc := loadFactorConfig()
	return StrategyMeta{
		ID: "leo_satellite", Skill: "leo_satellite_desk",
		Description: "LEO satellite deployment cycle and infrastructure build-out detector",
		Factors:     []string{"momentum"},
		Parameters:  momentumParams(fc),
	}
}

func (ETFRotationExecutor) StrategyMeta() StrategyMeta {
	fc := loadFactorConfig()
	return StrategyMeta{
		ID: "etf_rotation", Skill: "etf_rotation_desk",
		Description: "Macro-aware ETF rotation with defensive/gold bias in risk-off regimes",
		Factors:     []string{"momentum", "liquidity"},
		Parameters:  append(momentumParams(fc), liquidityParams(fc)...),
	}
}

func (FinancialsExecutor) StrategyMeta() StrategyMeta {
	fc := loadFactorConfig()
	return StrategyMeta{
		ID: "financials", Skill: "financials_desk",
		Description: "Financial carry trade with balance-sheet resilience and credit quality gates",
		Factors:     []string{"value", "quality"},
		Parameters:  append(valueParams(fc), qualityParams(fc)...),
	}
}

func (ShippingExecutor) StrategyMeta() StrategyMeta {
	fc := loadFactorConfig()
	return StrategyMeta{
		ID: "shipping", Skill: "shipping_desk",
		Description: "Shipping cycle exposure using tactical momentum and weak-close avoidance",
		Factors:     []string{"momentum", "liquidity"},
		Parameters:  append(momentumParams(fc), liquidityParams(fc)...),
	}
}

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

func (FinancialsExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	conviction, cb := finConviction(agent, prompt, quote, fq)
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

func (FinancialsExecutor) EvaluatePosition(pos domain.Position, quote domain.Quote, agent domain.AgentSpec, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	if !slices.Contains(agent.Universe, pos.Symbol) {
		return domain.Recommendation{}, false
	}
	ss := dynamicSignalStrength(quote, signalParamsFromAgent(agent))
	if quote.Last < quote.Open*0.97 {
		return domain.Recommendation{
			Agent: agent.ID, Skill: agent.Skill, Layer: agent.Layer, Symbol: pos.Symbol,
			Side: domain.SideSell, Conviction: 100 - ss,
			Reason: "momentum decay: significant weakness (Last << Open)",
		}, true
	}
	if shouldReducePosition(quote) {
		return domain.Recommendation{
			Agent: agent.ID, Skill: agent.Skill, Layer: agent.Layer, Symbol: pos.Symbol,
			Side: domain.SideReduce, Conviction: 50,
			Reason: "signal weakening: reduce exposure",
		}, true
	}
	return domain.Recommendation{
		Agent: agent.ID, Skill: agent.Skill, Layer: agent.Layer, Symbol: pos.Symbol,
		Side: domain.SideBuy, Conviction: ss,
		Reason: "position evaluation: maintain holding",
	}, true
}

func finConviction(agent domain.AgentSpec, prompt string, quote domain.Quote, fq FactorQuery) (int, *domain.ConvictionBreakdown) {
	db, bp, cqb, cqp, ssb, ssp, cab := finDividendBoost, finBalanceSheetPenalty, finCreditQualityBoost, finCreditQualityPenalty, finSpreadSensitivityBoost, finSpreadSensitivityPenalty, finCapitalAdequacyBoost
	pto, pth := finPriceToOpenThreshold, finPriceToHighThreshold
	if cfg := config.GetParametersConfig(); cfg != nil {
		fp := cfg.SectorExecutor.Financials
		db, bp, cqb, cqp, ssb, ssp, cab = fp.DividendBoost.Value, fp.BalanceSheetPenalty.Value, fp.CreditQualityBoost.Value, fp.CreditQualityPenalty.Value, fp.SpreadSensitivityBoost.Value, fp.SpreadSensitivityPenalty.Value, fp.CapitalAdequacyBoost.Value
		pto, pth = fp.PriceToOpenThreshold.Value, fp.PriceToHighThreshold.Value
	}
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), 50)
	if strings.Contains(prompt, "dividend") && quote.Last >= quote.Open {
		b.add("dividend_boost", db, "dividend keyword + last >= open")
	}
	if strings.Contains(prompt, "balance-sheet") && quote.Low < quote.Open*pto {
		b.add("balance_sheet_penalty", -bp, "balance-sheet keyword + low < open*threshold")
	}
	if strings.Contains(prompt, "credit quality gate") {
		if quote.Last >= quote.Open {
			b.add("credit_quality_boost", cqb, "credit quality gate + last >= open")
		} else {
			b.add("credit_quality_penalty", -cqp, "credit quality gate + last < open")
		}
	}
	if strings.Contains(prompt, "spread sensitivity downgrade") {
		if quote.Last >= quote.High*pth {
			b.add("spread_sensitivity_boost", ssb, "last >= high*threshold")
		} else {
			b.add("spread_sensitivity_penalty", -ssp, "last < high*threshold")
		}
	}
	if strings.Contains(prompt, "capital adequacy premium") && quote.Last >= quote.Open && quote.Last >= quote.High*pth {
		b.add("capital_adequacy_boost", cab, "capital adequacy premium + last >= open & high*threshold")
	}
	fc := loadFactorConfig()
	addValueAdjustment(b, fq, quote.Symbol, fc)
	addQualityAdjustment(b, fq, quote.Symbol, fc)
	return b.build()
}

type ShippingExecutor struct{}

func (ShippingExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "shipping_desk"
}

func (ShippingExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	tb, wcp, wct := shipTacticalBoost, shipWeakClosePenalty, shipWeakCloseThreshold
	if cfg := config.GetParametersConfig(); cfg != nil {
		if sp := cfg.SectorExecutor.Shipping; sp.TacticalBoost.Value != 0 {
			tb, wcp, wct = sp.TacticalBoost.Value, sp.WeakClosePenalty.Value, sp.WeakCloseThreshold.Value
		}
	}
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), defaultConvictionFloor)
	if strings.Contains(prompt, "tactical") && quote.Last > quote.Open {
		b.add("tactical_boost", tb, "tactical keyword + last > open")
	}
	if strings.Contains(prompt, "avoid weak closes") && quote.Last < quote.High*wct {
		b.add("weak_close_penalty", -wcp, "avoid weak closes + last < high*threshold")
	}
	fc := loadFactorConfig()
	addMomentumAdjustment(b, fq, quote.Symbol, fc)
	addLiquidityAdjustment(b, fq, quote.Symbol, fc)
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

func (ShippingExecutor) EvaluatePosition(pos domain.Position, quote domain.Quote, agent domain.AgentSpec, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	if !slices.Contains(agent.Universe, pos.Symbol) {
		return domain.Recommendation{}, false
	}
	ss := dynamicSignalStrength(quote, signalParamsFromAgent(agent))
	if quote.Last < quote.Open*0.97 {
		return domain.Recommendation{
			Agent: agent.ID, Skill: agent.Skill, Layer: agent.Layer, Symbol: pos.Symbol,
			Side: domain.SideSell, Conviction: 100 - ss,
			Reason: "momentum decay: significant weakness (Last << Open)",
		}, true
	}
	if shouldReducePosition(quote) {
		return domain.Recommendation{
			Agent: agent.ID, Skill: agent.Skill, Layer: agent.Layer, Symbol: pos.Symbol,
			Side: domain.SideReduce, Conviction: 50,
			Reason: "signal weakening: reduce exposure",
		}, true
	}
	return domain.Recommendation{
		Agent: agent.ID, Skill: agent.Skill, Layer: agent.Layer, Symbol: pos.Symbol,
		Side: domain.SideBuy, Conviction: ss,
		Reason: "position evaluation: maintain holding",
	}, true
}

type RoboticsDeskExecutor struct{}

func (RoboticsDeskExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "robotics_desk"
}

func (RoboticsDeskExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), defaultConvictionFloor)
	if strings.Contains(prompt, "automation") {
		b.add("automation_boost", 5, "automation keyword")
	}
	if strings.Contains(prompt, "servo") {
		b.add("servo_boost", 5, "servo keyword")
	}
	if strings.Contains(prompt, "ai_robot") {
		b.add("ai_robot_boost", 8, "ai_robot keyword")
	}
	fc := loadFactorConfig()
	addMomentumAdjustment(b, fq, quote.Symbol, fc)
	addLiquidityAdjustment(b, fq, quote.Symbol, fc)
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
		Reason:              "robotics automation capex cycle",
		TargetPrice:         tp,
		StopLossPrice:       slp,
		ConvictionBreakdown: cb,
	}, true
}

type MiningDeskExecutor struct{}

func (MiningDeskExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "mining_desk"
}

func (MiningDeskExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), defaultConvictionFloor)
	if strings.Contains(prompt, "precious_metals") && regime == domain.RegimeRiskOff {
		b.add("precious_metals_boost", 8, "precious_metals keyword in risk-off regime")
	}
	if strings.Contains(prompt, "copper") {
		b.add("copper_boost", 4, "copper keyword")
	}
	if strings.Contains(prompt, "safe_haven") {
		b.add("safe_haven_boost", 6, "safe_haven keyword")
	}
	fc := loadFactorConfig()
	addMomentumAdjustment(b, fq, quote.Symbol, fc)
	addLiquidityAdjustment(b, fq, quote.Symbol, fc)
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
		Reason:              "mining and precious metals cycle",
		TargetPrice:         tp,
		StopLossPrice:       slp,
		ConvictionBreakdown: cb,
	}, true
}

type EnergyDeskExecutor struct{}

func (EnergyDeskExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "energy_desk"
}

func (EnergyDeskExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), defaultConvictionFloor)
	if strings.Contains(prompt, "oil_surge") {
		b.add("oil_surge_boost", 6, "oil_surge keyword")
	}
	if strings.Contains(prompt, "renewable") {
		b.add("renewable_boost", 5, "renewable keyword")
	}
	if strings.Contains(prompt, "grid") {
		b.add("grid_boost", 4, "grid keyword")
	}
	fc := loadFactorConfig()
	addMomentumAdjustment(b, fq, quote.Symbol, fc)
	addLiquidityAdjustment(b, fq, quote.Symbol, fc)
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
		Reason:              "energy commodity cycle",
		TargetPrice:         tp,
		StopLossPrice:       slp,
		ConvictionBreakdown: cb,
	}, true
}

type ElectronicsDeskExecutor struct{}

func (ElectronicsDeskExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "electronics_desk"
}

func (ElectronicsDeskExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), defaultConvictionFloor)
	if strings.Contains(prompt, "passive_components") {
		b.add("passive_components_boost", 5, "passive_components keyword")
	}
	if strings.Contains(prompt, "connectors") {
		b.add("connectors_boost", 5, "connectors keyword")
	}
	if strings.Contains(prompt, "thermal") {
		b.add("thermal_boost", 6, "thermal keyword")
	}
	fc := loadFactorConfig()
	addMomentumAdjustment(b, fq, quote.Symbol, fc)
	addLiquidityAdjustment(b, fq, quote.Symbol, fc)
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
		Reason:              "electronics component demand cycle",
		TargetPrice:         tp,
		StopLossPrice:       slp,
		ConvictionBreakdown: cb,
	}, true
}

type ConsumerDeskExecutor struct{}

func (ConsumerDeskExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "consumer_desk"
}

func (ConsumerDeskExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), defaultConvictionFloor)
	if strings.Contains(prompt, "dividend") {
		b.add("dividend_boost", 5, "dividend keyword")
	}
	if strings.Contains(prompt, "staples") {
		b.add("staples_boost", 6, "staples keyword")
	}
	if strings.Contains(prompt, "retail") {
		b.add("retail_boost", 4, "retail keyword")
	}
	fc := loadFactorConfig()
	addMomentumAdjustment(b, fq, quote.Symbol, fc)
	addLiquidityAdjustment(b, fq, quote.Symbol, fc)
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
		Reason:              "consumer staples and retail cycle",
		TargetPrice:         tp,
		StopLossPrice:       slp,
		ConvictionBreakdown: cb,
	}, true
}

type IndustrialDeskExecutor struct{}

func (IndustrialDeskExecutor) Supports(agent domain.AgentSpec) bool {
	return agent.Skill == "industrial_desk"
}

func (IndustrialDeskExecutor) Recommend(agent domain.AgentSpec, quote domain.Quote, prompt string, regime domain.Regime, fq FactorQuery) (domain.Recommendation, bool) {
	b := newConvictionBuilder(dynamicSignalStrength(quote, signalParamsFromAgent(agent)), defaultConvictionFloor)
	if strings.Contains(prompt, "steel") {
		b.add("steel_boost", 5, "steel keyword")
	}
	if strings.Contains(prompt, "cement") {
		b.add("cement_boost", 4, "cement keyword")
	}
	if strings.Contains(prompt, "infrastructure") {
		b.add("infrastructure_boost", 7, "infrastructure keyword")
	}
	fc := loadFactorConfig()
	addMomentumAdjustment(b, fq, quote.Symbol, fc)
	addLiquidityAdjustment(b, fq, quote.Symbol, fc)
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
		Reason:              "industrial manufacturing and infrastructure cycle",
		TargetPrice:         tp,
		StopLossPrice:       slp,
		ConvictionBreakdown: cb,
	}, true
}

// shouldReducePosition returns true when the quote shows signs of momentum decay
// or volume atrophy that warrant reducing or selling the position.
func shouldReducePosition(quote domain.Quote) bool {
	return quote.Last < quote.Open || quote.Volume < 1_000_000
}
