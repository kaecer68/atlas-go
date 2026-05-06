package baseline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestLoadMissingReturnsDefaultPolicy(t *testing.T) {
	policy, err := Load("")
	if err != nil {
		t.Fatalf("load default policy: %v", err)
	}
	if policy.ExecutionPolicy.ConvictionFloor != 60 {
		t.Fatalf("expected default conviction floor 60, got %d", policy.ExecutionPolicy.ConvictionFloor)
	}
	if !policy.ExecutionPolicy.RequireCROPass {
		t.Fatalf("expected default CRO pass enabled")
	}
}

func TestPromoteAcceptedPromptExperiment(t *testing.T) {
	policy := DefaultPolicy()
	result := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			ID:            "exp-1",
			TargetAgentID: "growth-momentum-01",
			Skill:         "growth_momentum",
			MutationType:  "prompt_tightening",
			Status:        domain.ExperimentAccepted,
		},
		CandidatePrompt: "prompts/experiments/growth/v2.md",
	}

	next, err := Promote(policy, result, "candidate prompt body")
	if err != nil {
		t.Fatalf("promote prompt: %v", err)
	}
	if next.PromptOverrides["growth-momentum-01"] == "" {
		t.Fatalf("expected promoted prompt override")
	}
	if len(next.Promotions) != 1 {
		t.Fatalf("expected promotion history entry")
	}
}

func TestPromoteAcceptedConstraintExperiment(t *testing.T) {
	policy := DefaultPolicy()
	result := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			ID:            "exp-2",
			TargetAgentID: "cio-01",
			Skill:         "cio_portfolio",
			MutationType:  "portfolio_constraint_revision",
			Status:        domain.ExperimentAccepted,
		},
		CandidatePrompt: "prompts/experiments/cio/v2.md",
	}

	candidate := "max_position_weight: 0.15\nreserve_cash_fraction: 0.12\nrequire_cro_pass: true\n"
	next, err := Promote(policy, result, candidate)
	if err != nil {
		t.Fatalf("promote constraint: %v", err)
	}
	if next.Constraints.MaxPositionWeight != 0.15 {
		t.Fatalf("expected promoted max position weight")
	}
	if next.ExecutionPolicy.ConvictionFloor != 60 {
		t.Fatalf("expected execution policy to remain aligned with conviction floor 60, got %d", next.ExecutionPolicy.ConvictionFloor)
	}
}

func TestPolicyMarshalUsesSnakeCase(t *testing.T) {
	policy := Policy{Version: 2, PromptOverrides: map[string]string{"growth_momentum": "v2"}}
	data, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"prompt_overrides"`) {
		t.Fatalf("expected prompt_overrides; got %s", text)
	}
	if strings.Contains(text, `"PromptOverrides"`) {
		t.Fatalf("unexpected PascalCase PromptOverrides; got %s", text)
	}
	if !strings.Contains(text, `"version"`) {
		t.Fatalf("expected snake_case version; got %s", text)
	}
	if !strings.Contains(text, `"constraints"`) {
		t.Fatalf("expected snake_case constraints; got %s", text)
	}
	if !strings.Contains(text, `"execution_policy"`) {
		t.Fatalf("expected snake_case execution_policy; got %s", text)
	}
	if !strings.Contains(text, `"promotions"`) {
		t.Fatalf("expected snake_case promotions; got %s", text)
	}
	if !strings.Contains(text, `"revert_history"`) {
		t.Fatalf("expected snake_case revert_history; got %s", text)
	}
	if !strings.Contains(text, `"last_updated_at"`) {
		t.Fatalf("expected snake_case last_updated_at; got %s", text)
	}
}

func TestManagerPromoteResultWritesPolicyFile(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "baseline_policy.json")
	resultPath := filepath.Join(dir, "result.json")
	candidatePath := filepath.Join(dir, "v2.md")

	if err := os.WriteFile(candidatePath, []byte("candidate prompt body"), 0o644); err != nil {
		t.Fatalf("write candidate: %v", err)
	}

	result := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			ID:            "exp-3",
			TargetAgentID: "growth-momentum-01",
			Skill:         "growth_momentum",
			MutationType:  "prompt_tightening",
			Status:        domain.ExperimentAccepted,
		},
		CandidatePrompt: candidatePath,
	}
	bytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if err := os.WriteFile(resultPath, bytes, 0o644); err != nil {
		t.Fatalf("write result: %v", err)
	}

	manager := NewManager(policyPath)
	policy, err := manager.PromoteResult(resultPath)
	if err != nil {
		t.Fatalf("promote result: %v", err)
	}
	if policy.PromptOverrides["growth-momentum-01"] == "" {
		t.Fatalf("expected prompt override to be persisted")
	}
	if _, err := os.Stat(policyPath); err != nil {
		t.Fatalf("expected policy file to be written: %v", err)
	}
}
