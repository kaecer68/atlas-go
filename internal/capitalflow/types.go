package capitalflow

import (
	"math"
	"time"
)

// ---------------------------------------------------------------------------
// Force types — the seven dimensions tracked by 七維錢潮雷達（3+2+2 分層）。
// See docs/specs/capital-flow-seven-dimension-spec.md §4 D-CF-04 for the
// canonical role taxonomy (official_actor / behavioral_proxy / positioning_indicator
// / cross_market_signal).
// ---------------------------------------------------------------------------

// ForceName identifies a capital force.
type ForceName string

const (
	ForceForeign       ForceName = "foreign"       // 外資：現貨買賣超
	ForceFutures       ForceName = "futures"       // 外資期貨未平倉淨額
	ForceTSMADR        ForceName = "tsm_adr"       // TSM ADR 溢價
	ForceInstitutional ForceName = "institutional" // 投信買賣超
	ForceDealer        ForceName = "dealer"        // 自營商買賣超
	ForceGovernment    ForceName = "government"    // 公股行庫（heuristic）
	ForceRetail        ForceName = "retail"        // 散戶（融資+當沖）
)

// DisplayName returns the Chinese display name for a force.
func (f ForceName) DisplayName() string {
	switch f {
	case ForceForeign:
		return "外資"
	case ForceFutures:
		return "外資期貨"
	case ForceTSMADR:
		return "TSM ADR"
	case ForceInstitutional:
		return "投信"
	case ForceDealer:
		return "自營商"
	case ForceGovernment:
		return "公股行庫"
	case ForceRetail:
		return "散戶"
	default:
		return string(f)
	}
}

// ForceScore is a standardized score for a single capital force.
//
// Role categorizes the force in the legacy §7 taxonomy (manifest #E05):
//
//   - "subject"          — one of the 5 main bodies (foreign, institutional,
//     dealer, government, retail); participates in resonance.
//   - "leading_indicator" — non-body input feeding a subject's leading Z
//     (e.g. foreign futures OI -> foreign.LeadingZ).
//   - "sentiment"         — non-body feature input (e.g. TSM ADR); never
//     influences resonance.
//
// DimensionRole (added in #E07) is the new authoritative 4-bucket
// classification (CF-INV-01 / spec §6): "official_actor" |
// "behavioral_proxy" | "positioning_indicator" | "cross_market_signal".
// The legacy Role field is retained for back-compat — do not break
// the JSON contract for consumers that still key off "subject /
// leading_indicator / sentiment" (spec §7.1).
//
// Deprecated=true marks forces kept in the API shape for backward
// compatibility but no longer driving resonance (futures + tsm_adr after #E05).
type ForceScore struct {
	Force       ForceName `json:"force"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role,omitempty"` // "subject" | "leading_indicator" | "sentiment"
	Deprecated  bool      `json:"deprecated,omitempty"`
	// DimensionRole classifies the dimension per the E07 4-bucket
	// taxonomy (spec §6 / CF-INV-01): one of
	// "official_actor" / "behavioral_proxy" / "positioning_indicator"
	// / "cross_market_signal". Always populated by the extractor.
	DimensionRole string `json:"dimension_role"`
	// EvidenceClass tags whether the reading is directly reported
	// by a first-party source ("official"), derived from a
	// first-party source by formula ("official_derived"), inferred
	// from a behavioral proxy ("proxy"), or imported from a
	// cross-market channel ("cross_market"). Spec §7 / CF-INV-02.
	EvidenceClass string `json:"evidence_class,omitempty"`
	// SourceID is the canonical first-party source registry key
	// (e.g. "SRC-TWSE-T86"). Spec §5.
	SourceID string `json:"source_id,omitempty"`
	// Unit is the measurement unit of RawValue (e.g.
	// "hundred_million_shares", "contracts", "pct").
	Unit string `json:"unit,omitempty"`
	// AsOfTradingDate is the trading day this reading corresponds
	// to (YYYY-MM-DD). Spec §6 / §7.
	AsOfTradingDate string `json:"as_of_trading_date,omitempty"`
	// SampleCount is the number of prior rolling samples the Z-score
	// was computed against. 0 means "fresh process / no calibration
	// samples yet". Spec §7.
	SampleCount int `json:"sample_count,omitempty"`
	// CalibrationStatus is one of "calibrating" / "eligible" /
	// "degraded". Spec §7 / §9.5.
	CalibrationStatus string `json:"calibration_status,omitempty"`
	// ParticipatesInActorConsensus is true only for the three
	// official_actor dimensions (foreign / institutional / dealer);
	// behavioral_proxy and signal dimensions stay false so the
	// actor consensus filter can exclude them (CF-INV-09).
	ParticipatesInActorConsensus bool    `json:"participates_in_actor_consensus,omitempty"`
	RawValue                     float64 `json:"raw_value"` // 原始值
	ZScore                       float64 `json:"z_score"`   // 60-day rolling Z-score
	Trend                        string  `json:"trend"`     // "bullish", "bearish", "neutral"
	// Weight is the legacy cross-unit weight; per spec §7.2 / CF-INV-07
	// the new contract zeros this and pairs it with WeightDeprecated=true
	// so existing readers see a no-op value while new readers gate on
	// the flag instead.
	Weight           float64 `json:"weight,omitempty"`
	WeightDeprecated bool    `json:"weight_deprecated,omitempty"`
	LeadingZ         float64 `json:"leading_z,omitempty"`     // foreign-only: Z of the leading indicator series (futures OI)
	LeadingTrend     string  `json:"leading_trend,omitempty"` // foreign-only: trend from LeadingZ
	DataAvailable    bool    `json:"data_available"`          // false when the source channel was empty; always emitted per #1262
}

