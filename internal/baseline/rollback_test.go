package baseline

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
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

func TestRevert_ConstraintStateLostAfterRollback(t *testing.T) {
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "baseline_policy.json")

	policy := Policy{
		Version: 4,
		PromptOverrides: map[string]string{
			"agent-1": "prompt-v2",
			"agent-2": "prompt-v3",
		},
		Constraints: domain.SimulationConstraints{
			StartingCash:                3000000,
			MaxPositionWeight:           0.15,
			MaxOpenPositions:            5,
			MinTradableVolume:           1000000,
			MinRecommendationConviction: 60,
			RequireCROPass:              true,
			TransactionCostBPS:          1.425,
			SlippageBPS:                 4.0,
			ReserveCashFraction:         0.1,
		},
		ExecutionPolicy: domain.ExecutionPolicy{
			ConvictionFloor: 60,
			RequireCROPass:  true,
		},
		Promotions: []PromotionRecord{
			{ExperimentID: "exp-1", TargetAgentID: "agent-1", MutationType: "prompt_tightening", VersionAfter: 2},
			{ExperimentID: "exp-2", TargetAgentID: "cio-01", MutationType: "portfolio_constraint_revision", VersionAfter: 3},
			{ExperimentID: "exp-3", TargetAgentID: "agent-2", MutationType: "prompt_tightening", VersionAfter: 4},
		},
	}
	if err := Save(policyPath, policy); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	manager := NewManager(policyPath)

	_, err := manager.Revert(RevertTarget{Type: RevertToVersion, Version: 3}, "revert to v3", false)
	if err != nil {
		t.Fatalf("revert: %v", err)
	}

	reverted, err := Load(policyPath)
	if err != nil {
		t.Fatalf("load reverted policy: %v", err)
	}

	if reverted.Version != 3 {
		t.Errorf("expected version 3, got %d", reverted.Version)
	}
	if reverted.Constraints.MaxPositionWeight != 0.15 {
		t.Errorf("rollback reconstruction lost constraint state: expected MaxPositionWeight=0.15 at version 3, got %v", reverted.Constraints.MaxPositionWeight)
	}
}

func TestRevert_MultipleConstraintChangesRestoresEarlierConstraintState(t *testing.T) {
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "baseline_policy.json")

	// History:
	// v1: default constraints (MaxPositionWeight=0.18)
	// v2: constraint promotion A -> MaxPositionWeight=0.15
	// v3: prompt promotion B
	// v4: constraint promotion C -> MaxPositionWeight=0.12
	// v5: prompt promotion D
	policy := Policy{
		Version: 5,
		PromptOverrides: map[string]string{
			"agent-1": "prompt-v2",
			"agent-2": "prompt-v3",
		},
		Constraints: domain.SimulationConstraints{
			StartingCash:                3000000,
			MaxPositionWeight:           0.12,
			MaxOpenPositions:            5,
			MinTradableVolume:           1000000,
			MinRecommendationConviction: 60,
			RequireCROPass:              true,
			TransactionCostBPS:          1.425,
			SlippageBPS:                 4.0,
			ReserveCashFraction:         0.1,
		},
		ExecutionPolicy: domain.ExecutionPolicy{
			ConvictionFloor: 60,
			RequireCROPass:  true,
		},
		Promotions: []PromotionRecord{
			{
				ExperimentID: "exp-A", TargetAgentID: "cio-01",
				MutationType: "portfolio_constraint_revision", VersionAfter: 2,
				ConstraintsSnapshot: &domain.SimulationConstraints{
					StartingCash:                3000000,
					MaxPositionWeight:           0.15,
					MaxOpenPositions:            5,
					MinTradableVolume:           1000000,
					MinRecommendationConviction: 60,
					RequireCROPass:              true,
					TransactionCostBPS:          1.425,
					SlippageBPS:                 4.0,
					ReserveCashFraction:         0.1,
				},
			},
			{ExperimentID: "exp-B", TargetAgentID: "agent-1", MutationType: "prompt_tightening", VersionAfter: 3},
			{
				ExperimentID: "exp-C", TargetAgentID: "cio-01",
				MutationType: "portfolio_constraint_revision", VersionAfter: 4,
				ConstraintsSnapshot: &domain.SimulationConstraints{
					StartingCash:                3000000,
					MaxPositionWeight:           0.12,
					MaxOpenPositions:            5,
					MinTradableVolume:           1000000,
					MinRecommendationConviction: 60,
					RequireCROPass:              true,
					TransactionCostBPS:          1.425,
					SlippageBPS:                 4.0,
					ReserveCashFraction:         0.1,
				},
			},
			{ExperimentID: "exp-D", TargetAgentID: "agent-2", MutationType: "prompt_tightening", VersionAfter: 5},
		},
	}
	if err := Save(policyPath, policy); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	manager := NewManager(policyPath)

	// Revert to v3 (between two constraint promotions).
	// Expected: MaxPositionWeight should be 0.15 (from exp-A), not default 0.18.
	_, err := manager.Revert(RevertTarget{Type: RevertToVersion, Version: 3}, "revert to v3", false)
	if err != nil {
		t.Fatalf("revert: %v", err)
	}

	reverted, err := Load(policyPath)
	if err != nil {
		t.Fatalf("load reverted policy: %v", err)
	}

	if reverted.Version != 3 {
		t.Errorf("expected version 3, got %d", reverted.Version)
	}
	if reverted.Constraints.MaxPositionWeight != 0.15 {
		t.Errorf("rollback reconstruction failed: expected MaxPositionWeight=0.15 at version 3 (after exp-A, before exp-C), got %v", reverted.Constraints.MaxPositionWeight)
	}

	// Also verify revert to v1 falls back to default correctly.
	// Restore original policy first.
	if err := Save(policyPath, policy); err != nil {
		t.Fatalf("save policy: %v", err)
	}
	_, err = manager.Revert(RevertTarget{Type: RevertToVersion, Version: 1}, "revert to v1", false)
	if err != nil {
		t.Fatalf("revert to v1: %v", err)
	}
	reverted, err = Load(policyPath)
	if err != nil {
		t.Fatalf("load reverted policy: %v", err)
	}
	if reverted.Constraints.MaxPositionWeight != 0.18 {
		t.Errorf("rollback to v1 should use default: expected MaxPositionWeight=0.18, got %v", reverted.Constraints.MaxPositionWeight)
	}

	// Verify revert to v4 uses exp-C constraint.
	if err := Save(policyPath, policy); err != nil {
		t.Fatalf("save policy: %v", err)
	}
	_, err = manager.Revert(RevertTarget{Type: RevertToVersion, Version: 4}, "revert to v4", false)
	if err != nil {
		t.Fatalf("revert to v4: %v", err)
	}
	reverted, err = Load(policyPath)
	if err != nil {
		t.Fatalf("load reverted policy: %v", err)
	}
	if reverted.Constraints.MaxPositionWeight != 0.12 {
		t.Errorf("rollback to v4 should use exp-C constraint: expected MaxPositionWeight=0.12, got %v", reverted.Constraints.MaxPositionWeight)
	}
}

