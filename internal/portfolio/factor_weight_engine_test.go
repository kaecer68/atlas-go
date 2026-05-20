package portfolio

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/narrative"
)

func TestFactorWeightEngine_GetWeights_Default(t *testing.T) {
	engine := NewFactorWeightEngine()
	weights := engine.GetWeights("")

	if len(weights) != 8 {
		t.Errorf("expected 8 factors, got %d", len(weights))
	}

	var total float64
	for _, w := range weights {
		total += w
	}
	if total < 0.99 || total > 1.01 {
		t.Errorf("weights should sum to ~1.0, got %f", total)
	}
}

func TestFactorWeightEngine_GetWeights_BullRegime(t *testing.T) {
	engine := NewFactorWeightEngine()
	weights := engine.GetWeights("bull")

	if weights[FactorMomentum] <= weights[FactorQuality] {
		t.Error("bull regime should boost momentum above quality")
	}
}

func TestFactorWeightEngine_GetWeights_BearRegime(t *testing.T) {
	engine := NewFactorWeightEngine()
	weights := engine.GetWeights("bear")

	if weights[FactorQuality] <= weights[FactorMomentum] {
		t.Error("bear regime should boost quality above momentum")
	}
}

func TestFactorWeightEngine_GetWeights_WithEvent(t *testing.T) {
	engine := NewFactorWeightEngine()
	
	event := &narrative.NarrativeEvent{
		ID:       "test-event-1",
		Theme:    "AI_capex_surge",
		Severity: "high",
		Status:   "active",
	}
	engine.AddEvent(event)
	
	weights := engine.GetWeights("")
	
	if weights[FactorQuality] <= 0.20 {
		t.Error("AI capex surge should boost quality weight")
	}
	if weights[FactorMomentum] <= 0.25 {
		t.Error("AI capex surge should boost momentum weight")
	}
}

func TestFactorWeightEngine_GetWeights_EventClamped(t *testing.T) {
	engine := NewFactorWeightEngine()
	
	for i := 0; i < 10; i++ {
		event := &narrative.NarrativeEvent{
			ID:       "test-event-" + string(rune('a'+i)),
			Theme:    "AI_capex_surge",
			Severity: "critical",
			Status:   "active",
		}
		engine.AddEvent(event)
	}
	
	weights := engine.GetWeights("")
	
	for ft, w := range weights {
		if w < 0.01 || w > 0.50 {
			t.Errorf("weight for %s should be clamped [0.01, 0.50], got %f", ft, w)
		}
	}
}

func TestFactorWeightEngine_OnRegimeChange(t *testing.T) {
	engine := NewFactorWeightEngine()
	
	engine.OnRegimeChange("", "RISK_ON", 0.8)
	weights := engine.GetWeights("")
	
	if weights[FactorMomentum] <= 0.20 {
		t.Error("RISK_ON regime should boost momentum")
	}
	
	engine.OnRegimeChange("RISK_ON", "RISK_OFF", 0.8)
	weights = engine.GetWeights("")
	
	if weights[FactorQuality] <= 0.15 {
		t.Error("RISK_OFF regime should boost quality")
	}
}

func TestFactorWeightEngine_RemoveEvent(t *testing.T) {
	engine := NewFactorWeightEngine()
	
	event := &narrative.NarrativeEvent{
		ID:       "test-event-1",
		Theme:    "US_rates_up",
		Severity: "high",
		Status:   "active",
	}
	engine.AddEvent(event)
	
	weightsWithEvent := engine.GetWeights("")
	engine.RemoveEvent("test-event-1")
	weightsWithoutEvent := engine.GetWeights("")
	
	if weightsWithEvent[FactorValue] <= weightsWithoutEvent[FactorValue] {
		t.Error("removing event should reduce value weight")
	}
}

func TestFactorWeightEngine_Normalization(t *testing.T) {
	engine := NewFactorWeightEngine()
	
	engine.OnRegimeChange("", "bull", 0.9)
	
	event := &narrative.NarrativeEvent{
		ID:       "test-event-1",
		Theme:    "oil_price_shock",
		Severity: "critical",
		Status:   "active",
	}
	engine.AddEvent(event)
	
	weights := engine.GetWeights("bull")
	
	var total float64
	for _, w := range weights {
		total += w
	}
	
	if total < 0.99 || total > 1.01 {
		t.Errorf("weights should normalize to ~1.0, got %f", total)
	}
}

func TestFactorWeightEngine_Update_RemovesExpired(t *testing.T) {
	engine := NewFactorWeightEngine()
	
	event := &narrative.NarrativeEvent{
		ID:       "test-event-1",
		Theme:    "AI_capex_surge",
		Severity: "high",
		Status:   "expired",
	}
	engine.AddEvent(event)
	engine.Update()
	
	weights := engine.GetWeights("")
	baseWeights := NewFactorWeightEngine().GetWeights("")
	
	for ft := range weights {
		diff := weights[ft] - baseWeights[ft]
		if diff < -0.001 || diff > 0.001 {
			t.Errorf("expired event should not affect weights, got diff %f for %s", diff, ft)
		}
	}
}
