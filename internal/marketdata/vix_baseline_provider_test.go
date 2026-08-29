package marketdata

import (
	"path/filepath"
	"testing"
)

func TestVIXBaselineTracker_Value_Empty(t *testing.T) {
	tracker := NewVIXBaselineTracker(filepath.Join(t.TempDir(), "vix_history.json"))
	if got := tracker.Value(); got != 0 {
		t.Fatalf("expected 0 for empty tracker, got %f", got)
	}
}

func TestVIXBaselineTracker_UpdateAndMedian(t *testing.T) {
	tracker := NewVIXBaselineTracker(filepath.Join(t.TempDir(), "vix_history.json"))

	// Feed values: 10, 20, 30 → median should be 20
	tracker.Update(10)
	tracker.Update(30)
	tracker.Update(20)

	if got := tracker.Value(); got != 20 {
		t.Fatalf("expected median 20, got %f", got)
	}
}

func TestVIXBaselineTracker_Update_IgnoresZero(t *testing.T) {
	tracker := NewVIXBaselineTracker(filepath.Join(t.TempDir(), "vix_history.json"))

	tracker.Update(10)
	tracker.Update(0) // should be ignored
	tracker.Update(30)

	if got := tracker.Value(); got != 20 {
		t.Fatalf("expected median 20 (ignoring zero), got %f", got)
	}
}

func TestVIXBaselineTracker_EvenCountMedian(t *testing.T) {
	tracker := NewVIXBaselineTracker(filepath.Join(t.TempDir(), "vix_history.json"))

	tracker.Update(10)
	tracker.Update(40)
	tracker.Update(20)
	tracker.Update(30)

	// Sorted: 10, 20, 30, 40 → median = (20+30)/2 = 25
	if got := tracker.Value(); got != 25 {
		t.Fatalf("expected median 25 for even count, got %f", got)
	}
}

func TestVIXBaselineTracker_Persistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vix_history.json")

	tracker1 := NewVIXBaselineTracker(path)
	tracker1.Update(15)
	tracker1.Update(25)
	tracker1.Update(35)

	// Create a new tracker instance reading the same file
	tracker2 := NewVIXBaselineTracker(path)
	if got := tracker2.Value(); got != 25 {
		t.Fatalf("expected persisted median 25, got %f", got)
	}
}

func TestVIXBaselineTracker_MaxSize(t *testing.T) {
	tracker := NewVIXBaselineTracker(filepath.Join(t.TempDir(), "vix_history.json"))

	n := vixBaselineMaxDays + 10 // 262 values: 0..261, trimmed to 10..261 (252 values)
	for i := range n {
		tracker.Update(float64(i))
	}

	// After trimming, kept values are 10..261. Median of 252 values = (135+136)/2 = 135.5
	if got := tracker.Value(); got != 135.5 {
		t.Fatalf("expected median 135.5 (10..261), got %f", got)
	}
}
