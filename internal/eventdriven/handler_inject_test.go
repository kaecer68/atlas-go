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

// injectTestNow pins both the calendar year and the handler clock for the three
// /api/events/prediction summary tests below.
//
// Why: those tests assert that the summary lists the driving events
// ("關鍵事件"). The calendar builds events from annual rules, so on a date that
// falls outside every window the clause legitimately disappears and the
// assertion fails — a wall-clock date bomb (same class as the 2026-08-01
// time-anchor and 2026-09-16 #1585 fixes). 2025-10-01 sits inside two annual
// windows (期貨結算日, 法說會旺季), so the assertion is stable on any run date.
var injectTestNow = time.Date(2025, 10, 1, 12, 0, 0, 0, time.UTC)

// dateBombE2EAnchor pins BOTH the calendar anchor (RefreshEvents) and the
// handler clock (SetNowFn) for TestE2E_EventTriggers_NonNeutralPrediction.
//
// Why a fixed anchor instead of time.Now(): that test asserts a direction
// verdict ("at least one non-neutral day"), which is a function of the event
// mix that happens to overlap the 5-day window on the run date. With
// RefreshEvents(time.Now()) plus a wall-clock handler the assertion was a date
// bomb: on 2026-10-06 the window resolves to 法說會旺季(bullish 0.6) +
// 期貨結算日(bearish 0.6) + 連假-國慶日(bearish 0.5) and 營收公布高峰(mixed 0.4
// ⇒ net 0), i.e. an event net of −0.5. The bullish baseline (0.8 × day-1 weight
// 0.7 = +0.56) cannot lift that over the ±0.3 direction threshold, so every day
// came back neutral and the test was red on a clean main. This is the surviving
// member of FU-20260930-07 (the 2026-08-01 time-anchor / 2026-09-16 #1585
// family): #2168 fixed four sibling tests in this file with a fixed clock plus a
// fixed calendar anchor and missed this one.
//
// 2025-03-20 is chosen so the prediction window (03-21..03-25) sits inside the
// 季底作帳行情 (bullish 0.8) and 期貨結算日 (bearish 0.6, a whole-month
// occurrence) windows, giving an event net of exactly +0.2 on all five days:
//   - +0.2 is below the ±0.3 threshold on its own, which is what lets the
//     zero-baseline control below assert "all five days neutral", and
//   - the eligible bullish baseline keeps every day above the threshold
//     (day-1: 0.8 × 0.7 = +0.56 ⇒ net +0.76; day-5: 0.8 × 0.358 = +0.29 ⇒ net
//     +0.49).
//
// The 2025-10-01 anchor used by the summary tests above is a poor host for a
// direction assertion: its window (10-02..10-06) is covered by the 中秋/國慶
// 連假 bearish pair from 10-03 on (法說會旺季 +0.6 − 期貨結算日 −0.6 − 連假
// −0.5 ⇒ event net −0.5), so only day 1 — which does not overlap a 連假 yet —
// clears the threshold (observed: day 1 inflow, days 2-5 neutral). That single
// surviving day depends on where the lunar 中秋 window opens, which is not a
// margin worth re-arming a date bomb with. 2025-03-20 has the same +0.2 event
// net on all five days instead.
var dateBombE2EAnchor = time.Date(2025, 3, 20, 12, 0, 0, 0, time.UTC)

// pinnedPredictionReport issues GET /api/events/prediction against a handler
// whose clock is pinned to now, and returns the decoded report. cf may be nil,
// in which case the predictor keeps its default zero-baseline staticCF.
func pinnedPredictionReport(t *testing.T, cal *industry.EventCalendar, cf CapitalFlowProvider, now time.Time) PredictionReport {
	t.Helper()

	mux := http.NewServeMux()
	h := RegisterRoutesWithDetectors(mux, cal, cf, nil, nil)
	h.SetNowFn(func() time.Time { return now })

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
	return report
}

