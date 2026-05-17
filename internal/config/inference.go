package config

import (
	"fmt"
	"math"
	"sort"
)

// InferenceEngine provides parameter inference and calibration capabilities.
// It can estimate GARCH parameters from historical returns, compute VaR/ES,
// and run parameter sweep backtests to suggest optimal values.
type InferenceEngine struct {
	params *ParametersConfig
}

// NewInferenceEngine creates an inference engine with the given parameters.
func NewInferenceEngine(params *ParametersConfig) *InferenceEngine {
	if params == nil {
		params = DefaultParametersConfig()
	}
	return &InferenceEngine{params: params}
}

// WithParameters updates the inference engine's parameter config.
func (ie *InferenceEngine) WithParameters(params *ParametersConfig) *InferenceEngine {
	ie.params = params
	return ie
}

// InferredGARCH holds inferred GARCH(1,1) coefficients.
type InferredGARCH struct {
	Omega float64
	Alpha float64
	Beta  float64
}

// InferGARCH estimates GARCH(1,1) parameters from a return series using
// variance-targeting MLE. Returns omega, alpha, beta.
func (ie *InferenceEngine) InferGARCH(returns []float64) (InferredGARCH, error) {
	if len(returns) < 100 {
		return InferredGARCH{}, fmt.Errorf("insufficient data: need at least 100 returns, got %d", len(returns))
	}

	// Unconditional variance target
	var unconditionalVariance float64
	for _, r := range returns {
		unconditionalVariance += r * r
	}
	unconditionalVariance /= float64(len(returns))

	// Grid search for alpha and beta
	bestAlpha, bestBeta := 0.1, 0.85
	bestLL := math.Inf(-1)

	// Coarse grid search
	for alpha := 0.05; alpha <= 0.25; alpha += 0.05 {
		for beta := 0.70; beta <= 0.95; beta += 0.05 {
			if alpha+beta >= 0.999 {
				continue // Stationarity constraint
			}
			omega := unconditionalVariance * (1.0 - alpha - beta)
			ll := garchLogLikelihood(returns, omega, alpha, beta)
			if ll > bestLL {
				bestLL = ll
				bestAlpha = alpha
				bestBeta = beta
			}
		}
	}

	// Fine grid search around best coarse values
	for alpha := bestAlpha - 0.025; alpha <= bestAlpha+0.025; alpha += 0.005 {
		for beta := bestBeta - 0.025; beta <= bestBeta+0.025; beta += 0.005 {
			if alpha <= 0 || beta <= 0 || alpha+beta >= 0.999 {
				continue
			}
			omega := unconditionalVariance * (1.0 - alpha - beta)
			ll := garchLogLikelihood(returns, omega, alpha, beta)
			if ll > bestLL {
				bestLL = ll
				bestAlpha = alpha
				bestBeta = beta
			}
		}
	}

	omega := unconditionalVariance * (1.0 - bestAlpha - bestBeta)
	return InferredGARCH{
		Omega: omega,
		Alpha: bestAlpha,
		Beta:  bestBeta,
	}, nil
}

// garchLogLikelihood computes the log-likelihood of a GARCH(1,1) model.
func garchLogLikelihood(returns []float64, omega, alpha, beta float64) float64 {
	if omega <= 0 || alpha <= 0 || beta <= 0 || alpha+beta >= 1.0 {
		return math.Inf(-1)
	}

	// Initialize variance with unconditional variance
	unconditionalVar := omega / (1.0 - alpha - beta)
	variance := unconditionalVar
	ll := 0.0

	for _, r := range returns {
		if variance <= 0 {
			return math.Inf(-1)
		}
		ll += -0.5*math.Log(2*math.Pi*variance) - (r*r)/(2*variance)
		variance = omega + alpha*r*r + beta*variance
	}

	return ll
}

// VaRResult holds Value-at-Risk and Expected Shortfall estimates.
type VaRResult struct {
	Confidence   float64
	VaR          float64 // Negative for losses
	ES           float64 // Expected Shortfall
	Method       string
	Observations int
}

