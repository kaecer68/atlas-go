package userstate

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func ackState(userID int64, signalKey string, ack bool, dismissed bool) UserSignalState {
	now := time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)
	s := UserSignalState{
		UserID:    userID,
		SignalKey: signalKey,
		Dismissed: dismissed,
		UpdatedAt: now,
	}
	if ack {
		t := now
		s.AcknowledgedAt = &t
	}
	return s
}

func TestJSONLStore_UpsertInsertsNewRecord(t *testing.T) {
	store := NewJSONLStore(t.TempDir())
	state := ackState(42, "foreign-3day-inflow", true, false)

	if err := store.Upsert(state); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	got, err := store.LoadByUserAndSignal(42, "foreign-3day-inflow")
	if err != nil {
		t.Fatalf("LoadByUserAndSignal: %v", err)
	}
	if got.UserID != 42 || got.SignalKey != "foreign-3day-inflow" {
		t.Errorf("got %+v, want UserID=42 SignalKey=foreign-3day-inflow", got)
	}
	if got.AcknowledgedAt == nil {
		t.Errorf("AcknowledgedAt = nil, want set")
	}
	if got.Dismissed {
		t.Errorf("Dismissed = true, want false")
	}
}

func TestJSONLStore_UpsertReplacesExistingRecord(t *testing.T) {
	store := NewJSONLStore(t.TempDir())
	if err := store.Upsert(ackState(42, "foreign-3day-inflow", false, false)); err != nil {
		t.Fatalf("Upsert #1: %v", err)
	}
	// Re-acknowledge: same tuple, new state.
	if err := store.Upsert(ackState(42, "foreign-3day-inflow", true, true)); err != nil {
		t.Fatalf("Upsert #2: %v", err)
	}
	records, err := store.LoadByUser(42)
	if err != nil {
		t.Fatalf("LoadByUser: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record (upsert replaced), got %d", len(records))
	}
	if records[0].AcknowledgedAt == nil {
		t.Error("AcknowledgedAt = nil after re-ack, want set")
	}
	if !records[0].Dismissed {
		t.Error("Dismissed = false after re-ack with dismissed=true, want true")
	}
}

func TestJSONLStore_LoadByUserFiltersByUser(t *testing.T) {
	store := NewJSONLStore(t.TempDir())
	_ = store.Upsert(ackState(1, "sig-a", false, false))
	_ = store.Upsert(ackState(1, "sig-b", false, false))
	_ = store.Upsert(ackState(2, "sig-c", false, false))

	got1, _ := store.LoadByUser(1)
	if len(got1) != 2 {
		t.Errorf("user 1: expected 2 records, got %d", len(got1))
	}
	got2, _ := store.LoadByUser(2)
	if len(got2) != 1 {
		t.Errorf("user 2: expected 1 record, got %d", len(got2))
	}
	got3, _ := store.LoadByUser(999)
	if len(got3) != 0 {
		t.Errorf("user 999: expected 0 records, got %d", len(got3))
	}
}

func TestJSONLStore_LoadByUserAndSignalNotFound(t *testing.T) {
	store := NewJSONLStore(t.TempDir())
	_, err := store.LoadByUserAndSignal(42, "missing")
	if !errors.Is(err, ErrSignalStateNotFound) {
		t.Fatalf("err = %v, want ErrSignalStateNotFound", err)
	}
}

func TestJSONLStore_PersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	s1 := NewJSONLStore(dir)
	if err := s1.Upsert(ackState(7, "sig-x", true, false)); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	s2 := NewJSONLStore(dir)
	got, err := s2.LoadByUserAndSignal(7, "sig-x")
	if err != nil {
		t.Fatalf("Load on fresh instance: %v", err)
	}
	if got.AcknowledgedAt == nil {
		t.Error("AcknowledgedAt = nil after reload, want set")
	}
}

func TestJSONLStore_RespectsFIFOCap(t *testing.T) {
	dir := t.TempDir()
	store := NewJSONLStoreWithCap(dir, 3)
	// 5 distinct (user, signal) tuples → oldest 2 evicted.
	for i := range 5 {
		_ = store.Upsert(ackState(int64(i), "sig", false, false))
	}
	records, _ := store.LoadByUser(0)
	if len(records) != 0 {
		t.Errorf("user 0 (oldest): expected 0 (evicted), got %d", len(records))
	}
	records4, _ := store.LoadByUser(4)
	if len(records4) != 1 {
		t.Errorf("user 4 (newest): expected 1, got %d", len(records4))
	}
}

func TestJSONLStore_LoadOnEmptyFile(t *testing.T) {
	store := NewJSONLStore(t.TempDir())
	// No file yet — must return empty slice (not error).
	got, err := store.LoadByUser(42)
	if err != nil {
		t.Fatalf("LoadByUser on empty store: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 records, got %d", len(got))
	}
	_, err = store.LoadByUserAndSignal(42, "anything")
	if !errors.Is(err, ErrSignalStateNotFound) {
		t.Errorf("err = %v, want ErrSignalStateNotFound", err)
	}
}

func TestJSONLStore_JSONLPath(t *testing.T) {
	dir := t.TempDir()
	store := NewJSONLStore(dir)
	// upsert one record, verify file lands at <dir>/user_signals.jsonl
	if err := store.Upsert(ackState(1, "x", false, false)); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	want := filepath.Join(dir, "user_signals.jsonl")
	if _, err := openAndClose(want); err != nil {
		t.Errorf("file not found at %s: %v", want, err)
	}
}

// TestJSONLStore_ConcurrentUpsertsNoLoss mirrors the same guard as
// event_flow_prediction_store (traps.md "JSONL ledger append must hold
// the Mutex"): 5 goroutines concurrently upsert distinct (user, signal)
// tuples. Verifies the read-modify-write under sync.Mutex produces
// exactly the expected record count with no race-related loss. Run with
// -race to catch lock-not-held regressions.
func TestJSONLStore_ConcurrentUpsertsNoLoss(t *testing.T) {
	store := NewJSONLStore(t.TempDir())
	const goroutines = 5
	const perGoroutine = 50
	// Each goroutine owns a distinct (user, signal) tuple so the test
	// exercises concurrent Upsert on the same file without depending on
	// the replace-in-place branch.
	var ready sync.WaitGroup
	ready.Add(goroutines)
	start := make(chan struct{})
	var failures atomic.Int32
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func(g int) {
			defer wg.Done()
			ready.Done()
			<-start
			for range perGoroutine {
				if err := store.Upsert(ackState(int64(g+1), "sig", false, false)); err != nil {
					failures.Add(1)
				}
			}
		}(g)
	}
	ready.Wait()
	close(start)
	wg.Wait()
	if failures.Load() != 0 {
		t.Fatalf("expected 0 upsert failures, got %d", failures.Load())
	}
	// 5 distinct users × 1 signal each = 5 records (each goroutine
	// overwrote the same tuple 50 times — the final record per tuple is
	// the survivor).
	for u := int64(1); u <= int64(goroutines); u++ {
		rs, _ := store.LoadByUser(u)
		if len(rs) != 1 {
			t.Errorf("user %d: expected 1 record, got %d", u, len(rs))
		}
	}
}

// openAndClose verifies the file exists (read-only).
func openAndClose(p string) (struct{}, error) {
	f, err := os.Open(p)
	if err != nil {
		return struct{}{}, err
	}
	_ = f.Close()
	return struct{}{}, nil
}
