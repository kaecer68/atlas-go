package marketdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestMockProvider_Name(t *testing.T) {
	p := NewMockProvider()
	if got := p.Name(); got != "mock" {
		t.Fatalf("Name() = %q, want %q", got, "mock")
	}
}

func TestMockProvider_GetQuotes(t *testing.T) {
	p := NewMockProvider()
	symbols := []string{"2330", "2317"}
	quotes, err := p.GetQuotes(context.Background(), time.Now(), symbols)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(quotes) != len(symbols) {
		t.Fatalf("expected %d quotes, got %d", len(symbols), len(quotes))
	}
	for _, q := range quotes {
		if q.Last <= 0 {
			t.Errorf("symbol %s: Last price should be > 0, got %f", q.Symbol, q.Last)
		}
		if q.Source != "mock" {
			t.Errorf("symbol %s: Source = %q, want %q", q.Symbol, q.Source, "mock")
		}
	}
}

func TestMockProvider_IsMock(t *testing.T) {
	p := NewMockProvider()
	if !p.IsMock() {
		t.Fatal("expected IsMock() to be true")
	}
}

// ─── HybridProvider ──────────────────────────────────────────────────────────

func TestHybridProvider_NoAPIKey(t *testing.T) {
	p := NewHybridProvider("", "")
	if p.Name() != "hybrid-fubon" {
		t.Fatalf("Name() = %q, want %q", p.Name(), "hybrid-fubon")
	}
	if p.GetFubonClient() == nil {
		t.Fatal("GetFubonClient() should not be nil")
	}
	if p.GetFugleClient() != nil {
		t.Fatal("GetFugleClient() should be nil when no API key")
	}
}

func TestHybridProvider_WithAPIKey(t *testing.T) {
	p := NewHybridProvider("", "test-key")
	if p.Name() != "hybrid-fubon" {
		t.Fatalf("Name() = %q, want %q", p.Name(), "hybrid-fubon")
	}
	if p.GetFubonClient() == nil {
		t.Fatal("GetFubonClient() should not be nil")
	}
	if p.GetFugleClient() == nil {
		t.Fatal("GetFugleClient() should not be nil when API key is set")
	}

	p2 := NewHybridProvider("finmind-key", "")
	if p2.Name() != "hybrid-fubon" {
		t.Fatalf("Name() = %q, want %q", p2.Name(), "hybrid-fubon")
	}

	p3 := NewHybridProvider("finmind-key", "fugle-key")
	if p3.Name() != "hybrid-fubon" {
		t.Fatalf("Name() = %q, want %q (Fubon primary)", p3.Name(), "hybrid-fubon")
	}
}

func TestHybridProvider_Reset(t *testing.T) {
	tests := []struct {
		name     string
		apiKey   string
		wantTWSE bool
	}{
		{"no fugle configured: stays twse after reset", "", true},
		{"fugle configured: resets to fugle after UseTWSE", "key", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewHybridProvider("", tt.apiKey)
			p.UseTWSE() // force TWSE
			p.Reset()
			if tt.wantTWSE && !p.IsUsingTWSE() {
				t.Error("expected IsUsingTWSE()=true after Reset with no Fugle configured")
			}
			if !tt.wantTWSE && p.IsUsingTWSE() {
				t.Error("expected IsUsingTWSE()=false after Reset when Fugle is configured")
			}
		})
	}
}

func TestHybridProvider_UseTWSE_UseFugle(t *testing.T) {
	p := NewHybridProvider("", "key")

	p.UseTWSE()
	if p.Name() != "hybrid-fubon" {
		t.Fatalf("after UseTWSE with Fugle: Name() = %q, want hybrid-fubon", p.Name())
	}

	p.UseFugle()
	if p.Name() != "hybrid-fubon" {
		t.Fatalf("after UseFugle: Name() = %q, want hybrid-fubon", p.Name())
	}

	p2 := NewHybridProvider("finmind-key", "fugle-key")
	p2.UseTWSE()
	if p2.Name() != "hybrid-fubon" {
		t.Fatalf("after UseTWSE with FinMind+Fugle: Name() = %q, want hybrid-fubon", p2.Name())
	}
}

