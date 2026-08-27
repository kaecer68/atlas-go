package stockpicker

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/ledger"
)

// openStockpickerTestDB opens an in-memory SQLite database and initializes
// the ledger schema (which now includes stock_signal_outcomes and stock_win_rate).
func openStockpickerTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := ledger.OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	return db
}

func TestSignalOutcomeStore_RecordAndLoad(t *testing.T) {
	db := openStockpickerTestDB(t)
	store := NewSignalOutcomeStore(db)
	ctx := context.Background()

	outcomes := []SignalOutcome{
		{
			Symbol:           "2330",
			TriggerDate:      "2026-01-02",
			ForwardReturn:    0.02,
			NetForwardReturn: 0.01415,
			Hit:              true,
			CostRate:         0.00585,
			Source:           "stockpicker-momentum",
			Regime:           "bull",
			CreatedAt:        "2026-01-02T18:00:00Z",
		},
		{
			Symbol:           "2330",
			TriggerDate:      "2026-01-05",
			ForwardReturn:    -0.01,
			NetForwardReturn: -0.01585,
			Hit:              false,
			CostRate:         0.00585,
			Source:           "stockpicker-momentum",
			CreatedAt:        "2026-01-05T18:00:00Z",
		},
	}

	if err := store.RecordOutcomes(ctx, outcomes); err != nil {
		t.Fatalf("RecordOutcomes: %v", err)
	}

	loaded, err := store.LoadOutcomes(ctx, "2330", "stockpicker-momentum", "")
	if err != nil {
		t.Fatalf("LoadOutcomes: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 outcomes, got %d", len(loaded))
	}

	first := loaded[0]
	if first.Symbol != "2330" || first.TriggerDate != "2026-01-02" || first.Source != "stockpicker-momentum" {
		t.Fatalf("key fields mismatch: %+v", first)
	}
	if first.ForwardReturn != 0.02 {
		t.Errorf("ForwardReturn = %v, want 0.02", first.ForwardReturn)
	}
	if first.NetForwardReturn != 0.01415 {
		t.Errorf("NetForwardReturn = %v, want 0.01415", first.NetForwardReturn)
	}
	if !first.Hit {
		t.Errorf("Hit = %v, want true", first.Hit)
	}
	if first.CostRate != 0.00585 {
		t.Errorf("CostRate = %v, want 0.00585", first.CostRate)
	}
	if first.Regime != "bull" {
		t.Errorf("Regime = %q, want bull", first.Regime)
	}
	if first.CreatedAt != "2026-01-02T18:00:00Z" {
		t.Errorf("CreatedAt = %q, want 2026-01-02T18:00:00Z", first.CreatedAt)
	}

	second := loaded[1]
	if second.TriggerDate != "2026-01-05" || second.ForwardReturn != -0.01 || second.Hit {
		t.Fatalf("second outcome fields = %+v, want date=2026-01-05 forward_return=-0.01 hit=false", second)
	}
}

func TestSignalOutcomeStore_DuplicateIdempotent(t *testing.T) {
	db := openStockpickerTestDB(t)
	store := NewSignalOutcomeStore(db)
	ctx := context.Background()

	first := SignalOutcome{
		Symbol:        "2330",
		TriggerDate:   "2026-01-02",
		ForwardReturn: 0.02,
		Hit:           true,
		Source:        "stockpicker-momentum",
		CreatedAt:     "2026-01-02T18:00:00Z",
	}
	second := SignalOutcome{
		Symbol:        "2330",
		TriggerDate:   "2026-01-02",
		ForwardReturn: 0.99,
		Hit:           false,
		Source:        "stockpicker-momentum",
		CreatedAt:     "2026-01-02T19:00:00Z",
	}

	if err := store.RecordOutcomes(ctx, []SignalOutcome{first}); err != nil {
		t.Fatalf("first RecordOutcomes: %v", err)
	}
	if err := store.RecordOutcomes(ctx, []SignalOutcome{second}); err != nil {
		t.Fatalf("second RecordOutcomes (duplicate): %v", err)
	}

	loaded, err := store.LoadOutcomes(ctx, "2330", "stockpicker-momentum", "")
	if err != nil {
		t.Fatalf("LoadOutcomes: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 outcome after duplicate insert, got %d", len(loaded))
	}
	if loaded[0].ForwardReturn != 0.02 || !loaded[0].Hit {
		t.Fatalf("duplicate overwrote original: got %+v, want first write preserved", loaded[0])
	}
}

func TestSignalOutcomeStore_MultipleSources(t *testing.T) {
	db := openStockpickerTestDB(t)
	store := NewSignalOutcomeStore(db)
	ctx := context.Background()

	outcomes := []SignalOutcome{
		{
			Symbol:        "2330",
			TriggerDate:   "2026-01-02",
			ForwardReturn: 0.02,
			Source:        "stockpicker-momentum",
			CreatedAt:     "2026-01-02T18:00:00Z",
		},
		{
			Symbol:        "2330",
			TriggerDate:   "2026-01-02",
			ForwardReturn: 0.015,
			Source:        "research-agent-1",
			CreatedAt:     "2026-01-02T18:00:00Z",
		},
	}

	if err := store.RecordOutcomes(ctx, outcomes); err != nil {
		t.Fatalf("RecordOutcomes: %v", err)
	}

	loaded, err := store.LoadOutcomes(ctx, "2330", "stockpicker-momentum", "")
	if err != nil {
		t.Fatalf("LoadOutcomes momentum: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 momentum outcome, got %d", len(loaded))
	}

	loaded, err = store.LoadOutcomes(ctx, "2330", "research-agent-1", "")
	if err != nil {
		t.Fatalf("LoadOutcomes research: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 research outcome, got %d", len(loaded))
	}
}

