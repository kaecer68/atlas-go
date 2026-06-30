package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// withMockAtlas spins up an httptest server that records every request. The
// returned handler is invoked for the matching MCP tool call; the URL and
// querystring are visible on the `got` recorder.
type reqRecorder struct {
	mu           sync.Mutex
	path         string
	headers      http.Header
	body         []byte
	query        url.Values
	responseBody []byte // overridable; default `[]`
}

func (r *reqRecorder) SetResponseBody(b []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.responseBody = b
}

func newTestHarness(t *testing.T) (*server, *reqRecorder, func()) {
	t.Helper()
	rec := &reqRecorder{responseBody: []byte(`[]`)}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		rec.path = r.URL.Path
		rec.query = r.URL.Query()
		rec.headers = r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		rec.body = b
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(rec.responseBody)
	}))

	tmpDir := t.TempDir()
	auditPath := filepath.Join(tmpDir, "audit.log")
	audit, err := NewAuditWriter(auditPath)
	if err != nil {
		ts.Close()
		t.Fatalf("audit: %v", err)
	}

	cfg := Config{
		AtlasBaseURL: ts.URL,
		AuditLogPath: auditPath,
		HTTPTimeout:  0,
	}
	s := &server{
		cfg:   cfg,
		audit: audit,
		cli:   newHTTPClient(cfg),
	}
	cleanup := func() {
		_ = audit.Close()
		ts.Close()
	}
	return s, rec, cleanup
}

// --- regime_get_history -------------------------------------------------------

func TestHandleRegimeGetHistory_DefaultDays(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	_, _, err := s.handleRegimeGetHistory(context.Background(), nil, RegimeGetHistoryInput{Days: 0})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/regime-history" {
		t.Fatalf("path=%s", rec.path)
	}
	if rec.query.Get("days") != "30" {
		t.Fatalf("expected days=30 default, got %q", rec.query.Get("days"))
	}
}

func TestHandleRegimeGetHistory_ClampedTo365(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	_, _, err := s.handleRegimeGetHistory(context.Background(), nil, RegimeGetHistoryInput{Days: 999})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if got := rec.query.Get("days"); got != "365" {
		t.Fatalf("expected days=365 clamp, got %q", got)
	}
}

func TestHandleRegimeGetHistory_ForwardsAPIToken(t *testing.T) {
	rec := &reqRecorder{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		rec.headers = r.Header.Clone()
		rec.path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"date":"2026-06-30","regime":"RISK_OFF","score":-12}]`))
	}))
	defer ts.Close()

	tmp := t.TempDir()
	audit, err := NewAuditWriter(filepath.Join(tmp, "audit.log"))
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	defer audit.Close()
	cfg := Config{AtlasBaseURL: ts.URL, APIToken: "secret-token", AuditLogPath: filepath.Join(tmp, "audit.log")}
	s := &server{cfg: cfg, audit: audit, cli: newHTTPClient(cfg)}
	_, _, err = s.handleRegimeGetHistory(context.Background(), nil, RegimeGetHistoryInput{Days: 7})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if got := rec.headers.Get("X-API-Key"); got != "secret-token" {
		t.Fatalf("expected X-API-Key forwarded, got %q", got)
	}
}

// --- strategy_list_active ----------------------------------------------------

func TestHandleStrategyListActive_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	_, _, err := s.handleStrategyListActive(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/strategies/active" {
		t.Fatalf("path=%s", rec.path)
	}
	if len(rec.query) != 0 {
		t.Fatalf("unexpected query: %v", rec.query)
	}
}

// --- experiment_judge --------------------------------------------------------

func TestHandleExperimentJudge_MissingID(t *testing.T) {
	s, _, done := newTestHarness(t)
	defer done()
	_, _, err := s.handleExperimentJudge(context.Background(), nil, ExperimentJudgeInput{ExperimentID: ""})
	if err == nil {
		t.Fatal("expected error when experiment_id is empty")
	}
}

func TestHandleExperimentJudge_PostsJSON(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.SetResponseBody([]byte(`{}`))
	_, _, err := s.handleExperimentJudge(context.Background(), nil, ExperimentJudgeInput{ExperimentID: "exp-42"})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/experiment/judge" {
		t.Fatalf("path=%s", rec.path)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.body, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["experiment_id"] != "exp-42" {
		t.Fatalf("expected experiment_id=exp-42, got %v", got)
	}
}

// --- alert_list_unacknowledged -----------------------------------------------

func TestHandleAlertListUnacknowledged(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	_, _, err := s.handleAlertListUnacknowledged(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/alerts/unacknowledged" {
		t.Fatalf("path=%s", rec.path)
	}
}

// --- system_get_health --------------------------------------------------------

func TestHandleSystemGetHealth_ParsesStatus(t *testing.T) {
	rec := &reqRecorder{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		defer rec.mu.Unlock()
		rec.path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"degraded","uptime_seconds":420}`))
	}))
	defer ts.Close()
	tmp := t.TempDir()
	audit, _ := NewAuditWriter(filepath.Join(tmp, "a.log"))
	defer audit.Close()
	cfg := Config{AtlasBaseURL: ts.URL, AuditLogPath: filepath.Join(tmp, "a.log")}
	s := &server{cfg: cfg, audit: audit, cli: newHTTPClient(cfg)}
	_, out, err := s.handleSystemGetHealth(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/system-health" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Status != "degraded" {
		t.Fatalf("expected status=degraded, got %q", out.Status)
	}
	if v, _ := out.Info["uptime_seconds"].(float64); v != 420 {
		t.Fatalf("expected uptime_seconds=420 in info, got %v", out.Info["uptime_seconds"])
	}
}

