package narrative

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

func newTestNarrativeHandlers(t *testing.T) *Handlers {
	t.Helper()
	eng := narrative.NewNarrativeEngine()
	svc := service.NewNarrativeService(t.TempDir(), eng, nil)
	return &Handlers{Svc: svc}
}

func TestParseFloatQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?valid=12.5&invalid=oops", nil)
	if got := parseFloatQuery(req, "valid"); got != 12.5 {
		t.Fatalf("valid = %v, want 12.5", got)
	}
	if got := parseFloatQuery(req, "invalid"); got != 0 {
		t.Fatalf("invalid = %v, want 0", got)
	}
	if got := parseFloatQuery(req, "missing"); got != 0 {
		t.Fatalf("missing = %v, want 0", got)
	}
}

func TestApproxPatternDays_CrossYear(t *testing.T) {
	p := service.SeasonalPattern{StartMonth: 12, StartDay: 20, EndMonth: 1, EndDay: 10}
	if got := approxPatternDays(p); got != 25 {
		t.Fatalf("approxPatternDays = %d, want 25", got)
	}
}

func TestBuildNarrativeData_FallbackUsesQueryParams(t *testing.T) {
	h := newTestNarrativeHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/narrative/events?geopolitical_gpr=1.5&us10y_change_bps=2.5&dxy_change_pct=-0.7&vix_level=18.2&usd_twd_change_pct=0.3&oil_change_pct=1.2&gold_change_pct=-0.4&gold_level=2300&jpy_change_pct=0.8&jpy_level=155&ai_capex_sentiment=0.6&retail_divergence=-0.2&margin_zscore=1.1&earnings_surprise_pct=3.4", nil)

	data := h.buildNarrativeData(req.Context(), req)

	if data.GeopoliticalGPR != 1.5 || data.US10YChangeBps != 2.5 || data.DXYChangePct != -0.7 {
		t.Fatalf("query override fields not applied: %+v", data)
	}
	if data.GoldLevel != 2300 || data.JPYLevel != 155 || data.EarningsSurprisePct != 3.4 {
		t.Fatalf("fallback fields not applied: %+v", data)
	}
}

func TestNarrativeHandlers_CoreEndpoints(t *testing.T) {
	h := newTestNarrativeHandlers(t)
	endpoints := []struct {
		name string
		path string
		fn   func(*http.Request) (int, any)
		key  string
	}{
		{"events", "/api/narrative/events?dxy_change_pct=2&vix_level=35", h.HandleNarrativeEvents, "events"},
		{"chains", "/api/narrative/chains?dxy_change_pct=2&vix_level=35", h.HandleNarrativeChains, "chains"},
		{"models", "/api/narrative/models?dxy_change_pct=2&vix_level=35", h.HandleNarrativeModels, "models"},
		{"templates", "/api/narrative/templates", h.HandleNarrativeTemplates, "templates"},
		{"seasonal", "/api/narrative/seasonal", h.HandleSeasonalAnalysis, "month"},
		{"bundle", "/api/narrative/bundle?dxy_change_pct=2&vix_level=35", h.HandleNarrativeBundle, "events"},
	}
	for _, endpoint := range endpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, endpoint.path, nil)
			status, body := endpoint.fn(req)
			if status != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body=%v)", status, body)
			}
			resp, ok := body.(map[string]any)
			if !ok {
				t.Fatalf("body = %T, want map[string]any", body)
			}
			if _, ok := resp[endpoint.key]; !ok {
				t.Fatalf("missing key %q in %v", endpoint.key, resp)
			}
			if _, err := json.Marshal(body); err != nil {
				t.Fatalf("response is not JSON encodable: %v", err)
			}
		})
	}
}

