//go:build integration

package stockpicker

// pg_primary_integration_test.go — C7 (2026-10-13) against a real PostgreSQL:
// on the postgres backend the daily update writes PostgreSQL as its PRIMARY
// destination and leaves the job-local SQLite artifact uncreated (unless the
// opt-in mirror is on). The hermetic tier
// (pg_primary_write_test.go) pins which destination receives which statement;
// this tier pins the row-level facts that need a server: the rows are really
// there, the second identical run really skips, and a dry run really writes
// nothing.
//
// The PG handle is injected (stdlib.OpenDBFromPool over the test pool) so the
// tests never touch DATA AT the production database; the row scoping is the
// one already used by dual_write_integration_test.go (single condition
// foreign-3d-net-buy + the synthetic 2026-01-05..2026-03-16 dates).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

// pgCurrentDatabase reads the connected database name: it is what the M12
// migration-target guard asserts against (ExpectDB), so the tests exercise the
// guard for real instead of stubbing it out.
func pgCurrentDatabase(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var name string
	if err := pool.QueryRow(context.Background(), "SELECT current_database()").Scan(&name); err != nil {
		t.Fatalf("read current_database: %v", err)
	}
	return name
}

// TestRunDailyUpdate_PGPrimary_Integration is the end-to-end C7 contract on a
// real PostgreSQL: PG gets the rows, the SQLite artifact is never created, a
// rerun of the same AsOf skips, and a dry run writes nothing anywhere.
func TestRunDailyUpdate_PGPrimary_Integration(t *testing.T) {
	pool := stockpickerPGTestDB(t)
	cleanupDualWriteRows(t, pool)

	dbName := pgCurrentDatabase(t, pool)
	pg := stdlib.OpenDBFromPool(pool)

	baseOpts := func(workdir string) RunDailyOptions {
		return RunDailyOptions{
			WorkDir:      workdir,
			Backend:      "postgres",
			ExpectDB:     dbName,
			PGOutcomesDB: pg,
			Idempotency:  IdempotencyDay,
			Conditions:   dwCondition,
		}
	}

	// Run 1: PG-primary, mirror off (the C7 default).
	workdir := writeTestWorkdir(t)
	_, res, err := runUpdate(t, baseOpts(workdir))
	if err != nil {
		t.Fatalf("PG-primary run: %v", err)
	}
	if res.Primary != "postgres" || res.SQLiteMirror {
		t.Fatalf("plan = primary %q, sqlite mirror %v; want postgres / false", res.Primary, res.SQLiteMirror)
	}
	if res.Outcomes == 0 {
		t.Fatal("expected outcomes to be produced")
	}
	// (1) PostgreSQL holds the rows.
	pgN := pgOutcomeCount(t, pool)
	if pgN != res.Outcomes {
		t.Fatalf("PostgreSQL holds %d outcome rows, want %d (the run's outcomes)", pgN, res.Outcomes)
	}
	var pgWR, pgRegime int
	if err := pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM stock_win_rate WHERE source = $1", dwSource).Scan(&pgWR); err != nil {
		t.Fatalf("count pg win_rate: %v", err)
	}
	if err := pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM stock_win_rate_by_regime WHERE source = $1", dwSource).Scan(&pgRegime); err != nil {
		t.Fatalf("count pg regime strata: %v", err)
	}
	if pgWR != 1 {
		t.Fatalf("stock_win_rate rows = %d, want 1 (single condition run)", pgWR)
	}
	if pgRegime == 0 {
		t.Fatal("stock_win_rate_by_regime received no strata")
	}
	// (2) The SQLite artifact was not written — and was not even created.
	if problems := sqliteArtifactProblems(res, workdir); len(problems) != 0 {
		t.Fatalf("PG-primary run violated the C7 contract:\n  - %s", strings.Join(problems, "\n  - "))
	}

	// (3) The same AsOf again skips (nothing new to compute, counted on PG).
	_, res2, err := runUpdate(t, baseOpts(workdir))
	if err != nil {
		t.Fatalf("second PG-primary run: %v", err)
	}
	if !res2.Skipped {
		t.Fatal("the second identical run must skip (the day's increment is recorded in PostgreSQL)")
	}
	if n := pgOutcomeCount(t, pool); n != pgN {
		t.Fatalf("the skipped rerun changed the PostgreSQL row count: %d → %d", pgN, n)
	}

	// (4) A dry run writes nothing on either destination.
	dryDir := writeTestWorkdir(t)
	before := pgOutcomeCount(t, pool)
	_, dryRes, err := runUpdate(t, func() RunDailyOptions {
		o := baseOpts(dryDir)
		o.DryRun = true
		return o
	}())
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if dryRes.Outcomes == 0 {
		t.Fatal("dry-run should still compute outcomes")
	}
	if after := pgOutcomeCount(t, pool); after != before {
		t.Fatalf("dry run wrote to PostgreSQL: %d → %d rows", before, after)
	}
	if _, err := os.Stat(filepath.Join(dryDir, "data", "state", "atlas.db")); !os.IsNotExist(err) {
		t.Fatalf("dry run created data/state/atlas.db (stat err=%v)", err)
	}
	if _, err := os.Stat(filepath.Join(dryDir, "data", "state", "stock_win_rate.json")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote the state snapshot (stat err=%v)", err)
	}
}

