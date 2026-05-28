package config

import (
	"context"
	"fmt"
)

// StructuralTrendCalibrator calibrates the 8 Engine StructuralTrend parameters
// using the ParameterCalibrator interface and Bayesian optimization pipeline.
// It enforces monotonic ordering constraints (e.g. override_threshold ≤ min_confidence,
// min_trend_strength < min_confidence) and keeps threshold values within empirically
// reasonable ranges.
type StructuralTrendCalibrator struct{}

// ParamNames returns the full list of engine_structural_trend_* parameter names
// registered in the parameterTable and markCalibrated lookup.
func (c *StructuralTrendCalibrator) ParamNames() []string {
	return []string{
		"engine_structural_trend_min_trend_strength",
		"engine_structural_trend_min_confidence",
		"engine_structural_trend_min_hit_rate",
		"engine_structural_trend_override_threshold",
		"engine_structural_trend_ai_revenue_growth_threshold",
		"engine_structural_trend_cowos_utilization_threshold",
		"engine_structural_trend_capex_growth_threshold",
		"engine_structural_trend_semiconductor_index_threshold",
	}
}

// BuildEvaluator returns a scoring function for StructuralTrend parameter calibration.
// The evaluator computes a composite score from three categories:
//
//  1. Monotonic ordering constraints — higher violation = higher penalty:
//     - override_threshold must be ≤ min_confidence (structural override easier than detection)
//     - min_trend_strength must be < min_confidence (detection bar above noise filter)
//     - min_hit_rate should be ≤ min_confidence
//
//  2. Empirical range checks — each % threshold is scored against its empirically
//     reasonable range; values outside the range incur a linear penalty.
//
//  3. Bonus points — awarded for well-ordered configurations where the gap between
//     min_confidence and subordinate thresholds is large enough to provide clean
//     separation between detection and action thresholds.
//
// The semiconductor_index_threshold is a placeholder (value 0.0). Any deviation from
// zero receives a heavy penalty, effectively locking the optimizer away from it.
func (c *StructuralTrendCalibrator) BuildEvaluator() func(cfg *ParametersConfig) (float64, error) {
	return func(cfg *ParametersConfig) (float64, error) {
		s := cfg.Engine.StructuralTrend
		score := 10.0

		// --- monotonic ordering constraints ---

		// override_threshold must be <= min_confidence
		if s.OverrideThreshold.Value > s.MinConfidence.Value {
			score -= (s.OverrideThreshold.Value - s.MinConfidence.Value) * 20.0
		}

		// min_trend_strength must be < min_confidence
		if s.MinTrendStrength.Value >= s.MinConfidence.Value {
			score -= (s.MinTrendStrength.Value - s.MinConfidence.Value + 0.01) * 20.0
		}

		// min_hit_rate should be <= min_confidence
		if s.MinHitRate.Value > s.MinConfidence.Value {
			score -= (s.MinHitRate.Value - s.MinConfidence.Value) * 15.0
		}

		// --- empirical range checks ---

		// AI revenue growth: search [30, 70]
		if s.AIRevenueGrowthThreshold.Value < 30 {
			score -= (30.0 - s.AIRevenueGrowthThreshold.Value) * 0.1
		} else if s.AIRevenueGrowthThreshold.Value > 70 {
			score -= (s.AIRevenueGrowthThreshold.Value - 70.0) * 0.1
		}

		// CoWoS utilization: search [75, 95]
		if s.CoWoSUtilizationThreshold.Value < 75 {
			score -= (75.0 - s.CoWoSUtilizationThreshold.Value) * 0.1
		} else if s.CoWoSUtilizationThreshold.Value > 95 {
			score -= (s.CoWoSUtilizationThreshold.Value - 95.0) * 0.1
		}

		// Capex growth: search [25, 55]
		if s.CapexGrowthThreshold.Value < 25 {
			score -= (25.0 - s.CapexGrowthThreshold.Value) * 0.05
		} else if s.CapexGrowthThreshold.Value > 55 {
			score -= (s.CapexGrowthThreshold.Value - 55.0) * 0.05
		}

		// --- bonus for well-ordered configurations ---

		// Larger gap between min_confidence and min_trend_strength is better
		if gap := s.MinConfidence.Value - s.MinTrendStrength.Value; gap > 0 {
			score += gap * 5.0
		}

		// Larger gap between min_confidence and override_threshold is better
		// (override should be easier to trigger than detection)
		if gap := s.MinConfidence.Value - s.OverrideThreshold.Value; gap > 0 {
			score += gap * 3.0
		}

		// --- semiconductor_index_threshold: placeholder, keep at 0 ---
		if s.SemiconductorIndexThreshold.Value != 0 {
			score -= s.SemiconductorIndexThreshold.Value * s.SemiconductorIndexThreshold.Value * 100.0
		}

		return score, nil
	}
}

// Calibrate runs the full calibration pipeline for StructuralTrend parameters
// using the default CalibrateConfig (InitialPoints: 6, Iterations: 10, MinImprovement: 0.02).
func (c *StructuralTrendCalibrator) Calibrate(ctx context.Context) (*CalibratorResult, error) {
	cfg := CalibrateConfig{
		InitialPoints:  6,
		Iterations:     10,
		MinImprovement: 0.02,
	}
	result, err := CalibrateParameters(ctx, c, c.BuildEvaluator(), cfg)
	if err != nil {
		return nil, fmt.Errorf("structural_trend_calibrate: %w", err)
	}
	return result, nil
}
