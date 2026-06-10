package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// newTestHandlers builds a Handlers backed by a temp work directory.
// Channel state file is intentionally not pre-populated — tests create
// only the directories they need.
func newTestHandlers(t *testing.T) *Handlers {
	t.Helper()
	workDir := t.TempDir()
	// Pre-create the data/state directory so channel toggle writes don't
	// race with directory creation in channel_state.go.
	if err := os.MkdirAll(filepath.Join(workDir, "data/state"), 0o755); err != nil {
		t.Fatalf("mkdir data/state: %v", err)
	}
	return &Handlers{
		WorkDir:       workDir,
		LedgerDir:     workDir,
		channelStates: make(map[string]channelState),
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
	channels, ok := resp["channels"]
	_ = ok
	_ = channels
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

// ---------- HandleChannelAction ----------

func TestHandleChannelAction_ToggleValid(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/channels/fugle/toggle",
		bytes.NewBufferString(`{"enabled":true}`))
	status, body := h.HandleChannelAction(req)

	resp := doRequest(t, status, body, http.StatusOK)
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", resp["status"])
	}
	if resp["enabled"] != true {
		t.Errorf("expected enabled=true, got %v", resp["enabled"])
	}
	if resp["channel_id"] != "fugle" {
		t.Errorf("expected channel_id=fugle, got %v", resp["channel_id"])
	}

	// Verify state file was persisted.
	path := filepath.Join(h.WorkDir, "data/state/channels.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read channels.json: %v", err)
	}
	var stored map[string]channelState
	if err := json.Unmarshal(b, &stored); err != nil {
		t.Fatalf("unmarshal channels.json: %v", err)
	}
	if stored["fugle"].Enabled != true {
		t.Errorf("expected persisted state for fugle=true, got %+v", stored["fugle"])
	}
}

func TestHandleChannelAction_ToggleDisable(t *testing.T) {
	h := newTestHandlers(t)
	// Pre-seed the state file with fugle=true.
	seed := map[string]channelState{
		"fugle": {Enabled: true},
	}
	b, _ := json.Marshal(seed)
	path := filepath.Join(h.WorkDir, "data/state/channels.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("seed channels.json: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/channels/fugle/toggle",
		bytes.NewBufferString(`{"enabled":false}`))
	status, body := h.HandleChannelAction(req)

	resp := doRequest(t, status, body, http.StatusOK)
	if resp["enabled"] != false {
		t.Errorf("expected enabled=false, got %v", resp["enabled"])
	}
}

func TestHandleChannelAction_TriggerStub(t *testing.T) {
	// STUB-LOCK: trigger is a pure stub that always returns status=ok.
	// This test pins that contract — do not "fix" it to actually trigger fetches.
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/channels/fugle/trigger", nil)
	status, body := h.HandleChannelAction(req)

	resp := doRequest(t, status, body, http.StatusOK)
	if resp["status"] != "ok" {
		t.Errorf("trigger stub: expected status=ok, got %v", resp["status"])
	}
	if resp["action"] != "trigger" {
		t.Errorf("trigger stub: expected action=trigger, got %v", resp["action"])
	}
}

func TestHandleChannelAction_InvalidPath(t *testing.T) {
	h := newTestHandlers(t)
	// URL with no action segment — too few parts.
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/channels/", nil)
	// Go's net/http will not match mux pattern for trailing slash, but our
	// path-stripping logic handles "/api/dashboard/channels/" by stripping the
	// prefix and splitting "" → [""] (len 1, no action). Return 400.
	status, body := h.HandleChannelAction(req)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for path with no action, got %d (body=%v)", status, body)
	}
}

func TestHandleChannelAction_UnknownAction(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/channels/fugle/yolo", nil)
	status, body := h.HandleChannelAction(req)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown action, got %d (body=%v)", status, body)
	}
}

func TestHandleChannelAction_ToggleMalformedBody(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/channels/fugle/toggle",
		bytes.NewBufferString(`{not valid json`))
	status, body := h.HandleChannelAction(req)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed body, got %d (body=%v)", status, body)
	}
}

// ---------- HandleAPIKeyUpdate ----------

