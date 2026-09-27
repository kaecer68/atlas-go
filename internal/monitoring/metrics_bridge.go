package monitoring

import (
	"maps"

	"github.com/kaecer68/atlas-go/internal/monitoring/metrics"
)

// CollectorOnInc returns a metrics.OnInc callback that mirrors every CounterVec
// increment into the MetricsCollector that backs /metrics.
//
// Why this exists (2026-09-25, both defects were live in production):
//
//   - MetricsCollector.RecordCounter ACCUMULATES (existing.Value += value), so
//     the callback must forward the per-event DELTA that metrics.Counter hands
//     it, never the counter's cumulative value. Forwarding the cumulative value
//     inflated every mirrored series to x·N(N+1)/2 after N runs instead of x·N
//     (production: atlas_universe_symbols_screened_total{daily="failed"} showed
//     4797 after only two runs of 1599).
//   - The label map is forwarded as-is. The previous implementation received
//     only label VALUES (ordered by the vector's label names) and paired them
//     positionally into name=value, which exposed {daily="failed"} instead of
//     {stage="daily",result="failed"} and dropped single-label series
//     (atlas_universe_symbols_ranked_total) entirely — so daily and weekly runs
//     shared one series and could mask each other for up to 6 days.
//
// Both production wirings go through this function (cmd/atlas/main.go for
// UniverseMetrics, DashboardAPI.RegisterCrossMarketRoutes for DegradedMetrics),
// so the series identity exposed on /metrics cannot drift between call sites.
//
// A nil collector yields a nil callback, which SetOnInc treats as "not wired".
func CollectorOnInc(collector *MetricsCollector) metrics.OnInc {
	if collector == nil {
		return nil
	}
	return func(counterName string, labels map[string]string, value float64) {
		// Clone the label map: the collector keeps the map it is handed
		// (Metric.Labels), while the counter's own map stays private to the vector.
		collector.RecordCounter(counterName, value, maps.Clone(labels))
	}
}

// GaugeSink returns a metrics.UniverseRunVerdictSink that writes the per-run
// verdict gauges into the MetricsCollector that backs /metrics.
//
// It is deliberately NOT built on CounterVec/OnInc: the gauges describe the
// pipeline's OUTPUT (what the last run produced), and the 2026-09-25 production
// incident was a defect *in the counter sink path* (single-label series dropped)
// that left the counter side silently empty while the pipeline was healthy. A
// verdict that shares the counter wiring would have been lost by the same
// defect, which is the whole reason it exists — see metrics/universe_run.go.
//
// A nil collector yields a nil sink, which ReportRun treats as "not wired".
func GaugeSink(collector *MetricsCollector) metrics.UniverseRunVerdictSink {
	if collector == nil {
		return nil
	}
	return func(name string, value float64, labels map[string]string) {
		// Clone for the same reason as CollectorOnInc: the collector keeps the
		// map it is handed.
		collector.RecordGauge(name, value, maps.Clone(labels))
	}
}
