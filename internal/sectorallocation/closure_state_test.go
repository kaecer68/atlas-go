package sectorallocation

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSACClosureStateManager_Default(t *testing.T) {
	dir := t.TempDir()
	m := NewSACClosureStateManager(dir)

	state := m.Get()
	if state.Enabled {
		t.Fatal("expected disabled by default")
	}
	if state.SessionCount != 0 {
		t.Fatalf("expected 0 sessions, got %d", state.SessionCount)
	}
	if state.ObservationWindow.Running {
		t.Fatal("expected observation not running")
	}
}

func TestSACClosureStateManager_SetEnabled(t *testing.T) {
	dir := t.TempDir()
	m := NewSACClosureStateManager(dir)

	if err := m.SetEnabled(true); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if !m.Get().Enabled {
		t.Fatal("expected enabled")
	}

	if err := m.SetEnabled(false); err != nil {
		t.Fatalf("SetEnabled(false): %v", err)
	}
	if m.Get().Enabled {
		t.Fatal("expected disabled")
	}
}

func TestSACClosureStateManager_Observation(t *testing.T) {
	dir := t.TempDir()
	m := NewSACClosureStateManager(dir)

	if err := m.StartObservation(14); err != nil {
		t.Fatalf("StartObservation: %v", err)
	}
	state := m.Get()
	if !state.ObservationWindow.Running {
		t.Fatal("expected running")
	}
	if state.ObservationWindow.Days != 14 {
		t.Fatalf("expected 14 days, got %d", state.ObservationWindow.Days)
	}
	if state.ObservationWindow.StartedAt == nil {
		t.Fatal("expected non-nil StartedAt")
	}

	// Restart should fail
	if err := m.StartObservation(7); err == nil {
		t.Fatal("expected error on double start")
	}

	if err := m.StopObservation(); err != nil {
		t.Fatalf("StopObservation: %v", err)
	}
	if m.Get().ObservationWindow.Running {
		t.Fatal("expected stopped")
	}

	// Stop when not running should fail
	if err := m.StopObservation(); err == nil {
		t.Fatal("expected error on stop when not running")
	}
}

func TestSACClosureStateManager_RecordSession(t *testing.T) {
	dir := t.TempDir()
	m := NewSACClosureStateManager(dir)

	for i := range 5 {
		if err := m.RecordSession("receipt-" + string(rune('a'+i))); err != nil {
			t.Fatalf("RecordSession %d: %v", i, err)
		}
	}
	state := m.Get()
	if state.SessionCount != 5 {
		t.Fatalf("expected 5 sessions, got %d", state.SessionCount)
	}
	if state.LastReceiptID != "receipt-e" {
		t.Fatalf("expected last receipt receipt-e, got %s", state.LastReceiptID)
	}
}

func TestSACClosureStateManager_InvariantViolations(t *testing.T) {
	dir := t.TempDir()
	m := NewSACClosureStateManager(dir)

	for range 3 {
		if err := m.RecordInvariantViolation(); err != nil {
			t.Fatalf("RecordInvariantViolation: %v", err)
		}
	}
	if m.Get().InvariantViolations != 3 {
		t.Fatalf("expected 3 violations, got %d", m.Get().InvariantViolations)
	}
}

func TestSACClosureStateManager_IsPromotable(t *testing.T) {
	dir := t.TempDir()
	m := NewSACClosureStateManager(dir)

	// Not promotable initially (0 sessions, not started)
	if m.IsPromotable() {
		t.Fatal("should not be promotable initially")
	}

	_ = m.StartObservation(14)
	for range 20 {
		_ = m.RecordSession("receipt")
	}

	// Still not promotable (observation running)
	if m.IsPromotable() {
		t.Fatal("should not be promotable while observation running")
	}

	_ = m.StopObservation()

	// Now promotable: ≥20 sessions, 0 violations, observation stopped
	if !m.IsPromotable() {
		t.Fatal("should be promotable")
	}

	// Add a violation → not promotable
	_ = m.RecordInvariantViolation()
	if m.IsPromotable() {
		t.Fatal("should not be promotable with violations")
	}
}

func TestSACClosureStateManager_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data", "state", sacStateFileName)

	m1 := NewSACClosureStateManager(dir)
	_ = m1.SetEnabled(true)
	_ = m1.StartObservation(30)
	_ = m1.RecordSession("abc")

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("state file not created")
	}

	// Create a new manager and verify it reads existing state
	m2 := NewSACClosureStateManager(dir)
	state := m2.Get()
	if !state.Enabled {
		t.Fatal("expected enabled from persisted state")
	}
	if !state.ObservationWindow.Running {
		t.Fatal("expected observation running from persisted state")
	}
	if state.SessionCount != 1 {
		t.Fatalf("expected 1 session, got %d", state.SessionCount)
	}
}
