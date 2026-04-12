package portfolio

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// FactorType 因子类型
type FactorType string

const (
	FactorMomentum FactorType = "momentum"
	FactorValue    FactorType = "value"
	FactorQuality  FactorType = "quality"
	FactorAgent    FactorType = "agent"
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

// DefaultConstraints 默认约束
func DefaultConstraints() Constraints {
	return Constraints{
		MaxPositionPct:   0.15,
		MaxSectorPct:     0.40,
		MaxTurnoverDaily: 0.20,
		TargetBeta:       1.0,
		BetaRange:        [2]float64{0.8, 1.2},
		MinTradeSize:     1,
		CashReserve:      0.05,
	}
}

// Optimizer 组合优化器
type Optimizer struct {
	constraints    Constraints
	agentWeights   map[string]float64
	styleWeights   map[string]float64
	factorWeights  map[FactorType]float64
	history        *HistoricalPrices
	fundamentals   *FundamentalProvider
	mu             sync.RWMutex
}

// NewOptimizer 创建优化器
func NewOptimizer() *Optimizer {
	return &Optimizer{
		constraints:   DefaultConstraints(),
		agentWeights:  make(map[string]float64),
		styleWeights:  make(map[string]float64),
		factorWeights: defaultFactorWeights(),
	}
}

// defaultFactorWeights 默认因子权重
func defaultFactorWeights() map[FactorType]float64 {
	return map[FactorType]float64{
		FactorMomentum: 0.30,
		FactorValue:    0.25,
		FactorQuality:  0.25,
		FactorAgent:    0.20,
	}
}

// SetConstraints 设置约束
func (o *Optimizer) SetConstraints(c Constraints) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.constraints = c
}

// WithHistoricalPrices attaches a historical price repository for momentum calc.
func (o *Optimizer) WithHistoricalPrices(hp *HistoricalPrices) *Optimizer {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.history = hp
	return o
}

// WithFundamentalProvider attaches a fundamental data provider.
func (o *Optimizer) WithFundamentalProvider(fp *FundamentalProvider) *Optimizer {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.fundamentals = fp
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
	o.mu.RUnlock()

	// 1. 聚合相同股票的推荐
	aggregated := o.aggregateRecommendations(recommendations)

	// 2. 计算多因子评分
	scores := o.calculateMultiFactorScores(aggregated, quotes, factorWeights)

	// 3. 初始权重分配 (基于评分)
	weights := o.allocateInitialWeights(scores, totalCapital)

	// 4. 应用约束调整
	weights = o.applyConstraints(weights, constraints, totalCapital)

	// 5. 生成最终仓位
	positions := o.buildPositions(weights, scores, aggregated, quotes, totalCapital)

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
	Symbol   string
	Side     domain.Side
	Momentum float64
	Value    float64
	Quality  float64
	Agent    float64
	Total    float64
	Agents   []string
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

		// 其他因子 (简化版，实际需要历史数据)
		momentumScore := o.calculateMomentumScore(symbol, quotes)
		valueScore := o.calculateValueScore(symbol, quotes)
		qualityScore := o.calculateQualityScore(symbol, quotes)

		// 综合评分
		totalScore := momentumScore*factorWeights[FactorMomentum] +
			valueScore*factorWeights[FactorValue] +
			qualityScore*factorWeights[FactorQuality] +
			agentScore*factorWeights[FactorAgent]

		scores[key] = &symbolScore{
			Symbol:   symbol,
			Side:     side,
			Momentum: momentumScore,
			Value:    valueScore,
			Quality:  qualityScore,
			Agent:    agentScore,
			Total:    totalScore,
			Agents:   agents,
		}
	}

	return scores
}

// calculateMomentumScore computes momentum based on 20-day price change.
// Falls back to intraday return when no historical data is available.
func (o *Optimizer) calculateMomentumScore(symbol string, quotes map[string]domain.Quote) float64 {
	o.mu.RLock()
	hp := o.history
	o.mu.RUnlock()

	if hp != nil {
		ret20 := hp.MomentumReturn(symbol, 20)
		if ret20 != 0 {
			// Normalize: assume ±30% over 20 days maps to ±1.0
			score := ret20 / 0.30
			if score > 1.0 {
				score = 1.0
			}
			if score < -1.0 {
				score = -1.0
			}
			return score
		}
	}

	// Fallback to intraday momentum proxy
	quote, ok := quotes[symbol]
	if !ok || quote.Open == 0 {
		return 0.0
	}
	intradayReturn := (quote.Last - quote.Open) / quote.Open
	score := intradayReturn / 0.10
	if score > 1.0 {
		score = 1.0
	}
	if score < -1.0 {
		score = -1.0
	}
	return score
}

