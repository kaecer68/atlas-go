package capitalflow

import (
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// ---------------------------------------------------------------------------
// Report generator
// ---------------------------------------------------------------------------

// GenerateDailyReport produces a full daily report from force scores.
// GenerateDailyReport produces a full daily report from force scores.
//
// PR-3a (capital-flow model plan v1.1): period is the seven-period market
// classification for the report's trading date (nil = unknown → legacy
// semantics). periodWeighted mirrors the config gate
// capitalflow.period_weighted_quality (default false): when true,
// quality_score switches to the period-weighted composite; when false the
// legacy equal-weight composite is kept bit-identical. Both values are
// always emitted (quality_score vs quality_score_period_weighted) for the
// 30-trading-day observation window (k3 review B4: observation-first).
func GenerateDailyReport(date time.Time, forces []ForceScore, resonance ResonanceResult, period *domain.MarketPeriod, periodWeighted bool) DailyReport {
	legacy := computeQualityScore(forces)
	weighted := computeQualityScoreWithPeriod(forces, period)
	quality := legacy
	if periodWeighted {
		quality = weighted
	}
	label := qualityLabel(quality)
	assessment := ComputeCapitalFlowAssessment(forces)
	if assessment.AsOfTradingDate == "" && !date.IsZero() {
		assessment.AsOfTradingDate = date.Format("2006-01-02")
	}
	dominantActor, dominantSignal := dominantActorAndSignal(forces)

	return DailyReport{
		Date:                       date,
		Forces:                     applyForceWeights(forces),
		Resonance:                  resonance,
		QualityScore:               round(quality, 2),
		QualityScorePeriodWeighted: round(weighted, 2),
		QualityLabel:               label,
		Summary:                    buildSummary(resonance, quality, label, assessment),
		Assessment:                 assessment,
		LegacyQuality:              !periodWeighted,
		DominantActor:              dominantActor,
		DominantSignal:             dominantSignal,
	}
}

// GenerateSummaryReport produces a condensed summary.
// GenerateSummaryReport produces a condensed summary. See
// GenerateDailyReport for the period / periodWeighted contract (PR-3a).
func GenerateSummaryReport(date time.Time, forces []ForceScore, resonance ResonanceResult, period *domain.MarketPeriod, periodWeighted bool) SummaryReport {
	legacy := computeQualityScore(forces)
	weighted := computeQualityScoreWithPeriod(forces, period)
	quality := legacy
	if periodWeighted {
		quality = weighted
	}
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
	// M6 (audit): when no actor and no signal reading is present the
	// dominant stays empty instead of fabricating ForceRetail — a
	// missing picture must not name 散戶 as the default protagonist.
	// buildShortSummary ignores dominant (it never produced 武斷文案),
	// and the API/frontend render an empty dominant_force as "—".

	return SummaryReport{
		Date:                       date,
		QualityScore:               round(quality, 2),
		QualityScorePeriodWeighted: round(weighted, 2),
		QualityLabel:               label,
		ResonanceDir:               resonance.Direction,
		DominantForce:              dominant,
		Forces:                     applyForceWeights(forces),
		Summary:                    buildShortSummary(label, resonance, dominant, assessment),
		Assessment:                 assessment,
		LegacyQuality:              !periodWeighted,
		DominantActor:              dominantActor,
		DominantSignal:             dominantSignal,
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

// computeQualityScoreWithPeriod = wF·Foreign(Z) + wI·Institutional(Z) − wR·Retail(Z)
// Weights adapt to the market period per 憲章 §4 "外資權威":
//
//	Bull / TurnaroundUp → wF=1.3 (foreign leads the trend)
//	Downturn / TurnaroundDown → wI=1.3, wF=0.7 (foreign panics)
//	BlackSwan → wR=1.5 (retail reverse indicator amplified)
//	Default / nil → wF=wI=wR=1.0 (equal weights)
func computeQualityScoreWithPeriod(forces []ForceScore, period *domain.MarketPeriod) float64 {
	wF, wI, wR := 1.0, 1.0, 1.0
	if period != nil {
		switch *period {
		case domain.PeriodBull, domain.PeriodTurnaroundUp:
			wF, wI, wR = 1.3, 1.0, 1.0
		case domain.PeriodDownturn, domain.PeriodTurnaroundDown:
			wF, wI, wR = 0.7, 1.3, 1.0
		case domain.PeriodBlackSwan:
			wF, wI, wR = 1.0, 1.0, 1.5
		}
	}
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
	return wF*foreignZ + wI*instZ - wR*retailZ
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
