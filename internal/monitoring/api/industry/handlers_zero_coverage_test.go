package industry

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// setupIndustryHandlers returns a Handlers with a fully-wired service, ready
// for tests on handlers that delegate to service methods.
func setupIndustryHandlers() *Handlers {
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

	return &Handlers{
		Svc:             svc,
		SectorAllocator: svc.WeightEngine,
	}
}

// =====================================================================
// HandleIndustryCycle
// =====================================================================

// All-industries path: no query param → returns list.
func TestHandleIndustryCycle_AllIndustries(t *testing.T) {
	h := setupIndustryHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-cycle", nil)

	status, body := h.HandleIndustryCycle(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", body)
	}
	industries, ok := resp["industries"].([]map[string]any)
	if !ok {
		t.Fatal("expected industries array")
	}
	count, ok := resp["count"].(int)
	if !ok {
		t.Fatal("expected count int")
	}
	if count != len(industries) {
		t.Errorf("count %d != industries length %d", count, len(industries))
	}
}

// Specific industry path: semiconductor returns a single-item response.
func TestHandleIndustryCycle_SpecificIndustry(t *testing.T) {
	h := setupIndustryHandlers()
	q := url.Values{}
	q.Set("industry", "semiconductor")
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-cycle?"+q.Encode(), nil)

	status, body := h.HandleIndustryCycle(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", body)
	}
	// Single-industry response has no "industries" key, just flat fields.
	if _, ok := resp["industry"]; !ok {
		t.Error("expected flat 'industry' key for single-industry response")
	}
	if _, ok := resp["business_cycle"]; !ok {
		t.Error("expected business_cycle field")
	}
}

// Unknown industry: GetCyclePositions returns !ok → 404.
func TestHandleIndustryCycle_NotFound(t *testing.T) {
	h := setupIndustryHandlers()
	q := url.Values{}
	q.Set("industry", "nonexistent_industry_xyz")
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-cycle?"+q.Encode(), nil)

	status, body := h.HandleIndustryCycle(req)
	if status != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %v", status, body)
	}
}

// =====================================================================
// HandleIndustryLinkage
// =====================================================================

// Happy path: valid industry → 200 with linkage info.
func TestHandleIndustryLinkage_Happy(t *testing.T) {
	h := setupIndustryHandlers()
	q := url.Values{}
	q.Set("industry", "semiconductor")
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-linkage?"+q.Encode(), nil)

	status, body := h.HandleIndustryLinkage(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", body)
	}
	for _, key := range []string{"industry", "upstream", "downstream", "correlations", "linkage_score"} {
		if _, ok := resp[key]; !ok {
			t.Errorf("missing key %q in response", key)
		}
	}
}

// Missing industry parameter → 400.
func TestHandleIndustryLinkage_MissingParam(t *testing.T) {
	h := setupIndustryHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-linkage", nil)

	status, body := h.HandleIndustryLinkage(req)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %v", status, body)
	}
	errResp, ok := body.(map[string]string)
	if !ok || errResp["error"] == "" {
		t.Error("expected error message")
	}
}

// =====================================================================
// HandleIndustryRisk
// =====================================================================

// Symbol parameter: risk lookup by symbol.
func TestHandleIndustryRisk_BySymbol(t *testing.T) {
	h := setupIndustryHandlers()
	q := url.Values{}
	q.Set("symbol", "2330")
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-risk?"+q.Encode(), nil)

	status, body := h.HandleIndustryRisk(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", body)
	}
	for _, key := range []string{"symbol", "risk_count", "risks"} {
		if _, ok := resp[key]; !ok {
			t.Errorf("missing key %q in response", key)
		}
	}
}

// Industry parameter: risk lookup by industry ID.
func TestHandleIndustryRisk_ByIndustry(t *testing.T) {
	h := setupIndustryHandlers()
	q := url.Values{}
	q.Set("industry", "semiconductor")
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-risk?"+q.Encode(), nil)

	status, body := h.HandleIndustryRisk(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", body)
	}
	if _, ok := resp["industry"]; !ok {
		t.Error("expected industry field in response")
	}
}

// Neither symbol nor industry → 400.
func TestHandleIndustryRisk_MissingParams(t *testing.T) {
	h := setupIndustryHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-risk", nil)

	status, body := h.HandleIndustryRisk(req)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %v", status, body)
	}
}

