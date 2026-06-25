// Package builtin provides Evaluator implementations that mirror the
// acceptance gates currently inlined in experiment/judge.go passesAcceptance.
//
// All 17 acceptance gates from judge.go's switch are now ported here. The
// legacy switch is preserved for back-compat until callers migrate to
// WithAcceptancePipeline(true).
package builtin

import (
	"fmt"
	"math"
	"strings"

	"github.com/kaecer68/atlas-go/internal/acceptance"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eval"
)

// -----------------------------------------------------------------------------
// Helpers (copied from internal/experiment/judge.go to avoid import cycle).
// experiment → acceptance would be a valid direction, but acceptance is
// designed as a leaf package consumed by experiment, not vice versa.
// -----------------------------------------------------------------------------

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

func promptMentionsHoldingPeriod(promptBytes []byte) bool {
	lower := strings.ToLower(string(promptBytes))
	return strings.Contains(lower, "holding_period") ||
		strings.Contains(lower, "max_holding_days") ||
		strings.Contains(lower, "holding days") ||
		strings.Contains(lower, "max holding") ||
		strings.Contains(lower, "exit_rule")
}

func requiredCheckCountForMaturity(maturity string) int {
	switch maturity {
	case "level_3_regime_aware":
		return 4
	case "level_2_window_validated", "level_2_validated":
		return 3
	default:
		return 2
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

func computeSharpeRatio(returns []float64) float64 {
	if len(returns) < 2 {
		return 0
	}
	mean, variance := meanAndVariance(returns)
	if variance == 0 {
		return 0
	}
	// Annualized Sharpe (rf=0, periods=days/252 for daily returns).
	return mean / math.Sqrt(variance) * math.Sqrt(252)
}

// -----------------------------------------------------------------------------
// Gates 1-5: previously ported (kept verbatim, plus computeWeightDrift).
// -----------------------------------------------------------------------------

func ImproveSharpeLike() acceptance.Evaluator {
	return acceptance.FuncEvaluator{
		N: "improve_sharpe_like",
		F: func(input domain.PromptExperimentResult, _ acceptance.EvalParams) acceptance.Result {
			if input.Experiment.CandidateValue <= input.Experiment.BaselineValue {
				return acceptance.Result{
					Passed: false,
					Reason: fmt.Sprintf("candidate %.4f did not improve over baseline %.4f", input.Experiment.CandidateValue, input.Experiment.BaselineValue),
				}
			}
			return acceptance.Result{Passed: true}
		},
	}
}

func PreserveDownsideProtection() acceptance.Evaluator {
	return acceptance.FuncEvaluator{
		N: "preserve_downside_protection",
		F: func(input domain.PromptExperimentResult, params acceptance.EvalParams) acceptance.Result {
			baselineDD := eval.MaxDrawdown(input.BaselineReturns)
			candidateDD := eval.MaxDrawdown(input.CandidateReturns)
			ratio := params.DrawdownProtectionRatio
			if candidateDD > baselineDD*ratio {
				return acceptance.Result{
					Passed: false,
					Reason: fmt.Sprintf("candidate drawdown %.4f exceeds %.0f%% of baseline %.4f", candidateDD, ratio*100, baselineDD),
				}
			}
			return acceptance.Result{Passed: true}
		},
	}
}

func NoDrawdownSpike() acceptance.Evaluator {
	return acceptance.FuncEvaluator{
		N: "no_drawdown_spike",
		F: func(input domain.PromptExperimentResult, _ acceptance.EvalParams) acceptance.Result {
			if input.OOSResult != nil && !input.OOSResult.Passed {
				return acceptance.Result{
					Passed: false,
					Reason: fmt.Sprintf("OOS validation failed: %s", input.OOSResult.Reason),
				}
			}
			return acceptance.Result{Passed: true}
		},
	}
}

func FactorWeightStability() acceptance.Evaluator {
	return acceptance.FuncEvaluator{
		N: "factor_weight_stability",
		F: func(input domain.PromptExperimentResult, params acceptance.EvalParams) acceptance.Result {
			drift := computeWeightDrift(input)
			maxDrift := params.FactorWeightDriftThreshold
			if drift > maxDrift {
				return acceptance.Result{
					Passed: false,
					Reason: fmt.Sprintf("factor weight drift %.1f%% exceeds threshold %.1f%%", drift*100, maxDrift*100),
				}
			}
			return acceptance.Result{Passed: true}
		},
	}
}

func RetailSentimentFilter() acceptance.Evaluator {
	return acceptance.FuncEvaluator{
		N: "retail_sentiment_filter",
		F: func(input domain.PromptExperimentResult, _ acceptance.EvalParams) acceptance.Result {
			if math.Abs(input.Brief.RSITwScore) >= 0.7 {
				return acceptance.Result{
					Passed: false,
					Reason: fmt.Sprintf("extreme retail sentiment (%.2f) — noisy environment", input.Brief.RSITwScore),
				}
			}
			return acceptance.Result{Passed: true}
		},
	}
}

func computeWeightDrift(input domain.PromptExperimentResult) float64 {
	if input.CandidateFactorCount == 0 || input.BaselineFactorCount == 0 {
		return 0
	}
	candidateRatio := float64(input.CandidateFallbackCount) / float64(input.CandidateFactorCount)
	baselineRatio := float64(input.BaselineFallbackCount) / float64(input.BaselineFactorCount)
	drift := candidateRatio - baselineRatio
	if drift < 0 {
		drift = -drift
	}
	return drift
}

// -----------------------------------------------------------------------------
// Gates 6-17: newly ported from experiment/judge.go switch.
// Each matches the legacy switch logic exactly. See judge.go lines 415-533
// for the source of truth.
// -----------------------------------------------------------------------------

// 6. no_material_drawdown_degradation
func NoMaterialDrawdownDegradation() acceptance.Evaluator {
	return acceptance.FuncEvaluator{
		N: "no_material_drawdown_degradation",
		F: func(input domain.PromptExperimentResult, _ acceptance.EvalParams) acceptance.Result {
			required := requiredCheckCountForProfile(
				input.Brief.MaturityLevel,
				input.Experiment.MutationType,
			)
			if len(input.JudgeChecks) < required {
				return acceptance.Result{
					Passed: false,
					Reason: fmt.Sprintf("insufficient replay checks for drawdown confidence (have %d, need %d)", len(input.JudgeChecks), required),
				}
			}
			return acceptance.Result{Passed: true}
		},
	}
}

// 7. no_constraint_bypass
func NoConstraintBypass() acceptance.Evaluator {
	return acceptance.FuncEvaluator{
		N: "no_constraint_bypass",
		F: func(input domain.PromptExperimentResult, _ acceptance.EvalParams) acceptance.Result {
			if math.IsNaN(input.Experiment.CandidateValue) {
				return acceptance.Result{
					Passed: false,
					Reason: "candidate score is NaN — constraint bypass detected",
				}
			}
			return acceptance.Result{Passed: true}
		},
	}
}

// 8. maintain_sharpe_like
// Approximates legacy SharpeStabilityCheck using simple magnitude threshold.
// Sharpe > threshold indicates the strategy's risk-adjusted return is meaningful.
func MaintainSharpeLike() acceptance.Evaluator {
	return acceptance.FuncEvaluator{
		N: "maintain_sharpe_like",
		F: func(input domain.PromptExperimentResult, params acceptance.EvalParams) acceptance.Result {
			threshold := params.SharpeStabilityThreshold
			if len(input.BaselineReturns) >= 2 {
				sharpe := computeSharpeRatio(input.BaselineReturns)
				if math.Abs(sharpe) < threshold {
					return acceptance.Result{
						Passed: false,
						Reason: fmt.Sprintf("baseline Sharpe %.2f below stability threshold %.2f", sharpe, threshold),
					}
				}
			}
			if len(input.CandidateReturns) >= 2 {
				sharpe := computeSharpeRatio(input.CandidateReturns)
				if math.Abs(sharpe) < threshold {
					return acceptance.Result{
						Passed: false,
						Reason: fmt.Sprintf("candidate Sharpe %.2f below stability threshold %.2f", sharpe, threshold),
					}
				}
			}
			return acceptance.Result{Passed: true}
		},
	}
}

// 9. reduce_concentration_risk
func ReduceConcentrationRisk() acceptance.Evaluator {
	return acceptance.FuncEvaluator{
		N: "reduce_concentration_risk",
		F: func(input domain.PromptExperimentResult, params acceptance.EvalParams) acceptance.Result {
			baselineVol := calculateVolatility(input.BaselineReturns)
			candidateVol := calculateVolatility(input.CandidateReturns)
			ratio := params.VolatilityToleranceRatio
			if candidateVol > baselineVol*ratio {
				return acceptance.Result{
					Passed: false,
					Reason: fmt.Sprintf("candidate volatility %.4f exceeds %.1fx baseline %.4f", candidateVol, ratio, baselineVol),
				}
			}
			return acceptance.Result{Passed: true}
		},
	}
}

// 10. factor_quality
// Reuses the fallback-ratio comparison (similar to FactorWeightStability but
// checks against MaxFallbackRatio rather than the per-pair drift threshold).
func FactorQuality() acceptance.Evaluator {
	return acceptance.FuncEvaluator{
		N: "factor_quality",
		F: func(input domain.PromptExperimentResult, params acceptance.EvalParams) acceptance.Result {
			maxRatio := params.MaxFallbackRatio
			if input.BaselineFactorCount > 0 {
				baselineRatio := float64(input.BaselineFallbackCount) / float64(input.BaselineFactorCount)
				if baselineRatio > maxRatio {
					return acceptance.Result{
						Passed: false,
						Reason: fmt.Sprintf("baseline fallback ratio %.1f%% exceeds threshold %.1f%%", baselineRatio*100, maxRatio*100),
					}
				}
			}
			if input.CandidateFactorCount > 0 {
				candidateRatio := float64(input.CandidateFallbackCount) / float64(input.CandidateFactorCount)
				if candidateRatio > maxRatio {
					return acceptance.Result{
						Passed: false,
						Reason: fmt.Sprintf("candidate fallback ratio %.1f%% exceeds threshold %.1f%%", candidateRatio*100, maxRatio*100),
					}
				}
			}
			return acceptance.Result{Passed: true}
		},
	}
}

// 11. reduce_false_positive_rate
func ReduceFalsePositiveRate() acceptance.Evaluator {
	return acceptance.FuncEvaluator{
		N: "reduce_false_positive_rate",
		F: func(input domain.PromptExperimentResult, params acceptance.EvalParams) acceptance.Result {
			baselineFPR := negativeReturnRatio(input.BaselineReturns)
			candidateFPR := negativeReturnRatio(input.CandidateReturns)
			ratio := params.VolatilityToleranceRatio
			if candidateFPR > baselineFPR*ratio {
				return acceptance.Result{
					Passed: false,
					Reason: fmt.Sprintf("candidate false positive rate %.1f%% exceeds %.1fx baseline %.1f%%", candidateFPR*100, ratio, baselineFPR*100),
				}
			}
			return acceptance.Result{Passed: true}
		},
	}
}

// 12. maintain_cro_authority
func MaintainCROAuthority() acceptance.Evaluator {
	return acceptance.FuncEvaluator{
		N: "maintain_cro_authority",
		F: func(input domain.PromptExperimentResult, params acceptance.EvalParams) acceptance.Result {
			if input.CandidateObservations > 0 && input.BaselineObservations > 0 {
				ratio := float64(input.CandidateObservations) / float64(input.BaselineObservations)
				maxGrowth := params.VolatilityToleranceRatio
				if ratio > maxGrowth {
					return acceptance.Result{
						Passed: false,
						Reason: fmt.Sprintf("candidate observation growth %.1fx exceeds authority threshold %.1fx", ratio, maxGrowth),
					}
				}
			}
			return acceptance.Result{Passed: true}
		},
	}
}

// 13. reduce_sector_blindspots
func ReduceSectorBlindspots() acceptance.Evaluator {
	return acceptance.FuncEvaluator{
		N: "reduce_sector_blindspots",
		F: func(input domain.PromptExperimentResult, _ acceptance.EvalParams) acceptance.Result {
			if input.CandidateObservations < input.BaselineObservations {
				ratio := float64(input.CandidateObservations) / float64(input.BaselineObservations)
				const minCoverage = 0.5
				if ratio < minCoverage {
					return acceptance.Result{
						Passed: false,
						Reason: fmt.Sprintf("candidate sector coverage %.0f%% below %.0f%% of baseline", ratio*100, minCoverage*100),
					}
				}
			}
			return acceptance.Result{Passed: true}
		},
	}
}

// 14. maintain_industry_coverage
// Kept as a separate gate (vs reduce_sector_blindspots) for forward-compat —
// the legacy switch duplicates the logic, suggesting these were originally
// intended to track different axes (sector vs industry).
func MaintainIndustryCoverage() acceptance.Evaluator {
	return acceptance.FuncEvaluator{
		N: "maintain_industry_coverage",
		F: func(input domain.PromptExperimentResult, _ acceptance.EvalParams) acceptance.Result {
			if input.CandidateObservations < input.BaselineObservations {
				ratio := float64(input.CandidateObservations) / float64(input.BaselineObservations)
				const minCoverage = 0.5
				if ratio < minCoverage {
					return acceptance.Result{
						Passed: false,
						Reason: fmt.Sprintf("candidate industry coverage %.0f%% below %.0f%% of baseline", ratio*100, minCoverage*100),
					}
				}
			}
			return acceptance.Result{Passed: true}
		},
	}
}

// 15. reduce_style_drift
func ReduceStyleDrift() acceptance.Evaluator {
	return acceptance.FuncEvaluator{
		N: "reduce_style_drift",
		F: func(input domain.PromptExperimentResult, params acceptance.EvalParams) acceptance.Result {
			if input.CandidateFactorCount > 0 && input.BaselineFactorCount > 0 {
				candidateRatio := float64(input.CandidateFallbackCount) / float64(input.CandidateFactorCount)
				baselineRatio := float64(input.BaselineFallbackCount) / float64(input.BaselineFactorCount)
				maxDrift := params.FactorWeightDriftThreshold
				if candidateRatio > baselineRatio+maxDrift {
					return acceptance.Result{
						Passed: false,
						Reason: fmt.Sprintf("candidate style drift %.1f%% exceeds baseline %.1f%% by > %.1f%%", candidateRatio*100, baselineRatio*100, maxDrift*100),
					}
				}
			}
			return acceptance.Result{Passed: true}
		},
	}
}

// 16. maintain_momentum_catch
func MaintainMomentumCatch() acceptance.Evaluator {
	return acceptance.FuncEvaluator{
		N: "maintain_momentum_catch",
		F: func(input domain.PromptExperimentResult, _ acceptance.EvalParams) acceptance.Result {
			baselineMCR := positiveReturnRatio(input.BaselineReturns)
			candidateMCR := positiveReturnRatio(input.CandidateReturns)
			if candidateMCR < baselineMCR-0.1 {
				return acceptance.Result{
					Passed: false,
					Reason: fmt.Sprintf("candidate momentum catch rate %.1f%% below baseline %.1f%% by > 10pp", candidateMCR*100, baselineMCR*100),
				}
			}
			return acceptance.Result{Passed: true}
		},
	}
}

// 17. respect_holding_period
func RespectHoldingPeriod() acceptance.Evaluator {
	return acceptance.FuncEvaluator{
		N: "respect_holding_period",
		F: func(_ domain.PromptExperimentResult, params acceptance.EvalParams) acceptance.Result {
			if !promptMentionsHoldingPeriod(params.PromptBytes) {
				return acceptance.Result{
					Passed: false,
					Reason: "candidate prompt does not declare a holding period or max_holding_days constraint",
				}
			}
			return acceptance.Result{Passed: true}
		},
	}
}
