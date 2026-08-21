package portfolio

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/logging"
)

const (
	// DEPRECATED: Use RuntimeParameters instead. These constants exist for test compatibility.
	DarwinianWeightMin = 0.3
	// DEPRECATED: Use RuntimeParameters instead. These constants exist for test compatibility.
	DarwinianWeightMax = 2.5
	// DEPRECATED: Use RuntimeParameters instead. These constants exist for test compatibility.
	DarwinianNeutralWeight = 1.0
	// DEPRECATED: Use RuntimeParameters instead. These constants exist for test compatibility.
	TopQuartileMultiplier = 1.05
	// DEPRECATED: Use RuntimeParameters instead. These constants exist for test compatibility.
	BottomQuartileMultiplier = 0.95
	// DEPRECATED: Use RuntimeParameters instead. These constants exist for test compatibility.
	DailyAdjustmentCooldown = 20 * time.Hour

	// minUniqueReturnsForSharpe is the minimum number of distinct values a
	// rolling-return window must contain for Sharpe/volatility to be
	// statistically meaningful. Windows with fewer unique values have a
	// degenerate (near-zero) sample stdDev and mean/std explodes. A4 audit:
	// 100% of historical |sharpe|>5 records had <=6 unique values.
	minUniqueReturnsForSharpe = 8

	// maxSharpeMagnitude clamps the stored RollingSharpe (defense-in-depth)
	// so downstream weight math never sees pathological values.
	maxSharpeMagnitude = 10.0
)

// ConvictionClampingEvent records when a conviction value was clamped.
type ConvictionClampingEvent struct {
	AgentID         string    `json:"agent_id"`
	Symbol          string    `json:"symbol"`
	RawConviction   int       `json:"raw_conviction"`
	FinalConviction int       `json:"final_conviction"`
	Weight          float64   `json:"weight"`
	Boundary        string    `json:"boundary"`
	Timestamp       time.Time `json:"timestamp"`
}

// ClampingEvent records when a weight was clamped to a boundary.
type ClampingEvent struct {
	AgentID     string    `json:"agent_id"`
	RawWeight   float64   `json:"raw_weight"`
	FinalWeight float64   `json:"final_weight"`
	Boundary    string    `json:"boundary"`
	Timestamp   time.Time `json:"timestamp"`
}

// DarwinianConfig holds configuration for the Darwinian weight system.
// These settings control exponential decay, new-agent protection,
// and adjustment frequency.
type DarwinianConfig struct {
	RollingWindowDays      int     `json:"rolling_window_days"`
	UseExponentialDecay    bool    `json:"use_exponential_decay"`
	DecayHalfLifeDays      int     `json:"decay_half_life_days"`
	NewAgentProtectionDays int     `json:"new_agent_protection_days"`
	NewAgentFixedWeight    float64 `json:"new_agent_fixed_weight"`
	MinAdjustmentInterval  int     `json:"min_adjustment_interval"`
	WeightMomentumFactor   float64 `json:"weight_momentum_factor"`
}

// DefaultDarwinianConfig returns sensible defaults for the weight system.
func DefaultDarwinianConfig() DarwinianConfig {
	return DarwinianConfig{
		RollingWindowDays:      60,
		UseExponentialDecay:    true,
		DecayHalfLifeDays:      10,
		NewAgentProtectionDays: 30,
		NewAgentFixedWeight:    1.0,
		MinAdjustmentInterval:  3,
		WeightMomentumFactor:   0.2,
	}
}

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
	DailyReturns      []float64 `json:"daily_returns"`      // Rolling window of returns for Sharpe calc
	ConsecutiveAtMin  int       `json:"consecutive_at_min"` // Days stuck at weight minimum
	// Per-day aggregation state (RecordOutcomeAt): forward returns recorded
	// on the same calendar day are accumulated here and flushed as a single
	// daily mean into DailyReturns when the next day arrives.
	LastOutcomeDay string  `json:"last_outcome_day,omitempty"` // yyyy-mm-dd of in-progress day
	LastDaySum     float64 `json:"last_day_sum,omitempty"`     // running sum of the day's returns
	LastDayCount   int     `json:"last_day_count,omitempty"`   // number of outcomes recorded that day
}

