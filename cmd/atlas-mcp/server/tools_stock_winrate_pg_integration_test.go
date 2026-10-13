//go:build integration

package server

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"

	"github.com/kaecer68/atlas-go/internal/stockpicker"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

// This file pins the PostgreSQL half of C6 against a real PostgreSQL
// (DATABASE_URL; skipped locally when unset, fatal in CI — internal/testdb
// policy):
//
//  1. parity   — the same rows in both backends produce byte-identical tool
//                output (the tool output is the contract, so the comparison is
//                made on the marshalled result, not on selected fields);
//  2. negative — when the two backends are deliberately made to disagree, the
//                output follows PostgreSQL and the parity assertion would be
//                red. Without this the parity test could pass while the tool
//                still reads SQLite;
//  3. read-only — the handle production wires rejects writes and still reads.
//
// Isolation: stock_picker_scan has no symbol filter, so the fixture uses a
// rolling-window label no other producer writes (pgTestWindow) and symbols that
// are not Taiwan tickers (pgTestSymbol*). Cleanup deletes by those keys before
// and after the test, so the shared database is left as found.

const (
	pgTestWindow  = "c6pgtest"
	pgTestSymbol  = "C6TST"
	pgTestSymbol2 = "C6TS2"
	pgTestCondTop = "price-volume-top-divergence"
)

// TestStockWinRateToolsPostgresSQLiteParity asserts row-for-row output parity.
func TestStockWinRateToolsPostgresSQLiteParity(t *testing.T) {
	_, pgHandle, pgWritable := pgWinRateFixture(t)
	sqliteDB := openWinRateLedger(t)
	seedBothBackends(t, pgWritable, sqliteDB)

	pgServer := pgWiredServer(t, pgHandle)
	sqliteServer := sqliteWiredServer(t, sqliteDB)

	for _, tc := range parityCases() {
		t.Run(tc.name, func(t *testing.T) {
			pgOut := tc.run(t, pgServer)
			sqliteOut := tc.run(t, sqliteServer)
			if pgOut != sqliteOut {
				t.Fatalf("postgres and sqlite outputs differ\npostgres: %s\nsqlite:   %s", pgOut, sqliteOut)
			}
			if tc.wantNonEmpty && !strings.Contains(pgOut, `"found":true`) {
				t.Fatalf("output is empty; the parity comparison would be vacuous: %s", pgOut)
			}
		})
	}

	// The PostgreSQL server must have resolved the PostgreSQL handle; a silent
	// fallback to SQLite would make every case above pass while comparing
	// SQLite against SQLite. (Resolution happens on the first tool call, so
	// this check runs after the loop.)
	pgServer.winRateMu.Lock()
	resolved := pgServer.winRateResolved
	pgServer.winRateMu.Unlock()
	if resolved == nil || resolved.backend != WinRateBackendPostgres {
		t.Fatalf("postgres server resolved %+v, want the postgres handle", resolved)
	}
}

// TestStockWinRateToolsFollowPostgresWhenBackendsDiverge is the negative proof
// for the parity test: the same fixture is made to disagree in PostgreSQL only.
// The parity assertion must now be false, and the tool must report the
// PostgreSQL numbers — i.e. the tool really reads PostgreSQL, it does not fall
// back to a stale SQLite ledger when PostgreSQL is present.
func TestStockWinRateToolsFollowPostgresWhenBackendsDiverge(t *testing.T) {
	_, pgHandle, pgWritable := pgWinRateFixture(t)
	sqliteDB := openWinRateLedger(t)
	seedBothBackends(t, pgWritable, sqliteDB)

	pgServer := pgWiredServer(t, pgHandle)
	sqliteServer := sqliteWiredServer(t, sqliteDB)
	in := stockWinRateInput{Symbol: pgTestSymbol, RollingWindow: pgTestWindow}

	before, err := callStockWinRate(pgServer, in)
	if err != nil {
		t.Fatalf("stock_get_win_rate (postgres): %v", err)
	}
	beforeSQLite, err := callStockWinRate(sqliteServer, in)
	if err != nil {
		t.Fatalf("stock_get_win_rate (sqlite): %v", err)
	}
	if jsonOf(t, before) != jsonOf(t, beforeSQLite) {
		t.Fatalf("fixture is not identical before divergence\npostgres: %s\nsqlite:   %s", jsonOf(t, before), jsonOf(t, beforeSQLite))
	}

	// Diverge PostgreSQL only.
	seedWinRateRow(t, pgWritable, winRateSeed{
		Symbol: pgTestSymbol, Source: testSourceBottom, Window: pgTestWindow,
		Observations: 9, Hits: 7, WinRate: 7.0 / 9.0, WilsonLower: 0.44, WilsonUpper: 0.93,
		AvgForwardReturn: 0.009, BreakEvenCostRate: 0.021,
	})

	after, err := callStockWinRate(pgServer, in)
	if err != nil {
		t.Fatalf("stock_get_win_rate (postgres, diverged): %v", err)
	}
	afterSQLite, err := callStockWinRate(sqliteServer, in)
	if err != nil {
		t.Fatalf("stock_get_win_rate (sqlite, diverged): %v", err)
	}
	if jsonOf(t, after) == jsonOf(t, afterSQLite) {
		t.Fatal("outputs still match after PostgreSQL was diverged; the parity test above proves nothing about which backend is read")
	}

	got := conditionByID(t, after, testCondBottom)
	if got.Observations != 9 || got.Hits != 7 {
		t.Fatalf("postgres output observations/hits = %d/%d, want 9/7 (the diverged PostgreSQL row)", got.Observations, got.Hits)
	}
	keep := conditionByID(t, afterSQLite, testCondBottom)
	if keep.Observations != 40 || keep.Hits != 30 {
		t.Fatalf("sqlite output observations/hits = %d/%d, want the original 40/30", keep.Observations, keep.Hits)
	}
}

