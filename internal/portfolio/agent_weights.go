package portfolio

import (
	"sort"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// AgentPerformance Agent 表现数据
type AgentPerformance struct {
	AgentID      string
	TotalSignals int
	WinCount     int
	LossCount    int
	WinRate      float64
	AvgReturn    float64
	SharpeLike   float64
	LastUpdated  time.Time
}

// AgentWeightManager Agent 权重管理器
type AgentWeightManager struct {
	weights        map[string]float64
	performances   map[string]*AgentPerformance
	lookbackWindow int     // 回望窗口天数
	minWeight      float64 // 最小权重
	maxWeight      float64 // 最大权重
	smoothingAlpha float64 // EMA 平滑系数
	defaultWeight  float64 // 默认权重
	mu             sync.RWMutex
}

// NewAgentWeightManager 创建权重管理器
func NewAgentWeightManager() *AgentWeightManager {
	return &AgentWeightManager{
		weights:        make(map[string]float64),
		performances:   make(map[string]*AgentPerformance),
		lookbackWindow: 20,
		minWeight:      0.05,
		maxWeight:      0.40,
		smoothingAlpha: 0.3,
		defaultWeight:  0.10,
	}
}

// SetParameters 设置参数
func (m *AgentWeightManager) SetParameters(
	lookbackWindow int,
	minWeight, maxWeight, smoothingAlpha float64,
) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lookbackWindow = lookbackWindow
	m.minWeight = minWeight
	m.maxWeight = maxWeight
	m.smoothingAlpha = smoothingAlpha
}

// RecordSignal 记录 Agent 信号结果
func (m *AgentWeightManager) RecordSignal(
	agentID string,
	isWin bool,
	returnPct float64,
) {
	m.mu.Lock()
	defer m.mu.Unlock()

	perf, exists := m.performances[agentID]
	if !exists {
		perf = &AgentPerformance{
			AgentID: agentID,
		}
		m.performances[agentID] = perf
	}

	perf.TotalSignals++
	if isWin {
		perf.WinCount++
	} else {
		perf.LossCount++
	}

	// 更新胜率
	if perf.TotalSignals > 0 {
		perf.WinRate = float64(perf.WinCount) / float64(perf.TotalSignals)
	}

	// 更新平均收益 (简化 EMA)
	if perf.TotalSignals == 1 {
		perf.AvgReturn = returnPct
	} else {
		perf.AvgReturn = m.smoothingAlpha*returnPct + (1-m.smoothingAlpha)*perf.AvgReturn
	}

	// 更新 Sharpe-like (简化版)
	if perf.TotalSignals >= 10 {
		perf.SharpeLike = perf.AvgReturn * perf.WinRate
	}

	perf.LastUpdated = time.Now()
}

// UpdateWeights 更新所有权重
func (m *AgentWeightManager) UpdateWeights() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.performances) == 0 {
		return
	}

	// 计算原始分数
	rawScores := make(map[string]float64)
	var totalScore float64

	for agentID, perf := range m.performances {
		// 基于胜率、平均收益计算分数
		score := m.calculateScore(perf)
		rawScores[agentID] = score
		totalScore += score
	}

	if totalScore == 0 {
		// 平均分配
		avgWeight := 1.0 / float64(len(m.performances))
		for agentID := range m.performances {
			m.weights[agentID] = m.constrainWeight(avgWeight)
		}
		return
	}

	// 归一化并应用约束
	for agentID, score := range rawScores {
		weight := score / totalScore
		m.weights[agentID] = m.constrainWeight(weight)
	}

	// 重新归一化 (因为约束可能改变了总和)
	m.renormalizeWeights()
}

// calculateScore 计算 Agent 分数
func (m *AgentWeightManager) calculateScore(perf *AgentPerformance) float64 {
	if perf.TotalSignals < 5 {
		// 信号太少，给予默认分数
		return m.defaultWeight
	}

	// 综合评分：胜率 * 平均收益 * 夏普-like
	score := perf.WinRate * perf.AvgReturn * (1 + perf.SharpeLike)

	// 信号数量惩罚 (太少信号的 Agent 降低权重)
	if perf.TotalSignals < m.lookbackWindow {
		penalty := float64(perf.TotalSignals) / float64(m.lookbackWindow)
		score *= penalty
	}

	if score < 0 {
		return m.defaultWeight * 0.5 // 表现差的给予低权重而非零
	}

	return score
}

// constrainWeight 应用权重约束
func (m *AgentWeightManager) constrainWeight(weight float64) float64 {
	if weight < m.minWeight {
		return m.minWeight
	}
	if weight > m.maxWeight {
		return m.maxWeight
	}
	return weight
}

// renormalizeWeights 重新归一化权重
func (m *AgentWeightManager) renormalizeWeights() {
	var total float64
	for _, w := range m.weights {
		total += w
	}

	if total == 0 {
		return
	}

	scale := 1.0 / total
	for agentID, w := range m.weights {
		m.weights[agentID] = w * scale
	}
}

// GetWeight 获取指定 Agent 的权重
func (m *AgentWeightManager) GetWeight(agentID string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if weight, ok := m.weights[agentID]; ok {
		return weight
	}

	return m.defaultWeight
}

