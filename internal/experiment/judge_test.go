package experiment

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eval"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

func TestEvaluateUpdatesStatus(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "../.."))
	stateDir := t.TempDir()
	store := ledger.NewStore(stateDir).(ledger.ExperimentStore)
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
	accepted, _ := testJudge().passesAcceptance(level1, nil)
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
	accepted, note := testJudge().passesAcceptance(level3, nil)
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
	accepted, _ := testJudge().passesAcceptance(promptMutation, nil)
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
	accepted, note := testJudge().passesAcceptance(riskMutation, nil)
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

	accepted, note := testJudge().passesAcceptance(result, nil)
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
	store := ledger.NewStore(stateDir).(ledger.ExperimentStore)
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
	store := ledger.NewStore(stateDir).(ledger.ExperimentStore)
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

	accepted, note := testJudge().passesAcceptance(result, nil)
	if accepted {
		t.Fatalf("expected rejection when baseline == candidate")
	}
	if note != "rejected: candidate score equals baseline (no constraint delta applied)" {
		t.Fatalf("expected explicit no-delta note, got: %s", note)
	}
}

func TestWelchTTest(t *testing.T) {
	// 63 samples required per group for statistical significance
	baseline := make([]float64, 63)
	candidate := make([]float64, 63)
	for i := range 63 {
		baseline[i] = []float64{0.01, 0.02, 0.015, 0.01, 0.02}[i%5]
		candidate[i] = []float64{0.02, 0.03, 0.025, 0.02, 0.03}[i%5]
	}

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

func TestCalculateVolatility(t *testing.T) {
	if v := calculateVolatility(nil); v != 0 {
		t.Errorf("nil returns: got %f, want 0", v)
	}
	if v := calculateVolatility([]float64{0.01}); v != 0 {
		t.Errorf("single return: got %f, want 0", v)
	}
	// 30 samples required for volatility calculation
	returns := make([]float64, 30)
	for i := range returns {
		returns[i] = []float64{0.01, -0.02, 0.03, -0.01, 0.005}[i%5]
	}
	v := calculateVolatility(returns)
	if v <= 0 {
		t.Errorf("expected positive volatility, got %f", v)
	}
}

func TestJudge_WithEventBus(t *testing.T) {
	dir := t.TempDir()
	store := ledger.NewStore(dir).(ledger.ExperimentStore)
	j := NewJudge(store, dir, dir)
	result := j.WithEventBus(nil)
	if result == nil {
		t.Fatal("WithEventBus returned nil")
	}
}

func TestJudge_WithParameters(t *testing.T) {
	dir := t.TempDir()
	store := ledger.NewStore(dir).(ledger.ExperimentStore)
	j := NewJudge(store, dir, dir)
	result := j.WithParameters(nil)
	if result == nil {
		t.Fatal("WithParameters returned nil")
	}
}

func TestEvaluatePopulatesMonetaryFields(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "../.."))
	stateDir := t.TempDir()
	store := ledger.NewStore(stateDir).(ledger.ExperimentStore)
	replayPath := filepath.Join(root, "samples", "replay", "twse_stock_day_all_sample.csv")
	baselinePolicyPath := filepath.Join(stateDir, "baseline_policy.json")
	startingCash := 1000000.0
	baselinePolicy := `{"version":1,"constraints":{"starting_cash":` + fmt.Sprintf("%.0f", startingCash) + `,"max_position_weight":0.25,"max_open_positions":10,"min_tradable_volume":1000,"min_recommendation_conviction":0,"require_cro_pass":false,"transaction_cost_bps":1,"slippage_bps":5,"reserve_cash_fraction":0.1},"execution_policy":{"conviction_floor":0,"require_cro_pass":false,"momentum_crash_protection":false}}`
	if err := os.WriteFile(baselinePolicyPath, []byte(baselinePolicy), 0o644); err != nil {
		t.Fatalf("write baseline policy: %v", err)
	}
	judge := NewJudge(store, replayPath, baselinePolicyPath)
	resultPath := filepath.Join(stateDir, "experiments", "test-monetary.json")
	promptPath := filepath.Join(t.TempDir(), "v2.md")
	baselinePromptPath := filepath.Join(root, "prompts/agents/growth_momentum.md")
	windowPath := filepath.Join(stateDir, "windows", "window-monetary.json")

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
		WindowID:             "window-monetary",
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
			ID:               "test-monetary",
			TargetAgentID:    "growth-momentum-01",
			Skill:            "growth_momentum",
			MutationType:     "prompt_tightening",
			AcceptanceMetric: "sharpe_like",
			AcceptanceGates:  []string{"improve_sharpe_like", "no_material_drawdown_degradation", "no_constraint_bypass"},
			Status:           domain.ExperimentRunning,
		},
		Brief: domain.MutationBrief{
			WindowID:            "window-monetary",
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

	if result.BaselineMonetaryNTD == 0 && result.Experiment.BaselineMonetaryNTD == 0 {
		if result.Experiment.BaselineValue != 0 {
			t.Errorf("expected BaselineMonetaryNTD to be populated from BaselineValue=%.4f", result.Experiment.BaselineValue)
		}
	} else {
		if result.BaselineMonetaryNTD != result.Experiment.BaselineMonetaryNTD {
			t.Errorf("expected top-level and Experiment-level monetary fields to match")
		}
	}
	if result.CandidateMonetaryNTD == 0 && result.Experiment.CandidateMonetaryNTD == 0 {
		if result.Experiment.CandidateValue != 0 {
			t.Errorf("expected CandidateMonetaryNTD to be populated from CandidateValue=%.4f", result.Experiment.CandidateValue)
		}
	} else {
		if result.CandidateMonetaryNTD != result.Experiment.CandidateMonetaryNTD {
			t.Errorf("expected top-level and Experiment-level monetary fields to match")
		}
	}
}

