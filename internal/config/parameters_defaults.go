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
		Drawdown:            defaultDrawdownParameters(),
		Realtime:            defaultRealtimeParameters(),
		Narrative:           defaultNarrativeParameters(),
		Janus:               defaultJanusParameters(),
		Marketdata:          defaultMarketdataParameters(),
		Industry:            defaultIndustryParameters(),
		Strategy:            defaultStrategyParameters(),
		PreciousMetals:      defaultPreciousMetalsParameters(),
		FactorWeight:        defaultFactorWeightParameters(),
		NarrativeConviction: defaultNarrativeConvictionParameters(),
		SectorExecutor:      defaultSectorExecutorParameters(),
		Alert:               defaultAlertParameters(),
		RiskGate:            defaultRiskGateParameters(),
		Engine:              defaultEngineParameters(),
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
			Todo:      "Calibrate neutral weight: test [0.8, 1.0, 1.2] with 30-day backtest",
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
			Todo:      "Validate 20h vs 24h cooldown impact on agent turnover and stability",
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
			Value:     0.02,
			Rationale: "2% daily volatility benchmark based on TW market empirical data; TW stocks typically exhibit 1-3% daily volatility with median ~1.8%",
			Source:    SourceEmpirical,
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
		FactorWeightDriftThreshold: ParameterMetadata[float64]{
			Value:     0.15,
			Rationale: "Maximum allowed factor weight drift (15%) before rejecting experiment as regime-confounded",
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
			Value:     50,
			Rationale: "Floor for superinvestor recommendations — lowered from 65 to match sector default",
			Source:    SourceHeuristic,
		},
		SuperinvestorConvictionBase: ParameterMetadata[int]{
			Value:     60,
			Rationale: "Base conviction for superinvestor Recommend() — above sector default (~55), allows keyword boosts of +8 to reach ~68",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from backtest comparing superinvestor hit rate vs sector agents",
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
		SectorRotationBaseAllocations: ParameterMetadata[map[string]float64]{
			Value: map[string]float64{
				"semiconductor":   0.19,
				"ai_supply_chain": 0.15,
				"robotics":        0.06,
				"financials":      0.11,
				"shipping":        0.07,
				"energy":          0.04,
				"electronics":     0.05,
				"consumer":        0.04,
				"industrial":      0.04,
				"leo_satellite":   0.05,
				"defensive":       0.10,
				"cash":            0.10,
			},
			Rationale: "Baseline sector allocation weights representing Taiwan equity market structure. Weights derived from economic importance (export share, market cap contribution, GDP linkage) with defensive/cash carve-out for risk management.",
			Source:    SourceHeuristic,
			Todo:      "Calibrate via backtest: sweep allocation grids (±0.02 per sector) over 2-year replay window to find max-Sharpe combo. Consider dynamic allocation based on macro regime.",
		},
		SectorRotationMacroAdjustments: ParameterMetadata[map[string]map[string]float64]{
			Value: map[string]map[string]float64{
				"yellow": {
					"defensive":       0.05,
					"cash":            0.03,
					"ai_supply_chain": -0.04,
					"semiconductor":   -0.04,
				},
				"orange": {
					"defensive":       0.10,
					"cash":            0.08,
					"gold":            0.05,
					"ai_supply_chain": -0.08,
					"semiconductor":   -0.08,
					"financials":      -0.05,
				},
				"red": {
					"cash":            0.25,
					"defensive":       0.15,
					"gold":            0.10,
					"ai_supply_chain": -0.15,
					"semiconductor":   -0.15,
					"financials":      -0.10,
					"shipping":        -0.05,
				},
			},
			Rationale: "Macro risk level → sector allocation adjustments; yellow=light defensive tilt, orange=moderate, red=severe risk-off; aligns with sector_rotator.go macroLevelKey mapping",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: derive from macro regime → sector performance analysis",
		},
		SectorRotationFlowAdjustments: ParameterMetadata[map[string]map[string]float64]{
			Value: map[string]map[string]float64{
				"risk_off": {
					"gold":            0.10,
					"utilities":       0.08,
					"high_dividend":   0.07,
					"ai_supply_chain": -0.10,
					"small_cap":       0.0,
				},
				"carry_trade_unwind": {
					"cash":             0.30,
					"short_term_bonds": 0.15,
					"jpy":              0.05,
					"ai_supply_chain":  0.02,
					"semiconductor":    0.03,
					"financials":       -0.10,
				},
				"sector_rotation": {
					"energy":              0.15,
					"oil_services":        0.08,
					"alternative_energy":  0.05,
					"shipping":            0.05,
					"high_valuation_tech": 0.02,
					"rate_sensitive":      -0.08,
				},
			},
			Rationale: "Capital flow patterns → sector allocation adjustments; risk_off=defensive, carry_trade_unwind=cash/JPY, sector_rotation=energy/shipping",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: derive from flow pattern → sector return regression analysis",
		},
	}
}

