package capitalflow

// CF-INV-06 input hygiene: missing data must never be stored as 0, and a
// stored 0 must never be read back as an observation (spec §8.3
// "參考窗不得混入缺失值" + §8.3 "缺資料不得解讀為 neutral").
//
// Issue #1940 found the write-side shape of the violation (government:
// 18 zero-valued samples 2026-07-22..08-14 → the next real reading scored
// z = -7028.5). The 2026-09-24 production audit found a second, LIVE
// instance of the same class in a dimension the issue did not name:
//
//	institutional, 2025-09-17..2026-05-13: 148 consecutive samples with
//	raw_value = 0. On 2026-05-15 a real reading (-0.3867209) scored
//	z = -38.67 against that window — and it did so *because* the old
//	stddev clamp (max(0.01, …)) replaced the window's true dispersion
//	(which is ~0) with a fake 0.01.
//
// So there are two independent jobs, and this file owns both:
//
//  1. usableReading — the write gate. A zero from a net-flow / level
//     channel is not an observation, so no sample is written and the
//     ForceScore reports data_available=false.
//  2. referenceWindowFor — the read filter. Legacy zeros already sit in
//     the persisted 252-day window; spec §8.3 forbids them from entering
//     the reference window, so they are dropped before the Z-score is
//     computed. Without this, removing the stddev clamp would turn
//     institutional's legacy window into z ≈ -4739 instead of -38.67.
//
// Both are scoped by zeroIsMissingDimension, which lists the dimensions
// whose raw value is an aggregate level (net shares / net TWD / open
// interest) rather than a ratio. For those, exactly 0 is the sentinel a
// failed or unpublished fetch leaves behind; the platform already treats
// it that way for the government_broker channel
// (SuccessCriteriaValueNonzero, channel_contract.go).

// zeroIsMissingDimension reports whether a raw value of exactly 0 means
// "no reading" for the dimension.
//
// Excluded on purpose:
//   - ForceRetail: a composite of margin/short change percentages; a flat
//     day is a real 0.
//   - ForceTSMADR: a daily percentage change; a flat day is a real 0.
//
// Included: the four net-flow subjects plus the futures open-interest
// level. Note this is also why ForceForeign is included even though its
// score carries a futures-derived LeadingZ: the spot leg is an aggregate
// net flow with the same sentinel semantics.
func zeroIsMissingDimension(dim ForceName) bool {
	switch dim {
	case ForceForeign, ForceInstitutional, ForceDealer, ForceGovernment, ForceFutures:
		return true
	default:
		return false
	}
}

// usableReading reports whether raw is an observation for dim: true for
// any value of a ratio dimension, and for any non-zero value of a
// net-flow / level dimension.
func usableReading(dim ForceName, raw float64) bool {
	return !(zeroIsMissingDimension(dim) && raw == 0)
}

// referenceWindowFor returns the samples that may enter the Z-score
// reference window for dim.
//
// Spec §8.3 forbids missing values in the reference window. Persisted
// windows written before this fix contain zero-valued samples for
// net-flow dimensions (production: 148 consecutive institutional zeros),
// so the filter runs at read time too, not only at write time. A
// dimension outside zeroIsMissingDimension keeps every sample — a flat
// percentage is data, not a gap.
func referenceWindowFor(samples []RollingSample, dim ForceName) []RollingSample {
	if !zeroIsMissingDimension(dim) {
		return samples
	}
	dropped := 0
	for _, s := range samples {
		if s.RawValue == 0 {
			dropped++
		}
	}
	if dropped == 0 {
		return samples
	}
	out := make([]RollingSample, 0, len(samples)-dropped)
	for _, s := range samples {
		if s.RawValue != 0 {
			out = append(out, s)
		}
	}
	return out
}