// calculateValueScore computes value based on P/E and P/B from fundamentals.
// Falls back to a mild positive constant when no data is available.
func (o *Optimizer) calculateValueScore(symbol string, quotes map[string]domain.Quote) float64 {
	_ = quotes
	o.mu.RLock()
	fp := o.fundamentals
	o.mu.RUnlock()

	if fp != nil && fp.HasData() {
		data := fp.Get(symbol)
		score := 0.0
		count := 0
		if data.PE > 0 {
			// Lower PE is better. Map PE 5->1.0, PE 50->-1.0 linearly
			peScore := 1.0 - (data.PE-5)/45.0
			if peScore > 1.0 {
				peScore = 1.0
			}
			if peScore < -1.0 {
				peScore = -1.0
			}
			score += peScore
			count++
		}
		if data.PB > 0 {
			pbScore := 1.0 - (data.PB-0.5)/4.5
			if pbScore > 1.0 {
				pbScore = 1.0
			}
			if pbScore < -1.0 {
				pbScore = -1.0
			}
			score += pbScore
			count++
		}
		if count > 0 {
			return score / float64(count)
		}
	}
	return 0.1 // fallback placeholder
}

// calculateQualityScore computes quality based on dividend yield and price stability.
// Falls back to a mild positive constant when no data is available.
func (o *Optimizer) calculateQualityScore(symbol string, quotes map[string]domain.Quote) float64 {
	o.mu.RLock()
	fp := o.fundamentals
	hp := o.history
	o.mu.RUnlock()

	score := 0.0
	count := 0

	if fp != nil && fp.HasData() {
		data := fp.Get(symbol)
		if data.DividendYield > 0 {
			// Higher yield suggests stability. Map 0->0, 5%->1.0
			dyScore := data.DividendYield / 5.0
			if dyScore > 1.0 {
				dyScore = 1.0
			}
			score += dyScore
			count++
		}
	}

	if hp != nil {
		vol := hp.Volatility(symbol, 20)
		if vol > 0 {
			// Lower volatility = higher quality. Map 0->1.0, 5%->0 linearly
			volScore := 1.0 - vol/0.05
			if volScore > 1.0 {
				volScore = 1.0
			}
			if volScore < -1.0 {
				volScore = -1.0
			}
			score += volScore
			count++
		}
	}

	if count > 0 {
		return score / float64(count)
	}
	return 0.05 // fallback placeholder
}

// allocateInitialWeights 初始权重分配
type weightInfo struct {
	Symbol string
	Side   domain.Side
	Weight float64
	Score  float64
}

func (o *Optimizer) allocateInitialWeights(
	scores map[string]*symbolScore,
	totalCapital float64,
) []weightInfo {
	// 归一化评分
	var totalScore float64
	for _, s := range scores {
		totalScore += math.Abs(s.Total)
	}

	if totalScore == 0 {
		return nil
	}

	var weights []weightInfo
	for key, s := range scores {
		weight := math.Abs(s.Total) / totalScore
		weights = append(weights, weightInfo{
			Symbol: s.Symbol,
			Side:   s.Side,
			Weight: weight,
			Score:  s.Total,
		})

		// 更新 scores 中的 key 引用
		_ = key
	}

	// 按权重排序
	sort.Slice(weights, func(i, j int) bool {
		return weights[i].Weight > weights[j].Weight
	})

	return weights
}

// applyConstraints 应用约束
func (o *Optimizer) applyConstraints(
	weights []weightInfo,
	constraints Constraints,
	totalCapital float64,
) []weightInfo {
	if len(weights) == 0 {
		return nil
	}

	// 1. 应用单票最大权重约束
	maxValue := totalCapital * constraints.MaxPositionPct
	for i := range weights {
		value := weights[i].Weight * totalCapital
		if value > maxValue {
			weights[i].Weight = constraints.MaxPositionPct
		}
	}

	// 2. 重新归一化
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

	// 3. 应用现金储备约束
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

	return weights
}

// buildPositions 构建最终仓位
func (o *Optimizer) buildPositions(
	weights []weightInfo,
	scores map[string]*symbolScore,
	aggregated map[string][]domain.Recommendation,
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

		// 计算股数
		shares := int(targetValue / quote.Last)
		if shares == 0 {
			continue
		}

		// 实际目标金额
		actualValue := float64(shares) * quote.Last

		positions = append(positions, OptimizedPosition{
			Symbol:       w.Symbol,
			Side:         w.Side,
			TargetValue:  actualValue,
			TargetWeight: targetWeight,
			Confidence:   score.Total,
			Factors: map[FactorType]float64{
				FactorMomentum: score.Momentum,
				FactorValue:    score.Value,
				FactorQuality:  score.Quality,
				FactorAgent:    score.Agent,
			},
			Agents: score.Agents,
		})
	}

	return positions
}

// GetEfficientFrontier 获取有效前沿 (简化版)
func (o *Optimizer) GetEfficientFrontier() []struct{ Return, Risk float64 } {
	// 实际实现需要协方差矩阵和优化算法
	// 这里返回占位数据
	return []struct{ Return, Risk float64 }{
		{0.05, 0.10},
		{0.08, 0.12},
		{0.10, 0.15},
		{0.12, 0.20},
	}
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
