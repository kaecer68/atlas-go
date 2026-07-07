package server

import (
	"context"
	"testing"
)

func TestHandleCapitalFlowDaily(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"force_scores":{"foreign":70},"resonance":0.6}`)
	_, out, err := s.handleCapitalFlowDaily(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/capital-flow/daily" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected result")
	}
}

func TestHandleCapitalFlowSummary(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"latest_date":"20260707","summary":"neutral"}`)
	_, out, err := s.handleCapitalFlowSummary(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/capital-flow/summary" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected result")
	}
}
