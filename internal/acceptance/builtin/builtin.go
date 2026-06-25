// Package builtin provides Evaluator implementations that mirror the
// acceptance gates currently inlined in experiment/judge.go passesAcceptance.
//
// These are the first 5 of 17 gates ported as a migration seed. The
// remaining 12 can be ported incrementally using the same pattern.
//
// Maturity: evolving
package builtin

import (
	"fmt"
	"math"

	"github.com/kaecer68/atlas-go/internal/acceptance"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eval"
)

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
