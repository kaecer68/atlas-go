package config

import (
	"context"
	"math"
	"testing"
)

func TestFactorWeightStrategyCalibrator_ParamNames(t *testing.T) {
	c := &FactorWeightStrategyCalibrator{}
	names := c.ParamNames()

	if len(names) != 12 {
		t.Errorf("ParamNames() returned %d names, want 12", len(names))
	}

	expected := map[string]bool{
		"factor_weight_conservative_value":    true,
		"factor_weight_conservative_quality":  true,
		"factor_weight_conservative_momentum": true,
		"factor_weight_aggressive_momentum":   true,
		"factor_weight_aggressive_inst_sent":  true,
		"factor_weight_aggressive_value":      true,
		"factor_weight_aggressive_quality":    true,
		"factor_weight_risk_on_momentum":      true,
		"factor_weight_risk_on_quality":       true,
		"factor_weight_risk_off_momentum":     true,
		"factor_weight_risk_off_quality":      true,
		"factor_weight_risk_off_liquidity":    true,
	}

	for _, n := range names {
		if !expected[n] {
			t.Errorf("unexpected param name: %s", n)
		}
		delete(expected, n)
	}
	for n := range expected {
		t.Errorf("missing param name: %s", n)
	}

	if got := c.ParamNames(); len(got) == 12 && &got[0] == &names[0] {
		t.Error("ParamNames() returned same backing array; should return a copy")
	}
}

func TestEvaluateFactorWeightStrategyDeltas_Defaults(t *testing.T) {
	cfg := DefaultParametersConfig()
	got, err := EvaluateFactorWeightStrategyDeltas(cfg)
	if err != nil {
		t.Fatalf("EvaluateFactorWeightStrategyDeltas(defaults): %v", err)
	}

	if got <= 0 {
		t.Errorf("default config score should be positive, got %.4f", got)
	}
	if got > 1.5 {
		t.Errorf("default config score unexpectedly high: %.4f", got)
	}
}

func TestEvaluateFactorWeightStrategyDeltas_AllViolations(t *testing.T) {
	cfg := DefaultParametersConfig()

	cfg.FactorWeight.ConservativeValue.Value = 0.20
	cfg.FactorWeight.AggressiveValue.Value = 0.10
	cfg.FactorWeight.ConservativeQuality.Value = 0.20
	cfg.FactorWeight.AggressiveQuality.Value = 0.10
	cfg.FactorWeight.ConservativeMomentum.Value = 0.20
	cfg.FactorWeight.AggressiveMomentum.Value = 0.10
	cfg.FactorWeight.RiskOnMomentum.Value = 0.20
	cfg.FactorWeight.RiskOnQuality.Value = 0.20
	cfg.FactorWeight.RiskOffMomentum.Value = 0.20
	cfg.FactorWeight.RiskOffQuality.Value = 0.20
	cfg.FactorWeight.AggressiveInstSent.Value = 0.20
	cfg.FactorWeight.RiskOffLiquidity.Value = 0.20

	got, err := EvaluateFactorWeightStrategyDeltas(cfg)
	if err != nil {
		t.Fatalf("EvaluateFactorWeightStrategyDeltas(violations): %v", err)
	}

	if got >= 0 {
		t.Errorf("all-violations config should score negative, got %.4f", got)
	}
}

func TestEvaluateFactorWeightStrategyDeltas_Perfect(t *testing.T) {
	cfg := DefaultParametersConfig()

	cfg.FactorWeight.ConservativeValue.Value = 0.05
	cfg.FactorWeight.AggressiveValue.Value = -0.03
	cfg.FactorWeight.ConservativeQuality.Value = 0.05
	cfg.FactorWeight.AggressiveQuality.Value = -0.03
	cfg.FactorWeight.ConservativeMomentum.Value = -0.05
	cfg.FactorWeight.AggressiveMomentum.Value = 0.05
	cfg.FactorWeight.RiskOnMomentum.Value = 0.05
	cfg.FactorWeight.RiskOffMomentum.Value = -0.05
	cfg.FactorWeight.RiskOnQuality.Value = -0.03
	cfg.FactorWeight.RiskOffQuality.Value = 0.05
	cfg.FactorWeight.AggressiveInstSent.Value = 0.03
	cfg.FactorWeight.RiskOffLiquidity.Value = 0.03

	got, err := EvaluateFactorWeightStrategyDeltas(cfg)
	if err != nil {
		t.Fatalf("EvaluateFactorWeightStrategyDeltas(perfect): %v", err)
	}

	got2, err2 := EvaluateFactorWeightStrategyDeltas(cfg)
	if err2 != nil {
		t.Fatalf("repeat eval: %v", err2)
	}
	if math.Abs(got-got2) > 1e-9 {
		t.Errorf("evaluator is not deterministic: %.6f vs %.6f", got, got2)
	}
}

