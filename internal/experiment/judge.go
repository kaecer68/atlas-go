package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/acceptance"
	"github.com/kaecer68/atlas-go/internal/acceptance/builtin"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eval"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/feature"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/replay"
)

type Judge struct {
	store                 ledger.ExperimentStore
	replayDataPath        string
	baselinePath          string
	oosValidator          *OOSValidator
	params                *config.ParametersConfig
	eventBus              *eventbus.ChannelEventBus
	maturityTracker       *domain.MaturityTracker
	useAcceptancePipeline bool
	lifecyclePublisher    *LifecyclePublisher
	historicalStore       ledger.HistoricalStore // optional: enables regime-conditional evaluation
}

func NewJudge(store ledger.ExperimentStore, replayDataPath, baselinePath string) *Judge {
	return &Judge{
		store:          store,
		replayDataPath: replayDataPath,
		baselinePath:   baselinePath,
		oosValidator:   NewOOSValidator(store, replayDataPath),
		params:         config.DefaultParametersConfig(),
	}
}

// WithEventBus sets the Judge's event bus for publishing insufficient data events.
func (j *Judge) WithEventBus(bus *eventbus.ChannelEventBus) *Judge {
	j.eventBus = bus
	return j
}

// WithLifecyclePublisher attaches a LifecyclePublisher for publishing
// EventExperiment{Accepted,Rejected} events when experiment status
// transitions. Decision 5 (alert-redesign-v2.md Part 3.6) reversal:
// the lifecycle event is published instead of being dropped to log.
func (j *Judge) WithLifecyclePublisher(p *LifecyclePublisher) *Judge {
	j.lifecyclePublisher = p
	return j
}

// transitionExperimentStatus routes through LifecyclePublisher if attached
// (publishes EventExperiment{Accepted,Rejected} per Decision 5 reversal);
// otherwise falls back to the direct domain call. Backward-compatible:
// judges without a publisher work exactly as before.
func (j *Judge) transitionExperimentStatus(record *domain.ExperimentRecord, next domain.ExperimentStatus) error {
	if j.lifecyclePublisher != nil {
		return j.lifecyclePublisher.TransitionAndPublish(record, next)
	}
	return domain.TransitionExperimentStatus(record, next)
}

// WithMaturityTracker attaches a maturity tracker for burn-in gating.
func (j *Judge) WithMaturityTracker(mt *domain.MaturityTracker) *Judge {
	j.maturityTracker = mt
	return j
}

