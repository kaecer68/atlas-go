package orchestrator_test

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/testutil/snapshot"
)

// TestOrchestrator_PublicAPI locks the exported surface of the orchestrator
// package after the executors.go 1284→30-line split (sub-issue-8, PR #684).
//
// Captures 12 files: the package-doc stub executors.go plus the 11
// concern-separated files (types, strategies, pipeline, darwinian, policy,
// muted_filter, regime, collection, momentum_crash, control, symbols).
//
// Any refactor that accidentally:
//   - renames a public function/type/const/var
//   - changes a signature (params or results)
//   - adds a new public symbol (intentional additions must update the golden)
//   - removes a public symbol
//
// will fail this test, forcing a conscious review.
//
// Layer 3 safety net (Issue #611 sub-issue-9).
func TestOrchestrator_PublicAPI(t *testing.T) {
	files := []string{
		"executors.go",
		"executor_types.go",
		"executor_strategies.go",
		"executor_pipeline.go",
		"executor_darwinian.go",
		"executor_policy.go",
		"executor_muted_filter.go",
		"executor_regime.go",
		"executor_collection.go",
		"executor_momentum_crash.go",
		"executor_control.go",
		"executor_symbols.go",
	}
	snap, err := snapshot.CaptureAPIs(files...)
	if err != nil {
		t.Fatalf("CaptureAPIs: %v", err)
	}

	snapshot.AssertAPI(t, snap, "testdata/orchestrator_api.golden.json")
}
