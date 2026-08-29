package clients

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// TestBaseClient_RetryOn5xx verifies that a transient 5xx error triggers
// a retry and the second attempt succeeds. The test server returns 500 on
// the first request and 200 on subsequent requests.
func TestBaseClient_RetryOn5xx(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`"ok"`))
	}))
	defer srv.Close()

	client := NewBaseClient("mock", BaseClientConfig{
		MaxAttempts: 3,
	})
	client.HTTPClient = srv.Client()

	resp, body, err := client.DoRequest(context.Background(), http.MethodPost, srv.URL, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if string(body) != `"ok"` {
		t.Errorf("expected body %q, got %q", `"ok"`, string(body))
	}
	if n := attempts.Load(); n != 2 {
		t.Errorf("expected 2 attempts, got %d", n)
	}
}

// TestBaseClient_FailFastOn4xx verifies that a non-retryable 4xx error
// (excluding 429) fails immediately without retrying.
func TestBaseClient_FailFastOn4xx(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	client := NewBaseClient("mock", BaseClientConfig{
		MaxAttempts: 3,
	})
	client.HTTPClient = srv.Client()

	_, _, err := client.DoRequest(context.Background(), http.MethodPost, srv.URL, nil, nil)
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if n := attempts.Load(); n != 1 {
		t.Errorf("expected exactly 1 attempt, got %d", n)
	}
}

// TestBaseClient_RateLimit verifies that the rate limiter gates concurrent
// calls. The limiter is configured to 10 req/s with zero burst, so 3
// sequential calls must take at least ~200ms.
func TestBaseClient_RateLimit(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`"ok"`))
	}))
	defer srv.Close()

	// 5 req/s with burst 1: first call uses burst (instant),
	// then each subsequent call waits ~200ms for the next token.
	client := NewBaseClient("mock", BaseClientConfig{
		RatePerSecond: 5,
		Burst:         1,
		MaxAttempts:   1,
	})
	client.HTTPClient = srv.Client()

	start := time.Now()
	for i := range 3 {
		_, _, err := client.DoRequest(context.Background(), http.MethodPost, srv.URL, nil, nil)
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i+1, err)
		}
	}
	elapsed := time.Since(start)

	if n := calls.Load(); n != 3 {
		t.Errorf("expected 3 server hits, got %d", n)
	}
	// 3 calls at 5/s with burst 1: minimum ~300ms.
	// Allow 150ms floor to account for scheduling variance.
	if elapsed < 150*time.Millisecond {
		t.Errorf("rate limiter appears ineffective: 3 calls completed in %v (expected >= ~400ms)", elapsed)
	}
}

// TestBaseClient_CircuitBreakerOpen verifies that when the circuit breaker
// is ForceOpen, DoRequest returns ErrCircuitOpen immediately without making
// an HTTP request.
func TestBaseClient_CircuitBreakerOpen(t *testing.T) {
	var called atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewBaseClient("mock", BaseClientConfig{})
	client.HTTPClient = srv.Client()
	client.Breaker.ForceOpen()

	_, _, err := client.DoRequest(context.Background(), http.MethodPost, srv.URL, nil, nil)
	if err == nil {
		t.Fatal("expected error when circuit breaker is open")
	}
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
	if called.Load() {
		t.Error("server was called despite circuit breaker being open")
	}
}
