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

	summary, err := comparePromptPerformanceDetailed(j.replayDataPath, j.baselinePath, result.Brief, windowSummary, result.CandidatePrompt)
	if err != nil {
		return domain.PromptExperimentResult{}, err
	}
	checks := judgeReplayChecks(string(promptBytes), result)
	checks = append(checks,
		fmt.Sprintf("baseline observations: %d", summary.BaselineObservations),
		fmt.Sprintf("candidate observations: %d", summary.CandidateObservations),
	)
	if summary.UsedFallbackWindow {
		checks = append(checks, "fallback replay window used due to sparse primary window")
	}

	result.Experiment.BaselineValue = summary.BaselineScore
	result.Experiment.CandidateValue = summary.CandidateScore
	result.BaselineObservations = summary.BaselineObservations
	result.CandidateObservations = summary.CandidateObservations
	result.UsedFallbackWindow = summary.UsedFallbackWindow
	result.Experiment.WindowStart = windowSummary.StartDate
	result.Experiment.WindowEnd = windowSummary.EndDate
	result.EvaluationMode = "prompt_aware_replay_judged"
	result.JudgeChecks = checks
	result.RecordedAt = time.Now()

	accepted, acceptanceNote := passesAcceptance(result)
	result.JudgeChecks = append(result.JudgeChecks, acceptanceNote)
	if result.Experiment.ApprovalID == "" {
		result.Experiment.ApprovalID = "approval-" + result.Experiment.ID
	}
	if accepted {
		if err := domain.TransitionExperimentStatus(&result.Experiment, domain.ExperimentAccepted); err != nil {
			return domain.PromptExperimentResult{}, fmt.Errorf("transition experiment status: %w", err)
		}
		result.Notes = append(result.Notes, "Replay judge accepted the candidate for the next baseline promotion step.")
	} else {
		if err := domain.TransitionExperimentStatus(&result.Experiment, domain.ExperimentRejected); err != nil {
			return domain.PromptExperimentResult{}, fmt.Errorf("transition experiment status: %w", err)
		}
		result.Experiment.RevertReason = "Replay judge did not satisfy maturity-aware acceptance gates."
		result.Notes = append(result.Notes, "Replay judge rejected the candidate.")
	}

	if err := j.store.UpdatePromptExperimentResult(result.Experiment.ID, result); err != nil {
		return domain.PromptExperimentResult{}, err
	}
	return result, nil
}

type judgeCheckFunc func(lower string, result domain.PromptExperimentResult) []string

var judgeCheckStrategies = map[string]judgeCheckFunc{
	"risk_rule_change":              riskRuleJudgeChecks,
	"portfolio_constraint_revision": portfolioConstraintJudgeChecks,
}

func judgeReplayChecks(candidatePrompt string, result domain.PromptExperimentResult) []string {
	checks := make([]string, 0)
	lower := strings.ToLower(candidatePrompt)

	if strategy, ok := judgeCheckStrategies[result.Experiment.MutationType]; ok {
		checks = append(checks, strategy(lower, result)...)
	} else {
		checks = append(checks, promptTighteningJudgeChecks(lower, result)...)
	}

	if len(result.PolicyChecks) >= len(result.Brief.RequiredSkills) {
		checks = append(checks, "required skill policy checks preserved")
	}
	if len(result.Brief.ForbiddenActions) > 0 {
		checks = append(checks, "forbidden actions still named in candidate prompt")
	}

	return checks
}

func riskRuleJudgeChecks(lower string, _ domain.PromptExperimentResult) []string {
	checks := make([]string, 0)
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
	return checks
}

func portfolioConstraintJudgeChecks(lower string, _ domain.PromptExperimentResult) []string {
	checks := make([]string, 0)
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
	return checks
}

