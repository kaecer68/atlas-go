package capitalflow

// ---------------------------------------------------------------------------
// Resonance calculator — determines force alignment
// ---------------------------------------------------------------------------

// ComputeResonance calculates resonance strength from a set of force scores.
//
// Rules:
//   - If foreign + institutional + government all share the same direction → coefficient = 1.5
//   - If foreign and government are opposing → coefficient = 0.5
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
		// All three major forces aligned
		r.Coefficient = 1.5
		r.Direction = foreignDir
	} else if foreignDir == "bullish" && governmentDir == "bearish" ||
		foreignDir == "bearish" && governmentDir == "bullish" {
		// Foreign vs government adversarial
		r.Coefficient = 0.5
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