func TestRevert_PromptStateUsesLatestOverrideNotHistorical(t *testing.T) {
	tmpDir := t.TempDir()
	policyPath := filepath.Join(tmpDir, "baseline_policy.json")

	// History:
	// v1: default (no overrides)
	// v2: agent-1 promoted -> "prompt-v2"
	// v3: agent-2 promoted -> "prompt-v3"
	// v4: agent-1 promoted AGAIN -> "prompt-v4"
	// v5: prompt promotion for agent-3
	policy := Policy{
		Version: 5,
		PromptOverrides: map[string]string{
			"agent-1": "prompt-v4",
			"agent-2": "prompt-v3",
			"agent-3": "prompt-v5",
		},
		Constraints: domain.SimulationConstraints{
			StartingCash:                3000000,
			MaxPositionWeight:           0.18,
			MaxOpenPositions:            5,
			MinTradableVolume:           1000000,
			MinRecommendationConviction: 60,
			RequireCROPass:              true,
			TransactionCostBPS:          1.425,
			SlippageBPS:                 4.0,
			ReserveCashFraction:         0.1,
		},
		ExecutionPolicy: domain.ExecutionPolicy{
			ConvictionFloor: 60,
			RequireCROPass:  true,
		},
		Promotions: []PromotionRecord{
			{ExperimentID: "exp-1", TargetAgentID: "agent-1", MutationType: "prompt_tightening", VersionAfter: 2, PromptSnapshot: "prompt-v2"},
			{ExperimentID: "exp-2", TargetAgentID: "agent-2", MutationType: "prompt_tightening", VersionAfter: 3, PromptSnapshot: "prompt-v3"},
			{ExperimentID: "exp-3", TargetAgentID: "agent-1", MutationType: "prompt_tightening", VersionAfter: 4, PromptSnapshot: "prompt-v4"},
			{ExperimentID: "exp-4", TargetAgentID: "agent-3", MutationType: "prompt_tightening", VersionAfter: 5, PromptSnapshot: "prompt-v5"},
		},
	}
	if err := Save(policyPath, policy); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	manager := NewManager(policyPath)

	// Revert to v3 (between agent-1's two promotions).
	// Expected: agent-1="prompt-v2", agent-2="prompt-v3", agent-3 should NOT exist.
	_, err := manager.Revert(RevertTarget{Type: RevertToVersion, Version: 3}, "revert to v3", false)
	if err != nil {
		t.Fatalf("revert: %v", err)
	}

	reverted, err := Load(policyPath)
	if err != nil {
		t.Fatalf("load reverted policy: %v", err)
	}

	if reverted.Version != 3 {
		t.Errorf("expected version 3, got %d", reverted.Version)
	}

	// agent-1 should have its v2 prompt, NOT the current v4 override.
	if reverted.PromptOverrides["agent-1"] != "prompt-v2" {
		t.Errorf("rollback used current prompt override instead of historical: expected agent-1='prompt-v2', got '%s'",
			reverted.PromptOverrides["agent-1"])
	}

	// agent-2 should still have its v3 prompt.
	if reverted.PromptOverrides["agent-2"] != "prompt-v3" {
		t.Errorf("agent-2 override lost: expected 'prompt-v3', got '%s'",
			reverted.PromptOverrides["agent-2"])
	}

	// agent-3 was promoted at v5, after target v3 — should NOT exist.
	if reverted.PromptOverrides["agent-3"] != "" {
		t.Errorf("agent-3 should not exist at version 3 (promoted at v5), got '%s'",
			reverted.PromptOverrides["agent-3"])
	}
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
