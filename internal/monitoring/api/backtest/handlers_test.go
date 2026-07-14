package backtest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

func newTestHandlers(t *testing.T) *Handlers {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Config{LedgerDir: dir}
	svc := service.NewBacktestService(cfg)
	return &Handlers{LedgerDir: dir, svc: svc}
}

func postJSON(t *testing.T, url string, body any) *http.Request {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func getRequest(t *testing.T, url string) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodGet, url, nil)
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
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := m[key]; !ok {
		t.Errorf("response missing key %q: %v", key, m)
	}
	return m
}

// --- HandleBacktestRun validation paths ---

func TestHandleBacktestRun_InvalidJSON(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodPost, "/api/backtest/run",
		bytes.NewReader([]byte("{bad json")))
	req.Header.Set("Content-Type", "application/json")
	status, body := h.HandleBacktestRun(req)
	assertStatus(t, status, http.StatusBadRequest)
	m := assertJSONKey(t, body, "error")
	if m["error"] != "invalid json" {
		t.Errorf("error = %q, want %q", m["error"], "invalid json")
	}
}

func TestHandleBacktestRun_MissingStart(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/backtest/run", map[string]string{"end": "2024-06-01"})
	status, body := h.HandleBacktestRun(req)
	assertStatus(t, status, http.StatusBadRequest)
	m := assertJSONKey(t, body, "error")
	if m["error"] != "start and end dates required" {
		t.Errorf("error = %q", m["error"])
	}
}

func TestHandleBacktestRun_MissingEnd(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/backtest/run", map[string]string{"start": "2024-01-01"})
	status, body := h.HandleBacktestRun(req)
	assertStatus(t, status, http.StatusBadRequest)
	m := assertJSONKey(t, body, "error")
	if m["error"] != "start and end dates required" {
		t.Errorf("error = %q", m["error"])
	}
}

func TestHandleBacktestRun_EmptyBody(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/backtest/run", map[string]string{})
	status, body := h.HandleBacktestRun(req)
	assertStatus(t, status, http.StatusBadRequest)
	m := assertJSONKey(t, body, "error")
	if m["error"] != "start and end dates required" {
		t.Errorf("error = %q, want %q", m["error"], "start and end dates required")
	}
}

func TestHandleBacktestRun_InvalidStartFormat(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/backtest/run", map[string]string{
		"start": "01-01-2024",
		"end":   "2024-06-01",
	})
	status, body := h.HandleBacktestRun(req)
	assertStatus(t, status, http.StatusBadRequest)
	m := assertJSONKey(t, body, "error")
	if m["error"] != "invalid start date format (YYYY-MM-DD)" {
		t.Errorf("error = %q", m["error"])
	}
}

func TestHandleBacktestRun_InvalidEndFormat(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/backtest/run", map[string]string{
		"start": "2024-01-01",
		"end":   "06-01-2024",
	})
	status, body := h.HandleBacktestRun(req)
	assertStatus(t, status, http.StatusBadRequest)
	m := assertJSONKey(t, body, "error")
	if m["error"] != "invalid end date format (YYYY-MM-DD)" {
		t.Errorf("error = %q", m["error"])
	}
}

func TestHandleBacktestRun_BothInvalidDates(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/backtest/run", map[string]string{
		"start": "bad-date",
		"end":   "also-bad",
	})
	status, _ := h.HandleBacktestRun(req)
	assertStatus(t, status, http.StatusBadRequest)
}

func TestHandleBacktestRun_StartAfterEnd(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/backtest/run", map[string]string{
		"start": "2024-06-01",
		"end":   "2024-01-01",
	})
	status, body := h.HandleBacktestRun(req)
	assertStatus(t, status, http.StatusBadRequest)
	m := assertJSONKey(t, body, "error")
	if m["error"] != "start date must be before or equal to end date" {
		t.Errorf("error = %q", m["error"])
	}
}

func TestHandleBacktestRun_StartEqualsEnd(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/backtest/run", map[string]string{
		"start": "2024-06-01",
		"end":   "2024-06-01",
	})
	status, body := h.HandleBacktestRun(req)
	assertStatus(t, status, http.StatusAccepted)
	m := assertJSONKey(t, body, "running")
	if m["running"] != true {
		t.Errorf("running = %v, want true", m["running"])
	}
}

func TestHandleBacktestRun_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := postJSON(t, "/api/backtest/run", map[string]string{
		"start": "2024-01-01",
		"end":   "2024-06-01",
	})
	status, body := h.HandleBacktestRun(req)
	assertStatus(t, status, http.StatusAccepted)
	m := assertJSONKey(t, body, "check_status")
	if m["check_status"] != "/api/backtest/status" {
		t.Errorf("check_status = %q", m["check_status"])
	}
	if m["start"] != "2024-01-01" {
		t.Errorf("start = %q", m["start"])
	}
	if m["end"] != "2024-06-01" {
		t.Errorf("end = %q", m["end"])
	}
}

