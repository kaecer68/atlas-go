package portfolio

import (
	"fmt"
	"math"
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func isFinite(f float64) bool {
	return !math.IsInf(f, 0) && !math.IsNaN(f)
}

type FactorEngine struct {
	history      *HistoricalPrices
	fundamentals *FundamentalProvider
	params       *RuntimeParameters
	mu           sync.RWMutex
}

func NewFactorEngine() *FactorEngine {
	return &FactorEngine{
		params: DefaultRuntimeParameters(),
	}
}

// WithHistoricalPrices attaches a historical price repository for momentum calc.
func (fe *FactorEngine) WithHistoricalPrices(hp *HistoricalPrices) *FactorEngine {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.history = hp
	return fe
}

// WithFundamentalProvider attaches a fundamental data provider.
func (fe *FactorEngine) WithFundamentalProvider(fp *FundamentalProvider) *FactorEngine {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.fundamentals = fp
	return fe
}

// WithParameters sets the runtime parameters for factor calculations.
// This allows configuration of lookback periods, thresholds, and weights
// without changing the public API. Returns the FactorEngine for chaining.
func (fe *FactorEngine) WithParameters(p *RuntimeParameters) *FactorEngine {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.params = p
	return fe
}

// CalculateMomentumScore computes momentum based on price change over the configured lookback period.
// Falls back to intraday return when no historical data is available.
func (fe *FactorEngine) CalculateMomentumScore(symbol string, quotes map[string]domain.Quote) float64 {
	return fe.calculateMomentumDetail(symbol, quotes).Score
}

// calculateMomentumDetail returns the full breakdown for momentum calculation.
func (fe *FactorEngine) calculateMomentumDetail(symbol string, quotes map[string]domain.Quote) domain.FactorScoreItem {
	fe.mu.RLock()
	hp := fe.history
	fe.mu.RUnlock()

	if hp != nil {
		ret := hp.MomentumReturn(symbol, fe.params.Factor.MomentumLookbackDays)
		if ret != 0 {
			score := ret / fe.params.Factor.MomentumStdDevDivisor
			if score > 1.0 {
				score = 1.0
			}
			if score < -1.0 {
				score = -1.0
			}
			return domain.FactorScoreItem{
				Score:     score,
				Formula:   fmt.Sprintf("clamp(ret%d / %.2f, -1, 1)", fe.params.Factor.MomentumLookbackDays, fe.params.Factor.MomentumStdDevDivisor),
				RawInputs: map[string]float64{fmt.Sprintf("ret%d", fe.params.Factor.MomentumLookbackDays): ret},
			}
		}
	}

	quote, ok := quotes[symbol]
	if !ok || quote.Open == 0 {
		return domain.FactorScoreItem{
			Score:      0.0,
			Formula:    fmt.Sprintf("clamp(intraday / %.2f * %.1f, -1, 1)", fe.params.Factor.MomentumIntradayThreshold, fe.params.Factor.MomentumIntradayDiscount),
			RawInputs:  map[string]float64{"open": 0, "last": 0},
			IsFallback: true,
		}
	}
	intradayReturn := (quote.Last - quote.Open) / quote.Open
	score := intradayReturn / fe.params.Factor.MomentumIntradayThreshold * fe.params.Factor.MomentumIntradayDiscount
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}
	return domain.FactorScoreItem{
		Score:      score,
		Formula:    fmt.Sprintf("clamp(intraday / %.2f * %.1f, -1, 1)", fe.params.Factor.MomentumIntradayThreshold, fe.params.Factor.MomentumIntradayDiscount),
		RawInputs:  map[string]float64{"open": quote.Open, "last": quote.Last, "intraday_return": intradayReturn},
		IsFallback: true,
	}
}

// CalculateValueScore computes value based on P/E and P/B from fundamentals.
// Falls back to a mild positive constant when no data is available.
func (fe *FactorEngine) CalculateValueScore(symbol string, quotes map[string]domain.Quote) float64 {
	return fe.calculateValueDetail(symbol, quotes).Score
}

