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
