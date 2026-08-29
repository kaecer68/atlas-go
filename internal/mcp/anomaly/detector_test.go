package anomaly

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testAuditEntry is a test double for the Entry interface.
type testAuditEntry struct {
	version int
	ts      time.Time
	tool    string
	tenant  string
	status  string
	err     string
}

func (e testAuditEntry) Version() int          { return e.version }
func (e testAuditEntry) ObservedAt() time.Time { return e.ts }
func (e testAuditEntry) GetTool() string       { return e.tool }
func (e testAuditEntry) GetTenantID() string   { return coalesce(e.tenant, "anonymous") }
func (e testAuditEntry) GetStatus() string     { return e.status }
func (e testAuditEntry) GetError() string      { return e.err }

func coalesce(a, b string) string {
	if a == "" {
		return b
	}
	return a
}

// fakeScoreRecorder records SetAnomalyScore calls for verification.
type fakeScoreRecorder struct {
	calls []scoreCall
}

type scoreCall struct {
	tenantID, anomalyType string
	score                 float64
}

func (f *fakeScoreRecorder) SetAnomalyScore(tenantID, anomalyType string, score float64) {
	f.calls = append(f.calls, scoreCall{tenantID: tenantID, anomalyType: anomalyType, score: score})
}

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

// Test_Detector_burst_short_vs_long_window_emits_anomaly verifies that a
// sudden burst of calls triggers a burst anomaly.
func Test_Detector_burst_short_vs_long_window_emits_anomaly(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	cfg := Config{ShortWindow: 5 * time.Minute, LongWindow: 24 * time.Hour, BurstZScoreThreshold: 3.0}
	rec := &fakeScoreRecorder{}
	store := NewStore(100)
	d := NewDetector(cfg, rec, store)
	d.now = fixedClock(now)

	for range 30 {
		d.Observe(testAuditEntry{version: 2, ts: now, tool: "t", tenant: "tenant-a", status: "ok"})
	}

	require.Len(t, store.Recent(10), 1)
	require.Equal(t, "burst", store.Recent(1)[0].AnomalyType)
	require.Equal(t, "tenant-a", store.Recent(1)[0].TenantID)
	require.Len(t, rec.calls, 1)
	require.Equal(t, "burst", rec.calls[0].anomalyType)
}

// Test_Detector_per_tool_error_spike_emits_anomaly verifies that a high
// error rate for a single tool triggers a tool_error_spike anomaly.
func Test_Detector_per_tool_error_spike_emits_anomaly(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	cfg := Config{
		ShortWindow:          5 * time.Minute,
		BurstZScoreThreshold: 50.0,
		ErrorRateThreshold:   0.5,
		MinObservations:      5,
	}
	rec := &fakeScoreRecorder{}
	store := NewStore(100)
	d := NewDetector(cfg, rec, store)
	d.now = fixedClock(now)

	for i := range 6 {
		status := "error"
		if i == 0 {
			status = "ok"
		}
		d.Observe(testAuditEntry{version: 2, ts: now, tool: "risk_get_metrics", tenant: fmt.Sprintf("tenant-b-%d", i), status: status})
	}

	recent := store.Recent(10)
	require.Len(t, recent, 1)
	require.Equal(t, "tool_error_spike", recent[0].AnomalyType)
	require.Equal(t, "risk_get_metrics", recent[0].Tool)
}

// Test_Detector_per_tenant_error_anomaly_emits_anomaly verifies that a high
// error rate for a tenant triggers a tenant_error_anomaly.
func Test_Detector_per_tenant_error_anomaly_emits_anomaly(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	cfg := Config{
		ShortWindow:          5 * time.Minute,
		BurstZScoreThreshold: 50.0,
		ErrorRateThreshold:   0.5,
		MinObservations:      5,
	}
	rec := &fakeScoreRecorder{}
	store := NewStore(100)
	d := NewDetector(cfg, rec, store)
	d.now = fixedClock(now)

	for i := range 5 {
		d.Observe(testAuditEntry{version: 2, ts: now, tool: fmt.Sprintf("tool-%d", i), tenant: "tenant-c", status: "error"})
	}

	recent := store.Recent(10)
	require.Len(t, recent, 1)
	require.Equal(t, "tenant_error_anomaly", recent[0].AnomalyType)
	require.Equal(t, "tenant-c", recent[0].TenantID)
}

