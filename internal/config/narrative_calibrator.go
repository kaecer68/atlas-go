package config

import (
	"math"
)

// NarrativeCalibrator calibrates 9 high-impact narrative event detection threshold
// parameters using Bayesian optimization via CalibrateParameters().
//
// The evaluator constructs a score surface that rewards:
//   - Per-threshold accuracy (Gaussian distance from ideal range centers)
//   - Threshold diversity (penalizes uniform clustering at extremes)
//   - Type I / Type II error balance (implied by search-range midpoints)
//
// Usage:
//
//	calibrator := &NarrativeCalibrator{}
//	cfg := DefaultCalibrateConfig{}
//	eval := NewNarrativeEvaluator()
//	result, err := CalibrateParameters(ctx, calibrator, eval, cfg)
type NarrativeCalibrator struct{}

// ParamNames returns the 9 narrative event detection thresholds that drive
// the narrative/knowledge_base.go event detection engines.
//
// All names are registered in the parameterTable (param_table.go) and resolve
// to cfg.Narrative.* fields.
func (nc *NarrativeCalibrator) ParamNames() []string {
	return []string{
		"narrative_ai_revenue_growth_threshold",
		"narrative_cowos_utilization_threshold",
		"narrative_capex_growth_threshold",
		"narrative_us10y_change_bps_threshold",
		"narrative_dxy_change_pct_threshold",
		"narrative_geopolitical_gpr_threshold",
		"narrative_oil_change_pct_threshold",
		"narrative_jpy_change_pct_threshold",
		"narrative_vix_level_threshold",
	}
}

// narrativeParamDef holds per-threshold evaluator metadata: its nominal search
// range and the ideal normalized position (0–1) within the (lo, hi) band.
type narrativeParamDef struct {
	value     float64
	lo, hi    float64
	idealNorm float64 // ideal position in [0,1] → idealRaw = lo + idealNorm*(hi-lo)
}

// NewNarrativeEvaluator creates an evaluator compatible with CalibrateParameters.
//
// Score = Σ Gaussian(rawΔ / (rangeSpan·σ)) × diversity_factor
//
// Scoring uses the raw value directly — no clamping — so the surface carries
// a gradient even far outside the nominal search range. This prevents flat
// plateaus that would stall Bayesian optimization.
//
// The diversity factor rewards per-threshold spread (std ≈ 0.22 peak),
// penalising both total uniformity and extreme dispersion.
func NewNarrativeEvaluator() func(cfg *ParametersConfig) (float64, error) {
	return func(cfg *ParametersConfig) (float64, error) {
		params := []narrativeParamDef{
			{cfg.Narrative.AIRevenueGrowthThreshold.Value, 30, 70, 0.55},
			{cfg.Narrative.CoWoSUtilizationThreshold.Value, 75, 95, 0.55},
			{cfg.Narrative.CapexGrowthThreshold.Value, 15, 40, 0.50},
			{cfg.Narrative.US10YChangeBpsThreshold.Value, 5, 20, 0.40},
			{cfg.Narrative.DXYChangePctThreshold.Value, 1.0, 3.0, 0.35},
			{cfg.Narrative.GeopoliticalGPRThreshold.Value, 100, 200, 0.45},
			{cfg.Narrative.OilChangePctThreshold.Value, 3, 10, 0.35},
			{cfg.Narrative.JPYChangePctThreshold.Value, 1.0, 4.0, 0.45},
			{cfg.Narrative.VIXLevelThreshold.Value, 20, 35, 0.40},
		}

		const spread = 0.30 // Gaussian σ as fraction of (hi-lo)
		normalized := make([]float64, len(params))
		thresholdScore := 0.0
		maxNorm := 0.0
		minNorm := 0.0
		normSum := 0.0

		for i, p := range params {
			span := p.hi - p.lo
			ideal := p.lo + p.idealNorm*span

			diff := (p.value - ideal) / (span * spread)
			thresholdScore += math.Exp(-0.5 * diff * diff)

			norm := (p.value - p.lo) / span
			normalized[i] = norm
			normSum += norm
			if i == 0 || norm > maxNorm {
				maxNorm = norm
			}
			if i == 0 || norm < minNorm {
				minNorm = norm
			}
		}

		mean := normSum / float64(len(params))

		variance := 0.0
		for _, n := range normalized {
			d := n - mean
			variance += d * d
		}
		variance /= float64(len(params))
		std := math.Sqrt(variance)

		distFromSweet := (std - 0.22) / 0.15
		diversity := math.Exp(-0.5 * distFromSweet * distFromSweet)
		if diversity < 0.2 {
			diversity = 0.2
		}

		_ = minNorm
		_ = maxNorm

		return thresholdScore * diversity, nil
	}
}
