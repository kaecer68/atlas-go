package server

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// openStockWinRateTestDB opens an in-memory SQLite ledger with the full
// schema and seeds one summary + outcome rows per condition for symbol 2330.
// It returns the handle plus the two source names seeded.
func openStockWinRateTestDB(t *testing.T) (*sql.DB, []string) {
	t.Helper()
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
	outStore := stockpicker.NewSignalOutcomeStore(db)

	sources := []string{"stockpicker-foreign-3d-net-buy", "stockpicker-momentum-20d-positive"}
	outcomes := []stockpicker.SignalOutcome{
		{Symbol: "2330", TriggerDate: "2026-03-02", ForwardReturn: 0.02, NetForwardReturn: 0.01415, Hit: true, CostRate: 0.00585, Source: sources[0]},
		{Symbol: "2330", TriggerDate: "2026-04-01", ForwardReturn: -0.01, NetForwardReturn: -0.01585, Hit: false, CostRate: 0.00585, Source: sources[0]},
		{Symbol: "2330", TriggerDate: "2026-05-04", ForwardReturn: 0.03, NetForwardReturn: 0.02415, Hit: true, CostRate: 0.00585, Source: sources[0]},
		{Symbol: "2330", TriggerDate: "2026-03-02", ForwardReturn: -0.02, NetForwardReturn: -0.02585, Hit: false, CostRate: 0.00585, Source: sources[1]},
		{Symbol: "2330", TriggerDate: "2026-06-01", ForwardReturn: 0.05, NetForwardReturn: 0.04415, Hit: true, CostRate: 0.00585, Source: sources[1]},
	}
	if err := outStore.RecordOutcomes(ctx, outcomes); err != nil {
		t.Fatalf("record outcomes: %v", err)
	}

	for i, src := range sources {
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
			AvgForwardReturn:  0.01 + float64(i)*0.01,
			UpdatedAt:         "2026-08-27T12:00:00Z",
		}
		if err := winStore.SaveWinRate(ctx, summary); err != nil {
			t.Fatalf("save win rate %s: %v", src, err)
		}
	}
	return db, sources
}

// stockWinRateHarness builds a server with winRateDB pre-wired so the
// handler reads the in-memory ledger without touching the filesystem.
func stockWinRateHarness(t *testing.T) (*server, *sql.DB) {
	t.Helper()
	s, _, done := newTestHarness(t)
	t.Cleanup(done)
	db, _ := openStockWinRateTestDB(t)
	s.winRateDB = db
	return s, db
}