func TestHandleAPIKeyUpdate_Valid(t *testing.T) {
	// t.Setenv auto-restores — safer than manual t.Cleanup with os.Setenv.
	t.Setenv("FINMIND_API_KEY", "original-finmind-key-12345")

	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/api-keys/update",
		bytes.NewBufferString(`{"provider":"finmind","api_key":"new-finmind-key-67890"}`))
	status, body := h.HandleAPIKeyUpdate(req)

	resp := doRequest(t, status, body, http.StatusOK)
	if resp["provider"] != "finmind" {
		t.Errorf("expected provider=finmind, got %v", resp["provider"])
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", resp["status"])
	}
	if got := os.Getenv("FINMIND_API_KEY"); got != "new-finmind-key-67890" {
		t.Errorf("expected FINMIND_API_KEY=updated, got %q", got)
	}
}

func TestHandleAPIKeyUpdate_MissingFields(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/api-keys/update",
		bytes.NewBufferString(`{"provider":"finmind"}`))
	status, body := h.HandleAPIKeyUpdate(req)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing api_key, got %d (body=%v)", status, body)
	}
}

func TestHandleAPIKeyUpdate_MalformedJSON(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/api-keys/update",
		bytes.NewBufferString(`not json at all`))
	status, body := h.HandleAPIKeyUpdate(req)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed JSON, got %d (body=%v)", status, body)
	}
}

func TestHandleAPIKeyUpdate_InvalidProvider(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/api-keys/update",
		bytes.NewBufferString(`{"provider":"not-a-real-provider","api_key":"abcdefghijkl"}`))
	status, body := h.HandleAPIKeyUpdate(req)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid provider, got %d (body=%v)", status, body)
	}
}

func TestHandleAPIKeyUpdate_KeyLengthOutOfRange(t *testing.T) {
	h := newTestHandlers(t)
	// Too short (< 8 chars).
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/api-keys/update",
		bytes.NewBufferString(`{"provider":"fugle","api_key":"short"}`))
	status, body := h.HandleAPIKeyUpdate(req)
	if status != http.StatusBadRequest {
		t.Fatalf("expected 400 for too-short key, got %d (body=%v)", status, body)
	}
}

func TestHandleAPIKeyUpdate_EnvLeakPrevented(t *testing.T) {
	// Regression guard: t.Setenv in TestHandleAPIKeyUpdate_Valid above
	// must not leak to subsequent tests. This test starts with the OS default
	// (unset or whatever was there before) and verifies it doesn't see the
	// "new-finmind-key-67890" value the earlier test wrote.
	if got := os.Getenv("FINMIND_API_KEY"); got == "new-finmind-key-67890" {
		t.Errorf("ENV LEAK: FINMIND_API_KEY still set to value written by earlier test: %q", got)
	}
}

// ---------- HandleRSITwCalibration ----------

func TestHandleRSITwCalibration_NotAvailable(t *testing.T) {
	// WorkDir has no .retail/rsi_tw_calibration.json — expected fresh-state path.
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
	// Seed a valid calibration report and verify the handler returns it.
	h := newTestHandlers(t)
	// Calibration report lives at <workDir>/data/state/rsi_tw_calibration.json
	retailDir := filepath.Join(h.WorkDir, "data", "state")
	if err := os.MkdirAll(retailDir, 0o755); err != nil {
		t.Fatalf("mkdir .retail: %v", err)
	}
	// The exact file format: array of CalibrationReport.
	// We just need a parseable JSON array — the loader reads most-recent.
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
	path := filepath.Join(retailDir, "rsi_tw_calibration.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/rsi-tw-calibration", nil)
	status, body := h.HandleRSITwCalibration(req)

	resp := doRequest(t, status, body, http.StatusOK)
	// When the report is found, the response wraps it under "report".
	if _, ok := resp["report"]; !ok {
		t.Errorf("expected 'report' key in response, got %v", resp)
	}
}

// ---------- HandleChannelsIngest (STUB-LOCK) ----------
// STUB-LOCK: current behavior is geo_ok: false. Fix tracked separately.
// The following test lives in the macro package — see macro/handlers_stub_test.go.
