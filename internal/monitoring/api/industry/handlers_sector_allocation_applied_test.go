package industry

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// Issue #1944 Batch 1: the outward applied/fallback_reason pair must be driven
// by consumption evidence (spec §8.3), never by the snapshot's stored value.

func fixtureAllocationSnapshot() *sectorallocation.SectorAllocationSnapshot {
	return &sectorallocation.SectorAllocationSnapshot{
		AsOfTradingDate:   "2026-09-24",
		EffectiveFrom:     "2026-09-25",
		Target:            map[industry.SectorID]float64{industry.SectorSemiconductor: 0.33},
		Current:           map[industry.SectorID]float64{industry.SectorSemiconductor: 0.30},
		Delta:             map[industry.SectorID]float64{industry.SectorSemiconductor: 0.03},
		ModelVersion:      "1.0.0",
		CalibrationStatus: "calibrating",
		WeightSource:      "heuristic",
	}
}

// TestHandleSectorAllocationPlan_StoredSnapshotIsNotApplied proves the handler
// refuses to pass through a claim it cannot back with evidence: even a reader
// that hands in Applied=true yields applied=false + allocator_unavailable.
func TestHandleSectorAllocationPlan_StoredSnapshotIsNotApplied(t *testing.T) {
	h := setupIndustryHandlers()

	snap := fixtureAllocationSnapshot()
	snap.Applied = true // unsupported claim from an alternative reader
	h.Svc.WithSnapshotReader(&mockSnapshotReader{snap: snap})

	status, body := h.HandleSectorAllocationPlan(httptest.NewRequest(http.MethodGet, "/api/dashboard/sector-allocation-plan", nil))
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, body)
	}
	resp, ok := body.(*sectorallocation.SectorAllocationSnapshot)
	if !ok {
		t.Fatalf("expected *SectorAllocationSnapshot, got %T", body)
	}
	if resp.Applied {
		t.Error("applied must be false: no consumption evidence")
	}
	if resp.FallbackReason != sectorallocation.FallbackAllocatorUnavailable {
		t.Errorf("fallback_reason = %q, want %q", resp.FallbackReason, sectorallocation.FallbackAllocatorUnavailable)
	}
	if len(resp.Target) == 0 {
		t.Error("target must still be exposed (not applied is not the same as no plan)")
	}
}

// TestHandleSectorAllocationPlan_ConsumedSnapshotIsApplied covers the positive
// path: consumption evidence makes the plan reported as applied, with no
// fallback reason.
func TestHandleSectorAllocationPlan_ConsumedSnapshotIsApplied(t *testing.T) {
	h := setupIndustryHandlers()

	snap := fixtureAllocationSnapshot()
	snap.Consumption = &sectorallocation.ConsumptionReceipt{
		FromReceiptID: "abc123",
		SessionID:     "next-session-001",
	}
	h.Svc.WithSnapshotReader(&mockSnapshotReader{snap: snap})

	status, body := h.HandleSectorAllocationPlan(httptest.NewRequest(http.MethodGet, "/api/dashboard/sector-allocation-plan", nil))
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, body)
	}
	resp, ok := body.(*sectorallocation.SectorAllocationSnapshot)
	if !ok {
		t.Fatalf("expected *SectorAllocationSnapshot, got %T", body)
	}
	if !resp.Applied {
		t.Fatalf("applied must be true with consumption evidence (reason=%q)", resp.FallbackReason)
	}
	if resp.FallbackReason != "" {
		t.Errorf("fallback_reason must be empty when applied, got %q", resp.FallbackReason)
	}
	if resp.Consumption == nil || resp.Consumption.SessionID != "next-session-001" {
		t.Errorf("consumption evidence must be echoed, got %+v", resp.Consumption)
	}
}

// TestHandleSectorAllocationPlan_NoSessionChecksAppliedZeroValue pins the
// baseline measured on production (fallback_reason=no_simulation_session):
// unchanged in wording, and applied must read false.
func TestHandleSectorAllocationPlan_NoSessionChecksAppliedZeroValue(t *testing.T) {
	h := setupIndustryHandlers()
	h.Svc.WithSnapshotReader(&mockSnapshotReader{snap: nil})

	status, body := h.HandleSectorAllocationPlan(httptest.NewRequest(http.MethodGet, "/api/dashboard/sector-allocation-plan", nil))
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, body)
	}
	resp, ok := body.(sectorallocation.SectorAllocationSnapshot)
	if !ok {
		t.Fatalf("expected SectorAllocationSnapshot, got %T", body)
	}
	if resp.Applied {
		t.Error("applied must be false when no session closed")
	}
	if resp.FallbackReason != sectorallocation.FallbackNoSimulationSession {
		t.Errorf("fallback_reason = %q, want %q", resp.FallbackReason, sectorallocation.FallbackNoSimulationSession)
	}
}

// TestHandleSectorAllocationPlan_UnavailableReaderStillReportsAppliedFalse keeps
// the 503 branch consistent with the 200 branches.
func TestHandleSectorAllocationPlan_UnavailableReaderStillReportsAppliedFalse(t *testing.T) {
	h := setupIndustryHandlers() // no snapshot reader wired

	status, body := h.HandleSectorAllocationPlan(httptest.NewRequest(http.MethodGet, "/api/dashboard/sector-allocation-plan", nil))
	if status != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %v", status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", body)
	}
	if applied, present := resp["applied"]; !present || applied != false {
		t.Errorf("503 body must carry applied=false, got %v", resp["applied"])
	}
	if resp["fallback_reason"] != sectorallocation.FallbackSnapshotUnavailable {
		t.Errorf("fallback_reason = %v, want %q", resp["fallback_reason"], sectorallocation.FallbackSnapshotUnavailable)
	}
}
