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

// ─── TWSEProvider ────────────────────────────────────────────────────────────

func TestTWSEProvider_Name(t *testing.T) {
	p := NewTWSEProvider()
	if got := p.Name(); got != "twse" {
		t.Fatalf("Name() = %q, want %q", got, "twse")
	}
}

func TestTWSEProvider_GetQuotes(t *testing.T) {
	p := NewTWSEProvider()
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
		if q.Source != "twse" {
			t.Errorf("symbol %s: Source = %q, want %q", q.Symbol, q.Source, "twse")
		}
	}
}

// ─── HybridProvider ──────────────────────────────────────────────────────────

func TestHybridProvider_NoAPIKey(t *testing.T) {
	p := NewHybridProvider("")
	if p.Name() != "hybrid-twse" {
		t.Fatalf("Name() = %q, want %q", p.Name(), "hybrid-twse")
	}
	if p.GetFugleClient() != nil {
		t.Fatal("GetFugleClient() should be nil when no API key")
	}
}

func TestHybridProvider_WithAPIKey(t *testing.T) {
	p := NewHybridProvider("test-key")
	if p.Name() != "hybrid-fugle" {
		t.Fatalf("Name() = %q, want %q", p.Name(), "hybrid-fugle")
	}
	if p.GetFugleClient() == nil {
		t.Fatal("GetFugleClient() should not be nil when API key is set")
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
			p := NewHybridProvider(tt.apiKey)
			p.UseTWSE() // force TWSE
			p.Reset()
			if tt.wantTWSE && !p.useTWSE {
				t.Error("expected useTWSE=true after Reset with no Fugle configured")
			}
			if !tt.wantTWSE && p.useTWSE {
				t.Error("expected useTWSE=false after Reset when Fugle is configured")
			}
		})
	}
}

func TestHybridProvider_UseTWSE_UseFugle(t *testing.T) {
	p := NewHybridProvider("key")

	p.UseTWSE()
	if p.Name() != "hybrid-twse" {
		t.Fatalf("after UseTWSE: Name() = %q, want hybrid-twse", p.Name())
	}

	p.UseFugle()
	if p.Name() != "hybrid-fugle" {
		t.Fatalf("after UseFugle: Name() = %q, want hybrid-fugle", p.Name())
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

	// Mixed: one valid is enough to not trigger
	mixed := []domain.Quote{
		{Symbol: "A", Last: 100, Open: 99, High: 101, Low: 98},
		{Symbol: "B", Last: 0, Open: 0, High: 0, Low: 0},
	}
	if !p.hasInvalidQuotes(mixed) {
		t.Error("expected hasInvalidQuotes=true when at least one all-zero quote is present")
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
	body, _ := json.Marshal(payload)

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
	body, _ := json.Marshal(payload)

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
