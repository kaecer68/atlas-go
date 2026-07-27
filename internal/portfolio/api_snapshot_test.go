package portfolio_test

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/testutil/snapshot"
)

// TestPortfolio_PublicAPI locks the exported surface of the portfolio
// package after the optimizer.go split (sub-issue-6: #680/#677/#683 merged)
// and DarwinianWeightManager refactor (sub-issue-1).
//
// Captures all 31 non-test files in the portfolio package:
//   - agent_health, analysis, capital_allocator, conviction_normalizer
//   - darwinian_weights, doc, etf_analysis, factor_bridge, factor_engine
//   - factor_weight_calibrator, factor_weight_engine, fundamental_loader
//   - historical_prices, optimizer (+ 4 split files), oscillation_detector
//   - parameter_runtime, regime, risk_adjuster, risk_manager, sector_rotator
//   - sharpe, sizing, stress_index, style, volatility_manager, window_splitter
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
func TestPortfolio_PublicAPI(t *testing.T) {
	files := []string{
		"agent_health.go",
		"agent_health_store.go",
		"analysis.go",
		"capital_allocator.go",
		"conviction_normalizer.go",
		"darwinian_weights.go",
		"doc.go",
		"etf_analysis.go",
		"factor_bridge.go",
		"factor_engine.go",
		"factor_engine_aggregate.go",
		"factor_engine_constructors.go",
		"factor_engine_etf.go",
		"factor_engine_helpers.go",
		"factor_engine_institutional.go",
		"factor_engine_liquidity.go",
		"factor_engine_momentum.go",
		"factor_engine_precious_metals.go",
		"factor_engine_quality.go",
		"factor_engine_types.go",
		"factor_engine_value.go",
		"factor_weight_calibrator.go",
		"factor_weight_engine.go",
		"fundamental_loader.go",
		"historical_prices.go",
		"optimizer.go",
		"optimizer_drawdown.go",
		"optimizer_frontier.go",
		"optimizer_math.go",
		"optimizer_pipeline.go",
		"oscillation_detector.go",
		"period_detector.go",
		"parameter_runtime.go",
		"regime.go",
		"risk_adjuster.go",
		"risk_manager.go",
		"sector_rotator.go",
		"sharpe.go",
		"sizing.go",
		"stress_index.go",
		"style.go",
		"volatility_manager.go",
		"window_splitter.go",
	}
	snap, err := snapshot.CaptureAPIs(files...)
	if err != nil {
		t.Fatalf("CaptureAPIs: %v", err)
	}

	snapshot.AssertAPI(t, snap, "testdata/portfolio_api.golden.json")
}
