package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWithTimeout_Triggers503(t *testing.T) {
	// Use a short timeout so the test runs quickly while still exercising the
	// "request duration exceeds deadline" path.
	cfg := timeoutConfig{
		DefaultTimeout: 50 * time.Millisecond,
	}

	blocked := make(chan struct{})
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block longer than the deadline.
		select {
		case <-r.Context().Done():
			// Handler should observe cancellation propagated by the middleware.
		case <-time.After(5 * time.Second):
		}
		close(blocked)
	})

	mux := http.NewServeMux()
	mux.Handle("/slow", inner)

	server := httptest.NewServer(withTimeout(mux, cfg))
	defer server.Close()

	start := time.Now()
	resp, err := http.Get(server.URL + "/slow")
	if err != nil {
		t.Fatalf("http.Get failed: %v", err)
	}
	defer resp.Body.Close()

	elapsed := time.Since(start)
	if elapsed > 8*time.Second {
		t.Fatalf("timeout middleware took too long: %v", elapsed)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["error"] != "upstream timeout" {
		t.Errorf("body.error = %q, want upstream timeout", body["error"])
	}
	if degraded, ok := body["degraded"].(bool); !ok || !degraded {
		t.Errorf("body.degraded = %v, want true", body["degraded"])
	}

	select {
	case <-blocked:
		// Handler observed cancellation and returned.
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not unblock after context cancellation")
	}
}

func TestWithTimeout_WhitelistedSSEExempt(t *testing.T) {
	cfg := timeoutConfig{
		DefaultTimeout: 50 * time.Millisecond,
		Overrides: map[string]time.Duration{
			"/test/sse-fake": 0,
		},
	}

	var calls atomic.Int32
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		// Sleep longer than the default timeout; if the middleware applied a
		// deadline this request would be cut off.
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	mux := http.NewServeMux()
	mux.Handle("/test/sse-fake", inner)

	server := httptest.NewServer(withTimeout(mux, cfg))
	defer server.Close()

	resp, err := http.Get(server.URL + "/test/sse-fake")
	if err != nil {
		t.Fatalf("http.Get failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if calls.Load() != 1 {
		t.Errorf("handler calls = %d, want 1", calls.Load())
	}
}

func TestWithTimeout_TaskEventsSSEExempt(t *testing.T) {
	cfg := timeoutConfig{
		DefaultTimeout: 50 * time.Millisecond,
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	mux := http.NewServeMux()
	mux.Handle("/api/tasks/abc-123/events", inner)

	server := httptest.NewServer(withTimeout(mux, cfg))
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/tasks/abc-123/events")
	if err != nil {
		t.Fatalf("http.Get failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestWithTimeout_BacktestWhitelistedLongTimeout(t *testing.T) {
	cfg := timeoutConfig{
		DefaultTimeout: 50 * time.Millisecond,
		Overrides: map[string]time.Duration{
			"/api/backtest/run": 120 * time.Millisecond,
		},
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleeps longer than default but shorter than whitelist duration.
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusAccepted)
	})

	mux := http.NewServeMux()
	mux.Handle("/api/backtest/run", inner)

	server := httptest.NewServer(withTimeout(mux, cfg))
	defer server.Close()

	resp, err := http.Post(server.URL+"/api/backtest/run", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("http.Post failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusAccepted)
	}
}

func TestWithTimeout_FastRoutePassesThrough(t *testing.T) {
	cfg := timeoutConfig{
		DefaultTimeout: 1 * time.Second,
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pong"))
	})

	mux := http.NewServeMux()
	mux.Handle("/ping", inner)

	server := httptest.NewServer(withTimeout(mux, cfg))
	defer server.Close()

	resp, err := http.Get(server.URL + "/ping")
	if err != nil {
		t.Fatalf("http.Get failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
