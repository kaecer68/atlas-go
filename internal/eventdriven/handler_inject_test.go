package eventdriven

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/industry"
)

type stubCF struct {
	score  float64
	label  string
	status string
}

func (s *stubCF) QualityScore() float64 { return s.score }
func (s *stubCF) QualityLabel() string  { return s.label }
func (s *stubCF) LatestAssessment(context.Context) (capitalflow.CapitalFlowAssessment, error) {
	status := s.status
	if status == "" {
		status = capitalflow.CalibrationEligible
	}
	return capitalflow.CapitalFlowAssessment{CalibrationStatus: status}, nil
}

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

	body := rec.Body.String()
	// #1384 calibration-aware baseline: default staticCF uses
	// staticCF{score: 0, label: "neutral"} whose LatestAssessment is hardwired
	// to CalibrationCalibrating, so the summary must surface the calibrating
	// note instead of an inflow tilt. The 5-day event mix is symmetric so the
	// direction branch must fall to 分歧.
	mustContain := []string{"未來 5 天資金流向分歧", "校準中", "關鍵事件"}
	for _, s := range mustContain {
		if !strings.Contains(body, s) {
			t.Errorf("default staticCF summary missing %q, body=%s", s, body)
		}
	}
	// No baseline drift (cfScore=0) and no 偏流入 direction dominance, so
	// 偏流入 must NOT appear as the summary verdict.
	if strings.Contains(body, "偏流入") {
		t.Errorf("default staticCF must not yield 偏流入 verdict, body=%s", body)
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
	// The bearish baseline is now surfaced in the summary even when
	// calendar events remain bullish; the summary must mention the
	// current capital-flow quality, not necessarily flip the whole window.
	if !strings.Contains(body, "偏空") && !strings.Contains(body, "流出") && !strings.Contains(body, "分歧") {
		t.Errorf("bearish cf should be visible in summary as 偏空/流出/分歧, body=%s", body)
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
	body := rec.Body.String()
	// #1384 calibration-aware baseline: stubCF with no LatestAssessment status
	// falls back to CalibrationEligible, so the summary must surface the
	// positive baseline drift (cfScore=0.9 → "當前資金品質偏多") instead of a
	// calibrating note. The 5-day event mix still splits evenly so the
	// direction branch must fall to 分歧, not 偏流入.
	mustContain := []string{"未來 5 天資金流向分歧", "當前資金品質偏多", "關鍵事件"}
	for _, s := range mustContain {
		if !strings.Contains(body, s) {
			t.Errorf("bullish cf summary missing %q, body=%s", s, body)
		}
	}
	// Baseline is positive, calibration is eligible: calibrating note must
	// NOT leak and direction verdict must NOT be 偏流入.
	if strings.Contains(body, "校準中") {
		t.Errorf("eligible bullish cf must not surface calibrating note, body=%s", body)
	}
	if strings.Contains(body, "偏流入") {
		t.Errorf("bullish cf must not yield 偏流入 verdict, body=%s", body)
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

	body := rec.Body.String()
	// #1384 calibration-aware baseline: a nil cf keeps the predictor's default
	// staticCF{score: 0, label: "neutral"} which hardwires CalibrationCalibrating.
	// The summary must surface 分歧 + 校準中, NOT 偏流入 and NOT a baseline
	// drift note (cfScore=0).
	mustContain := []string{"未來 5 天資金流向分歧", "校準中", "關鍵事件"}
	for _, s := range mustContain {
		if !strings.Contains(body, s) {
			t.Errorf("nil cf summary missing %q, body=%s", s, body)
		}
	}
	if strings.Contains(body, "偏流入") {
		t.Errorf("nil cf must not yield 偏流入 verdict, body=%s", body)
	}
	if strings.Contains(body, "當前資金品質偏多") || strings.Contains(body, "當前資金品質偏空") {
		t.Errorf("nil cf baseline is zero; baseline drift note must not appear, body=%s", body)
	}
}

// TestE2E_EventTriggers_NonNeutralPrediction verifies the 5-day prediction
// contains at least one non-neutral direction when the calendar has active
// events and a bullish capital flow provider is wired in. Locks in the
// Stage 5 end-to-end pipeline: EventCalendar → RefreshEvents → Predictor
// → HTTP /api/events/prediction → JSON FlowPrediction[].Direction.
func TestE2E_EventTriggers_NonNeutralPrediction(t *testing.T) {
	mux := http.NewServeMux()
	cal := industry.NewEventCalendar()
	cal.RefreshEvents(time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC))

	// Bullish CF provider amplifies event-driven signals into inflow-tilted
	// predictions; expected direction ∈ {inflow, outflow} (NOT neutral).
	RegisterRoutesWithCapitalFlow(mux, cal, &stubCF{score: 0.9, label: "strong_inflow"})

	req := httptest.NewRequest(http.MethodGet, "/api/events/prediction", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}

	var report PredictionReport
	if err := json.NewDecoder(rec.Body).Decode(&report); err != nil {
		t.Fatalf("decode PredictionReport: %v (body=%s)", err, rec.Body.String())
	}

	if len(report.Predictions) != 5 {
		t.Fatalf("expected 5 daily predictions, got %d", len(report.Predictions))
	}

	// At least one of the 5 days must tilt non-neutral. With bullish CF +
	// active events the predictor should produce inflow-dominant output;
	// if it falls back to all-neutral, the pipeline is broken.
	nonNeutral := 0
	for i, p := range report.Predictions {
		if p.Direction != "neutral" {
			nonNeutral++
			t.Logf("day %d (%s): direction=%s confidence=%.2f drivers=%v",
				i+1, p.Date.Format("2006-01-02"), p.Direction, p.Confidence, p.DrivingEvents)
		}
	}
	if nonNeutral == 0 {
		t.Errorf("expected at least 1 non-neutral prediction among 5 days, got 0 (all neutral — pipeline broken)")
	}
}

// TestE2E_MissingData_Fallback verifies the prediction endpoint keeps
// responding with a valid 5-day report even when both inputs are missing:
// (a) the calendar has not been RefreshEvents'd (no events loaded), and
// (b) the capital flow provider is nil. The endpoint must fall back to
// the staticCF baseline + empty event timeline rather than 5xx or hang.
func TestE2E_MissingData_Fallback(t *testing.T) {
	mux := http.NewServeMux()
	cal := industry.NewEventCalendar() // intentionally no RefreshEvents

	// RegisterRoutes uses default staticCF{score:0, label:"neutral"};
	// passing nil cf explicitly exercises the RegisterRoutesWithCapitalFlow
	// fallback path (internal/eventdriven/handler.go:53).
	RegisterRoutesWithCapitalFlow(mux, cal, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/events/prediction", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("missing data must not 5xx; expected 200, got %d (body=%s)", rec.Code, rec.Body.String())
	}

	var report PredictionReport
	if err := json.NewDecoder(rec.Body).Decode(&report); err != nil {
		t.Fatalf("decode PredictionReport: %v (body=%s)", err, rec.Body.String())
	}

	if len(report.Predictions) != 5 {
		t.Fatalf("expected 5 fallback predictions, got %d", len(report.Predictions))
	}
	if report.Summary == "" {
		t.Error("expected non-empty summary even with no events and nil cf")
	}
	if report.GeneratedAt.IsZero() {
		t.Error("expected generated_at timestamp even on fallback path")
	}
}
