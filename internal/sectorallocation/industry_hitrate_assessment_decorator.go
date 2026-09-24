package sectorallocation

import (
	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/config"
)

// ---- capital-flow assessment side of the consumption chain ---------------
//
// The sectorallocation recommendation path consumes the industry hit-rate
// evidence as driver tilts (industry_hitrate_consume.go). The capital-flow
// assessment is the second, independent consumer: it echoes WHAT the chain saw
// and WHY nothing was applied, so an operator can tell "gate off" apart from
// "gate on, evidence thin" without reading server logs.
//
// The evidence block is advisory: it never changes CalibrationStatus or
// Eligibility (the E07 calibration pipeline owns those), and it is nil when the
// gate is off, so the assessment JSON is byte-identical with the pre-change
// revision in the default configuration.

// Production query tuple. These mirror the canonical stockpicker condition the
// industry aggregate is published under; they are package-private because the
// chain must not silently read a different condition than the one the
// observation window was opened on. Promoting them to config knobs is a
// follow-up once the gate is promoted.
const (
	industryHitRateSource        = "stockpicker-momentum-20d-positive"
	industryHitRateCondition     = "momentum-20d-positive"
	industryHitRateRollingWindow = "120d"
)

// industryHitRateAssessmentDecorator attaches the consumption evidence to the
// assessment. Registered once in init(); de-registered only by
// capitalflow.ResetAssessmentDecorators (tests).
func industryHitRateAssessmentDecorator(a *capitalflow.CapitalFlowAssessment) {
	if !config.GetIndustryHitRateConsumeEnabled() {
		return // gate off: industry_hit_rate_evidence stays absent
	}
	decision := EvaluateIndustryHitRateConsume(GetRegisteredIndustryHitRateProvider(),
		industryHitRateSource, industryHitRateCondition, industryHitRateRollingWindow)
	a.IndustryHitRateEvidence = buildIndustryHitRateEvidence(decision)
}

// buildIndustryHitRateEvidence summarizes one decision for audit: the query
// tuple, whether tilts were applied, the reason when they were not, the row
// counts, and the tilt extremes (0 when not applied).
func buildIndustryHitRateEvidence(d IndustryHitRateDecision) *capitalflow.IndustryHitRateEvidence {
	ev := &capitalflow.IndustryHitRateEvidence{
		Source:         industryHitRateSource,
		ConditionID:    industryHitRateCondition,
		RollingWindow:  industryHitRateRollingWindow,
		Applied:        d.Applied,
		Reason:         d.Reason,
		RowsTotal:      d.RowsTotal,
		RowsCalibrated: d.RowsCalibrated,
	}
	if !d.Applied || len(d.Tilts) == 0 {
		return ev
	}
	var wilsonSum float64
	maxTilt := d.Tilts[0].TiltMagnitude
	minTilt := d.Tilts[0].TiltMagnitude
	for _, t := range d.Tilts {
		wilsonSum += t.WilsonLower
		maxTilt = max(maxTilt, t.TiltMagnitude)
		minTilt = min(minTilt, t.TiltMagnitude)
	}
	ev.MeanWilsonLower = roundTo(wilsonSum/float64(len(d.Tilts)), 4)
	ev.MaxTilt = roundTo(maxTilt, 6)
	ev.MinTilt = roundTo(minTilt, 6)
	return ev
}

// roundTo rounds v to n decimal places so the evidence block is stable in
// JSON (mirrors the stockpicker reporting rounding policy).
func roundTo(v float64, n int) float64 {
	m := 1.0
	for range n {
		m *= 10
	}
	if v >= 0 {
		return float64(int64(v*m+0.5)) / m
	}
	return float64(int64(v*m-0.5)) / m
}

// init wires the assessment decorator. The registration is cheap and inert by
// default: the decorator returns immediately while the config gate is off, and
// capitalflow.applyAssessmentDecorators does not copy the assessment when no
// decorator is registered.
func init() {
	capitalflow.RegisterAssessmentDecorator(industryHitRateAssessmentDecorator)
}
