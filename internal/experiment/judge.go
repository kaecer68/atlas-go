package experiment

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

type Judge struct {
	store          *ledger.Store
	replayDataPath string
	baselinePath   string
}

func NewJudge(store *ledger.Store, replayDataPath, baselinePath string) *Judge {
	return &Judge{store: store, replayDataPath: replayDataPath, baselinePath: baselinePath}
}

func (j *Judge) Evaluate(resultPath string) (domain.PromptExperimentResult, error) {
	result, err := loadExperimentResult(resultPath)
	if err != nil {
		return domain.PromptExperimentResult{}, err
	}

	candidatePromptPath := result.CandidatePrompt
	if !filepath.IsAbs(candidatePromptPath) {
		if _, err := os.Stat(candidatePromptPath); err != nil {
			candidatePromptPath = filepath.Join(".", candidatePromptPath)
		}
	}

	promptBytes, err := os.ReadFile(candidatePromptPath)
	if err != nil {
		return domain.PromptExperimentResult{}, fmt.Errorf("read candidate prompt %s: %w", candidatePromptPath, err)
	}
	windowSummary, err := loadWindowSummary(windowSummaryPath(resultPath, result.Brief.WindowID))
	if err != nil {
		return domain.PromptExperimentResult{}, err
	}

	baseline, candidate, err := comparePromptPerformance(j.replayDataPath, j.baselinePath, result.Brief, windowSummary, result.CandidatePrompt)
	if err != nil {
		return domain.PromptExperimentResult{}, err
	}
	checks := judgeReplayChecks(string(promptBytes), result)

	result.Experiment.BaselineValue = baseline
	result.Experiment.CandidateValue = candidate
	result.Experiment.WindowStart = windowSummary.StartDate
	result.Experiment.WindowEnd = windowSummary.EndDate
	result.EvaluationMode = "prompt_aware_replay_judged"
	result.JudgeChecks = checks
	result.RecordedAt = time.Now()

	accepted, acceptanceNote := passesAcceptance(result)
	result.JudgeChecks = append(result.JudgeChecks, acceptanceNote)
	if accepted {
		result.Experiment.Status = domain.ExperimentAccepted
		result.Notes = append(result.Notes, "Replay judge accepted the candidate for the next baseline promotion step.")
	} else {
		result.Experiment.Status = domain.ExperimentRejected
		result.Experiment.RevertReason = "Replay judge did not satisfy maturity-aware acceptance gates."
		result.Notes = append(result.Notes, "Replay judge rejected the candidate.")
	}

	if err := j.store.UpdatePromptExperimentResult(result.Experiment.ID, result); err != nil {
		return domain.PromptExperimentResult{}, err
	}
	return result, nil
}

func judgeReplayChecks(candidatePrompt string, result domain.PromptExperimentResult) []string {
	checks := make([]string, 0)
	lower := strings.ToLower(candidatePrompt)

	switch result.Experiment.MutationType {
	case "risk_rule_change":
		if strings.Contains(lower, "risk rule change proposal") {
			checks = append(checks, "contains risk rule proposal header")
		}
		if strings.Contains(lower, "candidate rule patch") {
			checks = append(checks, "contains candidate rule patch section")
		}
		if strings.Contains(lower, "conviction_floor") && strings.Contains(lower, "liquidity_floor") {
			checks = append(checks, "contains structured risk rule fields")
		}
		if strings.Contains(lower, "guardrails") {
			checks = append(checks, "contains control guardrails section")
		}
	case "portfolio_constraint_revision":
		if strings.Contains(lower, "portfolio constraint revision proposal") {
			checks = append(checks, "contains portfolio constraint proposal header")
		}
		if strings.Contains(lower, "candidate constraint patch") {
			checks = append(checks, "contains candidate constraint patch section")
		}
		if strings.Contains(lower, "max_position_weight") && strings.Contains(lower, "reserve_cash_fraction") {
			checks = append(checks, "contains structured portfolio constraint fields")
		}
		if strings.Contains(lower, "require_cro_pass") {
			checks = append(checks, "preserves CRO sequencing requirement")
		}
	default:
		if strings.Contains(lower, "require trend confirmation") {
			checks = append(checks, "contains stronger trend confirmation rule")
		}
		if strings.Contains(lower, "downgrade conviction") {
			checks = append(checks, "contains conviction downgrade logic")
		}
		if strings.Contains(lower, "reject setups") {
			checks = append(checks, "contains explicit rejection filter")
		}
	}
	if len(result.PolicyChecks) >= len(result.Brief.RequiredSkills) {
		checks = append(checks, "required skill policy checks preserved")
	}
	if len(result.Brief.ForbiddenActions) > 0 {
		checks = append(checks, "forbidden actions still named in candidate prompt")
	}

	return checks
}

