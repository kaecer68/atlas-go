package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// =====================================================================
// LoadChannelStates
// =====================================================================

func TestLoadChannelStates_FileDoesNotExist(t *testing.T) {
	h := &Handlers{channelStates: make(map[string]channelState)}
	dir := t.TempDir()
	h.WorkDir = dir
	h.LoadChannelStates()
	if len(h.channelStates) != 0 {
		t.Errorf("expected empty state when file does not exist, got %d", len(h.channelStates))
	}
}

func TestLoadChannelStates_ValidFile(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "data", "state", "channels.json")
	if err := os.MkdirAll(filepath.Dir(stateFile), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	seed := map[string]channelState{
		"twse":    {Enabled: true},
		"finmind": {Enabled: false},
	}
	b, _ := json.Marshal(seed)
	if err := os.WriteFile(stateFile, b, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	h := &Handlers{channelStates: make(map[string]channelState)}
	h.WorkDir = dir
	h.LoadChannelStates()

	if len(h.channelStates) != 2 {
		t.Errorf("expected 2 states, got %d", len(h.channelStates))
	}
	if !h.channelStates["twse"].Enabled {
		t.Error("expected twse to be enabled")
	}
	if h.channelStates["finmind"].Enabled {
		t.Error("expected finmind to be disabled")
	}
}

func TestLoadChannelStates_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "data", "state", "channels.json")
	if err := os.MkdirAll(filepath.Dir(stateFile), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(stateFile, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	h := &Handlers{channelStates: make(map[string]channelState)}
	h.WorkDir = dir
	h.LoadChannelStates()
	if len(h.channelStates) != 0 {
		t.Errorf("expected empty state for invalid JSON, got %d", len(h.channelStates))
	}
}

// =====================================================================
// setChannelEnabled + saveChannelStates
// =====================================================================

func TestSetChannelEnabled_SavesState(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "data", "state"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	h := &Handlers{channelStates: make(map[string]channelState)}
	h.WorkDir = dir
	h.setChannelEnabled("fugle", true)

	if !h.channelStates["fugle"].Enabled {
		t.Error("expected fugle to be enabled in memory")
	}
	stateFile := filepath.Join(dir, "data", "state", "channels.json")
	b, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	var stored map[string]channelState
	if err := json.Unmarshal(b, &stored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !stored["fugle"].Enabled {
		t.Error("expected fugle to be enabled in persisted file")
	}
}

// =====================================================================
// HandleDrawdown
// =====================================================================

type nilDrawdownProvider struct{}

func (nilDrawdownProvider) GetLatestDrawdown() *portfolio.DrawdownResult { return nil }

func TestHandleDrawdown_NotAvailable(t *testing.T) {
	h := &Handlers{DrawdownProvider: nilDrawdownProvider{}}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/drawdown", nil)

	status, body := h.HandleDrawdown(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, body)
	}
	resp := body.(map[string]any)
	if resp["status"] != "not_available" {
		t.Errorf("expected status=not_available, got %v", resp["status"])
	}
	if _, ok := resp["generated"]; !ok {
		t.Error("expected generated timestamp")
	}
}

type mockDrawdownProvider struct {
	result *portfolio.DrawdownResult
}

func (m mockDrawdownProvider) GetLatestDrawdown() *portfolio.DrawdownResult { return m.result }

func TestHandleDrawdown_WithResult(t *testing.T) {
	h := &Handlers{
		DrawdownProvider: mockDrawdownProvider{
			result: &portfolio.DrawdownResult{
				MaxDrawdown: -0.15,
				VaR95:       -0.08,
				WorstPath:   []float64{-0.05, -0.10, -0.15},
			},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/drawdown", nil)

	status, body := h.HandleDrawdown(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, body)
	}
	resp := body.(map[string]any)
	if resp["status"] != nil {
		t.Errorf("expected no status key when result present, got %v", resp["status"])
	}
	if resp["max_drawdown"] != -0.15 {
		t.Errorf("expected max_drawdown=-0.15, got %v", resp["max_drawdown"])
	}
	if resp["var_95"] != -0.08 {
		t.Errorf("expected var_95=-0.08, got %v", resp["var_95"])
	}
}

// =====================================================================
// HandleDataPipeline
// =====================================================================

func TestHandleDataPipeline_HappyPath(t *testing.T) {
	dir := t.TempDir()
	ledgerDir := t.TempDir()
	h := &Handlers{WorkDir: dir, LedgerDir: ledgerDir}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/data-pipeline", nil)

	status, body := h.HandleDataPipeline(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, body)
	}
	resp := body.(map[string]any)
	if _, ok := resp["sources"]; !ok {
		t.Error("expected sources key")
	}
	if _, ok := resp["generated"]; !ok {
		t.Error("expected generated timestamp")
	}
}

// =====================================================================
// HandleSimLatest
// =====================================================================

func TestHandleSimLatest_NoTraces(t *testing.T) {
	dir := t.TempDir()
	h := &Handlers{WorkDir: dir}
	req := httptest.NewRequest(http.MethodGet, "/api/traces/sim-latest", nil)

	status, body := h.HandleSimLatest(req)
	if status != http.StatusOK {
		t.Fatalf("expected 200 for no traces, got %d: %v", status, body)
	}
	if body == nil {
		t.Fatal("expected non-nil body")
	}
	switch v := body.(type) {
	case []any:
		if len(v) != 0 {
			t.Errorf("expected 0 records, got %d", len(v))
		}
	default:
		t.Logf("records type: %T", body)
	}
}

// =====================================================================
// NewHandlers
// =====================================================================

func TestNewHandlers_InitializesChannelStates(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "data", "state"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	stateFile := filepath.Join(dir, "data", "state", "channels.json")
	seed := map[string]channelState{"twse": {Enabled: true}}
	b, _ := json.Marshal(seed)
	if err := os.WriteFile(stateFile, b, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	h := NewHandlers(dir, dir)
	if len(h.channelStates) != 1 {
		t.Errorf("expected 1 channel state, got %d", len(h.channelStates))
	}
	if !h.channelStates["twse"].Enabled {
		t.Error("expected twse enabled from persisted state")
	}
}

func TestNewHandlers_NonExistentWorkDir(t *testing.T) {
	// Should not panic, just start with empty states.
	h := NewHandlers("/nonexistent/work/dir", "/nonexistent/ledger/dir")
	if h == nil {
		t.Fatal("expected non-nil handlers")
	}
	if len(h.channelStates) != 0 {
		t.Errorf("expected 0 channel states for nonexistent dir, got %d", len(h.channelStates))
	}
}
