// Package retail provides RSI-tw (Retail Sentiment Index — Taiwan).
package retail

import (
	"math"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// RSITwSnapshot is the output of one RSI-tw calculation.
type RSITwSnapshot struct {
	Score            float64                    `json:"score"`
	PartAScore       float64                    `json:"part_a_score"`
	PartCScore       float64                    `json:"part_c_score"`
	AdjustmentFactor float64                    `json:"adjustment_factor"`
	SubIndicators    map[string]RSISubIndicator `json:"sub_indicators"`
	Timestamp        time.Time                  `json:"timestamp"`
}

// RSISubIndicator holds the detail for a single sub-indicator.
type RSISubIndicator struct {
	Value      float64 `json:"value"`
	Weight     float64 `json:"weight"`
	ZScore     float64 `json:"z_score"`
	IsFallback bool    `json:"is_fallback"`
}

// DayTradingStats carries day-trading volume information.
type DayTradingStats struct {
	Volume      float64 `json:"volume"`
	VolumeRatio float64 `json:"volume_ratio"`
}

// MarginStats carries margin trading statistics.
type MarginStats struct {
	Balance          float64 `json:"balance"`
	ShortBalance     float64 `json:"short_balance"`
	DayTradingRatio  float64 `json:"day_trading_ratio"`
	MarginPercentile float64 `json:"margin_percentile"`
}

// RSITwInput is the complete input needed for one RSI-tw computation.
type RSITwInput struct {
	MarginBalance      float64          `json:"margin_balance"`
	MarginPercentile   float64          `json:"margin_percentile"`
	DayTrading         *DayTradingStats `json:"day_trading,omitempty"`
	VIXLevel           float64          `json:"vix_level"`
	ForeignInvestorNet float64          `json:"foreign_investor_net"`
	DomesticFundNet    float64          `json:"domestic_fund_net"`
	GeopoliticalRisk   float64          `json:"geopolitical_risk"`
	CreditTightening   bool             `json:"credit_tightening"`
	PutCallRatio       float64          `json:"put_call_ratio"`
	OddLotImbalance    float64          `json:"odd_lot_imbalance"`
	RetailFuturesPct   float64          `json:"retail_futures_pct"`
	ETFNetSubscription float64          `json:"etf_net_subscription"`
	// Phase 2 placeholders
	FuturesOI float64 `json:"futures_oi"`
	PCR       float64 `json:"pcr"`
	OddLot    float64 `json:"odd_lot"`
}

// Calculator computes RSI-tw sentiment snapshots.
type Calculator struct {
	mu            sync.RWMutex
	marginHistory []float64
	vixHistory    []float64
}

// NewCalculator returns an initialised Calculator with empty histories.
func NewCalculator() *Calculator {
	return &Calculator{
		marginHistory: make([]float64, 0, 90),
		vixHistory:    make([]float64, 0, 90),
	}
}

// ---------------------------------------------------------------------------
// Public methods
// ---------------------------------------------------------------------------

// ComputeFinal calculates the full RSI-tw snapshot for the given input.
func (c *Calculator) ComputeFinal(data RSITwInput) RSITwSnapshot {
	c.mu.RLock()
	marginSnap := make([]float64, len(c.marginHistory))
	copy(marginSnap, c.marginHistory)
	c.mu.RUnlock()

	subs := make(map[string]RSISubIndicator, 9)
	partA := c.computePartA(data, marginSnap, subs)
	partC := c.computePartC(data, subs)
	adj := c.computeAdjustmentFactor(data, subs)
	final := (partA*0.40 + partC*0.25) * adj
	final = clamp(final, -1.0, 1.0)

	return RSITwSnapshot{
		Score:            round4(final),
		PartAScore:       round4(partA),
		PartCScore:       round4(partC),
		AdjustmentFactor: round4(adj),
		SubIndicators:    subs,
		Timestamp:        time.Now(),
	}
}

// UpdateHistory appends the input values to the internal rolling histories.
// It maintains at most 90 entries for Z-score calculations.
func (c *Calculator) UpdateHistory(data RSITwInput) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.marginHistory = append(c.marginHistory, data.MarginBalance)
	if len(c.marginHistory) > 90 {
		c.marginHistory = c.marginHistory[len(c.marginHistory)-90:]
	}

	c.vixHistory = append(c.vixHistory, data.VIXLevel)
	if len(c.vixHistory) > 90 {
		c.vixHistory = c.vixHistory[len(c.vixHistory)-90:]
	}
}

// ---------------------------------------------------------------------------
// Part A — Retail Sentiment (40 % weight)
// ---------------------------------------------------------------------------

func (c *Calculator) computePartA(data RSITwInput, marginHistory []float64, subs map[string]RSISubIndicator) float64 {
	var total float64

	// A1: Margin Balance Δ Z-score (weight 0.25)
	total += c.subA1(data, marginHistory, subs)

	// A2: Day Trading Ratio (weight 0.20)
	total += c.subA2(data, subs)

	// A3: Margin Maintenance Proxy (weight 0.20)
	total += c.subA3(data, subs)

	// A4: VIX Nonlinear Mapping (weight 0.15)
	total += c.subA4(data, subs)

	// A5: Weekly PCR Proxy (weight 0.10)
	total += c.subA5(data, subs)

	// A6: Odd-Lot Trading (weight 0.10)
	total += c.subA6(data, subs)

	return clamp(total, -1.0, 1.0)
}

