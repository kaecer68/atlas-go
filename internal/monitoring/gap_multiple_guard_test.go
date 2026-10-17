package monitoring

import "testing"

// TestRealizedVolatility_RejectsGapMultiple locks k3 decision 3-1 in the Layer
// 2.5 volatility check: a corrupted stored multiple (FU-20260930-03: 5904.TW was
// stored at 720 for a session whose official close was 72) is made of two usable
// prices, so the price guard passes and the +900% / -90% pair used to dominate
// the estimate.
func TestRealizedVolatility_RejectsGapMultiple(t *testing.T) {
	f := NewRiskExclusionFilter(nil, nil, &badPriceHistory{series: map[string][]float64{
		"BAD": {100, 100, 100, 100, 100, 100, 100, 1000, 100, 100},
		"OK":  {100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
	}})

	if got := f.realizedVolatility("BAD", 9); got != 0 {
		t.Errorf("realizedVolatility with a 10x stored close = %v, want 0 (the two odd pairs are rejected, the rest are flat)", got)
	}
	if got := f.realizedVolatility("OK", 9); got != 0 {
		t.Errorf("clean realizedVolatility = %v, want 0 (normal data must not change)", got)
	}
}
