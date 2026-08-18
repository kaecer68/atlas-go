package experiment

import (
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain/shared"
)

func TestIsValidAgentLayer(t *testing.T) {
	valid := []shared.AgentLayer{
		shared.LayerContext,
		shared.LayerMacro,
		shared.LayerSector,
		shared.LayerStyle,
		shared.LayerSuperinvestor,
		shared.LayerControl,
	}
	for _, layer := range valid {
		if !isValidAgentLayer(layer) {
			t.Errorf("isValidAgentLayer(%q) = false, want true", layer)
		}
	}

	invalid := []shared.AgentLayer{
		"",
		"unknown",
		"invalid",
		"quant",
	}
	for _, layer := range invalid {
		if isValidAgentLayer(layer) {
			t.Errorf("isValidAgentLayer(%q) = true, want false", layer)
		}
	}
}

func validBrief() *MutationBrief {
	return &MutationBrief{
		ContractVersion:     MutationBriefContractVersion,
		WindowID:            "window-001",
		TargetAgentID:       "agent-001",
		TargetSkill:         "momentum",
		TargetLayer:         shared.LayerSector,
		PromptFile:          "prompts/agents/agent-001.md",
		MutationType:        "prompt_tightening",
		Hypothesis:          "test",
		AcceptanceMetric:    "sharpe_ratio",
		AcceptanceGates:     []string{"oos_performance"},
		ObservedWindowCount: 5,
	}
}

