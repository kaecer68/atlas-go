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

// FactorWeightStrategyCalibrator calibrates the 12 strategy delta parameters
// that adjust factor weights based on strategy state (conservative/aggressive/risk-on/risk-off).
type FactorWeightStrategyCalibrator struct{}

// ParamNames returns the 12 factor_weight strategy delta parameter names.
func (c *FactorWeightStrategyCalibrator) ParamNames() []string {
	names := make([]string, len(factorWeightStrategyDeltaNames))
	copy(names, factorWeightStrategyDeltaNames)
	return names
}

// EvaluateFactorWeightStrategyDeltas scores a ParametersConfig based on how well
// the factor weight strategy deltas satisfy logical constraints:
//
//   - Conservative moves opposite-sign from aggressive: +score per matched pair
//   - Risk-on/risk-off moves opposite-direction: +score per matched pair
//   - All 12 deltas sum near-zero (no net factor drift): +score
//   - Any delta outside [-0.15, 0.15]: -score penalty
func EvaluateFactorWeightStrategyDeltas(cfg *ParametersConfig) (float64, error) {
	score := 0.0

	fw := cfg.FactorWeight

	cValue := fw.ConservativeValue.Value
	aValue := fw.AggressiveValue.Value
	cQuality := fw.ConservativeQuality.Value
	aQuality := fw.AggressiveQuality.Value
	cMomentum := fw.ConservativeMomentum.Value
	aMomentum := fw.AggressiveMomentum.Value

	roMomentum := fw.RiskOnMomentum.Value
	rfMomentum := fw.RiskOffMomentum.Value
	roQuality := fw.RiskOnQuality.Value
	rfQuality := fw.RiskOffQuality.Value

	all := []float64{
		cValue, cQuality, cMomentum,
		aMomentum, fw.AggressiveInstSent.Value, aValue, aQuality,
		roMomentum, roQuality,
		rfMomentum, rfQuality, fw.RiskOffLiquidity.Value,
	}

	// 1. Conservative/aggressive opposite-sign pairs (+0.15 each)
	if cValue > 0 && aValue < 0 {
		score += 0.15
	}
	if cQuality > 0 && aQuality < 0 {
		score += 0.15
	}
	if cMomentum < 0 && aMomentum > 0 {
		score += 0.15
	}

	// 2. Risk-on/risk-off opposite-direction pairs (+0.10 each)
	if roMomentum > 0 && rfMomentum < 0 {
		score += 0.10
	}
	if roQuality < 0 && rfQuality > 0 {
		score += 0.10
	}

	// 3. Drift: all deltas should sum near zero (+0.20 for zero drift)
	total := 0.0
	for _, v := range all {
		total += v
	}
	drift := math.Abs(total)
	score += (1.0 - math.Min(drift, 1.0)) * 0.20

	// 4. Range penalty: -0.10 for each delta outside [-0.15, 0.15]
	for _, v := range all {
		if v < -0.15 || v > 0.15 {
			score -= 0.10
		}
	}

	return score, nil
}

// CalibrateStrategyDeltas runs Bayesian optimization on the 12 factor_weight strategy
// delta parameters using the provided configuration.
func CalibrateStrategyDeltas(ctx context.Context, cfg CalibrateConfig) (*CalibratorResult, error) {
	calibrator := &FactorWeightStrategyCalibrator{}
	return CalibrateParameters(ctx, calibrator, EvaluateFactorWeightStrategyDeltas, cfg)
}
