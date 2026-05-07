package taskexec

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/experiment"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

type executeExperimentRunner struct {
	cfg config.Config
}

func NewExecuteExperimentRunner(cfg config.Config) Runner {
	return &executeExperimentRunner{cfg: cfg}
}

func (r *executeExperimentRunner) Name() string {
	return "execute-experiment"
}

func (r *executeExperimentRunner) Run(ctx context.Context, req SubmitRequest, sink EventSink) error {
	briefPath, _ := req.Payload["brief_path"].(string)
	if briefPath == "" {
		briefPath = "data/state/windows/window-20260326-20260327-mutation-brief.json"
	}

	sink.Emit(domain.TaskExecutionEvent{
		EventType: domain.TaskEventStatus,
		Stream:    "system",
		Message:   fmt.Sprintf("loading brief: %s", briefPath),
	})

	cfg := r.cfg
	if cfg.ReplayDataPath == "samples/replay/twse_stock_day_all_sample.csv" {
		cfg.ReplayDataPath = "data/replay/tw_extended_90days.csv"
	}

	if _, err := baseline.Load(cfg.BaselinePolicyPath); err != nil {
		return fmt.Errorf("load baseline policy: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	sink.Emit(domain.TaskExecutionEvent{
		EventType: domain.TaskEventProgress,
		Stream:    "system",
		Message:   "executing experiment",
	})

	executor := experiment.NewExecutor(ledger.NewStore(cfg.LedgerDir), cfg.BaselinePolicyPath)
	result, err := executor.Run(briefPath, cfg.ReplayDataPath)
	if err != nil {
		return fmt.Errorf("execute experiment: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	sink.Emit(domain.TaskExecutionEvent{
		EventType: domain.TaskEventSummary,
		Stream:    "system",
		Message:   fmt.Sprintf("experiment %s completed with status %s", result.Experiment.ID, result.Experiment.Status),
	})

	_ = sink.RecordLineage(domain.ExperimentLineageRecord{
		ExperimentID:     result.Experiment.ID,
		ExecutionID:      sink.ExecutionID(),
		RootExperimentID: result.Experiment.ID,
		TargetAgentID:    result.Experiment.TargetAgentID,
		TargetSkill:      result.Experiment.Skill,
		MutationType:     result.Experiment.MutationType,
		BriefPath:        briefPath,
		CandidatePath:    result.CandidatePrompt,
		Status:           string(result.Experiment.Status),
		RecordedAt:       time.Now(),
	})

	return nil
}

type judgeExperimentRunner struct {
	cfg config.Config
}

func NewJudgeExperimentRunner(cfg config.Config) Runner {
	return &judgeExperimentRunner{cfg: cfg}
}

func (r *judgeExperimentRunner) Name() string {
	return "judge-experiment"
}

