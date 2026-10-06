package eventdriven

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// These tests pin the Phase 0 advisory contract for /api/events/prediction
// (2026-10-06 owner decision): the payload must disclose that it is withdrawn
// and must name the mechanisms that made it abstain. Two halves are covered:
//
//   - the withdrawal itself is unconditional (a directional report is still
//     not advisory-usable), and
//   - the abstention reasons are derived per request, never hard-coded.

// allNeutralReport builds a report whose days all stayed inside the neutral band.
func allNeutralReport() PredictionReport {
	preds := make([]FlowPrediction, forecastDays)
	for i := range preds {
		preds[i] = FlowPrediction{Direction: "neutral", Confidence: 0.5}
	}
	return PredictionReport{Predictions: preds}
}

// directionalReport is the mutation control for the all-neutral branch: one day
// cleared the band, so no abstention mechanism describes this report.
func directionalReport() PredictionReport {
	report := allNeutralReport()
	report.Predictions[0] = FlowPrediction{Direction: "inflow", Confidence: 0.8}
	return report
}

func TestAdvisoryStatus_WithdrawnIsUnconditional(t *testing.T) {
	p := NewPredictor(industry.NewEventCalendar())
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

	for name, report := range map[string]PredictionReport{
		"all-neutral": allNeutralReport(),
		"directional": directionalReport(),
	} {
		got := p.advisoryStatus(report, nil, now, 0, capitalflow.CalibrationEligible)
		if got == nil {
			t.Fatalf("%s: advisory status must never be nil", name)
		}
		if got.AdvisoryUsable {
			t.Errorf("%s: AdvisoryUsable must stay false while the family has not passed G2/G3/G4'", name)
		}
		if got.Status != AdvisoryStatusWithdrawn {
			t.Errorf("%s: Status = %q, want %q", name, got.Status, AdvisoryStatusWithdrawn)
		}
		if got.Message == "" {
			t.Errorf("%s: Message must carry the consumer-facing warning", name)
		}
		if got.EvidenceRef == "" {
			t.Errorf("%s: EvidenceRef must point at the recorded evidence", name)
		}
		if got.AbstentionReasons == nil {
			t.Errorf("%s: AbstentionReasons must be an empty slice, not nil (stable JSON shape)", name)
		}
	}

	// The directional control must not claim any abstention mechanism: those
	// reasons describe the neutral branch only.
	if got := p.advisoryStatus(directionalReport(), nil, now, 0, capitalflow.CalibrationEligible); len(got.AbstentionReasons) != 0 {
		t.Errorf("directional report: AbstentionReasons = %v, want none", got.AbstentionReasons)
	}
}

func TestAdvisoryStatus_ConstructiveAbstentionReasons(t *testing.T) {
	p := NewPredictor(industry.NewEventCalendar())
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	timeline := []industry.CalendarEvent{{
		Name:      "除權息旺季",
		Direction: "mixed",
		StartDate: now.AddDate(0, 0, 1),
		EndDate:   now.AddDate(0, 0, 3),
	}}

	got := p.advisoryStatus(allNeutralReport(), timeline, now, 0, capitalflow.CalibrationCalibrating)

	want := []string{
		AdvisoryReasonCalibrationDiscountBelowThreshold,
		AdvisoryReasonMixedEventCancellation,
		AdvisoryReasonNoEdgeEvidence,
	}
	if len(got.AbstentionReasons) != len(want) {
		t.Fatalf("AbstentionReasons = %v, want %v", got.AbstentionReasons, want)
	}
	for i := range want {
		if got.AbstentionReasons[i] != want[i] {
			t.Fatalf("AbstentionReasons = %v, want %v (sorted, one per mechanism)", got.AbstentionReasons, want)
		}
	}

	// An eligible baseline described by a non-zero score must not report the
	// calibration-discount reason: the discount is the cause, not the level.
	noDiscount := p.advisoryStatus(allNeutralReport(), nil, now, 0.8, capitalflow.CalibrationEligible)
	for _, reason := range noDiscount.AbstentionReasons {
		if reason == AdvisoryReasonCalibrationDiscountBelowThreshold {
			t.Errorf("eligible baseline: %q must not be reported (the discount is what caps it)", reason)
		}
	}
}

