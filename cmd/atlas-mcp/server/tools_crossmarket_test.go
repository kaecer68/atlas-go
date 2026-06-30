package server

import (
	"context"
	"testing"
)

func TestHandleCrossmarketGetStatus_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"sp500":"ok","nasdaq":"ok"}`)
	_, out, err := s.handleCrossmarketGetStatus(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/cross-market/status" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleCrossmarketGetCorrelation_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"correlations":{"2330":0.72}}`)
	_, out, err := s.handleCrossmarketGetCorrelation(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/cross-market/correlation" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}

func TestHandleCrossmarketGetUsIndices_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"sp500":4523,"nasdaq":14123}`)
	_, out, err := s.handleCrossmarketGetUsIndices(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/us-indices" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected Result non-nil")
	}
}
