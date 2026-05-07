package portfolio

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

const (
	// DarwinianWeightMin minimum agent weight multiplier (0.3 = whisper)
	DarwinianWeightMin = 0.3
	// DarwinianWeightMax maximum agent weight multiplier (2.5 = shout)
	DarwinianWeightMax = 2.5
	// DarwinianNeutralWeight starting neutral weight
	DarwinianNeutralWeight = 1.0
	// TopQuartileMultiplier multiplier for top 25% performers
	TopQuartileMultiplier = 1.05
	// BottomQuartileMultiplier multiplier for bottom 25% performers
	BottomQuartileMultiplier = 0.95
	// DailyAdjustmentCooldown minimum time between adjustments
	DailyAdjustmentCooldown = 20 * time.Hour
)

// DarwinianAgentWeight represents an agent's Darwinian weight with performance tracking
type DarwinianAgentWeight struct {
	AgentID           string    `json:"agent_id"`
	Skill             string    `json:"skill"`
	Layer             string    `json:"layer"`
	Weight            float64   `json:"weight"`             // Current multiplier (0.3 - 2.5)
	RollingSharpe     float64   `json:"rolling_sharpe"`     // 20-day rolling Sharpe
	RollingVolatility float64   `json:"rolling_volatility"` // Rolling volatility
	TotalSignals      int       `json:"total_signals"`
	WinCount          int       `json:"win_count"`
	LossCount         int       `json:"loss_count"`
	HitRate           float64   `json:"hit_rate"`
	AvgReturn         float64   `json:"avg_return"`
	LastAdjustedAt    time.Time `json:"last_adjusted_at"`
	LastUpdatedAt     time.Time `json:"last_updated_at"`
	DailyReturns      []float64 `json:"daily_returns"` // Last 20 days returns for Sharpe calc
}

// DarwinianWeightManager implements Atlas-GIC style Darwinian weight system
type DarwinianWeightManager struct {
	weights      map[string]*DarwinianAgentWeight
	configPath   string
	lookbackDays int
	mu           sync.RWMutex
}

// NewDarwinianWeightManager creates a new Darwinian weight manager
func NewDarwinianWeightManager(configPath string) *DarwinianWeightManager {
	return &DarwinianWeightManager{
		weights:      make(map[string]*DarwinianAgentWeight),
		configPath:   configPath,
		lookbackDays: 20,
	}
}

// InitializeFromRegistry initializes weights from agent registry
func (m *DarwinianWeightManager) InitializeFromRegistry(registry domain.AgentRegistry) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, agent := range registry.Agents {
		if !agent.Enabled {
			continue
		}
		// Only initialize for Sector, Style, and Superinvestor layers
		if agent.Layer != domain.LayerSector && agent.Layer != domain.LayerStyle {
			// Will add LayerSuperinvestor later
			continue
		}

		if _, exists := m.weights[agent.ID]; !exists {
			// Use agent's DarwinianWeight if specified, otherwise use neutral weight
			initialWeight := agent.DarwinianWeight
			if initialWeight <= 0 {
				initialWeight = DarwinianNeutralWeight
			}

			m.weights[agent.ID] = &DarwinianAgentWeight{
				AgentID:        agent.ID,
				Skill:          agent.Skill,
				Layer:          string(agent.Layer),
				Weight:         initialWeight,
				DailyReturns:   make([]float64, 0, m.lookbackDays),
				LastAdjustedAt: time.Now(),
				LastUpdatedAt:  time.Now(),
			}
		}
	}
}

// RecordOutcome records a recommendation outcome for an agent
func (m *DarwinianWeightManager) RecordOutcome(agentID string, forwardReturn float64, isHit bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	w, exists := m.weights[agentID]
	if !exists {
		return
	}

	w.TotalSignals++
	if isHit {
		w.WinCount++
	} else {
		w.LossCount++
	}

	if w.TotalSignals > 0 {
		w.HitRate = float64(w.WinCount) / float64(w.TotalSignals)
	}

	// Update average return with EMA
	alpha := 0.3 // smoothing factor
	if w.TotalSignals == 1 {
		w.AvgReturn = forwardReturn
	} else {
		w.AvgReturn = alpha*forwardReturn + (1-alpha)*w.AvgReturn
	}

	// Add to daily returns for Sharpe calculation
	w.DailyReturns = append(w.DailyReturns, forwardReturn)
	if len(w.DailyReturns) > m.lookbackDays {
		w.DailyReturns = w.DailyReturns[1:]
	}

	m.updateRollingMetrics(w)
	w.LastUpdatedAt = time.Now()
}

