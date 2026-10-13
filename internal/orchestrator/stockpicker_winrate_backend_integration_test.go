//go:build integration

package orchestrator

// PostgreSQL half of C6c: the stockpicker win-rate executor (agent
// stockpicker-winrate-01) must read PostgreSQL when the deployment declares it,
// because C7 stops writing the SQLite ledger (the mirror becomes opt-in and
// defaults to off). A reader that still answered from `data/state/atlas.db`
// would then gate recommendations on frozen numbers.
//
// These tests run against the real server (DATABASE_URL; skipped locally when
// unset, fatal in CI — internal/testdb policy) and pin three executor-level
// facts:
//
//  1. direction — with the declaration on postgres, the recommendation follows
//     the PostgreSQL row; flipping only the declaration to sqlite must flip the
//     outcome, so the direction claim cannot be satisfied by "always SQLite";
//  2. empty ≠ outage — a reachable PostgreSQL holding no row for the key must
//     NOT fall back to a SQLite row that would pass the gates;
//  3. fallback — a PostgreSQL that cannot answer the probe still leaves a
//     working read path (the SQLite ledger), so a database outage degrades
//     instead of removing the agent.
//
// The read-only contract of the PostgreSQL handle (default_transaction_read_only
// = on) is inherited from the C6b constructor the executor now reuses
// (stocktools.OpenWinRateSource → OpenReadOnlyWinRatePGDB) and is pinned there
// by TestWinRatePostgresReadHandleRejectsWrites; duplicating it here would mean
// two places to keep in step, without adding an executor-level fact.
//
// Isolation: every fixture uses a symbol that is not a Taiwan ticker and a
// rolling-window label no producer writes, and the rows are deleted before and
// after the test, so the shared database is left as found.

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

const (
	// c6cPGSymbol / c6cPGSymbol2 are not Taiwan tickers and never appear in a
	// producer's payload.
	c6cPGSymbol  = "C6CT"
	c6cPGSymbol2 = "C6C2"
	// c6cPGWindow / c6cPGWindow2 are rolling-window labels no producer writes
	// (the stockpicker daily update uses "Nd" labels such as 120d).
	c6cPGWindow  = "c6ctest"
	c6cPGWindow2 = "c6ctest2"
)

// TestStockpickerWinrateReadsPostgresWhenDeclared is the direction test: the
// two stores disagree, so the recommendation's reason names the store that
// answered. The control (same rows, declaration flipped to sqlite) must give the
// opposite verdict — that is the negative proof that this test measures the
// PostgreSQL read and not "SQLite by accident".
func TestStockpickerWinrateReadsPostgresWhenDeclared(t *testing.T) {
	ctx := context.Background()
	pg := c6cPGWritable(t)
	c6cCleanPGWinRate(t, pg, c6cPGSymbol)

	// PostgreSQL: a row that clears every eligibility gate (60 observations,
	// win_rate 0.70).
	pgRow := c6cPassingSummary()
	pgRow.Symbol, pgRow.Window = c6cPGSymbol, c6cPGWindow
	pgRow.Observations, pgRow.Hits, pgRow.WinRate, pgRow.WilsonLower = 60, 42, 0.70, 0.58
	if err := stockpicker.NewWinRateStore(pg).SaveWinRate(ctx, pgRow); err != nil {
		t.Fatalf("seed postgres win rate: %v", err)
	}

	// SQLite: the same key, below the win-rate gate (40 observations,
	// win_rate 0.30).
	sqlitePath := c6cSQLiteLedger(t, c6cFailingSummary())
	sqliteRow := c6cFailingSummary()
	sqliteRow.Symbol, sqliteRow.Window = c6cPGSymbol, c6cPGWindow
	c6cRewriteSQLiteRow(t, sqlitePath, sqliteRow)

	// Production wiring: PGURL/StoreBackend as cmd/atlas would carry them, no
	// OpenSource seam, so the executor builds stocktools.OpenWinRateSource and
	// its read-only PostgreSQL handle itself.
	pgExec := c6cBackendExecutor("postgres", testdb.URL(t), sqlitePath, c6cPGWindow)
	ok, reason := c6cRecommendFor(t, pgExec, c6cQuoteFor(c6cPGSymbol))
	if !ok {
		t.Fatalf("declared postgres: Recommend = false (reason %q); the executor did not read the postgres row", reason)
	}
	if !strings.Contains(reason, "observations=60") || !strings.Contains(reason, "win_rate=0.700") {
		t.Fatalf("reason = %q, want the postgres row (observations=60, win_rate=0.700)", reason)
	}

	// Control: identical rows, only the declaration changes. SQLite holds the
	// sub-threshold row, so the outcome must flip.
	sqliteExec := c6cBackendExecutor("sqlite", testdb.URL(t), sqlitePath, c6cPGWindow)
	if ok, reason := c6cRecommendFor(t, sqliteExec, c6cQuoteFor(c6cPGSymbol)); ok {
		t.Fatalf("declared sqlite: Recommend = true (reason %q); the control must read the SQLite row, otherwise the postgres case proves nothing", reason)
	}
}

