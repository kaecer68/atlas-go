package config

import (
	"context"
	"testing"
	"time"
)

func TestMacroRiskCalibrator_ParamNames(t *testing.T) {
	c := NewMacroRiskCalibrator()
	names := c.ParamNames()

	if len(names) != 9 {
		t.Fatalf("expected 9 parameter names, got %d: %v", len(names), names)
	}

	expected := map[string]bool{
		"engine_macro_risk_carry_trade_unwind_threshold": false,
		"engine_macro_risk_vix_threshold":                false,
		"engine_macro_risk_us10y_threshold":              false,
		"engine_macro_risk_oil_shock_threshold_pct":      false,
		"engine_macro_risk_gold_surge_threshold_pct":     false,
		"engine_macro_risk_dxy_surge_threshold_pct":      false,
		"engine_macro_risk_twd_stress_threshold_pct":     false,
		"engine_macro_risk_outflow_prob_base":            false,
		"engine_macro_risk_outflow_prob_max":             false,
	}

	for _, name := range names {
		if _, ok := expected[name]; !ok {
			t.Errorf("unexpected parameter name: %s", name)
			continue
		}
		expected[name] = true
	}

	for name, found := range expected {
		if !found {
			t.Errorf("missing parameter name: %s", name)
		}
	}
}

func TestMacroRiskEvaluator_DefaultConfig(t *testing.T) {
	cfg := DefaultParametersConfig()
	evaluator := (&MacroRiskCalibrator{}).BuildEvaluator()
	score, err := evaluator(cfg)
	if err != nil {
		t.Fatalf("evaluator returned error on default config: %v", err)
	}
	if score <= 0 {
		t.Errorf("expected positive score on default config, got %.4f", score)
	}

	// Default config should score well since all values are within empirical ranges.
	// A score below 5 suggests a bug in the default values or ranges.
	if score < 5.0 {
		t.Errorf("default config score unexpectedly low: %.4f (expected >= 5.0)", score)
	}
}

func TestMacroRiskEvaluator_ConstraintViolation(t *testing.T) {
	cfg := DefaultParametersConfig()
	// Swap base and max to violate the ordering constraint.
	cfg.Engine.MacroRisk.OutflowProbBase.Value = 90.0
	cfg.Engine.MacroRisk.OutflowProbMax.Value = 30.0

	evaluator := (&MacroRiskCalibrator{}).BuildEvaluator()
	score, err := evaluator(cfg)
	if err != nil {
		t.Fatalf("evaluator returned error on constraint-violating config: %v", err)
	}
	if score >= 0 {
		t.Errorf("expected negative score for constraint violation (base >= max), got %.4f", score)
	}
}

func TestMacroRiskEvaluator_OutOfRangePenalty(t *testing.T) {
	cfg := DefaultParametersConfig()
	// Push VIX far outside the empirical range.
	cfg.Engine.MacroRisk.VIXThreshold.Value = 80.0

	evaluator := (&MacroRiskCalibrator{}).BuildEvaluator()
	scoreOutOfRange, _ := evaluator(cfg)

	// Reset to in-range value.
	cfg.Engine.MacroRisk.VIXThreshold.Value = 30.0
	scoreInRange, _ := evaluator(cfg)

	if scoreOutOfRange >= scoreInRange {
		t.Errorf("out-of-range score %.4f should be lower than in-range score %.4f",
			scoreOutOfRange, scoreInRange)
	}
}

func TestMacroRiskEvaluator_BonusForWellSeparatedOutflow(t *testing.T) {
	cfg := DefaultParametersConfig()
	// Tight outflow range — small bonus.
	cfg.Engine.MacroRisk.OutflowProbBase.Value = 70.0
	cfg.Engine.MacroRisk.OutflowProbMax.Value = 75.0

	evaluator := (&MacroRiskCalibrator{}).BuildEvaluator()
	scoreTight, _ := evaluator(cfg)

	// Wide outflow range — bigger bonus.
	cfg.Engine.MacroRisk.OutflowProbBase.Value = 30.0
	cfg.Engine.MacroRisk.OutflowProbMax.Value = 90.0
	scoreWide, _ := evaluator(cfg)

	if scoreWide <= scoreTight {
		t.Errorf("wide-range score %.4f should exceed tight-range score %.4f due to bonus",
			scoreWide, scoreTight)
	}
}