func TestEvaluateMonetaryFieldsJSONSerialization(t *testing.T) {
	result := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			ID:                   "test-json",
			BaselineValue:        0.05,
			CandidateValue:       0.07,
			BaselineMonetaryNTD:  50000.0,
			CandidateMonetaryNTD: 70000.0,
		},
		BaselineMonetaryNTD:  50000.0,
		CandidateMonetaryNTD: 70000.0,
	}

	bytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}

	var decoded domain.PromptExperimentResult
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if decoded.BaselineMonetaryNTD != 50000.0 {
		t.Errorf("expected BaselineMonetaryNTD=50000.0, got %.2f", decoded.BaselineMonetaryNTD)
	}
	if decoded.CandidateMonetaryNTD != 70000.0 {
		t.Errorf("expected CandidateMonetaryNTD=70000.0, got %.2f", decoded.CandidateMonetaryNTD)
	}
	if decoded.Experiment.BaselineMonetaryNTD != 50000.0 {
		t.Errorf("expected Experiment.BaselineMonetaryNTD=50000.0, got %.2f", decoded.Experiment.BaselineMonetaryNTD)
	}
	if decoded.Experiment.CandidateMonetaryNTD != 70000.0 {
		t.Errorf("expected Experiment.CandidateMonetaryNTD=70000.0, got %.2f", decoded.Experiment.CandidateMonetaryNTD)
	}
}

func TestMaxDrawdownSignConversion(t *testing.T) {
	returns := []float64{-0.01, -0.03, -0.02, 0.01, -0.01, -0.02, 0.005, -0.015}

	dd := eval.MaxDrawdown(returns)
	if dd <= 0 {
		t.Errorf("eval.MaxDrawdown should return positive value, got %f", dd)
	}
	if dd > 1.0 {
		t.Errorf("eval.MaxDrawdown should be <= 1.0 for reasonable inputs, got %f", dd)
	}
}

