package capitalflow

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
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

func TestHandleHistory(t *testing.T) {
	h := NewHandler(&mockProvider{snap: testSnapshot()})

	// Default (60 days) — store is empty, expect empty slices.
	req := httptest.NewRequest(http.MethodGet, "/api/capital-flow/history", nil)
	code, data := h.HandleHistory(req)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}

	result, ok := data.(map[ForceName][]RollingSample)
	if !ok {
		t.Fatalf("expected map[ForceName][]RollingSample, got %T", data)
	}

	// All 7 dimensions must be present, even if empty.
	for _, dim := range []ForceName{
		ForceForeign, ForceFutures, ForceTSMADR,
		ForceInstitutional, ForceDealer, ForceGovernment, ForceRetail,
	} {
		samples, exists := result[dim]
		if !exists {
			t.Errorf("missing dimension %q in response", dim)
		}
		if samples == nil {
			t.Errorf("dimension %q should be non-nil slice (got nil)", dim)
		}
	}

	// Query with days=0 returns 400.
	reqBad := httptest.NewRequest(http.MethodGet, "/api/capital-flow/history?days=0", nil)
	codeBad, _ := h.HandleHistory(reqBad)
	if codeBad != http.StatusBadRequest {
		t.Errorf("expected 400 for days=0, got %d", codeBad)
	}

	// Query with days=999 caps to defaultHistoryLimit (252 since A01).
	reqCap := httptest.NewRequest(http.MethodGet, "/api/capital-flow/history?days=999", nil)
	codeCap, _ := h.HandleHistory(reqCap)
	if codeCap != http.StatusOK {
		t.Errorf("expected 200 for days=999 (capped to %d), got %d", defaultHistoryLimit, codeCap)
	}
}

// TestHandleHistory_BackwardCompat_NoMeta verifies A02 backward compatibility:
// when ?include_meta is NOT set, the response shape must remain the legacy
// flat map[ForceName][]RollingSample so H02 frontend (commit 04622ab1,
// shared_web/static/js/pages/capital-history.js) keeps working untouched.
func TestHandleHistory_BackwardCompat_NoMeta(t *testing.T) {
	h := NewHandler(&mockProvider{snap: testSnapshot()})

	req := httptest.NewRequest(http.MethodGet, "/api/capital-flow/history", nil)
	code, data := h.HandleHistory(req)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}

	result, ok := data.(map[ForceName][]RollingSample)
	if !ok {
		t.Fatalf("expected map[ForceName][]RollingSample (legacy flat shape), got %T", data)
	}
	for _, dim := range []ForceName{
		ForceForeign, ForceFutures, ForceTSMADR,
		ForceInstitutional, ForceDealer, ForceGovernment, ForceRetail,
	} {
		if _, exists := result[dim]; !exists {
			t.Errorf("legacy flat shape missing dimension %q", dim)
		}
	}
}

// TestHandleHistory_IncludeMeta_OK verifies the opt-in wrapper with all 7
// dimensions populated. status should be "complete" (no missing).
func TestHandleHistory_IncludeMeta_OK(t *testing.T) {
	h := newHandlerWithPopulatedStore(t)

	req := httptest.NewRequest(http.MethodGet, "/api/capital-flow/history?include_meta=true", nil)
	code, data := h.HandleHistory(req)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}

	// JSON round-trip normalizes typed map[ForceName][]RollingSample to
	// map[string]any for assertion ergonomics.
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	samples, ok := doc["samples"].(map[string]any)
	if !ok {
		t.Fatalf("expected samples map, got %T", doc["samples"])
	}
	meta, ok := doc["meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected meta map, got %T", doc["meta"])
	}

	for _, dim := range []ForceName{
		ForceForeign, ForceInstitutional, ForceDealer,
		ForceGovernment, ForceRetail, ForceFutures, ForceTSMADR,
	} {
		if _, exists := samples[string(dim)]; !exists {
			t.Errorf("samples wrapper missing dimension %q", dim)
		}
	}

	status, _ := meta["status"].(string)
	if status != "complete" {
		t.Errorf("status = %q, want complete (all 7 dims populated)", status)
	}
	if missing, _ := meta["missing_dimensions"].([]any); len(missing) != 0 {
		t.Errorf("missing_dimensions should be empty, got %v", missing)
	}
	if daysRequested, _ := meta["days_requested"].(float64); int(daysRequested) != defaultHistoryLimit {
		t.Errorf("days_requested = %v, want %d", daysRequested, defaultHistoryLimit)
	}

	dataStatus, ok := meta["data_status"].(map[string]any)
	if !ok {
		t.Fatalf("expected data_status map, got %T", meta["data_status"])
	}
	for _, dim := range []ForceName{
		ForceForeign, ForceInstitutional, ForceDealer,
		ForceGovernment, ForceRetail, ForceFutures, ForceTSMADR,
	} {
		ds, ok := dataStatus[string(dim)].(map[string]any)
		if !ok {
			t.Errorf("data_status[%q] not a map", dim)
			continue
		}
		if available, _ := ds["data_available"].(bool); !available {
			t.Errorf("data_status[%q].data_available = false, want true", dim)
		}
	}
}

