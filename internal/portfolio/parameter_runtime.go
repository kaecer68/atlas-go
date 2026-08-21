package portfolio

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// RuntimeDarwinianConfig holds runtime values for Darwinian weight system.
type RuntimeDarwinianConfig struct {
	WeightMin                   float64
	WeightMax                   float64
	WeightNeutral               float64
	TopQuartileMultiplier       float64
	BottomQuartileMultiplier    float64
	DailyAdjustmentCooldown     time.Duration
	LookbackDays                int
	EMAAlpha                    float64
	SharpeNormalizeDenom        float64
	MaxPerformanceBonusPct      float64
	VolatilityPenaltyThreshold  float64
	VolatilityPenaltyMultiplier float64
	RiskVolatilityThreshold     float64
	RiskMultiplier              float64
	HitRateHighThreshold        float64
	HitRateLowThreshold         float64
	MiddleTierBoostMultiplier   float64
	MiddleTierCutMultiplier     float64
	SharpeMinSampleSize         int
	StdDevMeanRatioThreshold    float64
	ConvictionClampMin          int
	ConvictionClampMax          int
	ZeroSignalPenaltyMultiplier float64
	ZeroSignalPenaltyAfterDays  int
	LossPenaltyMultiplier       float64
	WeightChangeAlertThreshold  float64
}

// RuntimeFactorConfig holds runtime values for factor engine.
type RuntimeFactorConfig struct {
	MomentumLookbackDays          int
	MomentumStdDevDivisor         float64
	MomentumIntradayDiscount      float64
	MomentumIntradayThreshold     float64
	ValuePERangeCenter            float64
	ValuePERangeWidth             float64
	ValuePBRangeCenter            float64
	ValuePBRangeWidth             float64
	ValuePSRangeCenter            float64
	ValuePSRangeWidth             float64
	QualityDividendYieldCap       float64
	QualityVolatilityStd          float64
	QualityFallbackScore          float64
	ValueFallbackScore            float64
	InstitutionalSentimentWeights map[string]float64
	FallbackWeightReduction       float64
}

// RuntimeOptimizerConfig holds runtime values for portfolio optimization.
type RuntimeOptimizerConfig struct {
	MaxPositionPct   float64
	MaxSectorPct     float64
	MaxTurnoverDaily float64
	TargetBeta       float64
	BetaRangeMin     float64
	BetaRangeMax     float64
	MinTradeSize     int
	CashReserve      float64
	FactorWeights    map[string]float64
}

// RuntimeSizingConfig holds runtime values for position sizing.
type RuntimeSizingConfig struct {
	KellyFraction            float64
	VolLookbackDays          int
	MaxPositionByADV         float64
	MaxDrawdownLimit         float64
	ATRMultiplier            float64
	CorrelationPenalty       float64
	CorrelationThreshold     float64
	DefaultWinRate           float64
	DefaultPayoffRatio       float64
	TargetVolatility         float64
	VolAdjustmentMin         float64
	VolAdjustmentMax         float64
	ATRTargetRisk            float64
	ATRAdjustmentMin         float64
	ATRAdjustmentMax         float64
	CorrelationPenaltyFactor float64
	MaxCorrelationPenalty    float64
	DefaultVolatility        float64
	DefaultADV               float64
	DefaultATR               float64
}

// RuntimeHealthConfig holds runtime values for agent health management.
type RuntimeHealthConfig struct {
	MuteThreshold           int
	UnmuteThreshold         int
	AutoRecoverDays         int
	MinSampleSize           int
	NegativeSharpeThreshold float64
	SharpeWeight            float64
	HitRateWeight           float64
	StreakWeight            float64
	MaxSharpe               float64
	MinSharpe               float64
	StreakMax               int
}

