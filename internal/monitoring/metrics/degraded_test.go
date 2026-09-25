package metrics

import (
	"reflect"
	"testing"
	"time"
)

func TestDegradedModeCounter_ZeroInitially(t *testing.T) {
	m := NewDegradedMetrics()
	if got := m.DegradedActivations.WithLabelValues("crossmarket", "snapshot_stale").Value(); got != 0 {
		t.Fatalf("expected 0, got %v", got)
	}
}

func TestDegradedModeCounter_Increments(t *testing.T) {
	m := NewDegradedMetrics()
	m.DegradedActivations.WithLabelValues("crossmarket", "snapshot_stale").Inc()
	if got := m.DegradedActivations.WithLabelValues("crossmarket", "snapshot_stale").Value(); got != 1 {
		t.Fatalf("expected 1, got %v", got)
	}
}

func TestDegradedModeProviderErrorCounter_Increments(t *testing.T) {
	m := NewDegradedMetrics()
	m.ProviderErrors.WithLabelValues("crossmarket", "fetch_timeout").Inc()
	if got := m.ProviderErrors.WithLabelValues("crossmarket", "fetch_timeout").Value(); got != 1 {
		t.Fatalf("expected 1, got %v", got)
	}
}

func TestDegradedMetrics_OnIncCallback(t *testing.T) {
	m := NewDegradedMetrics()
	var gotName string
	var gotLabels map[string]string
	var gotValue float64
	m.SetOnInc(func(name string, labels map[string]string, value float64) {
		gotName = name
		gotLabels = labels
		gotValue = value
	})

	m.DegradedActivations.WithLabelValues("crossmarket", "snapshot_stale").Inc()

	if gotName != "degraded_activations" {
		t.Fatalf("callback name = %q, want degraded_activations", gotName)
	}
	// Label NAMES must come from the vector, never from the values: the
	// production defect paired values positionally and emitted
	// {"crossmarket": "snapshot_stale"} instead of the real label names.
	wantLabels := map[string]string{"service": "crossmarket", "reason": "snapshot_stale"}
	if !reflect.DeepEqual(gotLabels, wantLabels) {
		t.Fatalf("callback labels = %v, want %v", gotLabels, wantLabels)
	}
	if gotValue != 1.0 {
		t.Fatalf("callback value = %v, want 1.0", gotValue)
	}
}

// TestCounterVec_OnIncReportsDeltaNotCumulative pins the sink contract that
// production violated: MetricsCollector.RecordCounter ACCUMULATES, so the
// callback must report the per-event delta. Reporting the cumulative value
// turned "N runs of x" into x·N(N+1)/2 (production measured
// symbols_screened_total{daily="failed"} = 4797 = 1599 + 3198 after 2 runs).
func TestCounterVec_OnIncReportsDeltaNotCumulative(t *testing.T) {
	const (
		runs       = 3
		perRun     = int64(1599)
		cumulative = perRun * runs * (runs + 1) / 2 // 9588 — the buggy total
	)

	um := NewUniverseMetrics()
	var deltas []float64
	um.SetOnInc(func(_ string, _ map[string]string, value float64) {
		deltas = append(deltas, value)
	})

	counter := um.SymbolsScreened.WithLabelValues("daily", "failed")
	for range runs {
		counter.Add(perRun)
	}

	if len(deltas) != runs {
		t.Fatalf("callback fired %d times, want %d", len(deltas), runs)
	}
	var total float64
	for i, d := range deltas {
		if d != float64(perRun) {
			t.Fatalf("delta[%d] = %v, want %v (cumulative values inflate the sink)", i, d, perRun)
		}
		total += d
	}
	if want := float64(perRun * runs); total != want {
		t.Fatalf("forwarded total = %v, want %v (buggy cumulative total would be %v)", total, want, cumulative)
	}

	// Negative control: the counter's own accounting must stay cumulative. Sinks
	// accumulate, so changing Counter to set-semantics would break them.
	if got := counter.Value(); got != float64(perRun*runs) {
		t.Fatalf("counter.Value() = %v, want %v (counter must keep accumulating)", got, perRun*runs)
	}
}

// TestCounterVec_OnIncReportsOnePerInc covers the Inc() path (delta 1).
func TestCounterVec_OnIncReportsOnePerInc(t *testing.T) {
	dm := NewDegradedMetrics()
	var total float64
	dm.SetOnInc(func(_ string, _ map[string]string, value float64) {
		total += value
	})

	c := dm.ProviderErrors.WithLabelValues("crossmarket", "fetch_timeout")
	for range 4 {
		c.Inc()
	}

	if total != 4 {
		t.Fatalf("forwarded total = %v, want 4", total)
	}
	if got := c.Value(); got != 4 {
		t.Fatalf("counter.Value() = %v, want 4", got)
	}
}

// TestCounterVec_AddZeroStillReportsZero guards the Add(0) contract the universe
// pipeline relies on to materialise a series with no increment (e.g.
// symbols_ranked_total{stage="daily"} = 0 after a run that ranked nothing).
func TestCounterVec_AddZeroStillReportsZero(t *testing.T) {
	um := NewUniverseMetrics()
	var (
		fired int
		value float64 = -1
	)
	um.SetOnInc(func(_ string, _ map[string]string, v float64) {
		fired++
		value = v
	})

	um.SymbolsRanked.WithLabelValues("daily").Add(0)

	if fired != 1 {
		t.Fatalf("callback fired %d times, want 1", fired)
	}
	if value != 0 {
		t.Fatalf("reported value = %v, want 0", value)
	}
}