func TestMutationBrief_NormalizeAndValidate(t *testing.T) {
	t.Run("nil brief returns error", func(t *testing.T) {
		err := (*MutationBrief)(nil).NormalizeAndValidate()
		if err == nil {
			t.Fatal("expected error for nil brief")
		}
	})

	t.Run("valid brief succeeds", func(t *testing.T) {
		b := validBrief()
		err := b.NormalizeAndValidate()
		if err != nil {
			t.Fatalf("unexpected error for valid brief: %v", err)
		}
	})

	t.Run("contract_version 0 gets filled to current", func(t *testing.T) {
		b := validBrief()
		b.ContractVersion = 0
		err := b.NormalizeAndValidate()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b.ContractVersion != MutationBriefContractVersion {
			t.Errorf("expected contract_version %d, got %d", MutationBriefContractVersion, b.ContractVersion)
		}
	})

	t.Run("unsupported contract_version returns error", func(t *testing.T) {
		b := validBrief()
		b.ContractVersion = 99
		err := b.NormalizeAndValidate()
		if err == nil {
			t.Fatal("expected error for unsupported contract_version")
		}
		if !strings.Contains(err.Error(), "unsupported contract_version") {
			t.Errorf("error message should mention unsupported contract_version, got: %v", err)
		}
	})

	t.Run("empty window_id returns error", func(t *testing.T) {
		b := validBrief()
		b.WindowID = ""
		err := b.NormalizeAndValidate()
		if err == nil {
			t.Fatal("expected error for empty window_id")
		}
	})

	t.Run("whitespace-only window_id returns error", func(t *testing.T) {
		b := validBrief()
		b.WindowID = "   "
		err := b.NormalizeAndValidate()
		if err == nil {
			t.Fatal("expected error for whitespace-only window_id")
		}
	})

	t.Run("empty proposal_id gets auto-generated", func(t *testing.T) {
		b := validBrief()
		b.ProposalID = ""
		err := b.NormalizeAndValidate()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b.ProposalID != "proposal-window-001" {
			t.Errorf("expected auto-generated proposal_id 'proposal-window-001', got %q", b.ProposalID)
		}
	})

	t.Run("existing proposal_id is preserved", func(t *testing.T) {
		b := validBrief()
		b.ProposalID = "custom-proposal"
		err := b.NormalizeAndValidate()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b.ProposalID != "custom-proposal" {
			t.Errorf("expected proposal_id to be preserved as 'custom-proposal', got %q", b.ProposalID)
		}
	})

	t.Run("empty target_agent_id returns error", func(t *testing.T) {
		b := validBrief()
		b.TargetAgentID = ""
		err := b.NormalizeAndValidate()
		if err == nil {
			t.Fatal("expected error for empty target_agent_id")
		}
	})

	t.Run("empty target_skill returns error", func(t *testing.T) {
		b := validBrief()
		b.TargetSkill = ""
		err := b.NormalizeAndValidate()
		if err == nil {
			t.Fatal("expected error for empty target_skill")
		}
	})

	t.Run("invalid target_layer returns error", func(t *testing.T) {
		b := validBrief()
		b.TargetLayer = "bogus"
		err := b.NormalizeAndValidate()
		if err == nil {
			t.Fatal("expected error for invalid target_layer")
		}
	})

	t.Run("empty prompt_file returns error", func(t *testing.T) {
		b := validBrief()
		b.PromptFile = ""
		err := b.NormalizeAndValidate()
		if err == nil {
			t.Fatal("expected error for empty prompt_file")
		}
	})

	t.Run("invalid mutation_type returns error", func(t *testing.T) {
		b := validBrief()
		b.MutationType = "unknown_mutation"
		err := b.NormalizeAndValidate()
		if err == nil {
			t.Fatal("expected error for invalid mutation_type")
		}
	})

	t.Run("all valid mutation types pass", func(t *testing.T) {
		validTypes := []string{
			"prompt_tightening",
			"risk_rule_change",
			"portfolio_constraint_revision",
			"onboarding",
			"promote_spawned",
			"auto_prompt_optimization",
		}
		for _, mt := range validTypes {
			b := validBrief()
			b.MutationType = mt
			err := b.NormalizeAndValidate()
			if err != nil {
				t.Errorf("unexpected error for mutation_type %q: %v", mt, err)
			}
		}
	})

	t.Run("empty acceptance_metric returns error", func(t *testing.T) {
		b := validBrief()
		b.AcceptanceMetric = ""
		err := b.NormalizeAndValidate()
		if err == nil {
			t.Fatal("expected error for empty acceptance_metric")
		}
	})

	t.Run("empty acceptance_gates returns error", func(t *testing.T) {
		b := validBrief()
		b.AcceptanceGates = nil
		err := b.NormalizeAndValidate()
		if err == nil {
			t.Fatal("expected error for nil acceptance_gates")
		}

		b.AcceptanceGates = []string{}
		err = b.NormalizeAndValidate()
		if err == nil {
			t.Fatal("expected error for empty acceptance_gates")
		}
	})

	t.Run("negative observed_window_count returns error", func(t *testing.T) {
		b := validBrief()
		b.ObservedWindowCount = -1
		err := b.NormalizeAndValidate()
		if err == nil {
			t.Fatal("expected error for negative observed_window_count")
		}
	})

	t.Run("zero observed_window_count is valid", func(t *testing.T) {
		b := validBrief()
		b.ObservedWindowCount = 0
		err := b.NormalizeAndValidate()
		if err != nil {
			t.Fatalf("unexpected error for zero observed_window_count: %v", err)
		}
	})

	t.Run("invalid maturity_level returns error", func(t *testing.T) {
		b := validBrief()
		b.MaturityLevel = "bogus_level"
		err := b.NormalizeAndValidate()
		if err == nil {
			t.Fatal("expected error for invalid maturity_level")
		}
	})

	t.Run("empty maturity_level is valid", func(t *testing.T) {
		b := validBrief()
		b.MaturityLevel = ""
		err := b.NormalizeAndValidate()
		if err != nil {
			t.Fatalf("unexpected error for empty maturity_level: %v", err)
		}
	})

	t.Run("all valid maturity_levels pass", func(t *testing.T) {
		validLevels := []string{
			"level_1_exploratory",
			"level_2_validated",
			"level_2_window_validated",
			"level_3_regime_aware",
		}
		for _, ml := range validLevels {
			b := validBrief()
			b.MaturityLevel = ml
			err := b.NormalizeAndValidate()
			if err != nil {
				t.Errorf("unexpected error for maturity_level %q: %v", ml, err)
			}
		}
	})

	t.Run("all valid agent layers pass", func(t *testing.T) {
		layers := []shared.AgentLayer{
			shared.LayerContext,
			shared.LayerMacro,
			shared.LayerSector,
			shared.LayerStyle,
			shared.LayerSuperinvestor,
			shared.LayerControl,
		}
		for _, layer := range layers {
			b := validBrief()
			b.TargetLayer = layer
			err := b.NormalizeAndValidate()
			if err != nil {
				t.Errorf("unexpected error for agent layer %q: %v", layer, err)
			}
		}
	})

	t.Run("whitespace proposal_id generates clean id", func(t *testing.T) {
		b := validBrief()
		b.ProposalID = ""
		b.WindowID = "  window-002  "
		err := b.NormalizeAndValidate()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "proposal-window-002"
		if b.ProposalID != expected {
			t.Errorf("expected %q, got %q", expected, b.ProposalID)
		}
	})
}

