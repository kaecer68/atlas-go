package main

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/testutil/snapshot"
)

// TestMain_PublicAPI locks the EXPORTED surface of the cmd/atlas/ package
// after #611 sub-issue-2 extraction. In package main, almost all top-level
// functions are unexported (lowercase, e.g. run, runSimulation, runLiveTrading).
// The snapshot tool (internal/testutil/snapshot.CaptureAPI) intentionally
// only captures identifiers whose name starts with an uppercase letter.
//
// After sub-issue-2 Wave 2+ extraction, exported symbols can live in any
// .go file in this package. The test walks all non-test .go files and
// aggregates their exported API into a single snapshot for comparison.
//
// As of 2026-06-22 the locked exported surface is two symbols:
//
//	(*experimentMonitorAdapter).Alert(string, string, string, map[string]any)
//	    (in bootstrap_helpers.go)
//	RegisterAdminRoutes(*http.ServeMux, config.Config)
//	    (in admin_routes.go)
//
// Anything else is package-private and protected by the per-function
// test files (run_simulation_test.go, run_live_test.go,
// run_simulation_mode_test.go, load_calibration_orders_test.go).
//
// Any addition or removal of an exported symbol during #611 refactor
// (sub-issue-2: cmd/atlas/ package split) fails this test. If you intend
// to add a new exported func, copy testdata/main_api.actual.json over
// testdata/main_api.golden.json after diff review.
func TestMain_PublicAPI(t *testing.T) {
	merged := snapshot.APISnapshot{Package: "main"}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		snap, err := snapshot.CaptureAPI(name)
		if err != nil {
			t.Fatalf("CaptureAPI(%s): %v", name, err)
		}
		merged.Funcs = append(merged.Funcs, snap.Funcs...)
		merged.Types = append(merged.Types, snap.Types...)
		merged.Consts = append(merged.Consts, snap.Consts...)
		merged.Vars = append(merged.Vars, snap.Vars...)
	}

	// Deterministic ordering — CaptureAPI sorts per-file, but the merge
	// across files can interleave. Re-sort the merged slice.
	sort.Slice(merged.Funcs, func(i, j int) bool {
		return funcKeyForSort(merged.Funcs[i]) < funcKeyForSort(merged.Funcs[j])
	})
	sort.Slice(merged.Types, func(i, j int) bool { return merged.Types[i].Name < merged.Types[j].Name })
	sort.Slice(merged.Consts, func(i, j int) bool { return merged.Consts[i].Name < merged.Consts[j].Name })
	sort.Slice(merged.Vars, func(i, j int) bool { return merged.Vars[i].Name < merged.Vars[j].Name })

	snapshot.AssertAPI(t, merged, "testdata/main_api.golden.json")
}

// funcKeyForSort mirrors snapshot.funcKey for the test side (we cannot
// import the unexported helper from the test package).
func funcKeyForSort(f snapshot.FuncSig) string {
	if f.Receiver != "" {
		return f.Receiver + "." + f.Name
	}
	return f.Name
}

// TestShouldStartFubonProxy_Golden locks the boolean decision logic that
// gates the Fubon proxy. This gate is part of the live-trading activation
// path; changing its truth table during refactor would silently enable or
// disable real-money broker connections.
//
// Truth table (4 cases, all must be stable across refactor):
//
//	mode="live"      key=""      → true  (live mode always starts proxy)
//	mode="live"      key="abc"   → true  (live + explicit key)
//	mode="simulation" key=""      → false (no live without live mode)
//	mode="simulation" key="abc"   → true  (key alone forces proxy)
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
