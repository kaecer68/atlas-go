package portfolio

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// fixtureSnapshot returns a MacroDataSnapshot with realistic values for testing.
func fixtureSnapshot() marketdata.MacroDataSnapshot {
	return marketdata.MacroDataSnapshot{
		VIX:                 marketdata.MacroDataPoint{Value: 22.0, ChangePct: 5.0},
		DXY:                 marketdata.MacroDataPoint{Value: 104.0, ChangePct: 0.5},
		US10Y:               marketdata.MacroDataPoint{Value: 4.2, ChangePct: 1.0},
		ForeignInvestorNet:  marketdata.MacroDataPoint{Value: 25e8, ChangePct: 10.0},
		DomesticFundNet:     marketdata.MacroDataPoint{Value: -5e8, ChangePct: -5.0},
		RetailMarginBalance: marketdata.MacroDataPoint{Value: 10e8, ChangePct: 3.0},
	}
}

func TestFactorBridge_Standardize(t *testing.T) {
	fb := NewFactorBridge()

	tests := []struct {
		name  string
		value float64
		avg   float64
		std   float64
		want  float64
	}{
		{"zero std returns 0", 100, 0, 0, 0},
		{"at mean", 50, 50, 10, 0},
		{"one std above", 60, 50, 10, 1.0},
		{"one std below", 40, 50, 10, -1.0},
		{"clamped above", 200, 50, 10, 1.0},
		{"clamped below", -100, 50, 10, -1.0},
		{"within range positive", 55, 50, 10, 0.5},
		{"within range negative", 45, 50, 10, -0.5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := fb.standardize(tc.value, tc.avg, tc.std)
			if !approxEqual(got, tc.want, 0.001) {
				t.Errorf("standardize(%v, %v, %v) = %v, want %v",
					tc.value, tc.avg, tc.std, got, tc.want)
			}
		})
	}
}

func TestFactorBridge_Convert(t *testing.T) {
	fb := NewFactorBridge()
	snap := fixtureSnapshot()
	input := fb.Convert(snap)

	// ForeignFlowScore: standardize(25e8, 0, 50e8) = 0.5
	if !approxEqual(input.ForeignFlowScore, 0.5, 0.001) {
		t.Errorf("ForeignFlowScore = %v, want 0.5", input.ForeignFlowScore)
	}

	// DomesticFlowScore: standardize(-5e8, 0, 50e8) = -0.1
	if !approxEqual(input.DomesticFlowScore, -0.1, 0.001) {
		t.Errorf("DomesticFlowScore = %v, want -0.1", input.DomesticFlowScore)
	}

	// RetailSentimentScore: fallback 3.0 / 10.0 = 0.3
	if !approxEqual(input.RetailSentimentScore, 0.3, 0.001) {
		t.Errorf("RetailSentimentScore = %v, want 0.3", input.RetailSentimentScore)
	}

	// StressLevel: VIX 22→8 + DXY 104→24 + US10Y 4.2→21 = 53
	if !approxEqual(input.StressLevel, 53.0, 0.001) {
		t.Errorf("StressLevel = %v, want 53.0", input.StressLevel)
	}
}

func TestFactorBridge_Convert_ZeroValues(t *testing.T) {
	fb := NewFactorBridge()
	snap := marketdata.MacroDataSnapshot{} // all zeros
	input := fb.Convert(snap)

	if input.ForeignFlowScore != 0 {
		t.Errorf("expected 0 ForeignFlowScore for zero input, got %v", input.ForeignFlowScore)
	}
	if input.DomesticFlowScore != 0 {
		t.Errorf("expected 0 DomesticFlowScore for zero input, got %v", input.DomesticFlowScore)
	}
}

func TestFactorBridge_ComputeRetailSentiment_FallbackPositive(t *testing.T) {
	fb := NewFactorBridge()
	snap := fixtureSnapshot()
	snap.RetailMarginBalance.ChangePct = 8.0

	score := fb.computeRetailSentiment(snap)
	if score <= 0 {
		t.Errorf("expected positive score for 8%% change, got %v", score)
	}
	// 8.0 / 10.0 = 0.8
	if !approxEqual(score, 0.8, 0.01) {
		t.Errorf("expected ~0.8, got %v", score)
	}
}

