package evolution

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestSelectWeakestAgent(t *testing.T) {
	reg := domain.AgentRegistry{
		Version: 1,
		Agents: []domain.AgentSpec{
			{ID: "a", Skill: "alpha", Layer: domain.LayerControl, Enabled: true},
			{ID: "b", Skill: "beta", Layer: domain.LayerStyle, Enabled: true},
		},
	}
	scorecards := []domain.Scorecard{
		{AgentID: "a", SharpeLike: -0.4, WindowCount: 2},
		{AgentID: "b", SharpeLike: -0.1, WindowCount: 2},
	}
	candidate := SelectWeakestAgent(reg, scorecards)
	if candidate == nil || candidate.Agent.ID != "b" {
		t.Fatalf("expected weakest optimizable agent b")
	}
}

func TestBuildMutationBriefCarriesLayerAndEvidence(t *testing.T) {
	candidate := &Candidate{
		Agent: domain.AgentSpec{
			ID:         "style-1",
			Skill:      "growth_momentum",
			Layer:      domain.LayerStyle,
			PromptFile: "prompts/agents/growth_momentum.md",
		},
		Scorecard: domain.Scorecard{
			AgentID:     "style-1",
			Skill:       "growth_momentum",
			Layer:       domain.LayerStyle,
			WindowCount: 2,
			SharpeLike:  -0.3,
		},
		Experiment: domain.ExperimentRecord{
			MutationType:     "prompt_tightening",
			AcceptanceMetric: "sharpe_like",
			AcceptanceGates:  []string{"improve_sharpe_like"},
			Hypothesis:       "Tighten entries.",
		},
	}

	brief := BuildMutationBrief("window-1", candidate)
	if brief == nil {
		t.Fatalf("expected mutation brief")
	}
	if brief.TargetLayer != domain.LayerStyle {
		t.Fatalf("expected style layer, got %s", brief.TargetLayer)
	}
	if brief.ObservedWindowCount != 2 {
		t.Fatalf("expected window count 2, got %d", brief.ObservedWindowCount)
	}
	if brief.MaturityLevel == "" {
		t.Fatalf("expected maturity level")
	}
	if len(brief.IterationGuidance) == 0 {
		t.Fatalf("expected iteration guidance")
	}
}