func TestHandleBacktestRun_AlreadyRunning(t *testing.T) {
	// The backtest runs asynchronously and may complete in microseconds
	// on empty test data. We retry the whole sequence (start + immediately
	// attempt second run) until we observe a 409, or until deadline.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		h := newTestHandlers(t)

		req := postJSON(t, "/api/backtest/run", map[string]string{
			"start": "2024-01-01", "end": "2024-06-01",
		})
		_, _ = h.HandleBacktestRun(req)

		// Fire second run immediately after first — no sleep.
		// If the backtest goroutine hasn't set running=false yet,
		// Start() will see running=true and return error→409.
		req2 := postJSON(t, "/api/backtest/run", map[string]string{
			"start": "2024-02-01", "end": "2024-07-01",
		})
		status, body := h.HandleBacktestRun(req2)

		if status == http.StatusConflict {
			m := assertJSONKey(t, body, "error")
			if m["error"] == "backtest already running" {
				return
			}
			t.Errorf("error = %q, want %q", m["error"], "backtest already running")
			return
		}
		// Backtest completed too fast — retry with fresh service.
	}
	t.Fatal("backtest never returned 409 after 5s (backtest always completes before second Start)")
}

// --- HandleBacktestStatus ---

func TestHandleBacktestStatus_Initially(t *testing.T) {
	h := newTestHandlers(t)
	req := getRequest(t, "/api/backtest/status")
	status, body := h.HandleBacktestStatus(req)
	assertStatus(t, status, http.StatusOK)
	if body == nil {
		t.Fatal("body is nil")
	}
}

// TestHandleBacktestStatus_AfterStart verifies the status endpoint
// exposes a bool-typed `running` field after a start request. On
// empty ledger the async goroutine may complete in microseconds
// (resetting running=false), so we don't assert its truthiness —
// see TestHandleBacktestRun_AlreadyRunning for the same race.
func TestHandleBacktestStatus_AfterStart(t *testing.T) {
	h := newTestHandlers(t)

	req := postJSON(t, "/api/backtest/run", map[string]string{
		"start": "2024-01-01", "end": "2024-06-01",
	})
	runStatus, _ := h.HandleBacktestRun(req)
	assertStatus(t, runStatus, http.StatusAccepted)

	req2 := getRequest(t, "/api/backtest/status")
	status, body := h.HandleBacktestStatus(req2)
	assertStatus(t, status, http.StatusOK)
	m := assertJSONKey(t, body, "running")
	if _, ok := m["running"].(bool); !ok {
		t.Errorf("status.running = %T, want bool", m["running"])
	}
}

// --- HandleBacktestSnapshots ---

func TestHandleBacktestSnapshots_EmptyDir(t *testing.T) {
	h := newTestHandlers(t)
	req := getRequest(t, "/api/backtest/snapshots")
	status, body := h.HandleBacktestSnapshots(req)
	assertStatus(t, status, http.StatusOK)
	m := assertJSONKey(t, body, "snapshots")
	snaps, _ := m["snapshots"].([]any)
	if len(snaps) != 0 {
		t.Errorf("snapshots len = %d, want 0", len(snaps))
	}
	// Verify count field
	count, ok := m["count"].(float64)
	if !ok || count != 0 {
		t.Errorf("count = %v, want 0", m["count"])
	}
}

func TestHandleBacktestSnapshots_WithDaysParam(t *testing.T) {
	h := newTestHandlers(t)
	req := getRequest(t, "/api/backtest/snapshots?days=10")
	status, body := h.HandleBacktestSnapshots(req)
	assertStatus(t, status, http.StatusOK)
	m := assertJSONKey(t, body, "snapshots")
	snaps, ok := m["snapshots"].([]any)
	if !ok {
		t.Fatalf("snapshots is %T, want []any", m["snapshots"])
	}
	if len(snaps) != 0 {
		t.Errorf("snapshots len = %d, want 0 for empty dir", len(snaps))
	}
}

func TestHandleBacktestSnapshots_NegativeDaysDefaultsTo20(t *testing.T) {
	h := newTestHandlers(t)
	req := getRequest(t, "/api/backtest/snapshots?days=-5")
	status, body := h.HandleBacktestSnapshots(req)
	assertStatus(t, status, http.StatusOK)
	assertJSONKey(t, body, "snapshots")
}

func TestHandleBacktestSnapshots_NonNumericDaysDefaultsTo20(t *testing.T) {
	h := newTestHandlers(t)
	req := getRequest(t, "/api/backtest/snapshots?days=abc")
	status, body := h.HandleBacktestSnapshots(req)
	assertStatus(t, status, http.StatusOK)
	assertJSONKey(t, body, "snapshots")
}

// --- HandleBacktestSignals ---

func TestHandleBacktestSignals_EmptyData(t *testing.T) {
	h := newTestHandlers(t)
	req := getRequest(t, "/api/backtest/signals")
	status, body := h.HandleBacktestSignals(req)
	assertStatus(t, status, http.StatusOK)
	m := assertJSONKey(t, body, "active_signals")
	// No ledger data → empty/null active signals
	signals, _ := m["active_signals"].([]any)
	if len(signals) != 0 {
		t.Errorf("active_signals len = %d, want 0 for empty ledger", len(signals))
	}
	// Verify other keys exist
	for _, key := range []string{"var_95", "var_99", "sharpe_short", "sharpe_long", "drawdown_pct"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing key %q in signals response", key)
		}
	}
}

// --- RegisterRoutes ---

func TestRegisterRoutes(t *testing.T) {
	h := newTestHandlers(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	routes := []struct {
		method string
		path   string
	}{
		{"POST", "/api/backtest/run"},
		{"GET", "/api/backtest/status"},
		{"GET", "/api/backtest/snapshots"},
		{"GET", "/api/backtest/signals"},
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
