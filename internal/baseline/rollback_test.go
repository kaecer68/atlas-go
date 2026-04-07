package baseline

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRevert_LastPromotion(t *testing.T) {
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "baseline_policy.json")

	// Create initial policy with promotions
	policy := Policy{
		Version:         3,
		PromptOverrides: map[string]string{"agent-1": "prompt-v2"},
		Promotions: []PromotionRecord{
			{ExperimentID: "exp-1", TargetAgentID: "agent-1", MutationType: "prompt_tightening"},
			{ExperimentID: "exp-2", TargetAgentID: "agent-1", MutationType: "prompt_tightening"},
		},
	}
	if err := Save(policyPath, policy); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	manager := NewManager(policyPath)

	// Test dry run revert last
	result, err := manager.Revert(RevertTarget{Type: RevertLast}, "test revert", true)
	if err != nil {
		t.Fatalf("revert dry run: %v", err)
	}

	if result.FromVersion != 3 {
		t.Errorf("expected from version 3, got %d", result.FromVersion)
	}
	if result.ToVersion != 2 {
		t.Errorf("expected to version 2, got %d", result.ToVersion)
	}
	if len(result.RevertedExperiments) != 1 || result.RevertedExperiments[0] != "exp-2" {
		t.Errorf("expected to revert exp-2, got %v", result.RevertedExperiments)
	}
	if !result.DryRun {
		t.Error("expected dry run to be true")
	}

	// Verify policy unchanged after dry run
	current, _ := Load(policyPath)
	if current.Version != 3 {
		t.Error("policy should be unchanged after dry run")
	}
}

func TestRevert_ToVersion(t *testing.T) {
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "baseline_policy.json")

	policy := Policy{
		Version:         4,
		PromptOverrides: map[string]string{"agent-1": "prompt-v3"},
		Promotions: []PromotionRecord{
			{ExperimentID: "exp-1", TargetAgentID: "agent-1"},
			{ExperimentID: "exp-2", TargetAgentID: "agent-1"},
			{ExperimentID: "exp-3", TargetAgentID: "agent-1"},
		},
	}
	if err := Save(policyPath, policy); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	manager := NewManager(policyPath)

	// Revert to version 2
	result, err := manager.Revert(RevertTarget{Type: RevertToVersion, Version: 2}, "revert to v2", false)
	if err != nil {
		t.Fatalf("revert: %v", err)
	}

	if result.ToVersion != 2 {
		t.Errorf("expected to version 2, got %d", result.ToVersion)
	}
	if len(result.RevertedExperiments) != 2 {
		t.Errorf("expected 2 reverted experiments, got %d", len(result.RevertedExperiments))
	}

	// Verify policy was updated
	current, _ := Load(policyPath)
	if current.Version != 2 {
		t.Errorf("expected policy version 2, got %d", current.Version)
	}
	if len(current.RevertHistory) != 1 {
		t.Errorf("expected 1 revert history entry, got %d", len(current.RevertHistory))
	}
}

func TestRevert_ToExperiment(t *testing.T) {
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "baseline_policy.json")

	policy := Policy{
		Version: 3,
		Promotions: []PromotionRecord{
			{ExperimentID: "exp-1", TargetAgentID: "agent-1"},
			{ExperimentID: "exp-2", TargetAgentID: "agent-1"},
		},
	}
	if err := Save(policyPath, policy); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	manager := NewManager(policyPath)

	// Revert to before exp-2
	result, err := manager.Revert(RevertTarget{Type: RevertToExperiment, ExperimentID: "exp-2"}, "revert before exp-2", false)
	if err != nil {
		t.Fatalf("revert: %v", err)
	}

	if result.ToVersion != 2 {
		t.Errorf("expected to version 2 (before exp-2), got %d", result.ToVersion)
	}
	if len(result.RevertedExperiments) != 1 || result.RevertedExperiments[0] != "exp-2" {
		t.Errorf("expected to revert exp-2, got %v", result.RevertedExperiments)
	}
}

func TestRevert_ValidationErrors(t *testing.T) {
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "baseline_policy.json")

	// Save default policy (version 1)
	if err := Save(policyPath, DefaultPolicy()); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	manager := NewManager(policyPath)

	// Test revert to current version
	_, err := manager.Revert(RevertTarget{Type: RevertToVersion, Version: 1}, "test", false)
	if err == nil || err.Error() != "already at target version, nothing to revert" {
		t.Errorf("expected 'already at target' error, got: %v", err)
	}

	// Test revert to future version
	_, err = manager.Revert(RevertTarget{Type: RevertToVersion, Version: 5}, "test", false)
	if err == nil {
		t.Error("expected error for future version")
	}

	// Test revert last with no promotions
	_, err = manager.Revert(RevertTarget{Type: RevertLast}, "test", false)
	if err == nil || !contains(err.Error(), "no promotions") {
		t.Errorf("expected 'no promotions' error, got: %v", err)
	}

	// Test revert to non-existent experiment
	_, err = manager.Revert(RevertTarget{Type: RevertToExperiment, ExperimentID: "non-existent"}, "test", false)
	if err == nil {
		t.Error("expected error for non-existent experiment")
	}
}

func TestGetPromotionHistory(t *testing.T) {
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "baseline_policy.json")

	policy := Policy{
		Version: 3,
		Promotions: []PromotionRecord{
			{ExperimentID: "exp-1", TargetAgentID: "agent-1", PromotedAt: time.Now()},
			{ExperimentID: "exp-2", TargetAgentID: "agent-2", PromotedAt: time.Now()},
		},
	}
	if err := Save(policyPath, policy); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	manager := NewManager(policyPath)
	history, err := manager.GetPromotionHistory()
	if err != nil {
		t.Fatalf("get history: %v", err)
	}

	if len(history) != 2 {
		t.Errorf("expected 2 history entries, got %d", len(history))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestGetRevertHistory(t *testing.T) {
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "baseline_policy.json")

	policy := Policy{
		Version: 2,
		Promotions: []PromotionRecord{
			{ExperimentID: "exp-1", TargetAgentID: "agent-1"},
		},
		RevertHistory: []RevertRecord{
			{FromVersion: 2, ToVersion: 1, Reason: "test revert"},
		},
	}
	if err := Save(policyPath, policy); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	manager := NewManager(policyPath)
	history, err := manager.GetRevertHistory()
	if err != nil {
		t.Fatalf("get revert history: %v", err)
	}

	if len(history) != 1 {
		t.Errorf("expected 1 revert history entry, got %d", len(history))
	}
	if history[0].Reason != "test revert" {
		t.Errorf("expected reason 'test revert', got %s", history[0].Reason)
	}
}
