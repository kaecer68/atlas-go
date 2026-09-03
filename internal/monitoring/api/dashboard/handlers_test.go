package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/live"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// newTestHandlers builds a Handlers backed by a temp work directory.
// Channel state file is intentionally not pre-populated — tests create
// only the directories they need.
func newTestHandlers(t *testing.T) *Handlers {
	t.Helper()
	workDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, "data/state"), 0o755); err != nil {
		t.Fatalf("mkdir data/state: %v", err)
	}
	return &Handlers{
		WorkDir:   workDir,
		LedgerDir: workDir,
	}
}

// doRequest runs a handler and returns the parsed response map.
func doRequest(t *testing.T, status int, body any, expectedStatus int) map[string]any {
	t.Helper()
	if status != expectedStatus {
		t.Fatalf("expected status %d, got %d (body=%v)", expectedStatus, status, body)
	}
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T (%v)", body, body)
	}
	return resp
}

// ---------- HandleDataChannels ----------

func TestHandleDataChannels_HappyPath(t *testing.T) {
	h := newTestHandlers(t)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/data-channels", nil)
	status, body := h.HandleDataChannels(req)

	resp := doRequest(t, status, body, http.StatusOK)
	for _, key := range []string{"channels", "alerts", "generated"} {
		if _, ok := resp[key]; !ok {
			t.Errorf("missing required key %q in response: %v", key, resp)
		}
	}
	channels, ok := resp["channels"].([]service.DataChannel)
	if !ok {
		t.Fatalf("expected channels to be []service.DataChannel, got %T", resp["channels"])
	}
	if len(channels) == 0 {
		t.Error("expected at least one channel (real service builds 14 stubs)")
	}
	if channels[0].ChannelID == "" {
		t.Error("expected first channel to have non-empty ChannelID")
	}
}

func TestHandleDataChannels_IncludesAlertsSliceEvenWhenEmpty(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/data-channels", nil)
	status, body := h.HandleDataChannels(req)

	resp := doRequest(t, status, body, http.StatusOK)
	alerts, ok := resp["alerts"]
	if !ok {
		t.Fatal("expected 'alerts' key present (even if empty)")
	}
	if alerts == nil {
		t.Error("expected non-nil alerts slice (frontend iterates data.alerts)")
	}
	// generated should be RFC3339-formatted timestamp
	gen, _ := resp["generated"].(string)
	if gen == "" {
		t.Error("expected 'generated' to be a non-empty RFC3339 timestamp")
	}
}

// ---------- HandleRSITwCalibration ----------

func TestHandleRSITwCalibration_NotAvailable(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/rsi-tw-calibration", nil)
	status, body := h.HandleRSITwCalibration(req)

	resp := doRequest(t, status, body, http.StatusOK)
	if resp["status"] != "not_available" {
		t.Errorf("expected status=not_available, got %v", resp["status"])
	}
	if msg, _ := resp["message"].(string); msg == "" {
		t.Error("expected non-empty message for not_available path")
	}
}

func TestHandleRSITwCalibration_ReportLoaded(t *testing.T) {
	h := newTestHandlers(t)
	stateDir := filepath.Join(h.WorkDir, "data", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir data/state: %v", err)
	}
	report := map[string]any{
		"timestamp":    "2026-01-15T00:00:00Z",
		"sample_count": 42,
		"score":        0.78,
		"verdict":      "improved",
		"summary":      "ok",
		"changes":      []any{},
	}
	arr := []any{report}
	b, _ := json.Marshal(arr)
	path := filepath.Join(stateDir, "rsi_tw_calibration.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/rsi-tw-calibration", nil)
	status, body := h.HandleRSITwCalibration(req)

	resp := doRequest(t, status, body, http.StatusOK)
	if _, ok := resp["report"]; !ok {
		t.Errorf("expected 'report' key in response, got %v", resp)
	}
}

func TestHandleRSITwCalibration_InternalError(t *testing.T) {
	h := newTestHandlers(t)
	stateDir := filepath.Join(h.WorkDir, "data", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir data/state: %v", err)
	}
	path := filepath.Join(stateDir, "rsi_tw_calibration.json")
	if err := os.WriteFile(path, []byte("{not valid json at all"), 0o644); err != nil {
		t.Fatalf("write malformed report: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/rsi-tw-calibration", nil)
	status, body := h.HandleRSITwCalibration(req)

	resp := doRequest(t, status, body, http.StatusInternalServerError)
	if resp["status"] != "error" {
		t.Errorf("expected status=error, got %v", resp["status"])
	}
	if errMsg, _ := resp["error"].(string); errMsg == "" {
		t.Error("expected non-empty error message on internal error")
	}
}

// ---------- HandleMaturity ----------

func TestHandleMaturity_OK(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/maturity", nil)
	status, body := h.HandleMaturity(req)

	resp := doRequest(t, status, body, http.StatusOK)
	if _, ok := resp["phase"]; !ok {
		t.Errorf("expected 'phase' key in response, got %v", resp)
	}
	if _, ok := resp["days_since_start"]; !ok {
		t.Errorf("expected 'days_since_start' key in response, got %v", resp)
	}
	if _, ok := resp["thresholds"]; !ok {
		t.Errorf("expected 'thresholds' key in response, got %v", resp)
	}
}

// ---------- HandleDataChannelDetail ----------

func TestHandleDataChannelDetail_OK(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/data-channels/fugle", nil)
	req.SetPathValue("name", "fugle")

	status, body := h.HandleDataChannelDetail(req)
	if status != http.StatusOK {
		t.Fatalf("expected status 200, got %d (body=%v)", status, body)
	}
	channel, ok := body.(service.DataChannel)
	if !ok {
		t.Fatalf("expected service.DataChannel, got %T", body)
	}
	if channel.ChannelID != "fugle" {
		t.Errorf("expected channel_id=fugle, got %q", channel.ChannelID)
	}
}

func TestHandleDataChannelDetail_MissingName(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/data-channels/", nil)
	req.SetPathValue("name", "")

	status, body := h.HandleDataChannelDetail(req)
	if status != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d (body=%v)", status, body)
	}
	resp, ok := body.(map[string]string)
	if !ok {
		t.Fatalf("expected map[string]string, got %T", body)
	}
	if resp["error"] == "" {
		t.Errorf("expected error message for missing channel name, got %v", resp)
	}
}

