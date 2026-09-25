package monitoring_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/llm"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// stubProvider is a minimal llm.ProviderImpl used to drive a real DefaultRouter
// from outside the llm package. fail=true makes the provider reject every call,
// which is how these tests force the router down its fallback chain.
type stubProvider struct {
	name llm.Provider
	fail bool
}

func (s stubProvider) Supports(llm.Capability) bool { return true }

func (s stubProvider) Call(context.Context, llm.Request) (llm.Response, error) {
	if s.fail {
		return llm.Response{}, errors.New("stub provider failure")
	}
	return llm.Response{Output: "stub answer", Provider: s.name}, nil
}

func (s stubProvider) Health() llm.HealthStatus {
	return llm.HealthStatus{Provider: s.name, Healthy: true}
}

// renderMetrics drives the same handler cmd/atlas/api_routes.go mounts at
// /metrics (monitoring.PrometheusHandler(collector)) and returns the body.
func renderMetrics(t *testing.T, collector *monitoring.MetricsCollector) string {
	t.Helper()
	rec := httptest.NewRecorder()
	monitoring.PrometheusHandler(collector).ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	if rec.Code != 200 {
		t.Fatalf("expected 200 from /metrics, got %d", rec.Code)
	}
	return rec.Body.String()
}

// logRouterMetricLines logs the llm_router_* lines of a scrape body. Run the
// tests with -v to see the exact text Prometheus would ingest.
func logRouterMetricLines(t *testing.T, body string) {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "llm_router_") {
			t.Log(line)
		}
	}
}

// TestLLMRouterMetrics_PrometheusEndpoint_FallbackTriggered is the end-to-end
// check for issue #1926: a router wired to the production collector
// (WithMetrics(collector)) must make its fallback counter readable on the
// /metrics endpoint that cmd/atlas exposes, with capability / from_provider /
// to_provider labels intact.
func TestLLMRouterMetrics_PrometheusEndpoint_FallbackTriggered(t *testing.T) {
	collector := monitoring.NewMetricsCollector()
	router := llm.NewDefaultRouter(
		stubProvider{name: llm.ProviderMiniMax, fail: true},
		stubProvider{name: llm.ProviderDeepSeek},
	).WithMetrics(collector)

	if _, err := router.Call(context.Background(), llm.Request{
		Capability: llm.CapabilityFailureAttribution,
	}); err != nil {
		t.Fatalf("router.Call: %v", err)
	}

	body := renderMetrics(t, collector)
	logRouterMetricLines(t, body)
	want := []string{
		"# TYPE llm_router_fallback_triggered_total counter",
		`llm_router_fallback_triggered_total{capability="failure_attribution",from_provider="minimax",to_provider="deepseek"} 1`,
	}
	for _, sub := range want {
		if !strings.Contains(body, sub) {
			t.Fatalf("missing /metrics line %q\n--- full body ---\n%s", sub, body)
		}
	}
}

// TestLLMRouterMetrics_PrometheusEndpoint_BackupChainExhausted covers the second
// counter: every chain member fails, so the exhausted counter must be readable
// with its capability label.
func TestLLMRouterMetrics_PrometheusEndpoint_BackupChainExhausted(t *testing.T) {
	collector := monitoring.NewMetricsCollector()
	router := llm.NewDefaultRouter(
		stubProvider{name: llm.ProviderMiniMax, fail: true},
		stubProvider{name: llm.ProviderDeepSeek, fail: true},
	).WithMetrics(collector)

	resp, err := router.Call(context.Background(), llm.Request{
		Capability: llm.CapabilityFailureAttribution,
	})
	if err != nil {
		t.Fatalf("router.Call: %v", err)
	}
	if resp.Provider != llm.ProviderMock {
		t.Fatalf("expected last-resort ProviderMock, got %v", resp.Provider)
	}

	body := renderMetrics(t, collector)
	logRouterMetricLines(t, body)
	want := []string{
		"# TYPE llm_router_backup_chain_exhausted_total counter",
		`llm_router_backup_chain_exhausted_total{capability="failure_attribution"} 1`,
		`llm_router_fallback_triggered_total{capability="failure_attribution",from_provider="minimax",to_provider="deepseek"} 1`,
	}
	for _, sub := range want {
		if !strings.Contains(body, sub) {
			t.Fatalf("missing /metrics line %q\n--- full body ---\n%s", sub, body)
		}
	}
}

// TestLLMRouterMetrics_PrometheusEndpoint_QuietWhenNotInjected is the control:
// without WithMetrics the /metrics body must stay free of router metrics, i.e.
// wiring the collector is what makes them visible (no accidental global emit).
func TestLLMRouterMetrics_PrometheusEndpoint_QuietWhenNotInjected(t *testing.T) {
	collector := monitoring.NewMetricsCollector()
	router := llm.NewDefaultRouter(
		stubProvider{name: llm.ProviderMiniMax, fail: true},
		stubProvider{name: llm.ProviderDeepSeek},
	)

	if _, err := router.Call(context.Background(), llm.Request{
		Capability: llm.CapabilityFailureAttribution,
	}); err != nil {
		t.Fatalf("router.Call: %v", err)
	}

	body := renderMetrics(t, collector)
	if strings.Contains(body, "llm_router_fallback_triggered_total") {
		t.Fatalf("router metric appeared without an injected recorder\n--- body ---\n%s", body)
	}
}
