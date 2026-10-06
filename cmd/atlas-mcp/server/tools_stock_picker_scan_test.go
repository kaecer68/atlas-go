package server

import (
	"context"
	"database/sql"
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

// The demotion contract under test (2026-10-06): stock_picker_scan ranks
// candidates for a reader who acts on them, so the two families with negative
// 5-day net-cost expectancy (foreign-3d-net-buy -0.517%, t=-11.4;
// momentum-20d-positive -0.987%, t=-21.3 — internal/config/stockpicker_edge.go)
// must never appear in its output, while the two retained families
// (price-volume-top-divergence, price-volume-bottom-divergence) keep ranking and
// the persisted rows stay readable through stock_get_win_rate.
const (
	retainedScanSource  = "stockpicker-price-volume-bottom-divergence"
	retainedScanCond    = "price-volume-bottom-divergence"
	retainedScanSource2 = "stockpicker-price-volume-top-divergence"
	retainedScanCond2   = "price-volume-top-divergence"
)

// seedRetainedFamilies adds one calibration-eligible summary per RETAINED
// family to the caller's ledger, so a scan can be shown ranking what is still
// allowed. It mirrors the fixture shape the shared harness uses for the
// demoted families (3 observations, 2 hits, eligible), so both groups are
// comparable under the same filters.
func seedRetainedFamilies(t *testing.T, db *sql.DB) []string {
	t.Helper()
	ctx := context.Background()
	winStore := stockpicker.NewWinRateStore(db)
	for _, src := range []string{retainedScanSource, retainedScanSource2} {
		summary := stockpicker.StockWinRateSummary{
			Symbol:            "2330",
			Source:            src,
			Window:            "120d",
			Observations:      3,
			Hits:              2,
			WinRate:           2.0 / 3.0,
			WilsonLower:       0.15,
			WilsonUpper:       0.90,
			Confidence:        0.95,
			CalibrationStatus: stockpicker.CalibrationEligible,
			NetCostRate:       0.00585,
			AvgForwardReturn:  0.01,
			UpdatedAt:         "2026-08-27T12:00:00Z",
		}
		if err := winStore.SaveWinRate(ctx, summary); err != nil {
			t.Fatalf("save retained win rate %s: %v", src, err)
		}
	}
	return []string{retainedScanSource, retainedScanSource2}
}

// TestHandleStockPickerScan_HasData: a retained family's 2330 summary (3
// observations, eligible) is returned when the default filters are met.
func TestHandleStockPickerScan_HasData(t *testing.T) {
	s, db := stockWinRateHarness(t)
	seedRetainedFamilies(t, db)
	// Seed summaries have 3 observations; lower the min to see them.
	out, err := callStockPickerScan(s, stockPickerScanInput{ConditionID: retainedScanCond, MinObservations: 3})
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
	if c.ConditionID != retainedScanCond {
		t.Errorf("condition_id = %q, want %q", c.ConditionID, retainedScanCond)
	}
	if c.CalibrationStatus != "eligible" {
		t.Errorf("calibration_status = %q, want eligible", c.CalibrationStatus)
	}
	if c.WinRate != 2.0/3.0 {
		t.Errorf("win_rate = %v, want %v", c.WinRate, 2.0/3.0)
	}
}

// TestHandleStockPickerScan_ExcludesDemotedFamiliesKeepsRetained is the core
// demotion test for this tool: the default cross-condition scan (no
// condition_id) returns exactly the two retained families and neither demoted
// one, even though the ledger holds rows for all four.
func TestHandleStockPickerScan_ExcludesDemotedFamiliesKeepsRetained(t *testing.T) {
	s, db := stockWinRateHarness(t)
	seedRetainedFamilies(t, db)

	out, err := callStockPickerScan(s, stockPickerScanInput{MinObservations: 3})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !out.Found {
		t.Fatalf("found=false, want true (message=%q)", out.Message)
	}

	got := map[string]bool{}
	for _, c := range out.Candidates {
		got[c.ConditionID] = true
	}
	for _, cond := range []string{retainedScanCond, retainedScanCond2} {
		if !got[cond] {
			t.Errorf("retained condition %q missing from the scan: %+v", cond, out.Candidates)
		}
	}
	for _, cond := range []string{"foreign-3d-net-buy", "momentum-20d-positive"} {
		if got[cond] {
			t.Errorf("demoted condition %q must not be ranked: %+v", cond, out.Candidates)
		}
	}
	if out.Total != 2 {
		t.Errorf("total = %d, want 2 (the two retained families)", out.Total)
	}
}

// TestHandleStockPickerScan_DemotedConditionRefused: naming a demoted family in
// condition_id does not unlock it — the tool answers found=false with the
// measurement that demoted it and points at the read-only surfaces.
func TestHandleStockPickerScan_DemotedConditionRefused(t *testing.T) {
	s, db := stockWinRateHarness(t)
	seedRetainedFamilies(t, db)

	for _, cond := range []string{"foreign-3d-net-buy", "momentum-20d-positive"} {
		out, err := callStockPickerScan(s, stockPickerScanInput{ConditionID: cond, MinObservations: 3})
		if err != nil {
			t.Fatalf("%s: handler: %v", cond, err)
		}
		if out.Found {
			t.Errorf("%s: found=true, want false (demoted)", cond)
		}
		if len(out.Candidates) != 0 {
			t.Errorf("%s: candidates = %+v, want none", cond, out.Candidates)
		}
		if !strings.Contains(out.Message, "demoted") {
			t.Errorf("%s: message = %q, want a demotion explanation", cond, out.Message)
		}
		if !strings.Contains(out.Message, "stock_get_win_rate") && !strings.Contains(out.Message, "stock_get_condition_winrate") {
			t.Errorf("%s: message = %q, want a pointer to the read-only surfaces", cond, out.Message)
		}
		if !strings.Contains(out.Message, "net-cost expectancy") {
			t.Errorf("%s: message = %q, want the demotion measurement", cond, out.Message)
		}
	}

	// Measurement is preserved: the same demoted rows stay readable through the
	// win-rate read path this tool shares a ledger with.
	winOut, err := callStockWinRate(s, stockWinRateInput{Symbol: "2330", ConditionID: "foreign-3d-net-buy"})
	if err != nil {
		t.Fatalf("win-rate read: %v", err)
	}
	if !winOut.Found || len(winOut.Conditions) != 1 {
		t.Fatalf("win-rate read = %+v, want the demoted family's row still readable", winOut)
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
// retained family's row sorted by wilson_lower.
func TestScanWinRateRows_Query(t *testing.T) {
	db, _ := openStockWinRateTestDB(t)
	seedRetainedFamilies(t, db)
	rows, err := scanWinRateRows(context.Background(), db, "120d", retainedScanCond, 3, 0.5, "wilson_lower", "buy")
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

// TestScanWinRateRows_ExcludesDemotedFamilies pins the exclusion at the query
// layer (not only in the handler): even an unfiltered scan never returns a
// demoted source, because a caller that reaches scanWinRateRows directly must
// not be able to rank one either.
func TestScanWinRateRows_ExcludesDemotedFamilies(t *testing.T) {
	db, demoted := openStockWinRateTestDB(t)
	seedRetainedFamilies(t, db)

	rows, err := scanWinRateRows(context.Background(), db, "120d", "", 3, 0.5, "wilson_lower", "buy")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 (the two retained families): %+v", len(rows), rows)
	}
	for _, r := range rows {
		for _, src := range demoted {
			if r.Source == src {
				t.Errorf("demoted source %q leaked into the scan: %+v", src, r)
			}
		}
		if r.Source != retainedScanSource && r.Source != retainedScanSource2 {
			t.Errorf("unexpected source %q in scan: %+v", r.Source, r)
		}
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
	// A normal buy-side row for contrast. It uses a RETAINED family: a demoted
	// source would be excluded outright by the scan query and the filter under
	// test would never be exercised.
	buy := stockpicker.StockWinRateSummary{
		Symbol:            "2330",
		Source:            retainedScanSource,
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
