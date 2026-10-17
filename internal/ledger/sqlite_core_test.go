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

// legacyParityTableSchema is the EXACT pre-000030 (2026-10-16) SQLite shape of
// the three tables that had drifted from their PostgreSQL counterparts, as
// produced by the SQLite CREATE TABLE IF NOT EXISTS blocks of that revision.
// The production 2026-10-12 file matches this shape: SQLite never had the PG
// columns (operator/criterion/time/…), and PG never had the legacy columns
// (action/reason/timestamp/…).
const legacyParityTableSchema = `
CREATE TABLE human_interventions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id TEXT NOT NULL,
	symbol TEXT NOT NULL,
	agent_id TEXT,
	action TEXT,
	timestamp TEXT
);
CREATE TABLE screening_rejects (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id TEXT NOT NULL,
	symbol TEXT NOT NULL,
	reason TEXT,
	timestamp TEXT,
	factor_scores_json TEXT
);
CREATE TABLE session_summaries (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id TEXT UNIQUE NOT NULL,
	total_recs INTEGER,
	passed_guards INTEGER,
	rejected INTEGER,
	timestamp TEXT
);`

// TestAddSchemaParityColumns_HealsLegacyTables is the regression test for
// FU-20261012-07/-09: the SQLite half of the schema reconciliation must reach
// databases that already exist. CREATE TABLE IF NOT EXISTS never alters an
// existing table, so without addSchemaParityColumns every column that 000030
// adds on the PostgreSQL side would be missing from the deployed atlas.db —
// exactly the 2026-10-12 break_even_cost_rate failure mode.
func TestAddSchemaParityColumns_HealsLegacyTables(t *testing.T) {
	db, err := OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("OpenSQLiteDB: %v", err)
	}
	defer func() { _ = db.Close() }()

	// 1. Legacy file: the pre-000030 tables plus one row per table.
	if _, err := db.Exec(legacyParityTableSchema); err != nil {
		t.Fatalf("create legacy tables: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO human_interventions (session_id, symbol, agent_id, action, timestamp)
		VALUES ('2026-10-11', '2330', 'agent-x', 'veto', '2026-10-11T09:05:00Z')`); err != nil {
		t.Fatalf("insert legacy human_interventions row: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO screening_rejects (session_id, symbol, reason, timestamp, factor_scores_json)
		VALUES ('2026-10-11', '2330', 'low volume', '2026-10-11T09:05:00Z', '{}')`); err != nil {
		t.Fatalf("insert legacy screening_rejects row: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO session_summaries (session_id, total_recs, passed_guards, rejected, timestamp)
		VALUES ('2026-10-11', 12, 9, 3, '2026-10-11T13:30:00Z')`); err != nil {
		t.Fatalf("insert legacy session_summaries row: %v", err)
	}

	// 2. Reproduce the drift: the PostgreSQL column names are still missing.
	for _, q := range []string{
		`SELECT operator FROM human_interventions`,
		`SELECT criterion FROM screening_rejects`,
		`SELECT risk_commentary FROM session_summaries`,
	} {
		if _, err := db.Exec(q); err == nil {
			t.Fatalf("precondition broken: legacy table already answers %q", q)
		} else if !strings.Contains(err.Error(), "no such column") {
			t.Fatalf("want a no-such-column error for %q, got %v", q, err)
		}
	}

	// 3. InitSchema heals it, and is idempotent.
	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	if err := InitSchema(db); err != nil {
		t.Fatalf("InitSchema must be idempotent: %v", err)
	}

	// 4. Every PostgreSQL-side column now answers on the legacy tables, and the
	//    legacy rows survive with their original values (nothing is rewritten).
	for _, q := range []string{
		`SELECT operator, intervention_id, type, target_symbol, value, reason FROM human_interventions`,
		`SELECT criterion, agent_id, skill, threshold, actual_value, factor_scores FROM screening_rejects`,
		`SELECT regime, order_count, broker_runtime, approval_id, tax_snapshots, after_tax_pnl, risk_commentary FROM session_summaries`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("after InitSchema %q must answer: %v", q, err)
		}
	}
	var action string
	if err := db.QueryRow(`SELECT action FROM human_interventions`).Scan(&action); err != nil || action != "veto" {
		t.Fatalf("legacy human_interventions row must survive: action=%q err=%v", action, err)
	}
	var reason string
	if err := db.QueryRow(`SELECT reason FROM screening_rejects`).Scan(&reason); err != nil || reason != "low volume" {
		t.Fatalf("legacy screening_rejects row must survive: reason=%q err=%v", reason, err)
	}
	var totalRecs int
	if err := db.QueryRow(`SELECT total_recs FROM session_summaries`).Scan(&totalRecs); err != nil || totalRecs != 12 {
		t.Fatalf("legacy session_summaries row must survive: total_recs=%d err=%v", totalRecs, err)
	}
}
