package narrative

import (
	"math"
	"testing"
)

func TestSeasonalAnalyzer_CalculateExpectationGap(t *testing.T) {
	sa := NewSeasonalAnalyzer()

	result := sa.CalculateExpectationGap(0.05, 0.02)

	if math.Abs(result.ExpectationGap-0.03) > 1e-9 {
		t.Errorf("expected gap 0.03, got %f", result.ExpectationGap)
	}
	if result.AlreadyPricedIn {
		t.Error("expected AlreadyPricedIn to be false")
	}
	if result.SurprisePotential != 0 {
		t.Errorf("expected zero SurprisePotential when current < avg, got %f", result.SurprisePotential)
	}
}

func TestSeasonalAnalyzer_CalculateExpectationGap_AlreadyPricedIn(t *testing.T) {
	sa := NewSeasonalAnalyzer()

	result := sa.CalculateExpectationGap(0.03, 0.05)

	if !result.AlreadyPricedIn {
		t.Error("expected AlreadyPricedIn to be true")
	}
}