// TestStockWinRateReadHandleRejectsWrites pins the no-write contract on the
// PostgreSQL side: production's handle is a read-only session, so a write fails
// in the server (SQLSTATE 25006) instead of depending on the queries staying
// SELECTs. The writable control on the same database keeps the assertion from
// passing just because the connection is broken.
func TestStockWinRateReadHandleRejectsWrites(t *testing.T) {
	_, pgHandle, pgWritable := pgWinRateFixture(t)
	ctx := context.Background()

	if _, err := pgWritable.ExecContext(ctx, `DELETE FROM stock_win_rate WHERE rolling_window = $1`, pgTestWindow); err != nil {
		t.Fatalf("writable control handle could not write: %v", err)
	}

	var mode string
	if err := pgHandle.QueryRowContext(ctx, `SELECT current_setting('default_transaction_read_only')`).Scan(&mode); err != nil {
		t.Fatalf("read-only handle cannot read: %v", err)
	}
	if mode != "on" {
		t.Fatalf("default_transaction_read_only = %q on the read handle, want \"on\"", mode)
	}

	if _, err := pgHandle.ExecContext(ctx, `DELETE FROM stock_win_rate WHERE rolling_window = $1`, pgTestWindow); err == nil {
		t.Fatal("DELETE succeeded on the win-rate read handle; the no-write contract is not structural")
	} else if !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("write failed with %v, want a read-only transaction error", err)
	}

	var count int
	if err := pgHandle.QueryRowContext(ctx, `SELECT count(*) FROM stock_win_rate WHERE rolling_window = $1`, pgTestWindow).Scan(&count); err != nil {
		t.Fatalf("read-only handle cannot read: %v", err)
	}
}

// --- fixture -----------------------------------------------------------------

// parityCase is one tool invocation compared across the two backends.
type parityCase struct {
	name         string
	wantNonEmpty bool
	run          func(t *testing.T, s *server) string
}

// parityCases covers both tools, every sort mode, the avoid direction, the
// default-condition scan, truncation, and the no-data paths.
func parityCases() []parityCase {
	window := pgTestWindow
	scan := func(in stockPickerScanInput) func(*testing.T, *server) string {
		return func(t *testing.T, s *server) string {
			out, err := callStockPickerScan(s, in)
			if err != nil {
				t.Fatalf("stock_picker_scan(%+v): %v", in, err)
			}
			return jsonOf(t, out)
		}
	}
	get := func(in stockWinRateInput) func(*testing.T, *server) string {
		return func(t *testing.T, s *server) string {
			out, err := callStockWinRate(s, in)
			if err != nil {
				t.Fatalf("stock_get_win_rate(%+v): %v", in, err)
			}
			return jsonOf(t, out)
		}
	}
	return []parityCase{
		{name: "get_win_rate_all_conditions", wantNonEmpty: true, run: get(stockWinRateInput{Symbol: pgTestSymbol, RollingWindow: window})},
		{name: "get_win_rate_one_condition", wantNonEmpty: true, run: get(stockWinRateInput{Symbol: pgTestSymbol, RollingWindow: window, ConditionID: testCondBottom})},
		{name: "get_win_rate_no_such_window", run: get(stockWinRateInput{Symbol: pgTestSymbol, RollingWindow: window + "-missing"})},
		{name: "get_win_rate_no_such_symbol", run: get(stockWinRateInput{Symbol: "C6NONE", RollingWindow: window})},
		{name: "scan_default", wantNonEmpty: true, run: scan(stockPickerScanInput{RollingWindow: window, MinObservations: 3})},
		{name: "scan_one_condition", wantNonEmpty: true, run: scan(stockPickerScanInput{RollingWindow: window, ConditionID: testCondBottom, MinObservations: 3})},
		{name: "scan_shrunk_rate", wantNonEmpty: true, run: scan(stockPickerScanInput{RollingWindow: window, MinObservations: 3, SortBy: "shrunk_rate"})},
		{name: "scan_truncated", wantNonEmpty: true, run: scan(stockPickerScanInput{RollingWindow: window, MinObservations: 3, SortBy: "win_rate", TopN: 1})},
		{name: "scan_avoid", wantNonEmpty: true, run: scan(stockPickerScanInput{RollingWindow: window, ConditionID: pgTestCondTop, MinObservations: 3, Direction: "avoid"})},
		{name: "scan_avoid_by_win_rate", wantNonEmpty: true, run: scan(stockPickerScanInput{RollingWindow: window, ConditionID: pgTestCondTop, MinObservations: 3, Direction: "avoid", SortBy: "win_rate"})},
		{name: "scan_min_observations_too_high", run: scan(stockPickerScanInput{RollingWindow: window, MinObservations: 999})},
	}
}