// TestCalibratingBaselineCeilingBelowNeutralBand is the arithmetic behind
// AdvisoryReasonCalibrationDiscountBelowThreshold: while the capital-flow
// baseline is not eligible, its day-1 contribution cannot reach the neutral
// band, so the baseline alone can never set a direction. The eligible control
// shows the discount — not the baseline level — is what keeps it under.
func TestCalibratingBaselineCeilingBelowNeutralBand(t *testing.T) {
	for _, qs := range []float64{-5, -1.5, -1, 0, 1, 1.5, 5} {
		baseline := scaleQualityScoreToBaseline(qs)
		got := math.Abs(baseline) * baselineWeightForDay(0, capitalflow.CalibrationCalibrating)
		if got >= neutralBand {
			t.Errorf("QualityScore %v: calibrating day-1 contribution %.4f must stay below the neutral band %.2f", qs, got, neutralBand)
		}
	}

	saturated := scaleQualityScoreToBaseline(5)
	if got := math.Abs(saturated) * baselineWeightForDay(0, capitalflow.CalibrationEligible); got <= neutralBand {
		t.Errorf("eligible day-1 contribution %.4f must exceed the neutral band %.2f (otherwise the calibrating ceiling above proves nothing)", got, neutralBand)
	}
}

func TestMixedEventsInWindow(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	timeline := []industry.CalendarEvent{
		{Name: "窗內 mixed A", Direction: "mixed", StartDate: now.AddDate(0, 0, 1), EndDate: now.AddDate(0, 0, 2)},
		{Name: "窗內 mixed B", Direction: "mixed", StartDate: now.AddDate(0, 0, 5), EndDate: now.AddDate(0, 0, 5)},
		{Name: "窗外 mixed", Direction: "mixed", StartDate: now.AddDate(0, 0, 8), EndDate: now.AddDate(0, 0, 9)},
		{Name: "窗內 bullish", Direction: "bullish", StartDate: now.AddDate(0, 0, 1), EndDate: now.AddDate(0, 0, 2)},
	}
	if got := mixedEventsInWindow(timeline, now); got != 2 {
		t.Errorf("mixedEventsInWindow = %d, want 2 (mixed events overlapping days 1..5 only)", got)
	}
	if got := mixedEventsInWindow(nil, now); got != 0 {
		t.Errorf("mixedEventsInWindow(nil) = %d, want 0", got)
	}
}

// TestE2E_PredictionReport_AdvisoryStatusInJSON guards the wire contract that
// non-Go consumers (web UI, MCP clients, agents) read: the withdrawal must be
// visible in the JSON body, and it must be additive — the endpoint keeps
// returning the 5-day report instead of being removed or emptied.
func TestE2E_PredictionReport_AdvisoryStatusInJSON(t *testing.T) {
	// Zero inputs (no RefreshEvents, nil CF) ⇒ every day is neutral, which is
	// the production shape this contract exists for.
	cal := industry.NewEventCalendar()
	mux := http.NewServeMux()
	h := RegisterRoutesWithDetectors(mux, cal, nil, nil, nil)
	h.SetNowFn(func() time.Time { return dateBombE2EAnchor })

	req := httptest.NewRequest(http.MethodGet, "/api/events/prediction", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 (endpoint must stay, only disclosed), got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"advisory_usable":false`) {
		t.Errorf("response body must carry the machine-readable withdrawal; body=%s", body)
	}
	if !strings.Contains(body, `"advisory_status"`) {
		t.Errorf("response body must carry advisory_status; body=%s", body)
	}

	var report PredictionReport
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&report); err != nil {
		t.Fatalf("decode PredictionReport: %v", err)
	}
	if len(report.Predictions) != forecastDays {
		t.Fatalf("predictions = %d, want %d (the endpoint must not be emptied)", len(report.Predictions), forecastDays)
	}
	if report.AdvisoryStatus == nil {
		t.Fatal("AdvisoryStatus must be populated by Predict")
	}
	if report.AdvisoryStatus.AdvisoryUsable {
		t.Error("AdvisoryUsable must be false")
	}
	if !slices.Contains(report.AdvisoryStatus.AbstentionReasons, AdvisoryReasonNoEdgeEvidence) {
		t.Errorf("AbstentionReasons = %v, want %q for an all-neutral report", report.AdvisoryStatus.AbstentionReasons, AdvisoryReasonNoEdgeEvidence)
	}
	if !slices.Contains(report.AdvisoryStatus.AbstentionReasons, AdvisoryReasonCalibrationDiscountBelowThreshold) {
		t.Errorf("AbstentionReasons = %v, want %q (default staticCF is calibrating with a zero baseline)", report.AdvisoryStatus.AbstentionReasons, AdvisoryReasonCalibrationDiscountBelowThreshold)
	}
}
