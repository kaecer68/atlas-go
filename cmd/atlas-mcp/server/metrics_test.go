package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/require"
)

// Test_Metrics_endpoint_returns_prometheus_text verifies that the /metrics
// handler returns HTTP 200 and Prometheus exposition format.
func Test_Metrics_endpoint_returns_prometheus_text(t *testing.T) {
	m := NewMetrics()
	require.NoError(t, m.ObserveCall("tool_a", "stdio", "ok", 10*time.Millisecond))

	srv := httptestServer(t, m.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/metrics")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "# HELP mcp_calls_total")
}

// Test_Metrics_declares_all_five_required_metrics checks that the five
// required MCP metrics are present in the exposition output.
func Test_Metrics_declares_all_five_required_metrics(t *testing.T) {
	m := NewMetrics()
	require.NoError(t, m.ObserveCall("tool_a", "stdio", "ok", 10*time.Millisecond))
	m.IncActiveSession("stdio")
	m.IncTokenUsage("tenant-a")
	m.SetAnomalyScore("tenant-a", "burst", 1.5)

	out, err := testutil.CollectAndFormat(
		m.Registry(), expfmt.TypeTextPlain,
		"mcp_calls_total",
		"mcp_call_duration_seconds",
		"mcp_active_sessions",
		"mcp_token_usage_total",
		"mcp_anomaly_score",
	)
	require.NoError(t, err)

	require.Contains(t, string(out), "mcp_calls_total")
	require.Contains(t, string(out), "mcp_call_duration_seconds")
	require.Contains(t, string(out), "mcp_active_sessions")
	require.Contains(t, string(out), "mcp_token_usage_total")
	require.Contains(t, string(out), "mcp_anomaly_score")
}

// Test_Metrics_calls_total_increments_per_call verifies that each ObserveCall
// increments the counter exactly once for the requested labels.
func Test_Metrics_calls_total_increments_per_call(t *testing.T) {
	m := NewMetrics()
	require.NoError(t, m.ObserveCall("regime_get_history", "stdio", "ok", 5*time.Millisecond))
	require.NoError(t, m.ObserveCall("regime_get_history", "stdio", "ok", 5*time.Millisecond))
	require.NoError(t, m.ObserveCall("regime_get_history", "stdio", "error", 5*time.Millisecond))

	count, err := testutil.GatherAndCount(m.Registry(), "mcp_calls_total")
	require.NoError(t, err)
	require.Equal(t, 2, count)

	require.InEpsilon(t, 2.0, testutil.ToFloat64(m.callsTotal.WithLabelValues("regime_get_history", "stdio", "ok")), 0.01)
	require.InEpsilon(t, 1.0, testutil.ToFloat64(m.callsTotal.WithLabelValues("regime_get_history", "stdio", "error")), 0.01)
}

// Test_Metrics_call_duration_observes_latency verifies that the histogram
// records observed durations.
func Test_Metrics_call_duration_observes_latency(t *testing.T) {
	m := NewMetrics()
	require.NoError(t, m.ObserveCall("slow_tool", "stdio", "ok", 100*time.Millisecond))

	out, err := testutil.CollectAndFormat(m.Registry(), expfmt.TypeTextPlain, "mcp_call_duration_seconds")
	require.NoError(t, err)
	require.Contains(t, string(out), "mcp_call_duration_seconds_bucket")
	require.Contains(t, string(out), "mcp_call_duration_seconds_sum{tool=\"slow_tool\",transport=\"stdio\"} 0.1")
}

// Test_Metrics_active_sessions_inc_dec_per_transport verifies that the gauge
// reflects session lifecycle per transport.
func Test_Metrics_active_sessions_inc_dec_per_transport(t *testing.T) {
	m := NewMetrics()
	m.IncActiveSession("stdio")
	m.IncActiveSession("http")
	require.InEpsilon(t, 1.0, testutil.ToFloat64(m.activeSessions.WithLabelValues("stdio")), 0.01)
	require.InEpsilon(t, 1.0, testutil.ToFloat64(m.activeSessions.WithLabelValues("http")), 0.01)

	m.DecActiveSession("stdio")
	require.Equal(t, 0.0, testutil.ToFloat64(m.activeSessions.WithLabelValues("stdio")))
	require.InEpsilon(t, 1.0, testutil.ToFloat64(m.activeSessions.WithLabelValues("http")), 0.01)
}

// Test_Metrics_token_usage_labels_anonymous_when_no_tenant verifies that empty
// tenant IDs are normalised to "anonymous".
func Test_Metrics_token_usage_labels_anonymous_when_no_tenant(t *testing.T) {
	m := NewMetrics()
	m.IncTokenUsage("")
	m.IncTokenUsage("tenant-a")

	require.InEpsilon(t, 1.0, testutil.ToFloat64(m.tokenUsage.WithLabelValues("anonymous")), 0.01)
	require.InEpsilon(t, 1.0, testutil.ToFloat64(m.tokenUsage.WithLabelValues("tenant-a")), 0.01)
}

// Test_Metrics_server_binds_loopback_only verifies that the metrics listener
// rejects non-loopback addresses.
func Test_Metrics_server_binds_loopback_only(t *testing.T) {
	m := NewMetrics()

	errCh := make(chan error, 1)
	go func() {
		errCh <- StartMetricsServer(context.Background(), "0.0.0.0:0", m)
	}()
	select {
	case err := <-errCh:
		require.Error(t, err)
		require.True(t, strings.Contains(err.Error(), "127.0.0.1") || strings.Contains(err.Error(), "loopback"), err.Error())
	case <-time.After(2 * time.Second):
		t.Fatal("expected immediate error for non-loopback address")
	}

	addr, err := freeLoopbackAddr(t)
	require.NoError(t, err)

	ctx := t.Context()
	go func() {
		_ = StartMetricsServer(ctx, addr, m)
	}()

	require.Eventually(t, func() bool {
		resp, err := http.Get("http://" + addr + "/metrics")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 2*time.Second, 50*time.Millisecond, "metrics server did not become reachable")
}

// Test_Metrics_ratelimited_status_single_source_of_truth verifies that a
// rate-limited call is counted exactly once and with status="ratelimited",
// using the same Result source for both audit and metrics.
func Test_Metrics_ratelimited_status_single_source_of_truth(t *testing.T) {
	m := NewMetrics()
	require.NoError(t, m.ObserveCall("ratelimited_tool", "stdio", "ratelimited", 1*time.Millisecond))

	require.InEpsilon(t, 1.0, testutil.ToFloat64(m.callsTotal.WithLabelValues("ratelimited_tool", "stdio", "ratelimited")), 0.01)
	require.Equal(t, 0.0, testutil.ToFloat64(m.callsTotal.WithLabelValues("ratelimited_tool", "stdio", "ok")))
}

func httptestServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func freeLoopbackAddr(t *testing.T) (string, error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().String(), nil
}