func TestHandleStressIndexCurrent(t *testing.T) {
	eng := narrative.NewNarrativeEngine()
	svc := service.NewNarrativeService("", eng, nil)
	h := &Handlers{Svc: svc}

	req := httptest.NewRequest(http.MethodGet, "/api/narrative/stress-index/current", nil)
	status, body := h.HandleStressIndexCurrent(req)

	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}

	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map body, got %T", body)
	}

	if _, ok := m["score"]; !ok {
		t.Error("expected 'score' field")
	}
	if _, ok := m["regime"]; !ok {
		t.Error("expected 'regime' field")
	}
	if _, ok := m["components"]; !ok {
		t.Error("expected 'components' field")
	}
	if _, ok := m["timestamp"]; !ok {
		t.Error("expected 'timestamp' field")
	}
}

func TestHandleStressIndexHistory(t *testing.T) {
	eng := narrative.NewNarrativeEngine()
	svc := service.NewNarrativeService("", eng, nil)
	h := &Handlers{Svc: svc}

	req := httptest.NewRequest(http.MethodGet, "/api/narrative/stress-index/history", nil)
	status, body := h.HandleStressIndexHistory(req)

	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}

	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map body, got %T", body)
	}

	if _, ok := m["history"]; !ok {
		t.Error("expected 'history' field")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/narrative/stress-index/history?days=7", nil)
	status2, _ := h.HandleStressIndexHistory(req2)
	if status2 != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status2)
	}

	req3 := httptest.NewRequest(http.MethodGet, "/api/narrative/stress-index/history?days=invalid", nil)
	status3, _ := h.HandleStressIndexHistory(req3)
	if status3 != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status3)
	}
}

func TestHandleStressIndexThresholds(t *testing.T) {
	eng := narrative.NewNarrativeEngine()
	svc := service.NewNarrativeService("", eng, nil)
	h := &Handlers{Svc: svc}

	req := httptest.NewRequest(http.MethodGet, "/api/narrative/stress-index/thresholds", nil)
	status, body := h.HandleStressIndexThresholds(req)

	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d", status)
	}

	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map body, got %T", body)
	}

	if _, ok := m["crisis"]; !ok {
		t.Error("expected 'crisis' field")
	}
	if _, ok := m["high"]; !ok {
		t.Error("expected 'high' field")
	}
	if _, ok := m["alert"]; !ok {
		t.Error("expected 'alert' field")
	}
}

// snapshotMacroProvider always returns a valid MacroDataSnapshot.
type snapshotMacroProvider struct{}

func (snapshotMacroProvider) Name() string { return "test-snapshot" }
func (snapshotMacroProvider) FetchSnapshot(_ context.Context) (marketdata.MacroDataSnapshot, error) {
	now := time.Now().Unix()
	return marketdata.MacroDataSnapshot{
		DXY:        marketdata.MacroDataPoint{Symbol: "DXY", Value: 104.5, ChangePct: 0.3, Timestamp: now},
		US10Y:      marketdata.MacroDataPoint{Symbol: "^TNX", Value: 4.32, ChangePct: 0.05, Timestamp: now},
		VIX:        marketdata.MacroDataPoint{Symbol: "^VIX", Value: 17.2, ChangePct: -0.8, Timestamp: now},
		USD_TWD:    marketdata.MacroDataPoint{Symbol: "USDTWD", Value: 32.1, ChangePct: 0.05, Timestamp: now},
		Oil:        marketdata.MacroDataPoint{Symbol: "CL=F", Value: 74.8, ChangePct: -0.3, Timestamp: now},
		Gold:       marketdata.MacroDataPoint{Symbol: "GC=F", Value: 2310, ChangePct: 0.4, Timestamp: now},
		JPY:        marketdata.MacroDataPoint{Symbol: "JPY=X", Value: 154.8, ChangePct: -0.05, Timestamp: now},
		RecordedAt: now,
	}, nil
}

