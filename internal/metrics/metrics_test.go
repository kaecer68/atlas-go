package metrics

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMetrics_RecordCallUpdatesCounter(t *testing.T) {
	reg := NewRegistry()

	// 3 statuses (ok/error/ratelimited) × 2 transports (stdio/sse) = 6 combos
	reg.RecordCall("macro_get_snapshot", "stdio", "ok", 42)
	reg.RecordCall("macro_get_snapshot", "stdio", "ok", 55)
	reg.RecordCall("macro_get_snapshot", "stdio", "error", 120)
	reg.RecordCall("macro_get_snapshot", "sse", "ok", 30)
	reg.RecordCall("macro_get_snapshot", "sse", "error", 100)
	reg.RecordCall("macro_get_snapshot", "sse", "ok", 33)

	body := getMetricsBody(t, reg)
	// prometheus sorts labels alphabetically: status, tool, transport
	mustContain(t, body, `mcp_calls_total{status="ok",tool="macro_get_snapshot",transport="stdio"} 2`)
	mustContain(t, body, `mcp_calls_total{status="error",tool="macro_get_snapshot",transport="stdio"} 1`)
	mustContain(t, body, `mcp_calls_total{status="ok",tool="macro_get_snapshot",transport="sse"} 2`)
	mustContain(t, body, `mcp_calls_total{status="error",tool="macro_get_snapshot",transport="sse"} 1`)
}

func TestMetrics_DurationHistogramBuckets(t *testing.T) {
	reg := NewRegistry()

	// durations in ms → seconds: 5→0.005, 50→0.05, 500→0.5, 5000→5.0
	reg.RecordCall("tool_a", "stdio", "ok", 5)
	reg.RecordCall("tool_a", "stdio", "ok", 50)
	reg.RecordCall("tool_a", "stdio", "ok", 500)
	reg.RecordCall("tool_a", "stdio", "ok", 5000)

	body := getMetricsBody(t, reg)
	// Sum: 0.005 + 0.05 + 0.5 + 5.0 = 5.555
	mustContain(t, body, `mcp_call_duration_seconds_sum{tool="tool_a",transport="stdio"}`)
	mustContain(t, body, `mcp_call_duration_seconds_count{tool="tool_a",transport="stdio"} 4`)
	// Bucket labels should be present
	mustContain(t, body, `mcp_call_duration_seconds_bucket{tool="tool_a",transport="stdio",le="`)
}

func TestMetrics_HandlerReturnsPrometheusFormat(t *testing.T) {
	reg := NewRegistry()
	reg.RecordCall("test_tool", "stdio", "ok", 10)

	body := getMetricsBody(t, reg)

	mustContain(t, body, "# HELP mcp_calls_total")
	mustContain(t, body, "# TYPE mcp_calls_total counter")
	mustContain(t, body, "# HELP mcp_call_duration_seconds")
	mustContain(t, body, "# TYPE mcp_call_duration_seconds histogram")
}

func TestMetrics_NilRegistryIsSafe(t *testing.T) {
	var reg *Registry

	// RecordCall on nil must not panic
	reg.RecordCall("foo", "bar", "ok", 100)

	// Handler on nil returns non-nil handler (NotFoundHandler)
	h := reg.Handler()
	if h == nil {
		t.Errorf("nil registry should return a non-nil handler")
	}

	// StartServer on nil must return error
	if err := reg.StartServer(context.Background(), "127.0.0.1:0"); err == nil {
		t.Errorf("expected error from nil registry StartServer")
	}
}

func TestMetrics_ConcurrentSafe(t *testing.T) {
	reg := NewRegistry()
	const (
		goroutines = 50
		callsPer   = 200
	)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < callsPer; i++ {
				status := "ok"
				if i%10 == 0 {
					status = "error"
				}
				reg.RecordCall("concurrent_tool", "stdio", status, 50)
			}
		}()
	}
	wg.Wait()

	body := getMetricsBody(t, reg)
	// All calls should be counted (50 × 200 = 10,000)
	mustContain(t, body, `mcp_call_duration_seconds_count{tool="concurrent_tool",transport="stdio"} 10000`)
}

func TestMetrics_EndpointBinds127(t *testing.T) {
	reg := NewRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- reg.StartServer(ctx, "127.0.0.1:0")
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		// Shutdown should be clean; accept nil or ErrServerClosed
		if err != nil && err != http.ErrServerClosed &&
			!strings.Contains(err.Error(), "use of closed network connection") {
			t.Errorf("StartServer: unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Errorf("StartServer did not shut down within timeout")
	}
}

func TestMetrics_StartServerServesMetrics(t *testing.T) {
	reg := NewRegistry()
	reg.RecordCall("srv_tool", "stdio", "ok", 100)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- reg.StartServer(ctx, "127.0.0.1:19091")
	}()

	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get("http://127.0.0.1:19091/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "mcp_calls_total") {
		t.Errorf("expected mcp_calls_total in response:\n%s", string(body))
	}

	cancel()
	<-errCh
}

func TestMetrics_RecordCallMultipleTools(t *testing.T) {
	reg := NewRegistry()

	reg.RecordCall("tool_a", "stdio", "ok", 10)
	reg.RecordCall("tool_a", "stdio", "error", 20)
	reg.RecordCall("tool_b", "sse", "ok", 30)
	reg.RecordCall("tool_c", "streamable-http", "ok", 40)

	body := getMetricsBody(t, reg)
	mustContain(t, body, `{status="ok",tool="tool_a",transport="stdio"} 1`)
	mustContain(t, body, `{status="error",tool="tool_a",transport="stdio"} 1`)
	mustContain(t, body, `{status="ok",tool="tool_b",transport="sse"} 1`)
	mustContain(t, body, `{status="ok",tool="tool_c",transport="streamable-http"} 1`)
}

func TestMetrics_RatelimitedCounter(t *testing.T) {
	reg := NewRegistry()
	reg.RecordCall("hot_tool", "stdio", "ratelimited", 0)
	reg.RecordCall("hot_tool", "stdio", "ratelimited", 1)

	body := getMetricsBody(t, reg)
	mustContain(t, body, `{status="ratelimited",tool="hot_tool",transport="stdio"} 2`)
	mustContain(t, body, `mcp_call_duration_seconds_count{tool="hot_tool",transport="stdio"} 2`)
}

// --- helpers ---

func getMetricsBody(t *testing.T, reg *Registry) string {
	t.Helper()
	h := reg.Handler()
	rec := &testResponseWriter{}
	req, err := http.NewRequest("GET", "/metrics", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	h.ServeHTTP(rec, req)
	if rec.status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.status, rec.body.String())
	}
	return rec.body.String()
}

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected body to contain %q, got:\n%s", needle, haystack)
	}
}

type testResponseWriter struct {
	status int
	body   strings.Builder
	header http.Header
}

func (w *testResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}

func (w *testResponseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(b)
}

func (w *testResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}