func TestSignalOutcomeStore_SourceColumnNotNull(t *testing.T) {
	db := openStockpickerTestDB(t)
	store := NewSignalOutcomeStore(db)
	ctx := context.Background()

	err := store.RecordOutcomes(ctx, []SignalOutcome{
		{
			Symbol:      "2330",
			TriggerDate: "2026-01-02",
			Source:      "",
			CreatedAt:   "2026-01-02T18:00:00Z",
		},
	})
	if err == nil {
		t.Fatal("RecordOutcomes with empty source: want error, got nil")
	}

	got, err := store.LoadOutcomes(ctx, "2330", "", "")
	if err != nil {
		t.Fatalf("LoadOutcomes: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 rows after rejected empty-source write, got %d", len(got))
	}
}

func TestSignalOutcomeStore_WindowFilter(t *testing.T) {
	db := openStockpickerTestDB(t)
	store := NewSignalOutcomeStore(db)
	ctx := context.Background()

	outcomes := []SignalOutcome{
		{Symbol: "2330", TriggerDate: "2000-01-01", Source: "s", ForwardReturn: 0.01, CreatedAt: "2000-01-01T18:00:00Z"},
		{Symbol: "2330", TriggerDate: "2099-01-01", Source: "s", ForwardReturn: 0.02, CreatedAt: "2099-01-01T18:00:00Z"},
	}
	if err := store.RecordOutcomes(ctx, outcomes); err != nil {
		t.Fatalf("RecordOutcomes: %v", err)
	}

	got, err := store.LoadOutcomes(ctx, "2330", "s", "120d")
	if err != nil {
		t.Fatalf("LoadOutcomes(window): %v", err)
	}
	if len(got) != 1 || got[0].TriggerDate != "2099-01-01" {
		t.Fatalf("window filter got %+v, want only the recent row", got)
	}

	if _, err := store.LoadOutcomes(ctx, "2330", "s", "bogus"); err == nil {
		t.Fatal("invalid window label: want error, got nil")
	}
}

func TestAggregateWinRate_SkipsConsistencyCheck(t *testing.T) {
	// PR 1a 驗收報告 §5-1: 跨 symbol / 跨 source 聚合時應跳過 SignalWinRate 的一致性檢查。
	outcomes := []SignalOutcome{
		{Symbol: "2330", Source: "stockpicker-momentum", ForwardReturn: 0.02},
		{Symbol: "2317", Source: "stockpicker-momentum", ForwardReturn: 0.015},
		{Symbol: "2330", Source: "research-agent-1", ForwardReturn: -0.01},
	}

	summary, err := aggregateWinRate(outcomes, 0.00585, 30, 0.95)
	if err != nil {
		t.Fatalf("aggregateWinRate: %v", err)
	}
	if summary.Observations != 3 {
		t.Fatalf("Observations = %d, want 3", summary.Observations)
	}
	if summary.Hits != 2 {
		t.Fatalf("Hits = %d, want 2", summary.Hits)
	}
	if summary.WinRate != 2.0/3.0 {
		t.Fatalf("WinRate = %v, want %v", summary.WinRate, 2.0/3.0)
	}
	if summary.Symbol != "" || summary.Source != "" {
		t.Fatalf("cross aggregate must not fill Symbol/Source, got %q/%q", summary.Symbol, summary.Source)
	}
}

func TestMigration_UpDownUp(t *testing.T) {
	db := openStockpickerTestDB(t)

	read := func(name string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("..", "..", "sql", "migrations", name))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		return string(b)
	}

	up18 := read("000018_stock_signal_outcomes.up.sql")
	down18 := read("000018_stock_signal_outcomes.down.sql")
	up19 := read("000019_stock_win_rate.up.sql")
	down19 := read("000019_stock_win_rate.down.sql")

	exec := func(stmts ...string) {
		t.Helper()
		for _, s := range stmts {
			if _, err := db.Exec(s); err != nil {
				t.Fatalf("exec migration: %v", err)
			}
		}
	}

	assertTables := func(present bool) {
		t.Helper()
		for _, table := range []string{"stock_signal_outcomes", "stock_win_rate"} {
			var n int
			if err := db.QueryRow(
				`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = ?`, table,
			).Scan(&n); err != nil {
				t.Fatalf("count table %s: %v", table, err)
			}
			if present && n != 1 {
				t.Fatalf("table %s should exist", table)
			}
			if !present && n != 0 {
				t.Fatalf("table %s should not exist", table)
			}
		}
	}

	// up -> both tables exist
	exec(up18, up19)
	assertTables(true)

	// down -> both dropped
	exec(down19, down18)
	assertTables(false)

	// up again -> both recreated
	exec(up18, up19)
	assertTables(true)
}
