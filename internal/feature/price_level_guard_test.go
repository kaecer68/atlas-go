package feature

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
)

// withUnusableClose returns a copy of bars whose bar at idx carries v as its
// close, used to stand in for the documented bad stored values of
// FU-20260930-03 (Close=0, negative) plus non-finite ones.
func withUnusableClose(bars []domain.DailyBar, idx int, v float64) []domain.DailyBar {
	out := make([]domain.DailyBar, len(bars))
	copy(out, bars)
	out[idx].Close = v
	return out
}

// usableMean returns the mean of the usable closes in bars[from..to] and the
// number of closes averaged. It is the specification the price LEVEL features
// below must follow (k3 decision 3-2): a close that is not a usable price is a
// missing observation and is skipped, so the mean divides by the count of the
// closes that were actually used.
// nearly reports whether got is within 1e-12 of want. Written in the positive
// form so a NaN result FAILS the assertion instead of slipping through a
// `math.Abs(NaN-want) > tol` comparison.
func nearly(got, want float64) bool {
	return math.Abs(got-want) <= 1e-12
}

func usableMean(bars []domain.DailyBar, from, to int) (float64, int) {
	sum := 0.0
	n := 0
	for j := from; j <= to; j++ {
		if !shared.IsUsablePrice(bars[j].Close) {
			continue
		}
		sum += bars[j].Close
		n++
	}
	if n == 0 {
		return 0, 0
	}
	return sum / float64(n), n
}

// TestFeatureMaRatio_IgnoresUnusableCloseInWindow locks k3 decision 3-2 for
// ma_ratio, the price LEVEL feature named by the decision: the 20-day mean is a
// level, and a stored Close=0 was summed into it. Pre-fix the window
// [100..119] with one zero close produced 119/(2080/20) = 1.1442; the bad close
// is a missing observation, so the ratio must use the mean of the 19 usable
// closes (1.0870).
func TestFeatureMaRatio_IgnoresUnusableCloseInWindow(t *testing.T) {
	bars := withUnusableClose(makeBars(20), 10, 0)
	got := Registry["ma_ratio"](bars[19], 19, bars)

	ma, n := usableMean(bars, 0, 19)
	if n != 19 {
		t.Fatalf("usable closes in the window = %d, want 19", n)
	}
	want := bars[19].Close / ma
	if !nearly(got, want) {
		t.Errorf("ma_ratio with a stored 0 close in the window = %v, want %v (mean of the %d usable closes, not sum/20)", got, want, n)
	}
}

// TestFeatureMaRatio_UnusableCurrentCloseIsNeutral covers the other end: the
// current close itself is missing, so there is no ratio to report. The feature
// keeps its existing neutral value (1.0, what it returns before it has a full
// window) instead of reporting 0 as if the stock traded at zero.
func TestFeatureMaRatio_UnusableCurrentCloseIsNeutral(t *testing.T) {
	for _, bad := range []float64{0, -5, math.NaN()} {
		bars := withUnusableClose(makeBars(20), 19, bad)
		if got := Registry["ma_ratio"](bars[19], 19, bars); got != 1.0 {
			t.Errorf("ma_ratio with current close %v = %v, want 1.0 (neutral)", bad, got)
		}
	}
}

// TestFeatureMaRatio_CleanDataUnchanged pins the normal-data value computed by
// the pre-fix implementation: closes 100..119 give mean 109.5 and ratio
// 119/109.5, so guarding the window must not move a clean observation.
func TestFeatureMaRatio_CleanDataUnchanged(t *testing.T) {
	bars := makeBars(20)
	if got := Registry["ma_ratio"](bars[19], 19, bars); !nearly(got, 119.0/109.5) {
		t.Errorf("ma_ratio on clean data = %v, want %v", got, 119.0/109.5)
	}
}

// TestFeatureBBPercentB_IgnoresUnusableCloseInWindow locks the same rule for
// bb_pct_b, which takes both the mean AND the standard deviation of the level.
func TestFeatureBBPercentB_IgnoresUnusableCloseInWindow(t *testing.T) {
	bars := withUnusableClose(makeBars(20), 5, 0)
	got := Registry["bb_pct_b"](bars[19], 19, bars)

	ma, n := usableMean(bars, 0, 19)
	if n != 19 {
		t.Fatalf("usable closes in the window = %d, want 19", n)
	}
	vr := 0.0
	for j := 0; j <= 19; j++ {
		if !shared.IsUsablePrice(bars[j].Close) {
			continue
		}
		d := bars[j].Close - ma
		vr += d * d
	}
	std := math.Sqrt(vr / float64(n))
	want := (bars[19].Close - ma) / (2.0 * std)
	if !nearly(got, want) {
		t.Errorf("bb_pct_b with a stored 0 close in the window = %v, want %v (%d usable closes)", got, want, n)
	}
}

