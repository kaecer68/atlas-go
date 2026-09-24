package sectorallocation

import (
	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/config"
)

// industryHitRateAssessmentDecorator populates the E07 assessment's
// IndustryHitRateEvidence field when the
// configs.sector_allocation.industry_hit_rate_consume_enabled gate is ON.
//
// PR-β (2026-09-24, #1942/#1948): this decorator is the deprecation-path
// side of the consumption chain. When the gate is OFF (default), this
// function short-circuits before any allocation, so the registered
// decorator is byte-identical to "no decorator at all" from the
// assessment's perspective.
//
// The decorator uses the global registered provider if one has been
// wired via RegisterIndustryHitRateProvider (production wiring happens
// in cmd/atlas composition root). When no provider is wired but the
// gate is ON, the decorator still returns gracefully (nil evidence) -
// the wiring is incomplete; the caller should fix that, not panic here.
func industryHitRateAssessmentDecorator(a *capitalflow.CapitalFlowAssessment) {
	if !config.GetIndustryHitRateConsumeEnabled() {
		return // config-off: explicit no-op (byte-identical to pre-PR-β)
	}
	provider := GetRegisteredIndustryHitRateProvider()
	if provider == nil {
		return // gate ON but no provider wired - leave evidence nil
	}
	tilts, err := BuildIndustryHitRateTilt(provider, industryHitRateSource, industryHitRateCondition, industryHitRateRollingWindow)
	if err != nil {
		return // provider returned error - leave evidence nil
	}
	if len(tilts) == 0 {
		return // empty report - leave evidence nil
	}
	ev := buildIndustryHitRateEvidenceFromTilts(tilts)
	a.IndustryHitRateEvidence = ev
}

// buildIndustryHitRateEvidenceFromTilts summarizes the per-L1 tilts into
// the advisory evidence block (PR-β spec). Calibrated rows are those
// whose Observations meet the min_samples gate (default 30 per
// stockpicker spec §1.1); the summary reports both total and calibrated
// counts so a consumer can audit "evidence present but thin" without
// re-querying stockpicker.
func buildIndustryHitRateEvidenceFromTilts(tilts []IndustryHitRateTilt) *capitalflow.IndustryHitRateEvidence {
	ev := &capitalflow.IndustryHitRateEvidence{
		Source:        industryHitRateSource,
		ConditionID:   industryHitRateCondition,
		RollingWindow: industryHitRateRollingWindow,
		RowsTotal:     len(tilts),
		Enabled:       config.GetIndustryHitRateConsumeEnabled(),
	}
	var wilsonSum float64
	maxT := tilts[0].TiltMagnitude
	minT := tilts[0].TiltMagnitude
	for _, t := range tilts {
		if t.Observations >= industryHitRateMinSamples {
			ev.RowsCalibrated++
			wilsonSum += t.WilsonLower
		}
		if t.TiltMagnitude > maxT {
			maxT = t.TiltMagnitude
		}
		if t.TiltMagnitude < minT {
			minT = t.TiltMagnitude
		}
	}
	if ev.RowsCalibrated > 0 {
		ev.MeanWilsonLower = roundTo(wilsonSum/float64(ev.RowsCalibrated), 4)
	}
	ev.MaxTilt = roundTo(maxT, 6)
	ev.MinTilt = roundTo(minT, 6)
	return ev
}

// industryHitRateSource/Condition/RollingWindow are the production query
// parameters the sectorallocation composition root will feed into the
// stockpicker IndustryWinRate call. They are package-private constants so
// only this file (and its tests) can change them; a future version may
// promote them to config knobs once the observation window completes.
const (
	industryHitRateSource        = "stockpicker-momentum-20d-positive"
	industryHitRateCondition     = "momentum-20d-positive"
	industryHitRateRollingWindow = "120d"
	industryHitRateMinSamples    = 30 // matches stockpicker.spec §1.1
)

// roundTo rounds v to n decimal places (used for evidence block JSON
// stability - matches stockpicker.industry_winrate.go's percentage()
// rounding policy).
func roundTo(v float64, n int) float64 {
	m := 1.0
	for i := 0; i < n; i++ {
		m *= 10
	}
	if v >= 0 {
		return float64(int64(v*m+0.5)) / m
	}
	return float64(int64(v*m-0.5)) / m
}

// init wires the assessment decorator so PR-β is in effect when the
// sectorallocation package is imported (production wiring happens in
// cmd/atlas composition root, which imports sectorallocation transitively
// from many modules). The decorator is registered exactly once per
// process; subsequent re-imports in tests are tolerated via the
// idempotency of RegisterAssessmentDecorator (a duplicate registration
// would run twice but is observation-only).
func init() {
	capitalflow.RegisterAssessmentDecorator(industryHitRateAssessmentDecorator)
}
