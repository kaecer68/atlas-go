//go:build integration

package main

// Integration test for the JSONL-side experiment-family backfill. It runs only
// when DATABASE_URL is set (testdb.Pool's skip policy) against the dev database,
// and every row it touches carries testExperimentIDPrefix, so the cleanup can
// never delete real experiment data.
//
// The hermetic suite (experiments_test.go) already pins the folding and
// classification rules; this test exists for the half that only PG can answer:
// the upsert really is idempotent, the folded latest status really overwrites a
// previously stored row, and a session_id established by the live pipeline
// really survives a NULL-writing backfill.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// cleanupExperimentTestRows deletes this suite's fixtures (both halves) before
// the run and again on cleanup. Rows outside testExperimentIDPrefix are never
// touched.
func cleanupExperimentTestRows(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	remove := func() {
		ctx := context.Background()
		for _, table := range []string{"experiments", "prompt_experiment_results"} {
			if _, err := pool.Exec(ctx,
				"DELETE FROM "+table+" WHERE experiment_id LIKE $1", testExperimentIDPrefix+"%"); err != nil {
				t.Logf("cleanup %s: %v", table, err)
			}
		}
	}
	remove()
	t.Cleanup(remove)
}

// TestMigrateExperimentsData exercises the real backfill: the first run writes
// both halves (folded experiments + result files), the mandatory second run
// proves idempotency, and a pre-seeded session_id proves the backfill never
// erases the attribution the live pipeline stored.
func TestMigrateExperimentsData(t *testing.T) {
	pool := connectTestPG(t)
	cleanupExperimentTestRows(t, pool)
	ctx := context.Background()

	idA := testExperimentIDPrefix + "a"
	idB := testExperimentIDPrefix + "b"
	stateDir := t.TempDir()
	jsonlPath := filepath.Join(stateDir, "experiments.jsonl")
	dir := filepath.Join(stateDir, "experiments")

	// planned → running → accepted for idA (the folded row must be the accepted
	// line), plus idB, plus two sources that must be dropped (a malformed line
	// and a record without an id).
	writeExperimentJSONLFixture(t, jsonlPath,
		[]domain.ExperimentRecord{
			experimentRecordFixture(idA, domain.ExperimentPlanned, "a-planned"),
			experimentRecordFixture(idA, domain.ExperimentRunning, "a-running"),
			experimentRecordFixture(idA, domain.ExperimentAccepted, "a-accepted"),
			experimentRecordFixture(idB, domain.ExperimentRunning, "b-running"),
			experimentRecordFixture("", domain.ExperimentRunning, "no-id"),
		},
		[]string{"{ this line is not json"})

	resultA := promptResultFixture(idA, domain.ExperimentAccepted)
	resultPathA := filepath.Join(dir, idA+".json")
	writeExperimentResultFixture(t, dir, idA+".json", mustJSONBytes(t, resultA))
	writeExperimentResultFixture(t, dir, idB+".json", mustJSONBytes(t, promptResultFixture(idB, domain.ExperimentRunning)))
	writeExperimentResultFixture(t, dir, testExperimentIDPrefix+"corrupt.json", []byte("{ truncated"))
	writeExperimentResultFixture(t, dir, idA+".json.tmp", mustJSONBytes(t, promptResultFixture(idA, domain.ExperimentRunning)))
	writeExperimentResultFixture(t, dir, "notes.txt", []byte("not a result"))

	// Deterministic created_at: the backfill stores the file mtime, so pin it
	// instead of racing the filesystem clock.
	mtime := time.Date(2026, 5, 3, 7, 8, 9, 0, time.UTC)
	if err := os.Chtimes(resultPathA, mtime, mtime); err != nil {
		t.Fatalf("chtimes fixture: %v", err)
	}

	// Pre-seed the experiments row the way the live pipeline would have left it:
	// an older folded state (planned) that this backfill must refresh, and a
	// session_id that it must not erase.
	const seededSession = "session-20260401-daily"
	seedRow, err := experimentRowValues(experimentRecordFixture(idA, domain.ExperimentPlanned, "seeded"))
	if err != nil {
		t.Fatalf("build seed row: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO experiments (experiment_id, session_id, mutation_brief_json, accepted, timestamp)
		VALUES ($1, $2, $3, $4, $5)`,
		seedRow.ExperimentID, seededSession, seedRow.MutationBriefJSON, seedRow.Accepted, seedRow.Timestamp); err != nil {
		t.Fatalf("seed experiments row: %v", err)
	}

	if err := migrateExperimentsData(ctx, pool, dir, jsonlPath); err != nil {
		t.Fatalf("migrateExperimentsData: %v", err)
	}

	// One row per id: 2 (the malformed line and the id-less record add nothing).
	var experimentRows int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM experiments WHERE experiment_id LIKE $1", testExperimentIDPrefix+"%").Scan(&experimentRows); err != nil {
		t.Fatalf("count experiments: %v", err)
	}
	if experimentRows != 2 {
		t.Fatalf("experiments rows = %d, want 2", experimentRows)
	}

	// The folded state must be the LAST line, and the seeded session_id must
	// survive (COALESCE keeps the stored value).
	var (
		briefJSON string
		accepted  int
		sessionID *string
	)
	if err := pool.QueryRow(ctx,
		`SELECT mutation_brief_json, accepted, session_id FROM experiments WHERE experiment_id = $1`,
		idA).Scan(&briefJSON, &accepted, &sessionID); err != nil {
		t.Fatalf("read experiments row: %v", err)
	}
	if accepted != 1 {
		t.Fatalf("accepted = %d for the folded accepted record, want 1", accepted)
	}
	var gotRecord domain.ExperimentRecord
	if err := json.Unmarshal([]byte(briefJSON), &gotRecord); err != nil {
		t.Fatalf("mutation_brief_json must parse: %v", err)
	}
	if gotRecord.Status != domain.ExperimentAccepted || gotRecord.RevertReason != "a-accepted" {
		t.Fatalf("stored record = %s/%s, want the last jsonl line (accepted/a-accepted)",
			gotRecord.Status, gotRecord.RevertReason)
	}
	if sessionID == nil || *sessionID != seededSession {
		t.Fatalf("session_id = %v, want the pre-existing %q to be preserved", sessionID, seededSession)
	}

	// Result half: 2 files ingested, corrupt/leftover/foreign ones dropped.
	var resultRows int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM prompt_experiment_results WHERE experiment_id LIKE $1", testExperimentIDPrefix+"%").Scan(&resultRows); err != nil {
		t.Fatalf("count prompt experiment results: %v", err)
	}
	if resultRows != 2 {
		t.Fatalf("prompt_experiment_results rows = %d, want 2", resultRows)
	}

	var (
		storedData string
		createdAt  string
	)
	if err := pool.QueryRow(ctx,
		`SELECT data_json, created_at FROM prompt_experiment_results WHERE experiment_id = $1`,
		idA).Scan(&storedData, &createdAt); err != nil {
		t.Fatalf("read result row: %v", err)
	}
	var gotResult domain.PromptExperimentResult
	if err := json.Unmarshal([]byte(storedData), &gotResult); err != nil {
		t.Fatalf("data_json must parse: %v", err)
	}
	if gotResult.Experiment.ID != idA || gotResult.CandidatePrompt != resultA.CandidatePrompt {
		t.Fatalf("stored result round-trip mismatch: %+v", gotResult.Experiment)
	}
	if want := mtime.Format(time.RFC3339); createdAt != want {
		t.Fatalf("created_at = %q, want the file mtime %q", createdAt, want)
	}

	// Idempotency: a second run must not duplicate and must not restamp.
	var firstCreatedAt string
	if err := migrateExperimentsData(ctx, pool, dir, jsonlPath); err != nil {
		t.Fatalf("second migrateExperimentsData: %v", err)
	}
	var experimentRowsAfter, resultRowsAfter int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM experiments WHERE experiment_id LIKE $1", testExperimentIDPrefix+"%").Scan(&experimentRowsAfter); err != nil {
		t.Fatalf("count experiments after re-run: %v", err)
	}
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM prompt_experiment_results WHERE experiment_id LIKE $1", testExperimentIDPrefix+"%").Scan(&resultRowsAfter); err != nil {
		t.Fatalf("count results after re-run: %v", err)
	}
	if experimentRowsAfter != experimentRows || resultRowsAfter != resultRows {
		t.Fatalf("re-run changed row counts: experiments %d→%d, results %d→%d (must be idempotent)",
			experimentRows, experimentRowsAfter, resultRows, resultRowsAfter)
	}
	if err := pool.QueryRow(ctx,
		`SELECT created_at FROM prompt_experiment_results WHERE experiment_id = $1`, idA).Scan(&firstCreatedAt); err != nil {
		t.Fatalf("read created_at after re-run: %v", err)
	}
	if firstCreatedAt != createdAt {
		t.Fatalf("created_at changed on re-run: %q → %q", createdAt, firstCreatedAt)
	}
}