func TestHandleDataChannelDetail_NotFound(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/data-channels/unknown-channel", nil)
	req.SetPathValue("name", "unknown-channel")

	status, body := h.HandleDataChannelDetail(req)
	if status != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d (body=%v)", status, body)
	}
	resp, ok := body.(map[string]string)
	if !ok {
		t.Fatalf("expected map[string]string, got %T", body)
	}
	if resp["error"] == "" {
		t.Errorf("expected error message for unknown channel, got %v", resp)
	}
}

func TestHandleJanusRegimeScore_EngineNotAvailable(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/janus/regime-score", nil)
	status, body := h.HandleJanusRegimeScore(req)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d (body=%v)", status, body)
	}
}

func TestHandleJanusRegimeScore_SyntheticFallback(t *testing.T) {
	h := newTestHandlers(t)
	engine := janus.NewEngine()
	engine.EnsureAllRegimes()
	engine.UpdateFromMacro(marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: 5.0},
		VIX:                marketdata.MacroDataPoint{Value: 15},
	})
	h.JanusEngine = engine

	req := httptest.NewRequest(http.MethodGet, "/api/janus/regime-score", nil)
	status, body := h.HandleJanusRegimeScore(req)
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%v)", status, body)
	}
	if resp["is_synthetic"] != true {
		t.Errorf("expected is_synthetic=true (synthesized fallback), got %v", resp["is_synthetic"])
	}
	score, ok := resp["score"].(float64)
	if !ok {
		t.Fatalf("expected float64 score, got %T (%v)", resp["score"], resp["score"])
	}
	if score < 20 || score > 25 {
		t.Errorf("expected score ~22.85 (tanh saturated), got %v", score)
	}
}

func TestHandleJanusRegimeScore_NoData(t *testing.T) {
	h := newTestHandlers(t)
	engine := janus.NewEngine()
	h.JanusEngine = engine

	req := httptest.NewRequest(http.MethodGet, "/api/janus/regime-score", nil)
	status, body := h.HandleJanusRegimeScore(req)
	resp, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("expected map response, got %T", body)
	}
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if resp["score"] != 0.0 {
		t.Errorf("expected score=0, got %v", resp["score"])
	}
	if resp["is_synthetic"] != false {
		t.Errorf("expected is_synthetic=false (no data at all), got %v", resp["is_synthetic"])
	}
}

// TestHandleOverview_NotWired verifies the overview endpoint reports 503
// until RegisterLiveRoutes bound the live handlers (SSOT P1-7).
func TestHandleOverview_NotWired(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/overview", nil)
	status, body := h.HandleOverview(req)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when Live is nil", status)
	}
	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("body = %T, want map", body)
	}
	if m["status"] != "service_unavailable" {
		t.Errorf("status key = %v, want service_unavailable", m["status"])
	}
}

// TestHandleOverview_Wired verifies the aggregate response carries the three
// sections with the standalone endpoints' shapes and honors the 60s TTL.
func TestHandleOverview_Wired(t *testing.T) {
	tmpDir := t.TempDir()
	ledgerDir := t.TempDir()

	// One session summary so risk/portfolio history is non-empty.
	sessDir := filepath.Join(ledgerDir, "sessions", "session-20260401-daily")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	summary := map[string]any{
		"session_id":      "session-20260401-daily",
		"portfolio_value": 1_000_000.0,
		"ending_cash":     200_000.0,
	}
	b, _ := json.Marshal(summary)
	if err := os.WriteFile(filepath.Join(sessDir, "summary.json"), b, 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	svc := service.NewLiveService(tmpDir, ledgerDir)
	liveHandlers := &live.Handlers{LedgerDir: ledgerDir, WorkDir: tmpDir, Svc: svc}

	h := newTestHandlers(t)
	h.Live = liveHandlers

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/overview", nil)
	status, body := h.HandleOverview(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	resp, ok := body.(*OverviewResponse)
	if !ok {
		t.Fatalf("body = %T, want *OverviewResponse", body)
	}
	if resp.LiveStatus == nil || resp.PortfolioState == nil || resp.RiskExposure == nil {
		t.Fatalf("overview missing sections: live=%v portfolio=%v risk=%v",
			resp.LiveStatus, resp.PortfolioState, resp.RiskExposure)
	}
	if resp.GeneratedAt.IsZero() {
		t.Error("generated_at must be set")
	}

	// 60s TTL: a second call within the window returns the identical cached
	// instance (same pointer) without rebuilding.
	status2, body2 := h.HandleOverview(req)
	if status2 != http.StatusOK {
		t.Fatalf("second status = %d, want 200", status2)
	}
	if body2 != body {
		t.Error("second call within 60s TTL must return the cached payload")
	}
}
