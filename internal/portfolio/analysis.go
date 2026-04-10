package portfolio

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// TradeRecord 交易记录
type TradeRecord struct {
	Symbol     string
	Side       domain.Side
	EntryPrice float64
	ExitPrice  float64
	Quantity   int
	EntryTime  time.Time
	ExitTime   time.Time
	Pnl        float64
	PnlPct     float64
	Agent      string
	Strategy   string
	Slippage   float64
	Commission float64
}

// IsWin 是否盈利
func (t TradeRecord) IsWin() bool {
	return t.Pnl > 0
}

// HoldingPeriod 持仓周期
func (t TradeRecord) HoldingPeriod() time.Duration {
	return t.ExitTime.Sub(t.EntryTime)
}

// AttributionResult 归因结果
type AttributionResult struct {
	AgentContributions  map[string]float64
	StyleContributions  map[Style]float64
	SymbolContributions map[string]float64
	TotalPnL            float64
}

// PostTradeAnalyzer 盘后分析器
type PostTradeAnalyzer struct {
	trades []TradeRecord
}

// NewPostTradeAnalyzer 创建分析器
func NewPostTradeAnalyzer() *PostTradeAnalyzer {
	return &PostTradeAnalyzer{
		trades: make([]TradeRecord, 0),
	}
}

// AddTrade 添加交易记录
func (a *PostTradeAnalyzer) AddTrade(trade TradeRecord) {
	a.trades = append(a.trades, trade)
}

// GetTrades 获取所有交易
func (a *PostTradeAnalyzer) GetTrades() []TradeRecord {
	return a.trades
}

// PerformanceMetrics 业绩指标
type PerformanceMetrics struct {
	TotalTrades      int
	WinningTrades    int
	LosingTrades     int
	WinRate          float64
	AvgWin           float64
	AvgLoss          float64
	ProfitFactor     float64
	TotalPnL         float64
	MaxDrawdown      float64
	SharpeRatio      float64
	AvgHoldingPeriod time.Duration
}

// CalculateMetrics 计算业绩指标
func (a *PostTradeAnalyzer) CalculateMetrics() PerformanceMetrics {
	if len(a.trades) == 0 {
		return PerformanceMetrics{}
	}

	var wins, losses float64
	var winCount, lossCount int
	var maxDrawdown, peak, currentPnL float64
	var totalHolding time.Duration

	for _, trade := range a.trades {
		currentPnL += trade.Pnl

		if trade.Pnl > 0 {
			wins += trade.Pnl
			winCount++
		} else {
			losses += math.Abs(trade.Pnl)
			lossCount++
		}

		// 计算最大回撤
		if currentPnL > peak {
			peak = currentPnL
		}
		drawdown := peak - currentPnL
		if drawdown > maxDrawdown {
			maxDrawdown = drawdown
		}

		totalHolding += trade.HoldingPeriod()
	}

	winRate := 0.0
	if len(a.trades) > 0 {
		winRate = float64(winCount) / float64(len(a.trades))
	}

	avgWin := 0.0
	if winCount > 0 {
		avgWin = wins / float64(winCount)
	}

	avgLoss := 0.0
	if lossCount > 0 {
		avgLoss = losses / float64(lossCount)
	}

	profitFactor := 0.0
	if losses > 0 {
		profitFactor = wins / losses
	}

	avgHolding := time.Duration(0)
	if len(a.trades) > 0 {
		avgHolding = totalHolding / time.Duration(len(a.trades))
	}

	return PerformanceMetrics{
		TotalTrades:      len(a.trades),
		WinningTrades:    winCount,
		LosingTrades:     lossCount,
		WinRate:          winRate,
		AvgWin:           avgWin,
		AvgLoss:          avgLoss,
		ProfitFactor:     profitFactor,
		TotalPnL:         currentPnL,
		MaxDrawdown:      maxDrawdown,
		AvgHoldingPeriod: avgHolding,
	}
}

// AttributionByAgent 按 Agent 归因
func (a *PostTradeAnalyzer) AttributionByAgent() map[string]float64 {
	attribution := make(map[string]float64)

	for _, trade := range a.trades {
		attribution[trade.Agent] += trade.Pnl
	}

	return attribution
}

