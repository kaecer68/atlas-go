package server

import (
	"context"
	"testing"
)

func TestHandleCapitalFlowDaily_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"date":"2026-07-07","forces":[],"resonance":{}}`)
	_, out, err := s.handleCapitalFlowDaily(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/capital-flow/daily" {
		t.Fatalf("path=%s", rec.path)
	}
	if out == nil {
		t.Fatal("expected out non-nil")
	}
}

func TestHandleCapitalFlowSummary_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"date":"2026-07-07","summary":"neutral"}`)
	_, out, err := s.handleCapitalFlowSummary(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/capital-flow/summary" {
		t.Fatalf("path=%s", rec.path)
	}
	if out == nil {
		t.Fatal("expected out non-nil")
	}
}
