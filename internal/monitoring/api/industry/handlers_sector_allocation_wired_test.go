package industry

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// mockSnapshotReader implements sectorallocation.SnapshotReader for tests.
type mockSnapshotReader struct {
	snap *sectorallocation.SectorAllocationSnapshot
}

func (m *mockSnapshotReader) LatestSnapshot() *sectorallocation.SectorAllocationSnapshot {
	return m.snap
}

// TestHandleSectorAllocationPlan_SnapshotWiredReturns200 verifies that when a
// SnapshotReader returns a valid snapshot, the handler returns 200 with
// target/current/delta/model_version/calibration_status fields (SA09 contract).
func TestHandleSectorAllocationPlan_SnapshotWiredReturns200(t *testing.T) {
	h := setupIndustryHandlers()

	snapshot := &sectorallocation.SectorAllocationSnapshot{
		AsOfTradingDate:   "2026-07-17",
		EffectiveFrom:     "2026-07-18",
		Target:            map[industry.SectorID]float64{industry.SectorSemiconductor: 0.33},
		Current:           map[industry.SectorID]float64{industry.SectorSemiconductor: 0.30},
		Delta:             map[industry.SectorID]float64{industry.SectorSemiconductor: 0.03},
		ModelVersion:      "v0.0.0.32-canonical",
		CalibrationStatus: "calibrating",
		WeightSource:      "parameters.json#strategic_prior",
		Applied:           false,
	}
	h.Svc.WithSnapshotReader(&mockSnapshotReader{snap: snapshot})

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/sector-allocation-plan", nil)
	status, body := h.HandleSectorAllocationPlan(req)

	if status != http.StatusOK {
		t.Fatalf("expected 200 (snapshot reader wired), got %d: %v", status, body)
	}

	resp, ok := body.(*sectorallocation.SectorAllocationSnapshot)
	if !ok {
		t.Fatalf("expected *SectorAllocationSnapshot, got %T", body)
	}
	if resp.AsOfTradingDate != "2026-07-17" {
		t.Errorf("AsOfTradingDate = %q, want 2026-07-17", resp.AsOfTradingDate)
	}
	if resp.ModelVersion != "v0.0.0.32-canonical" {
		t.Errorf("ModelVersion = %q", resp.ModelVersion)
	}
	if len(resp.Target) == 0 {
		t.Error("target weights must be non-empty")
	}
}

// TestHandleSectorAllocationPlan_WiredNoSnapshotReturns200 verifies that when a
// SnapshotReader is wired but returns nil (no simulation session has closed yet),
// the handler returns 200 with an empty plan + fallback_reason, not 503.
func TestHandleSectorAllocationPlan_WiredNoSnapshotReturns200(t *testing.T) {
	h := setupIndustryHandlers()

	// Wire the reader but have it return nil — cold-start scenario.
	h.Svc.WithSnapshotReader(&mockSnapshotReader{snap: nil})

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/sector-allocation-plan", nil)
	status, body := h.HandleSectorAllocationPlan(req)

	if status != http.StatusOK {
		t.Fatalf("expected 200 (reader wired, no snapshot yet), got %d: %v", status, body)
	}

	resp, ok := body.(sectorallocation.SectorAllocationSnapshot)
	if !ok {
		t.Fatalf("expected SectorAllocationSnapshot, got %T", body)
	}
	if resp.FallbackReason != "no_simulation_session" {
		t.Errorf("expected fallback_reason=no_simulation_session, got %q", resp.FallbackReason)
	}
}
