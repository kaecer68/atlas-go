package metrics

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry holds MCP-specific Prometheus metrics. It wraps
// prometheus.Registry and provides RecordCall for integration with the
// audit log path (cmd/atlas-mcp/server/tools.go:withAudit).
//
// A nil *Registry is safe to use — RecordCall and Handler are no-ops
// when the receiver is nil (useful when MetricsAddr is empty).
type Registry struct {
	callsTotal   *prometheus.CounterVec
	callDuration *prometheus.HistogramVec
	reg          *prometheus.Registry
}

// NewRegistry creates a Registry with the standard MCP metrics registered:
//   - mcp_calls_total{tool, transport, status} — Counter
//   - mcp_call_duration_seconds{tool, transport} — Histogram
func NewRegistry() *Registry {
	reg := prometheus.NewRegistry()
	callsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mcp_calls_total",
		Help: "Total number of MCP tool calls.",
	}, []string{"tool", "transport", "status"})

	callDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "mcp_call_duration_seconds",
		Help: "Duration of MCP tool calls in seconds.",
		// prometheus.DefBuckets = .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10
		Buckets: prometheus.DefBuckets,
	}, []string{"tool", "transport"})

	reg.MustRegister(callsTotal)
	reg.MustRegister(callDuration)

	return &Registry{
		callsTotal:   callsTotal,
		callDuration: callDuration,
		reg:          reg,
	}
}

// RecordCall records an MCP tool call. It is safe to call on a nil
// receiver (no-op). durationMs is the call latency in milliseconds;
// it is converted to seconds for the histogram.
func (r *Registry) RecordCall(tool, transport, status string, durationMs int64) {
	if r == nil {
		return
	}
	r.callsTotal.WithLabelValues(tool, transport, status).Inc()
	r.callDuration.WithLabelValues(tool, transport).Observe(float64(durationMs) / 1000.0)
}

// Handler returns an http.Handler serving Prometheus text format for the
// /metrics endpoint. Returns http.NotFoundHandler when r is nil.
func (r *Registry) Handler() http.Handler {
	if r == nil {
		return http.NotFoundHandler()
	}
	return promhttp.HandlerFor(r.reg, promhttp.HandlerOpts{})
}

// StartServer starts the metrics HTTP server at addr. It blocks until ctx
// is cancelled, then shuts down gracefully with a 5-second deadline.
// Returns nil when shutdown completes cleanly.
func (r *Registry) StartServer(ctx context.Context, addr string) error {
	if r == nil {
		return fmt.Errorf("metrics: cannot start server with nil registry")
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", r.Handler())

	srv := &http.Server{
		Addr:        addr,
		Handler:     mux,
		BaseContext: func(_ net.Listener) context.Context { return ctx },
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("metrics: listen on %s: %w", addr, err)
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
