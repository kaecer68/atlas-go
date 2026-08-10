// Package retail provides RSI-tw (Retail Sentiment Index — Taiwan).
package retail

import (
	"math"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
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
	params        config.RSITwParameters
	lastScore     float64
}

var (
	calcOnce     sync.Once
	calcInstance *Calculator
)

// GetCalculator returns the singleton Calculator instance.
func GetCalculator() *Calculator {
	calcOnce.Do(func() {
		calcInstance = &Calculator{
			marginHistory: make([]float64, 0, 90),
			vixHistory:    make([]float64, 0, 90),
			params:        config.DefaultParametersConfig().RSITw,
		}
	})
	return calcInstance
}

// LastScore returns the most recent RSI-tw score, or 0 if none computed yet.
func (c *Calculator) LastScore() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastScore
}

// NewCalculator returns an initialized Calculator with empty histories.
// Deprecated: use GetCalculator() for the singleton; this is kept for tests.
func NewCalculator() *Calculator {
	return &Calculator{
		marginHistory: make([]float64, 0, 90),
		vixHistory:    make([]float64, 0, 90),
		params:        config.DefaultParametersConfig().RSITw,
	}
}

// SetParams updates the calculator's parameter set at runtime.
func (c *Calculator) SetParams(p config.RSITwParameters) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.params = p
}

// ---------------------------------------------------------------------------
// Public methods
// ---------------------------------------------------------------------------

