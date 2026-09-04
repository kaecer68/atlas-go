package main

// cf_hypothesis_validation — scheduled auto-rerun of the Phase 1
// pre-registered capital-flow hypothesis validator
// (cmd/validate-capital-flow-hypotheses; spec
// docs/specs/capital-flow-seven-dimension-spec.md §10).
//
// Why: H-CF-01/02/05 first-round verdicts came back INSUFFICIENT_DATA
// (samples below the pre-registered 252-trading-day threshold) while
// historical backfill is still landing. Instead of relying on a human
// remembering to rerun the offline CLI, the BTM scheduler replays the
// exact same read-only procedure daily (in-process via
// capitalflow.RunHypothesisValidation — no exec, no network, no FinMind
// quota: every input is a local snapshot file under the workdir).
//
// Cadence: 1h tick, gated to the 08:30–09:59 Taipei window (after the
// 00:00 FinMind quota reset / overnight backfill, before market open),
// once per day, weekdays only — trading data does not change on
// weekends, so weekend ticks are skipped to avoid no-op reports.
//
// Governance boundaries (spec §10; plan
// .omo/plans/2026-09-04-capital-flow-model-plan.md §3):
//
//   - The automated rerun only handles "data unlock" re-judgements:
//     INSUFFICIENT_DATA → a real PASS/PASS(improved)/FAIL verdict once
//     samples cross the 252-day threshold. It writes reports and logs
//     NOTHING else.
//   - Pre-registered governance one-shots — e.g. the H-CF-01
//     v2a/v2a′/v2b same-batch Holm correction — are manual-only
//     executions; this task must never be their vehicle.
//   - The CLI/task NEVER writes state or config, and automation
//     eligibility NEVER auto-flips: eligible_recommendation=true is a
//     suggestion for a human config PR (CF-INV-13).
//
// Verdict-change handling: the run is diffed against the latest
// previous report (capitalflow.DetectVerdictChanges). No change → the
// task stays quiet (one info log line). A change raises
// cf_verdict_changed — info for a data unlock, warning for a
// judged→judged flip — through the standard monitor alert channel
// (same notifiers as every other alert, e.g. the Telegram webhook),
// plus an INFO log line per change.

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// cfHypothesisAlertFunc is the notification seam of the task body:
// monitor alerts in production, a captured sink in tests.
type cfHypothesisAlertFunc func(level monitoring.AlertLevel, message string, meta map[string]any)

// cfHypothesisGate decides whether the task body should do real work at
// tick `now` (already in Asia/Taipei). Runs once per weekday inside the
// 08:30–09:59 window; lastRunDate ("2006-01-02") is the daily-once
// guard, set only after a fully successful run. Returns a skip reason
// for logging.
func cfHypothesisGate(now time.Time, lastRunDate string) (bool, string) {
	if wd := now.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return false, "weekend"
	}
	if lastRunDate == now.Format("2006-01-02") {
		return false, "already_ran_today"
	}
	h, m := now.Hour(), now.Minute()
	if (h == 8 && m >= 30) || h == 9 {
		return true, ""
	}
	return false, "outside_window"
}

