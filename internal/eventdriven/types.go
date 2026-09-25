package eventdriven

import "time"

// MinHitSamples is the minimum number of T+1-reconciled predictions before
// a historical hit rate is shown as calibrated. Aligned with product
// positioning §6 "校準中" semantics — below this the frontend shows a
// calibrating badge instead of a misleading percentage.
const MinHitSamples = 30

// PredictionDistribution is the probability mass across the three possible
// capital-flow directions for a single day.
type PredictionDistribution struct {
	Inflow  float64 `json:"inflow"`
	Outflow float64 `json:"outflow"`
	Neutral float64 `json:"neutral"`
}

// FlowPrediction is a single day's predicted capital flow direction.
type FlowPrediction struct {
	Date            time.Time              `json:"date"`
	Direction       string                 `json:"direction"`        // "inflow", "outflow", "neutral"
	Confidence      float64                `json:"confidence"`       // 0-1
	Distribution    PredictionDistribution `json:"distribution"`     // probability mass across directions
	DrivingEvents   []string               `json:"driving_events"`   // event names driving this prediction
	PredictedForces []string               `json:"predicted_forces"` // which forces likely to move
}

// EventCalendarItem is a view of an upcoming event, sourced from industry.CalendarEvent.
type EventCalendarItem struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	NameEN              string    `json:"name_en,omitempty"`
	EventType           string    `json:"event_type"`
	Description         string    `json:"description,omitempty"`
	Direction           string    `json:"direction"`
	StartDate           time.Time `json:"start_date"`
	EndDate             time.Time `json:"end_date"`
	PeakDate            time.Time `json:"peak_date,omitzero"`
	DecayDays           int       `json:"decay_days"`
	AffectedIndustries  []string  `json:"affected_industries,omitempty"`
	ExpectedFlowImpact  string    `json:"expected_flow_impact"`
	Confidence          float64   `json:"confidence"`
	SentimentAdjustment float64   `json:"sentiment_adjustment"`
	DataSource          string    `json:"data_source,omitempty"`
	EvidenceQuality     string    `json:"evidence_quality,omitempty"`
	Backfilled          bool      `json:"backfilled"`
	CrossSourceStatus   string    `json:"cross_source_status,omitempty"`
	GeneratedAt         time.Time `json:"generated_at,omitzero"`
}

// ETFEstimate represents the predicted capital flow from an ETF rebalance event.
type ETFEstimate struct {
	ETFName     string  `json:"etf_name"`
	StockSymbol string  `json:"stock_symbol"`
	StockName   string  `json:"stock_name"`
	Direction   string  `json:"direction"`  // "add" or "remove"
	EstWeight   float64 `json:"est_weight"` // 0-1
	ETFAUM      float64 `json:"etf_aum"`    // in NTD billions
	EstFlow     float64 `json:"est_flow"`   // = etf_aum × est_weight (NTD millions)
}

// RevenueSurprise is a revenue-surprise event analysis.
type RevenueSurprise struct {
	StockSymbol string  `json:"stock_symbol"`
	StockName   string  `json:"stock_name"`
	Expected    float64 `json:"expected"`     // expected revenue (NTD millions)
	Actual      float64 `json:"actual"`       // actual revenue (NTD millions)
	SurprisePct float64 `json:"surprise_pct"` // (actual - expected) / expected
	FlowImpact  string  `json:"flow_impact"`  // "bullish" if >10%, "bearish" if <-10%, else "neutral"
}

// SectorPrediction is a per-sector direction for a single day.
type SectorPrediction struct {
	SectorID     string                 `json:"sector_id"`
	SectorName   string                 `json:"sector_name"`
	Direction    string                 `json:"direction"`  // "inflow" | "outflow" | "neutral"
	Confidence   float64                `json:"confidence"` // 0..1
	Distribution PredictionDistribution `json:"distribution"`
	Drivers      []string               `json:"drivers"` // top 2 contributing factors
}

// SectorDayPrediction groups all L1 sector predictions for a single forecast date.
//
// Persistence: NOT persisted. The ledger landing type
// (ledger.EventFlowPredictionRecord) has no sector column and the handler's
// persistTodayPrediction() writes only the day-1 whole-market flow, so sector
// rows are recomputed per request and cannot be reconciled T+1 (#1944 Batch 3,
// item I6). SectorPredictionPersisted / SectorPredictionPersistenceReason below
// are the machine-readable form of that fact.
type SectorDayPrediction struct {
	Date    string             `json:"date"`
	Sectors []SectorPrediction `json:"sectors"`
}

// Per-sector prediction wiring facts (#1944 Batch 3, items I4/I5/I6).
//
// These constants are the machine-readable record of what the event-driven
// sector predictor does NOT do in production. They are asserted by
// sector_prediction_status_test.go; flipping one requires updating that test
// and the corresponding entry in docs/reference/inert-registry.md.
const (
	// SectorPredictionPersisted reports whether SectorDayPrediction rows are
	// written anywhere durable. false: the ledger schema has no sector column
	// (I6), so predictions vanish on the next request.
	SectorPredictionPersisted = false
	// SectorPredictionPersistenceReason explains the false above.
	SectorPredictionPersistenceReason = "ledger_event_flow_prediction_record_has_no_sector_column"

	// SectorCycleProviderWired reports whether production injects a cycle-score
	// provider (industry.CycleTracker.GetContinuousPhaseScore) into the
	// SectorPredictor. false: the two CycleTracker instances that exist at
	// runtime (orchestrator composition root and monitoring dashboard) are not
	// synchronized, and the dashboard one is seeded from
	// industry.default_metrics config rather than measured data (I13). Feeding
	// either into the predictor would present a config seed as a measured cycle
	// position. To flip this: (1) make one CycleTracker instance shared and fed
	// by a real UpdatePosition producer (I13), (2) pass it through
	// RegisterRoutesWithDetectors → Handler.SetSectorCycleProvider, (3) update
	// TestProductionSectorPredictionStatusLeavesCycleUnwired.
	SectorCycleProviderWired = false
)