func defaultDrawdownParameters() DrawdownParameters {
	return DrawdownParameters{
		NonePercentage: ParameterMetadata[float64]{
			Value:     0.0,
			Rationale: "No drawdown reduction at green macro risk; full exposure maintained",
			Source:    SourceHeuristic,
		},
		NoneMaxExposure: ParameterMetadata[float64]{
			Value:     1.0,
			Rationale: "100% exposure allowed when no macro risk detected",
			Source:    SourceHeuristic,
		},
		LightPercentage: ParameterMetadata[float64]{
			Value:     0.15,
			Rationale: "15% reduction for yellow macro risk level (elevated but manageable)",
			Source:    SourceHeuristic,
		},
		LightMaxExposure: ParameterMetadata[float64]{
			Value:     0.85,
			Rationale: "85% max exposure for light drawdown; allows most positions intact",
			Source:    SourceHeuristic,
		},
		ModeratePercentage: ParameterMetadata[float64]{
			Value:     0.35,
			Rationale: "35% reduction for orange macro risk (significant outflow concerns)",
			Source:    SourceHeuristic,
		},
		ModerateMaxExposure: ParameterMetadata[float64]{
			Value:     0.65,
			Rationale: "65% max exposure for moderate drawdown; significant position trimming",
			Source:    SourceHeuristic,
		},
		SeverePercentage: ParameterMetadata[float64]{
			Value:     0.60,
			Rationale: "60% reduction for red macro risk (crisis-level threat); most positions liquidated",
			Source:    SourceHeuristic,
		},
		SevereMaxExposure: ParameterMetadata[float64]{
			Value:     0.40,
			Rationale: "40% max exposure for severe drawdown; only conviction positions survive",
			Source:    SourceHeuristic,
		},
		EmergencyPercentage: ParameterMetadata[float64]{
			Value:     0.90,
			Rationale: "90% reduction for emergency drawdown; near-total liquidation",
			Source:    SourceHeuristic,
		},
		EmergencyMaxExposure: ParameterMetadata[float64]{
			Value:     0.10,
			Rationale: "10% max exposure for emergency drawdown; minimum viable positions only",
			Source:    SourceHeuristic,
		},
		OrangeOverrideMinScore: ParameterMetadata[float64]{
			Value:     0.55,
			Rationale: "Minimum structural trend override score needed to withstand orange macro risk; 0.55 requires moderately strong structural signal",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.50, 0.65] range against historical orange-regime drawdown outcomes",
		},
		RedOverrideMinScore: ParameterMetadata[float64]{
			Value:     0.75,
			Rationale: "Minimum structural trend override score needed to withstand red macro risk; 0.75 requires very strong structural signal to justify staying invested",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.70, 0.85] range against historical red-regime drawdown outcomes; higher bar warranted for crisis-level risk",
		},
		SectorConstraintsRiskOff: ParameterMetadata[map[string]float64]{
			Value: map[string]float64{
				"ai_supply_chain": 0.3,
				"small_cap":       0.2,
				"emerging_market": 0.1,
				"gold":            1.5,
				"utilities":       1.2,
			},
			Rationale: "Risk-off flow: reduce growth/risk assets (AI supply chain 0.3, small cap 0.2, EM 0.1), rotate to defensive (gold 1.5, utilities 1.2)",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: derive multipliers from risk-off regime sector return analysis",
		},
		SectorConstraintsCarryTradeUnwind: ParameterMetadata[map[string]float64]{
			Value: map[string]float64{
				"all_equities": 0.1,
				"tech":         0.05,
				"financials":   0.1,
				"cash":         2.0,
			},
			Rationale: "Carry trade unwind: exit equities (0.1), minimal tech (0.05), minimal financials (0.1), maximum cash (2.0)",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: derive multipliers from carry trade unwind episodes (2024/8, 2008/10, 1998/10)",
		},
		SectorConstraintsSectorRotation: ParameterMetadata[map[string]float64]{
			Value: map[string]float64{
				"energy":              1.8,
				"oil_services":        1.5,
				"high_valuation_tech": 0.3,
				"rate_sensitive":      0.4,
			},
			Rationale: "Sector rotation flow: overweight energy (1.8) and oil services (1.5), reduce high-valuation tech (0.3) and rate-sensitive (0.4)",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: derive multipliers from sector rotation episode return analysis",
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
			Rationale: "3M TWD (~$90K USD) approximates minimum viable capital for 5-position TW equity portfolio with meaningful position sizes (~600K/position); represents conservative retail investor starting point above minimum lot thresholds",
			Source:    SourceHeuristic,
			Todo:      "Validate against typical TW retail account sizes (CBC Financial Stability Report) and adjust for different investor segments",
		},
		MaxPositionWeight: ParameterMetadata[float64]{
			Value:     0.18,
			Rationale: "18% single-position cap provides ~5-6 name diversification with room for conviction weighting; conservative end of typical 15-25% single-name limit for concentrated portfolios; above 20% would allow only 4-5 equally-weighted positions risking under-diversification",
			Source:    SourceLiterature,
			Todo:      "Validate against TW market concentration cost analysis and adjust per market cap segment",
		},
		MaxOpenPositions: ParameterMetadata[int]{
			Value:     5,
			Rationale: "5 open positions balances diversification with monitoring overhead; at 18% max weight, 5 positions allow up to 90% deployed capital; each position gets meaningful allocation (~600K TWD on 3M base) without dilution",
			Source:    SourceHeuristic,
			Todo:      "Backtest [3,8] range to find optimal number of positions for risk-adjusted returns",
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
		RetailFrenzyPercentileThreshold: ParameterMetadata[float64]{
			Value:     90.0,
			Rationale: "Margin balance percentile threshold for retail frenzy event; >=90th percentile indicates extreme retail buying",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from historical margin balance distribution during retail-driven rallies",
		},
		RetailFearPercentileThreshold: ParameterMetadata[float64]{
			Value:     10.0,
			Rationale: "Margin balance percentile threshold for retail fear event; <=10th percentile indicates extreme retail selling",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from historical margin balance distribution during retail-driven selloffs",
		},
		RetailAccelerationWindowDays: ParameterMetadata[int]{
			Value:     5,
			Rationale: "Rolling acceleration window in trading days for retail margin velocity confirmation",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from backtest: test [3, 10] range for optimal retail momentum detection",
		},
		InflationEstimate: ParameterMetadata[float64]{
			Value:     2.5,
			Rationale: "long-run US inflation target used as fallback when live CPI data is unavailable",
			Source:    "heuristic",
			Todo:      "replace with live CPI from FRED API when available",
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
		ConfidenceDeviationCeiling: ParameterMetadata[float64]{
			Value:     0.95,
			Rationale: "Upper bound for deviation-based dynamic confidence; prevents perfect certainty (1.0) for any single indicator",
			Source:    SourceHeuristic,
			Todo:      "Backtest optimal ceiling against forward returns; consider per-theme ceilings",
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
		BDIAPITimeoutSec: ParameterMetadata[int]{
			Value:     10,
			Rationale: "HTTP timeout for CNBC BDI API; public free endpoint, accept slower response",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [5, 15] range based on observed CNBC latency",
		},
		BDIEndpoint: ParameterMetadata[string]{
			Value:     "https://quote.cnbc.com/quote-html-webservice/quote.htm?symbols=.BADI&output=json",
			Rationale: "CNBC free REST JSON API for Baltic Dry Index; no API key required, /BADI symbol includes change_pct and last_time_msec",
			Source:    SourceEmpirical,
			Todo:      "",
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
				"semiconductor":   0.23,
				"ai_supply_chain": 0.18,
				"robotics":        0.07,
				"financials":      0.14,
				"shipping":        0.09,
				"energy":          0.05,
				"electronics":     0.07,
				"consumer":        0.05,
				"industrial":      0.05,
				"leo_satellite":   0.06,
			},
			Rationale: "Industry classification reference weights representing economic importance in DefaultClassification(). Aligned with Level 1 industry segments for consistent display and weight derivation across the industry ecosystem.",
			Source:    SourceHeuristic,
			Todo:      "Recalibrate: consider deriving from sector-level market cap composition or backtest-optimized weight impact on industry recommendation quality.",
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
		AsymmetricDropCritical: ParameterMetadata[float64]{
			Value:     0.10,
			Rationale: "10% price drop triggers critical asymmetric risk severity; aligns with historical TW stock tail-event thresholds where >10% single-day drops correlate with structural deterioration",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: derive from TWSE historical single-day drawdown distribution (99th percentile)",
		},
		AsymmetricDropHigh: ParameterMetadata[float64]{
			Value:     0.07,
			Rationale: "7% price drop triggers high asymmetric risk severity; captures significant but non-extreme downside moves (e.g., sector-wide selloffs, geopolitical events)",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: derive from TWSE historical single-day drawdown distribution (95th percentile)",
		},
		AsymmetricDropMedium: ParameterMetadata[float64]{
			Value:     0.05,
			Rationale: "5% price drop triggers medium asymmetric risk severity; captures moderate downside moves that still warrant stop-loss monitoring",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: derive from TWSE historical single-day drawdown distribution (90th percentile)",
		},
		NewsImpactMultiplier: ParameterMetadata[float64]{
			Value:     0.05,
			Rationale: "5% estimated price impact multiplier for news latency risk scoring; represents conservative estimate of information disadvantage cost for TW investors vs US Tier-1 sources",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: derive from TW stock price response to delayed news versus US peers (event study methodology)",
		},
		BoundaryFallback: ParameterMetadata[float64]{
			Value:     0.25,
			Rationale: "Fallback threshold range width (25%) when cycle phase thresholds (expansion - mature) produce a non-positive range; prevents division-by-zero in boundary confidence calculation",
			Source:    SourceHeuristic,
			Todo:      "Audit: verify this fallback is rarely triggered once per-industry thresholds are properly calibrated",
		},
		AdjustmentFloor: ParameterMetadata[float64]{
			Value:     0.01,
			Rationale: "Minimum seasonal adjustment floor (1% of baseline); prevents complete elimination of industry weighting when combined adjustments drop below zero",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: test [0.005, 0.02] range for floor impact on portfolio stability",
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
				ScoreExpansion: 20, ScoreRecovery: 10, ScoreMature: 0, ScoreRecession: -20,
			},
			Rationale: "Cycle phase conviction deltas; expansion (+20) and recovery (+10) boost, recession (-20) penalizes, mature (0) neutral",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: derive phase deltas from sector-regime conviction regression",
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
		SkillToIndustries: ParameterMetadata[map[string][]string]{
			Value: map[string][]string{
				"semiconductor_desk":   {"semiconductor", "foundry"},
				"ai_supply_chain_desk": {"ai_supply_chain", "pcb", "thermal"},
				"financials_desk":      {"financials"},
				"shipping_desk":        {"shipping"},
				"leo_satellite_desk":   {"leo_satellite", "satellite_rf_components", "satellite_pcb"},
				"etf_rotation_desk":    {"high_dividend", "etf_rotation"},
			},
			Rationale: "Maps agent skills to one or more industry sectors for sector-ban filtering",
			Source:    SourceHeuristic,
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
				RecessionShockAmplifier:   1.30,
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
				"semiconductor_desk":   "AI_capex_surge",
				"ai_supply_chain_desk": "AI_capex_surge",
				"shipping_desk":        "oil_price_shock",
				"financials_desk":      "US_rates_up",
				"etf_rotation_desk":    "JPY_carry_unwind",
				"value_yield":          "US_rates_up",
				"earnings_quality":     "AI_capex_surge",
				"growth_momentum":      "AI_capex_surge",
				"technical_breakout":   "AI_capex_surge",
			},
			Rationale: "Maps agent skills to narrative themes; semiconductor/ai_supply_chain/earnings/growth/technical → AI_capex_surge, financials/value_yield → US_rates_up, etf_rotation → JPY_carry_unwind, shipping → oil_price_shock; aligns with narrative_conviction_modulator.go defaultSkillToTheme",
			Source:    SourceHeuristic,
			Todo:      "Validate: review agent-narrative alignment with domain experts",
		},
	}
}

