// Package risk: dynamic_hedge.go
//
// ComputeHedgeRatio derives a beta-style hedge ratio that adapts in real time
// to two market-state signals:
//
//  1. The prevailing VIX regime ("calm" / "stress" / "crisis").
//  2. The sign of foreign-investor net flow (positive = net buy / inflow,
//     negative = net sell / outflow).
//
// The result is the product of the base ratio, a regime multiplier, and a
// flow-direction adjustment, clamped to [0.0, 2.0] so the figure can be
// interpreted as a fraction-of-portfolio hedge that never goes naked-short
// (negative) and never over-hedges to a level that would imply leverage
// beyond a 2x notional stance.
//
// All parameters travel by value through the HedgeRatio struct so callers can
// override defaults (e.g. from a calibrated ParametersConfig) without
// mutating shared state. Zero-valued fields fall back to the documented
// defaults below.
package risk

import "math"

// Default values applied when the corresponding HedgeRatio field is non-positive.
// These are the values documented in the spec for a baseline Taiwan-equity
// portfolio: a β of 0.95 against the TAIEX, with stress/crisis scaling of 1.3
// and 1.5 respectively, and a 10% flow-direction sensitivity.
const (
	defaultHedgeBase       = 0.95
	defaultHedgeStressMult = 1.3
	defaultHedgeCrisisMult = 1.5
	defaultHedgeFlowFactor = 0.1
)

// HedgeRatioInputs captures the real-time market signals used to derive a
// dynamic hedge ratio. All numeric fields are dimensionless or in the units
// noted below; the function performs no further normalization.
type HedgeRatioInputs struct {
	// VIX is the current VIX level. Used for context only — the regime
	// label is the authoritative input for scaling.
	VIX float64
	// VIXBaseline is the 20-day moving average of VIX. Default 20.0.
	// Reserved for future use (e.g. regime re-classification); the current
	// implementation relies on the explicit VIXRegime string.
	VIXBaseline float64
	// ForeignFlow is the foreign-investor daily net buy/sell in NT$ bn.
	// Positive = net buy (inflow); negative = net sell (outflow).
	ForeignFlow float64
	// VIXRegime classifies the current volatility regime. One of
	// "calm", "stress", "crisis". Any other value (including "") falls
	// back to the calm regime to avoid silent under-hedging.
	VIXRegime string
}

// HedgeRatio captures the tunable multipliers that govern the dynamic hedge
// ratio formula. Callers may pass a zero-valued HedgeRatio to use the
// defaults; otherwise any non-positive field is treated as "unset" and
// replaced with its default.
type HedgeRatio struct {
	// Base is the foundation hedge ratio (β), default 0.95.
	Base float64
	// StressMult is the regime multiplier when VIXRegime == "stress", default 1.3.
	StressMult float64
	// CrisisMult is the regime multiplier when VIXRegime == "crisis" (VIX > 35),
	// default 1.5.
	CrisisMult float64
	// FlowFactor is the sensitivity coefficient to the foreign-flow sign,
	// default 0.1. Increasing this amplifies the inflow/outflow tilt.
	FlowFactor float64
}

// resolveHedgeParams fills in default values for any HedgeRatio field that is
// non-positive. It is called once at the top of ComputeHedgeRatio so callers
// can safely pass either a fully-populated or a zero-valued struct.
func resolveHedgeParams(h HedgeRatio) HedgeRatio {
	if h.Base <= 0 {
		h.Base = defaultHedgeBase
	}
	if h.StressMult <= 0 {
		h.StressMult = defaultHedgeStressMult
	}
	if h.CrisisMult <= 0 {
		h.CrisisMult = defaultHedgeCrisisMult
	}
	if h.FlowFactor <= 0 {
		h.FlowFactor = defaultHedgeFlowFactor
	}
	return h
}

// regimeMultiplier returns the regime multiplier for the given VIX regime
// label. Unknown values (including the empty string) fall back to 1.0 so
// that unrecognized regimes do not silently reduce the hedge.
func regimeMultiplier(regime string, h HedgeRatio) float64 {
	switch regime {
	case "stress":
		return h.StressMult
	case "crisis":
		return h.CrisisMult
	default:
		// "calm" and any unknown / empty label.
		return 1.0
	}
}

// flowSign returns the sign of x as -1.0, 0.0, or +1.0. Zero input
// (interpreted as "neutral flow") contributes nothing to the hedge
// multiplier.
func flowSign(x float64) float64 {
	switch {
	case x > 0:
		return 1.0
	case x < 0:
		return -1.0
	default:
		return 0.0
	}
}

// ComputeHedgeRatio returns the dynamic hedge ratio for the given inputs.
//
//	hedge = Base × RegimeMult(VIXRegime) × (1 - FlowFactor × sign(ForeignFlow))
//
// The result is clamped to [0.0, 2.0]:
//   - The lower bound 0.0 prevents a negative hedge (no naked shorting).
//   - The upper bound 2.0 caps gross notional at 2x exposure, matching the
//     Darwinian weight ceiling used elsewhere in the risk module.
//
// Rationale:
//   - Calm regime + neutral flow → Base (the documented β).
//   - Stress/crisis regimes scale up the hedge to compensate for elevated
//     volatility and tail risk.
//   - Persistent inflows (ForeignFlow > 0) reduce the required hedge
//     because strong foreign demand lowers the local-market beta exposure.
//   - Persistent outflows (ForeignFlow < 0) increase the required hedge
//     because capitulation flows amplify drawdown risk.
func ComputeHedgeRatio(h HedgeRatio, in HedgeRatioInputs) float64 {
	h = resolveHedgeParams(h)

	regimeM := regimeMultiplier(in.VIXRegime, h)
	flowM := 1.0 - h.FlowFactor*flowSign(in.ForeignFlow)
	hedge := h.Base * regimeM * flowM

	// Clamp to [0.0, 2.0] via math.Max/Min to keep the body branch-free.
	return math.Max(0.0, math.Min(2.0, hedge))
}
