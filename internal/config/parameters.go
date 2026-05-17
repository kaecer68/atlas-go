package config

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"
)

// ParameterSource indicates how a parameter value was determined.
type ParameterSource string

const (
	SourceLiterature ParameterSource = "literature" // from academic/practitioner literature
	SourceEmpirical  ParameterSource = "empirical"  // from historical data analysis
	SourceHeuristic  ParameterSource = "heuristic"  // from domain expert judgment
	SourceInferred   ParameterSource = "inferred"   // from automated inference/calibration
	SourceCalibrated ParameterSource = "calibrated" // from backtest optimization
)

// ParameterMetadata holds the value and provenance of a tunable parameter.
type ParameterMetadata[T any] struct {
	Value             T                  `json:"value"`
	Rationale         string             `json:"rationale"`
	Source            ParameterSource    `json:"source"`
	LastCalibrated    *time.Time         `json:"last_calibrated,omitempty"`
	CalibrationMethod string             `json:"calibration_method,omitempty"`
	Todo              string             `json:"todo,omitempty"`
	Citation          *ParameterCitation `json:"citation,omitempty"`
}

// ParameterCitation holds the citation/source tracking metadata for a parameter.
type ParameterCitation struct {
	SourceType       string   `json:"source_type"`
	SourceReference  string   `json:"source_reference"`
	EvidenceQuality  string   `json:"evidence_quality"`
	UpdatePolicy     string   `json:"update_policy"`
	ValidationMethod string   `json:"validation_method"`
	Dependencies     []string `json:"dependencies"`
	LastValidated    string   `json:"last_validated"`
}

// DarwinianParameters holds all tunable values for the Darwinian weight system.
type DarwinianParameters struct {
	WeightMin                   ParameterMetadata[float64] `json:"weight_min"`
	WeightMax                   ParameterMetadata[float64] `json:"weight_max"`
	WeightNeutral               ParameterMetadata[float64] `json:"weight_neutral"`
	TopQuartileMultiplier       ParameterMetadata[float64] `json:"top_quartile_multiplier"`
	BottomQuartileMultiplier    ParameterMetadata[float64] `json:"bottom_quartile_multiplier"`
	DailyAdjustmentCooldown     ParameterMetadata[string]  `json:"daily_adjustment_cooldown"`
	LookbackDays                ParameterMetadata[int]     `json:"lookback_days"`
	EMAAlpha                    ParameterMetadata[float64] `json:"ema_alpha"`
	SharpeNormalizeDenom        ParameterMetadata[float64] `json:"sharpe_normalize_denom"`
	MaxPerformanceBonusPct      ParameterMetadata[float64] `json:"max_performance_bonus_pct"`
	VolatilityPenaltyThreshold  ParameterMetadata[float64] `json:"volatility_penalty_threshold"`
	VolatilityPenaltyMultiplier ParameterMetadata[float64] `json:"volatility_penalty_multiplier"`
	RiskVolatilityThreshold     ParameterMetadata[float64] `json:"risk_volatility_threshold"`
	RiskMultiplier              ParameterMetadata[float64] `json:"risk_multiplier"`
	HitRateHighThreshold        ParameterMetadata[float64] `json:"hit_rate_high_threshold"`
	HitRateLowThreshold         ParameterMetadata[float64] `json:"hit_rate_low_threshold"`
	MiddleTierBoostMultiplier   ParameterMetadata[float64] `json:"middle_tier_boost_multiplier"`
	MiddleTierCutMultiplier     ParameterMetadata[float64] `json:"middle_tier_cut_multiplier"`
	SharpeMinSampleSize         ParameterMetadata[int]     `json:"sharpe_min_sample_size"`
	StdDevMeanRatioThreshold    ParameterMetadata[float64] `json:"stddev_mean_ratio_threshold"`
	ConvictionClampMin          ParameterMetadata[int]     `json:"conviction_clamp_min"`
	ConvictionClampMax          ParameterMetadata[int]     `json:"conviction_clamp_max"`
}

// FactorParameters holds tunable values for the factor engine.
type FactorParameters struct {
	MomentumLookbackDays          ParameterMetadata[int]                `json:"momentum_lookback_days"`
	MomentumStdDevDivisor         ParameterMetadata[float64]            `json:"momentum_stddev_divisor"`
	MomentumIntradayDiscount      ParameterMetadata[float64]            `json:"momentum_intraday_discount"`
	MomentumIntradayThreshold     ParameterMetadata[float64]            `json:"momentum_intraday_threshold"`
	ValuePERangeCenter            ParameterMetadata[float64]            `json:"value_pe_range_center"`
	ValuePERangeWidth             ParameterMetadata[float64]            `json:"value_pe_range_width"`
	ValuePBRangeCenter            ParameterMetadata[float64]            `json:"value_pb_range_center"`
	ValuePBRangeWidth             ParameterMetadata[float64]            `json:"value_pb_range_width"`
	ValuePSRangeCenter            ParameterMetadata[float64]            `json:"value_ps_range_center"`
	ValuePSRangeWidth             ParameterMetadata[float64]            `json:"value_ps_range_width"`
	QualityDividendYieldCap       ParameterMetadata[float64]            `json:"quality_dividend_yield_cap"`
	QualityVolatilityStd          ParameterMetadata[float64]            `json:"quality_volatility_std"`
	QualityFallbackScore          ParameterMetadata[float64]            `json:"quality_fallback_score"`
	ValueFallbackScore            ParameterMetadata[float64]            `json:"value_fallback_score"`
	InstitutionalSentimentWeights ParameterMetadata[map[string]float64] `json:"institutional_sentiment_weights"`
	FallbackWeightReduction       ParameterMetadata[float64]            `json:"fallback_weight_reduction"`
}

