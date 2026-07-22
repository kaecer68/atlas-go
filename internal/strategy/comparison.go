package strategy

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
)

type Trade struct {
	StrategyID string
	Date       time.Time
	Return     float64
	Symbol     string
}

type ComparisonEngine struct {
	mu          sync.RWMutex
	window      int
	history     []*ComparisonResult
	trades      map[string][]*Trade
	shadowDays  []ComparisonDay
	shadowStore ComparisonStore
}

func NewComparisonEngine(window int, store ComparisonStore) *ComparisonEngine {
	if window <= 0 {
		window = config.GetParametersConfig().Strategy.ScoreLookbackDays.Value
	}
	return &ComparisonEngine{
		window:      window,
		trades:      make(map[string][]*Trade),
		shadowStore: store,
	}
}

// RecordShadowDay persists a ComparisonDay entry to the shadow store.
// Benchmark must be Available or the entry is silently skipped.
func (e *ComparisonEngine) RecordShadowDay(day ComparisonDay) error {
	if !day.Benchmark.Available {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	replaced := false
	for i, d := range e.shadowDays {
		if d.TradingDate == day.TradingDate {
			e.shadowDays[i] = day
			replaced = true
			break
		}
	}
	if !replaced {
		e.shadowDays = append(e.shadowDays, day)
	}
	sort.Slice(e.shadowDays, func(i, j int) bool {
		return e.shadowDays[i].TradingDate < e.shadowDays[j].TradingDate
	})

	if e.shadowStore != nil {
		return e.shadowStore.Upsert(context.Background(), day)
	}
	return nil
}

func (e *ComparisonEngine) Record(trades []*Trade, benchmarkReturn float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	for _, t := range trades {
		e.trades[t.StrategyID] = append(e.trades[t.StrategyID], t)
	}

	e.pruneOldTrades(now)

	result := e.calculateComparison(now, benchmarkReturn)
	e.history = append(e.history, result)
}

func (e *ComparisonEngine) pruneOldTrades(now time.Time) {
	cutoff := now.AddDate(0, 0, -e.window*2)
	for id, trades := range e.trades {
		filtered := make([]*Trade, 0)
		for _, t := range trades {
			if t.Date.After(cutoff) {
				filtered = append(filtered, t)
			}
		}
		if len(filtered) == 0 {
			delete(e.trades, id)
		} else {
			e.trades[id] = filtered
		}
	}
}

func (e *ComparisonEngine) calculateComparison(date time.Time, benchmarkReturn float64) *ComparisonResult {
	result := &ComparisonResult{
		Date:        date.Format("2006-01-02"),
		Comparisons: make([]*StrategyComparison, 0),
	}

	strategyIDs := make(map[string]bool)
	for id := range e.trades {
		strategyIDs[id] = true
	}

	for id := range strategyIDs {
		trades := e.trades[id]
		if len(trades) == 0 {
			continue
		}

		comp := &StrategyComparison{
			Date:       date.Format("2006-01-02"),
			StrategyID: id,
		}

		var totalReturn float64
		var winCount int
		for _, t := range trades {
			totalReturn += t.Return
			if t.Return > 0 {
				winCount++
			}
		}

		comp.DailyReturn = totalReturn
		if len(trades) > 0 {
			comp.WinRate = float64(winCount) / float64(len(trades))
		}
		comp.Outperformance = totalReturn - benchmarkReturn

		result.Comparisons = append(result.Comparisons, comp)
	}

	if len(result.Comparisons) > 0 {
		sort.Slice(result.Comparisons, func(i, j int) bool {
			return result.Comparisons[i].DailyReturn > result.Comparisons[j].DailyReturn
		})
		result.BestByReturn = result.Comparisons[0].StrategyID
	}

	e.calculateSharpeRatios(result)
	e.calculateMaxDrawdowns(result)

	return result
}

func (e *ComparisonEngine) calculateSharpeRatios(result *ComparisonResult) {
	if len(result.Comparisons) == 0 {
		return
	}
	cfg := shared.SharpeConfig{
		Frequency:  shared.FrequencyPerOutcome,
		MinSamples: 2,
	}
	for _, comp := range result.Comparisons {
		trades := e.trades[comp.StrategyID]
		if len(trades) < 2 {
			comp.SharpeRatio = 0.5
			continue
		}
		returns := make([]float64, len(trades))
		for i, t := range trades {
			returns[i] = t.Return
		}
		comp.SharpeRatio = shared.ComputeSharpe(returns, cfg)
		if comp.SharpeRatio == 0 && len(returns) >= 2 {
			// Canonical sharp returns 0 for near-constant series; preserve
			// the original 0.5 fallback for the recommender consumer.
			comp.SharpeRatio = 0.5
		}
	}
}

func (e *ComparisonEngine) calculateMaxDrawdowns(result *ComparisonResult) {
	for _, comp := range result.Comparisons {
		trades := e.trades[comp.StrategyID]
		if len(trades) == 0 {
			continue
		}
		var peak float64
		var maxDD float64
		var cumulative float64
		for _, t := range trades {
			cumulative += t.Return
			if cumulative > peak {
				peak = cumulative
			}
			dd := peak - cumulative
			if dd > maxDD {
				maxDD = dd
			}
		}
		comp.MaxDrawdown = maxDD
	}
}

func (e *ComparisonEngine) GetResult(date time.Time) (*ComparisonResult, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	dateStr := date.Format("2006-01-02")
	for _, h := range e.history {
		if h.Date == dateStr {
			return h, true
		}
	}
	return nil, false
}

func (e *ComparisonEngine) BestStrategy(by string) (string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if len(e.history) == 0 {
		return "", nil
	}
	last := e.history[len(e.history)-1]
	switch by {
	case "return":
		return last.BestByReturn, nil
	case "sharpe":
		return last.BestBySharpe, nil
	case "drawdown":
		return last.BestByDrawdown, nil
	default:
		return "", fmt.Errorf("unknown comparison criteria: %s", by)
	}
}

func (e *ComparisonEngine) GetScore(strategyID string, days int) (float64, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if len(e.history) < days {
		return 0.5, nil
	}
	var totalScore float64
	count := 0
	for i := len(e.history) - days; i < len(e.history); i++ {
		h := e.history[i]
		for _, comp := range h.Comparisons {
			if comp.StrategyID == strategyID {
				score := comp.SharpeRatio*0.4 + comp.DailyReturn*30*0.3 + comp.WinRate*0.3
				totalScore += score
				count++
				break
			}
		}
	}
	if count == 0 {
		return 0.5, nil
	}
	return totalScore / float64(count), nil
}
