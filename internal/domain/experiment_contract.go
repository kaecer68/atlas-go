package domain

import (
	"fmt"
	"strings"
)

const MutationBriefContractVersion = 1

var supportedMutationTypes = map[string]struct{}{
	"prompt_tightening":             {},
	"risk_rule_change":              {},
	"portfolio_constraint_revision": {},
}

var validMaturityLevels = map[string]struct{}{
	"level_1_exploratory":      {},
	"level_2_validated":        {},
	"level_2_window_validated": {},
	"level_3_regime_aware":     {},
}

func (b *MutationBrief) NormalizeAndValidate() error {
	if b == nil {
		return fmt.Errorf("mutation brief is nil")
	}

	if b.ContractVersion == 0 {
		b.ContractVersion = MutationBriefContractVersion
	}
	if b.ContractVersion != MutationBriefContractVersion {
		return fmt.Errorf("unsupported contract_version %d", b.ContractVersion)
	}
	if strings.TrimSpace(b.WindowID) == "" {
		return fmt.Errorf("window_id is required")
	}
	if strings.TrimSpace(b.ProposalID) == "" {
		b.ProposalID = "proposal-" + strings.TrimSpace(b.WindowID)
	}
	if strings.TrimSpace(b.TargetAgentID) == "" {
		return fmt.Errorf("target_agent_id is required")
	}
	if strings.TrimSpace(b.TargetSkill) == "" {
		return fmt.Errorf("target_skill is required")
	}
	if !isValidAgentLayer(b.TargetLayer) {
		return fmt.Errorf("target_layer is invalid: %q", b.TargetLayer)
	}
	if strings.TrimSpace(b.PromptFile) == "" {
		return fmt.Errorf("prompt_file is required")
	}
	if _, ok := supportedMutationTypes[b.MutationType]; !ok {
		return fmt.Errorf("mutation_type is invalid: %q", b.MutationType)
	}
	if strings.TrimSpace(b.AcceptanceMetric) == "" {
		return fmt.Errorf("acceptance_metric is required")
	}
	if len(b.AcceptanceGates) == 0 {
		return fmt.Errorf("acceptance_gates is required")
	}
	if b.ObservedWindowCount < 0 {
		return fmt.Errorf("observed_window_count must be >= 0")
	}
	if b.MaturityLevel != "" {
		if _, ok := validMaturityLevels[b.MaturityLevel]; !ok {
			return fmt.Errorf("invalid maturity_level: %q", b.MaturityLevel)
		}
	}

	return nil
}

func (r *PromptExperimentResult) NormalizeAndValidateForJudge() error {
	if r == nil {
		return fmt.Errorf("prompt experiment result is nil")
	}
	if strings.TrimSpace(r.Experiment.ID) == "" {
		return fmt.Errorf("experiment.id is required")
	}
	if strings.TrimSpace(r.CandidatePrompt) == "" {
		return fmt.Errorf("candidate_prompt is required")
	}
	if err := r.Brief.NormalizeAndValidate(); err != nil {
		return fmt.Errorf("invalid brief: %w", err)
	}

	if strings.TrimSpace(r.Experiment.TargetAgentID) == "" {
		r.Experiment.TargetAgentID = r.Brief.TargetAgentID
	}
	if strings.TrimSpace(r.Experiment.ProposalID) == "" {
		r.Experiment.ProposalID = r.Brief.ProposalID
	}
	if strings.TrimSpace(r.Experiment.Skill) == "" {
		r.Experiment.Skill = r.Brief.TargetSkill
	}
	if strings.TrimSpace(r.Experiment.MutationType) == "" {
		r.Experiment.MutationType = r.Brief.MutationType
	}
	if len(r.Experiment.AcceptanceGates) == 0 {
		r.Experiment.AcceptanceGates = append([]string(nil), r.Brief.AcceptanceGates...)
	}
	if strings.TrimSpace(r.Experiment.AcceptanceMetric) == "" {
		r.Experiment.AcceptanceMetric = r.Brief.AcceptanceMetric
	}
	if strings.TrimSpace(r.Experiment.ProposalID) == "" {
		r.Experiment.ProposalID = "proposal-" + r.Experiment.ID
	}
	if strings.TrimSpace(r.Experiment.CommitID) == "" {
		r.Experiment.CommitID = "commit-" + r.Experiment.ID
	}

	if strings.TrimSpace(r.Experiment.TargetAgentID) == "" {
		return fmt.Errorf("experiment.target_agent_id is required")
	}
	if strings.TrimSpace(r.Experiment.Skill) == "" {
		return fmt.Errorf("experiment.skill is required")
	}
	if _, ok := supportedMutationTypes[r.Experiment.MutationType]; !ok {
		return fmt.Errorf("experiment.mutation_type is invalid: %q", r.Experiment.MutationType)
	}

	return nil
}

func isValidAgentLayer(layer AgentLayer) bool {
	switch layer {
	case LayerContext, LayerMacro, LayerSector, LayerStyle, LayerSuperinvestor, LayerControl:
		return true
	default:
		return false
	}
}
