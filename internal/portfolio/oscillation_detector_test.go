package portfolio

import "testing"

func TestDetectOscillation_NoOpOnFirstCall(t *testing.T) {
	resetOscillationState()
	current := map[FactorType]float64{"momentum": 0.3, "value": 0.3}
	optimal := map[FactorType]float64{"momentum": 0.35, "value": 0.25}
	v := detectOscillation(current, optimal)
	if v.Detected {
		t.Fatalf("expected no oscillation on first call, got %+v", v)
	}
	if v.DampingFactor != 1.0 {
		t.Fatalf("expected DampingFactor=1.0, got %v", v.DampingFactor)
	}
}

func TestDetectOscillation_FlipsTriggerDampening(t *testing.T) {
	resetOscillationState()
	cur := map[FactorType]float64{"momentum": 0.30, "value": 0.30}
	opt1 := map[FactorType]float64{"momentum": 0.40, "value": 0.40}
	_ = detectOscillation(cur, opt1)

	cur2 := opt1
	opt2 := map[FactorType]float64{"momentum": 0.25, "value": 0.25}
	v := detectOscillation(cur2, opt2)
	if !v.Detected {
		t.Fatalf("expected oscillation after both factors flipped, got %+v", v)
	}
	if v.DampingFactor != 0.5 {
		t.Fatalf("expected DampingFactor=0.5, got %v", v.DampingFactor)
	}
	if len(v.AffectedFactors) != 2 {
		t.Fatalf("expected 2 affected factors, got %d", len(v.AffectedFactors))
	}
}

func TestDampenWeights_HalfwayFromCurrent(t *testing.T) {
	current := map[FactorType]float64{"momentum": 0.30}
	optimal := map[FactorType]float64{"momentum": 0.50}
	dampened := dampenWeights(optimal, current, 0.5)
	if got := dampened["momentum"]; got != 0.40 {
		t.Fatalf("expected midpoint 0.40, got %v", got)
	}
}