func TestFactorBridge_ComputeRetailSentiment_FallbackNegative(t *testing.T) {
	fb := NewFactorBridge()
	snap := fixtureSnapshot()
	snap.RetailMarginBalance.ChangePct = -5.0

	score := fb.computeRetailSentiment(snap)
	if score >= 0 {
		t.Errorf("expected negative score for -5%% change, got %v", score)
	}
	// -5.0 / 10.0 = -0.5
	if !approxEqual(score, -0.5, 0.01) {
		t.Errorf("expected ~-0.5, got %v", score)
	}
}

func TestFactorBridge_ComputeRetailSentiment_FallbackExtremePositive(t *testing.T) {
	fb := NewFactorBridge()
	snap := fixtureSnapshot()
	snap.RetailMarginBalance.ChangePct = 15.0

	score := fb.computeRetailSentiment(snap)
	if score != 1.0 {
		t.Errorf("expected 1.0 for >10%% change, got %v", score)
	}
}

func TestFactorBridge_ComputeRetailSentiment_FallbackExtremeNegative(t *testing.T) {
	fb := NewFactorBridge()
	snap := fixtureSnapshot()
	snap.RetailMarginBalance.ChangePct = -15.0

	score := fb.computeRetailSentiment(snap)
	if score != -1.0 {
		t.Errorf("expected -1.0 for < -10%% change, got %v", score)
	}
}

func TestFactorBridge_ComputeStressLevel_LowStress(t *testing.T) {
	fb := NewFactorBridge()
	snap := marketdata.MacroDataSnapshot{
		VIX:   marketdata.MacroDataPoint{Value: 15.0},
		DXY:   marketdata.MacroDataPoint{Value: 100.0},
		US10Y: marketdata.MacroDataPoint{Value: 3.0},
	}

	level := fb.computeStressLevel(snap)
	if level != 0 {
		t.Errorf("expected 0 stress for low indicators, got %v", level)
	}
}

func TestFactorBridge_ComputeStressLevel_HighStress(t *testing.T) {
	fb := NewFactorBridge()
	snap := marketdata.MacroDataSnapshot{
		VIX:   marketdata.MacroDataPoint{Value: 35.0},
		DXY:   marketdata.MacroDataPoint{Value: 110.0},
		US10Y: marketdata.MacroDataPoint{Value: 5.0},
	}

	level := fb.computeStressLevel(snap)
	// VIX 35 > 30 → 40, DXY 110 > 105 → 30, US10Y 5.0 > 4.5 → 30 → total 100, capped
	if level != 100.0 {
		t.Errorf("expected stress level 100.0 (capped), got %v", level)
	}
}

func TestFactorBridge_ComputeStressLevel_MediumStress(t *testing.T) {
	fb := NewFactorBridge()
	snap := marketdata.MacroDataSnapshot{
		VIX:   marketdata.MacroDataPoint{Value: 25.0},
		DXY:   marketdata.MacroDataPoint{Value: 102.0},
		US10Y: marketdata.MacroDataPoint{Value: 4.0},
	}

	level := fb.computeStressLevel(snap)
	// VIX 25 (> 20): (25-20)*4 = 20
	// DXY 102 (> 100): (102-100)*6 = 12
	// US10Y 4.0 (> 3.5): (4.0-3.5)*30 = 15
	// total = 47
	if level != 47.0 {
		t.Errorf("expected stress level 47.0, got %v", level)
	}
}

func TestFactorBridge_ComputeStressLevel_CappedAt100(t *testing.T) {
	fb := NewFactorBridge()
	snap := marketdata.MacroDataSnapshot{
		VIX:   marketdata.MacroDataPoint{Value: 50.0},
		DXY:   marketdata.MacroDataPoint{Value: 120.0},
		US10Y: marketdata.MacroDataPoint{Value: 8.0},
	}

	level := fb.computeStressLevel(snap)
	// VIX 50 > 30 → 40, DXY 120 > 105 → 30, US10Y 8.0 > 4.5 → 30 → total 100, capped
	if level != 100.0 {
		t.Errorf("stress level %v should be capped at 100.0", level)
	}
}

// approxEqual checks if two float64 values are within epsilon of each other.
func approxEqual(a, b, epsilon float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}
