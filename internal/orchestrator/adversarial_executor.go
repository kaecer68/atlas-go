package orchestrator

import (
	"fmt"
	"math"
	"time"

	"github.com/kaecer68/atlas-go/internal/adversarial"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// AdversarialScenarioRunner executes real adversarial stress tests by mutating
// replay quotes and measuring agent performance degradation.
type AdversarialScenarioRunner struct {
	dataset  *replay.Dataset
	registry domain.AgentRegistry
}

// NewAdversarialScenarioRunner creates a runner backed by replay data.
func NewAdversarialScenarioRunner(dataset *replay.Dataset, registry domain.AgentRegistry) *AdversarialScenarioRunner {
	return &AdversarialScenarioRunner{
		dataset:  dataset,
		registry: registry,
	}
}

// RunStressTest executes all adversarial scenarios against a target agent
// and returns real metrics based on mutated backtests.
func (r *AdversarialScenarioRunner) RunStressTest(agentID string, agent domain.AgentSpec) *adversarial.StressTestResult {
	result := &adversarial.StressTestResult{
		AgentID:         agentID,
		Timestamp:       time.Now(),
		Scenarios:       make([]adversarial.ScenarioResult, 0),
		Vulnerabilities: make([]string, 0),
	}

	scenarios := []struct {
		typ adversarial.ScenarioType
		fn  func([]domain.Quote) []domain.Quote
	}{
		{adversarial.ScenarioFlashCrash, r.mutateFlashCrash},
		{adversarial.ScenarioLiquidityCrisis, r.mutateLiquidityCrisis},
		{adversarial.ScenarioCorrelationSpike, r.mutateCorrelationSpike},
		{adversarial.ScenarioFlashRally, r.mutateFlashRally},
		{adversarial.ScenarioSectorRotation, r.mutateSectorRotation},
	}

	baseline := r.agentScoreOverWindow(agent)

	for _, sc := range scenarios {
		stressed := r.agentScoreOverWindowWithMutation(agent, sc.fn)
		score := r.calculateScenarioScore(baseline, stressed)
		passed := score >= 0.6
		result.Scenarios = append(result.Scenarios, adversarial.ScenarioResult{
			ScenarioType: string(sc.typ),
			Score:        score,
			Passed:       passed,
			Details:      fmt.Sprintf("baseline sharpe %.3f → stressed %.3f", baseline.SharpeLike, stressed.SharpeLike),
		})
		if !passed {
			result.Vulnerabilities = append(result.Vulnerabilities, string(sc.typ))
		}
	}

	total := 0.0
	for _, sr := range result.Scenarios {
		total += sr.Score
	}
	result.OverallScore = total / float64(len(result.Scenarios))
	result.Passed = result.OverallScore >= 0.6
	return result
}

type stressMetrics struct {
	HitRate    float64
	SharpeLike float64
	MaxDD      float64
	Signals    int
}

func (r *AdversarialScenarioRunner) agentScoreOverWindow(agent domain.AgentSpec) stressMetrics {
	return r.agentScoreOverWindowWithMutation(agent, nil)
}

func (r *AdversarialScenarioRunner) agentScoreOverWindowWithMutation(agent domain.AgentSpec, mutate func([]domain.Quote) []domain.Quote) stressMetrics {
	if r.dataset == nil {
		return stressMetrics{}
	}
	symbols := RegistrySymbols(r.registry)
	outcomes := make([]domain.RecommendationOutcome, 0)

	for _, date := range r.dataset.Dates {
		nextDate, ok := r.dataset.NextDate(date, 1)
		if !ok {
			continue
		}
		quotes := r.dataset.QuotesForDate(date, symbols)
		if mutate != nil {
			quotes = mutate(quotes)
		}
		regime, rawRecs, _, _ := ExecuteRegistryResearchDetailedWithPolicyAndGuards(
			r.registry, quotes, map[string]string{}, domain.ExecutionPolicy{RequireCROPass: false},
		)
		for _, rec := range rawRecs {
			if rec.Agent != agent.ID {
				continue
			}
			fr, ok := r.dataset.ForwardReturn(rec.Symbol, date, 1)
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
		_ = nextDate
	}

	if len(outcomes) == 0 {
		return stressMetrics{}
	}
	scorecards := ledger.BuildScorecards(outcomes)
	var sc *domain.Scorecard
	for i := range scorecards {
		if scorecards[i].AgentID == agent.ID {
			sc = &scorecards[i]
			break
		}
	}
	if sc == nil {
		return stressMetrics{}
	}
	returns := make([]float64, len(outcomes))
	for i, o := range outcomes {
		returns[i] = o.ForwardReturn
	}
	return stressMetrics{
		HitRate:    sc.HitRate,
		SharpeLike: sc.SharpeLike,
		MaxDD:      calculateMaxDrawdown(returns),
		Signals:    sc.Observations,
	}
}

func (r *AdversarialScenarioRunner) calculateScenarioScore(baseline, stressed stressMetrics) float64 {
	if baseline.SharpeLike == 0 && stressed.SharpeLike == 0 {
		return 0.5
	}
	if baseline.SharpeLike == 0 {
		return math.Max(0, math.Min(1, 0.5+stressed.SharpeLike))
	}
	retention := stressed.SharpeLike / baseline.SharpeLike
	// Penalize severe degradation, reward resilience
	score := 0.5 + retention*0.5
	if retention < 0 {
		score = 0.3
	}
	if stressed.MaxDD < -0.5 {
		score -= 0.2
	}
	return math.Max(0, math.Min(1, score))
}

// --- quote mutators ---

func (r *AdversarialScenarioRunner) mutateFlashCrash(quotes []domain.Quote) []domain.Quote {
	mutated := make([]domain.Quote, len(quotes))
	copy(mutated, quotes)
	for i := range mutated {
		mutated[i].Last *= 0.80
		mutated[i].High = math.Min(mutated[i].High, mutated[i].Last*1.02)
		mutated[i].Low *= 0.78
	}
	return mutated
}

func (r *AdversarialScenarioRunner) mutateLiquidityCrisis(quotes []domain.Quote) []domain.Quote {
	mutated := make([]domain.Quote, len(quotes))
	copy(mutated, quotes)
	for i := range mutated {
		mutated[i].Volume = 1
	}
	return mutated
}

func (r *AdversarialScenarioRunner) mutateCorrelationSpike(quotes []domain.Quote) []domain.Quote {
	mutated := make([]domain.Quote, len(quotes))
	copy(mutated, quotes)
	if len(mutated) == 0 {
		return mutated
	}
	// Make every symbol drop by exactly -3% to force perfect correlation
	for i := range mutated {
		mutated[i].Last *= 0.97
		mutated[i].High = math.Min(mutated[i].High, mutated[i].Last*1.01)
		mutated[i].Low *= 0.96
	}
	return mutated
}

func (r *AdversarialScenarioRunner) mutateFlashRally(quotes []domain.Quote) []domain.Quote {
	mutated := make([]domain.Quote, len(quotes))
	copy(mutated, quotes)
	for i := range mutated {
		mutated[i].Last *= 1.15
		mutated[i].High = math.Max(mutated[i].High, mutated[i].Last*1.02)
		mutated[i].Low *= 1.12
	}
	return mutated
}

func (r *AdversarialScenarioRunner) mutateSectorRotation(quotes []domain.Quote) []domain.Quote {
	mutated := make([]domain.Quote, len(quotes))
	copy(mutated, quotes)
	for i := range mutated {
		// Simple heuristic: semiconductor symbols get boosted, financials get crushed
		switch mutated[i].Symbol {
		case "2330.TW", "2303.TW", "2454.TW", "3034.TW":
			mutated[i].Last *= 1.08
		case "2881.TW", "2882.TW", "2883.TW", "2884.TW", "2885.TW", "2886.TW":
			mutated[i].Last *= 0.92
		}
	}
	return mutated
}
