package main

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/narrative"
)

func TestWeightedAccuracy(t *testing.T) {
	weights := narrative.StressIndexWeights{DXY: 0.5, US10Y: 0.5}
	accuracies := map[string]float64{"dxy": 0.8, "us10y": 0.6}
	got := weightedAccuracy(weights, accuracies)
	if math.Abs(got-0.7) > 1e-9 {
		t.Fatalf("weightedAccuracy() = %v, want 0.7", got)
	}
}

func TestFactorsBelowFloor(t *testing.T) {
	weights := narrative.StressIndexWeights{DXY: 0.04, US10Y: 0.06, ForeignFlow: 0.05}
	got := factorsBelowFloor(weights, 0.05)
	want := []string{"dxy", "vix", "jpy", "geopolitical", "oil", "gold"}
	if len(got) != len(want) {
		t.Fatalf("factorsBelowFloor() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("factorsBelowFloor() = %v, want %v", got, want)
		}
	}
}
