package scheduler

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
)

// usableCloseStats returns the mean and the population standard deviation of
// the usable closes in bars, plus the number of closes used. It is the
// specification the price LEVEL statistics in extractFeatures must follow (k3
// decision 3-2): a close that is not a usable price is a missing observation,
// so it may not enter the mean / std that the close z-score is measured
// against.
func usableCloseStats(bars []domain.DailyBar) (mean, std float64, n int) {
	sum := 0.0
	for _, b := range bars {
		if !shared.IsUsablePrice(b.Close) {
			continue
		}
		sum += b.Close
		n++
	}
	if n == 0 {
		return 0, 0, 0
	}
	mean = sum / float64(n)
	sq := 0.0
	for _, b := range bars {
		if !shared.IsUsablePrice(b.Close) {
			continue
		}
		d := b.Close - mean
		sq += d * d
	}
	return mean, math.Sqrt(sq / float64(n)), n
}

// nearly reports whether got is within 1e-12 of want (positive form: a NaN
// result must fail the assertion).
func nearly(got, want float64) bool {
	return math.Abs(got-want) <= 1e-12
}

// TestExtractFeatures_SkipsUnusableCloseInLevelStats locks k3 decision 3-2 for
// the ML retrain feature row: row[3] is the z-score of the close LEVEL and
// row[6] is the close against its own 5-day mean level. Pre-fix both used the
// raw closes, so one stored Close=0 (a missing observation, FU-20260930-03)
// pulled the reference mean down and manufactured a large positive row[6]
// (0.2735 instead of 0.0189) for a sample the label filter had accepted.
func TestExtractFeatures_SkipsUnusableCloseInLevelStats(t *testing.T) {
	bars := badCloseBars(12, 6) // bars[6].Close = 0
	X := extractFeatures(bars)
	y := extractLabels(bars)
	if len(X) != len(y) || len(X) != 4 {
		t.Fatalf("got %d feature rows and %d labels, want 4 of each", len(X), len(y))
	}

	// sampleIndexes starts at 5 and drops the two pairs that touch bar 6, so the
	// rows are bars 5, 8, 9 and 10.
	row := X[1] // bars[8]
	mean, std, n := usableCloseStats(bars)
	if n != 11 {
		t.Fatalf("usable closes = %d, want 11", n)
	}

	wantZ := (bars[8].Close - mean) / std
	if !nearly(row[3], wantZ) {
		t.Errorf("row[3] (close z-score) with a stored 0 close = %v, want %v (mean/std over the %d usable closes)", row[3], wantZ, n)
	}

	// bars[4..8] closes are 104, 105, 0, 107, 108 -> the usable mean is
	// 424/4 = 106, and the same value the clean series produces.
	wantMa5 := bars[8].Close/106.0 - 1
	if !nearly(row[6], wantMa5) {
		t.Errorf("row[6] (close / 5-day mean level - 1) = %v, want %v (missing close skipped)", row[6], wantMa5)
	}
}

// TestExtractFeatures_CleanDataUnchanged pins the normal-data side: with no
// unusable close the row values are the plain population z-score and the plain
// 5-day mean, i.e. identical to the pre-fix implementation.
func TestExtractFeatures_CleanDataUnchanged(t *testing.T) {
	bars := badCloseBars(12, -1) // closes 100..111
	X := extractFeatures(bars)
	if len(X) != 6 {
		t.Fatalf("got %d feature rows, want 6", len(X))
	}
	row := X[3] // bars[8]
	mean, std, n := usableCloseStats(bars)
	if n != 12 {
		t.Fatalf("usable closes = %d, want 12", n)
	}
	if wantZ := (bars[8].Close - mean) / std; !nearly(row[3], wantZ) {
		t.Errorf("clean row[3] = %v, want %v", row[3], wantZ)
	}
	if want := bars[8].Close/106.0 - 1; !nearly(row[6], want) {
		t.Errorf("clean row[6] = %v, want %v", row[6], want)
	}
}