// calculateValueDetail returns the full breakdown for value calculation.
// Implements SCOR-02 (industry-relative P/E) and SCOR-03 (negative/undefined P/E handling).
func (fe *FactorEngine) calculateValueDetail(symbol string, quotes map[string]domain.Quote) domain.FactorScoreItem {
	_ = quotes
	fe.mu.RLock()
	fp := fe.fundamentals
	fe.mu.RUnlock()

	if fp != nil && fp.HasData() {
		data := fp.Get(symbol)
		score := 0.0
		count := 0
		raw := map[string]float64{}
		var formula string
		var isFallback bool

		// SCOR-03: Handle negative/undefined P/E
		// Check if P/E is valid (positive and not NaN/Inf)
		peValid := data.PE > 0 && isFinite(data.PE)

		if peValid {
			// SCOR-02: Industry-relative P/E comparison
			sector := data.Sector
			if sector == "" {
				sector = "other" // Default to "other" if no sector specified
			}
			sectorMedianPE := fp.SectorMedianPE(sector)

			var peScore float64
			if sectorMedianPE > 0 {
				// Relative P/E: PE / SectorMedianPE
				// PE = sector median → score 1.0
				// PE = 2x median → score 0.0
				// PE = 0.5x median → score 1.5 (capped)
				relativePE := data.PE / sectorMedianPE
				peScore = 1.0 - (relativePE - 1.0)
				raw["sector_median_pe"] = sectorMedianPE
				raw["relative_pe"] = relativePE
				formula = "clamp(1 - (PE/sectorMedianPE - 1), -1, 1)"
			} else {
				// Fallback to absolute P/E if no sector data available
				peScore = 1.0 - (data.PE-fe.params.Factor.ValuePERangeCenter)/fe.params.Factor.ValuePERangeWidth
				formula = fmt.Sprintf("clamp(1 - (PE-%.2f)/%.2f, -1, 1)", fe.params.Factor.ValuePERangeCenter, fe.params.Factor.ValuePERangeWidth)
			}

			if peScore > 1.0 {
				peScore = 1.0
			}
			if peScore < -1.0 {
				peScore = -1.0
			}
			score += peScore
			count++
			raw["pe"] = data.PE
			raw["pe_score"] = peScore
		} else {
			// P/E is invalid (negative, zero, NaN, or Inf)
			// Try P/B first
			if data.PB > 0 && isFinite(data.PB) {
				pbScore := 1.0 - (data.PB-fe.params.Factor.ValuePBRangeCenter)/fe.params.Factor.ValuePBRangeWidth
				if pbScore > 1.0 {
					pbScore = 1.0
				}
				if pbScore < -1.0 {
					pbScore = -1.0
				}
				score += pbScore
				count++
				raw["pb"] = data.PB
				raw["pb_score"] = pbScore
				raw["pe_switched_to_pb"] = 1.0 // Mark that we switched from P/E to P/B
				formula = fmt.Sprintf("clamp(1 - (PB-%.2f)/%.2f, -1, 1)", fe.params.Factor.ValuePBRangeCenter, fe.params.Factor.ValuePBRangeWidth)
			} else if data.PS > 0 && isFinite(data.PS) {
				// P/B also invalid, try P/S
				psScore := 1.0 - (data.PS-fe.params.Factor.ValuePSRangeCenter)/fe.params.Factor.ValuePSRangeWidth
				if psScore > 1.0 {
					psScore = 1.0
				}
				if psScore < -1.0 {
					psScore = -1.0
				}
				score += psScore
				count++
				raw["ps"] = data.PS
				raw["ps_score"] = psScore
				raw["pe_switched_to_ps"] = 1.0 // Mark that we switched from P/E to P/S
				formula = fmt.Sprintf("clamp(1 - (PS-%.2f)/%.2f, -1, 1)", fe.params.Factor.ValuePSRangeCenter, fe.params.Factor.ValuePSRangeWidth)
			} else {
				// All value metrics invalid, use fallback
				isFallback = true
				formula = "fallback: no valid value metrics"
			}
		}

		// If P/E was valid, also include P/B as secondary metric
		if peValid && data.PB > 0 && isFinite(data.PB) {
			pbScore := 1.0 - (data.PB-fe.params.Factor.ValuePBRangeCenter)/fe.params.Factor.ValuePBRangeWidth
			if pbScore > 1.0 {
				pbScore = 1.0
			}
			if pbScore < -1.0 {
				pbScore = -1.0
			}
			score += pbScore
			count++
			raw["pb"] = data.PB
			raw["pb_score"] = pbScore
			formula = fmt.Sprintf("avg(clamp(1 - (PE/sectorMedianPE - 1), -1, 1), clamp(1 - (PB-%.2f)/%.2f, -1, 1))", fe.params.Factor.ValuePBRangeCenter, fe.params.Factor.ValuePBRangeWidth)
		}

		if count > 0 {
			return domain.FactorScoreItem{
				Score:      score / float64(count),
				Formula:    formula,
				RawInputs:  raw,
				IsFallback: isFallback,
			}
		}
	}
	return domain.FactorScoreItem{
		Score:      fe.params.Factor.ValueFallbackScore,
		Formula:    "fallback: no fundamentals available",
		RawInputs:  map[string]float64{},
		IsFallback: true,
	}
}

