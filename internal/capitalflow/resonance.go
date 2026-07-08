package capitalflow

import (
	"github.com/kaecer68/atlas-go/internal/config"
)

// ---------------------------------------------------------------------------
// Resonance calculator — determines force alignment
// ---------------------------------------------------------------------------

// ComputeResonance calculates resonance strength from a set of force scores.
//
// Rules (bounds from config; default to [0.5, 1.5] per PR #1007 invariant test):
//   - If foreign + institutional + government all share the same direction → coefficient = max bound
//   - If foreign and government are opposing → coefficient = min bound
//   - Otherwise → coefficient = 1.0
func ComputeResonance(forces []ForceScore) ResonanceResult {
	r := ResonanceResult{
		Coefficient: 1.0,
	}

	// Classify forces by direction
	var foreignDir, institutionalDir, governmentDir string
	var foreignScore, institutionalScore, governmentScore ForceScore
	foundForeign, foundInst, foundGovt := false, false, false

	for _, f := range forces {
		switch f.Force {
		case ForceForeign:
			foreignDir = f.Trend
			foreignScore = f
			foundForeign = true
		case ForceInstitutional:
			institutionalDir = f.Trend
			institutionalScore = f
			foundInst = true
		case ForceGovernment:
			governmentDir = f.Trend
			governmentScore = f
			foundGovt = true
		}
		// Collect non-neutral forces
		if f.Trend != "neutral" {
			r.Aligned = append(r.Aligned, f.Force)
		}
	}

	if !foundForeign || !foundInst || !foundGovt {
		r.Direction = "mixed"
		return r
	}

	// Check alignment
	if foreignDir == institutionalDir && institutionalDir == governmentDir && foreignDir != "neutral" {
		// All three major forces aligned → max bound (default 1.5, see config)
		r.Coefficient = config.GetCapitalflowResonanceCoefficientMax()
		r.Direction = foreignDir
	} else if foreignDir == "bullish" && governmentDir == "bearish" ||
		foreignDir == "bearish" && governmentDir == "bullish" {
		// Foreign vs government adversarial → min bound (default 0.5, see config)
		r.Coefficient = config.GetCapitalflowResonanceCoefficientMin()
		r.Opposing = []ForceName{ForceForeign, ForceGovernment}
		r.Direction = "mixed"
	} else {
		r.Direction = foreignDir
		if foreignDir == "neutral" {
			r.Direction = "mixed"
		}
	}

	// Filter aligned to only same-direction forces
	aligned := make([]ForceName, 0)
	for _, f := range forces {
		if f.Trend != "neutral" && f.Trend == foreignDir {
			aligned = append(aligned, f.Force)
		}
	}
	r.Aligned = aligned

	_ = foreignScore
	_ = institutionalScore
	_ = governmentScore
	return r
}
