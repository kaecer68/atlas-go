package domain

import (
	"fmt"
	"slices"
	"time"
)

type Candidate struct {
	Agent      AgentSpec
	Scorecard  Scorecard
	Experiment ExperimentRecord
}

func SelectWeakestAgent(registry AgentRegistry, scorecards []Scorecard) *Candidate {
	return SelectWeakestAgentExcluding(registry, scorecards, nil)
}

func SelectWeakestAgentExcluding(registry AgentRegistry, scorecards []Scorecard, extinctAgentIDs map[string]bool) *Candidate {
	if len(scorecards) == 0 {
		return nil
	}
	byID := make(map[string]AgentSpec, len(registry.Agents))
	for _, a := range registry.Agents {
		byID[a.ID] = a
	}
	ord := slices.Clone(scorecards)
	slices.SortFunc(ord, func(a, b Scorecard) int {
		aA, aO := byID[a.AgentID]
		bA, bO := byID[b.AgentID]
		aP := clp(aA, aO)
		bP := clp(bA, bO)
		switch {
		case aP < bP:
			return -1
		case aP > bP:
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
	for _, sc := range ord {
		ag, ok := byID[sc.AgentID]
		if !ok || !ag.Enabled {
			continue
		}
		if extinctAgentIDs != nil && extinctAgentIDs[ag.ID] {
			continue
		}
		sc.Layer = ag.Layer
		mt, hp, gt := smt(ag, sc)
		eid := fmt.Sprintf("exp-%s-%d", ag.ID, time.Now().Unix())
		return &Candidate{Agent: ag, Scorecard: sc, Experiment: ExperimentRecord{ID: eid, ProposalID: "proposal-" + eid, TargetAgentID: ag.ID, Skill: ag.Skill, Hypothesis: hp, PromptVersionFrom: "v1", PromptVersionTo: "v2", MutationType: mt, AcceptanceGates: gt, WindowStart: time.Now().AddDate(0, 0, -20), WindowEnd: time.Now(), AcceptanceMetric: "sharpe_like", BaselineValue: sc.SharpeLike, Status: ExperimentPlanned}}
	}
	return nil
}

func SelectBestSpawnedAgent(registry AgentRegistry, scorecards []Scorecard, spawnedAgentIDs map[string]bool, baselineSharpe float64) *Candidate {
	if len(scorecards) == 0 || len(spawnedAgentIDs) == 0 {
		return nil
	}
	byID := make(map[string]AgentSpec, len(registry.Agents))
	for _, a := range registry.Agents {
		byID[a.ID] = a
	}
	var best *Candidate
	for _, sc := range scorecards {
		ag, ok := byID[sc.AgentID]
		if !ok || !ag.Enabled || !spawnedAgentIDs[ag.ID] {
			continue
		}
		if sc.SharpeLike <= baselineSharpe {
			continue
		}
		if best == nil || sc.SharpeLike > best.Scorecard.SharpeLike {
			eid := fmt.Sprintf("promote-%s-%d", ag.ID, time.Now().Unix())
			best = &Candidate{Agent: ag, Scorecard: sc, Experiment: ExperimentRecord{ID: eid, ProposalID: "proposal-" + eid, TargetAgentID: ag.ID, Skill: ag.Skill, Hypothesis: "Promote spawned agent.", PromptVersionFrom: "v1", PromptVersionTo: "v1", MutationType: "promote_spawned", AcceptanceGates: []string{"maintain_sharpe_like", "no_drawdown_spike", "factor_quality"}, WindowStart: time.Now().AddDate(0, 0, -20), WindowEnd: time.Now(), AcceptanceMetric: "sharpe_like", BaselineValue: sc.SharpeLike, Status: ExperimentPlanned}}
		}
	}
	return best
}

func BuildMutationBrief(windowID string, candidate *Candidate) *MutationBrief {
	if candidate == nil {
		return nil
	}
	return &MutationBrief{ContractVersion: MutationBriefContractVersion, ProposalID: pidf(candidate.Experiment), WindowID: windowID, TargetAgentID: candidate.Agent.ID, TargetSkill: candidate.Agent.Skill, TargetLayer: candidate.Agent.Layer, PromptFile: candidate.Agent.PromptFile, MutationType: candidate.Experiment.MutationType, FailurePattern: "Repeated negative outcomes.", Hypothesis: candidate.Experiment.Hypothesis, AcceptanceMetric: candidate.Experiment.AcceptanceMetric, AcceptanceGates: candidate.Experiment.AcceptanceGates, ForbiddenActions: candidate.Agent.ForbiddenActions, RequiredSkills: candidate.Agent.RequiredSkills, ObservedWindowCount: candidate.Scorecard.WindowCount, MaturityLevel: cml(candidate.Scorecard.WindowCount), IterationGuidance: cig(candidate.Agent.Layer, candidate.Scorecard.WindowCount), RecommendedWindow: crw(candidate.Scorecard.WindowCount), GeneratedAt: time.Now()}
}

func pidf(exp ExperimentRecord) string {
	if exp.ProposalID != "" {
		return exp.ProposalID
	}
	if exp.ID != "" {
		return "proposal-" + exp.ID
	}
	return ""
}

func clp(a AgentSpec, ok bool) int {
	if !ok {
		return 99
	}
	switch a.Layer {
	case LayerSector, LayerStyle:
		return 0
	case LayerContext:
		return 1
	case LayerControl:
		return 2
	default:
		return 3
	}
}

func cml(w int) string {
	switch {
	case w >= 5:
		return "level_3_regime_aware"
	case w >= 3:
		return "level_2_window_validated"
	default:
		return "level_1_exploratory"
	}
}

func crw(w int) string {
	switch {
	case w >= 5:
		return "next cross-regime replay window"
	case w >= 3:
		return "next multi-session replay window"
	default:
		return "next short validation window"
	}
}

func cig(layer AgentLayer, w int) []string {
	g := []string{"Change one bounded behavior only.", "Preserve required skills and forbidden action boundaries."}
	switch layer {
	case LayerSector:
		g = append(g, "Refine sector thesis.", "Prefer industry-specific evidence.")
	case LayerStyle:
		g = append(g, "Tighten entry filters.", "Do not smuggle sector policy into style mutation.")
	case LayerContext:
		g = append(g, "Adjust regime interpretation carefully.", "Prefer threshold refinement.")
	case LayerControl:
		g = append(g, "Conservative governance changes.", "Do not widen risk limits without evidence.")
	}
	if w < 3 {
		g = append(g, "Evidence thin; prefer conservative tightening.")
	} else {
		g = append(g, "Evidence adequate for focused replay challenge.")
	}
	return g
}

func smt(ag AgentSpec, sc Scorecard) (string, string, []string) {
	s, w, l := sc.SharpeLike, sc.WindowCount, ag.Layer
	if l == LayerControl {
		if w >= 5 && s < 0 {
			return "portfolio_constraint_revision", "Portfolio governance needs adjustment.", []string{"improve_sharpe_like", "reduce_concentration_risk", "maintain_cro_authority"}
		}
		return "risk_rule_change", "Risk rules need tightening.", []string{"improve_sharpe_like", "reduce_false_positive_rate", "preserve_downside_protection"}
	}
	if l == LayerSector && w >= 4 && s < 0.1 {
		return "risk_rule_change", "Sector criteria need stronger thresholds.", []string{"improve_sharpe_like", "reduce_sector_blindspots", "maintain_industry_coverage"}
	}
	if l == LayerStyle && w >= 4 && s < 0.1 {
		return "risk_rule_change", "Style filters should be raised.", []string{"improve_sharpe_like", "reduce_style_drift", "maintain_momentum_catch"}
	}
	return "prompt_tightening", "Prompt refinement can improve quality.", []string{"improve_sharpe_like", "no_material_drawdown_degradation", "no_constraint_bypass"}
}
