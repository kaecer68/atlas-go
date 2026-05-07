package experiment

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/replay"
)

type OOSValidator struct {
	replayDataPath string
	store          ledger.ExperimentStore
	params         *config.ParametersConfig
}

func NewOOSValidator(store ledger.ExperimentStore, replayDataPath string) *OOSValidator {
	return &OOSValidator{
		replayDataPath: replayDataPath,
		store:          store,
		params:         config.DefaultParametersConfig(),
	}
}

func (v *OOSValidator) WithParameters(p *config.ParametersConfig) *OOSValidator {
	v.params = p
	return v
}

func (v *OOSValidator) oosWindow(primaryWindowEnd time.Time) (start, end time.Time) {
	start = primaryWindowEnd.AddDate(0, 0, 1)
	days := 30
	if v.params != nil {
		days = v.params.Experiment.OOSWindowDays.Value
	}
	end = start.AddDate(0, 0, days)
	return start, end
}

// Validate validates a candidate using OOS scoring. It infers the mutation type
// from the candidate file path when no brief is available.
func (v *OOSValidator) Validate(candidatePath, baselinePath string, primaryWindowEnd time.Time) (*domain.OOSResult, error) {
	// Infer mutation type from candidate path for constraint mutations.
	var mutationType string
	switch {
	case filepath.Ext(candidatePath) == ".json" || filepath.Ext(candidatePath) == ".yaml":
		mutationType = "portfolio_constraint_revision"
	default:
		mutationType = "prompt_tightening"
	}
	brief := domain.MutationBrief{
		TargetAgentID: "",
		TargetSkill:   "",
		MutationType:  mutationType,
		PromptFile:    candidatePath,
	}
	return v.ValidateWithBrief(candidatePath, baselinePath, brief, primaryWindowEnd)
}

func (v *OOSValidator) ValidateWithBrief(candidatePath, baselinePath string, brief domain.MutationBrief, primaryWindowEnd time.Time) (*domain.OOSResult, error) {
	oosStart, oosEnd := v.oosWindow(primaryWindowEnd)

	result := &domain.OOSResult{
		OOSWindowStart: oosStart,
		OOSWindowEnd:   oosEnd,
		ValidationAt:   time.Now(),
	}

	ds, err := replay.LoadTWSEOpenDataCSV(v.replayDataPath)
	if err != nil {
		result.Passed = false
		result.Reason = fmt.Sprintf("OOS dataset unavailable: %v", err)
		return result, nil
	}

	policy, err := baseline.Load(baselinePath)
	if err != nil {
		result.Passed = false
		result.Reason = fmt.Sprintf("baseline policy unavailable: %v", err)
		return result, nil
	}

	candidateBytes, err := os.ReadFile(candidatePath)
	if err != nil {
		result.Passed = false
		result.Reason = fmt.Sprintf("candidate file unavailable: %v", err)
		return result, nil
	}

	switch brief.MutationType {
	case "risk_rule_change", "portfolio_constraint_revision":
		baselineConstraints := policy.Constraints
		candidateConstraints := baseline.ApplyConstraintCandidate(policy.Constraints, string(candidateBytes))

		baselineScore, baselineObs, _, _ := scoreConstraintWindowWithObservations(ds, baselineConstraints, oosStart, oosEnd)
		candidateScore, candidateObs, _, _ := scoreConstraintWindowWithObservations(ds, candidateConstraints, oosStart, oosEnd)

		result.BaselineScore = baselineScore
		result.CandidateScore = candidateScore
		result.Observations = min(baselineObs, candidateObs)
		result.UsedFallback = baselineObs == 0 && candidateObs == 0

		if result.UsedFallback {
			fbStart, fbEnd, ok := fallbackWindow(ds, 1)
			if ok {
				baselineScore, baselineObs, _, _ = scoreConstraintWindowWithObservations(ds, baselineConstraints, fbStart, fbEnd)
				candidateScore, candidateObs, _, _ = scoreConstraintWindowWithObservations(ds, candidateConstraints, fbStart, fbEnd)
				result.BaselineScore = baselineScore
				result.CandidateScore = candidateScore
				result.Observations = min(baselineObs, candidateObs)
			}
		}
	default:
		baselinePrompt := baseline.ResolvePromptOverride(policy, brief.TargetAgentID, brief.TargetSkill)
		if baselinePrompt == "" {
			if pf := brief.PromptFile; pf != "" {
				if bytes, err := os.ReadFile(pf); err == nil {
					baselinePrompt = string(bytes)
				}
			}
		}

		baselineScore, baselineObs, _, _ := scorePromptWindowWithObservations(ds, brief.TargetSkill, baselinePrompt, policy.ExecutionPolicy, oosStart, oosEnd)
		candidateScore, candidateObs, _, _ := scorePromptWindowWithObservations(ds, brief.TargetSkill, string(candidateBytes), policy.ExecutionPolicy, oosStart, oosEnd)

		result.BaselineScore = baselineScore
		result.CandidateScore = candidateScore
		result.Observations = min(baselineObs, candidateObs)
		result.UsedFallback = baselineObs == 0 && candidateObs == 0

		if result.UsedFallback {
			fbStart, fbEnd, ok := fallbackWindow(ds, 1)
			if ok {
				baselineScore, baselineObs, _, _ = scorePromptWindowWithObservations(ds, brief.TargetSkill, baselinePrompt, policy.ExecutionPolicy, fbStart, fbEnd)
				candidateScore, candidateObs, _, _ = scorePromptWindowWithObservations(ds, brief.TargetSkill, string(candidateBytes), policy.ExecutionPolicy, fbStart, fbEnd)
				result.BaselineScore = baselineScore
				result.CandidateScore = candidateScore
				result.Observations = min(baselineObs, candidateObs)
			}
		}
	}

	result.Improvement = result.CandidateScore - result.BaselineScore

	// Apply OOS acceptance gates.
	minObs := v.params.Experiment.MaturityLevel1Observations.Value
	minImprovement := v.params.Experiment.ImprovementThreshold.Value

	if result.Observations < minObs {
		result.Passed = false
		result.Reason = fmt.Sprintf("insufficient OOS observations: got %d, need %d", result.Observations, minObs)
		return result, nil
	}
	if result.Improvement < minImprovement {
		result.Passed = false
		result.Reason = fmt.Sprintf("OOS improvement %.6f below threshold %.6f", result.Improvement, minImprovement)
		return result, nil
	}

	result.Passed = true
	result.Reason = "OOS validation passed"
	return result, nil
}

