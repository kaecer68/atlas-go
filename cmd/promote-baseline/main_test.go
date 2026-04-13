package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"encoding/json"
	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestRunRejectsMissingResult(t *testing.T) {
	if err := run([]string{"--result", "/nonexistent/path.json"}); err == nil {
		t.Fatalf("expected error for missing result file")
	}
}

func TestRunPromotesAcceptedExperiment(t *testing.T) {
	dir := t.TempDir()

	// Create candidate prompt file
	candidateDir := filepath.Join(dir, "prompts", "experiments", "test-agent", "exec-test-1")
	os.MkdirAll(candidateDir, 0o755)
	promptPath := filepath.Join(candidateDir, "v2.md")
	if err := os.WriteFile(promptPath, []byte("# Candidate Prompt\n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}

	result := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			ID:            "exec-test-1",
			TargetAgentID: "test-agent",
			Skill:         "test_skill",
			MutationType:  "prompt_tightening",
			Status:        domain.ExperimentAccepted,
			WindowStart:   time.Now().AddDate(0, 0, -10),
			WindowEnd:     time.Now(),
		},
		CandidatePrompt: promptPath,
		EvaluationMode:  "test",
		RecordedAt:      time.Now(),
	}

	resultPath := filepath.Join(dir, "result.json")
	b, _ := json.MarshalIndent(result, "", "  ")
	if err := os.WriteFile(resultPath, b, 0o644); err != nil {
		t.Fatalf("write result: %v", err)
	}

	origBaseline := os.Getenv("ATLAS_BASELINE_POLICY_PATH")
	defer os.Setenv("ATLAS_BASELINE_POLICY_PATH", origBaseline)
	os.Setenv("ATLAS_BASELINE_POLICY_PATH", filepath.Join(dir, "baseline.json"))

	if err := run([]string{"--result", resultPath}); err != nil {
		t.Fatalf("run promote-baseline: %v", err)
	}

	// Verify baseline was written
	baselinePath := filepath.Join(dir, "baseline.json")
	if _, err := os.Stat(baselinePath); err != nil {
		t.Fatalf("expected baseline policy to be created: %v", err)
	}
}