// updateRollingMetrics updates rolling Sharpe ratio and volatility for an agent
func (m *DarwinianWeightManager) updateRollingMetrics(w *DarwinianAgentWeight) {
	if len(w.DailyReturns) < 2 {
		w.RollingSharpe = 0
		w.RollingVolatility = 0
		return
	}

	// Take the most recent returns for rolling calculation
	recentReturns := w.DailyReturns
	if len(w.DailyReturns) > m.lookbackDays {
		recentReturns = w.DailyReturns[len(w.DailyReturns)-m.lookbackDays:]
	}

	// Calculate rolling Sharpe
	w.RollingSharpe = m.calculateSharpe(recentReturns)

	// Calculate rolling volatility
	mean := 0.0
	for _, r := range recentReturns {
		mean += r
	}
	mean /= float64(len(recentReturns))

	variance := 0.0
	for _, r := range recentReturns {
		variance += (r - mean) * (r - mean)
	}
	w.RollingVolatility = math.Sqrt(variance / float64(len(recentReturns)))
}

// calculateSharpe calculates Sharpe ratio for a series of returns
func (m *DarwinianWeightManager) calculateSharpe(returns []float64) float64 {
	if len(returns) < 5 {
		return 0.0
	}

	// Calculate mean
	var sum float64
	for _, r := range returns {
		sum += r
	}
	mean := sum / float64(len(returns))

	// Calculate standard deviation
	var variance float64
	for _, r := range returns {
		variance += (r - mean) * (r - mean)
	}
	stdDev := math.Sqrt(variance / float64(len(returns)-1))
	if stdDev == 0 {
		return 0.0
	}

	// Sharpe = mean / stdDev (assuming risk-free rate = 0 for simplicity)
	return (mean / stdDev) * math.Sqrt(252)
}

// PerformDailyAdjustment performs the daily Darwinian weight adjustment
// Enhanced algorithm with performance-based scaling and volatility adjustment
func (m *DarwinianWeightManager) PerformDailyAdjustment() map[string]float64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	adjustments := make(map[string]float64)

	// Check cooldown and collect eligible agents
	now := time.Now()
	eligible := make([]*DarwinianAgentWeight, 0)

	for _, w := range m.weights {
		if now.Sub(w.LastAdjustedAt) >= DailyAdjustmentCooldown {
			eligible = append(eligible, w)
		}
	}

	if len(eligible) < 2 {
		return adjustments
	}

	// Calculate performance metrics for all eligible agents
	for _, w := range eligible {
		m.updateRollingMetrics(w)
	}

	// Sort by rolling Sharpe ratio
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].RollingSharpe > eligible[j].RollingSharpe
	})

	// Enhanced adjustment algorithm
	n := len(eligible)
	topTier := n / 3
	if topTier < 1 {
		topTier = 1
	}
	bottomTier := n / 3
	if bottomTier < 1 {
		bottomTier = 1
	}

	// Top tier: significant increase with performance scaling
	for i := 0; i < topTier; i++ {
		w := eligible[i]
		oldWeight := w.Weight

		// Performance-based scaling: better performance = bigger boost
		performanceBonus := 1.0 + (w.RollingSharpe * 0.1) // Up to 10% extra for high Sharpe
		volatilityPenalty := 1.0
		if w.RollingVolatility > 0.05 { // Penalize high volatility
			volatilityPenalty = 0.95
		}

		multiplier := TopQuartileMultiplier * performanceBonus * volatilityPenalty
		w.Weight = m.constrainWeight(w.AgentID, oldWeight*multiplier)
		w.LastAdjustedAt = now

		adjustments[w.AgentID] = w.Weight - oldWeight
	}

	// Middle tier: maintain or slight adjustment
	for i := topTier; i < n-bottomTier; i++ {
		w := eligible[i]
		oldWeight := w.Weight

		// Slight adjustment based on hit rate
		if w.HitRate > 0.6 {
			w.Weight = m.constrainWeight(w.AgentID, oldWeight*1.02)
		} else if w.HitRate < 0.4 {
			w.Weight = m.constrainWeight(w.AgentID, oldWeight*0.98)
		}

		w.LastAdjustedAt = now
		adjustments[w.AgentID] = w.Weight - oldWeight
	}

	// Bottom tier: decrease with risk consideration
	for i := n - bottomTier; i < n; i++ {
		w := eligible[i]
		oldWeight := w.Weight

		// Risk-based reduction: poor performance + high volatility = bigger cut
		riskMultiplier := 1.0
		if w.RollingVolatility > 0.08 {
			riskMultiplier = 0.9 // Extra 10% cut for high volatility
		}

		multiplier := BottomQuartileMultiplier * riskMultiplier
		w.Weight = m.constrainWeight(w.AgentID, oldWeight*multiplier)
		w.LastAdjustedAt = now

		adjustments[w.AgentID] = w.Weight - oldWeight
	}

	return adjustments
}

