package eventdriven

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// fakePredictionStore is a test double for PredictionHistoryStore.
type fakePredictionStore struct {
	records     []PredictionRecord
	err         error
	appended    []PredictionRecord
	existsOn    map[string]bool // Taipei date key -> existence
	existsErr   error
	existsCalls int
}

func (f *fakePredictionStore) AppendPrediction(rec PredictionRecord) error {
	if f.err != nil {
		return f.err
	}
	f.appended = append(f.appended, rec)
	f.records = append(f.records, rec)
	return nil
}

func (f *fakePredictionStore) HasPredictionOn(t time.Time) (bool, error) {
	f.existsCalls++
	if f.existsErr != nil {
		return false, f.existsErr
	}
	if f.existsOn != nil {
		if v, ok := f.existsOn[t.Format("2006-01-02")]; ok {
			return v, nil
		}
	}
	return false, nil
}

func (f *fakePredictionStore) LoadRecentPredictions(limit int) ([]PredictionRecord, error) {
	if f.err != nil {
		return nil, f.err
	}
	if limit > 0 && len(f.records) > limit {
		return f.records[len(f.records)-limit:], nil
	}
	return f.records, nil
}

func reconTime() time.Time {
	t := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	return t
}

func TestComputeHistoricalHitRate_UnwiredStoreReturnsNil(t *testing.T) {
	h := NewHandler(industry.NewEventCalendar())
	if got := h.computeHistoricalHitRate(); got != nil {
		t.Fatalf("expected nil when store not wired, got %+v", got)
	}
}

func TestComputeHistoricalHitRate_CountsDirectionalHits(t *testing.T) {
	h := NewHandler(industry.NewEventCalendar())
	rec := reconTime()
	h.SetPredictionStore(&fakePredictionStore{records: []PredictionRecord{
		{DirectionSign: 0.5, ActualSign: 0.6, ActualCapturedAt: &rec},   // inflow hit
		{DirectionSign: -0.5, ActualSign: -0.4, ActualCapturedAt: &rec}, // outflow hit
		{DirectionSign: 0.5, ActualSign: -0.2, ActualCapturedAt: &rec},  // miss
		{DirectionSign: 0.5, ActualSign: 0},                             // unreconciled — skipped (ActualCapturedAt nil)
	}})

	got := h.computeHistoricalHitRate()
	if got == nil {
		t.Fatal("expected non-nil hit rate")
	}
	if got.Samples != 3 {
		t.Fatalf("expected 3 reconciled samples, got %d", got.Samples)
	}
	if got.Hits != 2 {
		t.Fatalf("expected 2 hits, got %d", got.Hits)
	}
	if got.HitRate != 2.0/3.0 {
		t.Fatalf("expected hit rate 2/3, got %v", got.HitRate)
	}
	if got.Calibrated {
		t.Fatal("expected not calibrated below MinHitSamples")
	}
	if got.Reason == "" {
		t.Fatal("expected calibrating reason when below MinHitSamples")
	}
}

// TestComputeHistoricalHitRate_ReconciledNeutralActualCountsAsSample guards
// the F3 fix: a realized neutral flow (ActualSign==0.0) with ActualCapturedAt
// set must count as a reconciled sample (a miss when prediction was
// directional), NOT be dropped as "unreconciled" (which would overstate the
// hit rate). ActualCapturedAt is the reconcile marker, never ActualSign.
func TestComputeHistoricalHitRate_ReconciledNeutralActualCountsAsSample(t *testing.T) {
	h := NewHandler(industry.NewEventCalendar())
	rec := reconTime()
	h.SetPredictionStore(&fakePredictionStore{records: []PredictionRecord{
		{DirectionSign: 0.5, ActualSign: 0.0, ActualCapturedAt: &rec}, // reconciled neutral actual → miss
	}})

	got := h.computeHistoricalHitRate()
	if got == nil {
		t.Fatal("expected non-nil hit rate")
	}
	if got.Samples != 1 {
		t.Fatalf("expected 1 reconciled sample (neutral actual), got %d", got.Samples)
	}
	if got.Hits != 0 {
		t.Fatalf("expected 0 hits for neutral actual, got %d", got.Hits)
	}
}

func TestComputeHistoricalHitRate_CalibratedAtEnoughSamples(t *testing.T) {
	h := NewHandler(industry.NewEventCalendar())
	rec := reconTime()
	records := make([]PredictionRecord, 0, MinHitSamples)
	for range MinHitSamples {
		records = append(records, PredictionRecord{DirectionSign: 0.5, ActualSign: 0.6, ActualCapturedAt: &rec})
	}
	h.SetPredictionStore(&fakePredictionStore{records: records})

	got := h.computeHistoricalHitRate()
	if got == nil {
		t.Fatal("expected non-nil hit rate")
	}
	if got.Samples != MinHitSamples {
		t.Fatalf("expected %d samples, got %d", MinHitSamples, got.Samples)
	}
	if !got.Calibrated {
		t.Fatal("expected calibrated at MinHitSamples")
	}
	if got.HitRate != 1.0 {
		t.Fatalf("expected hit rate 1.0, got %v", got.HitRate)
	}
}

