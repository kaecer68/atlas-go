package main

import (
	"testing"
)

// TestRunSimulation_ImmediateShutdownReturnsShutdownError locks the
// shutdown-channel race: when shutdown fires before the simulation
// goroutine completes, runSimulation must return a "simulation: shutdown"
// error rather than blocking forever or returning the inner error.
//
// This is a safety net for #611 sub-issue-2 refactor (cmd/atlas/ package
// split): the shutdown signal handling must remain observable to callers
// without depending on real market data.
func TestRunSimulation_ImmediateShutdownReturnsShutdownError(t *testing.T) {
	shutdown := make(chan struct{})
	close(shutdown) // pre-close so the select fires immediately

	err := runSimulation(configStub(t), nil, false, nil, nil, shutdown)
	if err == nil {
		t.Fatal("expected shutdown error, got nil")
	}
	if got, want := err.Error(), "simulation: shutdown"; got != want {
		t.Errorf("shutdown error = %q, want %q", got, want)
	}
}

// TestRunSimulation_NilCollectorNilRepoDoesNotPanic locks the dependency-
// optional branches in runSimulation: collector and repo are both passed
// as nil here, exercising the `if collector != nil` and `if repo != nil`
// guards. The shutdown channel is pre-closed to make the test deterministic.
//
// This catches a class of refactor regressions where the optional injection
// becomes mandatory.
func TestRunSimulation_NilCollectorNilRepoDoesNotPanic(t *testing.T) {
	shutdown := make(chan struct{})
	close(shutdown)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("runSimulation panicked with nil collector/repo: %v", r)
		}
	}()

	_ = runSimulation(configStub(t), nil, false, nil, nil, shutdown)
}