// GetAllWeights 获取所有权重
func (m *AgentWeightManager) GetAllWeights() map[string]float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]float64)
	for k, v := range m.weights {
		result[k] = v
	}

	// 补充未初始化的 Agent
	for agentID := range m.performances {
		if _, ok := result[agentID]; !ok {
			result[agentID] = m.defaultWeight
		}
	}

	return result
}

// GetPerformance 获取 Agent 表现数据
func (m *AgentWeightManager) GetPerformance(agentID string) (*AgentPerformance, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	perf, ok := m.performances[agentID]
	return perf, ok
}

// GetAllPerformances 获取所有 Agent 表现
func (m *AgentWeightManager) GetAllPerformances() map[string]*AgentPerformance {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*AgentPerformance)
	for k, v := range m.performances {
		copy := *v
		result[k] = &copy
	}

	return result
}

// RankAgents 按表现排序 Agents
func (m *AgentWeightManager) RankAgents() []AgentRanking {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var rankings []AgentRanking

	for agentID, perf := range m.performances {
		weight := m.defaultWeight
		if w, ok := m.weights[agentID]; ok {
			weight = w
		}

		rankings = append(rankings, AgentRanking{
			AgentID:    agentID,
			Weight:     weight,
			WinRate:    perf.WinRate,
			AvgReturn:  perf.AvgReturn,
			SharpeLike: perf.SharpeLike,
			Score:      m.calculateScore(perf),
		})
	}

	// 按分数排序
	sort.Slice(rankings, func(i, j int) bool {
		return rankings[i].Score > rankings[j].Score
	})

	return rankings
}

// AgentRanking Agent 排名
type AgentRanking struct {
	AgentID    string
	Weight     float64
	WinRate    float64
	AvgReturn  float64
	SharpeLike float64
	Score      float64
}

// Reset 重置所有数据
func (m *AgentWeightManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.weights = make(map[string]float64)
	m.performances = make(map[string]*AgentPerformance)
}

// PruneInactiveAgents 清理不活跃的 Agent
func (m *AgentWeightManager) PruneInactiveAgents(days int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().AddDate(0, 0, -days)

	for agentID, perf := range m.performances {
		if perf.LastUpdated.Before(cutoff) {
			delete(m.performances, agentID)
			delete(m.weights, agentID)
		}
	}
}

// BatchUpdateFromBacktest 从回测结果批量更新
func (m *AgentWeightManager) BatchUpdateFromBacktest(results map[string]AgentPerformance) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for agentID, perf := range results {
		if existing, ok := m.performances[agentID]; ok {
			// 合并数据
			existing.TotalSignals += perf.TotalSignals
			existing.WinCount += perf.WinCount
			existing.LossCount += perf.LossCount
			if existing.TotalSignals > 0 {
				existing.WinRate = float64(existing.WinCount) / float64(existing.TotalSignals)
			}
			// 使用新的平均收益
			existing.AvgReturn = perf.AvgReturn
			existing.SharpeLike = perf.SharpeLike
			existing.LastUpdated = time.Now()
		} else {
			// 新增
			copy := perf
			copy.LastUpdated = time.Now()
			m.performances[agentID] = &copy
		}
	}

	// 触发权重更新
	m.UpdateWeights()
}

// ExportWeights 导出权重 (用于持久化)
func (m *AgentWeightManager) ExportWeights() map[string]float64 {
	return m.GetAllWeights()
}

// ImportWeights 导入权重
func (m *AgentWeightManager) ImportWeights(weights map[string]float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for agentID, weight := range weights {
		m.weights[agentID] = m.constrainWeight(weight)
	}

	m.renormalizeWeights()
}

// WeightedRecommendation 带权重的推荐
type WeightedRecommendation struct {
	domain.Recommendation
	AgentWeight float64
	FinalScore  float64
}

// ApplyWeightsToRecommendations 将权重应用到推荐
func (m *AgentWeightManager) ApplyWeightsToRecommendations(
	recommendations []domain.Recommendation,
) []WeightedRecommendation {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var weighted []WeightedRecommendation

	for _, rec := range recommendations {
		weight := m.defaultWeight
		if w, ok := m.weights[rec.Agent]; ok {
			weight = w
		}

		finalScore := float64(rec.Conviction) * weight / 100.0

		weighted = append(weighted, WeightedRecommendation{
			Recommendation: rec,
			AgentWeight:    weight,
			FinalScore:     finalScore,
		})
	}

	return weighted
}

// CalculatePortfolioAgentContribution 计算 Agent 对组合的贡献
func (m *AgentWeightManager) CalculatePortfolioAgentContribution(
	positions []domain.Position,
	agentRecommendations map[string][]string, // agentID -> []symbols
) map[string]float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	contributions := make(map[string]float64)
	totalValue := 0.0

	for _, pos := range positions {
		totalValue += pos.MarketValue
	}

	if totalValue == 0 {
		return contributions
	}

	// 计算每个 Agent 贡献的仓位价值
	for agentID, symbols := range agentRecommendations {
		agentValue := 0.0
		for _, pos := range positions {
			for _, sym := range symbols {
				if pos.Symbol == sym {
					agentValue += pos.MarketValue
					break
				}
			}
		}

		// 结合权重和实际贡献
		weight := m.defaultWeight
		if w, ok := m.weights[agentID]; ok {
			weight = w
		}

		contributions[agentID] = (agentValue / totalValue) * weight
	}

	return contributions
}
