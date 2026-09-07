package server

import (
	"context"
	"testing"
)

func TestHandleStockGetQuote(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"symbol":"2330","last":680}`)
	_, out, err := s.handleStockGetQuote(context.Background(), nil, stockSymbolInput{Symbol: "2330"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/stock/quote" {
		t.Fatalf("path=%s", rec.path)
	}
	if rec.query.Get("symbol") != "2330" {
		t.Fatalf("symbol=%s", rec.query.Get("symbol"))
	}
	if out.Result == nil {
		t.Fatal("expected result")
	}
}

func TestHandleStockGetQuoteMissingSymbol(t *testing.T) {
	s, _, done := newTestHarness(t)
	defer done()
	_, _, err := s.handleStockGetQuote(context.Background(), nil, stockSymbolInput{})
	if err == nil {
		t.Fatal("expected error for missing symbol")
	}
}

func TestHandleStockGetFundamentals(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"PE":25,"PB":6}`)
	_, out, err := s.handleStockGetFundamentals(context.Background(), nil, stockSymbolInput{Symbol: "2330"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/stock/fundamentals" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Result == nil {
		t.Fatal("expected result")
	}
}

func TestHandleStockGetChips(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"symbol":"2330","foreign_investor_net":100}`)
	_, out, err := s.handleStockGetChips(context.Background(), nil, stockChipsInput{Symbol: "2330", Date: "20260701"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/stock/chips" {
		t.Fatalf("path=%s", rec.path)
	}
	if rec.query.Get("date") != "20260701" {
		t.Fatalf("date=%s", rec.query.Get("date"))
	}
	if out.Result == nil {
		t.Fatal("expected result")
	}
}

func TestHandleStockGetTechnical(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"close":690,"sma20":680}`)
	_, out, err := s.handleStockGetTechnical(context.Background(), nil, stockTechnicalInput{Symbol: "2330", Days: 30})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/stock/technical" {
		t.Fatalf("path=%s", rec.path)
	}
	if rec.query.Get("days") != "30" {
		t.Fatalf("days=%s", rec.query.Get("days"))
	}
	if out.Result == nil {
		t.Fatal("expected result")
	}
}

func TestHandleStockGetTechnicalDefaults(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, _, err := s.handleStockGetTechnical(context.Background(), nil, stockTechnicalInput{Symbol: "2330", Days: 0})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.query.Get("days") != "90" {
		t.Fatalf("expected default days=90, got %s", rec.query.Get("days"))
	}
}

func TestHandleStockGetTechnicalClamped(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, _, err := s.handleStockGetTechnical(context.Background(), nil, stockTechnicalInput{Symbol: "2330", Days: 9999})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.query.Get("days") != "365" {
		t.Fatalf("expected clamped days=365, got %s", rec.query.Get("days"))
	}
}

func TestHandleStockGetVolumeDivergence(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"symbol":"2330.TW","top_divergence":true,"bottom_divergence":false}`)
	_, out, err := s.handleStockGetVolumeDivergence(context.Background(), nil, stockVolumeDivergenceInput{Symbol: "2330"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/stock/volume_divergence" {
		t.Fatalf("path=%s", rec.path)
	}
	if rec.query.Get("symbol") != "2330" {
		t.Fatalf("symbol=%s", rec.query.Get("symbol"))
	}
	if rec.query.Get("window") != "30" {
		t.Fatalf("window default=%s, want 30", rec.query.Get("window"))
	}
	if out.Result == nil {
		t.Fatal("expected result")
	}
}

func TestHandleStockGetVolumeDivergenceMissingSymbol(t *testing.T) {
	s, _, done := newTestHarness(t)
	defer done()
	_, _, err := s.handleStockGetVolumeDivergence(context.Background(), nil, stockVolumeDivergenceInput{})
	if err == nil {
		t.Fatal("expected error for missing symbol")
	}
}

func TestHandleStockGetVolumeDivergenceWindowClamped(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"symbol":"2330.TW"}`)
	_, _, err := s.handleStockGetVolumeDivergence(context.Background(), nil, stockVolumeDivergenceInput{Symbol: "2330", Window: 9999})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.query.Get("window") != "120" {
		t.Fatalf("window clamp=%s, want 120", rec.query.Get("window"))
	}
}

func TestHandleStockGetConditionWinRate(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"found":true,"condition_id":"momentum-20d-positive","direction":"buy","observations":120}`)
	_, out, err := s.handleStockGetConditionWinRate(context.Background(), nil, stockConditionWinRateInput{ConditionID: "momentum-20d-positive"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/stock/condition_winrate" {
		t.Fatalf("path=%s", rec.path)
	}
	if rec.query.Get("condition_id") != "momentum-20d-positive" || rec.query.Get("rolling_window") != "120d" {
		t.Fatalf("query=%v", rec.query)
	}
	if out.Result == nil {
		t.Fatal("expected result")
	}
}

func TestHandleStockGetConditionWinRateMissingCondition(t *testing.T) {
	s, _, done := newTestHarness(t)
	defer done()
	_, _, err := s.handleStockGetConditionWinRate(context.Background(), nil, stockConditionWinRateInput{})
	if err == nil {
		t.Fatal("expected error for missing condition_id")
	}
}
