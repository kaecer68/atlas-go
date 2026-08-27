//go:build integration

package liveness

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/testdb"
)

const testMigrationsPath = "../../sql/migrations"

// connectLivenessTestDB connects to PostgreSQL (DATABASE_URL only, no
// hardcoded DSN), runs migrations (including 000016_task_liveness), and
// returns the store. See testdb.Pool for the CI/local skip policy.
func connectLivenessTestDB(t *testing.T) *Store {
	t.Helper()
	pool := testdb.Pool(t, testMigrationsPath)
	return NewStore(pool)
}

func TestStoreUpsert_SecondWriteUpdatesSameRow(t *testing.T) {
	store := connectLivenessTestDB(t)
	ctx := context.Background()

	// Ensure a clean slate for this task name (unique per run).
	task := "test_liveness_upsert"
	_, _ = store.db.Exec(ctx, `DELETE FROM task_liveness WHERE task_name = $1`, task)
	t.Cleanup(func() {
		_, _ = store.db.Exec(context.Background(), `DELETE FROM task_liveness WHERE task_name = $1`, task)
	})

	// First execution: failure.
	firstErr := errors.New("upstream timeout")
	first := time.Now().UTC()
	if err := store.Record(ctx, RecordInput{TaskName: task, Err: firstErr, Duration: 2500 * time.Millisecond}); err != nil {
		t.Fatalf("Record#1: %v", err)
	}

	rows, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	count := countRows(rows, task)
	if count != 1 {
		t.Fatalf("after first write: row count = %d, want 1", count)
	}
	row := findRow(rows, task)
	if row.LastError != "upstream timeout" {
		t.Errorf("LastError = %q, want %q", row.LastError, "upstream timeout")
	}
	if row.ConsecutiveFailures != 1 {
		t.Errorf("ConsecutiveFailures = %d, want 1", row.ConsecutiveFailures)
	}
	if !row.LastSuccessAt.IsZero() {
		t.Errorf("LastSuccessAt = %v, want zero (never succeeded)", row.LastSuccessAt)
	}
	if row.LastDurationMs != 2500 {
		t.Errorf("LastDurationMs = %d, want 2500", row.LastDurationMs)
	}
	if row.LastRunAt.Before(first.Add(-time.Second)) {
		t.Errorf("LastRunAt = %v, want >= %v", row.LastRunAt, first)
	}

	// Second execution, same task: success. Must UPDATE the same row, not
	// insert a new one.
	time.Sleep(10 * time.Millisecond) // ensure last_run_at ordering is observable
	if err := store.Record(ctx, RecordInput{TaskName: task, Err: nil, Duration: 300 * time.Millisecond}); err != nil {
		t.Fatalf("Record#2: %v", err)
	}

	rows, err = store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if count := countRows(rows, task); count != 1 {
		t.Fatalf("after second write: row count = %d, want 1 (upsert must not append)", count)
	}
	row = findRow(rows, task)
	if row.LastError != "" {
		t.Errorf("LastError = %q, want empty after success", row.LastError)
	}
	if row.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0 after success", row.ConsecutiveFailures)
	}
	if row.LastSuccessAt.IsZero() {
		t.Error("LastSuccessAt must be set after success")
	}
	if row.LastDurationMs != 300 {
		t.Errorf("LastDurationMs = %d, want 300", row.LastDurationMs)
	}
	if row.LastRunAt.Before(row.LastSuccessAt.Add(-time.Second)) {
		t.Errorf("LastRunAt = %v should be close to LastSuccessAt = %v", row.LastRunAt, row.LastSuccessAt)
	}

	// Third execution: failure again — consecutive_failures must resume from 0
	// (it must NOT carry the pre-success count, and must not double-count).
	if err := store.Record(ctx, RecordInput{TaskName: task, Err: errors.New("boom again"), Duration: 0}); err != nil {
		t.Fatalf("Record#3: %v", err)
	}
	rows, _ = store.List(ctx)
	row = findRow(rows, task)
	if row.ConsecutiveFailures != 1 {
		t.Errorf("ConsecutiveFailures after fail-success-fail = %d, want 1", row.ConsecutiveFailures)
	}
	// last_success_at survives a later failure.
	if row.LastSuccessAt.IsZero() {
		t.Error("LastSuccessAt must survive a later failure")
	}
}

func TestStoreUpsert_TwoDistinctTasksTwoRows(t *testing.T) {
	store := connectLivenessTestDB(t)
	ctx := context.Background()
	taskA, taskB := "test_liveness_a", "test_liveness_b"
	_, _ = store.db.Exec(ctx, `DELETE FROM task_liveness WHERE task_name IN ($1,$2)`, taskA, taskB)
	t.Cleanup(func() {
		_, _ = store.db.Exec(context.Background(), `DELETE FROM task_liveness WHERE task_name IN ($1,$2)`, taskA, taskB)
	})

	if err := store.Record(ctx, RecordInput{TaskName: taskA}); err != nil {
		t.Fatalf("Record A: %v", err)
	}
	if err := store.Record(ctx, RecordInput{TaskName: taskB, Err: errors.New("x")}); err != nil {
		t.Fatalf("Record B: %v", err)
	}
	rows, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if countRows(rows, taskA) != 1 || countRows(rows, taskB) != 1 {
		t.Fatalf("expected one row per task, got %d/%d", countRows(rows, taskA), countRows(rows, taskB))
	}
}

func countRows(rows []Row, name string) int {
	n := 0
	for _, r := range rows {
		if r.TaskName == name {
			n++
		}
	}
	return n
}

func findRow(rows []Row, name string) Row {
	for _, r := range rows {
		if r.TaskName == name {
			return r
		}
	}
	return Row{}
}