// --- error path surfaced from httpClient -------------------------------------

func TestHandle_AtlasErrorSurfacesAsMCPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer ts.Close()
	tmp := t.TempDir()
	audit, _ := NewAuditWriter(filepath.Join(tmp, "a.log"))
	defer audit.Close()
	cfg := Config{AtlasBaseURL: ts.URL, AuditLogPath: filepath.Join(tmp, "a.log")}
	s := &server{cfg: cfg, audit: audit, cli: newHTTPClient(cfg)}
	_, _, err := s.handleStrategyListActive(context.Background(), nil, struct{}{})
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 error, got %v", err)
	}
}

// --- audit log emitted on both success and error ----------------------------

func TestAuditEntry_WrittenOnBothSuccessAndError(t *testing.T) {
	// We make an "atlas" that always 500 so the call fails.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer ts.Close()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "audit.log")
	audit, err := NewAuditWriter(path)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	defer audit.Close()
	cfg := Config{AtlasBaseURL: ts.URL, AuditLogPath: path}
	s := &server{cfg: cfg, audit: audit, cli: newHTTPClient(cfg)}

	if _, _, err := s.handleStrategyListActive(context.Background(), nil, struct{}{}); err == nil {
		t.Fatal("expected error from atlas")
	}
	raw, _ := os.ReadFile(path)
	if len(raw) == 0 {
		t.Fatal("expected audit entry, got empty file")
	}
	if !strings.Contains(string(raw), `"status":"error"`) {
		t.Fatalf("expected status=error in audit entry, got %s", string(raw))
	}
	if !strings.Contains(string(raw), `"tool":"strategy_list_active"`) {
		t.Fatalf("expected tool=strategy_list_active, got %s", string(raw))
	}
}

// --- run-level guard against config errors ----------------------------------

func TestRun_RequiresBaseURL(t *testing.T) {
	if err := Run(context.Background(), Config{AuditLogPath: filepath.Join(t.TempDir(), "a.log")}); err == nil {
		t.Fatal("expected error when AtlasBaseURL is empty")
	}
}

func TestRun_RequiresAuditPath(t *testing.T) {
	if err := Run(context.Background(), Config{AtlasBaseURL: "http://x"}); err == nil {
		t.Fatal("expected error when AuditLogPath is empty")
	}
}

// guard the registry entry so a future bug doesn't silently diverge.
var _ = errors.New

// --- audit v2: withAudit writes schema_version=2 entries ----------------------
// TODO: uncomment after implementing ContextWithAgentID/ContextWithTenantID and
// updating withAudit to accept context.Context as the first parameter.

/*
func TestWithAudit_WritesV2Schema(t *testing.T) {
	// ... requires ContextWithAgentID + context-aware withAudit
}

func TestWithAudit_V2LatencyMS(t *testing.T) {
	// ... requires context-aware withAudit
}

func TestWithAudit_V2OnError(t *testing.T) {
	// ... requires context-aware withAudit
}
*/

// TestNoToolWithoutDescription verifies that every auto-generated tool descriptor has a non-empty description.
func TestNoToolWithoutDescription(t *testing.T) {
	if len(autoDescMap) == 0 {
		t.Fatal("auto-desc map is empty")
	}
	for name, desc := range autoDescMap {
		if desc.Description == "" {
			t.Errorf("tool %q has empty description", name)
		}
	}
	if len(autoDescMap) < 60 {
		t.Errorf("auto-desc map has %d tools, want >= 60", len(autoDescMap))
	}
}