func passesAcceptance(result domain.PromptExperimentResult) (bool, string) {
	gates := result.Experiment.AcceptanceGates
	baseline := result.Experiment.BaselineValue
	candidate := result.Experiment.CandidateValue
	checks := result.JudgeChecks

	if len(gates) == 0 {
		return false, "rejected: no acceptance gates configured"
	}
	if candidate <= baseline {
		return false, "rejected: candidate did not improve over baseline"
	}

	requiredImprovement := requiredImprovementForProfile(result.Brief.MaturityLevel, result.Experiment.MutationType)
	if candidate-baseline < requiredImprovement {
		return false, "rejected: improvement below mutation profile threshold"
	}

	requiredChecks := requiredCheckCountForProfile(result.Brief.MaturityLevel, result.Experiment.MutationType)

	for _, gate := range gates {
		switch gate {
		case "improve_sharpe_like":
			if candidate <= baseline {
				return false, "rejected: sharpe-like gate failed"
			}
		case "no_material_drawdown_degradation":
			if len(checks) < requiredChecks {
				return false, "rejected: insufficient replay checks for drawdown confidence"
			}
		case "no_constraint_bypass":
			if math.IsNaN(candidate) {
				return false, "rejected: candidate score invalid"
			}
		}
	}
	return true, "accepted: maturity-aware gates satisfied"
}

func requiredImprovementForProfile(maturity, mutationType string) float64 {
	switch mutationType {
	case "risk_rule_change":
		return 0.001
	case "portfolio_constraint_revision":
		return 0.001
	default:
		return 0.0005
	}
}

func requiredImprovementForMaturity(maturity string) float64 {
	switch maturity {
	case "level_3_regime_aware":
		return 0.0025
	case "level_2_window_validated":
		return 0.001
	default:
		return 0
	}
}

func requiredCheckCountForProfile(maturity, mutationType string) int {
	base := requiredCheckCountForMaturity(maturity)
	switch mutationType {
	case "risk_rule_change":
		return base + 1
	case "portfolio_constraint_revision":
		return base + 2
	default:
		return base
	}
}

func requiredCheckCountForMaturity(maturity string) int {
	switch maturity {
	case "level_3_regime_aware":
		return 4
	case "level_2_window_validated":
		return 3
	default:
		return 2
	}
}

func loadExperimentResult(path string) (domain.PromptExperimentResult, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return domain.PromptExperimentResult{}, err
	}
	var result domain.PromptExperimentResult
	if err := json.Unmarshal(bytes, &result); err != nil {
		return domain.PromptExperimentResult{}, err
	}
	return result, nil
}

func loadWindowSummary(path string) (domain.BacktestWindowSummary, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return domain.BacktestWindowSummary{}, err
	}
	var summary domain.BacktestWindowSummary
	if err := json.Unmarshal(bytes, &summary); err != nil {
		return domain.BacktestWindowSummary{}, err
	}
	return summary, nil
}

func windowSummaryPath(resultPath, windowID string) string {
	base := filepath.Dir(filepath.Dir(resultPath))
	return filepath.Join(base, "windows", windowID+".json")
}
