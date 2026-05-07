package baseline

import (
	"errors"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type RevertType string

const (
	RevertLast         RevertType = "last"
	RevertToVersion    RevertType = "version"
	RevertToExperiment RevertType = "experiment"
)

type RevertTarget struct {
	Type         RevertType
	Version      int
	ExperimentID string
}

type RevertResult struct {
	FromVersion         int
	ToVersion           int
	RevertedExperiments []string
	Reason              string
	RevertedAt          time.Time
	DryRun              bool
}

func (m *Manager) Revert(target RevertTarget, reason string, dryRun bool) (RevertResult, error) {
	current, err := Load(m.path)
	if err != nil {
		return RevertResult{}, fmt.Errorf("load current policy: %w", err)
	}

	targetVersion, err := m.resolveTargetVersion(current, target)
	if err != nil {
		return RevertResult{}, fmt.Errorf("resolve target: %w", err)
	}

	if targetVersion == current.Version {
		return RevertResult{}, errors.New("already at target version, nothing to revert")
	}

	if targetVersion > current.Version {
		return RevertResult{}, fmt.Errorf("cannot revert to version %d (current is %d)", targetVersion, current.Version)
	}

	revertedExperiments := m.findRevertedExperiments(current, targetVersion)

	result := RevertResult{
		FromVersion:         current.Version,
		ToVersion:           targetVersion,
		RevertedExperiments: revertedExperiments,
		Reason:              reason,
		RevertedAt:          time.Now(),
		DryRun:              dryRun,
	}

	if dryRun {
		return result, nil
	}

	revertedPolicy, err := m.reconstructPolicyAtVersion(current, targetVersion)
	if err != nil {
		return RevertResult{}, fmt.Errorf("reconstruct policy: %w", err)
	}

	revertedPolicy.RevertHistory = append(revertedPolicy.RevertHistory, RevertRecord{
		FromVersion:         current.Version,
		ToVersion:           targetVersion,
		RevertedExperiments: revertedExperiments,
		Reason:              reason,
		RevertedAt:          time.Now(),
	})

	if err := Save(m.path, revertedPolicy); err != nil {
		return RevertResult{}, fmt.Errorf("save reverted policy: %w", err)
	}

	return result, nil
}

func (m *Manager) resolveTargetVersion(current Policy, target RevertTarget) (int, error) {
	switch target.Type {
	case RevertLast:
		if len(current.Promotions) == 0 {
			return 0, errors.New("no promotions to revert")
		}

		lastPromoIdx := len(current.Promotions) - 1
		if lastPromoIdx == 0 {
			return 1, nil
		}

		return lastPromoIdx + 1, nil

	case RevertToVersion:
		if target.Version < 1 {
			return 0, errors.New("target version must be >= 1")
		}
		return target.Version, nil

	case RevertToExperiment:
		if target.ExperimentID == "" {
			return 0, errors.New("experiment ID required for experiment revert")
		}

		for i, promo := range current.Promotions {
			if promo.ExperimentID == target.ExperimentID {
				if i == 0 {
					return 1, nil
				}
				return i + 1, nil
			}
		}
		return 0, fmt.Errorf("experiment %s not found in promotion history", target.ExperimentID)

	default:
		return 0, fmt.Errorf("unknown revert type: %s", target.Type)
	}
}

func (m *Manager) findRevertedExperiments(current Policy, targetVersion int) []string {
	var reverted []string

	for i, promo := range current.Promotions {
		promoVersion := i + 2
		if promoVersion > targetVersion {
			reverted = append(reverted, promo.ExperimentID)
		}
	}

	return reverted
}

func (m *Manager) reconstructPolicyAtVersion(current Policy, targetVersion int) (Policy, error) {
	reconstructed := DefaultPolicy()
	reconstructed.Version = targetVersion

	// Find the last constraint promotion at or before targetVersion that has a snapshot.
	var lastConstraintSnapshot *domain.SimulationConstraints
	for i, promo := range current.Promotions {
		promoVersion := i + 2
		if promoVersion > targetVersion {
			break
		}
		if (promo.MutationType == "risk_rule_change" || promo.MutationType == "portfolio_constraint_revision") && promo.ConstraintsSnapshot != nil {
			lastConstraintSnapshot = promo.ConstraintsSnapshot
		}
	}

	if lastConstraintSnapshot != nil {
		reconstructed.Constraints = *lastConstraintSnapshot
		reconstructed.ExecutionPolicy = ExecutionPolicyFromConstraints(*lastConstraintSnapshot)
	} else {
		// Fallback: if no later constraint changes, current constraints are valid.
		hasLaterConstraintChange := false
		for i, promo := range current.Promotions {
			promoVersion := i + 2
			if promoVersion > targetVersion && (promo.MutationType == "risk_rule_change" || promo.MutationType == "portfolio_constraint_revision") {
				hasLaterConstraintChange = true
				break
			}
		}
		if !hasLaterConstraintChange {
			reconstructed.Constraints = current.Constraints
			reconstructed.ExecutionPolicy = current.ExecutionPolicy
		}
	}

	for i, promo := range current.Promotions {
		promoVersion := i + 2
		if promoVersion > targetVersion {
			break
		}

		if promo.MutationType == "" || promo.MutationType == "prompt_tightening" {
			if promo.PromptSnapshot != "" {
				reconstructed.PromptOverrides[promo.TargetAgentID] = promo.PromptSnapshot
			} else if currentOverride, ok := current.PromptOverrides[promo.TargetAgentID]; ok {
				reconstructed.PromptOverrides[promo.TargetAgentID] = currentOverride
			}
		}

		reconstructed.Promotions = append(reconstructed.Promotions, promo)
	}

	return reconstructed, nil
}

func (m *Manager) GetPromotionHistory() ([]PromotionRecordWithVersion, error) {
	policy, err := Load(m.path)
	if err != nil {
		return nil, err
	}

	history := make([]PromotionRecordWithVersion, len(policy.Promotions))
	for i, promo := range policy.Promotions {
		history[i] = PromotionRecordWithVersion{
			PromotionRecord: promo,
			Version:         i + 2,
		}
	}

	return history, nil
}

type PromotionRecordWithVersion struct {
	PromotionRecord
	Version int
}

func (m *Manager) GetRevertHistory() ([]RevertRecord, error) {
	policy, err := Load(m.path)
	if err != nil {
		return nil, err
	}
	return policy.RevertHistory, nil
}
