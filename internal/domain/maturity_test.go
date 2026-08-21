package domain

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMaturityGated(t *testing.T) {
	tests := []struct {
		current  SystemMaturity
		minimum  SystemMaturity
		expected bool
	}{
		{MaturityBurnIn, MaturityBurnIn, true},
		{MaturityBurnIn, MaturityCalibrating, false},
		{MaturityBurnIn, MaturityFullAuto, false},
		{MaturityCalibrating, MaturityBurnIn, true},
		{MaturityCalibrating, MaturityCalibrating, true},
		{MaturityCalibrating, MaturityFullAuto, false},
		{MaturityFullAuto, MaturityBurnIn, true},
		{MaturityFullAuto, MaturityCalibrating, true},
		{MaturityFullAuto, MaturityFullAuto, true},
	}
	for _, tc := range tests {
		got := MaturityGated(tc.current, tc.minimum)
		if got != tc.expected {
			t.Errorf("MaturityGated(%q, %q) = %v, want %v", tc.current, tc.minimum, got, tc.expected)
		}
	}
}

func TestMaturityTracker_NewWithStart(t *testing.T) {
	// 300 days ago → FULL_AUTO
	start := time.Now().UTC().Add(-300 * 24 * time.Hour)
	tr := NewMaturityTrackerWithStart(start)
	if tr.Current() != MaturityFullAuto {
		t.Errorf("expected FULL_AUTO for 300d old system, got %q", tr.Current())
	}
	if tr.DaysSinceStart() < 299 {
		t.Errorf("expected DaysSinceStart >= 299, got %d", tr.DaysSinceStart())
	}
	if d := tr.DaysUntil(MaturityFullAuto); d != 0 {
		t.Errorf("expected DaysUntil(FULL_AUTO) = 0, got %d", d)
	}

	// 90 days ago → CALIBRATING
	start = time.Now().UTC().Add(-90 * 24 * time.Hour)
	tr = NewMaturityTrackerWithStart(start)
	if tr.Current() != MaturityCalibrating {
		t.Errorf("expected CALIBRATING for 90d old system, got %q", tr.Current())
	}
	if d := tr.DaysUntil(MaturityFullAuto); d == 0 {
		t.Error("expected DaysUntil(FULL_AUTO) > 0 for 90d system")
	}

	// 10 days ago → BURN_IN
	start = time.Now().UTC().Add(-10 * 24 * time.Hour)
	tr = NewMaturityTrackerWithStart(start)
	if tr.Current() != MaturityBurnIn {
		t.Errorf("expected BURN_IN for 10d old system, got %q", tr.Current())
	}
	if d := tr.DaysUntil(MaturityCalibrating); d == 0 {
		t.Error("expected DaysUntil(CALIBRATING) > 0 for 10d system")
	}
}

func TestMaturityTracker_OnTransition(t *testing.T) {
	// Start at burn_in, back-date so we can transition forward
	start := time.Now().UTC().Add(-59 * 24 * time.Hour)
	tr := NewMaturityTrackerWithStart(start)

	var transitionCalled bool
	var oldM, newM SystemMaturity
	tr.OnTransition(func(o, n SystemMaturity) {
		transitionCalled = true
		oldM = o
		newM = n
	})

	// Advance start date to trigger transition
	tr.firstStartDate = time.Now().UTC().Add(-61 * 24 * time.Hour)
	tr.Refresh()

	if !transitionCalled {
		t.Fatal("expected OnTransition to be called")
	}
	if oldM != MaturityBurnIn {
		t.Errorf("expected old=BURN_IN, got %q", oldM)
	}
	if newM != MaturityCalibrating {
		t.Errorf("expected new=CALIBRATING, got %q", newM)
	}
}

func TestMaturityTracker_SaveOnTransition(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "maturity.json")

	// Start at burn_in, back-date so we can trigger a transition.
	start := time.Now().UTC().Add(-59 * 24 * time.Hour)
	tr := NewMaturityTrackerWithStart(start)

	var saved bool
	var saveErr error
	tr.OnTransition(func(oldM, newM SystemMaturity) {
		if err := tr.Save(statePath); err != nil {
			saveErr = err
			return
		}
		saved = true
	})

	// Trigger transition by advancing the start date.
	tr.firstStartDate = time.Now().UTC().Add(-61 * 24 * time.Hour)
	tr.Refresh()

	if !saved {
		t.Fatal("expected Save to be called on transition")
	}
	if saveErr != nil {
		t.Fatalf("Save failed: %v", saveErr)
	}

	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("expected state file to exist: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty state file")
	}
}

