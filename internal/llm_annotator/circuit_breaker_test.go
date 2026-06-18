package llm_annotator

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestKimiClient_BreakerNil_DoesNotPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(successResponse("ok")))
	}))
	defer srv.Close()
	c, err := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	c.cacheTTL = 0
	if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"}); err != nil {
		t.Fatalf("Annotate with nil breaker: %v", err)
	}
	if c.BreakerState() != "disabled" {
		t.Errorf("BreakerState = %q, want disabled", c.BreakerState())
	}
}

func TestKimiClient_BreakerOpensAfterRepeatedFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"boom"}`))
	}))
	defer srv.Close()
	c, err := NewKimiClient(Config{
		APIKey: "k", BaseURL: srv.URL,
		Breaker: newCircuitBreaker(),
	})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	c.cacheTTL = 0

	// 3 consecutive breaker failures (= 3 Annotate calls each exhausting
	// their retry budget) opens the breaker.
	for i := 0; i < cbFailureThreshold; i++ {
		_, _ = c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	}
	if got := c.BreakerState(); got != "open" {
		t.Errorf("BreakerState after %d failures = %q, want open", cbFailureThreshold, got)
	}

	// Next call: short-circuits with circuit_open metric, no HTTP call.
	var hits int32
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write([]byte(successResponse("ok")))
	}))
	defer srv2.Close()
	c.cfg.BaseURL = srv2.URL
	_, _ = c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	if got := atomic.LoadInt32(&hits); got != 0 {
		t.Errorf("HTTP server hit count = %d while breaker open, want 0", got)
	}
}

func TestKimiClient_BreakerRecoversOnSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c, err := NewKimiClient(Config{
		APIKey: "k", BaseURL: srv.URL,
		Breaker: newCircuitBreaker(),
	})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	c.cacheTTL = 0

	for i := 0; i < cbFailureThreshold; i++ {
		_, _ = c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	}
	if c.BreakerState() != "open" {
		t.Fatalf("setup: breaker state = %q, want open", c.BreakerState())
	}

	// Manual reset + point to a healthy server simulates recovery.
	c.breaker.Reset()
	c.cfg.BaseURL = newHealthyServer(t)
	if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"}); err != nil {
		t.Fatalf("Annotate on healthy server: %v", err)
	}
	if got := c.BreakerState(); got != "closed" {
		t.Errorf("BreakerState after success = %q, want closed", got)
	}
}

func TestKimiClient_BudgetForcesBreakerOpen(t *testing.T) {
	var fired int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(successResponse("ok")))
	}))
	defer srv.Close()
	c, err := NewKimiClient(Config{
		APIKey:          "k",
		BaseURL:         srv.URL,
		BudgetThreshold: 19,
		BudgetCallback:  func(u Usage) { atomic.StoreInt32(&fired, 1) },
		Breaker:         newCircuitBreaker(),
	})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	c.cacheTTL = 0

	if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"}); err != nil {
		t.Fatalf("first Annotate: %v", err)
	}
	if atomic.LoadInt32(&fired) != 1 {
		t.Fatalf("budget callback not invoked after first call")
	}
	if got := c.BreakerState(); got != "open" {
		t.Errorf("BreakerState after budget fire = %q, want open (ForceOpen wired)", got)
	}
}

func TestKimiClient_Annotate_RecordsCircuitOpenMetric(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	m := newCountingMetrics()
	c, err := NewKimiClient(Config{
		APIKey: "k", BaseURL: srv.URL,
		Metrics: m,
		Breaker: newCircuitBreaker(),
	})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	c.cacheTTL = 0

	for i := 0; i < cbFailureThreshold; i++ {
		_, _ = c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	}
	_, _ = c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	labels := map[string]string{"provider": "kimi", "outcome": "circuit_open"}
	if got := m.CounterValue("llm_annotator_requests_total", labels); got < 1 {
		t.Errorf("circuit_open counter = %v, want >= 1", got)
	}
}

// newHealthyServer returns the URL of an httptest server that always 200s.
func newHealthyServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(successResponse("ok")))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}
