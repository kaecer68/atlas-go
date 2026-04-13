package marketdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFugleClient_GetQuote_Success(t *testing.T) {
	payload := FugleQuoteResponse{
		APIVersion: "v0.3",
	}
	payload.Data.Info.Symbol = "2330"
	payload.Data.Quote.Trade.Price = 785.0
	payload.Data.Quote.PriceOpen.Price = 780.0
	payload.Data.Quote.PriceHigh.Price = 790.0
	payload.Data.Quote.PriceLow.Price = 775.0
	payload.Data.Quote.Total.TradeVolume = 1500000

	body, _ := json.Marshal(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("symbolId") != "2330" {
			t.Errorf("unexpected symbolId: %s", r.URL.Query().Get("symbolId"))
		}
		w.Write(body)
	}))
	defer srv.Close()

	client := NewFugleClient("test-key")
	client.baseURL = srv.URL

	quote, err := client.GetQuote(context.Background(), "2330")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if quote.Symbol != "2330" {
		t.Errorf("Symbol = %q, want 2330", quote.Symbol)
	}
	if quote.Last != 785.0 {
		t.Errorf("Last = %f, want 785.0", quote.Last)
	}
	if quote.Source != "fugle" {
		t.Errorf("Source = %q, want fugle", quote.Source)
	}
}

func TestFugleClient_GetQuote_NonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := NewFugleClient("bad-key")
	client.baseURL = srv.URL

	_, err := client.GetQuote(context.Background(), "2330")
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

func TestFugleClient_GetQuotes(t *testing.T) {
	payload := FugleQuoteResponse{
		APIVersion: "v0.3",
	}
	payload.Data.Info.Symbol = "2330"
	payload.Data.Quote.Trade.Price = 785.0
	payload.Data.Quote.PriceOpen.Price = 780.0
	payload.Data.Quote.PriceHigh.Price = 790.0
	payload.Data.Quote.PriceLow.Price = 775.0
	payload.Data.Quote.Total.TradeVolume = 1000

	body, _ := json.Marshal(payload)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	client := NewFugleClient("test-key")
	client.baseURL = srv.URL

	quotes, err := client.GetQuotes(context.Background(), []string{"2330", "2317"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(quotes) != 2 {
		t.Fatalf("expected 2 quotes, got %d", len(quotes))
	}
}

func TestNewFugleProviderWithAPIKey(t *testing.T) {
	p := NewFugleProviderWithAPIKey("test-key")
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	if p.Name() != "fugle" {
		t.Errorf("Name = %q, want fugle", p.Name())
	}
}
