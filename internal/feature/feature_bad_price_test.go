package feature

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// withZeroCurrentClose returns a copy of bars whose bar at idx carries the
// documented bad stored value Close=0 (FU-20260930-03).
func withZeroCurrentClose(bars []domain.DailyBar, idx int) []domain.DailyBar {
	out := make([]domain.DailyBar, len(bars))
	copy(out, bars)
	out[idx].Close = 0
	return out
}

// TestFeatureReturn1d_CurrentCloseZero locks FU-20260930-04: the pre-fix guard
// checked only the PREVIOUS close, so a stored Close=0 on the current bar was
// returned as (0-100)/100 = -1 — a fabricated -100% feature for the ML model.
func TestFeatureReturn1d_CurrentCloseZero(t *testing.T) {
	bars := withZeroCurrentClose(makeBars(3), 1)
	if got := Registry["return_1d"](bars[1], 1, bars); got != 0 {
		t.Errorf("return_1d with current close 0 = %v, want 0 (missing observation, not -100%%)", got)
	}
}

// TestFeatureReturn5d_CurrentCloseZero is the 5-day sibling of the above.
func TestFeatureReturn5d_CurrentCloseZero(t *testing.T) {
	bars := withZeroCurrentClose(makeBars(10), 5)
	if got := Registry["return_5d"](bars[5], 5, bars); got != 0 {
		t.Errorf("return_5d with current close 0 = %v, want 0", got)
	}
}

// TestFeaturePricePosition_NegativeClose locks the `== 0` -> usable-price
// change: a negative close passed the old test and flipped the sign of the
// deviation from the mean.
func TestFeaturePricePosition_NegativeClose(t *testing.T) {
	bars := makeBars(20)
	bars[19].Close = -5
	if got := Registry["price_position"](bars[19], 19, bars); got != 0 {
		t.Errorf("price_position with close -5 = %v, want 0", got)
	}
}

// TestFeatureQualityIntra_NegativeClose locks the same `== 0` gap for
// quality_intra.
func TestFeatureQualityIntra_NegativeClose(t *testing.T) {
	b := makeBars(1)[0]
	b.Close = -5
	if got := Registry["quality_intra"](b, 0, nil); got != 0 {
		t.Errorf("quality_intra with close -5 = %v, want 0", got)
	}
}

// TestFeatureReturnAutocorr_CurrentCloseZero locks FU-20260930-04 in the
// autocorrelation feature: the pre-fix guard checked only bars[pos-1], so a
// stored Close=0 on bars[pos] contributed -1 to the return series. The fixed
// result must equal the one obtained when that bar legitimately has a zero
// return (its close equals the previous close) — i.e. the missing pair
// contributes nothing.
func TestFeatureReturnAutocorr_CurrentCloseZero(t *testing.T) {
	idx := 24
	broken := withZeroCurrentClose(makeBars(30), idx)

	reference := makeBars(30)
	reference[idx].Close = reference[idx-1].Close // genuine 0% return

	got := Registry["return_autocorr"](broken[idx], idx, broken)
	want := Registry["return_autocorr"](reference[idx], idx, reference)
	if math.Abs(got-want) > 1e-12 {
		t.Errorf("return_autocorr with current close 0 = %v, want %v (equal to a genuine 0%% return)", got, want)
	}
}

// TestForwardReturnLabel_NextCloseZero locks FU-20260930-04 in the label
// extractor: the pre-fix guard checked only bars[i], so a missing FORWARD close
// produced the label -1 for a training sample.
func TestForwardReturnLabel_NextCloseZero(t *testing.T) {
	bars := withZeroCurrentClose(makeBars(5), 2)
	labels := ForwardReturnLabel()(bars)
	if len(labels) != 5 {
		t.Fatalf("got %d labels, want 5", len(labels))
	}
	if labels[1] != 0 {
		t.Errorf("label with a missing forward close = %v, want 0 (missing observation, not -100%%)", labels[1])
	}
	// The sample whose own close is missing is also skipped.
	if labels[2] != 0 {
		t.Errorf("label whose own close is missing = %v, want 0", labels[2])
	}
	// Control: the untouched pair keeps its exact value.
	if want := (104.0 - 103.0) / 103.0; math.Abs(labels[3]-want) > 1e-12 {
		t.Errorf("clean label = %v, want %v (normal data must not change)", labels[3], want)
	}
}
