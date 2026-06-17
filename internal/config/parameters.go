package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// ParameterSource indicates how a parameter value was determined.
type ParameterSource string

const (
	SourceLiterature   ParameterSource = "literature"   // from academic/practitioner literature
	SourceEmpirical    ParameterSource = "empirical"    // from historical data analysis
	SourceHeuristic    ParameterSource = "heuristic"    // from domain expert judgment
	SourceInferred     ParameterSource = "inferred"     // from automated inference/calibration
	SourceCalibrated   ParameterSource = "calibrated"   // from backtest optimization
	SourceExperimental ParameterSource = "experimental" // from ML experiment / not yet validated
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
	FactorWeightDriftThreshold ParameterMetadata[float64] `json:"factor_weight_drift_threshold"`
	WalkForwardEmbargoDays     ParameterMetadata[int]     `json:"walk_forward_embargo_days"`
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
	DiscountedCommissionBps     ParameterMetadata[float64] `json:"discounted_commission_bps"`
	CommissionDiscountThreshold ParameterMetadata[float64] `json:"commission_discount_threshold"`
	SlippageBPS                 ParameterMetadata[float64] `json:"slippage_bps"`
	AvgTradingCost              ParameterMetadata[float64] `json:"avg_trading_cost"`
	ReserveCashFraction         ParameterMetadata[float64] `json:"reserve_cash_fraction"`
}

// OrchestratorParameters holds tunable values for the orchestrator executors,
// control layer (CRO/CIO), and Phase3 controller.
type OrchestratorParameters struct {
	ConvictionFloorDefault           ParameterMetadata[int]                           `json:"conviction_floor_default"`
	SuperinvestorMinConviction       ParameterMetadata[int]                           `json:"superinvestor_min_conviction"`
	SuperinvestorConvictionBase      ParameterMetadata[int]                           `json:"superinvestor_conviction_base"`
	CROZScoreThreshold               ParameterMetadata[float64]                       `json:"cro_zscore_threshold"`
	SectorConcentrationThreshold     ParameterMetadata[float64]                       `json:"sector_concentration_threshold"`
	SectorConcentrationThresholdHigh ParameterMetadata[float64]                       `json:"sector_concentration_threshold_high"`
	SectorConvictionMultiplier       ParameterMetadata[float64]                       `json:"sector_conviction_multiplier"`
	CrowdedConvictionMultiplier      ParameterMetadata[float64]                       `json:"crowded_conviction_multiplier"`
	FactorWeightMomentum             ParameterMetadata[float64]                       `json:"factor_weight_momentum"`
	FactorWeightValue                ParameterMetadata[float64]                       `json:"factor_weight_value"`
	FactorWeightQuality              ParameterMetadata[float64]                       `json:"factor_weight_quality"`
	FactorWeightAgent                ParameterMetadata[float64]                       `json:"factor_weight_agent"`
	PRISMBoostMultiplier             ParameterMetadata[float64]                       `json:"prism_boost_multiplier"`
	PRISMBoostMin                    ParameterMetadata[int]                           `json:"prism_boost_min"`
	PRISMBoostMax                    ParameterMetadata[int]                           `json:"prism_boost_max"`
	PromotionMinObservations         ParameterMetadata[int]                           `json:"promotion_min_observations"`
	PromotionSharpeThreshold         ParameterMetadata[float64]                       `json:"promotion_sharpe_threshold"`
	PromotionHitRateThreshold        ParameterMetadata[float64]                       `json:"promotion_hitrate_threshold"`
	RejectionSharpeThreshold         ParameterMetadata[float64]                       `json:"rejection_sharpe_threshold"`
	RejectionHitRateThreshold        ParameterMetadata[float64]                       `json:"rejection_hitrate_threshold"`
	SectorRotationMacroAdjustments   ParameterMetadata[map[string]map[string]float64] `json:"sector_rotation_macro_adjustments,omitempty"`
	SectorRotationFlowAdjustments    ParameterMetadata[map[string]map[string]float64] `json:"sector_rotation_flow_adjustments,omitempty"`
	UseMLScoring                     ParameterMetadata[bool]                          `json:"use_ml_scoring"`
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

// DrawdownParameters holds tunable values for macro-aware drawdown decision engine,
// including drawdown levels, structural override thresholds, and sector constraints.
type DrawdownParameters struct {
	NonePercentage                    ParameterMetadata[float64]            `json:"none_percentage"`
	NoneMaxExposure                   ParameterMetadata[float64]            `json:"none_max_exposure"`
	LightPercentage                   ParameterMetadata[float64]            `json:"light_percentage"`
	LightMaxExposure                  ParameterMetadata[float64]            `json:"light_max_exposure"`
	ModeratePercentage                ParameterMetadata[float64]            `json:"moderate_percentage"`
	ModerateMaxExposure               ParameterMetadata[float64]            `json:"moderate_max_exposure"`
	SeverePercentage                  ParameterMetadata[float64]            `json:"severe_percentage"`
	SevereMaxExposure                 ParameterMetadata[float64]            `json:"severe_max_exposure"`
	EmergencyPercentage               ParameterMetadata[float64]            `json:"emergency_percentage"`
	EmergencyMaxExposure              ParameterMetadata[float64]            `json:"emergency_max_exposure"`
	OrangeOverrideMinScore            ParameterMetadata[float64]            `json:"orange_override_min_score"`
	RedOverrideMinScore               ParameterMetadata[float64]            `json:"red_override_min_score"`
	SectorConstraintsRiskOff          ParameterMetadata[map[string]float64] `json:"sector_constraints_risk_off"`
	SectorConstraintsCarryTradeUnwind ParameterMetadata[map[string]float64] `json:"sector_constraints_carry_trade_unwind"`
	SectorConstraintsSectorRotation   ParameterMetadata[map[string]float64] `json:"sector_constraints_sector_rotation"`
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

	// Extended event detection thresholds
	GoldChangePctThreshold                 ParameterMetadata[float64] `json:"gold_change_pct_threshold"`
	USDTWDChangePctThreshold               ParameterMetadata[float64] `json:"usdtwd_change_pct_threshold"`
	SemiconductorExportDropThreshold       ParameterMetadata[float64] `json:"semiconductor_export_drop_threshold"`
	RetailMarginZScoreThreshold            ParameterMetadata[float64] `json:"retail_margin_zscore_threshold"`
	AICapexSentimentThreshold              ParameterMetadata[float64] `json:"ai_capex_sentiment_threshold"`
	TSMCRevenueYoYThreshold                ParameterMetadata[float64] `json:"tsmc_revenue_yoy_threshold"`
	TaiwanStressUSDTWDThreshold            ParameterMetadata[float64] `json:"taiwan_stress_usdtwd_threshold"`
	RetailInstitutionalDivergenceThreshold ParameterMetadata[float64] `json:"retail_institutional_divergence_threshold"`
	AICapexNegativeSentimentThreshold      ParameterMetadata[float64] `json:"ai_capex_negative_sentiment_threshold"`
	AICapexFallbackSentiment               ParameterMetadata[float64] `json:"ai_capex_fallback_sentiment"`
	TSMCRevenuePositiveThreshold           ParameterMetadata[float64] `json:"tsmc_revenue_positive_threshold"`

	// Confidence base values for deviation-based dynamic confidence calculation
	ConfidenceBaseUSRates      ParameterMetadata[float64] `json:"confidence_base_us_rates"`
	ConfidenceBaseJPYCarry     ParameterMetadata[float64] `json:"confidence_base_jpy_carry"`
	ConfidenceBaseGeopolitical ParameterMetadata[float64] `json:"confidence_base_geopolitical"`
	ConfidenceBaseOilShock     ParameterMetadata[float64] `json:"confidence_base_oil_shock"`
	ConfidenceBaseAICapex      ParameterMetadata[float64] `json:"confidence_base_ai_capex"`
	ConfidenceBaseTSMCRevenue  ParameterMetadata[float64] `json:"confidence_base_tsmc_revenue"`
	ConfidenceBaseTaiwanStress ParameterMetadata[float64] `json:"confidence_base_taiwan_stress"`

	// Ceiling cap for deviation-based dynamic confidence; prevents perfect certainty (1.0).
	ConfidenceDeviationCeiling ParameterMetadata[float64] `json:"confidence_deviation_ceiling"`

	// SOX index drop threshold for semiconductor stress detection
	SOXIndexDropThreshold ParameterMetadata[float64] `json:"sox_index_drop_threshold"`

	TaiwanStressDXYWeight     ParameterMetadata[float64] `json:"taiwan_stress_dxy_weight"`
	TaiwanStressUS10YWeight   ParameterMetadata[float64] `json:"taiwan_stress_us10y_weight"`
	TaiwanStressForeignWeight ParameterMetadata[float64] `json:"taiwan_stress_foreign_weight"`
	TaiwanStressVIXWeight     ParameterMetadata[float64] `json:"taiwan_stress_vix_weight"`
	TaiwanStressJPYWeight     ParameterMetadata[float64] `json:"taiwan_stress_jpy_weight"`
	TaiwanStressGeoWeight     ParameterMetadata[float64] `json:"taiwan_stress_geo_weight"`
	TaiwanStressOilWeight     ParameterMetadata[float64] `json:"taiwan_stress_oil_weight"`
	TaiwanStressGoldWeight    ParameterMetadata[float64] `json:"taiwan_stress_gold_weight"`

	TaiwanStressDXYScale     ParameterMetadata[float64] `json:"taiwan_stress_dxy_scale"`
	TaiwanStressUS10YScale   ParameterMetadata[float64] `json:"taiwan_stress_us10y_scale"`
	TaiwanStressForeignScale ParameterMetadata[float64] `json:"taiwan_stress_foreign_scale"`
	TaiwanStressVIXScale     ParameterMetadata[float64] `json:"taiwan_stress_vix_scale"`
	TaiwanStressJPYScale     ParameterMetadata[float64] `json:"taiwan_stress_jpy_scale"`
	TaiwanStressGeoScale     ParameterMetadata[float64] `json:"taiwan_stress_geo_scale"`
	TaiwanStressOilScale     ParameterMetadata[float64] `json:"taiwan_stress_oil_scale"`
	TaiwanStressGoldScale    ParameterMetadata[float64] `json:"taiwan_stress_gold_scale"`

	TaiwanStressCrisisThreshold ParameterMetadata[float64] `json:"taiwan_stress_crisis_threshold"`
	TaiwanStressHighThreshold   ParameterMetadata[float64] `json:"taiwan_stress_high_threshold"`
	TaiwanStressAlertThreshold  ParameterMetadata[float64] `json:"taiwan_stress_alert_threshold"`

	// --- Rolling Calibration Framework Parameters ---
	CalibrationBaselineWindow ParameterMetadata[int]     `json:"calibration_baseline_window"`
	CalibrationTargetMedian   ParameterMetadata[float64] `json:"calibration_target_median"`
	CalibrationValidationPct  ParameterMetadata[float64] `json:"calibration_validation_pct"`
	CalibrationMinRecords     ParameterMetadata[int]     `json:"calibration_min_records"`
	CalibrationEnabled        ParameterMetadata[bool]    `json:"calibration_enabled"`

	// Event lifecycle TTL multipliers (days per theme)
	EventTTLMultiplier ParameterMetadata[map[string]float64] `json:"event_ttl_multiplier"`

	// Model evaluation windows
	ModelLookbackDays   ParameterMetadata[int] `json:"model_lookback_days"`
	ModelHoldWindowDays ParameterMetadata[int] `json:"model_hold_window_days"`

	// Retail margin event detection thresholds (ingestor)
	RetailFrenzyPercentileThreshold ParameterMetadata[float64] `json:"retail_frenzy_percentile_threshold"`
	RetailFearPercentileThreshold   ParameterMetadata[float64] `json:"retail_fear_percentile_threshold"`
	RetailAccelerationWindowDays    ParameterMetadata[int]     `json:"retail_acceleration_window_days"`
	InflationEstimate               ParameterMetadata[float64] `json:"inflation_estimate,omitempty"`

	// Seasonal event detection confidence values (calendar-based events)
	SpringFestivalConfidence        ParameterMetadata[float64] `json:"spring_festival_confidence"`
	ElectionCycleConfidence         ParameterMetadata[float64] `json:"election_cycle_confidence"`
	EarningsBlackoutConfidence      ParameterMetadata[float64] `json:"earnings_blackout_confidence"`
	TechPeakSeasonConfidence        ParameterMetadata[float64] `json:"tech_peak_season_confidence"`
	YearEndWindowDressingConfidence ParameterMetadata[float64] `json:"year_end_window_dressing_confidence"`

	// Externally-triggered event confidence baselines (not calendar-based; consumed by ingestor/swarm detectors)
	EarningsSurpriseConfidence ParameterMetadata[float64] `json:"earnings_surprise_confidence"`
	EarningsSurpriseThreshold  ParameterMetadata[float64] `json:"earnings_surprise_threshold"`
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
	BDIAPITimeoutSec     ParameterMetadata[int]     `json:"bdi_api_timeout_sec"`
	BDIEndpoint          ParameterMetadata[string]  `json:"bdi_endpoint"`
	MaxRetryAttempts     ParameterMetadata[int]     `json:"max_retry_attempts"`
	RetryBackoffMs       ParameterMetadata[int]     `json:"retry_backoff_ms"`
}

// IndustrySegmentConfig holds a single industry segment definition for the classification tree.
// This is the ParameterConfig-compatible version of industry.IndustrySegment.
type IndustrySegmentConfig struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	NameEN               string   `json:"name_en"`
	Level                int      `json:"level"`
	ParentID             string   `json:"parent_id,omitempty"`
	Weight               float64  `json:"weight,omitempty"`
	GeographicExposure   string   `json:"geographic_exposure"`
	Cyclicality          string   `json:"cyclicality"`
	TechnologyIntensity  string   `json:"technology_intensity"`
	CapitalIntensity     string   `json:"capital_intensity"`
	RepresentativeStocks []string `json:"representative_stocks,omitempty"`
	Description          string   `json:"description,omitempty"`
}

