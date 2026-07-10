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

	// Find dominant force (highest absolute Z-score)
	dominant := ForceRetail
	maxAbsZ := 0.0
	for _, f := range forces {
		absZ := f.ZScore
		if absZ < 0 {
			absZ = -absZ
		}
		if absZ > maxAbsZ {
			maxAbsZ = absZ
			dominant = f.Force
		}
	}
	_ = dominant // used later in summary context

	return DailyReport{
		Date:         date,
		Forces:       forces,
		Resonance:    resonance,
		QualityScore: round(quality, 2),
		QualityLabel: label,
		Summary:      buildSummary(resonance, quality, label),
	}
}

// GenerateSummaryReport produces a condensed summary.
func GenerateSummaryReport(date time.Time, forces []ForceScore, resonance ResonanceResult) SummaryReport {
	quality := computeQualityScore(forces)
	label := qualityLabel(quality)

	dominant := ForceRetail
	maxAbsZ := 0.0
	for _, f := range forces {
		absZ := f.ZScore
		if absZ < 0 {
			absZ = -absZ
		}
		if absZ > maxAbsZ {
			maxAbsZ = absZ
			dominant = f.Force
		}
	}

	return SummaryReport{
		Date:          date,
		QualityScore:  round(quality, 2),
		QualityLabel:  label,
		ResonanceDir:  resonance.Direction,
		DominantForce: dominant,
		Forces:        forces,
		Summary:       buildShortSummary(label, resonance, dominant),
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

func buildSummary(resonance ResonanceResult, quality float64, label string) string {
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

	_ = quality
	return strings.Join(parts, "，") + "。"
}

func buildShortSummary(label string, resonance ResonanceResult, dominant ForceName) string {
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

	return strings.Join(parts, "，") + "。"
}
