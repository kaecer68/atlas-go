package main

// Stockpicker SSOT backfill: copy stock_signal_outcomes + stock_win_rate from
// the job-local SQLite artifact (data/state/atlas.db) into PostgreSQL
// (migrations 000018/000019). See the design doc
// "atlas SSOT → PG 收斂計畫" batch A (2026-09-30, atlas-notes).
//
// Why this exists: the stockpicker writer (stockpicker.RunDailyUpdate →
// openOutcomeDB) ignores ATLAS_STORE_BACKEND and always lands outcomes in the
// job-local SQLite file, so production PG carries 0 rows while SQLite holds
// the full history (73,401 outcomes / 3,051 win-rate rows at 2026-09-30).
//
// Contract:
//   - Idempotent: INSERT ... ON CONFLICT DO NOTHING on the natural unique
//     keys; re-running inserts 0 (asserted by the integration test's
//     double-run case).
//   - Chunked: explicit PG transaction per 5,000 source rows.
//   - Read-only source: os.Stat + mode=ro open (traps.md: never use the
//     opens-or-creates ledger.OpenSQLiteDB for a read-only source — a
//     mistyped path must fail, not silently migrate an empty fresh DB).
//   - dryRun: counts + source/target digests only, zero target writes.

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const stockpickerChunkSize = 5000

// spExec is satisfied by *pgxpool.Pool and pgx.Tx.
type spExec interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// stockpickerReport summarizes one backfill run (also printed in dry-run).
type stockpickerReport struct {
	TargetHost       string
	TargetPort       int
	TargetDB         string
	OutcomesTotal    int64 // source rows scanned
	WinRatesTotal    int64
	OutcomesInserted int64
	WinRatesInserted int64
	PGOutcomesBefore int64
	PGOutcomesAfter  int64
	PGWinRatesBefore int64
	PGWinRatesAfter  int64
	DigestMismatches []string
}

// requireStockpickerTargetAssertion enforces the fail-closed rule for the
// stockpicker backfill (independent-review P1): it writes production data, so
// it refuses to run without an explicit -expect-db target assertion. A
// forgotten flag must abort the run — never backfill whatever DATABASE_URL
// happens to point at.
func requireStockpickerTargetAssertion(stockpicker bool, expectDB string) error {
	if stockpicker && expectDB == "" {
		return fmt.Errorf("-stockpicker requires -expect-db=<name> (refusing to write without a target assertion)")
	}
	return nil
}

// guardMigrateTarget reports the postgres connection target and, when expectDB
// is set, aborts on mismatch before db.Init can apply any migration, so a
// stray DATABASE_URL (e.g. source .env pointing at atlas_dev) can never
// migrate the wrong database. Mirrors the stockpicker M12 guard
// (internal/stockpicker/daily_update.go guardDatabaseTarget). Read-only: no
// migrations, no schema writes. Identity is printed host/db only — never the
// DSN (it embeds credentials).
func guardMigrateTarget(ctx context.Context, dsn, expectDB string) error {
	if expectDB == "" {
		return nil
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect for db target check: %w", err)
	}
	defer pool.Close()
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		return fmt.Errorf("ping for db target check: %w", err)
	}
	var actual, host string
	var port int
	if err := pool.QueryRow(pingCtx,
		`SELECT current_database(),
		        COALESCE(inet_server_addr()::text, 'local'),
		        COALESCE(inet_server_port(), 0)`,
	).Scan(&actual, &host, &port); err != nil {
		return fmt.Errorf("read migration target identity: %w", err)
	}
	log.Printf("migration target: host=%s port=%d db=%s", host, port, actual)
	if actual != expectDB {
		return fmt.Errorf("migration target mismatch: connected db=%q but -expect-db=%q; refusing to migrate (M12 guard)", actual, expectDB)
	}
	return nil
}

// openStockpickerSource opens the SQLite backfill source read-only, failing
// loudly when the file or the stockpicker tables are missing (instead of
// recreating an empty DB and silently migrating zero rows).
func openStockpickerSource(path string) (*sql.DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("stockpicker source path %s: %w", path, err)
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("stockpicker source sqlite %s: %w", abs, err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(abs)+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open stockpicker source %s: %w", abs, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open stockpicker source %s: %w", abs, err)
	}
	for _, table := range []string{"stock_signal_outcomes", "stock_win_rate"} {
		var name string
		if err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table,
		).Scan(&name); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("stockpicker source %s missing table %s: %w", abs, table, err)
		}
	}
	return db, nil
}

// spDigestRow is one integer-exact digest bucket: row count plus two integer
// sums, chosen so both engines agree bit-for-bit (no float sums).
type spDigestRow struct {
	n  int64
	s1 int64
	s2 int64
}

// digestKey identifies one digest bucket ("source|month" or "source|window").
type digestKey struct {
	a, b string
}

