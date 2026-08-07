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
	"time"
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

func (r *reqRecorder) getResponseBody() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.responseBody
}

func newTestHarness(t *testing.T) (*server, *reqRecorder, func()) {
	t.Helper()
	rec := &reqRecorder{responseBody: []byte(`[]`)}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/macro/snapshot/latest":
			rec.mu.Lock()
			rec.path = r.URL.Path
			rec.mu.Unlock()
			_, _ = w.Write([]byte(`{"foreign_investor_net":{"value":1.0},"vix":{"value":15}}`))
		case "/api/janus/regime-score":
			rec.mu.Lock()
			rec.path = r.URL.Path
			rec.mu.Unlock()
			_, _ = w.Write([]byte(`{"score":7.7,"is_synthetic":true}`))
		default:
			rec.mu.Lock()
			rec.path = r.URL.Path
			rec.query = r.URL.Query()
			rec.headers = r.Header.Clone()
			b, _ := io.ReadAll(r.Body)
			rec.body = b
			rec.mu.Unlock()
			_, _ = w.Write(rec.getResponseBody())
		}
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
		cli:   NewHTTPClient(cfg),
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
	rec.responseBody = []byte(`{"sessions":[]}`)
	_, _, err := s.handleRegimeGetHistory(context.Background(), nil, RegimeGetHistoryInput{Days: 0})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	// rec.path reflects LAST HTTP call (fetchRegimeScore → /api/janus/regime-score).
	if rec.query.Get("limit") != "30" {
		t.Fatalf("expected limit=30 default, got %q", rec.query.Get("limit"))
	}
}

func TestHandleRegimeGetHistory_ClampedTo365(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"sessions":[]}`)
	_, _, err := s.handleRegimeGetHistory(context.Background(), nil, RegimeGetHistoryInput{Days: 999})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if got := rec.query.Get("limit"); got != "365" {
		t.Fatalf("expected limit=365 clamp, got %q", got)
	}
}

func TestHandleRegimeGetHistory_PrefersRealEngineScore(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{"sessions":[{"session_id":"s1","regime":"RISK_OFF","recorded_at":"2026-06-30T00:00:00Z"}],"current_regime":"RISK_OFF"}`)
	_, out, err := s.handleRegimeGetHistory(context.Background(), nil, RegimeGetHistoryInput{Days: 7})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if len(out.Regimes) != 1 {
		t.Fatalf("expected 1 session, got %d", len(out.Regimes))
	}
	// Historical rows no longer carry the current score — it is in the
	// output envelope as CurrentRegimeScore (#1263).
	if out.Regimes[0].Score != nil {
		t.Fatal("expected nil Score on historical row; score is in CurrentRegimeScore")
	}
	if out.CurrentRegimeScore == nil {
		t.Fatal("expected non-nil CurrentRegimeScore")
	}
	if got := *out.CurrentRegimeScore; got != 7.7 {
		t.Errorf("expected current_regime_score 7.7 (float64, not int), got %v", got)
	}
	if got := out.CurrentScoreSource; got != "janus_composite" {
		t.Errorf("expected current_score_source=janus_composite, got %q", got)
	}
	if !out.CurrentScoreSynthetic {
		t.Error("expected current_score_synthetic=true")
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
		_, _ = w.Write([]byte(`{"sessions":[{"session_id":"s1","regime":"RISK_OFF","recorded_at":"2026-06-30T00:00:00Z"}],"current_regime":"RISK_OFF"}`))
	}))
	defer ts.Close()

	tmp := t.TempDir()
	audit, err := NewAuditWriter(filepath.Join(tmp, "audit.log"))
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	defer audit.Close()
	cfg := Config{AtlasBaseURL: ts.URL, APIToken: "secret-token", AuditLogPath: filepath.Join(tmp, "audit.log")}
	s := &server{cfg: cfg, audit: audit, cli: NewHTTPClient(cfg)}
	_, out, err := s.handleRegimeGetHistory(context.Background(), nil, RegimeGetHistoryInput{Days: 7})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if got := rec.headers.Get("X-API-Key"); got != "secret-token" {
		t.Fatalf("expected X-API-Key forwarded, got %q", got)
	}
	if len(out.Regimes) != 1 {
		t.Fatalf("expected 1 regime point, got %d", len(out.Regimes))
	}
	if out.Regimes[0].Regime != "RISK_OFF" {
		t.Fatalf("expected RISK_OFF, got %q", out.Regimes[0].Regime)
	}
}

// --- strategy_list_active ----------------------------------------------------

