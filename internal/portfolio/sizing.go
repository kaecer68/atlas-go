package portfolio

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

// RiskParameters 风险参数
type RiskParameters struct {
	KellyFraction      float64 // Kelly 系数 (0.1-0.5)
	VolLookback        int     // 波动率回望窗口
	MaxPositionByADV   float64 // 日成交量限制 (0.01 = 1%)
	MaxDrawdownLimit   float64 // 最大回撤限制
	ATRMultiplier      float64 // ATR 乘数
	CorrelationPenalty float64 // 相关性惩罚系数
}

// DefaultRiskParameters 默认风险参数
func DefaultRiskParameters() RiskParameters {
	return RiskParameters{
		KellyFraction:      0.5, // half-Kelly per Thorp (2006)
		VolLookback:        20,
		MaxPositionByADV:   0.01,
		MaxDrawdownLimit:   0.10,
		ATRMultiplier:      2.0,
		CorrelationPenalty: 0.5,
	}
}

// Signal 交易信号
type Signal struct {
	Symbol      string
	Side        domain.Side
	Conviction  float64 // 置信度 0-1
	WinRate     float64 // 历史胜率
	PayoffRatio float64 // 盈亏比
}

// Sizer 仓位规模管理器
type Sizer struct {
	params               RiskParameters
	runtimeParams        *RuntimeParameters
	volatilities         map[string]float64            // 股票波动率缓存
	correlations         map[string]map[string]float64 // 相关性矩阵
	advCache             map[string]float64            // 日均成交量缓存
	atrCache             map[string]float64            // ATR缓存
	correlationThreshold float64                       // 动态相关性阈值
	mu                   sync.RWMutex
}

// NewSizer 创建仓位管理器
func NewSizer() *Sizer {
	return &Sizer{
		params:               DefaultRiskParameters(),
		runtimeParams:        DefaultRuntimeParameters(),
		volatilities:         make(map[string]float64),
		correlations:         make(map[string]map[string]float64),
		advCache:             make(map[string]float64),
		atrCache:             make(map[string]float64),
		correlationThreshold: DefaultRuntimeParameters().Sizing.CorrelationThreshold,
	}
}

// NewSizerFromConfig 從 ParametersConfig 構造 Sizer，將可用的 Sizing 參數
// 同步進 RiskParameters，解決 SetRiskParameters 從未被 production code 呼叫的
// dead-code 問題。VolLookback 因 ParametersConfig 未提供，保留預設值。
func NewSizerFromConfig(cfg *config.ParametersConfig) *Sizer {
	s := NewSizer()
	if cfg == nil {
		return s
	}
	rp := RiskParameters{
		KellyFraction:      cfg.Sizing.KellyFraction.Value,
		MaxPositionByADV:   cfg.Sizing.MaxPositionByADV.Value,
		MaxDrawdownLimit:   cfg.Sizing.MaxDrawdownLimit.Value,
		ATRMultiplier:      cfg.Sizing.ATRMultiplier.Value,
		CorrelationPenalty: cfg.Sizing.CorrelationPenalty.Value,
	}
	s.SetRiskParameters(rp)
	return s
}

// SetRiskParameters 设置风险参数
func (s *Sizer) SetRiskParameters(params RiskParameters) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.params = params
}

// WithParameters 设置运行时参数（链式调用）
func (s *Sizer) WithParameters(p *RuntimeParameters) *Sizer {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runtimeParams = p
	return s
}

// SetCorrelationThreshold 设置动态相关性阈值
func (s *Sizer) SetCorrelationThreshold(threshold float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.correlationThreshold = threshold
}

