package orchestrator

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain/recommendation"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// TestApplyNarrativeContextWithEvents_AllLayers verifies the narrative
// attribution gate is gone: every recommendation layer (not just
// context/superinvestor) receives SupportingEvents + ReasoningChain.
func TestApplyNarrativeContextWithEvents_AllLayers(t *testing.T) {
	sys, err := NewProductionSystemWithEventBus(testProductionSystemConfig(t), nil, nil)
	if err != nil {
		t.Fatalf("NewProductionSystemWithEventBus: %v", err)
	}

	events := []narrative.NarrativeEvent{
		{ID: "evt-ai", Theme: "AI_capex_surge", Region: "Global", Confidence: 0.8, HitRate: 0.9},
		{ID: "evt-fed", Theme: "US_rates_down", Region: "US", Confidence: 0.7, HitRate: 0.8},
	}

	layers := []shared.AgentLayer{
		shared.LayerMacro, shared.LayerContext, shared.LayerSuperinvestor, shared.LayerSector,
	}
	var recs []recommendation.Recommendation
	for _, l := range layers {
		recs = append(recs, recommendation.Recommendation{
			Agent:  "agent-" + string(l),
			Layer:  l,
			Reason: "base reason",
		})
	}

	enriched := sys.applyNarrativeContextWithEvents(recs, events)
	if len(enriched) != len(recs) {
		t.Fatalf("expected %d enriched recs, got %d", len(recs), len(enriched))
	}

	for i, rec := range enriched {
		if len(rec.SupportingEvents) != 2 {
			t.Errorf("layer %s: expected 2 supporting events, got %d", layers[i], len(rec.SupportingEvents))
		}
		if len(rec.ReasoningChain) == 0 {
			t.Errorf("layer %s: expected non-empty reasoning chain", layers[i])
		}
		if rec.Reason == "base reason" {
			t.Errorf("layer %s: expected narrative suffix appended to Reason", layers[i])
		}
	}
}

// TestApplyNarrativeContextWithEvents_NoEventsNoop verifies the function
// leaves recs untouched when no events are detected.
func TestApplyNarrativeContextWithEvents_NoEventsNoop(t *testing.T) {
	sys, err := NewProductionSystemWithEventBus(testProductionSystemConfig(t), nil, nil)
	if err != nil {
		t.Fatalf("NewProductionSystemWithEventBus: %v", err)
	}

	recs := []recommendation.Recommendation{
		{Agent: "agent-x", Layer: shared.LayerMacro, Reason: "plain"},
	}
	enriched := sys.applyNarrativeContextWithEvents(recs, nil)
	if len(enriched) != 1 {
		t.Fatalf("expected 1 rec, got %d", len(enriched))
	}
	if len(enriched[0].ReasoningChain) != 0 {
		t.Errorf("expected empty reasoning chain with no events, got %v", enriched[0].ReasoningChain)
	}
	if enriched[0].Reason != "plain" {
		t.Errorf("expected unchanged reason, got %q", enriched[0].Reason)
	}
}