// ClassificationTreeConfig holds the complete industry classification tree.
type ClassificationTreeConfig struct {
	Segments []IndustrySegmentConfig `json:"segments"`
}

// IndustryParameters holds tunable values for industry analysis, seasonality,
// business cycle detection, and risk scoring.
type IndustryParameters struct {
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

	AsymmetricRisk  ParameterMetadata[AsymmetricRiskConfig] `json:"asymmetric_risk"`
	NewsLatencyRisk ParameterMetadata[NewsLatencyConfig]    `json:"news_latency_risk"`

	AsymmetricDropCritical ParameterMetadata[float64]                 `json:"asymmetric_drop_critical"` // was 0.10 in risk.go:298
	AsymmetricDropHigh     ParameterMetadata[float64]                 `json:"asymmetric_drop_high"`     // was 0.07 in risk.go:300
	AsymmetricDropMedium   ParameterMetadata[float64]                 `json:"asymmetric_drop_medium"`   // was 0.05 in risk.go:302
	NewsImpactMultiplier   ParameterMetadata[float64]                 `json:"news_impact_multiplier"`   // was 0.05 in risk.go:275
	BoundaryFallback       ParameterMetadata[float64]                 `json:"boundary_fallback"`        // was 0.25 in cycle.go:602
	AdjustmentFloor        ParameterMetadata[float64]                 `json:"adjustment_floor"`         // was 0.01 in seasonality.go:270
	FreshnessScores        ParameterMetadata[FreshnessScoresConfig]   `json:"freshness_scores"`
	PhaseScores            ParameterMetadata[PhaseScoresConfig]       `json:"phase_scores"`
	SkillToIndustry        ParameterMetadata[map[string]string]       `json:"skill_to_industry,omitempty"`
	SkillToIndustries      ParameterMetadata[map[string][]string]     `json:"skill_to_industries,omitempty"`
	CycleTransitions       ParameterMetadata[[]CycleTransitionConfig] `json:"cycle_transitions"`

	CycleWeightMultipliers ParameterMetadata[CycleWeightMultipliersConfig] `json:"cycle_weight_multipliers"`
	LinkageWeightImpact    ParameterMetadata[float64]                      `json:"linkage_weight_impact"`
	WeightFloor            ParameterMetadata[float64]                      `json:"weight_floor"`
	MaxDailyWeightChange   ParameterMetadata[float64]                      `json:"max_daily_weight_change"`

	LinkageParams ParameterMetadata[LinkageConfig] `json:"linkage_params"`

	DynamicEnv ParameterMetadata[DynamicEnvConfig] `json:"dynamic_env"`

	// Cycle compass calibration parameters for per-layer accuracy tracking.
	CycleCalibration ParameterMetadata[CycleCalibrationConfig] `json:"cycle_calibration"`

	// Cycle tracking operational parameters
	HistoryRetentionDays ParameterMetadata[int] `json:"history_retention_days"`

	// Bootstrap seed values for CycleTracker initialization, used before
	// real FinMind data becomes available (replaced within 6h by auto_cycle_update).
	DefaultMetrics ParameterMetadata[map[string]IndustryDefaultMetrics] `json:"default_metrics"`

	SiliconCycle       ParameterMetadata[SiliconCycleParameters] `json:"silicon_cycle"`
	EventCalendarRules ParameterMetadata[[]EventCalendarRule]    `json:"event_calendar_rules"`

	// EventSentimentCap limits the per-event sentiment adjustment to prevent
	// any single calendar event from dominating the composite signal.
	EventSentimentCap ParameterMetadata[float64] `json:"event_sentiment_cap"`

	// CompositeCard holds tunable parameters for building the CycleStatusCard composite sentiment gauge.
	CompositeCard ParameterMetadata[CompositeCardConfig] `json:"composite_card"`

	// SeasonalMultipliers holds theme→industry multiplier maps for seasonal bridge narrative adjustments.
	SeasonalMultipliers ParameterMetadata[SeasonalMultiplierConfig] `json:"seasonal_multipliers"`

	// ClassificationTree defines the complete industry hierarchy (L1/L2/L3).
	// Previously hardcoded in internal/industry/types.go; now parameter-managed.
	ClassificationTree ParameterMetadata[ClassificationTreeConfig] `json:"classification_tree"`
}

// CompositeCardConfig holds tunable parameters for building the CycleStatusCard composite sentiment gauge.
type CompositeCardConfig struct {
	LayerWeights        map[string]float64         `json:"layer_weights"`
	SentimentThresholds map[string]SentimentBounds `json:"sentiment_thresholds"`
	ClampMin            float64                    `json:"clamp_min"`
	ClampMax            float64                    `json:"clamp_max"`
}

// SentimentBounds defines the value range for a sentiment label.
type SentimentBounds struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// SeasonalMultiplierConfig holds theme→industry multiplier maps for seasonal bridge narrative adjustments.
type SeasonalMultiplierConfig struct {
	ThemeMultipliers  map[string]IndustryMultiplierMap `json:"theme_multipliers"`
	ThemeCorrelations map[string]map[string]float64    `json:"theme_correlations"`
}

// IndustryMultiplierMap holds bull/bear multipliers per industry for a theme.
type IndustryMultiplierMap struct {
	BullMultiplier map[string]float64 `json:"bull_multiplier"`
	BearMultiplier map[string]float64 `json:"bear_multiplier"`
}

// MarshalJSON implements json.Marshaler. When Max is +Inf, it serializes as the
// string "+Inf"; otherwise it uses the standard numeric representation.
func (s SentimentBounds) MarshalJSON() ([]byte, error) {
	enc := struct {
		Min float64 `json:"min"`
		Max any     `json:"max"`
	}{Min: s.Min}
	if math.IsInf(s.Max, 1) {
		enc.Max = "+Inf"
	} else {
		enc.Max = s.Max
	}
	return json.Marshal(enc)
}

// UnmarshalJSON implements json.Unmarshaler, accepting "+Inf" string for Max
// in addition to standard numeric values.
func (s *SentimentBounds) UnmarshalJSON(data []byte) error {
	var num struct {
		Min float64 `json:"min"`
		Max float64 `json:"max"`
	}
	if err := json.Unmarshal(data, &num); err == nil {
		s.Min = num.Min
		s.Max = num.Max
		return nil
	}
	var str struct {
		Min float64 `json:"min"`
		Max string  `json:"max"`
	}
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	s.Min = str.Min
	if str.Max == "+Inf" {
		s.Max = math.Inf(1)
		return nil
	}
	return fmt.Errorf("unknown max value: %q (expected \"+Inf\" or number)", str.Max)
}

// IndustryDefaultMetrics holds bootstrap seed values for CycleTracker initialization.
// These replace the previously hardcoded defaults in initializeDefaultPositions(),
// allowing operators to tune seed values without recompiling. Replaced by real
// FinMind data within 6h (auto_cycle_update background task).
type IndustryDefaultMetrics struct {
	RevenueGrowthYoY    float64 `json:"revenue_growth_yoy"`
	ProfitGrowthYoY     float64 `json:"profit_growth_yoy"`
	InventoryTurnover   float64 `json:"inventory_turnover"`
	CapacityUtilization float64 `json:"capacity_utilization"`
}

// CycleCalibrationConfig holds calibration parameters for the cycle compass
// layer weight self-calibration loop. Each layer's hit rate is tracked over
// a rolling window and weights are adjusted up/down based on accuracy.
type CycleCalibrationConfig struct {
	MinSamples     int     `json:"min_samples"`
	LearningRate   float64 `json:"learning_rate"`
	HitRateHigh    float64 `json:"hit_rate_high"`
	HitRateLow     float64 `json:"hit_rate_low"`
	WeightClampMin float64 `json:"weight_clamp_min"`
	WeightClampMax float64 `json:"weight_clamp_max"`
	WindowSize     int     `json:"window_size"`
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
	RecessionCorrelationBoost float64            `json:"recession_correlation_boost"`
	RecessionShockAmplifier   float64            `json:"recession_shock_amplifier"`
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
	OilHighThreshold       float64 `json:"oil_high_threshold"`
	OilLowThreshold        float64 `json:"oil_low_threshold"`
	OilEnergyMult          float64 `json:"oil_energy_mult"`
	OilShippingPenalty     float64 `json:"oil_shipping_penalty"`
	OilShippingBenefit     float64 `json:"oil_shipping_benefit"`
	OilIndustrialPenalty   float64 `json:"oil_industrial_penalty"`
	OilIndustrialBenefit   float64 `json:"oil_industrial_benefit"`
	BDIHighThreshold       float64 `json:"bdi_high_threshold"`
	BDILowThreshold        float64 `json:"bdi_low_threshold"`
	BDIShippingBoost       float64 `json:"bdi_shipping_boost"`
	BDICostPenalty         float64 `json:"bdi_cost_penalty"`
	DXYHighThreshold       float64 `json:"dxy_high_threshold"`
	DXYLowThreshold        float64 `json:"dxy_low_threshold"`
	DXYExportPenalty       float64 `json:"dxy_export_penalty"`
	DXYExportBenefit       float64 `json:"dxy_export_benefit"`
	HistoryWindowDays      int     `json:"history_window_days"`
	HistoryCapMultiplier   int     `json:"history_cap_multiplier"`
	OilPriceShockThreshold float64 `json:"oil_price_shock_threshold"`
	UsRatesDxyThreshold    float64 `json:"us_rates_dxy_threshold"`
	JpyCarryDxyThreshold   float64 `json:"jpy_carry_dxy_threshold"`
}

// SiliconCycleParameters holds thresholds for semiconductor silicon cycle phase detection.
// Field names correspond 1:1 with SiliconCycleParams in internal/industry/silicon_cycle.go,
// plus two forward-looking fields (InventoryDaysThreshold, UtilizationThreshold) reserved
// for future inventory-based and capacity-utilization-based cycle detection layers.
type SiliconCycleParameters struct {
	RevenueYoYThreshold            float64 `json:"revenue_yoy_threshold"`
	BillingsYoYThreshold           float64 `json:"billings_yoy_threshold"`
	DRAMStabilizationThreshold     float64 `json:"dram_stabilization_threshold"`
	BillingsStabilizationThreshold float64 `json:"billings_stabilization_threshold"`
	InventoryDaysThreshold         float64 `json:"inventory_days_threshold"`
	UtilizationThreshold           float64 `json:"utilization_threshold"`
	IndexMAPercentThreshold        float64 `json:"index_ma_percent_threshold"`
	SOXExtremeThreshold            float64 `json:"sox_extreme_threshold"`
	CapexCutThreshold              float64 `json:"capex_cut_threshold"`
	MinConfidence                  float64 `json:"min_confidence"`
	HistoryWindowSize              int     `json:"history_window_size"`
}

// EventCalendarRule defines a Taiwan market calendar event rule
// configurable via ParametersConfig.Industry.EventCalendarRules.
// EventType is the canonical key used to match this rule to the corresponding
// entry in defaultEventRules(). If EventType is empty, Name is used as fallback.
type EventCalendarRule struct {
	EventType  string  `json:"event_type"`
	Name       string  `json:"name"`
	BaseWeight float64 `json:"base_weight"`
	DecayDays  int     `json:"decay_days"`
	Direction  string  `json:"direction"`
}

// StrategyParameters holds tunable values for strategy selection and switching.
type StrategyParameters struct {
	MinSwitchIntervalDays ParameterMetadata[int]     `json:"min_switch_interval_days"`
	SwitchThreshold       ParameterMetadata[float64] `json:"switch_threshold"`
	ScoreLookbackDays     ParameterMetadata[int]     `json:"score_lookback_days"`
	FallbackStrategy      ParameterMetadata[string]  `json:"fallback_strategy"`
}

