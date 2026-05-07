package baseline

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestDefaultPolicy_ContractGuards(t *testing.T) {
	policy := DefaultPolicy()

	if policy.Constraints.StartingCash != 3000000 {
		t.Errorf("default StartingCash drift: expected 3000000, got %v", policy.Constraints.StartingCash)
	}
	if policy.Constraints.MaxPositionWeight != 0.18 {
		t.Errorf("default MaxPositionWeight drift: expected 0.18, got %v", policy.Constraints.MaxPositionWeight)
	}
	if policy.Constraints.MaxOpenPositions != 5 {
		t.Errorf("default MaxOpenPositions drift: expected 5, got %d", policy.Constraints.MaxOpenPositions)
	}
	if policy.Constraints.MinTradableVolume != 1000000 {
		t.Errorf("default MinTradableVolume drift: expected 1000000, got %v", policy.Constraints.MinTradableVolume)
	}
	if policy.Constraints.MinRecommendationConviction != 60 {
		t.Errorf("default MinRecommendationConviction drift: expected 60, got %d", policy.Constraints.MinRecommendationConviction)
	}
	if !policy.Constraints.RequireCROPass {
		t.Error("default RequireCROPass drift: expected true")
	}
	if policy.Constraints.TransactionCostBPS != 1.425 {
		t.Errorf("default TransactionCostBPS drift: expected 1.425, got %v", policy.Constraints.TransactionCostBPS)
	}
	if policy.Constraints.SlippageBPS != 4.0 {
		t.Errorf("default SlippageBPS drift: expected 4.0, got %v", policy.Constraints.SlippageBPS)
	}
	if policy.Constraints.ReserveCashFraction != 0.1 {
		t.Errorf("default ReserveCashFraction drift: expected 0.1, got %v", policy.Constraints.ReserveCashFraction)
	}
	if policy.Constraints.StopLossPct != 0 {
		t.Errorf("default StopLossPct drift: expected 0, got %v", policy.Constraints.StopLossPct)
	}
	if policy.Constraints.TakeProfitPct != 0 {
		t.Errorf("default TakeProfitPct drift: expected 0, got %v", policy.Constraints.TakeProfitPct)
	}
	if policy.ExecutionPolicy.ConvictionFloor != 60 {
		t.Errorf("default ConvictionFloor drift: expected 60, got %d", policy.ExecutionPolicy.ConvictionFloor)
	}
	if !policy.ExecutionPolicy.RequireCROPass {
		t.Error("default ExecutionPolicy.RequireCROPass drift: expected true")
	}
	if policy.ExecutionPolicy.MomentumCrashProtection {
		t.Error("default MomentumCrashProtection drift: expected false")
	}
	if policy.ExecutionPolicy.EnableConvictionNormalization {
		t.Error("default EnableConvictionNormalization drift: expected false")
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
