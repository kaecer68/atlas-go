package config

import (
	"time"
)

// DefaultParametersConfig returns a configuration that exactly mirrors
// the current hard-coded values in the portfolio, experiment, and baseline
// packages. This ensures zero behavioral drift when no config file exists.
func DefaultParametersConfig() *ParametersConfig {
	now := time.Now()
	return &ParametersConfig{
		Version:             "1.0",
		UpdatedAt:           now,
		Darwinian:           defaultDarwinianParameters(),
		Factor:              defaultFactorParameters(),
		Optimizer:           defaultOptimizerParameters(),
		Sizing:              defaultSizingParameters(),
		Health:              defaultHealthParameters(),
		GARCH:               defaultGARCHParameters(),
		Experiment:          defaultExperimentParameters(),
		Baseline:            defaultBaselineParameters(),
		Orchestrator:        defaultOrchestratorParameters(),
		Risk:                defaultRiskParameters(),
		Realtime:            defaultRealtimeParameters(),
		Narrative:           defaultNarrativeParameters(),
		Janus:               defaultJanusParameters(),
		Marketdata:          defaultMarketdataParameters(),
		Industry:            defaultIndustryParameters(),
		Strategy:            defaultStrategyParameters(),
		FactorWeight:        defaultFactorWeightParameters(),
		NarrativeConviction: defaultNarrativeConvictionParameters(),
	}
}

func defaultDarwinianParameters() DarwinianParameters {
	return DarwinianParameters{
		WeightMin: ParameterMetadata[float64]{
			Value:     0.3,
			Rationale: "Whisper level: agent influence reduced to 30%",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from backtest: test [0.2, 0.5] range",
		},
		WeightMax: ParameterMetadata[float64]{
			Value:     2.5,
			Rationale: "Shout level: agent influence amplified to 250%",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from backtest: test [2.0, 3.0] range",
		},
		WeightNeutral: ParameterMetadata[float64]{
			Value:     1.0,
			Rationale: "Baseline: no adjustment",
			Source:    SourceHeuristic,
		},
		TopQuartileMultiplier: ParameterMetadata[float64]{
			Value:     1.05,
			Rationale: "5% daily boost for top performers",
			Source:    SourceHeuristic,
			Todo:      "Literature review: Atlas-GIC uses tiered scaling, exact value unverified",
		},
		BottomQuartileMultiplier: ParameterMetadata[float64]{
			Value:     0.95,
			Rationale: "5% daily cut for bottom performers",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test asymmetric penalties",
		},
		DailyAdjustmentCooldown: ParameterMetadata[string]{
			Value:     "20h",
			Rationale: "Slightly less than 24h for daily trading frequency",
			Source:    SourceHeuristic,
		},
		LookbackDays: ParameterMetadata[int]{
			Value:     20,
			Rationale: "One trading month (approx 20 business days)",
			Source:    SourceHeuristic,
			Todo:      "Literature: 20-day Sharpe has low statistical power; consider 60-day",
		},
		EMAAlpha: ParameterMetadata[float64]{
			Value:     0.3,
			Rationale: "Standard EMA smoothing factor",
			Source:    SourceLiterature,
		},
		SharpeNormalizeDenom: ParameterMetadata[float64]{
			Value:     1.5,
			Rationale: "Sigmoid normalization knee: Sharpe/(Sharpe+1.5); calibrated to top-quartile agent Sharpe for middle-range discrimination",
			Source:    SourceHeuristic,
		},
		MaxPerformanceBonusPct: ParameterMetadata[float64]{
			Value:     0.20,
			Rationale: "Cap performance bonus at +20%",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from backtest optimization",
		},
		VolatilityPenaltyThreshold: ParameterMetadata[float64]{
			Value:     0.05,
			Rationale: "5% daily volatility (~79% annualized) is high for TW market",
			Source:    SourceEmpirical,
		},
		VolatilityPenaltyMultiplier: ParameterMetadata[float64]{
			Value:     0.95,
			Rationale: "5% penalty for high volatility agents",
			Source:    SourceHeuristic,
		},
		RiskVolatilityThreshold: ParameterMetadata[float64]{
			Value:     0.08,
			Rationale: "8% daily volatility is extreme; extra penalty warranted",
			Source:    SourceEmpirical,
		},
		RiskMultiplier: ParameterMetadata[float64]{
			Value:     0.9,
			Rationale: "Extra 10% cut for extreme volatility",
			Source:    SourceHeuristic,
		},
		HitRateHighThreshold: ParameterMetadata[float64]{
			Value:     0.6,
			Rationale: "60% hit rate is good in TW equity market",
			Source:    SourceEmpirical,
		},
		HitRateLowThreshold: ParameterMetadata[float64]{
			Value:     0.4,
			Rationale: "40% hit rate is poor and warrants reduction",
			Source:    SourceEmpirical,
		},
		MiddleTierBoostMultiplier: ParameterMetadata[float64]{
			Value:     1.02,
			Rationale: "2% mild boost for middle tier with good hit rate",
			Source:    SourceHeuristic,
		},
		MiddleTierCutMultiplier: ParameterMetadata[float64]{
			Value:     0.98,
			Rationale: "2% mild cut for middle tier with poor hit rate",
			Source:    SourceHeuristic,
		},
		SharpeMinSampleSize: ParameterMetadata[int]{
			Value:     5,
			Rationale: "Minimum observations for Sharpe calculation",
			Source:    SourceLiterature,
			Todo:      "Literature: 5 is insufficient for stable Sharpe; recommend 20+",
		},
		StdDevMeanRatioThreshold: ParameterMetadata[float64]{
			Value:     0.001,
			Rationale: "Guard against IEEE 754 precision edge case for identical returns",
			Source:    SourceInferred,
		},
		ConvictionClampMin: ParameterMetadata[int]{
			Value:     1,
			Rationale: "Minimum conviction after weight scaling",
			Source:    SourceHeuristic,
		},
		ConvictionClampMax: ParameterMetadata[int]{
			Value:     250,
			Rationale: "Maximum conviction after weight scaling (100 * 2.5)",
			Source:    SourceHeuristic,
		},
	}
}

func defaultFactorParameters() FactorParameters {
	return FactorParameters{
		MomentumLookbackDays: ParameterMetadata[int]{
			Value:     20,
			Rationale: "Standard 20-day momentum window",
			Source:    SourceLiterature,
		},
		MomentumStdDevDivisor: ParameterMetadata[float64]{
			Value:     0.30,
			Rationale: "30% return = full score; most TW stocks score below 0.3",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: use historical return distribution 90th percentile",
		},
		MomentumIntradayDiscount: ParameterMetadata[float64]{
			Value:     0.5,
			Rationale: "50% discount for intraday returns when historical data unavailable",
			Source:    SourceHeuristic,
			Todo:      "SCOR-01: review discount factor with more data",
		},
		MomentumIntradayThreshold: ParameterMetadata[float64]{
			Value:     0.10,
			Rationale: "10% intraday return = full score",
			Source:    SourceHeuristic,
		},
		ValuePERangeCenter: ParameterMetadata[float64]{
			Value:     5.0,
			Rationale: "P/E=5 is cheap; P/E=50 is expensive",
			Source:    SourceHeuristic,
		},
		ValuePERangeWidth: ParameterMetadata[float64]{
			Value:     45.0,
			Rationale: "Range [5, 50] covers most TW stocks",
			Source:    SourceEmpirical,
		},
		ValuePBRangeCenter: ParameterMetadata[float64]{
			Value:     0.5,
			Rationale: "P/B=0.5 is cheap",
			Source:    SourceHeuristic,
		},
		ValuePBRangeWidth: ParameterMetadata[float64]{
			Value:     4.5,
			Rationale: "Range [0.5, 5.0]",
			Source:    SourceHeuristic,
		},
		ValuePSRangeCenter: ParameterMetadata[float64]{
			Value:     0.5,
			Rationale: "P/S=0.5 is cheap",
			Source:    SourceHeuristic,
		},
		ValuePSRangeWidth: ParameterMetadata[float64]{
			Value:     9.5,
			Rationale: "Range [0.5, 10.0]",
			Source:    SourceHeuristic,
		},
		QualityDividendYieldCap: ParameterMetadata[float64]{
			Value:     5.0,
			Rationale: "5% dividend yield is excellent for TW market",
			Source:    SourceEmpirical,
		},
		QualityVolatilityStd: ParameterMetadata[float64]{
			Value:     0.05,
			Rationale: "5% daily volatility as quality benchmark",
			Source:    SourceHeuristic,
			Todo:      "Review: TW market average daily vol is 1-3%, this may be too high",
		},
		QualityFallbackScore: ParameterMetadata[float64]{
			Value:     0.05,
			Rationale: "Low fallback score when data unavailable",
			Source:    SourceHeuristic,
		},
		ValueFallbackScore: ParameterMetadata[float64]{
			Value:     0.05,
			Rationale: "Conservative fallback score when data unavailable (harmonized with QualityFallbackScore)",
			Source:    SourceHeuristic,
		},
		InstitutionalSentimentWeights: ParameterMetadata[map[string]float64]{
			Value: map[string]float64{
				"foreign":  0.50,
				"domestic": 0.30,
				"margin":   0.20,
			},
			Rationale: "Foreign flow 50%, domestic 30%, margin balance 20%",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from regression: predict next-day returns",
		},
		FallbackWeightReduction: ParameterMetadata[float64]{
			Value:     0.5,
			Rationale: "50% weight reduction for fallback estimates",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: measure fallback estimate accuracy",
		},
	}
}

