package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// =====================================================================
// LoadChannelStates
// =====================================================================

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
	resp := body.(DrawdownResponse)
	if resp.Status != "not_available" {
		t.Errorf("expected status=not_available, got %v", resp.Status)
	}
	if resp.Generated == "" {
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
	resp := body.(DrawdownResponse)
	if resp.Status != "" {
		t.Errorf("expected no status key when result present, got %v", resp.Status)
	}
	if resp.MaxDrawdown != -0.15 {
		t.Errorf("expected max_drawdown=-0.15, got %v", resp.MaxDrawdown)
	}
	if resp.VaR95 != -0.08 {
		t.Errorf("expected var_95=-0.08, got %v", resp.VaR95)
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
	h := &Handlers{LedgerDir: dir}
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

func TestHandleSimLatest_HappyPath(t *testing.T) {
	dir := t.TempDir()
	tracesDir := filepath.Join(dir, "traces")
	if err := os.MkdirAll(tracesDir, 0o755); err != nil {
		t.Fatalf("mkdir traces: %v", err)
	}

	ts1 := time.Date(2026, 6, 19, 9, 0, 0, 0, time.UTC)
	ts2 := time.Date(2026, 6, 19, 9, 5, 0, 0, time.UTC)
	records := []orchestrator.SimTraceRecord{
		{Step: 1, Layer: "data", Status: "OK", TS: ts1, SessionID: "20260619"},
		{
			Step: 2, Layer: "signal", Status: "OK", TS: ts2, SessionID: "20260619",
			Metadata: map[string]any{"symbol": "2330.TW"},
		},
	}
	path := filepath.Join(tracesDir, "sim-20260619.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for _, r := range records {
		b, _ := json.Marshal(r)
		if _, err := f.Write(append(b, '\n')); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	h := &Handlers{LedgerDir: dir}
	req := httptest.NewRequest(http.MethodGet, "/api/traces/sim-latest", nil)
	status, body := h.HandleSimLatest(req)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, body)
	}
	got, ok := body.([]orchestrator.SimTraceRecord)
	if !ok {
		t.Fatalf("expected []SimTraceRecord, got %T", body)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 records, got %d", len(got))
	}
	if got[0].Layer != "data" || got[1].Layer != "signal" {
		t.Errorf("record order mismatch: %+v", got)
	}
	if got[1].Metadata["symbol"] != "2330.TW" {
		t.Errorf("metadata not preserved: %+v", got[1].Metadata)
	}
}

func TestHandleSimLatest_PicksLatestOfMultiple(t *testing.T) {
	dir := t.TempDir()
	tracesDir := filepath.Join(dir, "traces")
	if err := os.MkdirAll(tracesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	older := filepath.Join(tracesDir, "sim-20260618.jsonl")
	newer := filepath.Join(tracesDir, "sim-20260619.jsonl")
	writeTrace := func(path, layer string) {
		r := orchestrator.SimTraceRecord{Step: 1, Layer: layer, Status: "OK", SessionID: "x"}
		b, _ := json.Marshal(r)
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	writeTrace(older, "old-layer")
	writeTrace(newer, "new-layer")

	h := &Handlers{LedgerDir: dir}
	req := httptest.NewRequest(http.MethodGet, "/api/traces/sim-latest", nil)
	status, body := h.HandleSimLatest(req)

	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d: %v", status, body)
	}
	got := body.([]orchestrator.SimTraceRecord)
	if len(got) != 1 || got[0].Layer != "new-layer" {
		t.Fatalf("expected 1 record from latest file, got %+v", got)
	}
}

func TestHandleSimLatest_ParseFail(t *testing.T) {
	dir := t.TempDir()
	tracesDir := filepath.Join(dir, "traces")
	if err := os.MkdirAll(tracesDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(tracesDir, "sim-bad.jsonl")
	content := "{\"step\":1,\"layer\":\"data\"}\nnot valid json\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	h := &Handlers{LedgerDir: dir}
	req := httptest.NewRequest(http.MethodGet, "/api/traces/sim-latest", nil)
	status, body := h.HandleSimLatest(req)

	if status != http.StatusInternalServerError {
		t.Fatalf("expected 500 on parse error, got %d: %v", status, body)
	}
	resp, ok := body.(map[string]string)
	if !ok {
		t.Fatalf("expected error map, got %T", body)
	}
	if !strings.Contains(resp["error"], "parse") {
		t.Errorf("expected parse error message, got %q", resp["error"])
	}
}

// =====================================================================
// NewHandlers
// =====================================================================

func TestNewHandlers_NonExistentWorkDir(t *testing.T) {
	// Should not panic with directories that do not exist yet.
	h := NewHandlers("/nonexistent/work/dir", "/nonexistent/ledger/dir")
	if h == nil {
		t.Fatal("expected non-nil handlers")
	}
	if h.WorkDir != "/nonexistent/work/dir" {
		t.Errorf("unexpected WorkDir: %q", h.WorkDir)
	}
}