func TestHandleStrategyListActive_OK(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.SetResponseBody([]byte(`{"strategies":[]}`))
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
	rec.responseBody = []byte(`{"alerts":[]}`)
	_, _, err := s.handleAlertListUnacknowledged(context.Background(), nil, alertListInput{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/alerts" {
		t.Fatalf("path=%s, want=/api/alerts", rec.path)
	}
}

// --- parameters / industry extension ----------------------------------------

func TestHandleNewReadOnlyTools_HitCorrectBackendPath(t *testing.T) {
	cases := []struct {
		name   string
		invoke func(s *server) error
		want   string
	}{
		{"parameters_get", func(s *server) error {
			_, _, e := s.handleParametersGet(context.Background(), nil, struct{}{})
			return e
		}, "/api/parameters"},
		{"parameters_get_categories", func(s *server) error {
			_, _, e := s.handleParametersGetCategories(context.Background(), nil, struct{}{})
			return e
		}, "/api/parameters/categories"},
		{"parameters_get_audit_log", func(s *server) error {
			_, _, e := s.handleParametersGetAuditLog(context.Background(), nil, struct{}{})
			return e
		}, "/api/parameters/audit-log"},
		{"parameters_get_metadata", func(s *server) error {
			_, _, e := s.handleParametersGetMetadata(context.Background(), nil, struct{}{})
			return e
		}, "/api/parameters/metadata"},
		{"parameters_get_snapshots_default", func(s *server) error {
			_, _, e := s.handleParametersGetSnapshots(context.Background(), nil, ParametersGetSnapshotsInput{Days: 0})
			return e
		}, "/api/parameters/snapshots"},
		{"sector_allocation_plan", func(s *server) error {
			_, _, e := s.handleSectorAllocationPlan(context.Background(), nil, struct{}{})
			return e
		}, "/api/dashboard/sector-allocation-plan"},
		{"channel_health", func(s *server) error {
			_, _, e := s.handleChannelHealth(context.Background(), nil, struct{}{})
			return e
		}, "/api/dashboard/channel-health"},
		{"risk_exposure", func(s *server) error {
			_, _, e := s.handleRiskExposure(context.Background(), nil, struct{}{})
			return e
		}, "/api/dashboard/risk-exposure"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, rec, done := newTestHarness(t)
			defer done()
			rec.responseBody = []byte(`{}`) // handlers unmarshal into *map[string]any
			if err := tc.invoke(s); err != nil {
				t.Fatalf("handler error: %v", err)
			}
			if rec.path != tc.want {
				t.Errorf("%s: path=%q want=%q", tc.name, rec.path, tc.want)
			}
		})
	}
}

func TestHandleParametersGetSnapshots_DefaultDays(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, _, err := s.handleParametersGetSnapshots(context.Background(), nil, ParametersGetSnapshotsInput{Days: 0})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.query.Get("days") != "20" {
		t.Errorf("days default: query=%q want=20", rec.query.Get("days"))
	}
}

func TestHandleParametersGetSnapshots_ClampUpperBound(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.responseBody = []byte(`{}`)
	_, _, err := s.handleParametersGetSnapshots(context.Background(), nil, ParametersGetSnapshotsInput{Days: 9999})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if rec.query.Get("days") != "365" {
		t.Errorf("days clamp: query=%q want=365", rec.query.Get("days"))
	}
}

// --- PR 3 write MCP tools ---------------------------------------------------

func TestHandleNewWriteTools_HitCorrectBackendPath(t *testing.T) {
	cases := []struct {
		name   string
		invoke func(s *server) error
		want   string
	}{
		{"experiment_promote", func(s *server) error {
			_, _, err := s.handleExperimentPromote(context.Background(), nil, experimentIDInput{ExperimentID: "exp-001"})
			return err
		}, "/api/experiment/promote"},
		{"experiment_revert", func(s *server) error {
			_, _, err := s.handleExperimentRevert(context.Background(), nil, experimentIDInput{ExperimentID: "exp-002"})
			return err
		}, "/api/experiment/revert"},
		{"control_pause_agent", func(s *server) error {
			_, _, err := s.handleControlPauseAgent(context.Background(), nil, controlAgentInterventionInput{AgentID: "semi-desk-01", Reason: "sharpe regression", Operator: "ci-bot"})
			return err
		}, "/api/control/pause-agent"},
		{"control_resume_agent", func(s *server) error {
			_, _, err := s.handleControlResumeAgent(context.Background(), nil, controlAgentInterventionInput{AgentID: "semi-desk-01"})
			return err
		}, "/api/control/resume-agent"},
		{"control_sector_ban", func(s *server) error {
			_, _, err := s.handleControlSectorBan(context.Background(), nil, controlSectorBanInput{Sector: "半導體", Banned: true, Reason: "valuation concern"})
			return err
		}, "/api/control/sector-ban"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, rec, done := newTestHarness(t)
			defer done()
			rec.responseBody = []byte(`{}`)
			if err := tc.invoke(s); err != nil {
				t.Fatalf("handler error: %v", err)
			}
			if rec.path != tc.want {
				t.Errorf("%s: path=%q want=%q", tc.name, rec.path, tc.want)
			}
		})
	}
}

