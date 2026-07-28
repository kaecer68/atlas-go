package marketdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestMarketVolumeProvider_ParseNormalData(t *testing.T) {
	twseResp := twseMIIndexResponse{
		Stat: "OK",
		Date: "20260725",
		Tables: []twseMITable{
			{},
			{},
			{},
			{},
			{},
			{},
			{
				Title:  "大盤統計資訊",
				Fields: []string{"成交統計", "成交金額(元)", "成交股數(股)", "成交筆數"},
				Data: [][]string{
					{"1.一般股票", "687,230,800,745", "4,331,426,130", "2,937,384"},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(twseResp)
	}))
	defer srv.Close()

	provider := NewMarketVolumeProvider()
	provider.SetHTTPClient(srv.Client())
	provider.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	provider.baseURL = srv.URL

	result, err := provider.FetchLatest(context.Background())
	if err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if result.MarketVolume <= 0 {
		t.Fatalf("expected positive MarketVolume, got %f", result.MarketVolume)
	}
	// 687,230,800,745 元 / 1e8 = 6872.30800745 億元
	expected := 687230800745.0 / 100_000_000
	if result.MarketVolume != expected {
		t.Errorf("MarketVolume = %f, want %f", result.MarketVolume, expected)
	}
	// FetchLatest 從今天開始往後掃 7 天，會先命中今天
	today := time.Now().UTC().Format("20060102")
	if result.Date != today {
		t.Errorf("Date = %q, want %s", result.Date, today)
	}
}

func TestMarketVolumeProvider_EmptyTable(t *testing.T) {
	// 休市日：tables 存在但 data 為空
	twseResp := twseMIIndexResponse{
		Stat: "OK",
		Date: "20260726",
		Tables: []twseMITable{
			{},
			{},
			{},
			{},
			{},
			{},
			{
				Title:  "大盤統計資訊",
				Fields: []string{"成交統計", "成交金額(元)", "成交股數(股)", "成交筆數"},
				Data:   [][]string{},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(twseResp)
	}))
	defer srv.Close()

	provider := NewMarketVolumeProvider()
	provider.SetHTTPClient(srv.Client())
	provider.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	provider.baseURL = srv.URL

	// 空資料 → 應該要 fallback 到下一天，最終回傳 error
	_, err := provider.FetchLatest(context.Background())
	if err == nil {
		t.Fatal("expected error for empty market stats, got nil")
	}
}

func TestMarketVolumeProvider_NonOKStat(t *testing.T) {
	twseResp := struct {
		Stat   string `json:"stat"`
		Date   string `json:"date"`
		Tables []twseMITable
	}{
		Stat:   "ERROR",
		Date:   "20260726",
		Tables: nil,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(twseResp)
	}))
	defer srv.Close()

	provider := NewMarketVolumeProvider()
	provider.SetHTTPClient(srv.Client())
	provider.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	provider.baseURL = srv.URL

	_, err := provider.FetchLatest(context.Background())
	if err == nil {
		t.Fatal("expected error for non-OK stat, got nil")
	}
}

func TestMarketVolumeProvider_NoPanicOnMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{not json"))
	}))
	defer srv.Close()

	provider := NewMarketVolumeProvider()
	provider.SetHTTPClient(srv.Client())
	provider.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	provider.baseURL = srv.URL

	// Should not panic — returns error gracefully.
	_, err := provider.FetchLatest(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestMarketVolumeProvider_InvalidAmountRow(t *testing.T) {
	// Row 0 col 1 is non-numeric → parseTWSEFloat returns 0 → treated as non-positive
	twseResp := twseMIIndexResponse{
		Stat: "OK",
		Date: "20260725",
		Tables: []twseMITable{
			{},
			{},
			{},
			{},
			{},
			{},
			{
				Title:  "大盤統計資訊",
				Fields: []string{"成交統計", "成交金額(元)", "成交股數(股)", "成交筆數"},
				Data: [][]string{
					{"1.一般股票", "--", "0", "0"},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(twseResp)
	}))
	defer srv.Close()

	provider := NewMarketVolumeProvider()
	provider.SetHTTPClient(srv.Client())
	provider.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	provider.baseURL = srv.URL

	_, err := provider.FetchLatest(context.Background())
	if err == nil {
		t.Fatal("expected error for non-positive amount, got nil")
	}
}
