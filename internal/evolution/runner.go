package evolution

import (
	"fmt"
	"slices"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type Candidate struct {
	Agent      domain.AgentSpec
	Scorecard  domain.Scorecard
	Experiment domain.ExperimentRecord
}

func SelectWeakestAgent(registry domain.AgentRegistry, scorecards []domain.Scorecard) *Candidate {
	if len(scorecards) == 0 {
		return nil
	}

	byID := make(map[string]domain.AgentSpec, len(registry.Agents))
	for _, agent := range registry.Agents {
		byID[agent.ID] = agent
	}

	ordered := slices.Clone(scorecards)
	slices.SortFunc(ordered, func(a, b domain.Scorecard) int {
		aAgent, aOK := byID[a.AgentID]
		bAgent, bOK := byID[b.AgentID]
		aPriority := layerPriority(aAgent, aOK)
		bPriority := layerPriority(bAgent, bOK)
		switch {
		case aPriority < bPriority:
			return -1
		case aPriority > bPriority:
			return 1
		case a.WindowCount < b.WindowCount:
			return -1
		case a.WindowCount > b.WindowCount:
			return 1
		case a.SharpeLike < b.SharpeLike:
			return -1
		case a.SharpeLike > b.SharpeLike:
			return 1
		default:
			return 0
		}
	})

	for _, scorecard := range ordered {
		agent, ok := byID[scorecard.AgentID]
		if !ok || !agent.Enabled {
			continue
		}
		scorecard.Layer = agent.Layer
		return &Candidate{
			Agent:     agent,
			Scorecard: scorecard,
			Experiment: domain.ExperimentRecord{
				ID:                fmt.Sprintf("exp-%s-%d", agent.ID, time.Now().Unix()),
				TargetAgentID:     agent.ID,
				Skill:             agent.Skill,
				Hypothesis:        "A targeted prompt refinement can improve risk-adjusted recommendation quality.",
				PromptVersionFrom: "v1",
				PromptVersionTo:   "v2",
				MutationType:      "prompt_tightening",
				AcceptanceGates:   []string{"improve_sharpe_like", "no_material_drawdown_degradation", "no_constraint_bypass"},
				WindowStart:       time.Now().AddDate(0, 0, -20),
				WindowEnd:         time.Now(),
				AcceptanceMetric:  "sharpe_like",
				BaselineValue:     scorecard.SharpeLike,
				Status:            domain.ExperimentPlanned,
			},
		}
	}

	return nil
}

func layerPriority(agent domain.AgentSpec, ok bool) int {
	if !ok {
		return 99
	}
	switch agent.Layer {
	case domain.LayerSector, domain.LayerStyle:
		return 0
	case domain.LayerContext:
		return 1
	case domain.LayerControl:
		return 2
	default:
		return 3
	}
}

func BuildMutationBrief(windowID string, candidate *Candidate) *domain.MutationBrief {
	if candidate == nil {
		return nil
	}

	return &domain.MutationBrief{
		WindowID:            windowID,
		TargetAgentID:       candidate.Agent.ID,
		TargetSkill:         candidate.Agent.Skill,
		TargetLayer:         candidate.Agent.Layer,
		PromptFile:          candidate.Agent.PromptFile,
		MutationType:        candidate.Experiment.MutationType,
		FailurePattern:      "Repeated negative risk-adjusted outcomes across replay sessions.",
		Hypothesis:          candidate.Experiment.Hypothesis,
		AcceptanceMetric:    candidate.Experiment.AcceptanceMetric,
		AcceptanceGates:     candidate.Experiment.AcceptanceGates,
		ForbiddenActions:    candidate.Agent.ForbiddenActions,
		RequiredSkills:      candidate.Agent.RequiredSkills,
		ObservedWindowCount: candidate.Scorecard.WindowCount,
		MaturityLevel:       maturityLevel(candidate.Scorecard.WindowCount),
		IterationGuidance:   iterationGuidance(candidate.Agent.Layer, candidate.Scorecard.WindowCount),
		RecommendedWindow:   recommendedWindow(candidate.Scorecard.WindowCount),
		GeneratedAt:         time.Now(),
	}
}

func maturityLevel(windowCount int) string {
	switch {
	case windowCount >= 5:
		return "level_3_regime_aware"
	case windowCount >= 3:
		return "level_2_window_validated"
	default:
		return "level_1_exploratory"
	}
}

func recommendedWindow(windowCount int) string {
	switch {
	case windowCount >= 5:
		return "next cross-regime replay window"
	case windowCount >= 3:
		return "next multi-session replay window"
	default:
		return "next short validation window before broader promotion"
	}
}

func iterationGuidance(layer domain.AgentLayer, windowCount int) []string {
	guidance := []string{
		"Change one bounded behavior only.",
		"Preserve required skills and forbidden action boundaries.",
	}

	switch layer {
	case domain.LayerSector:
		guidance = append(guidance,
			"Refine sector thesis quality and symbol qualification, not portfolio sizing.",
			"Prefer industry-specific evidence over broad market narration.",
		)
	case domain.LayerStyle:
		guidance = append(guidance,
			"Tighten entry filters and false-positive control before changing role identity.",
			"Do not smuggle sector or portfolio policy into a style mutation.",
		)
	case domain.LayerContext:
		guidance = append(guidance,
			"Adjust regime interpretation carefully because context changes influence many downstream agents.",
			"Prefer threshold refinement over wholesale narrative rewrites.",
		)
	case domain.LayerControl:
		guidance = append(guidance,
			"Treat control-layer mutations as conservative governance changes.",
			"Do not widen risk limits unless replay evidence is unusually strong.",
		)
	}

	if windowCount < 3 {
		guidance = append(guidance,
			"Evidence is still thin; prefer conservative prompt tightening over ambitious rewrites.",
		)
	} else {
		guidance = append(guidance,
			"Evidence coverage is adequate for a focused replay challenge.",
		)
	}

	return guidance
}
