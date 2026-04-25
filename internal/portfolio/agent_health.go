package portfolio

import (
	"fmt"
	"sync"
	"time"
)

type AgentHealthStatus string

const (
	HealthStatusHealthy    AgentHealthStatus = "healthy"
	HealthStatusDegraded   AgentHealthStatus = "degraded"
	HealthStatusMuted      AgentHealthStatus = "muted"
	HealthStatusRecovering AgentHealthStatus = "recovering"
)

type AgentHealth struct {
	AgentID           string            `json:"agent_id"`
	Status            AgentHealthStatus `json:"status"`
	AnnualizedSharpe  float64           `json:"annualized_sharpe"`
	HitRate           float64           `json:"hit_rate"`
	ConsecutiveLosses int               `json:"consecutive_losses"`
	ConsecutiveWins   int               `json:"consecutive_wins"`
	CompositeScore    float64           `json:"composite_score"`
	MutedAt           *time.Time        `json:"muted_at,omitempty"`
	UnmutedAt         *time.Time        `json:"unmuted_at,omitempty"`
}

type AgentHealthConfig struct {
	DefaultMuteThreshold    int     `json:"default_mute_threshold"`
	DefaultUnmuteThreshold  int     `json:"default_unmute_threshold"`
	DefaultAutoRecoverDays  int     `json:"default_auto_recover_days"`
	MinSampleSize           int     `json:"min_sample_size"`
	NegativeSharpeThreshold float64 `json:"negative_sharpe_threshold"`
}

func DefaultAgentHealthConfig() AgentHealthConfig {
	return AgentHealthConfig{
		DefaultMuteThreshold:    5,
		DefaultUnmuteThreshold:  3,
		DefaultAutoRecoverDays:  7,
		MinSampleSize:           10,
		NegativeSharpeThreshold: -0.5,
	}
}

type AgentHealthManager struct {
	mu     sync.RWMutex
	health map[string]*AgentHealth
	config AgentHealthConfig
	store  *AgentHealthStore
}

func NewAgentHealthManager() *AgentHealthManager {
	return NewAgentHealthManagerWithConfig(DefaultAgentHealthConfig())
}

func NewAgentHealthManagerWithConfig(config AgentHealthConfig) *AgentHealthManager {
	return &AgentHealthManager{
		health: make(map[string]*AgentHealth),
		config: config,
	}
}

func NewAgentHealthManagerWithStore(config AgentHealthConfig, store *AgentHealthStore) *AgentHealthManager {
	m := NewAgentHealthManagerWithConfig(config)
	if store != nil {
		m.store = store
		if saved, err := store.LoadAll(); err == nil && saved != nil {
			m.health = saved
		}
	}
	return m
}

func (m *AgentHealthManager) GetHealth(agentID string) *AgentHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.health[agentID]
}

func (m *AgentHealthManager) IsAgentHealthy(agentID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	h, ok := m.health[agentID]
	if !ok {
		return true
	}
	return h.Status == HealthStatusHealthy || h.Status == HealthStatusRecovering
}

func (m *AgentHealthManager) GetMutedAgents() []*AgentHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var muted []*AgentHealth
	for _, h := range m.health {
		if h.Status == HealthStatusMuted {
			muted = append(muted, h)
		}
	}
	return muted
}

func (m *AgentHealthManager) RecordOutcome(agentID string, isWin bool, sharpe float64, hitRate float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	h, ok := m.health[agentID]
	if !ok {
		h = &AgentHealth{
			AgentID: agentID,
			Status:  HealthStatusHealthy,
		}
		m.health[agentID] = h
	}

	if isWin {
		h.ConsecutiveWins++
		h.ConsecutiveLosses = 0
	} else {
		h.ConsecutiveLosses++
		h.ConsecutiveWins = 0
	}

	h.AnnualizedSharpe = sharpe
	h.HitRate = hitRate
	h.CompositeScore = m.calculateCompositeScore(sharpe, hitRate, h.ConsecutiveWins, h.ConsecutiveLosses)

	m.evaluateInterventions(h)

	if m.store != nil {
		_ = m.store.Save(h)
	}
}

func (m *AgentHealthManager) calculateCompositeScore(sharpe, hitRate float64, consecutiveWins, consecutiveLosses int) float64 {
	const (
		sharpeWeight  = 0.40
		hitRateWeight = 0.30
		streakWeight  = 0.30
		maxSharpe     = 5.0
		minSharpe     = -5.0
		streakMax     = 10
	)

	sharpeNorm := (sharpe - minSharpe) / (maxSharpe - minSharpe)
	if sharpeNorm < 0 {
		sharpeNorm = 0
	}
	if sharpeNorm > 1 {
		sharpeNorm = 1
	}

	hitRateNorm := hitRate
	if hitRateNorm < 0 {
		hitRateNorm = 0
	}
	if hitRateNorm > 1 {
		hitRateNorm = 1
	}

	streakScore := float64(consecutiveWins) / float64(streakMax)
	if streakScore > 1 {
		streakScore = 1
	}

	return (sharpeWeight * sharpeNorm * 100) +
		(hitRateWeight * hitRateNorm * 100) +
		(streakWeight * streakScore * 100)
}

func (m *AgentHealthManager) evaluateInterventions(h *AgentHealth) {
	switch h.Status {
	case HealthStatusHealthy, HealthStatusDegraded:
		if h.ConsecutiveLosses >= m.config.DefaultMuteThreshold {
			h.Status = HealthStatusMuted
			now := time.Now()
			h.MutedAt = &now
			h.UnmutedAt = nil
			return
		}
		if h.AnnualizedSharpe < m.config.NegativeSharpeThreshold {
			h.Status = HealthStatusMuted
			now := time.Now()
			h.MutedAt = &now
			h.UnmutedAt = nil
			return
		}

	case HealthStatusMuted:
		if h.ConsecutiveWins >= m.config.DefaultUnmuteThreshold {
			h.Status = HealthStatusRecovering
			h.UnmutedAt = nil
			return
		}
		if h.MutedAt != nil {
			recoverDays := m.config.DefaultAutoRecoverDays
			if time.Since(*h.MutedAt) >= time.Duration(recoverDays)*time.Hour*24 {
				h.Status = HealthStatusRecovering
				h.UnmutedAt = nil
			}
		}
	}
}

func (m *AgentHealthManager) EvaluateAgentBreakers() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, h := range m.health {
		m.evaluateInterventions(h)
	}
}

func (m *AgentHealthManager) String() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var lines []string
	lines = append(lines, fmt.Sprintf("AgentHealthManager{config: %+v, agents:", m.config))
	for id, h := range m.health {
		lines = append(lines, fmt.Sprintf("  %s: status=%s sharpe=%.2f hitrate=%.2f losses=%d wins=%d composite=%.2f",
			id, h.Status, h.AnnualizedSharpe, h.HitRate, h.ConsecutiveLosses, h.ConsecutiveWins, h.CompositeScore))
	}
	lines = append(lines, "}")
	return lines[0] + "\n" + joinLines(lines[1:]...)
}

func joinLines(parts ...string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "\n"
		}
		result += p
	}
	return result
}
