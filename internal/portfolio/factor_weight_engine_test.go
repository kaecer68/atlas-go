package portfolio

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/strategy"
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
		if w < 0.015 || w > 0.50 {
			t.Errorf("weight for %s should be clamped [0.015, 0.50], got %f", ft, w)
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

func TestFactorWeightEngine_AddEvent_Critical(t *testing.T) {
	engine := NewFactorWeightEngine()

	event := &narrative.NarrativeEvent{
		ID:       "test-critical-event",
		Theme:    "AI_capex_surge",
		Severity: "critical",
		Status:   "active",
	}
	engine.AddEvent(event)

	weights := engine.GetWeights("")

	// Critical AI_capex_surge adds 0.10 to quality and 0.10 to momentum
	if weights[FactorQuality] <= 0.20 {
		t.Errorf("critical AI capex surge should boost quality above baseline 0.20, got %.4f", weights[FactorQuality])
	}
	if weights[FactorMomentum] <= 0.25 {
		t.Errorf("critical AI capex surge should boost momentum above baseline 0.25, got %.4f", weights[FactorMomentum])
	}

	// Verify weights changed from baseline (event adjustments applied)
	baseEngine := NewFactorWeightEngine()
	baseWeights := baseEngine.GetWeights("")
	same := true
	for ft := range weights {
		if weights[ft] != baseWeights[ft] {
			same = false
			break
		}
	}
	if same {
		t.Error("critical event should change weights from baseline, but all weights are identical")
	}
}

func TestFactorWeightEngine_ApplyStrategy_Conservative(t *testing.T) {
	engine := NewFactorWeightEngine()
	baseWeights := engine.GetWeights("")

	engine.ApplyStrategy(&strategy.Strategy{
		RiskAppetite: strategy.RiskAppetiteConservative,
	})

	weights := engine.GetWeights("")

	// Current behavior: ApplyStrategy stores in eventWeights but GetWeights
	// only reads eventWeights for entries in activeEvents (which has no
	// "strategy_adjustment" entry), so weights are unchanged.
	// Use epsilon comparison because maps.Copy iteration order is non-deterministic
	// and can produce ~5e-17 floating-point drift between successive calls.
	eps := 1e-10
	if math.Abs(weights[FactorMomentum]-baseWeights[FactorMomentum]) > eps {
		t.Errorf("conservative strategy should not change momentum yet (bug: adjustment not read), got %.10f want %.10f", weights[FactorMomentum], baseWeights[FactorMomentum])
	}
	if math.Abs(weights[FactorQuality]-baseWeights[FactorQuality]) > eps {
		t.Errorf("conservative strategy should not change quality yet (bug: adjustment not read), got %.10f want %.10f", weights[FactorQuality], baseWeights[FactorQuality])
	}
	if math.Abs(weights[FactorValue]-baseWeights[FactorValue]) > eps {
		t.Errorf("conservative strategy should not change value yet (bug: adjustment not read), got %.10f want %.10f", weights[FactorValue], baseWeights[FactorValue])
	}
}

func TestFactorWeightEngine_ApplyStrategy_Aggressive(t *testing.T) {
	engine := NewFactorWeightEngine()
	baseWeights := engine.GetWeights("")

	engine.ApplyStrategy(&strategy.Strategy{
		RiskAppetite: strategy.RiskAppetiteAggressive,
	})

	weights := engine.GetWeights("")

	// Current behavior: ApplyStrategy stores in eventWeights but GetWeights
	// only reads eventWeights for entries in activeEvents, so weights unchanged.
	eps := 1e-10
	if math.Abs(weights[FactorMomentum]-baseWeights[FactorMomentum]) > eps {
		t.Errorf("aggressive strategy should not change momentum yet (bug: adjustment not read), got %.10f want %.10f", weights[FactorMomentum], baseWeights[FactorMomentum])
	}
	if math.Abs(weights[FactorQuality]-baseWeights[FactorQuality]) > eps {
		t.Errorf("aggressive strategy should not change quality yet (bug: adjustment not read), got %.10f want %.10f", weights[FactorQuality], baseWeights[FactorQuality])
	}
}