// CalculateSize 计算建议仓位规模
func (s *Sizer) CalculateSize(
	signal Signal,
	currentPortfolio PortfolioSnapshot,
	quote domain.Quote,
) (int, float64, error) {
	s.mu.RLock()
	params := s.params
	s.mu.RUnlock()

	if quote.Last == 0 {
		return 0, 0, fmt.Errorf("invalid price for %s", signal.Symbol)
	}

	// 1. 基于 Kelly Criterion 计算基础仓位
	kellySize := s.calculateKellySize(signal, currentPortfolio)

	// 2. 基于波动率调整
	vol := s.getVolatility(signal.Symbol)
	volAdjustedSize := s.adjustForVolatility(kellySize, vol)

	// 3. 基于 ATR 调整 (ATR 止损法)
	atr := s.getATR(signal.Symbol)
	atrAdjustedSize := s.adjustForATR(volAdjustedSize, atr, quote.Last, params.ATRMultiplier)

	// 4. 应用流动性限制
	adv := s.getADV(signal.Symbol)
	liquidityLimitedSize := s.applyLiquidityLimit(atrAdjustedSize, adv, quote.Last, params.MaxPositionByADV)

	// 5. 应用相关性惩罚
	corrPenalty := s.calculateCorrelationPenalty(signal.Symbol, currentPortfolio)
	finalSize := liquidityLimitedSize * (1 - corrPenalty*params.CorrelationPenalty)

	// 6. 转换为股数
	shares := int(finalSize / quote.Last)

	// 计算风险金额
	stopLoss := atr * params.ATRMultiplier
	riskAmount := float64(shares) * stopLoss

	return shares, riskAmount, nil
}

// calculateKellySize 计算 Kelly 最优规模
func (s *Sizer) calculateKellySize(signal Signal, portfolio PortfolioSnapshot) float64 {
	s.mu.RLock()
	runtimeParams := s.runtimeParams
	s.mu.RUnlock()

	// Kelly 公式: f* = (p*b - q) / b
	// p = 胜率, q = 败率 = 1-p, b = 盈亏比
	p := signal.WinRate
	if p == 0 {
		p = runtimeParams.Sizing.DefaultWinRate
	}
	q := 1 - p
	b := signal.PayoffRatio
	if b == 0 {
		b = runtimeParams.Sizing.DefaultPayoffRatio
	}

	kelly := (p*b - q) / b
	if kelly <= 0 {
		return 0
	}

	s.mu.RLock()
	params := s.params
	runtimeParams = s.runtimeParams
	s.mu.RUnlock()

	// 应用分数 Kelly (保守)
	fractionalKelly := kelly * params.KellyFraction

	// 限制在组合资金的合理范围内
	maxPosition := portfolio.TotalValue * runtimeParams.Optimizer.MaxPositionPct
	size := portfolio.TotalValue * fractionalKelly

	if size > maxPosition {
		size = maxPosition
	}

	return size
}

// adjustForVolatility 基于波动率调整
func (s *Sizer) adjustForVolatility(baseSize, volatility float64) float64 {
	s.mu.RLock()
	runtimeParams := s.runtimeParams
	s.mu.RUnlock()

	if volatility == 0 {
		return baseSize
	}

	// 波动率越高，仓位越小
	targetVol := runtimeParams.Sizing.TargetVolatility
	adjustment := targetVol / volatility

	// 限制调整范围
	if adjustment < runtimeParams.Sizing.VolAdjustmentMin {
		adjustment = runtimeParams.Sizing.VolAdjustmentMin
	}
	if adjustment > runtimeParams.Sizing.VolAdjustmentMax {
		adjustment = runtimeParams.Sizing.VolAdjustmentMax
	}

	return baseSize * adjustment
}

// adjustForATR 基于 ATR 调整仓位 (ATR 止损法)
func (s *Sizer) adjustForATR(baseSize, atr, price, multiplier float64) float64 {
	s.mu.RLock()
	runtimeParams := s.runtimeParams
	s.mu.RUnlock()

	if atr == 0 || price == 0 {
		return baseSize
	}

	atrPct := (atr * multiplier) / price
	if atrPct == 0 {
		return baseSize
	}

	// ATR 越大，仓位越小
	adjustment := runtimeParams.Sizing.ATRTargetRisk / atrPct
	if adjustment > runtimeParams.Sizing.ATRAdjustmentMax {
		adjustment = runtimeParams.Sizing.ATRAdjustmentMax
	}
	if adjustment < runtimeParams.Sizing.ATRAdjustmentMin {
		adjustment = runtimeParams.Sizing.ATRAdjustmentMin
	}

	return baseSize * adjustment
}

// applyLiquidityLimit 应用流动性限制
// adv 為日均成交股數，price 為當前報價，maxPct 為單一持倉占 ADV 的上限比例
// (例如 0.01 代表 1% 的日成交量)。maxByLiquidity = adv * price * maxPct 為
// 流動性允許的最大持倉金額(原為簡化計算直接乘 100，已修正)。
func (s *Sizer) applyLiquidityLimit(size, adv, price, maxPct float64) float64 {
	if adv == 0 || price <= 0 {
		return size
	}

	maxByLiquidity := adv * price * maxPct
	if size > maxByLiquidity {
		return maxByLiquidity
	}

	return size
}

