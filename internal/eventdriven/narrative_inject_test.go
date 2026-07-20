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

func TestSetNarrativeProvider_CachesSnapshot(t *testing.T) {
	models := []ModelView{
		{ID: "x", Name: "test", Weight: 0.5, Direction: "bullish", ActiveThemes: []string{"earnings_surprise"}},
	}
	h := newTestHandler()
	h.SetNarrativeProvider(&stubNarrative{models: models})

	if len(h.predictor.narrativeModels) != 1 {
		t.Fatalf("narrativeModels should cache 1 entry, got %d", len(h.predictor.narrativeModels))
	}
	if h.predictor.narrativeModels[0].ID != "x" {
		t.Errorf("cached model ID mismatch: %s", h.predictor.narrativeModels[0].ID)
	}
}

func TestSetNarrativeProvider_NilProviderClearsCache(t *testing.T) {
	h := newTestHandler()
	h.SetNarrativeProvider(&stubNarrative{models: []ModelView{{ID: "x", Weight: 0.5}}})
	if len(h.predictor.narrativeModels) != 1 {
		t.Fatalf("expected 1 cached model, got %d", len(h.predictor.narrativeModels))
	}
	h.SetNarrativeProvider(nil)
	if h.predictor.narrativeModels != nil {
		t.Errorf("nil provider must clear cache, got %v", h.predictor.narrativeModels)
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

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); !strings.Contains(got, "偏流入") {
		t.Errorf("strong bull narrative should yield inflow-dominant summary, body=%s", got)
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