func defaultOptimizerParameters() OptimizerParameters {
	return OptimizerParameters{
		MaxPositionPct: ParameterMetadata[float64]{
			Value:     0.15,
			Rationale: "15% max single position for diversification",
			Source:    SourceLiterature,
		},
		MaxSectorPct: ParameterMetadata[float64]{
			Value:     0.40,
			Rationale: "40% max single sector concentration",
			Source:    SourceHeuristic,
		},
		MaxTurnoverDaily: ParameterMetadata[float64]{
			Value:     0.20,
			Rationale: "20% daily turnover limit",
			Source:    SourceHeuristic,
			Todo:      "Currently unused in optimizer; implement or remove",
		},
		TargetBeta: ParameterMetadata[float64]{
			Value:     1.0,
			Rationale: "Market-neutral target beta",
			Source:    SourceLiterature,
			Todo:      "Currently unused; implement beta constraint",
		},
		BetaRangeMin: ParameterMetadata[float64]{
			Value:     0.8,
			Rationale: "Minimum portfolio beta",
			Source:    SourceHeuristic,
			Todo:      "Currently unused",
		},
		BetaRangeMax: ParameterMetadata[float64]{
			Value:     1.2,
			Rationale: "Maximum portfolio beta",
			Source:    SourceHeuristic,
			Todo:      "Currently unused",
		},
		MinTradeSize: ParameterMetadata[int]{
			Value:     1,
			Rationale: "Minimum 1 share",
			Source:    SourceHeuristic,
			Todo:      "Currently unused; implement minimum trade size",
		},
		CashReserve: ParameterMetadata[float64]{
			Value:     0.05,
			Rationale: "5% cash reserve for liquidity",
			Source:    SourceHeuristic,
		},
		FactorWeights: ParameterMetadata[map[string]float64]{
			Value: map[string]float64{
				"momentum":       0.25,
				"value":          0.20,
				"quality":        0.20,
				"agent":          0.15,
				"narrative":      0.10,
				"industry_cycle": 0.10,
			},
			Rationale: "Momentum 25%, Value 20%, Quality 20%, Agent 15%, Narrative 10%, Industry Cycle 10%",
			Source:    SourceHeuristic,
			Todo:      "Calibrate narrative and industry cycle weights via backtest; verify sum = 1.0",
		},
	}
}

func defaultSizingParameters() SizingParameters {
	return SizingParameters{
		KellyFraction: ParameterMetadata[float64]{
			Value:     0.25,
			Rationale: "Half-Kelly for safety; literature recommends 0.2-0.5",
			Source:    SourceLiterature,
		},
		VolLookbackDays: ParameterMetadata[int]{
			Value:     20,
			Rationale: "20-day volatility lookback",
			Source:    SourceLiterature,
		},
		MaxPositionByADV: ParameterMetadata[float64]{
			Value:     0.01,
			Rationale: "1% of average daily volume",
			Source:    SourceLiterature,
		},
		MaxDrawdownLimit: ParameterMetadata[float64]{
			Value:     0.15,
			Rationale: "15% max drawdown limit (professional fund risk controls: 15-25% range)",
			Source:    SourceHeuristic,
		},
		ATRMultiplier: ParameterMetadata[float64]{
			Value:     2.0,
			Rationale: "2x ATR stop-loss is standard",
			Source:    SourceLiterature,
		},
		CorrelationPenalty: ParameterMetadata[float64]{
			Value:     0.5,
			Rationale: "50% correlation penalty scaling",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from portfolio optimization backtest",
		},
		CorrelationThreshold: ParameterMetadata[float64]{
			Value:     0.70,
			Rationale: "70% correlation is high and warrants penalty",
			Source:    SourceHeuristic,
		},
		DefaultWinRate: ParameterMetadata[float64]{
			Value:     0.5,
			Rationale: "Neutral prior: 50% win rate",
			Source:    SourceHeuristic,
			Todo:      "Should be overridden by agent historical stats",
		},
		DefaultPayoffRatio: ParameterMetadata[float64]{
			Value:     1.0,
			Rationale: "Neutral prior: 1:1 payoff ratio",
			Source:    SourceHeuristic,
			Todo:      "Should be overridden by agent historical stats",
		},
		TargetVolatility: ParameterMetadata[float64]{
			Value:     0.20,
			Rationale: "20% annual target volatility",
			Source:    SourceLiterature,
		},
		VolAdjustmentMin: ParameterMetadata[float64]{
			Value:     0.25,
			Rationale: "Minimum 0.25x size adjustment",
			Source:    SourceHeuristic,
		},
		VolAdjustmentMax: ParameterMetadata[float64]{
			Value:     2.0,
			Rationale: "Maximum 2.0x size adjustment",
			Source:    SourceHeuristic,
		},
		ATRTargetRisk: ParameterMetadata[float64]{
			Value:     0.02,
			Rationale: "2% risk per trade",
			Source:    SourceLiterature,
		},
		ATRAdjustmentMin: ParameterMetadata[float64]{
			Value:     0.25,
			Rationale: "Minimum 0.25x ATR adjustment",
			Source:    SourceHeuristic,
		},
		ATRAdjustmentMax: ParameterMetadata[float64]{
			Value:     2.0,
			Rationale: "Maximum 2.0x ATR adjustment",
			Source:    SourceHeuristic,
		},
		CorrelationPenaltyFactor: ParameterMetadata[float64]{
			Value:     0.7,
			Rationale: "70% of correlation used as penalty",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from backtest",
		},
		MaxCorrelationPenalty: ParameterMetadata[float64]{
			Value:     0.7,
			Rationale: "Maximum 70% correlation penalty",
			Source:    SourceHeuristic,
		},
		DefaultVolatility: ParameterMetadata[float64]{
			Value:     0.25,
			Rationale: "25% annualized default volatility",
			Source:    SourceHeuristic,
		},
		DefaultADV: ParameterMetadata[float64]{
			Value:     100000000,
			Rationale: "100M TWD default average daily volume",
			Source:    SourceHeuristic,
		},
		DefaultATR: ParameterMetadata[float64]{
			Value:     0.02,
			Rationale: "Non-zero default for ATR-based position sizing (2% of price as fallback)",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from backtest: test [0.01, 0.03] range",
		},
	}
}

func defaultHealthParameters() HealthParameters {
	return HealthParameters{
		MuteThreshold: ParameterMetadata[int]{
			Value:     5,
			Rationale: "5 consecutive losses triggers mute",
			Source:    SourceHeuristic,
		},
		UnmuteThreshold: ParameterMetadata[int]{
			Value:     3,
			Rationale: "3 consecutive wins triggers recovery",
			Source:    SourceHeuristic,
		},
		AutoRecoverDays: ParameterMetadata[int]{
			Value:     7,
			Rationale: "Auto-recover after 7 days muted",
			Source:    SourceHeuristic,
		},
		MinSampleSize: ParameterMetadata[int]{
			Value:     10,
			Rationale: "Minimum 10 observations for health assessment",
			Source:    SourceLiterature,
			Todo:      "Currently defined but unused; implement in evaluateInterventions",
		},
		NegativeSharpeThreshold: ParameterMetadata[float64]{
			Value:     -0.5,
			Rationale: "Sharpe below -0.5 indicates severe underperformance",
			Source:    SourceHeuristic,
		},
		SharpeWeight: ParameterMetadata[float64]{
			Value:     0.40,
			Rationale: "Sharpe contributes 40% to composite score",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from predictive power regression",
		},
		HitRateWeight: ParameterMetadata[float64]{
			Value:     0.30,
			Rationale: "Hit rate contributes 30% to composite score",
			Source:    SourceHeuristic,
		},
		StreakWeight: ParameterMetadata[float64]{
			Value:     0.30,
			Rationale: "Streak contributes 30% to composite score",
			Source:    SourceHeuristic,
		},
		MaxSharpe: ParameterMetadata[float64]{
			Value:     5.0,
			Rationale: "Cap Sharpe at 5.0 for normalization",
			Source:    SourceHeuristic,
			Todo:      "Use historical distribution instead of hard cap",
		},
		MinSharpe: ParameterMetadata[float64]{
			Value:     -5.0,
			Rationale: "Floor Sharpe at -5.0 for normalization",
			Source:    SourceHeuristic,
		},
		StreakMax: ParameterMetadata[int]{
			Value:     10,
			Rationale: "10 consecutive wins = full streak score",
			Source:    SourceHeuristic,
		},
	}
}

func defaultGARCHParameters() GARCHParameters {
	return GARCHParameters{
		Omega: ParameterMetadata[float64]{
			Value:     0.000001,
			Rationale: "GARCH(1,1) omega: long-run variance component",
			Source:    SourceLiterature,
			Todo:      "MLE estimation from historical returns",
		},
		Alpha: ParameterMetadata[float64]{
			Value:     0.1,
			Rationale: "GARCH(1,1) alpha: shock persistence",
			Source:    SourceLiterature,
			Todo:      "MLE estimation from historical returns",
		},
		Beta: ParameterMetadata[float64]{
			Value:     0.85,
			Rationale: "GARCH(1,1) beta: variance persistence",
			Source:    SourceLiterature,
			Todo:      "MLE estimation from historical returns",
		},
		MaxHistory: ParameterMetadata[int]{
			Value:     252,
			Rationale: "One year of trading days",
			Source:    SourceLiterature,
		},
		CorrelationMinDays: ParameterMetadata[int]{
			Value:     30,
			Rationale: "Minimum 30 days for correlation calculation",
			Source:    SourceLiterature,
		},
		SmoothingFactor: ParameterMetadata[float64]{
			Value:     0.3,
			Rationale: "EMA smoothing factor for volatility",
			Source:    SourceLiterature,
		},
		RebalanceThreshold: ParameterMetadata[float64]{
			Value:     0.05,
			Rationale: "5% deviation threshold for rebalancing",
			Source:    SourceHeuristic,
		},
		MinForecastDays: ParameterMetadata[int]{
			Value:     100,
			Rationale: "Minimum 100 days for GARCH forecast",
			Source:    SourceHeuristic,
		},
		MaxHistoryPoints: ParameterMetadata[int]{
			Value:     1000,
			Rationale: "Maximum volatility history points to retain",
			Source:    SourceHeuristic,
		},
		HighVolThreshold: ParameterMetadata[float64]{
			Value:     1.5,
			Rationale: "Asset vol > target*1.5 triggers reduction",
			Source:    SourceHeuristic,
		},
		LowVolThreshold: ParameterMetadata[float64]{
			Value:     0.7,
			Rationale: "Current vol < target*0.7 triggers increase",
			Source:    SourceHeuristic,
		},
		ReduceMagnitude: ParameterMetadata[float64]{
			Value:     0.5,
			Rationale: "Reduce by 50% for high vol assets",
			Source:    SourceHeuristic,
		},
		IncreaseMagnitude: ParameterMetadata[float64]{
			Value:     0.2,
			Rationale: "Increase by 20% for low vol assets",
			Source:    SourceHeuristic,
		},
		WeeklyRebalanceDays: ParameterMetadata[int]{
			Value:     7,
			Rationale: "Weekly rebalancing interval",
			Source:    SourceHeuristic,
		},
	}
}