func TestComputeHistoricalHitRate_ZeroSamples(t *testing.T) {
	h := NewHandler(industry.NewEventCalendar())
	h.SetPredictionStore(&fakePredictionStore{records: []PredictionRecord{
		{DirectionSign: 0.5, ActualSign: 0}, // no actual yet (ActualCapturedAt nil)
	}})

	got := h.computeHistoricalHitRate()
	if got == nil {
		t.Fatal("expected non-nil (calibrating) hit rate")
	}
	if got.Samples != 0 {
		t.Fatalf("expected 0 samples, got %d", got.Samples)
	}
	if got.Calibrated {
		t.Fatal("expected not calibrated with 0 samples")
	}
}

// TestPersistTodayPrediction_AppendsOnce guards the F1 production writer:
// first call appends the day-1 prediction; a second call for the same Taipei
// day (HasPredictionOn=true) must not append a duplicate.
func TestPersistTodayPrediction_AppendsOncePerDay(t *testing.T) {
	h := NewHandler(industry.NewEventCalendar())
	store := &fakePredictionStore{
		existsOn: map[string]bool{},
	}
	h.SetPredictionStore(store)

	report := PredictionReport{
		Predictions: []FlowPrediction{
			{Date: time.Date(2026, 8, 7, 5, 45, 0, 0, time.UTC), Direction: "inflow", Confidence: 0.6},
		},
	}

	// First call: no existing prediction → append.
	store.existsOn["2026-08-07"] = false
	h.persistTodayPrediction(report)
	if len(store.appended) != 1 {
		t.Fatalf("expected 1 append, got %d", len(store.appended))
	}
	if store.appended[0].DirectionSign != 0.6 {
		t.Fatalf("expected DirectionSign 0.6, got %v", store.appended[0].DirectionSign)
	}

	// Second call same day: HasPredictionOn=true → skip.
	store.existsOn["2026-08-07"] = true
	h.persistTodayPrediction(report)
	if len(store.appended) != 1 {
		t.Fatalf("expected still 1 append (daily once), got %d", len(store.appended))
	}
}

func TestPersistTodayPrediction_NoStoreIsNoop(t *testing.T) {
	h := NewHandler(industry.NewEventCalendar())
	report := PredictionReport{
		Predictions: []FlowPrediction{
			{Date: time.Date(2026, 8, 7, 5, 45, 0, 0, time.UTC), Direction: "inflow", Confidence: 0.6},
		},
	}
	h.persistTodayPrediction(report) // must not panic with nil store
}

func TestPredictEmptyCalendar(t *testing.T) {
	cal := industry.NewEventCalendar()
	p := NewPredictor(cal)
	now := time.Date(2025, 1, 15, 9, 0, 0, 0, time.UTC)
	report := p.Predict(now)

	if len(report.Predictions) != 5 {
		t.Fatalf("expected 5 predictions, got %d", len(report.Predictions))
	}
	for i, pred := range report.Predictions {
		expected := now.AddDate(0, 0, i+1)
		if !pred.Date.Equal(expected) {
			t.Errorf("day %d: expected %v, got %v", i+1, expected, pred.Date)
		}
	}
}

func TestPredictWithActiveEvents(t *testing.T) {
	cal := industry.NewEventCalendar()
	p := NewPredictor(cal)

	now := time.Date(2025, 12, 29, 9, 0, 0, 0, time.UTC)
	report := p.Predict(now)

	// Should generate predictions regardless of event presence
	if len(report.Predictions) != 5 {
		t.Fatalf("expected 5 predictions, got %d", len(report.Predictions))
	}

	// Report should have a summary
	if report.Summary == "" {
		t.Error("expected non-empty summary")
	}
	t.Logf("Summary: %s", report.Summary)
	t.Logf("Active events: %d", len(report.ActiveEvents))
}

func TestHandlerCalendar(t *testing.T) {
	cal := industry.NewEventCalendar()
	h := NewHandler(cal)

	req, _ := http.NewRequest(http.MethodGet, "/api/events/calendar", nil)
	code, data := h.HandleCalendar(req)

	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}

	resp, ok := data.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", data)
	}
	if _, ok := resp["events"]; !ok {
		t.Error("missing 'events' key")
	}
}

