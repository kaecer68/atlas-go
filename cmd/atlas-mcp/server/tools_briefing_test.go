package server

import (
	"context"
	"testing"
)

func TestHandleMCPQuickstart_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleMCPQuickstart(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out == nil {
		t.Fatal("expected out non-nil")
	}
	if _, ok := out["macro_snapshot"]; !ok {
		t.Fatal("expected macro_snapshot key")
	}
	if _, ok := out["active_strategies"]; !ok {
		t.Fatal("expected active_strategies key")
	}
}

func TestHandleMCPQuickstart_DegradedVisible(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	// Invalid JSON makes every section fetch fail; the handler must surface
	// per-section degraded markers instead of swallowing errors.
	rec.responseBody = []byte(`not json`)
	_, out, err := s.handleMCPQuickstart(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	sections, ok := out["degraded_sections"].([]string)
	if !ok || len(sections) != 4 {
		t.Fatalf("expected 4 degraded sections (macro harness path succeeds), got %v", out["degraded_sections"])
	}
	// The harness serves valid JSON for /api/macro/snapshot/latest, so that
	// section must NOT be degraded; a failing one (stress_index) must be.
	macro, ok := out["macro_snapshot"].(map[string]any)
	if !ok || macro["degraded"] == true {
		t.Fatalf("macro_snapshot should succeed via harness path, got %v", out["macro_snapshot"])
	}
	stress, ok := out["stress_index"].(map[string]any)
	if !ok || stress["degraded"] != true {
		t.Fatalf("stress_index should carry degraded marker, got %v", out["stress_index"])
	}
}

func TestHandleDailyReport_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"report":"daily"}`)
	_, out, err := s.handleDailyReport(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/reports/latest" {
		t.Fatalf("path=%s", rec.path)
	}
	if out == nil {
		t.Fatal("expected out non-nil")
	}
}
