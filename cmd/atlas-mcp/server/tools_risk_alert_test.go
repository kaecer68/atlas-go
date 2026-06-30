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