// ComputeFinal calculates the full RSI-tw snapshot for the given input.
func (c *Calculator) ComputeFinal(data RSITwInput) RSITwSnapshot {
	c.mu.RLock()
	marginSnap := make([]float64, len(c.marginHistory))
	copy(marginSnap, c.marginHistory)
	params := c.params // snapshot params under lock to avoid data race with SetParams
	c.mu.RUnlock()

	subs := make(map[string]RSISubIndicator, 9)
	partA := c.computePartA(data, marginSnap, subs, &params)
	partC := c.computePartC(data, subs, &params)
	adj := c.computeAdjustmentFactor(data, subs, &params)
	final := (partA*params.APartWeight.Value + partC*params.CPartWeight.Value) * adj
	final = clamp(final, -1.0, 1.0)

	c.mu.Lock()
	c.lastScore = final
	c.mu.Unlock()

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

func (c *Calculator) computePartA(data RSITwInput, marginHistory []float64, subs map[string]RSISubIndicator, params *config.RSITwParameters) float64 {
	var total float64

	total += c.subA1(data, marginHistory, subs, params)
	total += c.subA2(data, subs, params)
	total += c.subA3(data, subs, params)
	total += c.subA4(data, subs, params)
	total += c.subA5(data, subs, params)
	total += c.subA6(data, subs, params)

	return clamp(total, -1.0, 1.0)
}

// A1: Margin Balance Δ Z-score (weight from params)
func (c *Calculator) subA1(data RSITwInput, history []float64, subs map[string]RSISubIndicator, params *config.RSITwParameters) float64 {
	w := params.A1Weight.Value
	ind := RSISubIndicator{Weight: w, Value: data.MarginBalance}

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
	return ind.ZScore * w
}

// A2: Day Trading Ratio (weight 0.20)
func (c *Calculator) subA2(data RSITwInput, subs map[string]RSISubIndicator, params *config.RSITwParameters) float64 {
	w := params.A2Weight.Value
	ind := RSISubIndicator{Weight: w}

	if data.DayTrading == nil {
		ind.IsFallback = true
		subs["a2_day_trading"] = ind
		return 0
	}

	ind.Value = data.DayTrading.Volume
	ind.ZScore = data.DayTrading.VolumeRatio
	subs["a2_day_trading"] = ind
	return ind.ZScore * w
}

func (c *Calculator) subA3(data RSITwInput, subs map[string]RSISubIndicator, params *config.RSITwParameters) float64 {
	w := params.A3Weight.Value
	ind := RSISubIndicator{
		Weight: w,
		Value:  data.MarginPercentile,
	}
	ind.ZScore = clamp((data.MarginPercentile-params.A3Midpoint.Value)*params.A3Scale.Value, -1.0, 1.0)
	subs["a3_margin_maint"] = ind
	return ind.ZScore * w
}

func (c *Calculator) subA4(data RSITwInput, subs map[string]RSISubIndicator, params *config.RSITwParameters) float64 {
	w := params.A4Weight.Value
	ind := RSISubIndicator{Weight: w, Value: data.VIXLevel}

	if data.VIXLevel <= 0 {
		ind.ZScore = 0.5
		ind.IsFallback = true
		subs["a4_vix_map"] = ind
		return ind.ZScore * w
	}

	ind.ZScore = vixMapParam(data.VIXLevel, params)
	subs["a4_vix_map"] = ind
	return ind.ZScore * w
}

func (c *Calculator) subA5(data RSITwInput, subs map[string]RSISubIndicator, params *config.RSITwParameters) float64 {
	w := params.A5Weight.Value
	pcr := data.PutCallRatio
	var score float64
	if pcr == 0 {
		score = params.A5PcrFallback.Value
	} else {
		ts := params.A5PcrThresholds.Value
		ss := params.A5PcrScores.Value
		score = ss[len(ss)-1] // default
		for i, t := range ts {
			if pcr > t {
				score = ss[i]
				break
			}
		}
	}
	ind := RSISubIndicator{Value: pcr, Weight: w, ZScore: score, IsFallback: pcr == 0}
	subs["a5_pcr_proxy"] = ind
	return score * w
}

func (c *Calculator) subA6(data RSITwInput, subs map[string]RSISubIndicator, params *config.RSITwParameters) float64 {
	w := params.A6Weight.Value
	imb := data.OddLotImbalance
	var score float64
	if imb == 0 {
		score = params.A6OddLotFallback.Value
	} else {
		ts := params.A6OddLotThresholds.Value
		ss := params.A6OddLotScores.Value
		score = ss[len(ss)-1]
		for i, t := range ts {
			if imb > t {
				score = ss[i]
				break
			}
		}
	}
	ind := RSISubIndicator{Value: imb, Weight: w, ZScore: score, IsFallback: imb == 0}
	subs["a6_odd_lot"] = ind
	return score * w
}

// ---------------------------------------------------------------------------
// Part C — Institutional / Derivative Flow (25 % weight)
// ---------------------------------------------------------------------------

func (c *Calculator) computePartC(data RSITwInput, subs map[string]RSISubIndicator, params *config.RSITwParameters) float64 {
	var total float64

	total += c.subC1(data, subs, params)
	total += c.subC2(data, subs, params)
	total += c.subC3(data, subs, params)

	return clamp(total, -1.0, 1.0)
}

// C1: Small TAIEX Futures OI (weight 0.40)
func (c *Calculator) subC1(data RSITwInput, subs map[string]RSISubIndicator, params *config.RSITwParameters) float64 {
	w := params.C1Weight.Value
	pct := data.RetailFuturesPct
	var score float64
	if pct == 0 {
		score = 0.5
	} else if pct > params.C1VeryBullishThreshold.Value {
		score = 0.9
	} else if pct > params.C1BullishThreshold.Value {
		score = 0.7
	} else if pct > params.C1BearishThreshold.Value {
		score = 0.5
	} else if pct > params.C1VeryBearishThreshold.Value {
		score = 0.25
	} else {
		score = 0.1
	}
	ind := RSISubIndicator{Value: pct, Weight: w, ZScore: score, IsFallback: pct == 0}
	subs["c1_futures_oi"] = ind
	return score * w
}

func (c *Calculator) subC2(data RSITwInput, subs map[string]RSISubIndicator, params *config.RSITwParameters) float64 {
	w := params.C2Weight.Value
	netFlow := data.ForeignInvestorNet + data.DomesticFundNet
	ind := RSISubIndicator{Weight: w, Value: netFlow}
	if netFlow == 0 {
		ind.IsFallback = true
		subs["c2_inst_flow"] = ind
		return 0
	}

	scaling := params.C2NetflowScalingFactor.Value
	mid := params.C2NeutralMidpoint.Value
	score := mid + (netFlow / scaling)
	ind.ZScore = clamp(score, 0.1, 0.9)
	subs["c2_inst_flow"] = ind
	return ind.ZScore * w
}

func (c *Calculator) subC3(data RSITwInput, subs map[string]RSISubIndicator, params *config.RSITwParameters) float64 {
	w := params.C3Weight.Value
	netSub := data.ETFNetSubscription
	// B03（2026-08-10 audit）：ETF 申購贖回資料源（TWSE TWT44U）已永久失效 —
	// 容器內實測 HTTP 307 → page-not-found.html（2026 年 ETF 申贖平台改版為
	// 投信/參與券商內部作業平台），且無公開替代源（FinMind 無此 dataset）。
	// netSub==0（資料不可用）時不貢獻 Part C，與 subC2 的 IsFallback pattern
	// 一致 — 資料缺失反映為「該維度無訊號」而非舊的 0.5 中性假裝。
	if netSub == 0 {
		subs["c3_etf_sub"] = RSISubIndicator{Value: 0, Weight: w, ZScore: 0, IsFallback: true}
		return 0
	}
	var score float64
	if netSub > params.C3VeryBullishThreshold.Value {
		score = 0.9
	} else if netSub > params.C3BullishThreshold.Value {
		score = 0.7
	} else if netSub > 0 {
		score = 0.55
	} else if netSub > params.C3BearishThreshold.Value {
		score = 0.45
	} else {
		score = 0.2
	}
	ind := RSISubIndicator{Value: netSub, Weight: w, ZScore: score, IsFallback: false}
	subs["c3_etf_sub"] = ind
	return score * w
}

// ---------------------------------------------------------------------------
// Part D — Adjustment Factor (0.8 – 1.2)
// ---------------------------------------------------------------------------

func (c *Calculator) computeAdjustmentFactor(data RSITwInput, subs map[string]RSISubIndicator, params *config.RSITwParameters) float64 {
	factor := 1.0

	// D1: Geopolitical Risk (multiplier 0.85)
	factor *= c.factorD1(data, subs, params)

	// D2: VIX Spike Override (multiplier 0.90)
	factor *= c.factorD2(data, subs, params)

	// D3: Credit Control Signal (multiplier 0.80)
	factor *= c.factorD3(data, subs, params)

	// [INTENTIONAL PLACEHOLDER] D4: Military / Flash Crash (multiplier 1.15).
	// Audit 2026-07-06: hardcoded return in factorD4(); no data integration yet.
	factor *= c.factorD4(subs)

	return clamp(factor, 0.8, 1.2)
}

func (c *Calculator) factorD1(data RSITwInput, subs map[string]RSISubIndicator, params *config.RSITwParameters) float64 {
	const key = "d1_geopolitical"
	if data.GeopoliticalRisk > params.DGeoPoliticalRiskThreshold.Value {
		mult := params.DGeoPoliticalRiskMultiplier.Value
		subs[key] = RSISubIndicator{Value: data.GeopoliticalRisk, ZScore: mult}
		return mult
	}
	subs[key] = RSISubIndicator{Value: data.GeopoliticalRisk, ZScore: 1.0}
	return 1.0
}

func (c *Calculator) factorD2(data RSITwInput, subs map[string]RSISubIndicator, params *config.RSITwParameters) float64 {
	const key = "d2_vix_spike"
	if data.VIXLevel > params.DVIXSpikeThreshold.Value {
		mult := params.DVIXSpikeMultiplier.Value
		subs[key] = RSISubIndicator{Value: data.VIXLevel, ZScore: mult}
		return mult
	}
	subs[key] = RSISubIndicator{Value: data.VIXLevel, ZScore: 1.0}
	return 1.0
}

func (c *Calculator) factorD3(data RSITwInput, subs map[string]RSISubIndicator, params *config.RSITwParameters) float64 {
	const key = "d3_credit_control"
	if data.CreditTightening {
		mult := params.DCreditTighteningMultiplier.Value
		subs[key] = RSISubIndicator{Value: 1, ZScore: mult, IsFallback: true}
		return mult
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

// vixMapParam applies the piecewise VIX → sentiment mapping using parameterized thresholds/scores.
func vixMapParam(v float64, params *config.RSITwParameters) float64 {
	ts := params.A4VixThresholds.Value
	ss := params.A4VixScores.Value
	for i, t := range ts {
		if v < t {
			return ss[i]
		}
	}
	return ss[len(ss)-1]
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