// calculateCorrelationPenalty 计算相关性惩罚
func (s *Sizer) calculateCorrelationPenalty(symbol string, portfolio PortfolioSnapshot) float64 {
	s.mu.RLock()
	correlations := s.correlations[symbol]
	runtimeParams := s.runtimeParams
	s.mu.RUnlock()

	if len(correlations) == 0 || len(portfolio.Positions) == 0 {
		return 0
	}

	var totalCorr float64
	var count int
	var highCorrCount int

	for _, pos := range portfolio.Positions {
		if corr, ok := correlations[pos.Symbol]; ok {
			absCorr := math.Abs(corr)
			totalCorr += absCorr
			count++
			if absCorr > runtimeParams.Sizing.CorrelationThreshold {
				highCorrCount++
			}
		}
	}

	if count == 0 {
		return 0
	}

	avgCorr := totalCorr / float64(count)
	penalty := avgCorr * runtimeParams.Sizing.CorrelationPenaltyFactor
	if penalty > runtimeParams.Sizing.MaxCorrelationPenalty {
		penalty = runtimeParams.Sizing.MaxCorrelationPenalty
	}

	return penalty
}

// PortfolioSnapshot 组合快照
type PortfolioSnapshot struct {
	TotalValue float64
	Cash       float64
	Positions  []domain.Position
	Timestamp  time.Time
}

// UpdateVolatility 更新波动率数据
func (s *Sizer) UpdateVolatility(symbol string, volatility float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.volatilities[symbol] = volatility
}

// UpdateCorrelation 更新相关性数据
func (s *Sizer) UpdateCorrelation(symbol1, symbol2 string, correlation float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.correlations[symbol1] == nil {
		s.correlations[symbol1] = make(map[string]float64)
	}
	s.correlations[symbol1][symbol2] = correlation

	if s.correlations[symbol2] == nil {
		s.correlations[symbol2] = make(map[string]float64)
	}
	s.correlations[symbol2][symbol1] = correlation
}

// UpdateADV 更新日均成交量
func (s *Sizer) UpdateADV(symbol string, adv float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.advCache[symbol] = adv
}

// UpdateATR 更新 ATR
func (s *Sizer) UpdateATR(symbol string, atr float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.atrCache[symbol] = atr
}

// getVolatility 获取波动率
func (s *Sizer) getVolatility(symbol string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.volatilities[symbol]; ok {
		return v
	}
	return s.runtimeParams.Sizing.DefaultVolatility
}

// getADV 获取日均成交量
func (s *Sizer) getADV(symbol string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.advCache[symbol]; ok {
		return v
	}
	return s.runtimeParams.Sizing.DefaultADV
}

// getATR 获取 ATR
func (s *Sizer) getATR(symbol string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if v, ok := s.atrCache[symbol]; ok {
		return v
	}
	return 0 // 未缓存时返回 0，使用其他方法
}

// PositionSizingResult 仓位计算结果
type PositionSizingResult struct {
	Symbol        string
	Shares        int
	TargetValue   float64
	RiskAmount    float64
	StopLossPrice float64
	Reason        string
}

// CalculatePositionSizing 批量计算仓位规模
func (s *Sizer) CalculatePositionSizing(
	signals []Signal,
	portfolio PortfolioSnapshot,
	quotes map[string]domain.Quote,
) []PositionSizingResult {
	var results []PositionSizingResult

	for _, signal := range signals {
		quote, ok := quotes[signal.Symbol]
		if !ok {
			continue
		}

		shares, riskAmount, err := s.CalculateSize(signal, portfolio, quote)
		if err != nil {
			continue
		}

		if shares == 0 {
			continue
		}

		stopLoss := s.getATR(signal.Symbol) * s.params.ATRMultiplier
		stopLossPrice := quote.Last - stopLoss
		if signal.Side == domain.SideSell {
			stopLossPrice = quote.Last + stopLoss
		}

		results = append(results, PositionSizingResult{
			Symbol:        signal.Symbol,
			Shares:        shares,
			TargetValue:   float64(shares) * quote.Last,
			RiskAmount:    riskAmount,
			StopLossPrice: stopLossPrice,
			Reason:        "kelly|vol|atr|liquidity|corr",
		})
	}

	return results
}
