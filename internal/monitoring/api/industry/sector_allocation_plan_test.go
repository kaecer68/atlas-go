package industry

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// TestHandleSectorAllocationPlan_ProductionWiring proves the wiring path
// from Svc.WeightEngine to SectorAllocator used in production
// (DashboardAPI.RegisterIndustryRoutes) makes HandleSectorAllocationPlan
// return 200 with a populated industries slice, instead of 503.
func TestHandleSectorAllocationPlan_ProductionWiring(t *testing.T) {
	h := setupIndustryHandlers()

	if h.SectorAllocator == nil {
		t.Fatal("SectorAllocator not wired — RegisterIndustryRoutes must set it from Svc.WeightEngine")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/sector-allocation-plan", nil)
	status, body := h.HandleSectorAllocationPlan(req)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	raw, present := resp["industries"]
	if !present {
		t.Fatal("response missing 'industries' key")
	}
	if raw == nil {
		t.Fatal("response.industries is nil")
	}
	t.Logf("industries runtime type: %T, sample: %+v", raw, raw)
}

// TestNewIndustryService_PopulatesWeightEngine ensures the service constructor
// sets WeightEngine so the wiring in RegisterIndustryRoutes can succeed.
func TestNewIndustryService_PopulatesWeightEngine(t *testing.T) {
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
	if svc.WeightEngine == nil {
		t.Fatal("NewIndustryService did not set WeightEngine — production wiring would fail")
	}
}
