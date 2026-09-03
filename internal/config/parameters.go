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

// UseLLMSectorAgents gates the L2.3 PoC SemiconductorLLMAgent
// (internal/orchestrator/semiconductor_llm_agent.go) behind a
// feature flag. Default false keeps the deterministic
// SemiconductorExecutor as the production path; flip to true to
// route the semiconductor desk to the LLM-driven implementation
// (gated by the agent's Supports() method which reads this flag).
//
// Plan: PR5b of 7 in the Wave 10 L2.3 execution plan. Tagged
// v0.0.0.21 once PR5b lands. Cross-ref
// docs/guides/adding-sector-agents.md (moved from docs/wave-11/SEMICONDUCTOR_EXECUTOR.md).
var UseLLMSectorAgentsMetadata = ParameterMetadata[bool]{
	Value: false,
	Rationale: "Gate L2.3 PoC SemiconductorLLMAgent behind a flag; keep " +
		"deterministic SemiconductorExecutor as default until L2.4 " +
		"observation window validates LLM behavior.",
	Source: SourceExperimental,
	Todo:   "Promote to SourceEmpirical after L2.4 observation window (7-14 day run).",
}

// GetUseLLMSectorAgents returns the current value of the
// UseLLMSectorAgents flag. Reads from the loaded parameters config
// (or default-off if not loaded) so production default-off
// semantics hold even before config load.
func GetUseLLMSectorAgents() bool {
	cfg := GetParametersConfig()
	if cfg == nil {
		return UseLLMSectorAgentsMetadata.Value
	}
	return cfg.Orchestrator.UseLLMSectorAgents.Value
}

// ResonanceCoefficientMaxMetadata is the upper bound of ResonanceResult.Coefficient.
// Set to 1.5: foreign + institutional + government all share the same direction
// → 「三勢力全對齊」最強信號。PR #1007 invariant test (handler_test.go) guards this.
var ResonanceCoefficientMaxMetadata = ParameterMetadata[float64]{
	Value: 1.5,
	Rationale: "Resonance coefficient upper bound: 1.5 = 三勢力全對齊 (foreign + " +
		"institutional + government all aligned). Bound on ComputeResonance in " +
		"internal/capitalflow/resonance.go:13. Tied to PR #1007 invariant test.",
	Source: SourceHeuristic,
	Todo:   "Promote to SourceEmpirical after backtest validation across L2.4 observation window.",
}

// ResonanceCoefficientMinMetadata is the lower bound of ResonanceResult.Coefficient.
// Set to 0.5: foreign vs government opposing → 「foreign vs government 對立」最弱信號。
// PR #1007 invariant test guards this.
var ResonanceCoefficientMinMetadata = ParameterMetadata[float64]{
	Value: 0.5,
	Rationale: "Resonance coefficient lower bound: 0.5 = foreign vs government 對立. " +
		"Bound on ComputeResonance in internal/capitalflow/resonance.go:13. " +
		"Tied to PR #1007 invariant test.",
	Source: SourceHeuristic,
	Todo:   "Promote to SourceEmpirical after backtest validation across L2.4 observation window.",
}

// GetCapitalflowResonanceCoefficientMax returns the upper bound of ResonanceResult.Coefficient.
// Falls back to ResonanceCoefficientMaxMetadata.Value (1.5) when config is not yet loaded.
func GetCapitalflowResonanceCoefficientMax() float64 {
	if cfg := GetParametersConfig(); cfg != nil {
		return cfg.Capitalflow.ResonanceCoefficientMax.Value
	}
	return ResonanceCoefficientMaxMetadata.Value
}

// GetCapitalflowResonanceCoefficientMin returns the lower bound of ResonanceResult.Coefficient.
// Falls back to ResonanceCoefficientMinMetadata.Value (0.5) when config is not yet loaded.
func GetCapitalflowResonanceCoefficientMin() float64 {
	if cfg := GetParametersConfig(); cfg != nil {
		return cfg.Capitalflow.ResonanceCoefficientMin.Value
	}
	return ResonanceCoefficientMinMetadata.Value
}

// TrendBullishThresholdMetadata is the Z-score cutoff above which a
// capital force's trend is labeled "bullish" (trendFor in
// internal/capitalflow/forces.go). Audit M8 (2026-09-04): the ±0.5
// threshold was hardcoded while the seven-period detector's thresholds
// were already parameterized — this makes capital-flow thresholds
// configurable too, with the legacy value as the default so behavior
// is unchanged until a tuned value is promoted.
var TrendBullishThresholdMetadata = ParameterMetadata[float64]{
	Value: 0.5,
	Rationale: "Z-score cutoff for bullish force trend (trendFor): z > threshold → " +
		"bullish. Legacy hardcoded 0.5 kept as default (audit M8); mirrors the " +
		"parameterized-threshold convention of PeriodDetectionConfig.",
	Source: SourceHeuristic,
	Todo:   "Promote to SourceEmpirical after capital-flow hypothesis validation (H-CF-01/02/05) observes trend-label quality.",
}

