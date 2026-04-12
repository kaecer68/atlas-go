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

func TestSelectWeakestAgentExcludingExtinct(t *testing.T) {
	reg := domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "a", Skill: "alpha", Layer: domain.LayerStyle, Enabled: true},
			{ID: "b", Skill: "beta", Layer: domain.LayerStyle, Enabled: true},
		},
	}
	scorecards := []domain.Scorecard{
		{AgentID: "a", SharpeLike: -0.4, WindowCount: 2},
		{AgentID: "b", SharpeLike: -0.1, WindowCount: 2},
	}

	extinct := map[string]bool{"a": true}
	candidate := SelectWeakestAgentExcluding(reg, scorecards, extinct)
	if candidate == nil || candidate.Agent.ID != "b" {
		t.Fatalf("expected b because a is extinct, got %+v", candidate)
	}
}

func TestSelectBestSpawnedAgent(t *testing.T) {
	reg := domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "spawn_1", Skill: "sector_bio", Layer: domain.LayerSector, Enabled: true},
			{ID: "baseline_1", Skill: "tech", Layer: domain.LayerSector, Enabled: true},
		},
	}
	scorecards := []domain.Scorecard{
		{AgentID: "spawn_1", SharpeLike: 0.8, WindowCount: 5},
		{AgentID: "baseline_1", SharpeLike: 0.5, WindowCount: 10},
	}

	spawned := map[string]bool{"spawn_1": true}
	candidate := SelectBestSpawnedAgent(reg, scorecards, spawned, 0.5)
	if candidate == nil || candidate.Agent.ID != "spawn_1" {
		t.Fatalf("expected spawn_1 to be selected for promotion, got %+v", candidate)
	}
	if candidate.Experiment.MutationType != "promote_spawned" {
		t.Fatalf("expected promote_spawned mutation type, got %s", candidate.Experiment.MutationType)
	}
}

func TestSelectBestSpawnedAgentIgnoresBelowBaseline(t *testing.T) {
	reg := domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "spawn_1", Skill: "sector_bio", Layer: domain.LayerSector, Enabled: true},
		},
	}
	scorecards := []domain.Scorecard{
		{AgentID: "spawn_1", SharpeLike: 0.4, WindowCount: 5},
	}

	spawned := map[string]bool{"spawn_1": true}
	candidate := SelectBestSpawnedAgent(reg, scorecards, spawned, 0.5)
	if candidate != nil {
		t.Fatalf("expected no candidate when spawned agent is below baseline")
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
			ID:               "exp-style-1",
			ProposalID:       "proposal-exp-style-1",
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
	if brief.ProposalID != "proposal-exp-style-1" {
		t.Fatalf("expected proposal id to be preserved, got %q", brief.ProposalID)
	}
}