// CalculateQualityScore computes quality based on dividend yield and price stability.
// Falls back to a mild positive constant when no data is available.
func (fe *FactorEngine) CalculateQualityScore(symbol string, quotes map[string]domain.Quote) float64 {
	return fe.calculateQualityDetail(symbol, quotes).Score
}

// calculateQualityDetail returns the full breakdown for quality calculation.
func (fe *FactorEngine) calculateQualityDetail(symbol string, quotes map[string]domain.Quote) domain.FactorScoreItem {
	fe.mu.RLock()
	fp := fe.fundamentals
	hp := fe.history
	fe.mu.RUnlock()

	score := 0.0
	count := 0
	raw := map[string]float64{}

	if fp != nil && fp.HasData() {
		data := fp.Get(symbol)
		if data.DividendYield > 0 {
			dyScore := data.DividendYield / fe.params.Factor.QualityDividendYieldCap
			if dyScore > 1.0 {
				dyScore = 1.0
			}
			score += dyScore
			count++
			raw["dividend_yield"] = data.DividendYield
			raw["dividend_yield_score"] = dyScore
		}
	}

	if hp != nil {
		vol := hp.Volatility(symbol, fe.params.Factor.MomentumLookbackDays)
		if vol > 0 {
			volScore := 1.0 - vol/fe.params.Factor.QualityVolatilityStd
			if volScore > 1.0 {
				volScore = 1.0
			}
			if volScore < -1.0 {
				volScore = -1.0
			}
			score += volScore
			count++
			raw[fmt.Sprintf("volatility_%dd", fe.params.Factor.MomentumLookbackDays)] = vol
			raw["volatility_score"] = volScore
		}
	}

	if count > 0 {
		return domain.FactorScoreItem{
			Score:     score / float64(count),
			Formula:   fmt.Sprintf("avg(DividendYield/%.2f, clamp(1 - Vol%dd/%.2f, -1, 1))", fe.params.Factor.QualityDividendYieldCap, fe.params.Factor.MomentumLookbackDays, fe.params.Factor.QualityVolatilityStd),
			RawInputs: raw,
		}
	}
	return domain.FactorScoreItem{
		Score:      fe.params.Factor.QualityFallbackScore,
		Formula:    fmt.Sprintf("avg(DividendYield/%.2f, clamp(1 - Vol%dd/%.2f, -1, 1))", fe.params.Factor.QualityDividendYieldCap, fe.params.Factor.MomentumLookbackDays, fe.params.Factor.QualityVolatilityStd),
		RawInputs:  map[string]float64{},
		IsFallback: true,
	}
}

func (fe *FactorEngine) CalculateInstitutionalSentimentScore(input FactorBridgeInput) domain.FactorScoreItem {
	weights := fe.params.Factor.InstitutionalSentimentWeights
	foreignWeight := weights["foreign"]
	domesticWeight := weights["domestic"]
	marginWeight := weights["margin"]
	score := foreignWeight*input.ForeignFlowScore +
		domesticWeight*input.DomesticFlowScore +
		marginWeight*input.MarginBalanceScore
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}
	return domain.FactorScoreItem{
		Score:   score,
		Formula: fmt.Sprintf("%.2f*ForeignFlowScore + %.2f*DomesticFlowScore + %.2f*MarginBalanceScore", foreignWeight, domesticWeight, marginWeight),
		RawInputs: map[string]float64{
			"foreign_score":   input.ForeignFlowScore,
			"domestic_score":  input.DomesticFlowScore,
			"margin_score":    input.MarginBalanceScore,
			"foreign_weight":  foreignWeight,
			"domestic_weight": domesticWeight,
			"margin_weight":   marginWeight,
		},
	}
}

