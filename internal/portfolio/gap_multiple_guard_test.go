package portfolio

import (
	"math"
	"testing"
)

// gapMultipleFixture builds a 25-bar series that is FLAT at 100 except one bar
// that carries the documented corrupted multiple (FU-20260930-03: 5904.TW was
// stored at 720 for a session whose official close was 72). Both closes are
// usable prices, so the price guard cannot see it.
func gapMultipleFixture(symbol string, n, badIdx int, bad float64) *HistoricalPrices {
	hp := NewHistoricalPrices()
	for i := range n {
		c := 100.0
		if i == badIdx {
			c = bad
		}
		hp.prices[symbol] = append(hp.prices[symbol], pricePoint{Close: c})
	}
	return hp
}

// TestVolatility_RejectsGapMultiple locks k3 decision 3-1 in the volatility
// consumer: before the gate, the 100 -> 1000 pair entered the estimate as a
// +900% return, so a flat series with one corrupted close reported a standard
// deviation of 1.815...
func TestVolatility_RejectsGapMultiple(t *testing.T) {
	hp := gapMultipleFixture("BAD.TW", 25, 20, 1000)
	if got := hp.Volatility("BAD.TW", 24); got != 0 {
		t.Errorf("Volatility with a 10x stored close = %v, want 0 (both odd pairs rejected, the rest are flat)", got)
	}
	// Control: the same series without the corrupted bar stays flat.
	clean := gapMultipleFixture("OK.TW", 25, -1, 0)
	if got := clean.Volatility("OK.TW", 24); got != 0 {
		t.Errorf("clean Volatility = %v, want 0 (normal data must not change)", got)
	}
}

// TestOptimizerReturnMatrix_RejectsGapMultiple locks the same rule in the
// Markowitz return matrix: the mean used for expected returns read the
// corrupted pair as a real +900% observation (0.3375 for the fixture below).
func TestOptimizerReturnMatrix_RejectsGapMultiple(t *testing.T) {
	hp := gapMultipleFixture("BAD.TW", 25, 20, 1000)
	hp2 := gapMultipleFixture("OK.TW", 25, -1, 0)
	for i := range hp2.prices["OK.TW"] {
		hp2.prices["OK.TW"][i].Close = 200
	}
	hp.prices["OK.TW"] = hp2.prices["OK.TW"]

	o := NewOptimizer().WithHistoricalPrices(hp)
	rm := o.extractReturnMatrix([]string{"BAD.TW", "OK.TW"})
	if rm == nil {
		t.Fatal("extractReturnMatrix returned nil")
	}
	idx := assetIndex(rm, "BAD.TW")
	if idx < 0 {
		t.Fatalf("BAD.TW missing from assets %v", rm.assets)
	}
	if got := rm.means[idx]; math.Abs(got) > 1e-12 {
		t.Errorf("means[BAD.TW] = %v, want 0: a corrupted multiple reached the expected-return mean", got)
	}
	for tk := range rm.returns {
		if got := rm.returns[tk][idx]; math.Abs(got) > 1e-12 {
			t.Errorf("returns[%d][BAD.TW] = %v, want 0 (flat series, gap multiple rejected)", tk, got)
		}
	}
}

// TestMomentumReturn_RejectsGapMultiple locks the same rule for the N-day
// momentum consumer, including the horizon rule: a 5-day window that still
// starts after the IPO days must reject a 10x move.
func TestMomentumReturn_RejectsGapMultiple(t *testing.T) {
	hp := gapMultipleFixture("BAD.TW", 25, 24, 1000) // 10x on the latest bar
	if got := hp.MomentumReturn("BAD.TW", 1); got != 0 {
		t.Errorf("MomentumReturn(1) with a 10x latest close = %v, want 0", got)
	}
	if got := hp.MomentumReturn("BAD.TW", 5); got != 0 {
		t.Errorf("MomentumReturn(5) across a 10x close = %v, want 0", got)
	}
	// Control: a genuine, legal 1-day +9.9% move still passes unchanged.
	ok := gapMultipleFixture("OK.TW", 3, -1, 0)
	ok.prices["OK.TW"][2].Close = 109.9
	if got, want := ok.MomentumReturn("OK.TW", 1), 109.9/100-1; math.Abs(got-want) > 1e-12 {
		t.Errorf("legal limit-up MomentumReturn = %v, want %v (normal data must not change)", got, want)
	}
}