// TestEvaluateNoDrawdownSpikeGateRunsOOSBeforePassesAcceptance verifies that
// OOS validation runs BEFORE passesAcceptance, so the no_drawdown_spike gate
// can inspect the populated OOSResult.
// Before fix: OOSResult is nil (OOS runs only if passesAcceptance returns true).
// After fix: OOSResult is populated (OOS runs unconditionally before passesAcceptance).
func TestEvaluateNoDrawdownSpikeGateRunsOOSBeforePassesAcceptance(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "../.."))
	stateDir := t.TempDir()
	store := ledger.NewStore(stateDir).(ledger.ExperimentStore)
	judge := NewJudge(store, filepath.Join(root, "samples", "replay", "twse_stock_day_all_sample.csv"), filepath.Join(stateDir, "baseline_policy.json"))
	resultPath := filepath.Join(stateDir, "experiments", "test-ood-reorder.json")
	promptPath := filepath.Join(t.TempDir(), "v2.md")
	baselinePromptPath := filepath.Join(root, "prompts/agents/growth_momentum.md")
	windowPath := filepath.Join(stateDir, "windows", "window-ood-reorder.json")

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
		WindowID:             "window-ood-reorder",
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
			ID:               "test-ood-reorder",
			TargetAgentID:    "growth-momentum-01",
			Skill:            "growth_momentum",
			MutationType:     "prompt_tightening",
			AcceptanceMetric: "sharpe_like",
			AcceptanceGates:  []string{"improve_sharpe_like", "no_material_drawdown_degradation", "no_drawdown_spike", "no_constraint_bypass"},
			Status:           domain.ExperimentRunning,
		},
		Brief: domain.MutationBrief{
			WindowID:            "window-ood-reorder",
			TargetAgentID:       "growth-momentum-01",
			TargetSkill:         "growth_momentum",
			TargetLayer:         domain.LayerStyle,
			PromptFile:          baselinePromptPath,
			MutationType:        "prompt_tightening",
			AcceptanceMetric:    "sharpe_like",
			AcceptanceGates:     []string{"improve_sharpe_like", "no_material_drawdown_degradation", "no_drawdown_spike", "no_constraint_bypass"},
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

	if result.OOSResult == nil {
		t.Fatal("Evaluate did not populate OOSResult — OOS validation must run before passesAcceptance")
	}
}

// TestNoDrawdownSpikeGateRejectsWhenOOSFails verifies the gate logic itself:
// the no_drawdown_spike gate in passesAcceptance rejects the experiment when
// OOSResult is populated with Passed=false.
func TestNoDrawdownSpikeGateRejectsWhenOOSFails(t *testing.T) {
	result := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			AcceptanceGates: []string{"improve_sharpe_like", "no_drawdown_spike"},
			BaselineValue:   0.0100,
			CandidateValue:  0.0150,
			MutationType:    "prompt_tightening",
		},
		Brief: domain.MutationBrief{
			MaturityLevel: "level_1_exploratory",
		},
		BaselineObservations:  5,
		CandidateObservations: 5,
		JudgeChecks:           []string{"a", "b"},
		OOSResult: &domain.OOSResult{
			Passed: false,
			Reason: "OOS drawdown spike detected",
		},
	}

	accepted, note := testJudge().passesAcceptance(result, nil)
	if accepted {
		t.Fatal("expected no_drawdown_spike to reject when OOSResult.Passed=false")
	}
	if !strings.Contains(note, "rejected") {
		t.Fatalf("expected rejection note, got: %s", note)
	}
}

func TestMaxDrawdownEmptyInput(t *testing.T) {
	if dd := eval.MaxDrawdown(nil); dd != 0 {
		t.Errorf("nil: got %f, want 0", dd)
	}
	if dd := eval.MaxDrawdown([]float64{}); dd != 0 {
		t.Errorf("empty: got %f, want 0", dd)
	}
	dd := eval.MaxDrawdown([]float64{0.01, 0.02, 0.03})
	if math.Abs(dd) > 1e-10 {
		t.Errorf("ever-increasing: got %.10f, want 0", dd)
	}
}

func TestPreserveDownsideProtectionGate_AcceptableDrawdown(t *testing.T) {
	// 63 samples required for Welch t-test statistical significance
	baseline := make([]float64, 63)
	candidate := make([]float64, 63)
	for i := range 63 {
		baseline[i] = []float64{0.01, -0.01, 0.02, -0.005, 0.015, -0.01}[i%6]
		candidate[i] = []float64{0.06, 0.04, 0.07, 0.045, 0.065, 0.04}[i%6]
	}

	result := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			AcceptanceGates: []string{"improve_sharpe_like", "preserve_downside_protection"},
			BaselineValue:   0.005,
			CandidateValue:  0.050,
			MutationType:    "prompt_tightening",
		},
		Brief: domain.MutationBrief{
			MaturityLevel: "level_1_exploratory",
		},
		BaselineObservations:  63,
		CandidateObservations: 63,
		BaselineReturns:       baseline,
		CandidateReturns:      candidate,
		JudgeChecks:           []string{"a", "b"},
	}

	accepted, note := testJudge().passesAcceptance(result, nil)
	if !accepted {
		t.Fatalf("expected acceptable drawdown to pass, got: %s", note)
	}
}

