package strategies

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kaecer68/atlas-go/internal/strategy_techniques"
)

const testSeedsJSON = `[
  {"id":"alpha","name":"alpha","layer":"L1","summary":"us rate down",
   "direction":"up","risk":"medium","source":"backtest","status":"active",
   "attribution_mode":"rule_based",
   "conditions":[{"field":"DXY.ChangePct","operator":"lt","value":-0.3,"string_value":"","timeframe":"1D","source":"us_yahoo"}]},
  {"id":"beta","name":"beta","layer":"L2","summary":"foreign inflow 3 days",
   "direction":"up","risk":"low","source":"backtest","status":"active",
   "attribution_mode":"rule_based",
   "conditions":[{"field":"ForeignInvestorNet.Value","operator":"gt","value":0,"string_value":"","timeframe":"3D","source":"twse_capital_flow"}]},
  {"id":"gamma","name":"gamma","layer":"L3","summary":"nvidia tsmadr confirm",
   "direction":"up","risk":"low","source":"backtest","status":"degraded",
   "attribution_mode":"rule_based","attribution":["regime_shift_q2_2026"],
   "conditions":[{"field":"NVDA.ChangePct","operator":"gt","value":0.5,"string_value":"","timeframe":"1D","source":"us_nvda"}]},
  {"id":"delta","name":"delta","layer":"L5","summary":"taiwan strait tension",
   "direction":"volatile","risk":"high","source":"manual","status":"active",
   "attribution_mode":"llm_annotated",
   "conditions":[{"field":"DXY.ChangePct","operator":"gt","value":0.5,"string_value":"","timeframe":"1D","source":"us_yahoo"}]},
  {"id":"epsilon","name":"epsilon","layer":"L5","summary":"us tariff shock",
   "direction":"down","risk":"high","source":"manual","status":"expired",
   "attribution_mode":"llm_annotated","attribution":["event_resolved_2026_05"],
   "conditions":[{"field":"VIX.Value","operator":"gt","value":30,"string_value":"","timeframe":"1D","source":"us_yahoo"}]}
]`

func newTestRegistry(t *testing.T) *strategy_techniques.Registry {
	t.Helper()
	reg, err := strategy_techniques.LoadFromBytes([]byte(testSeedsJSON))
	if err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	return reg
}

func newTestMux(reg *strategy_techniques.Registry) *http.ServeMux {
	mux := http.NewServeMux()
	h := NewHandlers(reg, nil)
	h.RegisterRoutes(mux)
	return mux
}

func doGET(t *testing.T, mux http.Handler, path string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	var body map[string]any
	if rr.Body.Len() > 0 {
		_ = json.Unmarshal(rr.Body.Bytes(), &body)
	}
	return rr.Code, body
}

func doPOST(t *testing.T, mux http.Handler, path string, payload any) (int, map[string]any) {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	var body map[string]any
	if rr.Body.Len() > 0 {
		_ = json.Unmarshal(rr.Body.Bytes(), &body)
	}
	return rr.Code, body
}

func TestNewHandlers_NilRegistrySafe(t *testing.T) {
	h := NewHandlers(nil, nil)
	if h == nil {
		t.Fatal("NewHandlers returned nil")
	}
}

func TestHandlers_ListStrategies_All(t *testing.T) {
	reg := newTestRegistry(t)
	mux := newTestMux(reg)
	code, body := doGET(t, mux, "/api/strategies")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, body)
	}
	if got := int(body["total"].(float64)); got != 5 {
		t.Errorf("total = %d, want 5", got)
	}
	strategies, ok := body["strategies"].([]any)
	if !ok || len(strategies) != 5 {
		t.Errorf("strategies len = %v, want 5", strategies)
	}
}

func TestHandlers_ListStrategies_FilterByLayer(t *testing.T) {
	reg := newTestRegistry(t)
	mux := newTestMux(reg)
	code, body := doGET(t, mux, "/api/strategies?layer=L5")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, body)
	}
	if got := int(body["total"].(float64)); got != 2 {
		t.Errorf("L5 total = %d, want 2", got)
	}
}

func TestHandlers_ListStrategies_InvalidLayer(t *testing.T) {
	reg := newTestRegistry(t)
	mux := newTestMux(reg)
	code, body := doGET(t, mux, "/api/strategies?layer=L9")
	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", code)
	}
	if _, ok := body["error"].(string); !ok {
		t.Errorf("missing error field: %v", body)
	}
}

