package eventdriven

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
)

type stubCF struct {
	score float64
	label string
}

func (s *stubCF) QualityScore() float64 { return s.score }
func (s *stubCF) QualityLabel() string  { return s.label }

func newTestHandler() *Handler {
	cal := industry.NewEventCalendar()
	cal.RefreshEvents(time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC))
	return NewHandler(cal)
}

func TestNewHandler_DefaultsToStaticCF(t *testing.T) {
	h := newTestHandler()
	if got := h.predictor.capitalFlow.QualityScore(); got != 0 {
		t.Errorf("default QualityScore: want 0, got %v", got)
	}
	if got := h.predictor.capitalFlow.QualityLabel(); got != "neutral" {
		t.Errorf("default QualityLabel: want neutral, got %q", got)
	}
}

func TestHandler_SetCapitalFlow_OverridesProvider(t *testing.T) {
	h := newTestHandler()
	h.SetCapitalFlow(&stubCF{score: 0.75, label: "bullish"})

	if got := h.predictor.capitalFlow.QualityScore(); got != 0.75 {
		t.Errorf("after SetCapitalFlow: QualityScore want 0.75, got %v", got)
	}
	if got := h.predictor.capitalFlow.QualityLabel(); got != "bullish" {
		t.Errorf("after SetCapitalFlow: QualityLabel want bullish, got %q", got)
	}
}

func TestRegisterRoutes_UsesDefaultStaticCF(t *testing.T) {
	mux := http.NewServeMux()
	cal := industry.NewEventCalendar()
	cal.RefreshEvents(time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC))

	RegisterRoutes(mux, cal)

	req := httptest.NewRequest(http.MethodGet, "/api/events/prediction", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); !strings.Contains(got, "偏流入") {
		t.Errorf("default staticCF should yield inflow-dominant summary, body=%s", got)
	}
}

func TestRegisterRoutesWithCapitalFlow_BearishTilt(t *testing.T) {
	mux := http.NewServeMux()
	cal := industry.NewEventCalendar()
	cal.RefreshEvents(time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC))

	RegisterRoutesWithCapitalFlow(mux, cal, &stubCF{score: -0.5, label: "bearish"})

	req := httptest.NewRequest(http.MethodGet, "/api/events/prediction", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "分歧") && !strings.Contains(body, "流出") {
		t.Errorf("bearish cf should tilt summary toward 分歧/流出, body=%s", body)
	}
}

func TestRegisterRoutesWithCapitalFlow_BullishTilt(t *testing.T) {
	mux := http.NewServeMux()
	cal := industry.NewEventCalendar()
	cal.RefreshEvents(time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC))

	RegisterRoutesWithCapitalFlow(mux, cal, &stubCF{score: 0.9, label: "bullish"})

	req := httptest.NewRequest(http.MethodGet, "/api/events/prediction", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); !strings.Contains(got, "偏流入") {
		t.Errorf("bullish cf should yield inflow-dominant summary, body=%s", got)
	}
}

func TestRegisterRoutesWithCapitalFlow_NilProviderFallsBack(t *testing.T) {
	mux := http.NewServeMux()
	cal := industry.NewEventCalendar()
	cal.RefreshEvents(time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC))

	RegisterRoutesWithCapitalFlow(mux, cal, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/events/prediction", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); !strings.Contains(got, "偏流入") {
		t.Errorf("nil cf should fall back to staticCF baseline (inflow-dominant), body=%s", got)
	}
}