func defaultSectorExecutorParameters() SectorExecutorParameters {
	return SectorExecutorParameters{
		LEOSatellite: LEOSatelliteExecutorParameters{
			ConvictionBase: ParameterMetadata[int]{
				Value:     60,
				Rationale: "Base conviction for LEOSatelliteExecutor; matches sector consensus used by SemiconductorExecutor and AISupplyChainExecutor",
				Source:    SourceHeuristic,
				Todo:      "Calibrate from backtest: compare hit rates at base 50/60/70 across all sector executors",
			},
			PricePenaltyDelta: ParameterMetadata[int]{
				Value:     -5,
				Rationale: "Intraday weakness penalty: reduces conviction when last < open; mirrors SemiconductorExecutor pattern",
				Source:    SourceHeuristic,
				Todo:      "Test asymmetric penalty levels (-3/-5/-8) via cross-executor backtest sweep",
			},
			LaunchBoostDelta: ParameterMetadata[int]{
				Value:     10,
				Rationale: "Launch keyword signal boost: +10 conviction when prompt mentions launch and price is up; captures LEO satellite launch catalyst events",
				Source:    SourceHeuristic,
				Todo:      "Validate with domain expert: verify launch keyword precision/recall against Starlink/Kuiper deployment news",
			},
			DeploymentBoostDelta: ParameterMetadata[int]{
				Value:     8,
				Rationale: "Deployment keyword signal boost: +8 conviction when prompt mentions deployment and price is up; captures constellation deployment cadence",
				Source:    SourceHeuristic,
				Todo:      "Validate: compare deployment boost hit rate vs launch boost hit rate; consider merging if statistically similar",
			},
			DowngradePenaltyDelta: ParameterMetadata[int]{
				Value:     -10,
				Rationale: "Downgrade signal penalty: -10 conviction when prompt mentions downgrade and last < high*0.99; matches AISupplyChainExecutor pattern",
				Source:    SourceHeuristic,
				Todo:      "Calibrate: sweep -8/-10/-12 penalty levels to find optimal precision-recall tradeoff",
			},
			TargetPriceMult: ParameterMetadata[float64]{
				Value:     1.08,
				Rationale: "8% upside target from entry price; matches AISupplyChainExecutor target multiplier reflecting growth-sector premium",
				Source:    SourceHeuristic,
				Todo:      "Validate: compare 1.08 target vs realized-forward-returns for LEO satellite names over 20-day horizon",
			},
			StopLossMult: ParameterMetadata[float64]{
				Value:     0.95,
				Rationale: "5% stop-loss below entry price; matches SemiconductorExecutor and AISupplyChainExecutor; standard high-volatility sector stop",
				Source:    SourceHeuristic,
				Todo:      "Calibrate: test 0.93/0.95/0.97 stop levels for LEO satellite names given sector volatility profile",
			},
		},
		Financials: FinancialsExecutorParameters{
			DividendBoost:            ParameterMetadata[int]{Value: 8, Rationale: "Rewards dividend-signaling financials where intraday price confirms distribution capacity; TW banks with >3% yield historically outperform sector by 4-6% annually", Source: SourceHeuristic, Todo: "Calibrate [5,8,12] against TW financials dividend backtest 2020-2026"},
			BalanceSheetPenalty:      ParameterMetadata[int]{Value: 6, Rationale: "Penalizes financials flagged for balance-sheet weakness when intraday low drops 1.5%+ below open, indicating market concern over asset quality or leverage ratios", Source: SourceHeuristic, Todo: "Backtest 4/6/8 against TW bank NPL ratio regime shifts"},
			CreditQualityBoost:       ParameterMetadata[int]{Value: 2, Rationale: "Small boost for credit-quality-gate financials with positive intraday price action; conservative magnitude reflects noisy credit-quality signal in short-term price data", Source: SourceHeuristic, Todo: "Validate SNR against TW insurer credit-rating upgrade events"},
			CreditQualityPenalty:     ParameterMetadata[int]{Value: 6, Rationale: "Penalizes financials with deteriorating credit metrics confirmed by negative intraday drift; larger magnitude than boost reflects asymmetric tail risk in credit events", Source: SourceHeuristic, Todo: "Calibrate 4/6/8 against TW financial credit downgrade history"},
			SpreadSensitivityBoost:   ParameterMetadata[int]{Value: 2, Rationale: "Modest boost when spread-sensitive financials trade near session high, signaling resilience to yield-curve widening; small magnitude due to indirect spread proxy via price", Source: SourceHeuristic, Todo: "Monitor precision against TW bank NIM expansion quarters"},
			SpreadSensitivityPenalty: ParameterMetadata[int]{Value: 4, Rationale: "Penalizes spread-sensitive financials failing to hold near highs, suggesting spread compression pressure; asymmetric to boost reflecting convex downside in NIM contraction", Source: SourceHeuristic, Todo: "Backtest 3/4/5 against TW yield-curve inversion periods"},
			CapitalAdequacyBoost:     ParameterMetadata[int]{Value: 3, Rationale: "Boosts well-capitalized financials requiring double price confirmation (last>=open AND near high); high selectivity bar ensures only strongest capital adequacy signals are rewarded", Source: SourceHeuristic, Todo: "Validate against TW FSC capital adequacy ratio filings for banks and insurers"},
			PriceToOpenThreshold:     ParameterMetadata[float64]{Value: 0.985, Rationale: "1.5% intraday drop threshold for balance-sheet penalty trigger; balances sensitivity to genuine distress signals versus normal TW financial sector intraday volatility of 1.0-1.2%", Source: SourceHeuristic, Todo: "Calibrate 0.980/0.985/0.990 against TW financials intraday range distribution"},
			PriceToHighThreshold:     ParameterMetadata[float64]{Value: 0.995, Rationale: "Near-high threshold (0.5% from session peak) for spread and capital adequacy confirmation; tight bar ensures price strength is sustained, not just gap-up noise", Source: SourceHeuristic, Todo: "Validate sensitivity against TW financials close-to-high ratio distribution"},
		},
		Shipping: ShippingExecutorParameters{
			TacticalBoost:      ParameterMetadata[int]{Value: 10, Rationale: "Rewards tactical BDI/container-rate cycle entry when intraday price confirms bullish momentum; high magnitude reflects shipping's high-beta cyclicality and strong regime-dependent edge", Source: SourceHeuristic, Todo: "Calibrate vs BDI lead-lag correlation with TW shipping names 2020-2026"},
			WeakClosePenalty:   ParameterMetadata[int]{Value: 12, Rationale: "Heavy penalty for session-end weakness in shipping names, which historically signals freight-rate deterioration or charterer pullback ahead of visible data", Source: SourceHeuristic, Todo: "Backtest 10/12/14 against TW shipping close-to-high ratio vs next-week returns"},
			WeakCloseThreshold: ParameterMetadata[float64]{Value: 0.992, Rationale: "0.8% below-session-high threshold for weak-close detection; shipping's intraday range is wider than average, requiring slightly looser threshold to avoid false positives", Source: SourceHeuristic, Todo: "Calibrate 0.988/0.992/0.995 against TW shipping intraday volatility profile"},
		},
		ValueYield: ValueYieldExecutorParameters{
			CashFlowBoost:    ParameterMetadata[int]{Value: 10, Rationale: "High conviction boost for free-cash-flow yield signal confirmed by intraday price strength; TW value stocks with FCF yield > 8% and positive price confirmation show 12-18% annual outperformance historically", Source: SourceHeuristic, Todo: "Backtest against TWSE FCF-yield sorted quintile returns 2020-2026 to validate boost magnitude"},
			YieldTrapPenalty: ParameterMetadata[int]{Value: 10, Rationale: "Penalizes suspiciously high dividend yield (>8%) when accompanied by negative intraday price action; flags potential yield traps where high yield masks deteriorating fundamentals in TW cyclical names", Source: SourceHeuristic, Todo: "Calibrate 8/10/12 against TWSE high-yield trap false-positive rate using forward-return analysis"},
		},
		EarningsQuality: EarningsQualityExecutorParameters{
			RepeatableBoost:   ParameterMetadata[int]{Value: 9, Rationale: "Strong conviction boost for companies showing 3+ consecutive quarters of positive earnings surprise exceeding analyst consensus; TW firms with repeatable earnings beat patterns sustain momentum for 6-9 months", Source: SourceHeuristic, Todo: "Validate against TWSE earnings-surprise persistence study: compare 1Q/3Q/5Q streak forward returns"},
			GuidancePenalty:   ParameterMetadata[int]{Value: 8, Rationale: "Penalizes stocks where management guidance revision is downward and confirmed by intraday sell pressure; TW companies issuing negative guidance after hours typically gap down 2-3% next session", Source: SourceHeuristic, Todo: "Backtest penalty accuracy against TWSE post-guidance-revision return distribution 2022-2026"},
			GuidanceThreshold: ParameterMetadata[float64]{Value: 0.99, Rationale: "1% below session high triggers guidance penalty; TW stocks failing to hold within 99% of highs after guidance events indicate market skepticism about management outlook", Source: SourceHeuristic, Todo: "Calibrate 0.985/0.990/0.995 against TWSE guidance-event intraday range patterns"},
		},
		TechnicalBreakout: TechnicalBreakoutExecutorParameters{
			DefaultVolumeFloor:     ParameterMetadata[int64]{Value: 5000000, Rationale: "5M shares is the baseline liquidity threshold for TWSE breakout candidates; filters out thin retail-driven names that lack institutional participation for conviction reliability", Source: SourceHeuristic, Todo: "Calibrate: backtest against TWSE historical breakout patterns (2024-2026 daily data) to optimize floor across 3M/5M/7M"},
			StrictVolumeFloor:      ParameterMetadata[int64]{Value: 7000000, Rationale: "Surge-signal validation requires 40% higher volume than default (7M vs 5M) to confirm genuine institutional buying pressure on TWSE, avoiding false breakouts from retail momentum traps", Source: SourceHeuristic, Todo: "Calibrate: backtest against TWSE historical breakout patterns (2024-2026 daily data) to optimize surge floor"},
			RelaxedVolumeFloor:     ParameterMetadata[int64]{Value: 0, Rationale: "Coverage expansion regime disables volume floor (zero) to permit scanning mid-cap and small-cap TWSE names that exhibit breakout structure but trade below standard liquidity thresholds", Source: SourceHeuristic, Todo: "Calibrate: backtest against TWSE historical breakout patterns (2024-2026 daily data) to assess expansion quality"},
			LowVolumeFloor:         ParameterMetadata[int64]{Value: 3000000, Rationale: "3M shares lower bound for low-volume participation acceptance; identifies TWSE stocks with meaningful but sub-standard liquidity that still warrant a modest conviction boost for thin-liquidity alpha", Source: SourceHeuristic, Todo: "Calibrate: backtest against TWSE historical breakout patterns (2024-2026 daily data) to optimize low-vol band"},
			LowVolumeBoost:         ParameterMetadata[int]{Value: 3, Rationale: "Small +3 conviction boost compensates for the information disadvantage of thin-liquidity TWSE stocks (3M-5M volume) by recognizing that even modest volume participation signals emerging interest in neglected names", Source: SourceHeuristic, Todo: "Calibrate: backtest against TWSE historical breakout patterns (2024-2026 daily data) to validate precision of low-vol boost"},
			RejectLowVolumeFloor:   ParameterMetadata[int64]{Value: 5000000, Rationale: "Hard rejection threshold at 5M shares for breakout candidates with 'reject low volume' prompt; prevents low-liquidity TWSE names from entering the pipeline when structure-first filtering is required", Source: SourceHeuristic, Todo: "Calibrate: backtest against TWSE historical breakout patterns (2024-2026 daily data) to calibrate false-breakout rejection rate"},
			VolumeBoost:            ParameterMetadata[int]{Value: 8, Rationale: "+8 conviction boost when volume meets the 5M default floor and prompt requests volume confirmation; rewards TWSE breakouts backed by sufficient participation, distinguishing genuine institutional interest from noise", Source: SourceHeuristic, Todo: "Calibrate: backtest against TWSE historical breakout patterns (2024-2026 daily data) across 6/8/10 values"},
			CloseStrengthPenalty:   ParameterMetadata[int]{Value: 1, Rationale: "Small -1 conviction penalty for weak close (last < high×0.985) to mildly flag TWSE intraday reversals where breakout fails to hold near highs, indicating potential distribution rather than accumulation", Source: SourceHeuristic, Todo: "Calibrate: backtest against TWSE historical breakout patterns (2024-2026 daily data) to validate magnitude"},
			CloseStrengthThreshold: ParameterMetadata[float64]{Value: 0.985, Rationale: "1.5% below intraday high (last < high×0.985) triggers weak-close penalty; calibrated to TWSE typical intraday range where a close below 98.5% of high suggests selling pressure overwhelmed breakout momentum", Source: SourceHeuristic, Todo: "Calibrate: backtest against TWSE historical breakout patterns (2024-2026 daily data) across 0.980/0.985/0.990"},
			CloseStrengthTolerance: ParameterMetadata[float64]{Value: 0.98, Rationale: "2% tolerance band (last >= high×0.98) exempts close from penalty when tolerance is enabled; prevents over-penalizing TWSE stocks that close modestly below highs but still within acceptable reversal range", Source: SourceHeuristic, Todo: "Calibrate: backtest against TWSE historical breakout patterns (2024-2026 daily data) to optimize tolerance band"},
			SurgeBoost:             ParameterMetadata[int]{Value: 4, Rationale: "+4 conviction boost when volume meets the strict surge floor (7M+); confirms that TWSE breakout is backed by abnormally high participation, signaling strong institutional conviction in the move", Source: SourceHeuristic, Todo: "Calibrate: backtest against TWSE historical breakout patterns (2024-2026 daily data) across 3/4/5 values"},
			SurgePenalty:           ParameterMetadata[int]{Value: 4, Rationale: "-4 conviction penalty when volume fails to meet strict surge floor despite surge requirement; flags TWSE breakouts that lack the expected institutional volume, indicating potential bull trap", Source: SourceHeuristic, Todo: "Calibrate: backtest against TWSE historical breakout patterns (2024-2026 daily data) to calibrate false-surge penalty"},
			OpenRejectionPenalty:   ParameterMetadata[int]{Value: 10, Rationale: "-10 conviction penalty when last < open under structure-first breakout filter; a strong TWSE intraday reversal below opening price indicates selling dominated the session, invalidating the breakout thesis", Source: SourceHeuristic, Todo: "Calibrate: backtest against TWSE historical breakout patterns (2024-2026 daily data) to validate false-breakout rate"},
			LateBreakoutPenalty:    ParameterMetadata[int]{Value: 8, Rationale: "-8 conviction penalty for late-session breakout fade (last < high×0.998); TWSE late-day reversals suggest exhaustion or program selling rather than sustained buying, reducing breakout credibility", Source: SourceHeuristic, Todo: "Calibrate: backtest against TWSE historical breakout patterns (2024-2026 daily data) across 6/8/10 values"},
			LateBreakoutThreshold:  ParameterMetadata[float64]{Value: 0.998, Rationale: "0.2% below intraday high (last < high×0.998) triggers late-breakout penalty; TWSE closing within 0.2% of high is acceptable, but failure to hold this level signals intraday distribution", Source: SourceHeuristic, Todo: "Calibrate: backtest against TWSE historical breakout patterns (2024-2026 daily data) across 0.995/0.998/0.999"},
			ConfirmationBoost:      ParameterMetadata[int]{Value: 12, Rationale: "+12 conviction boost for confirmed breakout (last >= high×0.998 AND volume >= 5M); dual-confirmation of price holding near highs with institutional volume is the strongest TWSE breakout signal", Source: SourceHeuristic, Todo: "Calibrate: backtest against TWSE historical breakout patterns (2024-2026 daily data) across 10/12/14 values"},
			ConfirmationThreshold:  ParameterMetadata[float64]{Value: 0.998, Rationale: "99.8% of intraday high required for confirmation boost; ensures TWSE close is essentially at or above the session high, validating that breakout momentum persisted through the close", Source: SourceHeuristic, Todo: "Calibrate: backtest against TWSE historical breakout patterns (2024-2026 daily data) to optimize confirmation quality"},
			CatchUpBoost:           ParameterMetadata[int]{Value: 6, Rationale: "+6 conviction boost for catch-up momentum (last in 0.993-0.998×high range, above open); identifies TWSE stocks mounting an intraday recovery that hasn't yet reached full breakout, offering early entry opportunity", Source: SourceHeuristic, Todo: "Calibrate: backtest against TWSE historical breakout patterns (2024-2026 daily data) across 4/6/8 values"},
			CatchUpLowerThreshold:  ParameterMetadata[float64]{Value: 0.993, Rationale: "0.7% below high (last >= high×0.993) is the lower bound for catch-up momentum detection; TWSE stocks recovering to within 0.7% of highs show meaningful buying interest resuming after a pullback", Source: SourceHeuristic, Todo: "Calibrate: backtest against TWSE historical breakout patterns (2024-2026 daily data) to optimize catch-up zone"},
			CatchUpUpperThreshold:  ParameterMetadata[float64]{Value: 0.998, Rationale: "0.2% below high (last < high×0.998) is the upper bound for catch-up zone; stocks above this level are classified as confirmed breakouts rather than catch-up candidates, avoiding overlap with ConfirmationBoost", Source: SourceHeuristic, Todo: "Calibrate: backtest against TWSE historical breakout patterns (2024-2026 daily data) to optimize catch-up zone"},
		},
		GrowthMomentum: GrowthMomentumExecutorParameters{
			ConvictionBase:           ParameterMetadata[int]{Value: 45, Rationale: "Moderate starting conviction for growth-momentum style; lower than sector base (60) because growth picks carry higher earnings-volatility risk requiring more confirmatory signals before full confidence", Source: SourceHeuristic, Todo: "Calibrate from backtest: compare hit rates at base 40/45/50 across growth-momentum picks"},
			PricePenalty:             ParameterMetadata[int]{Value: 8, Rationale: "Intraday weakness penalty: when last < open the closing buyer lost control, signalling potential distribution in growth names that rely on sustained buying pressure", Source: SourceHeuristic, Todo: "Calibrate 6/8/10 via cross-executor backtest sweep comparing forward returns on penalized vs non-penalized picks"},
			TrendConfirmationPenalty: ParameterMetadata[int]{Value: 15, Rationale: "Heavy penalty when prompt demands trend confirmation but price closes below open, indicating the trend hypothesis is contradicted by intraday price action", Source: SourceHeuristic, Todo: "Backtest 12/15/18 to measure false-trend-filter precision at each penalty level"},
			DowngradePricePenalty:    ParameterMetadata[int]{Value: 12, Rationale: "Price-based downgrade penalty: when last falls below high×threshold (0.995), the stock failed to hold its peak, signalling potential reversal from a downgrade event", Source: SourceHeuristic, Todo: "Calibrate 10/12/14: compare downgrade penalty hit rate against post-downgrade 5-day forward returns"},
			DowngradeOpenPenalty:     ParameterMetadata[int]{Value: 8, Rationale: "Open-based downgrade penalty: when last < open alongside downgrade signals, intraday selling pressure compounds the downgrade thesis with visible bearish conviction", Source: SourceHeuristic, Todo: "Backtest 6/8/10: measure whether open-penalty adds discriminative value beyond price-penalty alone"},
			ExploratoryPricePenalty:  ParameterMetadata[int]{Value: 6, Rationale: "Reduced price penalty for exploratory-mode picks (6 vs standard 12); exploratory signals have wider hypothesis space so equal-weighting full downgrade penalty would over-penalize novel growth candidates", Source: SourceHeuristic, Todo: "Monitor exploratory vs standard penalty hit rates separately; merge if statistically indistinguishable"},
			ExploratoryOpenPenalty:   ParameterMetadata[int]{Value: 4, Rationale: "Reduced open penalty for exploratory mode (4 vs standard 8); acknowledges higher uncertainty tolerance when probing nascent growth themes before trend is established", Source: SourceHeuristic, Todo: "Track exploratory-penalty cohort performance vs standard cohort to calibrate optimal discount factor"},
			DowngradeThreshold:       ParameterMetadata[float64]{Value: 0.995, Rationale: "Downgrade trigger threshold: last < high×0.995 means a 0.5% drop from session high, sensitive enough to catch early distribution in high-momentum names without false-triggering on noise", Source: SourceHeuristic, Todo: "Calibrate 0.990/0.995/0.998: measure false-positive rate at each threshold on growth-momentum backtest universe"},
		},
		FactorConviction: FactorConvictionParams{
			// --- Momentum factor ---
			MomentumHighThreshold: ParameterMetadata[float64]{Value: 0.4, Rationale: "Momentum factor score ≥0.4 triggers high-conviction delta (+8); top 20% of TWSE momentum distribution, used by TechnicalBreakout for breakout-confirmed momentum picks", Source: SourceHeuristic, Todo: "Calibrate 0.3/0.4/0.5 against TWSE momentum-quintile forward returns; higher threshold increases selectivity"},
			MomentumHighDelta:     ParameterMetadata[int]{Value: 8, Rationale: "+8 conviction boost for strong momentum (>0.4); magnitude reflects momentum factor's historical 60-65% hit rate on TWSE mid/large-cap universe over 20-day forward window", Source: SourceHeuristic, Todo: "Calibrate 6/8/10: measure hit-rate uplift per delta increment on backtest"},
			MomentumModThreshold:  ParameterMetadata[float64]{Value: 0.15, Rationale: "Momentum score ≥0.15 triggers moderate delta (+4); captures above-median momentum stocks (40th-80th percentile), used by TechnicalBreakout for standard picks and Shipping for BDI-cycle entries", Source: SourceHeuristic, Todo: "Calibrate 0.10/0.15/0.20 against TWSE momentum moderate-tier hit rate and false-positive rate"},
			MomentumModDelta:      ParameterMetadata[int]{Value: 4, Rationale: "+4 conviction boost for moderate momentum; conservative half-weighting relative to high tier reflects lower signal strength at median momentum levels where noise increases", Source: SourceHeuristic, Todo: "Calibrate 3/4/5: verify linearity of delta-to-forward-return relationship"},
			MomentumWeakThreshold: ParameterMetadata[float64]{Value: -0.1, Rationale: "Momentum score ≤-0.1 triggers penalty (-4); identifies negative-momentum TWSE stocks where trend has reversed, protecting against value-trap entries in declining names", Source: SourceHeuristic, Todo: "Calibrate -0.15/-0.10/-0.05: compare penalty-hit rate against subsequent drawdown severity"},
			MomentumWeakDelta:     ParameterMetadata[int]{Value: -4, Rationale: "-4 conviction penalty for weak/negative momentum; symmetric to moderate delta (+4) reflecting equal conviction adjustment magnitude in both directions", Source: SourceHeuristic, Todo: "Calibrate -5/-4/-3: validate symmetry assumption against TWSE bear-regime momentum patterns"},
			// --- Value factor ---
			ValueHighThreshold: ParameterMetadata[float64]{Value: 0.3, Rationale: "Value factor score ≥0.3 triggers high-conviction delta (+8); captured by ValueYield executor for deep-value picks with strong P/B and P/E discounts on TWSE", Source: SourceHeuristic, Todo: "Calibrate 0.25/0.30/0.40 against TWSE value-quintile forward returns and value-premium persistence"},
			ValueHighDelta:     ParameterMetadata[int]{Value: 8, Rationale: "+8 conviction boost for deep value (>0.3); TW value premium averages 3-5% annual excess return in top value quintile, warranting conviction proportional to momentum boost", Source: SourceHeuristic, Todo: "Calibrate 6/8/10: measure hit-rate uplift and alpha contribution per delta increment"},
			ValueModThreshold:  ParameterMetadata[float64]{Value: 0.1, Rationale: "Value score ≥0.1 triggers moderate delta (+4); above-median value used by Financials (bank valuations) and ValueYield (broad value screening) for moderate conviction", Source: SourceHeuristic, Todo: "Calibrate 0.05/0.10/0.15 against TWSE value moderate-tier forward returns; optimize for Sharpe improvement"},
			ValueModDelta:      ParameterMetadata[int]{Value: 4, Rationale: "+4 conviction boost for moderate value; consistent with momentum moderate delta, maintaining factor-weight symmetry across the multi-factor scoring framework", Source: SourceHeuristic, Todo: "Calibrate 3/4/5: validate cross-factor delta consistency improves overall portfolio Sharpe"},
			ValueWeakThreshold: ParameterMetadata[float64]{Value: -0.2, Rationale: "Value score ≤-0.2 triggers penalty (-5); flags overvalued stocks (negative value factor) where premium valuations lack fundamental support, common in TW growth-bubble names", Source: SourceHeuristic, Todo: "Calibrate -0.25/-0.20/-0.15: compare penalty-hit rate against value-trap false-positive rate"},
			ValueWeakDelta:     ParameterMetadata[int]{Value: -5, Rationale: "-5 conviction penalty for negative value factor; slightly larger magnitude than momentum weak (-4) because value-factor reversals tend to be more persistent in TW market", Source: SourceHeuristic, Todo: "Calibrate -6/-5/-4: verify value-penalty asymmetry improves risk-adjusted returns vs symmetric approach"},
			// --- Quality factor ---
			QualityThreshold: ParameterMetadata[float64]{Value: 0.2, Rationale: "Quality factor score ≥0.2 triggers +4 delta; captures firms with strong ROE, low accruals, and stable margins — used by ValueYield, EarningsQuality, and Financials executors", Source: SourceHeuristic, Todo: "Calibrate 0.15/0.20/0.30 against quality-quintile forward returns on TWSE; higher threshold increases purity"},
			QualityDelta:     ParameterMetadata[int]{Value: 4, Rationale: "+4 conviction boost for quality; matches moderate-tier delta convention across all factors, ensuring uniform factor-weight contribution scaling", Source: SourceHeuristic, Todo: "Calibrate 3/4/5: validate hit-rate improvement per delta increment in quality-sorted TWSE backtest"},
			// --- Liquidity factor ---
			LiquidityHighThreshold: ParameterMetadata[float64]{Value: 0.5, Rationale: "Liquidity score ≥0.5 triggers +5 delta; identifies highly liquid TWSE stocks (top quartile turnover, tight spreads) where execution risk is minimal — used by TechnicalBreakout for volume-confirmed entries", Source: SourceHeuristic, Todo: "Calibrate 0.4/0.5/0.6 against liquidity-quartile execution slippage and fill-rate data"},
			LiquidityHighDelta:     ParameterMetadata[int]{Value: 5, Rationale: "+5 conviction boost for high liquidity; slightly higher than standard moderate delta (+4) because liquidity directly reduces execution costs, providing a tangible alpha enhancement", Source: SourceHeuristic, Todo: "Calibrate 4/5/6: measure alpha contribution of liquidity boost vs execution-cost savings"},
			LiquidityGoodThreshold: ParameterMetadata[float64]{Value: 0.2, Rationale: "Liquidity score ≥0.2 triggers +3 delta; adequate liquidity for institutional-sized TWSE orders without significant market impact — used by TechnicalBreakout and ETFRotation executors", Source: SourceHeuristic, Todo: "Calibrate 0.15/0.20/0.30 against TWSE institutional order market-impact models"},
			LiquidityGoodDelta:     ParameterMetadata[int]{Value: 3, Rationale: "+3 conviction boost for adequate liquidity; modest delta reflecting that good liquidity enables but doesn't generate alpha — it removes a constraint rather than adding edge", Source: SourceHeuristic, Todo: "Calibrate 2/3/4: verify marginal benefit of liquidity boost beyond execution-cost reduction"},
			LiquidityLowThreshold:  ParameterMetadata[float64]{Value: -0.3, Rationale: "Liquidity score ≤-0.3 triggers -5 penalty; identifies stocks where bid-ask spreads and turnover indicate prohibitive execution costs (>50bp estimated slippage) for institutional positions", Source: SourceHeuristic, Todo: "Calibrate -0.35/-0.30/-0.20 against TWSE low-liquidity stock execution-cost and fill-failure data"},
			LiquidityLowDelta:      ParameterMetadata[int]{Value: -5, Rationale: "-5 conviction penalty for insufficient liquidity; magnitude reflects that poor liquidity can turn a good signal into a losing trade through slippage, making it a first-order risk factor", Source: SourceHeuristic, Todo: "Calibrate -6/-5/-4: validate penalty magnitude against actual TWSE slippage costs in low-liquidity regime"},
		},
	}
}

