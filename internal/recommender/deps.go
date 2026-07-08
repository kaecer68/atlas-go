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

// CapitalFlowProvider 供 recommender 查詢當日七大資金勢力 summary。
// 對應 producer: capitalflow.Service (added in commit 661f2dc7)。
type CapitalFlowProvider interface {
	// LatestDaily returns the full DailyReport (forces + resonance + quality).
	// Recommender reads the Summary field for response.market.capital_flow.
	LatestDaily(ctx context.Context) (capitalflow.DailyReport, error)
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
}

// =====================================================================
// Functional types
// =====================================================================

// RegimeChangeListener fires when HandleRecommendations observes a regime
// transition (e.g. RISK_ON → RISK_OFF); caller decides what to trigger.
type RegimeChangeListener func(oldRegime, newRegime string)
