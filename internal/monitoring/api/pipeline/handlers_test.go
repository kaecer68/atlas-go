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
