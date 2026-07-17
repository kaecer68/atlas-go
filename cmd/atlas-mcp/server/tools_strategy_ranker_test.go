package server

import (
	"context"
	"testing"
)

func TestHandleStrategyRanker(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	// Backend returns a bare JSON array (internal/strategy_ranker/handler.go
	// HandleRank), not an object — the mock must match the real shape.
	rec.responseBody = []byte(`[{"strategy_id":"S1","win_rate":0.7,"tier":"free"}]`)
	_, out, err := s.handleStrategyRanker(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/strategy-ranker/rank" {
		t.Fatalf("path=%s", rec.path)
	}
	if len(out.Strategies) != 1 {
		t.Fatalf("expected 1 strategy, got %d", len(out.Strategies))
	}
	if out.Strategies[0]["strategy_id"] != "S1" {
		t.Fatalf("unexpected payload: %v", out.Strategies[0])
	}
}

func TestHandleStrategyRankerEmpty(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`[]`)
	_, out, err := s.handleStrategyRanker(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if len(out.Strategies) != 0 {
		t.Fatalf("expected 0 strategies, got %d", len(out.Strategies))
	}
}