func TestPreserveDownsideProtectionGate_ExcessiveDrawdown(t *testing.T) {
	baseline := []float64{0.01, -0.01, 0.02, -0.005, 0.015, -0.01}
	candidate := []float64{0.02, -0.05, -0.08, 0.02, -0.04, 0.01}

	result := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			AcceptanceGates: []string{"improve_sharpe_like", "preserve_downside_protection"},
			BaselineValue:   0.005,
			CandidateValue:  0.007,
			MutationType:    "prompt_tightening",
		},
		Brief: domain.MutationBrief{
			MaturityLevel: "level_1_exploratory",
		},
		BaselineObservations:  6,
		CandidateObservations: 6,
		BaselineReturns:       baseline,
		CandidateReturns:      candidate,
		JudgeChecks:           []string{"a", "b"},
	}

	accepted, note := testJudge().passesAcceptance(result, nil)
	if accepted {
		t.Fatalf("expected excessive drawdown to be rejected, got: %s", note)
	}
}

// TestPassesAcceptanceRejectsFallbackWindowWhenNotInBurnIn verifies that
// experiments which relied on the fallback backtest window are rejected once
// the system has matured past burn-in. The fallback window can overlap the
// OOS window, so accepting such results in calibrating/full-auto maturity
// would contaminate the promotion path.
func TestPassesAcceptanceRejectsFallbackWindowWhenNotInBurnIn(t *testing.T) {
	result := domain.PromptExperimentResult{
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
		UsedFallbackWindow:    true,
	}

	judge := testJudge()
	// Simulate calibrating maturity: tracker present, but past burn-in
	// (backdate start by 60 days so computeMaturity returns Calibrating).
	judge.maturityTracker = domain.NewMaturityTrackerWithStart(time.Now().AddDate(0, 0, -60))

	accepted, note := judge.passesAcceptance(result, nil)
	if accepted {
		t.Fatalf("expected rejection for fallback-window experiment in calibrating maturity, got accepted")
	}
	if !strings.Contains(note, "fallback") {
		t.Fatalf("expected rejection note to mention fallback, got %q", note)
	}
}

// TestPassesAcceptanceAllowsFallbackWindowDuringBurnIn verifies that
// during the burn-in phase, when no real replay data exists, the system
// permits fallback-window experiments. This unblocks early iteration before
// the system has accumulated its own window history.
func TestPassesAcceptanceAllowsFallbackWindowDuringBurnIn(t *testing.T) {
	result := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			AcceptanceGates: []string{"improve_sharpe_like", "no_material_drawdown_degradation", "no_constraint_bypass"},
			BaselineValue:   0.0100,
			CandidateValue:  0.0120,
			MutationType:    "prompt_tightening",
		},
		Brief: domain.MutationBrief{
			MaturityLevel: "level_1_exploratory",
		},
		BaselineObservations:  8,
		CandidateObservations: 8,
		JudgeChecks:           []string{"a", "b", "c"},
		UsedFallbackWindow:    true,
	}

	judge := testJudge()
	// In burn-in: gate is the very first check, so this experiment is
	// rejected by burn-in (not the fallback gate). The fallback gate must
	// not run first.
	// Start 3 days ago so computeMaturity returns BurnIn (threshold = 0).
	judge.maturityTracker = domain.NewMaturityTrackerWithStart(time.Now().AddDate(0, 0, -3))

	accepted, note := judge.passesAcceptance(result, nil)
	if accepted {
		t.Fatalf("expected burn-in rejection (not fallback gate), got accepted")
	}
	if !strings.Contains(note, "burn_in") {
		t.Fatalf("expected rejection note to mention burn_in, got %q", note)
	}
}
