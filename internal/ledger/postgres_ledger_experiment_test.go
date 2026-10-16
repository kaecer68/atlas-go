//go:build integration

package ledger

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// cleanupExperimentTestRows removes pgsqltest- prefixed rows from the five
// experiment/backtest/spawn tables so tests stay isolated.
func cleanupExperimentTestRows(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	cleanups := []string{
		"DELETE FROM experiments WHERE experiment_id LIKE 'pgsqltest-%'",
		"DELETE FROM prompt_experiment_results WHERE experiment_id LIKE 'pgsqltest-%'",
		"DELETE FROM window_summaries WHERE window_id LIKE 'pgsqltest-%'",
		"DELETE FROM mutation_briefs WHERE window_id LIKE 'pgsqltest-%'",
		"DELETE FROM spawn_records WHERE data_json LIKE '%pgsqltest-%'",
	}
	run := func() {
		ctx := context.Background()
		for _, q := range cleanups {
			_, _ = pool.Exec(ctx, q)
		}
	}
	run()
	t.Cleanup(run)
}

func TestPostgresLedgerStore_ExperimentRoundTrip(t *testing.T) {
	pool := connectTestPG(t)
	cleanupExperimentTestRows(t, pool)
	store := NewPostgresLedgerStore(pool)

	rec := domain.ExperimentRecord{
		ID:            "pgsqltest-exp-1",
		TargetAgentID: "pgsqltest-agent",
		MutationType:  "prompt_tweak",
		WindowStart:   time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		WindowEnd:     time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
		Status:        domain.ExperimentAccepted,
	}
	if err := store.RecordExperiment(rec); err != nil {
		t.Fatalf("RecordExperiment: %v", err)
	}
	// Session-scoped variant (distinct experiment_id — the column is UNIQUE).
	sessRec := rec
	sessRec.ID = "pgsqltest-exp-1s"
	if err := store.RecordSessionExperiment(domain.ReplaySession{ID: "pgsqltest-sess-1"}, sessRec); err != nil {
		t.Fatalf("RecordSessionExperiment: %v", err)
	}

	records, err := store.LoadExperiments()
	if err != nil {
		t.Fatalf("LoadExperiments: %v", err)
	}
	found := false
	for _, r := range records {
		if r.ID == "pgsqltest-exp-1" {
			found = true
			if r.Status != domain.ExperimentAccepted || r.MutationType != "prompt_tweak" {
				t.Fatalf("experiment mismatch: %+v", r)
			}
		}
	}
	if !found {
		t.Fatalf("pgsqltest-exp-1 not found")
	}
}