// FactorWeightParameters holds regime-aware factor weights used by the factor engine
// to adjust allocation across market regimes (bull/bear/high-vol/risk-on/risk-off/etc.).
type FactorWeightParameters struct {
	BaseWeights            ParameterMetadata[map[string]float64] `json:"base_weights,omitempty"`
	RegimeBullMomentum     ParameterMetadata[float64]            `json:"regime_bull_momentum,omitempty"`
	RegimeBullQuality      ParameterMetadata[float64]            `json:"regime_bull_quality,omitempty"`
	RegimeBullValue        ParameterMetadata[float64]            `json:"regime_bull_value,omitempty"`
	RegimeBearQuality      ParameterMetadata[float64]            `json:"regime_bear_quality,omitempty"`
	RegimeBearValue        ParameterMetadata[float64]            `json:"regime_bear_value,omitempty"`
	RegimeBearMomentum     ParameterMetadata[float64]            `json:"regime_bear_momentum,omitempty"`
	RegimeHighVolLiquidity ParameterMetadata[float64]            `json:"regime_high_vol_liquidity,omitempty"`
	RegimeHighVolMomentum  ParameterMetadata[float64]            `json:"regime_high_vol_momentum,omitempty"`
	RegimeHighVolInstSent  ParameterMetadata[float64]            `json:"regime_high_vol_inst_sent,omitempty"`
	SeverityCritical       ParameterMetadata[float64]            `json:"severity_critical,omitempty"`
	SeverityHigh           ParameterMetadata[float64]            `json:"severity_high,omitempty"`
	SeverityMedium         ParameterMetadata[float64]            `json:"severity_medium,omitempty"`
	SeverityLow            ParameterMetadata[float64]            `json:"severity_low,omitempty"`
	ClampMin               ParameterMetadata[float64]            `json:"clamp_min,omitempty"`
	ClampMax               ParameterMetadata[float64]            `json:"clamp_max,omitempty"`
	RiskOnMomentum         ParameterMetadata[float64]            `json:"risk_on_momentum,omitempty"`
	RiskOnQuality          ParameterMetadata[float64]            `json:"risk_on_quality,omitempty"`
	RiskOffMomentum        ParameterMetadata[float64]            `json:"risk_off_momentum,omitempty"`
	RiskOffQuality         ParameterMetadata[float64]            `json:"risk_off_quality,omitempty"`
	RiskOffLiquidity       ParameterMetadata[float64]            `json:"risk_off_liquidity,omitempty"`
	ConservativeValue      ParameterMetadata[float64]            `json:"conservative_value,omitempty"`
	ConservativeQuality    ParameterMetadata[float64]            `json:"conservative_quality,omitempty"`
	ConservativeMomentum   ParameterMetadata[float64]            `json:"conservative_momentum,omitempty"`
	AggressiveMomentum     ParameterMetadata[float64]            `json:"aggressive_momentum,omitempty"`
	AggressiveInstSent     ParameterMetadata[float64]            `json:"aggressive_inst_sent,omitempty"`
	AggressiveValue        ParameterMetadata[float64]            `json:"aggressive_value,omitempty"`
	AggressiveQuality      ParameterMetadata[float64]            `json:"aggressive_quality,omitempty"`
}

// NarrativeConvictionParameters maps skill types to narrative themes and their
// historical hit rates for conviction-driven weight adjustments.
type NarrativeConvictionParameters struct {
	ThemeHitRates ParameterMetadata[map[string]float64] `json:"theme_hit_rates,omitempty"`
	SkillToTheme  ParameterMetadata[map[string]string]  `json:"skill_to_theme,omitempty"`
}

// SectorExecutorParameters holds tunable conviction and price values
// for sector-level executors (LEO Satellite, Semiconductor, etc.).
// Each sub-struct groups parameters for a specific executor, so adding
// a new executor only requires adding its block of ParameterMetadata fields.
type SectorExecutorParameters struct {
	LEOSatellite      LEOSatelliteExecutorParameters      `json:"leo_satellite,omitempty"`
	Financials        FinancialsExecutorParameters        `json:"financials,omitempty"`
	Shipping          ShippingExecutorParameters          `json:"shipping,omitempty"`
	ValueYield        ValueYieldExecutorParameters        `json:"value_yield,omitempty"`
	EarningsQuality   EarningsQualityExecutorParameters   `json:"earnings_quality,omitempty"`
	TechnicalBreakout TechnicalBreakoutExecutorParameters `json:"technical_breakout,omitempty"`
	GrowthMomentum    GrowthMomentumExecutorParameters    `json:"growth_momentum,omitempty"`
	FactorConviction  FactorConvictionParams              `json:"factor_conviction,omitempty"`
}

// FactorConvictionParams holds all factor-score thresholds and conviction deltas
// used by layer-sector and layer-style executors for factor-driven conviction
// adjustments (Waves 2-3 of the factor-driven conviction migration).
// All executors share this single struct to avoid duplicating 15+ parameters
// across each executor's parameter block.
// Zero-default behavior: when no config file is loaded, the ParameterMetadata
// zero-value (Value=0) causes executors to look up the globally defined
// hard-coded constants instead (see internal/orchestrator/plugin_sector.go).
type FactorConvictionParams struct {
	// --- Momentum factor ---
	MomentumHighThreshold ParameterMetadata[float64] `json:"momentum_high_threshold"`
	MomentumHighDelta     ParameterMetadata[int]     `json:"momentum_high_delta"`
	MomentumModThreshold  ParameterMetadata[float64] `json:"momentum_mod_threshold"`
	MomentumModDelta      ParameterMetadata[int]     `json:"momentum_mod_delta"`
	MomentumWeakThreshold ParameterMetadata[float64] `json:"momentum_weak_threshold"`
	MomentumWeakDelta     ParameterMetadata[int]     `json:"momentum_weak_delta"`

	// --- Value factor ---
	ValueHighThreshold ParameterMetadata[float64] `json:"value_high_threshold"`
	ValueHighDelta     ParameterMetadata[int]     `json:"value_high_delta"`
	ValueModThreshold  ParameterMetadata[float64] `json:"value_mod_threshold"`
	ValueModDelta      ParameterMetadata[int]     `json:"value_mod_delta"`
	ValueWeakThreshold ParameterMetadata[float64] `json:"value_weak_threshold"`
	ValueWeakDelta     ParameterMetadata[int]     `json:"value_weak_delta"`

	// --- Quality factor ---
	QualityThreshold ParameterMetadata[float64] `json:"quality_threshold"`
	QualityDelta     ParameterMetadata[int]     `json:"quality_delta"`

	// --- Liquidity factor ---
	LiquidityHighThreshold ParameterMetadata[float64] `json:"liquidity_high_threshold"`
	LiquidityHighDelta     ParameterMetadata[int]     `json:"liquidity_high_delta"`
	LiquidityGoodThreshold ParameterMetadata[float64] `json:"liquidity_good_threshold"`
	LiquidityGoodDelta     ParameterMetadata[int]     `json:"liquidity_good_delta"`
	LiquidityLowThreshold  ParameterMetadata[float64] `json:"liquidity_low_threshold"`
	LiquidityLowDelta      ParameterMetadata[int]     `json:"liquidity_low_delta"`
}

// LEOSatelliteExecutorParameters holds all tunable values for the
// LEOSatelliteExecutor (internal/orchestrator/plugin_sector.go).
type LEOSatelliteExecutorParameters struct {
	ConvictionBase        ParameterMetadata[int]     `json:"conviction_base"`
	PricePenaltyDelta     ParameterMetadata[int]     `json:"price_penalty_delta"`
	LaunchBoostDelta      ParameterMetadata[int]     `json:"launch_boost_delta"`
	DeploymentBoostDelta  ParameterMetadata[int]     `json:"deployment_boost_delta"`
	DowngradePenaltyDelta ParameterMetadata[int]     `json:"downgrade_penalty_delta"`
	TargetPriceMult       ParameterMetadata[float64] `json:"target_price_multiplier"`
	StopLossMult          ParameterMetadata[float64] `json:"stop_loss_multiplier"`
}
type FinancialsExecutorParameters struct {
	DividendBoost            ParameterMetadata[int]     `json:"dividend_boost"`
	BalanceSheetPenalty      ParameterMetadata[int]     `json:"balance_sheet_penalty"`
	CreditQualityBoost       ParameterMetadata[int]     `json:"credit_quality_boost"`
	CreditQualityPenalty     ParameterMetadata[int]     `json:"credit_quality_penalty"`
	SpreadSensitivityBoost   ParameterMetadata[int]     `json:"spread_sensitivity_boost"`
	SpreadSensitivityPenalty ParameterMetadata[int]     `json:"spread_sensitivity_penalty"`
	CapitalAdequacyBoost     ParameterMetadata[int]     `json:"capital_adequacy_boost"`
	PriceToOpenThreshold     ParameterMetadata[float64] `json:"price_to_open_threshold"`
	PriceToHighThreshold     ParameterMetadata[float64] `json:"price_to_high_threshold"`
}
type ShippingExecutorParameters struct {
	TacticalBoost      ParameterMetadata[int]     `json:"tactical_boost"`
	WeakClosePenalty   ParameterMetadata[int]     `json:"weak_close_penalty"`
	WeakCloseThreshold ParameterMetadata[float64] `json:"weak_close_threshold"`
}
type ValueYieldExecutorParameters struct {
	CashFlowBoost    ParameterMetadata[int] `json:"cash_flow_boost"`
	YieldTrapPenalty ParameterMetadata[int] `json:"yield_trap_penalty"`
}
type EarningsQualityExecutorParameters struct {
	RepeatableBoost   ParameterMetadata[int]     `json:"repeatable_boost"`
	GuidancePenalty   ParameterMetadata[int]     `json:"guidance_penalty"`
	GuidanceThreshold ParameterMetadata[float64] `json:"guidance_threshold"`
}
type TechnicalBreakoutExecutorParameters struct {
	DefaultVolumeFloor     ParameterMetadata[int64]   `json:"default_volume_floor"`
	StrictVolumeFloor      ParameterMetadata[int64]   `json:"strict_volume_floor"`
	RelaxedVolumeFloor     ParameterMetadata[int64]   `json:"relaxed_volume_floor"`
	LowVolumeFloor         ParameterMetadata[int64]   `json:"low_volume_floor"`
	LowVolumeBoost         ParameterMetadata[int]     `json:"low_volume_boost"`
	RejectLowVolumeFloor   ParameterMetadata[int64]   `json:"reject_low_volume_floor"`
	VolumeBoost            ParameterMetadata[int]     `json:"volume_boost"`
	CloseStrengthPenalty   ParameterMetadata[int]     `json:"close_strength_penalty"`
	CloseStrengthThreshold ParameterMetadata[float64] `json:"close_strength_threshold"`
	CloseStrengthTolerance ParameterMetadata[float64] `json:"close_strength_tolerance"`
	SurgeBoost             ParameterMetadata[int]     `json:"surge_boost"`
	SurgePenalty           ParameterMetadata[int]     `json:"surge_penalty"`
	OpenRejectionPenalty   ParameterMetadata[int]     `json:"open_rejection_penalty"`
	LateBreakoutPenalty    ParameterMetadata[int]     `json:"late_breakout_penalty"`
	LateBreakoutThreshold  ParameterMetadata[float64] `json:"late_breakout_threshold"`
	ConfirmationBoost      ParameterMetadata[int]     `json:"confirmation_boost"`
	ConfirmationThreshold  ParameterMetadata[float64] `json:"confirmation_threshold"`
	CatchUpBoost           ParameterMetadata[int]     `json:"catch_up_boost"`
	CatchUpLowerThreshold  ParameterMetadata[float64] `json:"catch_up_lower_threshold"`
	CatchUpUpperThreshold  ParameterMetadata[float64] `json:"catch_up_upper_threshold"`
}
type GrowthMomentumExecutorParameters struct {
	ConvictionBase           ParameterMetadata[int]     `json:"conviction_base"`
	PricePenalty             ParameterMetadata[int]     `json:"price_penalty"`
	TrendConfirmationPenalty ParameterMetadata[int]     `json:"trend_confirmation_penalty"`
	DowngradePricePenalty    ParameterMetadata[int]     `json:"downgrade_price_penalty"`
	DowngradeOpenPenalty     ParameterMetadata[int]     `json:"downgrade_open_penalty"`
	ExploratoryPricePenalty  ParameterMetadata[int]     `json:"exploratory_price_penalty"`
	ExploratoryOpenPenalty   ParameterMetadata[int]     `json:"exploratory_open_penalty"`
	DowngradeThreshold       ParameterMetadata[float64] `json:"downgrade_threshold"`
}

// PreciousMetalsParameters holds tunable values for precious metals factor scoring.
type PreciousMetalsParameters struct {
	CentralBankBuyingTrend ParameterMetadata[string]  `json:"central_bank_buying_trend"`
	CentralBankNetBuy      ParameterMetadata[float64] `json:"central_bank_net_buy"`
	IndiaGoldImportsYoY    ParameterMetadata[float64] `json:"india_gold_imports_yoy"`
	ChinaGoldImportsYoY    ParameterMetadata[float64] `json:"china_gold_imports_yoy"`
	COMEXDefaultNetLong    ParameterMetadata[float64] `json:"comex_default_net_long"`
}

type AlertParameters struct {
	MinCashThreshold         ParameterMetadata[float64]  `json:"min_cash_threshold"`
	MaxPositionsCount        ParameterMetadata[int]      `json:"max_positions_count"`
	MaxPositionWeightPct     ParameterMetadata[float64]  `json:"max_position_weight_pct"`
	MaxUnrealizedLossPct     ParameterMetadata[float64]  `json:"max_unrealized_loss_pct"`
	DailyLossWarningPct      ParameterMetadata[float64]  `json:"daily_loss_warning_pct"`
	DailyLossCriticalPct     ParameterMetadata[float64]  `json:"daily_loss_critical_pct"`
	RuleEngineIntervalSec    ParameterMetadata[int]      `json:"rule_engine_interval_sec"`
	RuleEngineCooldownSec    ParameterMetadata[int]      `json:"rule_engine_cooldown_sec"`
	SystemMetricsIntervalSec ParameterMetadata[int]      `json:"system_metrics_interval_sec"`
	MinScreeningRate         ParameterMetadata[float64]  `json:"min_screening_rate"`
	MaxAlertTriggerRate      ParameterMetadata[float64]  `json:"max_alert_trigger_rate"`
	MaxUnacknowledgedAlerts  ParameterMetadata[int]      `json:"max_unacknowledged_alerts"`
	SuppressCategories       ParameterMetadata[[]string] `json:"suppress_categories"`
}

