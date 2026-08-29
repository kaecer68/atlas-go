package server

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

// Test_Metrics_ObserveAnomaly_sets_gauge_and_increments_counter verifies
// the Phase 4 T1.4 contract: a single ObserveAnomaly call must (a) set
// the per-tenant / per-type score gauge and (b) bump the per-severity
// emission counter so dashboards can plot both current score and rate.
func Test_Metrics_ObserveAnomaly_sets_gauge_and_increments_counter(t *testing.T) {
	m := NewMetrics()
	m.ObserveAnomaly("tenant-a", "burst", "high", 4.2)

	require.InDelta(t, 4.2, testutil.ToFloat64(m.anomalyScore.WithLabelValues("tenant-a", "burst")), 0.001)
	require.InDelta(t, 1.0, testutil.ToFloat64(m.anomalyEmitted.WithLabelValues("tenant-a", "burst", "high")), 0.001)
}

// Test_Metrics_ObserveAnomaly_accumulates_counter verifies that successive
// calls with the same labels increment the counter (rate calculation depends
// on this).
func Test_Metrics_ObserveAnomaly_accumulates_counter(t *testing.T) {
	m := NewMetrics()
	for range 3 {
		m.ObserveAnomaly("tenant-a", "burst", "high", 1.0)
	}
	require.InDelta(t, 3.0, testutil.ToFloat64(m.anomalyEmitted.WithLabelValues("tenant-a", "burst", "high")), 0.001)
}

// Test_Metrics_ObserveAnomaly_separate_severities verifies that high and
// medium severities are tracked independently so an SRE alert can target
// "3 high-severity anomalies in 5m" without conflating with medium.
func Test_Metrics_ObserveAnomaly_separate_severities(t *testing.T) {
	m := NewMetrics()
	m.ObserveAnomaly("tenant-a", "burst", "high", 5.0)
	m.ObserveAnomaly("tenant-a", "burst", "medium", 2.0)
	m.ObserveAnomaly("tenant-a", "burst", "high", 5.0)

	require.InDelta(t, 2.0, testutil.ToFloat64(m.anomalyEmitted.WithLabelValues("tenant-a", "burst", "high")), 0.001)
	require.InDelta(t, 1.0, testutil.ToFloat64(m.anomalyEmitted.WithLabelValues("tenant-a", "burst", "medium")), 0.001)
}

// Test_Metrics_ObserveAnomaly_normalises_empty_tenant verifies that empty
// tenant IDs become "anonymous" (matches IncTokenUsage normalisation).
func Test_Metrics_ObserveAnomaly_normalises_empty_tenant(t *testing.T) {
	m := NewMetrics()
	m.ObserveAnomaly("", "burst", "low", 1.5)
	require.InDelta(t, 1.5, testutil.ToFloat64(m.anomalyScore.WithLabelValues("anonymous", "burst")), 0.001)
	require.InDelta(t, 1.0, testutil.ToFloat64(m.anomalyEmitted.WithLabelValues("anonymous", "burst", "low")), 0.001)
}

// Test_Metrics_ObserveAnomaly_normalises_empty_type verifies that empty
// anomaly_type becomes "unknown" (matches SetAnomalyScore normalisation).
func Test_Metrics_ObserveAnomaly_normalises_empty_type(t *testing.T) {
	m := NewMetrics()
	m.ObserveAnomaly("tenant-a", "", "medium", 1.0)
	require.InDelta(t, 1.0, testutil.ToFloat64(m.anomalyScore.WithLabelValues("tenant-a", "unknown")), 0.001)
}

// Test_Metrics_ObserveAnomaly_normalises_empty_severity verifies that empty
// severity becomes "unknown" — important because Alertmanager rules often
// match on severity and an unlabelled alert would silently fall through.
func Test_Metrics_ObserveAnomaly_normalises_empty_severity(t *testing.T) {
	m := NewMetrics()
	m.ObserveAnomaly("tenant-a", "burst", "", 1.0)
	require.InDelta(t, 1.0, testutil.ToFloat64(m.anomalyEmitted.WithLabelValues("tenant-a", "burst", "unknown")), 0.001)
}

// Test_Metrics_ObserveAnomaly_nil_metrics_safe verifies the nil receiver
// guard — the detector may run before metrics are wired (e.g. tests).
func Test_Metrics_ObserveAnomaly_nil_metrics_safe(t *testing.T) {
	var m *Metrics
	require.NotPanics(t, func() {
		m.ObserveAnomaly("t", "burst", "high", 1.0)
	})
}