func (r *judgeExperimentRunner) Run(ctx context.Context, req SubmitRequest, sink EventSink) error {
	resultPath, _ := req.Payload["result_path"].(string)
	if resultPath == "" {
		resultPath = findLatestExperiment("data/state/experiments")
		if resultPath == "" {
			resultPath = "data/state/experiments/exec-value-yield-01-1776084503.json"
		}
	}

	sink.Emit(domain.TaskExecutionEvent{
		EventType: domain.TaskEventStatus,
		Stream:    "system",
		Message:   fmt.Sprintf("evaluating experiment: %s", resultPath),
	})

	cfg := r.cfg
	if cfg.ReplayDataPath == "samples/replay/twse_stock_day_all_sample.csv" {
		cfg.ReplayDataPath = "data/replay/tw_extended_90days.csv"
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	judge := experiment.NewJudge(ledger.NewStore(cfg.LedgerDir), cfg.ReplayDataPath, cfg.BaselinePolicyPath)
	result, err := judge.Evaluate(resultPath)
	if err != nil {
		return fmt.Errorf("judge experiment: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	sink.Emit(domain.TaskExecutionEvent{
		EventType: domain.TaskEventSummary,
		Stream:    "system",
		Message:   fmt.Sprintf("experiment %s judged as %s (baseline: %.6f, candidate: %.6f)", result.Experiment.ID, result.Experiment.Status, result.Experiment.BaselineValue, result.Experiment.CandidateValue),
	})

	_ = sink.RecordLineage(domain.ExperimentLineageRecord{
		ExperimentID:     result.Experiment.ID,
		ExecutionID:      sink.ExecutionID(),
		RootExperimentID: result.Experiment.ID,
		TargetAgentID:    result.Experiment.TargetAgentID,
		TargetSkill:      result.Experiment.Skill,
		MutationType:     result.Experiment.MutationType,
		Status:           string(result.Experiment.Status),
		BaselineValue:    &result.Experiment.BaselineValue,
		CandidateValue:   &result.Experiment.CandidateValue,
		RecordedAt:       time.Now(),
		JudgedAt:         func() *time.Time { t := time.Now(); return &t }(),
	})

	return nil
}

type promoteBaselineRunner struct {
	cfg config.Config
}

func NewPromoteBaselineRunner(cfg config.Config) Runner {
	return &promoteBaselineRunner{cfg: cfg}
}

func (r *promoteBaselineRunner) Name() string {
	return "promote-baseline"
}

func (r *promoteBaselineRunner) Run(ctx context.Context, req SubmitRequest, sink EventSink) error {
	resultPath, _ := req.Payload["result_path"].(string)
	if resultPath == "" {
		resultPath = findLatestExperiment("data/state/experiments")
		if resultPath == "" {
			resultPath = "data/state/experiments/exec-value-yield-01-1776084503.json"
		}
	}

	sink.Emit(domain.TaskExecutionEvent{
		EventType: domain.TaskEventStatus,
		Stream:    "system",
		Message:   fmt.Sprintf("promoting baseline from: %s", resultPath),
	})

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	prevPolicy, _ := baseline.Load(r.cfg.BaselinePolicyPath)
	versionBefore := prevPolicy.Version

	manager := baseline.NewManager(r.cfg.BaselinePolicyPath)
	policy, err := manager.PromoteResult(resultPath)
	if err != nil {
		return fmt.Errorf("promote baseline: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	sink.Emit(domain.TaskExecutionEvent{
		EventType: domain.TaskEventSummary,
		Stream:    "system",
		Message:   fmt.Sprintf("baseline promoted to version %d with %d prompt overrides", policy.Version, len(policy.PromptOverrides)),
	})

	_ = sink.RecordBaselineHistory(domain.BaselineHistoryRecord{
		ExecutionID:   sink.ExecutionID(),
		VersionBefore: versionBefore,
		VersionAfter:  policy.Version,
		PromotedBy:    "web_ui",
		PromotedAt:    time.Now(),
		BaselinePath:  r.cfg.BaselinePolicyPath,
	})

	return nil
}

type backtestWindowRunner struct {
	cfg config.Config
}

func NewBacktestWindowRunner(cfg config.Config) Runner {
	return &backtestWindowRunner{cfg: cfg}
}

func (r *backtestWindowRunner) Name() string {
	return "backtest-window"
}

func (r *backtestWindowRunner) Run(ctx context.Context, req SubmitRequest, sink EventSink) error {
	startStr, _ := req.Payload["start_date"].(string)
	endStr, _ := req.Payload["end_date"].(string)
	if startStr == "" {
		startStr = "2026-03-26"
	}
	if endStr == "" {
		endStr = "2026-03-27"
	}

	startDate, err := time.Parse("2006-01-02", startStr)
	if err != nil {
		return fmt.Errorf("parse start date: %w", err)
	}
	endDate, err := time.Parse("2006-01-02", endStr)
	if err != nil {
		return fmt.Errorf("parse end date: %w", err)
	}

	sink.Emit(domain.TaskExecutionEvent{
		EventType: domain.TaskEventStatus,
		Stream:    "system",
		Message:   fmt.Sprintf("running backtest for %s to %s", startStr, endStr),
	})

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	runner := backtest.NewRunner(r.cfg)
	summary, err := runner.Run(startDate, endDate)
	if err != nil {
		return fmt.Errorf("run backtest window: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if err := runner.GenerateReport(summary); err != nil {
		sink.Emit(domain.TaskExecutionEvent{
			EventType: domain.TaskEventStderr,
			Stream:    "system",
			Message:   fmt.Sprintf("failed to generate report: %v", err),
		})
	}

	sink.Emit(domain.TaskExecutionEvent{
		EventType: domain.TaskEventSummary,
		Stream:    "system",
		Message:   fmt.Sprintf("backtest completed: %d sessions, %d outcomes", summary.SessionCount, summary.OutcomeCount),
	})

	if summary.WorstAgentID != "" {
		_ = sink.RecordMetrics([]domain.MetricTrendPoint{
			{
				ExecutionID: sink.ExecutionID(),
				SeriesKey:   "backtest_window",
				MetricName:  "worst_agent_sharpe",
				MetricScope: summary.WorstAgentID,
				MetricValue: summary.WorstAgentSharpeLike,
				SampledAt:   time.Now(),
			},
			{
				ExecutionID: sink.ExecutionID(),
				SeriesKey:   "backtest_window",
				MetricName:  "session_count",
				MetricScope: summary.WindowID,
				MetricValue: float64(summary.SessionCount),
				SampledAt:   time.Now(),
			},
			{
				ExecutionID: sink.ExecutionID(),
				SeriesKey:   "backtest_window",
				MetricName:  "outcome_count",
				MetricScope: summary.WindowID,
				MetricValue: float64(summary.OutcomeCount),
				SampledAt:   time.Now(),
			},
		})
	}

	return nil
}

func findLatestExperiment(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) == ".json" && name != "test-experiment.json" {
			files = append(files, name)
		}
	}
	if len(files) == 0 {
		return ""
	}
	sort.Slice(files, func(i, j int) bool {
		return extractTimestamp(files[i]) > extractTimestamp(files[j])
	})
	return filepath.Join(dir, files[0])
}

func extractTimestamp(filename string) int64 {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	parts := strings.Split(base, "-")
	if len(parts) > 0 {
		if ts, err := strconv.ParseInt(parts[len(parts)-1], 10, 64); err == nil {
			return ts
		}
	}
	return 0
}
