// SPDX-License-Identifier: AGPL-3.0

package config

import (
	"fmt"
	"math"
)

func (p *ParametersConfig) validateAlert() error {
	if p.Alert.MinCashThreshold.Value < 0 {
		return fmt.Errorf("alert.min_cash_threshold (%.2f) must be non-negative", p.Alert.MinCashThreshold.Value)
	}
	if p.Alert.MaxPositionsCount.Value < 1 {
		return fmt.Errorf("alert.max_positions_count (%d) must be >= 1", p.Alert.MaxPositionsCount.Value)
	}
	if p.Alert.MaxPositionWeightPct.Value <= 0 || p.Alert.MaxPositionWeightPct.Value > 1 {
		return fmt.Errorf("alert.max_position_weight_pct (%.3f) must be in (0,1]", p.Alert.MaxPositionWeightPct.Value)
	}
	if p.Alert.MaxUnrealizedLossPct.Value >= 0 {
		return fmt.Errorf("alert.max_unrealized_loss_pct (%.3f) must be negative", p.Alert.MaxUnrealizedLossPct.Value)
	}
	if p.Alert.DailyLossWarningPct.Value >= 0 {
		return fmt.Errorf("alert.daily_loss_warning_pct (%.3f) must be negative", p.Alert.DailyLossWarningPct.Value)
	}
	if p.Alert.DailyLossCriticalPct.Value >= p.Alert.DailyLossWarningPct.Value {
		return fmt.Errorf("alert.daily_loss_critical_pct (%.3f) must be < daily_loss_warning_pct (%.3f)", p.Alert.DailyLossCriticalPct.Value, p.Alert.DailyLossWarningPct.Value)
	}
	if p.Alert.RuleEngineIntervalSec.Value < 1 {
		return fmt.Errorf("alert.rule_engine_interval_sec (%d) must be >= 1", p.Alert.RuleEngineIntervalSec.Value)
	}
	if p.Alert.RuleEngineCooldownSec.Value < 1 {
		return fmt.Errorf("alert.rule_engine_cooldown_sec (%d) must be >= 1", p.Alert.RuleEngineCooldownSec.Value)
	}
	if p.Alert.SlippageWarningBps.Value <= 0 {
		return fmt.Errorf("alert.slippage_warning_bps (%.2f) must be positive", p.Alert.SlippageWarningBps.Value)
	}
	if p.Alert.SlippageErrorBps.Value <= 0 {
		return fmt.Errorf("alert.slippage_error_bps (%.2f) must be positive", p.Alert.SlippageErrorBps.Value)
	}
	if p.Alert.SlippageErrorBps.Value < p.Alert.SlippageWarningBps.Value {
		return fmt.Errorf("alert.slippage_error_bps (%.2f) must be >= slippage_warning_bps (%.2f)", p.Alert.SlippageErrorBps.Value, p.Alert.SlippageWarningBps.Value)
	}
	if p.Alert.SystemMetricsIntervalSec.Value < 1 {
		return fmt.Errorf("alert.system_metrics_interval_sec (%d) must be >= 1", p.Alert.SystemMetricsIntervalSec.Value)
	}
	if p.Alert.MinScreeningRate.Value < 0 || p.Alert.MinScreeningRate.Value > 1 {
		return fmt.Errorf("alert.min_screening_rate (%.3f) must be in [0,1]", p.Alert.MinScreeningRate.Value)
	}
	if p.Alert.MaxAlertTriggerRate.Value < 1 {
		return fmt.Errorf("alert.max_alert_trigger_rate (%.0f) must be >= 1", p.Alert.MaxAlertTriggerRate.Value)
	}
	if p.Alert.MaxUnacknowledgedAlerts.Value < 1 {
		return fmt.Errorf("alert.max_unacknowledged_alerts (%d) must be >= 1", p.Alert.MaxUnacknowledgedAlerts.Value)
	}
	if p.Alert.HeartbeatTTLMinutes.Value < 1 {
		return fmt.Errorf("alert.heartbeat_ttl_minutes (%d) must be >= 1", p.Alert.HeartbeatTTLMinutes.Value)
	}
	if p.Alert.AlertSLACriticalSec.Value < 1 {
		return fmt.Errorf("alert.alert_sla_critical_sec (%d) must be >= 1", p.Alert.AlertSLACriticalSec.Value)
	}
	if p.Alert.AlertSLAErrorSec.Value < 1 {
		return fmt.Errorf("alert.alert_sla_error_sec (%d) must be >= 1", p.Alert.AlertSLAErrorSec.Value)
	}
	if p.Alert.AlertSLAWarningSec.Value < 1 {
		return fmt.Errorf("alert.alert_sla_warning_sec (%d) must be >= 1", p.Alert.AlertSLAWarningSec.Value)
	}
	if p.Alert.AlertSLACriticalSec.Value >= p.Alert.AlertSLAErrorSec.Value {
		return fmt.Errorf("alert.alert_sla_critical_sec (%d) must be < alert_sla_error_sec (%d)",
			p.Alert.AlertSLACriticalSec.Value, p.Alert.AlertSLAErrorSec.Value)
	}
	if p.Alert.AlertSLAErrorSec.Value >= p.Alert.AlertSLAWarningSec.Value {
		return fmt.Errorf("alert.alert_sla_error_sec (%d) must be < alert_sla_warning_sec (%d)",
			p.Alert.AlertSLAErrorSec.Value, p.Alert.AlertSLAWarningSec.Value)
	}
	return nil
}

