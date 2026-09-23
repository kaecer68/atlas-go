package apigateway

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// waitFor polls cond until it is true or the deadline expires.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func countingTask(name string, runs *atomic.Int64) *ScheduledTask {
	return &ScheduledTask{
		Name:     name,
		Interval: time.Hour,
		Enabled:  true,
		Task: func(context.Context) error {
			runs.Add(1)
			return nil
		},
	}
}

// TestBackgroundTaskManager_RegisterAndStart_SchedulesTaskOnRunningManager
// pins the contract the fubon self-heal path depends on: a task registered
// while the manager is already running actually executes.
//
// Start() snapshots the registry once, so a plain Register() at runtime leaves
// the task in the map unrun — "registered on paper, never scheduled", which is
// indistinguishable from a healthy registration on any dashboard.
func TestBackgroundTaskManager_RegisterAndStart_SchedulesTaskOnRunningManager(t *testing.T) {
	mgr := NewBackgroundTaskManager(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var preRuns atomic.Int64
	if err := mgr.RegisterAndStart(countingTask("pre_start_task", &preRuns)); err != nil {
		t.Fatalf("RegisterAndStart before Start() = %v, want nil", err)
	}
	if _, ok := mgr.Get("pre_start_task"); !ok {
		t.Fatal("task was not registered before Start()")
	}
	time.Sleep(50 * time.Millisecond)
	if got := preRuns.Load(); got != 0 {
		t.Fatalf("pre-Start registration ran %d times, want 0 (no manager loop yet)", got)
	}

	mgr.Start(ctx)
	defer mgr.Stop()
	waitFor(t, "pre-Start task first run", func() bool { return preRuns.Load() >= 1 })

	var runtimeRuns atomic.Int64
	if err := mgr.RegisterAndStart(countingTask("runtime_task", &runtimeRuns)); err != nil {
		t.Fatalf("RegisterAndStart on a running manager = %v, want nil", err)
	}
	waitFor(t, "runtime-registered task first run", func() bool { return runtimeRuns.Load() >= 1 })
}

func TestBackgroundTaskManager_RegisterAndStart_RejectsDuplicateName(t *testing.T) {
	mgr := NewBackgroundTaskManager(nil)
	var runs atomic.Int64

	if err := mgr.RegisterAndStart(countingTask("dup_task", &runs)); err != nil {
		t.Fatalf("first RegisterAndStart = %v, want nil", err)
	}
	err := mgr.RegisterAndStart(countingTask("dup_task", &runs))
	if err == nil {
		t.Fatal("second RegisterAndStart with the same name = nil, want a duplicate-name error")
	}
	if !strings.Contains(err.Error(), "duplicate task name") {
		t.Errorf("duplicate error = %v, want it to mention duplicate task name", err)
	}
	if got := len(mgr.List()); got != 1 {
		t.Errorf("registered tasks = %v, want exactly one entry", mgr.List())
	}
}