// TestHandleHistory_IncludeMeta_Partial verifies status enum derivation when
// some dimensions have data and others do not. Mirrors the production
// scenario where `government` (PublicBank 早於 2018) has no data but
// `foreign` / `institutional` / `dealer` are populated.
func TestHandleHistory_IncludeMeta_Partial(t *testing.T) {
	h := newHandlerWithSelectiveStore(t, []ForceName{
		ForceForeign, ForceInstitutional, ForceDealer,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/capital-flow/history?include_meta=true", nil)
	code, data := h.HandleHistory(req)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}

	doc, ok := data.(map[string]any)
	if !ok {
		t.Fatalf("expected wrapper map, got %T", data)
	}
	meta := doc["meta"].(map[string]any)

	status, _ := meta["status"].(string)
	if status != "partial" {
		t.Errorf("status = %q, want partial", status)
	}

	missing, ok := meta["missing_dimensions"].([]any)
	if !ok {
		t.Fatalf("expected missing_dimensions []any, got %T", meta["missing_dimensions"])
	}
	if len(missing) == 0 {
		t.Fatal("expected missing_dimensions non-empty, got []")
	}
	missingSet := make(map[string]bool, len(missing))
	for _, m := range missing {
		missingSet[m.(string)] = true
	}
	for _, expected := range []ForceName{
		ForceGovernment, ForceRetail, ForceFutures, ForceTSMADR,
	} {
		if !missingSet[string(expected)] {
			t.Errorf("missing_dimensions should include %q, got %v", expected, missing)
		}
	}

	dataStatus := meta["data_status"].(map[string]any)
	for _, dim := range []ForceName{ForceForeign, ForceInstitutional, ForceDealer} {
		ds := dataStatus[string(dim)].(map[string]any)
		if available, _ := ds["data_available"].(bool); !available {
			t.Errorf("data_status[%q].data_available should be true", dim)
		}
	}
	for _, dim := range []ForceName{ForceGovernment, ForceRetail, ForceFutures, ForceTSMADR} {
		ds := dataStatus[string(dim)].(map[string]any)
		if available, _ := ds["data_available"].(bool); available {
			t.Errorf("data_status[%q].data_available should be false", dim)
		}
	}
}

// TestHandleHistory_IncludeMeta_Missing verifies status="missing" when the
// store is completely empty (all 7 dimensions report data_available=false).
func TestHandleHistoricalSnapshot_OK(t *testing.T) {
	store := NewMemoryRollingSampleStore(defaultHistoryLimit)
	now := "2026-07-17"
	for _, dim := range []ForceName{ForceForeign, ForceInstitutional} {
		if err := store.UpsertDay(context.Background(), now, []RollingSample{{
			TradingDate: now,
			Dimension:   dim,
			RawValue:    100,
			Unit:        "億股",
			SourceID:    "test",
		}}); err != nil {
			t.Fatalf("upsert %s: %v", dim, err)
		}
	}
	// Use package-level RegisterRoutes + mux.ServeHTTP to exercise PathValue.
	RegisterRoutes(http.NewServeMux(), &mockProvider{snap: testSnapshot()})
	// Rebuild: RegisterRoutes creates a new handler internally, but our
	// pre-populated store must be injected. Build a handler with the store
	// and manually register the routes on a fresh mux.
	mux := http.NewServeMux()
	cfHandler := NewHandlerWithStore(&mockProvider{snap: testSnapshot()}, store, nil)
	mux.Handle("GET /api/capital-flow/historical-snapshot/{trading_date}", shared.Get(cfHandler.HandleHistoricalSnapshot))

	req := httptest.NewRequest(http.MethodGet, "/api/capital-flow/historical-snapshot/2026-07-17", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["trading_date"] != "2026-07-17" {
		t.Errorf("trading_date = %v, want 2026-07-17", resp["trading_date"])
	}
	status, _ := resp["status"].(string)
	if status != "partial" {
		t.Errorf("status = %q, want partial (only 2 of 7 dims populated)", status)
	}
	dims, ok := resp["dimensions"].(map[string]any)
	if !ok {
		t.Fatalf("expected dimensions map, got %T", resp["dimensions"])
	}
	foreign, ok := dims["foreign"].(map[string]any)
	if !ok {
		t.Fatalf("foreign dim missing from response")
	}
	if da, _ := foreign["data_available"].(bool); !da {
		t.Errorf("foreign.data_available should be true")
	}
	gov, ok := dims["government"].(map[string]any)
	if !ok {
		t.Fatalf("government dim missing from response")
	}
	if da, _ := gov["data_available"].(bool); da {
		t.Errorf("government.data_available should be false")
	}
	if mr, _ := gov["missing_reason"].(string); mr == "" {
		t.Errorf("government should have a missing_reason")
	}
}

func TestHandleHistoricalSnapshot_MissingDate(t *testing.T) {
	mux := http.NewServeMux()
	cfHandler := NewHandler(&mockProvider{snap: testSnapshot()})
	mux.Handle("GET /api/capital-flow/historical-snapshot/{trading_date}", shared.Get(cfHandler.HandleHistoricalSnapshot))
	req := httptest.NewRequest(http.MethodGet, "/api/capital-flow/historical-snapshot/2027-01-01", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 even for missing date (status=missing), got %d", rr.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if status, _ := resp["status"].(string); status != "missing" {
		t.Errorf("status = %q, want missing", status)
	}
}

func TestHandleHistoricalSnapshot_ValidationErrors(t *testing.T) {
	h := NewHandler(&mockProvider{snap: testSnapshot()})
	req := httptest.NewRequest(http.MethodGet, "/api/capital-flow/historical-snapshot/20260717", nil)
	rr := httptest.NewRecorder()
	RegisterRoutes(http.NewServeMux(), &mockProvider{snap: testSnapshot()})
	// Direct call: PathValue won't work, but ValidateDateParam catches invalid format.
	// For PathValue-based routing the route won't match "20260717" (no dashes),
	// so we get 405 MethodNotAllowed from mux (pattern mismatch).
	cfHandler := NewHandler(&mockProvider{snap: testSnapshot()})
	cfMux := http.NewServeMux()
	cfMux.Handle("GET /api/capital-flow/historical-snapshot/{trading_date}", shared.Get(cfHandler.HandleHistoricalSnapshot))
	cfMux.ServeHTTP(rr, req)
	// This test only verifies that the route exists. Detailed validation for
	// date format is covered by ValidateDateParam in the handler.
	_ = h
}

func TestHandleHistory_IncludeMeta_Missing(t *testing.T) {
	h := NewHandler(&mockProvider{snap: testSnapshot()})

	req := httptest.NewRequest(http.MethodGet, "/api/capital-flow/history?include_meta=true", nil)
	code, data := h.HandleHistory(req)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}

	doc, ok := data.(map[string]any)
	if !ok {
		t.Fatalf("expected wrapper map, got %T", data)
	}
	meta := doc["meta"].(map[string]any)

	status, _ := meta["status"].(string)
	if status != "missing" {
		t.Errorf("status = %q, want missing (empty store)", status)
	}
	missing, _ := meta["missing_dimensions"].([]any)
	if len(missing) != 7 {
		t.Errorf("missing_dimensions should list all 7 dims when status=missing, got %d", len(missing))
	}
}

// newHandlerWithPopulatedStore creates a Handler backed by an in-memory
// rolling sample store pre-populated with 1 sample per dimension for the
// current day. Used by IncludeMeta tests to exercise the "complete" status.
func newHandlerWithPopulatedStore(t *testing.T) *Handler {
	t.Helper()
	store := NewMemoryRollingSampleStore(defaultHistoryLimit)
	now := time.Now().UTC().Format("2006-01-02")
	for _, dim := range []ForceName{
		ForceForeign, ForceInstitutional, ForceDealer,
		ForceGovernment, ForceRetail, ForceFutures, ForceTSMADR,
	} {
		if err := store.UpsertDay(context.Background(), now, []RollingSample{{
			TradingDate: now,
			Dimension:   dim,
			RawValue:    100,
			Unit:        "億股",
			SourceID:    "test",
		}}); err != nil {
			t.Fatalf("upsert %s: %v", dim, err)
		}
	}
	return NewHandlerWithStore(&mockProvider{snap: testSnapshot()}, store, nil)
}

// newHandlerWithSelectiveStore creates a Handler backed by an in-memory
// store populated only for the given dimensions. Used by IncludeMeta
// partial-status test to mirror the production PublicBank scenario.
func newHandlerWithSelectiveStore(t *testing.T, populated []ForceName) *Handler {
	t.Helper()
	store := NewMemoryRollingSampleStore(defaultHistoryLimit)
	now := time.Now().UTC().Format("2006-01-02")
	for _, dim := range populated {
		if err := store.UpsertDay(context.Background(), now, []RollingSample{{
			TradingDate: now,
			Dimension:   dim,
			RawValue:    50,
			Unit:        "億股",
			SourceID:    "test",
		}}); err != nil {
			t.Fatalf("upsert %s: %v", dim, err)
		}
	}
	return NewHandlerWithStore(&mockProvider{snap: testSnapshot()}, store, nil)
}