// RiskGateParameters holds all tunable parameters for the unified risk gate system.
type RiskGateParameters struct {
	PreTrade  PreTradeGateParameters  `json:"pre_trade"`
	InTrade   InTradeGateParameters   `json:"in_trade,omitempty"`
	PostTrade PostTradeGateParameters `json:"post_trade,omitempty"`
}

// PreTradeGateParameters holds pre-trade risk check parameters.
type PreTradeGateParameters struct {
	MaxPositionPct       ParameterMetadata[float64] `json:"max_position_pct"`
	MaxSectorExposurePct ParameterMetadata[float64] `json:"max_sector_exposure_pct"`
	VaRConfidenceLevel   ParameterMetadata[float64] `json:"var_confidence_level"`
	VarLimitPct          ParameterMetadata[float64] `json:"var_limit_pct"`
	MinCashBufferPct     ParameterMetadata[float64] `json:"min_cash_buffer_pct"`
	MaxCorrelation       ParameterMetadata[float64] `json:"max_correlation"`
	MinADVRatio          ParameterMetadata[float64] `json:"min_adv_ratio"`
	MaxOpenPositions     ParameterMetadata[int]     `json:"max_open_positions"`
}

// InTradeGateParameters holds in-trade monitoring parameters.
type InTradeGateParameters struct {
	MonitorIntervalSec         ParameterMetadata[int]     `json:"monitor_interval_sec"`
	StopLossPct                ParameterMetadata[float64] `json:"stop_loss_pct"`
	TakeProfitPct              ParameterMetadata[float64] `json:"take_profit_pct"`
	TrailingStopATRMult        ParameterMetadata[float64] `json:"trailing_stop_atr_mult"`
	VolatilitySpikeMult        ParameterMetadata[float64] `json:"volatility_spike_mult"`
	CircuitBreakerDailyLossPct ParameterMetadata[float64] `json:"circuit_breaker_daily_loss_pct"`
}

// EngineConfig type definitions — migrated from engine_config.go, retained as
// output types for EngineParameters.ToConfig() conversion methods.

type MacroRiskConfig struct {
	CarryTradeUnwindThreshold float64 `json:"carry_trade_unwind_threshold"`
	VIXThreshold              float64 `json:"vix_threshold"`
	US10YThreshold            float64 `json:"us10y_threshold"`
	OilShockThresholdPct      float64 `json:"oil_shock_threshold_pct"`
	GoldSurgeThresholdPct     float64 `json:"gold_surge_threshold_pct"`
	DXYSurgeThresholdPct      float64 `json:"dxy_surge_threshold_pct"`
	TWDStressThresholdPct     float64 `json:"twd_stress_threshold_pct"`
	OutflowProbBase           float64 `json:"outflow_prob_base"`
	OutflowProbMax            float64 `json:"outflow_prob_max"`
}

type StructuralTrendConfig struct {
	MinTrendStrength            float64 `json:"min_trend_strength"`
	MinConfidence               float64 `json:"min_confidence"`
	MinHitRate                  float64 `json:"min_hit_rate"`
	OverrideThreshold           float64 `json:"override_threshold"`
	AIRevenueGrowthThreshold    float64 `json:"ai_revenue_growth_threshold"`
	CoWoSUtilizationThreshold   float64 `json:"cowos_utilization_threshold"`
	CapexGrowthThreshold        float64 `json:"capex_growth_threshold"`
	SemiconductorIndexThreshold float64 `json:"semiconductor_index_threshold"`
}

type DrawdownLevel struct {
	Percentage  float64 `json:"percentage"`
	MaxExposure float64 `json:"max_exposure"`
}

type DrawdownConfig struct {
	Levels                            map[string]DrawdownLevel `json:"levels"`
	OrangeOverrideMinScore            float64                  `json:"orange_override_min_score"`
	RedOverrideMinScore               float64                  `json:"red_override_min_score"`
	SectorConstraintsRiskOff          map[string]float64       `json:"sector_constraints_risk_off"`
	SectorConstraintsCarryTradeUnwind map[string]float64       `json:"sector_constraints_carry_trade_unwind"`
	SectorConstraintsSectorRotation   map[string]float64       `json:"sector_constraints_sector_rotation"`
}

type SectorRotationConfig struct {
	BaseAllocations    map[string]float64 `json:"base_allocations"`
	MinAllocation      float64            `json:"min_allocation"`
	MaxAllocation      float64            `json:"max_allocation"`
	RebalanceThreshold float64            `json:"rebalance_threshold"`
}

type StrategyStateConfig struct {
	MaxPositionSize    float64 `json:"max_position_size"`
	MaxSectorExposure  float64 `json:"max_sector_exposure"`
	MinCashReserve     float64 `json:"min_cash_reserve"`
	HedgeRatio         float64 `json:"hedge_ratio"`
	AllowNewPositions  bool    `json:"allow_new_positions"`
	AllowConcentration bool    `json:"allow_concentration"`
}

type StrategyEvolutionConfig struct {
	CooldownPeriodHours int                            `json:"cooldown_period_hours"`
	Configs             map[string]StrategyStateConfig `json:"configs"`
}

func (c StrategyEvolutionConfig) GetCooldownDuration() time.Duration {
	return time.Duration(c.CooldownPeriodHours) * time.Hour
}

type ExecutorsConfig struct {
	VIXMomentumCrashThreshold float64 `json:"vix_momentum_crash_threshold"`
	CrowdingPenaltyAgents3    float64 `json:"crowding_penalty_agents_3"`
	CrowdingPenaltyAgents4    float64 `json:"crowding_penalty_agents_4"`
	MinTradeAmount            float64 `json:"min_trade_amount"`
	MaxStocksDefault          int     `json:"max_stocks_default"`
	MaxStocksMin              int     `json:"max_stocks_min"`
	MaxStocksMax              int     `json:"max_stocks_max"`
	ConvictionFloorDefault    int     `json:"conviction_floor_default"`
}

type SimulationConfig struct {
	NeutralRegimeSizingFactor float64 `json:"neutral_regime_sizing_factor"`
}

// PostTradeGateParameters holds post-trade evaluation parameters.
type PostTradeGateParameters struct {
	MaxDrawdownHaltPct      ParameterMetadata[float64] `json:"max_drawdown_halt_pct"`
	MaxDrawdownDefensivePct ParameterMetadata[float64] `json:"max_drawdown_defensive_pct"`
	MinRollingSharpe        ParameterMetadata[float64] `json:"min_rolling_sharpe"`
	ConsecutiveLossDays     ParameterMetadata[int]     `json:"consecutive_loss_days"`
	EvaluationIntervalHours ParameterMetadata[int]     `json:"evaluation_interval_hours"`
}

// RSITwParameters holds tunable values for the RSI-tw retail sentiment calculator.
type RSITwParameters struct {
	// Part A — Retail Sentiment (40% overall weight)
	A1Weight    ParameterMetadata[float64] `json:"a1_weight"`     // Margin Balance Δ Z-score (default 0.25)
	A2Weight    ParameterMetadata[float64] `json:"a2_weight"`     // Day Trading Ratio (default 0.20)
	A3Weight    ParameterMetadata[float64] `json:"a3_weight"`     // Margin Maintenance Proxy (default 0.20)
	A4Weight    ParameterMetadata[float64] `json:"a4_weight"`     // VIX Nonlinear Mapping (default 0.15)
	A5Weight    ParameterMetadata[float64] `json:"a5_weight"`     // Weekly PCR Proxy (default 0.10)
	A6Weight    ParameterMetadata[float64] `json:"a6_weight"`     // Odd-Lot Trading (default 0.10)
	APartWeight ParameterMetadata[float64] `json:"a_part_weight"` // Part A overall weight (default 0.40)
	CPartWeight ParameterMetadata[float64] `json:"c_part_weight"` // Part C overall weight (default 0.25)

	// A3: Margin Maintenance formula (z = (p - midpoint) * scale)
	A3Midpoint ParameterMetadata[float64] `json:"a3_midpoint"` // neutral midpoint (default 0.5)
	A3Scale    ParameterMetadata[float64] `json:"a3_scale"`    // Z-score scaling factor (default 2.0)

	// A4: VIX piecewise mapping — thresholds are lower bounds (exclusive), scores are the mapping result.
	// thresholds[0]=15, thresholds[1]=20, ...; scores[0]=0.1 (vix<15), scores[5]=1.0 (vix>=35)
	A4VixThresholds ParameterMetadata[[]float64] `json:"a4_vix_thresholds"` // [15, 20, 25, 30, 35]
	A4VixScores     ParameterMetadata[[]float64] `json:"a4_vix_scores"`     // [0.1, 0.3, 0.5, 0.7, 0.85, 1.0]

	// A5: PCR piecewise mapping — thresholds are compared with > (strict), scores in order
	A5PcrThresholds ParameterMetadata[[]float64] `json:"a5_pcr_thresholds"` // [1.5, 1.0, 0.8]
	A5PcrScores     ParameterMetadata[[]float64] `json:"a5_pcr_scores"`     // [0.9, 0.7, 0.5, 0.1]
	A5PcrFallback   ParameterMetadata[float64]   `json:"a5_pcr_fallback"`   // score when pcr==0 (default 0.5)

	// A6: Odd-lot imbalance mapping — thresholds with > (strict), scores in order
	A6OddLotThresholds ParameterMetadata[[]float64] `json:"a6_oddlot_thresholds"` // [0.2, 0.1, -0.1, -0.2]
	A6OddLotScores     ParameterMetadata[[]float64] `json:"a6_oddlot_scores"`     // [0.85, 0.65, 0.5, 0.35, 0.15]
	A6OddLotFallback   ParameterMetadata[float64]   `json:"a6_oddlot_fallback"`   // score when imb==0 (default 0.5)

	// Part C — Institutional / Derivative Flow (25% weight)
	C1Weight               ParameterMetadata[float64] `json:"c1_weight"`                 // Small TAIEX Futures OI (default 0.40)
	C2Weight               ParameterMetadata[float64] `json:"c2_weight"`                 // Foreign/Inst Net Flow (default 0.35)
	C3Weight               ParameterMetadata[float64] `json:"c3_weight"`                 // ETF Net Subscription (default 0.25)
	C1VeryBullishThreshold ParameterMetadata[float64] `json:"c1_very_bullish_threshold"` // futures OI pct above this → 0.9
	C1BullishThreshold     ParameterMetadata[float64] `json:"c1_bullish_threshold"`      // futures OI pct above this → 0.7
	C1BearishThreshold     ParameterMetadata[float64] `json:"c1_bearish_threshold"`      // futures OI pct below this → 0.5
	C1VeryBearishThreshold ParameterMetadata[float64] `json:"c1_very_bearish_threshold"` // futures OI pct below this → 0.25
	C2NeutralMidpoint      ParameterMetadata[float64] `json:"c2_neutral_midpoint"`       // base score when netFlow ≈ 0 (0.5)
	C2NetflowScalingFactor ParameterMetadata[float64] `json:"c2_netflow_scaling_factor"` // divisor for continuous scoring
	C3VeryBullishThreshold ParameterMetadata[float64] `json:"c3_very_bullish_threshold"` // ETF net sub above this → 0.9
	C3BullishThreshold     ParameterMetadata[float64] `json:"c3_bullish_threshold"`      // ETF net sub above this → 0.7
	C3BearishThreshold     ParameterMetadata[float64] `json:"c3_bearish_threshold"`      // ETF net sub below this → 0.45

	// Part D — Event-Driven Adjustment Factors
	DGeoPoliticalRiskThreshold  ParameterMetadata[float64] `json:"d_geopolitical_risk_threshold"`  // geopolitical risk above this → 0.85
	DGeoPoliticalRiskMultiplier ParameterMetadata[float64] `json:"d_geopolitical_risk_multiplier"` // 0.85
	DVIXSpikeThreshold          ParameterMetadata[float64] `json:"d_vix_spike_threshold"`          // VIX above this → 0.90
	DVIXSpikeMultiplier         ParameterMetadata[float64] `json:"d_vix_spike_multiplier"`         // 0.90
	DCreditTighteningMultiplier ParameterMetadata[float64] `json:"d_credit_tightening_multiplier"` // 0.80

	// LastCalibratedScore records the most recent autonomous calibration score,
	// loaded at startup so PreTradeGate does not start with a blind 0.0.
	LastCalibratedScore ParameterMetadata[float64] `json:"last_calibrated_score,omitempty"`
}

// FallbackPriceTarget holds per-skill target and stop-loss multipliers
// used by the monitoring service when price targets are not explicitly set.
type FallbackPriceTarget struct {
	TargetMultiplier   ParameterMetadata[float64] `json:"target_multiplier"`
	StopLossMultiplier ParameterMetadata[float64] `json:"stop_loss_multiplier"`
}

