// Package risk_test exercises the public API surface of internal/risk via the
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
package risk_test

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/testutil/snapshot"
)

func TestRisk_PublicAPI(t *testing.T) {
	files := []string{
		"approval_workflow.go",
		"audit_subscriber.go",
		"capital_controller.go",
		"circuit_breaker.go",
		"confidence_hook.go",
		"copula.go",
		"dcc_garch.go",
		"decision.go",
		"doc.go",
		"dynamic_hedge.go",
		"forensics_hook.go",
		"gate.go",
		"in_trade.go",
		"industry_risk.go",
		"lead_lag.go",
		"macro_aware_drawdown.go",
		"portfolio_risk.go",
		"post_trade.go",
		"pre_trade.go",
		"self_calibrate.go",
		"spillover.go",
		"var_calculator.go",
	}

	paths := make([]string, len(files))
	copy(paths, files)

	snap, err := snapshot.CaptureAPIs(paths...)
	if err != nil {
		t.Fatalf("capture API: %v", err)
	}

	snapshot.AssertAPI(t, snap, "testdata/risk_api.golden.json")
}
