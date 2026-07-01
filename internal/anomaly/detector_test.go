package anomaly

import (
	"context"
	"testing"
	"time"
)

func TestDefaultConfig_returnsExpectedDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.DetectIntervalSec != 60 {
		t.Errorf("DetectIntervalSec = %d, want 60", cfg.DetectIntervalSec)
	}
	if cfg.BaselineWindowMin != 1440 {
		t.Errorf("BaselineWindowMin = %d, want 1440", cfg.BaselineWindowMin)
	}
	if cfg.CurrentWindowMin != 5 {
		t.Errorf("CurrentWindowMin = %d, want 5", cfg.CurrentWindowMin)
	}
	if cfg.ZScoreThreshold != 2.5 {
		t.Errorf("ZScoreThreshold = %v, want 2.5", cfg.ZScoreThreshold)
	}
	if cfg.MinBaselineSamples != 30 {
		t.Errorf("MinBaselineSamples = %d, want 30", cfg.MinBaselineSamples)
	}
}

func TestFilterEntries_keepsEntriesInWindow(t *testing.T) {
	now := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	entries := []AuditEntryV2{
		{TS: now.Add(-10 * time.Minute).Format(time.RFC3339), Tool: "t1"},
		{TS: now.Add(-3 * time.Minute).Format(time.RFC3339), Tool: "t2"},
		{TS: now.Add(1 * time.Minute).Format(time.RFC3339), Tool: "t3"},
	}
	since := now.Add(-5 * time.Minute)
	got := FilterEntries(entries, since, now)
	if len(got) != 1 || got[0].Tool != "t2" {
		t.Errorf("FilterEntries = %+v, want [t2]", got)
	}
}

func TestFilterEntries_excludesUnparseableTimestamps(t *testing.T) {
	entries := []AuditEntryV2{
		{TS: "not-a-time", Tool: "t1"},
		{TS: time.Now().Format(time.RFC3339), Tool: "t2"},
	}
	got := FilterEntries(entries, time.Time{}, time.Time{})
	if len(got) != 1 || got[0].Tool != "t2" {
		t.Errorf("FilterEntries = %+v, want [t2]", got)
	}
}

func TestDefaultZScoreFunc_returnsZeroForInsufficientSamples(t *testing.T) {
	f := DefaultZScoreFunc()
	if got := f(Baseline{SampleN: 1}, Current{SampleN: 1}); got != 0 {
		t.Errorf("ZScore = %v, want 0 for insufficient baseline", got)
	}
}

func TestDefaultZScoreFunc_returnsZeroWhenStdDevZero(t *testing.T) {
	f := DefaultZScoreFunc()
	if got := f(Baseline{SampleN: 10, Median: 5}, Current{SampleN: 1, Median: 10}); got != 0 {
		t.Errorf("ZScore = %v, want 0 when stddev is zero", got)
	}
}

func TestDefaultZScoreFunc_computesNormalizedDifference(t *testing.T) {
	f := DefaultZScoreFunc()
	baseline := Baseline{SampleN: 10, Median: 10, StdDev: 2}
	current := Current{SampleN: 1, Median: 14}
	if got := f(baseline, current); got != 2 {
		t.Errorf("ZScore = %v, want 2", got)
	}
}

func TestDetectorInterface_canBeImplemented(t *testing.T) {
	var _ Detector = (*stubDetector)(nil)
}

type stubDetector struct{}

func (stubDetector) Name() string { return "stub" }

func (stubDetector) Detect(_ context.Context, _ []AuditEntryV2) ([]Anomaly, error) {
	return nil, nil
}
