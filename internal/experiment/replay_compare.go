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
	BaselineScore          float64
	CandidateScore         float64
	BaselineObservations   int
	CandidateObservations  int
	UsedFallbackWindow     bool
	BaselineReturns        []float64
	CandidateReturns       []float64
	BaselineFallbackStats  FallbackStats
	CandidateFallbackStats FallbackStats
	StartingCash           float64
	BaselineMonetaryNTD    float64
	CandidateMonetaryNTD   float64
}

// FallbackStats tracks factor quality based on IsFallback ratio.
type FallbackStats struct {
	FallbackCount int // number of factors marked IsFallback=true
	TotalCount    int // total number of factors evaluated
}

// Ratio returns the fraction of fallback factors (0.0 to 1.0).
// Returns 0.0 if TotalCount is 0.
func (fs FallbackStats) Ratio() float64 {
	if fs.TotalCount == 0 {
		return 0.0
	}
	return float64(fs.FallbackCount) / float64(fs.TotalCount)
}

// IsHighFallback returns true if more than maxRatio of factors are fallbacks.
func (fs FallbackStats) IsHighFallback(maxRatio float64) bool {
	return fs.Ratio() > maxRatio
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
		baseline, baselineObs, baselineReturns, baselineFallback := scoreConstraintWindowWithObservations(ds, baselineConstraints, window.StartDate, window.EndDate)
		candidate, candidateObs, candidateReturns, candidateFallback := scoreConstraintWindowWithObservations(ds, candidateConstraints, window.StartDate, window.EndDate)
		summary.BaselineScore = baseline
		summary.CandidateScore = candidate
		summary.BaselineObservations = baselineObs
		summary.CandidateObservations = candidateObs
		summary.BaselineReturns = baselineReturns
		summary.CandidateReturns = candidateReturns
		summary.BaselineFallbackStats = baselineFallback
		summary.CandidateFallbackStats = candidateFallback
		if baselineObs == 0 && candidateObs == 0 {
			fallbackStart, fallbackEnd, ok := fallbackWindow(ds, 1)
			if ok {
				baseline, baselineObs, baselineReturns, baselineFallback = scoreConstraintWindowWithObservations(ds, baselineConstraints, fallbackStart, fallbackEnd)
				candidate, candidateObs, candidateReturns, candidateFallback = scoreConstraintWindowWithObservations(ds, candidateConstraints, fallbackStart, fallbackEnd)
				summary.BaselineScore = baseline
				summary.CandidateScore = candidate
				summary.BaselineObservations = baselineObs
				summary.CandidateObservations = candidateObs
				summary.BaselineReturns = baselineReturns
				summary.CandidateReturns = candidateReturns
				summary.BaselineFallbackStats = baselineFallback
				summary.CandidateFallbackStats = candidateFallback
				summary.UsedFallbackWindow = true
			}
		}
		startingCash := baselineConstraints.StartingCash
		summary.StartingCash = startingCash
		summary.BaselineMonetaryNTD = baseline * startingCash
		summary.CandidateMonetaryNTD = candidate * startingCash
		return summary, nil
	default:
		baseline, baselineObs, baselineReturns, baselineFallback := scorePromptWindowWithObservations(ds, brief.TargetSkill, baselinePrompt, policy.ExecutionPolicy, window.StartDate, window.EndDate)
		candidate, candidateObs, candidateReturns, candidateFallback := scorePromptWindowWithObservations(ds, brief.TargetSkill, string(candidatePromptBytes), policy.ExecutionPolicy, window.StartDate, window.EndDate)
		summary.BaselineScore = baseline
		summary.CandidateScore = candidate
		summary.BaselineObservations = baselineObs
		summary.CandidateObservations = candidateObs
		summary.BaselineReturns = baselineReturns
		summary.CandidateReturns = candidateReturns
		summary.BaselineFallbackStats = baselineFallback
		summary.CandidateFallbackStats = candidateFallback
		if baselineObs == 0 && candidateObs == 0 {
			fallbackStart, fallbackEnd, ok := fallbackWindow(ds, 1)
			if ok {
				baseline, baselineObs, baselineReturns, baselineFallback = scorePromptWindowWithObservations(ds, brief.TargetSkill, baselinePrompt, policy.ExecutionPolicy, fallbackStart, fallbackEnd)
				candidate, candidateObs, candidateReturns, candidateFallback = scorePromptWindowWithObservations(ds, brief.TargetSkill, string(candidatePromptBytes), policy.ExecutionPolicy, fallbackStart, fallbackEnd)
				summary.BaselineScore = baseline
				summary.CandidateScore = candidate
				summary.BaselineObservations = baselineObs
				summary.CandidateObservations = candidateObs
				summary.BaselineReturns = baselineReturns
				summary.CandidateReturns = candidateReturns
				summary.BaselineFallbackStats = baselineFallback
				summary.CandidateFallbackStats = candidateFallback
				summary.UsedFallbackWindow = true
			}
		}
		startingCash := policy.Constraints.StartingCash
		summary.StartingCash = startingCash
		summary.BaselineMonetaryNTD = baseline * startingCash
		summary.CandidateMonetaryNTD = candidate * startingCash
		return summary, nil
	}
}

