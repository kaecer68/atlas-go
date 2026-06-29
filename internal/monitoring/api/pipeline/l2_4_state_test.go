package pipeline

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestManager(t *testing.T) *L24StateManager {
	t.Helper()
	dir := t.TempDir()
	return NewL24StateManager(dir)
}

func TestL24_NewManager_MissingFile_NoError(t *testing.T) {
	m := newTestManager(t)
	if m.Get().Status.Running {
		t.Error("expected status.Running=false on fresh manager")
	}
}

func TestL24_Get_ZeroValueWhenEmpty(t *testing.T) {
	m := newTestManager(t)
	got := m.Get()
	if got.Status.Running {
		t.Error("zero-value state should have Running=false")
	}
	if got.Status.StartedAt != nil {
		t.Error("zero-value state should have nil StartedAt")
	}
}

func TestL24_Start_SetsRunning(t *testing.T) {
	m := newTestManager(t)
	// Seed default config first (normally done via SetConfig at boot)
	if err := m.SetConfig(L24ScheduleConfig{DefaultStartTime: "13:45", DefaultPeriodDays: 7}); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	before := time.Now()
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	after := time.Now()
	got := m.Get()
	if !got.Status.Running {
		t.Error("expected Running=true after Start")
	}
	if got.Status.StartedAt == nil {
		t.Fatal("StartedAt should be set")
	}
	if got.Status.StartedAt.Before(before) || got.Status.StartedAt.After(after) {
		t.Errorf("StartedAt %v not in [%v, %v]", got.Status.StartedAt, before, after)
	}
	if got.Status.EndsAt == nil {
		t.Fatal("EndsAt should be set")
	}
	// EndsAt should be ~7 days after StartedAt
	expectedEnd := got.Status.StartedAt.Add(7 * 24 * time.Hour)
	diff := got.Status.EndsAt.Sub(expectedEnd)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("EndsAt %v, expected ~%v (diff %v)", got.Status.EndsAt, expectedEnd, diff)
	}
}

func TestL24_Start_Twice_ReturnsError(t *testing.T) {
	m := newTestManager(t)
	_ = m.SetConfig(L24ScheduleConfig{DefaultStartTime: "13:45", DefaultPeriodDays: 7})
	if err := m.Start(); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if err := m.Start(); err == nil {
		t.Error("expected error on second Start while running")
	}
}

func TestL24_Stop_ClearsRunning(t *testing.T) {
	m := newTestManager(t)
	_ = m.SetConfig(L24ScheduleConfig{DefaultStartTime: "13:45", DefaultPeriodDays: 7})
	_ = m.Start()
	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if m.Get().Status.Running {
		t.Error("expected Running=false after Stop")
	}
}

func TestL24_Stop_WhenNotRunning_ReturnsError(t *testing.T) {
	m := newTestManager(t)
	if err := m.Stop(); err == nil {
		t.Error("expected error on Stop when not running")
	}
}

func TestL24_ApplyOverride_Valid(t *testing.T) {
	m := newTestManager(t)
	if err := m.ApplyOverride("14:30", 14); err != nil {
		t.Fatalf("ApplyOverride: %v", err)
	}
	got := m.Get()
	if got.Config.OverrideStartTime != "14:30" {
		t.Errorf("OverrideStartTime = %q, want 14:30", got.Config.OverrideStartTime)
	}
	if got.Config.OverridePeriodDays != 14 {
		t.Errorf("OverridePeriodDays = %d, want 14", got.Config.OverridePeriodDays)
	}
}

func TestL24_ApplyOverride_InvalidTime(t *testing.T) {
	m := newTestManager(t)
	for _, bad := range []string{"", "25:99", "9:30", "13:45am", "noon"} {
		if err := m.ApplyOverride(bad, 7); err == nil {
			t.Errorf("expected error for time %q", bad)
		}
	}
}

func TestL24_ApplyOverride_InvalidPeriod(t *testing.T) {
	m := newTestManager(t)
	for _, bad := range []int{0, -1, 31, 365} {
		if err := m.ApplyOverride("13:45", bad); err == nil {
			t.Errorf("expected error for period %d", bad)
		}
	}
}

func TestL24_Reset_ClearsOverrideOnly(t *testing.T) {
	m := newTestManager(t)
	_ = m.SetConfig(L24ScheduleConfig{DefaultStartTime: "13:45", DefaultPeriodDays: 7})
	_ = m.Start()
	_ = m.ApplyOverride("14:30", 14)
	if err := m.Reset(); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	got := m.Get()
	if got.Config.OverrideStartTime != "" {
		t.Errorf("OverrideStartTime should be empty after Reset, got %q", got.Config.OverrideStartTime)
	}
	if got.Config.OverridePeriodDays != 0 {
		t.Errorf("OverridePeriodDays should be 0 after Reset, got %d", got.Config.OverridePeriodDays)
	}
	if !got.Status.Running {
		t.Error("Status.Running should be preserved by Reset")
	}
}

func TestL24_Persistence_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	m1 := NewL24StateManager(dir)
	_ = m1.SetConfig(L24ScheduleConfig{DefaultStartTime: "13:45", DefaultPeriodDays: 7})
	_ = m1.Start()
	_ = m1.ApplyOverride("14:30", 14)

	// Construct a new manager pointing at the same file
	m2 := NewL24StateManager(dir)
	got := m2.Get()
	if !got.Status.Running {
		t.Error("running state should survive restart")
	}
	if got.Config.OverrideStartTime != "14:30" {
		t.Errorf("override should survive restart, got %q", got.Config.OverrideStartTime)
	}
	if got.Config.OverridePeriodDays != 14 {
		t.Errorf("override period should survive restart, got %d", got.Config.OverridePeriodDays)
	}
	// Verify file exists
	if _, err := os.Stat(filepath.Join(dir, "data/state/l2-4-schedule.json")); err != nil {
		t.Errorf("state file should exist: %v", err)
	}
}

func TestL24_Start_UsesOverridePeriod(t *testing.T) {
	m := newTestManager(t)
	_ = m.SetConfig(L24ScheduleConfig{DefaultStartTime: "13:45", DefaultPeriodDays: 7})
	_ = m.ApplyOverride("14:30", 10)
	if err := m.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	got := m.Get()
	if got.Status.CurrentPeriodDays != 10 {
		t.Errorf("CurrentPeriodDays = %d, want 10 (from override)", got.Status.CurrentPeriodDays)
	}
}