// TrendBearishThresholdMetadata is the Z-score cutoff below which a
// capital force's trend is labeled "bearish" (trendFor in
// internal/capitalflow/forces.go). Negative mirror of
// TrendBullishThresholdMetadata; default -0.5 preserves the legacy
// hardcoded behavior (audit M8).
var TrendBearishThresholdMetadata = ParameterMetadata[float64]{
	Value: -0.5,
	Rationale: "Z-score cutoff for bearish force trend (trendFor): z < threshold → " +
		"bearish. Legacy hardcoded -0.5 kept as default (audit M8); mirrors the " +
		"parameterized-threshold convention of PeriodDetectionConfig.",
	Source: SourceHeuristic,
	Todo:   "Promote to SourceEmpirical after capital-flow hypothesis validation (H-CF-01/02/05) observes trend-label quality.",
}

// GetCapitalflowTrendBullishThreshold returns the configured bullish
// Z-score cutoff used by trendFor. Falls back to
// TrendBullishThresholdMetadata.Value (0.5) when config is not loaded.
func GetCapitalflowTrendBullishThreshold() float64 {
	if cfg := GetParametersConfig(); cfg != nil {
		return cfg.Capitalflow.TrendBullishThreshold.Value
	}
	return TrendBullishThresholdMetadata.Value
}

// GetCapitalflowTrendBearishThreshold returns the configured bearish
// Z-score cutoff used by trendFor. Falls back to
// TrendBearishThresholdMetadata.Value (-0.5) when config is not loaded.
func GetCapitalflowTrendBearishThreshold() float64 {
	if cfg := GetParametersConfig(); cfg != nil {
		return cfg.Capitalflow.TrendBearishThreshold.Value
	}
	return TrendBearishThresholdMetadata.Value
}

// PeriodWeightedQualityMetadata gates the capital-flow quality score
// formula switch (capital-flow model plan v1.1, k3 review B4):
//
//	false (default) → quality_score keeps the legacy equal-weight
//	                      Foreign(Z)+Institutional(Z)-Retail(Z) composite;
//	true            → quality_score switches to the period-weighted
//	                  computeQualityScoreWithPeriod composite.
//
// The switch is a behavior change (eventdriven scaleQualityScoreToBaseline
// consumes QualityScore), so it must stay observation-first: flip only
// after the 30-trading-day observation report passes human review.
var PeriodWeightedQualityMetadata = ParameterMetadata[bool]{
	Value: false,
	Rationale: "Config gate for the period-weighted capital-flow quality " +
		"score. Default false preserves the legacy equal-weight composite " +
		"(k3 review B4: switching semantics directly would change the " +
		"eventdriven baseline input — a hidden behavior change). " +
		"quality_score_period_weighted is always emitted alongside for " +
		"the 30-day observation window.",
	Source: SourceExperimental,
	Todo:   "Flip to true only after the 30-trading-day observation report (PR-3c) shows no regression and passes human review.",
}

// GetCapitalflowPeriodWeightedQuality returns whether the capital-flow
// quality score uses the period-weighted formula. Falls back to
// PeriodWeightedQualityMetadata.Value (false) when config is not loaded.
func GetCapitalflowPeriodWeightedQuality() bool {
	if cfg := GetParametersConfig(); cfg != nil {
		return cfg.Capitalflow.PeriodWeightedQuality.Value
	}
	return PeriodWeightedQualityMetadata.Value
}

// ActionObservationModeMetadata gates the capital-flow observation-mode
// decision log (PR-3c / plan §5.1). Default true: when
// ApplySectorRotation runs, the E07 capital-flow action is WRITTEN to
// data/state/capital_flow_observation.jsonl but never executed —
// the action is label-only (k3 B1: DriverProvenance["capital_flow"],
// weights come from DriverInputs.CapitalFlow, not the action).
// Setting false disables the log entirely (no observation data).
var ActionObservationModeMetadata = ParameterMetadata[bool]{
	Value: true,
	Rationale: "Observation-first decision wiring: record what the E07 " +
		"capital-flow action WOULD be at plan time without changing any " +
		"weight. The 30-trading-day observation report (label agreement, " +
		"hit rates) is the human-gate evidence for any future mutation " +
		"design PR (capitalflow.mutation_enabled stays undefined).",
	Source: SourceExperimental,
	Todo:   "After the 30-trading-day observation report passes human review, a separate action→delta mapper design PR decides the next step.",
}

// GetCapitalflowActionObservationMode returns whether the observation-mode
// decision log is enabled. Falls back to
// ActionObservationModeMetadata.Value (true, observe-only) when config is
// not loaded.
func GetCapitalflowActionObservationMode() bool {
	if cfg := GetParametersConfig(); cfg != nil {
		return cfg.Capitalflow.ActionObservationMode.Value
	}
	return ActionObservationModeMetadata.Value
}

// L2_4ScheduleParameters holds tunable values for the L2.4 LLM-driven
// sector agent observation period. Used by the synergy page admin
// UI to expose manual start/stop controls (auto-cron deferred to a
// follow-up round per user decision 3A).
type L2_4ScheduleParameters struct {
	DefaultStartTime   ParameterMetadata[string] `json:"default_start_time"`
	DefaultPeriodDays  ParameterMetadata[int]    `json:"default_period_days"`
	OverrideStartTime  ParameterMetadata[string] `json:"override_start_time"`
	OverridePeriodDays ParameterMetadata[int]    `json:"override_period_days"`
	AutoEnabled        ParameterMetadata[bool]   `json:"auto_enabled"`
}