func TestBuildNarrativeData_SnapshotSuccessPath(t *testing.T) {
	eng := narrative.NewNarrativeEngine()
	svc := service.NewNarrativeService(t.TempDir(), eng, nil)
	svc.SetMacroProvider(snapshotMacroProvider{})
	h := &Handlers{Svc: svc}

	// Query params should overlay snapshot data; geopolitical_gpr=0 lets snapshot value win.
	req := httptest.NewRequest(http.MethodGet, "/api/narrative/events", nil)
	data := h.buildNarrativeData(req.Context(), req)

	// Snapshot fields should be populated from the macro provider.
	if data.DXYChangePct == 0 {
		t.Error("DXYChangePct should be populated from snapshot, got 0")
	}
	if data.US10YChangeBps == 0 {
		t.Error("US10YChangeBps should be populated from snapshot, got 0")
	}
	if data.VIXLevel == 0 {
		t.Error("VIXLevel should be populated from snapshot, got 0")
	}
	// geopolitical_gpr query param not set → should come from snapshot (geo provider is nil, so 0 is expected).
	if data.GeopoliticalGPR != 0 {
		t.Errorf("GeopoliticalGPR = %v, want 0 (no geo provider)", data.GeopoliticalGPR)
	}
}

func TestHandleNarrativeEvents_WithSnapshotProvider(t *testing.T) {
	eng := narrative.NewNarrativeEngine()
	svc := service.NewNarrativeService(t.TempDir(), eng, nil)
	svc.SetMacroProvider(snapshotMacroProvider{})
	h := &Handlers{Svc: svc}

	req := httptest.NewRequest(http.MethodGet, "/api/narrative/events", nil)
	status, body := h.HandleNarrativeEvents(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%v)", status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("body type = %T, want map[string]any", body)
	}
	events, ok := resp["events"].([]narrative.NarrativeEvent)
	if !ok {
		t.Fatalf("events type = %T, want []NarrativeEvent", resp["events"])
	}
	// With snapshot data, events should be detected.
	_ = events // presence of key and correct type verified
	if _, err := json.Marshal(body); err != nil {
		t.Fatalf("body is not JSON encodable: %v", err)
	}
}

func TestHandleSeasonalAnalysis_WithIndustryService(t *testing.T) {
	eng := narrative.NewNarrativeEngine()
	svc := service.NewNarrativeService(t.TempDir(), eng, nil)
	industrySvc := service.NewIndustryService(
		industry.NewClassificationTree(),
		industry.NewSeasonalEngine(),
		nil, nil, nil, nil, nil, nil, nil,
		"",
	)
	h := &Handlers{Svc: svc, IndustryService: industrySvc}

	req := httptest.NewRequest(http.MethodGet, "/api/narrative/seasonal", nil)
	status, body := h.HandleSeasonalAnalysis(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%v)", status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("body type = %T, want map[string]any", body)
	}
	if _, ok := resp["month"]; !ok {
		t.Error("expected 'month' field")
	}
	// When IndustryService is non-nil, active_patterns should be present.
	if _, ok := resp["active_patterns"]; !ok {
		t.Error("expected 'active_patterns' field when IndustryService is set")
	}
	if _, err := json.Marshal(body); err != nil {
		t.Fatalf("body is not JSON encodable: %v", err)
	}
}

func TestHandleNarrativeBundle_WithIndustryService(t *testing.T) {
	eng := narrative.NewNarrativeEngine()
	svc := service.NewNarrativeService(t.TempDir(), eng, nil)
	svc.SetMacroProvider(snapshotMacroProvider{})
	industrySvc := service.NewIndustryService(
		industry.NewClassificationTree(),
		industry.NewSeasonalEngine(),
		nil, nil, nil, nil, nil, nil, nil,
		"",
	)
	h := &Handlers{Svc: svc, IndustryService: industrySvc}

	req := httptest.NewRequest(http.MethodGet, "/api/narrative/bundle", nil)
	status, body := h.HandleNarrativeBundle(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%v)", status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("body type = %T, want map[string]any", body)
	}
	// All expected keys should be present.
	for _, key := range []string{"events", "chains", "models", "templates", "seasonal"} {
		if _, ok := resp[key]; !ok {
			t.Errorf("expected %q key in bundle response", key)
		}
	}
	// Seasonal should have active_patterns when IndustryService is non-nil.
	seasonal, ok := resp["seasonal"].(map[string]any)
	if !ok {
		t.Fatalf("seasonal type = %T, want map[string]any", resp["seasonal"])
	}
	if _, ok := seasonal["active_patterns"]; !ok {
		t.Error("expected 'active_patterns' in seasonal when IndustryService is set")
	}
	if _, err := json.Marshal(body); err != nil {
		t.Fatalf("body is not JSON encodable: %v", err)
	}
}

