package eventdriven

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
)

type stubNarrative struct {
	models []ModelView
}

func (s *stubNarrative) ListModels() []ModelView { return s.models }

func TestSetNarrativeProvider_StoresProvider(t *testing.T) {
	models := []ModelView{
		{ID: "x", Name: "test", Weight: 0.5, Direction: "bullish", ActiveThemes: []string{"earnings_surprise"}},
	}
	h := newTestHandler()
	stub := &stubNarrative{models: models}
	h.SetNarrativeProvider(stub)

	if h.predictor.narrativeProvider == nil {
		t.Fatal("SetNarrativeProvider must store the provider, not a snapshot")
	}
	got := h.predictor.narrativeProvider.ListModels()
	if len(got) != 1 || got[0].ID != "x" {
		t.Fatalf("stored provider should expose wired models, got %v", got)
	}
}

func TestSetNarrativeProvider_NilProviderClearsProvider(t *testing.T) {
	h := newTestHandler()
	h.SetNarrativeProvider(&stubNarrative{models: []ModelView{{ID: "x", Weight: 0.5}}})
	if h.predictor.narrativeProvider == nil {
		t.Fatal("expected provider to be stored")
	}
	h.SetNarrativeProvider(nil)
	if h.predictor.narrativeProvider != nil {
		t.Errorf("nil provider must clear the provider, got %v", h.predictor.narrativeProvider)
	}
}

// TestPredictor_RefreshesNarrativeModelsOnEachPredict verifies H1: the
// predictor re-queries the provider on every Predict, so Darwinian weight
// updates (hourly scheduler) flow into the narrative tilt instead of a
// one-time wiring snapshot.
func TestPredictor_RefreshesNarrativeModelsOnEachPredict(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	cal := industry.NewEventCalendar()
	p := NewPredictor(cal)

	stub := &stubNarrative{models: []ModelView{
		{
			ID: "ai_supercycle", Weight: 1.0, Direction: "bullish",
			ActiveThemes: []string{"AI_capex_surge", "earnings_surprise"},
		},
	}}
	p.SetNarrativeProvider(stub)

	report := p.Predict(now)
	if len(report.Predictions) == 0 {
		t.Fatal("expected predictions")
	}
	for _, pred := range report.Predictions {
		if pred.Direction != "inflow" {
			t.Errorf("single bullish model should drive inflow (got %s on %s)",
				pred.Direction, pred.Date.Format("2006-01-02"))
		}
	}

	// Simulate an hourly UpdateModelWeights outcome: a heavy bearish model
	// joins the same theme set. The second Predict must observe it.
	stub.models = append(stub.models, ModelView{
		ID: "heavy_bear", Weight: 3.0, Direction: "bearish",
		ActiveThemes: []string{"AI_capex_surge", "earnings_surprise"},
	})

	report = p.Predict(now)
	for _, pred := range report.Predictions {
		if pred.Direction != "outflow" {
			t.Errorf("second predict must observe updated models (got %s on %s)",
				pred.Direction, pred.Date.Format("2006-01-02"))
		}
	}
}

func TestPredictor_MatchingThemeBoostsWeight(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	cal := industry.NewEventCalendar()
	cal.RefreshEvents(now)
	nowPlus3 := now.AddDate(0, 0, 3)
	p := NewPredictor(cal)
	p.SetNarrativeProvider(&stubNarrative{models: []ModelView{
		{
			ID: "msci_bull", Name: "MSCI bull", Weight: 0.4, Direction: "bullish",
			ActiveThemes: []string{"msci_rebalance", "index_rebalance"},
		},
	}})

	report := p.Predict(now)
	if len(report.Predictions) == 0 {
		t.Fatal("expected predictions")
	}
	for _, pred := range report.Predictions {
		if pred.Direction != "inflow" && pred.Direction != "neutral" {
			continue
		}
		if !dayOverlapsMSCIWindow(pred.Date, now, nowPlus3) {
			continue
		}
		if pred.Confidence < 0.5 {
			t.Errorf("day %s overlapping MSCI window: confidence should be boosted above 0.5, got %v",
				pred.Date.Format("2006-01-02"), pred.Confidence)
		}
	}
}

func TestPredictor_NonMatchingThemeIgnored(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	cal := industry.NewEventCalendar()
	cal.RefreshEvents(now)
	p := NewPredictor(cal)
	p.SetNarrativeProvider(&stubNarrative{models: []ModelView{
		{
			ID: "unrelated", Weight: 1.0, Direction: "bullish",
			ActiveThemes: []string{"never_matches_any_event"},
		},
	}})

	report := p.Predict(now)
	for _, pred := range report.Predictions {
		if pred.Confidence > 0.95 {
			t.Errorf("non-matching model should not push confidence to 0.95 on day %s (got %v)",
				pred.Date.Format("2006-01-02"), pred.Confidence)
		}
	}
}

