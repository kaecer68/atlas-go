package narrative

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestTaiwanStressCalculator_Calculate(t *testing.T) {
	calc := NewTaiwanStressCalculator(nil)

	snap := marketdata.MacroDataSnapshot{
		DXY:                marketdata.MacroDataPoint{Value: 104, ChangePct: 0.5},
		US10Y:              marketdata.MacroDataPoint{Value: 4.5},
		VIX:                marketdata.MacroDataPoint{Value: 20},
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: -5},
		RecordedAt:         1713000000,
	}
	geo := GeopoliticalRiskScore{Intensity: 30}

	idx := calc.Calculate(snap, geo)

	if idx.Score < 0 || idx.Score > 100 {
		t.Fatalf("score out of range: %v", idx.Score)
	}
	if idx.Regime == "" {
		t.Fatal("expected non-empty regime")
	}
	if idx.Timestamp != snap.RecordedAt {
		t.Fatalf("timestamp mismatch: got %v, want %v", idx.Timestamp, snap.RecordedAt)
	}

	// Verify component contributions exist.
	expectedKeys := []string{"dxy", "us10y", "foreign_flow", "vix", "geopolitical"}
	for _, k := range expectedKeys {
		if _, ok := idx.Components[k]; !ok {
			t.Fatalf("missing component %s", k)
		}
	}
}

func TestTaiwanStressCalculator_RegimeThresholds(t *testing.T) {
	calc := NewTaiwanStressCalculator(nil)

	tests := []struct {
		name string
		snap marketdata.MacroDataSnapshot
		geo  float64
		want string
	}{
		{
			name: "low",
			snap: marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{ChangePct: 8.5}, US10Y: marketdata.MacroDataPoint{Value: 0}, VIX: marketdata.MacroDataPoint{Value: 0}, ForeignInvestorNet: marketdata.MacroDataPoint{Value: 0}},
			geo:  10,
			want: "low",
		},
		{
			name: "alert",
			snap: marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{ChangePct: 20}, US10Y: marketdata.MacroDataPoint{Value: 11}, VIX: marketdata.MacroDataPoint{Value: 0}, ForeignInvestorNet: marketdata.MacroDataPoint{Value: 0}},
			geo:  30,
			want: "alert",
		},
		{
			name: "high",
			snap: marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{ChangePct: 20}, US10Y: marketdata.MacroDataPoint{Value: 40}, VIX: marketdata.MacroDataPoint{Value: 40}, ForeignInvestorNet: marketdata.MacroDataPoint{Value: 0}},
			geo:  60,
			want: "high",
		},
		{
			name: "crisis",
			snap: marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{ChangePct: 20}, US10Y: marketdata.MacroDataPoint{Value: 80}, VIX: marketdata.MacroDataPoint{Value: 80}, ForeignInvestorNet: marketdata.MacroDataPoint{Value: -10}},
			geo:  100,
			want: "crisis",
		},
	}

	for _, tt := range tests {
		geo := GeopoliticalRiskScore{Intensity: tt.geo}
		idx := calc.Calculate(tt.snap, geo)
		if idx.Regime != tt.want {
			t.Fatalf("%s: expected regime %q, got %q (score=%.1f)", tt.name, tt.want, idx.Regime, idx.Score)
		}
	}
}
