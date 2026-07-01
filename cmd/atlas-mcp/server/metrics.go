package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics owns the Prometheus registry and all MCP observability metrics.
// It is safe for concurrent use.
type Metrics struct {
	registry       *prometheus.Registry
	callsTotal     *prometheus.CounterVec
	callDuration   *prometheus.HistogramVec
	activeSessions *prometheus.GaugeVec
	tokenUsage     *prometheus.CounterVec
	anomalyScore   *prometheus.GaugeVec
}

// NewMetrics builds a Metrics instance with the five required MCP metrics:
//   - mcp_calls_total{tool, transport, status}
//   - mcp_call_duration_seconds{tool, transport}
//   - mcp_active_sessions{transport}
//   - mcp_token_usage_total{tenant_id}
//   - mcp_anomaly_score{tenant_id, anomaly_type}
func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()

	callsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mcp_calls_total",
		Help: "Total MCP tool calls by tool, transport and status.",
	}, []string{"tool", "transport", "status"})

	callDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "mcp_call_duration_seconds",
		Help:    "MCP tool call latency distribution by tool and transport.",
		Buckets: prometheus.DefBuckets,
	}, []string{"tool", "transport"})

	activeSessions := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "mcp_active_sessions",
		Help: "Number of active MCP sessions by transport.",
	}, []string{"transport"})

	tokenUsage := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "mcp_token_usage_total",
		Help: "Total successful token authentications by tenant_id.",
	}, []string{"tenant_id"})

	anomalyScore := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "mcp_anomaly_score",
		Help: "Current anomaly score by tenant_id and anomaly_type.",
	}, []string{"tenant_id", "anomaly_type"})

	reg.MustRegister(callsTotal, callDuration, activeSessions, tokenUsage, anomalyScore)

	return &Metrics{
		registry:       reg,
		callsTotal:     callsTotal,
		callDuration:   callDuration,
		activeSessions: activeSessions,
		tokenUsage:     tokenUsage,
		anomalyScore:   anomalyScore,
	}
}

// ObserveCall records one tool invocation. status should come from the same
// source of truth as the audit entry (e.g. "ok", "error", "ratelimited").
func (m *Metrics) ObserveCall(tool, transport, status string, duration time.Duration) error {
	if m == nil {
		return nil
	}
	if tool == "" {
		tool = "unknown"
	}
	if transport == "" {
		transport = "unknown"
	}
	if status == "" {
		status = "unknown"
	}
	m.callsTotal.WithLabelValues(tool, transport, status).Inc()
	m.callDuration.WithLabelValues(tool, transport).Observe(duration.Seconds())
	return nil
}

// IncActiveSession increments the active session gauge for transport.
func (m *Metrics) IncActiveSession(transport string) {
	if m == nil || transport == "" {
		return
	}
	m.activeSessions.WithLabelValues(transport).Inc()
}

// DecActiveSession decrements the active session gauge for transport.
func (m *Metrics) DecActiveSession(transport string) {
	if m == nil || transport == "" {
		return
	}
	m.activeSessions.WithLabelValues(transport).Dec()
}

// IncTokenUsage increments the token usage counter for tenantID, normalising
// an empty tenant to "anonymous".
func (m *Metrics) IncTokenUsage(tenantID string) {
	if m == nil {
		return
	}
	if tenantID == "" {
		tenantID = "anonymous"
	}
	m.tokenUsage.WithLabelValues(tenantID).Inc()
}

// SetAnomalyScore sets the anomaly score gauge for a tenant and anomaly type.
func (m *Metrics) SetAnomalyScore(tenantID, anomalyType string, score float64) {
	if m == nil {
		return
	}
	if tenantID == "" {
		tenantID = "anonymous"
	}
	if anomalyType == "" {
		anomalyType = "unknown"
	}
	m.anomalyScore.WithLabelValues(tenantID, anomalyType).Set(score)
}

// Handler returns the Prometheus scrape handler.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// Registry exposes the underlying Prometheus registry for tests.
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

// StartMetricsServer starts an HTTP server on addr bound to 127.0.0.1 only.
// It blocks until ctx is cancelled.
func StartMetricsServer(ctx context.Context, addr string, m *Metrics) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("metrics: invalid address %q: %w", addr, err)
	}
	if host != "127.0.0.1" {
		return fmt.Errorf("metrics: address %q must bind 127.0.0.1", addr)
	}
	if m == nil {
		return errors.New("metrics: nil Metrics")
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           m.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("metrics: listen %s: %w", addr, err)
	}

	//nolint:gosec
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()

	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("metrics: serve %s: %w", addr, err)
	}
	return nil
}