// GetL2_4Schedule returns the current L2.4 schedule parameters
// from the loaded config, or zero-value defaults if config is
// not yet loaded.
func GetL2_4Schedule() L2_4ScheduleParameters {
	cfg := GetParametersConfig()
	if cfg == nil {
		return L2_4ScheduleParameters{
			DefaultStartTime:  ParameterMetadata[string]{Value: "13:45"},
			DefaultPeriodDays: ParameterMetadata[int]{Value: 7},
		}
	}
	return cfg.Orchestrator.L2_4Schedule
}

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
	// MinUniqueReturnsForSharpe is the minimum distinct values a rolling
	// window must contain for Sharpe to be statistically meaningful.
	// Default 10 (per-outcome sampling v2, 2026-08-27 mechanism fix).
	MinUniqueReturnsForSharpe ParameterMetadata[int]     `json:"min_unique_returns_for_sharpe"`
	StdDevMeanRatioThreshold  ParameterMetadata[float64] `json:"stddev_mean_ratio_threshold"`
	ConvictionClampMin        ParameterMetadata[int]     `json:"conviction_clamp_min"`
	ConvictionClampMax        ParameterMetadata[int]     `json:"conviction_clamp_max"`
	// ZeroSignalPenaltyMultiplier is the extra daily cut applied to agents
	// with zero signals for ZeroSignalPenaltyAfterDays (B3). Default 0.9.
	ZeroSignalPenaltyMultiplier ParameterMetadata[float64] `json:"zero_signal_penalty_multiplier"`
	// ZeroSignalPenaltyAfterDays is the number of days an agent must have
	// zero signals before the zero-signal penalty applies (B3). Default 14.
	ZeroSignalPenaltyAfterDays ParameterMetadata[int] `json:"zero_signal_penalty_after_days"`
	// LossPenaltyMultiplier is the extra daily cut applied to bottom-tier
	// agents with negative Sharpe and >=30 signals (B3). Default 0.9.
	LossPenaltyMultiplier ParameterMetadata[float64] `json:"loss_penalty_multiplier"`
	// WeightChangeAlertThreshold is the absolute daily weight change that
	// triggers a weight-change alert (B3). Default 0.15.
	WeightChangeAlertThreshold ParameterMetadata[float64] `json:"weight_change_alert_threshold"`
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
	SectorRotationMacroAdjustments   ParameterMetadata[map[string]map[string]float64] `json:"sector_rotation_macro_adjustments"`
	SectorRotationFlowAdjustments    ParameterMetadata[map[string]map[string]float64] `json:"sector_rotation_flow_adjustments"`
	UseMLScoring                     ParameterMetadata[bool]                          `json:"use_ml_scoring"`
	UseLLMSectorAgents               ParameterMetadata[bool]                          `json:"use_llm_sector_agents"`
	L2_4Schedule                     L2_4ScheduleParameters                           `json:"l2_4_schedule"`
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
	InflationEstimate               ParameterMetadata[float64] `json:"inflation_estimate"`

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
	SkillToIndustry        ParameterMetadata[map[string]string]       `json:"skill_to_industry"`
	SkillToIndustries      ParameterMetadata[map[string][]string]     `json:"skill_to_industries"`
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
	BaseWeights            ParameterMetadata[map[string]float64] `json:"base_weights"`
	RegimeBullMomentum     ParameterMetadata[float64]            `json:"regime_bull_momentum"`
	RegimeBullQuality      ParameterMetadata[float64]            `json:"regime_bull_quality"`
	RegimeBullValue        ParameterMetadata[float64]            `json:"regime_bull_value"`
	RegimeBearQuality      ParameterMetadata[float64]            `json:"regime_bear_quality"`
	RegimeBearValue        ParameterMetadata[float64]            `json:"regime_bear_value"`
	RegimeBearMomentum     ParameterMetadata[float64]            `json:"regime_bear_momentum"`
	RegimeHighVolLiquidity ParameterMetadata[float64]            `json:"regime_high_vol_liquidity"`
	RegimeHighVolMomentum  ParameterMetadata[float64]            `json:"regime_high_vol_momentum"`
	RegimeHighVolInstSent  ParameterMetadata[float64]            `json:"regime_high_vol_inst_sent"`
	SeverityCritical       ParameterMetadata[float64]            `json:"severity_critical"`
	SeverityHigh           ParameterMetadata[float64]            `json:"severity_high"`
	SeverityMedium         ParameterMetadata[float64]            `json:"severity_medium"`
	SeverityLow            ParameterMetadata[float64]            `json:"severity_low"`
	ClampMin               ParameterMetadata[float64]            `json:"clamp_min"`
	ClampMax               ParameterMetadata[float64]            `json:"clamp_max"`
	RiskOnMomentum         ParameterMetadata[float64]            `json:"risk_on_momentum"`
	RiskOnQuality          ParameterMetadata[float64]            `json:"risk_on_quality"`
	RiskOffMomentum        ParameterMetadata[float64]            `json:"risk_off_momentum"`
	RiskOffQuality         ParameterMetadata[float64]            `json:"risk_off_quality"`
	RiskOffLiquidity       ParameterMetadata[float64]            `json:"risk_off_liquidity"`
	ConservativeValue      ParameterMetadata[float64]            `json:"conservative_value"`
	ConservativeQuality    ParameterMetadata[float64]            `json:"conservative_quality"`
	ConservativeMomentum   ParameterMetadata[float64]            `json:"conservative_momentum"`
	AggressiveMomentum     ParameterMetadata[float64]            `json:"aggressive_momentum"`
	AggressiveInstSent     ParameterMetadata[float64]            `json:"aggressive_inst_sent"`
	AggressiveValue        ParameterMetadata[float64]            `json:"aggressive_value"`
	AggressiveQuality      ParameterMetadata[float64]            `json:"aggressive_quality"`
}

