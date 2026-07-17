package capitalflow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// mockProvider returns a fixed MacroDataSnapshot for testing.
type mockProvider struct {
	snap marketdata.MacroDataSnapshot
}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) FetchSnapshot(_ context.Context) (marketdata.MacroDataSnapshot, error) {
	return m.snap, nil
}

func testSnapshot() marketdata.MacroDataSnapshot {
	return marketdata.MacroDataSnapshot{
		ForeignInvestorNet:  marketdata.MacroDataPoint{Value: 50, ChangePct: 5},
		DomesticFundNet:     marketdata.MacroDataPoint{Value: 30, ChangePct: 3},
		DealerNet:           marketdata.MacroDataPoint{Value: 10, ChangePct: 1},
		TSMADR:              marketdata.MacroDataPoint{Value: 180, ChangePct: 2},
		RetailMarginBalance: marketdata.MacroDataPoint{Value: 100000, ChangePct: -1},
		RetailShortBalance:  marketdata.MacroDataPoint{Value: 5000, ChangePct: 0.5},
		RecordedAt:          1704067200,
	}
}

func TestForceExtract(t *testing.T) {
	ext := NewForceExtractor()
	snap := testSnapshot()
	forces := ext.Extract(snap)

	if len(forces) != 7 {
		t.Fatalf("expected 7 forces, got %d", len(forces))
	}

	// Check each force exists
	forceNames := map[ForceName]bool{}
	for _, f := range forces {
		forceNames[f.Force] = true
	}

	for _, name := range []ForceName{
		ForceForeign, ForceFutures, ForceTSMADR,
		ForceInstitutional, ForceDealer, ForceGovernment, ForceRetail,
	} {
		if !forceNames[name] {
			t.Errorf("missing force: %s", name)
		}
	}

	// Foreign should be positive (value=50)
	for _, f := range forces {
		if f.Force == ForceForeign && f.RawValue != 50 {
			t.Errorf("expected foreign raw=50, got %.2f", f.RawValue)
		}
	}
}

func TestResonanceAligned(t *testing.T) {
	// All three major forces bullish
	forces := []ForceScore{
		{Force: ForceForeign, Role: ForceRoleSubject, ZScore: 2.0, Trend: "bullish"},
		{Force: ForceInstitutional, Role: ForceRoleSubject, ZScore: 1.5, Trend: "bullish"},
		{Force: ForceGovernment, Role: ForceRoleSubject, ZScore: 0.8, Trend: "bullish"},
		{Force: ForceRetail, Role: ForceRoleSubject, ZScore: -0.5, Trend: "neutral"},
	}

	r := ComputeResonance(forces)
	if r.Coefficient != config.GetCapitalflowResonanceCoefficientMax() {
		t.Errorf("expected coefficient %.2f, got %.2f", config.GetCapitalflowResonanceCoefficientMax(), r.Coefficient)
	}
	if r.Direction != "bullish" {
		t.Errorf("expected direction bullish, got %s", r.Direction)
	}
}

func TestResonanceAdversarial(t *testing.T) {
	// Foreign bullish, government bearish
	forces := []ForceScore{
		{Force: ForceForeign, Role: ForceRoleSubject, ZScore: 2.0, Trend: "bullish"},
		{Force: ForceInstitutional, Role: ForceRoleSubject, ZScore: 0.2, Trend: "neutral"},
		{Force: ForceGovernment, Role: ForceRoleSubject, ZScore: -1.5, Trend: "bearish"},
	}

	r := ComputeResonance(forces)
	if r.Coefficient != config.GetCapitalflowResonanceCoefficientMin() {
		t.Errorf("expected coefficient %.2f, got %.2f", config.GetCapitalflowResonanceCoefficientMin(), r.Coefficient)
	}
}

// TestResonanceCoefficientRange guards the [0.5, 1.5] invariant documented
// in AGENTS.md (ResonanceResult row).
func TestResonanceCoefficientRange(t *testing.T) {
	cases := []struct {
		name   string
		forces []ForceScore
	}{
		{
			name: "all_bullish_aligned",
			forces: []ForceScore{
				{Force: ForceForeign, ZScore: 2.0, Trend: "bullish"},
				{Force: ForceInstitutional, ZScore: 1.5, Trend: "bullish"},
				{Force: ForceGovernment, ZScore: 0.8, Trend: "bullish"},
				{Force: ForceRetail, ZScore: -0.5, Trend: "neutral"},
			},
		},
		{
			name: "all_bearish_aligned",
			forces: []ForceScore{
				{Force: ForceForeign, ZScore: -2.0, Trend: "bearish"},
				{Force: ForceInstitutional, ZScore: -1.5, Trend: "bearish"},
				{Force: ForceGovernment, ZScore: -0.8, Trend: "bearish"},
			},
		},
		{
			name: "adversarial_foreign_vs_government",
			forces: []ForceScore{
				{Force: ForceForeign, ZScore: 2.0, Trend: "bullish"},
				{Force: ForceInstitutional, ZScore: 0.2, Trend: "neutral"},
				{Force: ForceGovernment, ZScore: -1.5, Trend: "bearish"},
			},
		},
		{
			name: "neutral_foreign",
			forces: []ForceScore{
				{Force: ForceForeign, ZScore: 0.1, Trend: "neutral"},
				{Force: ForceInstitutional, ZScore: 1.0, Trend: "bullish"},
				{Force: ForceGovernment, ZScore: 0.5, Trend: "bullish"},
			},
		},
		{
			name: "mixed_no_three_aligned",
			forces: []ForceScore{
				{Force: ForceForeign, ZScore: 2.0, Trend: "bullish"},
				{Force: ForceInstitutional, ZScore: 1.0, Trend: "bullish"},
				{Force: ForceGovernment, ZScore: -0.5, Trend: "bearish"},
			},
		},
		{
			name: "missing_government",
			forces: []ForceScore{
				{Force: ForceForeign, ZScore: 2.0, Trend: "bullish"},
				{Force: ForceInstitutional, ZScore: 1.0, Trend: "bullish"},
			},
		},
		{
			name:   "empty_forces",
			forces: nil,
		},
		{
			name: "single_force_extreme",
			forces: []ForceScore{
				{Force: ForceForeign, ZScore: 5.0, Trend: "bullish"},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := ComputeResonance(c.forces)
			minCoeff := config.GetCapitalflowResonanceCoefficientMin()
			maxCoeff := config.GetCapitalflowResonanceCoefficientMax()
			if r.Coefficient < minCoeff || r.Coefficient > maxCoeff {
				t.Errorf("Coefficient %.3f out of documented range [%.2f, %.2f]", r.Coefficient, minCoeff, maxCoeff)
			}
		})
	}
}