func defaultExperimentParameters() ExperimentParameters {
	return ExperimentParameters{
		MaturityLevel1Observations: ParameterMetadata[int]{
			Value:     3,
			Rationale: "Minimum 3 observations for exploratory experiments",
			Source:    SourceHeuristic,
			Todo:      "Literature suggests n>=10 for any statistical conclusion",
		},
		MaturityLevel2Observations: ParameterMetadata[int]{
			Value:     8,
			Rationale: "Minimum 8 observations for window-validated experiments",
			Source:    SourceHeuristic,
		},
		MaturityLevel3Observations: ParameterMetadata[int]{
			Value:     12,
			Rationale: "Minimum 12 observations for regime-aware experiments",
			Source:    SourceHeuristic,
		},
		ImprovementThreshold: ParameterMetadata[float64]{
			Value:     0.0005,
			Rationale: "0.05% minimum improvement over baseline",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: too low may accept noise; too high rejects real improvements",
		},
		WelchTTestThreshold: ParameterMetadata[float64]{
			Value:     2.0,
			Rationale: "|t| >= 2.0 corresponds to ~95% confidence",
			Source:    SourceLiterature,
		},
		DrawdownProtectionRatio: ParameterMetadata[float64]{
			Value:     0.8,
			Rationale: "Candidate drawdown must be >= 80% of baseline",
			Source:    SourceHeuristic,
		},
		VolatilityToleranceRatio: ParameterMetadata[float64]{
			Value:     1.5,
			Rationale: "Candidate volatility <= 150% of baseline",
			Source:    SourceHeuristic,
		},
		OOSWindowDays: ParameterMetadata[int]{
			Value:     30,
			Rationale: "30-day out-of-sample validation window",
			Source:    SourceHeuristic,
		},
		SharpeStabilityThreshold: ParameterMetadata[float64]{
			Value:     0.5,
			Rationale: "Sharpe stability: stderr < 0.5 AND |mean|/stddev >= 0.5",
			Source:    SourceHeuristic,
		},
		MaxFallbackRatio: ParameterMetadata[float64]{
			Value:     0.6,
			Rationale: "Maximum fraction of factors allowed to be IsFallback (60%) before degrading experiment confidence",
			Source:    SourceHeuristic,
		},
	}
}

func defaultOrchestratorParameters() OrchestratorParameters {
	return OrchestratorParameters{
		ConvictionFloorDefault: ParameterMetadata[int]{
			Value:     50,
			Rationale: "Default conviction floor when not set by policy",
			Source:    SourceHeuristic,
		},
		SuperinvestorMinConviction: ParameterMetadata[int]{
			Value:     65,
			Rationale: "Higher bar for superinvestor recommendations",
			Source:    SourceHeuristic,
		},
		CROZScoreThreshold: ParameterMetadata[float64]{
			Value:     -1.5,
			Rationale: "Z-score threshold for CRO conviction normalization filter",
			Source:    SourceHeuristic,
		},
		SectorConcentrationThreshold: ParameterMetadata[float64]{
			Value:     0.40,
			Rationale: "Sector concentration threshold (< 10 recommendations)",
			Source:    SourceHeuristic,
		},
		SectorConcentrationThresholdHigh: ParameterMetadata[float64]{
			Value:     0.35,
			Rationale: "Sector concentration threshold (>= 10 recommendations)",
			Source:    SourceHeuristic,
		},
		SectorConvictionMultiplier: ParameterMetadata[float64]{
			Value:     0.7,
			Rationale: "Conviction multiplier when sector is overcrowded",
			Source:    SourceHeuristic,
		},
		CrowdedConvictionMultiplier: ParameterMetadata[float64]{
			Value:     0.7,
			Rationale: "Conviction multiplier when 3+ agents recommend same symbol",
			Source:    SourceHeuristic,
		},
		FactorWeightMomentum: ParameterMetadata[float64]{
			Value:     0.30,
			Rationale: "Momentum factor weight in total score",
			Source:    SourceHeuristic,
		},
		FactorWeightValue: ParameterMetadata[float64]{
			Value:     0.25,
			Rationale: "Value factor weight in total score",
			Source:    SourceHeuristic,
		},
		FactorWeightQuality: ParameterMetadata[float64]{
			Value:     0.25,
			Rationale: "Quality factor weight in total score",
			Source:    SourceHeuristic,
		},
		FactorWeightAgent: ParameterMetadata[float64]{
			Value:     0.20,
			Rationale: "Agent factor weight in total score",
			Source:    SourceHeuristic,
		},
		PRISMBoostMultiplier: ParameterMetadata[float64]{
			Value:     20.0,
			Rationale: "Multiplier for PRISM Sharpe to conviction boost",
			Source:    SourceHeuristic,
		},
		PRISMBoostMin: ParameterMetadata[int]{
			Value:     -10,
			Rationale: "Minimum PRISM conviction boost",
			Source:    SourceHeuristic,
		},
		PRISMBoostMax: ParameterMetadata[int]{
			Value:     15,
			Rationale: "Maximum PRISM conviction boost",
			Source:    SourceHeuristic,
		},
		PromotionMinObservations: ParameterMetadata[int]{
			Value:     10,
			Rationale: "Minimum observations before auto-promotion consideration",
			Source:    SourceHeuristic,
		},
		PromotionSharpeThreshold: ParameterMetadata[float64]{
			Value:     0.5,
			Rationale: "Sharpe threshold for auto-promotion",
			Source:    SourceHeuristic,
		},
		PromotionHitRateThreshold: ParameterMetadata[float64]{
			Value:     0.45,
			Rationale: "Hit rate threshold for auto-promotion",
			Source:    SourceHeuristic,
		},
		RejectionSharpeThreshold: ParameterMetadata[float64]{
			Value:     0.0,
			Rationale: "Sharpe threshold for auto-rejection",
			Source:    SourceHeuristic,
		},
		RejectionHitRateThreshold: ParameterMetadata[float64]{
			Value:     0.30,
			Rationale: "Hit rate threshold for auto-rejection",
			Source:    SourceHeuristic,
		},
		SectorRotationMacroAdjustments: ParameterMetadata[map[string]map[string]float64]{
			Value: map[string]map[string]float64{
				"high_risk": {
					"gold":      0.05,
					"cash":      0.10,
					"defensive": 0.08,
					"value":     0.03,
				},
				"moderate_risk": {
					"growth": 0.04,
					"value":  0.03,
					"cash":   -0.03,
				},
				"low_risk": {
					"growth":    0.08,
					"momentum":  0.05,
					"defensive": -0.04,
				},
			},
			Rationale: "Macro risk level → sector allocation adjustments; de-risk to gold/cash/defensive in high risk, rotate to growth in low risk",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: derive from macro regime → sector performance analysis",
		},
		SectorRotationFlowAdjustments: ParameterMetadata[map[string]map[string]float64]{
			Value: map[string]map[string]float64{
				"gold_surge": {
					"gold":      0.10,
					"mining":    0.05,
					"defensive": 0.03,
				},
				"cash_surge": {
					"cash":       0.30,
					"short_term": 0.15,
				},
				"tech_exodus": {
					"semiconductor":   -0.08,
					"ai_supply_chain": -0.05,
				},
				"defensive_flight": {
					"defensive": 0.08,
					"utilities": 0.05,
					"staples":   0.05,
					"growth":    -0.05,
				},
			},
			Rationale: "Capital flow patterns → sector allocation adjustments; gold/cash surges trigger defensive rotation, tech exodus reduces semiconductor exposure, defensive flight shifts to utilities/staples",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: derive from flow pattern → sector return regression analysis",
		},
	}
}

func defaultRiskParameters() RiskParameters {
	return RiskParameters{
		VaRConfidenceLevel: ParameterMetadata[float64]{
			Value:     0.95,
			Rationale: "95% VaR confidence level (primary)",
			Source:    SourceLiterature,
		},
		VaRSecondaryConfidence: ParameterMetadata[float64]{
			Value:     0.99,
			Rationale: "99% VaR confidence level (secondary/tail risk)",
			Source:    SourceLiterature,
		},
		VaRAlertThreshold: ParameterMetadata[float64]{
			Value:     0.02,
			Rationale: "2% portfolio value at risk triggers alert",
			Source:    SourceHeuristic,
		},
		VaRCriticalThreshold: ParameterMetadata[float64]{
			Value:     0.05,
			Rationale: "5% portfolio value at risk is critical",
			Source:    SourceHeuristic,
		},
		ConsecutiveLossLimit: ParameterMetadata[int]{
			Value:     5,
			Rationale: "5 consecutive losses triggers capital phase review",
			Source:    SourceHeuristic,
		},
		SectorConstraintsRiskOff: ParameterMetadata[map[string]float64]{
			Value: map[string]float64{
				"ai_supply_chain": 0.3,
				"small_cap":       0.2,
				"emerging_market": 0.1,
				"gold":            1.5,
				"utilities":       1.2,
			},
			Rationale: "Reduce risk assets, increase defensive assets during risk_off",
			Source:    SourceHeuristic,
		},
		SectorConstraintsCarryTrade: ParameterMetadata[map[string]float64]{
			Value: map[string]float64{
				"all_equities": 0.1,
				"tech":         0.05,
				"financials":   0.1,
				"cash":         2.0,
			},
			Rationale: "Exit equities, move to cash/bonds during carry trade unwind",
			Source:    SourceHeuristic,
		},
		SectorConstraintsSectorRotation: ParameterMetadata[map[string]float64]{
			Value: map[string]float64{
				"energy":              1.8,
				"oil_services":        1.5,
				"high_valuation_tech": 0.3,
				"rate_sensitive":      0.4,
			},
			Rationale: "Rotate to energy, reduce tech during sector rotation",
			Source:    SourceHeuristic,
		},
		MaxDrawdownPct: ParameterMetadata[float64]{
			Value:     0.08,
			Rationale: "8% max portfolio drawdown",
			Source:    SourceHeuristic,
		},
		MaxPositionSize: ParameterMetadata[float64]{
			Value:     0.15,
			Rationale: "15% max single position size",
			Source:    SourceLiterature,
		},
		MaxDailyLossPct: ParameterMetadata[float64]{
			Value:     0.03,
			Rationale: "3% max daily loss",
			Source:    SourceHeuristic,
		},
		StopLoss: ParameterMetadata[float64]{
			Value:     -0.05,
			Rationale: "5% stop loss triggers alert",
			Source:    SourceHeuristic,
		},
		TakeProfit: ParameterMetadata[float64]{
			Value:     0.20,
			Rationale: "20% take profit triggers alert",
			Source:    SourceHeuristic,
		},
		MaxLossPerTrade: ParameterMetadata[float64]{
			Value:     0.02,
			Rationale: "2% max loss per trade for Kelly calculation",
			Source:    SourceLiterature,
		},
		MaxTotalExposure: ParameterMetadata[float64]{
			Value:     0.80,
			Rationale: "80% max total exposure",
			Source:    SourceHeuristic,
		},
	}
}