// NarrativeConvictionParameters maps skill types to narrative themes and their
// historical hit rates for conviction-driven weight adjustments.
type NarrativeConvictionParameters struct {
	ThemeHitRates ParameterMetadata[map[string]float64] `json:"theme_hit_rates"`
	SkillToTheme  ParameterMetadata[map[string]string]  `json:"skill_to_theme"`
}

// SectorExecutorParameters holds tunable conviction and price values
// for sector-level executors (LEO Satellite, Semiconductor, etc.).
// Each sub-struct groups parameters for a specific executor, so adding
// a new executor only requires adding its block of ParameterMetadata fields.
type SectorExecutorParameters struct {
	LEOSatellite      LEOSatelliteExecutorParameters      `json:"leo_satellite"`
	Financials        FinancialsExecutorParameters        `json:"financials"`
	Shipping          ShippingExecutorParameters          `json:"shipping"`
	ValueYield        ValueYieldExecutorParameters        `json:"value_yield"`
	EarningsQuality   EarningsQualityExecutorParameters   `json:"earnings_quality"`
	TechnicalBreakout TechnicalBreakoutExecutorParameters `json:"technical_breakout"`
	GrowthMomentum    GrowthMomentumExecutorParameters    `json:"growth_momentum"`
	FactorConviction  FactorConvictionParams              `json:"factor_conviction"`
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
	SlippageErrorBps         ParameterMetadata[float64]  `json:"slippage_error_bps"`
	SlippageWarningBps       ParameterMetadata[float64]  `json:"slippage_warning_bps"`
	SystemMetricsIntervalSec ParameterMetadata[int]      `json:"system_metrics_interval_sec"`
	MinScreeningRate         ParameterMetadata[float64]  `json:"min_screening_rate"`
	MaxAlertTriggerRate      ParameterMetadata[float64]  `json:"max_alert_trigger_rate"`
	MaxUnacknowledgedAlerts  ParameterMetadata[int]      `json:"max_unacknowledged_alerts"`
	SuppressCategories       ParameterMetadata[[]string] `json:"suppress_categories"`

	// Decision 1 (alert-redesign-v2.md Part 3.1): heartbeat staleness
	// threshold in minutes. Channel health summaries older than this
	// are considered "down" by the health check; older alerts with
	// rule=channel_health_summary are candidates for one-time cleanup.
	HeartbeatTTLMinutes ParameterMetadata[int] `json:"heartbeat_ttl_minutes"`

	// Decision 9 (alert-redesign-v2.md Part 3.7): per-severity alert SLA
	// thresholds (in seconds). AcknowledgedWithinSec > threshold = a
	// compliance violation. Default: 30m/2h/24h for critical/error/warning.
	AlertSLACriticalSec   ParameterMetadata[int]  `json:"alert_sla_critical_sec"`
	AlertSLAErrorSec      ParameterMetadata[int]  `json:"alert_sla_error_sec"`
	AlertSLAWarningSec    ParameterMetadata[int]  `json:"alert_sla_warning_sec"`
	SLAViolationMetaAlert ParameterMetadata[bool] `json:"sla_violation_meta_alert"`
}

// CapitalflowParameters holds tunables for the capital-flow analysis
// pipeline: the resonance coefficient bounds (see ComputeResonance in
// internal/capitalflow/resonance.go:13) and, since audit M8, the
// bullish/bearish Z-score trend thresholds used by ForceExtractor.trendFor
// (internal/capitalflow/forces.go). Values are design constants — not
// statistical fit — so they live under SourceHeuristic until backtest /
// hypothesis validation promotes them to SourceEmpirical.
type CapitalflowParameters struct {
	ResonanceCoefficientMax ParameterMetadata[float64] `json:"resonance_coefficient_max"`
	ResonanceCoefficientMin ParameterMetadata[float64] `json:"resonance_coefficient_min"`
	// TrendBullishThreshold / TrendBearishThreshold are the Z-score
	// cutoffs ForceExtractor.trendFor applies when labeling a
	// dimension "bullish" / "bearish" (internal/capitalflow/forces.go).
	// Parameterized in audit M8; the defaults (+0.5 / -0.5) reproduce
	// the pre-parameterization hardcoded thresholds exactly.
	TrendBullishThreshold ParameterMetadata[float64] `json:"trend_bullish_threshold"`
	TrendBearishThreshold ParameterMetadata[float64] `json:"trend_bearish_threshold"`
	// PeriodWeightedQuality gates the capital-flow quality score formula
	// (PR-3a, k3 review B4). Default false keeps the legacy equal-weight
	// composite; true switches quality_score to the period-weighted
	// formula. Observation-first: quality_score_period_weighted is always
	// emitted for side-by-side comparison regardless of the switch.
	PeriodWeightedQuality ParameterMetadata[bool] `json:"period_weighted_quality"`
	// ActionObservationMode gates the PR-3c observation-mode decision log
	// (default true = record-only). The E07 action is written to
	// data/state/capital_flow_observation.jsonl but never executed;
	// action_is_label_only is always true. No mutation switch exists in
	// this PR — a future action→delta mapper design PR adds
	// capitalflow.mutation_enabled (default off).
	ActionObservationMode ParameterMetadata[bool] `json:"action_observation_mode"`
}

