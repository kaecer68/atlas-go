//go:build integration

package ledger

// FU-20261016-07, PostgreSQL half.
//
// The hermetic tests in mutation_brief_upsert_test.go cover the SQLite backend
// and the legacy-file heal. This file covers the two claims that need a real
// server:
//
//  1. migration 000029 really does make mutation_briefs.window_id unique, so a
//     store write for a window that already has a row refreshes it;
//  2. the migration REFUSES to apply while duplicates exist — and refuses
//     without deleting anything. That is the decision recorded when 000029 was
//     written: cleaning duplicate rows is an operator data operation, never a
//     schema step (production carried 0 brief rows on 2026-10-16, verified
//     read-only, so no cleanup was needed to ship it).

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/domain"
)

const (
	fu101607MigrationFile = "000029_mutation_briefs_unique_window_id.up.sql"
	fu101607ScratchSchema = "fu101607_migration_probe"
	// The scratch-schema tests build their own schema; the store test writes
	// through the shared tables, so its window id carries the suite prefix that
	// q2b2wCleanPG removes (before and after every run).
	fu101607WindowID = q2b2wWindowPrefix + "fu101607-upsert"
)

// fu101607MigrationStatement returns the up migration as one executable
// statement. It fails the test if the file ever grows a second statement, which
// keeps the "one statement per up migration" property visible (and keeps this
// test honest: it executes exactly what the migration runner would).
func fu101607MigrationStatement(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "sql", "migrations", fu101607MigrationFile))
	if err != nil {
		t.Fatalf("read %s: %v", fu101607MigrationFile, err)
	}
	var lines []string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		lines = append(lines, line)
	}
	stmt := strings.TrimSpace(strings.Join(lines, "\n"))
	if stmt == "" {
		t.Fatalf("%s has no statement", fu101607MigrationFile)
	}
	if strings.Contains(strings.TrimSuffix(stmt, ";"), ";") {
		t.Fatalf("%s carries more than one statement; this test executes one", fu101607MigrationFile)
	}
	return stmt
}

// fu101607ScratchSetup builds a throwaway schema holding the PRE-000029 shape of
// mutation_briefs plus `duplicates` rows for one window, and returns a single
// acquired connection whose search_path points at that schema (the migration
// statement names the table unqualified, so it must run on that connection).
// The schema is dropped on cleanup, so the shared database is left as found.
func fu101607ScratchSetup(t *testing.T, duplicates int) *pgxpool.Conn {
	t.Helper()
	pool := connectTestPG(t)
	ctx := context.Background()

	drop := func() {
		if _, err := pool.Exec(ctx, "DROP SCHEMA IF EXISTS "+fu101607ScratchSchema+" CASCADE"); err != nil {
			t.Logf("drop scratch schema: %v", err)
		}
	}
	drop()
	t.Cleanup(drop)

	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire scratch connection: %v", err)
	}
	t.Cleanup(conn.Release)

	statements := []string{
		"CREATE SCHEMA " + fu101607ScratchSchema,
		"SET search_path TO " + fu101607ScratchSchema,
		`CREATE TABLE mutation_briefs (
			id BIGSERIAL PRIMARY KEY,
			window_id TEXT NOT NULL,
			data_json TEXT NOT NULL,
			created_at TEXT NOT NULL)`,
		"CREATE INDEX idx_mutation_briefs_window_id ON mutation_briefs (window_id)",
	}
	days := []string{"01", "02", "03"}
	for i := 0; i < duplicates; i++ {
		statements = append(statements, `INSERT INTO mutation_briefs (window_id, data_json, created_at)
			VALUES ('`+fu101607WindowID+`', '{"mutation_type":"pre-000029"}', '2026-01-`+days[i]+`T00:00:00Z')`)
	}
	for _, stmt := range statements {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			t.Fatalf("scratch setup (%s): %v", strings.SplitN(stmt, "\n", 2)[0], err)
		}
	}
	return conn
}