// Test_Detector_rolling_window_evicts_old_entries verifies that observations
// older than the configured windows are not used for anomaly detection.
func Test_Detector_rolling_window_evicts_old_entries(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	cfg := Config{
		ShortWindow:          5 * time.Minute,
		LongWindow:           24 * time.Hour,
		BurstZScoreThreshold: 50.0,
	}
	rec := &fakeScoreRecorder{}
	store := NewStore(100)
	d := NewDetector(cfg, rec, store)
	d.now = fixedClock(now)

	old := now.Add(-25 * time.Hour)
	for range 100 {
		d.Observe(testAuditEntry{version: 2, ts: old, tenant: "tenant-d", status: "ok"})
	}
	// A single current observation should not be a burst against empty windows.
	d.Observe(testAuditEntry{version: 2, ts: now, tenant: "tenant-d", status: "ok"})

	require.Empty(t, store.Recent(10))
}

// Test_Detector_AnomalyEvent_schema verifies the emitted event schema fields.
func Test_Detector_AnomalyEvent_schema(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	cfg := Config{ShortWindow: 5 * time.Minute, LongWindow: 24 * time.Hour, BurstZScoreThreshold: 3.0}
	store := NewStore(100)
	d := NewDetector(cfg, &fakeScoreRecorder{}, store)
	d.now = fixedClock(now)

	for range 30 {
		d.Observe(testAuditEntry{version: 2, ts: now, tool: "x", tenant: "tenant-e", status: "ok"})
	}

	event := store.Recent(1)[0]
	require.Equal(t, "tenant-e", event.TenantID)
	require.Equal(t, "burst", event.AnomalyType)
	require.Greater(t, event.Score, 0.0)
	require.NotEmpty(t, event.TS)
}

// Test_Detector_ignores_v1_entries verifies that schema_version < 2 entries
// are ignored.
func Test_Detector_ignores_v1_entries(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	cfg := Config{ShortWindow: 5 * time.Minute, LongWindow: 24 * time.Hour, BurstZScoreThreshold: 0.1}
	store := NewStore(100)
	rec := &fakeScoreRecorder{}
	d := NewDetector(cfg, rec, store)
	d.now = fixedClock(now)

	for range 30 {
		d.Observe(testAuditEntry{version: 1, ts: now, tenant: "tenant-f", status: "ok"})
	}

	require.Empty(t, store.Recent(10))
	require.Empty(t, rec.calls)
}

// Test_Detector_gauge_updated_on_detection verifies that the score recorder
// is invoked when an anomaly is detected.
func Test_Detector_gauge_updated_on_detection(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	cfg := Config{ShortWindow: 5 * time.Minute, LongWindow: 24 * time.Hour, BurstZScoreThreshold: 3.0}
	rec := &fakeScoreRecorder{}
	d := NewDetector(cfg, rec, NewStore(100))
	d.now = fixedClock(now)

	for range 30 {
		d.Observe(testAuditEntry{version: 2, ts: now, tenant: "tenant-g", status: "ok"})
	}

	require.Len(t, rec.calls, 1)
	require.Equal(t, "tenant-g", rec.calls[0].tenantID)
	require.Equal(t, "burst", rec.calls[0].anomalyType)
	require.Greater(t, rec.calls[0].score, 0.0)
}

// Test_Detector_conservative_threshold_avoids_false_positive verifies that
// when the score is below the conservative threshold no event is emitted.
func Test_Detector_conservative_threshold_avoids_false_positive(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	cfg := Config{
		ShortWindow:          5 * time.Minute,
		LongWindow:           24 * time.Hour,
		BurstZScoreThreshold: 50.0,
	}
	rec := &fakeScoreRecorder{}
	store := NewStore(100)
	d := NewDetector(cfg, rec, store)
	d.now = fixedClock(now)

	for range 2 {
		d.Observe(testAuditEntry{version: 2, ts: now, tenant: "tenant-h", status: "ok"})
	}

	require.Empty(t, store.Recent(10))
	require.Empty(t, rec.calls)
}