// digestScanner is satisfied by *sql.Rows and pgx.Rows.
type digestScanner interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanDigest(rows digestScanner) (map[digestKey]spDigestRow, error) {
	out := make(map[digestKey]spDigestRow)
	for rows.Next() {
		var k digestKey
		var d spDigestRow
		if err := rows.Scan(&k.a, &k.b, &d.n, &d.s1, &d.s2); err != nil {
			return nil, fmt.Errorf("scan digest row: %w", err)
		}
		out[k] = d
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// compareDigests diffs two digest maps; returned strings name every bucket
// missing from / extra to / disagreeing with want (pure, unit-testable).
func compareDigests(want, got map[digestKey]spDigestRow) []string {
	var out []string
	keys := make(map[digestKey]struct{}, len(want)+len(got))
	for k := range want {
		keys[k] = struct{}{}
	}
	for k := range got {
		keys[k] = struct{}{}
	}
	sorted := make([]digestKey, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].a != sorted[j].a {
			return sorted[i].a < sorted[j].a
		}
		return sorted[i].b < sorted[j].b
	})
	for _, k := range sorted {
		w, wok := want[k]
		g, gok := got[k]
		switch {
		case !gok:
			out = append(out, fmt.Sprintf("bucket %s|%s missing in target (source n=%d)", k.a, k.b, w.n))
		case !wok:
			out = append(out, fmt.Sprintf("bucket %s|%s extra in target (n=%d)", k.a, k.b, g.n))
		case w != g:
			out = append(out, fmt.Sprintf("bucket %s|%s differs: source(n=%d,%d,%d) target(n=%d,%d,%d)",
				k.a, k.b, w.n, w.s1, w.s2, g.n, g.s1, g.s2))
		}
	}
	return out
}

const outcomesDigestSQL = `
	SELECT source, substr(trigger_date, 1, 7) AS ym,
	       COUNT(*), SUM(COALESCE(hit, 0)), COUNT(forward_return)
	FROM stock_signal_outcomes
	GROUP BY source, ym`

const winRateDigestSQL = `
	SELECT source, rolling_window,
	       COUNT(*), SUM(observations), SUM(hits)
	FROM stock_win_rate
	GROUP BY source, rolling_window`

// migrateStockpickerData backfills both stockpicker tables. dryRun computes
// counts + digests and prints the expected migration volume without writing.
func migrateStockpickerData(ctx context.Context, pool *pgxpool.Pool, sqlitePath string, dryRun bool) error {
	src, err := openStockpickerSource(sqlitePath)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	rep := stockpickerReport{}
	rep.TargetHost, rep.TargetPort, rep.TargetDB, err = targetIdentity(ctx, pool)
	if err != nil {
		return fmt.Errorf("read target identity: %w", err)
	}

	if rep.PGOutcomesBefore, err = pgCount(ctx, pool, "stock_signal_outcomes"); err != nil {
		return fmt.Errorf("count pg outcomes: %w", err)
	}
	if rep.PGWinRatesBefore, err = pgCount(ctx, pool, "stock_win_rate"); err != nil {
		return fmt.Errorf("count pg win_rate: %w", err)
	}

	if dryRun {
		log.Printf("[stockpicker:dry-run] target: host=%s port=%d db=%s", rep.TargetHost, rep.TargetPort, rep.TargetDB)
	} else {
		if rep.OutcomesInserted, err = copyOutcomes(ctx, pool, src); err != nil {
			return err
		}
		if rep.WinRatesInserted, err = copyWinRates(ctx, pool, src); err != nil {
			return err
		}
	}

	// Totals are the source row counts regardless of dry-run.
	if rep.OutcomesTotal, err = sqliteCount(src, "stock_signal_outcomes"); err != nil {
		return err
	}
	if rep.WinRatesTotal, err = sqliteCount(src, "stock_win_rate"); err != nil {
		return err
	}

	if rep.PGOutcomesAfter, err = pgCount(ctx, pool, "stock_signal_outcomes"); err != nil {
		return fmt.Errorf("count pg outcomes after: %w", err)
	}
	if rep.PGWinRatesAfter, err = pgCount(ctx, pool, "stock_win_rate"); err != nil {
		return fmt.Errorf("count pg win_rate after: %w", err)
	}

	// Digests: source vs target agreement (target side reflects the writes
	// just done, or the pre-existing state in dry-run).
	srcOutRows, err := src.Query(outcomesDigestSQL)
	if err != nil {
		return fmt.Errorf("sqlite outcomes digest: %w", err)
	}
	srcOutDigest, err := scanDigest(srcOutRows)
	_ = srcOutRows.Close()
	if err != nil {
		return fmt.Errorf("sqlite outcomes digest: %w", err)
	}
	srcWRRows, err := src.Query(winRateDigestSQL)
	if err != nil {
		return fmt.Errorf("sqlite win-rate digest: %w", err)
	}
	srcWRDigest, err := scanDigest(srcWRRows)
	_ = srcWRRows.Close()
	if err != nil {
		return fmt.Errorf("sqlite win-rate digest: %w", err)
	}
	pgOutRows, err := pool.Query(ctx, outcomesDigestSQL)
	if err != nil {
		return fmt.Errorf("pg outcomes digest: %w", err)
	}
	pgOutDigest, err := scanDigest(pgOutRows)
	pgOutRows.Close()
	if err != nil {
		return fmt.Errorf("pg outcomes digest: %w", err)
	}
	pgWRRows, err := pool.Query(ctx, winRateDigestSQL)
	if err != nil {
		return fmt.Errorf("pg win-rate digest: %w", err)
	}
	pgWRDigest, err := scanDigest(pgWRRows)
	pgWRRows.Close()
	if err != nil {
		return fmt.Errorf("pg win-rate digest: %w", err)
	}
	rep.DigestMismatches = append(rep.DigestMismatches, compareDigests(srcOutDigest, pgOutDigest)...)
	rep.DigestMismatches = append(rep.DigestMismatches, compareDigests(srcWRDigest, pgWRDigest)...)

	printStockpickerReport(rep, dryRun)
	// In dry-run nothing was written, so source-vs-target digest differences
	// are expected and informational; the hard check only applies to real
	// backfill runs.
	if !dryRun && len(rep.DigestMismatches) > 0 {
		return fmt.Errorf("stockpicker digest mismatch: %d bucket(s) disagree (see report above)", len(rep.DigestMismatches))
	}
	return nil
}

