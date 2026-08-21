package config

// paramAccessor defines the getter and setter for a single parameter.
// Get returns the current value; Set updates the config with the given float64 value.
// For int-typed parameters, Set handles the conversion internally.
type paramAccessor struct {
	get func(*ParametersConfig) float64
	set func(*ParametersConfig, float64)
}

// parameterTable is the single source of truth for all parameter access paths.
// Each parameter is declared once with its getter and setter.
// Adding a new parameter: add one entry here. No switch statements needed.
var parameterTable = map[string]paramAccessor{
	// ===== Darwinian parameters =====
	"darwinian_weight_min": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Darwinian.WeightMin.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.WeightMin.Value = v },
	},
	"darwinian_weight_max": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Darwinian.WeightMax.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.WeightMax.Value = v },
	},
	"darwinian_weight_neutral": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Darwinian.WeightNeutral.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.WeightNeutral.Value = v },
	},
	"darwinian_top_quartile_multiplier": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Darwinian.TopQuartileMultiplier.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.TopQuartileMultiplier.Value = v },
	},
	"darwinian_bottom_quartile_multiplier": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Darwinian.BottomQuartileMultiplier.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.BottomQuartileMultiplier.Value = v },
	},
	"darwinian_lookback_days": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Darwinian.LookbackDays.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.LookbackDays.Value = int(v) },
	},
	"darwinian_ema_alpha": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Darwinian.EMAAlpha.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.EMAAlpha.Value = v },
	},
	"darwinian_sharpe_normalize_denom": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Darwinian.SharpeNormalizeDenom.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.SharpeNormalizeDenom.Value = v },
	},
	"darwinian_max_performance_bonus_pct": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Darwinian.MaxPerformanceBonusPct.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.MaxPerformanceBonusPct.Value = v },
	},
	"darwinian_volatility_penalty_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Darwinian.VolatilityPenaltyThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.VolatilityPenaltyThreshold.Value = v },
	},
	"darwinian_volatility_penalty_multiplier": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Darwinian.VolatilityPenaltyMultiplier.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.VolatilityPenaltyMultiplier.Value = v },
	},
	"darwinian_risk_volatility_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Darwinian.RiskVolatilityThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.RiskVolatilityThreshold.Value = v },
	},
	"darwinian_risk_multiplier": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Darwinian.RiskMultiplier.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.RiskMultiplier.Value = v },
	},
	"darwinian_hit_rate_high_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Darwinian.HitRateHighThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.HitRateHighThreshold.Value = v },
	},
	"darwinian_hit_rate_low_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Darwinian.HitRateLowThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.HitRateLowThreshold.Value = v },
	},
	"darwinian_middle_tier_boost_multiplier": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Darwinian.MiddleTierBoostMultiplier.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.MiddleTierBoostMultiplier.Value = v },
	},
	"darwinian_middle_tier_cut_multiplier": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Darwinian.MiddleTierCutMultiplier.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.MiddleTierCutMultiplier.Value = v },
	},
	"darwinian_sharpe_min_sample_size": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Darwinian.SharpeMinSampleSize.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.SharpeMinSampleSize.Value = int(v) },
	},
	"darwinian_stddev_mean_ratio_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Darwinian.StdDevMeanRatioThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.StdDevMeanRatioThreshold.Value = v },
	},
	"darwinian_conviction_clamp_min": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Darwinian.ConvictionClampMin.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.ConvictionClampMin.Value = int(v) },
	},
	"darwinian_conviction_clamp_max": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Darwinian.ConvictionClampMax.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.ConvictionClampMax.Value = int(v) },
	},
	"darwinian_zero_signal_penalty_multiplier": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Darwinian.ZeroSignalPenaltyMultiplier.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.ZeroSignalPenaltyMultiplier.Value = v },
	},
	"darwinian_zero_signal_penalty_after_days": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Darwinian.ZeroSignalPenaltyAfterDays.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.ZeroSignalPenaltyAfterDays.Value = int(v) },
	},
	"darwinian_loss_penalty_multiplier": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Darwinian.LossPenaltyMultiplier.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.LossPenaltyMultiplier.Value = v },
	},
	"darwinian_weight_change_alert_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Darwinian.WeightChangeAlertThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Darwinian.WeightChangeAlertThreshold.Value = v },
	},

	// ===== Factor parameters =====
	"factor_momentum_lookback_days": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Factor.MomentumLookbackDays.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Factor.MomentumLookbackDays.Value = int(v) },
	},
	"factor_momentum_stddev_divisor": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Factor.MomentumStdDevDivisor.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Factor.MomentumStdDevDivisor.Value = v },
	},
	"factor_momentum_intraday_discount": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Factor.MomentumIntradayDiscount.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Factor.MomentumIntradayDiscount.Value = v },
	},
	"factor_momentum_intraday_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Factor.MomentumIntradayThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Factor.MomentumIntradayThreshold.Value = v },
	},
	"factor_value_pe_range_center": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Factor.ValuePERangeCenter.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Factor.ValuePERangeCenter.Value = v },
	},
	"factor_value_pe_range_width": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Factor.ValuePERangeWidth.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Factor.ValuePERangeWidth.Value = v },
	},
	"factor_value_pb_range_center": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Factor.ValuePBRangeCenter.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Factor.ValuePBRangeCenter.Value = v },
	},
	"factor_value_pb_range_width": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Factor.ValuePBRangeWidth.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Factor.ValuePBRangeWidth.Value = v },
	},
	"factor_value_ps_range_center": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Factor.ValuePSRangeCenter.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Factor.ValuePSRangeCenter.Value = v },
	},
	"factor_value_ps_range_width": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Factor.ValuePSRangeWidth.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Factor.ValuePSRangeWidth.Value = v },
	},
	"factor_quality_dividend_yield_cap": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Factor.QualityDividendYieldCap.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Factor.QualityDividendYieldCap.Value = v },
	},
	"factor_quality_volatility_std": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Factor.QualityVolatilityStd.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Factor.QualityVolatilityStd.Value = v },
	},
	"factor_quality_fallback_score": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Factor.QualityFallbackScore.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Factor.QualityFallbackScore.Value = v },
	},
	"factor_value_fallback_score": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Factor.ValueFallbackScore.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Factor.ValueFallbackScore.Value = v },
	},
	"factor_fallback_weight_reduction": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Factor.FallbackWeightReduction.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Factor.FallbackWeightReduction.Value = v },
	},

	// ===== Optimizer parameters =====
	"optimizer_max_position_pct": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Optimizer.MaxPositionPct.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Optimizer.MaxPositionPct.Value = v },
	},
	"optimizer_max_sector_pct": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Optimizer.MaxSectorPct.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Optimizer.MaxSectorPct.Value = v },
	},
	"optimizer_max_turnover_daily": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Optimizer.MaxTurnoverDaily.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Optimizer.MaxTurnoverDaily.Value = v },
	},
	"optimizer_target_beta": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Optimizer.TargetBeta.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Optimizer.TargetBeta.Value = v },
	},
	"optimizer_beta_range_min": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Optimizer.BetaRangeMin.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Optimizer.BetaRangeMin.Value = v },
	},
	"optimizer_beta_range_max": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Optimizer.BetaRangeMax.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Optimizer.BetaRangeMax.Value = v },
	},
	"optimizer_min_trade_size": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Optimizer.MinTradeSize.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Optimizer.MinTradeSize.Value = int(v) },
	},
	"optimizer_cash_reserve": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Optimizer.CashReserve.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Optimizer.CashReserve.Value = v },
	},

	// ===== Sizing parameters =====
	"sizing_kelly_fraction": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Sizing.KellyFraction.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Sizing.KellyFraction.Value = v },
	},
	"sizing_vol_lookback_days": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Sizing.VolLookbackDays.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Sizing.VolLookbackDays.Value = int(v) },
	},
	"sizing_max_position_by_adv": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Sizing.MaxPositionByADV.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Sizing.MaxPositionByADV.Value = v },
	},
	"sizing_max_drawdown_limit": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Sizing.MaxDrawdownLimit.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Sizing.MaxDrawdownLimit.Value = v },
	},
	"sizing_atr_multiplier": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Sizing.ATRMultiplier.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Sizing.ATRMultiplier.Value = v },
	},
	"sizing_correlation_penalty": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Sizing.CorrelationPenalty.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Sizing.CorrelationPenalty.Value = v },
	},
	"sizing_correlation_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Sizing.CorrelationThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Sizing.CorrelationThreshold.Value = v },
	},
	"sizing_default_win_rate": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Sizing.DefaultWinRate.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Sizing.DefaultWinRate.Value = v },
	},
	"sizing_default_payoff_ratio": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Sizing.DefaultPayoffRatio.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Sizing.DefaultPayoffRatio.Value = v },
	},
	"sizing_target_volatility": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Sizing.TargetVolatility.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Sizing.TargetVolatility.Value = v },
	},
	"sizing_vol_adjustment_min": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Sizing.VolAdjustmentMin.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Sizing.VolAdjustmentMin.Value = v },
	},
	"sizing_vol_adjustment_max": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Sizing.VolAdjustmentMax.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Sizing.VolAdjustmentMax.Value = v },
	},
	"sizing_atr_target_risk": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Sizing.ATRTargetRisk.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Sizing.ATRTargetRisk.Value = v },
	},
	"sizing_atr_adjustment_min": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Sizing.ATRAdjustmentMin.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Sizing.ATRAdjustmentMin.Value = v },
	},
	"sizing_atr_adjustment_max": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Sizing.ATRAdjustmentMax.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Sizing.ATRAdjustmentMax.Value = v },
	},
	"sizing_correlation_penalty_factor": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Sizing.CorrelationPenaltyFactor.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Sizing.CorrelationPenaltyFactor.Value = v },
	},
	"sizing_max_correlation_penalty": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Sizing.MaxCorrelationPenalty.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Sizing.MaxCorrelationPenalty.Value = v },
	},
	"sizing_default_volatility": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Sizing.DefaultVolatility.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Sizing.DefaultVolatility.Value = v },
	},
	"sizing_default_adv": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Sizing.DefaultADV.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Sizing.DefaultADV.Value = v },
	},
	"sizing_default_atr": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Sizing.DefaultATR.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Sizing.DefaultATR.Value = v },
	},

	// ===== Health parameters =====
	"health_mute_threshold": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Health.MuteThreshold.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Health.MuteThreshold.Value = int(v) },
	},
	"health_unmute_threshold": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Health.UnmuteThreshold.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Health.UnmuteThreshold.Value = int(v) },
	},
	"health_auto_recover_days": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Health.AutoRecoverDays.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Health.AutoRecoverDays.Value = int(v) },
	},
	"health_min_sample_size": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Health.MinSampleSize.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Health.MinSampleSize.Value = int(v) },
	},
	"health_negative_sharpe_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Health.NegativeSharpeThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Health.NegativeSharpeThreshold.Value = v },
	},
	"health_sharpe_weight": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Health.SharpeWeight.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Health.SharpeWeight.Value = v },
	},
	"health_hitrate_weight": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Health.HitRateWeight.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Health.HitRateWeight.Value = v },
	},
	"health_streak_weight": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Health.StreakWeight.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Health.StreakWeight.Value = v },
	},
	"health_max_sharpe": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Health.MaxSharpe.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Health.MaxSharpe.Value = v },
	},
	"health_min_sharpe": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Health.MinSharpe.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Health.MinSharpe.Value = v },
	},
	"health_streak_max": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Health.StreakMax.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Health.StreakMax.Value = int(v) },
	},

	// ===== GARCH parameters =====
	"garch_omega": {
		get: func(cfg *ParametersConfig) float64 { return cfg.GARCH.Omega.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.GARCH.Omega.Value = v },
	},
	"garch_alpha": {
		get: func(cfg *ParametersConfig) float64 { return cfg.GARCH.Alpha.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.GARCH.Alpha.Value = v },
	},
	"garch_beta": {
		get: func(cfg *ParametersConfig) float64 { return cfg.GARCH.Beta.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.GARCH.Beta.Value = v },
	},
	"garch_max_history": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.GARCH.MaxHistory.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.GARCH.MaxHistory.Value = int(v) },
	},
	"garch_correlation_min_days": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.GARCH.CorrelationMinDays.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.GARCH.CorrelationMinDays.Value = int(v) },
	},
	"garch_smoothing_factor": {
		get: func(cfg *ParametersConfig) float64 { return cfg.GARCH.SmoothingFactor.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.GARCH.SmoothingFactor.Value = v },
	},
	"garch_rebalance_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.GARCH.RebalanceThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.GARCH.RebalanceThreshold.Value = v },
	},
	"garch_min_forecast_days": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.GARCH.MinForecastDays.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.GARCH.MinForecastDays.Value = int(v) },
	},
	"garch_max_history_points": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.GARCH.MaxHistoryPoints.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.GARCH.MaxHistoryPoints.Value = int(v) },
	},
	"garch_high_vol_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.GARCH.HighVolThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.GARCH.HighVolThreshold.Value = v },
	},
	"garch_low_vol_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.GARCH.LowVolThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.GARCH.LowVolThreshold.Value = v },
	},
	"garch_reduce_magnitude": {
		get: func(cfg *ParametersConfig) float64 { return cfg.GARCH.ReduceMagnitude.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.GARCH.ReduceMagnitude.Value = v },
	},
	"garch_increase_magnitude": {
		get: func(cfg *ParametersConfig) float64 { return cfg.GARCH.IncreaseMagnitude.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.GARCH.IncreaseMagnitude.Value = v },
	},
	"garch_weekly_rebalance_days": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.GARCH.WeeklyRebalanceDays.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.GARCH.WeeklyRebalanceDays.Value = int(v) },
	},

	// ===== Experiment parameters =====
	"experiment_maturity_level1_observations": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Experiment.MaturityLevel1Observations.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Experiment.MaturityLevel1Observations.Value = int(v) },
	},
	"experiment_maturity_level2_observations": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Experiment.MaturityLevel2Observations.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Experiment.MaturityLevel2Observations.Value = int(v) },
	},
	"experiment_maturity_level3_observations": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Experiment.MaturityLevel3Observations.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Experiment.MaturityLevel3Observations.Value = int(v) },
	},
	"experiment_improvement_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Experiment.ImprovementThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Experiment.ImprovementThreshold.Value = v },
	},
	"experiment_welch_ttest_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Experiment.WelchTTestThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Experiment.WelchTTestThreshold.Value = v },
	},
	"experiment_drawdown_protection_ratio": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Experiment.DrawdownProtectionRatio.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Experiment.DrawdownProtectionRatio.Value = v },
	},
	"experiment_volatility_tolerance_ratio": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Experiment.VolatilityToleranceRatio.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Experiment.VolatilityToleranceRatio.Value = v },
	},
	"experiment_oos_window_days": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Experiment.OOSWindowDays.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Experiment.OOSWindowDays.Value = int(v) },
	},
	"experiment_sharpe_stability_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Experiment.SharpeStabilityThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Experiment.SharpeStabilityThreshold.Value = v },
	},
	"experiment_max_fallback_ratio": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Experiment.MaxFallbackRatio.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Experiment.MaxFallbackRatio.Value = v },
	},
	"experiment_walk_forward_embargo_days": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Experiment.WalkForwardEmbargoDays.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Experiment.WalkForwardEmbargoDays.Value = int(v) },
	},

	// ===== Baseline parameters =====
	"baseline_starting_cash": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Baseline.StartingCash.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Baseline.StartingCash.Value = v },
	},
	"baseline_max_position_weight": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Baseline.MaxPositionWeight.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Baseline.MaxPositionWeight.Value = v },
	},
	"baseline_max_open_positions": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Baseline.MaxOpenPositions.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Baseline.MaxOpenPositions.Value = int(v) },
	},
	"baseline_min_tradable_volume": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Baseline.MinTradableVolume.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Baseline.MinTradableVolume.Value = v },
	},
	"baseline_min_recommendation_conviction": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Baseline.MinRecommendationConviction.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Baseline.MinRecommendationConviction.Value = int(v) },
	},
	"baseline_transaction_cost_bps": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Baseline.TransactionCostBPS.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Baseline.TransactionCostBPS.Value = v },
	},
	"baseline_slippage_bps": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Baseline.SlippageBPS.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Baseline.SlippageBPS.Value = v },
	},
	"baseline_reserve_cash_fraction": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Baseline.ReserveCashFraction.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Baseline.ReserveCashFraction.Value = v },
	},

	// ===== Orchestrator parameters =====
	"orchestrator_conviction_floor_default": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Orchestrator.ConvictionFloorDefault.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Orchestrator.ConvictionFloorDefault.Value = int(v) },
	},
	"orchestrator_superinvestor_min_conviction": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Orchestrator.SuperinvestorMinConviction.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Orchestrator.SuperinvestorMinConviction.Value = int(v) },
	},
	"orchestrator_superinvestor_conviction_base": {
		get: func(cfg *ParametersConfig) float64 {
			return float64(cfg.Orchestrator.SuperinvestorConvictionBase.Value)
		},
		set: func(cfg *ParametersConfig, v float64) { cfg.Orchestrator.SuperinvestorConvictionBase.Value = int(v) },
	},
	"orchestrator_cro_zscore_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Orchestrator.CROZScoreThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Orchestrator.CROZScoreThreshold.Value = v },
	},
	"orchestrator_sector_concentration_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Orchestrator.SectorConcentrationThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Orchestrator.SectorConcentrationThreshold.Value = v },
	},
	"orchestrator_sector_concentration_threshold_high": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Orchestrator.SectorConcentrationThresholdHigh.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Orchestrator.SectorConcentrationThresholdHigh.Value = v },
	},
	"orchestrator_sector_conviction_multiplier": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Orchestrator.SectorConvictionMultiplier.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Orchestrator.SectorConvictionMultiplier.Value = v },
	},
	"orchestrator_crowded_conviction_multiplier": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Orchestrator.CrowdedConvictionMultiplier.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Orchestrator.CrowdedConvictionMultiplier.Value = v },
	},
	"orchestrator_factor_weight_momentum": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Orchestrator.FactorWeightMomentum.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Orchestrator.FactorWeightMomentum.Value = v },
	},
	"orchestrator_factor_weight_value": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Orchestrator.FactorWeightValue.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Orchestrator.FactorWeightValue.Value = v },
	},
	"orchestrator_factor_weight_quality": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Orchestrator.FactorWeightQuality.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Orchestrator.FactorWeightQuality.Value = v },
	},
	"orchestrator_factor_weight_agent": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Orchestrator.FactorWeightAgent.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Orchestrator.FactorWeightAgent.Value = v },
	},
	"orchestrator_prism_boost_multiplier": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Orchestrator.PRISMBoostMultiplier.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Orchestrator.PRISMBoostMultiplier.Value = v },
	},
	"orchestrator_prism_boost_min": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Orchestrator.PRISMBoostMin.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Orchestrator.PRISMBoostMin.Value = int(v) },
	},
	"orchestrator_prism_boost_max": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Orchestrator.PRISMBoostMax.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Orchestrator.PRISMBoostMax.Value = int(v) },
	},
	"orchestrator_promotion_min_observations": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Orchestrator.PromotionMinObservations.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Orchestrator.PromotionMinObservations.Value = int(v) },
	},
	"orchestrator_promotion_sharpe_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Orchestrator.PromotionSharpeThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Orchestrator.PromotionSharpeThreshold.Value = v },
	},
	"orchestrator_promotion_hitrate_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Orchestrator.PromotionHitRateThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Orchestrator.PromotionHitRateThreshold.Value = v },
	},
	"orchestrator_rejection_sharpe_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Orchestrator.RejectionSharpeThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Orchestrator.RejectionSharpeThreshold.Value = v },
	},
	"orchestrator_rejection_hitrate_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Orchestrator.RejectionHitRateThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Orchestrator.RejectionHitRateThreshold.Value = v },
	},

	// ===== Risk parameters =====
	"risk_var_confidence_level": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Risk.VaRConfidenceLevel.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Risk.VaRConfidenceLevel.Value = v },
	},
	"risk_var_secondary_confidence": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Risk.VaRSecondaryConfidence.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Risk.VaRSecondaryConfidence.Value = v },
	},
	"risk_var_alert_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Risk.VaRAlertThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Risk.VaRAlertThreshold.Value = v },
	},
	"risk_var_critical_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Risk.VaRCriticalThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Risk.VaRCriticalThreshold.Value = v },
	},
	"risk_consecutive_loss_limit": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Risk.ConsecutiveLossLimit.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Risk.ConsecutiveLossLimit.Value = int(v) },
	},
	"risk_max_drawdown_pct": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Risk.MaxDrawdownPct.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Risk.MaxDrawdownPct.Value = v },
	},
	"risk_max_position_size": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Risk.MaxPositionSize.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Risk.MaxPositionSize.Value = v },
	},
	"risk_max_daily_loss_pct": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Risk.MaxDailyLossPct.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Risk.MaxDailyLossPct.Value = v },
	},
	"risk_stop_loss": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Risk.StopLoss.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Risk.StopLoss.Value = v },
	},
	"risk_take_profit": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Risk.TakeProfit.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Risk.TakeProfit.Value = v },
	},
	"risk_max_loss_per_trade": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Risk.MaxLossPerTrade.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Risk.MaxLossPerTrade.Value = v },
	},
	"risk_max_total_exposure": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Risk.MaxTotalExposure.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Risk.MaxTotalExposure.Value = v },
	},

	// ===== Realtime parameters =====
	"realtime_volatility_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Realtime.VolatilityThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Realtime.VolatilityThreshold.Value = v },
	},
	"realtime_volume_spike_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Realtime.VolumeSpikeThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Realtime.VolumeSpikeThreshold.Value = v },
	},
	"realtime_price_change_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Realtime.PriceChangeThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Realtime.PriceChangeThreshold.Value = v },
	},
	"realtime_min_confidence": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Realtime.MinConfidence.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Realtime.MinConfidence.Value = v },
	},
	"realtime_weight_adjustment_rate": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Realtime.WeightAdjustmentRate.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Realtime.WeightAdjustmentRate.Value = v },
	},
	"realtime_max_weight_change": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Realtime.MaxWeightChange.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Realtime.MaxWeightChange.Value = v },
	},
	"realtime_min_weight": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Realtime.MinWeight.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Realtime.MinWeight.Value = v },
	},
	"realtime_update_interval_ms": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Realtime.UpdateIntervalMs.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Realtime.UpdateIntervalMs.Value = int(v) },
	},

	// ===== Janus parameters =====
	"janus_short_window_days": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Janus.ShortWindowDays.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Janus.ShortWindowDays.Value = int(v) },
	},
	"janus_medium_window_days": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Janus.MediumWindowDays.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Janus.MediumWindowDays.Value = int(v) },
	},
	"janus_long_window_days": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Janus.LongWindowDays.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Janus.LongWindowDays.Value = int(v) },
	},
	"janus_max_history_days": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Janus.MaxHistoryDays.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Janus.MaxHistoryDays.Value = int(v) },
	},
	"janus_min_weight": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Janus.MinWeight.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Janus.MinWeight.Value = v },
	},
	"janus_max_weight": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Janus.MaxWeight.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Janus.MaxWeight.Value = v },
	},
	"janus_novel_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Janus.NovelThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Janus.NovelThreshold.Value = v },
	},
	"janus_historical_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Janus.HistoricalThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Janus.HistoricalThreshold.Value = v },
	},
	"janus_epsilon_weight": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Janus.EpsilonWeight.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Janus.EpsilonWeight.Value = v },
	},
	"janus_short_window_blend": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Janus.ShortWindowBlend.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Janus.ShortWindowBlend.Value = v },
	},
	"janus_medium_window_blend": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Janus.MediumWindowBlend.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Janus.MediumWindowBlend.Value = v },
	},
	"janus_long_window_blend": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Janus.LongWindowBlend.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Janus.LongWindowBlend.Value = v },
	},
	"janus_health_stale_hours": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Janus.HealthStaleHours.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Janus.HealthStaleHours.Value = int(v) },
	},
	"janus_health_warn_hours": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Janus.HealthWarnHours.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Janus.HealthWarnHours.Value = int(v) },
	},

	// ===== Narrative parameters =====
	"narrative_min_trend_strength": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Narrative.MinTrendStrength.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.MinTrendStrength.Value = v },
	},
	"narrative_min_confidence": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Narrative.MinConfidence.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.MinConfidence.Value = v },
	},
	"narrative_min_hit_rate": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Narrative.MinHitRate.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.MinHitRate.Value = v },
	},
	"narrative_override_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Narrative.OverrideThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.OverrideThreshold.Value = v },
	},
	"narrative_ai_revenue_growth_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Narrative.AIRevenueGrowthThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.AIRevenueGrowthThreshold.Value = v },
	},
	"narrative_cowos_utilization_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Narrative.CoWoSUtilizationThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.CoWoSUtilizationThreshold.Value = v },
	},
	"narrative_capex_growth_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Narrative.CapexGrowthThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.CapexGrowthThreshold.Value = v },
	},
	"narrative_us10y_change_bps_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Narrative.US10YChangeBpsThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.US10YChangeBpsThreshold.Value = v },
	},
	"narrative_dxy_change_pct_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Narrative.DXYChangePctThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.DXYChangePctThreshold.Value = v },
	},
	"narrative_geopolitical_gpr_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Narrative.GeopoliticalGPRThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.GeopoliticalGPRThreshold.Value = v },
	},
	"narrative_oil_change_pct_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Narrative.OilChangePctThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.OilChangePctThreshold.Value = v },
	},
	"narrative_jpy_change_pct_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Narrative.JPYChangePctThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.JPYChangePctThreshold.Value = v },
	},
	"narrative_vix_level_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Narrative.VIXLevelThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.VIXLevelThreshold.Value = v },
	},
	"narrative_taiwan_stress_dxy_weight": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Narrative.TaiwanStressDXYWeight.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.TaiwanStressDXYWeight.Value = v },
	},
	"narrative_taiwan_stress_us10y_weight": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Narrative.TaiwanStressUS10YWeight.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.TaiwanStressUS10YWeight.Value = v },
	},
	"narrative_taiwan_stress_foreign_weight": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Narrative.TaiwanStressForeignWeight.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.TaiwanStressForeignWeight.Value = v },
	},
	"narrative_taiwan_stress_vix_weight": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Narrative.TaiwanStressVIXWeight.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.TaiwanStressVIXWeight.Value = v },
	},
	"narrative_taiwan_stress_jpy_weight": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Narrative.TaiwanStressJPYWeight.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.TaiwanStressJPYWeight.Value = v },
	},
	"narrative_taiwan_stress_geo_weight": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Narrative.TaiwanStressGeoWeight.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.TaiwanStressGeoWeight.Value = v },
	},
	"narrative_model_lookback_days": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Narrative.ModelLookbackDays.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.ModelLookbackDays.Value = int(v) },
	},
	"narrative_model_hold_window_days": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Narrative.ModelHoldWindowDays.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Narrative.ModelHoldWindowDays.Value = int(v) },
	},

	// ===== Marketdata parameters =====
	"marketdata_twse_api_rate_limit": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Marketdata.TWSEAPIRateLimit.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Marketdata.TWSEAPIRateLimit.Value = v },
	},
	"marketdata_twse_api_rate_burst": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Marketdata.TWSEAPIRateBurst.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Marketdata.TWSEAPIRateBurst.Value = int(v) },
	},
	"marketdata_twse_api_timeout_sec": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Marketdata.TWSEAPITimeoutSec.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Marketdata.TWSEAPITimeoutSec.Value = int(v) },
	},
	"marketdata_fubon_intraday_limit": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Marketdata.FubonIntradayLimit.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Marketdata.FubonIntradayLimit.Value = int(v) },
	},
	"marketdata_fubon_historical_limit": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Marketdata.FubonHistoricalLimit.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Marketdata.FubonHistoricalLimit.Value = int(v) },
	},
	"marketdata_fubon_api_timeout_sec": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Marketdata.FubonAPITimeoutSec.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Marketdata.FubonAPITimeoutSec.Value = int(v) },
	},
	"marketdata_tej_calls_per_second": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Marketdata.TEJCallsPerSecond.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Marketdata.TEJCallsPerSecond.Value = int(v) },
	},
	"marketdata_tej_api_timeout_sec": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Marketdata.TEJAPITimeoutSec.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Marketdata.TEJAPITimeoutSec.Value = int(v) },
	},
	"marketdata_fugle_rate_limit": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Marketdata.FugleRateLimit.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Marketdata.FugleRateLimit.Value = int(v) },
	},
	"marketdata_fugle_api_timeout_sec": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Marketdata.FugleAPITimeoutSec.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Marketdata.FugleAPITimeoutSec.Value = int(v) },
	},
	"marketdata_max_retry_attempts": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Marketdata.MaxRetryAttempts.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Marketdata.MaxRetryAttempts.Value = int(v) },
	},
	"marketdata_retry_backoff_ms": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Marketdata.RetryBackoffMs.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Marketdata.RetryBackoffMs.Value = int(v) },
	},

	// ===== Strategy parameters =====
	"strategy_min_switch_interval_days": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Strategy.MinSwitchIntervalDays.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Strategy.MinSwitchIntervalDays.Value = int(v) },
	},
	"strategy_switch_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Strategy.SwitchThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Strategy.SwitchThreshold.Value = v },
	},
	"strategy_score_lookback_days": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Strategy.ScoreLookbackDays.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Strategy.ScoreLookbackDays.Value = int(v) },
	},

	// ===== Engine — MacroRisk parameters =====
	"engine_macro_risk_carry_trade_unwind_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.MacroRisk.CarryTradeUnwindThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.MacroRisk.CarryTradeUnwindThreshold.Value = v },
	},
	"engine_macro_risk_vix_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.MacroRisk.VIXThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.MacroRisk.VIXThreshold.Value = v },
	},
	"engine_macro_risk_us10y_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.MacroRisk.US10YThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.MacroRisk.US10YThreshold.Value = v },
	},
	"engine_macro_risk_oil_shock_threshold_pct": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.MacroRisk.OilShockThresholdPct.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.MacroRisk.OilShockThresholdPct.Value = v },
	},
	"engine_macro_risk_gold_surge_threshold_pct": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.MacroRisk.GoldSurgeThresholdPct.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.MacroRisk.GoldSurgeThresholdPct.Value = v },
	},
	"engine_macro_risk_dxy_surge_threshold_pct": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.MacroRisk.DXYSurgeThresholdPct.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.MacroRisk.DXYSurgeThresholdPct.Value = v },
	},
	"engine_macro_risk_twd_stress_threshold_pct": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.MacroRisk.TWDStressThresholdPct.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.MacroRisk.TWDStressThresholdPct.Value = v },
	},
	"engine_macro_risk_outflow_prob_base": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.MacroRisk.OutflowProbBase.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.MacroRisk.OutflowProbBase.Value = v },
	},
	"engine_macro_risk_outflow_prob_max": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.MacroRisk.OutflowProbMax.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.MacroRisk.OutflowProbMax.Value = v },
	},

	// ===== Engine — StructuralTrend parameters =====
	"engine_structural_trend_min_trend_strength": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.StructuralTrend.MinTrendStrength.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.StructuralTrend.MinTrendStrength.Value = v },
	},
	"engine_structural_trend_min_confidence": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.StructuralTrend.MinConfidence.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.StructuralTrend.MinConfidence.Value = v },
	},
	"engine_structural_trend_min_hit_rate": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.StructuralTrend.MinHitRate.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.StructuralTrend.MinHitRate.Value = v },
	},
	"engine_structural_trend_override_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.StructuralTrend.OverrideThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.StructuralTrend.OverrideThreshold.Value = v },
	},
	"engine_structural_trend_ai_revenue_growth_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.StructuralTrend.AIRevenueGrowthThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.StructuralTrend.AIRevenueGrowthThreshold.Value = v },
	},
	"engine_structural_trend_cowos_utilization_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.StructuralTrend.CoWoSUtilizationThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.StructuralTrend.CoWoSUtilizationThreshold.Value = v },
	},
	"engine_structural_trend_capex_growth_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.StructuralTrend.CapexGrowthThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.StructuralTrend.CapexGrowthThreshold.Value = v },
	},
	"engine_structural_trend_semiconductor_index_threshold": {
		get: func(cfg *ParametersConfig) float64 {
			return cfg.Engine.StructuralTrend.SemiconductorIndexThreshold.Value
		},
		set: func(cfg *ParametersConfig, v float64) {
			cfg.Engine.StructuralTrend.SemiconductorIndexThreshold.Value = v
		},
	},

	// ===== Engine — Drawdown parameters =====
	"engine_drawdown_orange_override_min_score": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.Drawdown.OrangeOverrideMinScore.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.Drawdown.OrangeOverrideMinScore.Value = v },
	},
	"engine_drawdown_red_override_min_score": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.Drawdown.RedOverrideMinScore.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.Drawdown.RedOverrideMinScore.Value = v },
	},

	// ===== Engine — Executors parameters =====
	"engine_executors_vix_momentum_crash_threshold": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.Executors.VIXMomentumCrashThreshold.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.Executors.VIXMomentumCrashThreshold.Value = v },
	},
	"engine_executors_crowding_penalty_agents_3": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.Executors.CrowdingPenaltyAgents3.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.Executors.CrowdingPenaltyAgents3.Value = v },
	},
	"engine_executors_crowding_penalty_agents_4": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.Executors.CrowdingPenaltyAgents4.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.Executors.CrowdingPenaltyAgents4.Value = v },
	},
	"engine_executors_min_trade_amount": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.Executors.MinTradeAmount.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.Executors.MinTradeAmount.Value = v },
	},
	"engine_executors_conviction_floor_default": {
		get: func(cfg *ParametersConfig) float64 { return float64(cfg.Engine.Executors.ConvictionFloorDefault.Value) },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.Executors.ConvictionFloorDefault.Value = int(v) },
	},

	// ===== Engine — Simulation parameters =====
	"engine_simulation_neutral_regime_sizing_factor": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Engine.Simulation.NeutralRegimeSizingFactor.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.Engine.Simulation.NeutralRegimeSizingFactor.Value = v },
	},

	// ===== FactorWeight strategy delta parameters =====
	"factor_weight_conservative_value": {
		get: func(cfg *ParametersConfig) float64 { return cfg.FactorWeight.ConservativeValue.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.FactorWeight.ConservativeValue.Value = v },
	},
	"factor_weight_conservative_quality": {
		get: func(cfg *ParametersConfig) float64 { return cfg.FactorWeight.ConservativeQuality.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.FactorWeight.ConservativeQuality.Value = v },
	},
	"factor_weight_conservative_momentum": {
		get: func(cfg *ParametersConfig) float64 { return cfg.FactorWeight.ConservativeMomentum.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.FactorWeight.ConservativeMomentum.Value = v },
	},
	"factor_weight_aggressive_momentum": {
		get: func(cfg *ParametersConfig) float64 { return cfg.FactorWeight.AggressiveMomentum.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.FactorWeight.AggressiveMomentum.Value = v },
	},
	"factor_weight_aggressive_inst_sent": {
		get: func(cfg *ParametersConfig) float64 { return cfg.FactorWeight.AggressiveInstSent.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.FactorWeight.AggressiveInstSent.Value = v },
	},
	"factor_weight_aggressive_value": {
		get: func(cfg *ParametersConfig) float64 { return cfg.FactorWeight.AggressiveValue.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.FactorWeight.AggressiveValue.Value = v },
	},
	"factor_weight_aggressive_quality": {
		get: func(cfg *ParametersConfig) float64 { return cfg.FactorWeight.AggressiveQuality.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.FactorWeight.AggressiveQuality.Value = v },
	},
	"factor_weight_risk_on_momentum": {
		get: func(cfg *ParametersConfig) float64 { return cfg.FactorWeight.RiskOnMomentum.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.FactorWeight.RiskOnMomentum.Value = v },
	},
	"factor_weight_risk_on_quality": {
		get: func(cfg *ParametersConfig) float64 { return cfg.FactorWeight.RiskOnQuality.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.FactorWeight.RiskOnQuality.Value = v },
	},
	"factor_weight_risk_off_momentum": {
		get: func(cfg *ParametersConfig) float64 { return cfg.FactorWeight.RiskOffMomentum.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.FactorWeight.RiskOffMomentum.Value = v },
	},
	"factor_weight_risk_off_quality": {
		get: func(cfg *ParametersConfig) float64 { return cfg.FactorWeight.RiskOffQuality.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.FactorWeight.RiskOffQuality.Value = v },
	},
	"factor_weight_risk_off_liquidity": {
		get: func(cfg *ParametersConfig) float64 { return cfg.FactorWeight.RiskOffLiquidity.Value },
		set: func(cfg *ParametersConfig, v float64) { cfg.FactorWeight.RiskOffLiquidity.Value = v },
	},
	"industry_linkage_recession_shock_amplifier": {
		get: func(cfg *ParametersConfig) float64 { return cfg.Industry.LinkageParams.Value.RecessionShockAmplifier },
		set: func(cfg *ParametersConfig, v float64) { cfg.Industry.LinkageParams.Value.RecessionShockAmplifier = v },
	},
}

