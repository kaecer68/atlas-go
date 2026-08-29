package anomaly

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// Test_MemoryStore_implements_AnomalyStore is a compile-time check that the
// concrete store satisfies the AnomalyStore contract. If the interface
// changes, the build fails here rather than at the wiring site.
func Test_MemoryStore_implements_AnomalyStore(t *testing.T) {
	var _ AnomalyStore = (*MemoryStore)(nil)
}

// Test_MemoryStore_Save_and_Get verifies the round-trip of an anomaly:
// Save assigns an AnomalyID, Get returns the same record.
func Test_MemoryStore_Save_and_Get(t *testing.T) {
	s := NewMemoryStore(100)
	ev := AnomalyEvent{
		TenantID:    "tenant-a",
		AnomalyType: "burst",
		Score:       4.2,
		TS:          "2026-07-01T12:00:00Z",
	}
	sa, err := s.Save(ev)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if sa.AnomalyID == "" {
		t.Error("expected AnomalyID to be assigned")
	}
	if sa.Acked {
		t.Error("new anomaly must not be pre-acked")
	}

	got, ok := s.Get(sa.AnomalyID)
	if !ok {
		t.Fatalf("Get(%q) not found", sa.AnomalyID)
	}
	if got.AnomalyID != sa.AnomalyID {
		t.Errorf("AnomalyID mismatch: %q vs %q", got.AnomalyID, sa.AnomalyID)
	}
	if got.Event.TenantID != "tenant-a" {
		t.Errorf("TenantID lost: %q", got.Event.TenantID)
	}
	if got.Event.AnomalyType != "burst" {
		t.Errorf("AnomalyType lost: %q", got.Event.AnomalyType)
	}
	if got.Event.Score != 4.2 {
		t.Errorf("Score lost: %v", got.Event.Score)
	}
}

// Test_MemoryStore_Save_assigns_unique_ids verifies that successive Save
// calls generate distinct AnomalyIDs even when events are identical.
func Test_MemoryStore_Save_assigns_unique_ids(t *testing.T) {
	s := NewMemoryStore(100)
	ev := AnomalyEvent{TenantID: "t", AnomalyType: "burst", Score: 1.0}
	ids := map[string]struct{}{}
	for range 5 {
		sa, err := s.Save(ev)
		if err != nil {
			t.Fatalf("Save: %v", err)
		}
		if _, dup := ids[sa.AnomalyID]; dup {
			t.Fatalf("duplicate AnomalyID: %q", sa.AnomalyID)
		}
		ids[sa.AnomalyID] = struct{}{}
	}
	if len(ids) != 5 {
		t.Errorf("expected 5 unique ids, got %d", len(ids))
	}
}

// Test_MemoryStore_Get_unknown_returns_false verifies that a missing
// AnomalyID returns (zero, false) so callers can distinguish "not found"
// from "found but empty".
func Test_MemoryStore_Get_unknown_returns_false(t *testing.T) {
	s := NewMemoryStore(10)
	_, ok := s.Get("does-not-exist")
	if ok {
		t.Error("expected ok=false for missing id")
	}
}

// Test_MemoryStore_Ack_marks_acked_and_records_user verifies that Ack
// records both the user (operator/agent) and the UTC timestamp. This is the
// minimum required for audit replay.
func Test_MemoryStore_Ack_marks_acked_and_records_user(t *testing.T) {
	s := NewMemoryStore(10)
	ev := AnomalyEvent{TenantID: "t", AnomalyType: "burst", Score: 1.0}
	sa, err := s.Save(ev)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	ackTime := time.Date(2026, 7, 1, 12, 30, 0, 0, time.UTC)
	s.now = func() time.Time { return ackTime }

	if err := s.Ack(sa.AnomalyID, "operator-7"); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	got, ok := s.Get(sa.AnomalyID)
	if !ok {
		t.Fatalf("Get after Ack: not found")
	}
	if !got.Acked {
		t.Error("Acked flag not set")
	}
	if got.AckedBy != "operator-7" {
		t.Errorf("AckedBy=%q", got.AckedBy)
	}
	if !got.AckedAt.Equal(ackTime) {
		t.Errorf("AckedAt=%v want %v", got.AckedAt, ackTime)
	}
}

// Test_MemoryStore_Ack_unknown_returns_error verifies the error path for
// acking a non-existent anomaly.
func Test_MemoryStore_Ack_unknown_returns_error(t *testing.T) {
	s := NewMemoryStore(10)
	err := s.Ack("ghost", "operator")
	if err == nil {
		t.Fatal("expected error for unknown id")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Errorf("error should mention id, got %q", err.Error())
	}
}

// Test_MemoryStore_Ack_twice_idempotent verifies that re-acking a known
// anomaly is a no-op (no error) but overwrites the previous AckedBy/AckedAt.
// This matches operator UI behavior (user re-acks after edit).
func Test_MemoryStore_Ack_twice_idempotent(t *testing.T) {
	s := NewMemoryStore(10)
	s.now = func() time.Time { return time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC) }
	sa, _ := s.Save(AnomalyEvent{TenantID: "t", AnomalyType: "burst"})

	if err := s.Ack(sa.AnomalyID, "first"); err != nil {
		t.Fatalf("first Ack: %v", err)
	}
	if err := s.Ack(sa.AnomalyID, "second"); err != nil {
		t.Fatalf("second Ack should be idempotent, got %v", err)
	}
	got, _ := s.Get(sa.AnomalyID)
	if got.AckedBy != "second" {
		t.Errorf("expected last writer wins, got %q", got.AckedBy)
	}
}