// WeightFactorConfig holds a single factor contribution to sector weight derivation.
type WeightFactorConfig struct {
	Factor   string  `json:"factor"`
	Weight   float64 `json:"weight"`
	Source   string  `json:"source"`
	Evidence string  `json:"evidence"`
}

// SectorAllocationConfig holds all tunable parameters for the unified sector allocation engine.
// Includes base weights, derivation factors, and formula multipliers for multi-factor
// sector weight computation (cycle × seasonal × linkage × narrative × macro × factor).
type SectorAllocationConfig struct {
	Rationale         string                          `json:"rationale"`
	Source            ParameterSource                 `json:"source"`
	Citation          *ParameterCitation              `json:"citation,omitempty"`
	BaseWeights       map[string]float64              `json:"base_weights"`
	DerivationFactors map[string][]WeightFactorConfig `json:"derivation_factors"`
	CycleWeight       float64                         `json:"cycle_weight"`
	SeasonalWeight    float64                         `json:"seasonal_weight"`
	LinkageWeight     float64                         `json:"linkage_weight"`
	NarrativeWeight   float64                         `json:"narrative_weight"`
	MacroWeight       float64                         `json:"macro_weight"`
	FactorWeight      float64                         `json:"factor_weight"`
	WeightFloor       float64                         `json:"weight_floor"`
}

type ParametersConfig struct {
	Version              string                         `json:"version"`
	UpdatedAt            time.Time                      `json:"updated_at"`
	FallbackPriceTargets map[string]FallbackPriceTarget `json:"fallback_price_targets,omitempty"`
	Darwinian            DarwinianParameters            `json:"darwinian"`
	Factor               FactorParameters               `json:"factor"`
	FactorWeight         FactorWeightParameters         `json:"factor_weight,omitempty"`
	Optimizer            OptimizerParameters            `json:"optimizer"`
	Sizing               SizingParameters               `json:"sizing"`
	Health               HealthParameters               `json:"health"`
	GARCH                GARCHParameters                `json:"garch"`
	Experiment           ExperimentParameters           `json:"experiment"`
	Baseline             BaselineParameters             `json:"baseline"`
	Orchestrator         OrchestratorParameters         `json:"orchestrator"`
	Risk                 RiskParameters                 `json:"risk"`
	Drawdown             DrawdownParameters             `json:"drawdown"`
	Realtime             RealtimeParameters             `json:"realtime"`
	Janus                JanusParameters                `json:"janus"`
	Narrative            NarrativeParameters            `json:"narrative"`
	NarrativeConviction  NarrativeConvictionParameters  `json:"narrative_conviction,omitempty"`
	Marketdata           MarketdataParameters           `json:"marketdata"`
	Industry             IndustryParameters             `json:"industry"`
	Strategy             StrategyParameters             `json:"strategy"`
	PreciousMetals       PreciousMetalsParameters       `json:"precious_metals"`
	SectorExecutor       SectorExecutorParameters       `json:"sector_executor,omitempty"`
	Alert                AlertParameters                `json:"alert"`
	RiskGate             RiskGateParameters             `json:"risk_gate,omitempty"`
	Engine               EngineParameters               `json:"engine,omitempty"`
	RSITw                RSITwParameters                `json:"rsi_tw,omitempty"`
	Tax                  TaxParameters                  `json:"tax,omitempty"`
	SectorAllocation     SectorAllocationConfig         `json:"sector_allocation"`
	Reporting            ReportingParameters            `json:"reporting"`
	ForwardReturn        ForwardReturnParameters        `json:"forward_return,omitempty"`
}

// TaxParameters holds tunable Taiwan tax rates with full provenance tracking.
type TaxParameters struct {
	DividendTaxRate    ParameterMetadata[float64] `json:"dividend_tax_rate"`
	TransactionTaxRate ParameterMetadata[float64] `json:"transaction_tax_rate"`
	NHISurchargeRate   ParameterMetadata[float64] `json:"nhi_surcharge_rate"`
}

type ForwardReturnParameters struct {
	RiskOnMean    ParameterMetadata[float64] `json:"risk_on_mean"`
	RiskOffMean   ParameterMetadata[float64] `json:"risk_off_mean"`
	RiskOnStdDev  ParameterMetadata[float64] `json:"risk_on_std_dev"`
	RiskOffStdDev ParameterMetadata[float64] `json:"risk_off_std_dev"`
}

// ToConfig converts TaxParameters to a domain.TaxConfig for use by tax calculators.
// Zero values in parameters.json carry the semantic meaning "use statutory default rate"
// (the rationale field in the JSON documents this convention). This sentinel fallback
// ensures that explicitly zero tax rates resolve to the statutory defaults rather than 0.
func (p TaxParameters) ToConfig() domain.TaxConfig {
	cfg := domain.DefaultTaiwanTaxConfig()
	if p.DividendTaxRate.Value != 0 {
		cfg.DividendTaxRate = p.DividendTaxRate.Value
	}
	if p.TransactionTaxRate.Value != 0 {
		cfg.TransactionTaxRate = p.TransactionTaxRate.Value
	}
	if p.NHISurchargeRate.Value != 0 {
		cfg.NHISurchargeRate = p.NHISurchargeRate.Value
	}
	cfg.IncludeNHI = true // NHI inclusion is a policy flag, not a tunable rate
	return cfg
}

// ReportingParameters holds tunables for the performance-report rendering pipeline.
// WinRateThreshold applies a cost-adjusted cutoff to per-recommendation ForwardReturn:
// values <= threshold are counted as losses (covers transaction cost + slippage).
// SharpeMinSamples is the minimum per-agent sample count for SharpeLike to be
// reported; below it, SharpeLike is null and the frontend renders "N/A".
type ReportingParameters struct {
	WinRateThreshold ParameterMetadata[float64] `json:"win_rate_threshold"`
	SharpeMinSamples ParameterMetadata[int]     `json:"sharpe_min_samples"`
}

// EngineParameters holds parameters migrated from EngineConfig with full ParameterMetadata wrapping.
type EngineParameters struct {
	MacroRisk         EngineMacroRiskParameters         `json:"macro_risk"`
	StructuralTrend   EngineStructuralTrendParameters   `json:"structural_trend"`
	Drawdown          EngineDrawdownParameters          `json:"drawdown"`
	SectorRotation    EngineSectorRotationParameters    `json:"sector_rotation"`
	StrategyEvolution EngineStrategyEvolutionParameters `json:"strategy_evolution"`
	Executors         EngineExecutorsParameters         `json:"executors"`
	Simulation        EngineSimulationParameters        `json:"simulation"`
}

type EngineMacroRiskParameters struct {
	CarryTradeUnwindThreshold ParameterMetadata[float64] `json:"carry_trade_unwind_threshold"`
	VIXThreshold              ParameterMetadata[float64] `json:"vix_threshold"`
	US10YThreshold            ParameterMetadata[float64] `json:"us10y_threshold"`
	OilShockThresholdPct      ParameterMetadata[float64] `json:"oil_shock_threshold_pct"`
	GoldSurgeThresholdPct     ParameterMetadata[float64] `json:"gold_surge_threshold_pct"`
	DXYSurgeThresholdPct      ParameterMetadata[float64] `json:"dxy_surge_threshold_pct"`
	TWDStressThresholdPct     ParameterMetadata[float64] `json:"twd_stress_threshold_pct"`
	OutflowProbBase           ParameterMetadata[float64] `json:"outflow_prob_base"`
	OutflowProbMax            ParameterMetadata[float64] `json:"outflow_prob_max"`
}

func (p EngineMacroRiskParameters) ToConfig() MacroRiskConfig {
	return MacroRiskConfig{
		CarryTradeUnwindThreshold: p.CarryTradeUnwindThreshold.Value,
		VIXThreshold:              p.VIXThreshold.Value,
		US10YThreshold:            p.US10YThreshold.Value,
		OilShockThresholdPct:      p.OilShockThresholdPct.Value,
		GoldSurgeThresholdPct:     p.GoldSurgeThresholdPct.Value,
		DXYSurgeThresholdPct:      p.DXYSurgeThresholdPct.Value,
		TWDStressThresholdPct:     p.TWDStressThresholdPct.Value,
		OutflowProbBase:           p.OutflowProbBase.Value,
		OutflowProbMax:            p.OutflowProbMax.Value,
	}
}

type EngineStructuralTrendParameters struct {
	MinTrendStrength            ParameterMetadata[float64] `json:"min_trend_strength"`
	MinConfidence               ParameterMetadata[float64] `json:"min_confidence"`
	MinHitRate                  ParameterMetadata[float64] `json:"min_hit_rate"`
	OverrideThreshold           ParameterMetadata[float64] `json:"override_threshold"`
	AIRevenueGrowthThreshold    ParameterMetadata[float64] `json:"ai_revenue_growth_threshold"`
	CoWoSUtilizationThreshold   ParameterMetadata[float64] `json:"cowos_utilization_threshold"`
	CapexGrowthThreshold        ParameterMetadata[float64] `json:"capex_growth_threshold"`
	SemiconductorIndexThreshold ParameterMetadata[float64] `json:"semiconductor_index_threshold"`
}

func (p EngineStructuralTrendParameters) ToConfig() StructuralTrendConfig {
	return StructuralTrendConfig{
		MinTrendStrength:            p.MinTrendStrength.Value,
		MinConfidence:               p.MinConfidence.Value,
		MinHitRate:                  p.MinHitRate.Value,
		OverrideThreshold:           p.OverrideThreshold.Value,
		AIRevenueGrowthThreshold:    p.AIRevenueGrowthThreshold.Value,
		CoWoSUtilizationThreshold:   p.CoWoSUtilizationThreshold.Value,
		CapexGrowthThreshold:        p.CapexGrowthThreshold.Value,
		SemiconductorIndexThreshold: p.SemiconductorIndexThreshold.Value,
	}
}

type EngineDrawdownParameters struct {
	Levels                        ParameterMetadata[map[string]DrawdownLevel] `json:"levels"`
	OrangeOverrideMinScore        ParameterMetadata[float64]                  `json:"orange_override_min_score"`
	RedOverrideMinScore           ParameterMetadata[float64]                  `json:"red_override_min_score"`
	SectorConstraintsRiskOff      ParameterMetadata[map[string]float64]       `json:"sector_constraints_risk_off"`
	SectorConstraintsCarryUnwind  ParameterMetadata[map[string]float64]       `json:"sector_constraints_carry_trade_unwind"`
	SectorConstraintsSectorRotate ParameterMetadata[map[string]float64]       `json:"sector_constraints_sector_rotation"`
}

func (p EngineDrawdownParameters) ToConfig() DrawdownConfig {
	return DrawdownConfig{
		Levels:                            p.Levels.Value,
		OrangeOverrideMinScore:            p.OrangeOverrideMinScore.Value,
		RedOverrideMinScore:               p.RedOverrideMinScore.Value,
		SectorConstraintsRiskOff:          p.SectorConstraintsRiskOff.Value,
		SectorConstraintsCarryTradeUnwind: p.SectorConstraintsCarryUnwind.Value,
		SectorConstraintsSectorRotation:   p.SectorConstraintsSectorRotate.Value,
	}
}

type EngineSectorRotationParameters struct {
	BaseAllocations    ParameterMetadata[map[string]float64] `json:"base_allocations"`
	MinAllocation      ParameterMetadata[float64]            `json:"min_allocation"`
	MaxAllocation      ParameterMetadata[float64]            `json:"max_allocation"`
	RebalanceThreshold ParameterMetadata[float64]            `json:"rebalance_threshold"`
}

func (p EngineSectorRotationParameters) ToConfig() SectorRotationConfig {
	return SectorRotationConfig{
		BaseAllocations:    p.BaseAllocations.Value,
		MinAllocation:      p.MinAllocation.Value,
		MaxAllocation:      p.MaxAllocation.Value,
		RebalanceThreshold: p.RebalanceThreshold.Value,
	}
}

type EngineStrategyEvolutionParameters struct {
	CooldownPeriodHours ParameterMetadata[int]                            `json:"cooldown_period_hours"`
	Configs             ParameterMetadata[map[string]StrategyStateConfig] `json:"configs"`
}

func (p EngineStrategyEvolutionParameters) ToConfig() StrategyEvolutionConfig {
	return StrategyEvolutionConfig{
		CooldownPeriodHours: p.CooldownPeriodHours.Value,
		Configs:             p.Configs.Value,
	}
}

type EngineExecutorsParameters struct {
	VIXMomentumCrashThreshold ParameterMetadata[float64] `json:"vix_momentum_crash_threshold"`
	CrowdingPenaltyAgents3    ParameterMetadata[float64] `json:"crowding_penalty_agents_3"`
	CrowdingPenaltyAgents4    ParameterMetadata[float64] `json:"crowding_penalty_agents_4"`
	MinTradeAmount            ParameterMetadata[float64] `json:"min_trade_amount"`
	MaxStocksDefault          ParameterMetadata[int]     `json:"max_stocks_default"`
	MaxStocksMin              ParameterMetadata[int]     `json:"max_stocks_min"`
	MaxStocksMax              ParameterMetadata[int]     `json:"max_stocks_max"`
	ConvictionFloorDefault    ParameterMetadata[int]     `json:"conviction_floor_default"`
}