func (fe *FactorEngine) CalculateLiquidityScore(symbol string, quotes map[string]domain.Quote) domain.FactorScoreItem {
	quote, ok := quotes[symbol]
	if !ok || quote.Open == 0 || quote.Volume == 0 {
		return domain.FactorScoreItem{
			Score:      0.0,
			Formula:    "-log(abs(return) / volume)",
			RawInputs:  map[string]float64{},
			IsFallback: true,
		}
	}
	ret := (quote.Last - quote.Open) / quote.Open
	if ret == 0 {
		return domain.FactorScoreItem{
			Score:     0.0,
			Formula:   "-log(abs(0) / volume) = 0",
			RawInputs: map[string]float64{"return": 0, "volume": float64(quote.Volume)},
		}
	}
	illiq := math.Abs(ret) / float64(quote.Volume)
	score := -math.Log(illiq + 1e-10)
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}
	return domain.FactorScoreItem{
		Score:   score,
		Formula: "clamp(-log(abs(return)/volume), -1, 1)",
		RawInputs: map[string]float64{
			"return": ret,
			"volume": float64(quote.Volume),
			"illiq":  illiq,
		},
	}
}

// CalculateAllScores returns momentum, value, quality, agent, institutional sentiment,
// and liquidity scores. The agent score is computed from the provided recommendations
// for the symbol. An optional FactorBridgeInput can be provided for macro-aware factors.
func (fe *FactorEngine) CalculateAllScores(
	symbol string,
	quotes map[string]domain.Quote,
	agentRecs []domain.Recommendation,
	agentWeights map[string]float64,
	factorWeights map[FactorType]float64,
	bridgeInputs ...FactorBridgeInput,
) map[FactorType]float64 {
	momentumScore := fe.CalculateMomentumScore(symbol, quotes)
	valueScore := fe.CalculateValueScore(symbol, quotes)
	qualityScore := fe.CalculateQualityScore(symbol, quotes)

	var agentScore float64
	var totalWeight float64
	for _, rec := range agentRecs {
		if rec.Symbol != symbol {
			continue
		}
		weight := 1.0
		if w, ok := agentWeights[rec.Agent]; ok {
			weight = w
		}
		agentScore += float64(rec.Conviction) * weight / 100.0
		totalWeight += weight
	}
	if totalWeight > 0 {
		agentScore /= totalWeight
	}

	result := map[FactorType]float64{
		FactorMomentum: momentumScore,
		FactorValue:    valueScore,
		FactorQuality:  qualityScore,
		FactorAgent:    agentScore,
	}

	// Add macro-aware factors if bridge input available
	if len(bridgeInputs) > 0 {
		input := bridgeInputs[0]
		instSentScore := fe.CalculateInstitutionalSentimentScore(input)
		liqScore := fe.CalculateLiquidityScore(symbol, quotes)
		result[FactorInstSent] = instSentScore.Score
		result[FactorLiquidity] = liqScore.Score
	}

	// Compute weighted total if factorWeights provided
	if len(factorWeights) > 0 {
		total := 0.0
		for ft, score := range result {
			if w, ok := factorWeights[ft]; ok {
				total += score * w
			}
		}
		result["total"] = total
	}

	return result
}

