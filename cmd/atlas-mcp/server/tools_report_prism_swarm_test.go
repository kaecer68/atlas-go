package server

import (
	"context"
	"testing"
)

func TestHandleReportGetDailySummary_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleReportGetDailySummary(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/daily-summary" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleReportGetPerformance_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleReportGetPerformance(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/performance-report" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleReportGetTaxSnapshot_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleReportGetTaxSnapshot(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/tax-snapshot" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleReportGetExportLink_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte("# Performance Report\n\nSample markdown.")
	_, out, err := s.handleReportGetExportLink(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/performance-report/export" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
	md, ok := (*out.Result)["markdown"].(string)
	if !ok {
		t.Fatalf("expected markdown string in result, got %T", (*out.Result)["markdown"])
	}
	if md != "# Performance Report\n\nSample markdown." {
		t.Fatalf("unexpected markdown content: %q", md)
	}
}

// Swarm handler tests removed — handlers deleted in swarm demotion (P1).
// File kept for report handler tests above.
