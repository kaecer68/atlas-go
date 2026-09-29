package icstudy

import (
	"math"
	"testing"
)

func TestRankAveragesTies(t *testing.T) {
	got := Rank([]float64{10, 20, 20, 30})
	want := []float64{1, 2.5, 2.5, 4}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Rank[%d] = %v, want %v (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestSpearmanMonotoneIsOne(t *testing.T) {
	xs := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9}
	ys := []float64{3, 9, 27, 81, 243, 729, 2187, 6561, 19683}
	if got := Spearman(xs, ys); math.Abs(got-1) > 1e-12 {
		t.Fatalf("Spearman = %v, want 1", got)
	}
	if got := Spearman(xs, []float64{9, 8, 7, 6, 5, 4, 3, 2, 1}); math.Abs(got+1) > 1e-12 {
		t.Fatalf("reverse Spearman = %v, want -1", got)
	}
}

func TestSpearmanMatchesClosedFormWithoutTies(t *testing.T) {
	// 1 - 6*sum(d^2) / (n*(n^2-1)) holds exactly for tie-free ranks.
	xs := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	ys := []float64{4, 1, 7, 9, 2, 10, 3, 8, 5, 6}
	var d2 float64
	for i := range xs {
		d := xs[i] - ys[i]
		d2 += d * d
	}
	want := 1 - 6*d2/float64(10*(10*10-1))
	if got := Spearman(xs, ys); math.Abs(got-want) > 1e-12 {
		t.Fatalf("Spearman = %v, want %v", got, want)
	}
}

func TestSpearmanShortCrossSectionIsNaN(t *testing.T) {
	if got := Spearman([]float64{1, 2, 3}, []float64{1, 2, 3}); !math.IsNaN(got) {
		t.Fatalf("Spearman on 3 obs = %v, want NaN", got)
	}
}

func TestPartialSpearmanRemovesCommonFactor(t *testing.T) {
	n := 400
	z := make([]float64, n)
	x := make([]float64, n)
	y := make([]float64, n)
	// Deterministic pseudo-random draws keep the test free of seeding choices.
	state := uint64(12345)
	next := func() float64 {
		state = state*6364136223846793005 + 1442695040888963407
		return float64(state>>11)/float64(1<<53) - 0.5
	}
	for i := range z {
		z[i] = next() * 20
		x[i] = z[i] + next()
		y[i] = z[i] + next()
	}
	raw := Spearman(x, y)
	part := PartialSpearman(x, y, [][]float64{z})
	if raw < 0.9 {
		t.Fatalf("raw Spearman = %v, want near 1 (shared factor dominates)", raw)
	}
	if math.Abs(part) > 0.15 {
		t.Fatalf("partial Spearman on the common factor = %v, want near 0", part)
	}
}

func TestPartialSpearmanKeepsIncrement(t *testing.T) {
	n := 400
	z := make([]float64, n)
	x := make([]float64, n)
	y := make([]float64, n)
	state := uint64(999)
	next := func() float64 {
		state = state*6364136223846793005 + 1442695040888963407
		return float64(state>>11)/float64(1<<53) - 0.5
	}
	for i := range z {
		z[i] = next() * 20
		x[i] = z[i] + next()
		y[i] = x[i] + next() // y depends on x beyond z
	}
	if part := PartialSpearman(x, y, [][]float64{z}); part < 0.5 {
		t.Fatalf("partial Spearman = %v, want a large positive increment", part)
	}
}

func TestPartialSpearmanSingularControlsAreNaN(t *testing.T) {
	n := 20
	a := make([]float64, n)
	b := make([]float64, n)
	for i := range a {
		a[i] = float64(i)
		b[i] = 2 * float64(i)
	}
	if got := PartialSpearman(a, b, [][]float64{a, b}); !math.IsNaN(got) {
		t.Fatalf("collinear controls = %v, want NaN", got)
	}
}

type testRow struct {
	date string
	f    float64
	l    float64
}

func TestComputeICSkipsNaNAndThinDates(t *testing.T) {
	rows := []testRow{}
	for d := 0; d < 3; d++ {
		date := string(rune('a' + d))
		for i := 0; i < 10; i++ {
			rows = append(rows, testRow{date: date, f: float64(i), l: float64(i)})
		}
	}
	// Thin date dropped by minPerDate.
	for i := 0; i < 4; i++ {
		rows = append(rows, testRow{date: "z", f: float64(i), l: float64(i)})
	}
	// NaN feature/label rows dropped before the date count.
	rows = append(rows, testRow{date: "a", f: math.NaN(), l: 10})
	rows = append(rows, testRow{date: "a", f: 11, l: math.NaN()})

	st := ComputeIC(rows, func(r testRow) string { return r.date },
		func(r testRow) float64 { return r.f }, func(r testRow) float64 { return r.l }, 8)
	if st.N != 3 {
		t.Fatalf("N = %d, want 3 (thin date excluded)", st.N)
	}
	if st.SumN != 30 {
		t.Fatalf("SumN = %d, want 30", st.SumN)
	}
	if math.Abs(st.MeanIC-1) > 1e-12 || math.Abs(st.ICIR) > 1e-12 || st.PosPct != 100 {
		t.Fatalf("perfect IC cell = %+v", st)
	}
}

func TestPartialICByDateDropsIncompleteControlRows(t *testing.T) {
	rows := []testRow{}
	for i := 0; i < 12; i++ {
		rows = append(rows, testRow{date: "a", f: float64(i), l: float64(i)})
	}
	rows = append(rows, testRow{date: "a", f: 1, l: math.NaN()})
	ctrl := func(r testRow) float64 { return r.f }
	out, _ := PartialICByDate(rows, func(r testRow) string { return r.date },
		func(r testRow) float64 { return r.f }, func(r testRow) float64 { return r.l }, []func(testRow) float64{ctrl}, 8)
	// A control identical to the feature makes the system singular: NaN, so the
	// date is reported as no-IC rather than silently wrong.
	if _, ok := out["a"]; ok {
		t.Fatalf("singular control should yield no IC, got %v", out)
	}
}