func (p EngineExecutorsParameters) ToConfig() ExecutorsConfig {
	return ExecutorsConfig{
		VIXMomentumCrashThreshold: p.VIXMomentumCrashThreshold.Value,
		CrowdingPenaltyAgents3:    p.CrowdingPenaltyAgents3.Value,
		CrowdingPenaltyAgents4:    p.CrowdingPenaltyAgents4.Value,
		MinTradeAmount:            p.MinTradeAmount.Value,
		MaxStocksDefault:          p.MaxStocksDefault.Value,
		MaxStocksMin:              p.MaxStocksMin.Value,
		MaxStocksMax:              p.MaxStocksMax.Value,
		ConvictionFloorDefault:    p.ConvictionFloorDefault.Value,
	}
}

type EngineSimulationParameters struct {
	NeutralRegimeSizingFactor ParameterMetadata[float64] `json:"neutral_regime_sizing_factor"`
}

func (p EngineSimulationParameters) ToConfig() SimulationConfig {
	return SimulationConfig{
		NeutralRegimeSizingFactor: p.NeutralRegimeSizingFactor.Value,
	}
}

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

func (p *ParametersConfig) validateEngine() error {
	e := p.Engine
	if e.MacroRisk.VIXThreshold.Value <= 0 {
		return fmt.Errorf("engine.macro_risk.vix_threshold (%.1f) must be positive", e.MacroRisk.VIXThreshold.Value)
	}
	if e.MacroRisk.CarryTradeUnwindThreshold.Value <= 0 {
		return fmt.Errorf("engine.macro_risk.carry_trade_unwind_threshold (%.1f) must be positive", e.MacroRisk.CarryTradeUnwindThreshold.Value)
	}
	if e.MacroRisk.US10YThreshold.Value < 0 {
		return fmt.Errorf("engine.macro_risk.us10y_threshold (%.1f) must be non-negative", e.MacroRisk.US10YThreshold.Value)
	}
	if e.MacroRisk.OutflowProbBase.Value < 0 || e.MacroRisk.OutflowProbBase.Value > 100 {
		return fmt.Errorf("engine.macro_risk.outflow_prob_base (%.1f) must be in [0,100]", e.MacroRisk.OutflowProbBase.Value)
	}
	if e.MacroRisk.OutflowProbMax.Value < 0 || e.MacroRisk.OutflowProbMax.Value > 100 {
		return fmt.Errorf("engine.macro_risk.outflow_prob_max (%.1f) must be in [0,100]", e.MacroRisk.OutflowProbMax.Value)
	}

	for name, level := range e.Drawdown.Levels.Value {
		if level.Percentage < 0 || level.Percentage > 1 {
			return fmt.Errorf("engine.drawdown.levels.%s.percentage (%.3f) must be in [0,1]", name, level.Percentage)
		}
		if level.MaxExposure < 0 || level.MaxExposure > 1 {
			return fmt.Errorf("engine.drawdown.levels.%s.max_exposure (%.3f) must be in [0,1]", name, level.MaxExposure)
		}
	}
	if e.Drawdown.OrangeOverrideMinScore.Value >= e.Drawdown.RedOverrideMinScore.Value {
		return fmt.Errorf("engine.drawdown.orange_override_min_score (%.3f) must be < red_override_min_score (%.3f)", e.Drawdown.OrangeOverrideMinScore.Value, e.Drawdown.RedOverrideMinScore.Value)
	}

	total := 0.0
	for _, alloc := range e.SectorRotation.BaseAllocations.Value {
		total += alloc
	}
	if total < 0.99 || total > 1.01 {
		return fmt.Errorf("engine.sector_rotation.base_allocations sum (%.4f) must be 1.0±0.01", total)
	}
	if e.SectorRotation.MinAllocation.Value >= e.SectorRotation.MaxAllocation.Value {
		return fmt.Errorf("engine.sector_rotation.min_allocation (%.3f) must be < max_allocation (%.3f)", e.SectorRotation.MinAllocation.Value, e.SectorRotation.MaxAllocation.Value)
	}

	if e.Executors.MaxStocksMin.Value > e.Executors.MaxStocksDefault.Value {
		return fmt.Errorf("engine.executors.max_stocks_min (%d) must be <= max_stocks_default (%d)", e.Executors.MaxStocksMin.Value, e.Executors.MaxStocksDefault.Value)
	}
	if e.Executors.MaxStocksDefault.Value > e.Executors.MaxStocksMax.Value {
		return fmt.Errorf("engine.executors.max_stocks_default (%d) must be <= max_stocks_max (%d)", e.Executors.MaxStocksDefault.Value, e.Executors.MaxStocksMax.Value)
	}
	if e.Executors.ConvictionFloorDefault.Value < 0 || e.Executors.ConvictionFloorDefault.Value > 100 {
		return fmt.Errorf("engine.executors.conviction_floor_default (%d) must be in [0,100]", e.Executors.ConvictionFloorDefault.Value)
	}
	if e.Executors.MinTradeAmount.Value <= 0 {
		return fmt.Errorf("engine.executors.min_trade_amount (%.0f) must be positive", e.Executors.MinTradeAmount.Value)
	}
	if e.Executors.CrowdingPenaltyAgents3.Value <= 0 || e.Executors.CrowdingPenaltyAgents3.Value > 1 {
		return fmt.Errorf("engine.executors.crowding_penalty_agents_3 (%.3f) must be in (0,1]", e.Executors.CrowdingPenaltyAgents3.Value)
	}
	if e.Executors.CrowdingPenaltyAgents4.Value <= 0 || e.Executors.CrowdingPenaltyAgents4.Value > 1 {
		return fmt.Errorf("engine.executors.crowding_penalty_agents_4 (%.3f) must be in (0,1]", e.Executors.CrowdingPenaltyAgents4.Value)
	}

	if e.Simulation.NeutralRegimeSizingFactor.Value <= 0 || e.Simulation.NeutralRegimeSizingFactor.Value > 1 {
		return fmt.Errorf("engine.simulation.neutral_regime_sizing_factor (%.3f) must be in (0,1]", e.Simulation.NeutralRegimeSizingFactor.Value)
	}

	if e.StrategyEvolution.CooldownPeriodHours.Value <= 0 {
		return fmt.Errorf("engine.strategy_evolution.cooldown_period_hours (%d) must be positive", e.StrategyEvolution.CooldownPeriodHours.Value)
	}

	if e.StructuralTrend.MinConfidence.Value < 0 || e.StructuralTrend.MinConfidence.Value > 1 {
		return fmt.Errorf("engine.structural_trend.min_confidence (%.3f) must be in [0,1]", e.StructuralTrend.MinConfidence.Value)
	}
	if e.StructuralTrend.MinHitRate.Value < 0 || e.StructuralTrend.MinHitRate.Value > 1 {
		return fmt.Errorf("engine.structural_trend.min_hit_rate (%.3f) must be in [0,1]", e.StructuralTrend.MinHitRate.Value)
	}
	if e.StructuralTrend.MinTrendStrength.Value < 0 || e.StructuralTrend.MinTrendStrength.Value > 1 {
		return fmt.Errorf("engine.structural_trend.min_trend_strength (%.3f) must be in [0,1]", e.StructuralTrend.MinTrendStrength.Value)
	}

	// RSITw threshold ordering — C1 (futures OI) thresholds must be monotonically non-decreasing
	if p.RSITw.C1VeryBullishThreshold.Value < p.RSITw.C1BullishThreshold.Value {
		return fmt.Errorf("rsi_tw.c1_very_bullish_threshold (%.0f) must be >= c1_bullish_threshold (%.0f)", p.RSITw.C1VeryBullishThreshold.Value, p.RSITw.C1BullishThreshold.Value)
	}
	if p.RSITw.C1BullishThreshold.Value < p.RSITw.C1BearishThreshold.Value {
		return fmt.Errorf("rsi_tw.c1_bullish_threshold (%.0f) must be >= c1_bearish_threshold (%.0f)", p.RSITw.C1BullishThreshold.Value, p.RSITw.C1BearishThreshold.Value)
	}
	if p.RSITw.C1BearishThreshold.Value < p.RSITw.C1VeryBearishThreshold.Value {
		return fmt.Errorf("rsi_tw.c1_bearish_threshold (%.0f) must be >= c1_very_bearish_threshold (%.0f)", p.RSITw.C1BearishThreshold.Value, p.RSITw.C1VeryBearishThreshold.Value)
	}

	// C3 (ETF net subscription) thresholds must be monotonically non-decreasing
	if p.RSITw.C3VeryBullishThreshold.Value < p.RSITw.C3BullishThreshold.Value {
		return fmt.Errorf("rsi_tw.c3_very_bullish_threshold (%.0f) must be >= c3_bullish_threshold (%.0f)", p.RSITw.C3VeryBullishThreshold.Value, p.RSITw.C3BullishThreshold.Value)
	}
	if p.RSITw.C3BullishThreshold.Value < p.RSITw.C3BearishThreshold.Value {
		return fmt.Errorf("rsi_tw.c3_bullish_threshold (%.0f) must be >= c3_bearish_threshold (%.0f)", p.RSITw.C3BullishThreshold.Value, p.RSITw.C3BearishThreshold.Value)
	}

	if p.SectorAllocation.WeightFloor <= 0 || p.SectorAllocation.WeightFloor >= 0.5 {
		return fmt.Errorf("sector_allocation.weight_floor (%.3f) must be in (0, 0.5)", p.SectorAllocation.WeightFloor)
	}
	for name, val := range map[string]float64{
		"cycle_weight":     p.SectorAllocation.CycleWeight,
		"seasonal_weight":  p.SectorAllocation.SeasonalWeight,
		"linkage_weight":   p.SectorAllocation.LinkageWeight,
		"narrative_weight": p.SectorAllocation.NarrativeWeight,
		"macro_weight":     p.SectorAllocation.MacroWeight,
		"factor_weight":    p.SectorAllocation.FactorWeight,
	} {
		if val < 0 {
			return fmt.Errorf("sector_allocation.%s (%.3f) must be non-negative", name, val)
		}
	}

	if p.Reporting.WinRateThreshold.Value < 0 || p.Reporting.WinRateThreshold.Value >= 1 {
		return fmt.Errorf("reporting.win_rate_threshold (%.3f) must be in [0, 1)", p.Reporting.WinRateThreshold.Value)
	}
	if p.Reporting.SharpeMinSamples.Value < 1 {
		return fmt.Errorf("reporting.sharpe_min_samples (%d) must be >= 1", p.Reporting.SharpeMinSamples.Value)
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

	// Merge defaults before validation so newly-added fields (missing from
	// the saved JSON) receive valid values instead of zero.
	mergeNarrativeDefaults(&cfg)
	mergeDrawdownDefaults(&cfg)
	mergeAlertDefaults(&cfg)
	mergeRiskGateDefaults(&cfg)
	mergeEngineDefaults(&cfg)
	mergeSectorExecutorDefaults(&cfg)
	mergeIndustryDefaults(&cfg)
	mergeRSITwDefaults(&cfg)
	mergeFallbackPriceTargetsDefaults(&cfg)
	mergeReportingDefaults(&cfg)

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
		// Defaults are already merged inside LoadParametersConfig.
		parametersConfig = cfg
	}
	return parametersConfig
}