func defaultBaselineParameters() BaselineParameters {
	return BaselineParameters{
		StartingCash: ParameterMetadata[float64]{
			Value:     3000000,
			Rationale: "3M TWD starting capital for simulation",
			Source:    SourceHeuristic,
		},
		MaxPositionWeight: ParameterMetadata[float64]{
			Value:     0.18,
			Rationale: "18% max position weight for diversification",
			Source:    SourceLiterature,
		},
		MaxOpenPositions: ParameterMetadata[int]{
			Value:     5,
			Rationale: "Maximum 5 open positions for focus",
			Source:    SourceHeuristic,
		},
		MinTradableVolume: ParameterMetadata[float64]{
			Value:     1000000,
			Rationale: "1M TWD minimum daily volume for liquidity",
			Source:    SourceEmpirical,
		},
		MinRecommendationConviction: ParameterMetadata[int]{
			Value:     60,
			Rationale: "Minimum 60 conviction for recommendations to pass CRO",
			Source:    SourceHeuristic,
		},
		RequireCROPass: ParameterMetadata[bool]{
			Value:     true,
			Rationale: "Require CRO approval before execution",
			Source:    SourceHeuristic,
		},
		TransactionCostBPS: ParameterMetadata[float64]{
			Value:     14.25,
			Rationale: "TW stock minimum brokerage fee: 0.1425% = 14.25 bps (tax calculated separately)",
			Source:    SourceEmpirical,
		},
		SlippageBPS: ParameterMetadata[float64]{
			Value:     4.0,
			Rationale: "4 bps estimated slippage for market orders",
			Source:    SourceEmpirical,
		},
		ReserveCashFraction: ParameterMetadata[float64]{
			Value:     0.1,
			Rationale: "10% cash reserve for liquidity and opportunities",
			Source:    SourceLiterature,
		},
	}
}

func defaultRealtimeParameters() RealtimeParameters {
	return RealtimeParameters{
		VolatilityThreshold: ParameterMetadata[float64]{
			Value:     0.02,
			Rationale: "2% daily volatility threshold for regime detection",
			Source:    SourceHeuristic,
		},
		VolumeSpikeThreshold: ParameterMetadata[float64]{
			Value:     2.0,
			Rationale: "2x average volume indicates unusual activity",
			Source:    SourceHeuristic,
		},
		PriceChangeThreshold: ParameterMetadata[float64]{
			Value:     0.01,
			Rationale: "1% price move threshold for signal detection",
			Source:    SourceHeuristic,
		},
		MinConfidence: ParameterMetadata[float64]{
			Value:     0.7,
			Rationale: "70% minimum confidence for real-time signals",
			Source:    SourceHeuristic,
		},
		WeightAdjustmentRate: ParameterMetadata[float64]{
			Value:     0.1,
			Rationale: "10% weight adjustment per signal for stability",
			Source:    SourceHeuristic,
		},
		MaxWeightChange: ParameterMetadata[float64]{
			Value:     0.5,
			Rationale: "Maximum 50% weight change per adjustment",
			Source:    SourceHeuristic,
		},
		MinWeight: ParameterMetadata[float64]{
			Value:     0.1,
			Rationale: "Minimum 10% weight to maintain position",
			Source:    SourceHeuristic,
		},
		UpdateIntervalMs: ParameterMetadata[int]{
			Value:     100,
			Rationale: "100ms update interval for real-time processing",
			Source:    SourceHeuristic,
		},
	}
}

func defaultNarrativeParameters() NarrativeParameters {
	return NarrativeParameters{
		MinTrendStrength: ParameterMetadata[float64]{
			Value:     0.7,
			Rationale: "Minimum trend strength (0-1) to be considered significant for structural override",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from backtest: test [0.5, 0.8] range",
		},
		MinConfidence: ParameterMetadata[float64]{
			Value:     0.75,
			Rationale: "Minimum confidence level for structural trend detection",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from backtest: test [0.6, 0.85] range",
		},
		MinHitRate: ParameterMetadata[float64]{
			Value:     0.70,
			Rationale: "Minimum historical hit rate for structural trend validity",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from backtest: test [0.6, 0.8] range",
		},
		OverrideThreshold: ParameterMetadata[float64]{
			Value:     0.65,
			Rationale: "Score threshold for structural trends to override macro risk signals",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from backtest: test [0.55, 0.75] range",
		},
		AIRevenueGrowthThreshold: ParameterMetadata[float64]{
			Value:     50.0,
			Rationale: "AI revenue YoY growth % threshold to detect AI capex surge (TSMC revenue proxy)",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from historical TSMC revenue data",
		},
		CoWoSUtilizationThreshold: ParameterMetadata[float64]{
			Value:     85.0,
			Rationale: "CoWoS capacity utilization % threshold indicating structural AI demand",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from CoWoS utilization reports",
		},
		CapexGrowthThreshold: ParameterMetadata[float64]{
			Value:     25.0,
			Rationale: "25% YoY capex growth threshold for AI infrastructure expansion (captures early-cycle)",
			Source:    SourceHeuristic,
		},
		US10YChangeBpsThreshold: ParameterMetadata[float64]{
			Value:     10.0,
			Rationale: "US 10Y yield change in basis points to trigger US_rates_up event",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from historical yield volatility distribution",
		},
		DXYChangePctThreshold: ParameterMetadata[float64]{
			Value:     1.5,
			Rationale: "DXY change % threshold to trigger US_rates_up event alongside yield move",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from historical DXY distribution",
		},
		GeopoliticalGPRThreshold: ParameterMetadata[float64]{
			Value:     150.0,
			Rationale: "Geopolitical risk index (GPR) threshold to trigger geopolitical_risk_spike event",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from Caldara-Iacoviello GPR index historical data",
		},
		OilChangePctThreshold: ParameterMetadata[float64]{
			Value:     5.0,
			Rationale: "Oil price change % threshold (absolute) to trigger oil_price_shock event",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from historical oil price volatility",
		},
		JPYChangePctThreshold: ParameterMetadata[float64]{
			Value:     2.0,
			Rationale: "JPY change % threshold to trigger JPY_carry_unwind event",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from historical JPY volatility around BOJ policy shifts",
		},
		VIXLevelThreshold: ParameterMetadata[float64]{
			Value:     25.0,
			Rationale: "VIX level threshold to trigger JPY_carry_unwind and geopolitical events",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from historical VIX distribution around crisis periods",
		},
		TaiwanStressDXYWeight: ParameterMetadata[float64]{
			Value:     0.13,
			Rationale: "DXY component weight in Taiwan stress index (13%)",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from regression: DXY vs foreign flow correlation",
		},
		TaiwanStressUS10YWeight: ParameterMetadata[float64]{
			Value:     0.18,
			Rationale: "US 10Y yield component weight in Taiwan stress index (18%)",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from regression: US10Y vs foreign flow correlation",
		},
		TaiwanStressForeignWeight: ParameterMetadata[float64]{
			Value:     0.22,
			Rationale: "Foreign investor net flow component weight in Taiwan stress index (22%)",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from regression: foreign flow vs TAIEX returns",
		},
		TaiwanStressVIXWeight: ParameterMetadata[float64]{
			Value:     0.13,
			Rationale: "VIX component weight in Taiwan stress index (13%)",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from regression: VIX vs foreign flow correlation",
		},
		TaiwanStressJPYWeight: ParameterMetadata[float64]{
			Value:     0.08,
			Rationale: "JPY component weight in Taiwan stress index (8%)",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from regression: JPY vs carry trade flow",
		},
		TaiwanStressGeoWeight: ParameterMetadata[float64]{
			Value:     0.13,
			Rationale: "Geopolitical risk component weight in Taiwan stress index (13%)",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from regression: GPR vs TAIEX returns",
		},
		TaiwanStressOilWeight: ParameterMetadata[float64]{
			Value:     0.07,
			Rationale: "Oil price component weight in Taiwan stress index (7%)",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from regression: oil price vs foreign flow correlation",
		},
		TaiwanStressGoldWeight: ParameterMetadata[float64]{
			Value:     0.06,
			Rationale: "Gold price component weight in Taiwan stress index (6%)",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from regression: gold price vs foreign flow correlation",
		},
		TaiwanStressDXYScale: ParameterMetadata[float64]{
			Value:     5.0,
			Rationale: "DXY change (%) scaling: 1% change = 5 stress points",
			Source:    SourceHeuristic,
		},
		TaiwanStressUS10YScale: ParameterMetadata[float64]{
			Value:     2.0,
			Rationale: "US10Y yield scaling: 1% yield = 2 stress points",
			Source:    SourceHeuristic,
		},
		TaiwanStressForeignScale: ParameterMetadata[float64]{
			Value:     10.0,
			Rationale: "Foreign flow scaling: 1B TWD outflow = 10 stress points",
			Source:    SourceHeuristic,
		},
		TaiwanStressVIXScale: ParameterMetadata[float64]{
			Value:     2.5,
			Rationale: "VIX scaling: VIX=40 maps to 100 stress points",
			Source:    SourceHeuristic,
		},
		TaiwanStressJPYScale: ParameterMetadata[float64]{
			Value:     10.0,
			Rationale: "JPY change (%) scaling: 1% change = 10 stress points",
			Source:    SourceHeuristic,
		},
		TaiwanStressGeoScale: ParameterMetadata[float64]{
			Value:     1.0,
			Rationale: "Geopolitical score scaling: direct pass-through (already 0-100)",
			Source:    SourceHeuristic,
		},
		TaiwanStressOilScale: ParameterMetadata[float64]{
			Value:     2.0,
			Rationale: "Oil change (%) scaling: 1% change = 2 stress points",
			Source:    SourceHeuristic,
		},
		TaiwanStressGoldScale: ParameterMetadata[float64]{
			Value:     2.0,
			Rationale: "Gold change (%) scaling: 1% change = 2 stress points",
			Source:    SourceHeuristic,
		},
		TaiwanStressCrisisThreshold: ParameterMetadata[float64]{
			Value:     70.0,
			Rationale: "Crisis regime threshold (red alert)",
			Source:    SourceHeuristic,
		},
		TaiwanStressHighThreshold: ParameterMetadata[float64]{
			Value:     50.0,
			Rationale: "High stress regime threshold (orange alert)",
			Source:    SourceHeuristic,
		},
		TaiwanStressAlertThreshold: ParameterMetadata[float64]{
			Value:     30.0,
			Rationale: "Alert regime threshold (yellow alert)",
			Source:    SourceHeuristic,
		},
		EventTTLMultiplier: ParameterMetadata[map[string]float64]{
			Value: map[string]float64{
				"AI_capex_surge":          90,
				"US_rates_up":             7,
				"JPY_carry_unwind":        14,
				"geopolitical_risk_spike": 30,
				"oil_price_shock":         15,
				"Fed_emergency_cut":       3,
				"earnings_surprise":       10,
			},
			Rationale: "Event TTL in days per theme: AI capex 90d (structural), rates 7d (transient), carry unwind 14d, geopolitical 30d, oil 15d, Fed emergency 3d, earnings 10d",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from historical event impact decay analysis",
		},
		ModelLookbackDays: ParameterMetadata[int]{
			Value:     10,
			Rationale: "Lookback window in trading days for model evaluation",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from backtest: test [5, 20] range",
		},
		ModelHoldWindowDays: ParameterMetadata[int]{
			Value:     5,
			Rationale: "Forward hold window in trading days for model evaluation",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from backtest: test [3, 10] range",
		},
		GoldChangePctThreshold: ParameterMetadata[float64]{
			Value:     2.0,
			Rationale: "Gold price change %% threshold for geopolitical risk detection; gold traditionally serves as safe-haven proxy",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from historical gold volatility around geopolitical events",
		},
		USDTWDChangePctThreshold: ParameterMetadata[float64]{
			Value:     1.0,
			Rationale: "USD/TWD daily change %% threshold for FX volatility event; 1%% daily move is significant for managed-float TWD",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from historical USD/TWD daily volatility distribution",
		},
		SemiconductorExportDropThreshold: ParameterMetadata[float64]{
			Value:     -5.0,
			Rationale: "Semiconductor export change %% threshold for downturn detection; -5%% MoM decline signals demand weakening",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from historical Taiwan electronics export cycles",
		},
		RetailMarginZScoreThreshold: ParameterMetadata[float64]{
			Value:     1.5,
			Rationale: "Margin balance z-score threshold for retail divergence event; z>1.5 indicates extreme positioning",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from historical margin balance distribution",
		},
		AICapexSentimentThreshold: ParameterMetadata[float64]{
			Value:     0.5,
			Rationale: "AI capex sentiment score threshold for AI_capex_surge event; >0.5 indicates bullish capex outlook",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from TSMC capex guidance vs forward returns",
		},
		TSMCRevenueYoYThreshold: ParameterMetadata[float64]{
			Value:     10.0,
			Rationale: "TSMC revenue YoY growth %% threshold for AI sentiment computation; >10%% indicates strong AI demand",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from TSMC quarterly revenue cycles",
		},
		TaiwanStressUSDTWDThreshold: ParameterMetadata[float64]{
			Value:     1.0,
			Rationale: "USD/TWD change %% threshold for Taiwan stress signal in geopolitical detection; 1%% depreciation signals capital outflow",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from historical TWD depreciation episodes",
		},
		RetailInstitutionalDivergenceThreshold: ParameterMetadata[float64]{
			Value:     0.0,
			Rationale: "Retail-institutional divergence threshold; divergence > 0 means retail is more bullish than institutions",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from divergence index backtest",
		},
		AICapexNegativeSentimentThreshold: ParameterMetadata[float64]{
			Value:     -0.3,
			Rationale: "AI capex sentiment threshold for semiconductor downturn detection; <-0.3 indicates bearish outlook",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from historical sentiment floor levels",
		},
		AICapexFallbackSentiment: ParameterMetadata[float64]{
			Value:     -0.3,
			Rationale: "Fallback AI capex sentiment when TSMC revenue YoY is not positive; conservative bearish default",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from historical TSMC revenue-to-sentiment mapping",
		},
		TSMCRevenuePositiveThreshold: ParameterMetadata[float64]{
			Value:     0.0,
			Rationale: "TSMC revenue YoY threshold for positive growth; >0%% triggers moderate bullish sentiment",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from TSMC revenue growth distribution",
		},
		ConfidenceBaseUSRates: ParameterMetadata[float64]{
			Value:     0.75,
			Rationale: "Base confidence for US rates event; derived from historical predictability of yield-driven episodes",
			Source:    SourceEmpirical,
			Todo:      "Recalibrate when significant Fed regime change occurs",
		},
		ConfidenceBaseJPYCarry: ParameterMetadata[float64]{
			Value:     0.65,
			Rationale: "Base confidence for JPY carry unwind event; derived from carry trade unwinding historical patterns",
			Source:    SourceEmpirical,
			Todo:      "Recalibrate after major BOJ policy shifts",
		},
		ConfidenceBaseGeopolitical: ParameterMetadata[float64]{
			Value:     0.65,
			Rationale: "Base confidence for geopolitical risk event; derived from GPR index historical correlations",
			Source:    SourceEmpirical,
			Todo:      "Recalibrate when GPR index methodology changes",
		},
		ConfidenceBaseOilShock: ParameterMetadata[float64]{
			Value:     0.60,
			Rationale: "Base confidence for oil price shock event; derived from Hamilton-type oil shock literature",
			Source:    SourceEmpirical,
			Todo:      "Recalibrate with structural oil market changes",
		},
		ConfidenceBaseAICapex: ParameterMetadata[float64]{
			Value:     0.70,
			Rationale: "Base confidence for AI capex event; derived from TSMC capex guidance vs semiconductor cycle correlation",
			Source:    SourceEmpirical,
			Todo:      "Recalibrate as AI capex cycle data accumulates",
		},
		ConfidenceBaseTSMCRevenue: ParameterMetadata[float64]{
			Value:     0.70,
			Rationale: "Base confidence for TSMC revenue-based events; derived from WSTS semiconductor forecast accuracy",
			Source:    SourceEmpirical,
			Todo:      "Recalibrate as TSMC revenue data accumulates",
		},
		ConfidenceBaseTaiwanStress: ParameterMetadata[float64]{
			Value:     0.60,
			Rationale: "Base confidence for Taiwan stress index-based events; conservative heuristic default",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from historical Taiwan stress index distribution",
		},
		SOXIndexDropThreshold: ParameterMetadata[float64]{
			Value:     -5.0,
			Rationale: "SOX index change %% threshold for semiconductor stress detection; -5%% is significant drawdown",
			Source:    SourceEmpirical,
			Todo:      "Calibrate from historical SOX index volatility",
		},
	}
}