// stringParamAccessor provides get/set accessors for string-type parameters.
type stringParamAccessor struct {
	get func(*ParametersConfig) string
	set func(*ParametersConfig, string)
}

var stringParameterTable = map[string]stringParamAccessor{
	"precious_metals_central_bank_buying_trend": {
		get: func(cfg *ParametersConfig) string { return cfg.PreciousMetals.CentralBankBuyingTrend.Value },
		set: func(cfg *ParametersConfig, v string) { cfg.PreciousMetals.CentralBankBuyingTrend.Value = v },
	},
}

// boolParamAccessor provides get/set accessors for bool-type parameters.
// The table is intentionally empty — bool parameter registration is planned
// but not yet implemented. See SetBoolParameter in inference.go.
//
//nolint:unused // scaffolding for planned bool parameter support
type boolParamAccessor struct {
	//nolint:unused
	get func(*ParametersConfig) bool
	//nolint:unused
	set func(*ParametersConfig, bool)
}

//nolint:unused // scaffolding, populated when bool params are registered
var boolParameterTable = map[string]boolParamAccessor{}

var (
	_ = boolParamAccessor{}.get // prevent unused-field warning on scaffolding
	_ = boolParamAccessor{}.set // prevent unused-field warning on scaffolding
)

var _ = func() int {
	_ = len(mapParamPrefixes)
	return len(mapParamPrefixes)
}

