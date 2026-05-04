package experiment

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

func TestEvaluateUpdatesStatus(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "../.."))
	stateDir := t.TempDir()
	store := ledger.NewStore(stateDir)
	judge := NewJudge(store, filepath.Join(root, "samples", "replay", "twse_stock_day_all_sample.csv"), filepath.Join(stateDir, "baseline_policy.json"))
	resultPath := filepath.Join(stateDir, "experiments", "test-experiment.json")
	promptPath := filepath.Join(t.TempDir(), "v2.md")
	baselinePromptPath := filepath.Join(root, "prompts/agents/growth_momentum.md")
	windowPath := filepath.Join(stateDir, "windows", "window-test.json")

	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatalf("mkdir prompt dir: %v", err)
	}
	if err := os.WriteFile(promptPath, []byte("require trend confirmation\ndowngrade conviction\nreject setups\n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(resultPath), 0o755); err != nil {
		t.Fatalf("mkdir result dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(windowPath), 0o755); err != nil {
		t.Fatalf("mkdir window dir: %v", err)
	}

	window := domain.BacktestWindowSummary{
		WindowID:             "window-test",
		StartDate:            time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC),
		EndDate:              time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC),
		WorstAgentSharpeLike: -100,
	}
	windowBytes, err := json.Marshal(window)
	if err != nil {
		t.Fatalf("marshal window: %v", err)
	}
	if err := os.WriteFile(windowPath, windowBytes, 0o644); err != nil {
		t.Fatalf("write window: %v", err)
	}

	resultFixture := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			ID:               "test-experiment",
			TargetAgentID:    "growth-momentum-01",
			Skill:            "growth_momentum",
			MutationType:     "prompt_tightening",
			AcceptanceMetric: "sharpe_like",
			AcceptanceGates:  []string{"improve_sharpe_like", "no_material_drawdown_degradation", "no_constraint_bypass"},
			Status:           domain.ExperimentRunning,
		},
		Brief: domain.MutationBrief{
			WindowID:            "window-test",
			TargetAgentID:       "growth-momentum-01",
			TargetSkill:         "growth_momentum",
			TargetLayer:         domain.LayerStyle,
			PromptFile:          baselinePromptPath,
			MutationType:        "prompt_tightening",
			AcceptanceMetric:    "sharpe_like",
			AcceptanceGates:     []string{"improve_sharpe_like", "no_material_drawdown_degradation", "no_constraint_bypass"},
			ForbiddenActions:    []string{"illiquid_breakout_chasing"},
			RequiredSkills:      []string{"growth_momentum"},
			ObservedWindowCount: 2,
			MaturityLevel:       "level_1_exploratory",
		},
		CandidatePrompt: promptPath,
		EvaluationMode:  "policy_checked_pending_replay",
		PolicyChecks:    []string{"required skill preserved: growth_momentum"},
	}
	resultBytes, err := json.Marshal(resultFixture)
	if err != nil {
		t.Fatalf("marshal result fixture: %v", err)
	}
	if err := os.WriteFile(resultPath, resultBytes, 0o644); err != nil {
		t.Fatalf("write result fixture: %v", err)
	}

	result, err := judge.Evaluate(resultPath)
	if err != nil {
		t.Fatalf("judge evaluate: %v", err)
	}
	if result.Experiment.Status == "" {
		t.Fatalf("expected status")
	}
}

