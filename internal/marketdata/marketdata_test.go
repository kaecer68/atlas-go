package marketdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"

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
	// When Fubon proxy is reachable, primary is fubon; otherwise falls back to twse.
	if p.Name() != "hybrid-fubon" && p.Name() != "hybrid-twse" {
		t.Fatalf("Name() = %q, want hybrid-fubon or hybrid-twse", p.Name())
	}
	if p.GetFubonClient() == nil && p.Name() == "hybrid-fubon" {
		t.Fatal("GetFubonClient() should not be nil when fubon is primary")
	}
	if p.GetFugleClient() != nil {
		t.Fatal("GetFugleClient() should be nil when no API key")
	}
}

func TestHybridProvider_WithAPIKey(t *testing.T) {
	p := NewHybridProvider("", "test-key")
	// When Fubon proxy is reachable, primary is fubon; otherwise fugle.
	if p.Name() != "hybrid-fubon" && p.Name() != "hybrid-fugle" {
		t.Fatalf("Name() = %q, want hybrid-fubon or hybrid-fugle", p.Name())
	}
	if p.GetFugleClient() == nil {
		t.Fatal("GetFugleClient() should not be nil when API key is set")
	}

	p2 := NewHybridProvider("finmind-key", "")
	if p2.Name() != "hybrid-fubon" && p2.Name() != "hybrid-finmind" {
		t.Fatalf("Name() = %q, want hybrid-fubon or hybrid-finmind", p2.Name())
	}

	p3 := NewHybridProvider("finmind-key", "fugle-key")
	if p3.Name() != "hybrid-fubon" && p3.Name() != "hybrid-finmind" && p3.Name() != "hybrid-fugle" {
		t.Fatalf("Name() = %q, want hybrid-fubon, hybrid-finmind, or hybrid-fugle", p3.Name())
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

	// When Fubon proxy is not reachable, the primary mode is fugle or twse.
	fubonPrimary := p.Name() == "hybrid-fubon"

	// UseTWSE sets the circuit breaker state but doesn't clear providers.
	// Without Fubon, Name() reflects the next available provider (e.g., fugle).
	p.UseTWSE()
	// Accept any name since UseTWSE only affects circuit state, not provider names.
	_ = p.Name()

	p.UseFugle()
	if !fubonPrimary && p.Name() != "hybrid-fugle" {
		t.Fatalf("after UseFugle: Name() = %q, want hybrid-fugle (Fubon proxy not reachable)", p.Name())
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
	payload := TWSEDailyResponse{
		Stat: "OK",
		Data: [][]string{
			{"2330", "台積電", "15000000", "11700000000", "780.00", "790.00", "775.00", "785.00", "+5.00", "100000"},
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
	payload := TWSEDailyResponse{
		Stat: "OK",
		Data: [][]string{
			{"2330", "台積電", "1000", "785000", "780.00", "790.00", "775.00", "785.00", "+5.00", "500"},
			{"2317", "鴻海", "2000", "324000", "160.00", "164.00", "159.00", "162.00", "+2.00", "800"},
			{"0050", "元大台灣50", "5000", "960000", "191.00", "193.00", "190.00", "192.00", "+1.00", "1200"},
		},
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

// ─── Fubon Circuit Breaker Integration ───────────────────────────────────────

func TestHybridProvider_FubonBreaker_OpensAfter3Failures(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	fc := &FubonClient{}
	fc.httpClient = srv.Client()
	fc.proxyURL = srv.URL
	fc.intradayLimiter = rate.NewLimiter(rate.Inf, 100)

	fp := NewFubonProviderWithClient(fc)

	p := &HybridProvider{
		fubonProvider: fp,
		twseClient:    GetSharedTWSEClient(),
		breakers: map[string]*providerBreaker{
			"fugle": newProviderBreaker("fugle", defaultCircuitBreakerConfig()),
			"fubon": newProviderBreaker("fubon", defaultCircuitBreakerConfig()),
		},
	}

	ctx := context.Background()
	now := time.Now()

	for range 3 {
		_, _ = p.GetQuotes(ctx, now, []string{"2330"})
	}

	if p.breakers["fubon"].stateSnapshot().State != ProviderCircuitOpen {
		t.Fatalf("expected fubon breaker Open, got %s", p.breakers["fubon"].stateSnapshot().State)
	}
	if p.breakers["fugle"].stateSnapshot().State != ProviderCircuitClosed {
		t.Fatalf("expected fugle breaker Closed (independent), got %s", p.breakers["fugle"].stateSnapshot().State)
	}
	if callCount != 3 {
		t.Fatalf("expected Fubon proxy called 3 times, got %d", callCount)
	}
}

func TestHybridProvider_FubonBreaker_OpenSkipsFubonCall(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	fc := &FubonClient{}
	fc.httpClient = srv.Client()
	fc.proxyURL = srv.URL
	fc.intradayLimiter = rate.NewLimiter(rate.Inf, 100)

	fp := NewFubonProviderWithClient(fc)

	p := &HybridProvider{
		fubonProvider: fp,
		twseClient:    GetSharedTWSEClient(),
		breakers: map[string]*providerBreaker{
			"fugle": newProviderBreaker("fugle", defaultCircuitBreakerConfig()),
			"fubon": newProviderBreaker("fubon", defaultCircuitBreakerConfig()),
		},
	}

	p.breakers["fubon"].forceState(ProviderCircuitOpen)

	ctx := context.Background()
	now := time.Now()

	_, _ = p.GetQuotes(ctx, now, []string{"2330"})

	if callCount != 0 {
		t.Fatalf("expected Fubon proxy NOT called (breaker Open), got %d calls", callCount)
	}
}

func TestHybridProvider_CircuitBreakerStats_IncludesAllProviders(t *testing.T) {
	fc := &FubonClient{}
	fc.intradayLimiter = rate.NewLimiter(rate.Inf, 100)

	fp := NewFubonProviderWithClient(fc)

	p := &HybridProvider{
		fugleProvider: &FugleProvider{},
		fubonProvider: fp,
		twseClient:    GetSharedTWSEClient(),
		breakers: map[string]*providerBreaker{
			"fugle": newProviderBreaker("fugle", defaultCircuitBreakerConfig()),
			"fubon": newProviderBreaker("fubon", defaultCircuitBreakerConfig()),
		},
	}

	stats := p.CircuitBreakerStats()

	providers, ok := stats["providers"].(map[string]ProviderBreakerInfo)
	if !ok {
		t.Fatal("stats must contain 'providers' key with map value")
	}
	if _, ok := providers["fugle"]; !ok {
		t.Fatal("providers map must contain 'fugle' key")
	}
	if _, ok := providers["fubon"]; !ok {
		t.Fatal("providers map must contain 'fubon' key")
	}
	// backward-compat top-level keys
	if _, ok := stats["state"]; !ok {
		t.Fatal("stats must contain backward-compat 'state' key")
	}
	if _, ok := stats["failure_count"]; !ok {
		t.Fatal("stats must contain backward-compat 'failure_count' key")
	}
}