func defaultJanusParameters() JanusParameters {
	return JanusParameters{
		ShortWindowDays: ParameterMetadata[int]{
			Value:     5,
			Rationale: "Short lookback window (~1 trading week) for cohort performance",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [3, 10] range for short-term sensitivity",
		},
		MediumWindowDays: ParameterMetadata[int]{
			Value:     20,
			Rationale: "Medium lookback window (~1 trading month) for cohort performance",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [15, 30] range for medium-term stability",
		},
		LongWindowDays: ParameterMetadata[int]{
			Value:     60,
			Rationale: "Long lookback window (~1 trading quarter) for cohort performance",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [40, 90] range for long-term memory",
		},
		MaxHistoryDays: ParameterMetadata[int]{
			Value:     90,
			Rationale: "Maximum rolling history per cohort; must cover longest window comfortably",
			Source:    SourceHeuristic,
		},
		MinWeight: ParameterMetadata[float64]{
			Value:     0.05,
			Rationale: "Floor for any cohort weight; prevents total elimination",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.01, 0.10] range",
		},
		MaxWeight: ParameterMetadata[float64]{
			Value:     0.60,
			Rationale: "Ceiling for any cohort weight; prevents single-cohort dominance",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.40, 0.80] range",
		},
		NovelThreshold: ParameterMetadata[float64]{
			Value:     0.15,
			Rationale: "Weight delta threshold for NOVEL_REGIME detection; short-window winner surges past long-window standing",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.10, 0.25] range",
		},
		HistoricalThreshold: ParameterMetadata[float64]{
			Value:     0.15,
			Rationale: "Weight delta threshold for HISTORICAL_REGIME detection; long-window winner maintains stable lead",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.10, 0.25] range",
		},
		EpsilonWeight: ParameterMetadata[float64]{
			Value:     0.02,
			Rationale: "Minimal weight assigned to negative-Sharpe cohorts when others are positive",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.01, 0.05] range",
		},
		ShortWindowBlend: ParameterMetadata[float64]{
			Value:     0.5,
			Rationale: "50% weight on short-window Sharpe in blended score; emphasizes recent accuracy",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.3, 0.6] range; must sum with medium+long to 1.0",
		},
		MediumWindowBlend: ParameterMetadata[float64]{
			Value:     0.3,
			Rationale: "30% weight on medium-window Sharpe in blended score",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.2, 0.4] range; must sum with short+long to 1.0",
		},
		LongWindowBlend: ParameterMetadata[float64]{
			Value:     0.2,
			Rationale: "20% weight on long-window Sharpe in blended score; retains long-term memory",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.1, 0.3] range; must sum with short+medium to 1.0",
		},
		HealthStaleHours: ParameterMetadata[int]{
			Value:     168,
			Rationale: "Hours before JANUS data is considered stale (7 days); triggers error-level health alert",
			Source:    SourceHeuristic,
		},
		HealthWarnHours: ParameterMetadata[int]{
			Value:     48,
			Rationale: "Hours before JANUS data triggers warning-level health alert (2 days)",
			Source:    SourceHeuristic,
		},
	}
}

