package strategy

import (
	"sort"
	"time"
)

// RankingSnapshot builds the full ranking for a given date from persisted
// shadow comparison days (not the in-memory ComparisonResult history).
func (e *ComparisonEngine) RankingSnapshot(asOf time.Time) RankingSnapshot {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var dates []string
	dayMap := make(map[string]ComparisonDay, len(e.shadowDays))
	for _, day := range e.shadowDays {
		dates = append(dates, day.TradingDate)
		dayMap[day.TradingDate] = day
	}
	sort.Strings(dates)
	warming := ComputeWarmingUpState(dates, 20, asOf.Format("2006-01-02"))

	// Collect all observed strategies across all comparison days.
	type stratScore struct {
		id           string
		totalReturn  float64
		count        int
		totalOutperf float64
	}
	stratScores := make(map[string]*stratScore)
	for _, day := range e.shadowDays {
		for _, obs := range day.Observations {
			if obs.EvaluationMode != EvaluationModeShadow {
				continue
			}
			s := stratScores[obs.StrategyID]
			if s == nil {
				s = &stratScore{id: obs.StrategyID}
				stratScores[obs.StrategyID] = s
			}
			s.totalReturn += obs.DailyReturn
			s.count++
			s.totalOutperf += obs.Outperformance
		}
	}

	// Rank by score (70% return + 30% outperformance average).
	var ranked []RankedStrategy
	for _, s := range stratScores {
		if s.count == 0 {
			continue
		}
		avg := s.totalReturn / float64(s.count)
		avgOutperf := s.totalOutperf / float64(s.count)
		score := avg*0.7 + avgOutperf*0.3
		ranked = append(ranked, RankedStrategy{
			StrategyID:     s.id,
			EvaluationMode: EvaluationModeShadow,
			SampleDays:     s.count,
			Score:          score,
		})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Score != ranked[j].Score {
			return ranked[i].Score > ranked[j].Score
		}
		return ranked[i].StrategyID < ranked[j].StrategyID
	})
	for i := range ranked {
		ranked[i].Rank = i + 1
	}

	// Latest deployed mix and benchmark from the most recent day.
	var deployedMix map[string]float64
	benchmark := BenchmarkObservation{ReasonCode: "warming_up"}
	if len(dates) > 0 {
		lastDay := dayMap[dates[len(dates)-1]]
		deployedMix = lastDay.DeployedMix
		benchmark = lastDay.Benchmark
	}
	if deployedMix == nil {
		deployedMix = map[string]float64{}
	}

	return RankingSnapshot{
		AsOfTradingDate: asOf.Format("2006-01-02"),
		WarmingUp:       warming,
		Ranked:          ranked,
		DeployedMix:     deployedMix,
		Benchmark:       benchmark,
	}
}

// RankedIDs returns a simplified ranked list of strategy IDs from the shadow
// comparison engine (F06). Returns nil when warming up.
func (e *ComparisonEngine) RankedIDs() ([]string, error) {
	snap := e.RankingSnapshot(time.Now())
	if snap.WarmingUp.Status != "eligible" {
		return nil, nil
	}
	ids := make([]string, len(snap.Ranked))
	for i, r := range snap.Ranked {
		ids[i] = r.StrategyID
	}
	return ids, nil
}