// pgWinRateFixture returns the read-only handle production uses (built by the
// same OpenReadOnlyWinRatePGDB main calls) plus a writable handle over the same
// migrated database for seeding. Both the fixture rows and the outcome rows are
// removed before and after the test.
func pgWinRateFixture(t *testing.T) (dsn string, readOnly, writable *sql.DB) {
	t.Helper()
	dsn = testdb.URL(t)
	writable = stdlib.OpenDBFromPool(testdb.Pool(t, "../../../sql/migrations"))
	t.Cleanup(func() { _ = writable.Close() })
	deleteWinRateFixture(t, writable)
	t.Cleanup(func() { deleteWinRateFixture(t, writable) })

	readOnly, err := OpenReadOnlyWinRatePGDB(dsn)
	if err != nil {
		t.Fatalf("OpenReadOnlyWinRatePGDB: %v", err)
	}
	t.Cleanup(func() { _ = readOnly.Close() })
	return dsn, readOnly, writable
}

func deleteWinRateFixture(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `DELETE FROM stock_win_rate WHERE rolling_window = $1`, pgTestWindow); err != nil {
		t.Fatalf("clean stock_win_rate: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`DELETE FROM stock_signal_outcomes WHERE symbol IN ($1, $2) AND source LIKE 'stockpicker-%'`,
		pgTestSymbol, pgTestSymbol2); err != nil {
		t.Fatalf("clean stock_signal_outcomes: %v", err)
	}
}

// seedBothBackends writes the identical fixture into both backends: three
// eligible summaries over two symbols and the two retained condition families,
// plus the outcome rows that produce data_start/data_end.
func seedBothBackends(t *testing.T, pgDB, sqliteDB *sql.DB) {
	t.Helper()
	seeds := []winRateSeed{
		{Symbol: pgTestSymbol, Source: testSourceBottom, Window: pgTestWindow, Observations: 40, Hits: 30, WinRate: 0.75, WilsonLower: 0.60, WilsonUpper: 0.86, AvgForwardReturn: 0.012, BreakEvenCostRate: 0.020},
		{Symbol: pgTestSymbol, Source: testSourceTop, Window: pgTestWindow, Observations: 30, Hits: 9, WinRate: 0.30, WilsonLower: 0.17, WilsonUpper: 0.47, AvgForwardReturn: -0.008, BreakEvenCostRate: 0.010},
		{Symbol: pgTestSymbol2, Source: testSourceBottom, Window: pgTestWindow, Observations: 25, Hits: 16, WinRate: 0.64, WilsonLower: 0.44, WilsonUpper: 0.80, AvgForwardReturn: 0.006, BreakEvenCostRate: 0.015},
	}
	outcomes := []stockpicker.SignalOutcome{
		{Symbol: pgTestSymbol, TriggerDate: "2026-03-02", ForwardReturn: 0.02, NetForwardReturn: 0.01415, Hit: true, CostRate: 0.00585, Source: testSourceBottom},
		{Symbol: pgTestSymbol, TriggerDate: "2026-05-04", ForwardReturn: 0.03, NetForwardReturn: 0.02415, Hit: true, CostRate: 0.00585, Source: testSourceBottom},
		{Symbol: pgTestSymbol, TriggerDate: "2026-04-01", ForwardReturn: -0.02, NetForwardReturn: -0.02585, Hit: false, CostRate: 0.00585, Source: testSourceTop},
	}
	for _, db := range []*sql.DB{pgDB, sqliteDB} {
		for _, seed := range seeds {
			seedWinRateRow(t, db, seed)
		}
		if err := stockpicker.NewSignalOutcomeStore(db).RecordOutcomes(context.Background(), outcomes); err != nil {
			t.Fatalf("record outcomes: %v", err)
		}
	}
}

func pgWiredServer(t *testing.T, pgHandle *sql.DB) *server {
	t.Helper()
	s := newWinRateReadServer(t)
	s.cfg.WinRatePGDB = pgHandle
	return s
}

func sqliteWiredServer(t *testing.T, sqliteDB *sql.DB) *server {
	t.Helper()
	s := newWinRateReadServer(t)
	s.winRateSQLiteDB = sqliteDB
	return s
}

func conditionByID(t *testing.T, out stockWinRateOutput, conditionID string) stockWinRateCondition {
	t.Helper()
	for _, c := range out.Conditions {
		if c.ConditionID == conditionID {
			return c
		}
	}
	t.Fatalf("condition %q missing from %+v", conditionID, out.Conditions)
	return stockWinRateCondition{}
}
