// Package-level test (no production code yet) — these tests are RED until
// internal/buildinfo/info.go is implemented in Task 10. Once Info/Version/
// Current exist, every assertion in this file must turn GREEN.
//
// Test contract (from docs/specs/capital-flow-seven-dimension-spec.md §11.4
// and Task 9 brief):
//
//   - buildinfo.Info has 4 fields: Version, Commit, BuildTime, GoVersion.
//   - JSON tags: "version", "commit", "build_time", "go_version".
//   - Defaults (no -ldflags injected): Version == Commit == BuildTime == "unknown".
//   - GoVersion auto-filled from runtime.Version(), never empty.
//   - Current() reads package vars at call time (so a test can mutate Version
//     and observe the change).
//   - No shell or git command is invoked — Current() must be cheap and pure.

package buildinfo

import (
	"encoding/json"
	"runtime"
	"testing"
)

// TestCurrent_DefaultsAreUnknown verifies the spec'd default values when no
// -ldflags injection has happened (which is the case during a plain `go test`).
func TestCurrent_DefaultsAreUnknown(t *testing.T) {
	info := Current()
	if info.Version != "unknown" {
		t.Errorf("Version: want %q, got %q", "unknown", info.Version)
	}
	if info.Commit != "unknown" {
		t.Errorf("Commit: want %q, got %q", "unknown", info.Commit)
	}
	if info.BuildTime != "unknown" {
		t.Errorf("BuildTime: want %q, got %q", "unknown", info.BuildTime)
	}
	if info.GoVersion == "" {
		t.Error("GoVersion must be non-empty (auto-filled from runtime.Version())")
	}
	if info.GoVersion != runtime.Version() {
		t.Errorf("GoVersion: want %q, got %q", runtime.Version(), info.GoVersion)
	}
}

// TestCurrent_InjectedVersion verifies that Current() reads the package-level
// var at call time (not cached at init), so test code (and the actual
// -ldflags injection at startup) can override Version without a rebuild.
func TestCurrent_InjectedVersion(t *testing.T) {
	originalVer := Version
	t.Cleanup(func() { Version = originalVer })

	Version = "v0.0.0.32"
	info := Current()
	if info.Version != "v0.0.0.32" {
		t.Errorf("Version after injection: want %q, got %q", "v0.0.0.32", info.Version)
	}
}

// TestInfo_JSONTags verifies that marshalling Info yields the snake_case keys
// promised in the brief (so downstream consumers / logs stay stable).
func TestInfo_JSONTags(t *testing.T) {
	originalVer, originalCommit, originalBuildTime := Version, Commit, BuildTime
	t.Cleanup(func() {
		Version = originalVer
		Commit = originalCommit
		BuildTime = originalBuildTime
	})

	Version = "v0.0.0.32"
	Commit = "abc1234"
	BuildTime = "2026-07-17T00:00:00Z"

	data, err := json.Marshal(Current())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := map[string]string{
		"version":    "v0.0.0.32",
		"commit":     "abc1234",
		"build_time": "2026-07-17T00:00:00Z",
		"go_version": runtime.Version(),
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("JSON key %q: want %q, got %q", k, v, got[k])
		}
	}
}

// TestCurrent_IsDeterministic guards against accidental shell-injected
// implementation (e.g. shelling out to `git rev-parse HEAD` on every call).
// If such implementation crept in, parallel tests would race and the values
// would still match within one process — so this is a belt-and-suspenders
// check rather than a behavioral assertion. Combined with the
// "no os/exec in this package" code review rule, it documents intent.
func TestCurrent_IsDeterministic(t *testing.T) {
	first := Current()
	second := Current()
	if first.Version != second.Version || first.Commit != second.Commit || first.BuildTime != second.BuildTime {
		t.Errorf("Current() non-deterministic: %+v vs %+v", first, second)
	}
}
