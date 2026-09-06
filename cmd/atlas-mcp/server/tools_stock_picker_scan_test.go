package server

import (
	"context"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// callStockPickerScan is a tiny wrapper reducing boilerplate.
func callStockPickerScan(s *server, in stockPickerScanInput) (stockPickerScanOutput, error) {
	_, out, err := s.handleStockPickerScan(context.Background(), nil, in)
	return out, err
}

// TestHandleStockPickerScan_HasData: the seeded 2330 summary (2 conditions,
// 3 observations each, eligible) is returned for the foreign condition when
// the default filters are met.
func TestHandleStockPickerScan_HasData(t *testing.T) {
	s, _ := stockWinRateHarness(t)
	// Seed summaries have 3 observations; lower the min to see them.
	out, err := callStockPickerScan(s, stockPickerScanInput{ConditionID: "foreign-3d-net-buy", MinObservations: 3})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !out.Found {
		t.Fatalf("found=false, want true (message=%q)", out.Message)
	}
	if out.Total != 1 {
		t.Fatalf("total = %d, want 1", out.Total)
	}
	if len(out.Candidates) != 1 {
		t.Fatalf("candidates = %d, want 1", len(out.Candidates))
	}
	c := out.Candidates[0]
	if c.Symbol != "2330" {
		t.Errorf("symbol = %q, want 2330", c.Symbol)
	}
	if c.CalibrationStatus != "eligible" {
		t.Errorf("calibration_status = %q, want eligible", c.CalibrationStatus)
	}
	if c.WinRate != 2.0/3.0 {
		t.Errorf("win_rate = %v, want %v", c.WinRate, 2.0/3.0)
	}
}

// TestHandleStockPickerScan_Filters: min_win_rate above the seeded rate
// returns no candidates (found=false with a clear message).
func TestHandleStockPickerScan_Filters(t *testing.T) {
	s, _ := stockWinRateHarness(t)
	out, err := callStockPickerScan(s, stockPickerScanInput{MinWinRate: 0.99})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Found {
		t.Fatalf("found=true, want false (min_win_rate too high)")
	}
	if !strings.Contains(out.Message, "no candidates") && !strings.Contains(out.Message, "no stockpicker") {
		t.Errorf("message = %q, want no-data mention", out.Message)
	}
}

// TestHandleStockPickerScan_DBUnconfigured: no winRateDB wired → found=false.
func TestHandleStockPickerScan_DBUnconfigured(t *testing.T) {
	s, _, done := newTestHarness(t)
	t.Cleanup(done)
	out, err := callStockPickerScan(s, stockPickerScanInput{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Found {
		t.Fatal("found=true, want false (db unconfigured)")
	}
}

// TestHandleStockPickerScan_InvalidParams: out-of-range params are rejected.
func TestHandleStockPickerScan_InvalidParams(t *testing.T) {
	s, _ := stockWinRateHarness(t)
	if _, err := callStockPickerScan(s, stockPickerScanInput{MinWinRate: 1.5}); err == nil {
		t.Fatal("min_win_rate > 1 must error")
	}
	if _, err := callStockPickerScan(s, stockPickerScanInput{SortBy: "bogus"}); err == nil {
		t.Fatal("unknown sort_by must error")
	}
}

// TestScanWinRateRows_Query: direct query with the seeded DB returns the
// foreign-3d-net-buy row sorted by wilson_lower.
func TestScanWinRateRows_Query(t *testing.T) {
	db, _ := openStockWinRateTestDB(t)
	rows, err := scanWinRateRows(context.Background(), db, "120d", "foreign-3d-net-buy", 3, 0.5, "wilson_lower", "buy")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].Symbol != "2330" {
		t.Errorf("symbol = %q, want 2330", rows[0].Symbol)
	}
}

// TestScanWinRateRows_AvoidDirection (k3 review F1): an avoid-semantics
// condition (price-volume-top-divergence 頂背離) is CONFIRMED by low
// forward win rates. Buy-mode defaults (win_rate >= 0.5, wilson_lower DESC)
// must hide such rows; direction=avoid must surface them first.
func TestScanWinRateRows_AvoidDirection(t *testing.T) {
	db, err := ledger.OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	ctx := context.Background()
	winStore := stockpicker.NewWinRateStore(db)
	// A strong avoid signal: 80% of post-trigger windows LOST money.
	avoid := stockpicker.StockWinRateSummary{
		Symbol:            "2330",
		Source:            "stockpicker-price-volume-top-divergence",
		Window:            "120d",
		Observations:      40,
		Hits:              8,
		WinRate:           0.20,
		WilsonLower:       0.09,
		WilsonUpper:       0.31,
		Confidence:        0.95,
		CalibrationStatus: stockpicker.CalibrationEligible,
		NetCostRate:       0.00585,
		AvgForwardReturn:  -0.03,
		UpdatedAt:         "2026-09-07T00:00:00Z",
	}
	// A normal buy-side row for contrast.
	buy := stockpicker.StockWinRateSummary{
		Symbol:            "2330",
		Source:            "stockpicker-foreign-3d-net-buy",
		Window:            "120d",
		Observations:      40,
		Hits:              30,
		WinRate:           0.75,
		WilsonLower:       0.60,
		WilsonUpper:       0.86,
		Confidence:        0.95,
		CalibrationStatus: stockpicker.CalibrationEligible,
		NetCostRate:       0.00585,
		AvgForwardReturn:  0.02,
		UpdatedAt:         "2026-09-07T00:00:00Z",
	}
	for _, s := range []stockpicker.StockWinRateSummary{avoid, buy} {
		if err := winStore.SaveWinRate(ctx, s); err != nil {
			t.Fatalf("save %s: %v", s.Source, err)
		}
	}

	// Buy mode: the avoid row (win_rate 0.20 < 0.5) must be filtered out.
	rows, err := scanWinRateRows(ctx, db, "120d", "", 20, 0.5, "wilson_lower", "buy")
	if err != nil {
		t.Fatalf("buy scan: %v", err)
	}
	for _, r := range rows {
		if r.Source == avoid.Source {
			t.Fatalf("buy mode must not surface avoid row: %+v", r)
		}
	}

	// Avoid mode: the row must surface, ranked first (lowest wilson_upper).
	rows, err = scanWinRateRows(ctx, db, "120d", "", 20, 0.5, "wilson_lower", "avoid")
	if err != nil {
		t.Fatalf("avoid scan: %v", err)
	}
	if len(rows) == 0 || rows[0].Source != avoid.Source {
		t.Fatalf("avoid mode must surface the top-divergence row first: %+v", rows)
	}
	// The buy row (win_rate 0.75 > ceiling 0.5) must not appear.
	for _, r := range rows {
		if r.Source == buy.Source {
			t.Fatalf("avoid mode must filter out buy row: %+v", r)
		}
	}
}

func TestHandleStockPickerScan_InvalidDirection(t *testing.T) {
	s, _, done := newTestHarness(t)
	defer done()
	_, _, err := s.handleStockPickerScan(context.Background(), nil, stockPickerScanInput{Direction: "sideways"})
	if err == nil {
		t.Fatal("expected error for invalid direction")
	}
}
