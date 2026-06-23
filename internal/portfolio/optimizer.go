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