func defaultAlertParameters() AlertParameters {
	return AlertParameters{
		MinCashThreshold: ParameterMetadata[float64]{
			Value:     100000,
			Rationale: "Minimum cash reserve before triggering low-cash warning",
			Source:    SourceHeuristic,
		},
		MaxPositionsCount: ParameterMetadata[int]{
			Value:     20,
			Rationale: "Maximum number of open positions before concentration warning",
			Source:    SourceHeuristic,
		},
		MaxPositionWeightPct: ParameterMetadata[float64]{
			Value:     0.15,
			Rationale: "Maximum single position weight (15%) before error alert",
			Source:    SourceHeuristic,
		},
		MaxUnrealizedLossPct: ParameterMetadata[float64]{
			Value:     -0.05,
			Rationale: "Unrealized loss threshold (-5%) before position warning",
			Source:    SourceHeuristic,
		},
		DailyLossWarningPct: ParameterMetadata[float64]{
			Value:     -0.015,
			Rationale: "Daily PnL warning threshold (-1.5%)",
			Source:    SourceHeuristic,
		},
		DailyLossCriticalPct: ParameterMetadata[float64]{
			Value:     -0.02,
			Rationale: "Daily PnL critical threshold (-2%)",
			Source:    SourceHeuristic,
		},
		RuleEngineIntervalSec: ParameterMetadata[int]{
			Value:     30,
			Rationale: "Rule engine evaluation interval (30 seconds)",
			Source:    SourceHeuristic,
		},
		RuleEngineCooldownSec: ParameterMetadata[int]{
			Value:     300,
			Rationale: "Default rule cooldown (5 minutes)",
			Source:    SourceHeuristic,
		},
		SystemMetricsIntervalSec: ParameterMetadata[int]{
			Value:     30,
			Rationale: "System metrics collection interval (30 seconds)",
			Source:    SourceHeuristic,
		},
		MinScreeningRate: ParameterMetadata[float64]{
			Value:     0.1,
			Rationale: "Minimum screening rate (10%) before warning",
			Source:    SourceHeuristic,
		},
		MaxAlertTriggerRate: ParameterMetadata[float64]{
			Value:     100,
			Rationale: "Maximum alert trigger rate per hour before critical",
			Source:    SourceHeuristic,
		},
		MaxUnacknowledgedAlerts: ParameterMetadata[int]{
			Value:     10,
			Rationale: "Maximum unacknowledged alerts before warning",
			Source:    SourceHeuristic,
		},
	}
}

