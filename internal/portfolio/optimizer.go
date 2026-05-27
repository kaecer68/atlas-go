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
		pmScore := o.factorEngine.CalculatePreciousMetalsScore(symbol, quotes).Score
		etfScore := 0.0
		liqScore := 0.0
		if quote, ok := quotes[symbol]; ok {
			etfScore = o.factorEngine.CalculateETFScore(symbol, quote).Score
			liqScore = o.factorEngine.CalculateLiquidityScore(symbol, quotes).Score
		}

		var narrativeScore, industryCycleScore, instSentScore float64
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

	scoreBySymbol := make(map[string]*symbolScore, len(scores))
	for _, s := range scores {
		scoreBySymbol[s.Symbol] = s
	}

	N := len(rm.assets)
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
//
// Data Dependencies
// =========================================================
// | Data               | Source                         | Computation          |
// |--------------------|--------------------------------|----------------------|
// | Daily price series | HistoricalPrices.GetCloseSeries | (Pₜ-Pₜ₋₁)/Pₜ₋₁     |
// | Factor scores      | FactorEngine                   | Momentum/Value/Quality|
// | Constraints        | configs/parameters.json         | w_max, cash_reserve  |
//
// Parameter Provenance
// =========================================================
// | Parameter    | Value  | Source                                        |
// |-------------|--------|-----------------------------------------------|
// | lookbackDays | 60     | Academic consensus (60-120 days for covariance)|
// | riskFreeRate | 0.015  | Taiwan 1Y government bond (≈1.5% p.a.)        |
// | w_max        | 0.15   | Constraints.MaxPositionPct (config-driven)     |
// | QP maxIter   | 100    | Standard active-set convergence bound          |
// | QP tol       | 1e-10  | Numerical stability for KKT optimality check   |

type returnMatrix struct {
	assets  []string
	returns [][]float64 // returns[t][i], T rows × N cols
	means   []float64
}

func (o *Optimizer) extractReturnMatrix(symbols []string) *returnMatrix {
	const minDays = 20
	o.mu.RLock()
	hp := o.history
	lookback := o.lookbackDays
	o.mu.RUnlock()

	if hp == nil {
		return nil
	}

	series := make([][]float64, 0, len(symbols))
	valid := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		prices := hp.GetCloseSeries(sym)
		if len(prices) < minDays+1 {
			continue
		}
		n := len(prices)
		start := n - lookback - 1
		if start < 0 {
			start = 0
		}
		window := prices[start:]
		if len(window) < minDays+1 {
			continue
		}
		series = append(series, window)
		valid = append(valid, sym)
	}

	N := len(valid)
	if N < 2 {
		return nil
	}

	T := len(series[0])
	for _, s := range series {
		if len(s) < T {
			T = len(s)
		}
	}
	if T < minDays+1 {
		return nil
	}

	Tret := T - 1
	ret := make([][]float64, Tret)
	for t := 0; t < Tret; t++ {
		ret[t] = make([]float64, N)
	}
	means := make([]float64, N)

	for i := 0; i < N; i++ {
		s := series[i]
		offset := len(s) - T
		sum := 0.0
		for t := 0; t < Tret; t++ {
			prev := s[offset+t]
			curr := s[offset+t+1]
			if prev == 0 {
				continue
			}
			r := curr/prev - 1
			ret[t][i] = r
			sum += r
		}
		means[i] = sum / float64(Tret)
	}

	return &returnMatrix{assets: valid, returns: ret, means: means}
}

func (o *Optimizer) sampleCov(rm *returnMatrix) *mat.SymDense {
	T := float64(len(rm.returns))
	N := len(rm.assets)
	if N == 0 || T == 0 {
		return nil
	}

	cov := mat.NewSymDense(N, nil)
	for i := 0; i < N; i++ {
		for j := i; j < N; j++ {
			var sum float64
			for t := 0; t < len(rm.returns); t++ {
				dx := rm.returns[t][i] - rm.means[i]
				dy := rm.returns[t][j] - rm.means[j]
				sum += dx * dy
			}
			cov.SetSym(i, j, sum/T)
		}
	}
	return cov
}