func TestPostgresLedgerStore_PromptResultUpsert(t *testing.T) {
	pool := connectTestPG(t)
	cleanupExperimentTestRows(t, pool)
	store := NewPostgresLedgerStore(pool)

	result := domain.PromptExperimentResult{
		Experiment:      domain.ExperimentRecord{ID: "pgsqltest-exp-2"},
		CandidatePrompt: "v1",
		EvaluationMode:  "replay",
	}
	if err := store.RecordPromptExperimentResult("pgsqltest-exp-2", result); err != nil {
		t.Fatalf("RecordPromptExperimentResult: %v", err)
	}
	result.CandidatePrompt = "v2"
	if err := store.UpdatePromptExperimentResult("pgsqltest-exp-2", result); err != nil {
		t.Fatalf("UpdatePromptExperimentResult: %v", err)
	}

	// Verify via direct read: the row must reflect v2 (upsert), one row total.
	ctx := context.Background()
	var data, createdAt string
	err := pool.QueryRow(ctx, `
		SELECT data_json, created_at FROM prompt_experiment_results WHERE experiment_id = $1
	`, "pgsqltest-exp-2").Scan(&data, &createdAt)
	if err != nil {
		t.Fatalf("query prompt result: %v", err)
	}
	if createdAt == "" {
		t.Fatalf("created_at empty")
	}
	var got domain.PromptExperimentResult
	if err := json.Unmarshal([]byte(data), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.CandidatePrompt != "v2" {
		t.Fatalf("expected v2 after upsert, got %q", got.CandidatePrompt)
	}
}

func TestPostgresLedgerStore_BacktestAndSpawnRoundTrip(t *testing.T) {
	pool := connectTestPG(t)
	cleanupExperimentTestRows(t, pool)
	store := NewPostgresLedgerStore(pool)

	summary := domain.BacktestWindowSummary{
		WindowID:     "pgsqltest-win-1",
		StartDate:    time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		EndDate:      time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
		SessionCount: 5,
	}
	if err := store.RecordWindowSummary(summary); err != nil {
		t.Fatalf("RecordWindowSummary: %v", err)
	}

	brief := domain.MutationBrief{
		ContractVersion: 1,
		TargetAgentID:   "pgsqltest-agent",
	}
	if err := store.RecordMutationBrief("pgsqltest-win-1", brief); err != nil {
		t.Fatalf("RecordMutationBrief: %v", err)
	}

	spawn := SpawnRecord{AgentID: "pgsqltest-spawn-agent", GapID: "pgsqltest-gap", FinalFate: "active"}
	if err := store.RecordSpawnRecord(spawn); err != nil {
		t.Fatalf("RecordSpawnRecord: %v", err)
	}
	spawns, err := store.LoadSpawnRecords()
	if err != nil {
		t.Fatalf("LoadSpawnRecords: %v", err)
	}
	if len(spawns) == 0 {
		t.Fatalf("expected at least 1 spawn record")
	}
}

// FU-20261016-03: production reported 894/894 experiments rows with
// session_id = NULL. The session-scoped write ran after the global write for
// the same UNIQUE experiment_id, so it raised 23505 on every attempt. These
// tests run on a real PostgreSQL (CI image timescale/timescaledb:2.14.2-pg15)
// because the upsert semantics — COALESCE(existing, proposed) with the target
// table qualified in DO UPDATE — are engine behaviour, not Go behaviour.
func TestPostgresLedgerStore_SessionExperimentBackfillsSessionID(t *testing.T) {
	pool := connectTestPG(t)
	cleanupExperimentTestRows(t, pool)
	store := NewPostgresLedgerStore(pool)
	ctx := context.Background()

	rec := domain.ExperimentRecord{
		ID:            "pgsqltest-exp-link-1",
		TargetAgentID: "pgsqltest-agent",
		MutationType:  "prompt_tweak",
		WindowStart:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		WindowEnd:     time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC),
		Status:        domain.ExperimentPlanned,
	}
	// Production order: the session-less write lands first.
	if err := store.RecordExperiment(rec); err != nil {
		t.Fatalf("RecordExperiment (first write): %v", err)
	}
	if err := store.RecordSessionExperiment(domain.ReplaySession{ID: "pgsqltest-sess-link-1"}, rec); err != nil {
		t.Fatalf("RecordSessionExperiment on an existing experiment_id must not fail: %v", err)
	}

	var sessionID *string
	if err := pool.QueryRow(ctx,
		`SELECT session_id FROM experiments WHERE experiment_id = $1`, rec.ID,
	).Scan(&sessionID); err != nil {
		t.Fatalf("select session_id: %v", err)
	}
	if sessionID == nil || *sessionID != "pgsqltest-sess-link-1" {
		t.Fatalf("session_id = %v, want pgsqltest-sess-link-1 (the link must be backfilled)", sessionID)
	}

	// Negative proof against the pre-fix statement: a bare INSERT of the same
	// experiment_id is rejected by the engine with 23505, which is exactly why
	// the old code could never attach a session to an already-written row.
	_, legacyErr := pool.Exec(ctx, `
		INSERT INTO experiments (experiment_id, session_id, mutation_brief_json, accepted, timestamp)
		VALUES ($1, $2, $3, $4, $5)
	`, rec.ID, "pgsqltest-sess-legacy", "{}", 0, rec.WindowStart.Format("2006-01-02T15:04:05Z07:00"))
	if legacyErr == nil {
		t.Fatal("the pre-fix bare INSERT must violate the UNIQUE index; the test would not catch a regression otherwise")
	}
	var pgErr *pgconn.PgError
	if !errors.As(legacyErr, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("pre-fix INSERT error = %v, want SQLSTATE 23505", legacyErr)
	}
}

// TestPostgresLedgerStore_SessionExperimentKeepsExistingSessionID pins the
// COALESCE direction: an experiment already attributed to a session must not be
// re-labelled by a later write.
func TestPostgresLedgerStore_SessionExperimentKeepsExistingSessionID(t *testing.T) {
	pool := connectTestPG(t)
	cleanupExperimentTestRows(t, pool)
	store := NewPostgresLedgerStore(pool)
	ctx := context.Background()

	rec := domain.ExperimentRecord{
		ID:            "pgsqltest-exp-link-keep",
		TargetAgentID: "pgsqltest-agent",
		MutationType:  "prompt_tweak",
		WindowStart:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		Status:        domain.ExperimentPlanned,
	}
	if err := store.RecordSessionExperiment(domain.ReplaySession{ID: "pgsqltest-sess-first"}, rec); err != nil {
		t.Fatalf("first session write: %v", err)
	}
	if err := store.RecordSessionExperiment(domain.ReplaySession{ID: "pgsqltest-sess-second"}, rec); err != nil {
		t.Fatalf("second session write must not fail: %v", err)
	}

	var sessionID *string
	if err := pool.QueryRow(ctx,
		`SELECT session_id FROM experiments WHERE experiment_id = $1`, rec.ID,
	).Scan(&sessionID); err != nil {
		t.Fatalf("select session_id: %v", err)
	}
	if sessionID == nil || *sessionID != "pgsqltest-sess-first" {
		t.Fatalf("session_id = %v, want pgsqltest-sess-first (existing value wins)", sessionID)
	}
}

