package capitalflow

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// ===========================================================================
// M6 — GenerateSummaryReport must not fabricate a dominant force.
// (audit 2026-09-04): with no actor and no signal reading, the report used
// to fall back to ForceRetail, naming 散戶 the default protagonist even when
// no retail signal exists. It now stays empty; the API/front-end render an
// empty dominant_force as "—".
// ===========================================================================

// emptySnapshot has every source channel missing so no dimension reports
// DataAvailable — the "no actor / no signal" scenario from audit M6.
func emptySnapshot() marketdata.MacroDataSnapshot {
	return marketdata.MacroDataSnapshot{
		RecordedAt: time.Date(2026, 9, 4, 4, 0, 0, 0, time.UTC).Unix(),
	}
}

func TestGenerateSummaryReport_NoActorNoSignal_LeavesDominantEmpty(t *testing.T) {
	snap := emptySnapshot()
	forces := NewForceExtractor().Score(snap, "2026-09-04", nil)
	summary := GenerateSummaryReport(time.Unix(snap.RecordedAt, 0), forces, ComputeResonance(forces), nil, false)

	if summary.DominantForce != "" {
		t.Errorf("DominantForce = %q, want empty (must NOT fall back to retail)", summary.DominantForce)
	}
	if summary.DominantActor != "" || summary.DominantSignal != "" {
		t.Errorf("DominantActor/DominantSignal = %q/%q, want empty/empty", summary.DominantActor, summary.DominantSignal)
	}
	// The short summary text must not invent a protagonist either.
	if got := summary.Summary; got == "" {
		t.Error("Summary empty for an all-missing snapshot")
	}
}

// TestGenerateSummaryReport_DominantPicksActorThenSignal locks the
// non-empty side of M6: dominant prefers the actor slot and only falls
// back to the signal slot — never to a random actor default.
func TestGenerateSummaryReport_DominantPicksActorThenSignal(t *testing.T) {
	date := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	forces := []ForceScore{
		{Force: ForceForeign, Role: ForceRoleSubject, DimensionRole: DimensionRoleOfficialActor, ZScore: 2.0, Trend: "bullish", DataAvailable: true},
		{Force: ForceInstitutional, Role: ForceRoleSubject, DimensionRole: DimensionRoleOfficialActor, ZScore: 1.5, Trend: "bullish", DataAvailable: true},
		{Force: ForceDealer, Role: ForceRoleSubject, DimensionRole: DimensionRoleOfficialActor, ZScore: 1.0, Trend: "bullish", DataAvailable: true},
		{Force: ForceFutures, Role: ForceRoleLeadingIndicator, DimensionRole: DimensionRolePositioningIndicator, Deprecated: true, ZScore: 5.0, Trend: "bullish", DataAvailable: true},
		{Force: ForceTSMADR, Role: ForceRoleSentiment, DimensionRole: DimensionRoleCrossMarketSignal, Deprecated: true, ZScore: 4.0, Trend: "bullish", DataAvailable: true},
		{Force: ForceGovernment, Role: ForceRoleSubject, DimensionRole: DimensionRoleBehavioralProxy, ZScore: -0.2, Trend: "neutral", DataAvailable: true},
		{Force: ForceRetail, Role: ForceRoleSubject, DimensionRole: DimensionRoleBehavioralProxy, ZScore: -0.1, Trend: "neutral", DataAvailable: true},
	}
	summary := GenerateSummaryReport(date, forces, ResonanceResult{Direction: "bullish", Coefficient: 1.5}, nil, false)
	if summary.DominantForce != ForceForeign {
		t.Errorf("DominantForce = %q, want %q (actor preferred; futures 5.0 must not win the actor slot)", summary.DominantForce, ForceForeign)
	}
	if summary.DominantActor != ForceForeign {
		t.Errorf("DominantActor = %q, want %q", summary.DominantActor, ForceForeign)
	}
}

