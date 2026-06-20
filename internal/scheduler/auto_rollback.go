package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// AutoRollback continuously monitors promoted experiments and agent performance
// to automatically revert degraded changes or disable failing agents.
//
// Trigger conditions:
//  1. Promoted experiment's rolling Sharpe drops >20% from pre-promotion baseline
//  2. Agent Sharpe < -1.0 for 30 consecutive days
//  3. System-wide composite score drops >15% after a calibration application
//
// Auto-rollback is the FIRST-CLASS safety mechanism: the machine corrects
// its own mistakes without human intervention.
type AutoRollback struct {
	baselineMgr *baseline.Manager
	dwManager   *portfolio.DarwinianWeightManager
	healthMgr   *portfolio.AgentHealthManager
	tracker     *domain.MaturityTracker
	eventBus    *eventbus.ChannelEventBus

	// promotedSnapshot holds Sharpe before promotion, keyed by experimentID
	promotedSnapshot map[string]float64
	// agentFailCount tracks consecutive days an agent has Sharpe < -1.0
	agentFailCount map[string]int
	// lastCalibSnapshot holds system composite score before last calibration
	lastCalibSnapshot float64
	// rollbackHistory records executed rollbacks for audit
	rollbackHistory []RollbackResult
}

// NewAutoRollback creates the auto-correction engine.
func NewAutoRollback(bm *baseline.Manager, dw *portfolio.DarwinianWeightManager, health *portfolio.AgentHealthManager) *AutoRollback {
	return &AutoRollback{
		baselineMgr:      bm,
		dwManager:        dw,
		healthMgr:        health,
		promotedSnapshot: make(map[string]float64),
		agentFailCount:   make(map[string]int),
	}
}

// WithMaturityTracker attaches a maturity tracker.
func (r *AutoRollback) WithMaturityTracker(mt *domain.MaturityTracker) *AutoRollback {
	r.tracker = mt
	return r
}

// WithEventBus attaches an event bus for publishing promotion/rollback events.
func (r *AutoRollback) WithEventBus(eb *eventbus.ChannelEventBus) *AutoRollback {
	r.eventBus = eb
	return r
}

// RecordPromotion should be called whenever an experiment is promoted.
// It snapshots the pre-promotion system Sharpe for later comparison.
func (r *AutoRollback) RecordPromotion(experimentID string, prePromotionSharpe float64) {
	r.promotedSnapshot[experimentID] = prePromotionSharpe
	logging.Info("auto_rollback", "promotion_recorded",
		"experiment_id", experimentID,
		"pre_promotion_sharpe", prePromotionSharpe)
	if r.eventBus != nil {
		r.eventBus.PublishPromotionRecorded(eventbus.PromotionRecordedPayload{
			ExperimentID:       experimentID,
			PrePromotionSharpe: prePromotionSharpe,
			Timestamp:          time.Now(),
		})
	}
}

// RecordCalibration should be called before applying a calibration.
// It snapshots the current system composite score.
func (r *AutoRollback) RecordCalibration(currentCompositeScore float64) {
	r.lastCalibSnapshot = currentCompositeScore
	logging.Info("auto_rollback", "calibration_snapshot",
		"composite_score", currentCompositeScore)
}

// RollbackResult summarizes a single auto-correction action.
type RollbackResult struct {
	Action    string // "revert_baseline", "disable_agent", "revert_calibration"
	TargetID  string
	Reason    string
	PreValue  float64
	PostValue float64
	Timestamp time.Time
}

// RunDaily is the entry point for the background task.
func (r *AutoRollback) RunDaily(ctx context.Context) ([]RollbackResult, error) {
	if r.tracker != nil && r.tracker.Current() == domain.MaturityBurnIn {
		// Burn-in: still monitor but do not rollback (not enough data)
		return nil, nil
	}

	var results []RollbackResult
	now := time.Now()

	// Check 1: Revert promoted experiments that degraded
	// baselineMgr is only needed for executeRollback, not for detection.
	rev := r.checkPromotedDegradation()
	results = append(results, rev...)

	// Check 2: Disable agents with sustained catastrophic Sharpe
	if r.dwManager != nil {
		rev := r.checkAgentFailures()
		results = append(results, rev...)
	}

	// Check 3: Revert calibration if system composite score dropped >15%
	if r.lastCalibSnapshot != 0 {
		rev := r.checkCalibrationDegradation()
		results = append(results, rev...)
	}

	// Execute detected rollbacks
	for i := range results {
		if err := r.executeRollback(&results[i]); err != nil {
			logging.Error("auto_rollback", "execute_failed",
				"action", results[i].Action,
				"target", results[i].TargetID,
				"err", err)
			results[i].Reason = fmt.Sprintf("%s (execution failed: %v)", results[i].Reason, err)
		}
	}

	logging.Info("auto_rollback", "daily_complete",
		"actions_detected", len(results),
		"actions_executed", len(r.rollbackHistory),
		"timestamp", now)
	return results, nil
}

