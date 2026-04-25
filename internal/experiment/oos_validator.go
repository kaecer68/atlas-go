package experiment

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/replay"
)

const (
	DefaultOOSWindowDays = 30
)

// oosAcceptanceThreshold returns the minimum improvement required for OOS validation.
func oosAcceptanceThreshold() float64 {
	return 0.0005
}

// oosMinimumObservations returns the minimum observations required for OOS validation.
func oosMinimumObservations() int {
	return 3
}

type OOSValidator struct {
	replayDataPath string
	store          *ledger.Store
}

func NewOOSValidator(store *ledger.Store, replayDataPath string) *OOSValidator {
	return &OOSValidator{
		replayDataPath: replayDataPath,
		store:          store,
	}
}

// oosWindow computes the out-of-sample window: the period immediately following
// the primary backtest window.
func oosWindow(primaryWindowEnd time.Time) (start, end time.Time) {
	start = primaryWindowEnd.AddDate(0, 0, 1)
	end = start.AddDate(0, 0, DefaultOOSWindowDays)
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

// ValidateWithBrief validates the candidate against the baseline using out-of-sample
// data. The OOS window starts the day after the primary window ends and spans
// DefaultOOSWindowDays days. It delegates to the appropriate scoring function based
// on mutation type (constraint-based vs prompt-based).
func (v *OOSValidator) ValidateWithBrief(candidatePath, baselinePath string, brief domain.MutationBrief, primaryWindowEnd time.Time) (*domain.OOSResult, error) {
	oosStart, oosEnd := oosWindow(primaryWindowEnd)

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

		baselineScore, baselineObs, _ := scoreConstraintWindowWithObservations(ds, baselineConstraints, oosStart, oosEnd)
		candidateScore, candidateObs, _ := scoreConstraintWindowWithObservations(ds, candidateConstraints, oosStart, oosEnd)

		result.BaselineScore = baselineScore
		result.CandidateScore = candidateScore
		result.Observations = min(baselineObs, candidateObs)
		result.UsedFallback = baselineObs == 0 && candidateObs == 0

		if result.UsedFallback {
			fbStart, fbEnd, ok := fallbackWindow(ds, 1)
			if ok {
				baselineScore, baselineObs, _ = scoreConstraintWindowWithObservations(ds, baselineConstraints, fbStart, fbEnd)
				candidateScore, candidateObs, _ = scoreConstraintWindowWithObservations(ds, candidateConstraints, fbStart, fbEnd)
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

		baselineScore, baselineObs, _ := scorePromptWindowWithObservations(ds, brief.TargetSkill, baselinePrompt, policy.ExecutionPolicy, oosStart, oosEnd)
		candidateScore, candidateObs, _ := scorePromptWindowWithObservations(ds, brief.TargetSkill, string(candidateBytes), policy.ExecutionPolicy, oosStart, oosEnd)

		result.BaselineScore = baselineScore
		result.CandidateScore = candidateScore
		result.Observations = min(baselineObs, candidateObs)
		result.UsedFallback = baselineObs == 0 && candidateObs == 0

		if result.UsedFallback {
			fbStart, fbEnd, ok := fallbackWindow(ds, 1)
			if ok {
				baselineScore, baselineObs, _ = scorePromptWindowWithObservations(ds, brief.TargetSkill, baselinePrompt, policy.ExecutionPolicy, fbStart, fbEnd)
				candidateScore, candidateObs, _ = scorePromptWindowWithObservations(ds, brief.TargetSkill, string(candidateBytes), policy.ExecutionPolicy, fbStart, fbEnd)
				result.BaselineScore = baselineScore
				result.CandidateScore = candidateScore
				result.Observations = min(baselineObs, candidateObs)
			}
		}
	}

	result.Improvement = result.CandidateScore - result.BaselineScore

	// Apply OOS acceptance gates.
	minObs := oosMinimumObservations()
	minImprovement := oosAcceptanceThreshold()

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

// ValidateWithConstraints validates constraint-based mutations where both baseline
// and candidate constraint patches are provided as file paths.
func (v *OOSValidator) ValidateWithConstraints(candidateConstraintsPath, baselineConstraintsPath string, brief domain.MutationBrief, primaryWindowEnd time.Time) (*domain.OOSResult, error) {
	oosStart, oosEnd := oosWindow(primaryWindowEnd)

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

	baselineScore, baselineObs, _ := scoreConstraintWindowWithObservations(ds, baselineConstraints, oosStart, oosEnd)
	candidateScore, candidateObs, _ := scoreConstraintWindowWithObservations(ds, candidateConstraints, oosStart, oosEnd)

	result.BaselineScore = baselineScore
	result.CandidateScore = candidateScore
	result.Observations = min(baselineObs, candidateObs)
	result.UsedFallback = baselineObs == 0 && candidateObs == 0

	if result.UsedFallback {
		fbStart, fbEnd, ok := fallbackWindow(ds, 1)
		if ok {
			baselineScore, baselineObs, _ = scoreConstraintWindowWithObservations(ds, baselineConstraints, fbStart, fbEnd)
			candidateScore, candidateObs, _ = scoreConstraintWindowWithObservations(ds, candidateConstraints, fbStart, fbEnd)
			result.BaselineScore = baselineScore
			result.CandidateScore = candidateScore
			result.Observations = min(baselineObs, candidateObs)
		}
	}

	result.Improvement = result.CandidateScore - result.BaselineScore

	minObs := oosMinimumObservations()
	minImprovement := oosAcceptanceThreshold()

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