// EstimateVaR computes historical VaR and ES at the given confidence level.
func (ie *InferenceEngine) EstimateVaR(returns []float64, confidence float64) (VaRResult, error) {
	if len(returns) < 30 {
		return VaRResult{}, fmt.Errorf("insufficient data: need at least 30 returns, got %d", len(returns))
	}
	if confidence <= 0 || confidence >= 1 {
		return VaRResult{}, fmt.Errorf("confidence must be in (0,1), got %f", confidence)
	}

	// Sort returns for quantile estimation
	sorted := make([]float64, len(returns))
	copy(sorted, returns)
	sort.Float64s(sorted)

	// Historical VaR: the quantile at (1-confidence) level
	idx := max(int(math.Floor(float64(len(sorted))*(1.0-confidence))), 0)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	varValue := sorted[idx]

	// Expected Shortfall: average of returns beyond VaR
	var sumBeyond float64
	var countBeyond int
	for _, r := range sorted {
		if r <= varValue {
			sumBeyond += r
			countBeyond++
		}
	}

	es := varValue
	if countBeyond > 0 {
		es = sumBeyond / float64(countBeyond)
	}

	return VaRResult{
		Confidence:   confidence,
		VaR:          varValue,
		ES:           es,
		Method:       "historical",
		Observations: len(returns),
	}, nil
}

// ParameterSweepResult holds the outcome of a parameter sweep backtest.
type ParameterSweepResult struct {
	ParameterName  string
	ValuesTested   []float64
	Scores         []float64 // Performance metric (e.g., Sharpe ratio)
	BestValue      float64
	BestScore      float64
	CurrentValue   float64
	Recommendation string
}

// BacktestEvaluator is a function type that evaluates a parameter set.
type BacktestEvaluator func(params *ParametersConfig) (score float64, err error)

// SweepParameter runs a parameter sweep over a single parameter and returns
// the best value found.
func (ie *InferenceEngine) SweepParameter(
	paramName string,
	currentValue float64,
	values []float64,
	evaluator BacktestEvaluator,
) (ParameterSweepResult, error) {
	if len(values) == 0 {
		return ParameterSweepResult{}, fmt.Errorf("no values to test")
	}

	result := ParameterSweepResult{
		ParameterName:  paramName,
		ValuesTested:   values,
		Scores:         make([]float64, len(values)),
		CurrentValue:   currentValue,
		BestScore:      math.Inf(-1),
		Recommendation: "keep_current",
	}

	for i, v := range values {
		// Create a copy of parameters with the test value
		testParams := ie.cloneParams()
		if err := ie.setParameterOnConfig(testParams, paramName, v); err != nil {
			return ParameterSweepResult{}, fmt.Errorf("set parameter %s=%f: %w", paramName, v, err)
		}

		score, err := evaluator(testParams)
		if err != nil {
			result.Scores[i] = math.NaN()
			continue
		}
		result.Scores[i] = score

		if score > result.BestScore {
			result.BestScore = score
			result.BestValue = v
		}
	}

	// Generate recommendation
	if math.Abs(result.BestValue-currentValue) > 1e-9 {
		improvement := result.BestScore
		if !math.IsNaN(improvement) && improvement > 0 {
			pctChange := (result.BestValue - currentValue) / currentValue * 100
			result.Recommendation = fmt.Sprintf("increase_%s_to_%.4f (%.1f%% change, score=%.4f)",
				paramName, result.BestValue, pctChange, result.BestScore)
			if pctChange < 0 {
				result.Recommendation = fmt.Sprintf("decrease_%s_to_%.4f (%.1f%% change, score=%.4f)",
					paramName, result.BestValue, -pctChange, result.BestScore)
			}
		}
	}

	return result, nil
}

// cloneParams creates a deep copy of the current parameters.
func (ie *InferenceEngine) cloneParams() *ParametersConfig {
	// Simple approach: marshal to JSON and back
	// For a production system, implement proper deep copy
	cfg := DefaultParametersConfig()
	// Copy current values
	*cfg = *ie.params
	return cfg
}

// SetParameter sets a single parameter by name on the engine's config.
func (ie *InferenceEngine) SetParameter(name string, value float64) error {
	return ie.setParameterOnConfig(ie.params, name, value)
}

