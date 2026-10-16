//go:build integration

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// cleanupWindowTestRows deletes this suite's fixtures — window_id values
// carrying testWindowIDPrefix (window-q2b2wbf-) — from both window tables,
// before the run and again on cleanup. Real window rows are never touched.
func cleanupWindowTestRows(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	remove := func() {
		ctx := context.Background()
		for _, table := range []string{"window_summaries", "mutation_briefs"} {
			if _, err := pool.Exec(ctx,
				"DELETE FROM "+table+" WHERE window_id LIKE $1", testWindowIDPrefix+"%"); err != nil {
				t.Logf("cleanup %s: %v", table, err)
			}
		}
	}
	remove()
	t.Cleanup(remove)
}

// windowRowCount counts this suite's fixture rows in one window table.
func windowRowCount(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM "+table+" WHERE window_id LIKE $1", testWindowIDPrefix+"%").Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func writeWindowFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
}

// TestMigrateWindowsData exercises the real backfill against PG (dev
// DATABASE_URL per testdb policy): the first run writes both halves of the
// window family, the mandatory second run proves idempotency (both halves are
// upserts), and the schema — not a delete-first step — is what keeps re-runs
// clean: since migration 000029 mutation_briefs.window_id is UNIQUE, so a raw
// duplicate INSERT for a window is rejected outright.
func TestMigrateWindowsData(t *testing.T) {
	pool := connectTestPG(t)
	cleanupWindowTestRows(t, pool)
	ctx := context.Background()

	summaryA := windowSummaryFixture(testWindowIDPrefix + "a")
	summaryB := windowSummaryFixture(testWindowIDPrefix + "b")
	briefA := mutationBriefFixture(summaryA.WindowID)

	dir := t.TempDir()
	writeWindowFixture(t, dir, summaryA.WindowID+".json", mustJSON(t, summaryA))
	writeWindowFixture(t, dir, summaryB.WindowID+".json", mustJSON(t, summaryB))
	writeWindowFixture(t, dir, summaryA.WindowID+"-mutation-brief.json", mustJSON(t, briefA))
	writeWindowFixture(t, dir, testWindowIDPrefix+"corrupt.json", "{ truncated")
	writeWindowFixture(t, dir, "notes.txt", "not a window artifact")

	if err := migrateWindowsData(ctx, pool, dir); err != nil {
		t.Fatalf("migrateWindowsData: %v", err)
	}

	// 2 summaries: the corrupt file and notes.txt contribute nothing, and the
	// brief file is NOT counted as a summary (it would be, if the classifier
	// looked at the JSON instead of the file name).
	if got := windowRowCount(t, pool, "window_summaries"); got != 2 {
		t.Fatalf("window_summaries rows = %d, want 2", got)
	}
	if got := windowRowCount(t, pool, "mutation_briefs"); got != 1 {
		t.Fatalf("mutation_briefs rows = %d, want 1", got)
	}

	// data_json must be the full domain object, readable by the store readers.
	var summaryJSON string
	if err := pool.QueryRow(ctx,
		`SELECT data_json FROM window_summaries WHERE window_id = $1`, summaryA.WindowID).Scan(&summaryJSON); err != nil {
		t.Fatalf("read stored summary: %v", err)
	}
	var gotSummary domain.BacktestWindowSummary
	if err := json.Unmarshal([]byte(summaryJSON), &gotSummary); err != nil {
		t.Fatalf("window_summaries.data_json must parse: %v", err)
	}
	if gotSummary.WindowID != summaryA.WindowID || gotSummary.SessionCount != summaryA.SessionCount ||
		gotSummary.OutcomeCount != summaryA.OutcomeCount || !gotSummary.StartDate.Equal(summaryA.StartDate) ||
		!gotSummary.EndDate.Equal(summaryA.EndDate) {
		t.Fatalf("stored summary round-trip mismatch: got %+v want %+v", gotSummary, summaryA)
	}

	var briefJSON string
	if err := pool.QueryRow(ctx,
		`SELECT data_json FROM mutation_briefs WHERE window_id = $1`, summaryA.WindowID).Scan(&briefJSON); err != nil {
		t.Fatalf("read stored brief: %v", err)
	}
	var gotBrief domain.MutationBrief
	if err := json.Unmarshal([]byte(briefJSON), &gotBrief); err != nil {
		t.Fatalf("mutation_briefs.data_json must parse: %v", err)
	}
	if gotBrief.TargetAgentID != briefA.TargetAgentID || gotBrief.MutationType != briefA.MutationType ||
		gotBrief.WindowID != briefA.WindowID {
		t.Fatalf("stored brief round-trip mismatch: got %+v want %+v", gotBrief, briefA)
	}

	// Idempotent re-run: no new rows on either side.
	if err := migrateWindowsData(ctx, pool, dir); err != nil {
		t.Fatalf("second migrateWindowsData: %v", err)
	}
	if got := windowRowCount(t, pool, "window_summaries"); got != 2 {
		t.Fatalf("window_summaries rows after re-run = %d, want 2 (upsert must not duplicate)", got)
	}
	if got := windowRowCount(t, pool, "mutation_briefs"); got != 1 {
		t.Fatalf("mutation_briefs rows after re-run = %d, want 1 (delete-first must not accumulate)", got)
	}

	// Duplicate-brief proof, inverted by migration 000029: window_id is UNIQUE
	// now, so a stray second row for the same window is REJECTED by the schema
	// instead of being tolerated and silently collapsed by a delete-first step.
	// Without 000029 this INSERT succeeds and the count below is 2.
	if _, err := pool.Exec(ctx,
		`INSERT INTO mutation_briefs (window_id, data_json, created_at) VALUES ($1, '{}', '2026-01-01T00:00:00Z')`,
		summaryA.WindowID); err == nil {
		t.Fatal("raw duplicate mutation_briefs INSERT returned nil error, want a unique-violation (window_id must be UNIQUE since 000029)")
	}
	if got := windowRowCount(t, pool, "mutation_briefs"); got != 1 {
		t.Fatalf("mutation_briefs rows after the rejected duplicate = %d, want 1", got)
	}
	if err := migrateWindowsData(ctx, pool, dir); err != nil {
		t.Fatalf("third migrateWindowsData: %v", err)
	}
	if got := windowRowCount(t, pool, "mutation_briefs"); got != 1 {
		t.Fatalf("mutation_briefs rows after replace = %d, want 1 (upsert must replace, not accumulate)", got)
	}
	if got := windowRowCount(t, pool, "window_summaries"); got != 2 {
		t.Fatalf("window_summaries rows = %d, want 2", got)
	}

	// A missing directory migrates nothing and must not fail: it is the normal
	// state of a deployment that never ran a backtest window.
	if err := migrateWindowsData(ctx, pool, filepath.Join(dir, "absent")); err != nil {
		t.Fatalf("missing windows dir must not fail the migration: %v", err)
	}
	if got := windowRowCount(t, pool, "window_summaries"); got != 2 {
		t.Fatalf("window_summaries rows = %d, want 2 after the missing-dir run", got)
	}
	if got := windowRowCount(t, pool, "mutation_briefs"); got != 1 {
		t.Fatalf("mutation_briefs rows = %d, want 1 after the missing-dir run", got)
	}
}