func defaultMarketdataParameters() MarketdataParameters {
	return MarketdataParameters{
		TWSEAPIRateLimit: ParameterMetadata[float64]{
			Value:     0.6,
			Rationale: "TWSE OpenAPI rate limit: 3 requests per 5 seconds = 0.6 req/s",
			Source:    SourceHeuristic,
			Todo:      "Verify: check TWSE documentation for current limits",
		},
		TWSEAPIRateBurst: ParameterMetadata[int]{
			Value:     3,
			Rationale: "TWSE OpenAPI burst limit: 3 requests per 5-second window",
			Source:    SourceHeuristic,
			Todo:      "Verify: check TWSE documentation for current burst limits",
		},
		TWSEAPITimeoutSec: ParameterMetadata[int]{
			Value:     15,
			Rationale: "HTTP timeout for TWSE API calls; balances responsiveness vs slow responses",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [10, 30] range based on observed latency distribution",
		},
		FubonIntradayLimit: ParameterMetadata[int]{
			Value:     30,
			Rationale: "Fubon intraday API burst limit; prevents overwhelming the proxy",
			Source:    SourceHeuristic,
			Todo:      "Verify: check Fubon DMA documentation for actual limits",
		},
		FubonHistoricalLimit: ParameterMetadata[int]{
			Value:     60,
			Rationale: "Fubon historical data API rate limit; conservative to avoid throttling",
			Source:    SourceHeuristic,
			Todo:      "Verify: check Fubon DMA documentation for actual limits",
		},
		FubonAPITimeoutSec: ParameterMetadata[int]{
			Value:     10,
			Rationale: "HTTP timeout for Fubon API calls; proxy adds latency, keep short",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [5, 15] range based on proxy latency",
		},
		TEJCallsPerSecond: ParameterMetadata[int]{
			Value:     5,
			Rationale: "TEJ free tier rate limit: 500 calls/day, burst at 5 calls/second",
			Source:    SourceHeuristic,
			Todo:      "Verify: check TEJ subscription tier for actual limits",
		},
		TEJAPITimeoutSec: ParameterMetadata[int]{
			Value:     30,
			Rationale: "HTTP timeout for TEJ API; historical queries can be slow",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [20, 45] range",
		},
		FugleRateLimit: ParameterMetadata[int]{
			Value:     60,
			Rationale: "Fugle API free tier: 60 req/min. Developer: 600/min. Advanced: 2000/min",
			Source:    SourceEmpirical,
			Todo:      "Set FUGLE_TIER env var if using paid plan",
		},
		FugleAPITimeoutSec: ParameterMetadata[int]{
			Value:     10,
			Rationale: "HTTP timeout for Fugle API; premium service, expect fast responses",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [5, 15] range",
		},
		MaxRetryAttempts: ParameterMetadata[int]{
			Value:     3,
			Rationale: "Maximum retry attempts for transient failures; exponential backoff",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [2, 5] range based on failure rate analysis",
		},
		RetryBackoffMs: ParameterMetadata[int]{
			Value:     1000,
			Rationale: "Base backoff between retries in milliseconds; doubles each attempt",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [500, 2000] range",
		},
	}
}

