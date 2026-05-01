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
			p.UseTWSE()
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

	if p.finmindCB == nil {
		t.Fatal("expected finmindCB to be initialized")
	}

	if !p.finmindCB.Allow() {
		t.Error("expected Allow()=true when circuit is closed")
	}

	p.finmindCB.RecordFailure()
	p.finmindCB.RecordFailure()
	p.finmindCB.RecordFailure()

	if p.finmindCB.Allow() {
		t.Error("expected Allow()=false after 3 failures (circuit open)")
	}

	stats := p.CircuitBreakerStats()
	if stats["finmind_failure_count"] != 3 {
		t.Errorf("finmind_failure_count = %d, want 3", stats["finmind_failure_count"])
	}
	if stats["finmind_state"] != string(ProviderCircuitOpen) {
		t.Errorf("finmind_state = %q, want %q", stats["finmind_state"], ProviderCircuitOpen)
	}

	p.finmindCB.mu.Lock()
	p.finmindCB.state = ProviderCircuitHalfOpen
	p.finmindCB.halfOpenCalls = 0
	p.finmindCB.mu.Unlock()

	// Allow() increments halfOpenCalls, RecordSuccess() checks threshold
	p.finmindCB.Allow() // halfOpenCalls = 1
	p.finmindCB.RecordSuccess()
	p.finmindCB.Allow()         // halfOpenCalls = 2
	p.finmindCB.RecordSuccess() // now halfOpenCalls >= halfOpenMaxCalls, circuit closes

	if !p.finmindCB.Allow() {
		t.Error("expected Allow()=true after recovery (circuit closed)")
	}

	stats = p.CircuitBreakerStats()
	if stats["finmind_state"] != string(ProviderCircuitClosed) {
		t.Errorf("finmind_state = %q, want %q after recovery", stats["finmind_state"], ProviderCircuitClosed)
	}
	if stats["finmind_failure_count"] != 0 {
		t.Errorf("finmind_failure_count = %d, want 0 after recovery", stats["finmind_failure_count"])
	}
}

func TestHybridProvider_FinMindCircuitBreaker_HalfOpen(t *testing.T) {
	p := NewHybridProvider("finmind-key", "fugle-key")

	p.finmindCB.RecordFailure()
	p.finmindCB.RecordFailure()
	p.finmindCB.RecordFailure()

	p.finmindCB.mu.Lock()
	p.finmindCB.state = ProviderCircuitHalfOpen
	p.finmindCB.halfOpenCalls = 0
	p.finmindCB.mu.Unlock()

	// First Allow() increments halfOpenCalls to 1, RecordSuccess() won't close yet (1 < 2)
	p.finmindCB.Allow()
	p.finmindCB.RecordSuccess()

	p.finmindCB.mu.RLock()
	state := p.finmindCB.state
	p.finmindCB.mu.RUnlock()
	if state != ProviderCircuitHalfOpen {
		t.Errorf("state = %q, want %q after first success in half-open", state, ProviderCircuitHalfOpen)
	}

	// Second Allow() increments halfOpenCalls to 2, RecordSuccess() closes (2 >= 2)
	p.finmindCB.Allow()
	p.finmindCB.RecordSuccess()

	p.finmindCB.mu.RLock()
	state = p.finmindCB.state
	p.finmindCB.mu.RUnlock()
	if state != ProviderCircuitClosed {
		t.Errorf("state = %q, want %q after second success in half-open", state, ProviderCircuitClosed)
	}
}

func TestHybridProvider_FinMindCircuitBreaker_RecoveryTimeout(t *testing.T) {
	p := NewHybridProvider("finmind-key", "fugle-key")

	p.finmindCB.RecordFailure()
	p.finmindCB.RecordFailure()
	p.finmindCB.RecordFailure()

	if p.finmindCB.Allow() {
		t.Error("expected Allow()=false immediately after opening circuit")
	}

	p.finmindCB.mu.Lock()
	p.finmindCB.lastFailure = time.Now().Add(-10 * time.Minute)
	p.finmindCB.mu.Unlock()

	if !p.finmindCB.Allow() {
		t.Error("expected Allow()=true after recovery timeout (half-open)")
	}

	p.finmindCB.mu.RLock()
	state := p.finmindCB.state
	p.finmindCB.mu.RUnlock()
	if state != ProviderCircuitHalfOpen {
		t.Errorf("state = %q, want %q after recovery timeout", state, ProviderCircuitHalfOpen)
	}
}

func TestHybridProvider_FinMindCircuitBreaker_HalfOpenLimit(t *testing.T) {
	p := NewHybridProvider("finmind-key", "fugle-key")

	p.finmindCB.RecordFailure()
	p.finmindCB.RecordFailure()
	p.finmindCB.RecordFailure()

	p.finmindCB.mu.Lock()
	p.finmindCB.state = ProviderCircuitHalfOpen
	p.finmindCB.halfOpenCalls = 0
	p.finmindCB.mu.Unlock()

	if !p.finmindCB.Allow() {
		t.Error("expected Allow()=true for first half-open call")
	}
	if !p.finmindCB.Allow() {
		t.Error("expected Allow()=true for second half-open call")
	}
	if p.finmindCB.Allow() {
		t.Error("expected Allow()=false for third half-open call (exceeds limit)")
	}
}

func TestHybridProvider_IndependentCircuitBreakers(t *testing.T) {
	p := NewHybridProvider("finmind-key", "fugle-key")

	p.finmindCB.RecordFailure()
	p.finmindCB.RecordFailure()
	p.finmindCB.RecordFailure()

	if p.finmindCB.Allow() {
		t.Error("expected finmindCB.Allow()=false after 3 failures")
	}

	if !p.fugleCB.Allow() {
		t.Error("expected fugleCB.Allow()=true (independent from FinMind failures)")
	}

	if p.IsUsingTWSE() {
		t.Error("expected IsUsingTWSE()=false when Fugle CB is still closed")
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