// A1: Margin Balance Δ Z-score (weight 0.25)
func (c *Calculator) subA1(data RSITwInput, history []float64, subs map[string]RSISubIndicator) float64 {
	const weight = 0.25
	ind := RSISubIndicator{Weight: weight, Value: data.MarginBalance}

	if len(history) < 2 {
		ind.ZScore = 0
		ind.IsFallback = true
		subs["a1_margin_z"] = ind
		return 0
	}

	// Use at most the last 30 snapshots.
	n := len(history)
	if n > 30 {
		history = history[n-30:]
	}

	mean := mean(history)
	std := stddev(history, mean)
	if std == 0 {
		ind.ZScore = 0
		ind.IsFallback = true
		subs["a1_margin_z"] = ind
		return 0
	}

	ind.ZScore = clamp((data.MarginBalance-mean)/std, -2.0, 2.0)
	subs["a1_margin_z"] = ind
	return ind.ZScore * weight
}

// A2: Day Trading Ratio (weight 0.20)
func (c *Calculator) subA2(data RSITwInput, subs map[string]RSISubIndicator) float64 {
	const weight = 0.20
	ind := RSISubIndicator{Weight: weight}

	if data.DayTrading == nil {
		ind.IsFallback = true
		subs["a2_day_trading"] = ind
		return 0
	}

	ind.Value = data.DayTrading.Volume
	ind.ZScore = data.DayTrading.VolumeRatio
	subs["a2_day_trading"] = ind
	return ind.ZScore * weight
}

// A3: Margin Maintenance Proxy (weight 0.20)
func (c *Calculator) subA3(data RSITwInput, subs map[string]RSISubIndicator) float64 {
	const weight = 0.20
	ind := RSISubIndicator{
		Weight: weight,
		Value:  data.MarginPercentile,
	}
	// Higher percentile → more strain → bearish sentiment
	ind.ZScore = clamp((data.MarginPercentile-0.5)*2, -1.0, 1.0)
	subs["a3_margin_maint"] = ind
	return ind.ZScore * weight
}

// A4: VIX Nonlinear Mapping (weight 0.15)
func (c *Calculator) subA4(data RSITwInput, subs map[string]RSISubIndicator) float64 {
	const weight = 0.15
	ind := RSISubIndicator{Weight: weight, Value: data.VIXLevel}

	if data.VIXLevel <= 0 {
		ind.ZScore = 0.5
		ind.IsFallback = true
		subs["a4_vix_map"] = ind
		return ind.ZScore * weight
	}

	ind.ZScore = vixMap(data.VIXLevel)
	subs["a4_vix_map"] = ind
	return ind.ZScore * weight
}

// A5: Weekly PCR (weight 0.10)
func (c *Calculator) subA5(data RSITwInput, subs map[string]RSISubIndicator) float64 {
	const weight = 0.10
	pcr := data.PutCallRatio
	var score float64
	if pcr == 0 {
		score = 0.5
	} else if pcr > 1.5 {
		score = 0.9
	} else if pcr > 1.0 {
		score = 0.7
	} else if pcr > 0.8 {
		score = 0.5
	} else {
		score = 0.1
	}
	ind := RSISubIndicator{Value: pcr, Weight: weight, ZScore: score, IsFallback: pcr == 0}
	subs["a5_pcr_proxy"] = ind
	return score * weight
}

// A6: Odd-Lot Trading (weight 0.10)
func (c *Calculator) subA6(data RSITwInput, subs map[string]RSISubIndicator) float64 {
	const weight = 0.10
	imb := data.OddLotImbalance
	var score float64
	if imb == 0 {
		score = 0.5
	} else if imb > 0.2 {
		score = 0.85
	} else if imb > 0.1 {
		score = 0.65
	} else if imb > -0.1 {
		score = 0.5
	} else if imb > -0.2 {
		score = 0.35
	} else {
		score = 0.15
	}
	ind := RSISubIndicator{Value: imb, Weight: weight, ZScore: score, IsFallback: imb == 0}
	subs["a6_odd_lot"] = ind
	return score * weight
}

// ---------------------------------------------------------------------------
// Part C — Institutional / Derivative Flow (25 % weight)
// ---------------------------------------------------------------------------

func (c *Calculator) computePartC(data RSITwInput, subs map[string]RSISubIndicator) float64 {
	var total float64

	// C1: Small TAIEX Futures OI (weight 0.40)
	total += c.subC1(data, subs)

	// C2: Foreign / Institutional Net Flow Proxy (weight 0.35)
	total += c.subC2(data, subs)

	// C3: ETF Net Subscription (weight 0.25)
	total += c.subC3(data, subs)

	return clamp(total, -1.0, 1.0)
}