// OptimizerParameters holds tunable values for portfolio optimization.
type OptimizerParameters struct {
	MaxPositionPct   ParameterMetadata[float64]            `json:"max_position_pct"`
	MaxSectorPct     ParameterMetadata[float64]            `json:"max_sector_pct"`
	MaxTurnoverDaily ParameterMetadata[float64]            `json:"max_turnover_daily"`
	TargetBeta       ParameterMetadata[float64]            `json:"target_beta"`
	BetaRangeMin     ParameterMetadata[float64]            `json:"beta_range_min"`
	BetaRangeMax     ParameterMetadata[float64]            `json:"beta_range_max"`
	MinTradeSize     ParameterMetadata[int]                `json:"min_trade_size"`
	CashReserve      ParameterMetadata[float64]            `json:"cash_reserve"`
	FactorWeights    ParameterMetadata[map[string]float64] `json:"factor_weights"`
}

// SizingParameters holds tunable values for position sizing.
type SizingParameters struct {
	KellyFraction            ParameterMetadata[float64] `json:"kelly_fraction"`
	VolLookbackDays          ParameterMetadata[int]     `json:"vol_lookback_days"`
	MaxPositionByADV         ParameterMetadata[float64] `json:"max_position_by_adv"`
	MaxDrawdownLimit         ParameterMetadata[float64] `json:"max_drawdown_limit"`
	ATRMultiplier            ParameterMetadata[float64] `json:"atr_multiplier"`
	CorrelationPenalty       ParameterMetadata[float64] `json:"correlation_penalty"`
	CorrelationThreshold     ParameterMetadata[float64] `json:"correlation_threshold"`
	DefaultWinRate           ParameterMetadata[float64] `json:"default_win_rate"`
	DefaultPayoffRatio       ParameterMetadata[float64] `json:"default_payoff_ratio"`
	TargetVolatility         ParameterMetadata[float64] `json:"target_volatility"`
	VolAdjustmentMin         ParameterMetadata[float64] `json:"vol_adjustment_min"`
	VolAdjustmentMax         ParameterMetadata[float64] `json:"vol_adjustment_max"`
	ATRTargetRisk            ParameterMetadata[float64] `json:"atr_target_risk"`
	ATRAdjustmentMin         ParameterMetadata[float64] `json:"atr_adjustment_min"`
	ATRAdjustmentMax         ParameterMetadata[float64] `json:"atr_adjustment_max"`
	CorrelationPenaltyFactor ParameterMetadata[float64] `json:"correlation_penalty_factor"`
	MaxCorrelationPenalty    ParameterMetadata[float64] `json:"max_correlation_penalty"`
	DefaultVolatility        ParameterMetadata[float64] `json:"default_volatility"`
	DefaultADV               ParameterMetadata[float64] `json:"default_adv"`
	DefaultATR               ParameterMetadata[float64] `json:"default_atr"`
}

// HealthParameters holds tunable values for agent health management.
type HealthParameters struct {
	MuteThreshold           ParameterMetadata[int]     `json:"mute_threshold"`
	UnmuteThreshold         ParameterMetadata[int]     `json:"unmute_threshold"`
	AutoRecoverDays         ParameterMetadata[int]     `json:"auto_recover_days"`
	MinSampleSize           ParameterMetadata[int]     `json:"min_sample_size"`
	NegativeSharpeThreshold ParameterMetadata[float64] `json:"negative_sharpe_threshold"`
	SharpeWeight            ParameterMetadata[float64] `json:"sharpe_weight"`
	HitRateWeight           ParameterMetadata[float64] `json:"hit_rate_weight"`
	StreakWeight            ParameterMetadata[float64] `json:"streak_weight"`
	MaxSharpe               ParameterMetadata[float64] `json:"max_sharpe"`
	MinSharpe               ParameterMetadata[float64] `json:"min_sharpe"`
	StreakMax               ParameterMetadata[int]     `json:"streak_max"`
}