func TestHandlers_ListActive(t *testing.T) {
	reg := newTestRegistry(t)
	mux := newTestMux(reg)
	code, body := doGET(t, mux, "/api/strategies/active")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, body)
	}
	if got := int(body["total"].(float64)); got != 3 {
		t.Errorf("active total = %d, want 3 (alpha+beta+delta)", got)
	}
}

// TestHandlers_ListActive_ReportsMeasuredFalseByDefault verifies that a
// strategy with no FeedbackStore record is reported as measured=false.
func TestHandlers_ListActive_ReportsMeasuredFalseByDefault(t *testing.T) {
	reg := newTestRegistry(t)
	h := NewHandlers(reg, NewFeedbackStore(t.TempDir()))
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	code, body := doGET(t, mux, "/api/strategies/active")
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%v", code, body)
	}
	strategies := body["strategies"].([]any)
	for _, s := range strategies {
		if got := s.(map[string]any)["measured"]; got != false {
			t.Errorf("strategy %v expected measured=false, got %v", s.(map[string]any)["id"], got)
		}
	}
}

// TestHandlers_ListActive_ReportsMeasuredTrueAfterAttribution verifies
// that a strategy with a FeedbackStore record is reported as measured=true
// with last_backtest_date set (#1259).
func TestHandlers_ListActive_ReportsMeasuredTrueAfterAttribution(t *testing.T) {
	reg := newTestRegistry(t)
	fb := NewFeedbackStore(t.TempDir())
	if err := fb.Write(Record{
		StrategyID: "alpha",
		TotalTests: 50,
		TotalHits:  32,
		HitRate:    0.64,
		Status:     "attribution_attempted",
	}); err != nil {
		t.Fatalf("seed feedback: %v", err)
	}
	h := NewHandlers(reg, fb)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	code, body := doGET(t, mux, "/api/strategies/active")
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%v", code, body)
	}
	strategies := body["strategies"].([]any)
	found := false
	for _, s := range strategies {
		sm := s.(map[string]any)
		if sm["id"] == "alpha" {
			found = true
			if sm["measured"] != true {
				t.Errorf("alpha expected measured=true, got %v", sm["measured"])
			}
			if sm["hit_rate"] == 0.0 {
				t.Errorf("alpha expected non-zero hit_rate after attribution, got %v", sm["hit_rate"])
			}
			if _, ok := sm["last_backtest_date"].(string); !ok {
				t.Errorf("alpha expected last_backtest_date string, got %T", sm["last_backtest_date"])
			}
		}
	}
	if !found {
		t.Fatal("alpha strategy not found in listActive response")
	}
}

func TestHandlers_ListByLayer_Valid(t *testing.T) {
	reg := newTestRegistry(t)
	mux := newTestMux(reg)
	code, body := doGET(t, mux, "/api/strategies?layer=L2")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, body)
	}
	if got := int(body["total"].(float64)); got != 1 {
		t.Errorf("L2 total = %d, want 1", got)
	}
}

func TestHandlers_ListByLayer_Invalid(t *testing.T) {
	reg := newTestRegistry(t)
	mux := newTestMux(reg)
	code, body := doGET(t, mux, "/api/strategies?layer=INVALID")
	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body=%v", code, body)
	}
}

func TestHandlers_ListLayers(t *testing.T) {
	reg := newTestRegistry(t)
	mux := newTestMux(reg)
	code, body := doGET(t, mux, "/api/strategies/layers")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, body)
	}
	if got := int(body["total"].(float64)); got != 5 {
		t.Errorf("total = %d, want 5", got)
	}
	layers, ok := body["layers"].([]any)
	if !ok || len(layers) != 5 {
		t.Errorf("layers len = %v, want 5", layers)
	}
}

func TestHandlers_GetStrategy_OK(t *testing.T) {
	reg := newTestRegistry(t)
	mux := newTestMux(reg)
	code, body := doGET(t, mux, "/api/strategies/beta")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, body)
	}
	if body["id"] != "beta" {
		t.Errorf("id = %v, want beta", body["id"])
	}
	if body["layer"] != "L2" {
		t.Errorf("layer = %v, want L2", body["layer"])
	}
}

func TestHandlers_GetStrategy_NotFound(t *testing.T) {
	reg := newTestRegistry(t)
	mux := newTestMux(reg)
	code, body := doGET(t, mux, "/api/strategies/missing")
	if code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body=%v", code, body)
	}
}

func TestHandlers_GetStrategy_PathInjection(t *testing.T) {
	reg := newTestRegistry(t)
	mux := newTestMux(reg)
	code, _ := doGET(t, mux, "/api/strategies/..%2Fetc%2Fpasswd")
	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (path injection rejected)", code)
	}
}

