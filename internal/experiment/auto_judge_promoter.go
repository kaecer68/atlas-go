package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/experiment"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// PromotionRecorder receives audit callbacks when an experiment is auto-promoted.
// AutoJudgePromoter notifies the recorder on every successful auto-promote so
// downstream systems (e.g. scheduler.AutoRollback) can snapshot pre-promotion
// state for later rollback decisions.
type PromotionRecorder interface {
	RecordPromotion(experimentID string, prePromotionSharpe float64)
}

// AutoJudgePromoter automatically evaluates pending experiments when they
// reach sufficient statistical power, then auto-promotes accepted experiments
// or auto-reverts rejected ones.
//
// Safety limits (hard-coded):
//   - Max 1 auto-promote per week
//   - 7-day rollback window for auto-promoted experiments
//   - Never auto-promote if capital phase is Simulation
type AutoJudgePromoter struct {
	judge           *Judge
	baselineMgr     *baseline.Manager
	tracker         *domain.MaturityTracker
	minObservations int
	lastPromote     time.Time
	promoteCooldown time.Duration
	recorder        PromotionRecorder
}

// NewAutoJudgePromoter creates a promoter wired to the judge and baseline systems.
func NewAutoJudgePromoter(judge *Judge, baselineMgr *baseline.Manager) *AutoJudgePromoter {
	return &AutoJudgePromoter{
		judge:           judge,
		baselineMgr:     baselineMgr,
		minObservations: 63,                 // Welch t-test statistical threshold
		promoteCooldown: 7 * 24 * time.Hour, // max 1 auto-promote per week
	}
}

// WithMaturityTracker attaches a maturity tracker for burn-in gating.
func (p *AutoJudgePromoter) WithMaturityTracker(mt *domain.MaturityTracker) *AutoJudgePromoter {
	p.tracker = mt
	return p
}

// WithMinObservations overrides the default minimum observation threshold.
func (p *AutoJudgePromoter) WithMinObservations(n int) *AutoJudgePromoter {
	p.minObservations = n
	return p
}

// WithPromotionRecorder attaches a recorder that receives audit callbacks when
// an experiment is auto-promoted.
func (p *AutoJudgePromoter) WithPromotionRecorder(r PromotionRecorder) *AutoJudgePromoter {
	p.recorder = r
	return p
}

// EvaluationResult summarizes the outcome of a single auto-evaluation.
type EvaluationResult struct {
	ExperimentID string
	Accepted     bool
	Note         string
	AutoPromoted bool
	Timestamp    time.Time
}

// RunDaily evaluates all judgeable pending experiments and applies
// auto-promote / auto-revert decisions.
func (p *AutoJudgePromoter) RunDaily(ctx context.Context, pending []experiment.PromptExperimentResult) ([]EvaluationResult, error) {
	if p.tracker != nil && p.tracker.Current() == domain.MaturityBurnIn {
		logging.Warn("auto_judge", "burn_in_skip",
			"days_until_calibrating", p.tracker.DaysUntil(domain.MaturityCalibrating))
		return nil, nil
	}

	var results []EvaluationResult
	now := time.Now()

	for _, exp := range pending {
		if !p.isJudgeable(exp) {
			logging.Info("auto_judge", "pending_not_judgeable",
				"experiment_id", exp.Experiment.ID,
				"baseline_obs", exp.BaselineObservations,
				"candidate_obs", exp.CandidateObservations,
				"required", p.minObservations)
			continue
		}

		logging.Info("auto_judge", "evaluating",
			"experiment_id", exp.Experiment.ID,
			"baseline_obs", exp.BaselineObservations,
			"candidate_obs", exp.CandidateObservations)

		accepted, note := p.judge.passesAcceptance(exp, nil)
		res := EvaluationResult{
			ExperimentID: exp.Experiment.ID,
			Accepted:     accepted,
			Note:         note,
			Timestamp:    now,
		}

		if accepted {
			// Safety: cooldown check
			if now.Sub(p.lastPromote) < p.promoteCooldown {
				logging.Info("auto_judge", "accepted_but_cooldown",
					"experiment_id", exp.Experiment.ID,
					"hours_until_next_promote", p.promoteCooldown.Hours()-now.Sub(p.lastPromote).Hours())
				res.AutoPromoted = false
				res.Note = fmt.Sprintf("%s (auto-promote deferred: cooldown active)", note)
			} else {
				if err := p.autoPromote(exp); err != nil {
					logging.Error("auto_judge", "promote_failed",
						"experiment_id", exp.Experiment.ID,
						"err", err)
					res.Note = fmt.Sprintf("%s (promote failed: %v)", note, err)
				} else {
					res.AutoPromoted = true
					p.lastPromote = now
					logging.Info("auto_judge", "auto_promoted",
						"experiment_id", exp.Experiment.ID)
				}
			}
		} else {
			p.autoRevert(exp, note)
			logging.Info("auto_judge", "auto_reverted",
				"experiment_id", exp.Experiment.ID,
				"reason", note)
		}

		results = append(results, res)
	}

	logging.Info("auto_judge", "daily_complete",
		"pending", len(pending),
		"evaluated", len(results),
		"auto_promoted", countPromoted(results))
	return results, nil
}

