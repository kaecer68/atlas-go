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

// GetParameter retrieves the current value of a parameter by name.
// Returns the value and true if found, or 0 and false if not found.
func (ie *InferenceEngine) GetParameter(name string) (float64, bool) {
	return ie.getParameterFromConfig(ie.params, name)
}

// getParameterFromConfig retrieves a parameter value from the given config.
func (ie *InferenceEngine) getParameterFromConfig(cfg *ParametersConfig, name string) (float64, bool) {
	// Handle map parameters with dot notation
	if strings.Contains(name, "_") {
		if val := ie.handleMapGetParameter(cfg, name); val != nil {
			return *val, true
		}
	}

	switch name {
	// Darwinian parameters
	case "darwinian_weight_min":
		return cfg.Darwinian.WeightMin.Value, true
	case "darwinian_weight_max":
		return cfg.Darwinian.WeightMax.Value, true
	case "darwinian_weight_neutral":
		return cfg.Darwinian.WeightNeutral.Value, true
	case "darwinian_top_quartile_multiplier":
		return cfg.Darwinian.TopQuartileMultiplier.Value, true
	case "darwinian_bottom_quartile_multiplier":
		return cfg.Darwinian.BottomQuartileMultiplier.Value, true
	case "darwinian_lookback_days":
		return float64(cfg.Darwinian.LookbackDays.Value), true
	case "darwinian_ema_alpha":
		return cfg.Darwinian.EMAAlpha.Value, true
	case "darwinian_sharpe_normalize_denom":
		return cfg.Darwinian.SharpeNormalizeDenom.Value, true
	case "darwinian_max_performance_bonus_pct":
		return cfg.Darwinian.MaxPerformanceBonusPct.Value, true
	case "darwinian_volatility_penalty_threshold":
		return cfg.Darwinian.VolatilityPenaltyThreshold.Value, true
	case "darwinian_volatility_penalty_multiplier":
		return cfg.Darwinian.VolatilityPenaltyMultiplier.Value, true
	case "darwinian_risk_volatility_threshold":
		return cfg.Darwinian.RiskVolatilityThreshold.Value, true
	case "darwinian_risk_multiplier":
		return cfg.Darwinian.RiskMultiplier.Value, true
	case "darwinian_hit_rate_high_threshold":
		return cfg.Darwinian.HitRateHighThreshold.Value, true
	case "darwinian_hit_rate_low_threshold":
		return cfg.Darwinian.HitRateLowThreshold.Value, true
	case "darwinian_middle_tier_boost_multiplier":
		return cfg.Darwinian.MiddleTierBoostMultiplier.Value, true
	case "darwinian_middle_tier_cut_multiplier":
		return cfg.Darwinian.MiddleTierCutMultiplier.Value, true
	case "darwinian_sharpe_min_sample_size":
		return float64(cfg.Darwinian.SharpeMinSampleSize.Value), true
	case "darwinian_stddev_mean_ratio_threshold":
		return cfg.Darwinian.StdDevMeanRatioThreshold.Value, true
	case "darwinian_conviction_clamp_min":
		return float64(cfg.Darwinian.ConvictionClampMin.Value), true
	case "darwinian_conviction_clamp_max":
		return float64(cfg.Darwinian.ConvictionClampMax.Value), true
	// Factor parameters
	case "factor_momentum_lookback_days":
		return float64(cfg.Factor.MomentumLookbackDays.Value), true
	case "factor_momentum_stddev_divisor":
		return cfg.Factor.MomentumStdDevDivisor.Value, true
	case "factor_momentum_intraday_discount":
		return cfg.Factor.MomentumIntradayDiscount.Value, true
	case "factor_momentum_intraday_threshold":
		return cfg.Factor.MomentumIntradayThreshold.Value, true
	case "factor_value_pe_range_center":
		return cfg.Factor.ValuePERangeCenter.Value, true
	case "factor_value_pe_range_width":
		return cfg.Factor.ValuePERangeWidth.Value, true
	case "factor_value_pb_range_center":
		return cfg.Factor.ValuePBRangeCenter.Value, true
	case "factor_value_pb_range_width":
		return cfg.Factor.ValuePBRangeWidth.Value, true
	case "factor_value_ps_range_center":
		return cfg.Factor.ValuePSRangeCenter.Value, true
	case "factor_value_ps_range_width":
		return cfg.Factor.ValuePSRangeWidth.Value, true
	case "factor_quality_dividend_yield_cap":
		return cfg.Factor.QualityDividendYieldCap.Value, true
	case "factor_quality_volatility_std":
		return cfg.Factor.QualityVolatilityStd.Value, true
	case "factor_quality_fallback_score":
		return cfg.Factor.QualityFallbackScore.Value, true
	case "factor_value_fallback_score":
		return cfg.Factor.ValueFallbackScore.Value, true
	case "factor_fallback_weight_reduction":
		return cfg.Factor.FallbackWeightReduction.Value, true
	// Optimizer parameters
	case "optimizer_max_position_pct":
		return cfg.Optimizer.MaxPositionPct.Value, true
	case "optimizer_max_sector_pct":
		return cfg.Optimizer.MaxSectorPct.Value, true
	case "optimizer_max_turnover_daily":
		return cfg.Optimizer.MaxTurnoverDaily.Value, true
	case "optimizer_target_beta":
		return cfg.Optimizer.TargetBeta.Value, true
	case "optimizer_beta_range_min":
		return cfg.Optimizer.BetaRangeMin.Value, true
	case "optimizer_beta_range_max":
		return cfg.Optimizer.BetaRangeMax.Value, true
	case "optimizer_min_trade_size":
		return float64(cfg.Optimizer.MinTradeSize.Value), true
	case "optimizer_cash_reserve":
		return cfg.Optimizer.CashReserve.Value, true
	// Sizing parameters
	case "sizing_kelly_fraction":
		return cfg.Sizing.KellyFraction.Value, true
	case "sizing_vol_lookback_days":
		return float64(cfg.Sizing.VolLookbackDays.Value), true
	case "sizing_max_position_by_adv":
		return cfg.Sizing.MaxPositionByADV.Value, true
	case "sizing_max_drawdown_limit":
		return cfg.Sizing.MaxDrawdownLimit.Value, true
	case "sizing_atr_multiplier":
		return cfg.Sizing.ATRMultiplier.Value, true
	case "sizing_correlation_penalty":
		return cfg.Sizing.CorrelationPenalty.Value, true
	case "sizing_correlation_threshold":
		return cfg.Sizing.CorrelationThreshold.Value, true
	case "sizing_default_win_rate":
		return cfg.Sizing.DefaultWinRate.Value, true
	case "sizing_default_payoff_ratio":
		return cfg.Sizing.DefaultPayoffRatio.Value, true
	case "sizing_target_volatility":
		return cfg.Sizing.TargetVolatility.Value, true
	case "sizing_vol_adjustment_min":
		return cfg.Sizing.VolAdjustmentMin.Value, true
	case "sizing_vol_adjustment_max":
		return cfg.Sizing.VolAdjustmentMax.Value, true
	case "sizing_atr_target_risk":
		return cfg.Sizing.ATRTargetRisk.Value, true
	case "sizing_atr_adjustment_min":
		return cfg.Sizing.ATRAdjustmentMin.Value, true
	case "sizing_atr_adjustment_max":
		return cfg.Sizing.ATRAdjustmentMax.Value, true
	case "sizing_correlation_penalty_factor":
		return cfg.Sizing.CorrelationPenaltyFactor.Value, true
	case "sizing_max_correlation_penalty":
		return cfg.Sizing.MaxCorrelationPenalty.Value, true
	case "sizing_default_volatility":
		return cfg.Sizing.DefaultVolatility.Value, true
	case "sizing_default_adv":
		return cfg.Sizing.DefaultADV.Value, true
	case "sizing_default_atr":
		return cfg.Sizing.DefaultATR.Value, true
	// Health parameters
	case "health_mute_threshold":
		return float64(cfg.Health.MuteThreshold.Value), true
	case "health_unmute_threshold":
		return float64(cfg.Health.UnmuteThreshold.Value), true
	case "health_auto_recover_days":
		return float64(cfg.Health.AutoRecoverDays.Value), true
	case "health_min_sample_size":
		return float64(cfg.Health.MinSampleSize.Value), true
	case "health_negative_sharpe_threshold":
		return cfg.Health.NegativeSharpeThreshold.Value, true
	case "health_sharpe_weight":
		return cfg.Health.SharpeWeight.Value, true
	case "health_hitrate_weight":
		return cfg.Health.HitRateWeight.Value, true
	case "health_streak_weight":
		return cfg.Health.StreakWeight.Value, true
	case "health_max_sharpe":
		return cfg.Health.MaxSharpe.Value, true
	case "health_min_sharpe":
		return cfg.Health.MinSharpe.Value, true
	case "health_streak_max":
		return float64(cfg.Health.StreakMax.Value), true
	// GARCH parameters
	case "garch_omega":
		return cfg.GARCH.Omega.Value, true
	case "garch_alpha":
		return cfg.GARCH.Alpha.Value, true
	case "garch_beta":
		return cfg.GARCH.Beta.Value, true
	case "garch_max_history":
		return float64(cfg.GARCH.MaxHistory.Value), true
	case "garch_correlation_min_days":
		return float64(cfg.GARCH.CorrelationMinDays.Value), true
	case "garch_smoothing_factor":
		return cfg.GARCH.SmoothingFactor.Value, true
	case "garch_rebalance_threshold":
		return cfg.GARCH.RebalanceThreshold.Value, true
	case "garch_min_forecast_days":
		return float64(cfg.GARCH.MinForecastDays.Value), true
	case "garch_max_history_points":
		return float64(cfg.GARCH.MaxHistoryPoints.Value), true
	case "garch_high_vol_threshold":
		return cfg.GARCH.HighVolThreshold.Value, true
	case "garch_low_vol_threshold":
		return cfg.GARCH.LowVolThreshold.Value, true
	case "garch_reduce_magnitude":
		return cfg.GARCH.ReduceMagnitude.Value, true
	case "garch_increase_magnitude":
		return cfg.GARCH.IncreaseMagnitude.Value, true
	case "garch_weekly_rebalance_days":
		return float64(cfg.GARCH.WeeklyRebalanceDays.Value), true
	// Experiment parameters
	case "experiment_maturity_level1_observations":
		return float64(cfg.Experiment.MaturityLevel1Observations.Value), true
	case "experiment_maturity_level2_observations":
		return float64(cfg.Experiment.MaturityLevel2Observations.Value), true
	case "experiment_maturity_level3_observations":
		return float64(cfg.Experiment.MaturityLevel3Observations.Value), true
	case "experiment_improvement_threshold":
		return cfg.Experiment.ImprovementThreshold.Value, true
	case "experiment_welch_ttest_threshold":
		return cfg.Experiment.WelchTTestThreshold.Value, true
	case "experiment_drawdown_protection_ratio":
		return cfg.Experiment.DrawdownProtectionRatio.Value, true
	case "experiment_volatility_tolerance_ratio":
		return cfg.Experiment.VolatilityToleranceRatio.Value, true
	case "experiment_oos_window_days":
		return float64(cfg.Experiment.OOSWindowDays.Value), true
	case "experiment_sharpe_stability_threshold":
		return cfg.Experiment.SharpeStabilityThreshold.Value, true
	case "experiment_max_fallback_ratio":
		return cfg.Experiment.MaxFallbackRatio.Value, true
	// Baseline parameters
	case "baseline_starting_cash":
		return cfg.Baseline.StartingCash.Value, true
	case "baseline_max_position_weight":
		return cfg.Baseline.MaxPositionWeight.Value, true
	case "baseline_max_open_positions":
		return float64(cfg.Baseline.MaxOpenPositions.Value), true
	case "baseline_min_tradable_volume":
		return cfg.Baseline.MinTradableVolume.Value, true
	case "baseline_min_recommendation_conviction":
		return float64(cfg.Baseline.MinRecommendationConviction.Value), true
	case "baseline_transaction_cost_bps":
		return cfg.Baseline.TransactionCostBPS.Value, true
	case "baseline_slippage_bps":
		return cfg.Baseline.SlippageBPS.Value, true
	case "baseline_reserve_cash_fraction":
		return cfg.Baseline.ReserveCashFraction.Value, true
	// Orchestrator parameters
	case "orchestrator_conviction_floor_default":
		return float64(cfg.Orchestrator.ConvictionFloorDefault.Value), true
	case "orchestrator_superinvestor_min_conviction":
		return float64(cfg.Orchestrator.SuperinvestorMinConviction.Value), true
	case "orchestrator_cro_zscore_threshold":
		return cfg.Orchestrator.CROZScoreThreshold.Value, true
	case "orchestrator_sector_concentration_threshold":
		return cfg.Orchestrator.SectorConcentrationThreshold.Value, true
	case "orchestrator_sector_concentration_threshold_high":
		return cfg.Orchestrator.SectorConcentrationThresholdHigh.Value, true
	case "orchestrator_sector_conviction_multiplier":
		return cfg.Orchestrator.SectorConvictionMultiplier.Value, true
	case "orchestrator_crowded_conviction_multiplier":
		return cfg.Orchestrator.CrowdedConvictionMultiplier.Value, true
	case "orchestrator_factor_weight_momentum":
		return cfg.Orchestrator.FactorWeightMomentum.Value, true
	case "orchestrator_factor_weight_value":
		return cfg.Orchestrator.FactorWeightValue.Value, true
	case "orchestrator_factor_weight_quality":
		return cfg.Orchestrator.FactorWeightQuality.Value, true
	case "orchestrator_factor_weight_agent":
		return cfg.Orchestrator.FactorWeightAgent.Value, true
	case "orchestrator_prism_boost_multiplier":
		return cfg.Orchestrator.PRISMBoostMultiplier.Value, true
	case "orchestrator_prism_boost_min":
		return float64(cfg.Orchestrator.PRISMBoostMin.Value), true
	case "orchestrator_prism_boost_max":
		return float64(cfg.Orchestrator.PRISMBoostMax.Value), true
	case "orchestrator_promotion_min_observations":
		return float64(cfg.Orchestrator.PromotionMinObservations.Value), true
	case "orchestrator_promotion_sharpe_threshold":
		return cfg.Orchestrator.PromotionSharpeThreshold.Value, true
	case "orchestrator_promotion_hitrate_threshold":
		return cfg.Orchestrator.PromotionHitRateThreshold.Value, true
	case "orchestrator_rejection_sharpe_threshold":
		return cfg.Orchestrator.RejectionSharpeThreshold.Value, true
	case "orchestrator_rejection_hitrate_threshold":
		return cfg.Orchestrator.RejectionHitRateThreshold.Value, true
	// Risk parameters
	case "risk_var_confidence_level":
		return cfg.Risk.VaRConfidenceLevel.Value, true
	case "risk_var_secondary_confidence":
		return cfg.Risk.VaRSecondaryConfidence.Value, true
	case "risk_var_alert_threshold":
		return cfg.Risk.VaRAlertThreshold.Value, true
	case "risk_var_critical_threshold":
		return cfg.Risk.VaRCriticalThreshold.Value, true
	case "risk_consecutive_loss_limit":
		return float64(cfg.Risk.ConsecutiveLossLimit.Value), true
	case "risk_max_drawdown_pct":
		return cfg.Risk.MaxDrawdownPct.Value, true
	case "risk_max_position_size":
		return cfg.Risk.MaxPositionSize.Value, true
	case "risk_max_daily_loss_pct":
		return cfg.Risk.MaxDailyLossPct.Value, true
	case "risk_stop_loss":
		return cfg.Risk.StopLoss.Value, true
	case "risk_take_profit":
		return cfg.Risk.TakeProfit.Value, true
	case "risk_max_loss_per_trade":
		return cfg.Risk.MaxLossPerTrade.Value, true
	case "risk_max_total_exposure":
		return cfg.Risk.MaxTotalExposure.Value, true
	// Realtime parameters
	case "realtime_volatility_threshold":
		return cfg.Realtime.VolatilityThreshold.Value, true
	case "realtime_volume_spike_threshold":
		return cfg.Realtime.VolumeSpikeThreshold.Value, true
	case "realtime_price_change_threshold":
		return cfg.Realtime.PriceChangeThreshold.Value, true
	case "realtime_min_confidence":
		return cfg.Realtime.MinConfidence.Value, true
	case "realtime_weight_adjustment_rate":
		return cfg.Realtime.WeightAdjustmentRate.Value, true
	case "realtime_max_weight_change":
		return cfg.Realtime.MaxWeightChange.Value, true
	case "realtime_min_weight":
		return cfg.Realtime.MinWeight.Value, true
	case "realtime_update_interval_ms":
		return float64(cfg.Realtime.UpdateIntervalMs.Value), true
	// Janus parameters
	case "janus_short_window_days":
		return float64(cfg.Janus.ShortWindowDays.Value), true
	case "janus_medium_window_days":
		return float64(cfg.Janus.MediumWindowDays.Value), true
	case "janus_long_window_days":
		return float64(cfg.Janus.LongWindowDays.Value), true
	case "janus_max_history_days":
		return float64(cfg.Janus.MaxHistoryDays.Value), true
	case "janus_min_weight":
		return cfg.Janus.MinWeight.Value, true
	case "janus_max_weight":
		return cfg.Janus.MaxWeight.Value, true
	case "janus_novel_threshold":
		return cfg.Janus.NovelThreshold.Value, true
	case "janus_historical_threshold":
		return cfg.Janus.HistoricalThreshold.Value, true
	case "janus_epsilon_weight":
		return cfg.Janus.EpsilonWeight.Value, true
	case "janus_short_window_blend":
		return cfg.Janus.ShortWindowBlend.Value, true
	case "janus_medium_window_blend":
		return cfg.Janus.MediumWindowBlend.Value, true
	case "janus_long_window_blend":
		return cfg.Janus.LongWindowBlend.Value, true
	case "janus_health_stale_hours":
		return float64(cfg.Janus.HealthStaleHours.Value), true
	case "janus_health_warn_hours":
		return float64(cfg.Janus.HealthWarnHours.Value), true
	// Narrative parameters
	case "narrative_min_trend_strength":
		return cfg.Narrative.MinTrendStrength.Value, true
	case "narrative_min_confidence":
		return cfg.Narrative.MinConfidence.Value, true
	case "narrative_min_hit_rate":
		return cfg.Narrative.MinHitRate.Value, true
	case "narrative_override_threshold":
		return cfg.Narrative.OverrideThreshold.Value, true
	case "narrative_ai_revenue_growth_threshold":
		return cfg.Narrative.AIRevenueGrowthThreshold.Value, true
	case "narrative_cowos_utilization_threshold":
		return cfg.Narrative.CoWoSUtilizationThreshold.Value, true
	case "narrative_capex_growth_threshold":
		return cfg.Narrative.CapexGrowthThreshold.Value, true
	case "narrative_us10y_change_bps_threshold":
		return cfg.Narrative.US10YChangeBpsThreshold.Value, true
	case "narrative_dxy_change_pct_threshold":
		return cfg.Narrative.DXYChangePctThreshold.Value, true
	case "narrative_geopolitical_gpr_threshold":
		return cfg.Narrative.GeopoliticalGPRThreshold.Value, true
	case "narrative_oil_change_pct_threshold":
		return cfg.Narrative.OilChangePctThreshold.Value, true
	case "narrative_jpy_change_pct_threshold":
		return cfg.Narrative.JPYChangePctThreshold.Value, true
	case "narrative_vix_level_threshold":
		return cfg.Narrative.VIXLevelThreshold.Value, true
	case "narrative_taiwan_stress_dxy_weight":
		return cfg.Narrative.TaiwanStressDXYWeight.Value, true
	case "narrative_taiwan_stress_us10y_weight":
		return cfg.Narrative.TaiwanStressUS10YWeight.Value, true
	case "narrative_taiwan_stress_foreign_weight":
		return cfg.Narrative.TaiwanStressForeignWeight.Value, true
	case "narrative_taiwan_stress_vix_weight":
		return cfg.Narrative.TaiwanStressVIXWeight.Value, true
	case "narrative_taiwan_stress_jpy_weight":
		return cfg.Narrative.TaiwanStressJPYWeight.Value, true
	case "narrative_taiwan_stress_geo_weight":
		return cfg.Narrative.TaiwanStressGeoWeight.Value, true
	case "narrative_model_lookback_days":
		return float64(cfg.Narrative.ModelLookbackDays.Value), true
	case "narrative_model_hold_window_days":
		return float64(cfg.Narrative.ModelHoldWindowDays.Value), true
	// Marketdata parameters
	case "marketdata_twse_api_rate_limit":
		return cfg.Marketdata.TWSEAPIRateLimit.Value, true
	case "marketdata_twse_api_rate_burst":
		return float64(cfg.Marketdata.TWSEAPIRateBurst.Value), true
	case "marketdata_twse_api_timeout_sec":
		return float64(cfg.Marketdata.TWSEAPITimeoutSec.Value), true
	case "marketdata_fubon_intraday_limit":
		return float64(cfg.Marketdata.FubonIntradayLimit.Value), true
	case "marketdata_fubon_historical_limit":
		return float64(cfg.Marketdata.FubonHistoricalLimit.Value), true
	case "marketdata_fubon_api_timeout_sec":
		return float64(cfg.Marketdata.FubonAPITimeoutSec.Value), true
	case "marketdata_tej_calls_per_second":
		return float64(cfg.Marketdata.TEJCallsPerSecond.Value), true
	case "marketdata_tej_api_timeout_sec":
		return float64(cfg.Marketdata.TEJAPITimeoutSec.Value), true
	case "marketdata_fugle_rate_limit":
		return float64(cfg.Marketdata.FugleRateLimit.Value), true
	case "marketdata_fugle_api_timeout_sec":
		return float64(cfg.Marketdata.FugleAPITimeoutSec.Value), true
	case "marketdata_max_retry_attempts":
		return float64(cfg.Marketdata.MaxRetryAttempts.Value), true
	case "marketdata_retry_backoff_ms":
		return float64(cfg.Marketdata.RetryBackoffMs.Value), true
	// Strategy parameters
	case "strategy_min_switch_interval_days":
		return float64(cfg.Strategy.MinSwitchIntervalDays.Value), true
	case "strategy_switch_threshold":
		return cfg.Strategy.SwitchThreshold.Value, true
	case "strategy_score_lookback_days":
		return float64(cfg.Strategy.ScoreLookbackDays.Value), true
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
	if strings.HasPrefix(name, "industry_sector_weights_") {
		key := strings.TrimPrefix(name, "industry_sector_weights_")
		if cfg.Industry.SectorWeights.Value != nil {
			if v, ok := cfg.Industry.SectorWeights.Value[key]; ok {
				return &v
			}
		}
		return nil
	}
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
	}
}

