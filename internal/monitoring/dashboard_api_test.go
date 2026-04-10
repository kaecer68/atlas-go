package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
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

	t.Run("agent-observatory invalid limit", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/dashboard/agent-observatory?limit=abc", nil)
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rr.Code)
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
