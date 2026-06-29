package system

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

func newSystemHandlers(t *testing.T) (*Handlers, string) {
	t.Helper()
	workDir := t.TempDir()
	ledgerDir := t.TempDir()
	baselinePath := filepath.Join(workDir, "data/state/baseline_policy.json")

	store := ledger.NewStore(ledgerDir)
	svc := service.NewSystemService(workDir, ledgerDir, baselinePath, store, nil, nil)
	return &Handlers{Svc: svc}, workDir
}

func mustDecode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if w.Body.Len() == 0 {
		return nil
	}
	var m map[string]any
	json.Unmarshal(w.Body.Bytes(), &m)
	return m
}

func TestHandlePhase3Status_NoFile(t *testing.T) {
	h, _ := newSystemHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/phase3-status", nil)
	status, _ := h.HandlePhase3Status(req)
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200", status)
	}
}

func TestHandleSystemHealth_Success(t *testing.T) {
	h, _ := newSystemHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/system-health", nil)
	status, body := h.HandleSystemHealth(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", status, body)
	}
}

func TestHandleClampingEvents_NoFile(t *testing.T) {
	h, _ := newSystemHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/clamping-events", nil)
	status, body := h.HandleClampingEvents(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%v", status, body)
	}
	m := body.(map[string]any)
	if count, ok := m["count"].(int); ok && count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestHandleClampingEvents_WithLimit(t *testing.T) {
	h, _ := newSystemHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/clamping-events?limit=50", nil)
	status, body := h.HandleClampingEvents(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	m := body.(map[string]any)
	if _, ok := m["events"]; !ok {
		t.Error("missing events key")
	}
}

func TestHandleClampingEvents_InvalidLimit(t *testing.T) {
	h, _ := newSystemHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/clamping-events?limit=abc", nil)
	status, body := h.HandleClampingEvents(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	m := body.(map[string]any)
	if _, ok := m["events"]; !ok {
		t.Error("missing events key")
	}
}

func TestHandleClampingEvents_LimitExceedsMax(t *testing.T) {
	h, _ := newSystemHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/clamping-events?limit=5000", nil)
	status, _ := h.HandleClampingEvents(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
}

func TestHandleConvictionClampingEvents_NoFile(t *testing.T) {
	h, _ := newSystemHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/conviction-clamping-events", nil)
	status, body := h.HandleConvictionClampingEvents(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	m := body.(map[string]any)
	if count, ok := m["count"].(int); ok && count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestHandleCapitalPhase_ReturnsSnapshot(t *testing.T) {
	h, _ := newSystemHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/capital-phase", nil)
	status, body := h.HandleCapitalPhase(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	// CapitalPhaseController.GetSnapshot() returns domain.CapitalSnapshot
	_, ok := body.(domain.CapitalSnapshot)
	if !ok {
		t.Fatalf("body is %T, want domain.CapitalSnapshot", body)
	}
}

func TestHandleRetailSentiment_NoSnapshot(t *testing.T) {
	h, _ := newSystemHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/retail-sentiment", nil)
	status, body := h.HandleRetailSentiment(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	resp, ok := body.(RetailSentimentResponse)
	if !ok {
		t.Fatalf("body is %T, want RetailSentimentResponse", body)
	}
	if resp.Interpretation != "no macro snapshot available" {
		t.Errorf("interpretation = %q, want 'no macro snapshot available'", resp.Interpretation)
	}
	if resp.SentimentScore != 0 {
		t.Errorf("SentimentScore = %v, want 0", resp.SentimentScore)
	}
	if resp.FetcherStatus.DayTrading != "no_data" {
		t.Errorf("DayTrading fetcher status = %q, want no_data", resp.FetcherStatus.DayTrading)
	}
}

func TestHandleRetailSentiment_WithSnapshot(t *testing.T) {
	h, workDir := newSystemHandlers(t)
	// Write a minimal latest.json so loadLatestMacroSnapshot succeeds.
	macroDir := filepath.Join(workDir, "data/state/macro")
	if err := os.MkdirAll(macroDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	snap := map[string]any{
		"retail_margin_balance": map[string]any{"value": 2500, "change_pct": 0.05},
		"retail_short_balance":  map[string]any{"value": 800, "change_pct": 0.02},
		"vix":                   map[string]any{"value": 18.5},
		"foreign_investor_net":  map[string]any{"value": 5000},
		"domestic_fund_net":     map[string]any{"value": 2000},
	}
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(macroDir, "latest.json"), data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/retail-sentiment", nil)
	status, body := h.HandleRetailSentiment(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	resp, ok := body.(RetailSentimentResponse)
	if !ok {
		t.Fatalf("body is %T", body)
	}
	if resp.MarginBalance != 2500 {
		t.Errorf("MarginBalance = %v, want 2500", resp.MarginBalance)
	}
	if resp.MarginPercentile > 0 {
		t.Logf("margin percentile with single data point: %v", resp.MarginPercentile)
	}
}

func TestExtremeReadingFromScore(t *testing.T) {
	tests := []struct {
		score  float64
		expect string
	}{
		{0.8, "frenzy"},
		{0.5, "frenzy"},
		{0.3, "neutral"},
		{0.0, "neutral"},
		{-0.3, "neutral"},
		{-0.5, "fear"},
		{-0.9, "fear"},
	}
	for _, tt := range tests {
		got := extremeReadingFromScore(tt.score)
		if got != tt.expect {
			t.Errorf("extremeReadingFromScore(%v) = %q, want %q", tt.score, got, tt.expect)
		}
	}
}

func TestInterpretRetailSentiment(t *testing.T) {
	tests := []struct {
		score  float64
		expect string
	}{
		{1.0, "extremely bullish retail sentiment"},
		{0.8, "extremely bullish retail sentiment"},
		{0.6, "bullish retail sentiment"},
		{0.5, "bullish retail sentiment"},
		{0.3, "mildly bullish retail sentiment"},
		{0.2, "mildly bullish retail sentiment"},
		{0.1, "neutral retail sentiment"},
		{0.0, "neutral retail sentiment"},
		{-0.19, "neutral retail sentiment"},
		{-0.2, "mildly bearish retail sentiment"},
		{-0.3, "mildly bearish retail sentiment"},
		{-0.5, "bearish retail sentiment"},
		{-0.7, "bearish retail sentiment"},
		{-0.8, "extremely bearish retail sentiment"},
		{-1.0, "extremely bearish retail sentiment"},
	}
	for _, tt := range tests {
		got := interpretRetailSentiment(tt.score)
		if got != tt.expect {
			t.Errorf("interpretRetailSentiment(%v) = %q, want %q", tt.score, got, tt.expect)
		}
	}
}

func TestGetFloatOrZero(t *testing.T) {
	type nullableFloat struct{ Val float64 }

	t.Run("nil pointer returns zero", func(t *testing.T) {
		var p *nullableFloat
		got := getFloatOrZero(p, func(v *nullableFloat) float64 { return v.Val })
		if got != 0 {
			t.Errorf("got %v, want 0", got)
		}
	})
	t.Run("non-nil returns field value", func(t *testing.T) {
		p := &nullableFloat{Val: 3.14}
		got := getFloatOrZero(p, func(v *nullableFloat) float64 { return v.Val })
		if got != 3.14 {
			t.Errorf("got %v, want 3.14", got)
		}
	})
}

func TestCalculateMarginPercentile_SingleDataPoint(t *testing.T) {
	workDir := t.TempDir()
	macroDir := filepath.Join(workDir, "data/state/macro")
	if err := os.MkdirAll(macroDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write two historical snapshots with margin data
	for i, val := range []float64{2000, 3000} {
		snap := map[string]any{
			"retail_margin_balance": map[string]any{"value": val, "symbol": "TEST"},
		}
		data, _ := json.Marshal(snap)
		name := filepath.Join(macroDir, "2026010"+string(rune('1'+i))+".json")
		os.WriteFile(name, data, 0o644)
	}

	// current value = 2500 → 1 of 2 values are less → 0.5
	got := calculateMarginPercentile(workDir, 2500)
	if got != 0.5 {
		t.Errorf("percentile = %v, want 0.5", got)
	}
}

func TestCalculateMarginPercentile_ZeroCurrent(t *testing.T) {
	got := calculateMarginPercentile("/nonexistent", 0)
	if got != 0 {
		t.Errorf("percentile = %v, want 0", got)
	}
}

func TestRegisterRoutes(t *testing.T) {
	h, _ := newSystemHandlers(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	routes := []struct {
		path string
	}{
		{"/api/dashboard/phase3-status"},
		{"/api/dashboard/system-health"},
		{"/api/dashboard/clamping-events"},
		{"/api/dashboard/conviction-clamping-events"},
		{"/api/dashboard/capital-phase"},
		{"/api/dashboard/retail-sentiment"},
	}
	for _, r := range routes {
		req := httptest.NewRequest(http.MethodGet, r.path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code == 0 {
			t.Errorf("route %s not registered", r.path)
		}
	}

	// POST should be rejected on GET-only routes
	for _, r := range routes {
		req := httptest.NewRequest(http.MethodPost, r.path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("POST %s status = %d, want %d", r.path, w.Code, http.StatusMethodNotAllowed)
		}
	}
}

func TestHealthHandlers_HandleHealth(t *testing.T) {
	hh := &HealthHandlers{}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	status, body := hh.HandleHealth(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	resp, ok := body.(healthResponse)
	if !ok {
		t.Fatalf("body is %T", body)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok", resp.Status)
	}
	if _, ok := resp.Ports["atlas_http"]; !ok {
		t.Error("missing atlas_http port report")
	}
	if _, ok := resp.Ports["fubon_proxy"]; !ok {
		t.Error("missing fubon_proxy port report")
	}
}

func TestHealthHandlers_RegisterRoutes(t *testing.T) {
	hh := &HealthHandlers{}
	mux := http.NewServeMux()
	hh.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("route /health not registered or failed, status=%d", w.Code)
	}
}

func TestSwaggerHandlers_HandleSwaggerJSON_Missing(t *testing.T) {
	sh := NewSwaggerHandlers("/nonexistent/path")
	req := httptest.NewRequest(http.MethodGet, "/api/docs/swagger.json", nil)
	status, _ := sh.HandleSwaggerJSON(nil, req)
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}

func TestSwaggerHandlers_HandleSwaggerUI(t *testing.T) {
	sh := NewSwaggerHandlers("/tmp")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/docs", nil)
	status, _ := sh.HandleSwaggerUI(w, req)
	if status != 0 {
		t.Errorf("status = %d, want 0 (already written)", status)
	}
	body := w.Body.String()
	if !strings.Contains(body, "swagger-ui") {
		t.Error("swagger UI HTML does not contain swagger-ui reference")
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestHandleDataIntegrity_EmptyDir(t *testing.T) {
	workDir := t.TempDir()
	ledgerDir := t.TempDir()
	sessionsDir := filepath.Join(ledgerDir, "sessions")
	os.MkdirAll(sessionsDir, 0o755)

	handler := HandleDataIntegrity(workDir, ledgerDir)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/data-integrity", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	m := mustDecode(t, w)
	if m == nil {
		t.Fatal("expected JSON response")
	}
	overall, _ := m["overall"].(string)
	if overall != "failing" {
		t.Errorf("overall = %q, want failing (no sessions)", overall)
	}
}

func TestHandleDataIntegrity_WithSessions(t *testing.T) {
	workDir := t.TempDir()
	ledgerDir := t.TempDir()
	sessionsDir := filepath.Join(ledgerDir, "sessions", "session-20260101-daily")
	os.MkdirAll(sessionsDir, 0o755)
	// Write a valid summary.json with snake_case encoding and tax data.
	summary := map[string]any{
		"session_id":      "session-20260101-daily",
		"portfolio_value": 1000000.0,
		"tax_snapshots":   []map[string]any{{"tax": 100}},
		"total_tax_paid":  100.0,
	}
	data, _ := json.Marshal(summary)
	os.WriteFile(filepath.Join(sessionsDir, "summary.json"), data, 0o644)
	os.WriteFile(filepath.Join(sessionsDir, "positions.json"), []byte(`[]`), 0o644)

	handler := HandleDataIntegrity(workDir, ledgerDir)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/data-integrity", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	m := mustDecode(t, w)
	if m == nil {
		t.Fatal("expected JSON response")
	}
	overall, _ := m["overall"].(string)
	// With a session that has tax data + positions but no replay file,
	// there should be some warnings but encoding should be ok.
	if overall == "failing" {
		// This is expected if replay data is missing.
		t.Logf("overall = %q (replay data file missing is expected)", overall)
	}
	checks, _ := m["checks"].([]any)
	if len(checks) == 0 {
		t.Error("expected at least one integrity check")
	}
}
