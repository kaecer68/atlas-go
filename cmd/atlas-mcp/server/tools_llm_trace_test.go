package server

import (
	"context"
	"testing"
)

func TestHandleLLMGetCost_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleLLMGetCost(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/llm_annotator/cost" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleLLMGetHealth_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleLLMGetHealth(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/llm/health" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleTraceGetSimLatest_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	// Backend returns a bare JSON array.
	rec.responseBody = []byte(`[{"agent":"ai-desk-01","action":"buy"}]`)
	_, out, err := s.handleTraceGetSimLatest(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/traces/sim-latest" {
		t.Fatalf("path=%s", rec.path)
	}
	if len(out.Traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(out.Traces))
	}
}

func TestHandleTraceGetAgentObservatory_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleTraceGetAgentObservatory(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/agent-observatory" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleTraceGetDecisionChain_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleTraceGetDecisionChain(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/decision-chain" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleTraceGetReasoning_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleTraceGetReasoning(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/reasoning-trace" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}
