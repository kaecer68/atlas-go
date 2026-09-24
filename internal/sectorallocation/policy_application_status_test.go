package sectorallocation

import (
	"bytes"
	"os"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
)

// resetPolicyConsumers clears the consumer registry between tests. The
// production registry has no unregister operation by design, so the reset
// lives in the test package (same-package access to the unexported state).
func resetPolicyConsumers() {
	policyConsumerMu.Lock()
	defer policyConsumerMu.Unlock()
	policyConsumers = nil
}

// ---- application status / consumption evidence (issue #1944 Batch 1) -----

func storeFixtureSnapshot(t *testing.T, store *FileClosureStore) *MutationReceipt {
	t.Helper()
	return storeFixtureSnapshotDated(t, store, "2026-09-24", "2026-09-25")
}

// storeFixtureSnapshotDated stores a valid snapshot for a specific trading date.
// Distinct dates matter: Store() derives the receipt ID from the serialized
// body, so two byte-identical snapshots share a receipt ID.
func storeFixtureSnapshotDated(t *testing.T, store *FileClosureStore, asOf, effectiveFrom string) *MutationReceipt {
	t.Helper()
	receipt, err := store.Store(SectorAllocationSnapshot{
		AsOfTradingDate:   asOf,
		EffectiveFrom:     effectiveFrom,
		Target:            map[industry.SectorID]float64{"semiconductor": 0.30},
		Current:           map[industry.SectorID]float64{"semiconductor": 0.28},
		Delta:             map[industry.SectorID]float64{"semiconductor": 0.02},
		ModelVersion:      "1.0.0",
		CalibrationStatus: "calibrating",
		WeightSource:      "heuristic",
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	return receipt
}

// TestApplicationStatus_NoConsumerIsNotApplied pins the outward contract: with
// no allocator consumer wired, a stored snapshot must be reported as
// applied=false with the machine-readable reason allocator_unavailable.
func TestApplicationStatus_NoConsumerIsNotApplied(t *testing.T) {
	resetPolicyConsumers()
	t.Cleanup(resetPolicyConsumers)

	store := NewFileClosureStore(t.TempDir())
	storeFixtureSnapshot(t, store)

	snap := store.LatestSnapshot()
	if snap == nil {
		t.Fatal("expected a snapshot")
	}
	if snap.Applied {
		t.Error("applied must be false without consumption evidence")
	}
	if snap.FallbackReason != FallbackAllocatorUnavailable {
		t.Errorf("fallback_reason = %q, want %q", snap.FallbackReason, FallbackAllocatorUnavailable)
	}
	if snap.Consumption != nil {
		t.Errorf("consumption evidence must be nil, got %+v", snap.Consumption)
	}
	if PolicyConsumerWired() {
		t.Error("no consumer should be registered in this test")
	}
}

// TestApplicationStatus_ConsumerWiredButUnconsumed covers the second reason:
// a consumer exists but has not consumed this snapshot yet.
func TestApplicationStatus_ConsumerWiredButUnconsumed(t *testing.T) {
	resetPolicyConsumers()
	t.Cleanup(resetPolicyConsumers)
	RegisterPolicyConsumer("test-allocator")
	RegisterPolicyConsumer("test-allocator") // idempotent

	if got := RegisteredPolicyConsumers(); len(got) != 1 || got[0] != "test-allocator" {
		t.Fatalf("registry = %v, want [test-allocator]", got)
	}

	store := NewFileClosureStore(t.TempDir())
	storeFixtureSnapshot(t, store)

	snap := store.LatestSnapshot()
	if snap.Applied {
		t.Error("applied must be false: nothing consumed the snapshot")
	}
	if snap.FallbackReason != FallbackPendingConsumption {
		t.Errorf("fallback_reason = %q, want %q", snap.FallbackReason, FallbackPendingConsumption)
	}
}

// TestApplicationStatus_ConsumptionEvidenceFlipsApplied is the positive case:
// a recorded ConsumptionReceipt is the ONLY thing that makes Applied true.
func TestApplicationStatus_ConsumptionEvidenceFlipsApplied(t *testing.T) {
	resetPolicyConsumers()
	t.Cleanup(resetPolicyConsumers)

	store := NewFileClosureStore(t.TempDir())
	receipt := storeFixtureSnapshot(t, store)

	before := store.LatestSnapshot()
	if before.Applied {
		t.Fatal("precondition: not applied before consumption")
	}

	if _, err := store.Consume(receipt.ReceiptID, "next-session-001"); err != nil {
		t.Fatalf("Consume: %v", err)
	}

	after := store.LatestSnapshot()
	if after == nil {
		t.Fatal("expected the newest snapshot to remain readable after consumption")
	}
	if !after.Applied {
		t.Fatalf("applied must be true with consumption evidence (reason=%q)", after.FallbackReason)
	}
	if after.FallbackReason != "" {
		t.Errorf("fallback_reason must be cleared when applied, got %q", after.FallbackReason)
	}
	if after.Consumption == nil || after.Consumption.FromReceiptID != receipt.ReceiptID {
		t.Fatalf("consumption evidence not attached: %+v", after.Consumption)
	}

	// Latest() keeps the allocator contract: consumed rows are skipped.
	if snap, err := store.Latest(); err != nil || snap != nil {
		t.Fatalf("Latest() = (%v, %v), want (nil, nil) after consumption", snap, err)
	}

	// ConsumptionFor answers for the store's own receipt.
	if c := ConsumptionFor(store, receipt.ReceiptID); c == nil || c.SessionID != "next-session-001" {
		t.Fatalf("ConsumptionFor = %+v, want session next-session-001", c)
	}
	if c := ConsumptionFor(store, "unknown-receipt"); c != nil {
		t.Errorf("ConsumptionFor(unknown) = %+v, want nil", c)
	}
}

// TestStoreDoesNotHardWriteApplied guards the "不得硬寫" rule directly: even
// when the caller hands in Applied=true, the stored JSON must not claim
// application.
func TestStoreDoesNotHardWriteApplied(t *testing.T) {
	resetPolicyConsumers()
	t.Cleanup(resetPolicyConsumers)

	dir := t.TempDir()
	store := NewFileClosureStore(dir)
	if _, err := store.Store(SectorAllocationSnapshot{
		AsOfTradingDate: "2026-09-24",
		EffectiveFrom:   "2026-09-25",
		Target:          map[industry.SectorID]float64{"semiconductor": 0.30},
		ModelVersion:    "1.0.0",
		Applied:         true, // caller claim — must not be persisted as truth
	}); err != nil {
		t.Fatalf("Store: %v", err)
	}

	raw, err := os.ReadFile(store.filePath())
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	if bytes.Contains(raw, []byte(`"applied":true`)) {
		t.Fatalf("store persisted a hard-written applied=true line: %s", raw)
	}

	snap := store.LatestSnapshot()
	if snap.Applied {
		t.Error("applied must stay false without consumption evidence")
	}
}

// TestDecorateApplicationStatus_PureAndIdempotent documents that decorating
// twice (once in the store reader, once in the HTTP handler) cannot change the
// verdict, and that the input must NOT be mutated (a SnapshotReader may hand
// out a shared pointer; concurrent requests must not write through it).
func TestDecorateApplicationStatus_PureAndIdempotent(t *testing.T) {
	resetPolicyConsumers()
	t.Cleanup(resetPolicyConsumers)

	in := SectorAllocationSnapshot{Applied: true, FallbackReason: "stale"}
	first := DecorateApplicationStatus(in)
	second := DecorateApplicationStatus(*first)

	if second.Applied {
		t.Error("Applied must be derived from evidence, not from the incoming value")
	}
	if second.FallbackReason != FallbackAllocatorUnavailable {
		t.Errorf("fallback_reason = %q, want %q", second.FallbackReason, FallbackAllocatorUnavailable)
	}
	if !in.Applied || in.FallbackReason != "stale" {
		t.Errorf("input mutated by decorator: %+v", in)
	}
}