// WithParameters sets the Judge's parameters configuration.
// Returns the Judge for chainable usage.
func (j *Judge) WithParameters(p *config.ParametersConfig) *Judge {
	j.params = p
	j.oosValidator = j.oosValidator.WithParameters(p)
	return j
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
	checks = append(
		checks,
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
	result.BaselineReturns = summary.BaselineReturns
	result.CandidateReturns = summary.CandidateReturns
	result.BaselineFallbackCount = summary.BaselineFallbackStats.FallbackCount
	result.CandidateFallbackCount = summary.CandidateFallbackStats.FallbackCount
	result.BaselineFactorCount = summary.BaselineFallbackStats.TotalCount
	result.CandidateFactorCount = summary.CandidateFallbackStats.TotalCount
	result.BaselineMonetaryNTD = summary.BaselineMonetaryNTD
	result.CandidateMonetaryNTD = summary.CandidateMonetaryNTD
	result.Experiment.BaselineMonetaryNTD = summary.BaselineMonetaryNTD
	result.Experiment.CandidateMonetaryNTD = summary.CandidateMonetaryNTD
	result.Experiment.WindowStart = windowSummary.StartDate
	result.Experiment.WindowEnd = windowSummary.EndDate
	result.EvaluationMode = "prompt_aware_replay_judged"
	result.JudgeChecks = checks
	result.RecordedAt = time.Now()

	// Attach formal eval metrics (R²_OOS, Sharpe, CumReturn, MaxDD) using the
	// canonical eval package. Uses candidate returns for strategy evaluation.
	if len(result.CandidateReturns) > 0 {
		var r2OOS float64
		if len(result.BaselineReturns) == len(result.CandidateReturns) && len(result.BaselineReturns) > 0 {
			r2OOS = eval.OOSR2(result.BaselineReturns, result.CandidateReturns)
		}
		result.EvalMetrics = &eval.EvalResult{
			R2OOS:     r2OOS,
			Sharpe:    eval.SharpeRatio(result.CandidateReturns, 0),
			CumReturn: eval.CumulativeReturn(result.CandidateReturns),
			MaxDD:     eval.MaxDrawdown(result.CandidateReturns),
		}
	}

	// Regime-conditional evaluation: when HistoricalStore is wired, split
	// performance by market regime to prevent promoting strategies that only
	// work in a single regime.
	if j.historicalStore != nil && len(result.CandidateReturns) > 0 {
		j.addRegimeConditionalChecks(&result)
	}

	// Load parameter snapshot and perform sensitivity analysis
	if result.ParameterSnapshotID != "" {
		snapStore := config.NewSnapshotStore(constants.StateParameterSnapshots)
		if snap, err := snapStore.LoadSnapshot(result.ParameterSnapshotID); err == nil {
			checks = append(checks, fmt.Sprintf("parameter snapshot loaded: %s", result.ParameterSnapshotID))
			if snap.Params != nil {
				// Compare current parameters with experiment parameters
				currentParams := j.params
				if currentParams != nil {
					diffs := config.DiffSnapshots(
						&config.ParameterSnapshot{Params: snap.Params},
						&config.ParameterSnapshot{Params: currentParams},
					)
					if len(diffs) > 0 {
						checks = append(checks, fmt.Sprintf("WARNING: %d parameters changed since experiment", len(diffs)))
						for _, diff := range diffs {
							checks = append(checks, fmt.Sprintf("  - %s: %v → %v", diff.Parameter, diff.OldValue, diff.NewValue))
						}
					} else {
						checks = append(checks, "parameters unchanged since experiment")
					}
				}
			}
		} else {
			checks = append(checks, fmt.Sprintf("parameter snapshot not found: %s", result.ParameterSnapshotID))
		}
		result.JudgeChecks = checks
	}

	// Compute factor weight deviation between experiment snapshot and current config.
	weightDrift := computeWeightDrift(result)
	if weightDrift > 0.05 {
		checks = append(checks, fmt.Sprintf("WARNING: factor weight drift %.1f%% (results may be regime-confounded)", weightDrift*100))
	} else if weightDrift > 0 {
		checks = append(checks, fmt.Sprintf("factor weight drift %.1f%% (acceptable)", weightDrift*100))
	}
	result.JudgeChecks = checks

	// OOS validation: run BEFORE passesAcceptance so the no_drawdown_spike
	// gate can inspect the populated OOSResult rather than always seeing nil.
	oosResult, oosErr := j.oosValidator.ValidateWithBrief(candidatePromptPath, j.baselinePath, result.Brief, windowSummary.EndDate)
	result.OOSResult = oosResult

	accepted, acceptanceNote := j.passesAcceptance(result, promptBytes)
	result.JudgeChecks = append(result.JudgeChecks, acceptanceNote)
	if result.Experiment.ApprovalID == "" {
		result.Experiment.ApprovalID = "approval-" + result.Experiment.ID
	}
	if accepted {
		if oosErr != nil {
			// OOS validation error — reject conservatively.
			if err := j.transitionExperimentStatus(&result.Experiment, domain.ExperimentRejected); err != nil {
				return domain.PromptExperimentResult{}, fmt.Errorf("transition experiment status: %w", err)
			}
			result.Experiment.RevertReason = fmt.Sprintf("OOS validation error: %v", oosErr)
			result.Notes = append(result.Notes, "Replay judge rejected due to OOS validation error.")
		} else if !oosResult.Passed {
			// OOS failed — reject without accepting.
			if err := j.transitionExperimentStatus(&result.Experiment, domain.ExperimentRejected); err != nil {
				return domain.PromptExperimentResult{}, fmt.Errorf("transition experiment status: %w", err)
			}
			result.Experiment.RevertReason = fmt.Sprintf("OOS validation failed: %s", oosResult.Reason)
			result.Notes = append(result.Notes, fmt.Sprintf("Replay judge rejected on OOS gate: %s.", oosResult.Reason))
		} else {
			// OOS passed — accept.
			if err := j.transitionExperimentStatus(&result.Experiment, domain.ExperimentAccepted); err != nil {
				return domain.PromptExperimentResult{}, fmt.Errorf("transition experiment status: %w", err)
			}
			result.Notes = append(result.Notes, "Replay judge accepted the candidate for the next baseline promotion step.")

			// Compute factor importance if replay data is available.
			j.computeAndAttachImportance(&result)
		}
	} else {
		if err := j.transitionExperimentStatus(&result.Experiment, domain.ExperimentRejected); err != nil {
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
func (j *Judge) WithAcceptancePipeline(enabled bool) *Judge {
	j.useAcceptancePipeline = enabled
	return j
}

// WithHistoricalStore injects a HistoricalStore for regime-conditional evaluation.
// When wired, Evaluate splits performance metrics by market regime and adds a
// regime_diversified acceptance gate. nil means no per-regime split (backward compatible).
func (j *Judge) WithHistoricalStore(hs ledger.HistoricalStore) *Judge {
	j.historicalStore = hs
	return j
}

func (j *Judge) passesAcceptance(result domain.PromptExperimentResult, promptBytes []byte) (bool, string) {
	// Burn-in gate: do not judge experiments until statistical engines are reliable.
	if j.maturityTracker != nil && j.maturityTracker.Current() == domain.MaturityBurnIn {
		return false, fmt.Sprintf("rejected: burn_in mode (%d days until calibrating)",
			j.maturityTracker.DaysUntil(domain.MaturityCalibrating))
	}

	// Fallback window gate: once the system has its own replay history, the
	// fallback window can overlap the OOS window and contaminate the result.
	// Permitted only while in burn-in (when no alternative data exists).
	if result.UsedFallbackWindow && j.maturityTracker != nil &&
		j.maturityTracker.Current() != domain.MaturityBurnIn {
		return false, "rejected: experiment used fallback backtest window after burn-in; results may overlap OOS window"
	}

	gates := result.Experiment.AcceptanceGates
	baseline := result.Experiment.BaselineValue
	candidate := result.Experiment.CandidateValue
	checks := result.JudgeChecks
	baselineObs := result.BaselineObservations
	candidateObs := result.CandidateObservations

	if len(gates) == 0 {
		return false, "rejected: no acceptance gates configured"
	}
	minObs := j.requiredObservationCountForProfile(result.Brief.MaturityLevel, result.Experiment.MutationType)
	if baselineObs < minObs || candidateObs < minObs {
		if j.eventBus != nil {
			j.eventBus.PublishExperimentInsufficientData(
				result.Experiment.ID,
				baselineObs,
				candidateObs,
				minObs,
				result.Brief.MaturityLevel,
				result.UsedFallbackWindow,
			)
		}
		return false, fmt.Sprintf("rejected: insufficient replay observations (baseline=%d candidate=%d required=%d)", baselineObs, candidateObs, minObs)
	}
	if candidate <= baseline {
		if candidate == baseline {
			return false, "rejected: candidate score equals baseline (no constraint delta applied)"
		}
		return false, "rejected: candidate did not improve over baseline"
	}

	requiredImprovement := j.requiredImprovementForProfile(result.Experiment.MutationType)
	if candidate-baseline < requiredImprovement {
		return false, "rejected: improvement below mutation profile threshold"
	}

	// Statistical significance check using Welch's t-test.
	// Run only when session returns are available to preserve backward compatibility.
	if len(result.BaselineReturns) >= 2 && len(result.CandidateReturns) >= 2 {
		tStat, _ := welchTTest(result.BaselineReturns, result.CandidateReturns)
		threshold := j.params.Experiment.WelchTTestThreshold.Value
		if math.Abs(tStat) < threshold {
			return false, fmt.Sprintf("rejected: candidate improvement not statistically significant (|t|=%.2f < %.1f)", tStat, threshold)
		}
	}

	requiredChecks := j.requiredCheckCountForProfile(result.Brief.MaturityLevel, result.Experiment.MutationType)

	if j.useAcceptancePipeline {
		return j.runAcceptancePipeline(result, promptBytes)
	}

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
		case "maintain_sharpe_like":
			if len(result.BaselineReturns) >= 2 {
				stable, _, err := SharpeStabilityCheck(result.BaselineReturns, j.params.Experiment.SharpeStabilityThreshold.Value)
				if err != nil {
					return false, fmt.Sprintf("rejected: baseline Sharpe stability check error: %v", err)
				}
				if !stable {
					return false, "rejected: baseline Sharpe ratio not statistically stable"
				}
			}
			if len(result.CandidateReturns) >= 2 {
				stable, _, err := SharpeStabilityCheck(result.CandidateReturns, j.params.Experiment.SharpeStabilityThreshold.Value)
				if err != nil {
					return false, fmt.Sprintf("rejected: candidate Sharpe stability check error: %v", err)
				}
				if !stable {
					return false, "rejected: candidate Sharpe ratio not statistically stable"
				}
			}
		case "no_drawdown_spike":
			if result.OOSResult != nil && !result.OOSResult.Passed {
				return false, fmt.Sprintf("rejected: OOS validation failed: %s", result.OOSResult.Reason)
			}
		case "regime_diversified":
			if j.historicalStore == nil {
				continue
			}
			regimePassed, regimeMsg := j.checkRegimeDiversified(result)
			if !regimePassed {
				return false, regimeMsg
			}
		case "preserve_downside_protection":
			baselineDD := eval.MaxDrawdown(result.BaselineReturns)
			candidateDD := eval.MaxDrawdown(result.CandidateReturns)
			ratio := j.params.Experiment.DrawdownProtectionRatio.Value
			if candidateDD > baselineDD*ratio {
				return false, fmt.Sprintf("rejected: candidate drawdown %.4f exceeds %.0f%% of baseline %.4f", candidateDD, ratio*100, baselineDD)
			}
		case "reduce_concentration_risk":
			baselineVol := calculateVolatility(result.BaselineReturns)
			candidateVol := calculateVolatility(result.CandidateReturns)
			ratio := j.params.Experiment.VolatilityToleranceRatio.Value
			if candidateVol > baselineVol*ratio {
				return false, fmt.Sprintf("rejected: candidate volatility %.2f exceeds %.1fx baseline %.2f", candidateVol, ratio, baselineVol)
			}
		case "factor_quality":
			maxRatio := j.params.Experiment.MaxFallbackRatio.Value
			if result.BaselineFactorCount > 0 {
				baselineRatio := float64(result.BaselineFallbackCount) / float64(result.BaselineFactorCount)
				if baselineRatio > maxRatio {
					checks = append(checks, fmt.Sprintf("WARNING: baseline fallback ratio %.1f%% exceeds threshold %.1f%%", baselineRatio*100, maxRatio*100))
				}
			}
			if result.CandidateFactorCount > 0 {
				candidateRatio := float64(result.CandidateFallbackCount) / float64(result.CandidateFactorCount)
				if candidateRatio > maxRatio {
					return false, fmt.Sprintf("rejected: candidate fallback ratio %.1f%% exceeds threshold %.1f%%", candidateRatio*100, maxRatio*100)
				}
			}
		case "factor_weight_stability":
			drift := computeWeightDrift(result)
			maxDrift := j.params.Experiment.FactorWeightDriftThreshold.Value
			if drift > maxDrift {
				return false, fmt.Sprintf("rejected: factor weight drift %.1f%% exceeds threshold %.1f%% (regime-confounded)", drift*100, maxDrift*100)
			}
		case "reduce_false_positive_rate":
			baselineFPR := negativeReturnRatio(result.BaselineReturns)
			candidateFPR := negativeReturnRatio(result.CandidateReturns)
			ratio := j.params.Experiment.VolatilityToleranceRatio.Value
			if candidateFPR > baselineFPR*ratio {
				return false, fmt.Sprintf("rejected: candidate false positive rate %.1f%% exceeds %.1fx baseline %.1f%%", candidateFPR*100, ratio, baselineFPR*100)
			}
		case "maintain_cro_authority":
			if result.CandidateObservations > 0 && result.BaselineObservations > 0 {
				ratio := float64(result.CandidateObservations) / float64(result.BaselineObservations)
				maxGrowth := j.params.Experiment.VolatilityToleranceRatio.Value
				if ratio > maxGrowth {
					return false, fmt.Sprintf("rejected: candidate observation growth %.1fx exceeds authority threshold %.1fx", ratio, maxGrowth)
				}
			}
		case "reduce_sector_blindspots":
			if result.CandidateObservations < result.BaselineObservations {
				ratio := float64(result.CandidateObservations) / float64(result.BaselineObservations)
				minCoverage := 0.5
				if ratio < minCoverage {
					return false, fmt.Sprintf("rejected: candidate sector coverage %.0f%% below %.0f%% of baseline", ratio*100, minCoverage*100)
				}
			}
		case "maintain_industry_coverage":
			if result.CandidateObservations < result.BaselineObservations {
				ratio := float64(result.CandidateObservations) / float64(result.BaselineObservations)
				minCoverage := 0.5
				if ratio < minCoverage {
					return false, fmt.Sprintf("rejected: candidate industry coverage %.0f%% below %.0f%% of baseline", ratio*100, minCoverage*100)
				}
			}
		case "reduce_style_drift":
			if result.CandidateFactorCount > 0 && result.BaselineFactorCount > 0 {
				candidateRatio := float64(result.CandidateFallbackCount) / float64(result.CandidateFactorCount)
				baselineRatio := float64(result.BaselineFallbackCount) / float64(result.BaselineFactorCount)
				maxDrift := j.params.Experiment.FactorWeightDriftThreshold.Value
				if candidateRatio > baselineRatio+maxDrift {
					return false, fmt.Sprintf("rejected: candidate style drift %.1f%% exceeds baseline %.1f%% by > %.1f%%", candidateRatio*100, baselineRatio*100, maxDrift*100)
				}
			}
		case "maintain_momentum_catch":
			baselineMCR := positiveReturnRatio(result.BaselineReturns)
			candidateMCR := positiveReturnRatio(result.CandidateReturns)
			if candidateMCR < baselineMCR-0.1 {
				return false, fmt.Sprintf("rejected: candidate momentum catch rate %.1f%% below baseline %.1f%%", candidateMCR*100, baselineMCR*100)
			}
		case "retail_sentiment_filter":
			if math.Abs(result.Brief.RSITwScore) >= 0.7 {
				return false, fmt.Sprintf("rejected: extreme retail sentiment (%.2f) — noisy environment", result.Brief.RSITwScore)
			}
		case "respect_holding_period":
			if !promptMentionsHoldingPeriod(string(promptBytes)) {
				return false, "rejected: candidate prompt does not declare a holding period or max_holding_days constraint"
			}
		default:
			return false, fmt.Sprintf("rejected: unknown gate %q", gate)
		}
	}
	return true, "accepted: maturity-aware gates satisfied"
}

func (j *Judge) runAcceptancePipeline(result domain.PromptExperimentResult, promptBytes []byte) (bool, string) {
	registry := acceptance.NewRegistry()
	registry.Register(builtin.ImproveSharpeLike())
	registry.Register(builtin.PreserveDownsideProtection())
	registry.Register(builtin.NoDrawdownSpike())
	registry.Register(builtin.FactorWeightStability())
	registry.Register(builtin.RetailSentimentFilter())
	// Remaining 12 gates ported from legacy switch.
	registry.Register(builtin.NoMaterialDrawdownDegradation())
	registry.Register(builtin.NoConstraintBypass())
	registry.Register(builtin.MaintainSharpeLike())
	registry.Register(builtin.ReduceConcentrationRisk())
	registry.Register(builtin.FactorQuality())
	registry.Register(builtin.ReduceFalsePositiveRate())
	registry.Register(builtin.MaintainCROAuthority())
	registry.Register(builtin.ReduceSectorBlindspots())
	registry.Register(builtin.MaintainIndustryCoverage())
	registry.Register(builtin.ReduceStyleDrift())
	registry.Register(builtin.MaintainMomentumCatch())
	registry.Register(builtin.RespectHoldingPeriod())

	params := acceptance.EvalParams{
		DrawdownProtectionRatio:    j.params.Experiment.DrawdownProtectionRatio.Value,
		VolatilityToleranceRatio:   j.params.Experiment.VolatilityToleranceRatio.Value,
		MaxFallbackRatio:           j.params.Experiment.MaxFallbackRatio.Value,
		FactorWeightDriftThreshold: j.params.Experiment.FactorWeightDriftThreshold.Value,
		SharpeStabilityThreshold:   j.params.Experiment.SharpeStabilityThreshold.Value,
		PromptBytes:                promptBytes,
	}

	for _, gate := range result.Experiment.AcceptanceGates {
		e, ok := registry.Get(gate)
		if !ok {
			return false, fmt.Sprintf("rejected: no evaluator registered for gate %q (legacy switch may handle it)", gate)
		}
		r := e.Eval(result, params)
		if !r.Passed {
			return false, r.Reason
		}
	}
	return true, "accepted: acceptance pipeline passed"
}

func welchTTest(baselineReturns, candidateReturns []float64) (tStat float64, df float64) {
	if len(baselineReturns) < 63 || len(candidateReturns) < 63 {
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

	df = (se1 + se2) * (se1 + se2) / (se1*se1/(n1-1) + se2*se2/(n2-1))

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

func calculateVolatility(returns []float64) float64 {
	if len(returns) < 30 {
		return 0
	}
	_, variance := meanAndVariance(returns)
	return math.Sqrt(variance)
}

func positiveReturnRatio(returns []float64) float64 {
	if len(returns) == 0 {
		return 0
	}
	var n int
	for _, r := range returns {
		if r > 0 {
			n++
		}
	}
	return float64(n) / float64(len(returns))
}

func negativeReturnRatio(returns []float64) float64 {
	if len(returns) == 0 {
		return 0
	}
	var n int
	for _, r := range returns {
		if r < 0 {
			n++
		}
	}
	return float64(n) / float64(len(returns))
}

func (j *Judge) requiredImprovementForProfile(mutationType string) float64 {
	base := j.params.Experiment.ImprovementThreshold.Value
	switch mutationType {
	case "risk_rule_change", "portfolio_constraint_revision":
		return base * 2
	default:
		return base
	}
}

func (j *Judge) requiredCheckCountForProfile(maturity, mutationType string) int {
	base := j.requiredCheckCountForMaturity(maturity)
	switch mutationType {
	case "risk_rule_change":
		return base + 1
	case "portfolio_constraint_revision":
		return base + 2
	default:
		return base
	}
}

func (j *Judge) requiredObservationCountForProfile(maturity, mutationType string) int {
	base := j.requiredObservationCountForMaturity(maturity)
	switch mutationType {
	case "risk_rule_change", "portfolio_constraint_revision":
		return base + 1
	default:
		return base
	}
}

func (j *Judge) requiredObservationCountForMaturity(maturity string) int {
	switch maturity {
	case "level_3_regime_aware":
		return j.params.Experiment.MaturityLevel3Observations.Value
	case "level_2_window_validated", "level_2_validated":
		return j.params.Experiment.MaturityLevel2Observations.Value
	case "level_1_exploratory":
		return j.params.Experiment.MaturityLevel1Observations.Value
	default:
		return j.params.Experiment.MaturityLevel1Observations.Value
	}
}

func (j *Judge) requiredCheckCountForMaturity(maturity string) int {
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

// addRegimeConditionalChecks loads regime history for the experiment window
// and appends per-regime distribution information to the JudgeChecks. It also
// sets result.RegimeDistribution for use by the regime_diversified gate.
func (j *Judge) addRegimeConditionalChecks(result *domain.PromptExperimentResult) {
	ctx := context.Background()
	// Load a generous limit to cover the entire experiment window.
	rows, err := j.historicalStore.LoadRegimeHistoryAll(ctx, 365)
	if err != nil {
		result.JudgeChecks = append(result.JudgeChecks,
			fmt.Sprintf("regime: failed to load history: %v", err))
		return
	}

	start := result.Experiment.WindowStart
	end := result.Experiment.WindowEnd

	// Build a map from date to regime for the experiment window.
	regimeCounts := make(map[string]int)
	regimeByDate := make(map[string]string)
	for _, row := range rows {
		d, parseErr := time.Parse("2006-01-02", row.Date)
		if parseErr != nil {
			continue
		}
		if (d.Equal(start) || d.After(start)) && (d.Equal(end) || d.Before(end)) {
			regimeByDate[row.Date] = row.Regime
			regimeCounts[row.Regime]++
		}
	}

	totalDays := 0
	for _, c := range regimeCounts {
		totalDays += c
	}

	if totalDays == 0 {
		result.JudgeChecks = append(result.JudgeChecks,
			"regime: no regime data available for experiment window (gate skipped)")
		return
	}

	// Build regime distribution report.
	var distParts []string
	regimes := []string{"RISK_ON", "NEUTRAL", "RISK_OFF", "TRANSITIONAL"}
	result.RegimeCounts = make(map[string]int)
	for _, r := range regimes {
		if c, ok := regimeCounts[r]; ok {
			result.RegimeCounts[r] = c
			pct := float64(c) / float64(totalDays) * 100
			distParts = append(distParts, fmt.Sprintf("%s=%d(%.0f%%)", r, c, pct))
		}
	}
	result.RegimeTotalDays = totalDays

	result.JudgeChecks = append(result.JudgeChecks,
		fmt.Sprintf("regime distribution: %s", strings.Join(distParts, " ")))

	// Warn if single-regime window.
	if len(result.RegimeCounts) == 1 {
		for r := range result.RegimeCounts {
			result.JudgeChecks = append(result.JudgeChecks,
				fmt.Sprintf("WARNING: experiment window is single-regime (%s); results may not generalize", r))
		}
	}
}

// checkRegimeDiversified evaluates the regime_diversified acceptance gate.
// It passes when the experiment window contains at least 2 distinct regimes
// each with ≥10% of trading days. A single-regime window is rejected.
func (j *Judge) checkRegimeDiversified(result domain.PromptExperimentResult) (bool, string) {
	if result.RegimeTotalDays == 0 {
		return true, "regime gate skipped: no regime data"
	}

	regimeCounts := result.RegimeCounts
	if len(regimeCounts) < 2 {
		return false, fmt.Sprintf("rejected: single-regime window (%d days total)", result.RegimeTotalDays)
	}

	// Count regimes with ≥10% representation.
	significantRegimes := 0
	for _, c := range regimeCounts {
		if float64(c)/float64(result.RegimeTotalDays) >= 0.10 {
			significantRegimes++
		}
	}

	if significantRegimes < 2 {
		return false, fmt.Sprintf("rejected: only %d regimes with ≥10%% representation (need ≥2)", significantRegimes)
	}

	return true, fmt.Sprintf("regime_diversified: %d significant regimes across %d days", significantRegimes, result.RegimeTotalDays)
}

// testJudge returns a Judge with default parameters for testing purposes.
func testJudge() *Judge {
	return &Judge{
		params: config.DefaultParametersConfig(),
	}
}

// computeWeightDrift calculates the average absolute deviation of factor weights
// between experiment snapshot and current config. Returns 0 if no drift detected.
func computeWeightDrift(result domain.PromptExperimentResult) float64 {
	if result.ParameterSnapshotID == "" {
		return 0
	}
	snapStore := config.NewSnapshotStore(constants.StateParameterSnapshots)
	snap, err := snapStore.LoadSnapshot(result.ParameterSnapshotID)
	if err != nil || snap.Params == nil {
		return 0
	}
	expWeights := snap.Params.FactorWeight.BaseWeights.Value
	curParams := config.GetParametersConfig()
	if curParams == nil || curParams.FactorWeight.BaseWeights.Value == nil {
		return 0
	}
	curWeights := curParams.FactorWeight.BaseWeights.Value
	if len(expWeights) == 0 || len(curWeights) == 0 {
		return 0
	}
	var totalDrift float64
	var count int
	for k, v := range curWeights {
		if old, ok := expWeights[k]; ok && old > 0 {
			totalDrift += math.Abs(v-old) / old
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return totalDrift / float64(count)
}

// computeAndAttachImportance loads replay data, extracts features, and runs
// permutation importance. Results are attached to result.ImportanceResult.
// Failures are non-fatal — they are logged as notes.
func (j *Judge) computeAndAttachImportance(result *domain.PromptExperimentResult) {
	if j.replayDataPath == "" {
		return
	}

	ds, err := replay.LoadTWSEOpenDataCSV(j.replayDataPath)
	if err != nil {
		result.Notes = append(result.Notes, fmt.Sprintf("importance: failed to load replay data: %v", err))
		return
	}

	// Flatten dataset into sorted bars.
	var bars []domain.DailyBar
	for _, date := range ds.Dates {
		for _, bar := range ds.ByDate[date.Format("2006-01-02")] {
			bars = append(bars, bar)
		}
	}
	sort.Slice(bars, func(i, j int) bool { return bars[i].Date.Before(bars[j].Date) })

	if len(bars) < 200 {
		result.Notes = append(result.Notes, fmt.Sprintf("importance: %d bars insufficient for reliable importance", len(bars)))
		return
	}

	defaultFeatures := []string{"close", "volume", "return_1d"}
	unknown := feature.Validate(defaultFeatures)
	if len(unknown) > 0 {
		result.Notes = append(result.Notes, fmt.Sprintf("importance: unknown features: %v", unknown))
		return
	}

	p := NewFactorPredictor()
	imp, err := p.ComputeImportanceFromBars(bars, defaultFeatures, 5)
	if err != nil {
		result.Notes = append(result.Notes, fmt.Sprintf("importance: computation failed: %v", err))
		return
	}

	result.ImportanceResult = &imp
}

func promptMentionsHoldingPeriod(prompt string) bool {
	lower := strings.ToLower(prompt)
	return strings.Contains(lower, "holding_period") ||
		strings.Contains(lower, "max_holding_days") ||
		strings.Contains(lower, "holding days") ||
		strings.Contains(lower, "max holding") ||
		strings.Contains(lower, "exit_rule")
}
