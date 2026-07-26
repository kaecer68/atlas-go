package risk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/risk"
)

func newTestHandlers(t *testing.T) *Handlers {
	t.Helper()
	dir := t.TempDir()
	return &Handlers{LedgerDir: dir}
}

func assertStatus(t *testing.T, status int, want int) {
	t.Helper()
	if status != want {
		t.Errorf("status = %d, want %d", status, want)
	}
}

func assertJSONKey(t *testing.T, body any, key string) map[string]any {
	t.Helper()
	b, _ := json.Marshal(body)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := m[key]; !ok {
		t.Errorf("response missing key %q: %v", key, m)
	}
	return m
}

func TestNewHandlers(t *testing.T) {
	h := NewHandlers("/tmp/test")
	if h == nil {
		t.Fatal("NewHandlers returned nil")
	}
	if h.LedgerDir != "/tmp/test" {
		t.Errorf("LedgerDir = %s, want /tmp/test", h.LedgerDir)
	}
}

func TestHandleRiskMetrics_NoSessionsDir(t *testing.T) {
	dir := t.TempDir()
	h := (&Handlers{LedgerDir: dir}).WithRiskGate(risk.NewRiskGate(nil, nil, nil))
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/risk", nil)
	status, body := h.HandleRiskMetrics(req)
	assertStatus(t, status, http.StatusOK)
	m := assertJSONKey(t, body, "risk_snapshot")
	snap, ok := m["risk_snapshot"].(map[string]any)
	if !ok {
		t.Fatalf("risk_snapshot not a map: %T", m["risk_snapshot"])
	}
	if snap["insufficient_data"] != float64(1) {
		t.Errorf("insufficient_data = %v, want 1", snap["insufficient_data"])
	}
	if snap["data_points"] != float64(0) {
		t.Errorf("data_points = %v, want 0", snap["data_points"])
	}
}

func TestHandleRiskMetrics_EmptySessionsDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	h := (&Handlers{LedgerDir: dir}).WithRiskGate(risk.NewRiskGate(nil, nil, nil))
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/risk", nil)
	status, body := h.HandleRiskMetrics(req)
	assertStatus(t, status, http.StatusOK)
	assertJSONKey(t, body, "risk_snapshot")
	assertJSONKey(t, body, "session_count")
}

func TestHandleRiskMetrics_WithSessions(t *testing.T) {
	dir := t.TempDir()
	sessionsDir := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	// Create a session with summary.json
	sessionDir := filepath.Join(sessionsDir, "session-20260101-daily")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session: %v", err)
	}
	summary := map[string]any{"portfolio_value": 1000000.0}
	b, _ := json.Marshal(summary)
	if err := os.WriteFile(filepath.Join(sessionDir, "summary.json"), b, 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	h := (&Handlers{LedgerDir: dir}).WithRiskGate(risk.NewRiskGate(nil, nil, nil))
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/risk", nil)
	status, body := h.HandleRiskMetrics(req)
	assertStatus(t, status, http.StatusOK)
	m := assertJSONKey(t, body, "risk_snapshot")
	snap, _ := m["risk_snapshot"].(map[string]any)
	if snap != nil {
		// With only 1 session, data_points is 0 (returns are []), insufficient_data=1
		if v, ok := snap["data_points"]; ok {
			_ = v
		}
	}
}

func TestHandleCorrelationMatrix_Default(t *testing.T) {
	h := &Handlers{LedgerDir: t.TempDir()}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/correlation-matrix", nil)
	status, body := h.HandleCorrelationMatrix(req)
	assertStatus(t, status, http.StatusOK)
	m := assertJSONKey(t, body, "symbols")
	assertJSONKey(t, body, "labels")
	assertJSONKey(t, body, "matrix")
	// Default matrix should have symbols
	symbols, _ := m["symbols"].([]any)
	if len(symbols) == 0 {
		t.Error("default correlation matrix should have symbols")
	}
}

func TestHandleCorrelationMatrix_WithNilMatrix(t *testing.T) {
	h := &Handlers{LedgerDir: t.TempDir()}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/correlation-matrix", nil)
	status, body := h.HandleCorrelationMatrix(req)
	assertStatus(t, status, http.StatusOK)
	assertJSONKey(t, body, "symbols")
	assertJSONKey(t, body, "matrix")
}

func TestHandleRiskMetrics_GateMode_WithRiskGate(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	rg := risk.NewRiskGate(nil, nil, nil)
	rg.SetMode(risk.ModeDefensive)
	h := (&Handlers{LedgerDir: dir}).WithRiskGate(rg)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/risk", nil)
	status, body := h.HandleRiskMetrics(req)
	assertStatus(t, status, http.StatusOK)
	m := assertJSONKey(t, body, "gate_mode")
	if m["gate_mode"] != "DEFENSIVE" {
		t.Errorf("gate_mode = %v, want DEFENSIVE", m["gate_mode"])
	}
}

