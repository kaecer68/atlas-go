//go:build integration

package stockpicker

// Dual-write (SSOT→PG batch B) integration tests: the daily update mirrors
// outcome + win-rate rows into PostgreSQL alongside the job-local SQLite
// artifact; a PG failure fails the whole run; dry-run writes neither side.
//
// Row scoping: the tests pin the single condition foreign-3d-net-buy and the
// synthetic fixture dates (2026-01-05..2026-03-16), so cleanup/assertions key
// on (source, trigger_date range) — safe on the shared dev database, where
// real runs only ever write recent dates.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/kaecer68/atlas-go/internal/testdb"
)

const (
	dwSource      = "stockpicker-foreign-3d-net-buy"
	dwCondition   = "foreign-3d-net-buy"
	dwStart       = "2026-01-05"
	dwEnd         = "2026-03-16"
	dwCleanupLike = "DELETE FROM %s WHERE source = '" + dwSource + "' AND trigger_date >= '" + dwStart + "' AND trigger_date <= '" + dwEnd + "'"
)

// stockpickerPGTestDB connects to the dev PG (testdb policy) and applies the
// two stockpicker migration files, so the test runs against the real checked
// in DDL without requiring the timescaledb-dependent full migration set.
func stockpickerPGTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := testdb.Connect(t, testdb.URL(t))
	ctx := context.Background()
	for _, file := range []string{
		"000018_stock_signal_outcomes.up.sql",
		"000019_stock_win_rate.up.sql",
	} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "sql", "migrations", file))
		if err != nil {
			t.Fatalf("read migration %s: %v", file, err)
		}
		var lines []string
		for _, line := range strings.Split(string(raw), "\n") {
			if trimmed := strings.TrimSpace(line); trimmed != "" && !strings.HasPrefix(trimmed, "--") {
				lines = append(lines, line)
			}
		}
		for _, stmt := range strings.Split(strings.Join(lines, "\n"), ";") {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := pool.Exec(ctx, stmt); err != nil {
				t.Fatalf("apply %s: %v", file, err)
			}
		}
	}
	return pool
}

func cleanupDualWriteRows(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	delete := func() {
		ctx := context.Background()
		if _, err := pool.Exec(ctx, fmt.Sprintf(dwCleanupLike, "stock_signal_outcomes")); err != nil {
			t.Logf("cleanup outcomes: %v", err)
		}
		// Win-rate rows carry no trigger_date; scope by the fixture symbol
		// ("2330" bare — real runs use suffixed symbols like "2330.TW") so a
		// real run's summary under the same source is untouched.
		if _, err := pool.Exec(ctx,
			"DELETE FROM stock_win_rate WHERE source = '"+dwSource+"' AND symbol = '2330'"); err != nil {
			t.Logf("cleanup win_rate: %v", err)
		}
	}
	delete()
	t.Cleanup(delete)
}

func pgOutcomeCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM stock_signal_outcomes
		WHERE source = $1 AND trigger_date >= $2 AND trigger_date <= $3`,
		dwSource, dwStart, dwEnd).Scan(&n); err != nil {
		t.Fatalf("count pg outcomes: %v", err)
	}
	return n
}

func TestRunDailyUpdate_DualWrite(t *testing.T) {
	pool := stockpickerPGTestDB(t)
	cleanupDualWriteRows(t, pool)

	workdir, res, err := runUpdate(t, RunDailyOptions{
		Idempotency:  IdempotencyNone,
		Conditions:   dwCondition,
		PGOutcomesDB: stdlib.OpenDBFromPool(pool),
	})
	if err != nil {
		t.Fatalf("RunDailyUpdate dual-write: %v", err)
	}
	if res.Outcomes == 0 {
		t.Fatal("expected outcomes to be produced")
	}

	// SQLite side: the artifact holds the rows.
	sqliteDB := openTestOutcomeDB(t, workdir)
	var sqliteN int
	if err := sqliteDB.QueryRow(
		"SELECT COUNT(*) FROM stock_signal_outcomes WHERE trigger_date >= ? AND trigger_date <= ?",
		dwStart, dwEnd).Scan(&sqliteN); err != nil {
		t.Fatalf("count sqlite outcomes: %v", err)
	}
	if pgN := pgOutcomeCount(t, pool); pgN != sqliteN {
		t.Fatalf("dual-write drift: sqlite=%d pg=%d", sqliteN, pgN)
	}

	// Win-rate rows mirrored too (single condition → one summary key).
	var pgWR int
	if err := pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM stock_win_rate WHERE source = $1 AND symbol = '2330'", dwSource).Scan(&pgWR); err != nil {
		t.Fatalf("count pg win_rate: %v", err)
	}
	var sqliteWR int
	if err := sqliteDB.QueryRow(
		"SELECT COUNT(*) FROM stock_win_rate WHERE source = ?", dwSource).Scan(&sqliteWR); err != nil {
		t.Fatalf("count sqlite win_rate: %v", err)
	}
	if pgWR != sqliteWR || pgWR != 1 {
		t.Fatalf("win-rate mirror drift: sqlite=%d pg=%d (want 1 each)", sqliteWR, pgWR)
	}

	// Second identical run must be idempotent on both sides (ON CONFLICT).
	if _, _, err := runUpdate(t, RunDailyOptions{
		Idempotency:  IdempotencyNone,
		Conditions:   dwCondition,
		PGOutcomesDB: stdlib.OpenDBFromPool(pool),
	}); err != nil {
		t.Fatalf("second dual-write run: %v", err)
	}
	if n := pgOutcomeCount(t, pool); n != sqliteN {
		t.Fatalf("dual-write rerun not idempotent: pg=%d want %d", n, sqliteN)
	}
}

// barePGPool returns a connection to a freshly created database WITHOUT any
// tables — every query against it fails (used to simulate a broken/migrated-
// away PG destination).
func barePGPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	admin := testdb.Connect(t, testdb.URL(t))
	ctx := context.Background()
	bareDB := "stockpicker_dwtest_bare_" + strings.ToLower(t.Name()[len("TestRunDailyUpdate_"):])
	if len(bareDB) > 60 {
		bareDB = bareDB[:60]
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+bareDB); err != nil {
		t.Skipf("cannot create bare database (needs createdb privilege): %v", err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+bareDB); err != nil {
			t.Logf("drop bare db: %v", err)
		}
		admin.Close()
	})
	pool := testdb.Connect(t, replaceDBName(t, testdb.URL(t), bareDB))
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestRunDailyUpdate_DualWrite_PGFailureFailsRun(t *testing.T) {
	// A PG destination without the stockpicker tables must fail the run
	// loudly (never a silent SQLite-only skip).
	_, _, err := runUpdate(t, RunDailyOptions{
		Idempotency:  IdempotencyNone,
		Conditions:   dwCondition,
		PGOutcomesDB: stdlib.OpenDBFromPool(barePGPool(t)),
	})
	if err == nil {
		t.Fatal("PG write failure must fail the run")
	}
	if !strings.Contains(err.Error(), "postgres") {
		t.Fatalf("error must name the postgres side, got: %v", err)
	}
}

func TestRunDailyUpdate_DualWrite_DryRunWritesNothing(t *testing.T) {
	pool := stockpickerPGTestDB(t)
	cleanupDualWriteRows(t, pool)

	_, res, err := runUpdate(t, RunDailyOptions{
		Idempotency:  IdempotencyNone,
		Conditions:   dwCondition,
		DryRun:       true,
		PGOutcomesDB: stdlib.OpenDBFromPool(pool),
	})
	if err != nil {
		t.Fatalf("dry-run dual-write: %v", err)
	}
	if res.Outcomes == 0 {
		t.Fatal("dry-run should still compute outcomes")
	}
	if n := pgOutcomeCount(t, pool); n != 0 {
		t.Fatalf("dry-run wrote %d pg outcome rows, want 0", n)
	}
}

// replaceDBName swaps the database name in a postgres DSN (tests only).
func replaceDBName(t *testing.T, dsn, dbName string) string {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	password := cfg.ConnConfig.Password
	if password != "" {
		password = ":" + password
	}
	return fmt.Sprintf("postgres://%s%s@%s:%d/%s?sslmode=disable",
		cfg.ConnConfig.User, password, cfg.ConnConfig.Host, cfg.ConnConfig.Port, dbName)
}

// TestRunDailyUpdate_DualWrite_IdempotencyDay_HealsPGGap is the direct
// regression of independent-review P1: with dual-write, a PG-side gap must
// NOT be hidden behind the SQLite idempotency gate.
//
// Step 1 (bad PG, IdempotencyDay): fresh workdir → gate sees 0 SQLite rows →
// run proceeds → PG write fails → error (the scheduler records the failure).
// Step 2 (good PG, same workdir, IdempotencyDay): OLD code would see the
// SQLite rows and return Skipped=true with PG forever empty (the P1
// disproof); the fix counts the PG side, reruns, and heals the gap.
// Step 3 (good PG, again): both destinations recorded → Skipped=true.
func TestRunDailyUpdate_DualWrite_IdempotencyDay_HealsPGGap(t *testing.T) {
	pool := stockpickerPGTestDB(t)
	cleanupDualWriteRows(t, pool)
	workdir := writeTestWorkdir(t)

	if _, _, err := runUpdate(t, RunDailyOptions{
		WorkDir:      workdir,
		Idempotency:  IdempotencyDay,
		Conditions:   dwCondition,
		PGOutcomesDB: stdlib.OpenDBFromPool(barePGPool(t)),
	}); err == nil {
		t.Fatal("step 1: PG write failure must fail the run")
	}

	_, res2, err := runUpdate(t, RunDailyOptions{
		WorkDir:      workdir,
		Idempotency:  IdempotencyDay,
		Conditions:   dwCondition,
		PGOutcomesDB: stdlib.OpenDBFromPool(pool),
	})
	if err != nil {
		t.Fatalf("step 2: %v", err)
	}
	if res2.Skipped {
		t.Fatal("step 2: P1 regression — SQLite rows hid the PG gap and the rerun was skipped")
	}
	if n := pgOutcomeCount(t, pool); n == 0 {
		t.Fatal("step 2: PG gap was not healed")
	}

	_, res3, err := runUpdate(t, RunDailyOptions{
		WorkDir:      workdir,
		Idempotency:  IdempotencyDay,
		Conditions:   dwCondition,
		PGOutcomesDB: stdlib.OpenDBFromPool(pool),
	})
	if err != nil {
		t.Fatalf("step 3: %v", err)
	}
	if !res3.Skipped {
		t.Fatal("step 3: both destinations recorded — the run must skip")
	}
}

// TestRunDailyUpdate_DualWrite_IdempotencyRange_HealsPGGap covers the
// IdempotencyRange gate with the same three-step contract as the Day case.
func TestRunDailyUpdate_DualWrite_IdempotencyRange_HealsPGGap(t *testing.T) {
	pool := stockpickerPGTestDB(t)
	cleanupDualWriteRows(t, pool)
	workdir := writeTestWorkdir(t)

	if _, _, err := runUpdate(t, RunDailyOptions{
		WorkDir:      workdir,
		Idempotency:  IdempotencyRange,
		Conditions:   dwCondition,
		PGOutcomesDB: stdlib.OpenDBFromPool(barePGPool(t)),
	}); err == nil {
		t.Fatal("step 1: PG write failure must fail the run")
	}

	_, res2, err := runUpdate(t, RunDailyOptions{
		WorkDir:      workdir,
		Idempotency:  IdempotencyRange,
		Conditions:   dwCondition,
		PGOutcomesDB: stdlib.OpenDBFromPool(pool),
	})
	if err != nil {
		t.Fatalf("step 2: %v", err)
	}
	if res2.Skipped {
		t.Fatal("step 2: P1 regression — range gate skipped the PG-healing rerun")
	}
	if n := pgOutcomeCount(t, pool); n == 0 {
		t.Fatal("step 2: PG gap was not healed")
	}

	_, res3, err := runUpdate(t, RunDailyOptions{
		WorkDir:      workdir,
		Idempotency:  IdempotencyRange,
		Conditions:   dwCondition,
		PGOutcomesDB: stdlib.OpenDBFromPool(pool),
	})
	if err != nil {
		t.Fatalf("step 3: %v", err)
	}
	if !res3.Skipped {
		t.Fatal("step 3: both destinations recorded — the run must skip")
	}
}

// TestRunDailyUpdate_DualWrite_PGCountFailureIsDistinct pins the hard
// requirement that a failing PG COUNT inside the idempotency gate is
// distinguishable from "the day genuinely has 0 rows": the error must name
// the postgres idempotency check (a "0 rows" outcome never errors).
func TestRunDailyUpdate_DualWrite_PGCountFailureIsDistinct(t *testing.T) {
	pool := stockpickerPGTestDB(t)
	cleanupDualWriteRows(t, pool)
	workdir := writeTestWorkdir(t)

	// Heal both destinations first.
	if _, _, err := runUpdate(t, RunDailyOptions{
		WorkDir:      workdir,
		Idempotency:  IdempotencyNone,
		Conditions:   dwCondition,
		PGOutcomesDB: stdlib.OpenDBFromPool(pool),
	}); err != nil {
		t.Fatalf("seed run: %v", err)
	}

	// Now the gate reaches the PG count (SQLite side has rows) and that
	// count fails against a table-less database.
	_, _, err := runUpdate(t, RunDailyOptions{
		WorkDir:      workdir,
		Idempotency:  IdempotencyDay,
		Conditions:   dwCondition,
		PGOutcomesDB: stdlib.OpenDBFromPool(barePGPool(t)),
	})
	if err == nil {
		t.Fatal("PG count failure inside the gate must fail the run")
	}
	if !strings.Contains(err.Error(), "idempotency check (postgres)") {
		t.Fatalf("error must name the postgres idempotency count, got: %v", err)
	}
}
