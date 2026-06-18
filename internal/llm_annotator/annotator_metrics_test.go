package llm_annotator

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// newMetricsTestClient builds a KimiClient wired to an httptest server with
// the given handler and a countingMetrics ready for assertion. cacheTTL=0
// keeps every test deterministic (no cache interference across tests).
func newMetricsTestClient(t *testing.T, handler http.Handler) (*KimiClient, *countingMetrics) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	m := newCountingMetrics()
	c, err := NewKimiClient(Config{
		APIKey:    "test-key",
		BaseURL:   srv.URL,
		Metrics:   m,
		Timeout:   2 * time.Second,
		MaxTokens: 256,
	})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	c.cacheTTL = 0
	return c, m
}

// successResponse returns a 200 OK chat-completions payload with the given content.
func successResponse(content string) string {
	return `{"choices":[{"message":{"role":"assistant","content":"` + content + `"}}],"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19}}`
}

func TestKimiClient_NilMetrics_DoesNotPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(successResponse("ok")))
	}))
	defer srv.Close()
	c, err := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL, Metrics: nil})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	c.cacheTTL = 0
	if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"}); err != nil {
		t.Fatalf("Annotate with nil Metrics: %v", err)
	}
}

func TestKimiClient_Annotate_RecordsSuccessMetric(t *testing.T) {
	c, m := newMetricsTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(successResponse("ok")))
	}))
	if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"}); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	labels := map[string]string{"provider": "kimi", "outcome": "success"}
	if got := m.CounterValue("llm_annotator_requests_total", labels); got != 1 {
		t.Errorf("success counter = %v, want 1", got)
	}
}

func TestKimiClient_Annotate_RecordsCacheHitMetric(t *testing.T) {
	// Re-enable cache so we can observe hit.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(successResponse("ok")))
	}))
	defer srv.Close()
	m := newCountingMetrics()
	c, err := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL, Metrics: m})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	fc := FailureContext{FrameID: "x", OccurredAt: time.Unix(1700000000, 0)}
	if _, err := c.Annotate(context.Background(), fc); err != nil {
		t.Fatalf("first Annotate: %v", err)
	}
	// Second call must be a cache hit.
	if _, err := c.Annotate(context.Background(), fc); err != nil {
		t.Fatalf("second Annotate: %v", err)
	}
	labels := map[string]string{"provider": "kimi", "outcome": "cache_hit"}
	if got := m.CounterValue("llm_annotator_requests_total", labels); got != 1 {
		t.Errorf("cache_hit counter = %v, want 1", got)
	}
	// Successful first call must also be recorded.
	successLabels := map[string]string{"provider": "kimi", "outcome": "success"}
	if got := m.CounterValue("llm_annotator_requests_total", successLabels); got != 1 {
		t.Errorf("success counter = %v, want 1", got)
	}
}

func TestKimiClient_Annotate_RecordsRateLimitedMetric(t *testing.T) {
	c, m := newMetricsTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":"rate limited"}`))
	}))
	_, _ = c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	labels := map[string]string{"provider": "kimi", "outcome": "retry"}
	if got := m.CounterValue("llm_annotator_requests_total", labels); got < 1 {
		t.Errorf("retry counter = %v, want >= 1 (after 3 attempts)", got)
	}
}

func TestKimiClient_Annotate_RecordsClientErrorMetric(t *testing.T) {
	c, m := newMetricsTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	_, _ = c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	labels := map[string]string{"provider": "kimi", "outcome": "client_error", "status": "400"}
	if got := m.CounterValue("llm_annotator_requests_total", labels); got != 1 {
		t.Errorf("client_error counter = %v, want 1", got)
	}
}

func TestKimiClient_Annotate_RecordsTransportErrorMetric(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Force a transport error by closing the connection without writing.
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer srv.Close()
	m := newCountingMetrics()
	c, err := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL, Metrics: m, Timeout: 500 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	c.cacheTTL = 0
	_, _ = c.Annotate(context.Background(), FailureContext{FrameID: "x"})
	labels := map[string]string{"provider": "kimi", "outcome": "transport_error"}
	if got := m.CounterValue("llm_annotator_requests_total", labels); got < 1 {
		t.Errorf("transport_error counter = %v, want >= 1", got)
	}
}

func TestKimiClient_Latency_ReportsLastCall(t *testing.T) {
	c, _ := newMetricsTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Write([]byte(successResponse("ok")))
	}))
	if c.Latency() != 0 {
		t.Errorf("Latency before any call = %v, want 0", c.Latency())
	}
	if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"}); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	got := c.Latency()
	if got < 50*time.Millisecond {
		t.Errorf("Latency = %v, want >= 50ms (handler sleeps 50ms)", got)
	}
	if got > 2*time.Second {
		t.Errorf("Latency = %v, want < 2s", got)
	}
}

func TestKimiClient_Annotate_RecordsLatencyGauge(t *testing.T) {
	c, m := newMetricsTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(successResponse("ok")))
	}))
	if _, err := c.Annotate(context.Background(), FailureContext{FrameID: "x"}); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	labels := map[string]string{"provider": "kimi"}
	if got := m.GaugeValue("llm_annotator_last_call_seconds", labels); got <= 0 {
		t.Errorf("latency gauge = %v, want > 0", got)
	}
}

func TestKimiClient_Usage_ConcurrentWithRecordUsage(t *testing.T) {
	// Fire concurrent Usage() reads and Annotate() calls so -race detects
	// the previously-unprotected k.usage read.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(successResponse("ok")))
	}))
	defer srv.Close()
	c, err := NewKimiClient(Config{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewKimiClient: %v", err)
	}
	c.cacheTTL = 0

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(3)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_, _ = c.Annotate(context.Background(), FailureContext{FrameID: "x"})
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = c.Usage()
			}
		}
	}()
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = c.UsageAll()
			}
		}
	}()
	time.Sleep(200 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// silence unused imports for variants we may remove during refactor.
var _ = errors.Is
