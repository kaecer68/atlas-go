package main

import (
	"testing"
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

	_ = runLiveTrading(configStub(t), appDeps{}, nil, nil, false)
}