var mapParamPrefixes = []mapParamPrefix{
	{prefix: "factor_institutional_sentiment_weights_", getMap: func(cfg *ParametersConfig) map[string]float64 {
		return cfg.Factor.InstitutionalSentimentWeights.Value
	}, setMap: func(cfg *ParametersConfig, m map[string]float64) {
		cfg.Factor.InstitutionalSentimentWeights.Value = m
	}},
	{prefix: "optimizer_factor_weights_", getMap: func(cfg *ParametersConfig) map[string]float64 {
		return cfg.Optimizer.FactorWeights.Value
	}, setMap: func(cfg *ParametersConfig, m map[string]float64) {
		cfg.Optimizer.FactorWeights.Value = m
	}},
	{prefix: "risk_sector_constraints_risk_off_", getMap: func(cfg *ParametersConfig) map[string]float64 {
		return cfg.Risk.SectorConstraintsRiskOff.Value
	}, setMap: func(cfg *ParametersConfig, m map[string]float64) {
		cfg.Risk.SectorConstraintsRiskOff.Value = m
	}},
	{prefix: "risk_sector_constraints_carry_trade_", getMap: func(cfg *ParametersConfig) map[string]float64 {
		return cfg.Risk.SectorConstraintsCarryTrade.Value
	}, setMap: func(cfg *ParametersConfig, m map[string]float64) {
		cfg.Risk.SectorConstraintsCarryTrade.Value = m
	}},
	{prefix: "risk_sector_constraints_sector_rotation_", getMap: func(cfg *ParametersConfig) map[string]float64 {
		return cfg.Risk.SectorConstraintsSectorRotation.Value
	}, setMap: func(cfg *ParametersConfig, m map[string]float64) {
		cfg.Risk.SectorConstraintsSectorRotation.Value = m
	}},
	{prefix: "narrative_event_ttl_multiplier_", getMap: func(cfg *ParametersConfig) map[string]float64 {
		return cfg.Narrative.EventTTLMultiplier.Value
	}, setMap: func(cfg *ParametersConfig, m map[string]float64) {
		cfg.Narrative.EventTTLMultiplier.Value = m
	}},
	// DEPRECATED: industry_sector_weights_* table entry removed; use sector_allocation.base_weights
	{prefix: "drawdown_sector_constraints_risk_off_", getMap: func(cfg *ParametersConfig) map[string]float64 {
		return cfg.Drawdown.SectorConstraintsRiskOff.Value
	}, setMap: func(cfg *ParametersConfig, m map[string]float64) {
		cfg.Drawdown.SectorConstraintsRiskOff.Value = m
	}},
	{prefix: "drawdown_sector_constraints_carry_trade_unwind_", getMap: func(cfg *ParametersConfig) map[string]float64 {
		return cfg.Drawdown.SectorConstraintsCarryTradeUnwind.Value
	}, setMap: func(cfg *ParametersConfig, m map[string]float64) {
		cfg.Drawdown.SectorConstraintsCarryTradeUnwind.Value = m
	}},
	{prefix: "drawdown_sector_constraints_sector_rotation_", getMap: func(cfg *ParametersConfig) map[string]float64 {
		return cfg.Drawdown.SectorConstraintsSectorRotation.Value
	}, setMap: func(cfg *ParametersConfig, m map[string]float64) {
		cfg.Drawdown.SectorConstraintsSectorRotation.Value = m
	}},
	// DEPRECATED: orchestrator_sector_rotation_base_allocations_* table entry removed; use sector_allocation.base_weights
}

// mapParamPrefix describes a dot-notation map parameter prefix and its accessors.
type mapParamPrefix struct {
	prefix string
	getMap func(*ParametersConfig) map[string]float64
	setMap func(*ParametersConfig, map[string]float64)
}
