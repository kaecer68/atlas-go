package server

import (
	"context"
	"testing"
)

func TestHandleEventCalendar_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"events":[],"total":0}`)
	_, out, err := s.handleEventCalendar(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/events/calendar" {
		t.Fatalf("path=%s", rec.path)
	}
	if out == nil {
		t.Fatal("expected out non-nil")
	}
}

func TestHandleEventFlowPrediction_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"days":[],"summary":"neutral"}`)
	_, out, err := s.handleEventFlowPrediction(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/events/prediction" {
		t.Fatalf("path=%s", rec.path)
	}
	if out == nil {
		t.Fatal("expected out non-nil")
	}
}
