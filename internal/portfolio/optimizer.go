package portfolio

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"sync"

	"gonum.org/v1/gonum/mat"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// FactorType 因子类型
type FactorType string

const (
	FactorMomentum       FactorType = "momentum"
	FactorValue          FactorType = "value"
	FactorQuality        FactorType = "quality"
	FactorAgent          FactorType = "agent"
	FactorInstSent       FactorType = "institutional_sentiment"
	FactorLiquidity      FactorType = "liquidity"
	FactorNarrative      FactorType = "narrative"
	FactorIndustryCycle  FactorType = "industry_cycle"
	FactorPreciousMetals FactorType = "precious_metals"
	FactorETF            FactorType = "etf"
	FactorLinkage        FactorType = "linkage"
	FactorTSMC           FactorType = "tsmc"
)

// FactorScore 因子评分
type FactorScore struct {
	Symbol string
	Factor FactorType
	Score  float64 // -1 到 1
	Weight float64
}

// OptimizedPosition 优化后的仓位
type OptimizedPosition struct {
	Symbol       string
	Side         domain.Side
	TargetValue  float64 // 目标金额
	TargetWeight float64 // 目标权重
	Confidence   float64 // 置信度 (0-1)
	Factors      map[FactorType]float64
	Agents       []string // 支持该仓位的 Agents
}

// Constraints 组合约束
type Constraints struct {
	MaxPositionPct   float64    // 单票最大权重
	MaxSectorPct     float64    // 行业最大权重
	MaxTurnoverDaily float64    // 日换手率上限
	TargetBeta       float64    // 目标 Beta
	BetaRange        [2]float64 // Beta 允许范围
	MinTradeSize     int        // 最小交易手数
	CashReserve      float64    // 现金储备比例
}

// DefaultConstraints 默认约束（使用运行时参数）
func DefaultConstraints() Constraints {
	params := DefaultRuntimeParameters()
	return Constraints{
		MaxPositionPct:   params.Optimizer.MaxPositionPct,
		MaxSectorPct:     params.Optimizer.MaxSectorPct,
		MaxTurnoverDaily: params.Optimizer.MaxTurnoverDaily,
		TargetBeta:       params.Optimizer.TargetBeta,
		BetaRange:        [2]float64{params.Optimizer.BetaRangeMin, params.Optimizer.BetaRangeMax},
		MinTradeSize:     params.Optimizer.MinTradeSize,
		CashReserve:      params.Optimizer.CashReserve,
	}
}

// Optimizer 组合优化器
type Optimizer struct {
	runtimeParams      *RuntimeParameters
	constraints        Constraints
	agentWeights       map[string]float64
	styleWeights       map[string]float64
	factorWeights      map[FactorType]float64
	history            *HistoricalPrices
	fundamentals       *FundamentalProvider
	factorEngine       *FactorEngine
	mu                 sync.RWMutex
	factorWeightEngine *FactorWeightEngine
	lookbackDays       int     // covariance estimation window
	riskFreeRate       float64 // annualized risk-free rate
	bridgeInput        FactorBridgeInput
	crisisMode         bool    // when true, inflate covariance diagonal + halve position limits
	crossMarketRho     float64 // SPX-TWSE dynamic correlation (default 0.5)
}

// NewOptimizer 创建优化器
func NewOptimizer() *Optimizer {
	params := DefaultRuntimeParameters()
	return newOptimizerWithParams(params)
}

func newOptimizerWithParams(params *RuntimeParameters) *Optimizer {
	constraints := Constraints{
		MaxPositionPct:   params.Optimizer.MaxPositionPct,
		MaxSectorPct:     params.Optimizer.MaxSectorPct,
		MaxTurnoverDaily: params.Optimizer.MaxTurnoverDaily,
		TargetBeta:       params.Optimizer.TargetBeta,
		BetaRange:        [2]float64{params.Optimizer.BetaRangeMin, params.Optimizer.BetaRangeMax},
		MinTradeSize:     params.Optimizer.MinTradeSize,
		CashReserve:      params.Optimizer.CashReserve,
	}

	factorWeights := make(map[FactorType]float64)
	for k, v := range params.Optimizer.FactorWeights {
		factorWeights[FactorType(k)] = v
	}

	return &Optimizer{
		runtimeParams:      params,
		constraints:        constraints,
		agentWeights:       make(map[string]float64),
		styleWeights:       make(map[string]float64),
		factorWeights:      factorWeights,
		factorEngine:       NewFactorEngine(),
		factorWeightEngine: NewFactorWeightEngine(),
		lookbackDays:       60,
		riskFreeRate:       0.015,
	}
}