// C1: Small TAIEX Futures OI (weight 0.40)
func (c *Calculator) subC1(data RSITwInput, subs map[string]RSISubIndicator) float64 {
	const weight = 0.40
	pct := data.RetailFuturesPct
	var score float64
	if pct == 0 {
		score = 0.5
	} else if pct > 20 {
		score = 0.9
	} else if pct > 10 {
		score = 0.7
	} else if pct > -10 {
		score = 0.5
	} else if pct > -20 {
		score = 0.25
	} else {
		score = 0.1
	}
	ind := RSISubIndicator{Value: pct, Weight: weight, ZScore: score, IsFallback: pct == 0}
	subs["c1_futures_oi"] = ind
	return score * weight
}

// C2: Foreign / Institutional Net Flow Proxy (weight 0.35)
func (c *Calculator) subC2(data RSITwInput, subs map[string]RSISubIndicator) float64 {
	const weight = 0.35
	netFlow := data.ForeignInvestorNet + data.DomesticFundNet
	ind := RSISubIndicator{Weight: weight, Value: netFlow}

	if netFlow == 0 {
		subs["c2_inst_flow"] = ind
		return 0
	}

	// Positive net → bullish [0.3, 0.7]; negative net → bearish [−0.7, −0.3].
	// Use the centre of each range until a more precise scaling is available.
	if netFlow > 0 {
		ind.ZScore = 0.5
	} else {
		ind.ZScore = -0.5
	}
	subs["c2_inst_flow"] = ind
	return ind.ZScore * weight
}

// C3: ETF Net Subscription (weight 0.25)
func (c *Calculator) subC3(data RSITwInput, subs map[string]RSISubIndicator) float64 {
	const weight = 0.25
	netSub := data.ETFNetSubscription
	var score float64
	if netSub == 0 {
		score = 0.5
	} else if netSub > 1_000_000_000 {
		score = 0.9
	} else if netSub > 100_000_000 {
		score = 0.7
	} else if netSub > 0 {
		score = 0.55
	} else if netSub > -100_000_000 {
		score = 0.45
	} else {
		score = 0.2
	}
	ind := RSISubIndicator{Value: netSub, Weight: weight, ZScore: score, IsFallback: netSub == 0}
	subs["c3_etf_sub"] = ind
	return score * weight
}

// ---------------------------------------------------------------------------
// Part D — Adjustment Factor (0.8 – 1.2)
// ---------------------------------------------------------------------------

func (c *Calculator) computeAdjustmentFactor(data RSITwInput, subs map[string]RSISubIndicator) float64 {
	factor := 1.0

	// D1: Geopolitical Risk (multiplier 0.85)
	factor *= c.factorD1(data, subs)

	// D2: VIX Spike Override (multiplier 0.90)
	factor *= c.factorD2(data, subs)

	// D3: Credit Control Signal (multiplier 0.80)
	factor *= c.factorD3(data, subs)

	// D4: Military / Flash Crash (multiplier 1.15, placeholder)
	factor *= c.factorD4(subs)

	return clamp(factor, 0.8, 1.2)
}

func (c *Calculator) factorD1(data RSITwInput, subs map[string]RSISubIndicator) float64 {
	const key = "d1_geopolitical"
	if data.GeopoliticalRisk > 0.5 {
		subs[key] = RSISubIndicator{Value: data.GeopoliticalRisk, ZScore: 0.85}
		return 0.85
	}
	subs[key] = RSISubIndicator{Value: data.GeopoliticalRisk, ZScore: 1.0}
	return 1.0
}

func (c *Calculator) factorD2(data RSITwInput, subs map[string]RSISubIndicator) float64 {
	const key = "d2_vix_spike"
	if data.VIXLevel > 30 {
		subs[key] = RSISubIndicator{Value: data.VIXLevel, ZScore: 0.90}
		return 0.90
	}
	subs[key] = RSISubIndicator{Value: data.VIXLevel, ZScore: 1.0}
	return 1.0
}

func (c *Calculator) factorD3(data RSITwInput, subs map[string]RSISubIndicator) float64 {
	const key = "d3_credit_control"
	if data.CreditTightening {
		subs[key] = RSISubIndicator{Value: 1, ZScore: 0.80, IsFallback: true}
		return 0.80
	}
	subs[key] = RSISubIndicator{ZScore: 1.0}
	return 1.0
}

func (c *Calculator) factorD4(subs map[string]RSISubIndicator) float64 {
	const key = "d4_flash_crash"
	subs[key] = RSISubIndicator{ZScore: 1.0, IsFallback: true}
	return 1.0
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// vixMap applies the piecewise VIX → sentiment mapping.
func vixMap(v float64) float64 {
	switch {
	case v < 15:
		return 0.1
	case v < 20:
		return 0.3
	case v < 25:
		return 0.5
	case v < 30:
		return 0.7
	case v < 35:
		return 0.85
	default:
		return 1.0
	}
}

func mean(vs []float64) float64 {
	var sum float64
	for _, v := range vs {
		sum += v
	}
	return sum / float64(len(vs))
}

func stddev(vs []float64, mean float64) float64 {
	var sumSq float64
	for _, v := range vs {
		d := v - mean
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(vs)))
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func round4(v float64) float64 {
	return math.Round(v*1e4) / 1e4
}
