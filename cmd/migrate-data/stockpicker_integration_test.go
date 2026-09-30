//go:build integration

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

// stockpickerTestPool connects without running the full migration set (the
// backfill only needs 000018/000019) and applies exactly those two migration
// files, so the test stays hermetic in environments without the timescaledb
// extension while always testing against the real checked-in DDL.
func stockpickerTestPool(t *testing.T) *pgxpool.Pool {
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
		// Strip line comments, then split the portable DDL into single
		// statements (pgx extended protocol rejects multi-statement text).
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

// TestMigrateStockpickerData exercises the real backfill against PG (dev
// DATABASE_URL per testdb policy): first run inserts the seeded rows, the
// mandatory second run proves idempotency (inserted = 0), and dry-run writes
// nothing. Seeded rows carry the migratetest-sp source prefix and are
// deleted on cleanup, mirroring cleanupMigrateTestRows.
func TestMigrateStockpickerData(t *testing.T) {
	pool := stockpickerTestPool(t)
	ctx := context.Background()
	cleanupStockpickerTestRows(t, pool)

	srcPath := seedStockpickerSQLite(t)

	if err := migrateStockpickerData(ctx, pool, srcPath, true /* dryRun */); err != nil {
		t.Fatalf("dry-run migrateStockpickerData: %v", err)
	}
	assertStockpickerPGCount(t, pool, 0, "dry-run must not write any outcome rows")

	if err := migrateStockpickerData(ctx, pool, srcPath, false); err != nil {
		t.Fatalf("migrateStockpickerData: %v", err)
	}
	assertStockpickerPGCount(t, pool, seededOutcomeRows, "first backfill must insert all seeded rows")

	// Idempotency pair-proof: a second full run inserts exactly 0 rows and
	// the digest still matches (re-running is safe by construction).
	if err := migrateStockpickerData(ctx, pool, srcPath, false); err != nil {
		t.Fatalf("second migrateStockpickerData: %v", err)
	}
	assertStockpickerPGCount(t, pool, seededOutcomeRows, "second backfill must insert 0 rows (idempotent)")

	// Digest hard-check negative case: corrupt one integer column in the
	// target, re-run, and the digest must fail loudly (protects the check
	// from being silently removed by a future refactor). ON CONFLICT keeps
	// the corrupted row, so only the digest can catch this class of drift.
	if _, err := pool.Exec(ctx,
		"UPDATE stock_signal_outcomes SET hit = 0 WHERE source LIKE 'migratetest-sp%'"); err != nil {
		t.Fatalf("corrupt target rows: %v", err)
	}
	err := migrateStockpickerData(ctx, pool, srcPath, false)
	if err == nil {
		t.Fatal("digest mismatch must fail the run")
	}
	if !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("error must name the digest check, got: %v", err)
	}
}

func cleanupStockpickerTestRows(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	delete := func() {
		ctx := context.Background()
		if _, err := pool.Exec(ctx, "DELETE FROM stock_signal_outcomes WHERE source LIKE 'migratetest-sp%'"); err != nil {
			t.Logf("cleanup outcomes: %v", err)
		}
		if _, err := pool.Exec(ctx, "DELETE FROM stock_win_rate WHERE source LIKE 'migratetest-sp%'"); err != nil {
			t.Logf("cleanup win_rate: %v", err)
		}
	}
	delete()
	t.Cleanup(delete)
}

const (
	seededOutcomeRows = 12
	seededWinRateRows = 4
)

// seedStockpickerSQLite builds a temp atlas.db with the stockpicker schema
// and seeded rows across two sources and two months.
func seedStockpickerSQLite(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "atlas.db")
	db, err := ledger.OpenSQLiteDB(path)
	if err != nil {
		t.Fatalf("create temp sqlite: %v", err)
	}
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("init temp sqlite schema: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for i := 0; i < seededOutcomeRows; i++ {
		source := "migratetest-sp-condA"
		month := "2026-08"
		if i >= 6 {
			source = "migratetest-sp-condB"
			month = "2026-09"
		}
		var hit sql.NullInt64
		if i%2 == 0 {
			hit = sql.NullInt64{Int64: 1, Valid: true}
		}
		if _, err := db.Exec(`
			INSERT INTO stock_signal_outcomes
				(symbol, trigger_date, source, forward_return, net_forward_return, hit, cost_rate, regime, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			fmt.Sprintf("%04d.TW", 2300+i), fmt.Sprintf("%s-%02d", month, i%28+1), source,
			0.01*float64(i+1), 0.009*float64(i+1), hit, 0.001, "risk_on", "2026-09-30T00:00:00Z"); err != nil {
			t.Fatalf("seed outcome %d: %v", i, err)
		}
	}
	for i := 0; i < seededWinRateRows; i++ {
		if _, err := db.Exec(`
			INSERT INTO stock_win_rate
				(symbol, source, rolling_window, observations, hits, win_rate,
				 wilson_lower, wilson_upper, confidence, calibration_status,
				 net_cost_rate, avg_forward_return, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			fmt.Sprintf("%04d.TW", 2300+i), "migratetest-sp-condA", "120d",
			6, 3, 0.5, 0.2, 0.8, 0.95, "eligible", 0.001, 0.01, "2026-09-30T00:00:00Z"); err != nil {
			t.Fatalf("seed win_rate %d: %v", i, err)
		}
	}
	return path
}

func assertStockpickerPGCount(t *testing.T, pool *pgxpool.Pool, want int, msg string) {
	t.Helper()
	ctx := context.Background()
	var n int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM stock_signal_outcomes WHERE source LIKE 'migratetest-sp%'").Scan(&n); err != nil {
		t.Fatalf("count pg outcomes: %v", err)
	}
	if n != want {
		t.Fatalf("%s: pg stock_signal_outcomes rows = %d, want %d", msg, n, want)
	}
	var wr int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM stock_win_rate WHERE source LIKE 'migratetest-sp%'").Scan(&wr); err != nil {
		t.Fatalf("count pg win_rate: %v", err)
	}
	if want == 0 && wr != 0 {
		t.Fatalf("%s: pg stock_win_rate rows = %d, want 0", msg, wr)
	}
	if want > 0 && wr != seededWinRateRows {
		t.Fatalf("%s: pg stock_win_rate rows = %d, want %d", msg, wr, seededWinRateRows)
	}
}

// TestGuardMigrateTarget exercises the M12 guard against the real dev PG:
// a wrong expect-db aborts with the actual database name, a matching one
// passes. Read-only.
func TestGuardMigrateTarget(t *testing.T) {
	ctx := context.Background()
	dsn := testdb.URL(t)

	var actual string
	pool := testdb.Connect(t, dsn)
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&actual); err != nil {
		t.Fatalf("read current_database: %v", err)
	}
	pool.Close()

	if err := guardMigrateTarget(ctx, dsn, actual); err != nil {
		t.Fatalf("guard with matching expect-db must pass, got: %v", err)
	}
	err := guardMigrateTarget(ctx, dsn, "definitely-not-this-db")
	if err == nil {
		t.Fatal("guard with wrong expect-db must fail")
	}
	if !containsAll(err.Error(), []string{"definitely-not-this-db", actual}) {
		t.Fatalf("guard error must name both expected and actual db, got: %v", err)
	}
}

func containsAll(s string, subs []string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