// setParameterOnConfig sets a single parameter by name on the given config.
func (ie *InferenceEngine) setParameterOnConfig(cfg *ParametersConfig, name string, value float64) error {
	// Handle map sub-keys with dot notation
	if strings.Contains(name, "_") {
		if ie.handleMapSetParameter(cfg, name, value) {
			return nil
		}
	}

	switch name {
	// Darwinian parameters
	case "darwinian_weight_min":
		cfg.Darwinian.WeightMin.Value = value
	case "darwinian_weight_max":
		cfg.Darwinian.WeightMax.Value = value
	case "darwinian_weight_neutral":
		cfg.Darwinian.WeightNeutral.Value = value
	case "darwinian_top_quartile_multiplier":
		cfg.Darwinian.TopQuartileMultiplier.Value = value
	case "darwinian_bottom_quartile_multiplier":
		cfg.Darwinian.BottomQuartileMultiplier.Value = value
	case "darwinian_lookback_days":
		cfg.Darwinian.LookbackDays.Value = int(value)
	case "darwinian_ema_alpha":
		cfg.Darwinian.EMAAlpha.Value = value
	case "darwinian_sharpe_normalize_denom":
		cfg.Darwinian.SharpeNormalizeDenom.Value = value
	case "darwinian_max_performance_bonus_pct":
		cfg.Darwinian.MaxPerformanceBonusPct.Value = value
	case "darwinian_volatility_penalty_threshold":
		cfg.Darwinian.VolatilityPenaltyThreshold.Value = value
	case "darwinian_volatility_penalty_multiplier":
		cfg.Darwinian.VolatilityPenaltyMultiplier.Value = value
	case "darwinian_risk_volatility_threshold":
		cfg.Darwinian.RiskVolatilityThreshold.Value = value
	case "darwinian_risk_multiplier":
		cfg.Darwinian.RiskMultiplier.Value = value
	case "darwinian_hit_rate_high_threshold":
		cfg.Darwinian.HitRateHighThreshold.Value = value
	case "darwinian_hit_rate_low_threshold":
		cfg.Darwinian.HitRateLowThreshold.Value = value
	case "darwinian_middle_tier_boost_multiplier":
		cfg.Darwinian.MiddleTierBoostMultiplier.Value = value
	case "darwinian_middle_tier_cut_multiplier":
		cfg.Darwinian.MiddleTierCutMultiplier.Value = value
	case "darwinian_sharpe_min_sample_size":
		cfg.Darwinian.SharpeMinSampleSize.Value = int(value)
	case "darwinian_stddev_mean_ratio_threshold":
		cfg.Darwinian.StdDevMeanRatioThreshold.Value = value
	case "darwinian_conviction_clamp_min":
		cfg.Darwinian.ConvictionClampMin.Value = int(value)
	case "darwinian_conviction_clamp_max":
		cfg.Darwinian.ConvictionClampMax.Value = int(value)
	// Factor parameters
	case "factor_momentum_lookback_days":
		cfg.Factor.MomentumLookbackDays.Value = int(value)
	case "factor_momentum_stddev_divisor":
		cfg.Factor.MomentumStdDevDivisor.Value = value
	case "factor_momentum_intraday_discount":
		cfg.Factor.MomentumIntradayDiscount.Value = value
	case "factor_momentum_intraday_threshold":
		cfg.Factor.MomentumIntradayThreshold.Value = value
	case "factor_value_pe_range_center":
		cfg.Factor.ValuePERangeCenter.Value = value
	case "factor_value_pe_range_width":
		cfg.Factor.ValuePERangeWidth.Value = value
	case "factor_value_pb_range_center":
		cfg.Factor.ValuePBRangeCenter.Value = value
	case "factor_value_pb_range_width":
		cfg.Factor.ValuePBRangeWidth.Value = value
	case "factor_value_ps_range_center":
		cfg.Factor.ValuePSRangeCenter.Value = value
	case "factor_value_ps_range_width":
		cfg.Factor.ValuePSRangeWidth.Value = value
	case "factor_quality_dividend_yield_cap":
		cfg.Factor.QualityDividendYieldCap.Value = value
	case "factor_quality_volatility_std":
		cfg.Factor.QualityVolatilityStd.Value = value
	case "factor_quality_fallback_score":
		cfg.Factor.QualityFallbackScore.Value = value
	case "factor_value_fallback_score":
		cfg.Factor.ValueFallbackScore.Value = value
	case "factor_fallback_weight_reduction":
		cfg.Factor.FallbackWeightReduction.Value = value
	// Optimizer parameters
	case "optimizer_max_position_pct":
		cfg.Optimizer.MaxPositionPct.Value = value
	case "optimizer_max_sector_pct":
		cfg.Optimizer.MaxSectorPct.Value = value
	case "optimizer_max_turnover_daily":
		cfg.Optimizer.MaxTurnoverDaily.Value = value
	case "optimizer_target_beta":
		cfg.Optimizer.TargetBeta.Value = value
	case "optimizer_beta_range_min":
		cfg.Optimizer.BetaRangeMin.Value = value
	case "optimizer_beta_range_max":
		cfg.Optimizer.BetaRangeMax.Value = value
	case "optimizer_min_trade_size":
		cfg.Optimizer.MinTradeSize.Value = int(value)
	case "optimizer_cash_reserve":
		cfg.Optimizer.CashReserve.Value = value
	// Sizing parameters
	case "sizing_kelly_fraction":
		cfg.Sizing.KellyFraction.Value = value
	case "sizing_vol_lookback_days":
		cfg.Sizing.VolLookbackDays.Value = int(value)
	case "sizing_max_position_by_adv":
		cfg.Sizing.MaxPositionByADV.Value = value
	case "sizing_max_drawdown_limit":
		cfg.Sizing.MaxDrawdownLimit.Value = value
	case "sizing_atr_multiplier":
		cfg.Sizing.ATRMultiplier.Value = value
	case "sizing_correlation_penalty":
		cfg.Sizing.CorrelationPenalty.Value = value
	case "sizing_correlation_threshold":
		cfg.Sizing.CorrelationThreshold.Value = value
	case "sizing_default_win_rate":
		cfg.Sizing.DefaultWinRate.Value = value
	case "sizing_default_payoff_ratio":
		cfg.Sizing.DefaultPayoffRatio.Value = value
	case "sizing_target_volatility":
		cfg.Sizing.TargetVolatility.Value = value
	case "sizing_vol_adjustment_min":
		cfg.Sizing.VolAdjustmentMin.Value = value
	case "sizing_vol_adjustment_max":
		cfg.Sizing.VolAdjustmentMax.Value = value
	case "sizing_atr_target_risk":
		cfg.Sizing.ATRTargetRisk.Value = value
	case "sizing_atr_adjustment_min":
		cfg.Sizing.ATRAdjustmentMin.Value = value
	case "sizing_atr_adjustment_max":
		cfg.Sizing.ATRAdjustmentMax.Value = value
	case "sizing_correlation_penalty_factor":
		cfg.Sizing.CorrelationPenaltyFactor.Value = value
	case "sizing_max_correlation_penalty":
		cfg.Sizing.MaxCorrelationPenalty.Value = value
	case "sizing_default_volatility":
		cfg.Sizing.DefaultVolatility.Value = value
	case "sizing_default_adv":
		cfg.Sizing.DefaultADV.Value = value
	case "sizing_default_atr":
		cfg.Sizing.DefaultATR.Value = value
	// Health parameters
	case "health_mute_threshold":
		cfg.Health.MuteThreshold.Value = int(value)
	case "health_unmute_threshold":
		cfg.Health.UnmuteThreshold.Value = int(value)
	case "health_auto_recover_days":
		cfg.Health.AutoRecoverDays.Value = int(value)
	case "health_min_sample_size":
		cfg.Health.MinSampleSize.Value = int(value)
	case "health_negative_sharpe_threshold":
		cfg.Health.NegativeSharpeThreshold.Value = value
	case "health_sharpe_weight":
		cfg.Health.SharpeWeight.Value = value
	case "health_hitrate_weight":
		cfg.Health.HitRateWeight.Value = value
	case "health_streak_weight":
		cfg.Health.StreakWeight.Value = value
	case "health_max_sharpe":
		cfg.Health.MaxSharpe.Value = value
	case "health_min_sharpe":
		cfg.Health.MinSharpe.Value = value
	case "health_streak_max":
		cfg.Health.StreakMax.Value = int(value)
	// GARCH parameters
	case "garch_omega":
		cfg.GARCH.Omega.Value = value
	case "garch_alpha":
		cfg.GARCH.Alpha.Value = value
	case "garch_beta":
		cfg.GARCH.Beta.Value = value
	case "garch_max_history":
		cfg.GARCH.MaxHistory.Value = int(value)
	case "garch_correlation_min_days":
		cfg.GARCH.CorrelationMinDays.Value = int(value)
	case "garch_smoothing_factor":
		cfg.GARCH.SmoothingFactor.Value = value
	case "garch_rebalance_threshold":
		cfg.GARCH.RebalanceThreshold.Value = value
	case "garch_min_forecast_days":
		cfg.GARCH.MinForecastDays.Value = int(value)
	case "garch_max_history_points":
		cfg.GARCH.MaxHistoryPoints.Value = int(value)
	case "garch_high_vol_threshold":
		cfg.GARCH.HighVolThreshold.Value = value
	case "garch_low_vol_threshold":
		cfg.GARCH.LowVolThreshold.Value = value
	case "garch_reduce_magnitude":
		cfg.GARCH.ReduceMagnitude.Value = value
	case "garch_increase_magnitude":
		cfg.GARCH.IncreaseMagnitude.Value = value
	case "garch_weekly_rebalance_days":
		cfg.GARCH.WeeklyRebalanceDays.Value = int(value)
	// Experiment parameters
	case "experiment_maturity_level1_observations":
		cfg.Experiment.MaturityLevel1Observations.Value = int(value)
	case "experiment_maturity_level2_observations":
		cfg.Experiment.MaturityLevel2Observations.Value = int(value)
	case "experiment_maturity_level3_observations":
		cfg.Experiment.MaturityLevel3Observations.Value = int(value)
	case "experiment_improvement_threshold":
		cfg.Experiment.ImprovementThreshold.Value = value
	case "experiment_welch_ttest_threshold":
		cfg.Experiment.WelchTTestThreshold.Value = value
	case "experiment_drawdown_protection_ratio":
		cfg.Experiment.DrawdownProtectionRatio.Value = value
	case "experiment_volatility_tolerance_ratio":
		cfg.Experiment.VolatilityToleranceRatio.Value = value
	case "experiment_oos_window_days":
		cfg.Experiment.OOSWindowDays.Value = int(value)
	case "experiment_sharpe_stability_threshold":
		cfg.Experiment.SharpeStabilityThreshold.Value = value
	case "experiment_max_fallback_ratio":
		cfg.Experiment.MaxFallbackRatio.Value = value
	// Baseline parameters
	case "baseline_starting_cash":
		cfg.Baseline.StartingCash.Value = value
	case "baseline_max_position_weight":
		cfg.Baseline.MaxPositionWeight.Value = value
	case "baseline_max_open_positions":
		cfg.Baseline.MaxOpenPositions.Value = int(value)
	case "baseline_min_tradable_volume":
		cfg.Baseline.MinTradableVolume.Value = value
	case "baseline_min_recommendation_conviction":
		cfg.Baseline.MinRecommendationConviction.Value = int(value)
	case "baseline_transaction_cost_bps":
		cfg.Baseline.TransactionCostBPS.Value = value
	case "baseline_slippage_bps":
		cfg.Baseline.SlippageBPS.Value = value
	case "baseline_reserve_cash_fraction":
		cfg.Baseline.ReserveCashFraction.Value = value
	// Orchestrator parameters
	case "orchestrator_conviction_floor_default":
		cfg.Orchestrator.ConvictionFloorDefault.Value = int(value)
	case "orchestrator_superinvestor_min_conviction":
		cfg.Orchestrator.SuperinvestorMinConviction.Value = int(value)
	case "orchestrator_cro_zscore_threshold":
		cfg.Orchestrator.CROZScoreThreshold.Value = value
	case "orchestrator_sector_concentration_threshold":
		cfg.Orchestrator.SectorConcentrationThreshold.Value = value
	case "orchestrator_sector_concentration_threshold_high":
		cfg.Orchestrator.SectorConcentrationThresholdHigh.Value = value
	case "orchestrator_sector_conviction_multiplier":
		cfg.Orchestrator.SectorConvictionMultiplier.Value = value
	case "orchestrator_crowded_conviction_multiplier":
		cfg.Orchestrator.CrowdedConvictionMultiplier.Value = value
	case "orchestrator_factor_weight_momentum":
		cfg.Orchestrator.FactorWeightMomentum.Value = value
	case "orchestrator_factor_weight_value":
		cfg.Orchestrator.FactorWeightValue.Value = value
	case "orchestrator_factor_weight_quality":
		cfg.Orchestrator.FactorWeightQuality.Value = value
	case "orchestrator_factor_weight_agent":
		cfg.Orchestrator.FactorWeightAgent.Value = value
	case "orchestrator_prism_boost_multiplier":
		cfg.Orchestrator.PRISMBoostMultiplier.Value = value
	case "orchestrator_prism_boost_min":
		cfg.Orchestrator.PRISMBoostMin.Value = int(value)
	case "orchestrator_prism_boost_max":
		cfg.Orchestrator.PRISMBoostMax.Value = int(value)
	case "orchestrator_promotion_min_observations":
		cfg.Orchestrator.PromotionMinObservations.Value = int(value)
	case "orchestrator_promotion_sharpe_threshold":
		cfg.Orchestrator.PromotionSharpeThreshold.Value = value
	case "orchestrator_promotion_hitrate_threshold":
		cfg.Orchestrator.PromotionHitRateThreshold.Value = value
	case "orchestrator_rejection_sharpe_threshold":
		cfg.Orchestrator.RejectionSharpeThreshold.Value = value
	case "orchestrator_rejection_hitrate_threshold":
		cfg.Orchestrator.RejectionHitRateThreshold.Value = value
	// Risk parameters
	case "risk_var_confidence_level":
		cfg.Risk.VaRConfidenceLevel.Value = value
	case "risk_var_secondary_confidence":
		cfg.Risk.VaRSecondaryConfidence.Value = value
	case "risk_var_alert_threshold":
		cfg.Risk.VaRAlertThreshold.Value = value
	case "risk_var_critical_threshold":
		cfg.Risk.VaRCriticalThreshold.Value = value
	case "risk_consecutive_loss_limit":
		cfg.Risk.ConsecutiveLossLimit.Value = int(value)
	case "risk_max_drawdown_pct":
		cfg.Risk.MaxDrawdownPct.Value = value
	case "risk_max_position_size":
		cfg.Risk.MaxPositionSize.Value = value
	case "risk_max_daily_loss_pct":
		cfg.Risk.MaxDailyLossPct.Value = value
	case "risk_stop_loss":
		cfg.Risk.StopLoss.Value = value
	case "risk_take_profit":
		cfg.Risk.TakeProfit.Value = value
	case "risk_max_loss_per_trade":
		cfg.Risk.MaxLossPerTrade.Value = value
	case "risk_max_total_exposure":
		cfg.Risk.MaxTotalExposure.Value = value
	// Realtime parameters
	case "realtime_volatility_threshold":
		cfg.Realtime.VolatilityThreshold.Value = value
	case "realtime_volume_spike_threshold":
		cfg.Realtime.VolumeSpikeThreshold.Value = value
	case "realtime_price_change_threshold":
		cfg.Realtime.PriceChangeThreshold.Value = value
	case "realtime_min_confidence":
		cfg.Realtime.MinConfidence.Value = value
	case "realtime_weight_adjustment_rate":
		cfg.Realtime.WeightAdjustmentRate.Value = value
	case "realtime_max_weight_change":
		cfg.Realtime.MaxWeightChange.Value = value
	case "realtime_min_weight":
		cfg.Realtime.MinWeight.Value = value
	case "realtime_update_interval_ms":
		cfg.Realtime.UpdateIntervalMs.Value = int(value)
	// Janus parameters
	case "janus_short_window_days":
		cfg.Janus.ShortWindowDays.Value = int(value)
	case "janus_medium_window_days":
		cfg.Janus.MediumWindowDays.Value = int(value)
	case "janus_long_window_days":
		cfg.Janus.LongWindowDays.Value = int(value)
	case "janus_max_history_days":
		cfg.Janus.MaxHistoryDays.Value = int(value)
	case "janus_min_weight":
		cfg.Janus.MinWeight.Value = value
	case "janus_max_weight":
		cfg.Janus.MaxWeight.Value = value
	case "janus_novel_threshold":
		cfg.Janus.NovelThreshold.Value = value
	case "janus_historical_threshold":
		cfg.Janus.HistoricalThreshold.Value = value
	case "janus_epsilon_weight":
		cfg.Janus.EpsilonWeight.Value = value
	case "janus_short_window_blend":
		cfg.Janus.ShortWindowBlend.Value = value
	case "janus_medium_window_blend":
		cfg.Janus.MediumWindowBlend.Value = value
	case "janus_long_window_blend":
		cfg.Janus.LongWindowBlend.Value = value
	case "janus_health_stale_hours":
		cfg.Janus.HealthStaleHours.Value = int(value)
	case "janus_health_warn_hours":
		cfg.Janus.HealthWarnHours.Value = int(value)
	// Narrative parameters
	case "narrative_min_trend_strength":
		cfg.Narrative.MinTrendStrength.Value = value
	case "narrative_min_confidence":
		cfg.Narrative.MinConfidence.Value = value
	case "narrative_min_hit_rate":
		cfg.Narrative.MinHitRate.Value = value
	case "narrative_override_threshold":
		cfg.Narrative.OverrideThreshold.Value = value
	case "narrative_ai_revenue_growth_threshold":
		cfg.Narrative.AIRevenueGrowthThreshold.Value = value
	case "narrative_cowos_utilization_threshold":
		cfg.Narrative.CoWoSUtilizationThreshold.Value = value
	case "narrative_capex_growth_threshold":
		cfg.Narrative.CapexGrowthThreshold.Value = value
	case "narrative_us10y_change_bps_threshold":
		cfg.Narrative.US10YChangeBpsThreshold.Value = value
	case "narrative_dxy_change_pct_threshold":
		cfg.Narrative.DXYChangePctThreshold.Value = value
	case "narrative_geopolitical_gpr_threshold":
		cfg.Narrative.GeopoliticalGPRThreshold.Value = value
	case "narrative_oil_change_pct_threshold":
		cfg.Narrative.OilChangePctThreshold.Value = value
	case "narrative_jpy_change_pct_threshold":
		cfg.Narrative.JPYChangePctThreshold.Value = value
	case "narrative_vix_level_threshold":
		cfg.Narrative.VIXLevelThreshold.Value = value
	case "narrative_taiwan_stress_dxy_weight":
		cfg.Narrative.TaiwanStressDXYWeight.Value = value
	case "narrative_taiwan_stress_us10y_weight":
		cfg.Narrative.TaiwanStressUS10YWeight.Value = value
	case "narrative_taiwan_stress_foreign_weight":
		cfg.Narrative.TaiwanStressForeignWeight.Value = value
	case "narrative_taiwan_stress_vix_weight":
		cfg.Narrative.TaiwanStressVIXWeight.Value = value
	case "narrative_taiwan_stress_jpy_weight":
		cfg.Narrative.TaiwanStressJPYWeight.Value = value
	case "narrative_taiwan_stress_geo_weight":
		cfg.Narrative.TaiwanStressGeoWeight.Value = value
	case "narrative_model_lookback_days":
		cfg.Narrative.ModelLookbackDays.Value = int(value)
	case "narrative_model_hold_window_days":
		cfg.Narrative.ModelHoldWindowDays.Value = int(value)
	// Marketdata parameters
	case "marketdata_twse_api_rate_limit":
		cfg.Marketdata.TWSEAPIRateLimit.Value = value
	case "marketdata_twse_api_rate_burst":
		cfg.Marketdata.TWSEAPIRateBurst.Value = int(value)
	case "marketdata_twse_api_timeout_sec":
		cfg.Marketdata.TWSEAPITimeoutSec.Value = int(value)
	case "marketdata_fubon_intraday_limit":
		cfg.Marketdata.FubonIntradayLimit.Value = int(value)
	case "marketdata_fubon_historical_limit":
		cfg.Marketdata.FubonHistoricalLimit.Value = int(value)
	case "marketdata_fubon_api_timeout_sec":
		cfg.Marketdata.FubonAPITimeoutSec.Value = int(value)
	case "marketdata_tej_calls_per_second":
		cfg.Marketdata.TEJCallsPerSecond.Value = int(value)
	case "marketdata_tej_api_timeout_sec":
		cfg.Marketdata.TEJAPITimeoutSec.Value = int(value)
	case "marketdata_fugle_rate_limit":
		cfg.Marketdata.FugleRateLimit.Value = int(value)
	case "marketdata_fugle_api_timeout_sec":
		cfg.Marketdata.FugleAPITimeoutSec.Value = int(value)
	case "marketdata_max_retry_attempts":
		cfg.Marketdata.MaxRetryAttempts.Value = int(value)
	case "marketdata_retry_backoff_ms":
		cfg.Marketdata.RetryBackoffMs.Value = int(value)
	// Strategy parameters
	case "strategy_min_switch_interval_days":
		cfg.Strategy.MinSwitchIntervalDays.Value = int(value)
	case "strategy_switch_threshold":
		cfg.Strategy.SwitchThreshold.Value = value
	case "strategy_score_lookback_days":
		cfg.Strategy.ScoreLookbackDays.Value = int(value)
	default:
		return fmt.Errorf("unknown parameter: %s", name)
	}
	return nil
}

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