// rankBySharpe ranks agents by rolling Sharpe ratio (descending)
func (m *DarwinianWeightManager) rankBySharpe() []*DarwinianAgentWeight {
	agents := make([]*DarwinianAgentWeight, 0, len(m.weights))
	for _, w := range m.weights {
		agents = append(agents, w)
	}

	sort.Slice(agents, func(i, j int) bool {
		return agents[i].RollingSharpe > agents[j].RollingSharpe
	})

	return agents
}

// constrainWeight ensures weight stays within [0.3, 2.5] bounds
func (m *DarwinianWeightManager) constrainWeight(agentID string, weight float64) float64 {
	if weight < DarwinianWeightMin {
		logging.Warn("darwinian_weights", "weight_clamped_min", logging.AgentID(agentID), logging.FFloat64("weight", weight), logging.FFloat64("min", DarwinianWeightMin))
		return DarwinianWeightMin
	}
	if weight > DarwinianWeightMax {
		logging.Warn("darwinian_weights", "weight_clamped_max", logging.AgentID(agentID), logging.FFloat64("weight", weight), logging.FFloat64("max", DarwinianWeightMax))
		return DarwinianWeightMax
	}
	return weight
}

// GetWeight gets the Darwinian weight for an agent
func (m *DarwinianWeightManager) GetWeight(agentID string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if w, ok := m.weights[agentID]; ok {
		return w.Weight
	}
	return DarwinianNeutralWeight
}

// GetAllWeights gets all agent weights
func (m *DarwinianWeightManager) GetAllWeights() map[string]float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]float64)
	for id, w := range m.weights {
		result[id] = w.Weight
	}
	return result
}

// GetAgentWeightData gets full weight data for an agent
func (m *DarwinianWeightManager) GetAgentWeightData(agentID string) (*DarwinianAgentWeight, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	w, ok := m.weights[agentID]
	if !ok {
		return nil, false
	}

	// Return a copy
	cp := *w
	cp.DailyReturns = make([]float64, len(w.DailyReturns))
	copy(cp.DailyReturns, w.DailyReturns)
	return &cp, true
}

// GetAllAgentWeightData gets all agent weight data
func (m *DarwinianWeightManager) GetAllAgentWeightData() []*DarwinianAgentWeight {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*DarwinianAgentWeight, 0, len(m.weights))
	for _, w := range m.weights {
		cp := *w
		cp.DailyReturns = make([]float64, len(w.DailyReturns))
		copy(cp.DailyReturns, w.DailyReturns)
		result = append(result, &cp)
	}

	// Sort by weight descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].Weight > result[j].Weight
	})

	return result
}

// GetAverageWeight gets the average weight across all agents
func (m *DarwinianWeightManager) GetAverageWeight() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.weights) == 0 {
		return DarwinianNeutralWeight
	}

	var sum float64
	for _, w := range m.weights {
		sum += w.Weight
	}
	return sum / float64(len(m.weights))
}

