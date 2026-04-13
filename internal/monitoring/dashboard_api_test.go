package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
)

func TestDashboardAPIEndpoints(t *testing.T) {
	ledgerDir := t.TempDir()
	setupDashboardFixtures(t, ledgerDir)

	api := NewDashboardAPI(ledgerDir)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	t.Run("macro-radar", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/dashboard/macro-radar", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var resp MacroRadarResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if resp.SessionID == "" {
			t.Fatalf("expected session id")
		}
		if len(resp.GuardOutcomes) == 0 {
			t.Fatalf("expected guard outcomes")
		}
		if resp.BrokerRuntime.NonceStore == "" {
			t.Fatalf("expected broker runtime context")
		}
	})

	t.Run("macro-radar with session_id", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/dashboard/macro-radar?session_id=session-1", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var resp MacroRadarResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if resp.SessionID != "session-1" {
			t.Fatalf("expected session-1, got %s", resp.SessionID)
		}
	})

	t.Run("agent-observatory", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/dashboard/agent-observatory", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var resp AgentObservatoryResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if resp.NextExperimentAgentID == "" {
			t.Fatalf("expected next experiment agent id")
		}
		if len(resp.WeakestAgentScorecards) == 0 {
			t.Fatalf("expected scorecards")
		}
		if resp.BrokerRuntime.Signer == "" {
			t.Fatalf("expected broker runtime signer")
		}
	})

	t.Run("agent-observatory with limit", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/dashboard/agent-observatory?limit=1", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var resp AgentObservatoryResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if len(resp.WeakestAgentScorecards) != 1 {
			t.Fatalf("expected 1 scorecard, got %d", len(resp.WeakestAgentScorecards))
		}
	})

	t.Run("forecast-vs-reality", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/dashboard/forecast-vs-reality", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var resp ForecastVsRealityResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if len(resp.Items) == 0 {
			t.Fatalf("expected forecast-vs-reality items")
		}
		if resp.Items[0].ProposalID == "" {
			t.Fatalf("expected proposal id in forecast-vs-reality item")
		}
		if resp.BrokerRuntime.NonceStore == "" {
			t.Fatalf("expected forecast-vs-reality broker runtime context")
		}
	})

	t.Run("forecast-vs-reality with agent_id and limit", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/dashboard/forecast-vs-reality?agent_id=growth-momentum-01&limit=1", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var resp ForecastVsRealityResponse
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if len(resp.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(resp.Items))
		}
		if resp.Items[0].TargetAgentID != "growth-momentum-01" {
			t.Fatalf("unexpected agent id: %s", resp.Items[0].TargetAgentID)
		}
	})

	t.Run("phase3-status", func(t *testing.T) {
		// Write a metrics fixture to the well-known path relative to test working dir
		_ = os.MkdirAll("data/state", 0o755)
		metrics := orchestrator.Phase3Metrics{
			SwarmRunning:           true,
			SwarmConsensusSymbols:  5,
			PRISMCompletedResults:  12,
			SpawningActive:         3,
			ReflexivityActiveLoops: 2,
			AdversarialLastScore:   0.82,
		}
		data, _ := json.MarshalIndent(metrics, "", "  ")
		_ = os.WriteFile("data/state/phase3_metrics.json", data, 0o644)
		defer os.RemoveAll("data/state")

		api.RegisterPhase3Routes(mux)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/dashboard/phase3-status", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var resp orchestrator.Phase3Metrics
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if !resp.SwarmRunning {
			t.Fatal("expected swarm running")
		}
		if resp.PRISMCompletedResults != 12 {
			t.Fatalf("expected 12 completed results, got %d", resp.PRISMCompletedResults)
		}
		if resp.AdversarialLastScore != 0.82 {
			t.Fatalf("expected adversarial score 0.82, got %.2f", resp.AdversarialLastScore)
		}
	})

	t.Run("agent-observatory invalid limit", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/dashboard/agent-observatory?limit=abc", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
	})

	t.Run("latest report", func(t *testing.T) {
		t.Chdir(t.TempDir())
		_ = os.MkdirAll("reports", 0o755)
		_ = os.WriteFile("reports/backtest_window-latest.md", []byte("# Latest Report\n"), 0o644)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/report/latest", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "# Latest Report") {
			t.Fatalf("expected markdown report content, got %s", body)
		}
		ct := rr.Header().Get("Content-Type")
		if !strings.Contains(ct, "text/markdown") {
			t.Fatalf("expected text/markdown content type, got %s", ct)
		}
	})

	t.Run("latest report not found", func(t *testing.T) {
		t.Chdir(t.TempDir())
		_ = os.MkdirAll("reports", 0o755)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/report/latest", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rr.Code)
		}
	})

	t.Run("report list", func(t *testing.T) {
		t.Chdir(t.TempDir())
		_ = os.MkdirAll("reports", 0o755)
		_ = os.WriteFile("reports/backtest_a.md", []byte("# A\n"), 0o644)
		time.Sleep(10 * time.Millisecond)
		_ = os.WriteFile("reports/backtest_b.md", []byte("# B\n"), 0o644)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/report/list", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		reports, ok := resp["reports"].([]any)
		if !ok || len(reports) != 2 {
			t.Fatalf("expected 2 reports, got %v", resp)
		}
		first := reports[0].(map[string]any)
		if !strings.Contains(first["filename"].(string), "backtest_") {
			t.Fatalf("expected backtest filename, got %v", first)
		}
	})

	t.Run("report list empty", func(t *testing.T) {
		t.Chdir(t.TempDir())
		_ = os.MkdirAll("reports", 0o755)

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/report/list", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		reports, ok := resp["reports"].([]any)
		if !ok || len(reports) != 0 {
			t.Fatalf("expected 0 reports, got %v", resp)
		}
	})
}