func TestPassesAcceptanceUsesMaturityThresholds(t *testing.T) {
	level1 := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			AcceptanceGates: []string{"improve_sharpe_like", "no_material_drawdown_degradation", "no_constraint_bypass"},
			BaselineValue:   0.0100,
			CandidateValue:  0.0105,
			MutationType:    "prompt_tightening",
		},
		Brief: domain.MutationBrief{
			MaturityLevel: "level_1_exploratory",
		},
		BaselineObservations:  6,
		CandidateObservations: 6,
		JudgeChecks:           []string{"a", "b"},
	}
	accepted, _ := testJudge().passesAcceptance(level1)
	if !accepted {
		t.Fatalf("expected level 1 experiment to accept modest improvement")
	}

	level3 := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			AcceptanceGates: []string{"improve_sharpe_like", "no_material_drawdown_degradation", "no_constraint_bypass"},
			BaselineValue:   0.0100,
			CandidateValue:  0.0102,
			MutationType:    "prompt_tightening",
		},
		Brief: domain.MutationBrief{
			MaturityLevel: "level_3_regime_aware",
		},
		BaselineObservations:  12,
		CandidateObservations: 12,
		JudgeChecks:           []string{"a", "b", "c", "d"},
	}
	accepted, note := testJudge().passesAcceptance(level3)
	if accepted {
		t.Fatalf("expected level 3 experiment to reject small improvement, got note %q", note)
	}
}

func TestPassesAcceptanceUsesMutationTypeProfiles(t *testing.T) {
	promptMutation := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			AcceptanceGates: []string{"improve_sharpe_like", "no_material_drawdown_degradation", "no_constraint_bypass"},
			BaselineValue:   0.0100,
			CandidateValue:  0.0120,
			MutationType:    "prompt_tightening",
		},
		Brief: domain.MutationBrief{
			MaturityLevel: "level_2_window_validated",
		},
		BaselineObservations:  8,
		CandidateObservations: 8,
		JudgeChecks:           []string{"a", "b", "c"},
	}
	accepted, _ := testJudge().passesAcceptance(promptMutation)
	if !accepted {
		t.Fatalf("expected prompt tightening to pass with sufficient level 2 improvement")
	}

	riskMutation := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			AcceptanceGates: []string{"improve_sharpe_like", "no_material_drawdown_degradation", "no_constraint_bypass"},
			BaselineValue:   0.0100,
			CandidateValue:  0.0105,
			MutationType:    "risk_rule_change",
		},
		Brief: domain.MutationBrief{
			MaturityLevel: "level_2_window_validated",
		},
		BaselineObservations:  9,
		CandidateObservations: 9,
		JudgeChecks:           []string{"a", "b", "c", "d"},
	}
	accepted, note := testJudge().passesAcceptance(riskMutation)
	if accepted {
		t.Fatalf("expected risk rule change to require a larger delta, got note %q", note)
	}
}

func TestPassesAcceptanceRejectsWhenObservationsInsufficient(t *testing.T) {
	result := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			AcceptanceGates: []string{"improve_sharpe_like", "no_material_drawdown_degradation", "no_constraint_bypass"},
			BaselineValue:   0.0100,
			CandidateValue:  0.0200,
			MutationType:    "risk_rule_change",
		},
		Brief: domain.MutationBrief{
			MaturityLevel: "level_2_window_validated",
		},
		BaselineObservations:  3,
		CandidateObservations: 3,
		JudgeChecks:           []string{"a", "b", "c", "d", "e"},
	}

	accepted, note := testJudge().passesAcceptance(result)
	if accepted {
		t.Fatalf("expected rejection for insufficient observations")
	}
	if note == "" {
		t.Fatalf("expected rejection note")
	}
}

func TestJudgeReplayChecksUsesRiskRuleArtifactStructure(t *testing.T) {
	result := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			MutationType: "risk_rule_change",
		},
		Brief: domain.MutationBrief{
			RequiredSkills:   []string{"cro_risk"},
			ForbiddenActions: []string{"risk_limit_override"},
		},
		PolicyChecks: []string{"required skill preserved: cro_risk"},
	}
	candidate := `# Risk Rule Change Proposal

## Candidate Rule Patch

conviction_floor: 55
liquidity_floor: 5000000

## Guardrails`

	checks := judgeReplayChecks(candidate, result)
	if len(checks) < 4 {
		t.Fatalf("expected structural risk-rule checks, got %v", checks)
	}
}

