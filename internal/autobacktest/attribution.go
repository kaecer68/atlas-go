// AttributionRecord is the minimal shape Runner needs to record per-
// strategy attribution. It is decoupled from apistrategies.Record to
// avoid an import cycle: monitoring/api/strategies already imports
// monitoring packages that eventually touch autobacktest through
// SystemService wiring, so importing back would cycle.
//
// The FeedbackStore satisfies this interface natively because
// apistrategies.Record and AttributionRecord have identical fields
// (matched by adapter in main.go). Tests use an in-memory fake.
package autobacktest

import "github.com/kaecer68/atlas-go/internal/strategy_techniques"

type AttributionRecord struct {
	StrategyID string
	TotalTests int
	TotalHits  int
	HitRate    float64
	Status     string
}

// AttributionWriter persists one record per active strategy. The
// production implementation is *apistrategies.FeedbackStore, adapted
// at the call site (main.go) since Go lacks structural typing across
// packages.
type AttributionWriter interface {
	Write(r AttributionRecord) error
}

// ConditionEvaluator evaluates strategy conditions against historical
// data and returns per-strategy performance metrics. Implementations
// live in strategy_techniques to avoid importing marketdata here.
type ConditionEvaluator interface {
	Evaluate(frame strategy_techniques.StrategyFrame) *strategy_techniques.EvalResult
}