// SetConstraints 设置约束
func (o *Optimizer) SetConstraints(c Constraints) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.constraints = c
}

// WithFactorEngine attaches a pre-configured factor engine.
func (o *Optimizer) WithFactorEngine(fe *FactorEngine) *Optimizer {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.factorEngine = fe
	return o
}

// WithHistoricalPrices attaches a historical price repository for momentum calc.
func (o *Optimizer) WithHistoricalPrices(hp *HistoricalPrices) *Optimizer {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.history = hp
	if o.factorEngine != nil {
		o.factorEngine.WithHistoricalPrices(hp)
	}
	return o
}

// WithFundamentalProvider attaches a fundamental data provider.
func (o *Optimizer) WithFundamentalProvider(fp *FundamentalProvider) *Optimizer {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.fundamentals = fp
	if o.factorEngine != nil {
		o.factorEngine.WithFundamentalProvider(fp)
	}
	return o
}

// SetAgentWeights 设置 Agent 权重
func (o *Optimizer) SetAgentWeights(weights map[string]float64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.agentWeights = weights
}

// SetStyleWeights 设置风格权重
func (o *Optimizer) SetStyleWeights(weights map[string]float64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.styleWeights = weights
}

// SetFactorWeights 设置因子权重
func (o *Optimizer) SetFactorWeights(weights map[FactorType]float64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.factorWeights = weights
}

func (o *Optimizer) WithFactorWeightEngine(fwe *FactorWeightEngine) *Optimizer {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.factorWeightEngine = fwe
	return o
}

func (o *Optimizer) WithBridgeInput(input FactorBridgeInput) *Optimizer {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.bridgeInput = input
	return o
}

func (o *Optimizer) WithCrossMarketCorrelation(rho float64) *Optimizer {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.crossMarketRho = rho
	return o
}

// SetCrisisMode activates or deactivates crisis mode.
// In crisis mode (e.g. VIX >= 35), covariance diagonal is inflated by 150% and
// MaxPositionPct is halved to force diversification during extreme market stress.
func (o *Optimizer) SetCrisisMode(active bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.crisisMode = active
}

// IsCrisisMode returns whether crisis mode is active.
func (o *Optimizer) IsCrisisMode() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.crisisMode
}

// SetCrossMarketCorrelation updates the dynamic SPX-TWSE correlation used
// to inflate off-diagonal covariance entries during extreme cross-market stress.
func (o *Optimizer) SetCrossMarketCorrelation(rho float64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.crossMarketRho = rho
}

// GetCrossMarketCorrelation returns the current cross-market correlation value.
func (o *Optimizer) GetCrossMarketCorrelation() float64 {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.crossMarketRho
}

// Optimize 执行组合优化
func (o *Optimizer) Optimize(
	ctx context.Context,
	recommendations []domain.Recommendation,
	quotes map[string]domain.Quote,
	totalCapital float64,
) ([]OptimizedPosition, error) {
	if len(recommendations) == 0 {
		return nil, nil
	}

	o.mu.RLock()
	constraints := o.constraints
	factorWeights := o.factorWeights
	fwe := o.factorWeightEngine
	o.mu.RUnlock()

	if fwe != nil {
		factorWeights = fwe.GetWeights("")
	}

	aggregated := o.aggregateRecommendations(recommendations)
	scores := o.calculateMultiFactorScores(aggregated, quotes, factorWeights)

	weights := o.allocateInitialWeights(scores)
	weights = o.applyConstraints(weights, constraints, totalCapital)
	positions := o.buildPositions(weights, scores, quotes, totalCapital)

	return positions, nil
}

