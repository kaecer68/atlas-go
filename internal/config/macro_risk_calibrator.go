package config

import (
	"context"
	"fmt"
)

// MacroRiskCalibrator calibrates the 9 Engine MacroRisk parameters
// using the ParameterCalibrator interface and Bayesian optimization pipeline.
// It enforces ordering constraints (outflow_prob_base < outflow_prob_max)
// and scores parameters against empirically-derived literature ranges.
type MacroRiskCalibrator struct{}

// NewMacroRiskCalibrator returns a new MacroRiskCalibrator.
func NewMacroRiskCalibrator() *MacroRiskCalibrator {
	return &MacroRiskCalibrator{}
}

// ParamNames returns the full list of engine_macro_risk_* parameter names
// registered in the parameterTable and markCalibrated lookup.
func (c *MacroRiskCalibrator) ParamNames() []string {
	return []string{
		"engine_macro_risk_carry_trade_unwind_threshold",
		"engine_macro_risk_vix_threshold",
		"engine_macro_risk_us10y_threshold",
		"engine_macro_risk_oil_shock_threshold_pct",
		"engine_macro_risk_gold_surge_threshold_pct",
		"engine_macro_risk_dxy_surge_threshold_pct",
		"engine_macro_risk_twd_stress_threshold_pct",
		"engine_macro_risk_outflow_prob_base",
		"engine_macro_risk_outflow_prob_max",
	}
}

// BuildEvaluator returns a scoring function for MacroRisk parameter calibration.
// The evaluator computes a composite score from three categories:
//
//  1. Ordering constraint — outflow_prob_base must be < outflow_prob_max
//     Violation incurs a heavy quadratic penalty.
//
//  2. Empirical range checks — each threshold is scored against its empirically
//     reasonable range derived from academic and practitioner literature.
//     Values inside the range receive no penalty; values outside incur a
//     linear penalty proportional to the distance from the nearest boundary.
//
//  3. Bonus points — awarded for configurations where outflow_prob_max has
//     sufficient headroom above outflow_prob_base, allowing clean separation
//     between baseline and extreme outflow probability regimes.
//
// Empirical range sources:
//   - VIX 25-35: Bloom, 2009; Whaley, 2000
//   - JPY/USD 140-150: Ito & McCauley, 2020; BIS carry trade literature
//   - US10Y 4-5%: Bernanke, 2015; Fed dot-plot implied terminal rate corridor
//   - Oil shock 8-15%: Hamilton, 2003; Kilian, 2009
func (c *MacroRiskCalibrator) BuildEvaluator() func(cfg *ParametersConfig) (float64, error) {
	return func(cfg *ParametersConfig) (float64, error) {
		m := cfg.Engine.MacroRisk
		score := 10.0

		// --- ordering constraint: outflow_prob_base < outflow_prob_max ---

		if m.OutflowProbBase.Value >= m.OutflowProbMax.Value {
			violation := m.OutflowProbBase.Value - m.OutflowProbMax.Value + 0.01
			score -= violation * violation * 50.0
		}

		// --- empirical range checks ---

		// VIX: [25, 35] — fear regime threshold
		if m.VIXThreshold.Value < 25 {
			score -= (25.0 - m.VIXThreshold.Value) * 0.5
		} else if m.VIXThreshold.Value > 35 {
			score -= (m.VIXThreshold.Value - 35.0) * 0.5
		}

		// JPY/USD carry trade unwind: [140, 150]
		if m.CarryTradeUnwindThreshold.Value < 140 {
			score -= (140.0 - m.CarryTradeUnwindThreshold.Value) * 0.2
		} else if m.CarryTradeUnwindThreshold.Value > 150 {
			score -= (m.CarryTradeUnwindThreshold.Value - 150.0) * 0.2
		}

		// US10Y: [4.0, 5.0]
		if m.US10YThreshold.Value < 4.0 {
			score -= (4.0 - m.US10YThreshold.Value) * 2.0
		} else if m.US10YThreshold.Value > 5.0 {
			score -= (m.US10YThreshold.Value - 5.0) * 2.0
		}

		// Oil shock: [8, 15]
		if m.OilShockThresholdPct.Value < 8 {
			score -= (8.0 - m.OilShockThresholdPct.Value) * 0.3
		} else if m.OilShockThresholdPct.Value > 15 {
			score -= (m.OilShockThresholdPct.Value - 15.0) * 0.3
		}

		// Gold surge: [3, 8]
		if m.GoldSurgeThresholdPct.Value < 3 {
			score -= (3.0 - m.GoldSurgeThresholdPct.Value) * 0.5
		} else if m.GoldSurgeThresholdPct.Value > 8 {
			score -= (m.GoldSurgeThresholdPct.Value - 8.0) * 0.5
		}

		// DXY surge: [1.0, 2.5]
		if m.DXYSurgeThresholdPct.Value < 1.0 {
			score -= (1.0 - m.DXYSurgeThresholdPct.Value) * 3.0
		} else if m.DXYSurgeThresholdPct.Value > 2.5 {
			score -= (m.DXYSurgeThresholdPct.Value - 2.5) * 3.0
		}

		// TWD stress: [1.5, 3.0]
		if m.TWDStressThresholdPct.Value < 1.5 {
			score -= (1.5 - m.TWDStressThresholdPct.Value) * 3.0
		} else if m.TWDStressThresholdPct.Value > 3.0 {
			score -= (m.TWDStressThresholdPct.Value - 3.0) * 3.0
		}

		// OutflowProbBase: [30, 50]
		if m.OutflowProbBase.Value < 30 {
			score -= (30.0 - m.OutflowProbBase.Value) * 0.2
		} else if m.OutflowProbBase.Value > 50 {
			score -= (m.OutflowProbBase.Value - 50.0) * 0.2
		}

		// OutflowProbMax: [70, 90]
		if m.OutflowProbMax.Value < 70 {
			score -= (70.0 - m.OutflowProbMax.Value) * 0.2
		} else if m.OutflowProbMax.Value > 90 {
			score -= (m.OutflowProbMax.Value - 90.0) * 0.2
		}

		// --- bonus for well-separated outflow probability ranges ---

		if gap := m.OutflowProbMax.Value - m.OutflowProbBase.Value; gap > 10 {
			score += (gap - 10.0) * 0.05
		}

		return score, nil
	}
}

// MacroRiskEvaluator provides a standalone scoring function for MacroRisk parameter
// evaluation. It is a convenience wrapper around BuildEvaluator that can be used
// directly with calibration or sweep workflows.
//
// Usage:
//
//	evaluator := NewMacroRiskCalibrator().BuildEvaluator()
//	score, err := evaluator(params)
func MacroRiskEvaluator(cfg *ParametersConfig) (float64, error) {
	c := &MacroRiskCalibrator{}
	return c.BuildEvaluator()(cfg)
}

// Calibrate runs the full calibration pipeline for MacroRisk parameters
// using the default CalibrateConfig (InitialPoints: 6, Iterations: 10, MinImprovement: 0.02).
func (c *MacroRiskCalibrator) Calibrate(ctx context.Context) (*CalibratorResult, error) {
	cfg := CalibrateConfig{
		InitialPoints:  6,
		Iterations:     10,
		MinImprovement: 0.02,
	}
	result, err := CalibrateParameters(ctx, c, c.BuildEvaluator(), cfg)
	if err != nil {
		return nil, fmt.Errorf("macro_risk_calibrate: %w", err)
	}
	return result, nil
}