// Validate checks that all parameters are within acceptable ranges.
func (p *ParametersConfig) Validate() error {
	if p.Darwinian.WeightMin.Value >= p.Darwinian.WeightMax.Value {
		return fmt.Errorf("darwinian.weight_min (%.3f) must be less than weight_max (%.3f)",
			p.Darwinian.WeightMin.Value, p.Darwinian.WeightMax.Value)
	}
	if p.Darwinian.EMAAlpha.Value < 0 || p.Darwinian.EMAAlpha.Value > 1 {
		return fmt.Errorf("darwinian.ema_alpha must be in [0,1], got %.3f", p.Darwinian.EMAAlpha.Value)
	}
	if p.Sizing.KellyFraction.Value <= 0 || p.Sizing.KellyFraction.Value > 1 {
		return fmt.Errorf("sizing.kelly_fraction must be in (0,1], got %.3f", p.Sizing.KellyFraction.Value)
	}
	if p.Health.SharpeWeight.Value+p.Health.HitRateWeight.Value+p.Health.StreakWeight.Value != 1.0 {
		return fmt.Errorf("health weights must sum to 1.0, got %.3f",
			p.Health.SharpeWeight.Value+p.Health.HitRateWeight.Value+p.Health.StreakWeight.Value)
	}

	// GARCH stationarity constraint
	if p.GARCH.Alpha.Value < 0 || p.GARCH.Beta.Value < 0 {
		return fmt.Errorf("garch.alpha (%.3f) and garch.beta (%.3f) must be non-negative", p.GARCH.Alpha.Value, p.GARCH.Beta.Value)
	}
	if p.GARCH.Alpha.Value+p.GARCH.Beta.Value >= 1.0 {
		return fmt.Errorf("garch.alpha (%.3f) + garch.beta (%.3f) must be < 1.0 for stationarity", p.GARCH.Alpha.Value, p.GARCH.Beta.Value)
	}
	if p.GARCH.Omega.Value <= 0 {
		return fmt.Errorf("garch.omega (%.6f) must be positive", p.GARCH.Omega.Value)
	}

	// Sizing constraints
	if p.Sizing.MaxPositionByADV.Value <= 0 || p.Sizing.MaxPositionByADV.Value >= 1 {
		return fmt.Errorf("sizing.max_position_by_adv (%.3f) must be in (0,1)", p.Sizing.MaxPositionByADV.Value)
	}
	if p.Sizing.ATRMultiplier.Value <= 0 {
		return fmt.Errorf("sizing.atr_multiplier (%.3f) must be positive", p.Sizing.ATRMultiplier.Value)
	}
	if p.Sizing.CorrelationThreshold.Value < 0 || p.Sizing.CorrelationThreshold.Value > 1 {
		return fmt.Errorf("sizing.correlation_threshold (%.3f) must be in [0,1]", p.Sizing.CorrelationThreshold.Value)
	}
	if p.Sizing.TargetVolatility.Value <= 0 {
		return fmt.Errorf("sizing.target_volatility (%.3f) must be positive", p.Sizing.TargetVolatility.Value)
	}
	if p.Sizing.MaxDrawdownLimit.Value <= 0 {
		return fmt.Errorf("sizing.max_drawdown_limit (%.3f) must be positive", p.Sizing.MaxDrawdownLimit.Value)
	}

	// Darwinian constraints
	if p.Darwinian.WeightMin.Value <= 0 {
		return fmt.Errorf("darwinian.weight_min (%.3f) must be positive", p.Darwinian.WeightMin.Value)
	}
	if p.Darwinian.WeightMax.Value <= 0 {
		return fmt.Errorf("darwinian.weight_max (%.3f) must be positive", p.Darwinian.WeightMax.Value)
	}
	if p.Darwinian.TopQuartileMultiplier.Value <= 0 {
		return fmt.Errorf("darwinian.top_quartile_multiplier (%.3f) must be positive", p.Darwinian.TopQuartileMultiplier.Value)
	}
	if p.Darwinian.BottomQuartileMultiplier.Value <= 0 {
		return fmt.Errorf("darwinian.bottom_quartile_multiplier (%.3f) must be positive", p.Darwinian.BottomQuartileMultiplier.Value)
	}
	if p.Darwinian.LookbackDays.Value <= 0 {
		return fmt.Errorf("darwinian.lookback_days (%d) must be positive", p.Darwinian.LookbackDays.Value)
	}
	if p.Darwinian.ZeroSignalPenaltyMultiplier.Value <= 0 || p.Darwinian.ZeroSignalPenaltyMultiplier.Value > 1 {
		return fmt.Errorf("darwinian.zero_signal_penalty_multiplier (%.3f) must be in (0,1]", p.Darwinian.ZeroSignalPenaltyMultiplier.Value)
	}
	if p.Darwinian.ZeroSignalPenaltyAfterDays.Value < 1 {
		return fmt.Errorf("darwinian.zero_signal_penalty_after_days (%d) must be >= 1", p.Darwinian.ZeroSignalPenaltyAfterDays.Value)
	}
	if p.Darwinian.LossPenaltyMultiplier.Value <= 0 || p.Darwinian.LossPenaltyMultiplier.Value > 1 {
		return fmt.Errorf("darwinian.loss_penalty_multiplier (%.3f) must be in (0,1]", p.Darwinian.LossPenaltyMultiplier.Value)
	}
	if p.Darwinian.WeightChangeAlertThreshold.Value < 0 {
		return fmt.Errorf("darwinian.weight_change_alert_threshold (%.3f) must be non-negative", p.Darwinian.WeightChangeAlertThreshold.Value)
	}

	// Factor constraints
	if p.Factor.MomentumStdDevDivisor.Value <= 0 {
		return fmt.Errorf("factor.momentum_stddev_divisor (%.3f) must be positive", p.Factor.MomentumStdDevDivisor.Value)
	}
	if p.Factor.MomentumIntradayDiscount.Value < 0 || p.Factor.MomentumIntradayDiscount.Value > 1 {
		return fmt.Errorf("factor.momentum_intraday_discount (%.3f) must be in [0,1]", p.Factor.MomentumIntradayDiscount.Value)
	}

	// Health constraints
	if p.Health.MuteThreshold.Value < 1 {
		return fmt.Errorf("health.mute_threshold (%d) must be >= 1", p.Health.MuteThreshold.Value)
	}
	if p.Health.AutoRecoverDays.Value < 1 {
		return fmt.Errorf("health.auto_recover_days (%d) must be >= 1", p.Health.AutoRecoverDays.Value)
	}
	if p.Health.MinSampleSize.Value < 1 {
		return fmt.Errorf("health.min_sample_size (%d) must be >= 1", p.Health.MinSampleSize.Value)
	}
	if p.Health.NegativeSharpeThreshold.Value >= 0 {
		return fmt.Errorf("health.negative_sharpe_threshold (%.3f) must be negative", p.Health.NegativeSharpeThreshold.Value)
	}

	// Experiment constraints
	if p.Experiment.ImprovementThreshold.Value <= 0 {
		return fmt.Errorf("experiment.improvement_threshold (%.3f) must be positive", p.Experiment.ImprovementThreshold.Value)
	}
	if p.Experiment.WelchTTestThreshold.Value <= 0 {
		return fmt.Errorf("experiment.welch_ttest_threshold (%.3f) must be positive", p.Experiment.WelchTTestThreshold.Value)
	}
	if p.Experiment.DrawdownProtectionRatio.Value <= 0 {
		return fmt.Errorf("experiment.drawdown_protection_ratio (%.3f) must be positive", p.Experiment.DrawdownProtectionRatio.Value)
	}
	if p.Experiment.VolatilityToleranceRatio.Value <= 0 {
		return fmt.Errorf("experiment.volatility_tolerance_ratio (%.3f) must be positive", p.Experiment.VolatilityToleranceRatio.Value)
	}

	// Baseline constraints
	if p.Baseline.MaxPositionWeight.Value <= 0 || p.Baseline.MaxPositionWeight.Value >= 1 {
		return fmt.Errorf("baseline.max_position_weight (%.3f) must be in (0,1)", p.Baseline.MaxPositionWeight.Value)
	}
	if p.Baseline.ReserveCashFraction.Value < 0 || p.Baseline.ReserveCashFraction.Value >= 1 {
		return fmt.Errorf("baseline.reserve_cash_fraction (%.3f) must be in [0,1)", p.Baseline.ReserveCashFraction.Value)
	}
	if p.Baseline.TransactionCostBPS.Value < 0 {
		return fmt.Errorf("baseline.transaction_cost_bps (%.3f) must be non-negative", p.Baseline.TransactionCostBPS.Value)
	}
	if p.Baseline.SlippageBPS.Value < 0 {
		return fmt.Errorf("baseline.slippage_bps (%.3f) must be non-negative", p.Baseline.SlippageBPS.Value)
	}

	// Risk constraints
	if p.Risk.VaRConfidenceLevel.Value <= 0 || p.Risk.VaRConfidenceLevel.Value >= 1 {
		return fmt.Errorf("risk.var_confidence_level (%.3f) must be in (0,1)", p.Risk.VaRConfidenceLevel.Value)
	}
	if p.Risk.VaRSecondaryConfidence.Value <= 0 || p.Risk.VaRSecondaryConfidence.Value >= 1 {
		return fmt.Errorf("risk.var_secondary_confidence (%.3f) must be in (0,1)", p.Risk.VaRSecondaryConfidence.Value)
	}
	if p.Risk.VaRAlertThreshold.Value <= 0 {
		return fmt.Errorf("risk.var_alert_threshold (%.3f) must be positive", p.Risk.VaRAlertThreshold.Value)
	}
	if p.Risk.VaRCriticalThreshold.Value <= p.Risk.VaRAlertThreshold.Value {
		return fmt.Errorf("risk.var_critical_threshold (%.3f) must be > var_alert_threshold (%.3f)", p.Risk.VaRCriticalThreshold.Value, p.Risk.VaRAlertThreshold.Value)
	}
	if p.Risk.ConsecutiveLossLimit.Value < 1 {
		return fmt.Errorf("risk.consecutive_loss_limit (%d) must be >= 1", p.Risk.ConsecutiveLossLimit.Value)
	}

	// RiskGate validation
	if p.RiskGate.PreTrade.MaxPositionPct.Value <= 0 || p.RiskGate.PreTrade.MaxPositionPct.Value > 1 {
		return fmt.Errorf("risk_gate.pre_trade.max_position_pct (%.3f) must be in (0,1]", p.RiskGate.PreTrade.MaxPositionPct.Value)
	}
	if p.RiskGate.PreTrade.MaxSectorExposurePct.Value <= 0 || p.RiskGate.PreTrade.MaxSectorExposurePct.Value > 1 {
		return fmt.Errorf("risk_gate.pre_trade.max_sector_exposure_pct (%.3f) must be in (0,1]", p.RiskGate.PreTrade.MaxSectorExposurePct.Value)
	}
	if p.RiskGate.PreTrade.VarLimitPct.Value <= 0 || p.RiskGate.PreTrade.VarLimitPct.Value > 1 {
		return fmt.Errorf("risk_gate.pre_trade.var_limit_pct (%.3f) must be in (0,1]", p.RiskGate.PreTrade.VarLimitPct.Value)
	}
	if p.RiskGate.PreTrade.MinCashBufferPct.Value < 0 || p.RiskGate.PreTrade.MinCashBufferPct.Value > 1 {
		return fmt.Errorf("risk_gate.pre_trade.min_cash_buffer_pct (%.3f) must be in [0,1]", p.RiskGate.PreTrade.MinCashBufferPct.Value)
	}
	if p.RiskGate.PreTrade.VaRConfidenceLevel.Value <= 0 || p.RiskGate.PreTrade.VaRConfidenceLevel.Value > 1 {
		return fmt.Errorf("risk_gate.pre_trade.var_confidence_level (%.3f) must be in (0,1]", p.RiskGate.PreTrade.VaRConfidenceLevel.Value)
	}
	if p.RiskGate.PreTrade.MaxOpenPositions.Value < 1 {
		return fmt.Errorf("risk_gate.pre_trade.max_open_positions (%d) must be >= 1", p.RiskGate.PreTrade.MaxOpenPositions.Value)
	}

	// Drawdown constraints
	if p.Drawdown.NonePercentage.Value < 0 || p.Drawdown.NonePercentage.Value > 1 {
		return fmt.Errorf("drawdown.none_percentage (%.3f) must be in [0,1]", p.Drawdown.NonePercentage.Value)
	}
	if p.Drawdown.NoneMaxExposure.Value < 0 || p.Drawdown.NoneMaxExposure.Value > 1 {
		return fmt.Errorf("drawdown.none_max_exposure (%.3f) must be in [0,1]", p.Drawdown.NoneMaxExposure.Value)
	}
	if p.Drawdown.LightPercentage.Value < 0 || p.Drawdown.LightPercentage.Value > 1 {
		return fmt.Errorf("drawdown.light_percentage (%.3f) must be in [0,1]", p.Drawdown.LightPercentage.Value)
	}
	if p.Drawdown.LightMaxExposure.Value < 0 || p.Drawdown.LightMaxExposure.Value > 1 {
		return fmt.Errorf("drawdown.light_max_exposure (%.3f) must be in [0,1]", p.Drawdown.LightMaxExposure.Value)
	}
	if p.Drawdown.ModeratePercentage.Value < 0 || p.Drawdown.ModeratePercentage.Value > 1 {
		return fmt.Errorf("drawdown.moderate_percentage (%.3f) must be in [0,1]", p.Drawdown.ModeratePercentage.Value)
	}
	if p.Drawdown.ModerateMaxExposure.Value < 0 || p.Drawdown.ModerateMaxExposure.Value > 1 {
		return fmt.Errorf("drawdown.moderate_max_exposure (%.3f) must be in [0,1]", p.Drawdown.ModerateMaxExposure.Value)
	}
	if p.Drawdown.SeverePercentage.Value < 0 || p.Drawdown.SeverePercentage.Value > 1 {
		return fmt.Errorf("drawdown.severe_percentage (%.3f) must be in [0,1]", p.Drawdown.SeverePercentage.Value)
	}
	if p.Drawdown.SevereMaxExposure.Value < 0 || p.Drawdown.SevereMaxExposure.Value > 1 {
		return fmt.Errorf("drawdown.severe_max_exposure (%.3f) must be in [0,1]", p.Drawdown.SevereMaxExposure.Value)
	}
	if p.Drawdown.EmergencyPercentage.Value < 0 || p.Drawdown.EmergencyPercentage.Value > 1 {
		return fmt.Errorf("drawdown.emergency_percentage (%.3f) must be in [0,1]", p.Drawdown.EmergencyPercentage.Value)
	}
	if p.Drawdown.EmergencyMaxExposure.Value < 0 || p.Drawdown.EmergencyMaxExposure.Value > 1 {
		return fmt.Errorf("drawdown.emergency_max_exposure (%.3f) must be in [0,1]", p.Drawdown.EmergencyMaxExposure.Value)
	}
	if p.Drawdown.LightPercentage.Value >= p.Drawdown.ModeratePercentage.Value {
		return fmt.Errorf("drawdown levels must be ordered: light (%.3f) < moderate (%.3f)", p.Drawdown.LightPercentage.Value, p.Drawdown.ModeratePercentage.Value)
	}
	if p.Drawdown.ModeratePercentage.Value >= p.Drawdown.SeverePercentage.Value {
		return fmt.Errorf("drawdown levels must be ordered: moderate (%.3f) < severe (%.3f)", p.Drawdown.ModeratePercentage.Value, p.Drawdown.SeverePercentage.Value)
	}
	if p.Drawdown.SeverePercentage.Value >= p.Drawdown.EmergencyPercentage.Value {
		return fmt.Errorf("drawdown levels must be ordered: severe (%.3f) < emergency (%.3f)", p.Drawdown.SeverePercentage.Value, p.Drawdown.EmergencyPercentage.Value)
	}
	if p.Drawdown.OrangeOverrideMinScore.Value < 0 || p.Drawdown.OrangeOverrideMinScore.Value > 1 {
		return fmt.Errorf("drawdown.orange_override_min_score (%.3f) must be in [0,1]", p.Drawdown.OrangeOverrideMinScore.Value)
	}
	if p.Drawdown.RedOverrideMinScore.Value < 0 || p.Drawdown.RedOverrideMinScore.Value > 1 {
		return fmt.Errorf("drawdown.red_override_min_score (%.3f) must be in [0,1]", p.Drawdown.RedOverrideMinScore.Value)
	}
	if p.Drawdown.RedOverrideMinScore.Value < p.Drawdown.OrangeOverrideMinScore.Value {
		return fmt.Errorf("drawdown.red_override_min_score (%.3f) must be >= orange_override_min_score (%.3f)", p.Drawdown.RedOverrideMinScore.Value, p.Drawdown.OrangeOverrideMinScore.Value)
	}

	// Realtime constraints
	if p.Realtime.VolatilityThreshold.Value <= 0 {
		return fmt.Errorf("realtime.volatility_threshold (%.3f) must be positive", p.Realtime.VolatilityThreshold.Value)
	}
	if p.Realtime.VolumeSpikeThreshold.Value <= 1 {
		return fmt.Errorf("realtime.volume_spike_threshold (%.3f) must be > 1", p.Realtime.VolumeSpikeThreshold.Value)
	}
	if p.Realtime.PriceChangeThreshold.Value <= 0 {
		return fmt.Errorf("realtime.price_change_threshold (%.3f) must be positive", p.Realtime.PriceChangeThreshold.Value)
	}
	if p.Realtime.MinConfidence.Value < 0 || p.Realtime.MinConfidence.Value > 1 {
		return fmt.Errorf("realtime.min_confidence (%.3f) must be in [0,1]", p.Realtime.MinConfidence.Value)
	}
	if p.Realtime.WeightAdjustmentRate.Value <= 0 {
		return fmt.Errorf("realtime.weight_adjustment_rate (%.3f) must be positive", p.Realtime.WeightAdjustmentRate.Value)
	}
	if p.Realtime.MaxWeightChange.Value <= 0 {
		return fmt.Errorf("realtime.max_weight_change (%.3f) must be positive", p.Realtime.MaxWeightChange.Value)
	}
	if p.Realtime.MinWeight.Value <= 0 {
		return fmt.Errorf("realtime.min_weight (%.3f) must be positive", p.Realtime.MinWeight.Value)
	}
	if p.Realtime.UpdateIntervalMs.Value <= 0 {
		return fmt.Errorf("realtime.update_interval_ms (%d) must be positive", p.Realtime.UpdateIntervalMs.Value)
	}

	// Narrative constraints
	if p.Narrative.MinTrendStrength.Value < 0 || p.Narrative.MinTrendStrength.Value > 1 {
		return fmt.Errorf("narrative.min_trend_strength (%.3f) must be in [0,1]", p.Narrative.MinTrendStrength.Value)
	}
	if p.Narrative.MinConfidence.Value < 0 || p.Narrative.MinConfidence.Value > 1 {
		return fmt.Errorf("narrative.min_confidence (%.3f) must be in [0,1]", p.Narrative.MinConfidence.Value)
	}
	if p.Narrative.MinHitRate.Value < 0 || p.Narrative.MinHitRate.Value > 1 {
		return fmt.Errorf("narrative.min_hit_rate (%.3f) must be in [0,1]", p.Narrative.MinHitRate.Value)
	}
	if p.Narrative.OverrideThreshold.Value < 0 || p.Narrative.OverrideThreshold.Value > 1 {
		return fmt.Errorf("narrative.override_threshold (%.3f) must be in [0,1]", p.Narrative.OverrideThreshold.Value)
	}
	if p.Narrative.ConfidenceDeviationCeiling.Value <= 0 || p.Narrative.ConfidenceDeviationCeiling.Value > 1.0 {
		return fmt.Errorf("narrative.confidence_deviation_ceiling (%.3f) must be in (0,1]", p.Narrative.ConfidenceDeviationCeiling.Value)
	}
	if p.Narrative.AIRevenueGrowthThreshold.Value <= 0 {
		return fmt.Errorf("narrative.ai_revenue_growth_threshold (%.3f) must be positive", p.Narrative.AIRevenueGrowthThreshold.Value)
	}
	if p.Narrative.CoWoSUtilizationThreshold.Value <= 0 || p.Narrative.CoWoSUtilizationThreshold.Value > 100 {
		return fmt.Errorf("narrative.cowos_utilization_threshold (%.3f) must be in (0,100]", p.Narrative.CoWoSUtilizationThreshold.Value)
	}
	if p.Narrative.CapexGrowthThreshold.Value <= 0 {
		return fmt.Errorf("narrative.capex_growth_threshold (%.3f) must be positive", p.Narrative.CapexGrowthThreshold.Value)
	}
	if p.Narrative.US10YChangeBpsThreshold.Value <= 0 {
		return fmt.Errorf("narrative.us10y_change_bps_threshold (%.3f) must be positive", p.Narrative.US10YChangeBpsThreshold.Value)
	}
	if p.Narrative.DXYChangePctThreshold.Value <= 0 {
		return fmt.Errorf("narrative.dxy_change_pct_threshold (%.3f) must be positive", p.Narrative.DXYChangePctThreshold.Value)
	}
	if p.Narrative.GeopoliticalGPRThreshold.Value <= 0 {
		return fmt.Errorf("narrative.geopolitical_gpr_threshold (%.3f) must be positive", p.Narrative.GeopoliticalGPRThreshold.Value)
	}
	if p.Narrative.OilChangePctThreshold.Value <= 0 {
		return fmt.Errorf("narrative.oil_change_pct_threshold (%.3f) must be positive", p.Narrative.OilChangePctThreshold.Value)
	}
	if p.Narrative.JPYChangePctThreshold.Value <= 0 {
		return fmt.Errorf("narrative.jpy_change_pct_threshold (%.3f) must be positive", p.Narrative.JPYChangePctThreshold.Value)
	}
	if p.Narrative.VIXLevelThreshold.Value <= 0 {
		return fmt.Errorf("narrative.vix_level_threshold (%.3f) must be positive", p.Narrative.VIXLevelThreshold.Value)
	}
	stressWeightSum := p.Narrative.TaiwanStressDXYWeight.Value +
		p.Narrative.TaiwanStressUS10YWeight.Value +
		p.Narrative.TaiwanStressForeignWeight.Value +
		p.Narrative.TaiwanStressVIXWeight.Value +
		p.Narrative.TaiwanStressJPYWeight.Value +
		p.Narrative.TaiwanStressGeoWeight.Value +
		p.Narrative.TaiwanStressOilWeight.Value +
		p.Narrative.TaiwanStressGoldWeight.Value
	if math.Abs(stressWeightSum-1.0) > 0.01 {
		return fmt.Errorf("narrative taiwan stress weights must sum to 1.0, got %.3f", stressWeightSum)
	}
	if p.Narrative.ModelLookbackDays.Value < 1 {
		return fmt.Errorf("narrative.model_lookback_days (%d) must be >= 1", p.Narrative.ModelLookbackDays.Value)
	}
	if p.Narrative.ModelHoldWindowDays.Value < 1 {
		return fmt.Errorf("narrative.model_hold_window_days (%d) must be >= 1", p.Narrative.ModelHoldWindowDays.Value)
	}
	if p.Narrative.RetailFrenzyPercentileThreshold.Value < 0 || p.Narrative.RetailFrenzyPercentileThreshold.Value > 100 {
		return fmt.Errorf("narrative.retail_frenzy_percentile_threshold (%.1f) must be in [0,100]", p.Narrative.RetailFrenzyPercentileThreshold.Value)
	}
	if p.Narrative.RetailFearPercentileThreshold.Value < 0 || p.Narrative.RetailFearPercentileThreshold.Value > 100 {
		return fmt.Errorf("narrative.retail_fear_percentile_threshold (%.1f) must be in [0,100]", p.Narrative.RetailFearPercentileThreshold.Value)
	}
	if p.Narrative.RetailAccelerationWindowDays.Value < 1 {
		return fmt.Errorf("narrative.retail_acceleration_window_days (%d) must be >= 1", p.Narrative.RetailAccelerationWindowDays.Value)
	}
	if p.Narrative.SpringFestivalConfidence.Value < 0 || p.Narrative.SpringFestivalConfidence.Value > 1 {
		return fmt.Errorf("narrative.spring_festival_confidence (%.3f) must be in [0,1]", p.Narrative.SpringFestivalConfidence.Value)
	}
	if p.Narrative.ElectionCycleConfidence.Value < 0 || p.Narrative.ElectionCycleConfidence.Value > 1 {
		return fmt.Errorf("narrative.election_cycle_confidence (%.3f) must be in [0,1]", p.Narrative.ElectionCycleConfidence.Value)
	}
	if p.Narrative.EarningsBlackoutConfidence.Value < 0 || p.Narrative.EarningsBlackoutConfidence.Value > 1 {
		return fmt.Errorf("narrative.earnings_blackout_confidence (%.3f) must be in [0,1]", p.Narrative.EarningsBlackoutConfidence.Value)
	}
	if p.Narrative.TechPeakSeasonConfidence.Value < 0 || p.Narrative.TechPeakSeasonConfidence.Value > 1 {
		return fmt.Errorf("narrative.tech_peak_season_confidence (%.3f) must be in [0,1]", p.Narrative.TechPeakSeasonConfidence.Value)
	}
	if p.Narrative.YearEndWindowDressingConfidence.Value < 0 || p.Narrative.YearEndWindowDressingConfidence.Value > 1 {
		return fmt.Errorf("narrative.year_end_window_dressing_confidence (%.3f) must be in [0,1]", p.Narrative.YearEndWindowDressingConfidence.Value)
	}
	if !(p.Narrative.EarningsSurpriseConfidence.Value >= 0 && p.Narrative.EarningsSurpriseConfidence.Value <= 1) {
		return fmt.Errorf("narrative.earnings_surprise_confidence (%.3f) must be in [0,1]", p.Narrative.EarningsSurpriseConfidence.Value)
	}
	if p.Narrative.EarningsSurpriseThreshold.Value <= 0 {
		return fmt.Errorf("narrative.earnings_surprise_threshold (%.3f) must be > 0", p.Narrative.EarningsSurpriseThreshold.Value)
	}

	if p.Janus.ShortWindowDays.Value < 1 {
		return fmt.Errorf("janus.short_window_days (%d) must be >= 1", p.Janus.ShortWindowDays.Value)
	}
	if p.Janus.MediumWindowDays.Value < 1 {
		return fmt.Errorf("janus.medium_window_days (%d) must be >= 1", p.Janus.MediumWindowDays.Value)
	}
	if p.Janus.LongWindowDays.Value < 1 {
		return fmt.Errorf("janus.long_window_days (%d) must be >= 1", p.Janus.LongWindowDays.Value)
	}
	if p.Janus.ShortWindowDays.Value >= p.Janus.MediumWindowDays.Value || p.Janus.MediumWindowDays.Value >= p.Janus.LongWindowDays.Value {
		return fmt.Errorf("janus window days must be ordered: short (%d) < medium (%d) < long (%d)",
			p.Janus.ShortWindowDays.Value, p.Janus.MediumWindowDays.Value, p.Janus.LongWindowDays.Value)
	}
	if p.Janus.MaxHistoryDays.Value < p.Janus.LongWindowDays.Value {
		return fmt.Errorf("janus.max_history_days (%d) must be >= long_window_days (%d)", p.Janus.MaxHistoryDays.Value, p.Janus.LongWindowDays.Value)
	}
	if p.Janus.MinWeight.Value < 0 || p.Janus.MinWeight.Value > 1 {
		return fmt.Errorf("janus.min_weight (%.3f) must be in [0,1]", p.Janus.MinWeight.Value)
	}
	if p.Janus.MaxWeight.Value < 0 || p.Janus.MaxWeight.Value > 1 {
		return fmt.Errorf("janus.max_weight (%.3f) must be in [0,1]", p.Janus.MaxWeight.Value)
	}
	if p.Janus.MinWeight.Value >= p.Janus.MaxWeight.Value {
		return fmt.Errorf("janus.min_weight (%.3f) must be < max_weight (%.3f)", p.Janus.MinWeight.Value, p.Janus.MaxWeight.Value)
	}
	if p.Janus.NovelThreshold.Value < 0 || p.Janus.NovelThreshold.Value > 1 {
		return fmt.Errorf("janus.novel_threshold (%.3f) must be in [0,1]", p.Janus.NovelThreshold.Value)
	}
	if p.Janus.HistoricalThreshold.Value < 0 || p.Janus.HistoricalThreshold.Value > 1 {
		return fmt.Errorf("janus.historical_threshold (%.3f) must be in [0,1]", p.Janus.HistoricalThreshold.Value)
	}
	if p.Janus.EpsilonWeight.Value < 0 || p.Janus.EpsilonWeight.Value > 1 {
		return fmt.Errorf("janus.epsilon_weight (%.3f) must be in [0,1]", p.Janus.EpsilonWeight.Value)
	}
	blendSum := p.Janus.ShortWindowBlend.Value + p.Janus.MediumWindowBlend.Value + p.Janus.LongWindowBlend.Value
	if math.Abs(blendSum-1.0) > 0.01 {
		return fmt.Errorf("janus window blend weights must sum to 1.0, got %.3f", blendSum)
	}
	if p.Janus.HealthStaleHours.Value < 1 {
		return fmt.Errorf("janus.health_stale_hours (%d) must be >= 1", p.Janus.HealthStaleHours.Value)
	}
	if p.Janus.HealthWarnHours.Value < 1 {
		return fmt.Errorf("janus.health_warn_hours (%d) must be >= 1", p.Janus.HealthWarnHours.Value)
	}
	if p.Janus.HealthWarnHours.Value >= p.Janus.HealthStaleHours.Value {
		return fmt.Errorf("janus.health_warn_hours (%d) must be < health_stale_hours (%d)", p.Janus.HealthWarnHours.Value, p.Janus.HealthStaleHours.Value)
	}

	if p.Marketdata.TWSEAPIRateLimit.Value <= 0 {
		return fmt.Errorf("marketdata.twse_api_rate_limit (%.3f) must be positive", p.Marketdata.TWSEAPIRateLimit.Value)
	}
	if p.Marketdata.TWSEAPIRateBurst.Value < 1 {
		return fmt.Errorf("marketdata.twse_api_rate_burst (%d) must be >= 1", p.Marketdata.TWSEAPIRateBurst.Value)
	}
	if p.Marketdata.TWSEAPITimeoutSec.Value < 1 {
		return fmt.Errorf("marketdata.twse_api_timeout_sec (%d) must be >= 1", p.Marketdata.TWSEAPITimeoutSec.Value)
	}
	if p.Marketdata.FubonIntradayLimit.Value < 1 {
		return fmt.Errorf("marketdata.fubon_intraday_limit (%d) must be >= 1", p.Marketdata.FubonIntradayLimit.Value)
	}
	if p.Marketdata.FubonHistoricalLimit.Value < 1 {
		return fmt.Errorf("marketdata.fubon_historical_limit (%d) must be >= 1", p.Marketdata.FubonHistoricalLimit.Value)
	}
	if p.Marketdata.FubonAPITimeoutSec.Value < 1 {
		return fmt.Errorf("marketdata.fubon_api_timeout_sec (%d) must be >= 1", p.Marketdata.FubonAPITimeoutSec.Value)
	}
	if p.Marketdata.TEJCallsPerSecond.Value < 1 {
		return fmt.Errorf("marketdata.tej_calls_per_second (%d) must be >= 1", p.Marketdata.TEJCallsPerSecond.Value)
	}
	if p.Marketdata.TEJAPITimeoutSec.Value < 1 {
		return fmt.Errorf("marketdata.tej_api_timeout_sec (%d) must be >= 1", p.Marketdata.TEJAPITimeoutSec.Value)
	}
	if p.Marketdata.FugleRateLimit.Value < 1 {
		return fmt.Errorf("marketdata.fugle_rate_limit (%d) must be >= 1", p.Marketdata.FugleRateLimit.Value)
	}
	if p.Marketdata.FugleAPITimeoutSec.Value < 1 {
		return fmt.Errorf("marketdata.fugle_api_timeout_sec (%d) must be >= 1", p.Marketdata.FugleAPITimeoutSec.Value)
	}
	if p.Marketdata.MaxRetryAttempts.Value < 0 {
		return fmt.Errorf("marketdata.max_retry_attempts (%d) must be >= 0", p.Marketdata.MaxRetryAttempts.Value)
	}
	if p.Marketdata.RetryBackoffMs.Value < 0 {
		return fmt.Errorf("marketdata.retry_backoff_ms (%d) must be >= 0", p.Marketdata.RetryBackoffMs.Value)
	}

	if p.Industry.CustomerConcentrationLimit.Value < 0 || p.Industry.CustomerConcentrationLimit.Value > 1 {
		return fmt.Errorf("industry.customer_concentration_limit (%.3f) must be in [0,1]", p.Industry.CustomerConcentrationLimit.Value)
	}
	if p.Industry.GeographicExposureLimit.Value < 0 || p.Industry.GeographicExposureLimit.Value > 1 {
		return fmt.Errorf("industry.geographic_exposure_limit (%.3f) must be in [0,1]", p.Industry.GeographicExposureLimit.Value)
	}

	cs := p.Industry.ConfidenceSignal.Value
	if cs.SignalBase < 0 || cs.SignalBase > 1 {
		return fmt.Errorf("industry.confidence_signal.signal_base (%.3f) must be in [0,1]", cs.SignalBase)
	}
	if cs.RevenueWeight+cs.ProfitWeight+cs.InventoryWeight+cs.UtilizationWeight > 1.0+0.01 {
		return fmt.Errorf("industry.confidence_signal weights (rev=%.2f, profit=%.2f, inv=%.2f, util=%.2f) sum exceeds 1.0", cs.RevenueWeight, cs.ProfitWeight, cs.InventoryWeight, cs.UtilizationWeight)
	}
	if cs.SignalBoundaryMix < 0 || cs.SignalBoundaryMix > 1 {
		return fmt.Errorf("industry.confidence_signal.signal_boundary_mix (%.3f) must be in [0,1]", cs.SignalBoundaryMix)
	}
	if cs.ConfidenceFloor < 0 {
		return fmt.Errorf("industry.confidence_signal.confidence_floor (%.3f) must be >= 0", cs.ConfidenceFloor)
	}
	if cs.ConfidenceCeiling <= cs.ConfidenceFloor {
		return fmt.Errorf("industry.confidence_signal.confidence_ceiling (%.3f) must be > floor (%.3f)", cs.ConfidenceCeiling, cs.ConfidenceFloor)
	}

	cm := p.Industry.ConfidenceMix.Value
	mixSum := cm.WeightBoundary + cm.WeightFreshness + cm.WeightSeasonal + cm.WeightLinkage + cm.WeightNarrative
	if math.Abs(mixSum-1.0) > 0.01 {
		return fmt.Errorf("industry.confidence_mix weights must sum to 1.0, got %.3f (b=%.2f f=%.2f s=%.2f l=%.2f n=%.2f)", mixSum, cm.WeightBoundary, cm.WeightFreshness, cm.WeightSeasonal, cm.WeightLinkage, cm.WeightNarrative)
	}
	if cm.FavorableConfidenceMin < 0 || cm.FavorableConfidenceMin > 1 {
		return fmt.Errorf("industry.confidence_mix.favorable_confidence_min (%.3f) must be in [0,1]", cm.FavorableConfidenceMin)
	}

	for i, sp := range p.Industry.SeasonalPatterns.Value {
		if sp.ID == "" {
			return fmt.Errorf("industry.seasonal_patterns[%d].id must not be empty", i)
		}
		if sp.StartMonth < 1 || sp.StartMonth > 12 || sp.EndMonth < 1 || sp.EndMonth > 12 {
			return fmt.Errorf("industry.seasonal_patterns[%d] invalid month: start=%d end=%d", i, sp.StartMonth, sp.EndMonth)
		}
		if sp.AdjustmentFactor == 0 {
			return fmt.Errorf("industry.seasonal_patterns[%d].adjustment_factor (%.3f) must not be zero", i, sp.AdjustmentFactor)
		}
		if sp.HistoricalAccuracy < 0 || sp.HistoricalAccuracy > 1 {
			return fmt.Errorf("industry.seasonal_patterns[%d].historical_accuracy (%.3f) must be in [0,1]", i, sp.HistoricalAccuracy)
		}
	}

	lp := p.Industry.LinkageParams.Value
	if lp.DownstreamDecayFactor < 0 || lp.DownstreamDecayFactor > 1 {
		return fmt.Errorf("industry.linkage_params.downstream_decay_factor (%.3f) must be in [0,1]", lp.DownstreamDecayFactor)
	}
	if lp.UpstreamDecayFactor < 0 || lp.UpstreamDecayFactor > 1 {
		return fmt.Errorf("industry.linkage_params.upstream_decay_factor (%.3f) must be in [0,1]", lp.UpstreamDecayFactor)
	}
	if lp.SeasonalDecayFactor < 0 || lp.SeasonalDecayFactor > 1 {
		return fmt.Errorf("industry.linkage_params.seasonal_decay_factor (%.3f) must be in [0,1]", lp.SeasonalDecayFactor)
	}
	if lp.DefaultCorrelation < 0 || lp.DefaultCorrelation > 1 {
		return fmt.Errorf("industry.linkage_params.default_correlation (%.3f) must be in [0,1]", lp.DefaultCorrelation)
	}
	if lp.SystemicImportanceDivisor <= 0 {
		return fmt.Errorf("industry.linkage_params.systemic_importance_divisor (%.3f) must be > 0", lp.SystemicImportanceDivisor)
	}
	if lp.MinCorrelationThreshold < 0 || lp.MinCorrelationThreshold > 1 {
		return fmt.Errorf("industry.linkage_params.min_correlation_threshold (%.3f) must be in [0,1]", lp.MinCorrelationThreshold)
	}
	if lp.RecessionShockAmplifier < 0.5 || lp.RecessionShockAmplifier > 5.0 {
		return fmt.Errorf("industry.linkage_params.recession_shock_amplifier (%.3f) must be in [0.5, 5.0]", lp.RecessionShockAmplifier)
	}

	de := p.Industry.DynamicEnv.Value
	if de.HistoryWindowDays < 7 || de.HistoryWindowDays > 365 {
		return fmt.Errorf("industry.dynamic_env.history_window_days (%d) must be in [7, 365]", de.HistoryWindowDays)
	}
	if de.HistoryCapMultiplier < 1 || de.HistoryCapMultiplier > 10 {
		return fmt.Errorf("industry.dynamic_env.history_cap_multiplier (%d) must be in [1, 10]", de.HistoryCapMultiplier)
	}
	if de.OilPriceShockThreshold < 0 || de.OilPriceShockThreshold > 1 {
		return fmt.Errorf("industry.dynamic_env.oil_price_shock_threshold (%.3f) must be in [0, 1]", de.OilPriceShockThreshold)
	}
	if de.UsRatesDxyThreshold < 0 || de.UsRatesDxyThreshold > 1 {
		return fmt.Errorf("industry.dynamic_env.us_rates_dxy_threshold (%.3f) must be in [0, 1]", de.UsRatesDxyThreshold)
	}
	if de.JpyCarryDxyThreshold < 0 || de.JpyCarryDxyThreshold > 1 {
		return fmt.Errorf("industry.dynamic_env.jpy_carry_dxy_threshold (%.3f) must be in [0, 1]", de.JpyCarryDxyThreshold)
	}

	// New asymmetric risk parameterized thresholds
	if p.Industry.AsymmetricDropCritical.Value <= 0 || p.Industry.AsymmetricDropCritical.Value > 1 {
		return fmt.Errorf("industry.asymmetric_drop_critical (%.3f) must be in (0,1]", p.Industry.AsymmetricDropCritical.Value)
	}
	if p.Industry.AsymmetricDropHigh.Value <= 0 || p.Industry.AsymmetricDropHigh.Value > 1 {
		return fmt.Errorf("industry.asymmetric_drop_high (%.3f) must be in (0,1]", p.Industry.AsymmetricDropHigh.Value)
	}
	if p.Industry.AsymmetricDropMedium.Value <= 0 || p.Industry.AsymmetricDropMedium.Value > 1 {
		return fmt.Errorf("industry.asymmetric_drop_medium (%.3f) must be in (0,1]", p.Industry.AsymmetricDropMedium.Value)
	}
	if p.Industry.AsymmetricDropMedium.Value >= p.Industry.AsymmetricDropHigh.Value {
		return fmt.Errorf("industry.asymmetric_drop_medium (%.3f) must be < asymmetric_drop_high (%.3f)", p.Industry.AsymmetricDropMedium.Value, p.Industry.AsymmetricDropHigh.Value)
	}
	if p.Industry.AsymmetricDropHigh.Value >= p.Industry.AsymmetricDropCritical.Value {
		return fmt.Errorf("industry.asymmetric_drop_high (%.3f) must be < asymmetric_drop_critical (%.3f)", p.Industry.AsymmetricDropHigh.Value, p.Industry.AsymmetricDropCritical.Value)
	}
	if p.Industry.NewsImpactMultiplier.Value < 0 || p.Industry.NewsImpactMultiplier.Value > 1 {
		return fmt.Errorf("industry.news_impact_multiplier (%.3f) must be in [0,1]", p.Industry.NewsImpactMultiplier.Value)
	}
	if p.Industry.BoundaryFallback.Value <= 0 {
		return fmt.Errorf("industry.boundary_fallback (%.3f) must be positive", p.Industry.BoundaryFallback.Value)
	}
	if p.Industry.AdjustmentFloor.Value < 0 || p.Industry.AdjustmentFloor.Value > 1 {
		return fmt.Errorf("industry.adjustment_floor (%.3f) must be in [0,1]", p.Industry.AdjustmentFloor.Value)
	}

	// FactorWeight constraints
	if p.FactorWeight.BaseWeights.Value != nil {
		expectedBaseKeys := []string{"momentum", "value", "quality", "agent", "inst_sent", "liquidity", "narrative", "industry_cycle"}
		for _, k := range expectedBaseKeys {
			if _, ok := p.FactorWeight.BaseWeights.Value[k]; !ok {
				return fmt.Errorf("factor_weight.base_weights: missing key %q", k)
			}
		}
	}
	if p.FactorWeight.ClampMin.Value >= p.FactorWeight.ClampMax.Value {
		return fmt.Errorf("factor_weight: clamp_min (%.2f) must be less than clamp_max (%.2f)", p.FactorWeight.ClampMin.Value, p.FactorWeight.ClampMax.Value)
	}
	if p.FactorWeight.SeverityCritical.Value < p.FactorWeight.SeverityHigh.Value {
		return fmt.Errorf("factor_weight.severity_critical (%.3f) must be >= severity_high (%.3f)", p.FactorWeight.SeverityCritical.Value, p.FactorWeight.SeverityHigh.Value)
	}
	if p.FactorWeight.SeverityHigh.Value < p.FactorWeight.SeverityMedium.Value {
		return fmt.Errorf("factor_weight.severity_high (%.3f) must be >= severity_medium (%.3f)", p.FactorWeight.SeverityHigh.Value, p.FactorWeight.SeverityMedium.Value)
	}
	if p.FactorWeight.SeverityMedium.Value < p.FactorWeight.SeverityLow.Value {
		return fmt.Errorf("factor_weight.severity_medium (%.3f) must be >= severity_low (%.3f)", p.FactorWeight.SeverityMedium.Value, p.FactorWeight.SeverityLow.Value)
	}
	regimeDeltaChecks := []struct {
		name  string
		value float64
	}{
		{"regime_bull_momentum", p.FactorWeight.RegimeBullMomentum.Value},
		{"regime_bull_quality", p.FactorWeight.RegimeBullQuality.Value},
		{"regime_bull_value", p.FactorWeight.RegimeBullValue.Value},
		{"regime_bear_quality", p.FactorWeight.RegimeBearQuality.Value},
		{"regime_bear_value", p.FactorWeight.RegimeBearValue.Value},
		{"regime_bear_momentum", p.FactorWeight.RegimeBearMomentum.Value},
		{"regime_high_vol_liquidity", p.FactorWeight.RegimeHighVolLiquidity.Value},
		{"regime_high_vol_momentum", p.FactorWeight.RegimeHighVolMomentum.Value},
		{"regime_high_vol_inst_sent", p.FactorWeight.RegimeHighVolInstSent.Value},
	}
	for _, rd := range regimeDeltaChecks {
		if rd.value < -0.15 || rd.value > 0.15 {
			return fmt.Errorf("factor_weight.%s (%.3f) must be in [-0.15, 0.15]", rd.name, rd.value)
		}
	}

	// NarrativeConviction constraints
	if p.NarrativeConviction.ThemeHitRates.Value != nil {
		expectedThemeKeys := []string{"AI_capex_surge", "US_rates_up", "JPY_carry_unwind", "geopolitical_risk_spike", "oil_price_shock"}
		for _, k := range expectedThemeKeys {
			if _, ok := p.NarrativeConviction.ThemeHitRates.Value[k]; !ok {
				return fmt.Errorf("narrative_conviction.theme_hit_rates: missing key %q", k)
			}
		}
		for k, v := range p.NarrativeConviction.ThemeHitRates.Value {
			if v < 0 || v > 1 {
				return fmt.Errorf("narrative_conviction.theme_hit_rates[%s] (%.3f) must be in [0,1]", k, v)
			}
		}
	}
	if len(p.NarrativeConviction.SkillToTheme.Value) == 0 {
		return fmt.Errorf("narrative_conviction.skill_to_theme must not be empty")
	}

	// Industry new field constraints
	if len(p.Industry.SkillToIndustry.Value) == 0 {
		return fmt.Errorf("industry.skill_to_industry must not be empty")
	}
	ps := p.Industry.PhaseScores.Value
	if ps.ScoreRecession > ps.ScoreMature || ps.ScoreMature > ps.ScoreRecovery || ps.ScoreRecovery > ps.ScoreExpansion {
		return fmt.Errorf("industry.phase_scores: must be monotonically decreasing (expansion=%.0f >= recovery=%.0f >= mature=%.0f >= recession=%.0f)",
			ps.ScoreExpansion, ps.ScoreRecovery, ps.ScoreMature, ps.ScoreRecession)
	}

	if len(p.Orchestrator.SectorRotationMacroAdjustments.Value) == 0 {
		return fmt.Errorf("orchestrator.sector_rotation_macro_adjustments must not be empty")
	}
	if len(p.Orchestrator.SectorRotationFlowAdjustments.Value) == 0 {
		return fmt.Errorf("orchestrator.sector_rotation_flow_adjustments must not be empty")
	}

	if p.Strategy.MinSwitchIntervalDays.Value < 0 {
		return fmt.Errorf("strategy.min_switch_interval_days (%d) must be >= 0", p.Strategy.MinSwitchIntervalDays.Value)
	}
	if p.Strategy.SwitchThreshold.Value < 0 || p.Strategy.SwitchThreshold.Value > 1 {
		return fmt.Errorf("strategy.switch_threshold (%.3f) must be in [0,1]", p.Strategy.SwitchThreshold.Value)
	}
	if p.Strategy.ScoreLookbackDays.Value < 1 {
		return fmt.Errorf("strategy.score_lookback_days (%d) must be >= 1", p.Strategy.ScoreLookbackDays.Value)
	}
	if p.Strategy.FallbackStrategy.Value == "" {
		return fmt.Errorf("strategy.fallback_strategy must not be empty")
	}

	// Sector executor constraints (only validate when configured, not zero-valued)
	if p.SectorExecutor.LEOSatellite.ConvictionBase.Value != 0 && p.SectorExecutor.LEOSatellite.ConvictionBase.Value < 0 {
		return fmt.Errorf("sector_executor.leo_satellite.conviction_base (%d) must be non-negative", p.SectorExecutor.LEOSatellite.ConvictionBase.Value)
	}
	if p.SectorExecutor.LEOSatellite.TargetPriceMult.Value != 0 && p.SectorExecutor.LEOSatellite.TargetPriceMult.Value <= 0 {
		return fmt.Errorf("sector_executor.leo_satellite.target_price_multiplier (%.3f) must be positive", p.SectorExecutor.LEOSatellite.TargetPriceMult.Value)
	}
	if p.SectorExecutor.LEOSatellite.StopLossMult.Value != 0 && (p.SectorExecutor.LEOSatellite.StopLossMult.Value <= 0 || p.SectorExecutor.LEOSatellite.StopLossMult.Value >= 1) {
		return fmt.Errorf("sector_executor.leo_satellite.stop_loss_multiplier (%.3f) must be in (0,1)", p.SectorExecutor.LEOSatellite.StopLossMult.Value)
	}
	if fp := p.SectorExecutor.Financials; fp.PriceToOpenThreshold.Value != 0 && (fp.PriceToOpenThreshold.Value <= 0 || fp.PriceToOpenThreshold.Value >= 1) {
		return fmt.Errorf("sector_executor.financials.price_to_open_threshold (%.3f) must be in (0,1)", fp.PriceToOpenThreshold.Value)
	}
	if eq := p.SectorExecutor.EarningsQuality; eq.GuidanceThreshold.Value != 0 && (eq.GuidanceThreshold.Value <= 0 || eq.GuidanceThreshold.Value >= 1) {
		return fmt.Errorf("sector_executor.earnings_quality.guidance_threshold (%.3f) must be in (0,1)", eq.GuidanceThreshold.Value)
	}
	if tp := p.SectorExecutor.TechnicalBreakout; tp.DefaultVolumeFloor.Value != 0 && tp.DefaultVolumeFloor.Value < 0 {
		return fmt.Errorf("sector_executor.technical_breakout.default_volume_floor (%d) must be non-negative", tp.DefaultVolumeFloor.Value)
	}

	if gm := p.SectorExecutor.GrowthMomentum; gm.DowngradeThreshold.Value != 0 && (gm.DowngradeThreshold.Value <= 0 || gm.DowngradeThreshold.Value >= 1) {
		return fmt.Errorf("sector_executor.growth_momentum.downgrade_threshold (%.3f) must be in (0,1)", gm.DowngradeThreshold.Value)
	}

	if err := p.validateAlert(); err != nil {
		return err
	}

	if err := p.validateEngine(); err != nil {
		return err
	}

	// Narrative parameters validation
	if p.Narrative.ModelLookbackDays.Value < 1 {
		return fmt.Errorf("narrative.model_lookback_days (%d) must be >= 1", p.Narrative.ModelLookbackDays.Value)
	}
	if p.Narrative.ModelHoldWindowDays.Value < 1 {
		return fmt.Errorf("narrative.model_hold_window_days (%d) must be >= 1", p.Narrative.ModelHoldWindowDays.Value)
	}
	if p.Narrative.ConfidenceDeviationCeiling.Value <= 0 || p.Narrative.ConfidenceDeviationCeiling.Value > 1 {
		return fmt.Errorf("narrative.confidence_deviation_ceiling (%.3f) must be in (0,1]", p.Narrative.ConfidenceDeviationCeiling.Value)
	}

	// Taiwan stress index weights should sum to 1.0
	stressWeights := p.Narrative.TaiwanStressDXYWeight.Value +
		p.Narrative.TaiwanStressUS10YWeight.Value +
		p.Narrative.TaiwanStressForeignWeight.Value +
		p.Narrative.TaiwanStressVIXWeight.Value +
		p.Narrative.TaiwanStressJPYWeight.Value +
		p.Narrative.TaiwanStressGeoWeight.Value +
		p.Narrative.TaiwanStressOilWeight.Value +
		p.Narrative.TaiwanStressGoldWeight.Value
	if math.Abs(stressWeights-1.0) > 0.01 {
		return fmt.Errorf("narrative taiwan stress weights must sum to 1.0, got %.3f", stressWeights)
	}

	return nil
}