func defaultIndustryParameters() IndustryParameters {
	return IndustryParameters{
		SectorWeights: ParameterMetadata[map[string]float64]{
			Value: map[string]float64{
				"semiconductor":    0.45,
				"ai_supply_chain":  0.15,
				"electronics":      0.10,
				"financials":       0.12,
				"shipping":         0.05,
				"biotech":          0.03,
				"traditional":      0.05,
				"renewable_energy": 0.03,
				"other":            0.02,
			},
			Rationale: "Taiwan market sector weights aligned with TAIEX composition (2024); semiconductor ~45% vs previous 25%",
			Source:    SourceEmpirical,
			Todo:      "Recalibrate: update quarterly from TAIEX sector breakdown",
		},
		CycleThresholds: ParameterMetadata[map[string]CycleThresholdConfig]{
			Value: map[string]CycleThresholdConfig{
				"semiconductor": {
					ExpansionRevenuePct: 0.20,
					ExpansionProfitPct:  0.25,
					RecoveryRevenuePct:  0.10,
					RecoveryProfitPct:   0.15,
					MatureRevenuePct:    0.05,
					MatureProfitPct:     0.08,
				},
				"financials": {
					ExpansionRevenuePct: 0.10,
					ExpansionProfitPct:  0.15,
					RecoveryRevenuePct:  0.05,
					RecoveryProfitPct:   0.08,
					MatureRevenuePct:    0.02,
					MatureProfitPct:     0.05,
				},
				"shipping": {
					ExpansionRevenuePct: 0.30,
					ExpansionProfitPct:  0.35,
					RecoveryRevenuePct:  0.15,
					RecoveryProfitPct:   0.20,
					MatureRevenuePct:    0.05,
					MatureProfitPct:     0.10,
				},
			},
			Rationale: "Per-industry business cycle thresholds; semiconductor high-growth, financials stable, shipping cyclical",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: derive from historical revenue/ profit CAGR per sector",
		},
		InventoryCycleThresholds: ParameterMetadata[InventoryCycleThresholdConfig]{
			Value: InventoryCycleThresholdConfig{
				ActiveRestockingInventoryMin:  6.0,
				ActiveRestockingCapacityMin:   0.80,
				PassiveRestockingInventoryMin: 4.0,
				PassiveRestockingCapacityMin:  0.70,
				ActiveDestockingInventoryMax:  3.0,
				ActiveDestockingCapacityMax:   0.60,
			},
			Rationale: "Inventory cycle thresholds derived from TW semiconductor and electronics supply chain behavior",
			Source:    SourceHeuristic,
		},
		CapexCycleThresholds: ParameterMetadata[CapexCycleThresholdConfig]{
			Value: CapexCycleThresholdConfig{
				ExpansionCapacityMin:   0.85,
				ExpansionRevenueMin:    0.15,
				MaintenanceCapacityMin: 0.70,
				MaintenanceRevenueMin:  0.05,
			},
			Rationale: "Capex cycle thresholds: expansion at >85% utilization + >15% revenue growth; maintenance at >70% + >5%",
			Source:    SourceHeuristic,
		},
		ConcentrationRiskEnabled: ParameterMetadata[bool]{
			Value:     true,
			Rationale: "Enable customer concentration risk scoring for supply chain analysis",
			Source:    SourceHeuristic,
		},
		NewsLatencyRiskEnabled: ParameterMetadata[bool]{
			Value:     true,
			Rationale: "Enable news latency risk scoring for event-driven positions",
			Source:    SourceHeuristic,
		},
		AsymmetricRiskEnabled: ParameterMetadata[bool]{
			Value:     true,
			Rationale: "Enable asymmetric downside risk scoring (tail risk analysis)",
			Source:    SourceHeuristic,
		},
		CustomerConcentrationLimit: ParameterMetadata[float64]{
			Value:     0.40,
			Rationale: "Flag customer concentration >40% as high risk; TSMC Apple exposure ~25% is acceptable",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.30, 0.50] range; consider industry norms",
		},
		GeographicExposureLimit: ParameterMetadata[float64]{
			Value:     0.60,
			Rationale: "Flag geographic revenue concentration >60% as high risk; China exposure threshold",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.50, 0.70] range based on geopolitical risk assessment",
		},
		CustomerShareThreshold1: ParameterMetadata[float64]{
			Value:     30.0,
			Rationale: "First tier customer concentration threshold (30%); triggers initial risk score",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [20, 40] range",
		},
		CustomerShareThreshold2: ParameterMetadata[float64]{
			Value:     50.0,
			Rationale: "Second tier customer concentration threshold (50%); triggers higher risk score",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [40, 60] range",
		},
		USExposureThreshold1: ParameterMetadata[float64]{
			Value:     50.0,
			Rationale: "US geographic exposure threshold (50%); triggers initial risk score",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [40, 60] range",
		},
		USExposureThreshold2: ParameterMetadata[float64]{
			Value:     70.0,
			Rationale: "High US geographic exposure threshold (70%); triggers higher risk score",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [60, 80] range",
		},
		RiskScoreWeight1: ParameterMetadata[float64]{
			Value:     0.4,
			Rationale: "Risk score weight for first tier customer concentration",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.2, 0.5] range",
		},
		RiskScoreWeight2: ParameterMetadata[float64]{
			Value:     0.3,
			Rationale: "Risk score weight for second tier customer concentration",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.2, 0.4] range",
		},
		RiskScoreWeight3: ParameterMetadata[float64]{
			Value:     0.2,
			Rationale: "Risk score weight for first tier US exposure",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.1, 0.3] range",
		},
		RiskScoreWeight4: ParameterMetadata[float64]{
			Value:     0.1,
			Rationale: "Risk score weight for second tier US exposure",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.05, 0.2] range",
		},
		SeverityThresholdMedium: ParameterMetadata[float64]{
			Value:     0.4,
			Rationale: "Risk score threshold for medium severity",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.3, 0.5] range",
		},
		SeverityThresholdHigh: ParameterMetadata[float64]{
			Value:     0.6,
			Rationale: "Risk score threshold for high severity",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.5, 0.7] range",
		},
		SeverityThresholdCritical: ParameterMetadata[float64]{
			Value:     0.8,
			Rationale: "Risk score threshold for critical severity",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.7, 0.9] range",
		},
		ImpactMultiplier: ParameterMetadata[float64]{
			Value:     0.15,
			Rationale: "Multiplier for estimated price impact from risk score",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.10, 0.25] range",
		},
		RiskConfidence: ParameterMetadata[float64]{
			Value:     0.85,
			Rationale: "Confidence level for risk assessment",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.70, 0.95] range",
		},
		ConfidenceSignal: ParameterMetadata[ConfidenceSignalConfig]{
			Value: ConfidenceSignalConfig{
				SignalBase:               0.30,
				RevenueNormDenom:         0.50,
				RevenueWeight:            0.25,
				ProfitNormDenom:          0.50,
				ProfitWeight:             0.25,
				InventoryNormDenom:       10.0,
				InventoryWeight:          0.10,
				UtilizationWeight:        0.15,
				SignalBoundaryMix:        0.60,
				BoundaryDenomFactor:      0.30,
				ConfidenceFloor:          0.10,
				ConfidenceCeiling:        1.00,
				RevenueTrendThreshold:    0.10,
				RevenueIndicatorWeight:   0.30,
				InventoryTrendThreshold:  4.0,
				InventoryIndicatorWeight: 0.25,
				ProfitTrendThreshold:     0.10,
				ProfitIndicatorWeight:    0.35,
				CapacityTrendThreshold:   0.75,
				CapacityIndicatorWeight:  0.30,
				TrendUpMultiplier:        1.1,
				TrendDownMultiplier:      0.9,
				ThresholdRangeFallback:   0.25,
			},
			Rationale: "Signal weights derived from relative informativeness of each metric; boundary denom based on threshold spread; mix ratio balances data amplitude vs phase clarity",
			Source:    SourceHeuristic,
		},
		ConfidenceMix: ParameterMetadata[ConfidenceMixConfig]{
			Value: ConfidenceMixConfig{
				WeightBoundary:         0.45,
				WeightFreshness:        0.20,
				WeightSeasonal:         0.15,
				WeightLinkage:          0.10,
				WeightNarrative:        0.1,
				FavorableConfidenceMin: 0.4,
			},
			Rationale: "Confidence mix: boundary(45%) + freshness(20%) + seasonal(15%) + linkage(10%) + narrative(10%)",
			Source:    SourceHeuristic,
		},
		SeasonalPatterns: ParameterMetadata[[]SeasonalPatternConfig]{
			Value: []SeasonalPatternConfig{
				{
					ID: "spring_festival", Name: "春節行情", NameEN: "Spring Festival Rally",
					StartMonth: 1, StartDay: 15, EndMonth: 2, EndDay: 15,
					FavoredIndustries: []string{"financials"}, AvoidedIndustries: []string{"semiconductor", "ai_supply_chain"},
					StyleTags:        []string{"high_dividend", "small_cap"},
					AdjustmentFactor: 1.15, HistoricalAccuracy: 0.70, AvgMarketReturn: 0.032,
					Description: "年前資金回籠，高股息與金融股受追捧；電子股進入淡季",
				},
				{
					ID: "earnings_window", Name: "季報空窗期", NameEN: "Earnings Report Window",
					StartMonth: 3, StartDay: 1, EndMonth: 4, EndDay: 15,
					FavoredIndustries: []string{"ai_supply_chain", "electronics"}, AvoidedIndustries: []string{"consumer", "industrial"},
					AdjustmentFactor: 1.10, HistoricalAccuracy: 0.55, AvgMarketReturn: 0.015,
					Description: "季報空窗期，成長股與AI供應鏈有表現空間",
				},
				{
					ID: "dividend_season", Name: "除權除息季", NameEN: "Dividend Season",
					StartMonth: 5, StartDay: 1, EndMonth: 6, EndDay: 30,
					FavoredIndustries: []string{"financials", "consumer"}, AvoidedIndustries: []string{"semiconductor", "ai_supply_chain", "electronics"},
					StyleTags:        []string{"high_dividend"},
					AdjustmentFactor: 1.20, HistoricalAccuracy: 0.65, AvgMarketReturn: 0.025,
					Description: "除權除息旺季，高股息股與金融股表現較佳；科技股相對弱勢",
				},
				{
					ID: "tech_peak_season", Name: "科技旺季", NameEN: "Technology Peak Season",
					StartMonth: 7, StartDay: 1, EndMonth: 9, EndDay: 15,
					FavoredIndustries: []string{"semiconductor", "ai_supply_chain", "electronics"}, AvoidedIndustries: []string{"consumer"},
					AdjustmentFactor: 1.25, HistoricalAccuracy: 0.75, AvgMarketReturn: 0.085,
					Description: "蘋果新機拉貨、AI晶片需求高峰，科技股表現最強",
				},
				{
					ID: "earnings_verification", Name: "季報驗證期", NameEN: "Earnings Verification",
					StartMonth: 9, StartDay: 15, EndMonth: 10, EndDay: 31,
					AdjustmentFactor: 1.10, HistoricalAccuracy: 0.60, AvgMarketReturn: 0.020,
					Description: "季報公布，獲利優於預期股受追捧，低於預期股遭拋售",
				},
				{
					ID: "year_end_rally", Name: "年底作帳", NameEN: "Year-End Window Dressing",
					StartMonth: 11, StartDay: 1, EndMonth: 12, EndDay: 31,
					FavoredIndustries: []string{"financials"}, StyleTags: []string{"large_cap", "index_heavyweights"},
					AdjustmentFactor: 1.12, HistoricalAccuracy: 0.58, AvgMarketReturn: 0.018,
					Description: "法人年底作帳，大型權值股與金融股相對強勢",
				},
				{
					ID: "summer_electricity", Name: "夏季用電高峰", NameEN: "Summer Electricity Peak",
					StartMonth: 6, StartDay: 1, EndMonth: 8, EndDay: 31,
					FavoredIndustries: []string{"energy"}, AvoidedIndustries: []string{"industrial"},
					AdjustmentFactor: 1.08, HistoricalAccuracy: 0.62, AvgMarketReturn: 0.012,
					Description: "夏季用電高峰，能源相對強勢；高耗電製造業成本上升",
				},
			},
			Rationale: "7 canonical Taiwan seasonal patterns from DefaultSeasonalPatterns(); values read from config by NewSeasonalEngine()",
			Source:    SourceHeuristic,
		},
		AsymmetricRisk: ParameterMetadata[AsymmetricRiskConfig]{
			Value: AsymmetricRiskConfig{
				BadNewsThreshold:      -0.03,
				GoodNewsThreshold:     0.05,
				ReactionTimeMinutes:   30,
				VolumeSpikeMultiplier: 2.0,
			},
			Rationale: "3% drop triggers bad news detection; 5% rise for good news; 30 min reaction window; 2x volume confirms",
			Source:    SourceHeuristic,
		},
		NewsLatencyRisk: ParameterMetadata[NewsLatencyConfig]{
			Value: NewsLatencyConfig{
				MaxLatencyHours:       24.0,
				SeverityCriticalMin:   0.8,
				SeverityHighMin:       0.5,
				ImpactMultiplier:      0.05,
				DropCriticalThreshold: 0.10,
				DropHighThreshold:     0.07,
				DropMediumThreshold:   0.05,
				ConfidenceDivisor:     3.0,
			},
			Rationale: "24h max latency gap; severity bands at 0.5/0.8; drop thresholds at 5%/7%/10%",
			Source:    SourceHeuristic,
		},
		FreshnessScores: ParameterMetadata[FreshnessScoresConfig]{
			Value: FreshnessScoresConfig{
				ScoreLive: 1.0, ScoreRecent: 0.8, ScoreStale: 0.4,
				ScoreFallback: 0.2, ScoreDefault: 0.3,
			},
			Rationale: "Maps DataFreshness enum to confidence weights",
			Source:    SourceHeuristic,
		},
		PhaseScores: ParameterMetadata[PhaseScoresConfig]{
			Value: PhaseScoresConfig{
				ScoreExpansion: 1.2, ScoreRecovery: 1.1, ScoreMature: 1.0, ScoreRecession: 0.8,
			},
			Rationale: "Cycle phase conviction multipliers; expansion (1.2x) and recovery (1.1x) amplify, recession (0.8x) dampens",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: derive phase scores from sector-level regime performance",
		},
		SkillToIndustry: ParameterMetadata[map[string]string]{
			Value: map[string]string{
				"semiconductor_desk": "semiconductor",
				"ai_infrastructure":  "ai_supply_chain",
				"macro_research":     "financials",
				"shipping_analyst":   "shipping",
				"energy_analyst":     "renewable_energy",
				"defense_industrial": "ai_supply_chain",
				"consumer_analyst":   "traditional",
			},
			Rationale: "Maps agent skills to industry sectors for cycle-aware conviction adjustments",
			Source:    SourceHeuristic,
			Todo:      "Validate: verify mapping aligns with sector coverage mandates",
		},
		CycleTransitions: ParameterMetadata[[]CycleTransitionConfig]{
			Value: []CycleTransitionConfig{
				{FromPhase: "recession", ToPhase: "recovery", Triggers: []string{"inventory_depletion", "demand_stabilization"}, Probability: 0.70, TypicalDurationDays: 180},
				{FromPhase: "recovery", ToPhase: "expansion", Triggers: []string{"revenue_acceleration", "capex_increase"}, Probability: 0.80, TypicalDurationDays: 270},
				{FromPhase: "expansion", ToPhase: "mature", Triggers: []string{"growth_deceleration", "margin_compression"}, Probability: 0.60, TypicalDurationDays: 360},
				{FromPhase: "mature", ToPhase: "recession", Triggers: []string{"demand_destruction", "overcapacity"}, Probability: 0.50, TypicalDurationDays: 180},
			},
			Rationale: "TW cycle transition probabilities and durations",
			Source:    SourceHeuristic,
		},
		CycleWeightMultipliers: ParameterMetadata[CycleWeightMultipliersConfig]{
			Value: CycleWeightMultipliersConfig{
				ExpansionMultiplier: 1.2,
				RecoveryMultiplier:  1.1,
				MatureMultiplier:    1.0,
				RecessionMultiplier: 0.7,
			},
			Rationale: "Cycle phase weight multipliers: expansion +20%, recovery +10%, mature neutral, recession -30%",
			Source:    SourceHeuristic,
		},
		LinkageWeightImpact: ParameterMetadata[float64]{
			Value:     0.2,
			Rationale: "Linkage systemic importance deviation scaling: ±20% max impact",
			Source:    SourceHeuristic,
		},
		WeightFloor: ParameterMetadata[float64]{
			Value:     0.03,
			Rationale: "Minimum 3% weight per industry after normalization",
			Source:    SourceHeuristic,
		},
		MaxDailyWeightChange: ParameterMetadata[float64]{
			Value:     0.05,
			Rationale: "Maximum 5% daily weight change to prevent excessive volatility",
			Source:    SourceHeuristic,
		},
		LinkageParams: ParameterMetadata[LinkageConfig]{
			Value: LinkageConfig{
				DownstreamDecayFactor:     0.80,
				UpstreamDecayFactor:       0.60,
				SeasonalDecayFactor:       0.30,
				DefaultCorrelation:        0.50,
				SystemicImportanceDivisor: 10.0,
				MinCorrelationThreshold:   0.30,
				CorrelationWindowDays:     30,
				RecessionCorrelationBoost: 0.30,
			},
			Rationale: "Downstream decay (0.80) > upstream (0.60); seasonal decay (0.30) for supply-chain propagation; default correlation (0.50); window 30 days; narrative-aware via SeasonalBridge.CorrelationMultiplier() for dynamic macro event modulation",
			Source:    SourceHeuristic,
		},
		DynamicEnv: ParameterMetadata[DynamicEnvConfig]{
			Value: DynamicEnvConfig{
				OilHighThreshold:     0.10,
				OilLowThreshold:      0.10,
				OilEnergyMult:        0.50,
				OilShippingPenalty:   0.05,
				OilShippingBenefit:   0.05,
				OilIndustrialPenalty: 0.06,
				OilIndustrialBenefit: 0.04,
				BDIHighThreshold:     0.10,
				BDILowThreshold:      0.10,
				BDIShippingBoost:     0.30,
				BDICostPenalty:       0.04,
				DXYHighThreshold:     0.05,
				DXYLowThreshold:      0.03,
				DXYExportPenalty:     0.05,
				DXYExportBenefit:     0.04,
			},
			Rationale: "Dynamic env modulation thresholds and multipliers: oil >10% triggers energy/shipping/industrial adjustments; BDI >10% amplifies shipping; DXY >5% penalizes exporters; values preserve original heuristic tuning from dynamic_env.go",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: backtest each multiplier against historical sector returns during macro regime shifts",
		},
		HistoryRetentionDays: ParameterMetadata[int]{
			Value:     90,
			Rationale: "Keep 90 days of cycle history for trend analysis",
			Source:    SourceHeuristic,
		},
	}
}

