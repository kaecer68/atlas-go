package portfolio

import (
	"context"
	"fmt"
	"math"
	"sort"

	"gonum.org/v1/gonum/mat"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func (o *Optimizer) aggregateRecommendations(
	recommendations []domain.Recommendation,
) map[string][]domain.Recommendation {
	aggregated := make(map[string][]domain.Recommendation)

	for _, rec := range recommendations {
		key := fmt.Sprintf("%s_%s", rec.Symbol, rec.Side)
		aggregated[key] = append(aggregated[key], rec)
	}

	return aggregated
}

// calculateMultiFactorScores 计算多因子评分
type symbolScore struct {
	Symbol                 string
	Side                   domain.Side
	Momentum               float64
	Value                  float64
	Quality                float64
	Agent                  float64
	InstitutionalSentiment float64
	Liquidity              float64
	Narrative              float64
	IndustryCycle          float64
	PreciousMetals         float64
	ETF                    float64
	Linkage                float64
	TSMC                   float64
	Total                  float64
	Agents                 []string
}

func (o *Optimizer) calculateMultiFactorScores(
	aggregated map[string][]domain.Recommendation,
	quotes map[string]domain.Quote,
	factorWeights map[FactorType]float64,
) map[string]*symbolScore {
	scores := make(map[string]*symbolScore)

	o.mu.RLock()
	agentWeights := o.agentWeights
	o.mu.RUnlock()

	for key, recs := range aggregated {
		if len(recs) == 0 {
			continue
		}

		symbol := recs[0].Symbol
		side := recs[0].Side

		// Agent 因子 (加权平均置信度)
		var agentScore float64
		var totalWeight float64
		var agents []string

		for _, rec := range recs {
			weight := 1.0
			if w, ok := agentWeights[rec.Agent]; ok {
				weight = w
			}
			agentScore += float64(rec.Conviction) * weight / 100.0
			totalWeight += weight
			agents = append(agents, rec.Agent)
		}

		if totalWeight > 0 {
			agentScore /= totalWeight
		}

		momentumScore := o.factorEngine.CalculateMomentumScore(symbol, quotes)
		valueScore := o.factorEngine.CalculateValueScore(symbol, quotes)
		qualityScore := o.factorEngine.CalculateQualityScore(symbol, quotes)
		pmScore := o.factorEngine.CalculatePreciousMetalsScore(context.Background(), symbol, quotes).Score
		etfScore := 0.0
		liqScore := 0.0
		if quote, ok := quotes[symbol]; ok {
			etfScore = o.factorEngine.CalculateETFScore(symbol, quote).Score
			liqScore = o.factorEngine.CalculateLiquidityScore(symbol, quotes).Score
		}

		var narrativeScore, industryCycleScore, linkageScore, instSentScore, tsmcScore float64
		o.mu.RLock()
		fe := o.factorEngine
		bridge := o.bridgeInput
		o.mu.RUnlock()
		if fe != nil {
			if bridge.ForeignFlowScore != 0 || bridge.MarginBalanceScore != 0 {
				instSentScore = fe.CalculateInstitutionalSentimentScore(bridge).Score
			}
			fe.mu.RLock()
			narProv := fe.narrativeProv
			iclProv := fe.cycleProv
			linkProv := fe.linkageProv
			tsmcProv := fe.tsmcProv
			fe.mu.RUnlock()
			if narProv != nil {
				if nfs := narProv(symbol); nfs != nil {
					narrativeScore = nfs.Score
				}
			}
			// Industry cycle does not apply to precious metals.
			if isPM := fe.IsPreciousMetal(symbol); !isPM {
				if iclProv != nil {
					if ics := iclProv(symbol); ics != nil {
						industryCycleScore = ics.Score
					}
				}
			}
			if isPM := fe.IsPreciousMetal(symbol); !isPM {
				if linkProv != nil {
					if lfs := linkProv(symbol); lfs != nil {
						linkageScore = lfs.Score
					}
				}
			}
			if tsmcProv != nil {
				if tfs := tsmcProv(symbol); tfs != nil {
					tsmcScore = tfs.Score
				}
			}
		}

		totalScore := momentumScore*factorWeights[FactorMomentum] +
			valueScore*factorWeights[FactorValue] +
			qualityScore*factorWeights[FactorQuality] +
			agentScore*factorWeights[FactorAgent]
		if instSentScore != 0 {
			totalScore += instSentScore * factorWeights[FactorInstSent]
		}
		if liqScore != 0 {
			totalScore += liqScore * factorWeights[FactorLiquidity]
		}
		if narrativeScore != 0 {
			totalScore += narrativeScore * factorWeights[FactorNarrative]
		}
		if industryCycleScore != 0 {
			totalScore += industryCycleScore * factorWeights[FactorIndustryCycle]
		}
		if pmScore != 0 {
			totalScore += pmScore * factorWeights[FactorPreciousMetals]
		}
		if etfScore != 0 {
			totalScore += etfScore * factorWeights[FactorETF]
		}
		if linkageScore != 0 {
			totalScore += linkageScore * factorWeights[FactorLinkage]
		}
		if tsmcScore != 0 {
			totalScore += tsmcScore * factorWeights[FactorTSMC]
		}

		scores[key] = &symbolScore{
			Symbol:                 symbol,
			Side:                   side,
			Momentum:               momentumScore,
			Value:                  valueScore,
			Quality:                qualityScore,
			Agent:                  agentScore,
			InstitutionalSentiment: instSentScore,
			Liquidity:              liqScore,
			Narrative:              narrativeScore,
			IndustryCycle:          industryCycleScore,
			PreciousMetals:         pmScore,
			ETF:                    etfScore,
			Linkage:                linkageScore,
			TSMC:                   tsmcScore,
			Total:                  totalScore,
			Agents:                 agents,
		}
	}

	return scores
}

// allocateInitialWeights 初始权重分配
type weightInfo struct {
	Symbol string
	Side   domain.Side
	Weight float64
	Score  float64
}

// allocateInitialWeights uses covariance optimization when history is available,
// falling back to linear score normalization otherwise.
func (o *Optimizer) allocateInitialWeights(
	scores map[string]*symbolScore,
) []weightInfo {
	if len(scores) == 0 {
		return nil
	}

	o.mu.RLock()
	wMax := o.constraints.MaxPositionPct
	o.mu.RUnlock()

	symbols := make([]string, 0, len(scores))
	for _, s := range scores {
		symbols = append(symbols, s.Symbol)
	}

	rm := o.extractReturnMatrix(symbols)
	if rm == nil || len(rm.assets) < 2 {
		return o.linearWeights(scores)
	}

	sample := o.sampleCov(rm)
	if sample == nil {
		return o.linearWeights(scores)
	}
	sigma := o.ledoitWolfShrink(rm, sample)

	N := len(rm.assets)

	o.mu.RLock()
	crisis := o.crisisMode
	o.mu.RUnlock()

	if crisis {
		// Inflate covariance diagonal: during extreme stress all assets become
		// more volatile and more correlated — diagonal inflation pushes QP toward
		// uniform weights (maximum diversification), countering panic concentration.
		for i := range N {
			oldVal := sigma.At(i, i)
			if oldVal > 0 {
				sigma.SetSym(i, i, oldVal*1.5)
			}
		}
		// Halve max position to force diversification during crisis.
		wMax = wMax / 2.0
		if wMax < 0.05 {
			wMax = 0.05
		}
	}

	// Cross-market correlation inflation: when SPX-TWSE correlation exceeds
	// 0.5, scale all off-diagonal entries proportionally. This captures the
	// "one market" effect — during global risk events, US and TW equities
	// move together, so diversification benefits shrink.
	o.mu.RLock()
	crossRho := o.crossMarketRho
	o.mu.RUnlock()
	if crossRho > 0.5 {
		scale := 1.0 + (crossRho-0.5)/0.5 // maps [0.5, 1.0] → [1.0, 2.0]
		for i := range N {
			for j := range N {
				if i != j {
					sigma.SetSym(i, j, sigma.At(i, j)*scale)
				}
			}
		}
	}

	scoreBySymbol := make(map[string]*symbolScore, len(scores))
	for _, s := range scores {
		scoreBySymbol[s.Symbol] = s
	}

	lb := make([]float64, N)
	ub := make([]float64, N)
	wInit := make([]float64, N)
	assetScores := make([]*symbolScore, N)

	for i, sym := range rm.assets {
		assetScores[i] = scoreBySymbol[sym]
		if assetScores[i] == nil {
			assetScores[i] = &symbolScore{Symbol: sym}
		}
		wInit[i] = 1.0 / float64(N)
		lb[i] = 0
		ub[i] = wMax
	}

	Aeq := mat.NewDense(1, N, nil)
	for j := range N {
		Aeq.Set(0, j, 1.0)
	}
	beq := []float64{1.0}

	wOpt := o.activeSetQP(sigma, Aeq, beq, lb, ub, wInit)

	var weights []weightInfo
	for i := range N {
		if wOpt[i] < 1e-10 {
			continue
		}
		weights = append(weights, weightInfo{
			Symbol: rm.assets[i],
			Side:   assetScores[i].Side,
			Weight: wOpt[i],
			Score:  assetScores[i].Total,
		})
	}

	sort.Slice(weights, func(i, j int) bool {
		return weights[i].Weight > weights[j].Weight
	})
	return weights
}

// linearWeights is the fallback linear normalization (no-history case).
func (o *Optimizer) linearWeights(scores map[string]*symbolScore) []weightInfo {
	var totalScore float64
	for _, s := range scores {
		totalScore += math.Abs(s.Total)
	}

	if totalScore == 0 {
		return nil
	}

	var weights []weightInfo
	for _, s := range scores {
		weight := math.Abs(s.Total) / totalScore
		weights = append(weights, weightInfo{
			Symbol: s.Symbol,
			Side:   s.Side,
			Weight: weight,
			Score:  s.Total,
		})
	}

	sort.Slice(weights, func(i, j int) bool {
		return weights[i].Weight > weights[j].Weight
	})

	return weights
}

// applyConstraints applies post-optimization constraints.
// This function serves both the QP path (where caps are already enforced by the solver)
// and the linear fallback path (where they must be applied here).
// All operations are idempotent — they are no-ops when constraints are already satisfied.
func (o *Optimizer) applyConstraints(
	weights []weightInfo,
	constraints Constraints,
	totalCapital float64,
) []weightInfo {
	if len(weights) == 0 {
		return nil
	}

	for i := range weights {
		if weights[i].Weight > constraints.MaxPositionPct {
			weights[i].Weight = constraints.MaxPositionPct
		}
	}

	var totalWeight float64
	for _, w := range weights {
		totalWeight += w.Weight
	}
	if totalWeight > 0 && totalWeight != 1.0 {
		scale := 1.0 / totalWeight
		for i := range weights {
			weights[i].Weight *= scale
		}
	}

	if constraints.CashReserve > 0 {
		investableCapital := totalCapital * (1 - constraints.CashReserve)
		currentExposure := 0.0
		for _, w := range weights {
			currentExposure += w.Weight * totalCapital
		}

		if currentExposure > investableCapital {
			scale := investableCapital / currentExposure
			for i := range weights {
				weights[i].Weight *= scale
			}
		}
	}

	return weights
}

// buildPositions 构建最终仓位
func (o *Optimizer) buildPositions(
	weights []weightInfo,
	scores map[string]*symbolScore,
	quotes map[string]domain.Quote,
	totalCapital float64,
) []OptimizedPosition {
	var positions []OptimizedPosition

	for _, w := range weights {
		key := fmt.Sprintf("%s_%s", w.Symbol, w.Side)
		score, ok := scores[key]
		if !ok {
			continue
		}

		quote, ok := quotes[w.Symbol]
		if !ok || quote.Last == 0 {
			continue
		}

		targetValue := w.Weight * totalCapital
		targetWeight := w.Weight

		shares := int(targetValue / quote.Last)
		if shares == 0 {
			continue
		}

		actualValue := float64(shares) * quote.Last

		factors := map[FactorType]float64{
			FactorMomentum:  score.Momentum,
			FactorValue:     score.Value,
			FactorQuality:   score.Quality,
			FactorAgent:     score.Agent,
			FactorInstSent:  score.InstitutionalSentiment,
			FactorLiquidity: score.Liquidity,
		}
		if score.Narrative != 0 {
			factors[FactorNarrative] = score.Narrative
		}
		if score.IndustryCycle != 0 {
			factors[FactorIndustryCycle] = score.IndustryCycle
		}
		if score.PreciousMetals != 0 {
			factors[FactorPreciousMetals] = score.PreciousMetals
		}
		if score.ETF != 0 {
			factors[FactorETF] = score.ETF
		}

		positions = append(positions, OptimizedPosition{
			Symbol:       w.Symbol,
			Side:         w.Side,
			TargetValue:  actualValue,
			TargetWeight: targetWeight,
			Confidence:   score.Total,
			Factors:      factors,
			Agents:       score.Agents,
		})
	}

	return positions
}

// ── Covariance-Based Portfolio Optimization ──
//
// Mathematical Model (Ledoit & Wolf, 2004; active-set QP)
// =========================================================
// Given N assets with daily return series rᵢ(t), i=1..N, t=1..T:
//
//    Sample covariance:  S_{ij} = (1/T) Σₜ (rᵢ(t)-μᵢ)(rⱼ(t)-μⱼ)
//    Average variance:   ν = (1/N) Σᵢ S_{ii}
//
//    Shrinkage target:   F = ν·I  (scaled identity)
//    Shrinkage estimator: Σ = (1-δ)·S + δ·F
//
//    Shrinkage intensity δ computed via:
//      π  = Σᵢⱼ π_{ij},   π_{ij} = (1/T) Σₜ ((r̃ᵢ(t)r̃ⱼ(t) - S_{ij})²)
//      ρ  = Σᵢ ρ_{ii},   ρ_{ii} = π_{ii}
//      γ  = Σᵢⱼ (S_{ij} - F_{ij})²
//      δ  = clamp₍₀,₁₎( (π - ρ) / (T·γ) ),  δ=0 if γ=0 or T=0
//
//    Portfolio optimization (min-variance, active-set QP):
//      minimize    (1/2) w'Σw
//      subject to  Σwᵢ = 1,  0 ≤ wᵢ ≤ w_max  (∀i)
//
//    KKT system per active-set iteration:
//      [2·Σ_FF   A']  [w_F]    [ -2·Σ_FA·w_A ]
//      [A        0 ]  [ λ ]  = [ b - A·w_A    ]
//
//    Fallback (singular KKT): gradient projection with re-normalization.
//    Fallback (no history, <2 assets): linear normalization by factor scores.
//
// Falsification Conditions
// =========================================================
// Scenario A (edge): N=2 assets, one high-volatility — low-volatility asset
//   must receive higher weight than equal-weight baseline.
//   Test: TestCovarianceOptimizer_N2EdgeCase
//
// Scenario B (stress): ±20% daily amplitude, 5 assets — shrinkage must
//   remain stable (no NaN), weights must be diversified (Herfindahl ≤ 0.5).
//   Test: TestCovarianceOptimizer_HighVolatilityStress
//
// Scenario C (correctness): N=4 assets, w_max=0.3 — every weight must
//   satisfy 0 ≤ wᵢ ≤ w_max, sum of weights = 1.0 ± 0.001.
//   Test: TestCovarianceOptimizer_CorrectnessCheck

// GetEfficientFrontier computes the mean-variance efficient frontier (20 points).
