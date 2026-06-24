package portfolio

import (
	"context"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// CalculateAllScores returns momentum, value, quality, agent, institutional sentiment,
// and liquidity scores. The agent score is computed from the provided recommendations
// for the symbol. An optional FactorBridgeInput can be provided for macro-aware factors.
//
// Includes all 12 factors conditionally:
//   - Agent: always present (from agentRecs filtered by symbol)
//   - Narrative / IndustryCycle / Linkage / TSMC: present iff the matching
//     provider was attached via With*Provider
//   - PreciousMetals: present iff isPreciousMetal(symbol)
//   - ETF: present iff etfAnalyzer attached AND ea.IsETF(symbol)
//   - InstitutionalSentiment / Liquidity: present iff len(bridgeInputs) > 0
//
// If factorWeights is non-empty, a "total" key is added to the result map
// equal to sum(score * weight) across all factors (with no fallback reduction).
// Use CalculateAllScoresWithBreakdown for the SCOR-04 fallback-aware total.
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
//
// Implements SCOR-04: when computing the weighted total, fallback-flagged
// factors receive FallbackWeightReduction (default 0.5) of their nominal weight.
// This downweights noisy estimates so they don't dominate the score.
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