// fu101607ScratchRows counts the rows of the scratch table from ANOTHER
// connection, so the count is independent of whatever the migration statement
// did to its own connection.
func fu101607ScratchRows(t *testing.T) int {
	t.Helper()
	pool := connectTestPG(t)
	var rows int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM `+fu101607ScratchSchema+`.mutation_briefs`).Scan(&rows); err != nil {
		t.Fatalf("count scratch rows: %v", err)
	}
	return rows
}

// TestMigration000029RefusesDuplicatesWithoutDeletingThem pins the data-safety
// decision: the unique index cannot be built over duplicates, the migration must
// say so, and it must leave the rows alone.
func TestMigration000029RefusesDuplicatesWithoutDeletingThem(t *testing.T) {
	conn := fu101607ScratchSetup(t, 2)
	ctx := context.Background()

	if rows := fu101607ScratchRows(t); rows != 2 {
		t.Fatalf("scratch precondition: rows = %d, want 2 duplicated rows", rows)
	}

	_, err := conn.Exec(ctx, fu101607MigrationStatement(t))
	if err == nil {
		t.Fatal("applying 000029 to a table with duplicate window_id rows returned nil error, want a loud failure")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("migration error = %v, want a PostgreSQL error", err)
	}
	if pgErr.Code != "23505" { // unique_violation
		t.Fatalf("migration SQLSTATE = %s (%s), want 23505 unique_violation named for the duplicated key", pgErr.Code, pgErr.Message)
	}
	t.Logf("migration refused the duplicates: %s / %s", pgErr.Message, pgErr.Detail)

	if rows := fu101607ScratchRows(t); rows != 2 {
		t.Fatalf("scratch rows after the refused migration = %d, want 2 (the migration must not delete data)", rows)
	}
}

// TestMigration000029AppliesToACleanTable is the positive control for the test
// above: the same statement succeeds when nothing is duplicated, and the index
// it leaves behind is unique and usable as an ON CONFLICT target.
func TestMigration000029AppliesToACleanTable(t *testing.T) {
	conn := fu101607ScratchSetup(t, 1)
	ctx := context.Background()

	if _, err := conn.Exec(ctx, fu101607MigrationStatement(t)); err != nil {
		t.Fatalf("applying 000029 to a clean table: %v", err)
	}
	// ON CONFLICT (window_id) is only valid when a unique index covers the
	// column — this statement is the upsert the store issues.
	var payload string
	if err := conn.QueryRow(ctx, `
		INSERT INTO mutation_briefs (window_id, data_json, created_at)
		VALUES ($1, '{"mutation_type":"upserted"}', '2026-02-01T00:00:00Z')
		ON CONFLICT (window_id) DO UPDATE SET data_json = excluded.data_json,
			created_at = excluded.created_at
		RETURNING data_json`, fu101607WindowID).Scan(&payload); err != nil {
		t.Fatalf("upsert into the migrated scratch table: %v", err)
	}
	var brief domain.MutationBrief
	if err := json.Unmarshal([]byte(payload), &brief); err != nil {
		t.Fatalf("upserted payload must parse: %v", err)
	}
	if brief.MutationType != "upserted" {
		t.Fatalf("upserted payload = %+v, want the refreshed value", brief)
	}
}

func TestWindowStorePostgresMutationBriefUpsertReplacesInsteadOfAccumulating(t *testing.T) {
	pool := connectTestPG(t)
	q2b2wCleanPG(t, pool)
	ctx := context.Background()

	store := q2b2wPostgresWindowStore(t, pool, t.TempDir())

	first := domain.MutationBrief{
		ContractVersion: 1,
		WindowID:        fu101607WindowID,
		TargetAgentID:   "agent-first",
		MutationType:    "prompt_tightening",
		GeneratedAt:     time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC),
	}
	second := first
	second.TargetAgentID = "agent-second"
	second.MutationType = "threshold_shift"

	if err := store.RecordMutationBrief(fu101607WindowID, first); err != nil {
		t.Fatalf("first RecordMutationBrief: %v", err)
	}
	// Before 000029 this second write appended a row (the column had no unique
	// key); with the upsert it must refresh the existing one.
	if err := store.RecordMutationBrief(fu101607WindowID, second); err != nil {
		t.Fatalf("second RecordMutationBrief: %v", err)
	}

	if got := q2b2wRowCount(t, pool, "mutation_briefs", fu101607WindowID); got != 1 {
		t.Fatalf("mutation_briefs rows = %d, want 1 (the second write must replace)", got)
	}

	var indexCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM pg_indexes WHERE tablename = 'mutation_briefs' AND indexname = 'ux_mutation_briefs_window_id'`,
	).Scan(&indexCount); err != nil {
		t.Fatalf("read pg_indexes: %v", err)
	}
	if indexCount != 1 {
		t.Fatalf("ux_mutation_briefs_window_id present %d times, want 1 (migration 000029 must be applied)", indexCount)
	}

	var payload string
	if err := pool.QueryRow(ctx,
		`SELECT data_json FROM mutation_briefs WHERE window_id = $1`, fu101607WindowID).Scan(&payload); err != nil {
		t.Fatalf("read stored brief: %v", err)
	}
	var got domain.MutationBrief
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("stored brief must parse: %v", err)
	}
	if got.MutationType != second.MutationType || got.TargetAgentID != second.TargetAgentID {
		t.Fatalf("stored brief = %+v, want the second write %+v", got, second)
	}

	// And the schema itself refuses a second row for the window, so no caller
	// can re-create the accumulation this change removes.
	if _, err := pool.Exec(ctx,
		`INSERT INTO mutation_briefs (window_id, data_json, created_at) VALUES ($1, '{}', '2026-01-01T00:00:00Z')`,
		fu101607WindowID); err == nil {
		t.Fatal("raw duplicate INSERT returned nil error, want a unique violation (window_id is UNIQUE since 000029)")
	}
	if got := q2b2wRowCount(t, pool, "mutation_briefs", fu101607WindowID); got != 1 {
		t.Fatalf("rows after the rejected duplicate = %d, want 1", got)
	}
}