func TestJudgeReplayChecksUsesPortfolioArtifactStructure(t *testing.T) {
	result := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			MutationType: "portfolio_constraint_revision",
		},
		Brief: domain.MutationBrief{
			RequiredSkills:   []string{"cio_portfolio"},
			ForbiddenActions: []string{"cro_bypass"},
		},
		PolicyChecks: []string{"required skill preserved: cio_portfolio"},
	}
	candidate := `# Portfolio Constraint Revision Proposal

## Candidate Constraint Patch

max_position_weight: 0.15
reserve_cash_fraction: 0.12
require_cro_pass: true`

	checks := judgeReplayChecks(candidate, result)
	if len(checks) < 4 {
		t.Fatalf("expected structural portfolio checks, got %v", checks)
	}
}

func TestEvaluateRejectsMalformedResultContract(t *testing.T) {
	stateDir := t.TempDir()
	store := ledger.NewStore(stateDir)
	judge := NewJudge(store, "", "")
	resultPath := filepath.Join(stateDir, "experiments", "malformed.json")

	if err := os.MkdirAll(filepath.Dir(resultPath), 0o755); err != nil {
		t.Fatalf("mkdir result dir: %v", err)
	}

	resultFixture := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{ID: "exp-malformed"},
		Brief: domain.MutationBrief{
			WindowID:         "window-test",
			TargetAgentID:    "growth-momentum-01",
			TargetSkill:      "growth_momentum",
			TargetLayer:      domain.LayerStyle,
			PromptFile:       "prompts/agents/growth_momentum.md",
			MutationType:     "prompt_tightening",
			AcceptanceMetric: "sharpe_like",
			AcceptanceGates:  []string{"improve_sharpe_like"},
		},
		CandidatePrompt: "",
	}
	resultBytes, err := json.Marshal(resultFixture)
	if err != nil {
		t.Fatalf("marshal malformed fixture: %v", err)
	}
	if err := os.WriteFile(resultPath, resultBytes, 0o644); err != nil {
		t.Fatalf("write malformed fixture: %v", err)
	}

	if _, err := judge.Evaluate(resultPath); err == nil {
		t.Fatalf("expected malformed result contract to fail")
	}
}

func TestEvaluateRejectsInvalidStatusTransition(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "../.."))
	stateDir := t.TempDir()
	store := ledger.NewStore(stateDir)
	judge := NewJudge(store, filepath.Join(root, "samples", "replay", "twse_stock_day_all_sample.csv"), filepath.Join(stateDir, "baseline_policy.json"))
	resultPath := filepath.Join(stateDir, "experiments", "test-experiment-invalid-transition.json")
	promptPath := filepath.Join(t.TempDir(), "v2.md")
	baselinePromptPath := filepath.Join(root, "prompts/agents/growth_momentum.md")
	windowPath := filepath.Join(stateDir, "windows", "window-test.json")

	if err := os.MkdirAll(filepath.Dir(promptPath), 0o755); err != nil {
		t.Fatalf("mkdir prompt dir: %v", err)
	}
	if err := os.WriteFile(promptPath, []byte("require trend confirmation\ndowngrade conviction\nreject setups\n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(resultPath), 0o755); err != nil {
		t.Fatalf("mkdir result dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(windowPath), 0o755); err != nil {
		t.Fatalf("mkdir window dir: %v", err)
	}

	window := domain.BacktestWindowSummary{
		WindowID:             "window-test",
		StartDate:            time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC),
		EndDate:              time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC),
		WorstAgentSharpeLike: -100,
	}
	windowBytes, err := json.Marshal(window)
	if err != nil {
		t.Fatalf("marshal window: %v", err)
	}
	if err := os.WriteFile(windowPath, windowBytes, 0o644); err != nil {
		t.Fatalf("write window: %v", err)
	}

	resultFixture := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			ID:               "test-experiment-invalid-transition",
			TargetAgentID:    "growth-momentum-01",
			Skill:            "growth_momentum",
			MutationType:     "risk_rule_change",
			AcceptanceMetric: "sharpe_like",
			AcceptanceGates:  []string{"improve_sharpe_like", "no_material_drawdown_degradation", "no_constraint_bypass"},
			Status:           domain.ExperimentAccepted,
		},
		Brief: domain.MutationBrief{
			WindowID:            "window-test",
			TargetAgentID:       "growth-momentum-01",
			TargetSkill:         "growth_momentum",
			TargetLayer:         domain.LayerStyle,
			PromptFile:          baselinePromptPath,
			MutationType:        "risk_rule_change",
			AcceptanceMetric:    "sharpe_like",
			AcceptanceGates:     []string{"improve_sharpe_like", "no_material_drawdown_degradation", "no_constraint_bypass"},
			ForbiddenActions:    []string{"illiquid_breakout_chasing"},
			RequiredSkills:      []string{"growth_momentum"},
			ObservedWindowCount: 2,
			MaturityLevel:       "level_1_exploratory",
		},
		CandidatePrompt: promptPath,
		EvaluationMode:  "policy_checked_pending_replay",
		PolicyChecks:    []string{"required skill preserved: growth_momentum"},
	}
	resultBytes, err := json.Marshal(resultFixture)
	if err != nil {
		t.Fatalf("marshal result fixture: %v", err)
	}
	if err := os.WriteFile(resultPath, resultBytes, 0o644); err != nil {
		t.Fatalf("write result fixture: %v", err)
	}

	if _, err := judge.Evaluate(resultPath); err == nil {
		t.Fatalf("expected invalid status transition to fail")
	}
}