// Q2 batch 2 unit A: the status transition must land in the experiments row.
//
// Before this unit RecordExperiment used ON CONFLICT DO NOTHING, so the first
// record written for an experiment_id won forever. Production therefore carried
// 894 rows with accepted <> 0 = 0 — a permanent "0% acceptance rate" produced by
// the writer, not by the pipeline. The JSONL store it mirrors is append-only
// with last-write-wins folding (LatestExperimentStatusByID), and these tests
// pin that the single-row PostgreSQL table now matches that semantics.
//
// Real PostgreSQL is required: ON CONFLICT DO UPDATE, which target row wins, and
// the UNIQUE index on experiment_id are engine behaviour.
func TestPostgresLedgerStore_ExperimentTransitionIsApplied(t *testing.T) {
	pool := connectTestPG(t)
	cleanupExperimentTestRows(t, pool)
	store := NewPostgresLedgerStore(pool)
	ctx := context.Background()

	planned := domain.ExperimentRecord{
		ID:            "pgsqltest-exp-transition",
		TargetAgentID: "pgsqltest-agent",
		MutationType:  "prompt_tightening",
		Status:        domain.ExperimentPlanned,
		WindowStart:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := store.RecordExperiment(planned); err != nil {
		t.Fatalf("RecordExperiment (planned): %v", err)
	}

	accepted := planned
	accepted.Status = domain.ExperimentAccepted
	accepted.WindowStart = time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	if err := store.RecordExperiment(accepted); err != nil {
		t.Fatalf("RecordExperiment (accepted transition): %v", err)
	}

	var acceptedFlag int
	var ts, briefJSON string
	if err := pool.QueryRow(ctx,
		`SELECT accepted, timestamp, mutation_brief_json FROM experiments WHERE experiment_id = $1`, planned.ID,
	).Scan(&acceptedFlag, &ts, &briefJSON); err != nil {
		t.Fatalf("select experiment row: %v", err)
	}
	if acceptedFlag != 1 {
		t.Fatalf("accepted = %d, want 1 (the transition must be applied)", acceptedFlag)
	}
	if want := accepted.WindowStart.Format("2006-01-02T15:04:05Z07:00"); ts != want {
		t.Fatalf("timestamp = %q, want %q (the row carries the newest record's window)", ts, want)
	}
	var roundTripped domain.ExperimentRecord
	if err := json.Unmarshal([]byte(briefJSON), &roundTripped); err != nil {
		t.Fatalf("unmarshal stored record: %v", err)
	}
	if roundTripped.Status != domain.ExperimentAccepted {
		t.Fatalf("stored record status = %s, want accepted", roundTripped.Status)
	}

	// One row, not two: the table stays a per-experiment status row.
	var rows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM experiments WHERE experiment_id = $1`, planned.ID,
	).Scan(&rows); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("experiments rows for %s = %d, want 1", planned.ID, rows)
	}
}

// TestPostgresLedgerStore_TransitionKeepsSessionID is the guard rail on the
// upsert above: session_id is owned by RecordSessionExperiment, so a
// session-less transition must not clear it (a "replace the whole row" upsert
// would silently recreate the 894/894 NULL defect of FU-20261016-03).
func TestPostgresLedgerStore_TransitionKeepsSessionID(t *testing.T) {
	pool := connectTestPG(t)
	cleanupExperimentTestRows(t, pool)
	store := NewPostgresLedgerStore(pool)
	ctx := context.Background()

	rec := domain.ExperimentRecord{
		ID:            "pgsqltest-exp-transition-session",
		TargetAgentID: "pgsqltest-agent",
		MutationType:  "prompt_tightening",
		Status:        domain.ExperimentPlanned,
		WindowStart:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := store.RecordSessionExperiment(domain.ReplaySession{ID: "pgsqltest-sess-transition"}, rec); err != nil {
		t.Fatalf("RecordSessionExperiment: %v", err)
	}

	accepted := rec
	accepted.Status = domain.ExperimentAccepted
	if err := store.RecordExperiment(accepted); err != nil {
		t.Fatalf("RecordExperiment (transition): %v", err)
	}

	var sessionID *string
	var acceptedFlag int
	if err := pool.QueryRow(ctx,
		`SELECT session_id, accepted FROM experiments WHERE experiment_id = $1`, rec.ID,
	).Scan(&sessionID, &acceptedFlag); err != nil {
		t.Fatalf("select row: %v", err)
	}
	if sessionID == nil || *sessionID != "pgsqltest-sess-transition" {
		t.Fatalf("session_id = %v, want pgsqltest-sess-transition (a transition must not clear the link)", sessionID)
	}
	if acceptedFlag != 1 {
		t.Fatalf("accepted = %d, want 1", acceptedFlag)
	}
}
