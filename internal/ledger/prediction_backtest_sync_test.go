package ledger

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// setupSyncTest builds a temp-dir EventFlowPredictionStore seeded with the
// given records plus an opened SQLite-backed HistoricalStore. The prediction
// store path is <dir>/event_flow_predictions.jsonl; the historical store
// opens <dir>/atlas.db.
func setupSyncTest(t *testing.T, dir string, records []EventFlowPredictionRecord) (EventFlowPredictionStore, HistoricalStore) {
	t.Helper()
	predStore := NewJSONLEventFlowPredictionStore(dir)
	for _, r := range records {
		if err := predStore.AppendPrediction(r); err != nil {
			t.Fatalf("append prediction: %v", err)
		}
		if r.ActualSign != 0 && r.ActualCapturedAt == nil {
			// caller forgot to set CapturedAt; ignore
		}
	}
	db, err := OpenSQLiteDB(filepath.Join(dir, "atlas.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	histStore := NewSQLiteHistoricalStore(db)
	return predStore, histStore
}

func TestSyncPredictionBacktestFromEventFlow_OnlyReconciledRowsUpserted(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 8, 6, 5, 45, 0, 0, time.UTC)
	captured := time.Date(2026, 8, 7, 5, 0, 0, 0, time.UTC)

	predStore, histStore := setupSyncTest(t, dir, []EventFlowPredictionRecord{
		// Reconciled inflow: predicted +, actual + → hit
		{PredictedAt: now, DirectionSign: 0.6, Confidence: 0.6, Direction: "inflow", ActualSign: 0.5, ActualSource: "twse_t86", ActualCapturedAt: &captured},
		// Reconciled outflow miss: predicted -, actual + → miss
		{PredictedAt: now.AddDate(0, 0, -1), DirectionSign: -0.4, Confidence: 0.4, Direction: "outflow", ActualSign: 0.3, ActualSource: "twse_t86", ActualCapturedAt: &captured},
		// Unreconciled (no ActualCapturedAt) → must NOT upsert
		{PredictedAt: now.AddDate(0, 0, -2), DirectionSign: 0.7, Confidence: 0.7, Direction: "inflow"},
	})

	n, err := SyncPredictionBacktestFromEventFlow(context.Background(), predStore, histStore, dir)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 upserted (reconciled only), got %d", n)
	}

	rows, err := histStore.LoadPredictionBacktestRange(context.Background(), "", "", 100)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows in prediction_backtest, got %d", len(rows))
	}
	for _, r := range rows {
		if r.IsSynthetic != 0 {
			t.Errorf("row %s: is_synthetic = %d, want 0 (production)", r.Date, r.IsSynthetic)
		}
	}
}

func TestSyncPredictionBacktestFromEventFlow_HitSemanticsMatchCalibrator(t *testing.T) {
	dir := t.TempDir()
	captured := time.Date(2026, 8, 7, 5, 0, 0, 0, time.UTC)

	predStore, histStore := setupSyncTest(t, dir, []EventFlowPredictionRecord{
		// Hit: same sign
		{PredictedAt: time.Date(2026, 8, 6, 5, 45, 0, 0, time.UTC), DirectionSign: 0.5, Confidence: 0.5, Direction: "inflow", ActualSign: 0.3, ActualSource: "twse_t86", ActualCapturedAt: &captured},
		// Miss: opposite sign
		{PredictedAt: time.Date(2026, 8, 5, 5, 45, 0, 0, time.UTC), DirectionSign: 0.5, Confidence: 0.5, Direction: "inflow", ActualSign: -0.4, ActualSource: "twse_t86", ActualCapturedAt: &captured},
	})

	if _, err := SyncPredictionBacktestFromEventFlow(context.Background(), predStore, histStore, dir); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	rows, err := histStore.LoadPredictionBacktestRangeAll(context.Background(), "", "", 100)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// Index by date for assertion.
	byDate := map[string]PredictionBacktestRow{}
	for _, r := range rows {
		byDate[r.Date] = r
	}
	if !byDate["2026-08-06"].Hit {
		t.Errorf("2026-08-06: Hit = false, want true (same sign)")
	}
	if byDate["2026-08-05"].Hit {
		t.Errorf("2026-08-05: Hit = true, want false (opposite sign)")
	}
	if byDate["2026-08-06"].IsSynthetic != 0 || byDate["2026-08-05"].IsSynthetic != 0 {
		t.Errorf("is_synthetic must be 0 for production reverse-write")
	}
}

func TestSyncPredictionBacktestFromEventFlow_IdempotentRerun(t *testing.T) {
	dir := t.TempDir()
	captured := time.Date(2026, 8, 7, 5, 0, 0, 0, time.UTC)
	predStore, histStore := setupSyncTest(t, dir, []EventFlowPredictionRecord{
		{PredictedAt: time.Date(2026, 8, 6, 5, 45, 0, 0, time.UTC), DirectionSign: 0.5, Confidence: 0.5, Direction: "inflow", ActualSign: 0.3, ActualSource: "twse_t86", ActualCapturedAt: &captured},
	})

	if _, err := SyncPredictionBacktestFromEventFlow(context.Background(), predStore, histStore, dir); err != nil {
		t.Fatalf("Sync #1: %v", err)
	}
	// Re-run — primary key (date) collision should replace, not duplicate.
	n2, err := SyncPredictionBacktestFromEventFlow(context.Background(), predStore, histStore, dir)
	if err != nil {
		t.Fatalf("Sync #2: %v", err)
	}
	if n2 != 1 {
		t.Fatalf("expected 1 re-upsert, got %d", n2)
	}
	rows, _ := histStore.LoadPredictionBacktestRangeAll(context.Background(), "", "", 100)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row after re-run, got %d", len(rows))
	}
}

func TestSyncPredictionBacktestFromEventFlow_NilStoreErrors(t *testing.T) {
	if _, err := SyncPredictionBacktestFromEventFlow(context.Background(), nil, nil, ""); err == nil {
		t.Fatal("expected error with nil stores, got nil")
	}
}