// DarwinianWeightManager implements Atlas-GIC style Darwinian weight system
type DarwinianWeightManager struct {
	weights         map[string]*DarwinianAgentWeight
	configPath      string
	historyPath     string
	lookbackDays    int
	params          *RuntimeParameters
	mu              sync.RWMutex
	eventBus        *eventbus.ChannelEventBus
	maturityTracker *domain.MaturityTracker
}

// NewDarwinianWeightManager creates a new Darwinian weight manager
func NewDarwinianWeightManager(configPath string) *DarwinianWeightManager {
	params := DefaultRuntimeParameters()
	return &DarwinianWeightManager{
		weights:      make(map[string]*DarwinianAgentWeight),
		configPath:   configPath,
		historyPath:  "",
		lookbackDays: params.Darwinian.LookbackDays,
		params:       params,
	}
}

// WithParameters sets runtime parameters for the manager.
// Call this immediately after NewDarwinianWeightManager to override defaults.
func (m *DarwinianWeightManager) WithParameters(p *RuntimeParameters) *DarwinianWeightManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p != nil {
		m.params = p
		m.lookbackDays = p.Darwinian.LookbackDays
	}
	return m
}

func (m *DarwinianWeightManager) WithHistoryPath(path string) *DarwinianWeightManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.historyPath = path
	return m
}

// AppendSnapshot appends current weights as a timestamped history entry to the JSONL file.
func (m *DarwinianWeightManager) AppendSnapshot() error {
	if m.historyPath == "" {
		return nil
	}
	m.mu.RLock()
	snapshot := DarwinianSnapshot{
		Timestamp: time.Now(),
		Weights:   make(map[string]DarwinianAgentWeight, len(m.weights)),
	}
	for id, w := range m.weights {
		cp := *w
		cp.DailyReturns = make([]float64, len(w.DailyReturns))
		copy(cp.DailyReturns, w.DailyReturns)
		snapshot.Weights[id] = cp
	}
	m.mu.RUnlock()

	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	dir := filepath.Dir(m.historyPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create history dir: %w", err)
	}
	f, err := os.OpenFile(m.historyPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open history file: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("append snapshot: %w", err)
	}
	return nil
}

type DarwinianSnapshot struct {
	Timestamp time.Time                       `json:"timestamp"`
	Weights   map[string]DarwinianAgentWeight `json:"weights"`
}

// LoadHistory loads up to limit recent snapshots from the JSONL history file.
// Returns snapshots in reverse chronological order (newest first, oldest last).
// The frontend reverses this again for sparkline display to show oldest-to-newest.
func (m *DarwinianWeightManager) LoadHistory(limit int) ([]DarwinianSnapshot, error) {
	if m.historyPath == "" {
		return nil, nil
	}
	data, err := os.ReadFile(m.historyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read history: %w", err)
	}
	var snapshots []DarwinianSnapshot
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for i := len(lines) - 1; i >= 0 && len(snapshots) < limit; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var snap DarwinianSnapshot
		if err := json.Unmarshal([]byte(line), &snap); err != nil {
			continue
		}
		snapshots = append(snapshots, snap)
	}
	return snapshots, nil
}

// WithEventBus sets the event bus for publishing clamping events.
func (m *DarwinianWeightManager) WithEventBus(eb *eventbus.ChannelEventBus) *DarwinianWeightManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.eventBus = eb
	return m
}

// WithMaturityTracker attaches a maturity tracker for burn-in gating.
func (m *DarwinianWeightManager) WithMaturityTracker(mt *domain.MaturityTracker) *DarwinianWeightManager {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.maturityTracker = mt
	return m
}

