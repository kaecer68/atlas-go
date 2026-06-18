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
	var gotLabels []string
	var gotValue float64
	m.SetOnInc(func(name string, labels []string, value float64) {
		gotName = name
		gotLabels = labels
		gotValue = value
	})

	m.DegradedActivations.WithLabelValues("crossmarket", "snapshot_stale").Inc()

	if gotName != "degraded_activations" {
		t.Fatalf("callback name = %q, want degraded_activations", gotName)
	}
	wantLabels := []string{"crossmarket", "snapshot_stale"}
	if !reflect.DeepEqual(gotLabels, wantLabels) {
		t.Fatalf("callback labels = %v, want %v", gotLabels, wantLabels)
	}
	if gotValue != 1.0 {
		t.Fatalf("callback value = %v, want 1.0", gotValue)
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
