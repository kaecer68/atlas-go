package server

import (
	"context"
	"testing"
)

func TestHandleStrategyRanker(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"strategies":[{"id":"S1","hit_rate":0.7,"tier":"free"}]}`)
	_, out, err := s.handleStrategyRanker(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/strategy-ranker/rank" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected result")
	}
}
