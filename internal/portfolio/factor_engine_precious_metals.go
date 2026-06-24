package portfolio

import (
	"context"
	"math"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// ── Precious Metals Factor (P0-2) ──
// SOURCE: Erb & Harvey (2013), World Gold Council, P0-2 task brief
//
// Composite precious metals score for gold:
//   PM_gold = 0.16*RealRate + 0.10*DXY + 0.10*InflExp + 0.12*CB + 0.08*Flow + 0.06*PhyDem + 0.10*COMEX + 0.28*RiskOff
// For silver:
//   PM_silver = 0.60*PM_gold + 0.15*Industrial + 0.10*GoldSilver + 0.15*COMEX

// CalculatePreciousMetalsScore returns the composite precious metals factor score.
// Returns 0 if symbol is not a known precious metal instrument.
//
// Composite construction:
//  1. Fetch PreciousMetalsContext from pmCtxProv (nil → sub-factor scores = 0)
//  2. Compute 8 gold sub-factors + COMEX
//  3. goldScore = weighted sum; risk-off floor: if RiskOff>=1.0 and goldScore<0.5,
//     clamp to 0.5 (extreme-panic gold bid)
//  4. If subtype == "silver": blend with industrial demand + gold/silver ratio
//
// All sub-scores handle nil context or NaN inputs by returning 0.
func (fe *FactorEngine) CalculatePreciousMetalsScore(ctx context.Context, symbol string, quotes map[string]domain.Quote) domain.FactorScoreItem {
	isPM, subtype := isPreciousMetal(symbol)
	if !isPM {
		return domain.FactorScoreItem{Score: 0.0, Formula: "not_precious_metal"}
	}

	pmCtx := fe.getPMContext(symbol)

	realRate := fe.pmRealRateScore(pmCtx)
	dxy := fe.pmDXYScore(pmCtx)
	inflExp := fe.pmInflationExpectScore(pmCtx)
	cbBuy := fe.pmCentralBankScore(pmCtx)
	etfFlow := fe.pmETFFlowScore(ctx)
	riskOff := fe.pmRiskOffScore(pmCtx)
	physDemand := fe.pmPhysicalDemandScore(pmCtx)
	comex := fe.pmCOMEXScore(pmCtx)

	goldScore := 0.16*realRate + 0.10*dxy + 0.10*inflExp + 0.12*cbBuy + 0.08*etfFlow + 0.06*physDemand + 0.10*comex + 0.28*riskOff

	if riskOff >= 1.0 && goldScore < 0.5 {
		goldScore = 0.5
	}

	score := goldScore
	formula := "gold: 0.16*RR + 0.10*DXY + 0.10*Inf + 0.12*CB + 0.08*Flow + 0.06*PhyDem + 0.10*COMEX + 0.28*RiskOff"

	if subtype == "silver" {
		indDemand := fe.pmIndustrialDemandScore(pmCtx)
		gsRatio := fe.pmGoldSilverRatioScore(pmCtx)
		score = 0.60*goldScore + 0.15*indDemand + 0.10*gsRatio + 0.15*comex
		formula = "silver: 0.60*PM_gold + 0.15*Ind + 0.10*GS + 0.15*COMEX"
	}

	return domain.FactorScoreItem{
		Score:   score,
		Formula: formula,
		RawInputs: map[string]float64{
			"real_rate":       realRate,
			"dxy":             dxy,
			"inflation":       inflExp,
			"cb_buy":          cbBuy,
			"etf_flow":        etfFlow,
			"risk_off":        riskOff,
			"physical_demand": physDemand,
			"comex":           comex,
			"gold_composite":  goldScore,
		},
	}
}

// getPMContext returns the macro context for a symbol from the attached
// pmCtxProv. Returns nil if no provider was attached (sub-factor scores
// will then default to 0).
func (fe *FactorEngine) getPMContext(symbol string) *PreciousMetalsContext {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	if fe.pmCtxProv != nil {
		return fe.pmCtxProv(symbol)
	}
	return nil
}

// pmRealRateScore: -normalize(r_real). Higher real rates -> lower gold demand.
// Uses linear scaling with center at 1.5%: r=0.5%->+0.67, r=2%->-0.33.
// SOURCE: Erb & Harvey (2013)
func (fe *FactorEngine) pmRealRateScore(ctx *PreciousMetalsContext) float64 {
	if ctx == nil || math.IsNaN(ctx.RealRate) {
		return 0
	}
	r := ctx.RealRate
	score := -(r*100 - 1.5) / 1.5
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}
	return score
}

// pmDXYScore: -normalize(DXY change). Stronger dollar -> lower gold.
// Center at 100; +/-10 point move = +/-1.0.
func (fe *FactorEngine) pmDXYScore(ctx *PreciousMetalsContext) float64 {
	if ctx == nil || math.IsNaN(ctx.DXY) {
		return 0
	}
	score := -(ctx.DXY - 100) / 10.0
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}
	return score
}

// pmInflationExpectScore: normalize(CPI). Higher inflation -> higher gold.
// Bonus +0.2 if CPI > 3%; capped at 1.0.
func (fe *FactorEngine) pmInflationExpectScore(ctx *PreciousMetalsContext) float64 {
	if ctx == nil || math.IsNaN(ctx.CPIYoY) {
		return 0
	}
	score := (ctx.CPIYoY - 0.02) * 50.0
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}
	if ctx.CPIYoY > 3.0 {
		score += 0.2
	}
	if score > 1.0 {
		score = 1.0
	}
	return score
}