func TestHandleStockGetWinRate_HasData(t *testing.T) {
	s, _ := stockWinRateHarness(t)
	out, err := callStockWinRate(s, stockWinRateInput{Symbol: "2330"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !out.Found {
		t.Fatalf("found=false, want true (message=%q)", out.Message)
	}
	if out.Symbol != "2330" || out.RollingWindow != "120d" {
		t.Fatalf("envelope = %+v, want symbol 2330 window 120d", out)
	}
	if len(out.Conditions) != 2 {
		t.Fatalf("conditions = %d, want 2: %+v", len(out.Conditions), out.Conditions)
	}
	byID := map[string]stockWinRateCondition{}
	for _, c := range out.Conditions {
		byID[c.ConditionID] = c
	}
	for _, id := range []string{"foreign-3d-net-buy", "momentum-20d-positive"} {
		c, ok := byID[id]
		if !ok {
			t.Fatalf("condition %q missing from output", id)
		}
		if c.Observations != 3 || c.Hits != 2 {
			t.Fatalf("%s: observations/hits = %d/%d, want 3/2", id, c.Observations, c.Hits)
		}
		if c.WinRate != 2.0/3.0 {
			t.Fatalf("%s: win_rate = %v, want %v", id, c.WinRate, 2.0/3.0)
		}
		if c.CalibrationStatus != "eligible" {
			t.Fatalf("%s: calibration_status = %q, want eligible", id, c.CalibrationStatus)
		}
		if c.DataStart != "2026-03-02" {
			t.Fatalf("%s: data_start = %q, want 2026-03-02", id, c.DataStart)
		}
		if c.DataEnd == "" {
			t.Fatalf("%s: data_end empty, want the latest stored trigger_date", id)
		}
	}
}

func TestHandleStockGetWinRate_SingleCondition(t *testing.T) {
	s, _ := stockWinRateHarness(t)
	out, err := callStockWinRate(s, stockWinRateInput{Symbol: "2330", ConditionID: "momentum-20d-positive"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !out.Found {
		t.Fatalf("found=false, want true")
	}
	if len(out.Conditions) != 1 {
		t.Fatalf("conditions = %d, want 1", len(out.Conditions))
	}
	if out.Conditions[0].ConditionID != "momentum-20d-positive" {
		t.Fatalf("condition = %q, want momentum-20d-positive", out.Conditions[0].ConditionID)
	}
	if out.Conditions[0].Source != "stockpicker-momentum-20d-positive" {
		t.Fatalf("source = %q, want stockpicker-momentum-20d-positive", out.Conditions[0].Source)
	}
}

func TestHandleStockGetWinRate_UnknownConditionIsNoData(t *testing.T) {
	s, _ := stockWinRateHarness(t)
	out, err := callStockWinRate(s, stockWinRateInput{Symbol: "2330", ConditionID: "does-not-exist"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Found {
		t.Fatal("found=true for unknown condition, want false")
	}
	if !strings.Contains(out.Message, "no stockpicker win-rate data") {
		t.Fatalf("message = %q, want a clear no-data message", out.Message)
	}
	if len(out.Conditions) != 0 {
		t.Fatalf("conditions = %d, want 0", len(out.Conditions))
	}
}

func TestHandleStockGetWinRate_NoData(t *testing.T) {
	s, _ := stockWinRateHarness(t)
	out, err := callStockWinRate(s, stockWinRateInput{Symbol: "9999"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Found {
		t.Fatal("found=true for symbol without data, want false")
	}
	if out.Message == "" {
		t.Fatal("expected a clear no-data message")
	}
	if !strings.Contains(out.Message, "9999") {
		t.Fatalf("message = %q, want the symbol mentioned", out.Message)
	}
	if len(out.Conditions) != 0 {
		t.Fatalf("conditions = %d, want 0", len(out.Conditions))
	}
}

func TestHandleStockGetWinRate_ParameterDefaults(t *testing.T) {
	s, _ := stockWinRateHarness(t)
	// Only symbol supplied: rolling_window defaults to 120d and all
	// conditions are returned (參數缺省 path).
	out, err := callStockWinRate(s, stockWinRateInput{Symbol: "2330"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !out.Found {
		t.Fatalf("found=false, want true")
	}
	if out.RollingWindow != "120d" {
		t.Fatalf("rolling_window = %q, want default 120d", out.RollingWindow)
	}
	if len(out.Conditions) != 2 {
		t.Fatalf("conditions = %d, want 2 (all conditions for symbol)", len(out.Conditions))
	}
	// Explicit rolling_window with no stored row → clear no-data message.
	out, err = callStockWinRate(s, stockWinRateInput{Symbol: "2330", RollingWindow: "60d"})
	if err != nil {
		t.Fatalf("handler(60d): %v", err)
	}
	if out.Found {
		t.Fatal("found=true for 60d window with no stored row, want false")
	}
	if out.RollingWindow != "60d" {
		t.Fatalf("rolling_window = %q, want 60d echoed", out.RollingWindow)
	}
}

func TestHandleStockGetWinRate_MissingSymbol(t *testing.T) {
	s, _ := stockWinRateHarness(t)
	_, _, err := s.handleStockGetWinRate(context.Background(), nil, stockWinRateInput{})
	if err == nil {
		t.Fatal("expected error for missing symbol")
	}
	if !strings.Contains(err.Error(), "symbol is required") {
		t.Fatalf("error = %q, want symbol is required", err.Error())
	}
}

func TestHandleStockGetWinRate_DBUnconfigured(t *testing.T) {
	s, _, done := newTestHarness(t)
	defer done()
	// winRateDB nil and StockpickerDBPath empty → handler must answer a
	// clear no-data message, not a hard error.
	out, err := callStockWinRate(s, stockWinRateInput{Symbol: "2330"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Found {
		t.Fatal("found=true with no DB configured, want false")
	}
	if !strings.Contains(out.Message, "no stockpicker win-rate data") {
		t.Fatalf("message = %q, want clear no-data message", out.Message)
	}
	if !strings.Contains(out.Message, "ATLAS_MCP_STOCKPICKER_DB") {
		t.Fatalf("message = %q, want the config hint", out.Message)
	}
}

// callStockWinRate is a tiny wrapper reducing boilerplate in the tests above.
func callStockWinRate(s *server, in stockWinRateInput) (stockWinRateOutput, error) {
	_, out, err := s.handleStockGetWinRate(context.Background(), nil, in)
	return out, err
}