func TestEvaluateFactorWeightStrategyDeltas_DriftPenalty(t *testing.T) {
	cfg := DefaultParametersConfig()

	// All deltas positive → large drift
	cfg.FactorWeight.ConservativeValue.Value = 0.10
	cfg.FactorWeight.ConservativeQuality.Value = 0.10
	cfg.FactorWeight.ConservativeMomentum.Value = 0.10
	cfg.FactorWeight.AggressiveMomentum.Value = 0.10
	cfg.FactorWeight.AggressiveInstSent.Value = 0.10
	cfg.FactorWeight.AggressiveValue.Value = 0.10
	cfg.FactorWeight.AggressiveQuality.Value = 0.10
	cfg.FactorWeight.RiskOnMomentum.Value = 0.10
	cfg.FactorWeight.RiskOnQuality.Value = 0.10
	cfg.FactorWeight.RiskOffMomentum.Value = 0.10
	cfg.FactorWeight.RiskOffQuality.Value = 0.10
	cfg.FactorWeight.RiskOffLiquidity.Value = 0.10

	gotLarge, err := EvaluateFactorWeightStrategyDeltas(cfg)
	if err != nil {
		t.Fatalf("EvaluateFactorWeightStrategyDeltas(large drift): %v", err)
	}

	// Near-zero drift with same other properties
	cfg2 := DefaultParametersConfig()
	cfg2.FactorWeight.ConservativeValue.Value = 0.06
	cfg2.FactorWeight.AggressiveValue.Value = -0.06
	cfg2.FactorWeight.ConservativeQuality.Value = 0.05
	cfg2.FactorWeight.AggressiveQuality.Value = -0.05
	cfg2.FactorWeight.ConservativeMomentum.Value = -0.04
	cfg2.FactorWeight.AggressiveMomentum.Value = 0.04
	cfg2.FactorWeight.RiskOnMomentum.Value = 0.05
	cfg2.FactorWeight.RiskOffMomentum.Value = -0.05
	cfg2.FactorWeight.RiskOnQuality.Value = -0.03
	cfg2.FactorWeight.RiskOffQuality.Value = 0.03
	cfg2.FactorWeight.AggressiveInstSent.Value = 0.03
	cfg2.FactorWeight.RiskOffLiquidity.Value = -0.03

	gotSmall, err := EvaluateFactorWeightStrategyDeltas(cfg2)
	if err != nil {
		t.Fatalf("EvaluateFactorWeightStrategyDeltas(small drift): %v", err)
	}

	if gotLarge >= gotSmall {
		t.Errorf("large-drift config (%0.4f) should score lower than near-zero-drift (%0.4f)", gotLarge, gotSmall)
	}
}

func TestCalibrateStrategyDeltas_EndToEnd(t *testing.T) {
	ResetParametersConfig()
	params := GetParametersConfig()

	before, err := EvaluateFactorWeightStrategyDeltas(params)
	if err != nil {
		t.Fatalf("pre-calibration eval: %v", err)
	}

	ctx := context.Background()
	calCfg := CalibrateConfig{
		InitialPoints:  6,
		Iterations:     10,
		MinImprovement: 0.02,
	}

	result, err := CalibrateStrategyDeltas(ctx, calCfg)
	if err != nil {
		t.Fatalf("CalibrateStrategyDeltas: %v", err)
	}

	if result.ParamCount != 12 {
		t.Errorf("expected 12 params, got %d", result.ParamCount)
	}
	if result.BaselineScore <= 0 {
		t.Errorf("baseline score should be positive: %.4f", result.BaselineScore)
	}
	if result.Verdict == "" {
		t.Error("expected non-empty verdict")
	}

	after, err := EvaluateFactorWeightStrategyDeltas(params)
	if err != nil {
		t.Fatalf("post-calibration eval: %v", err)
	}

	if result.Verdict == "calibrated" && after <= before {
		t.Errorf("calibrated config did not improve: before=%.4f after=%.4f", before, after)
	}
}