// AttributionBySymbol 按股票归因
func (a *PostTradeAnalyzer) AttributionBySymbol() map[string]float64 {
	attribution := make(map[string]float64)

	for _, trade := range a.trades {
		attribution[trade.Symbol] += trade.Pnl
	}

	return attribution
}

// AgentStats Agent 统计
type AgentStats struct {
	AgentID     string
	TotalTrades int
	WinCount    int
	TotalPnL    float64
	AvgPnL      float64
	WinRate     float64
	SharpeLike  float64
}

// CalculateAgentStats 计算各 Agent 统计
func (a *PostTradeAnalyzer) CalculateAgentStats() []AgentStats {
	stats := make(map[string]*AgentStats)

	for _, trade := range a.trades {
		if _, ok := stats[trade.Agent]; !ok {
			stats[trade.Agent] = &AgentStats{AgentID: trade.Agent}
		}

		s := stats[trade.Agent]
		s.TotalTrades++
		s.TotalPnL += trade.Pnl
		if trade.Pnl > 0 {
			s.WinCount++
		}
	}

	// 计算派生指标
	var result []AgentStats
	for _, s := range stats {
		if s.TotalTrades > 0 {
			s.WinRate = float64(s.WinCount) / float64(s.TotalTrades)
			s.AvgPnL = s.TotalPnL / float64(s.TotalTrades)
			s.SharpeLike = s.AvgPnL * s.WinRate
		}
		result = append(result, *s)
	}

	// 按盈亏排序
	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalPnL > result[j].TotalPnL
	})

	return result
}

// DailyPnL 每日盈亏
type DailyPnL struct {
	Date   string
	PnL    float64
	Trades int
}

// CalculateDailyPnL 计算每日盈亏
func (a *PostTradeAnalyzer) CalculateDailyPnL() []DailyPnL {
	dailyMap := make(map[string]*DailyPnL)

	for _, trade := range a.trades {
		date := trade.ExitTime.Format("2006-01-02")
		if _, ok := dailyMap[date]; !ok {
			dailyMap[date] = &DailyPnL{Date: date}
		}
		dailyMap[date].PnL += trade.Pnl
		dailyMap[date].Trades++
	}

	var result []DailyPnL
	for _, d := range dailyMap {
		result = append(result, *d)
	}

	// 按日期排序
	sort.Slice(result, func(i, j int) bool {
		return result[i].Date < result[j].Date
	})

	return result
}

// ExecutionQuality 执行质量
type ExecutionQuality struct {
	AvgSlippage      float64
	AvgCommission    float64
	FillRate         float64
	AvgExecutionTime time.Duration
}

// CalculateExecutionQuality 计算执行质量
func (a *PostTradeAnalyzer) CalculateExecutionQuality() ExecutionQuality {
	if len(a.trades) == 0 {
		return ExecutionQuality{}
	}

	var totalSlippage, totalCommission float64
	var totalExecutionTime time.Duration

	for _, trade := range a.trades {
		totalSlippage += trade.Slippage
		totalCommission += trade.Commission
		totalExecutionTime += trade.HoldingPeriod()
	}

	n := float64(len(a.trades))

	return ExecutionQuality{
		AvgSlippage:      totalSlippage / n,
		AvgCommission:    totalCommission / n,
		FillRate:         1.0, // 简化处理
		AvgExecutionTime: totalExecutionTime / time.Duration(len(a.trades)),
	}
}

// ImprovementSuggestion 改进建议
type ImprovementSuggestion struct {
	Category    string
	Priority    int // 1-5，5 最高
	Title       string
	Description string
	Metrics     map[string]float64
}

