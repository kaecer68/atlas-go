package config

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

// configAccessMu serializes mutations to the shared ParametersConfig singleton.
// SelfCalibrate callers create per-call InferenceEngines but all share the same
// underlying *ParametersConfig (via GetParametersConfig). Without this lock,
// concurrent OptimizeBayesian (which clones via cloneParams) races against
// applyCalibrationChange (which writes via SetParameter).
var configAccessMu sync.RWMutex

// InferenceEngine provides parameter inference and calibration capabilities.
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

// Parameters returns the current parameter config held by the engine.
func (ie *InferenceEngine) Parameters() *ParametersConfig {
	return ie.params
}

// InferredGARCH holds inferred GARCH(1,1) coefficients.
type InferredGARCH struct {
	Omega float64
	Alpha float64
	Beta  float64
}

// InferGARCH estimates GARCH(1,1) parameters from a return series.
func (ie *InferenceEngine) InferGARCH(returns []float64) (InferredGARCH, error) {
	if len(returns) < 100 {
		return InferredGARCH{}, fmt.Errorf("insufficient data: need at least 100 returns, got %d", len(returns))
	}

	var unconditionalVariance float64
	for _, r := range returns {
		unconditionalVariance += r * r
	}
	unconditionalVariance /= float64(len(returns))

	bestAlpha, bestBeta := 0.1, 0.85
	bestLL := math.Inf(-1)

	for alpha := 0.05; alpha <= 0.25; alpha += 0.05 {
		for beta := 0.70; beta <= 0.95; beta += 0.05 {
			if alpha+beta >= 0.999 {
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
	return InferredGARCH{Omega: omega, Alpha: bestAlpha, Beta: bestBeta}, nil
}

func garchLogLikelihood(returns []float64, omega, alpha, beta float64) float64 {
	if omega <= 0 || alpha <= 0 || beta <= 0 || alpha+beta >= 1.0 {
		return math.Inf(-1)
	}
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
	VaR          float64
	ES           float64
	Method       string
	Observations int
}

// // EstimateVaR uses the same percentile formula as risk.CalculateVaR
// but a different estimand: this is for model calibration (30 obs minimum,
// embedded ES in VaRResult), while risk.CalculateVaR is for production
// monitoring (252 obs minimum, separate CalculateCVaR). Do not unify —
// the different gates serve different purposes (#1265 canonical metric source).
// EstimateVaR computes historical VaR and ES at the given confidence level.
func (ie *InferenceEngine) EstimateVaR(returns []float64, confidence float64) (VaRResult, error) {
	if len(returns) < 30 {
		return VaRResult{}, fmt.Errorf("insufficient data: need at least 30 returns, got %d", len(returns))
	}
	if confidence <= 0 || confidence >= 1 {
		return VaRResult{}, fmt.Errorf("confidence must be in (0,1), got %f", confidence)
	}

	sorted := make([]float64, len(returns))
	copy(sorted, returns)
	sort.Float64s(sorted)

	idx := max(int(math.Floor(float64(len(sorted))*(1.0-confidence))), 0)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	varValue := sorted[idx]

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
	Scores         []float64
	BestValue      float64
	BestScore      float64
	CurrentValue   float64
	Recommendation string
}

// BacktestEvaluator is a function type that evaluates a parameter set.
type BacktestEvaluator func(params *ParametersConfig) (score float64, err error)

// SweepParameter runs a parameter sweep over a single parameter.
func (ie *InferenceEngine) SweepParameter(paramName string, currentValue float64, values []float64, evaluator BacktestEvaluator) (ParameterSweepResult, error) {
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

func (ie *InferenceEngine) cloneParams() *ParametersConfig {
	configAccessMu.RLock()
	defer configAccessMu.RUnlock()
	cfg := DefaultParametersConfig()
	*cfg = *ie.params
	return cfg
}

// SetParameter sets a single parameter by name on the engine's config.
// For map parameters, use dot notation: "factor_institutional_sentiment_weights_foreign"
func (ie *InferenceEngine) SetParameter(name string, value float64) error {
	configAccessMu.Lock()
	defer configAccessMu.Unlock()
	return ie.setParameterOnConfig(ie.params, name, value)
}

func (ie *InferenceEngine) SetStringParameter(name string, value string) error {
	configAccessMu.Lock()
	defer configAccessMu.Unlock()
	return ie.setStringParameterOnConfig(ie.params, name, value)
}

func (ie *InferenceEngine) setStringParameterOnConfig(cfg *ParametersConfig, name string, value string) error {
	if accessor, ok := stringParameterTable[name]; ok {
		accessor.set(cfg, value)
		return nil
	}
	return fmt.Errorf("unknown string parameter: %s", name)
}

// SetMapParameter performs a bulk replacement of a map[string]float64 parameter
// using the mapParamPrefixes table. Each key in the map is set as a sub-key.
func (ie *InferenceEngine) SetMapParameter(prefix string, value map[string]float64) error {
	configAccessMu.Lock()
	defer configAccessMu.Unlock()
	for _, mp := range mapParamPrefixes {
		if mp.prefix == prefix {
			mp.setMap(ie.params, value)
			return nil
		}
	}
	return fmt.Errorf("unknown map parameter prefix: %s", prefix)
}

func (ie *InferenceEngine) SetBoolParameter(name string, value bool) error {
	configAccessMu.Lock()
	defer configAccessMu.Unlock()
	if accessor, ok := boolParameterTable[name]; ok {
		accessor.set(ie.params, value)
		return nil
	}
	return fmt.Errorf("unknown bool parameter: %s", name)
}

// GetParameter retrieves the current value of a parameter by name.
// Returns the value and true if found, or 0 and false if not found.
func (ie *InferenceEngine) GetParameter(name string) (float64, bool) {
	configAccessMu.RLock()
	defer configAccessMu.RUnlock()
	return ie.getParameterFromConfig(ie.params, name)
}

// getParameterFromConfig retrieves a parameter value from the given config.
func (ie *InferenceEngine) getParameterFromConfig(cfg *ParametersConfig, name string) (float64, bool) {
	if accessor, ok := parameterTable[name]; ok {
		return accessor.get(cfg), true
	}
	if val := ie.handleMapGetParameter(cfg, name); val != nil {
		return *val, true
	}
	return 0, false
}

// handleMapGetParameter handles getting map sub-keys via dot notation.
func (ie *InferenceEngine) handleMapGetParameter(cfg *ParametersConfig, name string) *float64 {
	// Factor.InstitutionalSentimentWeights
	if strings.HasPrefix(name, "factor_institutional_sentiment_weights_") {
		key := strings.TrimPrefix(name, "factor_institutional_sentiment_weights_")
		if cfg.Factor.InstitutionalSentimentWeights.Value != nil {
			if v, ok := cfg.Factor.InstitutionalSentimentWeights.Value[key]; ok {
				return &v
			}
		}
		return nil
	}
	// Optimizer.FactorWeights
	if strings.HasPrefix(name, "optimizer_factor_weights_") {
		key := strings.TrimPrefix(name, "optimizer_factor_weights_")
		if cfg.Optimizer.FactorWeights.Value != nil {
			if v, ok := cfg.Optimizer.FactorWeights.Value[key]; ok {
				return &v
			}
		}
		return nil
	}
	// Risk.SectorConstraintsRiskOff
	if strings.HasPrefix(name, "risk_sector_constraints_risk_off_") {
		key := strings.TrimPrefix(name, "risk_sector_constraints_risk_off_")
		if cfg.Risk.SectorConstraintsRiskOff.Value != nil {
			if v, ok := cfg.Risk.SectorConstraintsRiskOff.Value[key]; ok {
				return &v
			}
		}
		return nil
	}
	// Risk.SectorConstraintsCarryTrade
	if strings.HasPrefix(name, "risk_sector_constraints_carry_trade_") {
		key := strings.TrimPrefix(name, "risk_sector_constraints_carry_trade_")
		if cfg.Risk.SectorConstraintsCarryTrade.Value != nil {
			if v, ok := cfg.Risk.SectorConstraintsCarryTrade.Value[key]; ok {
				return &v
			}
		}
		return nil
	}
	// Risk.SectorConstraintsSectorRotation
	if strings.HasPrefix(name, "risk_sector_constraints_sector_rotation_") {
		key := strings.TrimPrefix(name, "risk_sector_constraints_sector_rotation_")
		if cfg.Risk.SectorConstraintsSectorRotation.Value != nil {
			if v, ok := cfg.Risk.SectorConstraintsSectorRotation.Value[key]; ok {
				return &v
			}
		}
		return nil
	}
	// Narrative.EventTTLMultiplier
	if strings.HasPrefix(name, "narrative_event_ttl_multiplier_") {
		key := strings.TrimPrefix(name, "narrative_event_ttl_multiplier_")
		if cfg.Narrative.EventTTLMultiplier.Value != nil {
			if v, ok := cfg.Narrative.EventTTLMultiplier.Value[key]; ok {
				return &v
			}
		}
		return nil
	}
	// Industry.SectorWeights
	// DEPRECATED: industry_sector_weights_* parameter paths removed in favor of
	// sector_allocation.base_weights (see internal/sectorallocation). Returns nil
	// for backward compatibility; callers should query the sectorallocation module.
	// Drawdown.SectorConstraintsRiskOff
	if strings.HasPrefix(name, "drawdown_sector_constraints_risk_off_") {
		key := strings.TrimPrefix(name, "drawdown_sector_constraints_risk_off_")
		if cfg.Drawdown.SectorConstraintsRiskOff.Value != nil {
			if v, ok := cfg.Drawdown.SectorConstraintsRiskOff.Value[key]; ok {
				return &v
			}
		}
		return nil
	}
	// Drawdown.SectorConstraintsCarryTradeUnwind
	if strings.HasPrefix(name, "drawdown_sector_constraints_carry_trade_unwind_") {
		key := strings.TrimPrefix(name, "drawdown_sector_constraints_carry_trade_unwind_")
		if cfg.Drawdown.SectorConstraintsCarryTradeUnwind.Value != nil {
			if v, ok := cfg.Drawdown.SectorConstraintsCarryTradeUnwind.Value[key]; ok {
				return &v
			}
		}
		return nil
	}
	// Drawdown.SectorConstraintsSectorRotation
	if strings.HasPrefix(name, "drawdown_sector_constraints_sector_rotation_") {
		key := strings.TrimPrefix(name, "drawdown_sector_constraints_sector_rotation_")
		if cfg.Drawdown.SectorConstraintsSectorRotation.Value != nil {
			if v, ok := cfg.Drawdown.SectorConstraintsSectorRotation.Value[key]; ok {
				return &v
			}
		}
		return nil
	}
	// Orchestrator.SectorRotationBaseAllocations
	// DEPRECATED: orchestrator_sector_rotation_base_allocations_* parameter paths
	// removed in favor of sector_allocation.base_weights (see internal/sectorallocation).
	// Returns nil for backward compatibility; callers should query the sectorallocation module.
	return nil
}

// ListParameters returns all supported parameter names.
func (ie *InferenceEngine) ListParameters() []string {
	return []string{
		// Darwinian parameters
		"darwinian_weight_min",
		"darwinian_weight_max",
		"darwinian_weight_neutral",
		"darwinian_top_quartile_multiplier",
		"darwinian_bottom_quartile_multiplier",
		"darwinian_lookback_days",
		"darwinian_ema_alpha",
		"darwinian_sharpe_normalize_denom",
		"darwinian_max_performance_bonus_pct",
		"darwinian_volatility_penalty_threshold",
		"darwinian_volatility_penalty_multiplier",
		"darwinian_risk_volatility_threshold",
		"darwinian_risk_multiplier",
		"darwinian_hit_rate_high_threshold",
		"darwinian_hit_rate_low_threshold",
		"darwinian_middle_tier_boost_multiplier",
		"darwinian_middle_tier_cut_multiplier",
		"darwinian_sharpe_min_sample_size",
		"darwinian_stddev_mean_ratio_threshold",
		"darwinian_conviction_clamp_min",
		"darwinian_conviction_clamp_max",
		// Factor parameters
		"factor_momentum_lookback_days",
		"factor_momentum_stddev_divisor",
		"factor_momentum_intraday_discount",
		"factor_momentum_intraday_threshold",
		"factor_value_pe_range_center",
		"factor_value_pe_range_width",
		"factor_value_pb_range_center",
		"factor_value_pb_range_width",
		"factor_value_ps_range_center",
		"factor_value_ps_range_width",
		"factor_quality_dividend_yield_cap",
		"factor_quality_volatility_std",
		"factor_quality_fallback_score",
		"factor_value_fallback_score",
		"factor_fallback_weight_reduction",
		// Optimizer parameters
		"optimizer_max_position_pct",
		"optimizer_max_sector_pct",
		"optimizer_max_turnover_daily",
		"optimizer_target_beta",
		"optimizer_beta_range_min",
		"optimizer_beta_range_max",
		"optimizer_min_trade_size",
		"optimizer_cash_reserve",
		// Sizing parameters
		"sizing_kelly_fraction",
		"sizing_vol_lookback_days",
		"sizing_max_position_by_adv",
		"sizing_max_drawdown_limit",
		"sizing_atr_multiplier",
		"sizing_correlation_penalty",
		"sizing_correlation_threshold",
		"sizing_default_win_rate",
		"sizing_default_payoff_ratio",
		"sizing_target_volatility",
		"sizing_vol_adjustment_min",
		"sizing_vol_adjustment_max",
		"sizing_atr_target_risk",
		"sizing_atr_adjustment_min",
		"sizing_atr_adjustment_max",
		"sizing_correlation_penalty_factor",
		"sizing_max_correlation_penalty",
		"sizing_default_volatility",
		"sizing_default_adv",
		"sizing_default_atr",
		// Health parameters
		"health_mute_threshold",
		"health_unmute_threshold",
		"health_auto_recover_days",
		"health_min_sample_size",
		"health_negative_sharpe_threshold",
		"health_sharpe_weight",
		"health_hitrate_weight",
		"health_streak_weight",
		"health_max_sharpe",
		"health_min_sharpe",
		"health_streak_max",
		// GARCH parameters
		"garch_omega",
		"garch_alpha",
		"garch_beta",
		"garch_max_history",
		"garch_correlation_min_days",
		"garch_smoothing_factor",
		"garch_rebalance_threshold",
		"garch_min_forecast_days",
		"garch_max_history_points",
		"garch_high_vol_threshold",
		"garch_low_vol_threshold",
		"garch_reduce_magnitude",
		"garch_increase_magnitude",
		"garch_weekly_rebalance_days",
		// Experiment parameters
		"experiment_maturity_level1_observations",
		"experiment_maturity_level2_observations",
		"experiment_maturity_level3_observations",
		"experiment_improvement_threshold",
		"experiment_welch_ttest_threshold",
		"experiment_drawdown_protection_ratio",
		"experiment_volatility_tolerance_ratio",
		"experiment_oos_window_days",
		"experiment_sharpe_stability_threshold",
		"experiment_max_fallback_ratio",
		// Baseline parameters
		"baseline_starting_cash",
		"baseline_max_position_weight",
		"baseline_max_open_positions",
		"baseline_min_tradable_volume",
		"baseline_min_recommendation_conviction",
		"baseline_transaction_cost_bps",
		"baseline_slippage_bps",
		"baseline_reserve_cash_fraction",
		// Orchestrator parameters
		"orchestrator_conviction_floor_default",
		"orchestrator_superinvestor_min_conviction",
		"orchestrator_superinvestor_conviction_base",
		"orchestrator_cro_zscore_threshold",
		"orchestrator_sector_concentration_threshold",
		"orchestrator_sector_concentration_threshold_high",
		"orchestrator_sector_conviction_multiplier",
		"orchestrator_crowded_conviction_multiplier",
		"orchestrator_factor_weight_momentum",
		"orchestrator_factor_weight_value",
		"orchestrator_factor_weight_quality",
		"orchestrator_factor_weight_agent",
		"orchestrator_prism_boost_multiplier",
		"orchestrator_prism_boost_min",
		"orchestrator_prism_boost_max",
		"orchestrator_promotion_min_observations",
		"orchestrator_promotion_sharpe_threshold",
		"orchestrator_promotion_hitrate_threshold",
		"orchestrator_rejection_sharpe_threshold",
		"orchestrator_rejection_hitrate_threshold",
		// Risk parameters
		"risk_var_confidence_level",
		"risk_var_secondary_confidence",
		"risk_var_alert_threshold",
		"risk_var_critical_threshold",
		"risk_consecutive_loss_limit",
		"risk_max_drawdown_pct",
		"risk_max_position_size",
		"risk_max_daily_loss_pct",
		"risk_stop_loss",
		"risk_take_profit",
		"risk_max_loss_per_trade",
		"risk_max_total_exposure",
		// Realtime parameters
		"realtime_volatility_threshold",
		"realtime_volume_spike_threshold",
		"realtime_price_change_threshold",
		"realtime_min_confidence",
		"realtime_weight_adjustment_rate",
		"realtime_max_weight_change",
		"realtime_min_weight",
		"realtime_update_interval_ms",
		// Janus parameters
		"janus_short_window_days",
		"janus_medium_window_days",
		"janus_long_window_days",
		"janus_max_history_days",
		"janus_min_weight",
		"janus_max_weight",
		"janus_novel_threshold",
		"janus_historical_threshold",
		"janus_epsilon_weight",
		"janus_short_window_blend",
		"janus_medium_window_blend",
		"janus_long_window_blend",
		"janus_health_stale_hours",
		"janus_health_warn_hours",
		// Narrative parameters
		"narrative_min_trend_strength",
		"narrative_min_confidence",
		"narrative_min_hit_rate",
		"narrative_override_threshold",
		"narrative_ai_revenue_growth_threshold",
		"narrative_cowos_utilization_threshold",
		"narrative_capex_growth_threshold",
		"narrative_us10y_change_bps_threshold",
		"narrative_dxy_change_pct_threshold",
		"narrative_geopolitical_gpr_threshold",
		"narrative_oil_change_pct_threshold",
		"narrative_jpy_change_pct_threshold",
		"narrative_vix_level_threshold",
		"narrative_taiwan_stress_dxy_weight",
		"narrative_taiwan_stress_us10y_weight",
		"narrative_taiwan_stress_foreign_weight",
		"narrative_taiwan_stress_vix_weight",
		"narrative_taiwan_stress_jpy_weight",
		"narrative_taiwan_stress_geo_weight",
		"narrative_model_lookback_days",
		"narrative_model_hold_window_days",
		// Marketdata parameters
		"marketdata_twse_api_rate_limit",
		"marketdata_twse_api_rate_burst",
		"marketdata_twse_api_timeout_sec",
		"marketdata_fubon_intraday_limit",
		"marketdata_fubon_historical_limit",
		"marketdata_fubon_api_timeout_sec",
		"marketdata_tej_calls_per_second",
		"marketdata_tej_api_timeout_sec",
		"marketdata_fugle_rate_limit",
		"marketdata_fugle_api_timeout_sec",
		"marketdata_max_retry_attempts",
		"marketdata_retry_backoff_ms",
		// Strategy parameters
		"strategy_min_switch_interval_days",
		"strategy_switch_threshold",
		"strategy_score_lookback_days",
		// Industry Linkage parameters
		"industry_linkage_recession_shock_amplifier",
	}
}

// setParameterOnConfig sets a single parameter by name on the given config.
func (ie *InferenceEngine) setParameterOnConfig(cfg *ParametersConfig, name string, value float64) error {
	if accessor, ok := parameterTable[name]; ok {
		accessor.set(cfg, value)
		return nil
	}
	if ie.handleMapSetParameter(cfg, name, value) {
		return nil
	}
	return fmt.Errorf("unknown parameter: %s", name)
}

// handleMapSetParameter handles setting map sub-keys via dot notation.
// Returns true if handled, false otherwise.
func (ie *InferenceEngine) handleMapSetParameter(cfg *ParametersConfig, name string, value float64) bool {
	// Factor.InstitutionalSentimentWeights
	if strings.HasPrefix(name, "factor_institutional_sentiment_weights_") {
		key := strings.TrimPrefix(name, "factor_institutional_sentiment_weights_")
		if cfg.Factor.InstitutionalSentimentWeights.Value == nil {
			cfg.Factor.InstitutionalSentimentWeights.Value = make(map[string]float64)
		}
		cfg.Factor.InstitutionalSentimentWeights.Value[key] = value
		return true
	}
	// Optimizer.FactorWeights
	if strings.HasPrefix(name, "optimizer_factor_weights_") {
		key := strings.TrimPrefix(name, "optimizer_factor_weights_")
		if cfg.Optimizer.FactorWeights.Value == nil {
			cfg.Optimizer.FactorWeights.Value = make(map[string]float64)
		}
		cfg.Optimizer.FactorWeights.Value[key] = value
		return true
	}
	// Risk.SectorConstraintsRiskOff
	if strings.HasPrefix(name, "risk_sector_constraints_risk_off_") {
		key := strings.TrimPrefix(name, "risk_sector_constraints_risk_off_")
		if cfg.Risk.SectorConstraintsRiskOff.Value == nil {
			cfg.Risk.SectorConstraintsRiskOff.Value = make(map[string]float64)
		}
		cfg.Risk.SectorConstraintsRiskOff.Value[key] = value
		return true
	}
	// Risk.SectorConstraintsCarryTrade
	if strings.HasPrefix(name, "risk_sector_constraints_carry_trade_") {
		key := strings.TrimPrefix(name, "risk_sector_constraints_carry_trade_")
		if cfg.Risk.SectorConstraintsCarryTrade.Value == nil {
			cfg.Risk.SectorConstraintsCarryTrade.Value = make(map[string]float64)
		}
		cfg.Risk.SectorConstraintsCarryTrade.Value[key] = value
		return true
	}
	// Risk.SectorConstraintsSectorRotation
	if strings.HasPrefix(name, "risk_sector_constraints_sector_rotation_") {
		key := strings.TrimPrefix(name, "risk_sector_constraints_sector_rotation_")
		if cfg.Risk.SectorConstraintsSectorRotation.Value == nil {
			cfg.Risk.SectorConstraintsSectorRotation.Value = make(map[string]float64)
		}
		cfg.Risk.SectorConstraintsSectorRotation.Value[key] = value
		return true
	}
	// Narrative.EventTTLMultiplier
	if strings.HasPrefix(name, "narrative_event_ttl_multiplier_") {
		key := strings.TrimPrefix(name, "narrative_event_ttl_multiplier_")
		if cfg.Narrative.EventTTLMultiplier.Value == nil {
			cfg.Narrative.EventTTLMultiplier.Value = make(map[string]float64)
		}
		cfg.Narrative.EventTTLMultiplier.Value[key] = value
		return true
	}
	// Industry.SectorWeights
	// DEPRECATED: industry_sector_weights_* write paths removed. Use
	// sector_allocation.base_weights instead. Returns false to indicate the
	// parameter name is no longer recognized.
	// Drawdown.SectorConstraintsRiskOff
	if strings.HasPrefix(name, "drawdown_sector_constraints_risk_off_") {
		key := strings.TrimPrefix(name, "drawdown_sector_constraints_risk_off_")
		if cfg.Drawdown.SectorConstraintsRiskOff.Value == nil {
			cfg.Drawdown.SectorConstraintsRiskOff.Value = make(map[string]float64)
		}
		cfg.Drawdown.SectorConstraintsRiskOff.Value[key] = value
		return true
	}
	// Drawdown.SectorConstraintsCarryTradeUnwind
	if strings.HasPrefix(name, "drawdown_sector_constraints_carry_trade_unwind_") {
		key := strings.TrimPrefix(name, "drawdown_sector_constraints_carry_trade_unwind_")
		if cfg.Drawdown.SectorConstraintsCarryTradeUnwind.Value == nil {
			cfg.Drawdown.SectorConstraintsCarryTradeUnwind.Value = make(map[string]float64)
		}
		cfg.Drawdown.SectorConstraintsCarryTradeUnwind.Value[key] = value
		return true
	}
	// Drawdown.SectorConstraintsSectorRotation
	if strings.HasPrefix(name, "drawdown_sector_constraints_sector_rotation_") {
		key := strings.TrimPrefix(name, "drawdown_sector_constraints_sector_rotation_")
		if cfg.Drawdown.SectorConstraintsSectorRotation.Value == nil {
			cfg.Drawdown.SectorConstraintsSectorRotation.Value = make(map[string]float64)
		}
		cfg.Drawdown.SectorConstraintsSectorRotation.Value[key] = value
		return true
	}
	// Orchestrator.SectorRotationBaseAllocations
	// DEPRECATED: orchestrator_sector_rotation_base_allocations_* write paths
	// removed. Use sector_allocation.base_weights instead. Returns false.
	return false
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

	empiricalMaxDD := math.Abs(var99.VaR)
	if empiricalMaxDD > 0 {
		suggestedMaxDD := empiricalMaxDD * 1.2
		ie.params.Sizing.MaxDrawdownLimit.Value = math.Min(suggestedMaxDD, 0.20)
	}

	if var95.VaR != 0 {
		empiricalVol := math.Abs(var95.VaR) / 1.645
		ie.params.Sizing.TargetVolatility.Value = empiricalVol
	}

	return nil
}

// SelfLearn runs Bayesian optimization on a set of parameters, applies the best
// values found, and returns the improvement delta. This is the self-learning
// loop hook: call it periodically (e.g. from MetaLearner) to continuously
// optimize parameters against a user-provided backtest evaluator.
// Parameters:
//   - paramNames: list of parameter names to optimize (e.g. ["risk_max_position_size"])
//   - evaluator: backtest scoring function, higher = better
//   - config: optimizer configuration (use DefaultOptimizerConfig() for default)
//
// Returns the absolute improvement in score after optimization.
func (ie *InferenceEngine) SelfLearn(paramNames []string, evaluator func(cfg *ParametersConfig) (float64, error), config OptimizerConfig) (float64, error) {
	result, err := ie.OptimizeBayesian(paramNames, evaluator, config)
	if err != nil {
		return 0, fmt.Errorf("self-learn: %w", err)
	}

	currentScore, err := evaluator(ie.params)
	if err != nil {
		currentScore = 0
	}

	bestScore := result.BestScore
	improvement := bestScore - currentScore

	for name, val := range result.ParamValues {
		_ = ie.SetParameter(name, val)
	}

	return improvement, nil
}