// TestStockpickerWinrateEmptyPostgresDoesNotFallBackToSQLite pins the "empty is
// data, not an outage" rule against a real server: the key exists only in the
// SQLite ledger, and that row would pass every gate — yet the executor must
// answer not-found rather than read a second store.
func TestStockpickerWinrateEmptyPostgresDoesNotFallBackToSQLite(t *testing.T) {
	pg := c6cPGWritable(t)
	c6cCleanPGWinRate(t, pg, c6cPGSymbol2)

	sqliteRow := c6cPassingSummary()
	sqliteRow.Symbol, sqliteRow.Window = c6cPGSymbol2, c6cPGWindow2
	sqliteRow.Observations, sqliteRow.Hits, sqliteRow.WinRate, sqliteRow.WilsonLower = 45, 30, 0.66, 0.52
	sqlitePath := c6cSQLiteLedger(t, sqliteRow)
	// The SQLite row would pass: without this the "no recommendation" below
	// could come from the fixture instead of the no-fallback rule.
	c6cAssertStoredRowEligible(t, sqlitePath, c6cPGSymbol2, c6cPGWindow2)

	e := c6cBackendExecutor("postgres", testdb.URL(t), sqlitePath, c6cPGWindow2)
	if ok, reason := c6cRecommendFor(t, e, c6cQuoteFor(c6cPGSymbol2)); ok {
		t.Fatalf("Recommend = true (reason %q) with an empty but reachable postgres: the executor fell back to SQLite", reason)
	}
}

// TestStockpickerWinrateFallsBackToSQLiteWhenPostgresUnreachable pins the
// fallback against the real wiring: an unreachable PostgreSQL must not remove
// the read path — the executor answers from the SQLite ledger, with that
// ledger's numbers (45 observations, not the postgres stand-in's 60).
func TestStockpickerWinrateFallsBackToSQLiteWhenPostgresUnreachable(t *testing.T) {
	pg := c6cPGWritable(t)
	c6cCleanPGWinRate(t, pg, c6cPGSymbol)

	sqliteRow := c6cPassingSummary()
	sqliteRow.Symbol, sqliteRow.Window = c6cPGSymbol, c6cPGWindow
	sqliteRow.Observations, sqliteRow.Hits, sqliteRow.WinRate, sqliteRow.WilsonLower = 45, 30, 0.66, 0.52
	sqlitePath := c6cSQLiteLedger(t, sqliteRow)

	e := c6cBackendExecutor("postgres", c6cUnreachableDSN, sqlitePath, c6cPGWindow)
	ok, reason := c6cRecommendFor(t, e, c6cQuoteFor(c6cPGSymbol))
	if !ok {
		t.Fatal("Recommend = false with an unreachable postgres and a passing SQLite ledger; the fallback must keep the read path alive")
	}
	if !strings.Contains(reason, "observations=45") {
		t.Fatalf("reason = %q, want the SQLite row (observations=45)", reason)
	}
}

// ── helpers ───────────────────────────────────────────────────────────

// c6cQuoteFor builds a tradable quote for symbol (with the .TW suffix callers
// use), so the executor's read asks for exactly the symbol the fixture stored.
func c6cQuoteFor(symbol string) domain.Quote {
	q := stockpickerWinrateQuote()
	q.Symbol = symbol + ".TW"
	return q
}

// c6cBackendExecutor builds the executor through the production constructor (no
// OpenSource seam) with the flow gate satisfied, so the win-rate read is the
// only thing that decides the outcome.
func c6cBackendExecutor(storeBackend, pgURL, dbPath, window string) StockpickerWinrateExecutor {
	e := c6cExecutor(nil, storeBackend, pgURL, dbPath)
	e.Window = window
	return e
}

// c6cPGWritable returns a writable handle over the migrated database, after
// deleting this file's fixture rows (and again on cleanup).
func c6cPGWritable(t *testing.T) *sql.DB {
	t.Helper()
	db := stdlib.OpenDBFromPool(testdb.Pool(t, "../../sql/migrations"))
	t.Cleanup(func() { _ = db.Close() })
	for _, symbol := range []string{c6cPGSymbol, c6cPGSymbol2} {
		c6cCleanPGWinRate(t, db, symbol)
	}
	t.Cleanup(func() {
		for _, symbol := range []string{c6cPGSymbol, c6cPGSymbol2} {
			c6cCleanPGWinRate(t, db, symbol)
		}
	})
	return db
}

// c6cCleanPGWinRate deletes exactly the rows this file writes (keyed by the
// fixture symbols, which no producer uses).
func c6cCleanPGWinRate(t *testing.T, db *sql.DB, symbol string) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), `DELETE FROM stock_win_rate WHERE symbol = $1`, symbol); err != nil {
		t.Fatalf("clean stock_win_rate for %s: %v", symbol, err)
	}
}

// c6cRewriteSQLiteRow replaces the summary c6cSQLiteLedger wrote with row (same
// key, different numbers), so a fixture helper shared with the hermetic tests
// can serve keyed integration fixtures.
func c6cRewriteSQLiteRow(t *testing.T, path string, row stockpicker.StockWinRateSummary) {
	t.Helper()
	db := c6cOpenLedger(t, path)
	defer func() { _ = db.Close() }()
	if err := stockpicker.NewWinRateStore(db).SaveWinRate(context.Background(), row); err != nil {
		t.Fatalf("rewrite sqlite row: %v", err)
	}
}

// c6cAssertStoredRowEligible fails when the SQLite fixture row would not clear
// the executor's gates, which is what makes a "no recommendation" assertion
// meaningful.
func c6cAssertStoredRowEligible(t *testing.T, path, symbol, window string) {
	t.Helper()
	db := c6cOpenLedger(t, path)
	defer func() { _ = db.Close() }()
	summary, found, err := stockpicker.NewWinRateStore(db).LoadWinRate(context.Background(), symbol, testRetainedSource, window)
	if err != nil || !found {
		t.Fatalf("fixture row %s/%s: found=%v err=%v, want a stored row", symbol, window, found, err)
	}
	if summary.CalibrationStatus != stockpicker.CalibrationEligible ||
		summary.Observations < stockpickerWinrateMinObservations ||
		summary.WinRate < stockpickerWinrateMinWinRate ||
		summary.WilsonLower < stockpickerWinrateMinWilsonLower {
		t.Fatalf("fixture row %+v does not clear the executor's gates; the assertion would be vacuous", summary)
	}
}