func defaultStrategyParameters() StrategyParameters {
	return StrategyParameters{
		MinSwitchIntervalDays: ParameterMetadata[int]{
			Value:     7,
			Rationale: "Minimum days between strategy switches; prevents whipsaw",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [3, 14] range based on transaction costs",
		},
		SwitchThreshold: ParameterMetadata[float64]{
			Value:     0.10,
			Rationale: "Minimum score advantage (10%) required to switch strategies",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.05, 0.20] range",
		},
		ScoreLookbackDays: ParameterMetadata[int]{
			Value:     30,
			Rationale: "Days of historical performance to evaluate when scoring strategies",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [14, 60] range",
		},
		FallbackStrategy: ParameterMetadata[string]{
			Value:     "momentum",
			Rationale: "Default strategy when no clear winner; momentum has broad applicability",
			Source:    SourceHeuristic,
			Todo:      "Validate: backtest fallback strategy vs alternatives",
		},
	}
}

func defaultFactorWeightParameters() FactorWeightParameters {
	return FactorWeightParameters{
		BaseWeights: ParameterMetadata[map[string]float64]{
			Value: map[string]float64{
				"momentum":       0.25,
				"value":          0.20,
				"quality":        0.20,
				"agent":          0.15,
				"inst_sent":      0.10,
				"liquidity":      0.05,
				"narrative":      0.05,
				"industry_cycle": 0.00,
			},
			Rationale: "Eight-factor weight distribution; momentum, value, quality as primary factors (65% combined), agent/inst_sent as secondary, liquidity/narrative as tertiary, industry_cycle as placeholder",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: derive from factor attribution backtest across 2024-2026 regime cycles",
		},
		RegimeBullMomentum: ParameterMetadata[float64]{
			Value:     0.05,
			Rationale: "Bull regime: shift toward momentum (+5%) to capture trend continuation",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: optimize regime delta range via walk-forward backtest",
		},
		RegimeBullQuality: ParameterMetadata[float64]{
			Value:     -0.03,
			Rationale: "Bull regime: reduce quality (-3%) — defensive quality underperforms in strong rallies",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: optimize regime delta range via walk-forward backtest",
		},
		RegimeBullValue: ParameterMetadata[float64]{
			Value:     -0.02,
			Rationale: "Bull regime: reduce value (-2%) — value lags in momentum-driven bull markets",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: optimize regime delta range via walk-forward backtest",
		},
		RegimeBearQuality: ParameterMetadata[float64]{
			Value:     0.05,
			Rationale: "Bear regime: shift toward quality (+5%) for defensive positioning",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: bear regime delta from 2022 drawdown data",
		},
		RegimeBearValue: ParameterMetadata[float64]{
			Value:     0.03,
			Rationale: "Bear regime: shift toward value (+3%) — value provides relative downside protection",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: bear regime delta from 2022 drawdown data",
		},
		RegimeBearMomentum: ParameterMetadata[float64]{
			Value:     -0.05,
			Rationale: "Bear regime: reduce momentum (-5%) — momentum strategies suffer in reversals",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: bear regime delta from 2022 drawdown data",
		},
		RegimeHighVolLiquidity: ParameterMetadata[float64]{
			Value:     0.05,
			Rationale: "High volatility regime: rotate to liquidity (+5%) for stability",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: high-vol delta from VIX regime analysis",
		},
		RegimeHighVolMomentum: ParameterMetadata[float64]{
			Value:     -0.03,
			Rationale: "High volatility regime: reduce momentum (-3%) — momentum fragile in volatile markets",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: high-vol delta from VIX regime analysis",
		},
		RegimeHighVolInstSent: ParameterMetadata[float64]{
			Value:     -0.02,
			Rationale: "High volatility regime: reduce institutional sentiment (-2%) — lagging indicator in fast markets",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: high-vol delta from VIX regime analysis",
		},
		SeverityCritical: ParameterMetadata[float64]{
			Value:     0.10,
			Rationale: "Critical event: ±10% delta for severe market-moving events",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: severity deltas from event study of 50+ significant events",
		},
		SeverityHigh: ParameterMetadata[float64]{
			Value:     0.05,
			Rationale: "High event: ±5% delta for significant events",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: severity deltas from event study",
		},
		SeverityMedium: ParameterMetadata[float64]{
			Value:     0.02,
			Rationale: "Medium event: ±2% delta for moderate events",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: severity deltas from event study",
		},
		SeverityLow: ParameterMetadata[float64]{
			Value:     0.01,
			Rationale: "Low event: ±1% delta for minor events",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: severity deltas from event study",
		},
		ClampMin: ParameterMetadata[float64]{
			Value:     0.02,
			Rationale: "Minimum factor weight floor (2%) prevents any factor from being eliminated",
			Source:    SourceHeuristic,
			Todo:      "Verify: test with 0.01 floor to assess extreme regime impact",
		},
		ClampMax: ParameterMetadata[float64]{
			Value:     0.50,
			Rationale: "Maximum factor weight ceiling (50%) prevents single-factor dominance",
			Source:    SourceHeuristic,
			Todo:      "Verify: test with 0.40 ceiling to assess diversification impact",
		},
		RiskOnMomentum: ParameterMetadata[float64]{
			Value:     0.05,
			Rationale: "RISK_ON mode: boost momentum (+5%) — risk appetite favors trend",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: event-mode deltas from 2023-2024 risk-on/risk-off regime data",
		},
		RiskOnQuality: ParameterMetadata[float64]{
			Value:     -0.03,
			Rationale: "RISK_ON mode: reduce quality (-3%) — quality premium compresses in risk-on",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: event-mode deltas from 2023-2024 risk-on/risk-off regime data",
		},
		RiskOffMomentum: ParameterMetadata[float64]{
			Value:     -0.05,
			Rationale: "RISK_OFF mode: reduce momentum (-5%) — momentum crashes in risk-off",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: event-mode deltas from 2023-2024 risk-on/risk-off regime data",
		},
		RiskOffQuality: ParameterMetadata[float64]{
			Value:     0.05,
			Rationale: "RISK_OFF mode: boost quality (+5%) — flight to quality",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: event-mode deltas from 2023-2024 risk-on/risk-off regime data",
		},
		RiskOffLiquidity: ParameterMetadata[float64]{
			Value:     0.03,
			Rationale: "RISK_OFF mode: boost liquidity (+3%) — liquidity premium in stress",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: event-mode deltas from 2023-2024 risk-on/risk-off regime data",
		},
		ConservativeValue: ParameterMetadata[float64]{
			Value:     0.05,
			Rationale: "Conservative strategy: boost value (+5%) — value orientation for safety",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: strategy deltas from factor-rotation backtest 2024-2026",
		},
		ConservativeQuality: ParameterMetadata[float64]{
			Value:     0.05,
			Rationale: "Conservative strategy: boost quality (+5%) — quality orientation for safety",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: strategy deltas from factor-rotation backtest 2024-2026",
		},
		ConservativeMomentum: ParameterMetadata[float64]{
			Value:     -0.05,
			Rationale: "Conservative strategy: reduce momentum (-5%) — dampen trend-chasing in defensive mode",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: strategy deltas from factor-rotation backtest 2024-2026",
		},
		AggressiveMomentum: ParameterMetadata[float64]{
			Value:     0.05,
			Rationale: "Aggressive strategy: boost momentum (+5%) — capitalize on trends",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: strategy deltas from factor-rotation backtest 2024-2026",
		},
		AggressiveInstSent: ParameterMetadata[float64]{
			Value:     0.03,
			Rationale: "Aggressive strategy: boost institutional sentiment (+3%) — follow smart money",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: strategy deltas from factor-rotation backtest 2024-2026",
		},
		AggressiveValue: ParameterMetadata[float64]{
			Value:     -0.03,
			Rationale: "Aggressive strategy: reduce value (-3%) — value underperforms in aggressive markets",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: strategy deltas from factor-rotation backtest 2024-2026",
		},
		AggressiveQuality: ParameterMetadata[float64]{
			Value:     -0.03,
			Rationale: "Aggressive strategy: reduce quality (-3%) — quality premium unnecessary in strong uptrend",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: strategy deltas from factor-rotation backtest 2024-2026",
		},
	}
}

func defaultNarrativeConvictionParameters() NarrativeConvictionParameters {
	return NarrativeConvictionParameters{
		ThemeHitRates: ParameterMetadata[map[string]float64]{
			Value: map[string]float64{
				"AI_capex_surge":          0.81,
				"US_rates_up":             0.72,
				"JPY_carry_unwind":        0.68,
				"geopolitical_risk_spike": 0.65,
				"oil_price_shock":         0.58,
			},
			Rationale: "Historical hit rates for narrative themes; AI_capex_surge highest (0.81), oil_price_shock lowest (0.58) due to TW semiconductor hedging",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: derive from 36-month narrative event backtest",
		},
		SkillToTheme: ParameterMetadata[map[string]string]{
			Value: map[string]string{
				"semiconductor_desk": "AI_capex_surge",
				"macro_research":     "US_rates_up",
				"currency_desk":      "JPY_carry_unwind",
				"risk_analytics":     "geopolitical_risk_spike",
				"commodity_research": "oil_price_shock",
				"ai_infrastructure":  "AI_capex_surge",
				"fixed_income":       "US_rates_up",
				"cross_asset":        "JPY_carry_unwind",
				"defense_industrial": "geopolitical_risk_spike",
			},
			Rationale: "Maps research agent skills to narrative themes; semiconductor/ai_infrastructure → AI_capex_surge, macro/fixed_income → US_rates_up, currency/cross_asset → JPY_carry_unwind, risk/defense → geopolitical_risk_spike, commodity → oil_price_shock",
			Source:    SourceHeuristic,
			Todo:      "Validate: review agent-narrative alignment with domain experts",
		},
	}
}