// aggregateRecommendations 聚合相同股票的推荐
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
		for i := 0; i < N; i++ {
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
		for i := 0; i < N; i++ {
			for j := 0; j < N; j++ {
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
	for j := 0; j < N; j++ {
		Aeq.Set(0, j, 1.0)
	}
	beq := []float64{1.0}

	wOpt := o.activeSetQP(sigma, Aeq, beq, lb, ub, wInit)

	var weights []weightInfo
	for i := 0; i < N; i++ {
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

// OptimizeToOrders 将优化结果转换为订单
func (o *Optimizer) OptimizeToOrders(
	ctx context.Context,
	recommendations []domain.Recommendation,
	quotes map[string]domain.Quote,
	totalCapital float64,
) ([]domain.Order, error) {
	positions, err := o.Optimize(ctx, recommendations, quotes, totalCapital)
	if err != nil {
		return nil, err
	}

	var orders []domain.Order

	for _, pos := range positions {
		quote, ok := quotes[pos.Symbol]
		if !ok {
			continue
		}

		shares := int(pos.TargetValue / quote.Last)
		if shares == 0 {
			continue
		}

		orders = append(orders, domain.Order{
			Symbol:   pos.Symbol,
			Side:     pos.Side,
			Quantity: shares,
			Price:    quote.Last,
			Reason:   fmt.Sprintf("optimized_position|weight:%.2f%%", pos.TargetWeight*100),
		})
	}

	return orders, nil
}

// ── Multi-Day Drawdown Simulation (P1) ──
// Uses the covariance matrix from P0-1 (Ledoit-Wolf shrinkage) to generate
// correlated multi-day return paths via Monte Carlo, then computes drawdown
// statistics for stress testing.

// DrawdownResult holds stress-test drawdown metrics for a portfolio.
type DrawdownResult struct {
	MaxDrawdown float64   // worst peak-to-trough across all paths
	VaR95       float64   // 95% Value-at-Risk (5th percentile of terminal return)
	WorstPath   []float64 // cumulative return of the worst-drawdown path
}

// SimulateDrawdown runs a Monte Carlo drawdown simulation.
//
// weights: portfolio weights (from Optimize output).
// volatilityScale: stress multiplier on daily volatility (e.g., 4.0 for VIX=80 vs normal VIX=20).
// numDays: trading days to simulate (e.g., 21 for ~1 month).
// numPaths: number of Monte Carlo paths (e.g., 1000).
//
// Uses the Cholesky decomposition of the shrunken covariance matrix to
// generate correlated normal returns: r_t = L · z_t, z_t ~ N(0, σ²·I).
func (o *Optimizer) SimulateDrawdown(
	weights []weightInfo,
	volatilityScale float64,
	numDays, numPaths int,
) DrawdownResult {
	symbols := make([]string, len(weights))
	for i, w := range weights {
		symbols[i] = w.Symbol
	}

	rm := o.extractReturnMatrix(symbols)
	if rm == nil || len(rm.assets) < 2 {
		return DrawdownResult{}
	}

	sample := o.sampleCov(rm)
	if sample == nil {
		return DrawdownResult{}
	}
	sigma := o.ledoitWolfShrink(rm, sample)

	N := len(rm.assets)

	// Build weight vector aligned with rm.assets order.
	w := make([]float64, N)
	weightBySym := make(map[string]float64, len(weights))
	for _, wi := range weights {
		weightBySym[wi.Symbol] = wi.Weight
	}
	for i, sym := range rm.assets {
		w[i] = weightBySym[sym]
	}

	// Cholesky decomposition of shrunken covariance.
	var chol mat.Cholesky
	if ok := chol.Factorize(sigma); !ok {
		return DrawdownResult{}
	}
	L := mat.NewTriDense(N, false, nil)
	chol.LTo(L)

	rng := rand.New(rand.NewPCG(42, 0))

	z := make([]float64, N)
	r := make([]float64, N)

	terminalReturns := make([]float64, numPaths)
	worstDD := 0.0
	var worstPath []float64

	for p := 0; p < numPaths; p++ {
		cumulative := 1.0
		peak := 1.0
		pathDD := 0.0
		pathVals := make([]float64, 0, numDays+1)
		pathVals = append(pathVals, 1.0)

		for d := 0; d < numDays; d++ {
			// Generate independent standard normals, then scale.
			for i := 0; i < N; i++ {
				z[i] = boxMuller(rng) * volatilityScale
			}

			// Correlated returns: r = L · z.
			for i := 0; i < N; i++ {
				var sum float64
				for j := 0; j <= i; j++ {
					// L is lower-triangular; L.At(i,j) accesses row i, col j.
					// Realistically the Cholesky factor from gonum is stored in
					// a dense TriDense; use L.At(i, j) for lower-tri access.
					sum += L.At(i, j) * z[j]
				}
				r[i] = sum
			}

			// Portfolio return = w' · r.
			portRet := 0.0
			for i := 0; i < N; i++ {
				portRet += w[i] * r[i]
			}

			cumulative *= (1 + portRet)
			pathVals = append(pathVals, cumulative)

			if cumulative > peak {
				peak = cumulative
			}
			dd := (peak - cumulative) / peak
			if dd > pathDD {
				pathDD = dd
			}
		}

		terminalReturns[p] = cumulative - 1.0

		if pathDD > worstDD {
			worstDD = pathDD
			worstPath = pathVals
		}
	}

	// Sort terminal returns for VaR computation.
	sort.Float64s(terminalReturns)
	idx95 := int(0.05 * float64(numPaths))
	vaR95 := -terminalReturns[idx95]

	return DrawdownResult{
		MaxDrawdown: worstDD,
		VaR95:       vaR95,
		WorstPath:   worstPath,
	}
}

// boxMuller generates a standard normal random variate.
func boxMuller(rng *rand.Rand) float64 {
	u1 := rng.Float64()
	u2 := rng.Float64()
	return math.Sqrt(-2*math.Log(max(u1, 1e-10))) * math.Cos(2*math.Pi*u2)
}

// GetCovarianceMatrix returns the Ledoit-Wolf shrunken covariance matrix for the
// given symbols as a plain [][]float64, plus the subset of symbols with sufficient
// history. Returns nil if fewer than 2 symbols have data. Designed for the stress
// test runner (P1) to enable correlated noise generation.
func (o *Optimizer) GetCovarianceMatrix(symbols []string) ([][]float64, []string) {
	rm := o.extractReturnMatrix(symbols)
	if rm == nil || len(rm.assets) < 2 {
		return nil, nil
	}
	sample := o.sampleCov(rm)
	if sample == nil {
		return nil, nil
	}
	sigma := o.ledoitWolfShrink(rm, sample)
	N := len(rm.assets)
	mat := make([][]float64, N)
	for i := 0; i < N; i++ {
		mat[i] = make([]float64, N)
		for j := 0; j < N; j++ {
			mat[i][j] = sigma.At(i, j)
		}
	}
	return mat, rm.assets
}

// SimulateDrawdownForMonitoring converts domain.Position to weightInfo and runs
// a standard 21-day, 1000-path Monte Carlo drawdown simulation. Used by the
// orchestrator's session-end hook for monitoring (P3-4).
func (o *Optimizer) SimulateDrawdownForMonitoring(positions []domain.Position, portfolioValue float64) DrawdownResult {
	if portfolioValue <= 0 {
		return DrawdownResult{}
	}
	weights := make([]weightInfo, 0, len(positions))
	for _, p := range positions {
		w := p.MarketValue / portfolioValue
		if w <= 0 {
			continue
		}
		weights = append(weights, weightInfo{
			Symbol: p.Symbol,
			Weight: w,
		})
	}
	if len(weights) == 0 {
		return DrawdownResult{}
	}
	return o.SimulateDrawdown(weights, 1.0, 21, 1000)
}