// targetIdentity reads host/port/db from the live target connection.
func targetIdentity(ctx context.Context, ex spExec) (host string, port int, dbName string, err error) {
	err = ex.QueryRow(ctx,
		`SELECT COALESCE(inet_server_addr()::text, 'local'),
		        COALESCE(inet_server_port(), 0),
		        current_database()`,
	).Scan(&host, &port, &dbName)
	return host, port, dbName, err
}

func pgCount(ctx context.Context, ex spExec, table string) (int64, error) {
	var n int64
	err := ex.QueryRow(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n)
	return n, err
}

func sqliteCount(db *sql.DB, table string) (int64, error) {
	var n int64
	err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n)
	return n, err
}

func printStockpickerReport(rep stockpickerReport, dryRun bool) {
	mode := "migrate"
	if dryRun {
		mode = "dry-run"
	}
	log.Printf("[stockpicker:%s] target host=%s port=%d db=%s", mode, rep.TargetHost, rep.TargetPort, rep.TargetDB)
	log.Printf("[stockpicker:%s] stock_signal_outcomes: source=%d pg_before=%d inserted=%d pg_after=%d",
		mode, rep.OutcomesTotal, rep.PGOutcomesBefore, rep.OutcomesInserted, rep.PGOutcomesAfter)
	log.Printf("[stockpicker:%s] stock_win_rate: source=%d pg_before=%d inserted=%d pg_after=%d",
		mode, rep.WinRatesTotal, rep.PGWinRatesBefore, rep.WinRatesInserted, rep.PGWinRatesAfter)
	if len(rep.DigestMismatches) == 0 {
		log.Printf("[stockpicker:%s] digest: source vs target MATCH", mode)
	} else if dryRun {
		log.Printf("[stockpicker:%s] digest: %d bucket(s) only in source (expected: dry-run writes nothing):", mode, len(rep.DigestMismatches))
		for _, m := range rep.DigestMismatches {
			log.Printf("[stockpicker:%s]   - %s", mode, m)
		}
	} else {
		log.Printf("[stockpicker:%s] digest: %d MISMATCH(es):", mode, len(rep.DigestMismatches))
		for _, m := range rep.DigestMismatches {
			log.Printf("[stockpicker:%s]   - %s", mode, m)
		}
	}
}

// chunkWriter accumulates rows and flushes them inside an explicit PG
// transaction every stockpickerChunkSize rows (fail → rollback, no partial
// chunk commits).
type chunkWriter struct {
	pool    *pgxpool.Pool
	ctx     context.Context
	flushFn func(tx pgx.Tx, batch [][]any) (int64, error)
	batch   [][]any
	total   int64
}

func (w *chunkWriter) add(row ...any) error {
	w.batch = append(w.batch, row)
	if len(w.batch) >= stockpickerChunkSize {
		return w.flush()
	}
	return nil
}

