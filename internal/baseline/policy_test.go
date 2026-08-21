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
	if policy.Constraints.TransactionCostBPS != 14.25 {
		t.Errorf("default TransactionCostBPS drift: expected 14.25, got %v", policy.Constraints.TransactionCostBPS)
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
	if policy.Constraints.MaxHoldingDays != 0 {
		t.Errorf("default MaxHoldingDays drift: expected 0 (unlimited), got %d", policy.Constraints.MaxHoldingDays)
	}
	if policy.ExecutionPolicy.ConvictionFloor != 60 {
		t.Errorf("default ConvictionFloor drift: expected 60, got %d", policy.ExecutionPolicy.ConvictionFloor)
	}
	if !policy.ExecutionPolicy.RequireCROPass {
		t.Error("default ExecutionPolicy.RequireCROPass drift: expected true")
	}
	if !policy.ExecutionPolicy.MomentumCrashProtection {
		t.Error("default MomentumCrashProtection drift: expected true (A3)")
	}
	if !policy.ExecutionPolicy.EnableConvictionNormalization {
		t.Error("default EnableConvictionNormalization drift: expected true (A3)")
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

func TestResolvePromptOverride(t *testing.T) {
	policy := Policy{
		PromptOverrides: map[string]string{
			"agent-1":         "custom prompt for agent-1",
			"growth_momentum": "custom prompt for skill",
		},
	}

	tests := []struct {
		name    string
		agentID string
		skill   string
		want    string
	}{
		{"agent match", "agent-1", "any_skill", "custom prompt for agent-1"},
		{"skill match", "agent-2", "growth_momentum", "custom prompt for skill"},
		{"agent priority over skill", "agent-1", "growth_momentum", "custom prompt for agent-1"},
		{"no match", "agent-3", "unknown_skill", ""},
		{"empty policy", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolvePromptOverride(policy, tt.agentID, tt.skill)
			if got != tt.want {
				t.Errorf("ResolvePromptOverride(%q, %q) = %q, want %q", tt.agentID, tt.skill, got, tt.want)
			}
		})
	}
}

func TestResolvePromptOverride_EmptyPolicy(t *testing.T) {
	policy := Policy{}
	if got := ResolvePromptOverride(policy, "agent-1", "skill"); got != "" {
		t.Errorf("expected empty string for empty policy, got %q", got)
	}
}

func TestParseIntValue(t *testing.T) {
	tests := []struct {
		line   string
		wantV  int
		wantOk bool
	}{
		{"conviction_floor: 5", 5, true},
		{"conviction_floor:0", 0, true},
		{"conviction_floor: -2", -2, true},
		{"conviction_floor: abc", 0, false},
		{"conviction_floor:", 0, false},
	}
	for _, tt := range tests {
		v, ok := parseIntValue(tt.line)
		if ok != tt.wantOk || v != tt.wantV {
			t.Errorf("parseIntValue(%q) = (%d, %v), want (%d, %v)", tt.line, v, ok, tt.wantV, tt.wantOk)
		}
	}
}

func TestParseInt64Value(t *testing.T) {
	v, ok := parseInt64Value("liquidity_floor: 1000000")
	if !ok || v != 1000000 {
		t.Errorf("parseInt64Value = (%d, %v), want (1000000, true)", v, ok)
	}
	_, ok = parseInt64Value("liquidity_floor: abc")
	if ok {
		t.Error("expected false for invalid input")
	}
}

func TestParsePctFloatValue(t *testing.T) {
	v, ok := parsePctFloatValue("stop_loss_pct: -0.05")
	if !ok || v != -0.05 {
		t.Errorf("parsePctFloatValue = (%f, %v), want (-0.05, true)", v, ok)
	}
	_, ok = parsePctFloatValue("stop_loss_pct: abc")
	if ok {
		t.Error("expected false for invalid input")
	}
}

func TestExecutionPolicyFromConstraints_DefaultFloor(t *testing.T) {
	c := domain.SimulationConstraints{MinRecommendationConviction: 0}
	ep := ExecutionPolicyFromConstraints(c)
	if ep.ConvictionFloor <= 0 {
		t.Errorf("expected positive conviction floor when input is 0, got %d", ep.ConvictionFloor)
	}
}

func TestExecutionPolicyFromConstraints_PreserveFloor(t *testing.T) {
	c := domain.SimulationConstraints{MinRecommendationConviction: 15, RequireCROPass: true}
	ep := ExecutionPolicyFromConstraints(c)
	if ep.ConvictionFloor != 15 {
		t.Errorf("ConvictionFloor = %d, want 15", ep.ConvictionFloor)
	}
	if !ep.RequireCROPass {
		t.Error("RequireCROPass should be true")
	}
}

func TestApplyConstraintCandidate(t *testing.T) {
	base := domain.SimulationConstraints{}
	result := ApplyConstraintCandidate(base, "conviction_floor: 10\nrequire_cro_pass: true")
	if result.MinRecommendationConviction != 10 {
		t.Errorf("MinRecommendationConviction = %d, want 10", result.MinRecommendationConviction)
	}
	if !result.RequireCROPass {
		t.Error("RequireCROPass should be true")
	}
}

func TestSaveWithLock_WritesPolicyFileAtomically(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "baseline_policy.json")

	policy := DefaultPolicy()
	policy.Version = 5
	policy.PromptOverrides = map[string]string{"agent-1": "v5 prompt"}

	if err := SaveWithLock(policyPath, policy); err != nil {
		t.Fatalf("SaveWithLock: %v", err)
	}

	if _, err := os.Stat(policyPath); err != nil {
		t.Fatalf("policy file not created: %v", err)
	}

	loaded, err := Load(policyPath)
	if err != nil {
		t.Fatalf("load saved policy: %v", err)
	}
	if loaded.Version != 5 {
		t.Errorf("Version = %d, want 5", loaded.Version)
	}
	if loaded.PromptOverrides["agent-1"] != "v5 prompt" {
		t.Errorf("PromptOverrides[agent-1] = %q, want %q", loaded.PromptOverrides["agent-1"], "v5 prompt")
	}

	if _, err := os.Stat(policyPath + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("expected .tmp to be cleaned up, got err=%v", err)
	}
	if _, err := os.Stat(policyPath + ".bak"); !os.IsNotExist(err) {
		t.Errorf("expected .bak to be cleaned up, got err=%v", err)
	}
}