func promptTighteningJudgeChecks(lower string, result domain.PromptExperimentResult) []string {
	checks := make([]string, 0)
	switch result.Brief.TargetSkill {
	case "financials_desk":
		if strings.Contains(lower, "credit quality gate") {
			checks = append(checks, "contains credit quality gate")
		}
		if strings.Contains(lower, "spread sensitivity downgrade") {
			checks = append(checks, "contains spread sensitivity downgrade")
		}
		if strings.Contains(lower, "capital adequacy premium") {
			checks = append(checks, "contains capital adequacy premium")
		}
	case "technical_breakout":
		if strings.Contains(lower, "structure-first breakout filter") {
			checks = append(checks, "contains structure-first breakout filter")
		}
		if strings.Contains(lower, "volume surge requirement") {
			checks = append(checks, "contains volume surge requirement")
		}
		if strings.Contains(lower, "late-breakout penalty") {
			checks = append(checks, "contains late-breakout penalty")
		}
		if strings.Contains(lower, "coverage expansion") {
			checks = append(checks, "contains coverage expansion mode")
		}
		if strings.Contains(lower, "catch-up momentum") {
			checks = append(checks, "contains catch-up momentum")
		}
		if strings.Contains(lower, "volume participation acceptance") {
			checks = append(checks, "contains volume participation acceptance")
		}
		if strings.Contains(lower, "close-strength tolerance") {
			checks = append(checks, "contains close-strength tolerance")
		}
		if strings.Contains(lower, "breakout confirmation bonus") {
			checks = append(checks, "contains breakout confirmation bonus")
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
	return checks
}

func passesAcceptance(result domain.PromptExperimentResult) (bool, string) {
	gates := result.Experiment.AcceptanceGates
	baseline := result.Experiment.BaselineValue
	candidate := result.Experiment.CandidateValue
	checks := result.JudgeChecks
	baselineObs := result.BaselineObservations
	candidateObs := result.CandidateObservations

	if len(gates) == 0 {
		return false, "rejected: no acceptance gates configured"
	}
	minObs := requiredObservationCountForProfile(result.Brief.MaturityLevel, result.Experiment.MutationType)
	if baselineObs < minObs || candidateObs < minObs {
		return false, fmt.Sprintf("rejected: insufficient replay observations (baseline=%d candidate=%d required=%d)", baselineObs, candidateObs, minObs)
	}
	if candidate <= baseline {
		if candidate == baseline {
			return false, "rejected: candidate score equals baseline (no constraint delta applied)"
		}
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

func welchTTest(baselineReturns, candidateReturns []float64) (tStat float64, df float64) {
	if len(baselineReturns) < 2 || len(candidateReturns) < 2 {
		return 0, 0
	}

	mean1, var1 := meanAndVariance(baselineReturns)
	mean2, var2 := meanAndVariance(candidateReturns)

	n1, n2 := float64(len(baselineReturns)), float64(len(candidateReturns))

	se1 := var1 / n1
	se2 := var2 / n2
	seDiff := math.Sqrt(se1 + se2)

	if seDiff == 0 {
		return 0, 0
	}

	tStat = (mean2 - mean1) / seDiff

	df = math.Pow(se1+se2, 2) / (math.Pow(se1, 2)/(n1-1) + math.Pow(se2, 2)/(n2-1))

	return tStat, df
}

func meanAndVariance(data []float64) (mean, variance float64) {
	if len(data) == 0 {
		return 0, 0
	}

	var sum float64
	for _, v := range data {
		sum += v
	}
	mean = sum / float64(len(data))

	if len(data) < 2 {
		return mean, 0
	}

	var sqDiffSum float64
	for _, v := range data {
		diff := v - mean
		sqDiffSum += diff * diff
	}
	variance = sqDiffSum / float64(len(data)-1)

	return mean, variance
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

func requiredObservationCountForProfile(maturity, mutationType string) int {
	base := requiredObservationCountForMaturity(maturity)
	switch mutationType {
	case "risk_rule_change", "portfolio_constraint_revision":
		return base + 1
	default:
		return base
	}
}

func requiredObservationCountForMaturity(maturity string) int {
	switch maturity {
	case "level_3_regime_aware":
		return 12
	case "level_2_window_validated", "level_2_validated":
		return 8
	case "level_1_exploratory":
		return 3
	default:
		return 3
	}
}

func requiredCheckCountForMaturity(maturity string) int {
	switch maturity {
	case "level_3_regime_aware":
		return 4
	case "level_2_window_validated", "level_2_validated":
		return 3
	case "level_1_exploratory":
		return 2
	default:
		return 2
	}
}

func loadExperimentResult(path string) (domain.PromptExperimentResult, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return domain.PromptExperimentResult{}, fmt.Errorf("read experiment result %s: %w", path, err)
	}
	var result domain.PromptExperimentResult
	if err := json.Unmarshal(bytes, &result); err != nil {
		return domain.PromptExperimentResult{}, fmt.Errorf("unmarshal experiment result %s: %w", path, err)
	}
	if err := result.NormalizeAndValidateForJudge(); err != nil {
		return domain.PromptExperimentResult{}, fmt.Errorf("validate experiment result %s: %w", path, err)
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