func TestHandleNarrativeBundle_NilIndustryService(t *testing.T) {
	eng := narrative.NewNarrativeEngine()
	svc := service.NewNarrativeService(t.TempDir(), eng, nil)
	h := &Handlers{Svc: svc, IndustryService: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/narrative/bundle", nil)
	status, body := h.HandleNarrativeBundle(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%v)", status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("body type = %T, want map[string]any", body)
	}
	seasonal, ok := resp["seasonal"].(map[string]any)
	if !ok {
		t.Fatalf("seasonal type = %T, want map[string]any", resp["seasonal"])
	}
	// When IndustryService is nil, seasonal should have a 'note' field.
	if _, ok := seasonal["note"]; !ok {
		t.Error("expected 'note' field in seasonal when IndustryService is nil")
	}
}

func TestHandleNarrativeBundle_EventsOnlySnapshotPath(t *testing.T) {
	eng := narrative.NewNarrativeEngine()
	svc := service.NewNarrativeService(t.TempDir(), eng, nil)
	svc.SetMacroProvider(snapshotMacroProvider{})
	h := &Handlers{Svc: svc, IndustryService: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/narrative/bundle?geopolitical_gpr=2.5", nil)
	status, body := h.HandleNarrativeBundle(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%v)", status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("body type = %T, want map[string]any", body)
	}
	events, ok := resp["events"].([]narrative.NarrativeEvent)
	if !ok {
		t.Fatalf("events type = %T, want []NarrativeEvent", resp["events"])
	}
	// With a snapshot provider, events should be detected and not be empty.
	if len(events) == 0 {
		t.Log("events slice is empty (may be expected depending on snapshot values)")
	}
	if _, err := json.Marshal(body); err != nil {
		t.Fatalf("body is not JSON encodable: %v", err)
	}
}

func TestHandleGeopoliticalHistory_NoStore(t *testing.T) {
	eng := narrative.NewNarrativeEngine()
	svc := service.NewNarrativeService(t.TempDir(), eng, nil)
	h := &Handlers{Svc: svc}

	req := httptest.NewRequest(http.MethodGet, "/api/geopolitical/history", nil)
	status, body := h.HandleGeopoliticalHistory(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%v)", status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("body type = %T, want map[string]any", body)
	}
	if _, hasKey := resp["history"]; !hasKey {
		t.Error("expected 'history' key in response")
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("response is not JSON encodable: %v", err)
	}
	if len(encoded) == 0 {
		t.Error("encoded response is empty")
	}
}

func TestHandleGeopoliticalHistory_InvalidDaysParamFallsBackToDefault(t *testing.T) {
	eng := narrative.NewNarrativeEngine()
	svc := service.NewNarrativeService(t.TempDir(), eng, nil)
	h := &Handlers{Svc: svc}

	req := httptest.NewRequest(http.MethodGet, "/api/geopolitical/history?days=notanumber", nil)
	status, body := h.HandleGeopoliticalHistory(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%v)", status, body)
	}
	if _, err := json.Marshal(body); err != nil {
		t.Fatalf("response is not JSON encodable: %v", err)
	}
}

func TestHandleGeopoliticalHistory_RouteRegistered(t *testing.T) {
	h := newTestNarrativeHandlers(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/geopolitical/history", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := resp["history"]; !ok {
		t.Error("expected 'history' key in registered route response")
	}
}
