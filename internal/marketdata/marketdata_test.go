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
	if p.Name() != "hybrid-twse" {
		t.Fatalf("Name() = %q, want %q", p.Name(), "hybrid-twse")
	}
	if p.GetFugleClient() != nil {
		t.Fatal("GetFugleClient() should be nil when no API key")
	}
}

func TestHybridProvider_WithAPIKey(t *testing.T) {
	p := NewHybridProvider("", "test-key")
	if p.Name() != "hybrid-fugle" {
		t.Fatalf("Name() = %q, want %q", p.Name(), "hybrid-fugle")
	}
	if p.GetFugleClient() == nil {
		t.Fatal("GetFugleClient() should not be nil when API key is set")
	}

	p2 := NewHybridProvider("finmind-key", "")
	if p2.Name() != "hybrid-finmind" {
		t.Fatalf("Name() = %q, want %q", p2.Name(), "hybrid-finmind")
	}

	p3 := NewHybridProvider("finmind-key", "fugle-key")
	if p3.Name() != "hybrid-finmind" {
		t.Fatalf("Name() = %q, want %q (FinMind primary)", p3.Name(), "hybrid-finmind")
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
	if p.Name() != "hybrid-fugle" {
		t.Fatalf("after UseTWSE with only Fugle: Name() = %q, want hybrid-fugle", p.Name())
	}

	p.UseFugle()
	if p.Name() != "hybrid-fugle" {
		t.Fatalf("after UseFugle: Name() = %q, want hybrid-fugle", p.Name())
	}

	p2 := NewHybridProvider("finmind-key", "fugle-key")
	p2.UseTWSE()
	if p2.Name() != "hybrid-finmind" {
		t.Fatalf("after UseTWSE with FinMind+Fugle: Name() = %q, want hybrid-finmind", p2.Name())
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

func TestHybridProvider_FinMindCircuitBreaker(t *testing.T) {
	p := NewHybridProvider("finmind-key", "fugle-key")

	// Initial state: circuit closed
	if !p.shouldTryFinMind() {
		t.Error("expected shouldTryFinMind()=true when circuit is closed")
	}

	// Simulate 3 failures
	p.recordFinMindFailure()
	p.recordFinMindFailure()
	p.recordFinMindFailure()

	// After 3 failures: circuit should be open
	if p.shouldTryFinMind() {
		t.Error("expected shouldTryFinMind()=false after 3 failures (circuit open)")
	}

	// Circuit breaker stats
	stats := p.CircuitBreakerStats()
	if stats["failure_count"] != 3 {
		t.Errorf("failure_count = %d, want 3", stats["failure_count"])
	}
	if stats["state"] != string(ProviderCircuitOpen) {
		t.Errorf("state = %q, want %q", stats["state"], ProviderCircuitOpen)
	}

	// Manually set to half-open for testing
	p.cbMutex.Lock()
	p.cbState = ProviderCircuitHalfOpen
	p.cbHalfOpenCalls = 0
	p.cbMutex.Unlock()

	// Simulate recovery (need 2 successes in half-open)
	p.recordFinMindSuccess()
	p.recordFinMindSuccess()

	// After success in half-open: circuit should be closed
	if !p.shouldTryFinMind() {
		t.Error("expected shouldTryFinMind()=true after recovery (circuit closed)")
	}

	stats = p.CircuitBreakerStats()
	if stats["state"] != string(ProviderCircuitClosed) {
		t.Errorf("state = %q, want %q after recovery", stats["state"], ProviderCircuitClosed)
	}
	if stats["failure_count"] != 0 {
		t.Errorf("failure_count = %d, want 0 after recovery", stats["failure_count"])
	}
}

func TestHybridProvider_FinMindCircuitBreaker_HalfOpen(t *testing.T) {
	p := NewHybridProvider("finmind-key", "fugle-key")

	// Open the circuit
	p.recordFinMindFailure()
	p.recordFinMindFailure()
	p.recordFinMindFailure()

	// Manually set to half-open for testing
	p.cbMutex.Lock()
	p.cbState = ProviderCircuitHalfOpen
	p.cbHalfOpenCalls = 0
	p.cbMutex.Unlock()

	if !p.shouldTryFinMind() {
		t.Error("expected shouldTryFinMind()=true in half-open state")
	}

	// First success in half-open
	p.recordFinMindSuccess()

	// Should still be half-open (need halfOpenMaxCalls=2)
	p.cbMutex.RLock()
	state := p.cbState
	p.cbMutex.RUnlock()
	if state != ProviderCircuitHalfOpen {
		t.Errorf("state = %q, want %q after first success in half-open", state, ProviderCircuitHalfOpen)
	}

	// Second success in half-open
	p.recordFinMindSuccess()

	// Now should be closed
	p.cbMutex.RLock()
	state = p.cbState
	p.cbMutex.RUnlock()
	if state != ProviderCircuitClosed {
		t.Errorf("state = %q, want %q after second success in half-open", state, ProviderCircuitClosed)
	}
}

func TestHybridProvider_FinMindCircuitBreaker_RecoveryTimeout(t *testing.T) {
	p := NewHybridProvider("finmind-key", "fugle-key")

	// Open the circuit
	p.recordFinMindFailure()
	p.recordFinMindFailure()
	p.recordFinMindFailure()

	if p.shouldTryFinMind() {
		t.Error("expected shouldTryFinMind()=false immediately after opening circuit")
	}

	// Manually set last failure to past recovery timeout
	p.cbMutex.Lock()
	p.cbLastFailure = time.Now().Add(-10 * time.Minute)
	p.cbMutex.Unlock()

	// After recovery timeout: should enter half-open
	if !p.shouldTryFinMind() {
		t.Error("expected shouldTryFinMind()=true after recovery timeout (half-open)")
	}

	p.cbMutex.RLock()
	state := p.cbState
	p.cbMutex.RUnlock()
	if state != ProviderCircuitHalfOpen {
		t.Errorf("state = %q, want %q after recovery timeout", state, ProviderCircuitHalfOpen)
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
