package capitalflow

import (
	"github.com/kaecer68/atlas-go/internal/config"
)

// ---------------------------------------------------------------------------
// Resonance calculator — determines force alignment
// ---------------------------------------------------------------------------

// ComputeResonance calculates resonance strength from a set of force scores.
// Operates ONLY on the 5 subject forces per §7 taxonomy (manifest #E05);
// leading_indicator and sentiment entries are present in the forces list
// for API back-compat but ignored by this function.
//
// Rules (bounds from config; default to [0.5, 1.5] per PR #1007 invariant test):
//   - If foreign + institutional + government all share the same direction → coefficient = max bound
//   - If foreign and government are opposing → coefficient = min bound
//   - Otherwise → coefficient = 1.0
//
// Aligned / Opposing (CF-INV-09): the rebuild at the bottom of this
// function filters forces by both the legacy Role=="subject" and
// the new E07 DimensionRole=="official_actor" classifications. The
// OR keeps government/retail (legacy subjects, but behavioral
// proxies) in the Aligned/Opposing list when their Trend matches
// the foreign direction; the AND-NOT pattern excludes the two
// non-subject signals (futures + tsm_adr) so a strongly bullish
// TSM ADR cannot artificially raise the actor-consensus Aligned
// count.
func ComputeResonance(forces []ForceScore) ResonanceResult {
	r := ResonanceResult{
		Coefficient: 1.0,
	}

	// Classify forces by direction — only subjects count.
	var foreignDir, institutionalDir, governmentDir string
	var foreignScore, institutionalScore, governmentScore ForceScore
	foundForeign, foundInst, foundGovt := false, false, false

	for _, f := range forces {
		if f.Role != ForceRoleSubject {
			continue
		}
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

	// Filter aligned to only same-direction forces that are
	// either a legacy subject OR an E07 official_actor. This
	// (CF-INV-09) keeps government/retail in the list (they
	// remain legacy subjects) while excluding positioning_indicator
	// and cross_market_signal dimensions that share a Trend with
	// foreign.
	aligned := make([]ForceName, 0)
	for _, f := range forces {
		if f.Trend == "neutral" {
			continue
		}
		if f.Trend != foreignDir {
			continue
		}
		if f.Role != ForceRoleSubject && f.DimensionRole != DimensionRoleOfficialActor {
			continue
		}
		aligned = append(aligned, f.Force)
	}
	r.Aligned = aligned

	_ = foreignScore
	_ = institutionalScore
	_ = governmentScore
	return r
}

// ---------------------------------------------------------------------------
// E07 layered assessment (spec §9.5 / CF-INV-08 / CF-INV-13)
//
// The 4 sub-assessments replace the single ResonanceResult for new
// consumers. Each sub-assessment reports whether it has enough data
// to speak (Available), the direction it sees, the dimensions that
// voted with / against that direction, and a free-form reasons
// slice. The overall CapitalFlowAssessment is the assembly
// ComputeCapitalFlowAssessment(forces) returns; it stays
// CalibrationStatus="calibrating" until H-CF-02 is validated, and
// the EligibleForAutomation() method is the canonical CF-INV-13
// gate.
// ---------------------------------------------------------------------------

// computeInstitutionalConsensus reads the three official_actor
// dimensions (foreign / institutional / dealer) and reports their
// joint direction. The sub-assessment is only Available when all
// three dimensions have data; when one is missing, the layer
// declines to speak. Aligned/Opposing list only the three
// official_actor dimensions, so a foreign-only alignment is
// expressed as Aligned=[foreign], not Aligned=[foreign, retail].
func computeInstitutionalConsensus(forces []ForceScore) DirectionalAssessment {
	byDim := map[ForceName]ForceScore{}
	for _, f := range forces {
		byDim[f.Force] = f
	}
	foreign, okF := byDim[ForceForeign]
	inst, okI := byDim[ForceInstitutional]
	dealer, okD := byDim[ForceDealer]
	if !okF || !okI || !okD || !foreign.DataAvailable || !inst.DataAvailable || !dealer.DataAvailable {
		return DirectionalAssessment{
			Available: false,
			Reasons:   []string{"缺任一 official_actor 維度資料"},
		}
	}
	trends := []string{foreign.Trend, inst.Trend, dealer.Trend}
	allSame := trends[0] != "neutral" && trends[0] == trends[1] && trends[1] == trends[2]
	out := DirectionalAssessment{Available: true}
	if allSame {
		out.Direction = trends[0]
		out.Aligned = []ForceName{ForceForeign, ForceInstitutional, ForceDealer}
		return out
	}
	out.Direction = "mixed"
	// Mixed: separate the 3 into aligned-with-foreign vs opposing.
	for _, f := range []ForceName{ForceForeign, ForceInstitutional, ForceDealer} {
		if byDim[f].Trend == foreign.Trend && byDim[f].Trend != "neutral" {
			out.Aligned = append(out.Aligned, f)
		} else if byDim[f].Trend != "neutral" {
			out.Opposing = append(out.Opposing, f)
		}
	}
	return out
}

// computeBehavioralConfirmation reads the two behavioral_proxy
// dimensions (government + retail). It is Available only when
// BOTH dimensions have data; either missing means the layer
// declines to speak. It does NOT participate in actor consensus
// overall — it is a separate signal that automation consumers
// must opt into via the Assessment.Behavioral sub-field.
func computeBehavioralConfirmation(forces []ForceScore) DirectionalAssessment {
	byDim := map[ForceName]ForceScore{}
	for _, f := range forces {
		byDim[f.Force] = f
	}
	gov, okG := byDim[ForceGovernment]
	ret, okR := byDim[ForceRetail]
	if !okG || !okR || !gov.DataAvailable || !ret.DataAvailable {
		return DirectionalAssessment{
			Available: false,
			Reasons:   []string{"缺公股或散戶資料"},
		}
	}
	out := DirectionalAssessment{Available: true}
	switch {
	case gov.ZScore >= 0 && ret.ZScore >= 0:
		out.Direction = "bullish"
		out.Aligned = []ForceName{ForceGovernment, ForceRetail}
	case gov.ZScore < 0 && ret.ZScore < 0:
		out.Direction = "bearish"
		out.Aligned = []ForceName{ForceGovernment, ForceRetail}
	default:
		out.Direction = "mixed"
		if gov.ZScore >= 0 {
			out.Aligned = []ForceName{ForceGovernment}
			out.Opposing = []ForceName{ForceRetail}
		} else {
			out.Aligned = []ForceName{ForceRetail}
			out.Opposing = []ForceName{ForceGovernment}
		}
	}
	return out
}

// computeForeignPositioningConfirmation compares the foreign
// spot trend with the foreign LeadingTrend (the futures-OI
// leading signal from spec §8.3). Confirms=true when the two
// agree. Available only when both the foreign spot channel and
// the foreign futures channel produced a reading.
func computeForeignPositioningConfirmation(forces []ForceScore) DirectionalAssessment {
	byDim := map[ForceName]ForceScore{}
	for _, f := range forces {
		byDim[f.Force] = f
	}
	foreign, okF := byDim[ForceForeign]
	fut, okFut := byDim[ForceFutures]
	if !okF || !okFut || !foreign.DataAvailable || !fut.DataAvailable {
		return DirectionalAssessment{
			Available: false,
			Reasons:   []string{"缺現貨或期貨領先訊號"},
		}
	}
	confirms := foreign.Trend == fut.Trend && foreign.Trend != "neutral"
	out := DirectionalAssessment{Available: true}
	switch {
	case foreign.Trend == "bullish" && fut.Trend == "bullish":
		out.Direction = "bullish"
		out.Aligned = []ForceName{ForceForeign, ForceFutures}
	case foreign.Trend == "bearish" && fut.Trend == "bearish":
		out.Direction = "bearish"
		out.Aligned = []ForceName{ForceForeign, ForceFutures}
	default:
		out.Direction = "mixed"
		out.Aligned = []ForceName{ForceForeign}
		out.Opposing = []ForceName{ForceFutures}
	}
	if !confirms {
		out.Reasons = []string{"現貨與期貨方向分歧"}
	}
	return out
}

// computeCrossMarketConfirmation reads the single cross_market_signal
// dimension (TSM ADR). Per spec §9.5 / H-CF-02 the layer is only
// Available once the cross-market calibration is validated; until
// then it reports Available=false with Reasons=["校準中"] so the
// home-page renderer can show a "cross-market pending" pill.
func computeCrossMarketConfirmation(forces []ForceScore) DirectionalAssessment {
	byDim := map[ForceName]ForceScore{}
	for _, f := range forces {
		byDim[f.Force] = f
	}
	adr, ok := byDim[ForceTSMADR]
	if !ok || !adr.DataAvailable {
		return DirectionalAssessment{
			Available: false,
			Reasons:   []string{"缺 TSM ADR 資料"},
		}
	}
	if adr.CalibrationStatus == CalibrationCalibrating {
		return DirectionalAssessment{
			Available: false,
			Reasons:   []string{"校準中"},
		}
	}
	out := DirectionalAssessment{
		Available: true,
		Direction: adr.Trend,
	}
	if adr.Trend == "bullish" || adr.Trend == "bearish" {
		out.Aligned = []ForceName{ForceTSMADR}
	}
	return out
}

// ComputeCapitalFlowAssessment assembles the E07 4-layer assessment
// from a slice of ForceScore. It is the canonical entry point
// automation consumers call (CF-INV-08 / spec §9.5).
//
// Rules (mirror the brief):
//   - Institutional: only when foreign/institutional/dealer all
//     available; same non-neutral trend => that direction, else
//     mixed.
//   - Behavioral: only when government + retail both available; no
//     overall vote (it is a separate signal consumers opt into).
//   - Foreign positioning: only when foreign spot + futures both
//     available; compare spot trend with LeadingTrend.
//   - Cross-market: Available only after H-CF-02 is validated; in
//     E07 this is always false (Reasons=["校準中"]).
//
// The overall CalibrationStatus stays "calibrating" in E07; the
// PrimaryFlow field is intentionally empty. We do NOT synthesize a
// weighted overall score — the brief is explicit that this contract
// is the assessment face, not an automation action.
func ComputeCapitalFlowAssessment(forces []ForceScore) CapitalFlowAssessment {
	asOf := ""
	for _, f := range forces {
		if f.AsOfTradingDate != "" {
			asOf = f.AsOfTradingDate
			break
		}
	}
	return CapitalFlowAssessment{
		AsOfTradingDate:   asOf,
		CalibrationStatus: CalibrationCalibrating,
		Institutional:     computeInstitutionalConsensus(forces),
		Behavioral:        computeBehavioralConfirmation(forces),
		ForeignPosition:   computeForeignPositioningConfirmation(forces),
		CrossMarket:       computeCrossMarketConfirmation(forces),
	}
}
