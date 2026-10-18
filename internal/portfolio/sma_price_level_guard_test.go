package portfolio

import (
	"math"
	"testing"
)

// nearly reports whether got is within 1e-12 of want. It is written in the
// positive form on purpose: a NaN result must FAIL the assertion, and
// `math.Abs(NaN-want) > tol` is false, which would let the NaN through.
func nearly(got, want float64) bool {
	return math.Abs(got-want) <= 1e-12
}

// TestSMA_IgnoresUnusableCloseInWindow locks k3 decision 3-2 for the price
// LEVEL consumer portfolio.HistoricalPrices.SMA: a close that is not a usable
// price (the stored Close=0 of FU-20260930-03, or a negative / non-finite
// value) is a MISSING observation, so it must not be summed into the average.
// Pre-fix the bad close entered the sum and the mean was divided by the fixed
// window length: the 5-day average of [0 106 107 108 109] came out as 86
// instead of 107.5, i.e. every consumer of the level read a price that never
// existed.
func TestSMA_IgnoresUnusableCloseInWindow(t *testing.T) {
	// closes 100..104, then the bad value at index 5, then 106..109.
	bad := badPriceGuardFixture("BAD.TW", 10, 5, 0)
	if got := bad.SMA("BAD.TW", 5); !nearly(got, 107.5) {
		t.Errorf("SMA(5) over [0 106 107 108 109] = %v, want 107.5 (missing observation skipped, mean of the 4 usable closes)", got)
	}

	// A negative close is the same kind of missing observation (the predicate
	// covers negative and non-finite values too).
	negative := badPriceGuardFixture("NEG.TW", 10, 5, -5)
	if got := negative.SMA("NEG.TW", 5); !nearly(got, 107.5) {
		t.Errorf("SMA(5) with a -5 close in the window = %v, want 107.5", got)
	}

	nan := badPriceGuardFixture("NAN.TW", 10, 5, math.NaN())
	if got := nan.SMA("NAN.TW", 5); !nearly(got, 107.5) {
		t.Errorf("SMA(5) with a NaN close in the window = %v, want 107.5", got)
	}
}

// TestSMA_CleanDataUnchanged pins the normal-data side: with no unusable close
// the mean is the plain sum over the window divided by the window length, so
// the reported level is byte-identical to the pre-fix implementation.
func TestSMA_CleanDataUnchanged(t *testing.T) {
	clean := badPriceGuardFixture("CLEAN.TW", 10, -1, 0) // closes 100..109
	if got := clean.SMA("CLEAN.TW", 5); !nearly(got, 107) {
		t.Errorf("SMA(5) over [105 106 107 108 109] = %v, want 107", got)
	}
}

// TestSMA_NoUsableCloseIsZero covers the degenerate window: when every close in
// it is unusable the result must be the same "no data" value the method
// returns for a short series (0), not NaN from dividing by a zero count.
func TestSMA_NoUsableCloseIsZero(t *testing.T) {
	hp := NewHistoricalPrices()
	for range 5 {
		hp.prices["ZERO.TW"] = append(hp.prices["ZERO.TW"], pricePoint{Close: 0})
	}
	if got := hp.SMA("ZERO.TW", 3); got != 0 {
		t.Errorf("SMA(3) over an all-unusable window = %v, want 0", got)
	}
}
