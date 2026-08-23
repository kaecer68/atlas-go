package main

import (
	"os"
	"path/filepath"
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
//
// The shutdown channel is pre-closed, so runSimulation must abort via the
// pre-goroutine shutdown check (main.go runSimulation) and never spawn the
// background simulation goroutine. That goroutine would otherwise keep
// writing session/trace files into cfg.LedgerDir after this test returns,
// racing t.TempDir() cleanup on slow CI runners ("directory not empty:
// unlinkat" — pre-existing flake). The ledger-content guard below locks
// this: no simulation artifacts may appear in LedgerDir.
func TestRunSimulation_ImmediateShutdownReturnsShutdownError(t *testing.T) {
	shutdown := make(chan struct{})
	close(shutdown) // pre-close so the shutdown check fires immediately

	cfg := configStub(t)
	err := runSimulation(cfg, nil, false, nil, nil, shutdown)
	if err == nil {
		t.Fatal("expected shutdown error, got nil")
	}
	if got, want := err.Error(), "simulation: shutdown"; got != want {
		t.Errorf("shutdown error = %q, want %q", got, want)
	}

	// Regression guard for the pre-goroutine shutdown abort: the background
	// simulation goroutine must not have started, so LedgerDir must contain
	// no simulation-run artifacts. ("traces/" is NOT checked: it is created
	// synchronously during system build by NewScratchpad — the goroutine-only
	// artifacts are the per-session files below.) If a future refactor spawns
	// the goroutine before honoring a pre-closed shutdown, this test fails
	// instead of flaking against t.TempDir() cleanup.
	for _, name := range []string{"sessions", "recommendation_outcomes.jsonl"} {
		if _, err := os.Stat(filepath.Join(cfg.LedgerDir, name)); err == nil {
			t.Errorf("simulation goroutine leaked artifact %q into LedgerDir after shutdown abort", name)
		}
	}
}

// TestRunSimulation_NilCollectorNilRepoDoesNotPanic locks the dependency-
// optional branches in runSimulation: collector and repo are both passed
// as nil here, exercising the `if collector != nil` and `if repo != nil`
// guards. The shutdown channel is pre-closed to make the test deterministic.
//
// This catches a class of refactor regressions where the optional injection
// becomes mandatory. Pre-closing shutdown also means the background
// simulation goroutine must never start (see the shutdown-abort contract in
// TestRunSimulation_ImmediateShutdownReturnsShutdownError), so the test
// never races its temp-dir cleanup.
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