// Force roles — manifest #E05 §7 taxonomy (legacy; kept for back-compat
// per spec §7.1 / CF-INV-01).
const (
	ForceRoleSubject          = "subject"
	ForceRoleLeadingIndicator = "leading_indicator"
	ForceRoleSentiment        = "sentiment"
)

// DimensionRole values — manifest #E07 §6 / CF-INV-01. The 4-bucket
// taxonomy replaces the legacy Role field for new consumers; legacy
// Role is preserved on ForceScore for back-compat (spec §7.1).
const (
	// DimensionRoleOfficialActor covers the 3 first-party 三大法人
	// T86 readings (foreign / institutional / dealer). These
	// dimensions vote in actor consensus.
	DimensionRoleOfficialActor = "official_actor"
	// DimensionRoleBehavioralProxy covers government + retail, the
	// two non-T86 dimensions whose readings are inferred rather than
	// directly reported. They are excluded from actor consensus but
	// still surface in the behavioral sub-assessment.
	DimensionRoleBehavioralProxy = "behavioral_proxy"
	// DimensionRolePositioningIndicator covers foreign futures OI, a
	// positioning signal that does not enter actor consensus.
	DimensionRolePositioningIndicator = "positioning_indicator"
	// DimensionRoleCrossMarketSignal covers TSM ADR, the only
	// cross-market channel today. It does not enter actor consensus.
	DimensionRoleCrossMarketSignal = "cross_market_signal"
)

// EvidenceClass values — spec §7 / CF-INV-02. EvidenceClass tags
// whether the reading is directly reported by a first-party source,
// derived from a first-party source by formula, inferred from a
// behavioral proxy, or imported from a cross-market channel.
const (
	EvidenceOfficial        = "official"
	EvidenceOfficialDerived = "official_derived"
	EvidenceProxy           = "proxy"
	EvidenceCrossMarket     = "cross_market"
)

// CalibrationStatus values — spec §7 / §9.5. "calibrating" is the
// default state for a fresh process (no calibration history yet).
// "eligible" is reached only after H-CF-02 / spec §8.4 is validated.
// "degraded" is reserved for processes that lose calibration
// integrity (e.g. fewer than the required sample count).
const (
	CalibrationCalibrating = "calibrating"
	CalibrationEligible    = "eligible"
	CalibrationDegraded    = "degraded"
)

// ---------------------------------------------------------------------------
// Resonance — how forces align
// ---------------------------------------------------------------------------

// ResonanceResult captures force alignment.
type ResonanceResult struct {
	// Coefficient is 1.5 when foreign + institutional + government align,
	// 0.5 when foreign vs government diverge, 1.0 otherwise.
	Coefficient float64 `json:"coefficient"`
	// Aligned groups (e.g. ["foreign", "institutional"] when both bullish)
	Aligned []ForceName `json:"aligned"`
	// Opposing groups
	Opposing []ForceName `json:"opposing,omitempty"`
	// Direction: "bullish", "bearish", "mixed"
	Direction string `json:"direction"`
}

// ---------------------------------------------------------------------------
// Daily capital flow report
// ---------------------------------------------------------------------------

