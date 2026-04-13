package experiment

import (
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/sim"
)

type replayScoreSummary struct {
	BaselineScore         float64
	CandidateScore        float64
	BaselineObservations  int
	CandidateObservations int
	UsedFallbackWindow    bool
}

func comparePromptPerformance(replayDataPath, baselinePolicyPath string, brief domain.MutationBrief, window domain.BacktestWindowSummary, candidatePromptPath string) (float64, float64, error) {
	summary, err := comparePromptPerformanceDetailed(replayDataPath, baselinePolicyPath, brief, window, candidatePromptPath)
	if err != nil {
		return 0, 0, err
	}
	return summary.BaselineScore, summary.CandidateScore, nil
}

func comparePromptPerformanceDetailed(replayDataPath, baselinePolicyPath string, brief domain.MutationBrief, window domain.BacktestWindowSummary, candidatePromptPath string) (replayScoreSummary, error) {
	ds, err := replay.LoadTWSEOpenDataCSV(replayDataPath)
	if err != nil {
		return replayScoreSummary{}, err
	}
	policy, err := baseline.Load(baselinePolicyPath)
	if err != nil {
		return replayScoreSummary{}, err
	}

	baselinePrompt := baseline.ResolvePromptOverride(policy, brief.TargetAgentID, brief.TargetSkill)
	if baselinePrompt == "" {
		promptFile := brief.PromptFile
		if !filepath.IsAbs(promptFile) {
			if _, err := os.Stat(promptFile); err != nil {
				promptFile = filepath.Join(".", promptFile)
			}
		}
		baselinePromptBytes, err := os.ReadFile(promptFile)
		if err != nil {
			return replayScoreSummary{}, err
		}
		baselinePrompt = string(baselinePromptBytes)
	}
	candidatePromptBytes, err := os.ReadFile(candidatePromptPath)
	if err != nil {
		return replayScoreSummary{}, err
	}

	summary := replayScoreSummary{}

	switch brief.MutationType {
	case "risk_rule_change", "portfolio_constraint_revision":
		baselineConstraints := policy.Constraints
		candidateConstraints := baseline.ApplyConstraintCandidate(policy.Constraints, string(candidatePromptBytes))
		baseline, baselineObs := scoreConstraintWindowWithObservations(ds, baselineConstraints, window.StartDate, window.EndDate)
		candidate, candidateObs := scoreConstraintWindowWithObservations(ds, candidateConstraints, window.StartDate, window.EndDate)
		summary.BaselineScore = baseline
		summary.CandidateScore = candidate
		summary.BaselineObservations = baselineObs
		summary.CandidateObservations = candidateObs
		if baselineObs == 0 && candidateObs == 0 {
			fallbackStart, fallbackEnd, ok := fallbackWindow(ds, 30)
			if ok {
				baseline, baselineObs = scoreConstraintWindowWithObservations(ds, baselineConstraints, fallbackStart, fallbackEnd)
				candidate, candidateObs = scoreConstraintWindowWithObservations(ds, candidateConstraints, fallbackStart, fallbackEnd)
				summary.BaselineScore = baseline
				summary.CandidateScore = candidate
				summary.BaselineObservations = baselineObs
				summary.CandidateObservations = candidateObs
				summary.UsedFallbackWindow = true
			}
		}
		return summary, nil
	default:
		baseline, baselineObs := scorePromptWindowWithObservations(ds, brief.TargetSkill, baselinePrompt, policy.ExecutionPolicy, window.StartDate, window.EndDate)
		candidate, candidateObs := scorePromptWindowWithObservations(ds, brief.TargetSkill, string(candidatePromptBytes), policy.ExecutionPolicy, window.StartDate, window.EndDate)
		summary.BaselineScore = baseline
		summary.CandidateScore = candidate
		summary.BaselineObservations = baselineObs
		summary.CandidateObservations = candidateObs
		if baselineObs == 0 && candidateObs == 0 {
			fallbackStart, fallbackEnd, ok := fallbackWindow(ds, 30)
			if ok {
				baseline, baselineObs = scorePromptWindowWithObservations(ds, brief.TargetSkill, baselinePrompt, policy.ExecutionPolicy, fallbackStart, fallbackEnd)
				candidate, candidateObs = scorePromptWindowWithObservations(ds, brief.TargetSkill, string(candidatePromptBytes), policy.ExecutionPolicy, fallbackStart, fallbackEnd)
				summary.BaselineScore = baseline
				summary.CandidateScore = candidate
				summary.BaselineObservations = baselineObs
				summary.CandidateObservations = candidateObs
				summary.UsedFallbackWindow = true
			}
		}
		return summary, nil
	}
}

func scorePromptWindowWithObservations(ds *replay.Dataset, skill, prompt string, policy domain.ExecutionPolicy, startDate, endDate time.Time) (float64, int) {
	if ds == nil {
		return 0, 0
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
		return 0, 0
	}
	return total / float64(observations), observations
}

func scoreConstraintWindowWithObservations(ds *replay.Dataset, constraints domain.SimulationConstraints, startDate, endDate time.Time) (float64, int) {
	if ds == nil {
		return 0, 0
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
		baseRegime, _, activeRecs := orchestrator.ExecuteRegistryResearchDetailedWithPolicy(registry, quotes, nil, execPolicy)
		ne := narrative.NewNarrativeEngine()
		events := ne.DetectEvents(orchestrator.QuotesToNarrativeData(quotes))
		regime := orchestrator.AdjustRegimeFromNarrative(baseRegime, events)
		activeRecs = filterRecommendationsForConstraints(activeRecs, constraints)
		result := engine.Run(regime, quotes, activeRecs)
		total += scoreSimulationResult(result, nextQuotes, constraints.StartingCash)
		observations++
	}

	if observations == 0 {
		return 0, 0
	}
	return total / float64(observations), observations
}

func fallbackWindow(ds *replay.Dataset, days int) (time.Time, time.Time, bool) {
	if ds == nil || len(ds.Dates) < 2 {
		return time.Time{}, time.Time{}, false
	}
	end := ds.Dates[len(ds.Dates)-1]
	start := end.AddDate(0, 0, -days)
	if start.Before(ds.Dates[0]) {
		start = ds.Dates[0]
	}
	return start, end, true
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