func TestHandleNewWriteTools_RejectEmptyIDs(t *testing.T) {
	cases := []struct {
		name   string
		invoke func(s *server) error
	}{
		{"experiment_promote_empty", func(s *server) error {
			_, _, err := s.handleExperimentPromote(context.Background(), nil, experimentIDInput{ExperimentID: ""})
			return err
		}},
		{"experiment_revert_empty", func(s *server) error {
			_, _, err := s.handleExperimentRevert(context.Background(), nil, experimentIDInput{ExperimentID: ""})
			return err
		}},
		{"control_pause_agent_empty", func(s *server) error {
			_, _, err := s.handleControlPauseAgent(context.Background(), nil, controlAgentInterventionInput{AgentID: ""})
			return err
		}},
		{"control_resume_agent_empty", func(s *server) error {
			_, _, err := s.handleControlResumeAgent(context.Background(), nil, controlAgentInterventionInput{AgentID: ""})
			return err
		}},
		{"control_sector_ban_empty", func(s *server) error {
			_, _, err := s.handleControlSectorBan(context.Background(), nil, controlSectorBanInput{Sector: ""})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _, done := newTestHarness(t)
			defer done()
			if err := tc.invoke(s); err == nil {
				t.Errorf("expected error for empty input, got nil")
			}
		})
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
	s := &server{cfg: cfg, audit: audit, cli: NewHTTPClient(cfg)}
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

func TestDeriveSystemHealthStatus(t *testing.T) {
	cases := []struct {
		name string
		raw  map[string]any
		want string
	}{
		{
			name: "production shape all ok",
			raw: map[string]any{"info": map[string]any{
				"replay_data_path_ok": true,
				"cycle_stale":         false,
				"data_channels":       []any{map[string]any{"status": "ok"}},
			}},
			want: "ok",
		},
		{
			name: "replay path broken",
			raw:  map[string]any{"info": map[string]any{"replay_data_path_ok": false}},
			want: "degraded",
		},
		{
			name: "channel not ok",
			raw: map[string]any{"info": map[string]any{
				"data_channels": []any{map[string]any{"status": "ok"}, map[string]any{"status": "error"}},
			}},
			want: "degraded",
		},
		{
			name: "flattened payload tolerated",
			raw:  map[string]any{"cycle_stale": true},
			want: "degraded",
		},
		{
			name: "empty payload defaults ok",
			raw:  map[string]any{},
			want: "ok",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := deriveSystemHealthStatus(c.raw); got != c.want {
				t.Fatalf("deriveSystemHealthStatus = %q, want %q", got, c.want)
			}
		})
	}
}

// --- error path surfaced from HttpClient -------------------------------------

func TestHandle_AtlasErrorSurfacesAsMCPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer ts.Close()
	tmp := t.TempDir()
	audit, _ := NewAuditWriter(filepath.Join(tmp, "a.log"))
	defer audit.Close()
	cfg := Config{AtlasBaseURL: ts.URL, AuditLogPath: filepath.Join(tmp, "a.log")}
	s := &server{cfg: cfg, audit: audit, cli: NewHTTPClient(cfg)}
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
	s := &server{cfg: cfg, audit: audit, cli: NewHTTPClient(cfg)}

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

func TestWithAudit_WritesV2Schema(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"date":"2026-07-01","regime":"RISK_ON","score":8}]`))
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
	s := &server{cfg: cfg, audit: audit, cli: NewHTTPClient(cfg)}

	ctx := ContextWithAgentID(context.Background(), "test-agent")
	ctx = ContextWithTenantID(ctx, "test-tenant")
	err = s.withAudit(ctx, "regime_get_history", []string{"days"}, func() error {
		var raw []RegimePoint
		return s.cli.Get(ctx, "/api/dashboard/regime-history", nil, &raw)
	})
	if err != nil {
		t.Fatalf("withAudit: %v", err)
	}

	raw, _ := os.ReadFile(path)
	var decoded map[string]any
	json.Unmarshal([]byte(strings.TrimRight(string(raw), "\n")), &decoded)

	if sv := decoded["schema_version"]; sv != float64(2) {
		t.Fatalf("expected schema_version=2, got %v", sv)
	}
	if hash := decoded["args_hash"]; hash == nil || hash == "" {
		t.Fatal("expected non-empty args_hash")
	}
	if lat := decoded["latency_ms"]; lat == nil {
		t.Fatal("expected latency_ms")
	}
	if tp := decoded["transport"]; tp != "stdio" {
		t.Fatalf("expected transport=stdio, got %v", tp)
	}
	if aid := decoded["agent_id"]; aid != "test-agent" {
		t.Fatalf("expected agent_id=test-agent, got %v", aid)
	}
}

func TestWithAudit_V2LatencyMS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[]`))
	}))
	defer ts.Close()

	tmp := t.TempDir()
	path := filepath.Join(tmp, "audit.log")
	audit, _ := NewAuditWriter(path)
	defer audit.Close()

	cfg := Config{AtlasBaseURL: ts.URL, AuditLogPath: path}
	s := &server{cfg: cfg, audit: audit, cli: NewHTTPClient(cfg)}

	err := s.withAudit(context.Background(), "strategy_list_active", nil, func() error {
		var strategies []map[string]any
		return s.cli.Get(context.Background(), "/api/strategies/active", nil, &strategies)
	})
	if err != nil {
		t.Fatalf("withAudit: %v", err)
	}

	raw, _ := os.ReadFile(path)
	var decoded map[string]any
	json.Unmarshal([]byte(strings.TrimRight(string(raw), "\n")), &decoded)

	lat, ok := decoded["latency_ms"].(float64)
	if !ok || lat < 50 {
		t.Fatalf("expected latency_ms >= 50, got %v", lat)
	}
	dur := decoded["duration_ms"].(float64)
	if dur != lat {
		t.Fatalf("expected duration_ms == latency_ms, got dur=%v lat=%v", dur, lat)
	}
}

