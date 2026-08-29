package orchestrator

import (
	"context"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// ScenarioExplainer is a package-level hook for LLM-based explanation of
// PRISM training results. When non-nil, PRISMTrainingExecutor.Run calls
// this function after computing a TrainingResult and, on success, stores
// the returned explanation in result.Explanation.
//
// Set by main.go when config.LLMPrismScenarioEnabled is true and an LLM
// router is available. Uses var indirection to avoid an orchestrator→llm
// import cycle.
var ScenarioExplainer func(ctx context.Context, result any) (string, error)

// PRISMTrainingExecutor runs actual backtests over replay data for PRISM training tasks.
type PRISMTrainingExecutor struct {
	dataset  *replay.Dataset
	registry domain.AgentRegistry
	policy   baseline.Policy
}

// NewPRISMTrainingExecutor creates a real training executor backed by replay data.
func NewPRISMTrainingExecutor(dataset *replay.Dataset, registry domain.AgentRegistry, policy baseline.Policy) *PRISMTrainingExecutor {
	return &PRISMTrainingExecutor{
		dataset:  dataset,
		registry: registry,
		policy:   policy,
	}
}

// Run runs the agent over the training window and returns real metrics.
func (e *PRISMTrainingExecutor) Run(task prism.TrainingTask) (prism.TrainingResult, error) {
	if e.dataset == nil {
		return prism.TrainingResult{}, fmt.Errorf("no replay dataset available")
	}

	symbols := RegistrySymbols(e.registry)
	outcomes := make([]domain.RecommendationOutcome, 0)

	for _, date := range e.dataset.WindowDates(task.WindowStart, task.WindowEnd, 1) {
		quotes := e.dataset.QuotesForDate(date, symbols)
		regime, rawRecs, _, _ := ExecuteRegistryResearchDetailedWithPolicyAndGuards(
			e.registry, quotes, e.policy.PromptOverrides, e.policy.ExecutionPolicy,
		)
		if mapDomainRegimeToPRISMTrainingRegime(regime) != task.Regime {
			continue
		}

		for _, rec := range rawRecs {
			if rec.Agent != task.AgentID {
				continue
			}
			fr, ok := e.dataset.ForwardReturn(rec.Symbol, date, 1)
			if !ok {
				continue
			}
			outcomes = append(outcomes, domain.RecommendationOutcome{
				AgentID:             rec.Agent,
				Skill:               rec.Skill,
				Symbol:              rec.Symbol,
				Window:              date.Format("2006-01-02"),
				ForwardReturn:       fr,
				BenchmarkDelta:      fr - 0.003,
				Hit:                 fr > 0,
				Reason:              rec.Reason,
				RecordedAt:          date,
				FactorScores:        rec.FactorScores,
				ConvictionBreakdown: rec.ConvictionBreakdown,
				Regime:              string(regime),
			})
		}
	}

	if len(outcomes) == 0 {
		return prism.TrainingResult{}, fmt.Errorf("no outcomes for agent %s in window", task.AgentID)
	}

	scorecards := ledger.BuildScorecards(outcomes)
	var sc *domain.Scorecard
	for i := range scorecards {
		if scorecards[i].AgentID == task.AgentID {
			sc = &scorecards[i]
			break
		}
	}
	if sc == nil {
		return prism.TrainingResult{}, fmt.Errorf("no scorecard for agent %s", task.AgentID)
	}

	returns := make([]float64, 0, len(outcomes))
	for _, o := range outcomes {
		returns = append(returns, o.ForwardReturn)
	}

	winCount := countPositive(returns)
	result := prism.TrainingResult{
		HitRate:      sc.HitRate,
		SharpeRatio:  sc.SharpeLike,
		MaxDrawdown:  sc.MaxDrawdown,
		TotalReturn:  sumReturns(returns),
		SignalsCount: sc.Observations,
		WinCount:     winCount,
		LossCount:    len(returns) - winCount,
	}

	if ScenarioExplainer != nil {
		explanation, err := ScenarioExplainer(context.Background(), result)
		if err == nil {
			result.Explanation = explanation
		}
	}

	return result, nil
}

func mapDomainRegimeToPRISMTrainingRegime(r domain.Regime) prism.RegimeType {
	switch r {
	case domain.RegimeRiskOn:
		return prism.RegimeRiskOn
	case domain.RegimeRiskOff:
		return prism.RegimeRiskOff
	case domain.RegimeNeutral:
		return prism.RegimeLowVolatility
	default:
		return prism.RegimeTransition
	}
}

// calculateMaxDrawdown is kept for compatibility with adversarial_executor.go.
// For PRISM training results, use ledger.BuildScorecards().MaxDrawdown instead.
func calculateMaxDrawdown(returns []float64) float64 {
	if len(returns) == 0 {
		return 0
	}

	peak := 1.0
	maxDD := 0.0
	cum := 1.0
	for _, r := range returns {
		cum *= 1 + r
		if cum > peak {
			peak = cum
		}
		dd := (peak - cum) / peak
		if dd > maxDD {
			maxDD = dd
		}
	}

	return -maxDD
}

func sumReturns(returns []float64) float64 {
	sum := 0.0
	for _, r := range returns {
		sum += r
	}
	return sum
}

func countPositive(returns []float64) int {
	count := 0
	for _, r := range returns {
		if r > 0 {
			count++
		}
	}
	return count
}
