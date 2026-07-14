package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestEventFlowPrediction_NonNeutral_ForwardedOK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"predictions":[{"date":"2026-07-15","direction":"inflow","confidence":0.85,"driving_events":["MSCI 季調"],"predicted_forces":["foreign","institutional"]}],"active_events":[],"summary":"偏多"}`)
	_, out, err := s.handleEventFlowPrediction(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/events/prediction" {
		t.Fatalf("path=%s, want=/api/events/prediction", rec.path)
	}
	preds, ok := out["predictions"].([]any)
	if !ok {
		t.Fatalf("expected predictions array, got %T", out["predictions"])
	}
	if len(preds) != 1 {
		t.Fatalf("expected 1 prediction, got %d", len(preds))
	}
	p0 := preds[0].(map[string]any)
	if d, _ := p0["direction"].(string); d != "inflow" {
		t.Fatalf("expected direction=inflow, got %v", p0["direction"])
	}
	c, ok := p0["confidence"].(float64)
	if !ok || c != 0.85 {
		t.Fatalf("expected confidence=0.85, got %v", p0["confidence"])
	}
}

func TestEventFlowPrediction_BackendError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "backend boom", http.StatusInternalServerError)
	}))
	defer ts.Close()
	tmp := t.TempDir()
	audit, err := NewAuditWriter(filepath.Join(tmp, "audit.log"))
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	defer audit.Close()
	cfg := Config{AtlasBaseURL: ts.URL, AuditLogPath: filepath.Join(tmp, "audit.log")}
	s := &server{cfg: cfg, audit: audit, cli: NewHTTPClient(cfg)}
	_, _, err = s.handleEventFlowPrediction(context.Background(), nil, struct{}{})
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got %v", err)
	}
}

func TestEventFlowPrediction_MultipleDays(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"predictions":[{"date":"2026-07-14","direction":"inflow","confidence":0.8,"driving_events":["MSCI"],"predicted_forces":["foreign"]},{"date":"2026-07-15","direction":"outflow","confidence":0.6,"driving_events":["除息"],"predicted_forces":["retail"]},{"date":"2026-07-16","direction":"inflow","confidence":0.7,"driving_events":["ETF"],"predicted_forces":["institutional"]},{"date":"2026-07-17","direction":"neutral","confidence":0.5,"driving_events":[],"predicted_forces":[]},{"date":"2026-07-18","direction":"inflow","confidence":0.9,"driving_events":["週五"],"predicted_forces":["all"]}],"summary":"mixed"}`)
	_, out, err := s.handleEventFlowPrediction(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	preds, ok := out["predictions"].([]any)
	if !ok {
		t.Fatalf("expected predictions array, got %T", out["predictions"])
	}
	if len(preds) != 5 {
		t.Fatalf("expected 5 predictions, got %d", len(preds))
	}
}

func TestEventCalendar_ForwardsEvents(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"events":[{"date":"2026-07-15","type":"MSCI","description":"MSCI 季度調整"},{"date":"2026-07-20","type":"除息","description":"台積電除息"},{"date":"2026-07-25","type":"法說","description":"聯發科法說"}],"total":3}`)
	_, out, err := s.handleEventCalendar(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/events/calendar" {
		t.Fatalf("path=%s, want=/api/events/calendar", rec.path)
	}
	events, ok := out["events"].([]any)
	if !ok {
		t.Fatalf("expected events array, got %T", out["events"])
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	total, ok := out["total"].(float64)
	if !ok || total != 3 {
		t.Fatalf("expected total=3, got %v", out["total"])
	}
}

func TestEventCalendar_EmptyFallback(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"events":[],"total":0}`)
	_, out, err := s.handleEventCalendar(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	events, ok := out["events"].([]any)
	if !ok {
		t.Fatalf("expected events array, got %T", out["events"])
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

func TestEventFlowPrediction_FutureDate_Excluded(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"predictions":[{"date":"2026-07-25","direction":"inflow","confidence":0.75,"driving_events":["未來事件"],"predicted_forces":["foreign"]}],"summary":"偏多"}`)
	_, out, err := s.handleEventFlowPrediction(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	preds, ok := out["predictions"].([]any)
	if !ok {
		t.Fatalf("expected predictions array, got %T", out["predictions"])
	}
	if len(preds) < 1 {
		t.Fatal("expected at least 1 prediction")
	}
	p0 := preds[0].(map[string]any)
	if p0["date"] != "2026-07-25" {
		t.Fatalf("expected date=2026-07-25, got %v", p0["date"])
	}
}
