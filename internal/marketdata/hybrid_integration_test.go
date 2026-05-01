package marketdata

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

type mockRoundTripper struct {
	serverURL string
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	parsed, _ := url.Parse(m.serverURL)
	req.URL.Scheme = parsed.Scheme
	req.URL.Host = parsed.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestHybridProvider_GetQuotes_FinMindSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"msg":"ok","status":200,"data":[{"close":100,"open":99,"max":101,"min":98,"Trading_Volume":1000}]}`)
	}))
	defer srv.Close()

	client := NewFinMindClient("test-key")
	client.SetHTTPClient(&http.Client{
		Transport: &mockRoundTripper{serverURL: srv.URL},
	})

	hp := NewHybridProvider("", "")
	hp.finmindProvider = NewFinMindProviderWithClient(client)
	hp.finmindCB = NewCircuitBreaker(defaultCircuitBreakerConfig())

	quotes, err := hp.GetQuotes(context.Background(), time.Now(), []string{"2330"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(quotes) != 1 {
		t.Fatalf("expected 1 quote, got %d", len(quotes))
	}
	if quotes[0].Symbol != "2330" {
		t.Errorf("symbol = %q, want 2330", quotes[0].Symbol)
	}
	if quotes[0].Last != 100.0 {
		t.Errorf("last = %f, want 100.0", quotes[0].Last)
	}
}

func TestHybridProvider_GetQuotes_FinMindFails_FallsBackToFugle(t *testing.T) {
	finmindSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer finmindSrv.Close()

	fugleSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"apiVersion":"1.0","data":{"info":{"symbol":"2330"},"quote":{"trade":{"price":150.0},"priceOpen":{"price":149.0},"priceHigh":{"price":151.0},"priceLow":{"price":148.0},"total":{"tradeVolume":5000}}}}`)
	}))
	defer fugleSrv.Close()

	finmindClient := NewFinMindClient("test-key")
	finmindClient.SetHTTPClient(&http.Client{
		Transport: &mockRoundTripper{serverURL: finmindSrv.URL},
	})

	fugleClient := NewFugleClient("test-key")
	fugleClient.SetHTTPClient(&http.Client{
		Transport: &mockRoundTripper{serverURL: fugleSrv.URL},
	})

	hp := NewHybridProvider("", "")
	hp.finmindProvider = NewFinMindProviderWithClient(finmindClient)
	hp.finmindCB = NewCircuitBreaker(defaultCircuitBreakerConfig())
	hp.fugleProvider = NewFugleProviderWithClient(fugleClient)
	hp.fugleCB = NewCircuitBreaker(defaultCircuitBreakerConfig())

	quotes, err := hp.GetQuotes(context.Background(), time.Now(), []string{"2330"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(quotes) == 0 {
		t.Fatal("expected quotes from fallback, got none")
	}
}

func TestHybridProvider_GetQuotes_FinMindOpen_FugleClosed(t *testing.T) {
	hp := NewHybridProvider("finmind-key", "fugle-key")

	hp.finmindCB.RecordFailure()
	hp.finmindCB.RecordFailure()
	hp.finmindCB.RecordFailure()

	if hp.finmindCB.State() != ProviderCircuitOpen {
		t.Fatal("expected finmindCB to be open")
	}

	if !hp.fugleCB.Allow() {
		t.Error("expected fugleCB.Allow()=true (independent from FinMind)")
	}

	if hp.IsUsingTWSE() {
		t.Error("expected IsUsingTWSE()=false when Fugle is still available")
	}
}

func TestHybridProvider_GetQuotes_FallbackCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := NewFinMindClient("test-key")
	client.SetHTTPClient(&http.Client{
		Transport: &mockRoundTripper{serverURL: srv.URL},
	})

	hp := NewHybridProvider("", "")
	hp.finmindProvider = NewFinMindProviderWithClient(client)
	hp.finmindCB = NewCircuitBreaker(defaultCircuitBreakerConfig())

	initialCount := hp.fallbackCount

	_, _ = hp.GetQuotes(context.Background(), time.Now(), []string{"2330"})

	if hp.fallbackCount <= initialCount {
		t.Errorf("expected fallbackCount to increase, got %d", hp.fallbackCount)
	}
}

