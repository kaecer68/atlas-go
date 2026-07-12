package eventdriven

import (
	"net/http"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
)

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

// Stage 1 PR#1 整合測試：wired factory → handler → 非空 calendar 回傳。
func TestHandlerCalendar_WithWiredFactory(t *testing.T) {
	cal := industry.NewEventCalendarWithProvider(nil)
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
	total, _ := resp["total"].(int)
	if total == 0 {
		t.Fatalf("wired factory must yield non-empty calendar (Stage 1 PR#1 root cause: %v)", resp)
	}
	t.Logf("wired factory + handler yielded %d events", total)
}
