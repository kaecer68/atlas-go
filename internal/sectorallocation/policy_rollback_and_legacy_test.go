package sectorallocation

import (
	"os"
	"testing"
)

// TestFileClosureStore_DeleteRemovesRowFromLatest is the regression test for the
// rollback contract broken while separating "consumed" from "deleted"
// (independent review G1): after Delete, the snapshot must not be served by
// Latest() (the allocator would resurrect a rolled-back policy) nor by
// LatestSnapshot() (the dashboard would show it as the current plan).
func TestFileClosureStore_DeleteRemovesRowFromLatest(t *testing.T) {
	resetPolicyConsumers()
	t.Cleanup(resetPolicyConsumers)

	store := NewFileClosureStore(t.TempDir())
	receipt := storeFixtureSnapshot(t, store)

	if err := store.Delete(receipt.ReceiptID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if snap, err := store.Latest(); err != nil || snap != nil {
		t.Fatalf("Latest() after Delete = (%+v, %v), want (nil, nil)", snap, err)
	}
	if snap := store.LatestSnapshot(); snap != nil {
		t.Fatalf("LatestSnapshot() after Delete = %+v, want nil", snap)
	}
}

// TestFileClosureStore_DeleteAfterConsumeStaysHidden covers the mixed case:
// a consumed-then-deleted receipt must stay hidden from both readers, and must
// not be reported as consumption evidence.
func TestFileClosureStore_DeleteAfterConsumeStaysHidden(t *testing.T) {
	resetPolicyConsumers()
	t.Cleanup(resetPolicyConsumers)

	store := NewFileClosureStore(t.TempDir())
	receipt := storeFixtureSnapshot(t, store)
	if _, err := store.Consume(receipt.ReceiptID, "next-session-001"); err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if err := store.Delete(receipt.ReceiptID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if snap, err := store.Latest(); err != nil || snap != nil {
		t.Fatalf("Latest() = (%+v, %v), want (nil, nil)", snap, err)
	}
	if snap := store.LatestSnapshot(); snap != nil {
		t.Fatalf("LatestSnapshot() = %+v, want nil", snap)
	}
	if c := store.ConsumptionFor(receipt.ReceiptID); c != nil {
		t.Errorf("ConsumptionFor(deleted) = %+v, want nil", c)
	}
}

// TestFileClosureStore_DeleteKeepsOlderRowServiceable: rolling back the newest
// snapshot must fall back to the previous still-valid one, not to nothing and
// not to the deleted row.
func TestFileClosureStore_DeleteKeepsOlderRowServiceable(t *testing.T) {
	resetPolicyConsumers()
	t.Cleanup(resetPolicyConsumers)

	store := NewFileClosureStore(t.TempDir())
	first := storeFixtureSnapshotDated(t, store, "2026-09-23", "2026-09-24")
	second := storeFixtureSnapshotDated(t, store, "2026-09-24", "2026-09-25")
	if second.ReceiptID == first.ReceiptID {
		t.Fatal("fixture must produce distinct receipts")
	}

	if err := store.Delete(second.ReceiptID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	snap, err := store.Latest()
	if err != nil || snap == nil {
		t.Fatalf("Latest() = (%+v, %v), want the older valid snapshot", snap, err)
	}
	if snap.MutationReceipt == nil || snap.MutationReceipt.ReceiptID != first.ReceiptID {
		t.Fatalf("Latest() served the deleted receipt (%+v)", snap.MutationReceipt)
	}
	out := store.LatestSnapshot()
	if out == nil || out.MutationReceipt == nil || out.MutationReceipt.ReceiptID != first.ReceiptID {
		t.Fatalf("LatestSnapshot() served %+v, want the older valid snapshot", out)
	}
}

// TestDecorateApplicationStatus_MigratesLegacyTargetNote covers pre-#1944 rows,
// which stored target-computation notes in fallback_reason and could carry a
// hard-written applied=true. On read the note must move to target_note and the
// status must be re-derived, otherwise existing JSONL rows lose their
// provenance (independent review G6).
func TestDecorateApplicationStatus_MigratesLegacyTargetNote(t *testing.T) {
	resetPolicyConsumers()
	t.Cleanup(resetPolicyConsumers)

	legacy := SectorAllocationSnapshot{
		AsOfTradingDate: "2026-09-01",
		EffectiveFrom:   "2026-09-02",
		FallbackReason:  "no weight engine",
		Applied:         true, // hard-written by the old Store()
	}
	got := DecorateApplicationStatus(legacy)
	if got.TargetNote != "no weight engine" {
		t.Errorf("legacy note not migrated: target_note=%q fallback_reason=%q", got.TargetNote, got.FallbackReason)
	}
	if got.Applied {
		t.Error("legacy hard-written applied=true must not survive decoration")
	}
	if got.FallbackReason != FallbackAllocatorUnavailable {
		t.Errorf("fallback_reason = %q, want %q", got.FallbackReason, FallbackAllocatorUnavailable)
	}

	// A status reason must NOT be mistaken for a legacy note.
	statusOnly := DecorateApplicationStatus(SectorAllocationSnapshot{FallbackReason: FallbackNoSimulationSession})
	if statusOnly.TargetNote != "" {
		t.Errorf("status reason leaked into target_note: %q", statusOnly.TargetNote)
	}
	if statusOnly.FallbackReason != FallbackAllocatorUnavailable {
		t.Errorf("fallback_reason = %q, want %q", statusOnly.FallbackReason, FallbackAllocatorUnavailable)
	}

	// An explicit TargetNote wins over the legacy field.
	both := DecorateApplicationStatus(SectorAllocationSnapshot{TargetNote: "projection failed: boom", FallbackReason: "no weight engine"})
	if both.TargetNote != "projection failed: boom" {
		t.Errorf("explicit target_note overwritten: %q", both.TargetNote)
	}
}

// TestFileClosureStore_LegacyRowIsMigratedOnRead writes a pre-#1944 JSONL line
// directly (applied=true + fallback_reason note) and checks both readers.
func TestFileClosureStore_LegacyRowIsMigratedOnRead(t *testing.T) {
	resetPolicyConsumers()
	t.Cleanup(resetPolicyConsumers)

	dir := t.TempDir()
	store := NewFileClosureStore(dir)
	line := `{"as_of_trading_date":"2026-09-01","effective_from":"2026-09-02",` +
		`"target":{"semiconductor":0.3},"model_version":"1.0.0",` +
		`"fallback_reason":"no weight engine","applied":true,` +
		`"mutation_receipt":{"receipt_id":"legacy1","stored_at":"2026-09-01T00:00:00Z","sha256":"deadbeef"}}` + "\n"
	if err := os.WriteFile(store.filePath(), []byte(line), 0o644); err != nil {
		t.Fatalf("write legacy row: %v", err)
	}

	for name, got := range map[string]*SectorAllocationSnapshot{
		"Latest":         mustLatest(t, store),
		"LatestSnapshot": store.LatestSnapshot(),
	} {
		if got == nil {
			t.Fatalf("%s: nil snapshot", name)
		}
		if got.Applied {
			t.Errorf("%s: legacy applied=true survived", name)
		}
		if got.TargetNote != "no weight engine" {
			t.Errorf("%s: target_note = %q, want the migrated legacy note", name, got.TargetNote)
		}
		if got.FallbackReason != FallbackAllocatorUnavailable {
			t.Errorf("%s: fallback_reason = %q, want %q", name, got.FallbackReason, FallbackAllocatorUnavailable)
		}
	}
}

func mustLatest(t *testing.T, store *FileClosureStore) *SectorAllocationSnapshot {
	t.Helper()
	snap, err := store.Latest()
	if err != nil {
		t.Fatalf("Latest: %v", err)
	}
	return snap
}
