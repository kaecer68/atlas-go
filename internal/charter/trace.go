package charter

import (
	"sort"
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// DailyRecommendationTrace captures one day's recommendation pipeline output
// for A/B attribution: the raw (post-collection) and final (post-control)
// recommendation counts, broken down by agent, plus the detected period.
// Comparing these counts across a baseline arm and a feature arm attributes
// each charter switch's effect (e.g. how many growth recs the period gate
// suppressed).
type DailyRecommendationTrace struct {
	Date         string               `json:"date"`
	Regime       domain.Regime        `json:"regime"`
	Period       *domain.MarketPeriod `json:"period,omitempty"`
	RawCount     int                  `json:"raw_count"`
	FinalCount   int                  `json:"final_count"`
	RawByAgent   map[string]int       `json:"raw_by_agent"`
	FinalByAgent map[string]int       `json:"final_by_agent"`
}

// RecommendationTrace is a thread-safe accumulator of per-day recommendation
// pipeline traces for one backtest arm.
type RecommendationTrace struct {
	mu   sync.Mutex
	Days []DailyRecommendationTrace `json:"days"`
}

// Append records one day's trace.
func (t *RecommendationTrace) Append(d DailyRecommendationTrace) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Days = append(t.Days, d)
}

// Totals aggregates the trace: total raw/final counts and per-agent raw/final
// counts across all days.
func (t *RecommendationTrace) Totals() (rawTotal, finalTotal int, rawByAgent, finalByAgent map[string]int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	rawByAgent = make(map[string]int)
	finalByAgent = make(map[string]int)
	for _, d := range t.Days {
		rawTotal += d.RawCount
		finalTotal += d.FinalCount
		for a, c := range d.RawByAgent {
			rawByAgent[a] += c
		}
		for a, c := range d.FinalByAgent {
			finalByAgent[a] += c
		}
	}
	return rawTotal, finalTotal, rawByAgent, finalByAgent
}

// PeriodDistribution counts detected market periods across the trace days.
func (t *RecommendationTrace) PeriodDistribution() map[string]int {
	t.mu.Lock()
	defer t.mu.Unlock()
	dist := make(map[string]int)
	for _, d := range t.Days {
		if d.Period != nil {
			dist[string(*d.Period)]++
		} else {
			dist["none"]++
		}
	}
	return dist
}

// AgentHitRate is a per-agent hit-rate row for the report.
type AgentHitRate struct {
	AgentID  string  `json:"agent_id"`
	Outcomes int     `json:"outcomes"`
	Hits     int     `json:"hits"`
	HitRate  float64 `json:"hit_rate"`
}

// CountByAgent builds an agent → count map from recommendations.
func CountByAgent(recs []domain.Recommendation) map[string]int {
	m := make(map[string]int, len(recs))
	for _, r := range recs {
		m[r.Agent]++
	}
	return m
}

// SortedKeys returns a sorted slice of a string→int map's keys (stable
// report ordering).
func SortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
