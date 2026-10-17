package shared

import (
	"math"
	"testing"
)

// TestPlausibleReturn pins the gross-error bound of k3 decision 3-1: TWSE/TPEX
// daily price limits are 10% (20% for a few classes), so a stored move beyond
// 50% in one session is corrupted data, while ordinary limit moves, corporate
// action artefacts below the bound, and multi-day compounding all pass.
func TestPlausibleReturn(t *testing.T) {
	cases := []struct {
		name    string
		r       float64
		horizon int
		want    bool
	}{
		{"limit-up 10%", 0.10, 1, true},
		{"limit-down 10%", -0.10, 1, true},
		{"20% class move", 0.20, 1, true},
		{"exactly at the daily bound", 0.50, 1, true},
		{"exactly at the daily bound, down", -0.50, 1, true},
		{"just past the bound", 0.5001, 1, false},
		{"10x corrupted close", 9.0, 1, false},
		{"10x corrupted close, down", -0.9, 1, false},
		{"+61% over 5 days is legal under a 10% limit", 0.61, 5, true},
		{"+900% is not", 9.0, 5, false},
		{"NaN", math.NaN(), 1, false},
		{"+Inf", math.Inf(1), 1, false},
	}
	for _, tc := range cases {
		if got := PlausibleReturn(tc.r, tc.horizon); got != tc.want {
			t.Errorf("PlausibleReturn(%v, %d) = %v, want %v (%s)", tc.r, tc.horizon, got, tc.want, tc.name)
		}
	}
}

// TestInIPOExemptWindow pins the exemption rule: the first IPOPriceLimitExemptDays
// returns of a symbol (and any multi-day window that starts inside them) are
// exempt because no daily price limit applies right after listing.
func TestInIPOExemptWindow(t *testing.T) {
	cases := []struct {
		pos, horizon int
		want         bool
	}{
		{1, 1, true},   // 1st daily return
		{5, 1, true},   // 5th daily return
		{6, 1, false},  // 6th daily return: limit applies again
		{5, 5, true},   // 5-day return that ends on the 5th session
		{10, 5, false}, // window entirely after the IPO days
		{0, 0, true},   // degenerate horizon treated as one day
	}
	for _, tc := range cases {
		if got := InIPOExemptWindow(tc.pos, tc.horizon); got != tc.want {
			t.Errorf("InIPOExemptWindow(%d, %d) = %v, want %v", tc.pos, tc.horizon, got, tc.want)
		}
	}
}

// TestMaxPlausibleReturn pins the horizon scaling: one day is the literal 50%
// bound, longer horizons compound it, and a non-positive horizon degrades to
// the one-day bound instead of returning an unusable value.
func TestMaxPlausibleReturn(t *testing.T) {
	if got := MaxPlausibleReturn(1); got != MaxPlausibleDailyReturn {
		t.Errorf("MaxPlausibleReturn(1) = %v, want %v", got, MaxPlausibleDailyReturn)
	}
	if got, want := MaxPlausibleReturn(5), math.Pow(1.5, 5)-1; math.Abs(got-want) > 1e-12 {
		t.Errorf("MaxPlausibleReturn(5) = %v, want %v", got, want)
	}
	for _, h := range []int{0, -3} {
		if got := MaxPlausibleReturn(h); got != MaxPlausibleDailyReturn {
			t.Errorf("MaxPlausibleReturn(%d) = %v, want the daily bound %v", h, got, MaxPlausibleDailyReturn)
		}
	}
}