func TestHybridProvider_GetQuotes_CircuitBreakerStats(t *testing.T) {
	hp := NewHybridProvider("finmind-key", "fugle-key")

	stats := hp.CircuitBreakerStats()

	if _, ok := stats["finmind_state"]; !ok {
		t.Error("expected finmind_state in stats")
	}
	if _, ok := stats["fugle_state"]; !ok {
		t.Error("expected fugle_state in stats")
	}
	if stats["finmind_state"] != string(ProviderCircuitClosed) {
		t.Errorf("finmind_state = %q, want %q", stats["finmind_state"], ProviderCircuitClosed)
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker(defaultCircuitBreakerConfig())

	done := make(chan bool, 100)
	for i := 0; i < 50; i++ {
		go func() {
			cb.Allow()
			done <- true
		}()
		go func() {
			cb.RecordFailure()
			done <- true
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	if cb.State() != ProviderCircuitOpen {
		t.Errorf("after concurrent failures: state = %q, want %q", cb.State(), ProviderCircuitOpen)
	}
}

func TestHybridProvider_GetQuotes_AllProvidersFail(t *testing.T) {
	hp := NewHybridProvider("finmind-key", "fugle-key")

	hp.finmindCB.ForceOpen()
	hp.fugleCB.ForceOpen()

	if !hp.IsUsingTWSE() {
		t.Error("expected IsUsingTWSE()=true when both CBs are open")
	}
}

func TestCircuitBreaker_AllowAfterReset(t *testing.T) {
	cb := NewCircuitBreaker(circuitBreakerConfig{
		failureThreshold: 1,
		recoveryTimeout:  5 * time.Minute,
		halfOpenMaxCalls: 1,
	})

	cb.RecordFailure()
	if cb.State() != ProviderCircuitOpen {
		t.Fatal("expected open")
	}

	cb.Reset()
	if !cb.Allow() {
		t.Error("expected Allow()=true after reset")
	}
	if cb.State() != ProviderCircuitClosed {
		t.Errorf("after reset: state = %q, want %q", cb.State(), ProviderCircuitClosed)
	}
}

func TestCircuitBreaker_StatsLastFailureFormat(t *testing.T) {
	cb := NewCircuitBreaker(defaultCircuitBreakerConfig())

	before := time.Now()
	cb.RecordFailure()
	after := time.Now()

	stats := cb.Stats()
	lastFailureStr, ok := stats["last_failure"].(string)
	if !ok {
		t.Fatal("expected last_failure to be string")
	}

	parsed, err := time.Parse(time.RFC3339, lastFailureStr)
	if err != nil {
		t.Fatalf("failed to parse last_failure: %v", err)
	}

	if parsed.Before(before.Add(-time.Second)) || parsed.After(after.Add(time.Second)) {
		t.Errorf("last_failure time %v not in expected range [%v, %v]", parsed, before, after)
	}
}

func TestCircuitBreaker_HalfOpenMultipleCycles(t *testing.T) {
	cb := NewCircuitBreaker(circuitBreakerConfig{
		failureThreshold: 1,
		recoveryTimeout:  50 * time.Millisecond,
		halfOpenMaxCalls: 1,
	})

	for cycle := 0; cycle < 3; cycle++ {
		cb.RecordFailure()
		if cb.State() != ProviderCircuitOpen {
			t.Fatalf("cycle %d: expected open after failure", cycle)
		}

		time.Sleep(100 * time.Millisecond)

		if !cb.Allow() {
			t.Fatalf("cycle %d: expected allow after timeout", cycle)
		}
		if cb.State() != ProviderCircuitHalfOpen {
			t.Fatalf("cycle %d: expected half-open", cycle)
		}

		cb.RecordFailure()
		if cb.State() != ProviderCircuitOpen {
			t.Fatalf("cycle %d: expected open after half-open failure", cycle)
		}
	}

	time.Sleep(100 * time.Millisecond)
	cb.Allow()
	cb.RecordSuccess()
	if cb.State() != ProviderCircuitClosed {
		t.Errorf("after final success: state = %q, want %q", cb.State(), ProviderCircuitClosed)
	}
}

func TestHybridProvider_GetQuotes_IndependentFallback(t *testing.T) {
	hp := NewHybridProvider("finmind-key", "fugle-key")

	hp.finmindCB.RecordFailure()
	hp.finmindCB.RecordFailure()
	hp.finmindCB.RecordFailure()

	if hp.finmindCB.State() != ProviderCircuitOpen {
		t.Fatal("expected finmindCB open")
	}

	if hp.fugleCB.State() != ProviderCircuitClosed {
		t.Fatalf("expected fugleCB closed, got %q", hp.fugleCB.State())
	}

	if hp.IsUsingTWSE() {
		t.Error("expected IsUsingTWSE()=false: Fugle should still be available when FinMind is open")
	}

	hp.fugleCB.RecordFailure()
	hp.fugleCB.RecordFailure()
	hp.fugleCB.RecordFailure()

	if hp.fugleCB.State() != ProviderCircuitOpen {
		t.Fatal("expected fugleCB open")
	}

	if !hp.IsUsingTWSE() {
		t.Error("expected IsUsingTWSE()=true: both CBs open, should fall back to TWSE")
	}
}

func TestCircuitBreaker_DefaultBranch(t *testing.T) {
	cb := NewCircuitBreaker(defaultCircuitBreakerConfig())

	cb.mu.Lock()
	cb.state = ProviderCircuitState("invalid")
	cb.mu.Unlock()

	if cb.Allow() {
		t.Error("expected Allow()=false for invalid state")
	}
}