// GetTopPerformers returns top N performers by rolling Sharpe
func (m *DarwinianWeightManager) GetTopPerformers(n int) []*DarwinianAgentWeight {
	ranked := m.rankBySharpe()
	if n > len(ranked) {
		n = len(ranked)
	}

	result := make([]*DarwinianAgentWeight, n)
	for i := 0; i < n; i++ {
		cp := *ranked[i]
		cp.DailyReturns = make([]float64, len(ranked[i].DailyReturns))
		copy(cp.DailyReturns, ranked[i].DailyReturns)
		result[i] = &cp
	}
	return result
}

// GetBottomPerformers returns bottom N performers by rolling Sharpe
func (m *DarwinianWeightManager) GetBottomPerformers(n int) []*DarwinianAgentWeight {
	ranked := m.rankBySharpe()
	if n > len(ranked) {
		n = len(ranked)
	}

	result := make([]*DarwinianAgentWeight, n)
	for i := 0; i < n; i++ {
		idx := len(ranked) - 1 - i
		cp := *ranked[idx]
		cp.DailyReturns = make([]float64, len(ranked[idx].DailyReturns))
		copy(cp.DailyReturns, ranked[idx].DailyReturns)
		result[i] = &cp
	}
	return result
}

// Save persists weights to disk
func (m *DarwinianWeightManager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Ensure directory exists
	dir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	data := struct {
		SavedAt  time.Time                        `json:"saved_at"`
		Weights  map[string]*DarwinianAgentWeight `json:"weights"`
		Lookback int                              `json:"lookback_days"`
	}{
		SavedAt:  time.Now(),
		Weights:  m.weights,
		Lookback: m.lookbackDays,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal weights: %w", err)
	}

	if err := os.WriteFile(m.configPath, jsonData, 0644); err != nil {
		return fmt.Errorf("write weights file: %w", err)
	}

	return nil
}

// Load loads weights from disk
func (m *DarwinianWeightManager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, that's ok
			return nil
		}
		return fmt.Errorf("read weights file: %w", err)
	}

	var saved struct {
		SavedAt  time.Time                        `json:"saved_at"`
		Weights  map[string]*DarwinianAgentWeight `json:"weights"`
		Lookback int                              `json:"lookback_days"`
	}

	if err := json.Unmarshal(data, &saved); err != nil {
		return fmt.Errorf("unmarshal weights: %w", err)
	}

	m.weights = saved.Weights
	m.lookbackDays = saved.Lookback

	return nil
}

// Reset resets all weights to neutral (1.0)
func (m *DarwinianWeightManager) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, w := range m.weights {
		w.Weight = DarwinianNeutralWeight
		w.RollingSharpe = 0
		w.DailyReturns = w.DailyReturns[:0]
		w.LastAdjustedAt = time.Now()
	}
}

// ResetAgent resets a specific agent's weight to neutral
func (m *DarwinianWeightManager) ResetAgent(agentID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	w, exists := m.weights[agentID]
	if !exists {
		return false
	}

	w.Weight = DarwinianNeutralWeight
	w.RollingSharpe = 0
	w.DailyReturns = w.DailyReturns[:0]
	w.LastAdjustedAt = time.Now()
	return true
}

// RemoveAgent removes an agent from tracking
func (m *DarwinianWeightManager) RemoveAgent(agentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.weights, agentID)
}

// ApplyDarwinianWeights applies Darwinian weights to recommendations
// Returns weighted recommendations where conviction is scaled by agent weight
func (m *DarwinianWeightManager) ApplyDarwinianWeights(
	recommendations []domain.Recommendation,
) []domain.Recommendation {
	m.mu.RLock()
	defer m.mu.RUnlock()

	weighted := make([]domain.Recommendation, 0, len(recommendations))

	for _, rec := range recommendations {
		weight := DarwinianNeutralWeight
		if w, ok := m.weights[rec.Agent]; ok {
			weight = w.Weight
		}

		// Scale conviction by weight
		// weight 2.5 = conviction × 2.5 (max 250 if conviction was 100)
		// weight 0.3 = conviction × 0.3 (min 3 if conviction was 10)
		weightedConviction := int(float64(rec.Conviction) * weight)

		// Clamp to valid range
		if weightedConviction > 250 {
			weightedConviction = 250
		}
		if weightedConviction < 1 {
			weightedConviction = 1
		}

		weighted = append(weighted, domain.Recommendation{
			Agent:      rec.Agent,
			Skill:      rec.Skill,
			Symbol:     rec.Symbol,
			Side:       rec.Side,
			Conviction: weightedConviction,
			Reason:     fmt.Sprintf("%s [DW:%.2f]", rec.Reason, weight),
		})
	}

	return weighted
}

