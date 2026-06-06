package config

import (
	"context"
	"math"
	"testing"
)

func TestStructuralTrendCalibratorParamNames(t *testing.T) {
	c := &StructuralTrendCalibrator{}
	names := c.ParamNames()

	if len(names) != 8 {
		t.Fatalf("ParamNames returned %d names, expected 8: %v", len(names), names)
	}

	// Verify all expected names are present
	expected := map[string]bool{
		"engine_structural_trend_min_trend_strength":            false,
		"engine_structural_trend_min_confidence":                false,
		"engine_structural_trend_min_hit_rate":                  false,
		"engine_structural_trend_override_threshold":            false,
		"engine_structural_trend_ai_revenue_growth_threshold":   false,
		"engine_structural_trend_cowos_utilization_threshold":   false,
		"engine_structural_trend_capex_growth_threshold":        false,
		"engine_structural_trend_semiconductor_index_threshold": false,
	}

	for _, name := range names {
		if _, ok := expected[name]; !ok {
			t.Errorf("unexpected parameter name: %q", name)
			continue
		}
		expected[name] = true
	}

	for name, found := range expected {
		if !found {
			t.Errorf("missing expected parameter name: %q", name)
		}
	}
}

func TestStructuralTrendCalibratorImplementsInterface(t *testing.T) {
	var c ParameterCalibrator = &StructuralTrendCalibrator{}
	_ = c // use the variable to avoid compile error
}

func TestStructuralTrendEvaluatorValidScores(t *testing.T) {
	c := &StructuralTrendCalibrator{}
	eval := c.BuildEvaluator()

	cfg := DefaultParametersConfig()

	score, err := eval(cfg)
	if err != nil {
		t.Fatalf("evaluator returned error on default config: %v", err)
	}

	if math.IsNaN(score) {
		t.Error("evaluator returned NaN score on default config")
	}
	if math.IsInf(score, 0) {
		t.Error("evaluator returned Inf score on default config")
	}

	t.Logf("default config score: %.4f", score)
}

func TestStructuralTrendEvaluatorDefaultConfigIsWellOrdered(t *testing.T) {
	c := &StructuralTrendCalibrator{}
	eval := c.BuildEvaluator()

	cfg := DefaultParametersConfig()
	s := cfg.Engine.StructuralTrend

	// Verify default config satisfies ordering constraints
	t.Logf("defaults: min_trend_strength=%.2f min_confidence=%.2f min_hit_rate=%.2f override_threshold=%.2f",
		s.MinTrendStrength.Value, s.MinConfidence.Value, s.MinHitRate.Value, s.OverrideThreshold.Value)
	t.Logf("defaults: ai_rev_growth=%.1f cowos_util=%.1f capex_growth=%.1f semi_idx=%.1f",
		s.AIRevenueGrowthThreshold.Value, s.CoWoSUtilizationThreshold.Value, s.CapexGrowthThreshold.Value, s.SemiconductorIndexThreshold.Value)

	score, err := eval(cfg)
	if err != nil {
		t.Fatalf("evaluator returned error: %v", err)
	}

	// Default config should score positively (well-ordered, no constraint violations)
	if score < 0 {
		t.Errorf("default config score %.4f should be non-negative (well-ordered config)", score)
	}
}

func TestStructuralTrendEvaluatorConstraintViolations(t *testing.T) {
	c := &StructuralTrendCalibrator{}
	eval := c.BuildEvaluator()

	cfg := DefaultParametersConfig()

	baseScore, err := eval(cfg)
	if err != nil {
		t.Fatalf("baseline evaluator error: %v", err)
	}

	// Violation 1: override > min_confidence (should reduce score)
	violated := DefaultParametersConfig()
	violated.Engine.StructuralTrend.OverrideThreshold.Value = 0.90 // above min_confidence (0.75)
	violated.Engine.StructuralTrend.MinConfidence.Value = 0.75

	vScore, err := eval(violated)
	if err != nil {
		t.Fatalf("violated evaluator error: %v", err)
	}
	if vScore >= baseScore {
		t.Errorf("constraint violation (override=0.90 > min_conf=0.75) did not reduce score: base=%.4f violated=%.4f", baseScore, vScore)
	}
}