func TestDegradedMetrics_OnIncNilSafe(t *testing.T) {
	m := NewDegradedMetrics()
	m.DegradedActivations.WithLabelValues("crossmarket", "snapshot_stale").Inc()
	if got := m.DegradedActivations.WithLabelValues("crossmarket", "snapshot_stale").Value(); got != 1 {
		t.Fatalf("expected 1, got %v", got)
	}
}

func TestDegradedMetrics_Snapshot_BothCounters(t *testing.T) {
	m := NewDegradedMetrics()
	m.DegradedActivations.WithLabelValues("crossmarket", "snapshot_stale").Inc()
	m.DegradedActivations.WithLabelValues("crossmarket", "snapshot_stale").Inc()
	m.ProviderErrors.WithLabelValues("crossmarket", "fetch_timeout").Inc()

	// T1-2: typed return must be DegradedSnapshot, not map[string][]map[string]any.
	snap := m.Snapshot()

	if len(snap.DegradedActivations) != 1 {
		t.Fatalf("expected 1 activation sample, got %d", len(snap.DegradedActivations))
	}
	act := snap.DegradedActivations[0]
	if act.Labels["service"] != "crossmarket" || act.Labels["reason"] != "snapshot_stale" {
		t.Fatalf("activation labels = %v, want service=crossmarket reason=snapshot_stale", act.Labels)
	}
	if act.Value != 2.0 {
		t.Fatalf("activation value = %v, want 2.0", act.Value)
	}

	if len(snap.ProviderErrors) != 1 {
		t.Fatalf("expected 1 error sample, got %d", len(snap.ProviderErrors))
	}
	perr := snap.ProviderErrors[0]
	if perr.Labels["service"] != "crossmarket" || perr.Labels["error_type"] != "fetch_timeout" {
		t.Fatalf("error labels = %v, want service=crossmarket error_type=fetch_timeout", perr.Labels)
	}
	if perr.Value != 1.0 {
		t.Fatalf("error value = %v, want 1.0", perr.Value)
	}
}

// T1-4
func TestDegradedMetrics_Snapshot_HasTimestamp(t *testing.T) {
	m := NewDegradedMetrics()
	before := time.Now()
	snap := m.Snapshot()
	after := time.Now()

	if snap.Timestamp.IsZero() {
		t.Fatalf("Snapshot.Timestamp is zero")
	}
	if snap.Timestamp.Before(before) || snap.Timestamp.After(after) {
		t.Fatalf("Snapshot.Timestamp = %v, want between %v and %v", snap.Timestamp, before, after)
	}
}

// T1-4
func TestDegradedMetrics_Snapshot_SampleTimestamp(t *testing.T) {
	m := NewDegradedMetrics()
	m.DegradedActivations.WithLabelValues("svc", "reason").Inc()
	m.ProviderErrors.WithLabelValues("svc", "etype").Inc()
	before := time.Now()
	snap := m.Snapshot()
	after := time.Now()

	if len(snap.DegradedActivations) != 1 {
		t.Fatalf("expected 1 activation sample, got %d", len(snap.DegradedActivations))
	}
	ts := snap.DegradedActivations[0].Timestamp
	if ts.Before(before) || ts.After(after) {
		t.Fatalf("activation sample Timestamp = %v, want between %v and %v", ts, before, after)
	}
	if snap.ProviderErrors[0].Timestamp.IsZero() {
		t.Fatalf("error sample Timestamp is zero")
	}
}

// T1-2
func TestDegradedMetrics_Snapshot_LabelsAreCopies(t *testing.T) {
	m := NewDegradedMetrics()
	m.DegradedActivations.WithLabelValues("svc", "reason").Inc()
	snap1 := m.Snapshot()

	snap1.DegradedActivations[0].Labels["service"] = "MUTATED"

	snap2 := m.Snapshot()
	if snap2.DegradedActivations[0].Labels["service"] != "svc" {
		t.Fatalf("snapshot labels are shared: second snapshot saw service=%q, want %q",
			snap2.DegradedActivations[0].Labels["service"], "svc")
	}
}

// T1-2
func TestDegradedMetrics_Snapshot_EmptyTypedShape(t *testing.T) {
	m := NewDegradedMetrics()
	snap := m.Snapshot()

	if snap.DegradedActivations == nil {
		t.Fatalf("DegradedActivations must not be nil for empty snapshot (use []Sample{})")
	}
	if snap.ProviderErrors == nil {
		t.Fatalf("ProviderErrors must not be nil for empty snapshot")
	}
	if len(snap.DegradedActivations) != 0 || len(snap.ProviderErrors) != 0 {
		t.Fatalf("empty snapshot should have zero samples, got activations=%d errors=%d",
			len(snap.DegradedActivations), len(snap.ProviderErrors))
	}
	if snap.Timestamp.IsZero() {
		t.Fatalf("empty snapshot Timestamp is zero")
	}
}

// T1-2
func TestDegradedMetrics_SampleStructShape(t *testing.T) {
	s := Sample{
		Labels:    map[string]string{"a": "1"},
		Value:     7.5,
		Timestamp: time.Now(),
	}
	if s.Labels["a"] != "1" || s.Value != 7.5 || s.Timestamp.IsZero() {
		t.Fatalf("Sample struct shape is wrong: %+v", s)
	}
	v := reflect.ValueOf(DegradedSnapshot{})
	if !v.FieldByName("Timestamp").IsValid() {
		t.Fatalf("DegradedSnapshot must have Timestamp field")
	}
}