// GARCHParameters holds tunable values for volatility forecasting.
type GARCHParameters struct {
	Omega               ParameterMetadata[float64] `json:"omega"`
	Alpha               ParameterMetadata[float64] `json:"alpha"`
	Beta                ParameterMetadata[float64] `json:"beta"`
	MaxHistory          ParameterMetadata[int]     `json:"max_history"`
	CorrelationMinDays  ParameterMetadata[int]     `json:"correlation_min_days"`
	SmoothingFactor     ParameterMetadata[float64] `json:"smoothing_factor"`
	RebalanceThreshold  ParameterMetadata[float64] `json:"rebalance_threshold"`
	MinForecastDays     ParameterMetadata[int]     `json:"min_forecast_days"`
	MaxHistoryPoints    ParameterMetadata[int]     `json:"max_history_points"`
	HighVolThreshold    ParameterMetadata[float64] `json:"high_vol_threshold"`
	LowVolThreshold     ParameterMetadata[float64] `json:"low_vol_threshold"`
	ReduceMagnitude     ParameterMetadata[float64] `json:"reduce_magnitude"`
	IncreaseMagnitude   ParameterMetadata[float64] `json:"increase_magnitude"`
	WeeklyRebalanceDays ParameterMetadata[int]     `json:"weekly_rebalance_days"`
}

// ExperimentParameters holds tunable values for experiment evaluation.
type ExperimentParameters struct {
	MaturityLevel1Observations ParameterMetadata[int]     `json:"maturity_level1_observations"`
	MaturityLevel2Observations ParameterMetadata[int]     `json:"maturity_level2_observations"`
	MaturityLevel3Observations ParameterMetadata[int]     `json:"maturity_level3_observations"`
	ImprovementThreshold       ParameterMetadata[float64] `json:"improvement_threshold"`
	WelchTTestThreshold        ParameterMetadata[float64] `json:"welch_t_test_threshold"`
	DrawdownProtectionRatio    ParameterMetadata[float64] `json:"drawdown_protection_ratio"`
	VolatilityToleranceRatio   ParameterMetadata[float64] `json:"volatility_tolerance_ratio"`
	OOSWindowDays              ParameterMetadata[int]     `json:"oos_window_days"`
	SharpeStabilityThreshold   ParameterMetadata[float64] `json:"sharpe_stability_threshold"`
	MaxFallbackRatio           ParameterMetadata[float64] `json:"max_fallback_ratio"`
}

// BaselineParameters holds tunable values for baseline policy defaults.
type BaselineParameters struct {
	StartingCash                ParameterMetadata[float64] `json:"starting_cash"`
	MaxPositionWeight           ParameterMetadata[float64] `json:"max_position_weight"`
	MaxOpenPositions            ParameterMetadata[int]     `json:"max_open_positions"`
	MinTradableVolume           ParameterMetadata[float64] `json:"min_tradable_volume"`
	MinRecommendationConviction ParameterMetadata[int]     `json:"min_recommendation_conviction"`
	RequireCROPass              ParameterMetadata[bool]    `json:"require_cro_pass"`
	TransactionCostBPS          ParameterMetadata[float64] `json:"transaction_cost_bps"`
	SlippageBPS                 ParameterMetadata[float64] `json:"slippage_bps"`
	ReserveCashFraction         ParameterMetadata[float64] `json:"reserve_cash_fraction"`
}

// OrchestratorParameters holds tunable values for the orchestrator executors,
// control layer (CRO/CIO), and Phase3 controller.
type OrchestratorParameters struct {
	ConvictionFloorDefault           ParameterMetadata[int]     `json:"conviction_floor_default"`
	SuperinvestorMinConviction       ParameterMetadata[int]     `json:"superinvestor_min_conviction"`
	CROZScoreThreshold               ParameterMetadata[float64] `json:"cro_zscore_threshold"`
	SectorConcentrationThreshold     ParameterMetadata[float64] `json:"sector_concentration_threshold"`
	SectorConcentrationThresholdHigh ParameterMetadata[float64] `json:"sector_concentration_threshold_high"`
	SectorConvictionMultiplier       ParameterMetadata[float64] `json:"sector_conviction_multiplier"`
	CrowdedConvictionMultiplier      ParameterMetadata[float64] `json:"crowded_conviction_multiplier"`
	FactorWeightMomentum             ParameterMetadata[float64] `json:"factor_weight_momentum"`
	FactorWeightValue                ParameterMetadata[float64] `json:"factor_weight_value"`
	FactorWeightQuality              ParameterMetadata[float64] `json:"factor_weight_quality"`
	FactorWeightAgent                ParameterMetadata[float64] `json:"factor_weight_agent"`
	PRISMBoostMultiplier             ParameterMetadata[float64] `json:"prism_boost_multiplier"`
	PRISMBoostMin                    ParameterMetadata[int]     `json:"prism_boost_min"`
	PRISMBoostMax                    ParameterMetadata[int]     `json:"prism_boost_max"`
	PromotionMinObservations         ParameterMetadata[int]     `json:"promotion_min_observations"`
	PromotionSharpeThreshold         ParameterMetadata[float64] `json:"promotion_sharpe_threshold"`
	PromotionHitRateThreshold        ParameterMetadata[float64] `json:"promotion_hitrate_threshold"`
	RejectionSharpeThreshold         ParameterMetadata[float64] `json:"rejection_sharpe_threshold"`
	RejectionHitRateThreshold        ParameterMetadata[float64] `json:"rejection_hitrate_threshold"`
}