// TestFeatureBBPercentB_CleanDataUnchanged pins the pre-fix clean value.
func TestFeatureBBPercentB_CleanDataUnchanged(t *testing.T) {
	bars := makeBars(20)
	if got := Registry["bb_pct_b"](bars[19], 19, bars); !nearly(got, 0.823754471047914) {
		t.Errorf("bb_pct_b on clean data = %v, want 0.823754471047914", got)
	}
}

// TestFeatureHLRangePct_IgnoresUnusableCloseInWindow covers hl_range_pct,
// whose denominator is a 20-day mean of the close level.
func TestFeatureHLRangePct_IgnoresUnusableCloseInWindow(t *testing.T) {
	bars := withUnusableClose(makeBars(20), 5, 0)
	got := Registry["hl_range_pct"](bars[19], 19, bars)

	ma, n := usableMean(bars, 0, 19)
	if n != 19 {
		t.Fatalf("usable closes in the window = %d, want 19", n)
	}
	want := (bars[19].High - bars[19].Low) / ma
	if !nearly(got, want) {
		t.Errorf("hl_range_pct with a stored 0 close in the window = %v, want %v", got, want)
	}
}

// TestFeatureHLRangePct_CleanDataUnchanged pins the pre-fix clean value.
func TestFeatureHLRangePct_CleanDataUnchanged(t *testing.T) {
	bars := makeBars(20)
	if got := Registry["hl_range_pct"](bars[19], 19, bars); !nearly(got, 0.0365296803652968) {
		t.Errorf("hl_range_pct on clean data = %v, want 0.0365296803652968", got)
	}
}

// TestFeaturePricePosition_IgnoresUnusableCloseInWindow extends the
// FU-20260930-04 numerator guard to the mean this feature divides by: a bad
// close inside the 20-day window used to shift the mean the price is compared
// against (pre-fix 0.1239 instead of 0.0778).
func TestFeaturePricePosition_IgnoresUnusableCloseInWindow(t *testing.T) {
	bars := withUnusableClose(makeBars(20), 5, 0)
	got := Registry["price_position"](bars[19], 19, bars)

	ma, n := usableMean(bars, 0, 19)
	if n != 19 {
		t.Fatalf("usable closes in the window = %d, want 19", n)
	}
	want := (bars[19].Close - ma) / bars[19].Close
	if !nearly(got, want) {
		t.Errorf("price_position with a stored 0 close in the window = %v, want %v", got, want)
	}
}

// TestFeaturePricePosition_CleanDataUnchanged pins the pre-fix clean value.
func TestFeaturePricePosition_CleanDataUnchanged(t *testing.T) {
	bars := makeBars(20)
	if got := Registry["price_position"](bars[19], 19, bars); !nearly(got, 0.07983193277310924) {
		t.Errorf("price_position on clean data = %v, want 0.07983193277310924", got)
	}
}

// TestFeatureMACD_IgnoresUnusableCloseInEMA covers the EMA the macd family is
// built on: for a series that closes at 100 every day the 12- and 26-period
// EMAs are exactly 100, so macd must be 0. Pre-fix one stored Close=0 inside the
// seed window and the propagation dragged both EMAs below 100 and macd went
// negative — a trend signal manufactured out of a missing observation.
func TestFeatureMACD_IgnoresUnusableCloseInEMA(t *testing.T) {
	bars := makeBars(30)
	for i := range bars {
		bars[i].Close = 100
	}
	bars[3].Close = 0 // inside the 12-period seed window
	if got := Registry["macd"](bars[29], 29, bars); !nearly(got, 0) {
		t.Errorf("macd on a 100-per-day series with one missing close = %v, want 0 (the missing close must be skipped)", got)
	}

	// Control: the same series without the missing close stays at 0.
	clean := makeBars(30)
	for i := range clean {
		clean[i].Close = 100
	}
	if got := Registry["macd"](clean[29], 29, clean); !nearly(got, 0) {
		t.Errorf("macd on the clean 100-per-day series = %v, want 0", got)
	}
}
