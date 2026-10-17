package portfolio

import (
	"math"
	"testing"
)

// meanTestPanel builds an n-bar panel for two symbols at a constant relative
// shape: AAA.TW = 100+i and BBB.TW = 2*(100+i), so every daily return is a
// clean, non-zero value. badLastZero stamps a stored Close=0 (FU-20260930-03)
// on the last bar of BBB.TW, which is one MISSING observation.
func meanTestPanel(n int, badLastZero bool) *HistoricalPrices {
	hp := badPriceGuardFixture("AAA.TW", n, -1, 0)
	hp2 := badPriceGuardFixture("BBB.TW", n, -1, 0)
	for _, p := range hp2.prices["BBB.TW"] {
		hp.prices["BBB.TW"] = append(hp.prices["BBB.TW"], pricePoint{Date: p.Date, Close: p.Close * 2})
	}
	if badLastZero {
		bad := hp.prices["BBB.TW"]
		bad[len(bad)-1].Close = 0
	}
	return hp
}

func assetIndex(rm *returnMatrix, symbol string) int {
	for i, sym := range rm.assets {
		if sym == symbol {
			return i
		}
	}
	return -1
}

// sumUsableReturns recomputes the mean of the usable returns directly from the
// matrix rows, so the expectation does not restate the implementation's
// denominator.
func sumUsableReturns(rm *returnMatrix, col int) (sum float64, n int) {
	for t := range rm.returns {
		if v := rm.returns[t][col]; v != 0 {
			sum += v
			n++
		}
	}
	return sum, n
}

// TestOptimizerMean_UsesActualSampleCount locks k3 decision 3-4 / the
// FU-20260930-04 follow-up at optimizer_math.go's mean: extractReturnMatrix
// skips unusable closes (one return row per missing observation) but still
// divided the per-asset sum by the FIXED T-1 denominator. One skipped row
// therefore scaled the Markowitz expected return by usable/(T-1) — 23/24 for a
// single missing day — with no error and no log.
func TestOptimizerMean_UsesActualSampleCount(t *testing.T) {
	o := NewOptimizer().WithHistoricalPrices(meanTestPanel(25, true))
	rm := o.extractReturnMatrix([]string{"AAA.TW", "BBB.TW"})
	if rm == nil {
		t.Fatal("extractReturnMatrix returned nil for a 25-bar, 2-symbol panel")
	}
	idx := assetIndex(rm, "BBB.TW")
	if idx < 0 {
		t.Fatalf("BBB.TW missing from return matrix assets %v", rm.assets)
	}

	sum, n := sumUsableReturns(rm, idx)
	if n != 23 {
		t.Fatalf("usable BBB.TW returns = %d, want 23 (one missing observation skipped)", n)
	}
	want := sum / float64(n)
	if math.Abs(rm.means[idx]-want) > 1e-12 {
		t.Errorf("means[BBB.TW] = %.12f, want %.12f (sum over %d usable returns, not sum/%d)",
			rm.means[idx], want, n, len(rm.returns))
	}

	// The clean asset has no skipped row, so its mean must be bit-identical to
	// sum/(T-1): normal data must not change.
	cleanIdx := assetIndex(rm, "AAA.TW")
	csum, cn := sumUsableReturns(rm, cleanIdx)
	if cn != 24 {
		t.Fatalf("usable AAA.TW returns = %d, want 24", cn)
	}
	if cwant := csum / float64(cn); math.Abs(rm.means[cleanIdx]-cwant) > 1e-12 {
		t.Errorf("means[AAA.TW] = %.12f, want %.12f", rm.means[cleanIdx], cwant)
	}
}

// TestOptimizerMean_NoUsableReturnsStaysZero pins the degenerate branch: when
// every close in the window is unusable, the mean must stay 0 rather than
// becoming NaN from 0/0.
func TestOptimizerMean_NoUsableReturnsStaysZero(t *testing.T) {
	hp := meanTestPanel(25, false)
	for i := range hp.prices["BBB.TW"] {
		hp.prices["BBB.TW"][i].Close = 0
	}
	o := NewOptimizer().WithHistoricalPrices(hp)
	rm := o.extractReturnMatrix([]string{"AAA.TW", "BBB.TW"})
	if rm == nil {
		t.Fatal("extractReturnMatrix returned nil")
	}
	idx := assetIndex(rm, "BBB.TW")
	if idx < 0 {
		t.Fatalf("BBB.TW missing from return matrix assets %v", rm.assets)
	}
	if got := rm.means[idx]; got != 0 {
		t.Errorf("means[BBB.TW] with no usable returns = %v, want 0", got)
	}
}