// ledoitWolfShrink: Σ_shrink = (1-δ)·S + δ·ν·I  (Ledoit & Wolf, 2004)
func (o *Optimizer) ledoitWolfShrink(rm *returnMatrix, sample *mat.SymDense) *mat.SymDense {
	N := len(rm.assets)
	T := float64(len(rm.returns))
	S := sample

	nu := 0.0
	for i := 0; i < N; i++ {
		nu += S.At(i, i)
	}
	nu /= float64(N)

	var pi, rho float64
	demeaned := make([][]float64, len(rm.returns))
	for t := 0; t < len(rm.returns); t++ {
		demeaned[t] = make([]float64, N)
		for i := 0; i < N; i++ {
			demeaned[t][i] = rm.returns[t][i] - rm.means[i]
		}
	}

	for i := 0; i < N; i++ {
		for j := 0; j < N; j++ {
			sij := S.At(i, j)
			var ssq float64
			for t := 0; t < len(demeaned); t++ {
				d := demeaned[t][i]*demeaned[t][j] - sij
				ssq += d * d
			}
			pij := ssq / T
			pi += pij
			if i == j {
				rho += pij
			}
		}
	}

	var gamma float64
	for i := 0; i < N; i++ {
		for j := 0; j < N; j++ {
			target := 0.0
			if i == j {
				target = nu
			}
			d := S.At(i, j) - target
			gamma += d * d
		}
	}

	var delta float64
	if gamma == 0 || T == 0 {
		delta = 0
	} else {
		delta = (pi - rho) / (T * gamma)
		if delta < 0 {
			delta = 0
		}
		if delta > 1 {
			delta = 1
		}
	}

	shrunk := mat.NewSymDense(N, nil)
	for i := 0; i < N; i++ {
		for j := i; j < N; j++ {
			val := (1 - delta) * S.At(i, j)
			if i == j {
				val += delta * nu
			}
			shrunk.SetSym(i, j, val)
		}
	}
	return shrunk
}

// activeSetQP solves: minimize ½ w' Σ w  s.t. A_eq' w = b_eq, lb ≤ w ≤ ub.
func (o *Optimizer) activeSetQP(sigma *mat.SymDense, Aeq *mat.Dense, beq []float64,
	lb, ub []float64, wInit []float64,
) []float64 {
	N := sigma.SymmetricDim()
	if N == 0 {
		return nil
	}
	mEq := len(beq)

	w := make([]float64, N)
	copy(w, wInit)

	const maxIter = 100
	const tol = 1e-10

	active := make([]bool, N)

	for iter := 0; iter < maxIter; iter++ {
		isActive := make([]bool, N)
		for i := 0; i < N; i++ {
			isActive[i] = active[i] || w[i] <= lb[i]+tol || w[i] >= ub[i]-tol
		}

		freeIdx := make([]int, 0, N)
		for i := 0; i < N; i++ {
			if !isActive[i] {
				freeIdx = append(freeIdx, i)
			}
		}
		nFree := len(freeIdx)

		if nFree == 0 {
			if o.isOptimal(sigma, w, isActive, lb, ub) {
				return w
			}
			o.releaseConstraint(sigma, w, &active, isActive, lb, ub)
			continue
		}

		// KKT system: [2·Σ_FF  A_eq_F'] [w_F] = [-2·Σ_FA·w_A]
		//             [A_eq_F    0   ] [ λ ]   [b_eq - A_eq_A·w_A]
		kktDim := nFree + mEq
		kkt := mat.NewDense(kktDim, kktDim, nil)

		for p := 0; p < nFree; p++ {
			for q := p; q < nFree; q++ {
				val := 2 * sigma.At(freeIdx[p], freeIdx[q])
				kkt.Set(p, q, val)
				if p != q {
					kkt.Set(q, p, val)
				}
			}
		}

		for p := 0; p < nFree; p++ {
			for k := 0; k < mEq; k++ {
				kkt.Set(p, nFree+k, Aeq.At(k, freeIdx[p]))
				kkt.Set(nFree+k, p, Aeq.At(k, freeIdx[p]))
			}
		}

		rhs := make([]float64, kktDim)
		for p := 0; p < nFree; p++ {
			var sum float64
			for j := 0; j < N; j++ {
				if isActive[j] {
					sum += sigma.At(freeIdx[p], j) * w[j]
				}
			}
			rhs[p] = -2 * sum
		}
		for k := 0; k < mEq; k++ {
			Aw := 0.0
			for j := 0; j < N; j++ {
				if isActive[j] {
					Aw += Aeq.At(k, j) * w[j]
				}
			}
			rhs[nFree+k] = beq[k] - Aw
		}

		rhsVec := mat.NewVecDense(kktDim, rhs)
		var soln mat.VecDense
		if err := soln.SolveVec(kkt, rhsVec); err != nil {
			return o.gradientProjection(sigma, w, lb, ub)
		}

		wFree := make([]float64, nFree)
		for p := 0; p < nFree; p++ {
			wFree[p] = soln.AtVec(p)
		}

		d := make([]float64, N)
		for p, fi := range freeIdx {
			d[fi] = wFree[p] - w[fi]
		}

		normD := 0.0
		for _, dv := range d {
			normD += dv * dv
		}
		if normD < tol*tol {
			if o.isOptimal(sigma, w, isActive, lb, ub) {
				return w
			}
			o.releaseConstraint(sigma, w, &active, isActive, lb, ub)
			continue
		}

		alpha := 1.0
		blockingIdx := -1
		for i := 0; i < N; i++ {
			if d[i] > tol {
				a := (ub[i] - w[i]) / d[i]
				if a < alpha {
					alpha = a
					blockingIdx = i
				}
			} else if d[i] < -tol {
				a := (lb[i] - w[i]) / d[i]
				if a < alpha {
					alpha = a
					blockingIdx = i
				}
			}
		}

		for i := 0; i < N; i++ {
			w[i] += alpha * d[i]
		}

		if blockingIdx >= 0 {
			active[blockingIdx] = true
		} else {
			if o.isOptimal(sigma, w, isActive, lb, ub) {
				return w
			}
			o.releaseConstraint(sigma, w, &active, isActive, lb, ub)
		}
	}

	return w
}

