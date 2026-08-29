package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func writeFetchLog(t *testing.T, dir string, entries []map[string]any) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	b, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, fetchLogFileName), b, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestHandleChannelFetchLog_Empty(t *testing.T) {
	h := &Handlers{WorkDir: t.TempDir()}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/channel-fetch-log", nil)
	status, data := h.HandleChannelFetchLog(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	m, ok := data.(map[string]any)
	if !ok {
		t.Fatalf("data type = %T, want map[string]any", data)
	}
	if m["empty_state"] == nil {
		t.Errorf("empty_state missing")
	}
	entries, ok := m["entries"].([]map[string]any)
	if !ok {
		t.Fatalf("entries type = %T, want []map[string]any", m["entries"])
	}
	if len(entries) != 0 {
		t.Errorf("entries len = %d, want 0", len(entries))
	}
}

func TestHandleChannelFetchLog_InvalidLimit(t *testing.T) {
	h := &Handlers{WorkDir: t.TempDir()}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/channel-fetch-log?limit=abc", nil)
	status, _ := h.HandleChannelFetchLog(req)
	if status != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
}

func TestHandleChannelFetchLog_ClampsLimitToMax(t *testing.T) {
	h := &Handlers{WorkDir: t.TempDir()}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/channel-fetch-log?limit=500", nil)
	status, _ := h.HandleChannelFetchLog(req)
	if status != http.StatusOK {
		t.Errorf("status = %d, want 200 (clamped)", status)
	}
}

func TestHandleChannelFetchLog_ReturnsEntries(t *testing.T) {
	dir := t.TempDir()
	writeFetchLog(t, filepath.Join(dir, "data/state"), []map[string]any{
		{"channel": "us_yahoo", "status": "ok", "latency_ms": 234, "timestamp": "2026-06-19T14:32:01Z"},
		{"channel": "finmind", "status": "error", "latency_ms": 1200, "timestamp": "2026-06-19T14:30:00Z", "error": "timeout"},
	})
	h := &Handlers{WorkDir: dir}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/channel-fetch-log?limit=10", nil)
	status, data := h.HandleChannelFetchLog(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	m := data.(map[string]any)
	entries := m["entries"].([]map[string]any)
	if len(entries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(entries))
	}
	if entries[0]["channel"] != "us_yahoo" {
		t.Errorf("entries[0].channel = %v, want us_yahoo", entries[0]["channel"])
	}
	if entries[0]["time"] != "14:32:01" {
		t.Errorf("entries[0].time = %v, want 14:32:01", entries[0]["time"])
	}
	if entries[0]["latency"] != "234ms" {
		t.Errorf("entries[0].latency = %v, want 234ms", entries[0]["latency"])
	}
	if entries[1]["channel"] != "finmind" {
		t.Errorf("entries[1].channel = %v, want finmind", entries[1]["channel"])
	}
	if entries[1]["latency"] != "1.2s" {
		t.Errorf("entries[1].latency = %v, want 1.2s", entries[1]["latency"])
	}
	if m["count"] != 2 {
		t.Errorf("count = %v, want 2", m["count"])
	}
}

func TestHandleChannelFetchLog_RespectsLimit(t *testing.T) {
	dir := t.TempDir()
	all := make([]map[string]any, 0, 5)
	for i := range 5 {
		all = append(all, map[string]any{
			"channel": "ch", "status": "ok", "latency_ms": i, "timestamp": "2026-06-19T14:00:0" + string(rune('0'+i)) + "Z",
		})
	}
	writeFetchLog(t, filepath.Join(dir, "data/state"), all)
	h := &Handlers{WorkDir: dir}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/channel-fetch-log?limit=2", nil)
	_, data := h.HandleChannelFetchLog(req)
	entries := data.(map[string]any)["entries"].([]map[string]any)
	if len(entries) != 2 {
		t.Fatalf("entries len = %d, want 2", len(entries))
	}
	if entries[0]["latency"] != "3ms" {
		t.Errorf("entries[0].latency = %v, want 3ms (newest 2 of 5)", entries[0]["latency"])
	}
}

func TestHandleChannelFetchLog_FileMissing_NotError(t *testing.T) {
	h := &Handlers{WorkDir: t.TempDir()}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/channel-fetch-log", nil)
	status, data := h.HandleChannelFetchLog(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (file missing is empty_state, not error)", status)
	}
	m := data.(map[string]any)
	if _, ok := m["empty_state"]; !ok {
		t.Errorf("expected empty_state when file missing")
	}
}
