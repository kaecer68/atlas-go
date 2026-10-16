//go:build integration

package ledger

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eval"
)

// Q2 batch 2 unit A — the experiments-family store against a REAL PostgreSQL.
//
// The unit tests cover the wiring (which backend is chosen, what is mirrored);
// this file covers the part that is engine behaviour: that a postgres
// deployment's family store really lands the lifecycle in the experiments table
// AND keeps the result file the path-based readers still consume, and that the
// DSR gate's type assertion still finds the trial loader through the wrapper.
//
// CI runs this with the timescale/timescaledb:2.14.2-pg15 service image; the
// sql migration test helper applies sql/migrations first.
func TestNewExperimentFamilyStore_PostgresRoundTrip(t *testing.T) {
	pool := connectTestPG(t)
	cleanupExperimentTestRows(t, pool)

	saved := postgresPool
	SetPostgresPool(pool)
	t.Cleanup(func() { SetPostgresPool(saved) })

	ledgerDir := t.TempDir()
	store, err := NewExperimentFamilyStore(config.Config{
		StoreBackend: "postgres",
		LedgerDir:    ledgerDir,
	})
	if err != nil {
		t.Fatalf("NewExperimentFamilyStore(postgres): %v", err)
	}

	const expID = "pgsqltest-family-1"
	planned := domain.ExperimentRecord{
		ID:            expID,
		TargetAgentID: "pgsqltest-agent",
		MutationType:  "prompt_tightening",
		Status:        domain.ExperimentPlanned,
		WindowStart:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := store.RecordExperiment(planned); err != nil {
		t.Fatalf("RecordExperiment (planned): %v", err)
	}

	// The result blob must reach BOTH halves: PG (the DSR gate's trial history)
	// and <LedgerDir>/experiments/<id>.json (the file contract).
	result := domain.PromptExperimentResult{
		Experiment:      domain.ExperimentRecord{ID: expID, Status: domain.ExperimentRunning},
		CandidatePrompt: "prompts/experiments/pgsqltest/v2.md",
		EvaluationMode:  "policy_checked_pending_replay",
		RecordedAt:      time.Now().UTC(),
		EvalMetrics:     ptrEvalMetrics(),
	}
	if err := store.RecordPromptExperimentResult(expID, result); err != nil {
		t.Fatalf("RecordPromptExperimentResult: %v", err)
	}

	ctx := context.Background()
	var data string
	if err := pool.QueryRow(ctx,
		`SELECT data_json FROM prompt_experiment_results WHERE experiment_id = $1`, expID,
	).Scan(&data); err != nil {
		t.Fatalf("prompt experiment result not in PostgreSQL: %v", err)
	}
	filePath := filepath.Join(ledgerDir, "experiments", expID+".json")
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("result file missing at %s: %v", filePath, err)
	}
	var fromFile, fromPG domain.PromptExperimentResult
	if err := json.Unmarshal(fileData, &fromFile); err != nil {
		t.Fatalf("unmarshal mirrored result: %v", err)
	}
	if err := json.Unmarshal([]byte(data), &fromPG); err != nil {
		t.Fatalf("unmarshal PG result: %v", err)
	}
	if fromFile.Experiment.ID != expID || fromPG.Experiment.ID != expID {
		t.Fatalf("result ids: file=%q pg=%q, want %q", fromFile.Experiment.ID, fromPG.Experiment.ID, expID)
	}

	// The judge writes its verdict back through the same store; both halves
	// must move together.
	result.Experiment.Status = domain.ExperimentAccepted
	result.EvaluationMode = "prompt_aware_replay_judged"
	if err := store.UpdatePromptExperimentResult(expID, result); err != nil {
		t.Fatalf("UpdatePromptExperimentResult: %v", err)
	}
	var updated string
	if err := pool.QueryRow(ctx,
		`SELECT data_json FROM prompt_experiment_results WHERE experiment_id = $1`, expID,
	).Scan(&updated); err != nil {
		t.Fatalf("reload PG result: %v", err)
	}
	var updatedFromPG domain.PromptExperimentResult
	if err := json.Unmarshal([]byte(updated), &updatedFromPG); err != nil {
		t.Fatalf("unmarshal PG result after update: %v", err)
	}
	if updatedFromPG.EvaluationMode != "prompt_aware_replay_judged" {
		t.Fatalf("PG result = %q, want the updated evaluation mode", updatedFromPG.EvaluationMode)
	}
	updatedFile, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("mirrored result disappeared after the update: %v", err)
	}
	var updatedFromFile domain.PromptExperimentResult
	if err := json.Unmarshal(updatedFile, &updatedFromFile); err != nil {
		t.Fatalf("unmarshal mirrored result after update: %v", err)
	}
	if updatedFromFile.EvaluationMode != "prompt_aware_replay_judged" {
		t.Fatalf("mirrored result = %q, want the updated evaluation mode", updatedFromFile.EvaluationMode)
	}

	// Status transition lands in the same table (the pre-fix DO NOTHING made
	// this structurally impossible).
	accepted := planned
	accepted.Status = domain.ExperimentAccepted
	if err := store.RecordExperiment(accepted); err != nil {
		t.Fatalf("RecordExperiment (accepted): %v", err)
	}
	var acceptedFlag int
	if err := pool.QueryRow(ctx,
		`SELECT accepted FROM experiments WHERE experiment_id = $1`, expID,
	).Scan(&acceptedFlag); err != nil {
		t.Fatalf("select accepted: %v", err)
	}
	if acceptedFlag != 1 {
		t.Fatalf("accepted = %d, want 1 (the transition must land through the family store)", acceptedFlag)
	}

	// The DSR gate looks the loader up with a type assertion on the store it is
	// handed. If the wrapper dropped that method the gate would silently stop
	// running, so the assertion is pinned against the real PG-backed wrapper.
	loader, ok := store.(TrialSharpeLoader)
	if !ok {
		t.Fatalf("family store %T does not satisfy TrialSharpeLoader", store)
	}
	trials, err := loader.LoadTrialSharpes()
	if err != nil {
		t.Fatalf("LoadTrialSharpes: %v", err)
	}
	found := false
	for _, trial := range trials {
		if trial.ExperimentID == expID {
			found = true
		}
	}
	if !found {
		t.Fatalf("trial %s not visible through the wrapper (got %d trials)", expID, len(trials))
	}
}

// ptrEvalMetrics returns a minimal EvalResult so the blob is eligible for the
// DSR gate's trial history (trialSharpeFromResult skips blobs without metrics).
func ptrEvalMetrics() *eval.EvalResult {
	return &eval.EvalResult{Sharpe: 1.25, CumReturn: 0.1, MaxDD: 0.02}
}
