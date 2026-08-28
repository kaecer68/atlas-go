package server

import (
	"context"
	"strings"
	"testing"
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
	rows, err := scanWinRateRows(context.Background(), db, "120d", "foreign-3d-net-buy", 3, 0.5, "wilson_lower")
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