func TestDashboardNarrativeRoutes(t *testing.T) {
	api := NewDashboardAPI(t.TempDir())
	mux := http.NewServeMux()
	api.RegisterNarrativeRoutes(mux)

	t.Run("narrative events", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/narrative/events?us10y_change_bps=15&ai_capex_sentiment=0.8", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		events, ok := resp["events"].([]any)
		if !ok || len(events) == 0 {
			t.Fatalf("expected events, got %v", resp)
		}
	})

	t.Run("narrative chains", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/narrative/chains?us10y_change_bps=15", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		chains, ok := resp["chains"].([]any)
		if !ok {
			t.Fatalf("expected chains array, got %v", resp)
		}
		if len(chains) == 0 {
			t.Fatalf("expected non-empty chains")
		}
	})

	t.Run("narrative models", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/narrative/models?us10y_change_bps=15", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		models, ok := resp["models"].([]any)
		if !ok || len(models) == 0 {
			t.Fatalf("expected models, got %v", resp)
		}
	})

	t.Run("narrative templates", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/narrative/templates", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		templates, ok := resp["templates"].([]any)
		if !ok || len(templates) == 0 {
			t.Fatalf("expected templates, got %v", resp)
		}
	})
}

func TestDashboardSwaggerRoutes(t *testing.T) {
	ledgerDir := t.TempDir()
	api := NewDashboardAPI(ledgerDir)
	mux := http.NewServeMux()
	api.RegisterSwaggerRoutes(mux)

	t.Run("swagger ui html", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/docs", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		ct := rr.Header().Get("Content-Type")
		if !strings.Contains(ct, "text/html") {
			t.Fatalf("expected text/html, got %s", ct)
		}
		body := rr.Body.String()
		if !strings.Contains(body, "swagger-ui") {
			t.Fatalf("expected swagger-ui in body, got %s", body)
		}
	})

	t.Run("swagger json exists", func(t *testing.T) {
		origDir, _ := os.Getwd()
		t.Cleanup(func() { _ = os.Chdir(origDir) })
		dir := t.TempDir()
		if err := os.Chdir(dir); err != nil {
			t.Fatalf("chdir: %v", err)
		}
		if err := os.MkdirAll("docs", 0o755); err != nil {
			t.Fatalf("mkdir docs: %v", err)
		}
		if err := os.WriteFile("docs/swagger.json", []byte(`{"openapi":"3.0.0"}`), 0o644); err != nil {
			t.Fatalf("write swagger.json: %v", err)
		}

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/docs/swagger.json", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		ct := rr.Header().Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Fatalf("expected application/json, got %s", ct)
		}
		if !strings.Contains(rr.Body.String(), "openapi") {
			t.Fatalf("expected openapi in body")
		}
	})

	t.Run("swagger json not found", func(t *testing.T) {
		origDir, _ := os.Getwd()
		t.Cleanup(func() { _ = os.Chdir(origDir) })
		dir := t.TempDir()
		if err := os.Chdir(dir); err != nil {
			t.Fatalf("chdir: %v", err)
		}
		_ = os.RemoveAll("docs")

		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/docs/swagger.json", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rr.Code)
		}
	})
}

