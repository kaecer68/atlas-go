package config

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
			Value:     240,
			Rationale: "Per-outcome sampling v2 (2026-08-27): the window holds raw outcomes (not day-means), so 240 entries cover ~8-30 trading days depending on daily recommendation volume (9-31/day observed). 240 keeps multi-day diversity for the unique-returns guard while staying responsive.",
			Source:    SourceHeuristic,
			Todo:      "Calibrate: 240 vs 480 (60d) for TW market agent population",
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
			Value:     20,
			Rationale: "20 samples minimum for statistically stable Sharpe (per-outcome sampling v2, 2026-08-27: window holds 10-30x more samples than v1 day-mean). Matches min-unique guard floor.",
			Source:    SourceLiterature,
			Todo:      "Calibrate: test 20 vs 30 vs 60 for TW market agent population",
		},
		MinUniqueReturnsForSharpe: ParameterMetadata[int]{
			Value:     10,
			Rationale: "10 distinct values minimum for Sharpe validity (per-outcome v2, 2026-08-27). v1 day-mean used 8; per-outcome yields 14-30 unique in 30d window so 10 is a safe degenerate guard.",
			Source:    SourceHeuristic,
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
		ZeroSignalPenaltyMultiplier: ParameterMetadata[float64]{
			Value:     0.9,
			Rationale: "10% extra daily cut for agents with zero signals for ZeroSignalPenaltyAfterDays; stops long-silent agents from idling at initial weight in the middle tier (B3)",
			Source:    SourceHeuristic,
		},
		ZeroSignalPenaltyAfterDays: ParameterMetadata[int]{
			Value:     14,
			Rationale: "14 days without a single signal marks an agent as long-silent; after this it receives the zero-signal penalty (B3)",
			Source:    SourceHeuristic,
		},
		LossPenaltyMultiplier: ParameterMetadata[float64]{
			Value:     0.9,
			Rationale: "10% extra daily cut for bottom-tier agents with negative Sharpe and >=30 signals; deepens the penalty only on statistically meaningful losses computed on the A4-corrected per-outcome Sharpe (B3)",
			Source:    SourceHeuristic,
		},
		WeightChangeAlertThreshold: ParameterMetadata[float64]{
			Value:     0.15,
			Rationale: "Absolute daily weight change above 0.15 triggers a darwinian weight-change alert (logging.Warn + event-bus health alert) (B3)",
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
				"foreign":  0.45,
				"domestic": 0.30,
				"margin":   0.20,
				"retail":   0.05,
			},
			Rationale: "Foreign flow 45%, domestic 30%, margin balance 20%, retail sentiment 5% — retail sentiment from RSI-tw calculator feeds into institutional sentiment as contrarian signal",
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
			Value:     0.5,
			Rationale: "Half-Kelly (0.5) per Thorp (2006) — balances growth and drawdown under parameter uncertainty.",
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
		WalkForwardEmbargoDays: ParameterMetadata[int]{
			Value:     5,
			Rationale: "5-trading-day embargo gap between walk-forward train/test folds and between primary training window and OOS window. Prevents data leakage from corporate-event reorgs and event-driven labeling lag.",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from leakage-sensitive backtest: test [3, 7, 10] range vs hit rate stability",
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
			PricePenalty:             ParameterMetadata[int]{Value: 8, Rationale: "Intraday weakness penalty: when last < open the closing buyer lost control, signaling potential distribution in growth names that rely on sustained buying pressure", Source: SourceHeuristic, Todo: "Calibrate 6/8/10 via cross-executor backtest sweep comparing forward returns on penalized vs non-penalized picks"},
			TrendConfirmationPenalty: ParameterMetadata[int]{Value: 15, Rationale: "Heavy penalty when prompt demands trend confirmation but price closes below open, indicating the trend hypothesis is contradicted by intraday price action", Source: SourceHeuristic, Todo: "Backtest 12/15/18 to measure false-trend-filter precision at each penalty level"},
			DowngradePricePenalty:    ParameterMetadata[int]{Value: 12, Rationale: "Price-based downgrade penalty: when last falls below high×threshold (0.995), the stock failed to hold its peak, signaling potential reversal from a downgrade event", Source: SourceHeuristic, Todo: "Calibrate 10/12/14: compare downgrade penalty hit rate against post-downgrade 5-day forward returns"},
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