// GenerateSuggestions 生成改进建议
func (a *PostTradeAnalyzer) GenerateSuggestions() []ImprovementSuggestion {
	var suggestions []ImprovementSuggestion
	metrics := a.CalculateMetrics()
	agentStats := a.CalculateAgentStats()

	// 1. 胜率分析
	if metrics.WinRate < 0.4 {
		suggestions = append(suggestions, ImprovementSuggestion{
			Category:    "risk_management",
			Priority:    4,
			Title:       "胜率偏低",
			Description: fmt.Sprintf("当前胜率 %.1f%%，建议检查止损设置和入场时机", metrics.WinRate*100),
			Metrics: map[string]float64{
				"win_rate": metrics.WinRate,
			},
		})
	}

	// 2. 盈亏比分析
	if metrics.ProfitFactor < 1.0 {
		suggestions = append(suggestions, ImprovementSuggestion{
			Category:    "profit_management",
			Priority:    5,
			Title:       "盈亏比失衡",
			Description: fmt.Sprintf("Profit Factor %.2f < 1.0，亏损大于盈利，需调整止盈止损", metrics.ProfitFactor),
			Metrics: map[string]float64{
				"profit_factor": metrics.ProfitFactor,
			},
		})
	}

	// 3. Agent 表现分析
	if len(agentStats) > 0 {
		worst := agentStats[len(agentStats)-1]
		if worst.TotalPnL < 0 && worst.TotalTrades >= 5 {
			suggestions = append(suggestions, ImprovementSuggestion{
				Category:    "agent_optimization",
				Priority:    3,
				Title:       fmt.Sprintf("Agent %s 表现不佳", worst.AgentID),
				Description: fmt.Sprintf("该 Agent 总盈亏 %.0f，胜率 %.1f%%，建议检查策略", worst.TotalPnL, worst.WinRate*100),
				Metrics: map[string]float64{
					"agent_pnl":     worst.TotalPnL,
					"agent_winrate": worst.WinRate,
				},
			})
		}
	}

	// 4. 最大回撤分析
	if metrics.TotalPnL > 0 && metrics.MaxDrawdown/metrics.TotalPnL > 0.3 {
		suggestions = append(suggestions, ImprovementSuggestion{
			Category:    "risk_management",
			Priority:    4,
			Title:       "回撤控制不佳",
			Description: fmt.Sprintf("最大回撤 %.0f 相对于总盈亏 %.0f 过高", metrics.MaxDrawdown, metrics.TotalPnL),
			Metrics: map[string]float64{
				"max_drawdown": metrics.MaxDrawdown,
				"total_pnl":    metrics.TotalPnL,
			},
		})
	}

	// 5. 持仓时间分析
	if metrics.AvgHoldingPeriod > 24*time.Hour && metrics.WinRate < 0.5 {
		suggestions = append(suggestions, ImprovementSuggestion{
			Category:    "timing",
			Priority:    2,
			Title:       "持仓时间过长",
			Description: fmt.Sprintf("平均持仓 %v，建议缩短持仓时间以提高资金周转", metrics.AvgHoldingPeriod),
			Metrics: map[string]float64{
				"avg_holding_hours": metrics.AvgHoldingPeriod.Hours(),
			},
		})
	}

	// 按优先级排序
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Priority > suggestions[j].Priority
	})

	return suggestions
}

// AnalysisReport 分析报告
type AnalysisReport struct {
	Period           string
	GeneratedAt      time.Time
	Metrics          PerformanceMetrics
	AgentStats       []AgentStats
	DailyPnLs        []DailyPnL
	ExecutionQuality ExecutionQuality
	Suggestions      []ImprovementSuggestion
	Attribution      AttributionResult
}

// GenerateReport 生成完整报告
func (a *PostTradeAnalyzer) GenerateReport(period string) AnalysisReport {
	return AnalysisReport{
		Period:           period,
		GeneratedAt:      time.Now(),
		Metrics:          a.CalculateMetrics(),
		AgentStats:       a.CalculateAgentStats(),
		DailyPnLs:        a.CalculateDailyPnL(),
		ExecutionQuality: a.CalculateExecutionQuality(),
		Suggestions:      a.GenerateSuggestions(),
		Attribution: AttributionResult{
			AgentContributions:  a.AttributionByAgent(),
			SymbolContributions: a.AttributionBySymbol(),
			TotalPnL:            a.CalculateMetrics().TotalPnL,
		},
	}
}

// Clear 清空历史数据
func (a *PostTradeAnalyzer) Clear() {
	a.trades = make([]TradeRecord, 0)
}

// FilterByPeriod 按时间段筛选交易
func (a *PostTradeAnalyzer) FilterByPeriod(start, end time.Time) []TradeRecord {
	var filtered []TradeRecord
	for _, trade := range a.trades {
		if trade.ExitTime.After(start) && trade.ExitTime.Before(end) {
			filtered = append(filtered, trade)
		}
	}
	return filtered
}
