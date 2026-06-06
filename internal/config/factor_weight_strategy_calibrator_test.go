package config

import (
	"context"
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

func TestFactorWeightStrategyCalibrator_ParamBounds(t *testing.T) {
	c := &FactorWeightStrategyCalibrator{}
	bounds := c.ParamBounds()

	if len(bounds) != 12 {
		t.Errorf("ParamBounds() returned %d entries, want 12", len(bounds))
	}
	for name, b := range bounds {
		if b[0] != -0.15 || b[1] != 0.15 {
			t.Errorf("ParamBounds()[%s] = [%.2f, %.2f], want [-0.15, 0.15]", name, b[0], b[1])
		}
	}
}

func TestEvaluateFactorWeightStrategyDeltas_Defaults(t *testing.T) {
	cfg := DefaultParametersConfig()
	got, err := EvaluateFactorWeightStrategyDeltas(cfg)
	if err != nil {
		t.Fatalf("EvaluateFactorWeightStrategyDeltas(defaults): %v", err)
	}

	// Defaults are near ideals — score should be high (all 12 Gaussian components near 1.0 + drift bonus)
	if got < 10.0 {
		t.Errorf("default config score too low: %.4f (expected > 10, near-ideal values)", got)
	}
	if got > 13.0 {
		t.Errorf("default config score unexpectedly high: %.4f", got)
	}
}

func TestEvaluateFactorWeightStrategyDeltas_FarFromIdeals(t *testing.T) {
	cfg := DefaultParametersConfig()

	// Push all deltas far from their ideals
	cfg.FactorWeight.ConservativeValue.Value = 0.20
	cfg.FactorWeight.AggressiveValue.Value = 0.15
	cfg.FactorWeight.ConservativeQuality.Value = 0.18
	cfg.FactorWeight.AggressiveQuality.Value = 0.15
	cfg.FactorWeight.ConservativeMomentum.Value = 0.20
	cfg.FactorWeight.AggressiveMomentum.Value = 0.15
	cfg.FactorWeight.RiskOnMomentum.Value = 0.20
	cfg.FactorWeight.RiskOnQuality.Value = 0.18
	cfg.FactorWeight.RiskOffMomentum.Value = 0.15
	cfg.FactorWeight.RiskOffQuality.Value = 0.15
	cfg.FactorWeight.AggressiveInstSent.Value = 0.20
	cfg.FactorWeight.RiskOffLiquidity.Value = 0.20

	gotBad, err := EvaluateFactorWeightStrategyDeltas(cfg)
	if err != nil {
		t.Fatalf("EvaluateFactorWeightStrategyDeltas(far): %v", err)
	}

	def, _ := EvaluateFactorWeightStrategyDeltas(DefaultParametersConfig())
	if gotBad >= def {
		t.Errorf("far-from-ideal config score (%.4f) should be lower than default (%.4f)", gotBad, def)
	}
}

func TestEvaluateFactorWeightStrategyDeltas_Perfect(t *testing.T) {
	cfg := DefaultParametersConfig()

	// Set all values exactly at their ideals
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

	// Perfect = all Gaussian components at 1.0 + drift bonus
	drift := 0.05 + (-0.03) + 0.05 + (-0.03) + (-0.05) + 0.05 + 0.05 + (-0.05) + (-0.03) + 0.05 + 0.03 + 0.03
	if drift < 0 {
		drift = -drift
	}
	expectedMax := 12.0 + (1.0-drift)*0.30
	if got > expectedMax+0.01 {
		t.Errorf("perfect score %.4f exceeds theoretical max %.4f", got, expectedMax)
	}

	// Determinism check
	got2, _ := EvaluateFactorWeightStrategyDeltas(cfg)
	if got != got2 {
		t.Errorf("evaluator is not deterministic: %.6f vs %.6f", got, got2)
	}
}

func TestEvaluateFactorWeightStrategyDeltas_DriftPenalty(t *testing.T) {
	cfg := DefaultParametersConfig()

	// All deltas positive → large drift (1.20 total)
	for _, field := range []*float64{
		&cfg.FactorWeight.ConservativeValue.Value,
		&cfg.FactorWeight.ConservativeQuality.Value,
		&cfg.FactorWeight.ConservativeMomentum.Value,
		&cfg.FactorWeight.AggressiveMomentum.Value,
		&cfg.FactorWeight.AggressiveInstSent.Value,
		&cfg.FactorWeight.AggressiveValue.Value,
		&cfg.FactorWeight.AggressiveQuality.Value,
		&cfg.FactorWeight.RiskOnMomentum.Value,
		&cfg.FactorWeight.RiskOnQuality.Value,
		&cfg.FactorWeight.RiskOffMomentum.Value,
		&cfg.FactorWeight.RiskOffQuality.Value,
		&cfg.FactorWeight.RiskOffLiquidity.Value,
	} {
		*field = 0.10
	}

	gotLarge, err := EvaluateFactorWeightStrategyDeltas(cfg)
	if err != nil {
		t.Fatalf("EvaluateFactorWeightStrategyDeltas(large drift): %v", err)
	}

	// Near-zero drift but same Gaussian distances
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
		t.Errorf("large-drift config (%.4f) should score lower than near-zero-drift (%.4f)", gotLarge, gotSmall)
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

	// With tight bounds and near-ideal defaults, optimizer should either
	// improve or report stability. Degradation is acceptable with 12D/16-eval budget.
	if after < before*0.5 {
		t.Errorf("calibrated config severely degraded: before=%.4f after=%.4f", before, after)
	}
}
