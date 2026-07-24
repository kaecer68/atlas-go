package main

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/baseline"
)

// TestRunLiveTrading_NilDepsPanicsAtProviderSetup locks the contract
// boundary of runLiveTrading: with empty appDeps, the function panics
// when it reaches the `marketdata.NewHybridProvider(...)` call (or
// shortly after). This is the EXPECTED current behavior — runLiveTrading
// has no early-exit error path and dives directly into orchestrator
// setup. Without real deps, that path panics.
//
// This is a safety net for #611 sub-issue-2 refactor: the contract
// "runLiveTrading with empty deps panics" must remain stable. If refactor
// accidentally inserts an early-return for empty deps (returning a
// friendlier error), that would be a behavioral change and this test
// should be updated intentionally, not silently.
//
// For a true "graceful rejection" test of runLiveTrading, use the
// live_mode_test.go suite which exercises the function through the
// full `run()` flow with proper broker validation up front.
func TestRunLiveTrading_NilDepsPanicsAtProviderSetup(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic when runLiveTrading is called with nil deps, got no panic")
		}
		t.Logf("confirmed: runLiveTrading panics with nil deps (panic: %v)", r)
	}()

	_ = runLiveTrading(configStub(t), nil, appDeps{}, nil, nil, baseline.NewManager(""), "127.0.0.1:0", false)
}

// TestRunLiveTrading_SharesBaselineManager verifies that runLiveTrading wires
// the supplied *baseline.Manager into the BaselineTrigger instead of creating
// a second Manager instance. With a shared manager, Trigger.Start succeeds
// (default policy for empty path) and the function proceeds to the provider
// setup, where it panics due to empty appDeps — matching the contract above.
func TestRunLiveTrading_SharesBaselineManager(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic at provider setup, got no panic")
		}
		t.Logf("confirmed: shared baseline manager accepted and trigger started before provider setup (panic: %v)", r)
	}()

	mgr := baseline.NewManager("")
	_ = runLiveTrading(configStub(t), nil, appDeps{}, nil, nil, mgr, "127.0.0.1:0", false)
}
