package server

import (
	"context"
	"testing"
)

func TestHandleStrategyGetLayers_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleStrategyGetLayers(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/strategies/layers" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleStrategyGet_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleStrategyGet(context.Background(), nil, strategyIDInput{ID: "strat-42"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/strategies/strat-42" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleStrategyGetAttribution_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleStrategyGetAttribution(context.Background(), nil, strategyIDInput{ID: "strat-42"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/strategies/strat-42/attribution" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleStrategyGetSummary_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleStrategyGetSummary(context.Background(), nil, strategyIDInput{ID: "strat-42"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/strategies/strat-42/summary" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}
