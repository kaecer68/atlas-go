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

	// SectorPredictionsFrontendConsumer reports whether any frontend READS
	// `sector_predictions` (issue #1944 item N-P2).
	//
	// None does. The array is produced (it is the sector forecast) and read by the
	// experimental C07 observability tools over the API, and it is mirrored into
	// the generated frontend field lists (shared_web/static/js/shared/
	// {valid_fields.json,field_types.ts}) — but a generated type mirror is not a
	// consumer: no page in shared_web/client_web/admin_web reads the value. The
	// declaration exists so that "it must be used, it is in the bundle" has a
	// machine-readable answer, and so that adding a real reader FAILS
	// TestSectorPredictions_HasNoFrontendReader until this constant is revisited.
	SectorPredictionsFrontendConsumer = false
	// SectorPredictionPersistenceReason explains the false above.
	SectorPredictionPersistenceReason = "ledger_event_flow_prediction_record_has_no_sector_column"

	// SectorCycleProviderWired reports whether production injects a cycle-score
	// provider (industry.CycleTracker.GetContinuousPhaseScore) into the
	// SectorPredictor. true since #1944 Batch 4 completed the I4 cycle half:
	// cmd/atlas injects the industry service's CycleTracker — the instance
	// auto_cycle_update writes to through UpdatePosition with FinMind-measured
	// revenue/profit growth, and the instance the composition root shares (I13)
	// — wrapped in MeasuredCycleProvider so industries that only carry the
	// startup seed still contribute the neutral 0.0 instead of a seed presented
	// as a measurement. Flip this back only with a documented reason; the
	// per-request status still derives from the live predictor
	// (SectorPredictionStatus.CycleProviderWired).
	SectorCycleProviderWired = true
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
	// CycleProviderWired is derived from the predictor's live state: true when
	// Handler.SetSectorCycleProvider was given a provider (production does this
	// whenever sector prediction is built — see SectorCycleProviderWired), which
	// is what lets the `cycle_position` driver contribute for industries with
	// measured evidence (#1944 item I4 cycle half).
	CycleProviderWired bool `json:"cycle_provider_wired"`
	// Persisted mirrors SectorPredictionPersisted for this payload.
	Persisted         bool   `json:"persisted"`
	PersistenceReason string `json:"persistence_reason,omitempty"`
	// Reason is set only when !Applied.
	Reason string `json:"reason,omitempty"`
}

// Advisory-status constants for PredictionAdvisoryStatus.
//
// Context (2026-10-06, edge-verification program Phase 0): the owner withdrew
// the 5-day direction card from the retail home page and required the endpoint
// to say out loud that its payload must not be used as a basis for advice. Two
// measured mechanisms made the output non-discriminating in production
// (day1-day5 identical direction/confidence/drivers):
//
//  1. mixed-direction calendar events add the same weight to BOTH sides, so
//     their net contribution is exactly zero (predictor.go, mixed factor);
//  2. while the capital-flow baseline is not eligible, its decay weight is
//     discounted, capping day-1 at |baseline| <= 0.8 * 0.7 * 0.5 = 0.28 —
//     below the +/-0.3 band, so the baseline alone can never set a direction.
//
// The endpoint itself stays (research/replay consumers read it); the evidence
// is recorded in docs/specs/eventdriven-spec.md and the freeze list in
// docs/operations/EDGE-PROGRAM-FREEZE.md.
const (
	// AdvisoryStatusWithdrawn is the only status shipped in this phase: the
	// report is disclosed and withdrawn at the same time.
	AdvisoryStatusWithdrawn = "withdrawn_constructive_abstention"

	// AdvisoryReasonNoEdgeEvidence — no day in the 5-day window cleared the
	// +/-0.3 net-weight band, i.e. the signal family produced no directional
	// evidence at all.
	AdvisoryReasonNoEdgeEvidence = "no_edge_evidence"
	// AdvisoryReasonMixedEventCancellation — mixed-direction events were active
	// inside the forecast window. They contribute w*0.3 to the bullish AND the
	// bearish side, so their net contribution is exactly 0 while still
	// inflating the driver list.
	AdvisoryReasonMixedEventCancellation = "mixed_event_cancellation"
	// AdvisoryReasonCalibrationDiscountBelowThreshold — the capital-flow
	// baseline was not eligible, so its weight is discounted. The resulting
	// ceiling (0.8 * day-1 weight 0.7 * discount 0.5 = 0.28) stays below the
	// +/-0.3 band: while calibrating the baseline alone cannot set a direction.
	AdvisoryReasonCalibrationDiscountBelowThreshold = "calibration_discount_below_threshold"

	// AdvisoryMessage is the human-readable warning that consumers (web UI,
	// MCP clients, agents) must render verbatim instead of presenting the
	// predictions as a forecast.
	AdvisoryMessage = "本報告未通過否證：5 日方向預測受構造性棄權影響（生產實測 day1-day5 同方向同信心同驅動），不得作為投資建議或任何決策依據。"
	// AdvisoryEvidenceRef points at the spec section that records the measured
	// mechanism.
	AdvisoryEvidenceRef = "docs/specs/eventdriven-spec.md 十一、構造性棄權與建議下架"
)