func (o *Optimizer) isOptimal(sigma *mat.SymDense, w []float64, active []bool, lb, ub []float64) bool {
	const tol = 1e-10
	N := sigma.SymmetricDim()

	for i := 0; i < N; i++ {
		g := 0.0
		for j := 0; j < N; j++ {
			g += sigma.At(i, j) * w[j]
		}
		if active[i] && w[i] <= lb[i]+tol && g < -tol {
			return false
		}
		if active[i] && w[i] >= ub[i]-tol && g > tol {
			return false
		}
	}
	return true
}

func (o *Optimizer) releaseConstraint(sigma *mat.SymDense, w []float64,
	active *[]bool, isActive []bool, lb, ub []float64,
) {
	const tol = 1e-10
	N := sigma.SymmetricDim()

	worstIdx := -1
	worstViolation := 0.0

	for i := 0; i < N; i++ {
		if !isActive[i] {
			continue
		}
		g := 0.0
		for j := 0; j < N; j++ {
			g += sigma.At(i, j) * w[j]
		}
		if w[i] <= lb[i]+tol && g < -worstViolation {
			worstViolation = -g
			worstIdx = i
		}
		if w[i] >= ub[i]-tol && g > worstViolation {
			worstViolation = g
			worstIdx = i
		}
	}

	if worstIdx >= 0 {
		(*active)[worstIdx] = false
	}
}

func (o *Optimizer) gradientProjection(sigma *mat.SymDense, w, lb, ub []float64) []float64 {
	N := len(w)
	grad := make([]float64, N)
	for i := 0; i < N; i++ {
		for j := 0; j < N; j++ {
			grad[i] += sigma.At(i, j) * w[j]
		}
	}

	wNew := make([]float64, N)
	for i := 0; i < N; i++ {
		wNew[i] = w[i] - 0.5*grad[i]
		if wNew[i] < lb[i] {
			wNew[i] = lb[i]
		}
		if wNew[i] > ub[i] {
			wNew[i] = ub[i]
		}
	}

	sum := 0.0
	for _, wi := range wNew {
		sum += wi
	}
	if sum > 1e-15 {
		for i := range wNew {
			wNew[i] /= sum
		}
	} else {
		for i := range wNew {
			wNew[i] = 1.0 / float64(N)
		}
	}
	return wNew
}

// GetEfficientFrontier computes the mean-variance efficient frontier (20 points).
func (o *Optimizer) GetEfficientFrontier() []struct{ Return, Risk float64 } {
	o.mu.RLock()
	hp := o.history
	wMax := o.constraints.MaxPositionPct
	o.mu.RUnlock()

	if hp == nil {
		return nil
	}

	symbols := []string{
		"2330.TW", "2317.TW", "2454.TW", "2308.TW", "2881.TW",
		"2882.TW", "1301.TW", "1303.TW", "2412.TW", "2002.TW",
	}
	rm := o.extractReturnMatrix(symbols)
	if rm == nil || len(rm.assets) < 2 {
		return nil
	}

	sample := o.sampleCov(rm)
	if sample == nil {
		return nil
	}
	sigma := o.ledoitWolfShrink(rm, sample)

	N := len(rm.assets)
	lb := make([]float64, N)
	ub := make([]float64, N)
	for i := 0; i < N; i++ {
		ub[i] = wMax
	}

	minRet := rm.means[0]
	maxRet := rm.means[0]
	for _, m := range rm.means {
		if m < minRet {
			minRet = m
		}
		if m > maxRet {
			maxRet = m
		}
	}

	daysPerYear := 252.0
	const numPoints = 20

	Aeq := mat.NewDense(2, N, nil)
	for j := 0; j < N; j++ {
		Aeq.Set(0, j, 1.0)
	}

	frontier := make([]struct{ Return, Risk float64 }, numPoints)
	for k := 0; k < numPoints; k++ {
		frac := float64(k) / float64(numPoints-1)
		rTarget := minRet + frac*(maxRet-minRet)
		for j := 0; j < N; j++ {
			Aeq.Set(1, j, rm.means[j])
		}
		beq := []float64{1.0, rTarget}

		wInit := make([]float64, N)
		for i := 0; i < N; i++ {
			wInit[i] = 1.0 / float64(N)
		}

		wOpt := o.activeSetQP(sigma, Aeq, beq, lb, ub, wInit)

		var portVar float64
		for i := 0; i < N; i++ {
			for j := 0; j < N; j++ {
				portVar += wOpt[i] * sigma.At(i, j) * wOpt[j]
			}
		}
		portRisk := math.Sqrt(portVar) * math.Sqrt(daysPerYear)
		annRet := rTarget * daysPerYear

		frontier[k] = struct{ Return, Risk float64 }{Return: annRet, Risk: portRisk}
	}

	return frontier
}

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