// RiskParameters holds tunable values for risk management.
type RiskParameters struct {
	VaRConfidenceLevel              ParameterMetadata[float64]            `json:"var_confidence_level"`
	VaRSecondaryConfidence          ParameterMetadata[float64]            `json:"var_secondary_confidence"`
	VaRAlertThreshold               ParameterMetadata[float64]            `json:"var_alert_threshold"`
	VaRCriticalThreshold            ParameterMetadata[float64]            `json:"var_critical_threshold"`
	ConsecutiveLossLimit            ParameterMetadata[int]                `json:"consecutive_loss_limit"`
	SectorConstraintsRiskOff        ParameterMetadata[map[string]float64] `json:"sector_constraints_risk_off"`
	SectorConstraintsCarryTrade     ParameterMetadata[map[string]float64] `json:"sector_constraints_carry_trade_unwind"`
	SectorConstraintsSectorRotation ParameterMetadata[map[string]float64] `json:"sector_constraints_sector_rotation"`
	MaxDrawdownPct                  ParameterMetadata[float64]            `json:"max_drawdown_pct"`
	MaxPositionSize                 ParameterMetadata[float64]            `json:"max_position_size"`
	MaxDailyLossPct                 ParameterMetadata[float64]            `json:"max_daily_loss_pct"`
	StopLoss                        ParameterMetadata[float64]            `json:"stop_loss"`
	TakeProfit                      ParameterMetadata[float64]            `json:"take_profit"`
	MaxLossPerTrade                 ParameterMetadata[float64]            `json:"max_loss_per_trade"`
	MaxTotalExposure                ParameterMetadata[float64]            `json:"max_total_exposure"`
}

// NarrativeParameters holds tunable values for macro narrative event detection,
// structural trend assessment, and Taiwan stress index computation.
type NarrativeParameters struct {
	// Structural trend thresholds
	MinTrendStrength  ParameterMetadata[float64] `json:"min_trend_strength"`
	MinConfidence     ParameterMetadata[float64] `json:"min_confidence"`
	MinHitRate        ParameterMetadata[float64] `json:"min_hit_rate"`
	OverrideThreshold ParameterMetadata[float64] `json:"override_threshold"`

	// Event detection thresholds (ingestor)
	AIRevenueGrowthThreshold  ParameterMetadata[float64] `json:"ai_revenue_growth_threshold"`
	CoWoSUtilizationThreshold ParameterMetadata[float64] `json:"cowos_utilization_threshold"`
	CapexGrowthThreshold      ParameterMetadata[float64] `json:"capex_growth_threshold"`
	US10YChangeBpsThreshold   ParameterMetadata[float64] `json:"us10y_change_bps_threshold"`
	DXYChangePctThreshold     ParameterMetadata[float64] `json:"dxy_change_pct_threshold"`
	GeopoliticalGPRThreshold  ParameterMetadata[float64] `json:"geopolitical_gpr_threshold"`
	OilChangePctThreshold     ParameterMetadata[float64] `json:"oil_change_pct_threshold"`
	JPYChangePctThreshold     ParameterMetadata[float64] `json:"jpy_change_pct_threshold"`
	VIXLevelThreshold         ParameterMetadata[float64] `json:"vix_level_threshold"`

	// Taiwan stress index weights (must sum to 1.0)
	TaiwanStressDXYWeight     ParameterMetadata[float64] `json:"taiwan_stress_dxy_weight"`
	TaiwanStressUS10YWeight   ParameterMetadata[float64] `json:"taiwan_stress_us10y_weight"`
	TaiwanStressForeignWeight ParameterMetadata[float64] `json:"taiwan_stress_foreign_weight"`
	TaiwanStressVIXWeight     ParameterMetadata[float64] `json:"taiwan_stress_vix_weight"`
	TaiwanStressJPYWeight     ParameterMetadata[float64] `json:"taiwan_stress_jpy_weight"`
	TaiwanStressGeoWeight     ParameterMetadata[float64] `json:"taiwan_stress_geo_weight"`

	// Event lifecycle TTL multipliers (days per theme)
	EventTTLMultiplier ParameterMetadata[map[string]float64] `json:"event_ttl_multiplier"`

	// Model evaluation windows
	ModelLookbackDays   ParameterMetadata[int] `json:"model_lookback_days"`
	ModelHoldWindowDays ParameterMetadata[int] `json:"model_hold_window_days"`
}

// RealtimeParameters holds tunable values for real-time regime detection and adaptation.
type RealtimeParameters struct {
	VolatilityThreshold  ParameterMetadata[float64] `json:"volatility_threshold"`
	VolumeSpikeThreshold ParameterMetadata[float64] `json:"volume_spike_threshold"`
	PriceChangeThreshold ParameterMetadata[float64] `json:"price_change_threshold"`
	MinConfidence        ParameterMetadata[float64] `json:"min_confidence"`
	WeightAdjustmentRate ParameterMetadata[float64] `json:"weight_adjustment_rate"`
	MaxWeightChange      ParameterMetadata[float64] `json:"max_weight_change"`
	MinWeight            ParameterMetadata[float64] `json:"min_weight"`
	UpdateIntervalMs     ParameterMetadata[int]     `json:"update_interval_ms"`
}

