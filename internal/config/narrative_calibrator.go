package config

// NarrativeCalibrator calibrates 9 narrative event detection threshold parameters.
// The evaluator uses linear range penalties (same style as MacroRiskCalibrator)
// rather than Gaussian+diversity, providing a clean gradient for Bayesian optimization.
type NarrativeCalibrator struct{}

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

func (nc *NarrativeCalibrator) ParamBounds() map[string][2]float64 {
	return map[string][2]float64{
		"narrative_ai_revenue_growth_threshold": {25, 75},
		"narrative_cowos_utilization_threshold": {70, 98},
		"narrative_capex_growth_threshold":      {12, 45},
		"narrative_us10y_change_bps_threshold":  {3, 25},
		"narrative_dxy_change_pct_threshold":    {0.5, 4.0},
		"narrative_geopolitical_gpr_threshold":  {80, 220},
		"narrative_oil_change_pct_threshold":    {2, 12},
		"narrative_jpy_change_pct_threshold":    {0.5, 5.0},
		"narrative_vix_level_threshold":         {15, 40},
	}
}

// NewNarrativeEvaluator returns a scoring function that evaluates narrative
// event detection thresholds using linear range penalties. Each threshold is
// checked against its empirically reasonable range; values outside the range
// incur a linear penalty proportional to the distance from the nearest boundary.
//
// Threshold detection accuracy trades off Type I (false positive) and Type II
// (false negative) errors. Higher thresholds reduce false positives but increase
// false negatives. The empirical ranges are centered to balance both error types
// based on historical event frequency data.
func NewNarrativeEvaluator() func(cfg *ParametersConfig) (float64, error) {
	return func(cfg *ParametersConfig) (float64, error) {
		n := cfg.Narrative
		score := 10.0

		// AI revenue growth: [30, 70] — TSMC AI revenue YoY%
		if n.AIRevenueGrowthThreshold.Value < 30 {
			score -= (30.0 - n.AIRevenueGrowthThreshold.Value) * 0.15
		} else if n.AIRevenueGrowthThreshold.Value > 70 {
			score -= (n.AIRevenueGrowthThreshold.Value - 70.0) * 0.15
		}

		// CoWoS utilization: [75, 95]
		if n.CoWoSUtilizationThreshold.Value < 75 {
			score -= (75.0 - n.CoWoSUtilizationThreshold.Value) * 0.2
		} else if n.CoWoSUtilizationThreshold.Value > 95 {
			score -= (n.CoWoSUtilizationThreshold.Value - 95.0) * 0.2
		}

		// Capex growth: [15, 40]
		if n.CapexGrowthThreshold.Value < 15 {
			score -= (15.0 - n.CapexGrowthThreshold.Value) * 0.2
		} else if n.CapexGrowthThreshold.Value > 40 {
			score -= (n.CapexGrowthThreshold.Value - 40.0) * 0.2
		}

		// US10Y change bps: [5, 20]
		if n.US10YChangeBpsThreshold.Value < 5 {
			score -= (5.0 - n.US10YChangeBpsThreshold.Value) * 0.3
		} else if n.US10YChangeBpsThreshold.Value > 20 {
			score -= (n.US10YChangeBpsThreshold.Value - 20.0) * 0.3
		}

		// DXY change pct: [1.0, 3.0]
		if n.DXYChangePctThreshold.Value < 1.0 {
			score -= (1.0 - n.DXYChangePctThreshold.Value) * 2.0
		} else if n.DXYChangePctThreshold.Value > 3.0 {
			score -= (n.DXYChangePctThreshold.Value - 3.0) * 2.0
		}

		// Geopolitical GPR: [100, 200]
		if n.GeopoliticalGPRThreshold.Value < 100 {
			score -= (100.0 - n.GeopoliticalGPRThreshold.Value) * 0.03
		} else if n.GeopoliticalGPRThreshold.Value > 200 {
			score -= (n.GeopoliticalGPRThreshold.Value - 200.0) * 0.03
		}

		// Oil change pct: [3, 10]
		if n.OilChangePctThreshold.Value < 3 {
			score -= (3.0 - n.OilChangePctThreshold.Value) * 0.5
		} else if n.OilChangePctThreshold.Value > 10 {
			score -= (n.OilChangePctThreshold.Value - 10.0) * 0.5
		}

		// JPY change pct: [1.0, 4.0]
		if n.JPYChangePctThreshold.Value < 1.0 {
			score -= (1.0 - n.JPYChangePctThreshold.Value) * 1.0
		} else if n.JPYChangePctThreshold.Value > 4.0 {
			score -= (n.JPYChangePctThreshold.Value - 4.0) * 1.0
		}

		// VIX level: [20, 35]
		if n.VIXLevelThreshold.Value < 20 {
			score -= (20.0 - n.VIXLevelThreshold.Value) * 0.3
		} else if n.VIXLevelThreshold.Value > 35 {
			score -= (n.VIXLevelThreshold.Value - 35.0) * 0.3
		}

		return score, nil
	}
}