func TestMaturityTracker_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "maturity.json")

	// Fresh start creates and persists
	tr, err := NewMaturityTracker(path)
	if err != nil {
		t.Fatalf("NewMaturityTracker: %v", err)
	}
	if tr.FirstStartDate().IsZero() {
		t.Fatal("expected FirstStartDate to be set on fresh start")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected state file to be created: %v", err)
	}

	// Restart loads same start date
	tr2, err := NewMaturityTracker(path)
	if err != nil {
		t.Fatalf("NewMaturityTracker restart: %v", err)
	}
	if !tr2.FirstStartDate().Equal(tr.FirstStartDate()) {
		t.Error("expected FirstStartDate to survive restart")
	}
}

func TestMaturityTracker_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "maturity.json")

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tr := NewMaturityTrackerWithStart(start)
	if err := tr.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	tr2, err := NewMaturityTracker(path)
	if err != nil {
		t.Fatalf("NewMaturityTracker after save: %v", err)
	}
	if !tr2.FirstStartDate().Equal(start) {
		t.Errorf("FirstStartDate mismatch: got %v, want %v", tr2.FirstStartDate(), start)
	}
}

func TestMaturityTracker_SeededFreshStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "maturity.json")

	// Fresh start with a valid RFC3339 seed → first_start = seed, persisted.
	seed := "2026-06-01T05:10:28.266647Z"
	tr, err := NewMaturityTrackerSeeded(path, seed)
	if err != nil {
		t.Fatalf("NewMaturityTrackerSeeded: %v", err)
	}
	want := time.Date(2026, 6, 1, 5, 10, 28, 266647000, time.UTC)
	if !tr.FirstStartDate().Equal(want) {
		t.Errorf("FirstStartDate = %v, want %v", tr.FirstStartDate(), want)
	}
	// A 2026-06-01 seed is >60 days old at any realistic test time →
	// the tracker must be past burn-in immediately (fresh-start refresh
	// computes from the seeded date, not from zero time).
	if tr.Current() != MaturityCalibrating {
		t.Errorf("expected CALIBRATING for seeded tracker, got %q", tr.Current())
	}

	// The seed must be persisted so a restart keeps it.
	tr2, err := NewMaturityTrackerSeeded(path, "2099-01-01T00:00:00Z")
	if err != nil {
		t.Fatalf("NewMaturityTrackerSeeded restart: %v", err)
	}
	if !tr2.FirstStartDate().Equal(want) {
		t.Errorf("restart FirstStartDate = %v, want persisted seed %v", tr2.FirstStartDate(), want)
	}
}

func TestMaturityTracker_SeededDateOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "maturity.json")

	tr, err := NewMaturityTrackerSeeded(path, "2026-06-01")
	if err != nil {
		t.Fatalf("NewMaturityTrackerSeeded: %v", err)
	}
	want := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if !tr.FirstStartDate().Equal(want) {
		t.Errorf("FirstStartDate = %v, want %v", tr.FirstStartDate(), want)
	}
}

func TestMaturityTracker_SeededInvalidFallsBackToNow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "maturity.json")

	tr, err := NewMaturityTrackerSeeded(path, "not-a-date")
	if err != nil {
		t.Fatalf("NewMaturityTrackerSeeded: %v", err)
	}
	got := tr.FirstStartDate()
	if got.IsZero() {
		t.Fatal("expected FirstStartDate to be set (fallback to now)")
	}
	if diff := time.Since(got); diff < 0 || diff > time.Hour {
		t.Errorf("expected FirstStartDate ≈ now, got %v (diff %v)", got, diff)
	}
}

func TestMaturityTracker_SeededExistingFileWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "maturity.json")

	// First creation with an old seed.
	old := "2026-06-01T05:10:28Z"
	if _, err := NewMaturityTrackerSeeded(path, old); err != nil {
		t.Fatalf("NewMaturityTrackerSeeded: %v", err)
	}

	// Restart with a different seed must NOT overwrite the persisted date.
	newer := "2026-08-01T00:00:00Z"
	tr, err := NewMaturityTrackerSeeded(path, newer)
	if err != nil {
		t.Fatalf("NewMaturityTrackerSeeded restart: %v", err)
	}
	want := time.Date(2026, 6, 1, 5, 10, 28, 0, time.UTC)
	if !tr.FirstStartDate().Equal(want) {
		t.Errorf("FirstStartDate = %v, want persisted %v (seed must not override existing file)", tr.FirstStartDate(), want)
	}
}

func TestMaturityThresholds(t *testing.T) {
	if MaturityThresholds[MaturityBurnIn] != 0 {
		t.Error("BURN_IN threshold should be 0")
	}
	if MaturityThresholds[MaturityCalibrating] != 60 {
		t.Error("CALIBRATING threshold should be 60")
	}
	if MaturityThresholds[MaturityFullAuto] != 252 {
		t.Error("FULL_AUTO threshold should be 252")
	}
}