// JanusParameters holds tunable values for the JANUS meta-layer (cross-cohort
// regime detection and dynamic weight adjustment).
type JanusParameters struct {
	ShortWindowDays     ParameterMetadata[int]     `json:"short_window_days"`
	MediumWindowDays    ParameterMetadata[int]     `json:"medium_window_days"`
	LongWindowDays      ParameterMetadata[int]     `json:"long_window_days"`
	MaxHistoryDays      ParameterMetadata[int]     `json:"max_history_days"`
	MinWeight           ParameterMetadata[float64] `json:"min_weight"`
	MaxWeight           ParameterMetadata[float64] `json:"max_weight"`
	NovelThreshold      ParameterMetadata[float64] `json:"novel_threshold"`
	HistoricalThreshold ParameterMetadata[float64] `json:"historical_threshold"`
	EpsilonWeight       ParameterMetadata[float64] `json:"epsilon_weight"`
	ShortWindowBlend    ParameterMetadata[float64] `json:"short_window_blend"`
	MediumWindowBlend   ParameterMetadata[float64] `json:"medium_window_blend"`
	LongWindowBlend     ParameterMetadata[float64] `json:"long_window_blend"`
	HealthStaleHours    ParameterMetadata[int]     `json:"health_stale_hours"`
	HealthWarnHours     ParameterMetadata[int]     `json:"health_warn_hours"`
}

// MarketdataParameters holds tunable values for data provider configuration.
type MarketdataParameters struct {
	TWSEAPIRateLimit     ParameterMetadata[float64] `json:"twse_api_rate_limit"`
	TWSEAPIRateBurst     ParameterMetadata[int]     `json:"twse_api_rate_burst"`
	TWSEAPITimeoutSec    ParameterMetadata[int]     `json:"twse_api_timeout_sec"`
	FubonIntradayLimit   ParameterMetadata[int]     `json:"fubon_intraday_limit"`
	FubonHistoricalLimit ParameterMetadata[int]     `json:"fubon_historical_limit"`
	FubonAPITimeoutSec   ParameterMetadata[int]     `json:"fubon_api_timeout_sec"`
	TEJCallsPerSecond    ParameterMetadata[int]     `json:"tej_calls_per_second"`
	TEJAPITimeoutSec     ParameterMetadata[int]     `json:"tej_api_timeout_sec"`
	FugleRateLimit       ParameterMetadata[int]     `json:"fugle_rate_limit"`
	FugleAPITimeoutSec   ParameterMetadata[int]     `json:"fugle_api_timeout_sec"`
	MaxRetryAttempts     ParameterMetadata[int]     `json:"max_retry_attempts"`
	RetryBackoffMs       ParameterMetadata[int]     `json:"retry_backoff_ms"`
}

// IndustryParameters holds tunable values for industry analysis, seasonality,
// business cycle detection, and risk scoring.
type IndustryParameters struct {
	// Sector weights aligned with Taiwan market structure
	SectorWeights ParameterMetadata[map[string]float64] `json:"sector_weights"`

	// Business cycle thresholds per industry
	CycleThresholds ParameterMetadata[map[string]CycleThresholdConfig] `json:"cycle_thresholds"`

	// Inventory cycle detection thresholds
	InventoryCycleThresholds ParameterMetadata[InventoryCycleThresholdConfig] `json:"inventory_cycle_thresholds"`

	// Capex cycle detection thresholds
	CapexCycleThresholds ParameterMetadata[CapexCycleThresholdConfig] `json:"capex_cycle_thresholds"`

	// Risk scoring parameters
	ConcentrationRiskEnabled   ParameterMetadata[bool]    `json:"concentration_risk_enabled"`
	NewsLatencyRiskEnabled     ParameterMetadata[bool]    `json:"news_latency_risk_enabled"`
	AsymmetricRiskEnabled      ParameterMetadata[bool]    `json:"asymmetric_risk_enabled"`
	CustomerConcentrationLimit ParameterMetadata[float64] `json:"customer_concentration_limit"`
	GeographicExposureLimit    ParameterMetadata[float64] `json:"geographic_exposure_limit"`
	CustomerShareThreshold1    ParameterMetadata[float64] `json:"customer_share_threshold_1"`
	CustomerShareThreshold2    ParameterMetadata[float64] `json:"customer_share_threshold_2"`
	USExposureThreshold1       ParameterMetadata[float64] `json:"us_exposure_threshold_1"`
	USExposureThreshold2       ParameterMetadata[float64] `json:"us_exposure_threshold_2"`
	RiskScoreWeight1           ParameterMetadata[float64] `json:"risk_score_weight_1"`
	RiskScoreWeight2           ParameterMetadata[float64] `json:"risk_score_weight_2"`
	RiskScoreWeight3           ParameterMetadata[float64] `json:"risk_score_weight_3"`
	RiskScoreWeight4           ParameterMetadata[float64] `json:"risk_score_weight_4"`
	SeverityThresholdMedium    ParameterMetadata[float64] `json:"severity_threshold_medium"`
	SeverityThresholdHigh      ParameterMetadata[float64] `json:"severity_threshold_high"`
	SeverityThresholdCritical  ParameterMetadata[float64] `json:"severity_threshold_critical"`
	ImpactMultiplier           ParameterMetadata[float64] `json:"impact_multiplier"`
	RiskConfidence             ParameterMetadata[float64] `json:"risk_confidence"`

	ConfidenceSignal ParameterMetadata[ConfidenceSignalConfig] `json:"confidence_signal"`
	ConfidenceMix    ParameterMetadata[ConfidenceMixConfig]    `json:"confidence_mix"`

	SeasonalPatterns ParameterMetadata[[]SeasonalPatternConfig] `json:"seasonal_patterns"`

	AsymmetricRisk   ParameterMetadata[AsymmetricRiskConfig]    `json:"asymmetric_risk"`
	NewsLatencyRisk  ParameterMetadata[NewsLatencyConfig]       `json:"news_latency_risk"`
	FreshnessScores  ParameterMetadata[FreshnessScoresConfig]   `json:"freshness_scores"`
	PhaseScores      ParameterMetadata[PhaseScoresConfig]       `json:"phase_scores"`
	CycleTransitions ParameterMetadata[[]CycleTransitionConfig] `json:"cycle_transitions"`

	CycleWeightMultipliers ParameterMetadata[CycleWeightMultipliersConfig] `json:"cycle_weight_multipliers"`
	LinkageWeightImpact    ParameterMetadata[float64]                      `json:"linkage_weight_impact"`
	WeightFloor            ParameterMetadata[float64]                      `json:"weight_floor"`
	MaxDailyWeightChange   ParameterMetadata[float64]                      `json:"max_daily_weight_change"`

	LinkageParams ParameterMetadata[LinkageConfig] `json:"linkage_params"`

	DynamicEnv ParameterMetadata[DynamicEnvConfig] `json:"dynamic_env"`

	// Cycle tracking operational parameters
	HistoryRetentionDays ParameterMetadata[int] `json:"history_retention_days"`
}