func (w *chunkWriter) flush() error {
	if len(w.batch) == 0 {
		return nil
	}
	tx, err := w.pool.Begin(w.ctx)
	if err != nil {
		return fmt.Errorf("begin chunk tx: %w", err)
	}
	inserted, err := w.flushFn(tx, w.batch)
	if err != nil {
		_ = tx.Rollback(w.ctx)
		return err
	}
	if err := tx.Commit(w.ctx); err != nil {
		return fmt.Errorf("commit chunk tx: %w", err)
	}
	w.total += inserted
	w.batch = w.batch[:0]
	return nil
}

// copyOutcomes streams stock_signal_outcomes SQLite → PG, chunked.
func copyOutcomes(ctx context.Context, pool *pgxpool.Pool, src *sql.DB) (int64, error) {
	rows, err := src.Query(`
		SELECT symbol, trigger_date, source, forward_return, net_forward_return,
		       hit, cost_rate, regime, created_at
		FROM stock_signal_outcomes
		ORDER BY symbol, trigger_date, source`)
	if err != nil {
		return 0, fmt.Errorf("query sqlite outcomes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	w := &chunkWriter{
		pool: pool,
		ctx:  ctx,
		flushFn: func(tx pgx.Tx, batch [][]any) (int64, error) {
			var inserted int64
			for _, r := range batch {
				tag, err := tx.Exec(ctx, `
					INSERT INTO stock_signal_outcomes
						(symbol, trigger_date, source, forward_return, net_forward_return,
						 hit, cost_rate, regime, created_at)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
					ON CONFLICT (symbol, trigger_date, source) DO NOTHING`, r...)
				if err != nil {
					return inserted, fmt.Errorf("insert outcome %v: %w", r[:3], err)
				}
				inserted += tag.RowsAffected()
			}
			return inserted, nil
		},
	}

	for rows.Next() {
		var symbol, triggerDate, source, createdAt string
		var fwd, netFwd, costRate sql.NullFloat64
		var hit sql.NullInt64
		var regime sql.NullString
		if err := rows.Scan(&symbol, &triggerDate, &source, &fwd, &netFwd, &hit, &costRate, &regime, &createdAt); err != nil {
			return w.total, fmt.Errorf("scan outcome row: %w", err)
		}
		if err := w.add(symbol, triggerDate, source, fwd, netFwd, hit, costRate, regime, createdAt); err != nil {
			return w.total, err
		}
	}
	if err := rows.Err(); err != nil {
		return w.total, fmt.Errorf("sqlite outcomes iteration: %w", err)
	}
	if err := w.flush(); err != nil {
		return w.total, err
	}
	return w.total, nil
}

// copyWinRates streams stock_win_rate SQLite → PG, chunked.
func copyWinRates(ctx context.Context, pool *pgxpool.Pool, src *sql.DB) (int64, error) {
	rows, err := src.Query(`
		SELECT symbol, source, rolling_window, observations, hits, win_rate,
		       wilson_lower, wilson_upper, confidence, calibration_status,
		       net_cost_rate, avg_forward_return, updated_at
		FROM stock_win_rate
		ORDER BY symbol, source, rolling_window`)
	if err != nil {
		return 0, fmt.Errorf("query sqlite win_rate: %w", err)
	}
	defer func() { _ = rows.Close() }()

	w := &chunkWriter{
		pool: pool,
		ctx:  ctx,
		flushFn: func(tx pgx.Tx, batch [][]any) (int64, error) {
			var inserted int64
			for _, r := range batch {
				tag, err := tx.Exec(ctx, `
					INSERT INTO stock_win_rate
						(symbol, source, rolling_window, observations, hits, win_rate,
						 wilson_lower, wilson_upper, confidence, calibration_status,
						 net_cost_rate, avg_forward_return, updated_at)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
					ON CONFLICT (symbol, source, rolling_window) DO NOTHING`, r...)
				if err != nil {
					return inserted, fmt.Errorf("insert win_rate %v: %w", r[:3], err)
				}
				inserted += tag.RowsAffected()
			}
			return inserted, nil
		},
	}

	for rows.Next() {
		var symbol, source, window, calibrationStatus, updatedAt string
		var observations, hits int
		var winRate float64
		var wilsonLower, wilsonUpper, confidence, netCostRate, avgFwd sql.NullFloat64
		if err := rows.Scan(&symbol, &source, &window, &observations, &hits, &winRate,
			&wilsonLower, &wilsonUpper, &confidence, &calibrationStatus,
			&netCostRate, &avgFwd, &updatedAt); err != nil {
			return w.total, fmt.Errorf("scan win_rate row: %w", err)
		}
		if err := w.add(symbol, source, window, observations, hits, winRate,
			wilsonLower, wilsonUpper, confidence, calibrationStatus,
			netCostRate, avgFwd, updatedAt); err != nil {
			return w.total, err
		}
	}
	if err := rows.Err(); err != nil {
		return w.total, fmt.Errorf("sqlite win_rate iteration: %w", err)
	}
	if err := w.flush(); err != nil {
		return w.total, err
	}
	return w.total, nil
}