func defaultRiskGateParameters() RiskGateParameters {
	return RiskGateParameters{
		PreTrade: PreTradeGateParameters{
			MaxPositionPct: ParameterMetadata[float64]{
				Value:     0.15,
				Rationale: "單一持股最大曝險 15%",
				Source:    SourceLiterature,
			},
			MaxSectorExposurePct: ParameterMetadata[float64]{
				Value:     0.40,
				Rationale: "單一產業最大曝險 40%",
				Source:    SourceHeuristic,
			},
			VaRConfidenceLevel: ParameterMetadata[float64]{
				Value:     0.95,
				Rationale: "VaR 信賴水準 95%",
				Source:    SourceLiterature,
			},
			VarLimitPct: ParameterMetadata[float64]{
				Value:     0.02,
				Rationale: "VaR 不得超過組合價值 2%",
				Source:    SourceHeuristic,
			},
			MinCashBufferPct: ParameterMetadata[float64]{
				Value:     0.05,
				Rationale: "至少保留 5% 現金緩衝",
				Source:    SourceHeuristic,
			},
			MaxCorrelation: ParameterMetadata[float64]{
				Value:     0.70,
				Rationale: "與現有持倉相關性 > 0.7 則降低權重",
				Source:    SourceHeuristic,
			},
			MinADVRatio: ParameterMetadata[float64]{
				Value:     0.01,
				Rationale: "下單量不得超過日均量 1%",
				Source:    SourceLiterature,
			},
		},
		InTrade: InTradeGateParameters{
			MonitorIntervalSec: ParameterMetadata[int]{
				Value:     30,
				Rationale: "每 30 秒檢查一次持倉狀態",
				Source:    SourceHeuristic,
			},
			StopLossPct: ParameterMetadata[float64]{
				Value:     -0.10,
				Rationale: "個股虧損達 10% 即止損",
				Source:    SourceHeuristic,
			},
			TakeProfitPct: ParameterMetadata[float64]{
				Value:     0.30,
				Rationale: "個股獲利達 30% 考慮部分獲利了結",
				Source:    SourceHeuristic,
			},
			TrailingStopATRMult: ParameterMetadata[float64]{
				Value:     2.0,
				Rationale: "2x ATR trailing stop",
				Source:    SourceLiterature,
			},
			VolatilitySpikeMult: ParameterMetadata[float64]{
				Value:     3.0,
				Rationale: "波動率超過 3 倍歷史均值 → 減碼",
				Source:    SourceEmpirical,
			},
			CircuitBreakerDailyLossPct: ParameterMetadata[float64]{
				Value:     -0.05,
				Rationale: "單日組合虧損 5% → 暫停交易",
				Source:    SourceHeuristic,
			},
		},
		PostTrade: PostTradeGateParameters{
			MaxDrawdownHaltPct: ParameterMetadata[float64]{
				Value:     0.20,
				Rationale: "最大回撤 20% → SUSPENDED",
				Source:    SourceHeuristic,
			},
			MaxDrawdownDefensivePct: ParameterMetadata[float64]{
				Value:     0.10,
				Rationale: "最大回撤 10% → DEFENSIVE（減半倉）",
				Source:    SourceHeuristic,
			},
			MinRollingSharpe: ParameterMetadata[float64]{
				Value:     0.0,
				Rationale: "滾動 Sharpe < 0 → CAUTIOUS",
				Source:    SourceLiterature,
			},
			ConsecutiveLossDays: ParameterMetadata[int]{
				Value:     5,
				Rationale: "連續虧損 5 天 → mute agent",
				Source:    SourceHeuristic,
			},
			EvaluationIntervalHours: ParameterMetadata[int]{
				Value:     24,
				Rationale: "每日盤後評估一次",
				Source:    SourceHeuristic,
			},
		},
	}
}