func TestHandlers_ValidateStrategy_Reactivated(t *testing.T) {
	reg := newTestRegistry(t)
	mux := newTestMux(reg)
	code, body := doPOST(t, mux, "/api/strategies/gamma/validate", ValidateRequest{
		TotalTests: 20, TotalHits: 14,
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, body)
	}
	if body["status"] != "active" {
		t.Errorf("status = %v, want active", body["status"])
	}
	if body["message"] != "strategy reactivated" {
		t.Errorf("message = %v, want 'strategy reactivated'", body["message"])
	}
}

func TestHandlers_ValidateStrategy_Degraded(t *testing.T) {
	reg := newTestRegistry(t)
	mux := newTestMux(reg)
	code, body := doPOST(t, mux, "/api/strategies/alpha/validate", ValidateRequest{
		TotalTests: 20, TotalHits: 5,
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, body)
	}
	if body["status"] != "degraded" {
		t.Errorf("status = %v, want degraded", body["status"])
	}
}

func TestHandlers_ValidateStrategy_InsufficientSample(t *testing.T) {
	reg := newTestRegistry(t)
	mux := newTestMux(reg)
	code, body := doPOST(t, mux, "/api/strategies/alpha/validate", ValidateRequest{
		TotalTests: 5, TotalHits: 4,
	})
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, body)
	}
	if body["status"] != "degraded" {
		t.Errorf("status = %v, want degraded (insufficient sample)", body["status"])
	}
}

func TestHandlers_ValidateStrategy_NotFound(t *testing.T) {
	reg := newTestRegistry(t)
	mux := newTestMux(reg)
	code, _ := doPOST(t, mux, "/api/strategies/missing/validate", ValidateRequest{
		TotalTests: 10, TotalHits: 5,
	})
	if code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
}

func TestHandlers_ValidateStrategy_InvalidBody(t *testing.T) {
	reg := newTestRegistry(t)
	mux := newTestMux(reg)
	req := httptest.NewRequest(http.MethodPost, "/api/strategies/alpha/validate",
		bytes.NewReader([]byte("not json")))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestHandlers_ValidateStrategy_NegativeHits(t *testing.T) {
	reg := newTestRegistry(t)
	mux := newTestMux(reg)
	code, _ := doPOST(t, mux, "/api/strategies/alpha/validate", ValidateRequest{
		TotalTests: 10, TotalHits: -1,
	})
	if code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", code)
	}
}

func TestHandlers_GetAttribution_OK(t *testing.T) {
	reg := newTestRegistry(t)
	mux := newTestMux(reg)
	code, body := doGET(t, mux, "/api/strategies/gamma/attribution")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", code, body)
	}
	attr, ok := body["attribution"].([]any)
	if !ok || len(attr) != 1 {
		t.Errorf("attribution = %v, want 1 entry", body["attribution"])
	}
}

func TestHandlers_GetAttribution_NotFound(t *testing.T) {
	reg := newTestRegistry(t)
	mux := newTestMux(reg)
	code, _ := doGET(t, mux, "/api/strategies/missing/attribution")
	if code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", code)
	}
}

func TestHandlers_NilRegistry_Returns503(t *testing.T) {
	mux := newTestMux(nil)
	code, _ := doGET(t, mux, "/api/strategies")
	if code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", code)
	}
}

func TestToSummary_PreservesAllFields(t *testing.T) {
	reg, _ := strategy_techniques.LoadFromBytes([]byte(testSeedsJSON))
	frame, err := reg.FindByID("gamma")
	if err != nil {
		t.Fatal("expected to find gamma frame")
	}
	h := &Handlers{registry: reg}
	sum := h.toSummary(*frame)
	if sum.ID != "gamma" || sum.Layer != "L3" || sum.Direction != "up" ||
		sum.Risk != "low" || sum.Status != "degraded" || sum.Source != "backtest" {
		t.Errorf("summary mismatch: %+v", sum)
	}
	if len(sum.Attribution) != 1 || sum.Attribution[0] != "regime_shift_q2_2026" {
		t.Errorf("attribution not preserved: %v", sum.Attribution)
	}
}

func TestStrategiesListResponse_Structure(t *testing.T) {
	resp := StrategiesListResponse{Total: 0, Strategies: []StrategyFrameSummary{}}
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(b, []byte(`"total":0`)) {
		t.Errorf("missing total: %s", string(b))
	}
}
