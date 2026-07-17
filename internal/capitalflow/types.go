package capitalflow

import (
	"math"
	"time"
)

// ---------------------------------------------------------------------------
// Force types — the seven capital forces tracked
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

// ForceScore is a standardized score for a single capital force.
//
// Role categorizes the force in the §7 taxonomy (manifest #E05):
//
//   - "subject"          — one of the 5 main bodies (foreign, institutional,
//     dealer, government, retail); participates in resonance.
//   - "leading_indicator" — non-body input feeding a subject's leading Z
//     (e.g. foreign futures OI -> foreign.LeadingZ).
//   - "sentiment"         — non-body feature input (e.g. TSM ADR); never
//     influences resonance.
//
// Deprecated=true marks forces kept in the API shape for backward
// compatibility but no longer driving resonance (futures + tsm_adr after #E05).
type ForceScore struct {
	Force         ForceName `json:"force"`
	Role          string    `json:"role,omitempty"` // "subject" | "leading_indicator" | "sentiment"
	Deprecated    bool      `json:"deprecated,omitempty"`
	RawValue      float64   `json:"raw_value"`                // 原始值
	ZScore        float64   `json:"z_score"`                  // 60-day rolling Z-score
	Trend         string    `json:"trend"`                    // "bullish", "bearish", "neutral"
	Weight        float64   `json:"weight"`                   // dynamic weight derived from relative magnitude
	LeadingZ      float64   `json:"leading_z,omitempty"`      // foreign-only: Z of the leading indicator series (futures OI)
	LeadingTrend  string    `json:"leading_trend,omitempty"`  // foreign-only: trend from LeadingZ
	DataAvailable bool      `json:"data_available,omitempty"` // false when the source channel was empty (e.g. no government file)
}

// Force roles — manifest #E05 §7 taxonomy.
const (
	ForceRoleSubject          = "subject"
	ForceRoleLeadingIndicator = "leading_indicator"
	ForceRoleSentiment        = "sentiment"
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
	QualityScore float64 `json:"quality_score"`
	QualityLabel string  `json:"quality_label"` // "strong_inflow", "inflow", "neutral", "outflow", "strong_outflow"
	Summary      string  `json:"summary"`       // 繁體中文 summary
}

// SummaryReport is a condensed version for the /api/capital-flow/summary endpoint.
// It includes the top forces so the home page can render metric cards without a
// second round-trip to /api/capital-flow/daily.
type SummaryReport struct {
	Date          time.Time    `json:"date"`
	QualityScore  float64      `json:"quality_score"`
	QualityLabel  string       `json:"quality_label"`
	ResonanceDir  string       `json:"resonance_dir"`
	DominantForce ForceName    `json:"dominant_force"` // force with highest absolute Z-score
	Forces        []ForceScore `json:"forces"`
	Summary       string       `json:"summary"`
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
