package strategy

import (
	"fmt"
	"sort"
	"time"
)

// ShadowStrategyEvaluator produces StrategyDailyObservation entries from
// real (non-synthetic, passed-guards) recommendation outcomes against a benchmark.
type ShadowStrategyEvaluator struct {
	strategies []Strategy
	meta       map[string]Strategy
}

// NewShadowStrategyEvaluator creates an evaluator backed by the strategy registry.
func NewShadowStrategyEvaluator(strategies []Strategy) *ShadowStrategyEvaluator {
	meta := make(map[string]Strategy, len(strategies))
	for _, s := range strategies {
		meta[s.ID] = s
	}
	return &ShadowStrategyEvaluator{strategies: strategies, meta: meta}
}

// Evaluate returns daily observations for each enabled strategy.
// Returns nil when tradingDate is zero or benchmark is unavailable.
// Skips synthetic outcomes, non-passed guards, empty agent/symbol, and conviction <= 0.
// Deduplicates AgentID+Symbol pairs (higher Conviction wins).
func (e *ShadowStrategyEvaluator) Evaluate(outcomes []RecommendationOutcome, tradingDate time.Time, benchmark BenchmarkObservation) []StrategyDailyObservation {
	if tradingDate.IsZero() || !benchmark.Available {
		return nil
	}

	// Build enabled strategy set.
	enabled := make(map[string]struct{}, len(e.strategies))
	for _, s := range e.strategies {
		if s.Enabled {
			enabled[s.ID] = struct{}{}
		}
	}

	// Collect per (AgentID, Symbol) best-conviction outcome.
	type key struct{ agentID, symbol string }
	best := make(map[key]*RecommendationOutcome, len(outcomes))
	for i := range outcomes {
		o := &outcomes[i]
		if o.IsSynthetic || !o.PassedGuards || o.AgentID == "" || o.Symbol == "" || o.Conviction <= 0 {
			continue
		}
		if _, ok := enabled[o.AgentID]; !ok {
			continue
		}
		k := key{o.AgentID, o.Symbol}
		if prev, exists := best[k]; !exists || o.Conviction > prev.Conviction {
			best[k] = o
		}
	}

	// Compute per-strategy conviction-weighted mean forward return.
	stratReturns := make(map[string]struct {
		sum, conv float64
		count     int
	})
	for _, o := range best {
		entry := stratReturns[o.AgentID]
		entry.sum += o.ForwardReturn * float64(o.Conviction)
		entry.conv += float64(o.Conviction)
		entry.count++
		stratReturns[o.AgentID] = entry
	}

	dateStr := tradingDate.Format("2006-01-02")
	var obs []StrategyDailyObservation
	for _, s := range e.strategies {
		if !s.Enabled {
			continue
		}
		entry := stratReturns[s.ID]
		dailyReturn := 0.0
		if entry.conv > 0 {
			dailyReturn = entry.sum / entry.conv
		}
		obs = append(obs, StrategyDailyObservation{
			TradingDate:     dateStr,
			StrategyID:      s.ID,
			EvaluationMode:  EvaluationModeShadow,
			DailyReturn:     dailyReturn,
			BenchmarkReturn: benchmark.Return,
			Outperformance:  dailyReturn - benchmark.Return,
			OutcomeCount:    entry.count,
		})
	}
	sort.Slice(obs, func(i, j int) bool { return obs[i].StrategyID < obs[j].StrategyID })
	return obs
}

// ComputeWarmingUpState determines whether the shadow ranking has enough
// history and returns the current state.
func ComputeWarmingUpState(dates []string, minDays int, asOf string) WarmingUpState {
	unique := deduplicateSorted(dates)
	sampleDays := len(unique)
	lastDate := ""
	if sampleDays > 0 {
		lastDate = unique[sampleDays-1]
	}
	state := WarmingUpState{
		Status:          "warming_up",
		LastTradingDate: lastDate,
		SampleDays:      sampleDays,
		MinHistoryDays:  minDays,
	}
	if sampleDays == 0 {
		state.ReasonCode = "no_history"
		return state
	}
	if sampleDays < minDays {
		state.ReasonCode = "below_floor"
		state.DaysUntilEligible = minDays - sampleDays
		return state
	}
	state.Status = "eligible"
	state.ReasonCode = ""
	return state
}

// deduplicateSorted returns sorted unique date strings from a pre-sorted slice.
func deduplicateSorted(dates []string) []string {
	if len(dates) == 0 {
		return nil
	}
	out := make([]string, 0, len(dates))
	prev := ""
	for _, d := range dates {
		if d != prev {
			out = append(out, d)
			prev = d
		}
	}
	return out
}

// Ensure context import is used.
var _ = fmt.Sprintf

// RecommendationOutcome copies the domain type locally to avoid import cycle.
// This is a forward-compatible minimal subset of domain.RecommendationOutcome.
type RecommendationOutcome struct {
	AgentID       string
	Skill         string
	Symbol        string
	Conviction    int
	ForwardReturn float64
	IsSynthetic   bool
	PassedGuards  bool
}
