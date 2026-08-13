package ledger

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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