// Test_MemoryStore_ListUnacked_excludes_acked verifies the primary use
// case: show operators only what still needs attention.
func Test_MemoryStore_ListUnacked_excludes_acked(t *testing.T) {
	s := NewMemoryStore(10)
	a, _ := s.Save(AnomalyEvent{TenantID: "t", AnomalyType: "burst"})
	b, _ := s.Save(AnomalyEvent{TenantID: "t", AnomalyType: "tool_error_spike", Tool: "mcp_x"})
	_, _ = s.Save(AnomalyEvent{TenantID: "t", AnomalyType: "tenant_error_anomaly"})
	if err := s.Ack(a.AnomalyID, "op"); err != nil {
		t.Fatalf("Ack: %v", err)
	}
	if err := s.Ack(b.AnomalyID, "op"); err != nil {
		t.Fatalf("Ack: %v", err)
	}

	unacked := s.ListUnacked(100)
	if len(unacked) != 1 {
		t.Fatalf("expected 1 unacked, got %d", len(unacked))
	}
	if unacked[0].Event.AnomalyType != "tenant_error_anomaly" {
		t.Errorf("unexpected unacked type: %q", unacked[0].Event.AnomalyType)
	}
}

// Test_MemoryStore_ListAll_includes_acked verifies the audit list returns
// everything regardless of ack state.
func Test_MemoryStore_ListAll_includes_acked(t *testing.T) {
	s := NewMemoryStore(10)
	a, _ := s.Save(AnomalyEvent{TenantID: "t", AnomalyType: "burst"})
	_, _ = s.Save(AnomalyEvent{TenantID: "t", AnomalyType: "tool_error_spike"})
	_ = s.Ack(a.AnomalyID, "op")

	all := s.ListAll(100)
	if len(all) != 2 {
		t.Fatalf("expected 2 records, got %d", len(all))
	}
}

// Test_MemoryStore_List_respects_limit verifies that list methods honour
// the limit (operators only see N most-recent rows in the dashboard).
func Test_MemoryStore_List_respects_limit(t *testing.T) {
	s := NewMemoryStore(10)
	for range 5 {
		_, _ = s.Save(AnomalyEvent{TenantID: "t", AnomalyType: "burst"})
	}
	if got := s.ListUnacked(2); len(got) != 2 {
		t.Errorf("expected 2 unacked with limit=2, got %d", len(got))
	}
	if got := s.ListAll(3); len(got) != 3 {
		t.Errorf("expected 3 with limit=3, got %d", len(got))
	}
}

// Test_MemoryStore_capacity_evicts_oldest verifies the cap-drop policy
// matches the existing ring-buffer Store semantics (FIFO eviction).
func Test_MemoryStore_capacity_evicts_oldest(t *testing.T) {
	s := NewMemoryStore(2)
	a, _ := s.Save(AnomalyEvent{TenantID: "t", AnomalyType: "burst"})           // evicted
	_, _ = s.Save(AnomalyEvent{TenantID: "t", AnomalyType: "tool_error_spike"}) //nolint:errcheck // second save is fixture
	_, _ = s.Save(AnomalyEvent{TenantID: "t", AnomalyType: "tenant_error_anomaly"})

	if _, ok := s.Get(a.AnomalyID); ok {
		t.Errorf("expected oldest anomaly to be evicted, still found")
	}
	if got := s.ListAll(10); len(got) != 2 {
		t.Errorf("expected 2 retained, got %d", len(got))
	}
}

// Test_MemoryStore_ConcurrentSafe stresses Save/Get/Ack/ListAll under
// concurrent writers and readers. Run with -race to verify lock correctness.
func Test_MemoryStore_ConcurrentSafe(t *testing.T) {
	s := NewMemoryStore(500)
	idsCh := make(chan string, 100)

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 10 {
				sa, err := s.Save(AnomalyEvent{TenantID: "t", AnomalyType: "burst"})
				if err != nil {
					t.Errorf("Save: %v", err)
					return
				}
				idsCh <- sa.AnomalyID
			}
		})
	}
	wg.Wait()
	close(idsCh)

	wg2 := sync.WaitGroup{}
	for range 5 {
		wg2.Go(func() {
			_ = s.ListAll(500)
			_ = s.ListUnacked(500)
		})
	}
	wg2.Wait()

	count := 0
	for id := range idsCh {
		if _, ok := s.Get(id); !ok {
			t.Errorf("Get(%q) lost under concurrency", id)
		}
		if err := s.Ack(id, "concurrent-op"); err != nil {
			t.Errorf("Ack(%q): %v", id, err)
		}
		count++
	}
	if count != 100 {
		t.Errorf("expected 100 saved anomalies, got %d", count)
	}
}

// Test_MemoryStore_Save_rejects_empty_event ensures the store refuses a
// fully-blank AnomalyEvent. A blank event is almost always a programming
// bug, not a real anomaly.
func Test_MemoryStore_Save_rejects_empty_event(t *testing.T) {
	s := NewMemoryStore(10)
	_, err := s.Save(AnomalyEvent{})
	if err == nil {
		t.Fatal("expected error on empty event")
	}
	if !errors.Is(err, ErrEmptyAnomaly) {
		t.Errorf("expected ErrEmptyAnomaly, got %v", err)
	}
}