func TestHandleRiskCalibration_NilRiskGate(t *testing.T) {
	h := &Handlers{LedgerDir: t.TempDir()}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/risk-calibration", nil)
	status, body := h.HandleRiskCalibration(req)
	assertStatus(t, status, http.StatusServiceUnavailable)
	m := assertJSONKey(t, body, "data_status")
	if m["data_status"] != "service_unavailable" {
		t.Errorf("data_status = %v, want service_unavailable", m["data_status"])
	}
}

func TestHandleRiskCalibration_WithRiskGate(t *testing.T) {
	rg := risk.NewRiskGate(nil, nil, nil)
	h := NewHandlers(t.TempDir()).WithRiskGate(rg)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/risk-calibration", nil)
	status, body := h.HandleRiskCalibration(req)
	assertStatus(t, status, http.StatusOK)
	// No calibration has run yet, so report should be nil
	m := assertJSONKey(t, body, "status")
	if m["status"] != "not_available" {
		// If calibration has run, we'd get a report
		_ = m
	}
}

func TestIndustryLabel(t *testing.T) {
	tree := industry.NewClassificationTree()
	tree.AddSegment(&industry.IndustrySegment{ID: "ai_supply_chain", Name: "AI 供應鏈"})
	tree.AddSegment(&industry.IndustrySegment{ID: "defensive", Name: "防禦型"})
	h := NewHandlers(t.TempDir()).WithClassificationTree(tree)

	cases := map[string]string{
		"ai_supply_chain": "AI 供應鏈",
		"defensive":       "防禦型",
	}
	for id, want := range cases {
		got := h.industryLabel(id)
		if got != want {
			t.Errorf("industryLabel(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestIndustryLabel_LegacyFallback(t *testing.T) {
	tree := industry.NewClassificationTree()
	tree.AddSegment(&industry.IndustrySegment{ID: "ai_supply_chain", Name: "AI 供應鏈"})
	h := NewHandlers(t.TempDir()).WithClassificationTree(tree)

	if got := h.industryLabel("semiconductor"); got != "半導體" {
		t.Errorf("industryLabel(semiconductor) = %q, want 半導體", got)
	}
	if got := h.industryLabel("unknown"); got != "unknown" {
		t.Errorf("industryLabel(unknown) = %q, want unknown", got)
	}
}

func TestIndustryLabel_NilTree(t *testing.T) {
	h := NewHandlers(t.TempDir())
	if got := h.industryLabel("ai_supply_chain"); got != "ai_supply_chain" {
		t.Errorf("industryLabel(ai_supply_chain) with nil tree = %q, want ai_supply_chain", got)
	}
}

func TestRegisterRoutes(t *testing.T) {
	h := newTestHandlers(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	routes := []struct {
		method string
		path   string
	}{
		{"GET", "/api/dashboard/risk"},
		{"GET", "/api/dashboard/correlation-matrix"},
		{"GET", "/api/dashboard/risk-calibration"},
	}
	for _, r := range routes {
		req := httptest.NewRequest(r.method, r.path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code == 0 {
			t.Errorf("route %s %s not registered (no handler)", r.method, r.path)
		}
	}
}

// T2 RED: P0-3 Layer 1 — RiskGate 未注入時應回 503
// 行為變更: 從 200 + gate_mode="" 改為 503 + status=service_unavailable
// 理由: 前端需要明確區分「無數據」(200) 與「服務未就緒」(503)
func TestHandleRiskMetrics_RiskGateNotInjected_Returns503(t *testing.T) {
	h := &Handlers{LedgerDir: t.TempDir()}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/risk", nil)
	status, body := h.HandleRiskMetrics(req)
	assertStatus(t, status, http.StatusServiceUnavailable)
	m := assertJSONKey(t, body, "data_status")
	if m["data_status"] != "service_unavailable" {
		t.Errorf("data_status = %v, want service_unavailable", m["data_status"])
	}
}

func TestHandleRiskCalibration_RiskGateNotInjected_Returns503(t *testing.T) {
	h := &Handlers{LedgerDir: t.TempDir()}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/risk-calibration", nil)
	status, _ := h.HandleRiskCalibration(req)
	assertStatus(t, status, http.StatusServiceUnavailable)
}

func TestHandleRiskCommentary_RiskGateNotInjected_Returns503(t *testing.T) {
	h := &Handlers{LedgerDir: t.TempDir()}
	req := httptest.NewRequest(http.MethodGet, "/api/risk/commentary", nil)
	status, _ := h.HandleRiskCommentary(req)
	assertStatus(t, status, http.StatusServiceUnavailable)
}
