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
	v, ok = parseInt64Value("liquidity_floor: abc")
	if ok {
		t.Error("expected false for invalid input")
	}
}

func TestParsePctFloatValue(t *testing.T) {
	v, ok := parsePctFloatValue("stop_loss_pct: -0.05")
	if !ok || v != -0.05 {
		t.Errorf("parsePctFloatValue = (%f, %v), want (-0.05, true)", v, ok)
	}
	v, ok = parsePctFloatValue("stop_loss_pct: abc")
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