// StockpickerParameters holds tunable parameters for the stock-picking
// backtest/aggregation subsystem (PR 1c). Values are heuristic until the
// backtest aggregation job produces enough samples to calibrate them
// (product-positioning §8 calibration philosophy).
type StockpickerParameters struct {
	Costs       StockpickerCostsParameters       `json:"costs"`
	Calibration StockpickerCalibrationParameters `json:"calibration"`
	Conditions  StockpickerConditionsParameters  `json:"conditions"`
	// FlowGateway configures the PR 2b two-level capital-flow gate
	// (internal/stockpicker/validator.go): a per-symbol foreign layer
	// (個股層) plus institutional/retail market-regime layers (市場 regime
	// 層). Read by the validator via config.GetParametersConfig().
	FlowGateway FlowGatewayParameters `json:"flow_gateway"`
}

// StockpickerCostsParameters holds trading-cost assumptions used by NetHit.
type StockpickerCostsParameters struct {
	// RoundTripPct is the round-trip transaction cost rate (Taiwan:
	// 0.1425% commission × 2 + 0.3% sell-side transaction tax ≈ 0.585%).
	RoundTripPct ParameterMetadata[float64] `json:"round_trip_pct"`
}

// StockpickerCalibrationParameters holds the sample-size gate for
// calibration status (observations < min_samples → calibrating).
type StockpickerCalibrationParameters struct {
	MinSamples ParameterMetadata[int] `json:"min_samples"`
}

// StockpickerConditionsParameters holds the tunable parameters of the
// registered stockpicker conditions (PR 2a). Each backtest-eligible
// condition reads its window (trading days) and trigger threshold from
// here; conditions.go contains no hard-coded numbers (P0-6).
type StockpickerConditionsParameters struct {
	Foreign3DNetBuy  StockpickerConditionWindow `json:"foreign_3d_net_buy"`
	Momentum20DPosit StockpickerConditionWindow `json:"momentum_20d_positive"`
}

// StockpickerConditionWindow is the numeric parameter set shared by the
// window+threshold conditions (foreign flow accumulation, price momentum).
type StockpickerConditionWindow struct {
	// WindowDays is the lookback window in trading days.
	WindowDays ParameterMetadata[float64] `json:"window_days"`
	// Threshold is the value the window aggregate must exceed to trigger.
	Threshold ParameterMetadata[float64] `json:"threshold"`
}

// FlowGatewayParameters is the stockpicker.flow_gateway section of
// configs/parameters.json (PR 2b). It configures the two-level capital-flow
// gateway in internal/stockpicker/validator.go: a per-symbol foreign layer
// (個股層) plus institutional/retail market-regime layers (市場 regime 層).
// Every value carries the repo-wide {"value", "rationale", "source"}
// provenance convention; the evaluator reads .Value only.
type FlowGatewayParameters struct {
	// FailClosedWhenAllMissing: when every enforced layer is skipped due to
	// missing data, close the gate (no-decision → fail). Default true so the
	// live path never trades on a fully-missing flow picture; backtests may
	// set false to keep evaluating on partial data.
	FailClosedWhenAllMissing ParameterMetadata[bool] `json:"fail_closed_when_all_missing"`
	Layers                   FlowGatewayLayers       `json:"layers"`
	Conditions               FlowGatewayConditions   `json:"conditions"`
}

// FlowGatewayLayers groups the per-layer threshold blocks.
type FlowGatewayLayers struct {
	// Foreign is the per-symbol layer (個股層): min_abs_net is the minimum
	// |foreign net buy| of the CHECKED SYMBOL in 億股. FlowPoint.ForeignNet
	// is 千股 (shares/1e3); the evaluator converts 億股 = ForeignNet / 1e5.
	Foreign FlowGatewayForeignThreshold `json:"foreign"`
	// Institutional and Retail are market-regime layers (市場 regime 層):
	// no per-symbol source exists, so they gate on the market-wide
	// capitalflow ForceScore readings (ForceInstitutional / ForceRetail) —
	// 無個股層級資料，僅供市場 regime 參考.
	Institutional FlowGatewayMarketThreshold `json:"institutional"`
	Retail        FlowGatewayMarketThreshold `json:"retail"`
}

// FlowGatewayForeignThreshold is the per-symbol foreign layer gate.
type FlowGatewayForeignThreshold struct {
	MinAbsNet ParameterMetadata[float64] `json:"min_abs_net"`
}

// FlowGatewayMarketThreshold is a market-regime layer gate (institutional /
// retail). A value <= 0 disables that metric (fail-open).
type FlowGatewayMarketThreshold struct {
	MinAbsRaw ParameterMetadata[float64] `json:"min_abs_raw"`
	MinAbsZ   ParameterMetadata[float64] `json:"min_abs_z"`
}

// FlowGatewayConditions maps each registered condition ID to its enforced
// layer set. A condition absent here enforces all three layers in the
// evaluator (AllFlowLayers).
type FlowGatewayConditions struct {
	Foreign3DNetBuy  FlowGatewayCondition `json:"foreign-3d-net-buy"`
	Momentum20DPosit FlowGatewayCondition `json:"momentum-20d-positive"`
}

