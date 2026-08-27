package stockpicker

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// newTestSQLiteDB 開啟一個 file-backed 的 SQLite（WAL + busy timeout + FK），
// 供本 package 的 store 測試共用。用檔案而非 :memory: 以避免 database/sql
// 連線池對 in-memory DB 的每個連線各自擁有一份獨立 schema 的陷阱。
func newTestSQLiteDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stockpicker_test.db")
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSignalOutcomeStore_RecordAndLoad(t *testing.T) {
	db := newTestSQLiteDB(t)
	store, err := NewSignalOutcomeStore(db)
	if err != nil {
		t.Fatalf("NewSignalOutcomeStore: %v", err)
	}
	ctx := context.Background()

	outcomes := []SignalOutcome{
		{Symbol: "2330", TriggerDate: "2026-01-02", Source: "stockpicker-momentum", ForwardReturn: 0.02, Hit: true},
		{Symbol: "2330", TriggerDate: "2026-01-05", Source: "stockpicker-momentum", ForwardReturn: -0.01, Hit: false},
	}
	if err := store.RecordOutcomes(ctx, outcomes); err != nil {
		t.Fatalf("RecordOutcomes: %v", err)
	}

	got, err := store.LoadOutcomes(ctx, "2330", "stockpicker-momentum", "")
	if err != nil {
		t.Fatalf("LoadOutcomes: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}

	first := got[0]
	if first.Symbol != "2330" || first.TriggerDate != "2026-01-02" || first.Source != "stockpicker-momentum" {
		t.Fatalf("first outcome = %+v, want symbol/date/source preserved", first)
	}
	if first.ForwardReturn != 0.02 || !first.Hit {
		t.Fatalf("first outcome fields = %+v, want forward_return=0.02 hit=true", first)
	}

	second := got[1]
	if second.TriggerDate != "2026-01-05" || second.ForwardReturn != -0.01 || second.Hit {
		t.Fatalf("second outcome fields = %+v, want date=2026-01-05 forward_return=-0.01 hit=false", second)
	}
}

func TestSignalOutcomeStore_DuplicateIdempotent(t *testing.T) {
	db := newTestSQLiteDB(t)
	store, err := NewSignalOutcomeStore(db)
	if err != nil {
		t.Fatalf("NewSignalOutcomeStore: %v", err)
	}
	ctx := context.Background()

	first := SignalOutcome{Symbol: "2330", TriggerDate: "2026-01-02", Source: "stockpicker-momentum", ForwardReturn: 0.02, Hit: true}
	second := SignalOutcome{Symbol: "2330", TriggerDate: "2026-01-02", Source: "stockpicker-momentum", ForwardReturn: 0.99, Hit: false}

	if err := store.RecordOutcomes(ctx, []SignalOutcome{first}); err != nil {
		t.Fatalf("first RecordOutcomes: %v", err)
	}
	// 同 key 再寫一次不得報錯，也不得產生第二列或覆寫第一列。
	if err := store.RecordOutcomes(ctx, []SignalOutcome{second}); err != nil {
		t.Fatalf("second RecordOutcomes (duplicate): %v", err)
	}

	got, err := store.LoadOutcomes(ctx, "2330", "stockpicker-momentum", "")
	if err != nil {
		t.Fatalf("LoadOutcomes: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (duplicate must be ignored)", len(got))
	}
	if got[0].ForwardReturn != 0.02 || !got[0].Hit {
		t.Fatalf("duplicate overwrote original: got %+v, want first write preserved", got[0])
	}
}

func TestSignalOutcomeStore_MultipleSources(t *testing.T) {
	db := newTestSQLiteDB(t)
	store, err := NewSignalOutcomeStore(db)
	if err != nil {
		t.Fatalf("NewSignalOutcomeStore: %v", err)
	}
	ctx := context.Background()

	outcomes := []SignalOutcome{
		{Symbol: "2330", TriggerDate: "2026-01-02", Source: "stockpicker-momentum", ForwardReturn: 0.02, Hit: true},
		{Symbol: "2330", TriggerDate: "2026-01-02", Source: "research-agent-1", ForwardReturn: 0.01, Hit: true},
	}
	if err := store.RecordOutcomes(ctx, outcomes); err != nil {
		t.Fatalf("RecordOutcomes: %v", err)
	}

	got, err := store.LoadOutcomes(ctx, "2330", "", "")
	if err != nil {
		t.Fatalf("LoadOutcomes (all sources): %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (same symbol/date, different sources)", len(got))
	}

	for _, source := range []string{"stockpicker-momentum", "research-agent-1"} {
		bySource, err := store.LoadOutcomes(ctx, "2330", source, "")
		if err != nil {
			t.Fatalf("LoadOutcomes(source=%s): %v", source, err)
		}
		if len(bySource) != 1 || bySource[0].Source != source {
			t.Fatalf("source %s: got %+v, want exactly one row with that source", source, bySource)
		}
	}
}

func TestSignalOutcomeStore_SourceColumnNotNull(t *testing.T) {
	db := newTestSQLiteDB(t)
	store, err := NewSignalOutcomeStore(db)
	if err != nil {
		t.Fatalf("NewSignalOutcomeStore: %v", err)
	}
	ctx := context.Background()

	err = store.RecordOutcomes(ctx, []SignalOutcome{
		{Symbol: "2330", TriggerDate: "2026-01-02", Source: "", ForwardReturn: 0.01},
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
	db := newTestSQLiteDB(t)
	store, err := NewSignalOutcomeStore(db)
	if err != nil {
		t.Fatalf("NewSignalOutcomeStore: %v", err)
	}
	ctx := context.Background()

	outcomes := []SignalOutcome{
		{Symbol: "2330", TriggerDate: "2000-01-01", Source: "s", ForwardReturn: 0.01},
		{Symbol: "2330", TriggerDate: "2099-01-01", Source: "s", ForwardReturn: 0.02},
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

func TestMigration_UpDownUp(t *testing.T) {
	db := newTestSQLiteDB(t)

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

	// up → both tables exist
	exec(up18, up19)
	assertTables(true)

	// down → both dropped
	exec(down19, down18)
	assertTables(false)

	// up again → both recreated（可重複執行）
	exec(up18, up19)
	assertTables(true)
}
