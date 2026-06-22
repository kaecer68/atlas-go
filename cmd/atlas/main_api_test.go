package main

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/testutil/snapshot"
)

// TestMain_PublicAPI locks the exported surface of cmd/atlas/main.go: the
// 14 exported funcs (main, run, runSimulation, runLiveTrading,
// runSimulationMode, buildBaseState, staticHandler, loadCalibrationOrders,
// getLatestReplayDate, publishBootstrapEvents, shouldStartFubonProxy,
// narrativeFeedFetcher, newUniverseBuilderDeps, defaultAppDeps).
//
// Any change to public signatures during #611 refactor (sub-issue-2:
// cmd/atlas/ package split) fails this test.
func TestMain_PublicAPI(t *testing.T) {
	snap, err := snapshot.CaptureAPI("main.go")
	if err != nil {
		t.Fatalf("CaptureAPI: %v", err)
	}
	snapshot.AssertAPI(t, snap, "testdata/main_api.golden.json")
}

// TestShouldStartFubonProxy_Golden locks the boolean decision logic that
// gates the Fubon proxy. This gate is part of the live-trading activation
// path; changing its truth table during refactor would silently enable or
// disable real-money broker connections.
//
// Truth table (4 cases, all must be stable across refactor):
//   mode="live"      key=""      → true  (live mode always starts proxy)
//   mode="live"      key="abc"   → true  (live + explicit key)
//   mode="simulation" key=""      → false (no live without live mode)
//   mode="simulation" key="abc"   → true  (key alone forces proxy)
func TestShouldStartFubonProxy_Golden(t *testing.T) {
	cases := []struct {
		mode, key string
		want      bool
	}{
		{"live", "", true},
		{"live", "abc", true},
		{"simulation", "", false},
		{"simulation", "abc", true},
	}
	for _, tc := range cases {
		got := shouldStartFubonProxy(tc.mode, tc.key)
		if got != tc.want {
			t.Errorf("shouldStartFubonProxy(%q, %q) = %v, want %v",
				tc.mode, tc.key, got, tc.want)
		}
	}
}

// TestDefaultAppDeps_Golden locks the default dependency-injection shape.
// After #611 sub-issue-2 (cmd/atlas/ package split), this struct must
// remain constructible with no external inputs and must leave dataFetcher
// nil (live paths override it).
func TestDefaultAppDeps_Golden(t *testing.T) {
	deps := defaultAppDeps()

	if deps.loadConfig == nil {
		t.Error("loadConfig must be non-nil (defaults to config.Load)")
	}
	if deps.newDashboardAPI == nil {
		t.Error("newDashboardAPI must be non-nil")
	}
	if deps.listenAndServe == nil {
		t.Error("listenAndServe must be non-nil (defaults to http.Server.ListenAndServe)")
	}
	if deps.shutdown == nil {
		t.Error("shutdown channel must be initialized (non-nil make(chan struct{}))")
	}
	if deps.dataFetcher != nil {
		t.Error("dataFetcher must be nil in default deps (live paths override)")
	}
}

// BenchmarkShouldStartFubonProxy captures the cost of the boolean gate.
// Trivially fast today; lock as a sanity baseline.
func BenchmarkShouldStartFubonProxy(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = shouldStartFubonProxy("simulation", "")
	}
}