// FlowGatewayCondition declares which gateway layers a condition enforces.
// Layer names must be one of foreign|institutional|retail (validated at
// config load — a layer-name typo is a load error, not a silent skip).
type FlowGatewayCondition struct {
	Layers ParameterMetadata[[]string] `json:"layers"`
}

// RiskGateParameters holds all tunable parameters for the unified risk gate system.
type RiskGateParameters struct {
	PreTrade  PreTradeGateParameters  `json:"pre_trade"`
	InTrade   InTradeGateParameters   `json:"in_trade"`
	PostTrade PostTradeGateParameters `json:"post_trade"`
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
	// thresholds[0]=15, thresholds[1]=20, ...; scores[0]=0.1 (vix<15), scores[5]=-1.0 (vix>=35)
	// Audit A10 (2026-08-12): 高分 = 狂熱（frenzy），VIX 高 = 市場恐慌 → 散戶恐慌 → 應推低分數。
	// 原 scores 全為正（VIX 高 → +1.0 推高狂熱），與 composite 語義矛盾。修正為負向：VIX 高 → 恐慌推低。
	A4VixThresholds ParameterMetadata[[]float64] `json:"a4_vix_thresholds"` // [15, 20, 25, 30, 35]
	A4VixScores     ParameterMetadata[[]float64] `json:"a4_vix_scores"`     // [0.1, 0.0, -0.3, -0.5, -0.7, -1.0]

	// A5: PCR piecewise mapping — thresholds are compared with > (strict), scores in order
	// Audit A10 (2026-08-12): PCR 高（賣權成交多）＝散戶避險/看空 → 恐慌 → 應推低分數。
	// 原 scores [0.9,0.7,0.5,0.1] 把恐慌訊號推高狂熱分數，與 composite 語義矛盾。修正為負向。
	A5PcrThresholds ParameterMetadata[[]float64] `json:"a5_pcr_thresholds"` // [1.5, 1.0, 0.8]
	A5PcrScores     ParameterMetadata[[]float64] `json:"a5_pcr_scores"`     // [-0.9, -0.5, -0.2, 0.3]
	A5PcrFallback   ParameterMetadata[float64]   `json:"a5_pcr_fallback"`   // score when pcr==0 (default 0.0)

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
	C1FallbackScore        ParameterMetadata[float64] `json:"c1_fallback_score"`         // score when futures OI pct == 0 (default 0.5)
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
	LastCalibratedScore ParameterMetadata[float64] `json:"last_calibrated_score"`
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
	FactorWeight         FactorWeightParameters         `json:"factor_weight"`
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
	NarrativeConviction  NarrativeConvictionParameters  `json:"narrative_conviction"`
	Marketdata           MarketdataParameters           `json:"marketdata"`
	Industry             IndustryParameters             `json:"industry"`
	Strategy             StrategyParameters             `json:"strategy"`
	PreciousMetals       PreciousMetalsParameters       `json:"precious_metals"`
	SectorExecutor       SectorExecutorParameters       `json:"sector_executor"`
	Alert                AlertParameters                `json:"alert"`
	Capitalflow          CapitalflowParameters          `json:"capitalflow"`
	RiskGate             RiskGateParameters             `json:"risk_gate"`
	Engine               EngineParameters               `json:"engine"`
	RSITw                RSITwParameters                `json:"rsi_tw"`
	Tax                  TaxParameters                  `json:"tax"`
	SectorAllocation     SectorAllocationConfig         `json:"sector_allocation"`
	Reporting            ReportingParameters            `json:"reporting"`
	SmartUniverse        SmartUniverseConfig            `json:"smart_universe"`
	ForwardReturn        ForwardReturnParameters        `json:"forward_return"`
	Stockpicker          StockpickerParameters          `json:"stockpicker"`
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
	StrategicPrior     EngineStrategicPriorParameters        `json:"strategic_prior"`
}

