package portfolio

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// RefreshETFNAV refreshes ETF NAV values for all tracked ETFs using
// the given QuoteProvider to fetch market quotes. Returns the number of
// symbols whose NAV was updated. If the provider is nil or no
// ETFAnalyzer is attached, returns 0.
func (fe *FactorEngine) RefreshETFNAV(ctx context.Context, provider QuoteProvider) int {
	fe.mu.RLock()
	ea := fe.etfAnalyzer
	fe.mu.RUnlock()
	if ea == nil || provider == nil {
		return 0
	}

	symbols := ea.AllSymbols()
	if len(symbols) == 0 {
		return 0
	}

	quotes, err := provider.GetQuotes(ctx, time.Now(), symbols)
	if err != nil {
		logging.Warn("factor_engine", "refresh_etf_nav_failed", logging.Err(err))
		return 0
	}

	return ea.UpdateNAVFromQuotes(quotes)
}

// CalculateMomentumScore computes momentum based on price change over the configured lookback period.
// Falls back to intraday return when no historical data is available.
func (fe *FactorEngine) CalculateMomentumScore(symbol string, quotes map[string]domain.Quote) float64 {
	return fe.calculateMomentumDetail(context.Background(), symbol, quotes).Score
}

// calculateMomentumDetail returns the full breakdown for momentum calculation.
func (fe *FactorEngine) calculateMomentumDetail(ctx context.Context, symbol string, quotes map[string]domain.Quote) domain.FactorScoreItem {
	fe.mu.RLock()
	hp := fe.history
	fe.mu.RUnlock()

	if hp != nil {
		if err := fe.ensureAdjusted(ctx, symbol); err != nil {
			logging.Warn("factor_engine", "ensure_adjusted_failed", logging.Symbol(symbol), logging.Err(err))
		}
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
// Precious metals (gold, silver) have no P/E or P/B — returns 0.
func (fe *FactorEngine) calculateValueDetail(symbol string, quotes map[string]domain.Quote) domain.FactorScoreItem {
	_ = quotes

	if isPM, _ := isPreciousMetal(symbol); isPM {
		return domain.FactorScoreItem{
			Score:      0.0,
			Formula:    "precious_metal: P/E not applicable",
			RawInputs:  map[string]float64{},
			IsFallback: true,
		}
	}
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
	return fe.calculateQualityDetail(context.Background(), symbol).Score
}

// calculateQualityDetail returns the full breakdown for quality calculation.
// Precious metals (gold, silver) have no ROE/profit-margin — returns 0.
func (fe *FactorEngine) calculateQualityDetail(ctx context.Context, symbol string) domain.FactorScoreItem {
	if isPM, _ := isPreciousMetal(symbol); isPM {
		return domain.FactorScoreItem{
			Score:      0.0,
			Formula:    "precious_metal: ROE not applicable",
			RawInputs:  map[string]float64{},
			IsFallback: true,
		}
	}
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
		if err := fe.ensureAdjusted(ctx, symbol); err != nil {
			logging.Warn("factor_engine", "ensure_adjusted_failed", logging.Symbol(symbol), logging.Err(err))
		}
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
	retailWeight := weights["retail"]
	score := foreignWeight*input.ForeignFlowScore +
		domesticWeight*input.DomesticFlowScore +
		marginWeight*input.MarginBalanceScore +
		retailWeight*input.RetailSentimentScore
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}
	return domain.FactorScoreItem{
		Score:   score,
		Formula: fmt.Sprintf("%.2f*ForeignFlowScore + %.2f*DomesticFlowScore + %.2f*MarginBalanceScore + %.2f*RetailSentimentScore", foreignWeight, domesticWeight, marginWeight, retailWeight),
		RawInputs: map[string]float64{
			"foreign_score":   input.ForeignFlowScore,
			"domestic_score":  input.DomesticFlowScore,
			"margin_score":    input.MarginBalanceScore,
			"retail_score":    input.RetailSentimentScore,
			"foreign_weight":  foreignWeight,
			"domestic_weight": domesticWeight,
			"margin_weight":   marginWeight,
			"retail_weight":   retailWeight,
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

	// Precious Metals factor
	if isPM, _ := isPreciousMetal(symbol); isPM {
		pmScore := fe.CalculatePreciousMetalsScore(context.Background(), symbol, quotes)
		result[FactorPreciousMetals] = pmScore.Score
	}

	// ETF factor
	fe.mu.RLock()
	ea := fe.etfAnalyzer
	fe.mu.RUnlock()
	if ea != nil && ea.IsETF(symbol) {
		if quote, ok := quotes[symbol]; ok {
			etfScore := fe.CalculateETFScore(symbol, quote)
			result[FactorETF] = etfScore.Score
		}
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
	mom := fe.calculateMomentumDetail(context.Background(), symbol, quotes)
	val := fe.calculateValueDetail(symbol, quotes)
	qly := fe.calculateQualityDetail(context.Background(), symbol)

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

	var nar, icl, link, tsmc domain.FactorScoreItem
	fe.mu.RLock()
	narProv := fe.narrativeProv
	iclProv := fe.cycleProv
	linkProv := fe.linkageProv
	tsmcProv := fe.tsmcProv
	fe.mu.RUnlock()

	if narProv != nil {
		if nfs := narProv(symbol); nfs != nil {
			nar = domain.FactorScoreItem{
				Score:     nfs.Score,
				Formula:   fmt.Sprintf("narrative(theme=%s, hit_rate=%.2f)", nfs.Theme, nfs.HitRate),
				RawInputs: map[string]float64{"theme_hit_rate": nfs.HitRate, "confidence": nfs.Confidence},
			}
			result[FactorNarrative] = nar.Score
		}
	}
	if iclProv != nil {
		if ics := iclProv(symbol); ics != nil {
			icl = domain.FactorScoreItem{
				Score:     ics.Score,
				Formula:   fmt.Sprintf("industry_cycle(phase=%s, phase_score=%.2f)", ics.Phase, ics.PhaseScore),
				RawInputs: map[string]float64{"phase_score": ics.PhaseScore, "confidence": ics.Confidence},
			}
			result[FactorIndustryCycle] = icl.Score
		}
	}
	if linkProv != nil {
		if lfs := linkProv(symbol); lfs != nil {
			link = domain.FactorScoreItem{
				Score:     lfs.Score,
				Formula:   fmt.Sprintf("linkage(systemic=%.2f, propagation=%.2f)", lfs.SystemicImportance, lfs.ShockPropagation),
				RawInputs: map[string]float64{"systemic_importance": lfs.SystemicImportance, "shock_propagation_speed": lfs.ShockPropagation, "avg_correlation": lfs.AvgCorrelation},
			}
			result[FactorLinkage] = link.Score
		}
	}
	if tsmcProv != nil {
		if tfs := tsmcProv(symbol); tfs != nil {
			tsmc = *tfs
			result[FactorTSMC] = tfs.Score
		}
	}

	breakdown := &domain.FactorScoreBreakdown{
		Momentum:               mom,
		Value:                  val,
		Quality:                qly,
		Agent:                  agent,
		InstitutionalSentiment: instSent,
		Liquidity:              liq,
		Narrative:              nar,
		IndustryCycle:          icl,
		Linkage:                link,
		TSMC:                   tsmc,
	}

	// Precious Metals: compute PM score when symbol is a known PM instrument.
	var pm domain.FactorScoreItem
	if isPM, _ := isPreciousMetal(symbol); isPM {
		pm = fe.CalculatePreciousMetalsScore(context.Background(), symbol, quotes)
		result[FactorPreciousMetals] = pm.Score
		breakdown.PreciousMetals = pm
	}

	// ETF: compute ETF factor score when ETF analyzer data is available.
	var etf domain.FactorScoreItem
	fe.mu.RLock()
	ea := fe.etfAnalyzer
	fe.mu.RUnlock()
	if ea != nil && ea.IsETF(symbol) {
		if quote, ok := quotes[symbol]; ok {
			etf = fe.CalculateETFScore(symbol, quote)
			result[FactorETF] = etf.Score
			breakdown.ETF = etf
		}
	}

	if len(bridgeInputs) > 0 {
		result[FactorInstSent] = instSent.Score
		result[FactorLiquidity] = liq.Score
	}

	// SCOR-04: Apply reduced weight for fallback factors in total calculation
	if len(factorWeights) > 0 {
		total := 0.0
		rawTotal := map[string]float64{}

		getEffectiveWeight := func(item domain.FactorScoreItem, defaultWeight float64) float64 {
			if item.IsFallback {
				return defaultWeight * fe.params.Factor.FallbackWeightReduction
			}
			return defaultWeight
		}

		momWeight := getEffectiveWeight(mom, factorWeights[FactorMomentum])
		total += mom.Score * momWeight
		rawTotal[string(FactorMomentum)] = mom.Score * momWeight

		valWeight := getEffectiveWeight(val, factorWeights[FactorValue])
		total += val.Score * valWeight
		rawTotal[string(FactorValue)] = val.Score * valWeight

		qlyWeight := getEffectiveWeight(qly, factorWeights[FactorQuality])
		total += qly.Score * qlyWeight
		rawTotal[string(FactorQuality)] = qly.Score * qlyWeight

		agentWeight := getEffectiveWeight(agent, factorWeights[FactorAgent])
		total += agent.Score * agentWeight
		rawTotal[string(FactorAgent)] = agent.Score * agentWeight

		if len(bridgeInputs) > 0 {
			instWeight := getEffectiveWeight(instSent, factorWeights[FactorInstSent])
			total += instSent.Score * instWeight
			rawTotal[string(FactorInstSent)] = instSent.Score * instWeight

			liqWeight := getEffectiveWeight(liq, factorWeights[FactorLiquidity])
			total += liq.Score * liqWeight
			rawTotal[string(FactorLiquidity)] = liq.Score * liqWeight
		}

		if nar.Score != 0 || nar.Formula != "" {
			narWeight := getEffectiveWeight(nar, factorWeights[FactorNarrative])
			total += nar.Score * narWeight
			rawTotal[string(FactorNarrative)] = nar.Score * narWeight
			breakdown.Narrative.Weight = factorWeights[FactorNarrative]
		}
		if icl.Score != 0 || icl.Formula != "" {
			iclWeight := getEffectiveWeight(icl, factorWeights[FactorIndustryCycle])
			total += icl.Score * iclWeight
			rawTotal[string(FactorIndustryCycle)] = icl.Score * iclWeight
			breakdown.IndustryCycle.Weight = factorWeights[FactorIndustryCycle]
		}
		if pm.Score != 0 || pm.Formula != "" {
			pmWeight := getEffectiveWeight(pm, factorWeights[FactorPreciousMetals])
			total += pm.Score * pmWeight
			rawTotal[string(FactorPreciousMetals)] = pm.Score * pmWeight
			breakdown.PreciousMetals.Weight = factorWeights[FactorPreciousMetals]
		}
		if etf.Score != 0 || etf.Formula != "" {
			etfWeight := getEffectiveWeight(etf, factorWeights[FactorETF])
			total += etf.Score * etfWeight
			rawTotal[string(FactorETF)] = etf.Score * etfWeight
			breakdown.ETF.Weight = factorWeights[FactorETF]
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

// CalculateETFScore returns an ETF-specific composite factor score.
// Delegates to the attached ETFAnalyzer. Returns a zero-score fallback if no analyzer is attached.
func (fe *FactorEngine) CalculateETFScore(symbol string, quote domain.Quote) domain.FactorScoreItem {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	if fe.etfAnalyzer == nil {
		return domain.FactorScoreItem{
			Score:      0.0,
			Formula:    "etf_score: no analyzer attached",
			RawInputs:  map[string]float64{},
			IsFallback: true,
		}
	}
	return fe.etfAnalyzer.CalculateETFScore(symbol, quote)
}

// ── Precious Metals Factor (P0-2) ──
// SOURCE: Erb & Harvey (2013), World Gold Council, P0-2 task brief
//
// Composite precious metals score for gold:
//   PM_gold = 0.16·RealRate + 0.10·DXY + 0.10·InflExp + 0.12·CB + 0.08·Flow + 0.06·PhyDem + 0.10·COMEX + 0.28·RiskOff
// For silver:
//   PM_silver = 0.60·PM_gold + 0.15·Industrial + 0.10·GoldSilver + 0.15·COMEX

// CalculatePreciousMetalsScore returns the composite precious metals factor score.
// Returns 0 if symbol is not a known precious metal instrument.
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

func (fe *FactorEngine) getPMContext(symbol string) *PreciousMetalsContext {
	fe.mu.RLock()
	defer fe.mu.RUnlock()
	if fe.pmCtxProv != nil {
		return fe.pmCtxProv(symbol)
	}
	return nil
}

// pmRealRateScore: −normalize(r_real). Higher real rates → lower gold demand.
// Uses linear scaling with center at 1.5%: r=0.5%→+0.67, r=2%→−0.33.
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

// pmDXYScore: −normalize(DXY change). Stronger dollar → lower gold.
// Center at 100; ±10 point move = ±1.0.
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

// pmInflationExpectScore: normalize(CPI). Higher inflation → higher gold.
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
// ReserveTrend > 0.3 → +0.7, > 0 → +0.3, > −0.3 → 0, > −1 → −0.2.
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
// GLD > 5% → +0.5 (strong inflows), > 0 → +0.2, < −5% → −0.3, else 0.
// Falls back to 0 if GLD price history unavailable.
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

// pmRiskOffScore: normalize(VIX). Higher VIX → higher gold.
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
// Normalizes imports to [-1, 1]: 10% YoY → +1.0, −10% → −1.0, linear between.
// 0.5 * India score + 0.5 * China score.
func (fe *FactorEngine) pmPhysicalDemandScore(ctx *PreciousMetalsContext) float64 {
	if ctx == nil {
		return 0
	}
	indiaScore := fe.normalizeImportYoY(ctx.IndiaGoldImportsYoY)
	chinaScore := fe.normalizeImportYoY(ctx.ChinaGoldImportsYoY)
	return 0.5*indiaScore + 0.5*chinaScore
}

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

// pmCOMEXScore: contrarian COT signal. COMEX net long > 200k → −0.5 (too bullish),
// < 50k → +0.5 (too bearish), between → 0.
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

// pmIndustrialDemandScore: VIX as PMI proxy. VIX < 15 → +0.5 (strong PMI),
// VIX < 20 → +0.2, VIX > 25 → −0.3, VIX > 30 → −0.5.
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

// pmGoldSilverRatioScore: mean-reversion signal. GoldSilverRatioZ > 1.5 → +0.5
// (gold overvalued, bullish silver mean reversion). Z < −1.5 → −0.3.
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
