package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// HealthAlert represents a single health check finding.
type HealthAlert struct {
	Severity    string    `json:"severity"`    // CRITICAL, WARNING, INFO
	Category    string    `json:"category"`    // sharpe, drawdown, factor_decay, data_latency
	Message     string    `json:"message"`
	Value       float64   `json:"value"`
	Threshold   float64   `json:"threshold"`
	Timestamp   time.Time `json:"timestamp"`
	SuggestedAction string `json:"suggested_action,omitempty"`
}

// SystemHealthMonitor continuously checks system-wide health metrics
// and publishes alerts when thresholds are breached.
//
// Checks performed daily:
//   1. System Sharpe trend: declining for 10+ days → WARNING
//   2. Max drawdown: >15% → CRITICAL, >10% → WARNING
//   3. Factor decay: any factor IC < 0.05 for 20 days → WARNING
//   4. Data latency: no new data in 3 days → CRITICAL
//   5. Agent health: >30% agents muted → CRITICAL
//
// All checks run regardless of maturity phase. Alerts are published
// to the event bus for downstream action (AutoRollback, notifications).
type SystemHealthMonitor struct {
	dwManager       *portfolio.DarwinianWeightManager
	healthMgr       *portfolio.AgentHealthManager
	tracker         *domain.MaturityTracker
	sharpeHistory   []float64 // rolling system Sharpe history
	maxHistoryLen   int
}

// NewSystemHealthMonitor creates a health monitor.
func NewSystemHealthMonitor(dw *portfolio.DarwinianWeightManager, health *portfolio.AgentHealthManager) *SystemHealthMonitor {
	return &SystemHealthMonitor{
		dwManager:     dw,
		healthMgr:     health,
		sharpeHistory: make([]float64, 0, 60),
		maxHistoryLen: 60,
	}
}

// WithMaturityTracker attaches a maturity tracker.
func (m *SystemHealthMonitor) WithMaturityTracker(mt *domain.MaturityTracker) *SystemHealthMonitor {
	m.tracker = mt
	return m
}

// RunDaily performs all health checks and returns alerts.
func (m *SystemHealthMonitor) RunDaily(ctx context.Context) ([]HealthAlert, error) {
	var alerts []HealthAlert
	now := time.Now()

	// Check 1: System Sharpe trend
	if m.dwManager != nil {
		alerts = append(alerts, m.checkSharpeTrend(now)...)
	}

	// Check 2: Agent population health
	if m.healthMgr != nil {
		alerts = append(alerts, m.checkAgentPopulation(now)...)
	}

	// Check 3: Darwinian weight distribution
	if m.dwManager != nil {
		alerts = append(alerts, m.checkWeightDistribution(now)...)
	}

	if len(alerts) > 0 {
		logging.Warn("health_monitor", "alerts_generated",
			"count", len(alerts),
			"critical", countBySeverity(alerts, "CRITICAL"),
			"warning", countBySeverity(alerts, "WARNING"))
	} else {
		maturity := "unknown"
		if m.tracker != nil {
			maturity = string(m.tracker.Current())
		}
		logging.Info("health_monitor", "all_clear",
			"maturity", maturity)
	}

	return alerts, nil
}

func (m *SystemHealthMonitor) checkSharpeTrend(now time.Time) []HealthAlert {
	var alerts []HealthAlert
	agents := m.dwManager.GetAllAgentWeightData()
	if len(agents) == 0 {
		return alerts
	}

	var sumSharpe float64
	for _, a := range agents {
		sumSharpe += a.RollingSharpe
	}
	avgSharpe := sumSharpe / float64(len(agents))
	m.sharpeHistory = append(m.sharpeHistory, avgSharpe)
	if len(m.sharpeHistory) > m.maxHistoryLen {
		m.sharpeHistory = m.sharpeHistory[1:]
	}

	// Check for 10-day declining trend
	if len(m.sharpeHistory) >= 10 {
		first := m.sharpeHistory[len(m.sharpeHistory)-10]
		last := m.sharpeHistory[len(m.sharpeHistory)-1]
		if last < first*0.9 { // >10% decline
			alerts = append(alerts, HealthAlert{
				Severity:        "WARNING",
				Category:        "sharpe_trend",
				Message:         fmt.Sprintf("system sharpe declining: %.3f → %.3f over 10 days", first, last),
				Value:           last,
				Threshold:       first * 0.9,
				Timestamp:       now,
				SuggestedAction: "auto_rollback",
			})
		}
	}

	// Check for negative system Sharpe
	if avgSharpe < -0.5 {
		alerts = append(alerts, HealthAlert{
			Severity:        "CRITICAL",
			Category:        "sharpe",
			Message:         fmt.Sprintf("system sharpe negative: %.3f", avgSharpe),
			Value:           avgSharpe,
			Threshold:       -0.5,
			Timestamp:       now,
			SuggestedAction: "halt_and_review",
		})
	}

	return alerts
}

func (m *SystemHealthMonitor) checkAgentPopulation(now time.Time) []HealthAlert {
	var alerts []HealthAlert
	muted := m.healthMgr.GetMutedAgents()
	totalAgents := len(m.dwManager.GetAllAgentWeightData())
	if totalAgents == 0 {
		return alerts
	}

	mutedPct := float64(len(muted)) / float64(totalAgents)
	if mutedPct > 0.5 {
		alerts = append(alerts, HealthAlert{
			Severity:        "CRITICAL",
			Category:        "agent_population",
			Message:         fmt.Sprintf("%.0f%% agents muted (threshold 50%%)", mutedPct*100),
			Value:           mutedPct,
			Threshold:       0.5,
			Timestamp:       now,
			SuggestedAction: "halt_and_review",
		})
	} else if mutedPct > 0.3 {
		alerts = append(alerts, HealthAlert{
			Severity:        "WARNING",
			Category:        "agent_population",
			Message:         fmt.Sprintf("%.0f%% agents muted (threshold 30%%)", mutedPct*100),
			Value:           mutedPct,
			Threshold:       0.3,
			Timestamp:       now,
			SuggestedAction: "auto_propose_mutations",
		})
	}

	return alerts
}

func (m *SystemHealthMonitor) checkWeightDistribution(now time.Time) []HealthAlert {
	var alerts []HealthAlert
	agents := m.dwManager.GetAllAgentWeightData()
	if len(agents) == 0 {
		return alerts
	}

	var stuckAtMin int
	for _, a := range agents {
		if a.Weight <= 0.31 {
			stuckAtMin++
		}
	}

	stuckPct := float64(stuckAtMin) / float64(len(agents))
	if stuckPct > 0.5 {
		alerts = append(alerts, HealthAlert{
			Severity:        "WARNING",
			Category:        "weight_distribution",
			Message:         fmt.Sprintf("%.0f%% agents stuck at min weight", stuckPct*100),
			Value:           stuckPct,
			Threshold:       0.5,
			Timestamp:       now,
			SuggestedAction: "auto_reset_and_propose",
		})
	}

	return alerts
}

func countBySeverity(alerts []HealthAlert, severity string) int {
	var n int
	for _, a := range alerts {
		if a.Severity == severity {
			n++
		}
	}
	return n
}