// runCFHypothesisValidationOnce executes one full read-only validation
// cycle: rerun the pre-registered validators, persist
// data/reports/cf-hypotheses-<today>.{json,md}, then diff against the
// latest previous report and emit one log line + alert per verdict
// change. workdir is the repo workdir holding data/; reportsDir is
// where report files live (split out so tests can point them at a
// tempdir).
func runCFHypothesisValidationOnce(ctx context.Context, workdir, reportsDir string, alert cfHypothesisAlertFunc) ([]capitalflow.VerdictChange, error) {
	report, err := capitalflow.RunHypothesisValidation(ctx, capitalflow.ValidationInputs{WorkDir: workdir})
	if err != nil {
		return nil, fmt.Errorf("cf hypothesis validation run: %w", err)
	}
	today := time.Now().In(taipeiLocation()).Format("2006-01-02")
	jsonPath := filepath.Join(reportsDir, fmt.Sprintf("cf-hypotheses-%s.json", today))
	mdPath := filepath.Join(reportsDir, fmt.Sprintf("cf-hypotheses-%s.md", today))
	if err := capitalflow.WriteValidationReportJSON(jsonPath, report); err != nil {
		return nil, fmt.Errorf("write cf hypotheses JSON report: %w", err)
	}
	if err := capitalflow.WriteValidationReportMarkdown(mdPath, report); err != nil {
		return nil, fmt.Errorf("write cf hypotheses markdown report: %w", err)
	}

	// Diff against the most recent report written BEFORE today so the
	// comparison is always "previous run vs this run". A missing or
	// unreadable previous report skips the diff (first-run quiet), it
	// does not fail the run — today's report is already persisted.
	var changes []capitalflow.VerdictChange
	prev, prevPath, found, diffErr := capitalflow.FindLatestValidationReport(reportsDir, today)
	if diffErr != nil {
		log.Printf("[CapitalTasks] cf_hypothesis_validation: previous report unreadable, skipping diff: %v", diffErr)
	} else if found {
		changes = capitalflow.DetectVerdictChanges(prev.Hypotheses, report.Hypotheses)
	}

	for _, c := range changes {
		logging.Info("capital_tasks", "cf_verdict_changed",
			"hypothesis", c.HypothesisID,
			"from", c.FromStatus,
			"to", c.ToStatus,
			"kind", string(c.Kind),
			"sample_count", c.SampleCount,
			"prev_report", prevPath)
		if alert != nil {
			level := monitoring.AlertLevelInfo
			if c.Kind == capitalflow.VerdictChangeFlip {
				level = monitoring.AlertLevelWarning
			}
			alert(level, "cf_verdict_changed: "+c.String(), map[string]any{
				"hypothesis":   c.HypothesisID,
				"from_status":  c.FromStatus,
				"to_status":    c.ToStatus,
				"change_kind":  string(c.Kind),
				"sample_count": c.SampleCount,
			})
		}
	}
	logging.Info("capital_tasks", "cf_hypothesis_validation_completed",
		"changes", len(changes), "report", jsonPath,
		"eligible_recommendation", report.EligibleRecommendation)
	return changes, nil
}

// registerCFHypothesisValidationTask wires the daily auto-rerun into
// the BackgroundTaskManager. TimeGated like daily_report_generate: the
// 1h tick mostly returns ErrTaskSkipped, so liveness is judged by time
// since last successful run, not by tick cadence.
func registerCFHypothesisValidationTask(d capitalDeps) {
	workdir := d.cfg.WorkDir
	reportsDir := filepath.Join(workdir, "data", "reports")
	var alert cfHypothesisAlertFunc
	if d.monitor != nil {
		mon := d.monitor
		alert = func(level monitoring.AlertLevel, message string, meta map[string]any) {
			mon.Alert(level, "capitalflow", message, meta)
		}
	}
	var lastRunDate string
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "cf_hypothesis_validation",
		Interval:  1 * time.Hour,
		TimeGated: true,
		Enabled:   true,
		Task: func(ctx context.Context) error {
			now := time.Now().In(taipeiLocation())
			ok, reason := cfHypothesisGate(now, lastRunDate)
			if !ok {
				logging.Info("capital_tasks", "cf_hypothesis_validation_skipped", "reason", reason)
				return apigateway.ErrTaskSkipped
			}
			if _, err := runCFHypothesisValidationOnce(ctx, workdir, reportsDir, alert); err != nil {
				return err
			}
			lastRunDate = now.Format("2006-01-02")
			return nil
		},
	})
	log.Printf("[Gateway] registered cf_hypothesis_validation background task (daily auto-rerun, 08:30-09:59 Taipei window, read-only, offline)")
}