// DailyReport is the complete daily capital flow analysis.
type DailyReport struct {
	Date      time.Time       `json:"date"`
	Forces    []ForceScore    `json:"forces"`
	Resonance ResonanceResult `json:"resonance"`
	// QualityScore = Foreign + Institutional – Retail (Z-score composite)
	// (period-weighted variant when capitalflow.period_weighted_quality=true,
	// PR-3a).
	QualityScore float64 `json:"quality_score"`
	// QualityScorePeriodWeighted is the period-weighted composite
	// (computeQualityScoreWithPeriod) emitted alongside quality_score for
	// the 30-trading-day observation window (PR-3a / k3 review B4). When
	// no period is known it equals the equal-weight legacy value.
	QualityScorePeriodWeighted float64 `json:"quality_score_period_weighted"`
	QualityLabel               string  `json:"quality_label"` // "strong_inflow", "inflow", "neutral", "outflow", "strong_outflow"
	Summary                    string  `json:"summary"`       // 繁體中文 summary
	// Assessment is the E07 4-layer sub-assessment
	// (Institutional / Behavioral / ForeignPositioning / CrossMarket)
	// built by ComputeCapitalFlowAssessment. It is the authoritative
	// face for automation (CF-INV-08 / spec §9.5).
	Assessment CapitalFlowAssessment `json:"assessment"`
	// LegacyQuality=true signals that QualityScore / QualityLabel
	// above are still produced by the legacy F+Inst-Retail
	// composite and MUST NOT be fed into automation until H-CF-02
	// validates the new contract.
	LegacyQuality bool `json:"legacy_quality"`
	// DominantActor is the official_actor dimension with the
	// largest |ZScore| on the trading day. Spec §7 / CF-INV-10.
	DominantActor ForceName `json:"dominant_actor,omitempty"`
	// DominantSignal is the positioning_indicator /
	// cross_market_signal dimension with the largest |ZScore|.
	// Empty when no signal data is present.
	DominantSignal ForceName `json:"dominant_signal,omitempty"`
}

// SummaryReport is a condensed version for the /api/capital-flow/summary endpoint.
// It includes the top forces so the home page can render metric cards without a
// second round-trip to /api/capital-flow/daily.
type SummaryReport struct {
	Date         time.Time `json:"date"`
	QualityScore float64   `json:"quality_score"`
	// QualityScorePeriodWeighted is the period-weighted composite emitted
	// alongside quality_score for the 30-trading-day observation window
	// (PR-3a / k3 review B4). Equals the legacy value when no period is
	// known.
	QualityScorePeriodWeighted float64      `json:"quality_score_period_weighted"`
	QualityLabel               string       `json:"quality_label"`
	ResonanceDir               string       `json:"resonance_dir"`
	DominantForce              ForceName    `json:"dominant_force"` // force with highest absolute Z-score
	Forces                     []ForceScore `json:"forces"`
	Summary                    string       `json:"summary"`
	// Assessment mirrors DailyReport.Assessment so the home page
	// can render the calibration gate without a second round-trip
	// (CF-INV-08). Summary flattens the layered sub-assessments in
	// the same JSON shape.
	Assessment     CapitalFlowAssessment `json:"assessment"`
	LegacyQuality  bool                  `json:"legacy_quality"`
	DominantActor  ForceName             `json:"dominant_actor,omitempty"`
	DominantSignal ForceName             `json:"dominant_signal,omitempty"`
}

// DirectionalAssessment is one layer of the E07 capital-flow
// assessment (spec §9.5 / CF-INV-08). Each layer reports whether it
// has enough data to speak (Available), the direction it sees
// (Direction: "bullish" / "bearish" / "mixed" / ""), the dimensions
// that voted with that direction (Aligned), the ones that voted
// against (Opposing), and a free-form reasons slice for the home-page
// renderer to display ("校準中", "缺公股資料", etc.).
type DirectionalAssessment struct {
	Available bool        `json:"available"`
	Direction string      `json:"direction,omitempty"`
	Aligned   []ForceName `json:"aligned,omitempty"`
	Opposing  []ForceName `json:"opposing,omitempty"`
	Reasons   []string    `json:"reasons,omitempty"`
}

// CapitalFlowAssessment is the E07 4-layer assessment of today's
// capital-flow picture (spec §9.5 / CF-INV-08). It is the
// authoritative face for automation; automation callers MUST gate
// on EligibleForAutomation() and stay neutral while the
// CalibrationStatus is "calibrating" or "degraded" (CF-INV-13).
//
// PrimaryFlow is intentionally empty in E07; Task 8 will populate it
// once the calibration gate lifts. Reasons carries free-form text for
// the home-page renderer to display.
type CapitalFlowAssessment struct {
	AsOfTradingDate   string                `json:"as_of_trading_date"`
	CalibrationStatus string                `json:"calibration_status"`
	Institutional     DirectionalAssessment `json:"institutional"`
	Behavioral        DirectionalAssessment `json:"behavioral"`
	ForeignPosition   DirectionalAssessment `json:"foreign_positioning"`
	CrossMarket       DirectionalAssessment `json:"cross_market"`
	PrimaryFlow       string                `json:"primary_flow,omitempty"`
	Reasons           []string              `json:"reasons,omitempty"`
}

