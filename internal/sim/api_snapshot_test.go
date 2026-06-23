// Package sim_test exercises the public API surface of internal/sim via the
// Layer 3 snapshot safety net (Issue #611 sub-issue-9).
//
// Any future refactor that:
//
//   - renames an exported symbol (func / type / const / var)
//   - changes a signature (params or results)
//   - adds a new public symbol (intentional additions must update the golden)
//   - removes a public symbol
//
// will fail this test, forcing a conscious review.
//
// Layer 3 safety net (Issue #611 sub-issue-9).
package sim_test

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/testutil/snapshot"
)

func TestSim_PublicAPI(t *testing.T) {
	files := []string{
		"doc.go",
		"dynamic_threshold.go",
		"engine.go",
		"slippage_model.go",
		"state_persistence.go",
	}

	paths := make([]string, len(files))
	copy(paths, files)

	snap, err := snapshot.CaptureAPIs(paths...)
	if err != nil {
		t.Fatalf("capture API: %v", err)
	}

	snapshot.AssertAPI(t, snap, "testdata/sim_api.golden.json")
}
