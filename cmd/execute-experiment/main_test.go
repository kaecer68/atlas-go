package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestRunExecutesExperiment(t *testing.T) {
	dir := t.TempDir()
	promptPath := filepath.Join(dir, "prompt.md")
	if err := os.WriteFile(promptPath, []byte("# Test prompt\n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	brief := domain.MutationBrief{
		ContractVersion:     domain.MutationBriefContractVersion,
		WindowID:            "window-test",
		TargetAgentID:       "test-agent-01",
		TargetSkill:         "test_skill",
		TargetLayer:         domain.LayerStyle,
		PromptFile:          promptPath,
		MutationType:        "prompt_tightening",
		AcceptanceMetric:    "sharpe_like",
		AcceptanceGates:     []string{"improve_sharpe_like"},
		ForbiddenActions:    []string{"bad_action"},
		RequiredSkills:      []string{"test_skill"},
		ObservedWindowCount: 1,
		MaturityLevel:       "level_1_exploratory",
		GeneratedAt:         time.Now(),
	}
	briefPath := filepath.Join(dir, "brief.json")
	b, err := json.Marshal(brief)
	if err != nil {
		t.Fatalf("marshal brief: %v", err)
	}
	if err := os.WriteFile(briefPath, b, 0o644); err != nil {
		t.Fatalf("write brief: %v", err)
	}

	// Point ledger and config to temp dir via env if needed, but config.Load()
	// reads from .env or defaults. We'll override via env for this test.
	origLedger := os.Getenv("ATLAS_LEDGER_DIR")
	origBaseline := os.Getenv("ATLAS_BASELINE_POLICY_PATH")
	defer func() {
		os.Setenv("ATLAS_LEDGER_DIR", origLedger)
		os.Setenv("ATLAS_BASELINE_POLICY_PATH", origBaseline)
	}()
	os.Setenv("ATLAS_LEDGER_DIR", dir)
	os.Setenv("ATLAS_BASELINE_POLICY_PATH", filepath.Join(dir, "baseline.json"))

	if err := run([]string{"--brief", briefPath}); err != nil {
		t.Fatalf("run execute-experiment: %v", err)
	}
}

func TestRunRejectsInvalidBrief(t *testing.T) {
	dir := t.TempDir()
	briefPath := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(briefPath, []byte(`{"invalid":true}`), 0o644); err != nil {
		t.Fatalf("write invalid brief: %v", err)
	}

	origLedger := os.Getenv("ATLAS_LEDGER_DIR")
	origBaseline := os.Getenv("ATLAS_BASELINE_POLICY_PATH")
	defer func() {
		os.Setenv("ATLAS_LEDGER_DIR", origLedger)
		os.Setenv("ATLAS_BASELINE_POLICY_PATH", origBaseline)
	}()
	os.Setenv("ATLAS_LEDGER_DIR", dir)
	os.Setenv("ATLAS_BASELINE_POLICY_PATH", filepath.Join(dir, "baseline.json"))

	if err := run([]string{"--brief", briefPath}); err == nil {
		t.Fatalf("expected error for invalid brief contract")
	}
}