// =====================================================================
// HandleIndustryCalibration
// =====================================================================

// Service is nil → 500.
func TestHandleIndustryCalibration_ServiceNil(t *testing.T) {
	h := &Handlers{Svc: nil}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-calibration", nil)

	status, body := h.HandleIndustryCalibration(req)
	if status != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %v", status, body)
	}
}

// Service present but CycleCalibration is nil → 200 with calibrated=false.
func TestHandleIndustryCalibration_NotCalibrated(t *testing.T) {
	h := setupIndustryHandlers()
	// CycleCalibration is nil by default from NewIndustryService with no calibration set.
	h.Svc.CycleCalibration = nil
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/industry-calibration", nil)

	status, body := h.HandleIndustryCalibration(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", body)
	}
	if resp["calibrated"] != false {
		t.Errorf("expected calibrated=false, got %v", resp["calibrated"])
	}
}

// =====================================================================
// /api/dashboard/calendar-events → 308 redirect to /api/events/calendar
// =====================================================================

func TestCalendarEventsRedirect_PreservesQuery(t *testing.T) {
	mux := http.NewServeMux()
	(&Handlers{}).RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/calendar-events?start=2026-07-01&end=2026-07-31", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusPermanentRedirect {
		t.Fatalf("expected 308, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	expected := "/api/events/calendar?start=2026-07-01&end=2026-07-31"
	if loc != expected {
		t.Fatalf("expected Location %q, got %q", expected, loc)
	}
}

func TestCalendarEventsRedirect_NoQuery(t *testing.T) {
	mux := http.NewServeMux()
	(&Handlers{}).RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/calendar-events", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusPermanentRedirect {
		t.Fatalf("expected 308, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/api/events/calendar" {
		t.Fatalf("expected Location %q, got %q", "/api/events/calendar", loc)
	}
}

// =====================================================================
// HandleSectorAllocationPlan
// =====================================================================

// SectorAllocator is nil → 503.
func TestHandleSectorAllocationPlan_SnapshotNotFound(t *testing.T) {
	h := setupIndustryHandlers()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/sector-allocation-plan", nil)

	status, body := h.HandleSectorAllocationPlan(req)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %v", status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", body)
	}
	if resp["error"] != "snapshot_unavailable" {
		t.Errorf("expected error code snapshot_unavailable, got %v", resp["error"])
	}
	if resp["fallback_reason"] != "snapshot_unavailable" {
		t.Errorf("expected fallback_reason=snapshot_unavailable, got %v", resp["fallback_reason"])
	}
}

// =====================================================================
// HandleShockSimulation – additional coverage
// =====================================================================

// Valid request: returns impact list.
func TestHandleShockSimulation_ValidRequest(t *testing.T) {
	h := setupIndustryHandlers()
	body := `{"source_industry":"semiconductor","shock_magnitude":-0.05,"max_depth":2}`
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/industry-shock-simulation",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	status, resp := h.HandleShockSimulation(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, resp)
	}
	m, ok := resp.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", resp)
	}
	if m["source"] != "semiconductor" {
		t.Errorf("expected source=semiconductor, got %v", m["source"])
	}
	if m["shock"] != -0.05 {
		t.Errorf("expected shock=-0.05, got %v", m["shock"])
	}
	if _, ok := m["impacts"]; !ok {
		t.Error("expected impacts key")
	}
}

// Missing source_industry → 400.
func TestHandleShockSimulation_MissingSource(t *testing.T) {
	h := setupIndustryHandlers()
	body := `{"shock_magnitude":-0.05}`
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/industry-shock-simulation",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	status, _ := h.HandleShockSimulation(req)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", status)
	}
}

// Zero max_depth → defaults to 3.
func TestHandleShockSimulation_DefaultMaxDepth(t *testing.T) {
	h := setupIndustryHandlers()
	body := `{"source_industry":"semiconductor","shock_magnitude":-0.05,"max_depth":0}`
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/industry-shock-simulation",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	status, resp := h.HandleShockSimulation(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, resp)
	}
	m := resp.(map[string]any)
	if m["max_depth"] != 3 {
		t.Errorf("expected max_depth=3 (default), got %v", m["max_depth"])
	}
}