// EngineStrategicPriorParameters 是 strategic sector prior 的 schema 載體。
// spec §4.1 + SA-INV-05：Source 鎖死 "heuristic"、CalibrationStatus 鎖死 "calibrating"；
// 升 empirical / calibrated 不在 plan scope。
type EngineStrategicPriorParameters struct {
	Weights           ParameterMetadata[map[string]float64] `json:"weights"`
	Source            ParameterMetadata[string]             `json:"source"`
	ModelVersion      ParameterMetadata[string]             `json:"model_version"`
	CalibrationStatus ParameterMetadata[string]             `json:"calibration_status"`
	AsOfDate          ParameterMetadata[string]             `json:"as_of_date"`
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

// SmartUniverseConfig holds tunable parameters for the Smart Universe Builder.
// Parameters are Oracle-audited defaults from the SP4 design doc §9.
type SmartUniverseConfig struct {
	TopN                      ParameterMetadata[int]     `json:"top_n"`
	PEWeight                  ParameterMetadata[float64] `json:"pe_weight"`
	PBWeight                  ParameterMetadata[float64] `json:"pb_weight"`
	VolumeWeight              ParameterMetadata[float64] `json:"volume_weight"`
	MomentumWeight            ParameterMetadata[float64] `json:"momentum_weight"`
	QualityWeight             ParameterMetadata[float64] `json:"quality_weight"`
	ForeignFlowWeight         ParameterMetadata[float64] `json:"foreign_flow_weight"`
	VolumeFloorTWD            ParameterMetadata[float64] `json:"volume_floor_twd"`
	MinDailyAmountTWD         ParameterMetadata[float64] `json:"min_daily_amount_twd"`
	MaxIndustryConcentration  ParameterMetadata[float64] `json:"max_industry_concentration"`
	PriceMinimum              ParameterMetadata[float64] `json:"price_minimum"`
	FactorScoreMaxAgeDays     ParameterMetadata[int]     `json:"factor_score_max_age_days"`
	D6ExpiryTradingDays       ParameterMetadata[int]     `json:"d6_expiry_trading_days"`
	VaRContributionMultiplier ParameterMetadata[float64] `json:"var_contribution_multiplier"`
	VolatilityMultiplier      ParameterMetadata[float64] `json:"volatility_multiplier"`
	DrawdownWindow            ParameterMetadata[int]     `json:"drawdown_window"`
	DrawdownThreshold         ParameterMetadata[float64] `json:"drawdown_threshold"`
	ConfidenceThreshold       ParameterMetadata[int]     `json:"confidence_threshold"`
	SupplyChainExpandDepth    ParameterMetadata[int]     `json:"supply_chain_expand_depth"`
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
	// Canonical parameters path env var is ATLAS_PARAMETERS_CONFIG_PATH
	// (config.go Load); the legacy ATLAS_PARAMETERS_CONFIG name is removed
	// (PR 2b review fix — 假 env var, 倉庫正規是 ATLAS_PARAMETERS_CONFIG_PATH).
	parametersPath      = envOr("ATLAS_PARAMETERS_CONFIG_PATH", "configs/parameters.json")
	parametersConfigDir string // set when loaded from directory, used by Save
)

func ResetParametersConfig() {
	parametersConfig = nil
	parametersConfigDir = ""
}

func SetParametersConfigPath(path string) {
	parametersPath = path
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

// validateFlowGateway enforces the stockpicker.flow_gateway contract:
// condition layer names must be one of foreign|institutional|retail and all
// thresholds must be non-negative. A layer-name typo is a config-load error
// rather than a silent skip (PR 2b review fix), so a typo can never silently
// narrow the gate.
func (p *ParametersConfig) validateFlowGateway() error {
	validLayer := map[string]bool{"foreign": true, "institutional": true, "retail": true}
	checkLayers := func(condID string, layers []string) error {
		for _, l := range layers {
			if !validLayer[l] {
				return fmt.Errorf("stockpicker.flow_gateway.conditions.%s.layers: unknown layer %q (want foreign|institutional|retail)", condID, l)
			}
		}
		return nil
	}
	if err := checkLayers("foreign-3d-net-buy", p.Stockpicker.FlowGateway.Conditions.Foreign3DNetBuy.Layers.Value); err != nil {
		return err
	}
	if err := checkLayers("momentum-20d-positive", p.Stockpicker.FlowGateway.Conditions.Momentum20DPosit.Layers.Value); err != nil {
		return err
	}
	fg := p.Stockpicker.FlowGateway
	if fg.Layers.Foreign.MinAbsNet.Value < 0 {
		return fmt.Errorf("stockpicker.flow_gateway.layers.foreign.min_abs_net (%.3f) must be non-negative", fg.Layers.Foreign.MinAbsNet.Value)
	}
	if fg.Layers.Institutional.MinAbsRaw.Value < 0 {
		return fmt.Errorf("stockpicker.flow_gateway.layers.institutional.min_abs_raw (%.3f) must be non-negative", fg.Layers.Institutional.MinAbsRaw.Value)
	}
	if fg.Layers.Institutional.MinAbsZ.Value < 0 {
		return fmt.Errorf("stockpicker.flow_gateway.layers.institutional.min_abs_z (%.3f) must be non-negative", fg.Layers.Institutional.MinAbsZ.Value)
	}
	if fg.Layers.Retail.MinAbsRaw.Value < 0 {
		return fmt.Errorf("stockpicker.flow_gateway.layers.retail.min_abs_raw (%.3f) must be non-negative", fg.Layers.Retail.MinAbsRaw.Value)
	}
	if fg.Layers.Retail.MinAbsZ.Value < 0 {
		return fmt.Errorf("stockpicker.flow_gateway.layers.retail.min_abs_z (%.3f) must be non-negative", fg.Layers.Retail.MinAbsZ.Value)
	}
	return nil
}

// SetRiskCalibrationMetadata records when and how a risk parameter was
// calibrated. Called by risk.SelfCalibrate after SetParameter to pair the
// threshold-value update with its provenance metadata under the same
// configAccessMu lock. Without this pairing, concurrent OptimizeBayesian
// callers can observe a partially-updated state (new value, stale metadata
// or vice versa) via cloneParams.
func SetRiskCalibrationMetadata(name string, t time.Time, method string) {
	configAccessMu.Lock()
	defer configAccessMu.Unlock()
	now := t
	cfg := GetParametersConfig()
	switch name {
	case "risk_max_position_size":
		cfg.Risk.MaxPositionSize.LastCalibrated = &now
		cfg.Risk.MaxPositionSize.CalibrationMethod = method
	case "risk_max_daily_loss_pct":
		cfg.Risk.MaxDailyLossPct.LastCalibrated = &now
		cfg.Risk.MaxDailyLossPct.CalibrationMethod = method
	}
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
// If loaded from a directory (parametersConfigDir is set), writes per-category
// JSON files. Otherwise writes a single JSON file (legacy mode).
func (p *ParametersConfig) SaveWithRollback(path string) error {
	// Directory mode: write per-category files.
	if parametersConfigDir != "" && path == parametersConfigDir {
		return p.saveToDir(path)
	}
	return p.saveWithRollbackSingle(path)
}

// saveWithRollbackSingle writes to a single JSON file with rollback.
func (p *ParametersConfig) saveWithRollbackSingle(path string) error {
	tmpPath := path + ".tmp"
	bakPath := path + ".bak"

	configAccessMu.Lock()
	p.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(p, "", "  ")
	configAccessMu.Unlock()
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

// saveToDir writes each category as an individual JSON file under dir.
// Uses atomic write per file (.tmp → rename) to prevent partial updates.
func (p *ParametersConfig) saveToDir(dir string) error {
	configAccessMu.Lock()
	defer configAccessMu.Unlock()
	p.UpdatedAt = time.Now()

	// Marshal the full struct once, then extract per-category from the JSON.
	full, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal parameters config: %w", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(full, &raw); err != nil {
		return fmt.Errorf("unmarshal for split: %w", err)
	}

	// Write _meta.json (version + updated_at).
	meta := map[string]any{
		"version":    raw["version"],
		"updated_at": raw["updated_at"],
	}
	if err := writeJSONFile(filepath.Join(dir, "_meta.json"), meta); err != nil {
		return fmt.Errorf("write _meta.json: %w", err)
	}

	// Write each category file.
	metaKeys := map[string]bool{"version": true, "updated_at": true}
	for key, val := range raw {
		if metaKeys[key] {
			continue
		}
		// Unmarshal and re-marshal for clean indented output.
		var obj any
		if err := json.Unmarshal(val, &obj); err != nil {
			return fmt.Errorf("unmarshal category %s: %w", key, err)
		}
		if err := writeJSONFile(filepath.Join(dir, key+".json"), obj); err != nil {
			return fmt.Errorf("write %s.json: %w", key, err)
		}
	}

	return nil
}

// writeJSONFile atomically writes a JSON value to path using .tmp → rename.
func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (p *ParametersConfig) LockedSaveWithRollback(path string) error {
	lockPath := path
	if parametersConfigDir != "" && path == parametersConfigDir {
		// In directory mode, lock on _meta.json to serialize writes.
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("create parameters dir: %w", err)
		}
		lockPath = filepath.Join(path, "_meta.json")
	} else {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create parent dir for parameters config: %w", err)
		}
	}
	locker := GetFileLocker(lockPath)
	unlock := locker.Lock()
	defer unlock()
	return p.SaveWithRollback(path)
}

// TryLockedSaveWithRollback attempts to acquire the advisory file lock for
// parameters.json within the given timeout, then performs the same atomic
// write as SaveWithRollback. If the lock cannot be acquired in time, it
// returns an error immediately instead of blocking indefinitely, allowing
// callers to log and skip the write rather than stalling the process.
func (p *ParametersConfig) TryLockedSaveWithRollback(path string, timeout time.Duration) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent dir for parameters config: %w", err)
	}
	locker := GetFileLocker(path)
	unlock, err := locker.TryLock(timeout)
	if err != nil {
		return fmt.Errorf("acquire parameter file lock: %w", err)
	}
	defer unlock()
	return p.SaveWithRollback(path)
}