// mergeNarrativeDefaults fills zero-valued narrative fields with defaults.
func mergeNarrativeDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Narrative
	n := &cfg.Narrative

	if n.GoldChangePctThreshold.Value == 0 {
		n.GoldChangePctThreshold = def.GoldChangePctThreshold
	}
	if n.USDTWDChangePctThreshold.Value == 0 {
		n.USDTWDChangePctThreshold = def.USDTWDChangePctThreshold
	}
	if n.SemiconductorExportDropThreshold.Value == 0 {
		n.SemiconductorExportDropThreshold = def.SemiconductorExportDropThreshold
	}
	if n.RetailMarginZScoreThreshold.Value == 0 {
		n.RetailMarginZScoreThreshold = def.RetailMarginZScoreThreshold
	}
	if n.AICapexSentimentThreshold.Value == 0 {
		n.AICapexSentimentThreshold = def.AICapexSentimentThreshold
	}
	if n.TSMCRevenueYoYThreshold.Value == 0 {
		n.TSMCRevenueYoYThreshold = def.TSMCRevenueYoYThreshold
	}
	if n.TaiwanStressUSDTWDThreshold.Value == 0 {
		n.TaiwanStressUSDTWDThreshold = def.TaiwanStressUSDTWDThreshold
	}
	if n.RetailInstitutionalDivergenceThreshold.Value == 0 {
		n.RetailInstitutionalDivergenceThreshold = def.RetailInstitutionalDivergenceThreshold
	}
	if n.AICapexNegativeSentimentThreshold.Value == 0 {
		n.AICapexNegativeSentimentThreshold = def.AICapexNegativeSentimentThreshold
	}
	if n.AICapexFallbackSentiment.Value == 0 {
		n.AICapexFallbackSentiment = def.AICapexFallbackSentiment
	}
	if n.TSMCRevenuePositiveThreshold.Value == 0 {
		n.TSMCRevenuePositiveThreshold = def.TSMCRevenuePositiveThreshold
	}
	if n.ConfidenceBaseUSRates.Value == 0 {
		n.ConfidenceBaseUSRates = def.ConfidenceBaseUSRates
	}
	if n.ConfidenceBaseJPYCarry.Value == 0 {
		n.ConfidenceBaseJPYCarry = def.ConfidenceBaseJPYCarry
	}
	if n.ConfidenceBaseGeopolitical.Value == 0 {
		n.ConfidenceBaseGeopolitical = def.ConfidenceBaseGeopolitical
	}
	if n.ConfidenceBaseOilShock.Value == 0 {
		n.ConfidenceBaseOilShock = def.ConfidenceBaseOilShock
	}
	if n.ConfidenceBaseAICapex.Value == 0 {
		n.ConfidenceBaseAICapex = def.ConfidenceBaseAICapex
	}
	if n.ConfidenceBaseTSMCRevenue.Value == 0 {
		n.ConfidenceBaseTSMCRevenue = def.ConfidenceBaseTSMCRevenue
	}
	if n.ConfidenceBaseTaiwanStress.Value == 0 {
		n.ConfidenceBaseTaiwanStress = def.ConfidenceBaseTaiwanStress
	}
	if n.ConfidenceDeviationCeiling.Value == 0 {
		n.ConfidenceDeviationCeiling = def.ConfidenceDeviationCeiling
	}
	if n.SOXIndexDropThreshold.Value == 0 {
		n.SOXIndexDropThreshold = def.SOXIndexDropThreshold
	}
	if n.RetailFrenzyPercentileThreshold.Value == 0 {
		n.RetailFrenzyPercentileThreshold = def.RetailFrenzyPercentileThreshold
	}
	if n.RetailFearPercentileThreshold.Value == 0 {
		n.RetailFearPercentileThreshold = def.RetailFearPercentileThreshold
	}
	if n.RetailAccelerationWindowDays.Value == 0 {
		n.RetailAccelerationWindowDays = def.RetailAccelerationWindowDays
	}
	if n.SpringFestivalConfidence.Value == 0 {
		n.SpringFestivalConfidence = def.SpringFestivalConfidence
	}
	if n.ElectionCycleConfidence.Value == 0 {
		n.ElectionCycleConfidence = def.ElectionCycleConfidence
	}
	if n.EarningsBlackoutConfidence.Value == 0 {
		n.EarningsBlackoutConfidence = def.EarningsBlackoutConfidence
	}
	if n.TechPeakSeasonConfidence.Value == 0 {
		n.TechPeakSeasonConfidence = def.TechPeakSeasonConfidence
	}
	if n.YearEndWindowDressingConfidence.Value == 0 {
		n.YearEndWindowDressingConfidence = def.YearEndWindowDressingConfidence
	}
	if n.EarningsSurpriseConfidence.Value == 0 {
		n.EarningsSurpriseConfidence = def.EarningsSurpriseConfidence
	}
	if n.EarningsSurpriseThreshold.Value == 0 {
		n.EarningsSurpriseThreshold = def.EarningsSurpriseThreshold
	}
	if n.TaiwanStressDXYScale.Value == 0 {
		n.TaiwanStressDXYScale = def.TaiwanStressDXYScale
	}
	if n.TaiwanStressUS10YScale.Value == 0 {
		n.TaiwanStressUS10YScale = def.TaiwanStressUS10YScale
	}
	if n.TaiwanStressForeignScale.Value == 0 {
		n.TaiwanStressForeignScale = def.TaiwanStressForeignScale
	}
	if n.TaiwanStressVIXScale.Value == 0 {
		n.TaiwanStressVIXScale = def.TaiwanStressVIXScale
	}
	if n.TaiwanStressJPYScale.Value == 0 {
		n.TaiwanStressJPYScale = def.TaiwanStressJPYScale
	}
	if n.TaiwanStressGeoScale.Value == 0 {
		n.TaiwanStressGeoScale = def.TaiwanStressGeoScale
	}
	if n.TaiwanStressOilScale.Value == 0 {
		n.TaiwanStressOilScale = def.TaiwanStressOilScale
	}
	if n.TaiwanStressGoldScale.Value == 0 {
		n.TaiwanStressGoldScale = def.TaiwanStressGoldScale
	}
	if n.TaiwanStressAlertThreshold.Value == 0 {
		n.TaiwanStressAlertThreshold = def.TaiwanStressAlertThreshold
	}
	if n.TaiwanStressHighThreshold.Value == 0 {
		n.TaiwanStressHighThreshold = def.TaiwanStressHighThreshold
	}
	if n.TaiwanStressCrisisThreshold.Value == 0 {
		n.TaiwanStressCrisisThreshold = def.TaiwanStressCrisisThreshold
	}
	if n.CalibrationBaselineWindow.Value == 0 {
		n.CalibrationBaselineWindow = def.CalibrationBaselineWindow
	}
	if n.CalibrationTargetMedian.Value == 0 {
		n.CalibrationTargetMedian = def.CalibrationTargetMedian
	}
	if n.CalibrationValidationPct.Value == 0 {
		n.CalibrationValidationPct = def.CalibrationValidationPct
	}
	if n.CalibrationMinRecords.Value == 0 {
		n.CalibrationMinRecords = def.CalibrationMinRecords
	}
	n.CalibrationEnabled = def.CalibrationEnabled
}

// mergeDrawdownDefaults fills zero-valued drawdown fields with defaults.
func mergeDrawdownDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Drawdown
	d := &cfg.Drawdown

	if d.NonePercentage.Value == 0 && d.NoneMaxExposure.Value == 0 {
		d.NonePercentage = def.NonePercentage
	}
	if d.NoneMaxExposure.Value == 0 {
		d.NoneMaxExposure = def.NoneMaxExposure
	}
	if d.LightPercentage.Value == 0 {
		d.LightPercentage = def.LightPercentage
	}
	if d.LightMaxExposure.Value == 0 {
		d.LightMaxExposure = def.LightMaxExposure
	}
	if d.ModeratePercentage.Value == 0 {
		d.ModeratePercentage = def.ModeratePercentage
	}
	if d.ModerateMaxExposure.Value == 0 {
		d.ModerateMaxExposure = def.ModerateMaxExposure
	}
	if d.SeverePercentage.Value == 0 {
		d.SeverePercentage = def.SeverePercentage
	}
	if d.SevereMaxExposure.Value == 0 {
		d.SevereMaxExposure = def.SevereMaxExposure
	}
	if d.EmergencyPercentage.Value == 0 {
		d.EmergencyPercentage = def.EmergencyPercentage
	}
	if d.EmergencyMaxExposure.Value == 0 {
		d.EmergencyMaxExposure = def.EmergencyMaxExposure
	}
	if d.OrangeOverrideMinScore.Value == 0 {
		d.OrangeOverrideMinScore = def.OrangeOverrideMinScore
	}
	if d.RedOverrideMinScore.Value == 0 {
		d.RedOverrideMinScore = def.RedOverrideMinScore
	}
	if len(d.SectorConstraintsRiskOff.Value) == 0 {
		d.SectorConstraintsRiskOff = def.SectorConstraintsRiskOff
	}
	if len(d.SectorConstraintsCarryTradeUnwind.Value) == 0 {
		d.SectorConstraintsCarryTradeUnwind = def.SectorConstraintsCarryTradeUnwind
	}
	if len(d.SectorConstraintsSectorRotation.Value) == 0 {
		d.SectorConstraintsSectorRotation = def.SectorConstraintsSectorRotation
	}
}

// mergeAlertDefaults fills zero-valued alert fields with defaults.
func mergeAlertDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Alert
	a := &cfg.Alert

	if a.MinCashThreshold.Value == 0 {
		a.MinCashThreshold = def.MinCashThreshold
	}
	if a.MaxPositionsCount.Value == 0 {
		a.MaxPositionsCount = def.MaxPositionsCount
	}
	if a.MaxPositionWeightPct.Value == 0 {
		a.MaxPositionWeightPct = def.MaxPositionWeightPct
	}
	if a.MaxUnrealizedLossPct.Value == 0 {
		a.MaxUnrealizedLossPct = def.MaxUnrealizedLossPct
	}
	if a.DailyLossWarningPct.Value == 0 {
		a.DailyLossWarningPct = def.DailyLossWarningPct
	}
	if a.DailyLossCriticalPct.Value == 0 {
		a.DailyLossCriticalPct = def.DailyLossCriticalPct
	}
	if a.RuleEngineIntervalSec.Value == 0 {
		a.RuleEngineIntervalSec = def.RuleEngineIntervalSec
	}
	if a.RuleEngineCooldownSec.Value == 0 {
		a.RuleEngineCooldownSec = def.RuleEngineCooldownSec
	}
	if a.SystemMetricsIntervalSec.Value == 0 {
		a.SystemMetricsIntervalSec = def.SystemMetricsIntervalSec
	}
	if a.MinScreeningRate.Value == 0 {
		a.MinScreeningRate = def.MinScreeningRate
	}
	if a.MaxAlertTriggerRate.Value == 0 {
		a.MaxAlertTriggerRate = def.MaxAlertTriggerRate
	}
	if a.MaxUnacknowledgedAlerts.Value == 0 {
		a.MaxUnacknowledgedAlerts = def.MaxUnacknowledgedAlerts
	}
}

// ResetParametersConfig clears the cached configuration so it will be reloaded
// from the JSON file on the next call to GetParametersConfig. This is intended
// for test environments where parameters.json may be updated between tests.
func ResetParametersConfig() {
	parametersConfig = nil
}

// ReloadParametersConfig re-reads the parameters JSON file and replaces the
// singleton configuration. Useful for hot-reload without server restart.
// Returns any parse or validation error.
func ReloadParametersConfig() error {
	cfg, err := LoadParametersConfig(parametersPath)
	if err != nil {
		return fmt.Errorf("reload parameters: %w", err)
	}
	parametersConfig = cfg
	return nil
}

// GetParametersConfigPath returns the path to the parameters configuration file.
func GetParametersConfigPath() string {
	return parametersPath
}

// mirrorCalibrationTimestamp injects a sibling `calibration_timestamp` field
// after every `last_calibrated` field found in the marshaled JSON, copying
// the same value. This keeps the two timestamp fields — one written by the
// Go struct (ParameterMetadata.LastCalibrated) and one written by raw-JSON
// consumers (cmd/calibrate-seasonal) — in sync after every Save() call.
//
// The injection operates line by line on the indented output, preserving
// the surrounding indentation. The original `last_calibrated` line is
// rewritten with a guaranteed trailing comma (the injected sibling requires
// it), and the mirror inherits a trailing comma only when the original
// line had one — preserving valid JSON when `last_calibrated` was the
// final field of its object. The function is a no-op when no
// `last_calibrated` field is present.
var calibrationTimestampLineRe = regexp.MustCompile(`^(\s*)"last_calibrated":\s*"([^"]*)"(,?)\s*$`)

func mirrorCalibrationTimestamp(data []byte) []byte {
	lines := bytes.Split(data, []byte("\n"))
	out := make([][]byte, 0, len(lines)+4)
	for _, line := range lines {
		m := calibrationTimestampLineRe.FindSubmatch(line)
		if m == nil {
			out = append(out, line)
			continue
		}
		indent := m[1]
		value := m[2]
		hadTrailingComma := len(m) >= 4 && len(m[3]) > 0
		rewritten := append([]byte{}, indent...)
		rewritten = append(rewritten, []byte(`"last_calibrated": "`)...)
		rewritten = append(rewritten, value...)
		rewritten = append(rewritten, '"', ',')
		out = append(out, rewritten)
		injected := append([]byte{}, indent...)
		injected = append(injected, []byte(`"calibration_timestamp": "`)...)
		injected = append(injected, value...)
		injected = append(injected, '"')
		if hadTrailingComma {
			injected = append(injected, ',')
		}
		out = append(out, injected)
	}
	return bytes.Join(out, []byte("\n"))
}

// Save writes the configuration to the given JSON file (non-atomic).
func (p *ParametersConfig) Save(path string) error {
	p.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal parameters config: %w", err)
	}
	data = mirrorCalibrationTimestamp(data)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write parameters config: %w", err)
	}
	return nil
}

// SaveWithRollback atomically writes the configuration with automatic rollback.
// Write pattern: .tmp → fsync → rename existing → .bak → rename .tmp → target.
// If any step after the .bak fails, the original file is restored from .bak.
func (p *ParametersConfig) SaveWithRollback(path string) error {
	tmpPath := path + ".tmp"
	bakPath := path + ".bak"

	p.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal parameters config: %w", err)
	}
	data = mirrorCalibrationTimestamp(data)

	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write temp parameters config: %w", err)
	}

	f, err := os.OpenFile(tmpPath, os.O_RDONLY, 0)
	if err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("open temp file for sync: %w", err)
	}
	_ = f.Sync()
	_ = f.Close()

	if _, statErr := os.Stat(path); statErr == nil {
		if err := os.Rename(path, bakPath); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("backup existing config: %w", err)
		}
	}

	if err := os.Rename(tmpPath, path); err != nil {
		if _, bakErr := os.Stat(bakPath); bakErr == nil {
			_ = os.Rename(bakPath, path)
		}
		return fmt.Errorf("promote temp config: %w", err)
	}

	_ = os.Remove(bakPath)
	return nil
}