func TestDashboardMacroRoutes(t *testing.T) {
	ledgerDir := t.TempDir()
	api := NewDashboardAPI(ledgerDir)
	// Inject mock provider to avoid real network calls in tests.
	api.macroIngestor = narrative.NewMacroIngestor(
		&marketdata.MockMacroProvider{
			Snapshot: marketdata.MacroDataSnapshot{
				US10Y: marketdata.MacroDataPoint{Symbol: "^TNX", Value: 150, ChangePct: 2.0},
				DXY:   marketdata.MacroDataPoint{Symbol: "DX-Y.NYB", Value: 105, ChangePct: 1.8},
			},
		},
		t.TempDir(),
	)
	mux := http.NewServeMux()
	api.RegisterMacroRoutes(mux)

	t.Run("macro snapshot latest not found", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/macro/snapshot/latest", nil)
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rr.Code)
		}
	})

	t.Run("macro snapshot history missing date", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/macro/snapshot/history", nil)
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
		}
	})

	t.Run("macro ingest triggers events", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/macro/ingest", nil)
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, ok := resp["snapshot"]; !ok {
			t.Fatalf("expected snapshot in response")
		}
		events, ok := resp["events"].([]any)
		if !ok || len(events) == 0 {
			t.Fatalf("expected events from mock data")
		}
	})
}

func TestDashboardControlRoutes(t *testing.T) {
	ledgerDir := t.TempDir()
	api := NewDashboardAPI(ledgerDir)
	mux := http.NewServeMux()
	api.RegisterControlRoutes(mux)

	t.Run("pause agent", func(t *testing.T) {
		body := `{"agent_id":"growth-momentum-01","reason":"underperforming","operator":"human"}`
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/control/pause-agent", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["success"] != true {
			t.Fatalf("expected success true, got %v", resp)
		}
	})

	t.Run("sector ban", func(t *testing.T) {
		body := `{"sector":"ai_supply_chain","banned":true,"reason":"overvaluation","operator":"human"}`
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/control/sector-ban", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["success"] != true {
			t.Fatalf("expected success true, got %v", resp)
		}
	})

	t.Run("audit log", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/control/audit-log", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		interventions, ok := resp["interventions"].([]any)
		if !ok || len(interventions) == 0 {
			t.Fatalf("expected interventions, got %v", resp)
		}
	})

	t.Run("active overrides", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/control/active-overrides", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rr.Code)
		}
		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		paused, ok := resp["paused_agents"].([]any)
		if !ok {
			t.Fatalf("expected paused_agents array, got %v", resp)
		}
		banned, ok := resp["banned_sectors"].([]any)
		if !ok {
			t.Fatalf("expected banned_sectors array, got %v", resp)
		}
		if len(paused) == 0 || len(banned) == 0 {
			t.Fatalf("expected non-empty overrides")
		}
	})
}