// CalculateAllScoresWithBreakdown returns both the score map and a detailed
// breakdown showing formulas, raw inputs, and fallback flags per factor.
// An optional FactorBridgeInput can be provided for macro-aware institutional sentiment and liquidity.
func (fe *FactorEngine) CalculateAllScoresWithBreakdown(
	symbol string,
	quotes map[string]domain.Quote,
	agentRecs []domain.Recommendation,
	agentWeights map[string]float64,
	factorWeights map[FactorType]float64,
	bridgeInputs ...FactorBridgeInput,
) (*domain.FactorScoreBreakdown, map[FactorType]float64) {
	mom := fe.calculateMomentumDetail(symbol, quotes)
	val := fe.calculateValueDetail(symbol, quotes)
	qly := fe.calculateQualityDetail(symbol, quotes)

	var agentScore float64
	var totalWeight float64
	rawAgent := map[string]float64{}
	for _, rec := range agentRecs {
		if rec.Symbol != symbol {
			continue
		}
		weight := 1.0
		if w, ok := agentWeights[rec.Agent]; ok {
			weight = w
		}
		agentScore += float64(rec.Conviction) * weight / 100.0
		totalWeight += weight
		rawAgent[rec.Agent+"_conviction"] = float64(rec.Conviction)
		rawAgent[rec.Agent+"_weight"] = weight
	}
	if totalWeight > 0 {
		agentScore /= totalWeight
	}

	agent := domain.FactorScoreItem{
		Score:     agentScore,
		Formula:   "weighted_avg(Conviction / 100)",
		RawInputs: rawAgent,
	}

	var instSent, liq domain.FactorScoreItem
	if len(bridgeInputs) > 0 {
		input := bridgeInputs[0]
		instSent = fe.CalculateInstitutionalSentimentScore(input)
		liq = fe.CalculateLiquidityScore(symbol, quotes)
	}

	result := map[FactorType]float64{
		FactorMomentum: mom.Score,
		FactorValue:    val.Score,
		FactorQuality:  qly.Score,
		FactorAgent:    agent.Score,
	}

	breakdown := &domain.FactorScoreBreakdown{
		Momentum:               mom,
		Value:                  val,
		Quality:                qly,
		Agent:                  agent,
		InstitutionalSentiment: instSent,
		Liquidity:              liq,
	}

	if len(bridgeInputs) > 0 {
		result[FactorInstSent] = instSent.Score
		result[FactorLiquidity] = liq.Score
	}

	// SCOR-04: Apply reduced weight for fallback factors in total calculation
	if len(factorWeights) > 0 {
		total := 0.0
		rawTotal := map[string]float64{}

		getEffectiveWeight := func(ft FactorType, item domain.FactorScoreItem, defaultWeight float64) float64 {
			if item.IsFallback {
				return defaultWeight * fe.params.Factor.FallbackWeightReduction
			}
			return defaultWeight
		}

		momWeight := getEffectiveWeight(FactorMomentum, mom, factorWeights[FactorMomentum])
		total += mom.Score * momWeight
		rawTotal[string(FactorMomentum)] = mom.Score * momWeight

		valWeight := getEffectiveWeight(FactorValue, val, factorWeights[FactorValue])
		total += val.Score * valWeight
		rawTotal[string(FactorValue)] = val.Score * valWeight

		qlyWeight := getEffectiveWeight(FactorQuality, qly, factorWeights[FactorQuality])
		total += qly.Score * qlyWeight
		rawTotal[string(FactorQuality)] = qly.Score * qlyWeight

		agentWeight := getEffectiveWeight(FactorAgent, agent, factorWeights[FactorAgent])
		total += agent.Score * agentWeight
		rawTotal[string(FactorAgent)] = agent.Score * agentWeight

		if len(bridgeInputs) > 0 {
			instWeight := getEffectiveWeight(FactorInstSent, instSent, factorWeights[FactorInstSent])
			total += instSent.Score * instWeight
			rawTotal[string(FactorInstSent)] = instSent.Score * instWeight

			liqWeight := getEffectiveWeight(FactorLiquidity, liq, factorWeights[FactorLiquidity])
			total += liq.Score * liqWeight
			rawTotal[string(FactorLiquidity)] = liq.Score * liqWeight
		}

		result["total"] = total
		breakdown.Total = domain.FactorScoreItem{
			Score:     total,
			Formula:   "sum(factor_score * effective_weight)",
			RawInputs: rawTotal,
		}
		breakdown.Momentum.Weight = factorWeights[FactorMomentum]
		breakdown.Value.Weight = factorWeights[FactorValue]
		breakdown.Quality.Weight = factorWeights[FactorQuality]
		breakdown.Agent.Weight = factorWeights[FactorAgent]
		if len(bridgeInputs) > 0 {
			breakdown.InstitutionalSentiment.Weight = factorWeights[FactorInstSent]
			breakdown.Liquidity.Weight = factorWeights[FactorLiquidity]
		}
	}

	return breakdown, result
}
