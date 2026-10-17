package monitoring

import "testing"

// badPriceHistory is a HistoricalPriceProvider whose series contains the
// documented bad stored value Close=0 (FU-20260930-03).
type badPriceHistory struct {
	series map[string][]float64
}

func (b *badPriceHistory) GetCloseSeries(symbol string) []float64 { return b.series[symbol] }

// TestRealizedVolatility_IgnoresNonPositiveClose locks FU-20260930-04 in the
// Layer 2.5 volatility check: the pre-fix loop skipped only series[i-1]==0, so
// a stored Close=0 contributed 0/101-1 = -1 and inflated the estimate (the
// fixture [100,110,0,100] used to return 0.55 instead of 0).
func TestRealizedVolatility_IgnoresNonPositiveClose(t *testing.T) {
	f := NewRiskExclusionFilter(nil, nil, &badPriceHistory{series: map[string][]float64{
		"BAD": {100, 110, 0, 100},
		"OK":  {100, 110, 100, 110},
	}})

	if got := f.realizedVolatility("BAD", 3); got != 0 {
		t.Errorf("realizedVolatility with a stored close 0 = %v, want 0", got)
	}
	if got, want := f.realizedVolatility("OK", 3), 0.08999540851465157; diff(got, want) > 1e-9 {
		t.Errorf("clean realizedVolatility = %v, want %v (normal data must not change)", got, want)
	}
}

// TestMaxDrawdown_IgnoresNonPositiveClose locks FU-20260930-04 in the drawdown
// check: a stored Close=0 was read as a full peak-to-trough collapse
// ((peak-0)/peak = 100%), which alone exceeds the 30% exclusion threshold and
// excluded the symbol on a data glitch.
func TestMaxDrawdown_IgnoresNonPositiveClose(t *testing.T) {
	f := NewRiskExclusionFilter(nil, nil, &badPriceHistory{series: map[string][]float64{
		"BAD": {100, 120, 0, 90},
		"OK":  {100, 120, 90},
	}})

	if got, want := f.maxDrawdown("BAD", 10), 0.25; diff(got, want) > 1e-12 {
		t.Errorf("maxDrawdown with a stored close 0 = %v, want %v", got, want)
	}
	if got, want := f.maxDrawdown("OK", 10), 0.25; diff(got, want) > 1e-12 {
		t.Errorf("clean maxDrawdown = %v, want %v (normal data must not change)", got, want)
	}
}

func diff(a, b float64) float64 {
	if a > b {
		return a - b
	}
	return b - a
}
