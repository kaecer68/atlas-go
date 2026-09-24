package orchestrator

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// consumingStore is a FileClosureStore whose Store() is immediately followed by
// a Consume(), simulating a wired allocator that read the policy before
// producing orders. It exists to prove applied=true is driven by consumption
// EVIDENCE, not by a code path that knows it stored something.
type consumingStore struct {
	*sectorallocation.FileClosureStore
	consumeSession string
}

func (s *consumingStore) Store(snap sectorallocation.SectorAllocationSnapshot) (*sectorallocation.MutationReceipt, error) {
	receipt, err := s.FileClosureStore.Store(snap)
	if err != nil || receipt == nil {
		return receipt, err
	}
	if _, err := s.FileClosureStore.Consume(receipt.ReceiptID, s.consumeSession); err != nil {
		return receipt, err
	}
	return receipt, nil
}

// TestApplySectorRotation_ConsumedPolicyIsApplied is the positive counterpart of
// TestApplySectorRotation_PersistsSnapshot: when a policy consumer records a
// ConsumptionReceipt, the same call reports applied=true.
func TestApplySectorRotation_ConsumedPolicyIsApplied(t *testing.T) {
	dir := t.TempDir()
	store := &consumingStore{
		FileClosureStore: sectorallocation.NewFileClosureStore(dir),
		consumeSession:   "next-session-001",
	}
	resolver := fixtureResolver(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), nil)
	evolver := NewStrategyEvolver().WithClosureStore(store).WithSessionResolver(resolver)

	plan := &portfolio.SectorRotationPlan{
		PrimaryFlow: "risk_on",
		Allocations: []portfolio.SectorAllocation{{Sector: "semiconductor", TargetPct: 0.30}},
	}
	_, applied, reason := evolver.ApplySectorRotation(plan, time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC), nil)
	if !applied {
		t.Fatalf("expected applied=true with consumption evidence, got reason=%q", reason)
	}
	if reason != "applied" {
		t.Fatalf("reason = %q, want %q", reason, "applied")
	}
}
