package capitalflow

import (
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Report generator
// ---------------------------------------------------------------------------

// GenerateDailyReport produces a full daily report from force scores.
func GenerateDailyReport(date time.Time, forces []ForceScore, resonance ResonanceResult) DailyReport {
	quality := computeQualityScore(forces)
	label := qualityLabel(quality)
	assessment := ComputeCapitalFlowAssessment(forces)
	if assessment.AsOfTradingDate == "" && !date.IsZero() {
		assessment.AsOfTradingDate = date.Format("2006-01-02")
	}
	dominantActor, dominantSignal := dominantActorAndSignal(forces)

	return DailyReport{
		Date:           date,
		Forces:         applyForceWeights(forces),
		Resonance:      resonance,
		QualityScore:   round(quality, 2),
		QualityLabel:   label,
		Summary:        buildSummary(resonance, quality, label, assessment),
		Assessment:     assessment,
		LegacyQuality:  true,
		DominantActor:  dominantActor,
		DominantSignal: dominantSignal,
	}
}

// GenerateSummaryReport produces a condensed summary.
func GenerateSummaryReport(date time.Time, forces []ForceScore, resonance ResonanceResult) SummaryReport {
	quality := computeQualityScore(forces)
	label := qualityLabel(quality)
	assessment := ComputeCapitalFlowAssessment(forces)
	if assessment.AsOfTradingDate == "" && !date.IsZero() {
		assessment.AsOfTradingDate = date.Format("2006-01-02")
	}
	dominantActor, dominantSignal := dominantActorAndSignal(forces)
	dominant := dominantActor
	if dominant == "" {
		dominant = dominantSignal
	}
	if dominant == "" {
		dominant = ForceRetail
	}

	return SummaryReport{
		Date:           date,
		QualityScore:   round(quality, 2),
		QualityLabel:   label,
		ResonanceDir:   resonance.Direction,
		DominantForce:  dominant,
		Forces:         applyForceWeights(forces),
		Summary:        buildShortSummary(label, resonance, dominant, assessment),
		Assessment:     assessment,
		LegacyQuality:  true,
		DominantActor:  dominantActor,
		DominantSignal: dominantSignal,
	}
}

// computeQualityScore = Foreign(Z) + Institutional(Z) – Retail(Z)
func computeQualityScore(forces []ForceScore) float64 {
	var foreignZ, instZ, retailZ float64
	for _, f := range forces {
		switch f.Force {
		case ForceForeign:
			foreignZ = f.ZScore
		case ForceInstitutional:
			instZ = f.ZScore
		case ForceRetail:
			retailZ = f.ZScore
		}
	}
	return foreignZ + instZ - retailZ
}

func qualityLabel(score float64) string {
	switch {
	case score > 1.5:
		return "strong_inflow"
	case score > 0.5:
		return "inflow"
	case score > -0.5:
		return "neutral"
	case score > -1.5:
		return "outflow"
	default:
		return "strong_outflow"
	}
}

func buildSummary(resonance ResonanceResult, quality float64, label string, assessment CapitalFlowAssessment) string {
	var parts []string

	// Quality
	switch label {
	case "strong_inflow":
		parts = append(parts, "資金強勁流入")
	case "inflow":
		parts = append(parts, "資金溫和流入")
	case "outflow":
		parts = append(parts, "資金溫和流出")
	case "strong_outflow":
		parts = append(parts, "資金強勁流出")
	default:
		parts = append(parts, "資金流向中性")
	}

	// Resonance
	if resonance.Coefficient >= 1.5 {
		parts = append(parts, "多勢力共鳴")
	} else if resonance.Coefficient <= 0.5 {
		parts = append(parts, "勢力對抗")
	}

	// Dominant direction
	switch resonance.Direction {
	case "bullish":
		parts = append(parts, "偏多格局")
	case "bearish":
		parts = append(parts, "偏空格局")
	}

	// E07 calibration gate — surface the "calibrating" status
	// so the home-page renderer can show the spec §9.5 pill.
	if assessment.CalibrationStatus == CalibrationCalibrating {
		parts = append(parts, "校準中")
	}

	_ = quality
	return strings.Join(parts, "，") + "。"
}

func buildShortSummary(label string, resonance ResonanceResult, dominant ForceName, assessment CapitalFlowAssessment) string {
	_ = dominant
	var parts []string
	switch label {
	case "strong_inflow":
		parts = append(parts, "資金強勁流入")
	case "inflow":
		parts = append(parts, "資金溫和流入")
	case "outflow":
		parts = append(parts, "資金溫和流出")
	case "strong_outflow":
		parts = append(parts, "資金強勁流出")
	default:
		parts = append(parts, "資金流向中性")
	}

	if resonance.Coefficient >= 1.5 {
		parts = append(parts, "多勢力共鳴")
	} else if resonance.Coefficient <= 0.5 {
		parts = append(parts, "勢力對抗")
	}

	// E07 calibration gate (spec §9.5 / CF-INV-13).
	if assessment.CalibrationStatus == CalibrationCalibrating {
		parts = append(parts, "校準中")
	}

	return strings.Join(parts, "，") + "。"
}

// dominantActorAndSignal picks the highest-|Z| official_actor
// dimension (DominantActor) and the highest-|Z| signal dimension
// (positioning_indicator / cross_market_signal) separately, per
// spec §7 / CF-INV-10. The two fields are independently
// addressable; a strong TSM ADR must NOT win the actor slot.
func dominantActorAndSignal(forces []ForceScore) (actor ForceName, signal ForceName) {
	var actorMaxAbs, signalMaxAbs float64
	for _, f := range forces {
		absZ := f.ZScore
		if absZ < 0 {
			absZ = -absZ
		}
		switch f.DimensionRole {
		case DimensionRoleOfficialActor:
			if absZ > actorMaxAbs {
				actorMaxAbs = absZ
				actor = f.Force
			}
		case DimensionRolePositioningIndicator, DimensionRoleCrossMarketSignal:
			if absZ > signalMaxAbs {
				signalMaxAbs = absZ
				signal = f.Force
			}
		}
	}
	return actor, signal
}

// applyForceWeights returns a deep copy of forces with every
// Weight forced to 0 and WeightDeprecated set to true. Per spec
// §7.2 / CF-INV-07 the legacy cross-unit weight is suppressed
// because it cannot meaningfully aggregate different unit scales
// (TWD vs shares vs contracts); new consumers must read the
// DominantActor / DominantSignal / Assessment fields instead.
//
// Note: the input slice is not mutated; a fresh slice is
// returned. This mirrors the old semantics (a copy was always
// returned) so existing callers that ignore Weight keep working.
func applyForceWeights(forces []ForceScore) []ForceScore {
	out := make([]ForceScore, len(forces))
	for i, f := range forces {
		f.Weight = 0
		f.WeightDeprecated = true
		out[i] = f
	}
	return out
}
