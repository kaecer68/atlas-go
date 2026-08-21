package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestRegisterMaturityTrackerSaveTask_SavesState(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "maturity_tracker.json")

	tracker := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-90 * 24 * time.Hour))
	if tracker.Current() != domain.MaturityCalibrating {
		t.Fatalf("setup: expected CALIBRATING, got %q", tracker.Current())
	}

	mgr := apigateway.NewBackgroundTaskManager(nil)
	registerMaturityTrackerSaveTask(mgr, tracker, statePath)

	task, ok := mgr.Get("maturity_tracker_save")
	if !ok {
		t.Fatal("expected maturity_tracker_save to be registered")
	}
	if !task.IsEnabled() {
		t.Fatal("expected task to be enabled")
	}

	if err := task.Task(context.Background()); err != nil {
		t.Fatalf("save task failed: %v", err)
	}

	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("expected state file to be saved: %v", err)
	}

	loaded, err := domain.NewMaturityTracker(statePath)
	if err != nil {
		t.Fatalf("load saved state: %v", err)
	}
	if !loaded.FirstStartDate().Equal(tracker.FirstStartDate()) {
		t.Errorf("FirstStartDate mismatch: got %v, want %v", loaded.FirstStartDate(), tracker.FirstStartDate())
	}
}

func TestRegisterMaturityTrackerSaveTask_NilTracker(t *testing.T) {
	mgr := apigateway.NewBackgroundTaskManager(nil)
	registerMaturityTrackerSaveTask(mgr, nil, "/dev/null/maturity.json")

	if _, ok := mgr.Get("maturity_tracker_save"); ok {
		t.Fatal("expected no task registration when tracker is nil")
	}
}
