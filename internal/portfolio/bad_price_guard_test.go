package portfolio

import (
	"math"
	"testing"
	"time"
)

// badPriceGuardFixture builds a series at a constant base price, then writes a
// single documented bad value (FU-20260930-03: Close<=0 rows exist in stored
// quote data) at index badIdx. A non-positive close is a MISSING observation,
// never a -100% move.
func badPriceGuardFixture(symbol string, n, badIdx int, bad float64) *HistoricalPrices {
	hp := NewHistoricalPrices()
	base := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	for i := range n {
		closePrice := 100.0 + float64(i)
		if i == badIdx {
			closePrice = bad
		}
		hp.prices[symbol] = append(hp.prices[symbol], pricePoint{
			Date:  base.AddDate(0, 0, i),
			Close: closePrice,
		})
	}
	return hp
}

// TestMomentumReturn_IgnoresNonPositiveClose locks the FU-20260930-04 fix: with
// the stored bad value 0 as the LATEST close, the pre-fix code returned
// 0/100-1 = -1 (a fabricated -100% momentum). The bad close is a missing
// observation, so the method returns the same 0 it returns for a short series.
func TestMomentumReturn_IgnoresNonPositiveClose(t *testing.T) {
	hp := badPriceGuardFixture("BAD.TW", 10, 9, 0)
	got := hp.MomentumReturn("BAD.TW", 5)
	if got != 0 {
		t.Errorf("MomentumReturn with latest close 0 = %v, want 0 (missing observation, not a -100%% move)", got)
	}

	nan := badPriceGuardFixture("NAN.TW", 10, 9, math.NaN())
	if got := nan.MomentumReturn("NAN.TW", 5); got != 0 {
		t.Errorf("MomentumReturn with latest close NaN = %v, want 0", got)
	}

	neg := badPriceGuardFixture("NEG.TW", 10, 9, -3)
	if got := neg.MomentumReturn("NEG.TW", 5); got != 0 {
		t.Errorf("MomentumReturn with latest close -3 = %v, want 0", got)
	}

	// Control: the same fixture without the bad value keeps its exact value.
	clean := badPriceGuardFixture("OK.TW", 10, -1, 0)
	if got, want := clean.MomentumReturn("OK.TW", 5), 109.0/104.0-1; math.Abs(got-want) > 1e-12 {
		t.Errorf("clean MomentumReturn = %v, want %v (normal data must not change)", got, want)
	}
}

// TestVolatility_IgnoresNonPositiveClose locks the FU-20260930-04 fix in the
// volatility consumer: the pre-fix loop skipped only series[i-1]==0, so a
// stored Close=0 contributed 0/101-1 = -1 to both the mean and the standard
// deviation. The fixture [100, 101, 0, 102] used to yield 0.505..., and must
// now yield 0 because the only remaining usable pair [100 -> 101] has a single
// observation.
func TestVolatility_IgnoresNonPositiveClose(t *testing.T) {
	hp := badPriceGuardFixture("BAD.TW", 4, 2, 0)
	// Series is 100, 101, 0, 102: rebuild explicitly for readability.
	hp.prices["BAD.TW"] = []pricePoint{
		{Close: 100}, {Close: 101}, {Close: 0}, {Close: 102},
	}
	if got := hp.Volatility("BAD.TW", 3); got != 0 {
		t.Errorf("Volatility with a stored close 0 = %v, want 0", got)
	}

	// Control: clean data keeps its exact value (hand-computed).
	hp.prices["OK.TW"] = []pricePoint{{Close: 100}, {Close: 110}, {Close: 100}, {Close: 110}}
	if got, want := hp.Volatility("OK.TW", 3), 0.08999540851465157; math.Abs(got-want) > 1e-9 {
		t.Errorf("clean Volatility = %v, want %v (normal data must not change)", got, want)
	}
}

// TestOptimizerReturnMatrix_IgnoresNonPositiveClose locks the same fix in the
// Markowitz return matrix (FU-20260930-04): the pre-fix loop guarded only the
// previous close, so a stored Close=0 entered the matrix as -1 and biased the
// mean used for the optimizer's expected returns.
func TestOptimizerReturnMatrix_IgnoresNonPositiveClose(t *testing.T) {
	hp := badPriceGuardFixture("AAA.TW", 25, -1, 0)
	hp2 := badPriceGuardFixture("BBB.TW", 25, -1, 0)
	for _, p := range hp2.prices["BBB.TW"] {
		hp.prices["BBB.TW"] = append(hp.prices["BBB.TW"], pricePoint{Date: p.Date, Close: p.Close * 2})
	}
	// Stamp the bad value on the LAST bar of BBB only.
	bad := hp.prices["BBB.TW"]
	bad[len(bad)-1].Close = 0

	o := NewOptimizer().WithHistoricalPrices(hp)
	rm := o.extractReturnMatrix([]string{"AAA.TW", "BBB.TW"})
	if rm == nil {
		t.Fatal("extractReturnMatrix returned nil for a 25-bar, 2-symbol panel")
	}
	idx := -1
	for i, sym := range rm.assets {
		if sym == "BBB.TW" {
			idx = i
		}
	}
	if idx < 0 {
		t.Fatalf("BBB.TW missing from return matrix assets %v", rm.assets)
	}
	last := len(rm.returns) - 1
	if got := rm.returns[last][idx]; got != 0 {
		t.Errorf("return from a stored close 0 = %v, want 0 (missing observation, not a -100%% return)", got)
	}
	for t2 := range rm.returns {
		if got := rm.returns[t2][idx]; got <= -0.5 {
			t.Errorf("returns[%d][BBB.TW] = %v: a non-positive close leaked into the matrix", t2, got)
		}
	}
}