func (p *ParametersConfig) LockedSaveWithRollback(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent dir for parameters config: %w", err)
	}
	locker := GetFileLocker(path)
	unlock := locker.Lock()
	defer unlock()
	return p.SaveWithRollback(path)
}

func mergeRiskGateDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().RiskGate
	r := &cfg.RiskGate

	if r.PreTrade.MaxPositionPct.Value == 0 {
		r.PreTrade.MaxPositionPct = def.PreTrade.MaxPositionPct
	}
	if r.PreTrade.MaxSectorExposurePct.Value == 0 {
		r.PreTrade.MaxSectorExposurePct = def.PreTrade.MaxSectorExposurePct
	}
	if r.PreTrade.VaRConfidenceLevel.Value == 0 {
		r.PreTrade.VaRConfidenceLevel = def.PreTrade.VaRConfidenceLevel
	}
	if r.PreTrade.VarLimitPct.Value == 0 {
		r.PreTrade.VarLimitPct = def.PreTrade.VarLimitPct
	}
	if r.PreTrade.MinCashBufferPct.Value == 0 {
		r.PreTrade.MinCashBufferPct = def.PreTrade.MinCashBufferPct
	}
	if r.PreTrade.MaxCorrelation.Value == 0 {
		r.PreTrade.MaxCorrelation = def.PreTrade.MaxCorrelation
	}
	if r.PreTrade.MinADVRatio.Value == 0 {
		r.PreTrade.MinADVRatio = def.PreTrade.MinADVRatio
	}
	if r.PreTrade.MaxOpenPositions.Value == 0 {
		r.PreTrade.MaxOpenPositions = def.PreTrade.MaxOpenPositions
	}
	if r.InTrade.MonitorIntervalSec.Value == 0 {
		r.InTrade.MonitorIntervalSec = def.InTrade.MonitorIntervalSec
	}
	if r.InTrade.StopLossPct.Value == 0 {
		r.InTrade.StopLossPct = def.InTrade.StopLossPct
	}
	if r.InTrade.TakeProfitPct.Value == 0 {
		r.InTrade.TakeProfitPct = def.InTrade.TakeProfitPct
	}
	if r.InTrade.TrailingStopATRMult.Value == 0 {
		r.InTrade.TrailingStopATRMult = def.InTrade.TrailingStopATRMult
	}
	if r.InTrade.VolatilitySpikeMult.Value == 0 {
		r.InTrade.VolatilitySpikeMult = def.InTrade.VolatilitySpikeMult
	}
	if r.InTrade.CircuitBreakerDailyLossPct.Value == 0 {
		r.InTrade.CircuitBreakerDailyLossPct = def.InTrade.CircuitBreakerDailyLossPct
	}
	if r.PostTrade.MaxDrawdownHaltPct.Value == 0 {
		r.PostTrade.MaxDrawdownHaltPct = def.PostTrade.MaxDrawdownHaltPct
	}
	if r.PostTrade.MaxDrawdownDefensivePct.Value == 0 {
		r.PostTrade.MaxDrawdownDefensivePct = def.PostTrade.MaxDrawdownDefensivePct
	}
	if r.PostTrade.MinRollingSharpe.Value == 0 {
		r.PostTrade.MinRollingSharpe = def.PostTrade.MinRollingSharpe
	}
	if r.PostTrade.ConsecutiveLossDays.Value == 0 {
		r.PostTrade.ConsecutiveLossDays = def.PostTrade.ConsecutiveLossDays
	}
	if r.PostTrade.EvaluationIntervalHours.Value == 0 {
		r.PostTrade.EvaluationIntervalHours = def.PostTrade.EvaluationIntervalHours
	}
}

func mergeEngineDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Engine
	e := &cfg.Engine

	if e.MacroRisk.VIXThreshold.Value == 0 {
		e.MacroRisk = def.MacroRisk
	}
	if e.StructuralTrend.MinConfidence.Value == 0 {
		e.StructuralTrend = def.StructuralTrend
	}
	if len(e.Drawdown.Levels.Value) == 0 {
		e.Drawdown = def.Drawdown
	}
	if len(e.SectorRotation.BaseAllocations.Value) == 0 {
		e.SectorRotation = def.SectorRotation
	}
	if e.StrategyEvolution.CooldownPeriodHours.Value == 0 {
		e.StrategyEvolution = def.StrategyEvolution
	}
	if e.Executors.VIXMomentumCrashThreshold.Value == 0 {
		e.Executors = def.Executors
	}
	if e.Simulation.NeutralRegimeSizingFactor.Value == 0 {
		e.Simulation = def.Simulation
	}
}

func mergeSectorExecutorDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().SectorExecutor
	s := &cfg.SectorExecutor

	if s.LEOSatellite.ConvictionBase.Value == 0 {
		s.LEOSatellite = def.LEOSatellite
	}
	if s.Financials.DividendBoost.Value == 0 {
		s.Financials = def.Financials
	}
	if s.Shipping.TacticalBoost.Value == 0 {
		s.Shipping = def.Shipping
	}
	if s.ValueYield.CashFlowBoost.Value == 0 {
		s.ValueYield = def.ValueYield
	}
	if s.EarningsQuality.RepeatableBoost.Value == 0 {
		s.EarningsQuality = def.EarningsQuality
	}
	if s.TechnicalBreakout.DefaultVolumeFloor.Value == 0 {
		s.TechnicalBreakout = def.TechnicalBreakout
	}
	if s.GrowthMomentum.ConvictionBase.Value == 0 {
		s.GrowthMomentum = def.GrowthMomentum
	}
	if s.FactorConviction.MomentumHighThreshold.Value == 0 {
		s.FactorConviction = def.FactorConviction
	}
}

func mergeIndustryDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Industry
	i := &cfg.Industry

	if i.AsymmetricDropCritical.Value == 0 {
		i.AsymmetricDropCritical = def.AsymmetricDropCritical
	}
	if i.AsymmetricDropHigh.Value == 0 {
		i.AsymmetricDropHigh = def.AsymmetricDropHigh
	}
	if i.AsymmetricDropMedium.Value == 0 {
		i.AsymmetricDropMedium = def.AsymmetricDropMedium
	}
	if i.NewsImpactMultiplier.Value == 0 {
		i.NewsImpactMultiplier = def.NewsImpactMultiplier
	}
	if i.BoundaryFallback.Value == 0 {
		i.BoundaryFallback = def.BoundaryFallback
	}
	if i.AdjustmentFloor.Value == 0 {
		i.AdjustmentFloor = def.AdjustmentFloor
	}
	if i.DynamicEnv.Value.HistoryWindowDays == 0 {
		i.DynamicEnv = def.DynamicEnv
	}
	if i.HistoryRetentionDays.Value == 0 {
		i.HistoryRetentionDays = def.HistoryRetentionDays
	}
	if i.SiliconCycle.Value.RevenueYoYThreshold == 0 &&
		i.SiliconCycle.Value.BillingsYoYThreshold == 0 &&
		i.SiliconCycle.Value.IndexMAPercentThreshold == 0 {
		i.SiliconCycle = def.SiliconCycle
	}
	if i.EventSentimentCap.Value == 0 {
		i.EventSentimentCap = def.EventSentimentCap
	}
	if len(i.ClassificationTree.Value.Segments) == 0 {
		i.ClassificationTree = def.ClassificationTree
	}
}

// mergeRSITwDefaults fills zero-valued RSITwParameters fields with defaults.
func mergeRSITwDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().RSITw
	r := &cfg.RSITw

	// Part A weights
	if r.A1Weight.Value == 0 {
		r.A1Weight = def.A1Weight
	}
	if r.A2Weight.Value == 0 {
		r.A2Weight = def.A2Weight
	}
	if r.A3Weight.Value == 0 {
		r.A3Weight = def.A3Weight
	}
	if r.A4Weight.Value == 0 {
		r.A4Weight = def.A4Weight
	}
	if r.A5Weight.Value == 0 {
		r.A5Weight = def.A5Weight
	}
	if r.A6Weight.Value == 0 {
		r.A6Weight = def.A6Weight
	}
	if r.APartWeight.Value == 0 {
		r.APartWeight = def.APartWeight
	}
	if r.CPartWeight.Value == 0 {
		r.CPartWeight = def.CPartWeight
	}

	// A3 formula
	if r.A3Midpoint.Value == 0 {
		r.A3Midpoint = def.A3Midpoint
	}
	if r.A3Scale.Value == 0 {
		r.A3Scale = def.A3Scale
	}

	// A4 VIX mapping
	if len(r.A4VixThresholds.Value) == 0 {
		r.A4VixThresholds = def.A4VixThresholds
	}
	if len(r.A4VixScores.Value) == 0 {
		r.A4VixScores = def.A4VixScores
	}

	// A5 PCR mapping
	if len(r.A5PcrThresholds.Value) == 0 {
		r.A5PcrThresholds = def.A5PcrThresholds
	}
	if len(r.A5PcrScores.Value) == 0 {
		r.A5PcrScores = def.A5PcrScores
	}
	if r.A5PcrFallback.Value == 0 {
		r.A5PcrFallback = def.A5PcrFallback
	}

	// A6 Odd-lot mapping
	if len(r.A6OddLotThresholds.Value) == 0 {
		r.A6OddLotThresholds = def.A6OddLotThresholds
	}
	if len(r.A6OddLotScores.Value) == 0 {
		r.A6OddLotScores = def.A6OddLotScores
	}
	if r.A6OddLotFallback.Value == 0 {
		r.A6OddLotFallback = def.A6OddLotFallback
	}

	// Part C sub-weights
	if r.C1Weight.Value == 0 {
		r.C1Weight = def.C1Weight
	}
	if r.C2Weight.Value == 0 {
		r.C2Weight = def.C2Weight
	}
	if r.C3Weight.Value == 0 {
		r.C3Weight = def.C3Weight
	}

	// Part C thresholds (existing)
	if r.C1VeryBullishThreshold.Value == 0 {
		r.C1VeryBullishThreshold = def.C1VeryBullishThreshold
	}
	if r.C1BullishThreshold.Value == 0 {
		r.C1BullishThreshold = def.C1BullishThreshold
	}
	if r.C1BearishThreshold.Value == 0 {
		r.C1BearishThreshold = def.C1BearishThreshold
	}
	if r.C1VeryBearishThreshold.Value == 0 {
		r.C1VeryBearishThreshold = def.C1VeryBearishThreshold
	}
	if r.C2NeutralMidpoint.Value == 0 {
		r.C2NeutralMidpoint = def.C2NeutralMidpoint
	}
	if r.C2NetflowScalingFactor.Value == 0 {
		r.C2NetflowScalingFactor = def.C2NetflowScalingFactor
	}
	if r.C3VeryBullishThreshold.Value == 0 {
		r.C3VeryBullishThreshold = def.C3VeryBullishThreshold
	}
	if r.C3BullishThreshold.Value == 0 {
		r.C3BullishThreshold = def.C3BullishThreshold
	}
	if r.C3BearishThreshold.Value == 0 {
		r.C3BearishThreshold = def.C3BearishThreshold
	}
	if r.DGeoPoliticalRiskThreshold.Value == 0 {
		r.DGeoPoliticalRiskThreshold = def.DGeoPoliticalRiskThreshold
	}
	if r.DGeoPoliticalRiskMultiplier.Value == 0 {
		r.DGeoPoliticalRiskMultiplier = def.DGeoPoliticalRiskMultiplier
	}
	if r.DVIXSpikeThreshold.Value == 0 {
		r.DVIXSpikeThreshold = def.DVIXSpikeThreshold
	}
	if r.DVIXSpikeMultiplier.Value == 0 {
		r.DVIXSpikeMultiplier = def.DVIXSpikeMultiplier
	}
	if r.DCreditTighteningMultiplier.Value == 0 {
		r.DCreditTighteningMultiplier = def.DCreditTighteningMultiplier
	}
}

// mergeFallbackPriceTargetsDefaults ensures every skill key (including _default)
// defined in defaults is present in the loaded config, and fills zero-valued
// target/stop-loss multipliers for any missing or partial entry. This prevents
// panics in monitoring/service/session.go when it looks up the _default key.
func mergeFallbackPriceTargetsDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().FallbackPriceTargets
	if cfg.FallbackPriceTargets == nil {
		cfg.FallbackPriceTargets = make(map[string]FallbackPriceTarget)
	}
	for key, defaultEntry := range def {
		entry, ok := cfg.FallbackPriceTargets[key]
		if !ok {
			cfg.FallbackPriceTargets[key] = defaultEntry
			continue
		}
		if entry.TargetMultiplier.Value == 0 {
			entry.TargetMultiplier = defaultEntry.TargetMultiplier
		}
		if entry.StopLossMultiplier.Value == 0 {
			entry.StopLossMultiplier = defaultEntry.StopLossMultiplier
		}
		cfg.FallbackPriceTargets[key] = entry
	}
}

// mergeReportingDefaults fills zero-valued fields with package-level defaults.
func mergeReportingDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Reporting
	if cfg.Reporting.WinRateThreshold.Value == 0 {
		cfg.Reporting.WinRateThreshold = def.WinRateThreshold
	}
	if cfg.Reporting.SharpeMinSamples.Value == 0 {
		cfg.Reporting.SharpeMinSamples = def.SharpeMinSamples
	}
}