// RuntimeGARCHConfig holds runtime values for volatility forecasting.
type RuntimeGARCHConfig struct {
	Omega               float64
	Alpha               float64
	Beta                float64
	MaxHistory          int
	CorrelationMinDays  int
	SmoothingFactor     float64
	RebalanceThreshold  float64
	MinForecastDays     int
	MaxHistoryPoints    int
	HighVolThreshold    float64
	LowVolThreshold     float64
	ReduceMagnitude     float64
	IncreaseMagnitude   float64
	WeeklyRebalanceDays int
}

// RuntimeExperimentConfig holds runtime values for experiment evaluation.
type RuntimeExperimentConfig struct {
	MaturityLevel1Observations int
	MaturityLevel2Observations int
	MaturityLevel3Observations int
	ImprovementThreshold       float64
	WelchTTestThreshold        float64
	DrawdownProtectionRatio    float64
	VolatilityToleranceRatio   float64
}

// RuntimeBaselineConfig holds runtime values for baseline policy defaults.
type RuntimeBaselineConfig struct {
	StartingCash                float64
	MaxPositionWeight           float64
	MaxOpenPositions            int
	MinTradableVolume           float64
	MinRecommendationConviction int
	RequireCROPass              bool
	TransactionCostBPS          float64
	SlippageBPS                 float64
	ReserveCashFraction         float64
}

// RuntimeRiskConfig holds runtime values for risk management.
type RuntimeRiskConfig struct {
	MaxDrawdownPct   float64
	MaxPositionSize  float64
	MaxDailyLossPct  float64
	StopLoss         float64
	TakeProfit       float64
	MaxLossPerTrade  float64
	MaxTotalExposure float64
}

// RuntimeMarketdataConfig holds runtime values for data provider configuration.
type RuntimeMarketdataConfig struct {
	TWSEAPIRateLimit     float64
	TWSEAPIRateBurst     int
	TWSEAPITimeoutSec    int
	FubonIntradayLimit   int
	FubonHistoricalLimit int
	FubonAPITimeoutSec   int
	TEJCallsPerSecond    int
	TEJAPITimeoutSec     int
	FugleRateLimit       int
	FugleAPITimeoutSec   int
	MaxRetryAttempts     int
	RetryBackoffMs       int
}

// RuntimeIndustryConfig holds runtime values for industry analysis.
type RuntimeIndustryConfig struct {
	InventoryCycleThresholds   config.InventoryCycleThresholdConfig
	CapexCycleThresholds       config.CapexCycleThresholdConfig
	CycleThresholds            map[string]config.CycleThresholdConfig
	ConcentrationRiskEnabled   bool
	NewsLatencyRiskEnabled     bool
	AsymmetricRiskEnabled      bool
	CustomerConcentrationLimit float64
	GeographicExposureLimit    float64
	CustomerShareThreshold1    float64
	CustomerShareThreshold2    float64
	USExposureThreshold1       float64
	USExposureThreshold2       float64
	RiskScoreWeight1           float64
	RiskScoreWeight2           float64
	RiskScoreWeight3           float64
	RiskScoreWeight4           float64
	SeverityThresholdMedium    float64
	SeverityThresholdHigh      float64
	SeverityThresholdCritical  float64
	ImpactMultiplier           float64
	RiskConfidence             float64
}

// RuntimeStrategyConfig holds runtime values for strategy selection.
type RuntimeStrategyConfig struct {
	MinSwitchIntervalDays int
	SwitchThreshold       float64
	ScoreLookbackDays     int
	FallbackStrategy      string
}

// RuntimeParameters holds all runtime parameter views.
type RuntimeParameters struct {
	Darwinian  RuntimeDarwinianConfig
	Factor     RuntimeFactorConfig
	Optimizer  RuntimeOptimizerConfig
	Sizing     RuntimeSizingConfig
	Health     RuntimeHealthConfig
	GARCH      RuntimeGARCHConfig
	Experiment RuntimeExperimentConfig
	Baseline   RuntimeBaselineConfig
	Risk       RuntimeRiskConfig
	Marketdata RuntimeMarketdataConfig
	Industry   RuntimeIndustryConfig
	Strategy   RuntimeStrategyConfig
}

