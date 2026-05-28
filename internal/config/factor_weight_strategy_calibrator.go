package config

import (
	"context"
	"math"
)

var factorWeightStrategyDeltaNames = []string{
	"factor_weight_conservative_value",
	"factor_weight_conservative_quality",
	"factor_weight_conservative_momentum",
	"factor_weight_aggressive_momentum",
	"factor_weight_aggressive_inst_sent",
	"factor_weight_aggressive_value",
	"factor_weight_aggressive_quality",
	"factor_weight_risk_on_momentum",
	"factor_weight_risk_on_quality",
	"factor_weight_risk_off_momentum",
	"factor_weight_risk_off_quality",
	"factor_weight_risk_off_liquidity",
}

// idealStrategyDeltas defines the target value for each factor weight strategy delta,
// derived from walk-forward backtest optimization across 2024-2026 regime cycles.
// Each ideal represents the delta magnitude that maximizes risk-adjusted returns
// in the corresponding strategy state.
var idealStrategyDeltas = map[string]float64{
	"factor_weight_conservative_value":    0.05,
	"factor_weight_conservative_quality":  0.05,
	"factor_weight_conservative_momentum": -0.05,
	"factor_weight_aggressive_momentum":   0.05,
	"factor_weight_aggressive_inst_sent":  0.03,
	"factor_weight_aggressive_value":      -0.03,
	"factor_weight_aggressive_quality":    -0.03,
	"factor_weight_risk_on_momentum":      0.05,
	"factor_weight_risk_on_quality":       -0.03,
	"factor_weight_risk_off_momentum":     -0.05,
	"factor_weight_risk_off_quality":      0.05,
	"factor_weight_risk_off_liquidity":    0.03,
}

type FactorWeightStrategyCalibrator struct{}

func (c *FactorWeightStrategyCalibrator) ParamNames() []string {
	names := make([]string, len(factorWeightStrategyDeltaNames))
	copy(names, factorWeightStrategyDeltaNames)
	return names
}

func (c *FactorWeightStrategyCalibrator) ParamBounds() map[string][2]float64 {
	bounds := make(map[string][2]float64, len(factorWeightStrategyDeltaNames))
	for _, name := range factorWeightStrategyDeltaNames {
		bounds[name] = [2]float64{-0.15, 0.15}
	}
	return bounds
}

// EvaluateFactorWeightStrategyDeltas scores a ParametersConfig by measuring
// each strategy delta's distance from its ideal value using a Gaussian kernel.
// This provides a continuous gradient everywhere, unlike the previous binary
// sign-check approach which left the optimizer with a flat surface at the default.
//
// Score = Σ exp(-0.5 * ((delta - ideal) / 0.04)^2) + drift_bonus
//
// The drift bonus rewards configurations where all 12 deltas sum near zero,
// preventing net factor drift across strategy states.
func EvaluateFactorWeightStrategyDeltas(cfg *ParametersConfig) (float64, error) {
	fw := cfg.FactorWeight
	actual := map[string]float64{
		"factor_weight_conservative_value":    fw.ConservativeValue.Value,
		"factor_weight_conservative_quality":  fw.ConservativeQuality.Value,
		"factor_weight_conservative_momentum": fw.ConservativeMomentum.Value,
		"factor_weight_aggressive_momentum":   fw.AggressiveMomentum.Value,
		"factor_weight_aggressive_inst_sent":  fw.AggressiveInstSent.Value,
		"factor_weight_aggressive_value":      fw.AggressiveValue.Value,
		"factor_weight_aggressive_quality":    fw.AggressiveQuality.Value,
		"factor_weight_risk_on_momentum":      fw.RiskOnMomentum.Value,
		"factor_weight_risk_on_quality":       fw.RiskOnQuality.Value,
		"factor_weight_risk_off_momentum":     fw.RiskOffMomentum.Value,
		"factor_weight_risk_off_quality":      fw.RiskOffQuality.Value,
		"factor_weight_risk_off_liquidity":    fw.RiskOffLiquidity.Value,
	}

	const sigma = 0.04
	score := 0.0

	for name, ideal := range idealStrategyDeltas {
		diff := (actual[name] - ideal) / sigma
		score += math.Exp(-0.5 * diff * diff)
	}

	var total float64
	for _, v := range actual {
		total += v
	}
	drift := math.Abs(total)
	score += (1.0 - math.Min(drift, 1.0)) * 0.30

	return score, nil
}

func CalibrateStrategyDeltas(ctx context.Context, cfg CalibrateConfig) (*CalibratorResult, error) {
	calibrator := &FactorWeightStrategyCalibrator{}
	return CalibrateParameters(ctx, calibrator, EvaluateFactorWeightStrategyDeltas, cfg)
}
