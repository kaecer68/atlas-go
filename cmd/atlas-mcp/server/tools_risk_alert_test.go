package server

import (
	"context"
	"testing"
)

func TestHandleRiskGetMetrics_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleRiskGetMetrics(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/risk" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleRiskGetCorrelationMatrix_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleRiskGetCorrelationMatrix(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/correlation-matrix" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleRiskGetDrawdown_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleRiskGetDrawdown(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/drawdown" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleRiskGetCalibration_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleRiskGetCalibration(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/risk-calibration" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleRiskGetCommentary_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleRiskGetCommentary(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/risk/commentary" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleAlertList_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleAlertList(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/alerts" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleAlertGetStats_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleAlertGetStats(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/alerts/stats" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleAlertGetRules_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, out, err := s.handleAlertGetRules(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/alerts/rules" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleAlertScan_HitsActiveEndpoint(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()

	// Simulate the new /api/alerts/active response shape (alertscanner.Snapshot).
	const response = `{
		"total": 5,
		"blocked": true,
		"by_severity": {"critical": 1, "high": 2, "medium": 1, "low": 1},
		"alerts": [{"id": "a1", "severity": "critical", "message": "circuit breaker open"}]
	}`
	rec.responseBody = []byte(response)

	_, out, err := s.handleAlertScan(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handleAlertScan: %v", err)
	}

	// Verify the correct endpoint was called.
	if rec.path != "/api/alerts/active" {
		t.Fatalf("endpoint path = %s, want /api/alerts/active", rec.path)
	}

	// Verify Result decodes the Snapshot shape.
	if out.Result == nil {
		t.Fatal("Result is nil")
	}
	if total, _ := out.Result["total"].(float64); total != 5 {
		t.Fatalf("total = %v, want 5", out.Result["total"])
	}
	if blocked, _ := out.Result["blocked"].(bool); !blocked {
		t.Fatal("blocked should be true")
	}
	if sev, ok := out.Result["by_severity"].(map[string]any); !ok {
		t.Fatal("by_severity missing or wrong type")
	} else if sev["critical"].(float64) != 1 {
		t.Fatalf("by_severity.critical = %v, want 1", sev["critical"])
	}
	if alerts, ok := out.Result["alerts"].([]any); !ok || len(alerts) != 1 {
		t.Fatalf("alerts missing or wrong count: %v", out.Result["alerts"])
	}
}