func TestQualityScore(t *testing.T) {
	forces := []ForceScore{
		{Force: ForceForeign, ZScore: 2.5},
		{Force: ForceInstitutional, ZScore: 1.0},
		{Force: ForceRetail, ZScore: 1.0},
	}
	quality := computeQualityScore(forces)
	if quality < 2.49 || quality > 2.51 {
		t.Errorf("expected quality ~2.5, got %.4f", quality)
	}
	label := qualityLabel(quality)
	if label != "strong_inflow" {
		t.Errorf("expected strong_inflow, got %s", label)
	}
}

func TestHandleDaily(t *testing.T) {
	h := NewHandler(&mockProvider{snap: testSnapshot()})

	req := httptest.NewRequest(http.MethodGet, "/api/capital-flow/daily", nil)
	code, data := h.HandleDaily(req)

	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}

	report, ok := data.(DailyReport)
	if !ok {
		t.Fatalf("expected DailyReport, got %T", data)
	}

	if len(report.Forces) != 7 {
		t.Errorf("expected 7 forces, got %d", len(report.Forces))
	}
	if report.Date.IsZero() {
		t.Error("report date should be set")
	}
}

func TestHandleSummary(t *testing.T) {
	h := NewHandler(&mockProvider{snap: testSnapshot()})

	req := httptest.NewRequest(http.MethodGet, "/api/capital-flow/summary", nil)
	code, data := h.HandleSummary(req)

	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}

	_, ok := data.(SummaryReport)
	if !ok {
		t.Fatalf("expected SummaryReport, got %T", data)
	}
}

func TestZScoreRollingWindow(t *testing.T) {
	w := newRollingWindow(5)
	for _, v := range []float64{10, 12, 11, 13, 14} {
		w.push(v)
	}

	z := w.zScore(15)
	// mean=12, std≈1.58, z≈1.89
	if z < 1.5 || z > 2.5 {
		t.Errorf("expected z around 1.89, got %.4f", z)
	}
}

// mustContainKey fails the test if key k is missing from map m. Mirrors the
// helper style used in internal/recommender/e2e_wired_test.go — stdlib
// only, no testify dependency.
func mustContainKey(t *testing.T, m map[string]any, k string) {
	t.Helper()
	if _, ok := m[k]; !ok {
		t.Errorf("response JSON missing key %q (existing keys: %v)", k, keysOf(m))
	}
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestHandleDaily_ResponseContainsNewAssessmentFields asserts the JSON wire
// of /api/capital-flow/daily carries the permanent E07 assessment and
// provenance contract (spec §9.5 / CF-INV-08 / CF-INV-11). Optional
// DominantActor and DominantSignal fields may be omitted while empty.
func TestHandleDaily_ResponseContainsNewAssessmentFields(t *testing.T) {
	h := NewHandler(&mockProvider{snap: testSnapshot()})

	req := httptest.NewRequest(http.MethodGet, "/api/capital-flow/daily", nil)
	code, data := h.HandleDaily(req)
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

	assessment, ok := doc["assessment"].(map[string]any)
	if !ok {
		t.Fatalf("response JSON missing 'assessment' object (spec §9.5 / CF-INV-08)")
	}
	mustContainKey(t, assessment, "calibration_status")
	mustContainKey(t, assessment, "as_of_trading_date")

	// Per-force provenance fields (spec §7).
	forces, ok := doc["forces"].([]any)
	if !ok {
		t.Fatalf("response JSON missing 'forces' array (got %T)", doc["forces"])
	}
	if len(forces) != 7 {
		t.Fatalf("forces len = %d, want 7 (CF-INV-01)", len(forces))
	}
	for i, raw := range forces {
		f, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("forces[%d] not a JSON object (got %T)", i, raw)
		}
		mustContainKey(t, f, "dimension_role")
		mustContainKey(t, f, "source_id")
		mustContainKey(t, f, "unit")
	}
}

// TestHandleSummary_ResponseContainsNewAssessmentFields mirrors the daily
// check for the /api/capital-flow/summary endpoint. Summary reuses the
// daily assessment under spec §9.5 / CF-INV-08, so the same JSON keys
// must appear.
func TestHandleSummary_ResponseContainsNewAssessmentFields(t *testing.T) {
	h := NewHandler(&mockProvider{snap: testSnapshot()})

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

	// Summary keeps the DailyReport.Assessment flat in the response so
	// the home-page renderer can show calibration status without a second
	// round-trip (CF-INV-08).
	mustContainKey(t, doc, "assessment")
}