// setParameterOnConfig sets a single parameter by name on the given config.
func (ie *InferenceEngine) setParameterOnConfig(cfg *ParametersConfig, name string, value float64) error {
	switch name {
	// Darwinian parameters
	case "darwinian_weight_min":
		cfg.Darwinian.WeightMin.Value = value
	case "darwinian_weight_max":
		cfg.Darwinian.WeightMax.Value = value
	case "darwinian_top_quartile_multiplier":
		cfg.Darwinian.TopQuartileMultiplier.Value = value
	case "darwinian_bottom_quartile_multiplier":
		cfg.Darwinian.BottomQuartileMultiplier.Value = value
	case "darwinian_em_alpha":
		cfg.Darwinian.EMAAlpha.Value = value
	case "darwinian_max_performance_bonus_pct":
		cfg.Darwinian.MaxPerformanceBonusPct.Value = value

	// Factor parameters
	case "factor_momentum_stddev_divisor":
		cfg.Factor.MomentumStdDevDivisor.Value = value
	case "factor_momentum_intraday_discount":
		cfg.Factor.MomentumIntradayDiscount.Value = value
	case "factor_value_pe_range_center":
		cfg.Factor.ValuePERangeCenter.Value = value
	case "factor_quality_dividend_yield_cap":
		cfg.Factor.QualityDividendYieldCap.Value = value

	// Sizing parameters
	case "sizing_kelly_fraction":
		cfg.Sizing.KellyFraction.Value = value
	case "sizing_max_position_by_adv":
		cfg.Sizing.MaxPositionByADV.Value = value
	case "sizing_atr_multiplier":
		cfg.Sizing.ATRMultiplier.Value = value
	case "sizing_correlation_threshold":
		cfg.Sizing.CorrelationThreshold.Value = value
	case "sizing_target_volatility":
		cfg.Sizing.TargetVolatility.Value = value

	// Health parameters
	case "health_mute_threshold":
		cfg.Health.MuteThreshold.Value = int(value)
	case "health_negative_sharpe_threshold":
		cfg.Health.NegativeSharpeThreshold.Value = value
	case "health_sharpe_weight":
		cfg.Health.SharpeWeight.Value = value
	case "health_hitrate_weight":
		cfg.Health.HitRateWeight.Value = value

	// GARCH parameters
	case "garch_omega":
		cfg.GARCH.Omega.Value = value
	case "garch_alpha":
		cfg.GARCH.Alpha.Value = value
	case "garch_beta":
		cfg.GARCH.Beta.Value = value

	// Experiment parameters
	case "experiment_improvement_threshold":
		cfg.Experiment.ImprovementThreshold.Value = value
	case "experiment_welch_ttest_threshold":
		cfg.Experiment.WelchTTestThreshold.Value = value

	// Baseline parameters
	case "baseline_max_position_weight":
		cfg.Baseline.MaxPositionWeight.Value = value
	case "baseline_reserve_cash_fraction":
		cfg.Baseline.ReserveCashFraction.Value = value
	case "baseline_transaction_cost_bps":
		cfg.Baseline.TransactionCostBPS.Value = value
	case "baseline_slippage_bps":
		cfg.Baseline.SlippageBPS.Value = value

	default:
		return fmt.Errorf("unknown parameter: %s", name)
	}
	return nil
}

// CalibrateGARCH updates the GARCH parameters in the config based on
// historical returns inference.
func (ie *InferenceEngine) CalibrateGARCH(returns []float64) error {
	garch, err := ie.InferGARCH(returns)
	if err != nil {
		return fmt.Errorf("infer GARCH: %w", err)
	}

	ie.params.GARCH.Omega.Value = garch.Omega
	ie.params.GARCH.Alpha.Value = garch.Alpha
	ie.params.GARCH.Beta.Value = garch.Beta

	return nil
}

// CalibrateVaR updates volatility-related parameters based on VaR estimates.
func (ie *InferenceEngine) CalibrateVaR(returns []float64) error {
	var95, err := ie.EstimateVaR(returns, 0.95)
	if err != nil {
		return fmt.Errorf("estimate 95%% VaR: %w", err)
	}

	var99, err := ie.EstimateVaR(returns, 0.99)
	if err != nil {
		return fmt.Errorf("estimate 99%% VaR: %w", err)
	}

	// Adjust max drawdown limit based on empirical VaR
	// Use 99% VaR as a conservative estimate
	empiricalMaxDD := math.Abs(var99.VaR)
	if empiricalMaxDD > 0 {
		// Add 20% buffer to empirical VaR
		suggestedMaxDD := empiricalMaxDD * 1.2
		ie.params.Sizing.MaxDrawdownLimit.Value = math.Min(suggestedMaxDD, 0.20)
	}

	// Adjust target volatility based on 95% VaR
	// VaR ≈ z_score * volatility, so volatility ≈ VaR / z_score
	// For 95%, z_score ≈ 1.645
	if var95.VaR != 0 {
		empiricalVol := math.Abs(var95.VaR) / 1.645
		ie.params.Sizing.TargetVolatility.Value = empiricalVol
	}

	return nil
}
