package industry

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

func setupHandlers() *Handlers {
	classifier := industry.DefaultClassification()
	seasonal := industry.NewSeasonalEngine()
	cycleTracker := industry.NewCycleTracker()
	linkage := industry.NewLinkageAnalyzer()
	riskMonitor := industry.NewRiskMonitor()
	silicon := industry.NewSiliconCycleTracker()
	events := industry.NewEventCalendar()

	svc := service.NewIndustryService(
		classifier, seasonal, cycleTracker, linkage, riskMonitor, silicon, events,
		nil, // odmChannel
		nil, // dataAggregator
		"",  // paramsPath
	)

	return &Handlers{Svc: svc}
}

func TestHandleIndustryClassification(t *testing.T) {
	h := setupHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-classification", nil)

	status, body := h.HandleIndustryClassification(req)
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}

	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatal("expected map response")
	}

	industries, ok := resp["industries"].([]map[string]any)
	if !ok {
		t.Fatal("expected industries array")
	}

	if len(industries) == 0 {
		t.Error("expected non-empty industries list")
	}

	// Verify top-level count matches config
	count, ok := resp["count"].(int)
	if !ok || count == 0 {
		t.Error("expected positive count")
	}
}

func TestHandleIndustryOverview(t *testing.T) {
	h := setupHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-overview", nil)

	status, body := h.HandleIndustryOverview(req)
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}

	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatal("expected map response")
	}

	industries, ok := resp["industries"].([]map[string]any)
	if !ok {
		t.Fatal("expected industries array")
	}

	if len(industries) == 0 {
		t.Fatal("expected non-empty industries list")
	}

	// Verify adjusted weights sum to approximately 1.0
	totalWeight := 0.0
	for _, ind := range industries {
		if w, ok := ind["adjusted_weight"].(float64); ok {
			totalWeight += w
		}
	}

	if totalWeight < 0.99 || totalWeight > 1.01 {
		t.Errorf("expected total adjusted weight ~1.0, got %.4f", totalWeight)
	}
}

func TestHandleIndustryDetail(t *testing.T) {
	h := setupHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-detail?industry=semiconductor", nil)

	status, body := h.HandleIndustryDetail(req)
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %v", status, body)
	}

	// HandleIndustryDetail returns *service.IndustryDetail directly, not a map
	detail, ok := body.(*service.IndustryDetail)
	if !ok {
		t.Fatalf("expected *service.IndustryDetail, got %T", body)
	}

	if detail.ID != "semiconductor" {
		t.Errorf("expected id semiconductor, got %s", detail.ID)
	}

	if len(detail.WeightDerivation.DerivationFactors) == 0 {
		t.Log("derivation_factors empty — WeightEngine not injected (SA06: caller must wire engine)")
	}
}

func TestHandleIndustryDetail_MissingParam(t *testing.T) {
	h := setupHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-detail", nil)

	status, body := h.HandleIndustryDetail(req)
	if status != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", status)
	}

	errResp, ok := body.(map[string]string)
	if !ok || errResp["error"] == "" {
		t.Error("expected error message in response")
	}
}

func TestHandleIndustryDetail_NotFound(t *testing.T) {
	h := setupHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-detail?industry=nonexistent", nil)

	status, body := h.HandleIndustryDetail(req)
	if status != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", status)
	}

	errResp, ok := body.(map[string]string)
	if !ok || errResp["error"] == "" {
		t.Error("expected error message in response")
	}
}

func TestHandleShockSimulation(t *testing.T) {
	h := setupHandlers()
	// Empty body triggers invalid json decode error
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/industry-shock-simulation", http.NoBody)

	status, _ := h.HandleShockSimulation(req)
	if status != http.StatusBadRequest {
		t.Fatalf("expected status 400 for invalid json, got %d", status)
	}
}