func TestSaveWithLock_OverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "baseline_policy.json")

	p1 := DefaultPolicy()
	p1.Version = 1
	p1.PromptOverrides = map[string]string{"agent-1": "v1"}
	if err := SaveWithLock(policyPath, p1); err != nil {
		t.Fatalf("first SaveWithLock: %v", err)
	}

	p2 := DefaultPolicy()
	p2.Version = 2
	p2.PromptOverrides = map[string]string{"agent-1": "v2"}
	if err := SaveWithLock(policyPath, p2); err != nil {
		t.Fatalf("second SaveWithLock: %v", err)
	}

	loaded, err := Load(policyPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Version != 2 {
		t.Errorf("Version = %d, want 2", loaded.Version)
	}
	if loaded.PromptOverrides["agent-1"] != "v2" {
		t.Errorf("PromptOverrides[agent-1] = %q, want %q", loaded.PromptOverrides["agent-1"], "v2")
	}
}

func TestSaveWithLock_RecoversFromStaleTmpFile(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "baseline_policy.json")

	p1 := DefaultPolicy()
	p1.Version = 1
	if err := SaveWithLock(policyPath, p1); err != nil {
		t.Fatalf("initial SaveWithLock: %v", err)
	}

	if err := os.WriteFile(policyPath+".tmp", []byte("garbage{{{"), 0o644); err != nil {
		t.Fatalf("write corrupt tmp: %v", err)
	}

	p2 := DefaultPolicy()
	p2.Version = 2
	if err := SaveWithLock(policyPath, p2); err != nil {
		t.Fatalf("SaveWithLock with corrupt tmp: %v", err)
	}

	loaded, err := Load(policyPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Version != 2 {
		t.Errorf("Version = %d, want 2", loaded.Version)
	}
}

func TestSaveWithLock_CreatesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "nested", "subdir", "baseline_policy.json")

	policy := DefaultPolicy()
	policy.Version = 7

	if err := SaveWithLock(policyPath, policy); err != nil {
		t.Fatalf("SaveWithLock with nested path: %v", err)
	}

	if _, err := os.Stat(policyPath); err != nil {
		t.Fatalf("policy file not created in nested dir: %v", err)
	}
}

func TestSaveWithLock_EmptyPath(t *testing.T) {
	if err := SaveWithLock("", DefaultPolicy()); err == nil {
		t.Error("expected error for empty path, got nil")
	}
}
