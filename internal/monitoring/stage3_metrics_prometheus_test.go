package monitoring

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/ledger"
)

// TestStage3Metrics_PrometheusHandler_ExposesBothCounters exercises the
// /metrics HTTP surface that cmd/atlas/api_routes.go wires up via
// monitoring.PrometheusHandler(collector). Without an emitting call the
// counter rows don't appear in `GetAllMetrics()`, so this test issues a
// sample increment of each helper and confirms the corresponding
// `<name>{<labels>} <value>` line is in the rendered body.
// Regression catch: if RecordStage3TaskRun / RecordStage3AlertFired are
// ever silently no-op (label cardinality drift, nil collector escape,
// name change), the metric disappears from /metrics and Prometheus
// scrape fails to track fire rate.
func TestStage3Metrics_PrometheusHandler_ExposesBothCounters(t *testing.T) {
	c := NewMetricsCollector()
	RecordStage3TaskRun(c, "sync-events-daily", "success")
	RecordStage3AlertFired(c, "data-staleness-critical", AlertLevelCritical)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	PrometheusHandler(c).ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	want := []string{
		`atlas_stage3_task_runs_total{result="success",task="sync-events-daily"} 1`,
		`atlas_stage3_alerts_fired_total{rule="data-staleness-critical",severity="critical"} 1`,
		`# TYPE atlas_stage3_task_runs_total counter`,
		`# TYPE atlas_stage3_alerts_fired_total counter`,
	}
	for _, sub := range want {
		if !strings.Contains(body, sub) {
			t.Fatalf("missing /metrics line %q\n--- full body ---\n%s", sub, body)
		}
	}
}

// TestStage3Metrics_PrometheusHandler_StableBeforeEmission verifies the
// counter name appears in /metrics registry scope as soon as the
// monitoring package is loaded, even with no emitted values yet (so an
// absent data point still surfaces a zero-count row in scrape output).
func TestStage3Metrics_PrometheusHandler_StableBeforeEmission(t *testing.T) {
	c := NewMetricsCollector()
	rec := httptest.NewRecorder()
	PrometheusHandler(c).ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))

	if rec.Code != 200 {
		t.Fatalf("expected 200 from empty collector, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), MetricStage3TaskRuns) {
		t.Fatalf("metric %q should not appear before first emission (got body lines including %q)", MetricStage3TaskRuns, MetricStage3TaskRuns)
	}
}

// TestStage3Metrics_PrometheusHandler_ExposesLedgerGauge verifies the
// gauge end of the Stage 3 metrics path renders as `# TYPE … gauge`
// in /metrics body, with the latest store.Len() reflected in the value
// column. Complements the counter test above; together they cover both
// metric types the /metrics route exposes for Stage 3.
func TestStage3Metrics_PrometheusHandler_ExposesLedgerGauge(t *testing.T) {
	c := NewMetricsCollector()
	store := ledger.NewJSONLEventFlowPredictionStore(t.TempDir())
	for i := range 5 {
		if err := store.AppendPrediction(ledger.EventFlowPredictionRecord{
			PredictedAt:   time.Now().Add(time.Duration(i) * time.Hour),
			DirectionSign: float64(i + 1),
			Confidence:    0.7,
			Direction:     "inflow",
		}); err != nil {
			t.Fatalf("AppendPrediction %d: %v", i, err)
		}
	}
	RecordStage3LedgerRecords(c, store)

	rec := httptest.NewRecorder()
	PrometheusHandler(c).ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	want := []string{
		`# TYPE atlas_stage3_ledger_records gauge`,
		`atlas_stage3_ledger_records{ledger="event_flow_prediction"} 5`,
	}
	for _, sub := range want {
		if !strings.Contains(body, sub) {
			t.Fatalf("missing /metrics gauge line %q\n--- full body ---\n%s", sub, body)
		}
	}
}
