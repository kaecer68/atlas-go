package experiment

import (
	"os"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/sim"
)

func comparePromptPerformance(replayDataPath, baselinePolicyPath string, brief domain.MutationBrief, window domain.BacktestWindowSummary, candidatePromptPath string) (float64, float64, error) {
	ds, err := replay.LoadTWSEOpenDataCSV(replayDataPath)
	if err != nil {
		return 0, 0, err
	}
	policy, err := baseline.Load(baselinePolicyPath)
	if err != nil {
		return 0, 0, err
	}

	baselinePrompt := baseline.ResolvePromptOverride(policy, brief.TargetAgentID, brief.TargetSkill)
	if baselinePrompt == "" {
		baselinePromptBytes, err := os.ReadFile(brief.PromptFile)
		if err != nil {
			return 0, 0, err
		}
		baselinePrompt = string(baselinePromptBytes)
	}
	candidatePromptBytes, err := os.ReadFile(candidatePromptPath)
	if err != nil {
		return 0, 0, err
	}

	switch brief.MutationType {
	case "risk_rule_change", "portfolio_constraint_revision":
		baselineConstraints := policy.Constraints
		candidateConstraints := baseline.ApplyConstraintCandidate(policy.Constraints, string(candidatePromptBytes))
		baseline := scoreConstraintWindow(ds, baselineConstraints, window.StartDate, window.EndDate)
		candidate := scoreConstraintWindow(ds, candidateConstraints, window.StartDate, window.EndDate)
		return baseline, candidate, nil
	default:
		baseline := scorePromptWindow(ds, brief.TargetSkill, baselinePrompt, policy.ExecutionPolicy, window.StartDate, window.EndDate)
		candidate := scorePromptWindow(ds, brief.TargetSkill, string(candidatePromptBytes), policy.ExecutionPolicy, window.StartDate, window.EndDate)
		return baseline, candidate, nil
	}
}

func scorePromptWindow(ds *replay.Dataset, skill, prompt string, policy domain.ExecutionPolicy, startDate, endDate time.Time) float64 {
	if ds == nil {
		return 0
	}

	registry := orchestrator.SeedRegistry()
	total := 0.0
	observations := 0
	for _, date := range ds.Dates {
		if date.Before(startDate) || date.After(endDate) {
			continue
		}
		nextDate, ok := ds.NextDate(date, 1)
		if !ok || nextDate.After(endDate) {
			continue
		}

		symbols := orchestrator.SymbolsForSkill(registry, skill)
		dayQuotes := ds.QuotesForDate(date, symbols)
		overrides := map[string]string{skill: prompt}
		_, rawRecs, _ := orchestrator.ExecuteRegistryResearchDetailedWithPolicy(registry, dayQuotes, overrides, policy)
		for _, rec := range rawRecs {
			if rec.Skill != skill {
				continue
			}
			forwardReturn, ok := ds.ForwardReturn(rec.Symbol, date, 1)
			if !ok {
				continue
			}
			total += forwardReturn * (float64(rec.Conviction) / 100.0)
			observations++
		}
	}

	if observations == 0 {
		return 0
	}
	return total / float64(observations)
}

func scoreConstraintWindow(ds *replay.Dataset, constraints domain.SimulationConstraints, startDate, endDate time.Time) float64 {
	if ds == nil {
		return 0
	}

	registry := orchestrator.SeedRegistry()
	symbols := orchestrator.RegistrySymbols(registry)
	engine := sim.NewEngine(constraints)

	total := 0.0
	observations := 0
	for _, date := range ds.Dates {
		if date.Before(startDate) || date.After(endDate) {
			continue
		}
		nextDate, ok := ds.NextDate(date, 1)
		if !ok || nextDate.After(endDate) {
			continue
		}

		quotes := ds.QuotesForDate(date, symbols)
		nextQuotes := ds.QuotesForDate(nextDate, symbols)
		execPolicy := baseline.ExecutionPolicyFromConstraints(constraints)
		regime, _, activeRecs := orchestrator.ExecuteRegistryResearchDetailedWithPolicy(registry, quotes, nil, execPolicy)
		activeRecs = filterRecommendationsForConstraints(activeRecs, constraints)
		result := engine.Run(regime, quotes, activeRecs)
		total += scoreSimulationResult(result, nextQuotes, constraints.StartingCash)
		observations++
	}

	if observations == 0 {
		return 0
	}
	return total / float64(observations)
}

func scoreSimulationResult(result domain.SimulationResult, nextQuotes []domain.Quote, startingCash float64) float64 {
	if startingCash == 0 {
		return 0
	}
	quoteBySymbol := make(map[string]domain.Quote, len(nextQuotes))
	for _, quote := range nextQuotes {
		quoteBySymbol[quote.Symbol] = quote
	}

	endingValue := result.EndingCash
	for _, position := range result.Positions {
		if quote, ok := quoteBySymbol[position.Symbol]; ok {
			endingValue += float64(position.Quantity) * quote.Last
			continue
		}
		endingValue += position.MarketValue
	}
	return (endingValue - startingCash) / startingCash
}

func filterRecommendationsForConstraints(recs []domain.Recommendation, constraints domain.SimulationConstraints) []domain.Recommendation {
	if constraints.MinRecommendationConviction <= 0 {
		return recs
	}
	filtered := make([]domain.Recommendation, 0, len(recs))
	for _, rec := range recs {
		if rec.Conviction < constraints.MinRecommendationConviction {
			continue
		}
		filtered = append(filtered, rec)
	}
	return filtered
}
