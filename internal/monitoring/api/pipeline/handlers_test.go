package pipeline

import (
	"encoding/json"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
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
