package ledger

import (
	"database/sql"
	"strings"
	"testing"
)

// legacyWinRateSchema is the EXACT pre-#2213 shape of stock_win_rate as
// measured on the production file (2026-10-12 PRAGMA table_info: 13 columns,
// no break_even_cost_rate). Reproducing the real legacy shape (instead of a
// convenient 2-column stub) is the point: it is what made the deployed binary
// answer HTTP 503.
const legacyWinRateSchema = `
CREATE TABLE stock_win_rate (
	symbol TEXT NOT NULL,
	source TEXT NOT NULL,
	rolling_window TEXT NOT NULL,
	observations INTEGER NOT NULL,
	hits INTEGER NOT NULL,
	win_rate REAL NOT NULL,
	wilson_lower REAL,
	wilson_upper REAL,
	confidence REAL,
	calibration_status TEXT NOT NULL,
	net_cost_rate REAL,
	avg_forward_return REAL,
	updated_at TEXT NOT NULL,
	UNIQUE(symbol, source, rolling_window)
);`

// TestAddStockWinRateBreakEvenColumn_HealsLegacyTable is the regression test
// for the 2026-10-12 production P0: PR #2213 added break_even_cost_rate to the
// code and to PG (sql/migrations/000028), but CREATE TABLE IF NOT EXISTS never
// adds a column to an existing SQLite file, so the deployed binary hit
// "no such column: break_even_cost_rate" (HTTP 503 on /api/stock/win_rate and
// the MCP win-rate tools). It fails without addStockWinRateBreakEvenColumn.
func TestAddStockWinRateBreakEvenColumn_HealsLegacyTable(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB: %v", err)
	}
	defer func() { _ = db.Close() }()

	// 1. Legacy file: old table + one row written by the old binary.
	if _, err := db.Exec(legacyWinRateSchema); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO stock_win_rate
		(symbol, source, rolling_window, observations, hits, win_rate,
		 calibration_status, updated_at)
		VALUES ('2330', 'price-volume-bottom-divergence', 'all', 40, 22, 0.55,
		        'eligible', '2026-10-08T15:16:33Z')`); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	// 2. Reproduce the P0: the reader's query must fail BEFORE the migration.
	if _, err := db.Exec(`SELECT break_even_cost_rate FROM stock_win_rate`); err == nil {
		t.Fatal("precondition broken: legacy table unexpectedly has break_even_cost_rate")
	} else if !strings.Contains(err.Error(), "no such column") {
		t.Fatalf("want a no-such-column error, got %v", err)
	}

	// 3. InitSchema must heal it (additive migration, idempotent).
	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema must be idempotent: %v", err)
	}

	// 4. The reader's query works again, and the legacy row is preserved with
	//    NULL (not silently dropped, not rewritten).
	var got sql.NullFloat64
	if err := db.QueryRow(`SELECT break_even_cost_rate FROM stock_win_rate
		WHERE symbol = '2330'`).Scan(&got); err != nil {
		t.Fatalf("post-migration read: %v", err)
	}
	if got.Valid {
		t.Fatalf("legacy row must keep NULL break_even_cost_rate, got %v", got.Float64)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM stock_win_rate`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("legacy row must survive: n=%d err=%v", n, err)
	}

	// 5. The regime table (Phase 1 strata) gets the column too.
	var regime sql.NullFloat64
	if err := db.QueryRow(`SELECT break_even_cost_rate FROM stock_win_rate_by_regime LIMIT 1`).Scan(&regime); err != nil && err != sql.ErrNoRows {
		t.Fatalf("stock_win_rate_by_regime must expose break_even_cost_rate: %v", err)
	}
}