func newTestHandler() *Handler {
	cal := industry.NewEventCalendar()
	cal.RefreshEvents(time.Now())
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
	cal.RefreshEvents(injectTestNow)

	h := RegisterRoutesWithDetectors(mux, cal, nil, nil, nil)
	h.SetNowFn(func() time.Time { return injectTestNow })

	req := httptest.NewRequest(http.MethodGet, "/api/events/prediction", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	body := rec.Body.String()
	// #1384 calibration-aware baseline: default staticCF uses
	// staticCF{score: 0, label: "neutral"} whose LatestAssessment is hardwired
	// to CalibrationCalibrating, so the summary must surface the calibrating
	// note instead of an inflow tilt. Date-bomb fix (2026-09-16, #1585
	// precedent): the direction verdict (分歧 vs 偏流入/偏流出) depends on
	// the calendar event mix at the run date — calendar revisions changed it
	// from the original symmetric mix. Hermetic invariants: calibration
	// surfaced, key events listed, NO fabricated baseline-drift note (cfScore=0).
	mustContain := []string{"校準中", "關鍵事件"}
	for _, s := range mustContain {
		if !strings.Contains(body, s) {
			t.Errorf("default staticCF summary missing %q, body=%s", s, body)
		}
	}
	if strings.Contains(body, "當前資金品質偏多") || strings.Contains(body, "當前資金品質偏空") {
		t.Errorf("default staticCF has zero baseline; baseline drift note must not appear, body=%s", body)
	}
}

func TestRegisterRoutesWithCapitalFlow_BearishTilt(t *testing.T) {
	mux := http.NewServeMux()
	cal := industry.NewEventCalendar()
	cal.RefreshEvents(time.Now())

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
	cal.RefreshEvents(injectTestNow)

	h := RegisterRoutesWithDetectors(mux, cal, &stubCF{score: 0.9, label: "bullish"}, nil, nil)
	h.SetNowFn(func() time.Time { return injectTestNow })

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
	// calibrating note. Date-bomb fix (2026-09-16, #1585 precedent): direction
	// verdict claims removed — they depended on the calendar event mix at the
	// run date. Hermetic invariants: positive drift surfaced, calibrating note
	// must NOT leak, key events listed.
	mustContain := []string{"當前資金品質偏多", "關鍵事件"}
	for _, s := range mustContain {
		if !strings.Contains(body, s) {
			t.Errorf("bullish cf summary missing %q, body=%s", s, body)
		}
	}
	if strings.Contains(body, "校準中") {
		t.Errorf("eligible bullish cf must not surface calibrating note, body=%s", body)
	}
}

func TestRegisterRoutesWithCapitalFlow_NilProviderFallsBack(t *testing.T) {
	mux := http.NewServeMux()
	cal := industry.NewEventCalendar()
	cal.RefreshEvents(injectTestNow)

	h := RegisterRoutesWithDetectors(mux, cal, nil, nil, nil)
	h.SetNowFn(func() time.Time { return injectTestNow })

	req := httptest.NewRequest(http.MethodGet, "/api/events/prediction", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	body := rec.Body.String()
	// #1384 calibration-aware baseline: a nil cf keeps the predictor's default
	// staticCF{score: 0, label: "neutral"} which hardwires CalibrationCalibrating.
	// Date-bomb fix (2026-09-16, #1585 precedent): direction verdict claims
	// removed (calendar-mix dependent). Hermetic invariants: calibrating note
	// surfaced, key events listed, no baseline drift note (cfScore=0).
	mustContain := []string{"校準中", "關鍵事件"}
	for _, s := range mustContain {
		if !strings.Contains(body, s) {
			t.Errorf("nil cf summary missing %q, body=%s", s, body)
		}
	}
	if strings.Contains(body, "當前資金品質偏多") || strings.Contains(body, "當前資金品質偏空") {
		t.Errorf("nil cf baseline is zero; baseline drift note must not appear, body=%s", body)
	}
}

// TestE2E_EventTriggers_NonNeutralPrediction verifies the 5-day prediction
// contains at least one non-neutral direction when the calendar has active
// events and a strong bullish capital flow provider is wired in. Locks in
// the Stage 5 end-to-end pipeline: EventCalendar → RefreshEvents →
// Predictor → HTTP /api/events/prediction → JSON FlowPrediction[].Direction.
//
// fix/20261006-eventdriven-date-bomb — the test is now hermetic: both the
// calendar anchor (RefreshEvents) and the handler clock (SetNowFn) are pinned
// to dateBombE2EAnchor, so no wall clock can reach the assertion. See the
// dateBombE2EAnchor docstring for the 2026-10-06 failure that made this
// necessary (surviving member of FU-20260930-07) and for the window arithmetic.
//
// Two earlier fragilities stay fixed here (test side only, no production change
// — kept from fix/20260801-eventdriven-test-timeanchor):
//
//  1. RefreshEvents was once hard-coded to a different date than the window
//     computed at time.Now(), which made the test silently assume "now is
//     7/12". Anchor and clock are now the same pinned instant.
//
//  2. The bullish stubCF score stays 1.5 (the value fix/20260801 chose over
//     0.9). Predictor baseline scaling (scaleQualityScoreToBaseline,
//     predictor.go:435; divisor 1.5 at predictor.go:426) maps QualityScore
//     (~[-3,3]) to [-0.8, 0.8], so score=1.5 saturates the scale at baseline
//     0.8. Saturation keeps the anchored days well clear of the ±0.3 direction
//     threshold (day-1 net = 0.8 × 0.7 + 0.2 = +0.76; day-5 net = 0.8 × 0.358
//     + 0.2 = +0.49), instead of leaving the assertion sitting on the boundary.
//     The semantic meaning of the test ("strong bullish CF + active events → at
//     least one non-neutral day") is preserved; the knob is not weakened.
func TestE2E_EventTriggers_NonNeutralPrediction(t *testing.T) {
	cal := industry.NewEventCalendar()
	cal.RefreshEvents(dateBombE2EAnchor)

	// Strong bullish CF provider amplifies event-driven signals into
	// inflow-tilted predictions. score=1.5 saturates the baseline scaler
	// (predictor.go:426) so the anchored days exceed the ±0.3 threshold; see
	// the test docstring and dateBombE2EAnchor for the derivation.
	report := pinnedPredictionReport(t, cal, &stubCF{score: 1.5, label: "strong_inflow"}, dateBombE2EAnchor)

	if len(report.Predictions) != 5 {
		t.Fatalf("expected 5 daily predictions, got %d", len(report.Predictions))
	}

	// At least one of the 5 days must tilt non-neutral, and the tilt must be
	// inflow (the wired baseline is bullish). With a strong bullish CF
	// (score=1.5 → baseline 0.8 eligible) plus the anchored events this holds
	// on all five days; if the pipeline falls back to all-neutral the
	// prediction path is broken.
	nonNeutral, inflow := 0, 0
	for i, p := range report.Predictions {
		if p.Direction != "neutral" {
			nonNeutral++
			if p.Direction == "inflow" {
				inflow++
			}
			t.Logf("day %d (%s): direction=%s confidence=%.2f drivers=%v",
				i+1, p.Date.Format("2006-01-02"), p.Direction, p.Confidence, p.DrivingEvents)
		}
	}
	if nonNeutral == 0 {
		t.Errorf("expected at least 1 non-neutral prediction among 5 days, got 0 (all neutral — pipeline broken)")
	}
	if inflow == 0 {
		t.Errorf("strong bullish baseline (score=1.5 → baseline 0.8, calibration eligible) plus event net +0.2 must yield at least one inflow day; got 0 inflow (%d non-neutral)", nonNeutral)
	}

	// Anchor/clock alignment guard. Every day must carry at least one calendar
	// event driver next to the baseline driver (measured: all five anchored days
	// carry [當前資金流向, 季底作帳行情, 期貨結算日]). A list of length 1 can only
	// be the baseline driver, because calendar drivers are appended only for
	// events that overlap the day. So if the calendar were ever refreshed
	// against a different instant than the pinned clock — the exact mismatch
	// that produced the date bomb — the window would no longer overlap any
	// generated event and every day would be baseline-only, and this guard
	// fails instead of the test silently degrading to "baseline only".
	for i, p := range report.Predictions {
		if len(p.DrivingEvents) < 2 {
			t.Errorf("day %d (%s): expected the pinned window to overlap calendar events (baseline driver + ≥1 event), got drivers=%v — anchor/clock misalignment (date bomb regression)", i+1, p.Date.Format("2006-01-02"), p.DrivingEvents)
		}
	}

	// Mutation self-proof, tight half: same calendar, same pinned clock, only
	// the capital-flow baseline removed (score=0 → baseline 0). The anchored
	// event net is +0.2 on every day, below the ±0.3 threshold, so every day
	// must now be neutral. If this run is not all-neutral the assertion above
	// would be satisfied by the calendar alone and the test would be vacuous.
	zeroReport := pinnedPredictionReport(t, cal, &stubCF{score: 0, label: "neutral"}, dateBombE2EAnchor)
	if len(zeroReport.Predictions) != 5 {
		t.Fatalf("control run: expected 5 daily predictions, got %d", len(zeroReport.Predictions))
	}
	for i, p := range zeroReport.Predictions {
		if p.Direction != "neutral" {
			t.Errorf("control run (zero capital-flow baseline, same calendar): day %d (%s) direction=%s confidence=%.2f drivers=%v — the anchored event net (+0.2) must stay under the ±0.3 threshold, otherwise the non-neutral assertion above proves nothing",
				i+1, p.Date.Format("2006-01-02"), p.Direction, p.Confidence, p.DrivingEvents)
		}
	}
}

// TestE2E_EventTriggers_ZeroInputsAllNeutral is the deliberate negative control
// for TestE2E_EventTriggers_NonNeutralPrediction: it proves that the
// "at least 1 non-neutral prediction" assertion is able to fail, i.e. that the
// positive test is a real check and not a claim that is true by construction
// (mutation self-proof, loose half).
//
// Both inputs are removed by construction rather than by calendar-rule
// coincidence, so the control cannot rot when event rules are revised:
//   - the calendar is never RefreshEvents'd, so it holds zero events (no window
//     can overlap anything), and
//   - cf is nil, so the predictor keeps its default staticCF{score: 0} baseline
//     (net weight 0 on every day).
//
// The clock is still pinned to dateBombE2EAnchor so the expected dates are
// fixed as well. Cf. TestE2E_MissingData_Fallback, which covers the same nil
// inputs but asserts only the response shape, not the neutral direction.
func TestE2E_EventTriggers_ZeroInputsAllNeutral(t *testing.T) {
	cal := industry.NewEventCalendar() // intentionally no RefreshEvents

	report := pinnedPredictionReport(t, cal, nil, dateBombE2EAnchor)

	if len(report.Predictions) != 5 {
		t.Fatalf("expected 5 daily predictions, got %d", len(report.Predictions))
	}

	nonNeutral := 0
	for i, p := range report.Predictions {
		if !p.Date.Equal(dateBombE2EAnchor.AddDate(0, 0, i+1)) {
			t.Errorf("day %d: pinned clock must place the window at %s, got %s",
				i+1, dateBombE2EAnchor.AddDate(0, 0, i+1).Format("2006-01-02"), p.Date.Format("2006-01-02"))
		}
		if p.Direction != "neutral" {
			nonNeutral++
			t.Errorf("day %d (%s): no events and no baseline ⇒ direction must be neutral, got %s (confidence=%.2f drivers=%v)",
				i+1, p.Date.Format("2006-01-02"), p.Direction, p.Confidence, p.DrivingEvents)
		}
		if len(p.DrivingEvents) != 0 {
			t.Errorf("day %d (%s): zero inputs ⇒ no drivers may be fabricated, got %v",
				i+1, p.Date.Format("2006-01-02"), p.DrivingEvents)
		}
	}
	if nonNeutral != 0 {
		t.Fatalf("%d non-neutral day(s) with zero inputs — the neutral branch is unreachable, so TestE2E_EventTriggers_NonNeutralPrediction cannot fail and proves nothing", nonNeutral)
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