func TestWithAudit_V2OnError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer ts.Close()

	tmp := t.TempDir()
	path := filepath.Join(tmp, "audit.log")
	audit, _ := NewAuditWriter(path)
	defer audit.Close()

	cfg := Config{AtlasBaseURL: ts.URL, AuditLogPath: path}
	s := &server{cfg: cfg, audit: audit, cli: NewHTTPClient(cfg)}

	_ = s.withAudit(context.Background(), "system_get_health", nil, func() error {
		return s.cli.Get(context.Background(), "/api/dashboard/system-health", nil, nil)
	})

	raw, _ := os.ReadFile(path)
	var decoded map[string]any
	json.Unmarshal([]byte(strings.TrimRight(string(raw), "\n")), &decoded)

	if decoded["status"] != "error" {
		t.Fatalf("expected status=error, got %v", decoded["status"])
	}
	if decoded["schema_version"] != float64(2) {
		t.Fatalf("expected schema_version=2, got %v", decoded["schema_version"])
	}
	if errMsg := decoded["error"]; errMsg == nil || errMsg == "" {
		t.Fatal("expected error field populated")
	}
}

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

// --- system_get_health: runtime build info preservation ---------------------
//
// Spec: docs/specs/capital-flow-seven-dimension-spec.md §11.4 — the upstream
// /api/dashboard/system-health payload must surface `runtime.commit` (and
// `version`, `build_time`, `go_version`) so atlas-mcp callers can audit a
// deployment's binary against the buildinfo manifest. This test pins the
// handler's contract that runtime.* keys are preserved verbatim into Info.

// TestHandleSystemGetHealth_PreservesRuntimeCommit feeds a mock payload
// containing a runtime block and asserts the block flows through Info
// without being stripped or reshaped.
func TestHandleSystemGetHealth_PreservesRuntimeCommit(t *testing.T) {
	s, rec, done := newTestHarness(t)
	defer done()
	rec.SetResponseBody([]byte(`{
		"replay_data_path_ok": true,
		"cycle_stale": false,
		"data_channels": [{"channel_id":"tsm_adr","status":"ok"}],
		"runtime": {
			"version": "v0.0.0.32",
			"commit": "abc1234",
			"build_time": "2026-07-17T10:00:00Z",
			"go_version": "go1.26"
		}
	}`))
	_, out, err := s.handleSystemGetHealth(context.Background(), nil, struct{}{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if rec.path != "/api/dashboard/system-health" {
		t.Fatalf("path=%s", rec.path)
	}
	if out.Status != "ok" {
		t.Errorf("expected status=ok, got %q", out.Status)
	}
	rt, ok := out.Info["runtime"].(map[string]any)
	if !ok {
		t.Fatalf("expected runtime block in Info, got %T (%v)", out.Info["runtime"], out.Info["runtime"])
	}
	if rt["commit"] != "abc1234" {
		t.Errorf("runtime.commit: want %q, got %v", "abc1234", rt["commit"])
	}
	if rt["version"] != "v0.0.0.32" {
		t.Errorf("runtime.version: want %q, got %v", "v0.0.0.32", rt["version"])
	}
}
