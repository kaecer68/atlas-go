package tax

import (
	"math"
	"testing"
)

func TestNewTaiwanCostModel(t *testing.T) {
	cm := NewTaiwanCostModel(0.00654, 0.003)
	if cm.AvgTradingCost != 0.00654 {
		t.Errorf("AvgTradingCost = %v, want 0.00654", cm.AvgTradingCost)
	}
	if cm.TaxRate != 0.003 {
		t.Errorf("TaxRate = %v, want 0.003", cm.TaxRate)
	}
}

func TestRoundTripCost(t *testing.T) {
	cm := NewTaiwanCostModel(0.00654, 0.003)

	tests := []struct {
		name     string
		turnover float64
		want     float64
	}{
		{"unit turnover", 1.0, 0.00954},
		{"zero turnover", 0.0, 0.0},
		{"negative turnover", -1.0, 0.0},
		{"half turnover", 0.5, 0.00477},
		{"large turnover", 100.0, 0.954},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cm.RoundTripCost(tt.turnover)
			if math.Abs(got-tt.want) > 0.00001 {
				t.Errorf("RoundTripCost(%v) = %v, want %v", tt.turnover, got, tt.want)
			}
		})
	}
}

func TestNetReturn(t *testing.T) {
	cm := NewTaiwanCostModel(0.00654, 0.003)

	tests := []struct {
		name      string
		rawReturn float64
		turnover  float64
		want      float64
	}{
		{"positive raw, positive turnover", 0.05, 1.0, 0.04046},
		{"zero raw", 0.0, 1.0, -0.00954},
		{"zero turnover", 0.05, 0.0, 0.05},
		{"negative raw", -0.02, 1.0, -0.02954},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cm.NetReturn(tt.rawReturn, tt.turnover)
			if math.Abs(got-tt.want) > 0.00001 {
				t.Errorf("NetReturn(%v, %v) = %v, want %v", tt.rawReturn, tt.turnover, got, tt.want)
			}
		})
	}
}

func TestApplyToSeries(t *testing.T) {
	cm := NewTaiwanCostModel(0.00654, 0.003)

	rawReturns := []float64{0.05, 0.02, -0.01}
	turnovers := []float64{1.0, 0.5, 1.0}

	result := cm.ApplyToSeries(rawReturns, turnovers)

	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3", len(result))
	}

	want := []float64{0.04046, 0.01523, -0.01954}
	for i, w := range want {
		if math.Abs(result[i]-w) > 0.00001 {
			t.Errorf("result[%d] = %v, want %v", i, result[i], w)
		}
	}

	if rawReturns[0] != 0.05 {
		t.Error("ApplyToSeries mutated rawReturns input")
	}

	t.Run("mismatched lengths", func(t *testing.T) {
		short := []float64{0.01, 0.02}
		result := cm.ApplyToSeries(rawReturns, short)
		if len(result) != 2 {
			t.Errorf("len(result) = %d, want 2", len(result))
		}
	})
}
