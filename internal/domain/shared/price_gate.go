package shared

import "math"

// MaxPlausibleDailyReturn is the gross-error bound for a SINGLE-DAY return:
// a stored daily move beyond +-50% is treated as a corrupted price, not as a
// session outcome.
//
// Rationale (FU-20260930-03 / k3 decision 3-1): TWSE/TPEX apply a daily price
// limit of 10% (20% for a few instrument classes), so normal data can never
// produce a 50% one-day move. Stored data can: FU-20260930-03 documents
// 5904.TW with an ~10x close (720 against an official 72), which every return
// consumer read as +900%. This bound is deliberately loose — it is a
// gross-error gate, not a volatility filter — so a legitimate limit-up day or a
// corporate-action artefact below 50% still passes.
const MaxPlausibleDailyReturn = 0.5

// IPOPriceLimitExemptDays is the number of trading days after listing during
// which TWSE/TPEX do NOT apply a daily price limit. A newly listed symbol can
// therefore move far more than MaxPlausibleDailyReturn and still be real data,
// so observations inside this window are exempt from the gross-error gate.
const IPOPriceLimitExemptDays = 5

// MaxPlausibleReturn returns the gross-error bound for a return measured over
// horizonDays trading days. A one-day horizon uses MaxPlausibleDailyReturn
// verbatim; longer horizons compound it (a 5-day return may legitimately reach
// 1.5^5-1 = 659%), so the bound never rejects a move that is merely large.
// A non-positive horizon is treated as one day.
func MaxPlausibleReturn(horizonDays int) float64 {
	if horizonDays <= 1 {
		return MaxPlausibleDailyReturn
	}
	return math.Pow(1+MaxPlausibleDailyReturn, float64(horizonDays)) - 1
}

// PlausibleReturn reports whether r is inside the gross-error bound for the
// given horizon. Non-finite returns are never plausible.
func PlausibleReturn(r float64, horizonDays int) bool {
	if math.IsNaN(r) || math.IsInf(r, 0) {
		return false
	}
	return math.Abs(r) <= MaxPlausibleReturn(horizonDays)
}

// InIPOExemptWindow reports whether a return measured over horizonDays and
// ending at position pos of a symbol's stored series overlaps the IPO
// no-price-limit window. pos is the 0-based index of the LAST observation used.
//
// A truncated series is safe in the fail-open direction: a window that starts
// inside its first IPOPriceLimitExemptDays observations is exempt, so an old
// symbol simply keeps its earliest few values unchecked instead of having valid
// values rejected.
func InIPOExemptWindow(pos, horizonDays int) bool {
	if horizonDays < 1 {
		horizonDays = 1
	}
	return pos-horizonDays < IPOPriceLimitExemptDays
}
