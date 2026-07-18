package industry

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// TestHandleSectorAllocationPlan_SnapshotUnavailable proves that without a
// SnapshotReader, the handler returns 503 with snapshot_unavailable reason
// (SA09 contract: no in-memory WeightEngine compute path).
func TestHandleSectorAllocationPlan_SnapshotUnavailable(t *testing.T) {
	h := setupIndustryHandlers()

	if h.Svc == nil {
		t.Fatal("Svc not wired")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/sector-allocation-plan", nil)
	status, body := h.HandleSectorAllocationPlan(req)

	if status != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %v", status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	if resp["fallback_reason"] != "snapshot_unavailable" {
		t.Fatalf("expected fallback_reason=snapshot_unavailable, got %v", resp["fallback_reason"])
	}
}

// TestNewIndustryService_WeightEngineInjected ensures the service constructor
// no longer creates a partial engine (SA06). WeightEngine is nil by design
// and must be injected by the caller (dashboard wiring or composition root).
func TestNewIndustryService_WeightEngineInjected(t *testing.T) {
	svc := service.NewIndustryService(
		industry.DefaultClassification(),
		industry.NewSeasonalEngine(),
		industry.NewCycleTracker(),
		industry.NewLinkageAnalyzer(),
		industry.NewRiskMonitor(),
		industry.NewSiliconCycleTracker(),
		industry.NewEventCalendar(),
		nil, // odmChannel
		nil, // dataAggregator
		"",  // paramsPath
	)
	if svc.WeightEngine != nil {
		t.Fatal("NewIndustryService should NOT create a WeightEngine — SA06 removed partial engine; callers must inject")
	}
}