func TestHybridProvider_hasInvalidQuotes(t *testing.T) {
	p := &HybridProvider{}

	allZero := []domain.Quote{{Symbol: "X", Last: 0, Open: 0, High: 0, Low: 0}}
	if !p.hasInvalidQuotes(allZero) {
		t.Error("expected hasInvalidQuotes=true for all-zero quote")
	}

	valid := []domain.Quote{{Symbol: "X", Last: 100, Open: 99, High: 101, Low: 98}}
	if p.hasInvalidQuotes(valid) {
		t.Error("expected hasInvalidQuotes=false for valid quote")
	}

	mixed := []domain.Quote{
		{Symbol: "A", Last: 100, Open: 99, High: 101, Low: 98},
		{Symbol: "B", Last: 0, Open: 0, High: 0, Low: 0},
	}
	if !p.hasInvalidQuotes(mixed) {
		t.Error("expected hasInvalidQuotes=true when at least one all-zero quote is present")
	}

	negativePrice := []domain.Quote{{Symbol: "X", Last: -100, Open: 99, High: 101, Low: 98}}
	if !p.hasInvalidQuotes(negativePrice) {
		t.Error("expected hasInvalidQuotes=true for negative price")
	}

	negativeVolume := []domain.Quote{{Symbol: "X", Last: 100, Open: 99, High: 101, Low: 98, Volume: -1}}
	if !p.hasInvalidQuotes(negativeVolume) {
		t.Error("expected hasInvalidQuotes=true for negative volume")
	}
}

// ─── TWSEClient via httptest ──────────────────────────────────────────────────

func TestTWSEClient_GetQuotes_Success(t *testing.T) {
	payload := []TWSEQuote{
		{
			Code:         "2330",
			Name:         "台積電",
			ClosingPrice: "785.00",
			OpeningPrice: "780.00",
			HighestPrice: "790.00",
			LowestPrice:  "775.00",
			TradeVolume:  "15000000",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	client := NewTWSEClient()
	client.baseURL = srv.URL

	quotes, err := client.GetQuotes(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(quotes) != 1 {
		t.Fatalf("expected 1 quote, got %d", len(quotes))
	}
	q := quotes[0]
	if q.Symbol != "2330" {
		t.Errorf("Symbol = %q, want 2330", q.Symbol)
	}
	if q.Last != 785.00 {
		t.Errorf("Last = %f, want 785.00", q.Last)
	}
	if q.Source != "twse" {
		t.Errorf("Source = %q, want twse", q.Source)
	}
}

func TestTWSEClient_GetQuotes_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := NewTWSEClient()
	client.baseURL = srv.URL

	_, err := client.GetQuotes(context.Background())
	if err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}

func TestTWSEClient_GetQuotesBySymbols(t *testing.T) {
	payload := []TWSEQuote{
		{Code: "2330", ClosingPrice: "785.00", OpeningPrice: "780.00", HighestPrice: "790.00", LowestPrice: "775.00", TradeVolume: "1000"},
		{Code: "2317", ClosingPrice: "162.00", OpeningPrice: "160.00", HighestPrice: "164.00", LowestPrice: "159.00", TradeVolume: "2000"},
		{Code: "0050", ClosingPrice: "192.00", OpeningPrice: "191.00", HighestPrice: "193.00", LowestPrice: "190.00", TradeVolume: "5000"},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	client := NewTWSEClient()
	client.baseURL = srv.URL

	quotes, err := client.GetQuotesBySymbols(context.Background(), []string{"2330", "0050"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(quotes) != 2 {
		t.Fatalf("expected 2 quotes (filtered), got %d", len(quotes))
	}
	symbols := make(map[string]bool)
	for _, q := range quotes {
		symbols[q.Symbol] = true
	}
	if !symbols["2330"] || !symbols["0050"] {
		t.Errorf("expected symbols 2330 and 0050 in result, got %v", symbols)
	}
	if symbols["2317"] {
		t.Error("2317 should have been filtered out")
	}
}
