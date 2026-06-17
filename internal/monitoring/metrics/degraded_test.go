package metrics

import "testing"

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