func setupDashboardFixtures(t *testing.T, ledgerDir string) {
	t.Helper()

	summary := domain.SessionSummary{
		SessionID:             "session-1",
		Regime:                domain.RegimeRiskOn,
		NextExperimentAgentID: "growth-momentum-01",
		BrokerRuntime: domain.BrokerRuntimeAudit{
			Mode:             "live",
			Adapter:          "http",
			Signer:           "hmac-sha256",
			SignerVersion:    "v1",
			KeyID:            "kid-dashboard-1",
			HTTPAttempts:     2,
			HTTPTimeoutSec:   5,
			RetryStatusCodes: []int{429, 503},
			MaxClockSkewSec:  120,
			NonceTTLSec:      180,
			NonceStore:       "redis",
			NonceRedisPrefix: "atlas:nonce:",
		},
		GuardOutcomes: []domain.GuardOutcome{{
			GuardID:     "cro-01",
			GuardSkill:  "cro_risk",
			Severity:    domain.GuardSeverityHard,
			Passed:      true,
			Reason:      "filtered 1 recommendation(s)",
			InputCount:  2,
			OutputCount: 1,
		}},
		RecordedAt: time.Now(),
	}

	summaryPath := filepath.Join(ledgerDir, "sessions", summary.SessionID, "summary.json")
	if err := os.MkdirAll(filepath.Dir(summaryPath), 0o755); err != nil {
		t.Fatalf("mkdir summary dir: %v", err)
	}
	summaryBytes, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if err := os.WriteFile(summaryPath, summaryBytes, 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	outcomesPath := filepath.Join(ledgerDir, "recommendation_outcomes.jsonl")
	if err := os.MkdirAll(filepath.Dir(outcomesPath), 0o755); err != nil {
		t.Fatalf("mkdir outcomes dir: %v", err)
	}
	outcomes := []domain.RecommendationOutcome{
		{AgentID: "growth-momentum-01", Skill: "growth_momentum", Window: "1d", ForwardReturn: -0.01, Hit: false, RecordedAt: time.Now()},
		{AgentID: "value-yield-01", Skill: "value_yield", Window: "1d", ForwardReturn: 0.02, Hit: true, RecordedAt: time.Now()},
	}
	f, err := os.Create(outcomesPath)
	if err != nil {
		t.Fatalf("create outcomes file: %v", err)
	}
	enc := json.NewEncoder(f)
	for _, outcome := range outcomes {
		if err := enc.Encode(outcome); err != nil {
			_ = f.Close()
			t.Fatalf("encode outcome: %v", err)
		}
	}
	_ = f.Close()

	experiment := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			ID:             "exp-1",
			ProposalID:     "proposal-exp-1",
			CommitID:       "commit-exp-1",
			ApprovalID:     "approval-exp-1",
			TargetAgentID:  "growth-momentum-01",
			Skill:          "growth_momentum",
			MutationType:   "prompt_tightening",
			Status:         domain.ExperimentAccepted,
			BaselineValue:  0.01,
			CandidateValue: 0.02,
		},
		RecordedAt: time.Now(),
	}
	resultPath := filepath.Join(ledgerDir, "experiments", "exp-1.json")
	if err := os.MkdirAll(filepath.Dir(resultPath), 0o755); err != nil {
		t.Fatalf("mkdir experiment dir: %v", err)
	}
	resultBytes, err := json.Marshal(experiment)
	if err != nil {
		t.Fatalf("marshal experiment result: %v", err)
	}
	if err := os.WriteFile(resultPath, resultBytes, 0o644); err != nil {
		t.Fatalf("write experiment result: %v", err)
	}
}

func TestDashboardLiveStatusEndpoint(t *testing.T) {
	ledgerDir := t.TempDir()
	api := NewDashboardAPI(ledgerDir)
	mux := http.NewServeMux()
	api.RegisterLiveRoutes(mux)

	// Setup circuit breaker state fixture directly in data/state.
	_ = os.RemoveAll("data/state")
	wd, _ := os.Getwd()
	cbDir := filepath.Join(wd, "data", "state")
	if err := os.MkdirAll(cbDir, 0o755); err != nil {
		t.Fatalf("mkdir cb dir: %v", err)
	}
	cbPath := filepath.Join(cbDir, "circuit_breaker_state.json")
	cbData := `{"state":"paused","state_changed_at":"2026-04-13T00:00:00Z","consecutive_sl":2,"cooldown_until":"2026-04-13T00:15:00Z","intraday_peak":1000000,"day_start_value":980000}`
	if err := os.WriteFile(cbPath, []byte(cbData), 0o644); err != nil {
		t.Fatalf("write cb state: %v", err)
	}
	t.Logf("wrote cb state to %s", cbPath)
	t.Logf("working dir: %s", wd)
	t.Cleanup(func() {
		_ = os.RemoveAll("data/state")
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/live-status", nil)
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	cb, ok := resp["circuit_breaker"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected circuit_breaker object")
	}
	if cb["state"] != "paused" {
		t.Fatalf("expected paused state, got %v", cb["state"])
	}

	portfolio, ok := resp["portfolio"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected portfolio object")
	}
	if _, ok := portfolio["cash"]; !ok {
		t.Fatalf("expected cash in portfolio")
	}
}