// EligibleForAutomation reports whether the assessment is safe to
// drive automation. Only the "eligible" CalibrationStatus opens the
// gate; "calibrating" (no calibration history yet) and "degraded"
// (calibration integrity lost) both keep the gate closed. This is
// the canonical CF-INV-13 guard.
func (a CapitalFlowAssessment) EligibleForAutomation() bool {
	return a.CalibrationStatus == CalibrationEligible
}

// ForceProvenance is the 4-field provenance row returned by
// ComputeForceProvenance (spec §6 / §7 / CF-INV-01). It is the
// per-dimension companion to ForceScore and intentionally lives at
// package scope so callers (and tests) can ask "what provenance
// does this dimension have" without needing a ForceScore at hand.
type ForceProvenance struct {
	// DimensionRole is one of the 4 E07 buckets: "official_actor",
	// "behavioral_proxy", "positioning_indicator",
	// "cross_market_signal". See DimensionRole* constants.
	DimensionRole string
	// SourceID is the canonical first-party source registry key
	// (e.g. "SRC-TWSE-T86"). See Source* constants in
	// rolling_store.go.
	SourceID string
	// Unit is the measurement unit of the reading (e.g.
	// "hundred_million_shares", "contracts", "pct").
	Unit string
	// ParticipatesInActorConsensus is true only for the 3
	// official_actor dimensions. Behavioral proxy and signal
	// dimensions stay false so the actor consensus filter
	// (CF-INV-09) can exclude them.
	ParticipatesInActorConsensus bool
}

// ---------------------------------------------------------------------------
// Rolling window for Z-score
// ---------------------------------------------------------------------------

// rollingWindow tracks a fixed-size window of values for Z-score calculation.
type rollingWindow struct {
	values []float64
	size   int
	pos    int
	count  int
}

// newRollingWindow creates a window with the given capacity.
func newRollingWindow(size int) *rollingWindow {
	return &rollingWindow{
		values: make([]float64, size),
		size:   size,
	}
}

// push adds a value. Returns true if the window is full.
func (w *rollingWindow) push(v float64) {
	w.values[w.pos] = v
	w.pos = (w.pos + 1) % w.size
	if w.count < w.size {
		w.count++
	}
}

// mean returns the average of values in the window.
func (w *rollingWindow) mean() float64 {
	if w.count == 0 {
		return 0
	}
	sum := 0.0
	for i := 0; i < w.count; i++ {
		sum += w.values[i]
	}
	return sum / float64(w.count)
}

// stddev returns the population standard deviation.
func (w *rollingWindow) stddev() float64 {
	if w.count < 2 {
		return 1.0 // avoid division by zero; return 1 so Z = raw – mean
	}
	m := w.mean()
	sumSq := 0.0
	for i := 0; i < w.count; i++ {
		d := w.values[i] - m
		sumSq += d * d
	}
	return math.Max(0.01, math.Sqrt(sumSq/float64(w.count)))
}

// zScore computes (v – mean) / stddev.
func (w *rollingWindow) zScore(v float64) float64 {
	s := w.stddev()
	if s <= 0 {
		return 0
	}
	return (v - w.mean()) / s
}

// ---------------------------------------------------------------------------
// Rolling-window persistence sample (BK-15 / spec §8.5)
// ---------------------------------------------------------------------------

// RollingSample is one persisted (dimension, trading_date) observation
// of a capital force. Samples are the only input to the rolling Z-score
// reference window after a process restart, and must therefore carry
// enough provenance (raw value, unit, source id) to be reproducible.
//
// Spec anchors:
//   - §8.2 (CF-INV-05): at most one sample per (dimension, trading_date).
//   - §8.5: persistence must round-trip across restart.
//   - §5: source_id must trace back to the first-party source registry.
type RollingSample struct {
	TradingDate string    `json:"trading_date"`
	Dimension   ForceName `json:"dimension"`
	RawValue    float64   `json:"raw_value"`
	Unit        string    `json:"unit"`
	SourceID    string    `json:"source_id"`
}