// TestHandleSummary_NoData_DominantForceEmptyOnTheWire asserts the JSON
// surface of /api/capital-flow/summary returns "dominant_force": "" —
// not "retail" — for a snapshot with no actor / no signal data.
func TestHandleSummary_NoData_DominantForceEmptyOnTheWire(t *testing.T) {
	h := NewHandler(&mockProvider{snap: emptySnapshot()})
	req := httptest.NewRequest(http.MethodGet, "/api/capital-flow/summary", nil)
	code, data := h.HandleSummary(req)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	dominant, ok := doc["dominant_force"].(string)
	if !ok {
		t.Fatalf("dominant_force missing or not a string: %v", doc["dominant_force"])
	}
	if dominant != "" {
		t.Errorf("dominant_force = %q, want empty string (M6: no retail fallback)", dominant)
	}
}

// ===========================================================================
// PR-3a — config-gated period-weighted quality score (plan v1.1 / k3 B4).
// The weight table 1.3/0.7/1.5 is locked here; the switch-off path must be
// bit-identical to the legacy composite.
// ===========================================================================

func bullPeriod() *domain.MarketPeriod {
	p := domain.PeriodBull
	return &p
}

func TestComputeQualityScoreWithPeriod_WeightTableLocked(t *testing.T) {
	forces := []ForceScore{
		{Force: ForceForeign, ZScore: 1.0},
		{Force: ForceInstitutional, ZScore: 1.0},
		{Force: ForceRetail, ZScore: 1.0},
	}
	cases := []struct {
		period domain.MarketPeriod
		want   float64 // wF*1 + wI*1 - wR*1
	}{
		{domain.PeriodBull, 1.3},           // wF=1.3
		{domain.PeriodTurnaroundUp, 1.3},   // wF=1.3
		{domain.PeriodDownturn, 1.0},       // wI=1.3, wF=0.7 → 0.7+1.3-1
		{domain.PeriodTurnaroundDown, 1.0}, // 0.7+1.3-1
		{domain.PeriodBlackSwan, 0.5},      // wR=1.5 → 1+1-1.5
		{domain.PeriodPlateau, 1.0},        // default equal weights
		{domain.PeriodConsolidation, 1.0},  // default equal weights
	}
	for _, tc := range cases {
		p := tc.period
		got := computeQualityScoreWithPeriod(forces, &p)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("period %s: got %.4f, want %.4f", tc.period, got, tc.want)
		}
	}
	// nil period → equal weights = legacy.
	if got := computeQualityScoreWithPeriod(forces, nil); got != 1.0 {
		t.Errorf("nil period: got %.4f, want 1.0", got)
	}
}

func TestGenerateDailyReport_SwitchOffLegacyBitIdentical(t *testing.T) {
	forces := []ForceScore{
		{Force: ForceForeign, ZScore: 1.2, DataAvailable: true},
		{Force: ForceInstitutional, ZScore: 0.4, DataAvailable: true},
		{Force: ForceRetail, ZScore: -0.3, DataAvailable: true},
	}
	date := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	res := ComputeResonance(forces)

	// Switch OFF + period present: quality_score must be bit-identical to
	// the legacy equal-weight composite; only the observation column
	// (quality_score_period_weighted) may differ.
	legacy := computeQualityScore(forces)
	off := GenerateDailyReport(date, forces, res, bullPeriod(), false)
	if off.QualityScore != round(legacy, 2) {
		t.Errorf("switch off: quality_score = %v, want legacy %v", off.QualityScore, round(legacy, 2))
	}
	if !off.LegacyQuality {
		t.Errorf("switch off: legacy_quality = false, want true")
	}
	if off.QualityScorePeriodWeighted == off.QualityScore {
		t.Errorf("switch off: observation column should reflect period weighting for comparison")
	}

	// Switch ON: quality_score takes the period-weighted value.
	on := GenerateDailyReport(date, forces, res, bullPeriod(), true)
	if on.QualityScore != on.QualityScorePeriodWeighted {
		t.Errorf("switch on: quality_score = %v, want period-weighted %v", on.QualityScore, on.QualityScorePeriodWeighted)
	}
	if on.LegacyQuality {
		t.Errorf("switch on: legacy_quality = true, want false")
	}
}