// CycleThresholdConfig holds business cycle thresholds for a specific industry.
type CycleThresholdConfig struct {
	ExpansionRevenuePct float64 `json:"expansion_revenue_pct"`
	ExpansionProfitPct  float64 `json:"expansion_profit_pct"`
	RecoveryRevenuePct  float64 `json:"recovery_revenue_pct"`
	RecoveryProfitPct   float64 `json:"recovery_profit_pct"`
	MatureRevenuePct    float64 `json:"mature_revenue_pct"`
	MatureProfitPct     float64 `json:"mature_profit_pct"`
}

// InventoryCycleThresholdConfig holds inventory cycle detection thresholds.
type InventoryCycleThresholdConfig struct {
	ActiveRestockingInventoryMin  float64 `json:"active_restocking_inventory_min"`
	ActiveRestockingCapacityMin   float64 `json:"active_restocking_capacity_min"`
	PassiveRestockingInventoryMin float64 `json:"passive_restocking_inventory_min"`
	PassiveRestockingCapacityMin  float64 `json:"passive_restocking_capacity_min"`
	ActiveDestockingInventoryMax  float64 `json:"active_destocking_inventory_max"`
	ActiveDestockingCapacityMax   float64 `json:"active_destocking_capacity_max"`
}

// CapexCycleThresholdConfig holds capex cycle detection thresholds.
type CapexCycleThresholdConfig struct {
	ExpansionCapacityMin   float64 `json:"expansion_capacity_min"`
	ExpansionRevenueMin    float64 `json:"expansion_revenue_min"`
	MaintenanceCapacityMin float64 `json:"maintenance_capacity_min"`
	MaintenanceRevenueMin  float64 `json:"maintenance_revenue_min"`
}

type ConfidenceSignalConfig struct {
	SignalBase          float64 `json:"signal_base"`
	RevenueNormDenom    float64 `json:"revenue_norm_denom"`
	RevenueWeight       float64 `json:"revenue_weight"`
	ProfitNormDenom     float64 `json:"profit_norm_denom"`
	ProfitWeight        float64 `json:"profit_weight"`
	InventoryNormDenom  float64 `json:"inventory_norm_denom"`
	InventoryWeight     float64 `json:"inventory_weight"`
	UtilizationWeight   float64 `json:"utilization_weight"`
	SignalBoundaryMix   float64 `json:"signal_boundary_mix"`
	BoundaryDenomFactor float64 `json:"boundary_denom_factor"`
	ConfidenceFloor     float64 `json:"confidence_floor"`
	ConfidenceCeiling   float64 `json:"confidence_ceiling"`

	// Indicator trend detection thresholds and weights
	RevenueTrendThreshold    float64 `json:"revenue_trend_threshold"`
	RevenueIndicatorWeight   float64 `json:"revenue_indicator_weight"`
	InventoryTrendThreshold  float64 `json:"inventory_trend_threshold"`
	InventoryIndicatorWeight float64 `json:"inventory_indicator_weight"`
	ProfitTrendThreshold     float64 `json:"profit_trend_threshold"`
	ProfitIndicatorWeight    float64 `json:"profit_indicator_weight"`
	CapacityTrendThreshold   float64 `json:"capacity_trend_threshold"`
	CapacityIndicatorWeight  float64 `json:"capacity_indicator_weight"`

	// Trend deviation multipliers (value > threshold*up = "up", < threshold*down = "down")
	TrendUpMultiplier   float64 `json:"trend_up_multiplier"`
	TrendDownMultiplier float64 `json:"trend_down_multiplier"`

	// Fallback when cycle threshold range (expansion - mature) <= 0
	ThresholdRangeFallback float64 `json:"threshold_range_fallback"`
}