func scorePromptWindowWithObservations(ds *replay.Dataset, skill, prompt string, policy domain.ExecutionPolicy, startDate, endDate time.Time) (float64, int, []float64, FallbackStats) {
	if ds == nil {
		return 0, 0, nil, FallbackStats{}
	}

	registry := orchestrator.SeedRegistry()
	sessionReturns := make([]float64, 0)
	var totalFallback, totalFactors int
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
		sessionTotal := 0.0
		sessionObs := 0
		for _, rec := range rawRecs {
			if rec.Skill != skill {
				continue
			}
			if rec.FactorScores.Breakdown != nil {
				bd := rec.FactorScores.Breakdown
				if bd.Momentum.IsFallback {
					totalFallback++
				}
				totalFactors++
				if bd.Value.IsFallback {
					totalFallback++
				}
				totalFactors++
				if bd.Quality.IsFallback {
					totalFallback++
				}
				totalFactors++
				if bd.Agent.IsFallback {
					totalFallback++
				}
				totalFactors++
			}
			forwardReturn, ok := ds.ForwardReturn(rec.Symbol, date, 1)
			if !ok {
				continue
			}
			sessionTotal += forwardReturn * (float64(rec.Conviction) / 100.0)
			sessionObs++
		}
		if sessionObs > 0 {
			sessionReturns = append(sessionReturns, sessionTotal/float64(sessionObs))
		}
	}

	if len(sessionReturns) == 0 {
		return 0, 0, nil, FallbackStats{}
	}
	total := 0.0
	for _, r := range sessionReturns {
		total += r
	}
	return total / float64(len(sessionReturns)), len(sessionReturns), sessionReturns, FallbackStats{FallbackCount: totalFallback, TotalCount: totalFactors}
}

func scoreConstraintWindowWithObservations(ds *replay.Dataset, constraints domain.SimulationConstraints, startDate, endDate time.Time) (float64, int, []float64, FallbackStats) {
	if ds == nil {
		return 0, 0, nil, FallbackStats{}
	}

	registry := orchestrator.SeedRegistry()
	symbols := orchestrator.RegistrySymbols(registry)
	engine := sim.NewEngine(constraints)

	sessionReturns := make([]float64, 0)
	var totalFallback, totalFactors int
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

		// Track fallback stats from FactorScores.Breakdown
		for _, rec := range activeRecs {
			if rec.FactorScores.Breakdown != nil {
				bd := rec.FactorScores.Breakdown
				if bd.Momentum.IsFallback {
					totalFallback++
				}
				totalFactors++
				if bd.Value.IsFallback {
					totalFallback++
				}
				totalFactors++
				if bd.Quality.IsFallback {
					totalFallback++
				}
				totalFactors++
				if bd.Agent.IsFallback {
					totalFallback++
				}
				totalFactors++
			}
		}

		result := engine.Run(regime, quotes, activeRecs)
		sessionReturns = append(sessionReturns, scoreSimulationResult(result, nextQuotes, constraints.StartingCash))
	}

	if len(sessionReturns) == 0 {
		return 0, 0, nil, FallbackStats{}
	}
	total := 0.0
	for _, r := range sessionReturns {
		total += r
	}
	return total / float64(len(sessionReturns)), len(sessionReturns), sessionReturns, FallbackStats{FallbackCount: totalFallback, TotalCount: totalFactors}
}

func fallbackWindow(ds *replay.Dataset, minDates int) (time.Time, time.Time, bool) {
	if ds == nil || len(ds.Dates) < 1 {
		return time.Time{}, time.Time{}, false
	}
	end := ds.Dates[len(ds.Dates)-1]
	for _, days := range []int{30, 60, 90, 180, 365} {
		start := end.AddDate(0, 0, -days)
		if start.Before(ds.Dates[0]) {
			start = ds.Dates[0]
		}
		count := 0
		for _, d := range ds.Dates {
			if !d.Before(start) && !d.After(end) {
				count++
			}
		}
		if count >= minDates {
			return start, end, true
		}
	}
	// Fall back to the full available dataset range.
	return ds.Dates[0], end, true
}

func scoreSimulationResult(result domain.SimulationResult, nextQuotes []domain.Quote, startingCash float64) float64 {
	if startingCash == 0 {
		return 0
	}
	if result.AfterTaxPnL != 0 || result.TotalTaxPaid != 0 {
		return result.AfterTaxPnL / startingCash
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
