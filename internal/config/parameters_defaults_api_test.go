package config_test

import (
	"encoding/json"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/testutil/snapshot"
)

// TestParametersDefaults_PublicAPI locks the public API surface of
// parameters_defaults.go. Any addition, removal, or signature change to
// exported funcs/types/consts will fail this test, forcing the refactor
// (#611 sub-issue-1) to be intentional and reviewable.
//
// To accept a change: diff testdata/parameters_defaults_api.actual.json
// against parameters_defaults_api.golden.json, then copy actual → golden.
func TestParametersDefaults_PublicAPI(t *testing.T) {
	const sourcePath = "parameters_defaults.go"

	snap, err := snapshot.CaptureAPI(sourcePath)
	if err != nil {
		t.Fatalf("CaptureAPI(%s): %v", sourcePath, err)
	}

	// Sanity: DefaultParametersConfig must be the single exported function.
	// If more funcs are added (e.g. helper extractions during refactor), the
	// golden file must be updated consciously.
	if len(snap.Funcs) != 1 {
		t.Logf("warning: expected exactly 1 exported function, got %d", len(snap.Funcs))
		for _, f := range snap.Funcs {
			t.Logf("  func %s%s %s", f.Name, f.Params, f.Results)
		}
	}

	snapshot.AssertAPI(t, snap, "testdata/parameters_defaults_api.golden.json")
}

// TestDefaultParametersConfig_Golden locks the full default-config output.
// DefaultParametersConfig is the canonical reference for every default
// value in the system (27+ default funcs inlined). Any change to default
// values during refactor will fail this test, protecting downstream
// dependencies that rely on these defaults.
//
// KNOWN ISSUE (Layer 4 backlog): DefaultParametersConfig currently sets
// `updated_at = time.Now()` on every call, which is non-deterministic and
// not appropriate for a "default" function. Until that's fixed, this test
// strips `updated_at` from comparison — we lock behavior, not metadata.
// To fix: change `UpdatedAt: time.Now().UTC()` in parameters_defaults.go
// to a deterministic value (e.g. compile-time constant or zero value).
//
// To accept a behavioral change: inspect testdata/default_parameters_config.actual.json,
// confirm the change is intentional, then copy actual → golden.
func TestDefaultParametersConfig_Golden(t *testing.T) {
	cfg := config.DefaultParametersConfig()
	if cfg == nil {
		t.Fatal("DefaultParametersConfig returned nil")
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal for normalization: %v", err)
	}
	delete(raw, "updated_at")
	normalized, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}

	snapshot.AssertGoldenJSON(t, json.RawMessage(normalized), "testdata/default_parameters_config.golden.json")
}

// BenchmarkDefaultParametersConfig captures the cost of building the full
// default config. Refactor must not regress this — the function is called
// at startup and on every LoadParametersConfig fallback.
func BenchmarkDefaultParametersConfig(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = config.DefaultParametersConfig()
	}
}