// ToRuntimeParameters converts a typed ParametersConfig into pure runtime values.
func ToRuntimeParameters(cfg *config.ParametersConfig) *RuntimeParameters {
	if cfg == nil {
		cfg = config.DefaultParametersConfig()
	}

	darwinianCooldown, _ := time.ParseDuration(cfg.Darwinian.DailyAdjustmentCooldown.Value)
	if darwinianCooldown <= 0 {
		darwinianCooldown = 20 * time.Hour
	}

	return &RuntimeParameters{
		Darwinian: RuntimeDarwinianConfig{
			WeightMin:                   cfg.Darwinian.WeightMin.Value,
			WeightMax:                   cfg.Darwinian.WeightMax.Value,
			WeightNeutral:               cfg.Darwinian.WeightNeutral.Value,
			TopQuartileMultiplier:       cfg.Darwinian.TopQuartileMultiplier.Value,
			BottomQuartileMultiplier:    cfg.Darwinian.BottomQuartileMultiplier.Value,
			DailyAdjustmentCooldown:     darwinianCooldown,
			LookbackDays:                cfg.Darwinian.LookbackDays.Value,
			EMAAlpha:                    cfg.Darwinian.EMAAlpha.Value,
			SharpeNormalizeDenom:        cfg.Darwinian.SharpeNormalizeDenom.Value,
			MaxPerformanceBonusPct:      cfg.Darwinian.MaxPerformanceBonusPct.Value,
			VolatilityPenaltyThreshold:  cfg.Darwinian.VolatilityPenaltyThreshold.Value,
			VolatilityPenaltyMultiplier: cfg.Darwinian.VolatilityPenaltyMultiplier.Value,
			RiskVolatilityThreshold:     cfg.Darwinian.RiskVolatilityThreshold.Value,
			RiskMultiplier:              cfg.Darwinian.RiskMultiplier.Value,
			HitRateHighThreshold:        cfg.Darwinian.HitRateHighThreshold.Value,
			HitRateLowThreshold:         cfg.Darwinian.HitRateLowThreshold.Value,
			MiddleTierBoostMultiplier:   cfg.Darwinian.MiddleTierBoostMultiplier.Value,
			MiddleTierCutMultiplier:     cfg.Darwinian.MiddleTierCutMultiplier.Value,
			SharpeMinSampleSize:         cfg.Darwinian.SharpeMinSampleSize.Value,
			StdDevMeanRatioThreshold:    cfg.Darwinian.StdDevMeanRatioThreshold.Value,
			ConvictionClampMin:          cfg.Darwinian.ConvictionClampMin.Value,
			ConvictionClampMax:          cfg.Darwinian.ConvictionClampMax.Value,
			ZeroSignalPenaltyMultiplier: cfg.Darwinian.ZeroSignalPenaltyMultiplier.Value,
			ZeroSignalPenaltyAfterDays:  cfg.Darwinian.ZeroSignalPenaltyAfterDays.Value,
			LossPenaltyMultiplier:       cfg.Darwinian.LossPenaltyMultiplier.Value,
			WeightChangeAlertThreshold:  cfg.Darwinian.WeightChangeAlertThreshold.Value,
		},
		Factor: RuntimeFactorConfig{
			MomentumLookbackDays:          cfg.Factor.MomentumLookbackDays.Value,
			MomentumStdDevDivisor:         cfg.Factor.MomentumStdDevDivisor.Value,
			MomentumIntradayDiscount:      cfg.Factor.MomentumIntradayDiscount.Value,
			MomentumIntradayThreshold:     cfg.Factor.MomentumIntradayThreshold.Value,
			ValuePERangeCenter:            cfg.Factor.ValuePERangeCenter.Value,
			ValuePERangeWidth:             cfg.Factor.ValuePERangeWidth.Value,
			ValuePBRangeCenter:            cfg.Factor.ValuePBRangeCenter.Value,
			ValuePBRangeWidth:             cfg.Factor.ValuePBRangeWidth.Value,
			ValuePSRangeCenter:            cfg.Factor.ValuePSRangeCenter.Value,
			ValuePSRangeWidth:             cfg.Factor.ValuePSRangeWidth.Value,
			QualityDividendYieldCap:       cfg.Factor.QualityDividendYieldCap.Value,
			QualityVolatilityStd:          cfg.Factor.QualityVolatilityStd.Value,
			QualityFallbackScore:          cfg.Factor.QualityFallbackScore.Value,
			ValueFallbackScore:            cfg.Factor.ValueFallbackScore.Value,
			InstitutionalSentimentWeights: cfg.Factor.InstitutionalSentimentWeights.Value,
			FallbackWeightReduction:       cfg.Factor.FallbackWeightReduction.Value,
		},
		Optimizer: RuntimeOptimizerConfig{
			MaxPositionPct:   cfg.Optimizer.MaxPositionPct.Value,
			MaxSectorPct:     cfg.Optimizer.MaxSectorPct.Value,
			MaxTurnoverDaily: cfg.Optimizer.MaxTurnoverDaily.Value,
			TargetBeta:       cfg.Optimizer.TargetBeta.Value,
			BetaRangeMin:     cfg.Optimizer.BetaRangeMin.Value,
			BetaRangeMax:     cfg.Optimizer.BetaRangeMax.Value,
			MinTradeSize:     cfg.Optimizer.MinTradeSize.Value,
			CashReserve:      cfg.Optimizer.CashReserve.Value,
			FactorWeights:    cfg.Optimizer.FactorWeights.Value,
		},
		Sizing: RuntimeSizingConfig{
			KellyFraction:            cfg.Sizing.KellyFraction.Value,
			VolLookbackDays:          cfg.Sizing.VolLookbackDays.Value,
			MaxPositionByADV:         cfg.Sizing.MaxPositionByADV.Value,
			MaxDrawdownLimit:         cfg.Sizing.MaxDrawdownLimit.Value,
			ATRMultiplier:            cfg.Sizing.ATRMultiplier.Value,
			CorrelationPenalty:       cfg.Sizing.CorrelationPenalty.Value,
			CorrelationThreshold:     cfg.Sizing.CorrelationThreshold.Value,
			DefaultWinRate:           cfg.Sizing.DefaultWinRate.Value,
			DefaultPayoffRatio:       cfg.Sizing.DefaultPayoffRatio.Value,
			TargetVolatility:         cfg.Sizing.TargetVolatility.Value,
			VolAdjustmentMin:         cfg.Sizing.VolAdjustmentMin.Value,
			VolAdjustmentMax:         cfg.Sizing.VolAdjustmentMax.Value,
			ATRTargetRisk:            cfg.Sizing.ATRTargetRisk.Value,
			ATRAdjustmentMin:         cfg.Sizing.ATRAdjustmentMin.Value,
			ATRAdjustmentMax:         cfg.Sizing.ATRAdjustmentMax.Value,
			CorrelationPenaltyFactor: cfg.Sizing.CorrelationPenaltyFactor.Value,
			MaxCorrelationPenalty:    cfg.Sizing.MaxCorrelationPenalty.Value,
			DefaultVolatility:        cfg.Sizing.DefaultVolatility.Value,
			DefaultADV:               cfg.Sizing.DefaultADV.Value,
			DefaultATR:               cfg.Sizing.DefaultATR.Value,
		},
		Health: RuntimeHealthConfig{
			MuteThreshold:           cfg.Health.MuteThreshold.Value,
			UnmuteThreshold:         cfg.Health.UnmuteThreshold.Value,
			AutoRecoverDays:         cfg.Health.AutoRecoverDays.Value,
			MinSampleSize:           cfg.Health.MinSampleSize.Value,
			NegativeSharpeThreshold: cfg.Health.NegativeSharpeThreshold.Value,
			SharpeWeight:            cfg.Health.SharpeWeight.Value,
			HitRateWeight:           cfg.Health.HitRateWeight.Value,
			StreakWeight:            cfg.Health.StreakWeight.Value,
			MaxSharpe:               cfg.Health.MaxSharpe.Value,
			MinSharpe:               cfg.Health.MinSharpe.Value,
			StreakMax:               cfg.Health.StreakMax.Value,
		},
		GARCH: RuntimeGARCHConfig{
			Omega:               cfg.GARCH.Omega.Value,
			Alpha:               cfg.GARCH.Alpha.Value,
			Beta:                cfg.GARCH.Beta.Value,
			MaxHistory:          cfg.GARCH.MaxHistory.Value,
			CorrelationMinDays:  cfg.GARCH.CorrelationMinDays.Value,
			SmoothingFactor:     cfg.GARCH.SmoothingFactor.Value,
			RebalanceThreshold:  cfg.GARCH.RebalanceThreshold.Value,
			MinForecastDays:     cfg.GARCH.MinForecastDays.Value,
			MaxHistoryPoints:    cfg.GARCH.MaxHistoryPoints.Value,
			HighVolThreshold:    cfg.GARCH.HighVolThreshold.Value,
			LowVolThreshold:     cfg.GARCH.LowVolThreshold.Value,
			ReduceMagnitude:     cfg.GARCH.ReduceMagnitude.Value,
			IncreaseMagnitude:   cfg.GARCH.IncreaseMagnitude.Value,
			WeeklyRebalanceDays: cfg.GARCH.WeeklyRebalanceDays.Value,
		},
		Experiment: RuntimeExperimentConfig{
			MaturityLevel1Observations: cfg.Experiment.MaturityLevel1Observations.Value,
			MaturityLevel2Observations: cfg.Experiment.MaturityLevel2Observations.Value,
			MaturityLevel3Observations: cfg.Experiment.MaturityLevel3Observations.Value,
			ImprovementThreshold:       cfg.Experiment.ImprovementThreshold.Value,
			WelchTTestThreshold:        cfg.Experiment.WelchTTestThreshold.Value,
			DrawdownProtectionRatio:    cfg.Experiment.DrawdownProtectionRatio.Value,
			VolatilityToleranceRatio:   cfg.Experiment.VolatilityToleranceRatio.Value,
		},
		Baseline: RuntimeBaselineConfig{
			StartingCash:                cfg.Baseline.StartingCash.Value,
			MaxPositionWeight:           cfg.Baseline.MaxPositionWeight.Value,
			MaxOpenPositions:            cfg.Baseline.MaxOpenPositions.Value,
			MinTradableVolume:           cfg.Baseline.MinTradableVolume.Value,
			MinRecommendationConviction: cfg.Baseline.MinRecommendationConviction.Value,
			RequireCROPass:              cfg.Baseline.RequireCROPass.Value,
			TransactionCostBPS:          cfg.Baseline.TransactionCostBPS.Value,
			SlippageBPS:                 cfg.Baseline.SlippageBPS.Value,
			ReserveCashFraction:         cfg.Baseline.ReserveCashFraction.Value,
		},
		Risk: RuntimeRiskConfig{
			MaxDrawdownPct:   cfg.Risk.MaxDrawdownPct.Value,
			MaxPositionSize:  cfg.Risk.MaxPositionSize.Value,
			MaxDailyLossPct:  cfg.Risk.MaxDailyLossPct.Value,
			StopLoss:         cfg.Risk.StopLoss.Value,
			TakeProfit:       cfg.Risk.TakeProfit.Value,
			MaxLossPerTrade:  cfg.Risk.MaxLossPerTrade.Value,
			MaxTotalExposure: cfg.Risk.MaxTotalExposure.Value,
		},
		Marketdata: RuntimeMarketdataConfig{
			TWSEAPIRateLimit:     cfg.Marketdata.TWSEAPIRateLimit.Value,
			TWSEAPIRateBurst:     cfg.Marketdata.TWSEAPIRateBurst.Value,
			TWSEAPITimeoutSec:    cfg.Marketdata.TWSEAPITimeoutSec.Value,
			FubonIntradayLimit:   cfg.Marketdata.FubonIntradayLimit.Value,
			FubonHistoricalLimit: cfg.Marketdata.FubonHistoricalLimit.Value,
			FubonAPITimeoutSec:   cfg.Marketdata.FubonAPITimeoutSec.Value,
			TEJCallsPerSecond:    cfg.Marketdata.TEJCallsPerSecond.Value,
			TEJAPITimeoutSec:     cfg.Marketdata.TEJAPITimeoutSec.Value,
			FugleRateLimit:       cfg.Marketdata.FugleRateLimit.Value,
			FugleAPITimeoutSec:   cfg.Marketdata.FugleAPITimeoutSec.Value,
			MaxRetryAttempts:     cfg.Marketdata.MaxRetryAttempts.Value,
			RetryBackoffMs:       cfg.Marketdata.RetryBackoffMs.Value,
		},
		Industry: RuntimeIndustryConfig{
			// DEPRECATED: SectorWeights field removed; use sector_allocation.base_weights
			InventoryCycleThresholds:   cfg.Industry.InventoryCycleThresholds.Value,
			CapexCycleThresholds:       cfg.Industry.CapexCycleThresholds.Value,
			CycleThresholds:            cfg.Industry.CycleThresholds.Value,
			ConcentrationRiskEnabled:   cfg.Industry.ConcentrationRiskEnabled.Value,
			NewsLatencyRiskEnabled:     cfg.Industry.NewsLatencyRiskEnabled.Value,
			AsymmetricRiskEnabled:      cfg.Industry.AsymmetricRiskEnabled.Value,
			CustomerConcentrationLimit: cfg.Industry.CustomerConcentrationLimit.Value,
			GeographicExposureLimit:    cfg.Industry.GeographicExposureLimit.Value,
			CustomerShareThreshold1:    cfg.Industry.CustomerShareThreshold1.Value,
			CustomerShareThreshold2:    cfg.Industry.CustomerShareThreshold2.Value,
			USExposureThreshold1:       cfg.Industry.USExposureThreshold1.Value,
			USExposureThreshold2:       cfg.Industry.USExposureThreshold2.Value,
			RiskScoreWeight1:           cfg.Industry.RiskScoreWeight1.Value,
			RiskScoreWeight2:           cfg.Industry.RiskScoreWeight2.Value,
			RiskScoreWeight3:           cfg.Industry.RiskScoreWeight3.Value,
			RiskScoreWeight4:           cfg.Industry.RiskScoreWeight4.Value,
			SeverityThresholdMedium:    cfg.Industry.SeverityThresholdMedium.Value,
			SeverityThresholdHigh:      cfg.Industry.SeverityThresholdHigh.Value,
			SeverityThresholdCritical:  cfg.Industry.SeverityThresholdCritical.Value,
			ImpactMultiplier:           cfg.Industry.ImpactMultiplier.Value,
			RiskConfidence:             cfg.Industry.RiskConfidence.Value,
		},
		Strategy: RuntimeStrategyConfig{
			MinSwitchIntervalDays: cfg.Strategy.MinSwitchIntervalDays.Value,
			SwitchThreshold:       cfg.Strategy.SwitchThreshold.Value,
			ScoreLookbackDays:     cfg.Strategy.ScoreLookbackDays.Value,
			FallbackStrategy:      cfg.Strategy.FallbackStrategy.Value,
		},
	}
}

// DefaultRuntimeParameters returns runtime parameters from default config.
func DefaultRuntimeParameters() *RuntimeParameters {
	return ToRuntimeParameters(config.DefaultParametersConfig())
}