type ConfidenceMixConfig struct {
	WeightBoundary         float64 `json:"weight_boundary"`
	WeightFreshness        float64 `json:"weight_freshness"`
	WeightSeasonal         float64 `json:"weight_seasonal"`
	WeightLinkage          float64 `json:"weight_linkage"`
	WeightNarrative        float64 `json:"weight_narrative"`
	FavorableConfidenceMin float64 `json:"favorable_confidence_min"`
}

type SeasonalPatternConfig struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	NameEN             string   `json:"name_en"`
	StartMonth         int      `json:"start_month"`
	StartDay           int      `json:"start_day"`
	EndMonth           int      `json:"end_month"`
	EndDay             int      `json:"end_day"`
	FavoredIndustries  []string `json:"favored_industries"`
	AvoidedIndustries  []string `json:"avoided_industries"`
	StyleTags          []string `json:"style_tags,omitempty"`
	AdjustmentFactor   float64  `json:"adjustment_factor"`
	HistoricalAccuracy float64  `json:"historical_accuracy"`
	AvgMarketReturn    float64  `json:"avg_market_return"`
	Description        string   `json:"description"`
}

type LinkageConfig struct {
	DownstreamDecayFactor     float64            `json:"downstream_decay_factor"`
	UpstreamDecayFactor       float64            `json:"upstream_decay_factor"`
	SeasonalDecayFactor       float64            `json:"seasonal_decay_factor"`
	DefaultCorrelation        float64            `json:"default_correlation"`
	SystemicImportanceDivisor float64            `json:"systemic_importance_divisor"`
	MinCorrelationThreshold   float64            `json:"min_correlation_threshold"`
	CorrelationWindowDays     int                `json:"correlation_window_days"`
	CorrelationMatrix         map[string]float64 `json:"correlation_matrix"`
	RecessionCorrelationBoost float64 `json:"recession_correlation_boost"`
}

// AsymmetricRiskConfig holds thresholds for asymmetric (bad news) risk detection.
type AsymmetricRiskConfig struct {
	BadNewsThreshold      float64 `json:"bad_news_threshold"`
	GoodNewsThreshold     float64 `json:"good_news_threshold"`
	ReactionTimeMinutes   int     `json:"reaction_time_minutes"`
	VolumeSpikeMultiplier float64 `json:"volume_spike_multiplier"`
}

// NewsLatencyConfig holds thresholds for news latency risk detection.
type NewsLatencyConfig struct {
	MaxLatencyHours       float64 `json:"max_latency_hours"`
	SeverityCriticalMin   float64 `json:"severity_critical_min"`
	SeverityHighMin       float64 `json:"severity_high_min"`
	ImpactMultiplier      float64 `json:"impact_multiplier"`
	DropCriticalThreshold float64 `json:"drop_critical_threshold"`
	DropHighThreshold     float64 `json:"drop_high_threshold"`
	DropMediumThreshold   float64 `json:"drop_medium_threshold"`
	ConfidenceDivisor     float64 `json:"confidence_divisor"`
}

// FreshnessScoresConfig maps DataFreshness enum values to numeric scores.
type FreshnessScoresConfig struct {
	ScoreLive     float64 `json:"score_live"`
	ScoreRecent   float64 `json:"score_recent"`
	ScoreStale    float64 `json:"score_stale"`
	ScoreFallback float64 `json:"score_fallback"`
	ScoreDefault  float64 `json:"score_default"`
}

// PhaseScoresConfig maps cycle phases to numeric scores.
type PhaseScoresConfig struct {
	ScoreExpansion float64 `json:"score_expansion"`
	ScoreRecovery  float64 `json:"score_recovery"`
	ScoreMature    float64 `json:"score_mature"`
	ScoreRecession float64 `json:"score_recession"`
}

type CycleWeightMultipliersConfig struct {
	ExpansionMultiplier float64 `json:"expansion_multiplier"`
	RecoveryMultiplier  float64 `json:"recovery_multiplier"`
	MatureMultiplier    float64 `json:"mature_multiplier"`
	RecessionMultiplier float64 `json:"recession_multiplier"`
}

// CycleTransitionConfig holds probability and typical duration for a cycle phase transition.
type CycleTransitionConfig struct {
	FromPhase           string   `json:"from_phase"`
	ToPhase             string   `json:"to_phase"`
	Triggers            []string `json:"triggers"`
	Probability         float64  `json:"probability"`
	TypicalDurationDays int      `json:"typical_duration_days"`
}