// TestRunDailyUpdate_PGPrimary_Integration_SQLiteMirror is the recoverable
// path on a real PostgreSQL: ATLAS_STOCKPICKER_SQLITE_MIRROR=on writes the
// same rows into the artifact while PostgreSQL stays authoritative — the PG
// and SQLite row counts must agree (the pre-C7 "both sides" shape).
func TestRunDailyUpdate_PGPrimary_Integration_SQLiteMirror(t *testing.T) {
	pool := stockpickerPGTestDB(t)
	cleanupDualWriteRows(t, pool)
	pg := stdlib.OpenDBFromPool(pool)

	workdir, res, err := runUpdate(t, RunDailyOptions{
		Backend:      "postgres",
		ExpectDB:     pgCurrentDatabase(t, pool),
		PGOutcomesDB: pg,
		SQLiteMirror: true,
		Idempotency:  IdempotencyNone,
		Conditions:   dwCondition,
	})
	if err != nil {
		t.Fatalf("PG-primary run with the SQLite mirror on: %v", err)
	}
	if !res.SQLiteMirror || res.Primary != "postgres" {
		t.Fatalf("plan = primary %q, sqlite mirror %v; want postgres / true", res.Primary, res.SQLiteMirror)
	}
	if _, err := os.Stat(filepath.Join(workdir, "data", "state", "atlas.db")); err != nil {
		t.Fatalf("mirror on must create the artifact: %v", err)
	}

	sqliteDB := openTestOutcomeDB(t, workdir)
	if n := outcomeCount(t, sqliteDB); n != res.Outcomes {
		t.Fatalf("SQLite mirror holds %d outcomes, want %d", n, res.Outcomes)
	}
	if n := pgOutcomeCount(t, pool); n != res.Outcomes {
		t.Fatalf("PostgreSQL holds %d outcomes, want %d", n, res.Outcomes)
	}
	if got := countRows(t, sqliteDB, "SELECT COUNT(*) FROM stock_win_rate WHERE source = '"+dwSource+"'"); got != 1 {
		t.Fatalf("SQLite mirror stock_win_rate rows = %d, want 1", got)
	}
	var pgWR int
	if err := pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM stock_win_rate WHERE source = $1", dwSource).Scan(&pgWR); err != nil {
		t.Fatalf("count pg win_rate: %v", err)
	}
	if pgWR != 1 {
		t.Fatalf("PostgreSQL stock_win_rate rows = %d, want 1", pgWR)
	}
	if got := countRows(t, sqliteDB,
		"SELECT COUNT(*) FROM stock_win_rate_by_regime WHERE source = '"+dwSource+"'"); got == 0 {
		t.Fatal("SQLite mirror received no regime strata")
	}
}

// TestRunDailyUpdate_PGPrimary_Integration_NegativeControl runs the SAME
// contract assertions against the pre-C7 shape (the DualWrite kill switch
// off ⇒ SQLite is primary, zero PG writes) with a real PostgreSQL present: the
// assertions must go red, and the artifact must be there. This is the proof
// that the green above is not green for the wrong reason.
func TestRunDailyUpdate_PGPrimary_Integration_NegativeControl(t *testing.T) {
	pool := stockpickerPGTestDB(t)
	cleanupDualWriteRows(t, pool)

	workdir, res, err := runUpdate(t, RunDailyOptions{
		Backend:     "postgres", // declared postgres backend...
		ExpectDB:    pgCurrentDatabase(t, pool),
		DualWrite:   false, // ...but the pre-C7 kill switch: SQLite stays primary
		Idempotency: IdempotencyNone,
		Conditions:  dwCondition,
	})
	if err != nil {
		t.Fatalf("legacy kill-switch run: %v", err)
	}
	if res.Primary != "sqlite" {
		t.Fatalf("primary = %q, want sqlite for the pre-C7 shape", res.Primary)
	}
	if _, err := os.Stat(filepath.Join(workdir, "data", "state", "atlas.db")); err != nil {
		t.Fatalf("the legacy shape must write the artifact: %v", err)
	}
	problems := sqliteArtifactProblems(res, workdir)
	if len(problems) == 0 {
		t.Fatal("the C7 assertions accepted a run that still writes atlas.db")
	}
	if !strings.Contains(strings.Join(problems, "\n"), "atlas.db was created") {
		t.Fatalf("the artifact must be reported as created; problems=%v", problems)
	}
	if n := pgOutcomeCount(t, pool); n != 0 {
		t.Fatalf("the kill switch wrote %d PostgreSQL rows, want 0", n)
	}
}
