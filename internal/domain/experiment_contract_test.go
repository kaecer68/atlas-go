package domain

import (
	"strings"
	"testing"
)

func TestMutationBriefNormalizeAndValidate_DefaultContractVersion(t *testing.T) {
	brief := MutationBrief{
		WindowID:         "window-1",
		TargetAgentID:    "growth-momentum-01",
		TargetSkill:      "growth_momentum",
		TargetLayer:      LayerStyle,
		PromptFile:       "prompts/agents/growth_momentum.md",
		MutationType:     "prompt_tightening",
		AcceptanceMetric: "sharpe_like",
		AcceptanceGates:  []string{"improve_sharpe_like"},
	}

	if err := brief.NormalizeAndValidate(); err != nil {
		t.Fatalf("expected valid brief, got error: %v", err)
	}
	if brief.ContractVersion != MutationBriefContractVersion {
		t.Fatalf("expected default contract version %d, got %d", MutationBriefContractVersion, brief.ContractVersion)
	}
	if brief.ProposalID == "" {
		t.Fatalf("expected proposal id to be normalized")
	}
}

func TestMutationBriefNormalizeAndValidate_RejectsInvalidMutationType(t *testing.T) {
	brief := MutationBrief{
		ContractVersion:  MutationBriefContractVersion,
		WindowID:         "window-1",
		TargetAgentID:    "growth-momentum-01",
		TargetSkill:      "growth_momentum",
		TargetLayer:      LayerStyle,
		PromptFile:       "prompts/agents/growth_momentum.md",
		MutationType:     "unknown_type",
		AcceptanceMetric: "sharpe_like",
		AcceptanceGates:  []string{"improve_sharpe_like"},
	}

	err := brief.NormalizeAndValidate()
	if err == nil {
		t.Fatalf("expected error for invalid mutation type")
	}
	if !strings.Contains(err.Error(), "mutation_type") {
		t.Fatalf("expected mutation_type error, got %v", err)
	}
}

func TestPromptExperimentResultNormalizeAndValidateForJudge_FillsExperimentDefaults(t *testing.T) {
	result := PromptExperimentResult{
		Experiment: ExperimentRecord{
			ID: "exp-1",
		},
		Brief: MutationBrief{
			WindowID:         "window-1",
			TargetAgentID:    "growth-momentum-01",
			TargetSkill:      "growth_momentum",
			TargetLayer:      LayerStyle,
			PromptFile:       "prompts/agents/growth_momentum.md",
			MutationType:     "prompt_tightening",
			AcceptanceMetric: "sharpe_like",
			AcceptanceGates:  []string{"improve_sharpe_like"},
		},
		CandidatePrompt: "prompts/experiments/growth-momentum-01/exp-1/v2.md",
	}

	if err := result.NormalizeAndValidateForJudge(); err != nil {
		t.Fatalf("expected valid result, got error: %v", err)
	}
	if result.Experiment.TargetAgentID != result.Brief.TargetAgentID {
		t.Fatalf("expected target agent default to brief")
	}
	if result.Experiment.Skill != result.Brief.TargetSkill {
		t.Fatalf("expected skill default to brief")
	}
	if result.Experiment.MutationType != result.Brief.MutationType {
		t.Fatalf("expected mutation type default to brief")
	}
	if result.Experiment.ProposalID == "" {
		t.Fatalf("expected proposal id to be normalized")
	}
	if result.Experiment.CommitID == "" {
		t.Fatalf("expected commit id to be normalized")
	}
}
