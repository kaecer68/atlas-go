package stockpicker

import (
	"context"
	"testing"

	"github.com/kaecer68/atlas-go/internal/ledger"
)

// TestLoadWinRate_NullableCostColumnsReadAsZero is the regression test for the
// second half of the 2026-10-12 production P0. After the additive migration
// added break_even_cost_rate, /api/stock/win_rate still answered HTTP 503:
//
//	Scan error on column index 12, name "break_even_cost_rate":
//	converting NULL to float64 is unsupported
//
// because every legacy row is NULL in the three nullable cost/return columns
// while the reader scanned them into bare float64. COALESCE(...,0) makes the
// read succeed and reports the documented "not computed yet" value instead of
// failing the request. Fails without the COALESCE in LoadWinRate.
func TestLoadWinRate_NullableCostColumnsReadAsZero(t *testing.T) {
	db, err := ledger.OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}

	// A row exactly as the pre-Phase-4 writer left it: the three nullable
	// columns are NULL.
	if _, err := db.Exec(`INSERT INTO stock_win_rate
		(symbol, source, rolling_window, observations, hits, win_rate,
		 wilson_lower, wilson_upper, confidence, calibration_status,
		 net_cost_rate, avg_forward_return, break_even_cost_rate, updated_at)
		VALUES ('2330', 'stockpicker-price-volume-bottom-divergence', 'all',
		        40, 22, 0.55, 0.41, 0.68, 0.9, 'eligible',
		        NULL, NULL, NULL, '2026-10-08T15:16:33Z')`); err != nil {
		t.Fatalf("insert legacy NULL row: %v", err)
	}

	got, ok, err := LoadWinRate(context.Background(), db, "2330",
		"stockpicker-price-volume-bottom-divergence", "all")
	if err != nil {
		t.Fatalf("NULL cost columns must not fail the read (prod 503): %v", err)
	}
	if !ok {
		t.Fatal("row must be found")
	}
	if got.BreakEvenCostRate != 0 || got.NetCostRate != 0 || got.AvgForwardReturn != 0 {
		t.Fatalf("NULLs must read as 0, got net=%v avg=%v breakEven=%v",
			got.NetCostRate, got.AvgForwardReturn, got.BreakEvenCostRate)
	}
	if got.WinRate != 0.55 || got.Observations != 40 {
		t.Fatalf("non-null columns must be unaffected, got %+v", got)
	}
}
