package capitalflow

import (
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// E07 assessment calibration status derivation (spec §8.4 / §9.5,
// CF-INV-13, issue #1941)
//
// Before #1941 the overall CapitalFlowAssessment.CalibrationStatus was a
// hardcoded "calibrating" literal in ComputeCapitalFlowAssessment, so
// EligibleForAutomation() was structurally unable to ever open and
// /api/recommendations emitted capital_flow_assessment_calibrating on every
// response no matter how complete the rolling history was.
//
// This file gives the status a reachable, auditable path:
//
//	override=false (default) → "calibrating"
//	override=true, every data-available dimension has
//	    >= CalibrationEligibleMinSamples samples → "eligible"
//	override=true, no data-available dimension, or an available
//	    dimension sits below the floor → "degraded"
//
// override is capitalflow.calibration_eligible_override, a human-gated config
// parameter (default false, plan v1.1 §3.3 C4). It is NEVER flipped
// automatically: the pre-registered validator only writes
// eligible_recommendation into data/reports/cf-hypotheses-<date>.json, and a
// config PR citing that report performs the flip (CF-INV-13).
//
// The default path is bit-identical to the pre-#1941 behavior: the same
// "calibrating" string, no Reasons entry added.
// ---------------------------------------------------------------------------

// CalibrationEligibleMinSamples is the per-dimension rolling-sample floor a
// dimension must clear before the overall assessment may report "eligible".
// It mirrors the per-dimension rule ForceExtractor.Score applies
// (forces.go: SampleCount >= 30 → CalibrationEligible) so the overall status
// cannot claim more calibration integrity than its dimensions have.
const CalibrationEligibleMinSamples = 30

// DeriveCalibrationStatus returns the overall E07 assessment calibration
// status for the given force scores. See the file comment for the rule table.
//
// Only dimensions with DataAvailable=true participate: a missing source is
// reported by its own layer (Available=false) and must not silently gate the
// assessment forever. An available dimension must itself be calibration-eligible
// — sample count at or above CalibrationEligibleMinSamples AND not flagged
// degraded by ForceExtractor.Score (issue #1940 R3: a degenerate reference
// window cannot standardize the value, so the window is unusable whatever its
// length). The check covers every available dimension because all four layers
// draw on them (official_actor → institutional consensus, behavioral_proxy →
// behavioral confirmation, positioning_indicator → foreign positioning,
// cross_market_signal → cross-market confirmation).
func DeriveCalibrationStatus(forces []ForceScore, eligibleOverride bool) string {
	if !eligibleOverride {
		// Human gate closed (default): un-calibrated by construction.
		return CalibrationCalibrating
	}
	available := 0
	for _, f := range forces {
		if !f.DataAvailable {
			continue
		}
		available++
		if !dimensionCalibrationEligible(f) {
			return CalibrationDegraded
		}
	}
	if available == 0 {
		return CalibrationDegraded
	}
	return CalibrationEligible
}

// dimensionCalibrationEligible reports whether one available dimension carries
// enough usable calibration evidence for the overall assessment to claim
// "eligible": the rolling window has at least CalibrationEligibleMinSamples
// samples after legacy missing values were filtered out, and the dimension is
// not already flagged CalibrationDegraded (degenerate window).
func dimensionCalibrationEligible(f ForceScore) bool {
	return f.CalibrationStatus != CalibrationDegraded &&
		f.SampleCount >= CalibrationEligibleMinSamples
}

// CalibrationStatusReason returns the human-readable reason for a derived
// status, or "" when the status needs no explanation (eligible, and the
// default calibrating state which is already rendered as 「校準中」).
func CalibrationStatusReason(forces []ForceScore, status string) string {
	if status != CalibrationDegraded {
		return ""
	}
	below := make([]string, 0, len(forces))
	available := 0
	for _, f := range forces {
		if !f.DataAvailable {
			continue
		}
		available++
		if !dimensionCalibrationEligible(f) {
			below = append(below, fmt.Sprintf("%s=%d/%s", f.Force, f.SampleCount, dimensionStatusLabel(f)))
		}
	}
	if available == 0 {
		return fmt.Sprintf("無可用維度樣本，無法校準（floor=%d）", CalibrationEligibleMinSamples)
	}
	return fmt.Sprintf("維度樣本未達 %d：%s", CalibrationEligibleMinSamples, strings.Join(below, ", "))
}

// dimensionStatusLabel renders the per-dimension calibration status for the
// degradation reason, defaulting to "calibrating" when the caller left it empty.
func dimensionStatusLabel(f ForceScore) string {
	if f.CalibrationStatus == "" {
		return CalibrationCalibrating
	}
	return f.CalibrationStatus
}