func TestRegisterRoutesWithNarrative_AppliesModelTilt(t *testing.T) {
	mux := http.NewServeMux()
	cal := industry.NewEventCalendar()
	cal.RefreshEvents(time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC))

	strongBull := &stubNarrative{models: []ModelView{
		{
			ID: "msci_strong", Weight: 1.0, Direction: "bullish",
			ActiveThemes: []string{"msci_rebalance", "index_rebalance"},
		},
	}}
	RegisterRoutesWithNarrative(mux, cal, nil, strongBull)

	req := httptest.NewRequest(http.MethodGet, "/api/events/prediction", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	body := rec.Body.String()
	// #1384 calibration-aware baseline: nil cf keeps the default staticCF which
	// is permanently CalibrationCalibrating. The strong bull narrative theme
	// boosts day-level weight but does not flip the 5-day summary verdict
	// (event mix stays symmetric), so the summary must surface 分歧 + 校準中,
	// NOT 偏流入 and NOT a baseline drift note (cfScore=0).
	mustContain := []string{"未來 5 天資金流向分歧", "校準中", "關鍵事件"}
	for _, s := range mustContain {
		if !strings.Contains(body, s) {
			t.Errorf("narrative tilt summary missing %q, body=%s", s, body)
		}
	}
	if strings.Contains(body, "偏流入") {
		t.Errorf("narrative tilt must not yield 偏流入 verdict, body=%s", body)
	}
	if strings.Contains(body, "當前資金品質偏多") || strings.Contains(body, "當前資金品質偏空") {
		t.Errorf("narrative tilt with nil cf has zero baseline; baseline drift note must not appear, body=%s", body)
	}
}

func TestPredictor_NarrativeDrivesDirectionOnEmptyTimeline(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	cal := industry.NewEventCalendar()

	p := NewPredictor(cal)
	p.SetNarrativeProvider(&stubNarrative{models: []ModelView{
		{
			ID: "ai_supercycle", Weight: 1.0, Direction: "bullish",
			ActiveThemes: []string{"AI_capex_surge", "earnings_surprise"},
		},
	}})

	report := p.Predict(now)
	if len(report.Predictions) != 5 {
		t.Fatalf("expected 5 predictions, got %d", len(report.Predictions))
	}
	for _, pred := range report.Predictions {
		if pred.Direction != "inflow" {
			t.Errorf("matching AI/earnings theme should drive inflow (got %s on %s)",
				pred.Direction, pred.Date.Format("2006-01-02"))
		}
	}
}

func TestPredictor_BearishNarrativeFlipsDirection(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	cal := industry.NewEventCalendar()

	p := NewPredictor(cal)
	p.SetNarrativeProvider(&stubNarrative{models: []ModelView{
		{
			ID: "hawkish_fed", Weight: 1.0, Direction: "bearish",
			ActiveThemes: []string{"US_rates_up", "earnings_surprise"},
		},
	}})

	report := p.Predict(now)
	for _, pred := range report.Predictions {
		if pred.Direction != "outflow" {
			t.Errorf("matching bearish theme should drive outflow (got %s on %s)",
				pred.Direction, pred.Date.Format("2006-01-02"))
		}
	}
}

func TestComputeNarrativeTilt_NoMatch(t *testing.T) {
	themes := map[string]struct{}{"earnings_surprise": {}}
	got := computeNarrativeTilt([]ModelView{
		{ID: "x", Weight: 1.0, Direction: "bullish", ActiveThemes: []string{"never_matches_any_event"}},
	}, themes)
	if got != 0 {
		t.Errorf("non-matching themes should produce zero tilt, got %v", got)
	}
}

func TestComputeNarrativeTilt_MixedDirections(t *testing.T) {
	themes := map[string]struct{}{"earnings_surprise": {}}
	got := computeNarrativeTilt([]ModelView{
		{ID: "bull", Weight: 1.0, Direction: "bullish", ActiveThemes: []string{"earnings_surprise"}},
		{ID: "bear", Weight: 0.5, Direction: "bearish", ActiveThemes: []string{"earnings_surprise"}},
		{ID: "neut", Weight: 2.0, Direction: "neutral", ActiveThemes: []string{"earnings_surprise"}},
	}, themes)
	want := 1.0*1.0 + 0.5*(-1.0) + 2.0*0
	if got != want {
		t.Errorf("mixed-direction tilt: got %v want %v", got, want)
	}
}

func TestEventTypeToThemes(t *testing.T) {
	cases := map[string][]string{
		string(industry.EventMSCIRebalance):   {"msci_rebalance", "tw50_rebalance", "index_rebalance"},
		string(industry.EventMonthlyRevenue):  {"monthly_revenue", "earnings_surprise"},
		string(industry.EventFinancialReport): {"financial_report", "earnings_surprise"},
	}
	for eventType, want := range cases {
		got := eventTypeToThemes(eventType)
		if !stringSliceEqual(got, want) {
			t.Errorf("eventType %s: want %v, got %v", eventType, want, got)
		}
	}
	if got := eventTypeToThemes("totally_unknown"); got != nil {
		t.Errorf("unknown event type must return nil, got %v", got)
	}
}

func TestThemeMatchesAny(t *testing.T) {
	active := []string{"msci_rebalance", "earnings_surprise"}
	if !themeMatchesAny([]string{"msci_rebalance"}, active) {
		t.Error("single match should return true")
	}
	if !themeMatchesAny([]string{"nothing", "msci_rebalance"}, active) {
		t.Error("second-element match should return true")
	}
	if themeMatchesAny([]string{"unrelated"}, active) {
		t.Error("no match should return false")
	}
	if themeMatchesAny(nil, active) {
		t.Error("nil model themes must return false")
	}
	if themeMatchesAny([]string{"a"}, nil) {
		t.Error("nil active themes must return false")
	}
}

func dayOverlapsMSCIWindow(d, start, end time.Time) bool {
	return !d.Before(start) && !d.After(end)
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