func (v *OOSValidator) ValidateWithConstraints(candidateConstraintsPath, baselineConstraintsPath string, brief domain.MutationBrief, primaryWindowEnd time.Time) (*domain.OOSResult, error) {
	oosStart, oosEnd := v.oosWindow(primaryWindowEnd)

	result := &domain.OOSResult{
		OOSWindowStart: oosStart,
		OOSWindowEnd:   oosEnd,
		ValidationAt:   time.Now(),
	}

	ds, err := replay.LoadTWSEOpenDataCSV(v.replayDataPath)
	if err != nil {
		result.Passed = false
		result.Reason = fmt.Sprintf("OOS dataset unavailable: %v", err)
		return result, nil
	}

	policy, err := baseline.Load(baselineConstraintsPath)
	if err != nil {
		result.Passed = false
		result.Reason = fmt.Sprintf("baseline constraints unavailable: %v", err)
		return result, nil
	}

	candidateBytes, err := os.ReadFile(candidateConstraintsPath)
	if err != nil {
		result.Passed = false
		result.Reason = fmt.Sprintf("candidate constraints unavailable: %v", err)
		return result, nil
	}

	baselineConstraints := policy.Constraints
	candidateConstraints := baseline.ApplyConstraintCandidate(policy.Constraints, string(candidateBytes))

	baselineScore, baselineObs, _, _ := scoreConstraintWindowWithObservations(ds, baselineConstraints, oosStart, oosEnd)
	candidateScore, candidateObs, _, _ := scoreConstraintWindowWithObservations(ds, candidateConstraints, oosStart, oosEnd)

	result.BaselineScore = baselineScore
	result.CandidateScore = candidateScore
	result.Observations = min(baselineObs, candidateObs)
	result.UsedFallback = baselineObs == 0 && candidateObs == 0

	if result.UsedFallback {
		fbStart, fbEnd, ok := fallbackWindow(ds, 1)
		if ok {
			baselineScore, baselineObs, _, _ = scoreConstraintWindowWithObservations(ds, baselineConstraints, fbStart, fbEnd)
			candidateScore, candidateObs, _, _ = scoreConstraintWindowWithObservations(ds, candidateConstraints, fbStart, fbEnd)
			result.BaselineScore = baselineScore
			result.CandidateScore = candidateScore
			result.Observations = min(baselineObs, candidateObs)
		}
	}

	result.Improvement = result.CandidateScore - result.BaselineScore

	minObs := v.params.Experiment.MaturityLevel1Observations.Value
	minImprovement := v.params.Experiment.ImprovementThreshold.Value

	if result.Observations < minObs {
		result.Passed = false
		result.Reason = fmt.Sprintf("insufficient OOS observations: got %d, need %d", result.Observations, minObs)
		return result, nil
	}
	if result.Improvement < minImprovement {
		result.Passed = false
		result.Reason = fmt.Sprintf("OOS improvement %.6f below threshold %.6f", result.Improvement, minImprovement)
		return result, nil
	}

	result.Passed = true
	result.Reason = "OOS validation passed"
	return result, nil
}