func TestPromptExperimentResult_NormalizeAndValidateForJudge(t *testing.T) {
	t.Run("nil result returns error", func(t *testing.T) {
		err := (*PromptExperimentResult)(nil).NormalizeAndValidateForJudge()
		if err == nil {
			t.Fatal("expected error for nil result")
		}
	})

	t.Run("empty experiment id returns error", func(t *testing.T) {
		r := &PromptExperimentResult{
			Experiment:      ExperimentRecord{ID: ""},
			Brief:           *validBrief(),
			CandidatePrompt: "some prompt content",
		}
		err := r.NormalizeAndValidateForJudge()
		if err == nil {
			t.Fatal("expected error for empty experiment.id")
		}
	})

	t.Run("empty candidate_prompt returns error", func(t *testing.T) {
		r := &PromptExperimentResult{
			Experiment:      ExperimentRecord{ID: "exp-001"},
			Brief:           *validBrief(),
			CandidatePrompt: "",
		}
		err := r.NormalizeAndValidateForJudge()
		if err == nil {
			t.Fatal("expected error for empty candidate_prompt")
		}
	})

	t.Run("invalid brief propagates error", func(t *testing.T) {
		r := &PromptExperimentResult{
			Experiment:      ExperimentRecord{ID: "exp-001"},
			Brief:           MutationBrief{WindowID: ""}, // missing required fields
			CandidatePrompt: "some prompt content",
		}
		err := r.NormalizeAndValidateForJudge()
		if err == nil {
			t.Fatal("expected error from invalid brief")
		}
	})

	t.Run("valid result succeeds", func(t *testing.T) {
		r := &PromptExperimentResult{
			Experiment:      ExperimentRecord{ID: "exp-001"},
			Brief:           *validBrief(),
			CandidatePrompt: "some prompt content",
		}
		err := r.NormalizeAndValidateForJudge()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("auto-fill experiment fields from brief when empty", func(t *testing.T) {
		r := &PromptExperimentResult{
			Experiment:      ExperimentRecord{ID: "exp-002"},
			Brief:           *validBrief(),
			CandidatePrompt: "some prompt content",
		}
		err := r.NormalizeAndValidateForJudge()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if r.Experiment.TargetAgentID != r.Brief.TargetAgentID {
			t.Errorf("expected TargetAgentID %q, got %q", r.Brief.TargetAgentID, r.Experiment.TargetAgentID)
		}
		if r.Experiment.ProposalID != r.Brief.ProposalID {
			t.Errorf("expected ProposalID %q, got %q", r.Brief.ProposalID, r.Experiment.ProposalID)
		}
		if r.Experiment.Skill != r.Brief.TargetSkill {
			t.Errorf("expected Skill %q, got %q", r.Brief.TargetSkill, r.Experiment.Skill)
		}
		if r.Experiment.MutationType != r.Brief.MutationType {
			t.Errorf("expected MutationType %q, got %q", r.Brief.MutationType, r.Experiment.MutationType)
		}
		if r.Experiment.AcceptanceMetric != r.Brief.AcceptanceMetric {
			t.Errorf("expected AcceptanceMetric %q, got %q", r.Brief.AcceptanceMetric, r.Experiment.AcceptanceMetric)
		}
	})

	t.Run("preserve existing experiment fields", func(t *testing.T) {
		r := &PromptExperimentResult{
			Experiment: ExperimentRecord{
				ID:               "exp-003",
				TargetAgentID:    "existing-agent",
				ProposalID:       "existing-proposal",
				Skill:            "existing-skill",
				MutationType:     "risk_rule_change",
				AcceptanceGates:  []string{"custom_gate"},
				AcceptanceMetric: "sortino",
			},
			Brief:           *validBrief(),
			CandidatePrompt: "some prompt content",
		}
		err := r.NormalizeAndValidateForJudge()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if r.Experiment.TargetAgentID != "existing-agent" {
			t.Errorf("expected TargetAgentID to be preserved, got %q", r.Experiment.TargetAgentID)
		}
		if r.Experiment.ProposalID != "existing-proposal" {
			t.Errorf("expected ProposalID to be preserved, got %q", r.Experiment.ProposalID)
		}
		if r.Experiment.Skill != "existing-skill" {
			t.Errorf("expected Skill to be preserved, got %q", r.Experiment.Skill)
		}
		if r.Experiment.MutationType != "risk_rule_change" {
			t.Errorf("expected MutationType to be preserved, got %q", r.Experiment.MutationType)
		}
		if r.Experiment.AcceptanceMetric != "sortino" {
			t.Errorf("expected AcceptanceMetric to be preserved, got %q", r.Experiment.AcceptanceMetric)
		}
		if len(r.Experiment.AcceptanceGates) != 1 || r.Experiment.AcceptanceGates[0] != "custom_gate" {
			t.Errorf("expected AcceptanceGates to be preserved, got %v", r.Experiment.AcceptanceGates)
		}
	})

	t.Run("auto-fill commit_id when empty", func(t *testing.T) {
		r := &PromptExperimentResult{
			Experiment:      ExperimentRecord{ID: "exp-004"},
			Brief:           *validBrief(),
			CandidatePrompt: "some prompt content",
		}
		err := r.NormalizeAndValidateForJudge()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if r.Experiment.CommitID != "commit-exp-004" {
			t.Errorf("expected CommitID 'commit-exp-004', got %q", r.Experiment.CommitID)
		}
	})

	t.Run("existing commit_id is preserved", func(t *testing.T) {
		r := &PromptExperimentResult{
			Experiment:      ExperimentRecord{ID: "exp-005", CommitID: "existing-commit"},
			Brief:           *validBrief(),
			CandidatePrompt: "some prompt content",
		}
		err := r.NormalizeAndValidateForJudge()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if r.Experiment.CommitID != "existing-commit" {
			t.Errorf("expected CommitID to be preserved, got %q", r.Experiment.CommitID)
		}
	})

	t.Run("invalid experiment.mutation_type returns error", func(t *testing.T) {
		r := &PromptExperimentResult{
			Experiment: ExperimentRecord{
				ID:           "exp-006",
				MutationType: "bogus_mutation",
			},
			Brief:           *validBrief(),
			CandidatePrompt: "some prompt content",
		}
		err := r.NormalizeAndValidateForJudge()
		if err == nil {
			t.Fatal("expected error for invalid experiment.mutation_type")
		}
	})
}

// TestIsSupportedMutationType anchors the whitelist contract so future mutation
// types have a single testable entry point (k3 correction: export the helper,
// not just extend the map).
func TestIsSupportedMutationType(t *testing.T) {
	valid := []string{
		"prompt_tightening",
		"risk_rule_change",
		"portfolio_constraint_revision",
		// N4: lifecycle mutations produced by other subsystems must be accepted.
		"onboarding",
		"promote_spawned",
		"auto_prompt_optimization",
	}
	for _, mt := range valid {
		if !IsSupportedMutationType(mt) {
			t.Errorf("IsSupportedMutationType(%q) = false, want true", mt)
		}
	}

	invalid := []string{
		"",
		"unknown_mutation",
		"bogus_mutation",
		"prompt_tightening ",
		"onboarding-v2",
		"PROMOTE_SPAWNED",
	}
	for _, mt := range invalid {
		if IsSupportedMutationType(mt) {
			t.Errorf("IsSupportedMutationType(%q) = true, want false", mt)
		}
	}
}
