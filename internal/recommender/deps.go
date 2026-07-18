package recommender

import (
	"context"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/eventdriven"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// =====================================================================
// Service consumer interfaces (definitions owned by recommender, the
// consumer side). Implementations live in adapters.go and wrap real
// producers in monitoring/capitalflow/eventdriven/strategy packages.
// =====================================================================

// NarrativeProvider 供 recommender 查詢當前 regime + Taiwan stress index。
// 對應 producer: monitoring/service.NarrativeService。
type NarrativeProvider interface {
	// GetCurrentStressIndex wraps narrative.NarrativeService.GetCurrentStressIndex().
	// Returns narrative.TaiwanStressIndex — the project canonical type for
	// regime + score; recommender reads Regime + Value fields.
	GetCurrentStressIndex() narrative.TaiwanStressIndex
	// BuildMarketNarrativeData returns the snapshot of US/DXY/VIX/macro inputs.
	// May return (zero, err) if macro provider is not wired.
	BuildMarketNarrativeData(ctx context.Context) (narrative.MarketNarrativeData, error)
}

// CapitalFlowProvider 供 recommender 查詢當日七維錢潮雷達（3+2+2 分層）summary。
// 對應 producer: capitalflow.Service (added in commit 661f2dc7; Summary added
// in commit b081f2f5 / PR #1002; 3+2+2 assessment layers added in #E07 commit
// ccd4e721; UI/MCP/runtime alignment in #E08).
//
// Automating callers MUST gate assessment-derived decisions on
// CapitalFlowAssessment.EligibleForAutomation(); calibrating/degraded
// assessments are explanation-only and must not affect automated actions.
type CapitalFlowProvider interface {
	// LatestDaily returns the full DailyReport (forces + resonance + quality).
	// Recommender reads the Summary field for response.market.capital_flow.
	LatestDaily(ctx context.Context) (capitalflow.DailyReport, error)
	// Summary returns the condensed SummaryReport (quality score + dominant
	// force + short summary). Opt-in for callers that need the summary view
	// without forcing a full DailyReport fetch; if the producer cannot
	// synthesize a summary independently, it may derive one from LatestDaily
	// (see capitalflow.Service.Summary for the canonical pattern).
	//
	// Nil-safe: nil adapter implementations MUST return (zero, nil) rather
	// than erroring — see adapters.go and capitalFlowFromCapitalFlow for the
	// graceful-degradation contract.
	Summary(ctx context.Context) (capitalflow.SummaryReport, error)
	// LatestAssessment returns the structured E07 assessment for direct
	// consumers. Handler request paths derive Assessment from LatestDaily so
	// they do not trigger an additional macro snapshot fetch.
	LatestAssessment(ctx context.Context) (capitalflow.CapitalFlowAssessment, error)
}

// EventPredictor 供 recommender 查詢當日事件 + 短期預測。
// 對應 producer: eventdriven.Predictor。
type EventPredictor interface {
	PredictToday() (eventdriven.FlowPrediction, error)
	NextNDays(n int) ([]eventdriven.FlowPrediction, error)
}

// ComparisonEngine 供 recommender 查詢策略分數。
// 對應 producer: strategy.ComparisonEngine。
// returns float64 only; EntrySignal/StopLoss 由 RISK layer 推導 (見 risk_signal.go)。
type ComparisonEngine interface {
	GetScore(strategyID string) (float64, error)
	// RankedStrategies returns strategy IDs ordered by score, highest first (F06).
	// Returns nil when warming up (insufficient shadow days).
	RankedStrategies() ([]string, error)
}

// =====================================================================
// Functional types
// =====================================================================

// RegimeChangeListener fires when HandleRecommendations observes a regime
// transition (e.g. RISK_ON → RISK_OFF); caller decides what to trigger.
type RegimeChangeListener func(oldRegime, newRegime string)