func TestPassesAcceptanceReportsNoConstraintDeltaWhenEqual(t *testing.T) {
	result := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			AcceptanceGates: []string{"improve_sharpe_like"},
			BaselineValue:   0.0075,
			CandidateValue:  0.0075,
			MutationType:    "risk_rule_change",
		},
		Brief: domain.MutationBrief{
			MaturityLevel: "level_2_window_validated",
		},
		BaselineObservations:  12,
		CandidateObservations: 12,
		JudgeChecks:           []string{"a", "b", "c"},
	}

	accepted, note := testJudge().passesAcceptance(result)
	if accepted {
		t.Fatalf("expected rejection when baseline == candidate")
	}
	if note != "rejected: candidate score equals baseline (no constraint delta applied)" {
		t.Fatalf("expected explicit no-delta note, got: %s", note)
	}
}

func TestWelchTTest(t *testing.T) {
	baseline := []float64{0.01, 0.02, 0.015, 0.01, 0.02, 0.01, 0.015, 0.01, 0.02, 0.01}
	candidate := []float64{0.02, 0.03, 0.025, 0.02, 0.03, 0.02, 0.025, 0.02, 0.03, 0.02}

	tStat, df := welchTTest(baseline, candidate)
	if tStat <= 0 {
		t.Fatalf("expected positive t-statistic for better candidate, got %.4f", tStat)
	}
	if df <= 0 {
		t.Fatalf("expected positive degrees of freedom, got %.4f", df)
	}
}

func TestWelchTTestInsufficientData(t *testing.T) {
	baseline := []float64{0.01}
	candidate := []float64{0.02}

	tStat, df := welchTTest(baseline, candidate)
	if tStat != 0 {
		t.Fatalf("expected t-statistic=0 for insufficient data, got %.4f", tStat)
	}
	if df != 0 {
		t.Fatalf("expected df=0 for insufficient data, got %.4f", df)
	}
}

func TestMeanAndVariance(t *testing.T) {
	data := []float64{1.0, 2.0, 3.0, 4.0, 5.0}
	mean, variance := meanAndVariance(data)

	if mean != 3.0 {
		t.Fatalf("expected mean=3.0, got %.4f", mean)
	}

	expectedVariance := 2.5
	if math.Abs(variance-expectedVariance) > 0.0001 {
		t.Fatalf("expected variance=%.4f, got %.4f", expectedVariance, variance)
	}
}
