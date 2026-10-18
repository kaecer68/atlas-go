package stocktools

import (
	"math"
	"testing"
)

// nearly reports whether got is within 1e-12 of want (positive form: a NaN
// result must fail the assertion).
func nearly(got, want float64) bool {
	return math.Abs(got-want) <= 1e-12
}

// TestSMA_SkipsUnusableClose locks k3 decision 3-2 for the technical endpoint:
// sma20 / sma50 are price LEVELS over stored closes, so a close that is not a
// usable price (the stored Close=0 of FU-20260930-03, or a negative value) is a
// missing observation and must not be averaged. Pre-fix [100 101 0 103 104]
// reported 81.6, i.e. the endpoint told the caller the 5-day average was 18%
// below a price that never traded.
func TestSMA_SkipsUnusableClose(t *testing.T) {
	if got := sma([]float64{100, 101, 0, 103, 104}, 5); !nearly(got, 102) {
		t.Errorf("sma over [100 101 0 103 104] = %v, want 102 (mean of the 4 usable closes)", got)
	}
	if got := sma([]float64{100, 101, -5, 103, 104}, 5); !nearly(got, 102) {
		t.Errorf("sma over [100 101 -5 103 104] = %v, want 102", got)
	}
}

// TestSMA_CleanDataUnchanged pins the normal-data value: rounding to 2 decimals
// and the plain mean over the window stay exactly as they were.
func TestSMA_CleanDataUnchanged(t *testing.T) {
	if got := sma([]float64{100, 101, 102, 103, 104}, 5); !nearly(got, 102) {
		t.Errorf("sma on clean data = %v, want 102", got)
	}
	// A short series still reports "no data" rather than a partial average.
	if got := sma([]float64{100, 101}, 5); got != 0 {
		t.Errorf("sma with a series shorter than the window = %v, want 0", got)
	}
}

// TestSMA_NoUsableCloseIsZero covers the degenerate window: dividing by a zero
// count would produce NaN, so the endpoint reports 0 (its short-series value).
func TestSMA_NoUsableCloseIsZero(t *testing.T) {
	if got := sma([]float64{0, 0, 0}, 3); got != 0 {
		t.Errorf("sma over an all-unusable window = %v, want 0", got)
	}
}