// PredictionAdvisoryStatus makes the withdrawn state of the 5-day report
// machine-readable, so no consumer has to infer it from the predictions. It
// mirrors the SectorPredictionStatus pattern: the fact is derived per request
// from the report and the live wiring, never hard-coded per deployment.
type PredictionAdvisoryStatus struct {
	// AdvisoryUsable is false for every report built in this phase. It is set
	// unconditionally (not derived) because the decision is a governance one:
	// the signal family has not passed G2/G3/G4', so no direction may be
	// presented as advice even on a day that clears the band.
	AdvisoryUsable bool `json:"advisory_usable"`
	// Status is the machine-readable lifecycle state; currently always
	// AdvisoryStatusWithdrawn.
	Status string `json:"status"`
	// AbstentionReasons lists, in sorted order, the mechanisms that were
	// actually observed for this report. It is empty when a directional call
	// cleared the band — the advisory withdrawal above still applies.
	AbstentionReasons []string `json:"abstention_reasons"`
	// Message is the consumer-facing warning (AdvisoryMessage).
	Message string `json:"message"`
	// EvidenceRef points at the spec section holding the measured evidence.
	EvidenceRef string `json:"evidence_ref"`
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

	// AdvisoryStatus states in machine-readable form that this report must not
	// be used as a basis for advice, and why it abstains from a directional
	// call. Always populated by Predictor.Predict; nil only for reports built
	// outside it (handler-constructed literals in tests).
	AdvisoryStatus *PredictionAdvisoryStatus `json:"advisory_status,omitempty"`

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

	// NeutralSamples counts reconciled predictions whose predicted direction
	// was neutral (DirectionSign == 0). A neutral prediction can never be a
	// directional hit, so it sits in the denominator and drags the rate down
	// without being an error (N-U6, #1944 Batch 4: production showed 12.1%
	// built from neutral rows). Exposed so the rate is never read as
	// "directionally wrong 88% of the time".
	NeutralSamples int `json:"neutral_samples"`
	// DirectionalSamples is Samples - NeutralSamples: the rows the rate can
	// actually judge.
	DirectionalSamples int `json:"directional_samples"`
	// HitRateBasis is the machine-readable definition of the ratio. Stable
	// string so a UI can label the number instead of guessing.
	HitRateBasis string `json:"hit_rate_basis"`
}

// HitRateBasisDirectionSign is the only supported basis for
// HistoricalHitRate.HitRate: hits / samples over T+1-reconciled records, where a
// hit requires the predicted sign and the realized sign to agree and be
// non-zero (a neutral prediction is always a miss).
const HitRateBasisDirectionSign = "t_plus_1_reconciled_direction_sign"