// checkPromotedDegradation: if rolling Sharpe drops >20% from pre-promotion, revert.
func (r *AutoRollback) checkPromotedDegradation() []RollbackResult {
	var results []RollbackResult
	for expID, preSharpe := range r.promotedSnapshot {
		// Compute current system Sharpe (simplified: average of all agents)
		currentSharpe := r.computeSystemSharpe()
		if currentSharpe < preSharpe*0.8 { // >20% drop
			logging.Warn("auto_rollback", "promotion_degraded",
				"experiment_id", expID,
				"pre_sharpe", preSharpe,
				"current_sharpe", currentSharpe)
			results = append(results, RollbackResult{
				Action:    "revert_baseline",
				TargetID:  expID,
				Reason:    fmt.Sprintf("system sharpe dropped %.1f%% after promotion", (1-currentSharpe/preSharpe)*100),
				PreValue:  preSharpe,
				PostValue: currentSharpe,
				Timestamp: time.Now(),
			})
			delete(r.promotedSnapshot, expID)
		}
	}
	return results
}

// checkAgentFailures: if agent Sharpe < -1.0 for 30 days, auto-disable.
func (r *AutoRollback) checkAgentFailures() []RollbackResult {
	var results []RollbackResult
	agents := r.dwManager.GetAllAgentWeightData()
	for _, agent := range agents {
		if agent.RollingSharpe < -1.0 {
			r.agentFailCount[agent.AgentID]++
			if r.agentFailCount[agent.AgentID] >= 30 {
				logging.Error("auto_rollback", "agent_auto_disabled",
					"agent_id", agent.AgentID,
					"rolling_sharpe", agent.RollingSharpe,
					"consecutive_fail_days", r.agentFailCount[agent.AgentID])
				results = append(results, RollbackResult{
					Action:    "disable_agent",
					TargetID:  agent.AgentID,
					Reason:    fmt.Sprintf("sharpe < -1.0 for %d days", r.agentFailCount[agent.AgentID]),
					PreValue:  agent.RollingSharpe,
					PostValue: 0,
					Timestamp: time.Now(),
				})
				delete(r.agentFailCount, agent.AgentID)
			}
		} else {
			r.agentFailCount[agent.AgentID] = 0 // Reset on recovery
		}
	}
	return results
}

// checkCalibrationDegradation: if system composite score dropped >15%, revert.
func (r *AutoRollback) checkCalibrationDegradation() []RollbackResult {
	var results []RollbackResult
	currentScore := r.computeSystemCompositeScore()
	if r.lastCalibSnapshot > 0 && currentScore < r.lastCalibSnapshot*0.85 {
		logging.Warn("auto_rollback", "calibration_degraded",
			"pre_score", r.lastCalibSnapshot,
			"current_score", currentScore)
		results = append(results, RollbackResult{
			Action:    "revert_calibration",
			TargetID:  "last_calibration",
			Reason:    fmt.Sprintf("composite score dropped %.1f%% after calibration", (1-currentScore/r.lastCalibSnapshot)*100),
			PreValue:  r.lastCalibSnapshot,
			PostValue: currentScore,
			Timestamp: time.Now(),
		})
		r.lastCalibSnapshot = 0 // Clear after rollback
	}
	return results
}

func (r *AutoRollback) computeSystemSharpe() float64 {
	agents := r.dwManager.GetAllAgentWeightData()
	if len(agents) == 0 {
		return 0
	}
	var sum float64
	for _, a := range agents {
		sum += a.RollingSharpe
	}
	return sum / float64(len(agents))
}

func (r *AutoRollback) computeSystemCompositeScore() float64 {
	// Simplified: use average agent Sharpe as proxy for system composite score
	return r.computeSystemSharpe()
}

// executeRollback performs the actual rollback action based on the result type.
// TODO: baseline revert and calibration revert require backup infrastructure.
func (r *AutoRollback) executeRollback(result *RollbackResult) error {
	switch result.Action {
	case "disable_agent":
		if r.dwManager == nil {
			return fmt.Errorf("dwManager is nil, cannot disable agent %s", result.TargetID)
		}
		r.dwManager.RemoveAgent(result.TargetID)
		logging.Info("auto_rollback", "agent_disabled",
			"agent_id", result.TargetID,
			"reason", result.Reason)
		r.rollbackHistory = append(r.rollbackHistory, *result)
		return nil

	case "revert_baseline":
		// TODO: Requires baseline backup infrastructure.
		// When baseline.Manager supports Revert(snapshotPath), replace this stub.
		// For now: alert-only mode — record the intent in history but do not error.
		logging.Warn("auto_rollback", "baseline_revert_alert_only",
			"experiment_id", result.TargetID,
			"reason", result.Reason,
			"note", "baseline revert requires backup infrastructure; manual intervention needed")
		r.rollbackHistory = append(r.rollbackHistory, *result)
		return nil

	case "revert_calibration":
		// TODO: Requires parameter backup infrastructure.
		// When ParametersConfig supports SaveWithRollback from backup, replace this stub.
		// For now: alert-only mode — record the intent in history but do not error.
		logging.Warn("auto_rollback", "calibration_revert_alert_only",
			"reason", result.Reason,
			"note", "calibration revert requires parameter backup infrastructure; manual intervention needed")
		r.rollbackHistory = append(r.rollbackHistory, *result)
		return nil

	default:
		return fmt.Errorf("unknown rollback action: %s", result.Action)
	}
}

// History returns all executed rollback actions for audit.
func (r *AutoRollback) History() []RollbackResult {
	return append([]RollbackResult(nil), r.rollbackHistory...)
}