func TestHandlerPrediction(t *testing.T) {
	cal := industry.NewEventCalendar()
	h := NewHandler(cal)

	req, _ := http.NewRequest(http.MethodGet, "/api/events/prediction", nil)
	code, data := h.HandlePrediction(req)

	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}

	report, ok := data.(PredictionReport)
	if !ok {
		t.Fatalf("expected PredictionReport, got %T", data)
	}

	if len(report.Predictions) != 5 {
		t.Errorf("expected 5 predictions, got %d", len(report.Predictions))
	}
	if report.Summary == "" {
		t.Error("expected non-empty summary")
	}
	if report.GeneratedAt.IsZero() {
		t.Error("expected non-zero generated_at")
	}
}

func TestStaticCF(t *testing.T) {
	cf := &staticCF{score: 1.5, label: "strong_inflow"}
	if cf.QualityScore() != 1.5 {
		t.Errorf("expected 1.5, got %.2f", cf.QualityScore())
	}
	if cf.QualityLabel() != "strong_inflow" {
		t.Errorf("expected strong_inflow, got %s", cf.QualityLabel())
	}
}

func TestExpectedFlow(t *testing.T) {
	tests := []struct {
		eventType string
		want      string
	}{
		{string(industry.EventMSCIRebalance), "bullish"},
		{string(industry.EventTaiwan50Rebalance), "bullish"},
		{string(industry.EventMonthlyRevenue), "bullish"},
		{string(industry.EventFinancialReport), "bullish"},
		{string(industry.EventExDividend), "mixed"},
		{string(industry.EventFuturesSettlement), "mixed"},
		{string(industry.EventWindowDressing), "mixed"},
		{string(industry.EventSpringFestival), "neutral"},
	}
	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			got := expectedFlow(tt.eventType)
			if got != tt.want {
				t.Errorf("expectedFlow(%s) = %s, want %s", tt.eventType, got, tt.want)
			}
		})
	}
}

func TestSigmoidBounds(t *testing.T) {
	if sigmoid(0) != 0.5 {
		t.Errorf("sigmoid(0) = %f, want 0.5", sigmoid(0))
	}
	if sigmoid(100) != 1.0 {
		t.Errorf("sigmoid(100) = %f, want 1.0", sigmoid(100))
	}
	if sigmoid(-100) != 0.0 {
		t.Errorf("sigmoid(-100) = %f, want 0.0", sigmoid(-100))
	}
}

func TestRegisterRoutes_RegistersBothExpectedPaths(t *testing.T) {
	mux := http.NewServeMux()
	cal := industry.NewEventCalendar()
	RegisterRoutes(mux, cal)

	h1, _ := mux.Handler(mustNewRequest("GET", "/api/events/prediction", nil))
	if h1 == nil {
		t.Error("expected /api/events/prediction route to be registered")
	}
	h2, _ := mux.Handler(mustNewRequest("GET", "/api/events/calendar", nil))
	if h2 == nil {
		t.Error("expected /api/events/calendar route to be registered")
	}
}

func TestRegisterRoutes_UnregisteredPath_Returns404(t *testing.T) {
	mux := http.NewServeMux()
	cal := industry.NewEventCalendar()
	RegisterRoutes(mux, cal)

	req := mustNewRequest("GET", "/api/does-not-exist", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unregistered path, got %d body=%s",
			rr.Code, rr.Body.String())
	}
}

func TestRegisterRoutes_FullHTTPFlow_Prediction(t *testing.T) {
	mux := http.NewServeMux()
	cal := industry.NewEventCalendar()
	RegisterRoutes(mux, cal)

	req := mustNewRequest("GET", "/api/events/prediction", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var report PredictionReport
	if err := json.Unmarshal(rr.Body.Bytes(), &report); err != nil {
		t.Fatalf("response not valid PredictionReport JSON: %v body=%s", err, rr.Body.String())
	}
	if len(report.Predictions) != 5 {
		t.Errorf("expected 5 predictions in response, got %d", len(report.Predictions))
	}
	if report.SectorPredictions == nil {
		t.Error("SectorPredictions must not be nil in JSON response (always present)")
	}
	if report.Window != "5-day forward" {
		t.Errorf("Window = %q, want 5-day forward", report.Window)
	}
}

func TestRegisterRoutes_FullHTTPFlow_Calendar(t *testing.T) {
	mux := http.NewServeMux()
	cal := industry.NewEventCalendar()
	RegisterRoutes(mux, cal)

	req := mustNewRequest("GET", "/api/events/calendar", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("response not valid JSON: %v body=%s", err, rr.Body.String())
	}
	if _, ok := body["events"]; !ok {
		t.Error("response missing 'events' key")
	}
	if _, ok := body["total"]; !ok {
		t.Error("response missing 'total' key")
	}
}

func mustNewRequest(method, target string, body *int) *http.Request {
	r, err := http.NewRequest(method, target, nil)
	if err != nil {
		panic(err)
	}
	return r
}
