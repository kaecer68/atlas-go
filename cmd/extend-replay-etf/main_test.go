package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchStockDay_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"stat": "OK",
			"date": "20260528",
			"title": "0050 元大台灣50 各日成交資訊",
			"data": [
				["115/05/28", "10,000,000", "500,000,000", "100.00", "101.00", "99.00", "100.50", "+0.50", "5,000"]
			]
		}`))
	}))
	defer server.Close()
	twseBaseURL = server.URL
	twseBaseURL = server.URL

	date := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	bar, ok, err := fetchStockDay(context.Background(), server.Client(), "0050", date)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected data, got not found")
	}
	if bar.Symbol != "0050" {
		t.Errorf("expected symbol 0050, got %s", bar.Symbol)
	}
	if bar.Close != 100.50 {
		t.Errorf("expected close 100.50, got %.2f", bar.Close)
	}
	if bar.Open != 100.00 {
		t.Errorf("expected open 100.00, got %.2f", bar.Open)
	}
}

func TestFetchStockDay_NoData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"stat": "OK", "date": "20260528", "title": "0050", "data": []}`))
	}))
	defer server.Close()
	twseBaseURL = server.URL

	date := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	_, ok, err := fetchStockDay(context.Background(), server.Client(), "0050", date)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected no data, got found")
	}
}

func TestFetchStockDay_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	twseBaseURL = server.URL

	date := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	_, ok, err := fetchStockDay(context.Background(), server.Client(), "0050", date)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if ok {
		t.Fatal("expected no data on error")
	}
}

func TestFetchStockDay_DateNotInMonth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"stat": "OK",
			"date": "20260501",
			"title": "0050 元大台灣50 各日成交資訊",
			"data": [
				["115/05/02", "1,000,000", "50,000,000", "99.00", "100.00", "98.00", "99.50", "+0.50", "500"]
			]
		}`))
	}))
	defer server.Close()
	twseBaseURL = server.URL

	// Request May 28, but only May 2 data exists
	date := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	_, ok, err := fetchStockDay(context.Background(), server.Client(), "0050", date)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected not found for date outside month data")
	}
}

func TestTradingDates_WeekdayOnly(t *testing.T) {
	start := time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC) // Friday
	end := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)    // Monday
	dates := tradingDates(start, end)
	if len(dates) != 2 { // Friday + Monday, skip Saturday+Sunday
		t.Fatalf("expected 2 trading days, got %d", len(dates))
	}
}