// Reasons reported by SectorPredictionStatus.Reason when sector rows are absent.
const (
	// SectorPredictionReasonFlagDisabled — SECTOR_PREDICTION_ENABLED is false
	// (its shipped default), so cmd/atlas never wires the macro provider and
	// the predictor is never built. I5.
	SectorPredictionReasonFlagDisabled = "sector_prediction_disabled_by_flag"
	// SectorPredictionReasonMacroUnavailable — the flag is on but the macro
	// snapshot fetch failed, so no predictor could be built for this request.
	SectorPredictionReasonMacroUnavailable = "macro_snapshot_unavailable"
	// SectorPredictionReasonNotBuilt — no predictor is attached to the handler.
	SectorPredictionReasonNotBuilt = "sector_predictor_not_attached"
)

// SectorPredictionStatus makes the sector-prediction wiring state visible on
// the prediction report. Without it a disabled flag surfaced as a silent
// `sector_predictions: []` — indistinguishable from "computed, no signal"
// (#1944 Batch 3, item I5).
type SectorPredictionStatus struct {
	// Enabled is true when the handler is allowed to build a predictor, i.e.
	// the SECTOR_PREDICTION_ENABLED gate in cmd/atlas wired a macro provider.
	Enabled bool `json:"enabled"`
	// Applied is true only when sector rows were actually produced for this
	// report. Never hard-coded.
	Applied bool `json:"applied"`
	// Days / SectorRows are the produced shape (0 when !Applied).
	Days       int `json:"days"`
	SectorRows int `json:"sector_rows"`
	// StrategicPriorApplied is derived from the predictor's live state: true
	// only when a sectorallocation.StrategicSectorPrior is attached, which is
	// what makes the `overall_baseline` driver able to contribute (#1944 item I4).
	StrategicPriorApplied bool `json:"strategic_prior_applied"`
	// CycleProviderWired is derived from the predictor's live state. In
	// production it is false (see SectorCycleProviderWired) and the
	// `cycle_position` driver can therefore never contribute.
	CycleProviderWired bool `json:"cycle_provider_wired"`
	// Persisted mirrors SectorPredictionPersisted for this payload.
	Persisted         bool   `json:"persisted"`
	PersistenceReason string `json:"persistence_reason,omitempty"`
	// Reason is set only when !Applied.
	Reason string `json:"reason,omitempty"`
}

// PredictionReport is the complete 5-day event-driven prediction.
//
// C06：etf_estimates 與 revenue_surprises 移除 omitempty，保證欄位總是出現
// （無資料時為 []，前端可穩定依欄位是否存在判斷渲染）。這與 ATLAS-Go
// 其他 event 列表（如 active_events / predictions）一致。
type PredictionReport struct {
	GeneratedAt       time.Time             `json:"generated_at"`
	Window            string                `json:"window"` // "5-day forward"
	Predictions       []FlowPrediction      `json:"predictions"`
	ActiveEvents      []EventCalendarItem   `json:"active_events"`
	ETFEstimates      []ETFEstimate         `json:"etf_estimates"`
	RevenueSurprises  []RevenueSurprise     `json:"revenue_surprises"`
	SectorPredictions []SectorDayPrediction `json:"sector_predictions"`
	// SectorPredictionStatus reports whether per-sector predictions were
	// produced and, when not, the machine-readable reason. Always populated
	// (nil only for reports built outside the handler) so a disabled
	// SECTOR_PREDICTION_ENABLED flag is visible instead of a silent empty
	// array (#1944 Batch 3, items I4/I5/I6).
	SectorPredictionStatus *SectorPredictionStatus `json:"sector_prediction_status,omitempty"`
	Summary                string                  `json:"summary"`

	// HistoricalHitRate is the realized directional hit rate over the
	// recent window of completed (T+1-reconciled) predictions. nil when
	// the prediction store is not wired or fewer than MinHitSamples
	// predictions have been reconciled (product positioning §6: 預測可信
	// 三要件 — 誤差回饋)。Frontend renders "校準中" in that case rather
	// than a misleading percentage.
	HistoricalHitRate *HistoricalHitRate `json:"historical_hit_rate,omitempty"`
}

// HistoricalHitRate summarizes realized prediction accuracy over a window.
// Hits compare the predicted direction sign against the reconciled actual
// sign (same-unit, §6). Calibrated is false until MinHitSamples are met.
// WindowRecords is the number of recent prediction records read (one per
// trading day ≈ 60 trading days, NOT 60 calendar days) — frontend must
// label it as a record span, not a day span, to stay honest (§9).
type HistoricalHitRate struct {
	WindowRecords int     `json:"window_records"`
	Samples       int     `json:"samples"`
	Hits          int     `json:"hits"`
	HitRate       float64 `json:"hit_rate"` // 0..1; 0 when Samples==0
	Calibrated    bool    `json:"calibrated"`
	Reason        string  `json:"reason,omitempty"`
}