// SnapshotToBackup atomically writes a copy of path to path + ".snapshot.bak".
// Uses the .tmp → fsync → rename pattern (matching LockedWriteFileWithRollback)
// so a process crash mid-write cannot leave a partial snapshot file. The
// .snapshot.bak suffix intentionally differs from SaveWithRollback's .bak
// to avoid the line 1743 os.Remove(bakPath) cleanup.
func SnapshotToBackup(path string) error {
	if path == "" {
		return fmt.Errorf("snapshot path must not be empty")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("snapshot: read source: %w", err)
	}
	snapshotPath := path + ".snapshot.bak"
	tmpPath := snapshotPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("snapshot: write tmp: %w", err)
	}
	if f, err := os.OpenFile(tmpPath, os.O_RDONLY, 0); err == nil {
		_ = f.Sync()
		_ = f.Close()
	}
	if err := os.Rename(tmpPath, snapshotPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("snapshot: rename: %w", err)
	}
	return nil
}

// RestoreFromBackup atomically restores path from path + ".snapshot.bak".
// Uses the .tmp → fsync → rename pattern so a crash mid-restore cannot
// corrupt the live parameters file. After a successful rename, the
// in-memory ParametersConfig singleton is NOT refreshed here — callers
// that need an in-memory refresh should call ReloadParametersConfig()
// (this decoupling matches LockedWriteFileWithRollback's contract).
func RestoreFromBackup(path string) error {
	if path == "" {
		return fmt.Errorf("restore path must not be empty")
	}
	snapshotPath := path + ".snapshot.bak"
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		return fmt.Errorf("restore: read snapshot: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("restore: write tmp: %w", err)
	}
	if f, err := os.OpenFile(tmpPath, os.O_RDONLY, 0); err == nil {
		_ = f.Sync()
		_ = f.Close()
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("restore: rename: %w", err)
	}
	return nil
}
