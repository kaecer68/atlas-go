package feature

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// withGapMultiple returns a copy of bars whose bar at idx carries a corrupted
// stored multiple (10x) instead of its real close (FU-20260930-03: 5904.TW was
// stored at 720 for a session whose official close was 72). Both closes remain
// usable prices, so the price guard alone cannot reject it.
func withGapMultiple(bars []domain.DailyBar, idx int) []domain.DailyBar {
	out := make([]domain.DailyBar, len(bars))
	copy(out, bars)
	out[idx].Close *= 10
	return out
}

// TestFeatureReturn1d_RejectsGapMultiple locks k3 decision 3-1 in the ML
// features: before the gate, a 10x stored close produced return_1d = +9 for the
// bar and -0.9 for the next one, and both went into the model unchanged.
func TestFeatureReturn1d_RejectsGapMultiple(t *testing.T) {
	idx := 10 // well past the IPO exemption window
	bars := withGapMultiple(makeBars(12), idx)
	if got := Registry["return_1d"](bars[idx], idx, bars); got != 0 {
		t.Errorf("return_1d with a 10x close = %v, want 0", got)
	}
	if got := Registry["return_1d"](bars[idx+1], idx+1, bars); got != 0 {
		t.Errorf("return_1d after a 10x close = %v, want 0", got)
	}
	// Control: the untouched neighbours keep their exact values.
	if got, want := Registry["return_1d"](bars[5], 5, bars), (105.0-104.0)/104.0; math.Abs(got-want) > 1e-12 {
		t.Errorf("clean return_1d = %v, want %v (normal data must not change)", got, want)
	}
}

// TestFeatureReturn5d_RejectsGapMultiple is the 5-day sibling. Its horizon is 5
// days, so the gross-error bound compounds instead of staying at 50%.
func TestFeatureReturn5d_RejectsGapMultiple(t *testing.T) {
	idx := 10
	bars := withGapMultiple(makeBars(12), idx)
	if got := Registry["return_5d"](bars[idx], idx, bars); got != 0 {
		t.Errorf("return_5d spanning a 10x close = %v, want 0", got)
	}
	// A legal +61% five-day move (10% limit compounds) must still pass.
	ok := makeBars(12)
	ok[10].Close = ok[5].Close * 1.61
	if got := Registry["return_5d"](ok[10], 10, ok); got <= 0.5 {
		t.Errorf("return_5d across a legal +61%% 5-day move = %v, want the real value", got)
	}
}

// TestForwardReturnLabel_RejectsGapMultiple locks the label extractor: a
// corrupted forward close used to become a training label of +9.
func TestForwardReturnLabel_RejectsGapMultiple(t *testing.T) {
	idx := 6
	bars := withGapMultiple(makeBars(10), idx)
	labels := ForwardReturnLabel()(bars)
	if labels[idx-1] != 0 {
		t.Errorf("label into a 10x close = %v, want 0", labels[idx-1])
	}
	if labels[idx] != 0 {
		t.Errorf("label out of a 10x close = %v, want 0", labels[idx])
	}
	if want := (108.0 - 107.0) / 107.0; math.Abs(labels[idx+1]-want) > 1e-12 {
		t.Errorf("clean label = %v, want %v (normal data must not change)", labels[idx+1], want)
	}
}

// TestFeatureReturn1d_IPOExemptMovePasses pins the first mandatory condition of
// k3 decision 3-1 at the feature layer: a newly listed symbol has no daily price
// limit, so a >50% move inside its first 5 sessions is real data.
func TestFeatureReturn1d_IPOExemptMovePasses(t *testing.T) {
	bars := makeBars(6)
	bars[2].Close = bars[1].Close * 1.8 // 4th session of the series
	if got := Registry["return_1d"](bars[2], 2, bars); math.Abs(got-0.8) > 1e-12 {
		t.Errorf("return_1d on an IPO-window move = %v, want 0.8 (no price limit applies)", got)
	}
	// The same move at bar 6 is outside the window and must be rejected.
	later := makeBars(10)
	later[7].Close = later[6].Close * 1.8
	if got := Registry["return_1d"](later[7], 7, later); got != 0 {
		t.Errorf("return_1d on a +80%% move outside the IPO window = %v, want 0", got)
	}
}