// DarwinianWeightReport represents a comprehensive weight status report
type DarwinianWeightReport struct {
	GeneratedAt        time.Time              `json:"generated_at"`
	TotalAgents        int                    `json:"total_agents"`
	TopPerformers      []DarwinianAgentWeight `json:"top_performers"`
	BottomPerformers   []DarwinianAgentWeight `json:"bottom_performers"`
	Neutrals           []DarwinianAgentWeight `json:"neutrals"`
	WeightDistribution map[string]int         `json:"weight_distribution"`
	Summary            string                 `json:"summary"`
}

// GenerateReport creates a comprehensive report of current weight distribution
func (m *DarwinianWeightManager) GenerateReport() *DarwinianWeightReport {
	m.mu.RLock()
	defer m.mu.RUnlock()

	report := &DarwinianWeightReport{
		GeneratedAt:        time.Now(),
		TotalAgents:        len(m.weights),
		TopPerformers:      make([]DarwinianAgentWeight, 0),
		BottomPerformers:   make([]DarwinianAgentWeight, 0),
		Neutrals:           make([]DarwinianAgentWeight, 0),
		WeightDistribution: make(map[string]int),
	}

	// Collect all agents
	var allAgents []DarwinianAgentWeight
	for _, w := range m.weights {
		allAgents = append(allAgents, *w)
	}

	// Sort by weight descending
	sort.Slice(allAgents, func(i, j int) bool {
		return allAgents[i].Weight > allAgents[j].Weight
	})

	// Categorize agents
	for _, agent := range allAgents {
		if agent.Weight >= 1.5 {
			report.TopPerformers = append(report.TopPerformers, agent)
		} else if agent.Weight <= 0.7 {
			report.BottomPerformers = append(report.BottomPerformers, agent)
		} else {
			report.Neutrals = append(report.Neutrals, agent)
		}
	}

	// Calculate distribution
	report.WeightDistribution["shouting_2.0+"] = 0
	report.WeightDistribution["strong_1.5-2.0"] = 0
	report.WeightDistribution["neutral_0.8-1.5"] = 0
	report.WeightDistribution["weak_0.5-0.8"] = 0
	report.WeightDistribution["whispering_0.3-0.5"] = 0

	for _, agent := range allAgents {
		w := agent.Weight
		switch {
		case w >= 2.0:
			report.WeightDistribution["shouting_2.0+"]++
		case w >= 1.5:
			report.WeightDistribution["strong_1.5-2.0"]++
		case w >= 0.8:
			report.WeightDistribution["neutral_0.8-1.5"]++
		case w >= 0.5:
			report.WeightDistribution["weak_0.5-0.8"]++
		default:
			report.WeightDistribution["whispering_0.3-0.5"]++
		}
	}

	// Generate summary
	shoutingCount := report.WeightDistribution["shouting_2.0+"]
	whisperingCount := report.WeightDistribution["whispering_0.3-0.5"]

	if shoutingCount > 0 && whisperingCount > 0 {
		report.Summary = fmt.Sprintf(
			"Darwinian selection active: %d agents shouting (weight ≥2.0), %d agents whispering (weight ≤0.5). "+
				"Top performer: %s (weight %.2f)",
			shoutingCount, whisperingCount,
			allAgents[0].AgentID, allAgents[0].Weight,
		)
	} else if shoutingCount > 0 {
		report.Summary = fmt.Sprintf(
			"Strong selection pressure: %d agents at maximum weight. "+
				"No agents at minimum weight - system performing well.",
			shoutingCount,
		)
	} else if whisperingCount > 0 {
		report.Summary = fmt.Sprintf(
			"Warning: %d agents at minimum weight may need review or retraining. "+
				"Consider prompt optimization or disabling underperformers.",
			whisperingCount,
		)
	} else {
		report.Summary = "Balanced weight distribution. No extreme outliers requiring immediate attention."
	}

	return report
}

// SaveReport saves the weight report to a JSON file
func (m *DarwinianWeightManager) SaveReport(path string) error {
	report := m.GenerateReport()

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	return nil
}