// InitializeFromRegistry initializes weights from agent registry
func (m *DarwinianWeightManager) InitializeFromRegistry(registry domain.AgentRegistry) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, agent := range registry.Agents {
		if !agent.Enabled {
			continue
		}
		// Initialize for Sector, Style, and Superinvestor layers
		if agent.Layer != domain.LayerSector && agent.Layer != domain.LayerStyle && agent.Layer != domain.LayerSuperinvestor {
			continue
		}

		if _, exists := m.weights[agent.ID]; !exists {
			// Use agent's DarwinianWeight if specified, otherwise use neutral weight
			initialWeight := agent.DarwinianWeight
			if initialWeight <= 0 {
				initialWeight = m.params.Darwinian.WeightNeutral
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

// RecordOutcome records a recommendation outcome for an agent.
//
// Legacy per-call path: each call appends one entry to the rolling window.
// Production should prefer RecordOutcomeAt, which aggregates all outcomes of
// the same calendar day into a single daily mean so DailyReturns is a true
// daily series (see A4 sharpe-outlier audit, L1 root cause).
func (m *DarwinianWeightManager) RecordOutcome(agentID string, forwardReturn float64, isHit bool) {
	m.recordOutcome(agentID, forwardReturn, isHit, "")
}

// RecordOutcomeAt records an outcome with its observation time. Outcomes
// recorded on the same calendar day (per agent) are aggregated into a single
// daily mean before entering the rolling window — one window entry per
// trading day instead of one per recommendation. This keeps the rolling
// window from collapsing into a fraction of a day when an agent emits many
// recommendations per day, and makes the window a true daily series.
func (m *DarwinianWeightManager) RecordOutcomeAt(agentID string, forwardReturn float64, isHit bool, recordedAt time.Time) {
	m.recordOutcome(agentID, forwardReturn, isHit, recordedAt.Format("2006-01-02"))
}

func (m *DarwinianWeightManager) recordOutcome(agentID string, forwardReturn float64, isHit bool, dayKey string) {
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
	alpha := m.params.Darwinian.EMAAlpha
	if w.TotalSignals == 1 {
		w.AvgReturn = forwardReturn
	} else {
		w.AvgReturn = alpha*forwardReturn + (1-alpha)*w.AvgReturn
	}

	if dayKey == "" {
		// Legacy per-call path: append immediately.
		w.DailyReturns = append(w.DailyReturns, forwardReturn)
		if len(w.DailyReturns) > m.lookbackDays {
			w.DailyReturns = w.DailyReturns[1:]
		}
	} else {
		// Per-day aggregation: flush the previous completed day as its mean,
		// then start accumulating the new day.
		if w.LastOutcomeDay != dayKey {
			if w.LastOutcomeDay != "" && w.LastDayCount > 0 {
				w.DailyReturns = append(w.DailyReturns, w.LastDaySum/float64(w.LastDayCount))
				if len(w.DailyReturns) > m.lookbackDays {
					w.DailyReturns = w.DailyReturns[1:]
				}
			}
			w.LastOutcomeDay = dayKey
			w.LastDaySum = forwardReturn
			w.LastDayCount = 1
		} else {
			w.LastDaySum += forwardReturn
			w.LastDayCount++
		}
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

	// Degenerate-window guard (A4 L3): with only a handful of distinct values
	// the sample stdDev is tiny and mean/std (Sharpe) explodes. 100% of
	// historical |sharpe|>5 records had <=6 unique values in the window.
	if uniqueFloat64Count(recentReturns) < minUniqueReturnsForSharpe {
		w.RollingSharpe = 0
		w.RollingVolatility = 0
		return
	}

	// Calculate rolling Sharpe.
	//
	// FrequencyPerOutcome (no sqrt(252) annualization): window entries are
	// per-day means (RecordOutcomeAt) or per-outcome values (legacy
	// RecordOutcome) — neither is a calendar daily series, so annualizing by
	// sqrt(252) overstated Sharpe by 100-600x (A4 L1 root cause) and pushed
	// the normalized weight sigmoid into saturation. Per-outcome Sharpe for
	// realistic TW returns lands in the ±3 band the SharpeNormalizeDenom
	// calibration assumes (median~0.5, top-quartile~1.5).
	w.RollingSharpe = ComputeSharpe(recentReturns, SharpeConfig{
		Frequency:                FrequencyPerOutcome,
		MinSamples:               m.params.Darwinian.SharpeMinSampleSize,
		StdDevMeanRatioThreshold: m.params.Darwinian.StdDevMeanRatioThreshold,
	})

	// Defense-in-depth clamp: even with the guards above, cap pathological
	// Sharpe before any downstream weight math consumes it.
	w.RollingSharpe = math.Max(-maxSharpeMagnitude, math.Min(maxSharpeMagnitude, w.RollingSharpe))

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
	w.RollingVolatility = math.Sqrt(variance / float64(len(recentReturns)-1))
}

// uniqueFloat64Count returns the number of distinct values in the slice.
func uniqueFloat64Count(values []float64) int {
	seen := make(map[float64]struct{}, len(values))
	for _, v := range values {
		seen[v] = struct{}{}
	}
	return len(seen)
}

// PerformDailyAdjustment performs the daily Darwinian weight adjustment.
// Enhanced algorithm with performance-based scaling and volatility adjustment.
// Returns adjustments map and any clamping events that occurred.
func (m *DarwinianWeightManager) PerformDailyAdjustment() (map[string]float64, []ClampingEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	adjustments := make(map[string]float64)
	var clampingEvents []ClampingEvent

	burnIn := m.maturityTracker != nil && m.maturityTracker.Current() == domain.MaturityBurnIn

	// Check cooldown and collect eligible agents
	now := time.Now()
	eligible := make([]*DarwinianAgentWeight, 0)
	cooldown := m.params.Darwinian.DailyAdjustmentCooldown
	if burnIn {
		cooldown = cooldown * 2 // Double cooldown during burn-in
	}

	for _, w := range m.weights {
		if now.Sub(w.LastAdjustedAt) >= cooldown {
			eligible = append(eligible, w)
		}
	}

	if len(eligible) < 2 {
		return adjustments, clampingEvents
	}

	if burnIn {
		logging.Info("darwinian_weights", "burn_in_conservative",
			"days_until_calibrating", m.maturityTracker.DaysUntil(domain.MaturityCalibrating),
			"eligible_agents", len(eligible))
	}

	// Auto-reset stuck agents: agents at minimum weight for consecutive cycles
	// get a fresh start. This prevents agents from being permanently trapped at
	// the floor when market regimes shift and their strategy becomes relevant again.
	const autoResetThreshold = 5
	for _, w := range m.weights {
		if math.Abs(w.Weight-m.params.Darwinian.WeightMin) < 0.001 {
			w.ConsecutiveAtMin++
			if w.ConsecutiveAtMin >= autoResetThreshold && w.TotalSignals > 10 {
				w.Weight = m.params.Darwinian.WeightNeutral
				w.RollingSharpe = 0
				w.DailyReturns = w.DailyReturns[:0]
				w.LastOutcomeDay = ""
				w.LastDaySum = 0
				w.LastDayCount = 0
				w.ConsecutiveAtMin = 0
				w.LastAdjustedAt = now
				logging.Info("darwinian_weights", "auto_reset_stuck_agent",
					logging.AgentID(w.AgentID),
					logging.FFloat64("reset_to", m.params.Darwinian.WeightNeutral))
			}
		} else {
			w.ConsecutiveAtMin = 0
		}
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
	topTier := max(n/3, 1)
	bottomTier := max(n/3, 1)

	// Top tier: significant increase with performance scaling
	for i := range topTier {
		w := eligible[i]
		oldWeight := w.Weight

		// During burn-in, disable performance bonus (Sharpe is unstable).
		// Use a conservative multiplier to allow slow evolution.
		var performanceBonus float64
		if burnIn {
			performanceBonus = 1.0 // No bonus during burn-in
		} else {
			denom := m.params.Darwinian.SharpeNormalizeDenom
			normalizedSharpe := w.RollingSharpe / (w.RollingSharpe + denom)
			performanceBonus = 1.0 + normalizedSharpe*m.params.Darwinian.MaxPerformanceBonusPct
		}

		volatilityPenalty := 1.0
		if w.RollingVolatility > m.params.Darwinian.VolatilityPenaltyThreshold {
			volatilityPenalty = m.params.Darwinian.VolatilityPenaltyMultiplier
		}

		multiplier := m.params.Darwinian.TopQuartileMultiplier * performanceBonus * volatilityPenalty
		if burnIn {
			multiplier = math.Sqrt(multiplier) // Reduce adjustment magnitude by ~50%
		}
		clamped, event := m.constrainWeight(w.AgentID, oldWeight*multiplier)
		w.Weight = clamped
		if event != nil {
			clampingEvents = append(clampingEvents, *event)
		}
		w.LastAdjustedAt = now

		adjustments[w.AgentID] = w.Weight - oldWeight
	}

	// Middle tier: maintain or slight adjustment
	for i := topTier; i < n-bottomTier; i++ {
		w := eligible[i]
		oldWeight := w.Weight

		// Slight adjustment based on hit rate
		var multiplier float64
		if w.HitRate > m.params.Darwinian.HitRateHighThreshold {
			multiplier = m.params.Darwinian.MiddleTierBoostMultiplier
		} else if w.HitRate < m.params.Darwinian.HitRateLowThreshold {
			multiplier = m.params.Darwinian.MiddleTierCutMultiplier
		} else {
			multiplier = 1.0
		}
		if burnIn {
			multiplier = math.Sqrt(multiplier)
		}
		if multiplier != 1.0 {
			clamped, event := m.constrainWeight(w.AgentID, oldWeight*multiplier)
			w.Weight = clamped
			if event != nil {
				clampingEvents = append(clampingEvents, *event)
			}
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
		if w.RollingVolatility > m.params.Darwinian.RiskVolatilityThreshold {
			riskMultiplier = m.params.Darwinian.RiskMultiplier
		}

		multiplier := m.params.Darwinian.BottomQuartileMultiplier * riskMultiplier
		if burnIn {
			multiplier = math.Sqrt(multiplier)
		}
		clamped, event := m.constrainWeight(w.AgentID, oldWeight*multiplier)
		w.Weight = clamped
		if event != nil {
			clampingEvents = append(clampingEvents, *event)
		}
		w.LastAdjustedAt = now

		adjustments[w.AgentID] = w.Weight - oldWeight
	}

	if len(clampingEvents) > 0 && m.eventBus != nil {
		payloads := make([]eventbus.ClampingEventPayload, len(clampingEvents))
		for i, e := range clampingEvents {
			payloads[i] = eventbus.ClampingEventPayload{
				AgentID:     e.AgentID,
				RawWeight:   e.RawWeight,
				FinalWeight: e.FinalWeight,
				Boundary:    e.Boundary,
				Timestamp:   e.Timestamp,
			}
		}
		m.eventBus.PublishDarwinianClamping(payloads)
	}

	return adjustments, clampingEvents
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

// constrainWeight ensures weight stays within configured bounds.
// Must be called while holding m.mu (either read or write lock).
func (m *DarwinianWeightManager) constrainWeight(agentID string, weight float64) (float64, *ClampingEvent) {
	minW := m.params.Darwinian.WeightMin
	maxW := m.params.Darwinian.WeightMax
	if weight < minW {
		logging.Warn("darwinian_weights", "weight_clamped_min", logging.AgentID(agentID), logging.FFloat64("weight", weight), logging.FFloat64("min", minW))
		return minW, &ClampingEvent{
			AgentID:     agentID,
			RawWeight:   weight,
			FinalWeight: minW,
			Boundary:    "min",
			Timestamp:   time.Now(),
		}
	}
	if weight > maxW {
		logging.Warn("darwinian_weights", "weight_clamped_max", logging.AgentID(agentID), logging.FFloat64("weight", weight), logging.FFloat64("max", maxW))
		return maxW, &ClampingEvent{
			AgentID:     agentID,
			RawWeight:   weight,
			FinalWeight: maxW,
			Boundary:    "max",
			Timestamp:   time.Now(),
		}
	}
	return weight, nil
}

// SetWeight manually sets the Darwinian weight for an agent.
// The weight is clamped to the configured [WeightMin, WeightMax] range.
// Returns the final (possibly clamped) weight and any clamping event.
func (m *DarwinianWeightManager) SetWeight(agentID string, weight float64) (float64, *ClampingEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	finalW, clamp := m.constrainWeight(agentID, weight)
	if existing, ok := m.weights[agentID]; ok {
		existing.Weight = finalW
		existing.LastUpdatedAt = time.Now()
	} else {
		m.weights[agentID] = &DarwinianAgentWeight{
			Weight:        finalW,
			AgentID:       agentID,
			LastUpdatedAt: time.Now(),
		}
	}
	return finalW, clamp
}

// GetWeight gets the Darwinian weight for an agent
func (m *DarwinianWeightManager) GetWeight(agentID string) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if w, ok := m.weights[agentID]; ok {
		return w.Weight
	}
	return m.params.Darwinian.WeightNeutral
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
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

	if err := os.WriteFile(m.configPath, jsonData, 0o644); err != nil {
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

	neutral := m.params.Darwinian.WeightNeutral
	for _, w := range m.weights {
		w.Weight = neutral
		w.RollingSharpe = 0
		w.DailyReturns = w.DailyReturns[:0]
		w.LastOutcomeDay = ""
		w.LastDaySum = 0
		w.LastDayCount = 0
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

	w.Weight = m.params.Darwinian.WeightNeutral
	w.RollingSharpe = 0
	w.DailyReturns = w.DailyReturns[:0]
	w.LastOutcomeDay = ""
	w.LastDaySum = 0
	w.LastDayCount = 0
	w.LastAdjustedAt = time.Now()
	return true
}

// RemoveAgent removes an agent from tracking
func (m *DarwinianWeightManager) RemoveAgent(agentID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.weights, agentID)
}

// ApplyDarwinianWeightsWithEvents applies Darwinian weights and returns clamping events for monitoring.
// This is the preferred method when you need audit trail of conviction clamping.
func (m *DarwinianWeightManager) ApplyDarwinianWeightsWithEvents(
	recommendations []domain.Recommendation,
) ([]domain.Recommendation, []ConvictionClampingEvent) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	weighted := make([]domain.Recommendation, 0, len(recommendations))
	var clampingEvents []ConvictionClampingEvent

	for _, rec := range recommendations {
		weight := m.params.Darwinian.WeightNeutral
		if w, ok := m.weights[rec.Agent]; ok {
			weight = w.Weight
		}

		rawConviction := rec.Conviction
		weightedConviction := int(float64(rec.Conviction) * weight)

		clampMin := m.params.Darwinian.ConvictionClampMin
		clampMax := m.params.Darwinian.ConvictionClampMax
		boundary := ""
		if weightedConviction > clampMax {
			boundary = "max"
			weightedConviction = clampMax
		} else if weightedConviction < clampMin {
			boundary = "min"
			weightedConviction = clampMin
		}

		if boundary != "" {
			clampingEvents = append(clampingEvents, ConvictionClampingEvent{
				AgentID:         rec.Agent,
				Symbol:          rec.Symbol,
				RawConviction:   rawConviction,
				FinalConviction: weightedConviction,
				Weight:          weight,
				Boundary:        boundary,
				Timestamp:       time.Now(),
			})
		}

		cb := rec.ConvictionBreakdown
		if cb != nil && weight != 1.0 {
			weightAdj := weightedConviction - rawConviction
			if weightAdj != 0 {
				cb.Steps = append(cb.Steps, domain.ConvictionStep{
					Rule:       "modulator:darwinian:weight_adjust",
					Delta:      weightAdj,
					Reason:     fmt.Sprintf("Darwinian weight %.2f applied (raw=%d → weighted=%d)", weight, rawConviction, weightedConviction),
					Source:     "config",
					ParamRef:   fmt.Sprintf("Darwinian.%s.Weight", rec.Agent),
					ParamValue: fmt.Sprintf("%.2f", weight),
				})
			}
			if boundary != "" {
				cb.Steps = append(cb.Steps, domain.ConvictionStep{
					Rule:       "modulator:darwinian:clamp_" + boundary,
					Delta:      weightedConviction - rawConviction - weightAdj,
					Reason:     fmt.Sprintf("Conviction clamped to %s=%d", boundary, weightedConviction),
					Source:     "config",
					ParamRef:   fmt.Sprintf("Darwinian.ConvictionClamp%s", boundary),
					ParamValue: fmt.Sprintf("%d", weightedConviction),
				})
			}
			cb.Final = weightedConviction
		}

		weighted = append(weighted, domain.Recommendation{
			Agent:               rec.Agent,
			Skill:               rec.Skill,
			Layer:               rec.Layer,
			Symbol:              rec.Symbol,
			Side:                rec.Side,
			Conviction:          weightedConviction,
			TargetPrice:         rec.TargetPrice,
			StopLossPrice:       rec.StopLossPrice,
			Reason:              fmt.Sprintf("%s [DW:%.2f]", rec.Reason, weight),
			ReasoningChain:      rec.ReasoningChain,
			SupportingEvents:    rec.SupportingEvents,
			FactorScores:        rec.FactorScores,
			ConvictionBreakdown: rec.ConvictionBreakdown,
		})
	}

	return weighted, clampingEvents
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

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	return nil
}