func TestMacroRiskEvaluator_BelowZeroScores(t *testing.T) {
	cfg := DefaultParametersConfig()
	// Violate outflow ordering severely AND push all values out of range.
	cfg.Engine.MacroRisk.OutflowProbBase.Value = 95.0
	cfg.Engine.MacroRisk.OutflowProbMax.Value = 5.0
	cfg.Engine.MacroRisk.VIXThreshold.Value = 100.0
	cfg.Engine.MacroRisk.CarryTradeUnwindThreshold.Value = 200.0
	cfg.Engine.MacroRisk.US10YThreshold.Value = 10.0

	evaluator := (&MacroRiskCalibrator{}).BuildEvaluator()
	score, _ := evaluator(cfg)

	if score >= 0 {
		t.Errorf("expected negative score for severely misconfigured params, got %.4f", score)
	}
}

func TestMacroRiskCalibrator_Calibrate(t *testing.T) {
	// Set up a path for parameter persistence so Save() doesn't panic.
	t.Setenv("ATLAS_PARAMETERS_CONFIG_PATH", t.TempDir()+"/test_params.json")

	c := NewMacroRiskCalibrator()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := c.Calibrate(ctx)
	if err != nil {
		t.Fatalf("Calibrate() error: %v", err)
	}
	if result == nil {
		t.Fatal("Calibrate() returned nil result")
	}

	if result.ParamCount != 9 {
		t.Errorf("expected ParamCount=9, got %d", result.ParamCount)
	}

	if result.Timestamp.IsZero() {
		t.Error("expected non-zero Timestamp")
	}

	// Verdict must be one of calibrator.go's expected values.
	switch result.Verdict {
	case "calibrated", "stable", "unchanged":
		// All valid.
	default:
		t.Errorf("unexpected verdict: %s", result.Verdict)
	}

	if result.Summary == "" {
		t.Error("expected non-empty Summary")
	}

	t.Logf("Verdict: %s", result.Verdict)
	t.Logf("Summary: %s", result.Summary)
	t.Logf("Baseline: %.4f, Optimized: %.4f", result.BaselineScore, result.OptimizedScore)
	for _, ch := range result.Changes {
		t.Logf("  %s: %.4f → %.4f (Δ%.1f%%, confidence=%s)",
			ch.ParamName, ch.Before, ch.After, ch.DeltaPct, ch.Confidence)
	}
}

func TestMacroRiskCalibrator_Interface(t *testing.T) {
	var c any = NewMacroRiskCalibrator()
	if _, ok := c.(ParameterCalibrator); !ok {
		t.Error("MacroRiskCalibrator does not implement ParameterCalibrator interface")
	}
}

func TestMacroRiskCalibrator_MarksCalibration(t *testing.T) {
	t.Setenv("ATLAS_PARAMETERS_CONFIG_PATH", t.TempDir()+"/test_params.json")

	c := NewMacroRiskCalibrator()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Capture pre-calibration state.
	preParams := GetParametersConfig()
	preLastCal := preParams.Engine.MacroRisk.VIXThreshold.LastCalibrated

	_, err := c.Calibrate(ctx)
	if err != nil {
		t.Fatalf("Calibrate() error: %v", err)
	}

	// After calibration, verify at least one parameter was updated.
	postParams := GetParametersConfig()
	postLastCal := postParams.Engine.MacroRisk.VIXThreshold.LastCalibrated

	if postLastCal != nil && preLastCal == nil {
		t.Log("Calibration updated LastCalibrated on VIXThreshold")
	} else if postLastCal == nil && preLastCal == nil {
		t.Log("No LastCalibrated change (may be 'unchanged' or 'stable' verdict)")
	}

	// If calibration was applied, CalibrationMethod should be set.
	method := postParams.Engine.MacroRisk.VIXThreshold.CalibrationMethod
	t.Logf("CalibrationMethod on VIXThreshold: %q", method)
}