type DynamicEnvConfig struct {
	OilHighThreshold     float64 `json:"oil_high_threshold"`
	OilLowThreshold      float64 `json:"oil_low_threshold"`
	OilEnergyMult        float64 `json:"oil_energy_mult"`
	OilShippingPenalty   float64 `json:"oil_shipping_penalty"`
	OilShippingBenefit   float64 `json:"oil_shipping_benefit"`
	OilIndustrialPenalty float64 `json:"oil_industrial_penalty"`
	OilIndustrialBenefit float64 `json:"oil_industrial_benefit"`
	BDIHighThreshold     float64 `json:"bdi_high_threshold"`
	BDILowThreshold      float64 `json:"bdi_low_threshold"`
	BDIShippingBoost     float64 `json:"bdi_shipping_boost"`
	BDICostPenalty       float64 `json:"bdi_cost_penalty"`
	DXYHighThreshold     float64 `json:"dxy_high_threshold"`
	DXYLowThreshold      float64 `json:"dxy_low_threshold"`
	DXYExportPenalty     float64 `json:"dxy_export_penalty"`
	DXYExportBenefit     float64 `json:"dxy_export_benefit"`
}

// StrategyParameters holds tunable values for strategy selection and switching.
type StrategyParameters struct {
	MinSwitchIntervalDays ParameterMetadata[int]     `json:"min_switch_interval_days"`
	SwitchThreshold       ParameterMetadata[float64] `json:"switch_threshold"`
	ScoreLookbackDays     ParameterMetadata[int]     `json:"score_lookback_days"`
	FallbackStrategy      ParameterMetadata[string]  `json:"fallback_strategy"`
}

// ParametersConfig is the top-level configuration for all investment model parameters.
type ParametersConfig struct {
	Version      string                 `json:"version"`
	UpdatedAt    time.Time              `json:"updated_at"`
	Darwinian    DarwinianParameters    `json:"darwinian"`
	Factor       FactorParameters       `json:"factor"`
	Optimizer    OptimizerParameters    `json:"optimizer"`
	Sizing       SizingParameters       `json:"sizing"`
	Health       HealthParameters       `json:"health"`
	GARCH        GARCHParameters        `json:"garch"`
	Experiment   ExperimentParameters   `json:"experiment"`
	Baseline     BaselineParameters     `json:"baseline"`
	Orchestrator OrchestratorParameters `json:"orchestrator"`
	Risk         RiskParameters         `json:"risk"`
	Realtime     RealtimeParameters     `json:"realtime"`
	Janus        JanusParameters        `json:"janus"`
	Narrative    NarrativeParameters    `json:"narrative"`
	Marketdata   MarketdataParameters   `json:"marketdata"`
	Industry     IndustryParameters     `json:"industry"`
	Strategy     StrategyParameters     `json:"strategy"`
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
		p.Narrative.TaiwanStressGeoWeight.Value
	if math.Abs(stressWeightSum-1.0) > 0.01 {
		return fmt.Errorf("narrative taiwan stress weights must sum to 1.0, got %.3f", stressWeightSum)
	}
	if p.Narrative.ModelLookbackDays.Value < 1 {
		return fmt.Errorf("narrative.model_lookback_days (%d) must be >= 1", p.Narrative.ModelLookbackDays.Value)
	}
	if p.Narrative.ModelHoldWindowDays.Value < 1 {
		return fmt.Errorf("narrative.model_hold_window_days (%d) must be >= 1", p.Narrative.ModelHoldWindowDays.Value)
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

	if p.Industry.SectorWeights.Value != nil {
		var totalWeight float64
		for sector, weight := range p.Industry.SectorWeights.Value {
			if weight < 0 || weight > 1 {
				return fmt.Errorf("industry.sector_weights[%s] (%.3f) must be in [0,1]", sector, weight)
			}
			totalWeight += weight
		}
		if math.Abs(totalWeight-1.0) > 0.05 {
			return fmt.Errorf("industry.sector_weights must sum to ~1.0, got %.3f", totalWeight)
		}
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
		if sp.AdjustmentFactor <= 0 {
			return fmt.Errorf("industry.seasonal_patterns[%d].adjustment_factor (%.3f) must be > 0", i, sp.AdjustmentFactor)
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

	return nil
}

var (
	parametersConfig *ParametersConfig
	parametersPath   = envOr("ATLAS_PARAMETERS_CONFIG", "configs/parameters.json")
)

// LoadParametersConfig loads parameters from the given JSON file.
// If the file does not exist or is invalid, it returns the default configuration.
func LoadParametersConfig(path string) (*ParametersConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultParametersConfig(), nil
		}
		return nil, fmt.Errorf("read parameters config: %w", err)
	}

	var cfg ParametersConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse parameters config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate parameters config: %w", err)
	}

	return &cfg, nil
}

// GetParametersConfig returns the singleton parameters configuration.
func GetParametersConfig() *ParametersConfig {
	if parametersConfig == nil {
		cfg, err := LoadParametersConfig(parametersPath)
		if err != nil {
			return DefaultParametersConfig()
		}
		parametersConfig = cfg
	}
	return parametersConfig
}

// Save writes the configuration to the given JSON file.
func (p *ParametersConfig) Save(path string) error {
	p.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal parameters config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write parameters config: %w", err)
	}
	return nil
}