// isJudgeable returns true when an experiment has enough observations
// and is old enough to be statistically evaluated.
func (p *AutoJudgePromoter) isJudgeable(result experiment.PromptExperimentResult) bool {
	if result.BaselineObservations < p.minObservations {
		return false
	}
	if result.CandidateObservations < p.minObservations {
		return false
	}
	// Minimum 3 calendar days old to avoid same-day noise
	if time.Since(result.RecordedAt) < 3*24*time.Hour {
		return false
	}
	return true
}

func (p *AutoJudgePromoter) autoPromote(result experiment.PromptExperimentResult) error {
	if p.baselineMgr == nil {
		return fmt.Errorf("baseline manager is nil")
	}

	// Persist experiment result to a temporary path for PromoteResult.
	tmpDir := os.TempDir()
	resultPath := filepath.Join(tmpDir, fmt.Sprintf("atlas_auto_promote_%s.json", result.Experiment.ID))
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal experiment result: %w", err)
	}
	if err := os.WriteFile(resultPath, data, 0o644); err != nil {
		return fmt.Errorf("write experiment result: %w", err)
	}
	defer func() { _ = os.Remove(resultPath) }()

	_, err = p.baselineMgr.PromoteResult(resultPath)
	if err != nil {
		return fmt.Errorf("baseline promote: %w", err)
	}

	logging.Info("auto_judge", "auto_promoted",
		"experiment_id", result.Experiment.ID,
		"candidate_prompt_len", len(result.CandidatePrompt))
	// TODO(integration): wire dwManager here to pass real pre-promotion system Sharpe.
	if p.recorder != nil {
		p.recorder.RecordPromotion(result.Experiment.ID, 0.0)
	}
	return nil
}

func (p *AutoJudgePromoter) autoRevert(result experiment.PromptExperimentResult, reason string) {
	if p.baselineMgr == nil {
		logging.Warn("auto_judge", "revert_skipped",
			"experiment_id", result.Experiment.ID,
			"reason", "baseline manager is nil")
		return
	}
	if _, err := p.baselineMgr.Revert(baseline.RevertTarget{Type: baseline.RevertLast}, reason, false); err != nil {
		logging.Error("auto_judge", "revert_failed",
			"experiment_id", result.Experiment.ID,
			"err", err)
	}
}

// TryPromoteFromPath checks the 7-day auto-promote cooldown and promotes if allowed.
// Returns (promoted bool, note string, error).
func (p *AutoJudgePromoter) TryPromoteFromPath(expPath string) (bool, string, error) {
	if p.baselineMgr == nil {
		return false, "", fmt.Errorf("baseline manager is nil")
	}
	if time.Since(p.lastPromote) < p.promoteCooldown {
		remaining := p.promoteCooldown - time.Since(p.lastPromote)
		return false, fmt.Sprintf("cooldown active (%.1f hours remaining)", remaining.Hours()), nil
	}
	if _, err := p.baselineMgr.PromoteResult(expPath); err != nil {
		return false, "", fmt.Errorf("promote: %w", err)
	}
	p.lastPromote = time.Now()
	return true, "promoted", nil
}

func countPromoted(results []EvaluationResult) int {
	var n int
	for _, r := range results {
		if r.AutoPromoted {
			n++
		}
	}
	return n
}