func TestStructuralTrendEvaluatorSemiconductorPenalty(t *testing.T) {
	c := &StructuralTrendCalibrator{}
	eval := c.BuildEvaluator()

	cfg := DefaultParametersConfig()

	baseScore, err := eval(cfg)
	if err != nil {
		t.Fatalf("baseline evaluator error: %v", err)
	}

	// Deviate from zero
	cfg.Engine.StructuralTrend.SemiconductorIndexThreshold.Value = 0.5
	penalizedScore, err := eval(cfg)
	if err != nil {
		t.Fatalf("penalized evaluator error: %v", err)
	}
	if penalizedScore >= baseScore {
		t.Errorf("semiconductor_index_threshold=0.5 should be penalized: base=%.4f penalized=%.4f", baseScore, penalizedScore)
	}
}

func TestStructuralTrendEvaluatorAllParamsNonZero(t *testing.T) {
	c := &StructuralTrendCalibrator{}
	eval := c.BuildEvaluator()

	cfg := DefaultParametersConfig()

	// All ordering params must be in [0,1] range
	s := cfg.Engine.StructuralTrend
	if s.MinTrendStrength.Value <= 0 || s.MinTrendStrength.Value > 1 {
		t.Errorf("min_trend_strength out of [0,1]: %.3f", s.MinTrendStrength.Value)
	}
	if s.MinConfidence.Value <= 0 || s.MinConfidence.Value > 1 {
		t.Errorf("min_confidence out of [0,1]: %.3f", s.MinConfidence.Value)
	}
	if s.MinHitRate.Value <= 0 || s.MinHitRate.Value > 1 {
		t.Errorf("min_hit_rate out of [0,1]: %.3f", s.MinHitRate.Value)
	}
	if s.OverrideThreshold.Value <= 0 || s.OverrideThreshold.Value > 1 {
		t.Errorf("override_threshold out of [0,1]: %.3f", s.OverrideThreshold.Value)
	}

	score, err := eval(cfg)
	if err != nil {
		t.Fatalf("evaluator error: %v", err)
	}
	t.Logf("default config score with all defaults: %.4f", score)
}

func TestStructuralTrendCalibrateEndToEnd(t *testing.T) {
	c := &StructuralTrendCalibrator{}
	ctx := context.Background()

	result, err := c.Calibrate(ctx)
	if err != nil {
		t.Fatalf("Calibrate() returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Calibrate() returned nil result")
	}

	t.Logf("calibration result: param_count=%d baseline=%.4f optimized=%.4f verdict=%s summary=%s",
		result.ParamCount, result.BaselineScore, result.OptimizedScore, result.Verdict, result.Summary)

	// Verify LastCalibrated and CalibrationMethod were updated
	params := GetParametersConfig()
	s := params.Engine.StructuralTrend

	calibratedParams := []struct {
		name string
		meta *ParameterMetadata[float64]
	}{
		{"min_trend_strength", &s.MinTrendStrength},
		{"min_confidence", &s.MinConfidence},
		{"min_hit_rate", &s.MinHitRate},
		{"override_threshold", &s.OverrideThreshold},
		{"ai_revenue_growth_threshold", &s.AIRevenueGrowthThreshold},
		{"cowos_utilization_threshold", &s.CoWoSUtilizationThreshold},
		{"capex_growth_threshold", &s.CapexGrowthThreshold},
		{"semiconductor_index_threshold", &s.SemiconductorIndexThreshold},
	}

	allCalibrated := true
	for _, cp := range calibratedParams {
		if cp.meta.LastCalibrated == nil {
			t.Errorf("%s.LastCalibrated is nil after calibration", cp.name)
			allCalibrated = false
		} else {
			t.Logf("%s.LastCalibrated = %s, Method = %s",
				cp.name, cp.meta.LastCalibrated.Format("2006-01-02T15:04:05"), cp.meta.CalibrationMethod)
		}
		if cp.meta.CalibrationMethod == "" {
			t.Errorf("%s.CalibrationMethod is empty after calibration", cp.name)
			allCalibrated = false
		}
	}

	if !allCalibrated {
		t.Error("not all 8 parameters were marked as calibrated")
	}
}