func defaultPreciousMetalsParameters() PreciousMetalsParameters {
	return PreciousMetalsParameters{
		CentralBankBuyingTrend: ParameterMetadata[string]{
			Value:     "stable",
			Rationale: "WGC quarterly CB gold buying trend: accelerating, stable, or decelerating",
			Source:    SourceLiterature,
			Todo:      "Update quarterly from WGC Gold Demand Trends report",
			Citation: &ParameterCitation{
				SourceType:      "report",
				SourceReference: "World Gold Council, Gold Demand Trends Q1 2026",
				EvidenceQuality: "medium",
				UpdatePolicy:    "quarterly",
				LastValidated:   "2026-05-01",
			},
		},
		CentralBankNetBuy: ParameterMetadata[float64]{
			Value:     800.0,
			Rationale: "Annualized central bank net gold purchases in tonnes (~800t in 2025)",
			Source:    SourceLiterature,
			Todo:      "Update from WGC quarterly report",
		},
		IndiaGoldImportsYoY: ParameterMetadata[float64]{
			Value:     0.0,
			Rationale: "India gold imports YoY % change; 0 means no change from prior year",
			Source:    SourceLiterature,
			Todo:      "Update monthly from India Ministry of Commerce data",
		},
		ChinaGoldImportsYoY: ParameterMetadata[float64]{
			Value:     0.0,
			Rationale: "China SGE withdrawal YoY % change; 0 means no change from prior year",
			Source:    SourceLiterature,
			Todo:      "Update monthly from Shanghai Gold Exchange data",
		},
		COMEXDefaultNetLong: ParameterMetadata[float64]{
			Value:     150000,
			Rationale: "CFTC COT managed money net long default (typical mid-cycle level)",
			Source:    SourceEmpirical,
			Todo:      "Update weekly from CFTC Commitment of Traders report",
		},
	}
}

