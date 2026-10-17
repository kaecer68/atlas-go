package shared

import (
	"math"
	"testing"
)

// TestIsUsablePrice pins the guard predicate used by every price->return
// consumer (FU-20260930-04). A close of 0 is the documented bad value written
// by the quote store (FU-20260930-03); it must be rejected so that a return
// computed from it is never -100%.
func TestIsUsablePrice(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want bool
	}{
		{"normal price", 123.5, true},
		{"fractional price", 0.01, true},
		{"zero close (bad stored value)", 0, false},
		{"negative close (bad stored value)", -5, false},
		{"NaN", math.NaN(), false},
		{"+Inf", math.Inf(1), false},
		{"-Inf", math.Inf(-1), false},
	}
	for _, tc := range cases {
		if got := IsUsablePrice(tc.in); got != tc.want {
			t.Errorf("IsUsablePrice(%v) = %v, want %v (%s)", tc.in, got, tc.want, tc.name)
		}
	}
}
