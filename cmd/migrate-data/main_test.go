//go:build integration

package main

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/testdb"
)

// connectTestPG connects to PostgreSQL (DATABASE_URL only, no hardcoded DSN)
// and returns the pool. See testdb.Pool for the CI/local skip policy.
func connectTestPG(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return testdb.Pool(t, "../../sql/migrations")
}

func cleanupMigrateTestRows(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, _ = pool.Exec(ctx, "DELETE FROM recommendation_outcomes WHERE agent_id LIKE 'migratetest-%'")
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, "DELETE FROM recommendation_outcomes WHERE agent_id LIKE 'migratetest-%'")
	})
}

// TestRemapOutcomeSessions verifies date-format session_id rows are rewritten
// to session-YYYYMMDD-daily and the remap is idempotent (second run touches 0).
func TestRemapOutcomeSessions(t *testing.T) {
	pool := connectTestPG(t)
	cleanupMigrateTestRows(t, pool)
	ctx := context.Background()

	// Run the whole-table remap inside a transaction and roll it back so the
	// test never mutates shared production data in the dev PG.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Insert date-format rows the way insertOutcomeBatch used to (session_id =
	// o.Window = date) plus an already-correct session-format row.
	for _, sid := range []string{"2026-07-22", "2026-07-23"} {
		if _, err := tx.Exec(ctx, `
			INSERT INTO recommendation_outcomes (time, session_id, symbol, agent_id, agent_layer, conviction, passed_guards, guard_reason, price, metadata)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, "2026-07-22T00:00:00Z", sid, "2330.TW", "migratetest-a1", "sector", 80, true, "", 100.0, `{}`); err != nil {
			t.Fatalf("seed insert: %v", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO recommendation_outcomes (time, session_id, symbol, agent_id, agent_layer, conviction, passed_guards, guard_reason, price, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, "2026-07-22T00:00:00Z", "session-20260724-daily", "2317.TW", "migratetest-a2", "sector", 80, true, "", 100.0, `{}`); err != nil {
		t.Fatalf("seed insert: %v", err)
	}

	if err := remapOutcomeSessions(ctx, tx); err != nil {
		t.Fatalf("remapOutcomeSessions: %v", err)
	}

	var dateCount int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM recommendation_outcomes WHERE agent_id LIKE 'migratetest-%' AND session_id ~ '^\d{4}-\d{2}-\d{2}$'`).Scan(&dateCount); err != nil {
		t.Fatalf("count date rows: %v", err)
	}
	if dateCount != 0 {
		t.Fatalf("expected 0 date-format session_id rows after remap, got %d", dateCount)
	}

	var sessionCount int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM recommendation_outcomes WHERE agent_id LIKE 'migratetest-%' AND session_id = 'session-20260722-daily'`).Scan(&sessionCount); err != nil {
		t.Fatalf("count session rows: %v", err)
	}
	if sessionCount != 1 {
		t.Fatalf("expected 1 remapped session-20260722-daily row, got %d", sessionCount)
	}

	// Idempotency: a second remap touches nothing new (all rows already remapped).
	var before int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM recommendation_outcomes WHERE agent_id LIKE 'migratetest-%'`).Scan(&before); err != nil {
		t.Fatalf("count before: %v", err)
	}
	if err := remapOutcomeSessions(ctx, tx); err != nil {
		t.Fatalf("second remapOutcomeSessions: %v", err)
	}
	var after int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM recommendation_outcomes WHERE agent_id LIKE 'migratetest-%'`).Scan(&after); err != nil {
		t.Fatalf("count after: %v", err)
	}
	if before != after {
		t.Fatalf("remap not idempotent: before=%d after=%d", before, after)
	}
}

func TestInsertSessionOutcomeBatch(t *testing.T) {
	pool := connectTestPG(t)
	cleanupMigrateTestRows(t, pool)
	ctx := context.Background()

	outcomes := []domain.RecommendationOutcome{
		{AgentID: "migratetest-s1", Symbol: "2330.TW", Side: "BUY", Conviction: 80, Window: "2026-07-22", PassedGuards: true},
		{AgentID: "migratetest-s2", Symbol: "2317.TW", Side: "BUY", Conviction: 70, Window: "2026-07-22", PassedGuards: false},
	}
	sessions := []string{"session-20260722-daily", "session-20260722-daily"}

	inserted, err := insertSessionOutcomeBatch(ctx, pool, sessions, outcomes)
	if err != nil {
		t.Fatalf("insertSessionOutcomeBatch: %v", err)
	}
	if inserted != 2 {
		t.Fatalf("expected 2 inserted rows, got %d", inserted)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM recommendation_outcomes WHERE agent_id LIKE 'migratetest-%' AND session_id = 'session-20260722-daily'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 session rows, got %d", n)
	}

	// Re-run: guard blocks duplicates, reports 0 inserted.
	inserted, err = insertSessionOutcomeBatch(ctx, pool, sessions, outcomes)
	if err != nil {
		t.Fatalf("second insertSessionOutcomeBatch: %v", err)
	}
	if inserted != 0 {
		t.Fatalf("expected 0 inserted on idempotent re-run, got %d", inserted)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM recommendation_outcomes WHERE agent_id LIKE 'migratetest-%' AND session_id = 'session-20260722-daily'`).Scan(&n); err != nil {
		t.Fatalf("count after re-run: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 rows after idempotent re-run, got %d", n)
	}
}

// TestInsertOutcomeBatchGlobal verifies SQLite global rows land with
// session_id=” (not o.Window) and the NOT EXISTS guard keeps re-runs
// idempotent — regression test for the -outcomes-sqlite non-idempotency
// fixed by insertOutcomeBatchGlobal.
func TestInsertOutcomeBatchGlobal(t *testing.T) {
	pool := connectTestPG(t)
	cleanupMigrateTestRows(t, pool)
	ctx := context.Background()

	// A dated-window global row: window is the evaluation date; session_id
	// must be '' (global aggregate), NOT the date.
	outcomes := []domain.RecommendationOutcome{
		{AgentID: "migratetest-g1", Symbol: "00713.TW", Side: "BUY", Conviction: 60, Window: "2026-07-22", PassedGuards: true},
	}

	inserted, err := insertOutcomeBatchGlobal(ctx, pool, outcomes)
	if err != nil {
		t.Fatalf("insertOutcomeBatchGlobal: %v", err)
	}
	if inserted != 1 {
		t.Fatalf("expected 1 inserted, got %d", inserted)
	}

	var sid string
	if err := pool.QueryRow(ctx, `SELECT session_id FROM recommendation_outcomes WHERE agent_id='migratetest-g1'`).Scan(&sid); err != nil {
		t.Fatalf("query session_id: %v", err)
	}
	if sid != "" {
		t.Fatalf("expected global session_id='', got %q (dated-window global row must NOT become a date session_id)", sid)
	}

	// Re-run: guard blocks, 0 inserted.
	inserted, err = insertOutcomeBatchGlobal(ctx, pool, outcomes)
	if err != nil {
		t.Fatalf("second insertOutcomeBatchGlobal: %v", err)
	}
	if inserted != 0 {
		t.Fatalf("expected 0 inserted on idempotent re-run, got %d", inserted)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM recommendation_outcomes WHERE agent_id='migratetest-g1'`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row after idempotent re-run, got %d", n)
	}
}