// pmCentralBankScore: heuristic based on CBReserveTrend from context.
// ReserveTrend > 0.3 -> +0.7, > 0 -> +0.3, > -0.3 -> 0, > -1 -> -0.2.
// SOURCE: World Gold Council quarterly report, central bank gold reserve trend.
func (fe *FactorEngine) pmCentralBankScore(ctx *PreciousMetalsContext) float64 {
	if ctx == nil || math.IsNaN(ctx.CBReserveTrend) {
		return 0
	}
	switch {
	case ctx.CBReserveTrend > 0.3:
		return 0.7
	case ctx.CBReserveTrend > 0:
		return 0.3
	case ctx.CBReserveTrend > -0.3:
		return 0
	case ctx.CBReserveTrend > -1:
		return -0.2
	default:
		return -0.2
	}
}

// pmETFFlowScore: uses GLD 20d momentum as ETF flow proxy.
// GLD > 5% -> +0.5 (strong inflows), > 0 -> +0.2, < -5% -> -0.3, else 0.
// Falls back to 0 if GLD price history unavailable.
// ensureAdjusted is called on GLD symbol (same TTL cache as momentum/quality).
func (fe *FactorEngine) pmETFFlowScore(ctx context.Context) float64 {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	if fe.history == nil {
		return 0
	}
	if err := fe.ensureAdjusted(ctx, "GLD"); err != nil {
		logging.Warn("factor_engine", "ensure_adjusted_failed", logging.Symbol("GLD"), logging.Err(err))
	}
	ret20d := fe.history.MomentumReturn("GLD", 20)
	switch {
	case ret20d > 0.05:
		return 0.5
	case ret20d > 0:
		return 0.2
	case ret20d < -0.05:
		return -0.3
	default:
		return 0
	}
}

// pmRiskOffScore: normalize(VIX). Higher VIX -> higher gold.
// Bonus +0.25 if VIX > 25; floor at 1.0 if VIX > 35 (extreme panic).
func (fe *FactorEngine) pmRiskOffScore(ctx *PreciousMetalsContext) float64 {
	if ctx == nil || math.IsNaN(ctx.VIX) {
		return 0
	}
	score := (ctx.VIX - 20) / 20.0
	if score > 1.0 {
		score = 1.0
	}
	if score < 0 {
		score = 0
	}
	if ctx.VIX > 35 {
		return 1.0
	}
	if ctx.VIX > 25 {
		score += 0.25
	}
	if score > 1.0 {
		score = 1.0
	}
	return score
}

// pmPhysicalDemandScore: composite of India and China gold import trends.
// Normalizes imports to [-1, 1]: 10% YoY -> +1.0, -10% -> -1.0, linear between.
// 0.5 * India score + 0.5 * China score.
func (fe *FactorEngine) pmPhysicalDemandScore(ctx *PreciousMetalsContext) float64 {
	if ctx == nil {
		return 0
	}
	indiaScore := fe.normalizeImportYoY(ctx.IndiaGoldImportsYoY)
	chinaScore := fe.normalizeImportYoY(ctx.ChinaGoldImportsYoY)
	return 0.5*indiaScore + 0.5*chinaScore
}

// normalizeImportYoY clamps a YoY % to [-1, 1] (10% YoY = +1.0).
func (fe *FactorEngine) normalizeImportYoY(yoy float64) float64 {
	if math.IsNaN(yoy) {
		return 0
	}
	score := yoy / 10.0
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}
	return score
}

// pmCOMEXScore: contrarian COT signal. COMEX net long > 200k -> -0.5 (too bullish),
// < 50k -> +0.5 (too bearish), between -> 0.
func (fe *FactorEngine) pmCOMEXScore(ctx *PreciousMetalsContext) float64 {
	if ctx == nil || math.IsNaN(ctx.COMEXNetLong) {
		return 0
	}
	switch {
	case ctx.COMEXNetLong > 200000:
		return -0.5
	case ctx.COMEXNetLong < 50000:
		return 0.5
	default:
		return 0
	}
}

// pmIndustrialDemandScore: VIX as PMI proxy. VIX < 15 -> +0.5 (strong PMI),
// VIX < 20 -> +0.2, VIX > 25 -> -0.3, VIX > 30 -> -0.5.
func (fe *FactorEngine) pmIndustrialDemandScore(ctx *PreciousMetalsContext) float64 {
	if ctx == nil || math.IsNaN(ctx.VIX) {
		return 0
	}
	switch {
	case ctx.VIX < 15:
		return 0.5
	case ctx.VIX < 20:
		return 0.2
	case ctx.VIX > 30:
		return -0.5
	case ctx.VIX > 25:
		return -0.3
	default:
		return 0
	}
}

// pmGoldSilverRatioScore: mean-reversion signal. GoldSilverRatioZ > 1.5 -> +0.5
// (gold overvalued, bullish silver mean reversion). Z < -1.5 -> -0.3.
// Linear interpolation between.
func (fe *FactorEngine) pmGoldSilverRatioScore(ctx *PreciousMetalsContext) float64 {
	if ctx == nil || math.IsNaN(ctx.GoldSilverRatioZ) {
		return 0
	}
	z := ctx.GoldSilverRatioZ
	if z > 1.5 {
		return 0.5
	}
	if z < -1.5 {
		return -0.3
	}
	if z > 0 {
		return z / 1.5 * 0.5
	}
	return z / 1.5 * 0.3
}
