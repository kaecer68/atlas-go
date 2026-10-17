package scheduler

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// gapMultipleBars builds n bars whose close is 100+i, with bars[badIdx]
// multiplied by 10 to reproduce the corrupted stored multiple of
// FU-20260930-03 (5904.TW stored at 720 for a session whose official close was
// 72). badIdx < 0 leaves the series clean.
func gapMultipleBars(n, badIdx int) []domain.DailyBar {
	bars := badCloseBars(n, -1)
	if badIdx >= 0 {
		bars[badIdx].Close *= 10
	}
	return bars
}

// TestExtractLabels_SkipsGapMultiple locks k3 decision 3-1 for the ML retrain
// job: a 10x stored close used to become a +800% training label while the feature
// row for the same sample looked normal, so the model learned from a price that
// never existed.
func TestExtractLabels_SkipsGapMultiple(t *testing.T) {
	bars := gapMultipleBars(12, 7) // bad bar inside the sampleable range [5, n-2]
	X := extractFeatures(bars)
	y := extractLabels(bars)

	if len(y) != len(X) {
		t.Fatalf("label/feature length mismatch: %d labels vs %d feature rows", len(y), len(X))
	}
	// Indexes 6 and 7 touch the corrupted bar (bar 6 -> bar 7 and bar 7 -> bar
	// 8), leaving indexes 5, 8, 9 and 10.
	if len(y) != 4 {
		t.Fatalf("got %d samples, want 4 (every pair touching the 10x bar is skipped)", len(y))
	}
	for i, v := range y {
		if math.Abs(v) > 0.5 || math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("label[%d] = %v: a corrupted multiple became a training label", i, v)
		}
	}

	// Control: the same series without the corrupted bar keeps its exact
	// label values.
	clean := gapMultipleBars(12, -1)
	cy := extractLabels(clean)
	if len(cy) != 6 {
		t.Fatalf("clean data produced %d labels, want 6 (normal data must not change)", len(cy))
	}
	if want := 107.0/106.0 - 1; math.Abs(cy[1]-want) > 1e-12 {
		t.Errorf("clean label[1] = %v, want %v", cy[1], want)
	}
}
