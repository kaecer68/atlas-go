package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestRunRejectsMissingResult(t *testing.T) {
	if err := run([]string{"--result", "/nonexistent/path.json"}); err == nil {
		t.Fatalf("expected error for missing result file")
	}
}

func TestRunJudgesValidExperimentResult(t *testing.T) {
	dir := t.TempDir()

	// Setup prompt file
	promptPath := filepath.Join(dir, "v2.md")
	if err := os.WriteFile(promptPath, []byte("# Candidate\nrequire trend confirmation\n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	// Setup brief
	brief := domain.MutationBrief{
		ContractVersion:     domain.MutationBriefContractVersion,
		WindowID:            "window-20260326-20260327",
		TargetAgentID:       "growth-momentum-01",
		TargetSkill:         "growth_momentum",
		TargetLayer:         domain.LayerStyle,
		PromptFile:          filepath.Join("..", "..", "prompts", "agents", "growth_momentum.md"),
		MutationType:        "prompt_tightening",
		AcceptanceMetric:    "sharpe_like",
		AcceptanceGates:     []string{"improve_sharpe_like"},
		ForbiddenActions:    []string{"illiquid_breakout_chasing"},
		RequiredSkills:      []string{"growth_momentum"},
		ObservedWindowCount: 2,
		MaturityLevel:       "level_1_exploratory",
		GeneratedAt:         time.Now(),
	}

	result := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			ID:              "exec-growth-momentum-01-test",
			ProposalID:      "proposal-test",
			TargetAgentID:   "growth-momentum-01",
			Skill:           "growth_momentum",
			MutationType:    "prompt_tightening",
			AcceptanceGates: []string{"improve_sharpe_like"},
			BaselineValue:   0,
			CandidateValue:  0,
			Status:          domain.ExperimentRunning,
			WindowStart:     time.Now().AddDate(0, 0, -10),
			WindowEnd:       time.Now(),
		},
		Brief:           brief,
		CandidatePrompt: promptPath,
		EvaluationMode:  "policy_checked_pending_replay",
		RecordedAt:      time.Now(),
	}

	experimentsDir := filepath.Join(dir, "experiments")
	os.MkdirAll(experimentsDir, 0o755)
	resultPath := filepath.Join(experimentsDir, "result.json")
	b, _ := json.MarshalIndent(result, "", "  ")
	if err := os.WriteFile(resultPath, b, 0o644); err != nil {
		t.Fatalf("write result: %v", err)
	}

	// Setup window summary (judge.go infers window dir as sibling of experiments dir)
	windowSummary := domain.BacktestWindowSummary{
		WindowID:  "window-20260326-20260327",
		StartDate: time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC),
	}
	windowsDir := filepath.Join(dir, "windows")
	os.MkdirAll(windowsDir, 0o755)
	wb, _ := json.MarshalIndent(windowSummary, "", "  ")
	os.WriteFile(filepath.Join(windowsDir, "window-20260326-20260327.json"), wb, 0o644)

	// Set env overrides
	origLedger := os.Getenv("ATLAS_LEDGER_DIR")
	origReplay := os.Getenv("ATLAS_REPLAY_DATA_PATH")
	origBaseline := os.Getenv("ATLAS_BASELINE_POLICY_PATH")
	defer func() {
		os.Setenv("ATLAS_LEDGER_DIR", origLedger)
		os.Setenv("ATLAS_REPLAY_DATA_PATH", origReplay)
		os.Setenv("ATLAS_BASELINE_POLICY_PATH", origBaseline)
	}()
	os.Setenv("ATLAS_LEDGER_DIR", dir)
	os.Setenv("ATLAS_REPLAY_DATA_PATH", filepath.Join("..", "..", "samples", "replay", "twse_stock_day_all_sample.csv"))
	os.Setenv("ATLAS_BASELINE_POLICY_PATH", filepath.Join(dir, "baseline.json"))

	if err := run([]string{"--result", resultPath}); err != nil {
		t.Fatalf("run judge-experiment: %v", err)
	}
}
