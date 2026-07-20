package pipeline

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// TestHandleSessions_IncludesTopStrategies exercises the CL-4 List endpoint
// (§18.7.2): the response must keep the original 4 metadata fields and add
// a fifth `top_strategies` array with the top-3 strategies ranked by
// Conviction DESC.
func TestHandleSessions_IncludesTopStrategies(t *testing.T) {
	ledgerDir := t.TempDir()
	sessionID := "session-20260101-daily"
	sessionDir := filepath.Join(ledgerDir, "sessions", sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	recordedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	summary := domain.SessionSummary{
		SessionID:    sessionID,
		Regime:       "RISK_ON",
		OutcomeCount: 3,
		RecordedAt:   recordedAt,
	}
	summaryData, _ := json.Marshal(summary)
	if err := os.WriteFile(filepath.Join(sessionDir, "summary.json"), summaryData, 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	store := ledger.NewStore(ledgerDir)
	outcomes := []domain.RecommendationOutcome{
		{AgentID: "agent-low", Symbol: "3008.TW", Side: "BUY", Conviction: 50},
		{AgentID: "agent-high", Symbol: "2330.TW", Side: "BUY", Conviction: 90},
		{AgentID: "agent-mid", Symbol: "2317.TW", Side: "BUY", Conviction: 70},
	}
	if err := store.RecordSessionOutcomes(domain.ReplaySession{ID: sessionID}, outcomes); err != nil {
		t.Fatalf("RecordSessionOutcomes: %v", err)
	}

	h := NewHandlers(service.NewPipelineService(ledgerDir, ledgerDir, store))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/sessions", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}
	var doc map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	sessions, _ := doc["sessions"].([]any)
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	entry := sessions[0].(map[string]any)
	for _, key := range []string{"session_id", "recorded_at", "regime", "outcome_count"} {
		if _, ok := entry[key]; !ok {
			t.Errorf("legacy field %q missing from response", key)
		}
	}
	if entry["session_id"] != sessionID {
		t.Errorf("session_id = %v, want %v", entry["session_id"], sessionID)
	}
	strategies, ok := entry["top_strategies"].([]any)
	if !ok {
		t.Fatalf("top_strategies missing or wrong type: %T", entry["top_strategies"])
	}
	if len(strategies) != 3 {
		t.Fatalf("expected 3 top_strategies, got %d", len(strategies))
	}
	first := strategies[0].(map[string]any)
	if first["agent_id"] != "agent-high" || first["conviction"].(float64) != 90 {
		t.Errorf("top[0] = %v, want agent-high/90", first)
	}
	last := strategies[2].(map[string]any)
	if last["agent_id"] != "agent-low" || last["conviction"].(float64) != 50 {
		t.Errorf("top[2] = %v, want agent-low/50", last)
	}
}

// TestHandleSessionDetail_OK exercises the CL-4 Detail endpoint (§18.7.3):
// 200 with session_id, summary, outcomes, outcome_count when the session
// exists. Outcomes are sorted by Conviction DESC by the service layer;
// the handler just proxies the result.
func TestHandleSessionDetail_OK(t *testing.T) {
	ledgerDir := t.TempDir()
	sessionID := "session-20260101-daily"
	sessionDir := filepath.Join(ledgerDir, "sessions", sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	recordedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	summary := domain.SessionSummary{
		SessionID:    sessionID,
		Regime:       "RISK_ON",
		OutcomeCount: 2,
		RecordedAt:   recordedAt,
		EndingCash:   100.5,
	}
	summaryData, _ := json.Marshal(summary)
	if err := os.WriteFile(filepath.Join(sessionDir, "summary.json"), summaryData, 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	store := ledger.NewStore(ledgerDir)
	outcomes := []domain.RecommendationOutcome{
		{AgentID: "agent-1", Symbol: "2330.TW", Side: "BUY", Conviction: 88},
		{AgentID: "agent-2", Symbol: "2317.TW", Side: "SELL", Conviction: 72},
	}
	if err := store.RecordSessionOutcomes(domain.ReplaySession{ID: sessionID}, outcomes); err != nil {
		t.Fatalf("RecordSessionOutcomes: %v", err)
	}

	h := NewHandlers(service.NewPipelineService(ledgerDir, ledgerDir, store))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/sessions/"+sessionID, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}
	var doc map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc["session_id"] != sessionID {
		t.Errorf("session_id = %v, want %v", doc["session_id"], sessionID)
	}
	summaryMap, ok := doc["summary"].(map[string]any)
	if !ok {
		t.Fatalf("summary missing or wrong type: %T", doc["summary"])
	}
	if summaryMap["ending_cash"].(float64) != 100.5 {
		t.Errorf("summary.ending_cash = %v, want 100.5", summaryMap["ending_cash"])
	}
	gotOutcomes, ok := doc["outcomes"].([]any)
	if !ok {
		t.Fatalf("outcomes missing or wrong type: %T", doc["outcomes"])
	}
	if len(gotOutcomes) != 2 {
		t.Errorf("outcomes len = %d, want 2", len(gotOutcomes))
	}
	if doc["outcome_count"].(float64) != 2 {
		t.Errorf("outcome_count = %v, want 2", doc["outcome_count"])
	}
}

// TestHandleSessionDetail_NotFound verifies the 404 path: sessionID is
// unknown (no summary.json on disk) and the handler must distinguish
// "not found" from "system error".
func TestHandleSessionDetail_NotFound(t *testing.T) {
	ledgerDir := t.TempDir()
	store := ledger.NewStore(ledgerDir)
	h := NewHandlers(service.NewPipelineService(ledgerDir, ledgerDir, store))

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/sessions/does-not-exist", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body=%s)", rr.Code, rr.Body.String())
	}
	var doc map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc["error"] == nil {
		t.Errorf("expected error message in 404 body, got %v", doc)
	}
	if doc["session_id"] != "does-not-exist" {
		t.Errorf("session_id in body = %v, want does-not-exist", doc["session_id"])
	}
}

// TestPipelineItem_MetricsOmittedWhenEmpty proves that the Metrics field
// is properly omitted from JSON when empty (omitempty behavior).
func TestPipelineItem_MetricsOmittedWhenEmpty(t *testing.T) {
	item := PipelineItem{
		Symbol:     "2330",
		AgentID:    "agent1",
		Skill:      "semiconductor_desk",
		Layer:      "sector",
		Side:       "BUY",
		Conviction: 75,
		Price:      550.0,
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Metrics should be omitted when zero value
	if _, hasMetrics := raw["metrics"]; hasMetrics {
		t.Error("expected 'metrics' to be omitted when empty")
	}
}

// TestPipelineItem_MetricsPresentWhenPopulated proves Metrics appears in JSON
// when populated.
func TestPipelineItem_MetricsPresentWhenPopulated(t *testing.T) {
	pe := 15.5
	pb := 2.3
	item := PipelineItem{
		Symbol:     "2330",
		AgentID:    "agent1",
		Skill:      "semiconductor_desk",
		Layer:      "sector",
		Side:       "BUY",
		Conviction: 75,
		Price:      550.0,
		Metrics: &PipelineItemMetrics{
			PriceToEarnings: &pe,
			PriceToBook:     &pb,
		},
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	metrics, ok := raw["metrics"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'metrics' to be present and be an object")
	}

	if metrics["price_to_earnings"] != 15.5 {
		t.Errorf("expected price_to_earnings=15.5, got %v", metrics["price_to_earnings"])
	}
	if metrics["price_to_book"] != 2.3 {
		t.Errorf("expected price_to_book=2.3, got %v", metrics["price_to_book"])
	}
}

// TestPipelineItem_FactorScoresAlignment verifies FactorScores serialization
// includes all expected fields.
func TestPipelineItem_FactorScoresAlignment(t *testing.T) {
	item := PipelineItem{
		Symbol:  "2330",
		AgentID: "agent1",
		FactorScores: domain.FactorScores{
			Momentum:               0.8,
			Value:                  0.6,
			Quality:                0.7,
			Agent:                  0.75,
			InstitutionalSentiment: 0.65,
			Liquidity:              0.9,
			Total:                  0.73,
		},
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	fs, ok := raw["factor_scores"].(map[string]interface{})
	if !ok {
		t.Fatal("expected factor_scores to be present")
	}

	expectedFields := []string{"momentum", "value", "quality", "agent", "institutional_sentiment", "liquidity", "total"}
	for _, field := range expectedFields {
		if _, has := fs[field]; !has {
			t.Errorf("expected factor_scores to have %q", field)
		}
	}
}

func TestRecommendationPipelineResponse_StatusFieldsSerialized(t *testing.T) {
	resp := RecommendationPipelineResponse{
		SessionID:     "session-20260614-daily",
		Status:        service.PipelineStatusDegraded,
		StatusMessage: "控制層過濾記錄未載入（summary.json 缺失），推薦清單仍可用",
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if raw["status"] != string(service.PipelineStatusDegraded) {
		t.Errorf("expected status=%q, got %v", service.PipelineStatusDegraded, raw["status"])
	}
	if raw["status_message"] != "控制層過濾記錄未載入（summary.json 缺失），推薦清單仍可用" {
		t.Errorf("expected status_message in JSON, got %v", raw["status_message"])
	}
}

func TestHandleRecommendationPipeline_StatusPropagatesToResponse(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "session-20260614-daily"
	sessionDir := filepath.Join(baseDir, "sessions", sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	outcome := domain.RecommendationOutcome{
		AgentID:      "agent-1",
		Skill:        "growth_momentum",
		Layer:        domain.LayerStyle,
		Symbol:       "2330.TW",
		Side:         domain.SideBuy,
		Conviction:   88,
		PassedGuards: true,
		RecordedAt:   time.Date(2026, 6, 14, 4, 0, 0, 0, time.UTC),
	}
	outcomeBytes, _ := json.Marshal(outcome)
	if err := os.WriteFile(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"), append(outcomeBytes, '\n'), 0o644); err != nil {
		t.Fatalf("write outcomes: %v", err)
	}

	svc := service.NewPipelineService(baseDir, baseDir, ledger.NewStore(baseDir))
	h := NewHandlers(svc)
	req := httptest.NewRequest("GET", "/api/dashboard/recommendation-pipeline?session_id="+sessionID, nil)
	code, body := h.HandleRecommendationPipeline(req)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if raw["status"] != string(service.PipelineStatusDegraded) {
		t.Errorf("expected status=%q in response body, got %v", service.PipelineStatusDegraded, raw["status"])
	}
	if msg, _ := raw["status_message"].(string); msg == "" {
		t.Error("expected non-empty status_message in response body")
	}
}

// TestHandleRegimeHistory_CanonicalPath verifies A04: GET /api/regime/history
// is registered as an alias for /api/dashboard/regime-history and returns the
// same session-based regime history data.
func TestHandleRegimeHistory_CanonicalPath(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "session-20260101-daily"
	sessionDir := filepath.Join(baseDir, "sessions", sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	recordedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	summary := domain.SessionSummary{
		SessionID:    sessionID,
		Regime:       "RISK_ON",
		OutcomeCount: 1,
		RecordedAt:   recordedAt,
	}
	summaryData, _ := json.Marshal(summary)
	if err := os.WriteFile(filepath.Join(sessionDir, "summary.json"), summaryData, 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	svc := service.NewPipelineService(baseDir, baseDir, ledger.NewStore(baseDir))
	h := NewHandlers(svc)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	for _, path := range []string{"/api/dashboard/regime-history", "/api/regime/history"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200 (body=%s)", path, rr.Code, rr.Body.String())
		}
		var data service.RegimeHistoryData
		if err := json.Unmarshal(rr.Body.Bytes(), &data); err != nil {
			t.Fatalf("%s unmarshal: %v", path, err)
		}
		if len(data.Sessions) != 1 {
			t.Fatalf("%s expected 1 session, got %d", path, len(data.Sessions))
		}
		if data.Sessions[0].Regime != "RISK_ON" {
			t.Errorf("%s regime = %q, want RISK_ON", path, data.Sessions[0].Regime)
		}
	}
}

// TestParseLimit_DaysAlias covers BUG-2 (manifest 2026-07-21-historical-store-time-and-limit-fixes.md).
// parseLimit must accept the `days` query parameter as an alias for `limit`
// so the MCP briefing tool's call to /api/regime/history?days=5 returns
// at most 5 entries rather than the default 30. `limit` retains priority
// when both are present.
func TestParseLimit_DaysAlias(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		def     int
		max     int
		want    int
		wantErr bool
	}{
		{"no_param_returns_default", "", 30, 365, 30, false},
		{"limit_only", "limit=5", 30, 365, 5, false},
		{"days_only", "days=5", 30, 365, 5, false},
		{"both_limit_wins", "limit=7&days=99", 30, 365, 7, false},
		{"days_clamped_to_max", "days=999", 30, 100, 100, false},
		{"limit_clamped_to_max", "limit=999", 30, 100, 100, false},
		{"days_zero_errors", "days=0", 30, 365, 0, true},
		{"days_negative_errors", "days=-5", 30, 365, 0, true},
		{"days_non_integer_errors", "days=abc", 30, 365, 0, true},
		{"days_one_returns_one", "days=1", 30, 365, 1, false},
		{"days_with_whitespace_trimmed", "days=%20%205%20%20", 30, 365, 5, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/regime/history?"+tc.query, nil)
			got, err := parseLimit(req, tc.def, tc.max)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Fatalf("parseLimit(%q)=%d want %d", tc.query, got, tc.want)
			}
		})
	}
}

// TestHandleRegimeHistory_DaysQueryParam is an end-to-end guard for BUG-2:
// /api/regime/history?days=5 must surface ≤5 sessions even though the
// handler reads via parseLimit (which now honours `days`).
func TestHandleRegimeHistory_DaysQueryParam(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "session-20260101-daily"
	sessionDir := filepath.Join(baseDir, "sessions", sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	summary := domain.SessionSummary{
		SessionID:    sessionID,
		Regime:       "RISK_ON",
		OutcomeCount: 1,
		RecordedAt:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	summaryData, _ := json.Marshal(summary)
	if err := os.WriteFile(filepath.Join(sessionDir, "summary.json"), summaryData, 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	svc := service.NewPipelineService(baseDir, baseDir, ledger.NewStore(baseDir))
	h := NewHandlers(svc)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/regime/history?days=1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rr.Code, rr.Body.String())
	}
	var data service.RegimeHistoryData
	if err := json.Unmarshal(rr.Body.Bytes(), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(data.Sessions) > 1 {
		t.Fatalf("?days=1 returned %d sessions, want ≤1", len(data.Sessions))
	}
}
