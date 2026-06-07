package risk

import (
	"math"
	"testing"
)

// defaultHedge returns a HedgeRatio populated with the documented defaults.
// Most tests start from this baseline and only override the inputs they care
// about, mirroring how a production caller would source the struct from
// config.GetParametersConfig() (or a stub during unit tests).
func defaultHedge() HedgeRatio {
	return HedgeRatio{
		Base:       0.95,
		StressMult: 1.3,
		CrisisMult: 1.5,
		FlowFactor: 0.1,
	}
}

// floatEq compares two float64 values with a small absolute tolerance. The
// tolerance is generous enough to absorb the rounding error of the documented
// calculation chains (0.95 × 1.0 × 0.9 etc.) while still catching genuine
// regressions like a sign-flip or a missed clamp.
func floatEq(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

// TestHedgeRatio_CalmRegime verifies the baseline case: VIX calm, foreign
// investors are net buyers. The inflow should *reduce* the hedge (strong
// foreign demand lowers the local-market beta exposure), so the result
// should sit just below Base.
//
// Calculation:
//
//	0.95 × 1.0 × (1 - 0.1 × sign(+100))
//
// = 0.95 × 1.0 × (1 - 0.1)
// = 0.95 × 0.9
// = 0.855
func TestHedgeRatio_CalmRegime(t *testing.T) {
	got := ComputeHedgeRatio(defaultHedge(), HedgeRatioInputs{
		VIX:         15,
		VIXBaseline: 20,
		ForeignFlow: 100, // inflow (buy)
		VIXRegime:   "calm",
	})
	const want = 0.855
	if !floatEq(got, want, 1e-9) {
		t.Fatalf("calm regime with inflow: got %v, want %v", got, want)
	}
}

// TestHedgeRatio_StressRegime verifies that a stress regime with neutral
// flow produces the Base × StressMult product exactly. No sign contribution
// from ForeignFlow = 0.
//
// Calculation:
//
//	0.95 × 1.3 × (1 - 0.1 × sign(0))
//
// = 0.95 × 1.3 × 1.0
// = 1.235
func TestHedgeRatio_StressRegime(t *testing.T) {
	got := ComputeHedgeRatio(defaultHedge(), HedgeRatioInputs{
		VIX:         28,
		VIXBaseline: 20,
		ForeignFlow: 0, // neutral
		VIXRegime:   "stress",
	})
	const want = 1.235
	if !floatEq(got, want, 1e-9) {
		t.Fatalf("stress regime with neutral flow: got %v, want %v", got, want)
	}
}

// TestHedgeRatio_CrisisRegime verifies the crisis-tier scaling combined with
// a foreign outflow. The outflow should *increase* the hedge because
// capitulation flows amplify drawdown risk.
//
// Calculation:
//
//	0.95 × 1.5 × (1 - 0.1 × sign(-100))
//
// = 0.95 × 1.5 × (1 + 0.1)
// = 0.95 × 1.5 × 1.1
// = 1.5675
func TestHedgeRatio_CrisisRegime(t *testing.T) {
	got := ComputeHedgeRatio(defaultHedge(), HedgeRatioInputs{
		VIX:         40,
		VIXBaseline: 20,
		ForeignFlow: -100, // outflow (sell)
		VIXRegime:   "crisis",
	})
	const want = 1.5675
	if !floatEq(got, want, 1e-9) {
		t.Fatalf("crisis regime with outflow: got %v, want %v", got, want)
	}
}

// TestHedgeRatio_Clamping verifies the [0.0, 2.0] safety clamp on both ends.
// The defaults alone cannot escape the clamp, so we feed extreme HedgeRatio
// values to push the unclamped math outside the bounds.
//
// Lower bound: huge FlowFactor + inflow → flowMult deeply negative →
//
//	Base × RegimeMult × flowMult < 0 → clamp to 0.0
//
// Upper bound: Base = 1.5, crisis regime, outflow →
//
//	1.5 × 1.5 × 1.1 = 2.475 > 2.0 → clamp to 2.0
func TestHedgeRatio_Clamping(t *testing.T) {
	t.Run("lower bound", func(t *testing.T) {
		extreme := HedgeRatio{
			Base:       0.95,
			StressMult: 1.3,
			CrisisMult: 1.5,
			FlowFactor: 5.0, // exaggerate flow direction
		}
		got := ComputeHedgeRatio(extreme, HedgeRatioInputs{
			VIX:         12,
			VIXBaseline: 20,
			ForeignFlow: 1, // tiny inflow
			VIXRegime:   "calm",
		})
		// flowMult = 1 - 5*1 = -4 → raw hedge = 0.95 * 1.0 * -4 = -3.8 → 0.0
		if got != 0.0 {
			t.Fatalf("expected clamp to 0.0, got %v", got)
		}
	})

	t.Run("upper bound", func(t *testing.T) {
		extreme := HedgeRatio{
			Base:       1.5,
			StressMult: 1.3,
			CrisisMult: 1.5,
			FlowFactor: 0.1,
		}
		got := ComputeHedgeRatio(extreme, HedgeRatioInputs{
			VIX:         45,
			VIXBaseline: 20,
			ForeignFlow: -100, // outflow pushes flowMult up
			VIXRegime:   "crisis",
		})
		// 1.5 × 1.5 × 1.1 = 2.475 → clamp to 2.0
		if got != 2.0 {
			t.Fatalf("expected clamp to 2.0, got %v", got)
		}
	})
}

// TestHedgeRatio_UnknownRegime verifies that any regime label outside the
// known set ("calm" / "stress" / "crisis") falls back to the calm regime
// (multiplier 1.0). This is the documented safety behaviour: an unrecognised
// label must not silently *reduce* the hedge to less than Base.
func TestHedgeRatio_UnknownRegime(t *testing.T) {
	cases := []struct {
		name   string
		regime string
	}{
		{"empty string", ""},
		{"unknown label", "unknown"},
		{"typo label", "strees"}, // intentionally misspelled
		{"legacy label", "normal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeHedgeRatio(defaultHedge(), HedgeRatioInputs{
				VIX:         18,
				VIXBaseline: 20,
				ForeignFlow: 0, // neutral so only the regime multiplier matters
				VIXRegime:   tc.regime,
			})
			const want = 0.95 // same as calm regime with neutral flow
			if !floatEq(got, want, 1e-9) {
				t.Fatalf("regime=%q: got %v, want %v (calm fallback)", tc.regime, got, want)
			}
		})
	}
}

// TestHedgeRatio_ZeroStructUsesDefaults verifies that a zero-valued
// HedgeRatio is filled with the documented defaults. Without this
// safety net, the function would multiply by 0 and return 0 for every
// call — a silent under-hedge that is dangerous in production.
func TestHedgeRatio_ZeroStructUsesDefaults(t *testing.T) {
	got := ComputeHedgeRatio(HedgeRatio{}, HedgeRatioInputs{
		VIX:         18,
		VIXBaseline: 20,
		ForeignFlow: 0,
		VIXRegime:   "calm",
	})
	// Zero struct + calm + neutral flow should match the defaultHedge case.
	const want = 0.95
	if !floatEq(got, want, 1e-9) {
		t.Fatalf("zero-valued HedgeRatio: got %v, want %v", got, want)
	}
}