func defaultEngineParameters() EngineParameters {
	return EngineParameters{
		MacroRisk: EngineMacroRiskParameters{
			CarryTradeUnwindThreshold: ParameterMetadata[float64]{
				Value:     145.0,
				Rationale: "JPY/USD above 145 triggers carry trade unwind concern; TW market foreign outflow accelerates above this threshold",
				Source:    SourceEmpirical,
				Todo:      "Recalibrate with 2024-2026 JPY-TWSE correlation data",
			},
			VIXThreshold: ParameterMetadata[float64]{
				Value:     30.0,
				Rationale: "VIX above 30 indicates global fear regime triggering TW foreign capital outflow",
				Source:    SourceEmpirical,
				Todo:      "Recalibrate with 2025-2026 VIX-TAIEX correlation",
			},
			US10YThreshold: ParameterMetadata[float64]{
				Value:     4.5,
				Rationale: "US 10Y above 4.5% signals rate-driven capital reallocation from EM equities; TW market shows sensitivity at this level",
				Source:    SourceEmpirical,
			},
			OilShockThresholdPct: ParameterMetadata[float64]{
				Value:     10.0,
				Rationale: "10% oil price surge triggers supply-chain cost shock for TW manufacturing; historical shocks: 2022 Ukraine (+30%), 2023 OPEC cut (+8%)",
				Source:    SourceEmpirical,
			},
			GoldSurgeThresholdPct: ParameterMetadata[float64]{
				Value:     5.0,
				Rationale: "5% gold surge signals risk-off flight to safety, correlating with TW equity outflows",
				Source:    SourceHeuristic,
				Todo:      "Validate against gold-USD-TWD correlation during 2022-2026 risk events",
			},
			DXYSurgeThresholdPct: ParameterMetadata[float64]{
				Value:     1.5,
				Rationale: "1.5% DXY surge signals USD strength that pressures EM currencies including TWD; TW exporters face FX headwind",
				Source:    SourceEmpirical,
			},
			TWDStressThresholdPct: ParameterMetadata[float64]{
				Value:     2.0,
				Rationale: "2% TWD depreciation in a session signals capital flight; TW central bank typically intervenes at this level",
				Source:    SourceEmpirical,
			},
			OutflowProbBase: ParameterMetadata[float64]{
				Value:     35.0,
				Rationale: "35% baseline foreign outflow probability under normal conditions; calibrated from TWSE foreign flow data 2020-2025",
				Source:    SourceEmpirical,
			},
			OutflowProbMax: ParameterMetadata[float64]{
				Value:     80.0,
				Rationale: "80% max outflow probability under extreme conditions; upper bound observed during 2020 COVID and 2022 rate shock",
				Source:    SourceEmpirical,
			},
		},
		StructuralTrend: EngineStructuralTrendParameters{
			MinTrendStrength: ParameterMetadata[float64]{
				Value:     0.7,
				Rationale: "Minimum 0.7 trend strength for structural classification; below this threshold trends are considered noise",
				Source:    SourceHeuristic,
				Todo:      "Calibrate against TW structural trend detection accuracy 2020-2026",
			},
			MinConfidence: ParameterMetadata[float64]{
				Value:     0.75,
				Rationale: "75% minimum confidence for structural trend signal; balances false positive rate vs early detection",
				Source:    SourceHeuristic,
			},
			MinHitRate: ParameterMetadata[float64]{
				Value:     0.70,
				Rationale: "70% minimum historical hit rate for narrative events to be considered valid structural trends",
				Source:    SourceHeuristic,
				Todo:      "Validate against narrative event hit rate distribution",
			},
			OverrideThreshold: ParameterMetadata[float64]{
				Value:     0.65,
				Rationale: "0.65 threshold for structural trend to override cyclical signals; must be lower than MinConfidence to prioritize structure",
				Source:    SourceHeuristic,
			},
			AIRevenueGrowthThreshold: ParameterMetadata[float64]{
				Value:     50.0,
				Rationale: "50% YoY AI revenue growth threshold flags structural AI capex surge; TSMC AI revenue grew ~50% in 2024-2025",
				Source:    SourceEmpirical,
			},
			CoWoSUtilizationThreshold: ParameterMetadata[float64]{
				Value:     85.0,
				Rationale: "85% CoWoS utilization signals near-capacity AI packaging demand; TSMC CoWoS utilization tracked at 90%+ in 2025",
				Source:    SourceEmpirical,
			},
			CapexGrowthThreshold: ParameterMetadata[float64]{
				Value:     40.0,
				Rationale: "40% YoY capex growth flags structural capacity expansion; TSMC capex grew ~34% in 2024, ~40% planned for 2025",
				Source:    SourceEmpirical,
			},
			SemiconductorIndexThreshold: ParameterMetadata[float64]{
				Value:     0.0,
				Rationale: "Placeholder for semiconductor index trend threshold; pending index construction",
				Source:    SourceHeuristic,
				Todo:      "Define once TW semiconductor composite index is built",
			},
		},
		Drawdown: EngineDrawdownParameters{
			Levels: ParameterMetadata[map[string]DrawdownLevel]{
				Value: map[string]DrawdownLevel{
					"none":      {Percentage: 0.0, MaxExposure: 1.0},
					"light":     {Percentage: 0.15, MaxExposure: 0.85},
					"moderate":  {Percentage: 0.35, MaxExposure: 0.65},
					"severe":    {Percentage: 0.60, MaxExposure: 0.40},
					"emergency": {Percentage: 0.90, MaxExposure: 0.10},
				},
				Rationale: "Five-tier drawdown risk framework: none (0%), light (15%), moderate (35%), severe (60%), emergency (90%); calibrated to TW market correction patterns",
				Source:    SourceHeuristic,
				Todo:      "Validate drawdown level thresholds against 2020-2026 TWSE drawdown distribution",
			},
			OrangeOverrideMinScore: ParameterMetadata[float64]{
				Value:     0.55,
				Rationale: "Minimum composite score for orange (moderate) drawdown override of agent recommendations",
				Source:    SourceHeuristic,
			},
			RedOverrideMinScore: ParameterMetadata[float64]{
				Value:     0.75,
				Rationale: "Minimum composite score for red (severe/emergency) drawdown override; must be > OrangeOverrideMinScore",
				Source:    SourceHeuristic,
			},
			SectorConstraintsRiskOff: ParameterMetadata[map[string]float64]{
				Value: map[string]float64{
					"ai_supply_chain": 0.3, "small_cap": 0.2, "emerging_market": 0.1,
					"gold": 1.5, "utilities": 1.2,
				},
				Rationale: "Risk-off sector constraints: AI supply chain cut to 30% of normal, small cap/EM near-zero; gold/utilities boosted as defensive rotation",
				Source:    SourceHeuristic,
			},
			SectorConstraintsCarryUnwind: ParameterMetadata[map[string]float64]{
				Value: map[string]float64{
					"all_equities": 0.1, "tech": 0.05, "financials": 0.1, "cash": 2.0,
				},
				Rationale: "Carry trade unwind constraints: equities severely curtailed (5-10%), cash doubled; based on 2024 JPY carry unwind impact on TW markets",
				Source:    SourceEmpirical,
			},
			SectorConstraintsSectorRotate: ParameterMetadata[map[string]float64]{
				Value: map[string]float64{
					"energy": 1.8, "oil_services": 1.5, "high_valuation_tech": 0.3, "rate_sensitive": 0.4,
				},
				Rationale: "Sector rotation constraints: energy/oil services boosted 1.5-1.8x on commodity surge; high-valuation tech and rate-sensitives cut to 30-40%",
				Source:    SourceHeuristic,
			},
		},
		SectorRotation: EngineSectorRotationParameters{
			BaseAllocations: ParameterMetadata[map[string]float64]{
				Value: map[string]float64{
					"semiconductor": 0.19, "ai_supply_chain": 0.15, "robotics": 0.06,
					"financials": 0.11, "shipping": 0.07, "energy": 0.04,
					"electronics": 0.05, "consumer": 0.04, "industrial": 0.04,
					"leo_satellite": 0.05, "defensive": 0.10, "cash": 0.10,
				},
				Rationale: "TW market sector allocation: semiconductor (19%) + AI supply chain (15%) dominate (34% combined); defensive+cash (20%) reserve; all sectors sum to 100%",
				Source:    SourceHeuristic,
				Todo:      "Calibrate sector weights against TWSE sector index market cap weights 2025-2026",
			},
			MinAllocation: ParameterMetadata[float64]{
				Value:     0.02,
				Rationale: "Minimum 2% sector allocation prevents zero-weight exclusion of small sectors",
				Source:    SourceHeuristic,
			},
			MaxAllocation: ParameterMetadata[float64]{
				Value:     0.40,
				Rationale: "Maximum 40% single-sector allocation prevents over-concentration; ~2x semiconductor base weight (19%) allows tactical overweight",
				Source:    SourceHeuristic,
			},
			RebalanceThreshold: ParameterMetadata[float64]{
				Value:     0.01,
				Rationale: "1% rebalance threshold avoids excessive trading; sector weight deviation must exceed 1% to trigger rebalance",
				Source:    SourceHeuristic,
			},
		},
		StrategyEvolution: EngineStrategyEvolutionParameters{
			CooldownPeriodHours: ParameterMetadata[int]{
				Value:     24,
				Rationale: "24-hour cooldown between strategy switches prevents whipsaw during volatile regime transitions",
				Source:    SourceHeuristic,
				Todo:      "Test 24h vs 48h vs 72h cooldown impact on strategy stability",
			},
			Configs: ParameterMetadata[map[string]StrategyStateConfig]{
				Value: map[string]StrategyStateConfig{
					"normal":    {MaxPositionSize: 0.15, MaxSectorExposure: 0.30, MinCashReserve: 0.05, HedgeRatio: 0.0, AllowNewPositions: true, AllowConcentration: true},
					"cautious":  {MaxPositionSize: 0.12, MaxSectorExposure: 0.25, MinCashReserve: 0.10, HedgeRatio: 0.10, AllowNewPositions: true, AllowConcentration: false},
					"defensive": {MaxPositionSize: 0.08, MaxSectorExposure: 0.20, MinCashReserve: 0.20, HedgeRatio: 0.20, AllowNewPositions: false, AllowConcentration: false},
					"hedged":    {MaxPositionSize: 0.10, MaxSectorExposure: 0.25, MinCashReserve: 0.15, HedgeRatio: 0.30, AllowNewPositions: true, AllowConcentration: false},
					"suspended": {MaxPositionSize: 0.0, MaxSectorExposure: 0.0, MinCashReserve: 1.0, HedgeRatio: 0.0, AllowNewPositions: false, AllowConcentration: false},
				},
				Rationale: "Five strategy states from normal (full risk) to suspended (no positions); progressive de-risking: cautious → defensive → hedged → suspended",
				Source:    SourceHeuristic,
				Todo:      "Validate strategy state transitions with backtest across 2022 (bear), 2023-2024 (bull), 2025 (mixed) cycles",
			},
		},
		Executors: EngineExecutorsParameters{
			VIXMomentumCrashThreshold: ParameterMetadata[float64]{
				Value:     30.0,
				Rationale: "VIX above 30 signals momentum crash regime; TW market foreign flow sensitivity increases ~3x above this level",
				Source:    SourceEmpirical,
			},
			CrowdingPenaltyAgents3: ParameterMetadata[float64]{
				Value:     0.75,
				Rationale: "When 3 agents crowd same stock, conviction reduced to 75% to prevent herding; based on Kelly criterion adjustment for correlated bets",
				Source:    SourceHeuristic,
			},
			CrowdingPenaltyAgents4: ParameterMetadata[float64]{
				Value:     0.60,
				Rationale: "When 4+ agents crowd same stock, conviction reduced to 60%; exponential penalty as crowding increases",
				Source:    SourceHeuristic,
			},
			MinTradeAmount: ParameterMetadata[float64]{
				Value:     100000.0,
				Rationale: "100K TWD minimum trade amount filters out uneconomically small orders; ~$3K USD at current FX",
				Source:    SourceHeuristic,
			},
			MaxStocksDefault: ParameterMetadata[int]{
				Value:     8,
				Rationale: "Default 8 stocks max per session balances diversification with position concentration; aligned with baseline MaxOpenPositions (5) + buffer",
				Source:    SourceHeuristic,
			},
			MaxStocksMin: ParameterMetadata[int]{
				Value:     5,
				Rationale: "Minimum 5 stocks ensures basic sector diversification across semiconductor, AI, financials, shipping, and one defensive",
				Source:    SourceHeuristic,
			},
			MaxStocksMax: ParameterMetadata[int]{
				Value:     12,
				Rationale: "Maximum 12 stocks prevents over-diversification diluting alpha; above 12, marginal diversification benefit < transaction cost",
				Source:    SourceLiterature,
			},
			ConvictionFloorDefault: ParameterMetadata[int]{
				Value:     50,
				Rationale: "Minimum 50 conviction (out of 100) for executor recommendations to be considered; below 50 is effectively a coin flip",
				Source:    SourceHeuristic,
				Todo:      "Calibrate floor against executor historical precision-recall curve",
			},
		},
		Simulation: EngineSimulationParameters{
			NeutralRegimeSizingFactor: ParameterMetadata[float64]{
				Value:     0.85,
				Rationale: "85% position sizing in neutral regime reduces max position from full weight to 85%; provides 15% buffer for regime uncertainty",
				Source:    SourceHeuristic,
				Todo:      "Calibrate sizing factor [0.70-1.00] via walk-forward backtest per regime",
			},
		},
	}
}
