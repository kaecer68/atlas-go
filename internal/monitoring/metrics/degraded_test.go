package metrics

import (
	"reflect"
	"testing"
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

	got := m.Snapshot()
	want := map[string][]map[string]any{
		"degraded_activations": {
			{"service": "crossmarket", "reason": "snapshot_stale", "value": 2.0},
		},
		"provider_errors": {
			{"service": "crossmarket", "error_type": "fetch_timeout", "value": 1.0},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Snapshot() = %v, want %v", got, want)
	}
}
