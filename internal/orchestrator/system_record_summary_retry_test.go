package orchestrator

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// flakyRecordStore is a test double for the ledger.Store.RecordSessionSummary call.
// It fails the first N times, then succeeds, simulating a transient I/O failure
// (e.g. fsync race, OS-level disk full flicker, temp-file rename collision).
type flakyRecordStore struct {
	calls        atomic.Int32
	failuresLeft atomic.Int32
	err          error
}

func (f *flakyRecordStore) RecordSessionSummary(_ domain.ReplaySession, _ domain.SessionSummary) error {
	f.calls.Add(1)
	if f.failuresLeft.Load() > 0 {
		f.failuresLeft.Add(-1)
		return f.err
	}
	return nil
}

// TestRecordSummaryWithRetry_SucceedsAfterTransientFailures: the helper must
// retry on transient errors and return nil once the underlying call succeeds.
// This prevents orphan session directories (summary.json missing while
// recommendation_outcomes.jsonl is present) when a single fsync races with
// concurrent session activity.
func TestRecordSummaryWithRetry_SucceedsAfterTransientFailures(t *testing.T) {
	store := &flakyRecordStore{err: errors.New("transient fsync failure")}
	store.failuresLeft.Store(2) // fail twice, succeed on 3rd attempt

	err := recordSummaryWithRetry(store, domain.ReplaySession{}, domain.SessionSummary{}, 3, 0)
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if got := store.calls.Load(); got != 3 {
		t.Errorf("expected 3 calls (2 failures + 1 success), got %d", got)
	}
}

// TestRecordSummaryWithRetry_GivesUpAfterMaxAttempts: persistent failures must
// surface the last error to the caller so the surrounding handler can decide
// whether to abort the run. We do NOT swallow the error — silent failure is
// exactly the bug class we are guarding against.
func TestRecordSummaryWithRetry_GivesUpAfterMaxAttempts(t *testing.T) {
	store := &flakyRecordStore{err: errors.New("persistent disk full")}
	store.failuresLeft.Store(10)

	err := recordSummaryWithRetry(store, domain.ReplaySession{}, domain.SessionSummary{}, 3, 0)
	if err == nil {
		t.Fatal("expected error after exhausting retries, got nil")
	}
	if got := store.calls.Load(); got != 3 {
		t.Errorf("expected 3 calls (1 initial + 2 retries), got %d", got)
	}
}

// TestRecordSummaryWithRetry_NoBackoffOnLastAttempt: the helper must not sleep
// after the final attempt — otherwise the caller observes artificially long
// failure latency with no chance of recovery.
func TestRecordSummaryWithRetry_NoBackoffOnLastAttempt(t *testing.T) {
	store := &flakyRecordStore{err: errors.New("never recovers")}
	store.failuresLeft.Store(5)

	start := time.Now()
	_ = recordSummaryWithRetry(store, domain.ReplaySession{}, domain.SessionSummary{}, 3, 50*time.Millisecond)
	elapsed := time.Since(start)

	// 3 attempts with backoff after attempt 1 and 2: 50ms + 100ms = 150ms minimum.
	// If we erroneously slept after attempt 3, total would be 50+100+150 = 300ms.
	if elapsed >= 200*time.Millisecond {
		t.Errorf("backoff likely applied after final attempt; elapsed=%v", elapsed)
	}
}
