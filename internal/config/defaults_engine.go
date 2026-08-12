package config

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
			Value:     30,
			Rationale: "Minimum 30 observations for health assessment; 30 is the standard rule-of-thumb for CLT applicability",
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
		UseMLScoring: ParameterMetadata[bool]{
			Value:     false,
			Rationale: "Enable ML-based factor scoring from internal/ml models",
			Source:    SourceExperimental,
			Todo:      "Validate with A/B backtest before enabling",
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
		DiscountedCommissionBps: ParameterMetadata[float64]{
			Value:     8.5,
			Rationale: "Discounted electronic trading commission rate: 0.085% = 8.5 bps",
			Source:    SourceEmpirical,
		},
		CommissionDiscountThreshold: ParameterMetadata[float64]{
			Value:     500000,
			Rationale: "Minimum order notional (NTD) to qualify for discounted commission rate",
			Source:    SourceHeuristic,
		},
		SlippageBPS: ParameterMetadata[float64]{
			Value:     4.0,
			Rationale: "4 bps estimated slippage for market orders",
			Source:    SourceEmpirical,
		},
		AvgTradingCost: ParameterMetadata[float64]{
			Value:     0.00654,
			Rationale: "Aggregate round-trip trading cost for Taiwan equities: commission (0.1425% × broker discount ~0.6 × 2 sides) + slippage (0.1% × 2 sides) + market impact. Calibrated from TWSE empirical data.",
			Source:    SourceEmpirical,
			Todo:      "Recalibrate against rolling 12-month TWSE trading cost distribution",
		},
		ReserveCashFraction: ParameterMetadata[float64]{
			Value:     0.1,
			Rationale: "10% cash reserve for liquidity and opportunities",
			Source:    SourceLiterature,
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
		SlippageErrorBps: ParameterMetadata[float64]{
			Value:     100,
			Rationale: "Trade slippage error threshold (100 BPS = 1.0%) — emits ERROR severity alert",
			Source:    SourceHeuristic,
		},
		SlippageWarningBps: ParameterMetadata[float64]{
			Value:     50,
			Rationale: "Trade slippage warning threshold (50 BPS = 0.5%) — emits WARNING severity alert",
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
		HeartbeatTTLMinutes: ParameterMetadata[int]{
			Value:     5,
			Rationale: "Decision 1 (alert-redesign-v2.md Part 3.1): channel heartbeat staleness threshold (5 min default); channel health summaries older than this are considered 'down'",
			Source:    SourceHeuristic,
		},
		AlertSLACriticalSec: ParameterMetadata[int]{
			Value:     1800,
			Rationale: "Decision 9 (alert-redesign-v2.md Part 3.7): CRITICAL alert must be acknowledged within 30 min; otherwise emits a meta-alert",
			Source:    SourceHeuristic,
		},
		AlertSLAErrorSec: ParameterMetadata[int]{
			Value:     7200,
			Rationale: "Decision 9: ERROR alert must be acknowledged within 2 hours",
			Source:    SourceHeuristic,
		},
		AlertSLAWarningSec: ParameterMetadata[int]{
			Value:     86400,
			Rationale: "Decision 9: WARNING alert must be acknowledged within 24 hours",
			Source:    SourceHeuristic,
		},
		SLAViolationMetaAlert: ParameterMetadata[bool]{
			Value:     true,
			Rationale: "Decision 9: when enabled, SLA violations emit a CRITICAL meta-alert for visibility",
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
			MaxOpenPositions: ParameterMetadata[int]{
				Value:     5,
				Rationale: "最多同時持有 5 檔標的，控制集中度風險",
				Source:    SourceHeuristic,
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
			StrategicPrior: EngineStrategicPriorParameters{
				Weights: ParameterMetadata[map[string]float64]{
					Value: map[string]float64{
						"semiconductor":     0.3300,
						"electronics":       0.1600,
						"financials":        0.1300,
						"shipping":          0.0800,
						"optoelectronics":   0.01875,
						"cement":            0.01875,
						"plastics":          0.01875,
						"textiles":          0.01875,
						"steel":             0.01875,
						"food":              0.01875,
						"auto":              0.01875,
						"telecom":           0.01875,
						"chemicals":         0.01875,
						"biotech":           0.01875,
						"construction":      0.01875,
						"other_electronics": 0.01875,
						"machinery":         0.01875,
						"tourism":           0.01875,
						"retail":            0.01875,
						"energy":            0.01875,
					},
					Rationale: "C07 heuristic seed (event-driven sector predictor prior); 20 canonical L1 IDs aligned with industry.L1Sectors(); SA02 SA-INV-05 single source of truth; sum = 1.0",
					Source:    SourceHeuristic,
				},
				Source: ParameterMetadata[string]{
					Value:     "heuristic",
					Rationale: "Permanent lock: empirical upgrade out of SA02 plan scope (spec §4.1 + capital-flow CF-INV-13 require walk-forward validation, not in scope)",
					Source:    SourceHeuristic,
				},
				ModelVersion: ParameterMetadata[string]{
					Value:     "v0.0.0-c07-heuristic",
					Rationale: "Semver 2.0 (MAJOR.MINOR.PATCH-prerelease); single source of truth via StrategicPrior (SA02/SA04)",
					Source:    SourceHeuristic,
				},
				CalibrationStatus: ParameterMetadata[string]{
					Value:     "calibrating",
					Rationale: "Permanent lock during observation window; PromotionGate()=false until source=calibrated and status=calibrated (out of plan scope)",
					Source:    SourceHeuristic,
				},
				AsOfDate: ParameterMetadata[string]{
					Value:     "2026-07-17",
					Rationale: "SA02 implementation date; informational only",
					Source:    SourceHeuristic,
				},
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

func defaultRSITwParameters() RSITwParameters {
	return RSITwParameters{
		// Part A — Retail Sentiment (40% overall weight)
		A1Weight: ParameterMetadata[float64]{
			Value: 0.25, Rationale: "Margin Balance Δ Z-score contributes 25% to Part A score",
			Source: SourceHeuristic, Todo: "Calibrate from historical margin balance vs. forward return IC",
		},
		A2Weight: ParameterMetadata[float64]{
			Value: 0.20, Rationale: "Day Trading Ratio contributes 20% to Part A score",
			Source: SourceHeuristic,
		},
		A3Weight: ParameterMetadata[float64]{
			Value: 0.20, Rationale: "Margin Maintenance Proxy contributes 20% to Part A score",
			Source: SourceHeuristic,
		},
		A4Weight: ParameterMetadata[float64]{
			Value: 0.15, Rationale: "VIX Nonlinear Mapping contributes 15% to Part A score",
			Source: SourceLiterature,
		},
		A5Weight: ParameterMetadata[float64]{
			Value: 0.10, Rationale: "Weekly PCR Proxy contributes 10% to Part A score",
			Source: SourceHeuristic,
		},
		A6Weight: ParameterMetadata[float64]{
			Value: 0.10, Rationale: "Odd-Lot Trading contributes 10% to Part A score",
			Source: SourceHeuristic,
		},
		APartWeight: ParameterMetadata[float64]{
			Value: 0.40, Rationale: "Part A contributes 40% to final RSI-tw score",
			Source: SourceHeuristic, Todo: "Calibrate optimal A/C split via walk-forward",
		},
		CPartWeight: ParameterMetadata[float64]{
			Value: 0.25, Rationale: "Part C contributes 25% to final RSI-tw score",
			Source: SourceHeuristic, Todo: "Calibrate optimal A/C split via walk-forward",
		},

		// A3: Margin Maintenance formula
		A3Midpoint: ParameterMetadata[float64]{
			Value: 0.5, Rationale: "50th percentile is the neutral midpoint for maintenance ratio",
			Source: SourceHeuristic,
		},
		A3Scale: ParameterMetadata[float64]{
			Value: 2.0, Rationale: "Scale factor transforms percentile deviation to Z-score in [-1, 1]",
			Source: SourceHeuristic,
		},

		// A4: VIX piecewise mapping
		A4VixThresholds: ParameterMetadata[[]float64]{
			Value:     []float64{15, 20, 25, 30, 35},
			Rationale: "Standard VIX ranges: calm (<15), low (15-20), moderate (20-25), elevated (25-30), high (30-35), extreme (>35)",
			Source:    SourceLiterature, Todo: "Validate thresholds against Taiwan VIX equivalent (TVIX) distribution",
		},
		A4VixScores: ParameterMetadata[[]float64]{
			Value:     []float64{0.1, 0.0, -0.3, -0.5, -0.7, -1.0},
			Rationale: "Audit A10 (2026-08-12)：VIX 高 = 市場恐慌 → 散戶恐慌 → 推低分數（與 composite +frenzy/-fear 語義一致）。低 VIX 輕微樂觀 (+0.1)，高 VIX 恐慌 (-1.0)",
			Source:    SourceHeuristic, Todo: "Calibrate scores from historical VIX vs. TWSE forward return",
		},

		// A5: PCR piecewise mapping
		A5PcrThresholds: ParameterMetadata[[]float64]{
			Value:     []float64{1.5, 1.0, 0.8},
			Rationale: "Standard PCR interpretation: >1.5 very bearish, >1.0 bearish, >0.8 neutral, <0.8 bullish",
			Source:    SourceHeuristic, Todo: "Calibrate against TAIFEX PCR historical distribution",
		},
		A5PcrScores: ParameterMetadata[[]float64]{
			Value:     []float64{-0.9, -0.5, -0.2, 0.3},
			Rationale: "Audit A10 (2026-08-12)：PCR 高（賣權成交多）= 散戶避險/看空 → 恐慌 → 推低分數；PCR 低（買權成交多）= 樂觀 → 推高。原 [0.9,0.7,0.5,0.1] 把恐慌訊號推高狂熱分數",
			Source:    SourceHeuristic,
		},
		A5PcrFallback: ParameterMetadata[float64]{
			Value: 0.0, Rationale: "Neutral score when PCR data is unavailable (0)——Audit A10：資料缺失不假裝中性 0.5，回 0 不貢獻",
			Source: SourceHeuristic,
		},

		// A6: Odd-lot imbalance mapping
		A6OddLotThresholds: ParameterMetadata[[]float64]{
			Value:     []float64{0.2, 0.1, -0.1, -0.2},
			Rationale: "Odd-lot imbalance ranges: heavy retail buying (>0.2), moderate (>0.1), neutral (-0.1 to 0.1), selling (<-0.1), heavy selling (<-0.2)",
			Source:    SourceHeuristic, Todo: "Calibrate thresholds from TWSE odd-lot historical distribution",
		},
		A6OddLotScores: ParameterMetadata[[]float64]{
			Value:     []float64{0.85, 0.65, 0.5, 0.35, 0.15},
			Rationale: "Score mapping: heavy buying (0.85 bearish) → heavy selling (0.15 bullish)",
			Source:    SourceHeuristic,
		},
		A6OddLotFallback: ParameterMetadata[float64]{
			Value: 0.5, Rationale: "Neutral score when odd-lot data is unavailable (0)",
			Source: SourceHeuristic,
		},

		// Part C — Institutional / Derivative Flow
		C1Weight: ParameterMetadata[float64]{
			Value: 0.40, Rationale: "Small TAIEX Futures OI contributes 40% to Part C score",
			Source: SourceHeuristic, Todo: "Calibrate from historical futures OI vs. forward return IC",
		},
		C2Weight: ParameterMetadata[float64]{
			Value: 0.35, Rationale: "Foreign/Inst Net Flow contributes 35% to Part C score",
			Source: SourceHeuristic, Todo: "Calibrate from historical institutional flow vs. forward return IC",
		},
		C3Weight: ParameterMetadata[float64]{
			Value: 0.25, Rationale: "ETF Net Subscription contributes 25% to Part C score",
			Source: SourceHeuristic, Todo: "Calibrate from historical ETF flow vs. forward return IC",
		},
		C1VeryBullishThreshold: ParameterMetadata[float64]{
			Value:     5,
			Rationale: "非前十大交易人多空差 pct above 5 → 散戶淨多頭明顯 (0.9 bearish score)。Audit A15 (2026-08-12)：RetailFuturesPct 實測量級約 ±10（前十大佔 OI 60-70%），原 20/10/-10/-20 threshold 幾乎永不觸發 → C1 恆等 0.5",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from 2Y historical futures OI distribution",
		},
		C1BullishThreshold: ParameterMetadata[float64]{
			Value:     2,
			Rationale: "非前十大交易人多空差 pct above 2 → 散戶淨多 (0.7)",
			Source:    SourceHeuristic,
		},
		C1BearishThreshold: ParameterMetadata[float64]{
			Value:     -2,
			Rationale: "非前十大交易人多空差 pct below -2 → 散戶淨空 (0.5 neutral)",
			Source:    SourceHeuristic,
		},
		C1VeryBearishThreshold: ParameterMetadata[float64]{
			Value:     -5,
			Rationale: "非前十大交易人多空差 pct below -5 → 散戶淨空明顯 (0.25 bullish)",
			Source:    SourceHeuristic,
		},
		C1FallbackScore: ParameterMetadata[float64]{
			Value:     0.5,
			Rationale: "Neutral score when futures OI data is unavailable (0)。Audit A02 (2026-08-12)：原 literal 0.5 參數化",
			Source:    SourceHeuristic,
		},
		C2NeutralMidpoint: ParameterMetadata[float64]{
			Value:     0.5,
			Rationale: "Institutional net flow ≈ 0 → neutral midpoint 0.5",
			Source:    SourceHeuristic,
			Todo:      "Calibrate from 2Y foreign/domestic fund net flow distribution",
		},
		C2NetflowScalingFactor: ParameterMetadata[float64]{
			Value:     10,
			Rationale: "Net flow (億股) divided by 10 to get deviation from neutral; score clamped [0.1, 0.9]。Audit A07 (2026-08-12)：ForeignInvestorNet.Value 單位為億股（T86 股數/1e8），原 scaling 1e9 假設 TWD 元 → netFlow 5.67 億股 ÷ 1e9 ≈ 0 → 恆等 0.5 無鑑別力",
			Source:    SourceHeuristic,
			Todo:      "Learn optimal scaling factor from historical flow distributions",
		},
		C3VeryBullishThreshold: ParameterMetadata[float64]{
			Value:     1_000_000_000,
			Rationale: "ETF net subscription above 1B TWD → heavy retail inflow (0.9 bearish)",
			Source:    SourceHeuristic,
		},
		C3BullishThreshold: ParameterMetadata[float64]{
			Value:     100_000_000,
			Rationale: "ETF net subscription above 100M TWD → moderate inflow (0.7)",
			Source:    SourceHeuristic,
		},
		C3BearishThreshold: ParameterMetadata[float64]{
			Value:     -100_000_000,
			Rationale: "ETF net subscription below -100M TWD → outflow (0.45)",
			Source:    SourceHeuristic,
		},

		// Part D — Event-Driven Adjustment Factors
		DGeoPoliticalRiskThreshold: ParameterMetadata[float64]{
			Value:     0.5,
			Rationale: "Geopolitical risk index above 0.5 → apply 0.85x multiplier",
			Source:    SourceHeuristic,
		},
		DGeoPoliticalRiskMultiplier: ParameterMetadata[float64]{
			Value:     0.85,
			Rationale: "15% sentiment reduction during elevated geopolitical risk",
			Source:    SourceHeuristic,
			Todo:      "Calibrate against actual market drawdowns during geopolitical events",
		},
		DVIXSpikeThreshold: ParameterMetadata[float64]{
			Value:     30,
			Rationale: "VIX above 30 → spike regime, apply 0.90x multiplier",
			Source:    SourceLiterature,
			Todo:      "Validate with Taiwan VIX equivalent (TVIX or VIX futures)",
		},
		DVIXSpikeMultiplier: ParameterMetadata[float64]{
			Value:     0.90,
			Rationale: "10% sentiment reduction during VIX spike >30",
			Source:    SourceHeuristic,
		},
		DCreditTighteningMultiplier: ParameterMetadata[float64]{
			Value:     0.80,
			Rationale: "20% sentiment reduction when credit tightening signal active",
			Source:    SourceHeuristic,
			Todo:      "Wire credit tightening signal to actual central bank / margin rate data",
		},
	}
}

func defaultSmartUniverseParams() SmartUniverseConfig {
	return SmartUniverseConfig{
		TopN: ParameterMetadata[int]{
			Value:     150,
			Rationale: "Default universe size of 150 names; Oracle recommends 120-180 dynamic range, with bear-market override to 100-120",
			Source:    SourceHeuristic,
		},
		PEWeight: ParameterMetadata[float64]{
			Value:     0.15,
			Rationale: "Oracle-calibrated weight for valuation factor; retained despite Taiwan PE distortion from semiconductors",
			Source:    SourceCalibrated,
		},
		PBWeight: ParameterMetadata[float64]{
			Value:     0.10,
			Rationale: "Oracle-calibrated weight for book-value factor; important for financials screening",
			Source:    SourceCalibrated,
		},
		VolumeWeight: ParameterMetadata[float64]{
			Value:     0.15,
			Rationale: "Oracle-calibrated weight reduced from 0.20 to 0.15 to make room for the foreign-flow factor",
			Source:    SourceCalibrated,
		},
		MomentumWeight: ParameterMetadata[float64]{
			Value:     0.15,
			Rationale: "Oracle-calibrated weight reduced sharply from 0.30 to 0.15 because Taiwan momentum reverses quickly",
			Source:    SourceCalibrated,
		},
		QualityWeight: ParameterMetadata[float64]{
			Value:     0.20,
			Rationale: "Oracle-calibrated weight combining ROE, ROA and accruals quality",
			Source:    SourceCalibrated,
		},
		ForeignFlowWeight: ParameterMetadata[float64]{
			Value:     0.20,
			Rationale: "Oracle-calibrated new foreign-investor flow factor; core driver for Taiwan equities",
			Source:    SourceCalibrated,
		},
		VolumeFloorTWD: ParameterMetadata[float64]{
			Value:     10_000_000.0,
			Rationale: "Minimum daily turnover of NT$10M replaces share-count floor to avoid low-priced stocks passing a 100-lot screen",
			Source:    SourceHeuristic,
		},
		MinDailyAmountTWD: ParameterMetadata[float64]{
			Value:     5_000_000.0,
			Rationale: "Relaxed Layer-2.5 re-check floor of NT$5M for liquidity verification before final inclusion",
			Source:    SourceHeuristic,
		},
		MaxIndustryConcentration: ParameterMetadata[float64]{
			Value:     0.40,
			Rationale: "Oracle-recommended 40% cap to avoid semiconductor over-concentration in the universe",
			Source:    SourceHeuristic,
		},
		PriceMinimum: ParameterMetadata[float64]{
			Value:     10.0,
			Rationale: "Minimum stock price of NT$10 for universe eligibility",
			Source:    SourceHeuristic,
		},
		FactorScoreMaxAgeDays: ParameterMetadata[int]{
			Value:     30,
			Rationale: "Factor scores older than 30 days are downgraded to avoid stale inputs",
			Source:    SourceHeuristic,
		},
		D6ExpiryTradingDays: ParameterMetadata[int]{
			Value:     60,
			Rationale: "Unselected Layer-2 candidates remain on the D6 watchlist for 60 trading days before expiry",
			Source:    SourceHeuristic,
		},
		VaRContributionMultiplier: ParameterMetadata[float64]{
			Value:     2.0,
			Rationale: "Exclude names with VaR contribution above 2x the portfolio average",
			Source:    SourceHeuristic,
		},
		VolatilityMultiplier: ParameterMetadata[float64]{
			Value:     2.0,
			Rationale: "Exclude names with 30-day realized volatility above 2x the median",
			Source:    SourceHeuristic,
		},
		DrawdownWindow: ParameterMetadata[int]{
			Value:     60,
			Rationale: "60-day window for drawdown flagging",
			Source:    SourceHeuristic,
		},
		DrawdownThreshold: ParameterMetadata[float64]{
			Value:     0.30,
			Rationale: "30% drawdown threshold for risk flag (flag only, not exclusion)",
			Source:    SourceHeuristic,
		},
		ConfidenceThreshold: ParameterMetadata[int]{
			Value:     3,
			Rationale: "Minimum of 3 supporting signals or confidence points required for inclusion",
			Source:    SourceHeuristic,
		},
		SupplyChainExpandDepth: ParameterMetadata[int]{
			Value:     2,
			Rationale: "Expand supply-chain mapping up to 2 hops when building representative stock sets",
			Source:    SourceHeuristic,
		},
	}
}

func defaultFallbackPriceTargets() map[string]FallbackPriceTarget {
	return map[string]FallbackPriceTarget{
		"semiconductor_desk": {
			TargetMultiplier:   ParameterMetadata[float64]{Value: 1.06, Rationale: "6% upside target for semiconductor sector", Source: SourceHeuristic},
			StopLossMultiplier: ParameterMetadata[float64]{Value: 0.95, Rationale: "5% stop-loss for semiconductor sector", Source: SourceHeuristic},
		},
		"ai_supply_chain_desk": {
			TargetMultiplier:   ParameterMetadata[float64]{Value: 1.08, Rationale: "8% upside target for AI supply chain", Source: SourceHeuristic},
			StopLossMultiplier: ParameterMetadata[float64]{Value: 0.95, Rationale: "5% stop-loss for AI supply chain", Source: SourceHeuristic},
		},
		"etf_rotation_desk": {
			TargetMultiplier:   ParameterMetadata[float64]{Value: 1.04, Rationale: "4% upside target for ETF rotation", Source: SourceHeuristic},
			StopLossMultiplier: ParameterMetadata[float64]{Value: 0.97, Rationale: "3% stop-loss for ETF rotation", Source: SourceHeuristic},
		},
		"financials_desk": {
			TargetMultiplier:   ParameterMetadata[float64]{Value: 1.05, Rationale: "5% upside target for financials", Source: SourceHeuristic},
			StopLossMultiplier: ParameterMetadata[float64]{Value: 0.96, Rationale: "4% stop-loss for financials", Source: SourceHeuristic},
		},
		"shipping_desk": {
			TargetMultiplier:   ParameterMetadata[float64]{Value: 1.07, Rationale: "7% upside target for shipping sector", Source: SourceHeuristic},
			StopLossMultiplier: ParameterMetadata[float64]{Value: 0.94, Rationale: "6% stop-loss for shipping sector", Source: SourceHeuristic},
		},
		"growth_momentum": {
			TargetMultiplier:   ParameterMetadata[float64]{Value: 1.08, Rationale: "8% upside target for growth momentum", Source: SourceHeuristic},
			StopLossMultiplier: ParameterMetadata[float64]{Value: 0.95, Rationale: "5% stop-loss for growth momentum", Source: SourceHeuristic},
		},
		"value_yield": {
			TargetMultiplier:   ParameterMetadata[float64]{Value: 1.05, Rationale: "5% upside target for value/yield", Source: SourceHeuristic},
			StopLossMultiplier: ParameterMetadata[float64]{Value: 0.96, Rationale: "4% stop-loss for value/yield", Source: SourceHeuristic},
		},
		"earnings_quality": {
			TargetMultiplier:   ParameterMetadata[float64]{Value: 1.06, Rationale: "6% upside target for earnings quality", Source: SourceHeuristic},
			StopLossMultiplier: ParameterMetadata[float64]{Value: 0.95, Rationale: "5% stop-loss for earnings quality", Source: SourceHeuristic},
		},
		"technical_breakout": {
			TargetMultiplier:   ParameterMetadata[float64]{Value: 1.10, Rationale: "10% upside target for technical breakout", Source: SourceHeuristic},
			StopLossMultiplier: ParameterMetadata[float64]{Value: 0.94, Rationale: "6% stop-loss for technical breakout", Source: SourceHeuristic},
		},
		"alpha_discovery": {
			TargetMultiplier:   ParameterMetadata[float64]{Value: 1.06, Rationale: "6% upside target for alpha discovery", Source: SourceHeuristic},
			StopLossMultiplier: ParameterMetadata[float64]{Value: 0.95, Rationale: "5% stop-loss for alpha discovery", Source: SourceHeuristic},
		},
		"_default": {
			TargetMultiplier:   ParameterMetadata[float64]{Value: 1.05, Rationale: "5% default upside target", Source: SourceHeuristic},
			StopLossMultiplier: ParameterMetadata[float64]{Value: 0.95, Rationale: "5% default stop-loss", Source: SourceHeuristic},
		},
	}
}

func deriveDefaultSectorAllocationConfig() SectorAllocationConfig {
	return SectorAllocationConfig{
		Rationale: "Multi-factor sector weight model: base × cycle × seasonal × linkage × narrative × macro × factor. Replaces three independent hard-coded weight sources (industry.calculateWeightDerivation, portfolio.sector_weights, orchestrator.base_allocations) with one auditable pipeline.",
		Source:    SourceHeuristic,
		Citation: &ParameterCitation{
			SourceType:       "heuristic",
			SourceReference:  "internal/sectorallocation/ + internal/industry/ + internal/portfolio/ unification",
			EvidenceQuality:  "medium",
			UpdatePolicy:     "recalibrate when backtest drift > 5% over 20 sessions",
			ValidationMethod: "Compare adjusted weights against historical regime accuracy (n>=30 sessions)",
			Dependencies:     []string{"darwinian.weight_min/max", "engine.sector_rotation.base_allocations"},
			LastValidated:    "2026-06-14",
		},
		BaseWeights: map[string]float64{
			"semiconductor": 0.30,
			"financials":    0.14,
			"electronics":   0.10,
			"materials":     0.08,
			"industrials":   0.07,
			"consumer":      0.06,
			"healthcare":    0.05,
			"energy":        0.05,
			"telecom":       0.05,
			"utilities":     0.04,
			"real_estate":   0.04,
			"_cash_reserve": 0.02,
		},
		DerivationFactors: map[string][]WeightFactorConfig{
			"semiconductor": {
				{Factor: "出口比重", Weight: 0.35, Source: "財政部海關統計", Evidence: "佔台灣總出口 35%+，晶片法案受惠"},
				{Factor: "景氣循環", Weight: 0.25, Source: "國發會景氣對策信號", Evidence: "與全球半導體銷售週期同步"},
				{Factor: "AI需求", Weight: 0.20, Source: "Gartner / TSMA 預估", Evidence: "AI 加速器 2025-2027 CAGR >40%"},
				{Factor: "地緣政治", Weight: 0.20, Source: "美中科技戰 / 出口管制", Evidence: "CHIPS Act + 台灣風險溢價"},
			},
			"financials": {
				{Factor: "升息循環", Weight: 0.40, Source: "Fed 政策利率 / 台灣央行", Evidence: "淨利差擴張"},
				{Factor: "信用品質", Weight: 0.30, Source: "金管會逾放比", Evidence: "M2 / 房貸 / 企業債"},
				{Factor: "保險需求", Weight: 0.20, Source: "保發中心", Evidence: "高利率保單吸引力回升"},
				{Factor: "台股量能", Weight: 0.10, Source: "TWSE 月成交", Evidence: "經紀/手續費收入"},
			},
			"electronics": {
				{Factor: "消費電子循環", Weight: 0.35, Source: "全球 PMI / 手機出貨", Evidence: "iPhone / 筆電 / 平板"},
				{Factor: "車用電子", Weight: 0.30, Source: "EV 滲透率 / Tier1 訂單", Evidence: "ADAS / 車載娛樂"},
				{Factor: "伺服器 / 資料中心", Weight: 0.25, Source: "雲端資本支出", Evidence: "Hyperscaler capex 成長"},
				{Factor: "存儲 / 被動元件", Weight: 0.10, Source: "DRAM / MLCC 報價", Evidence: "週期性觸底回升"},
			},
			"materials": {
				{Factor: "原物料價格", Weight: 0.40, Source: "CRB / LME / 鋼鐵指數", Evidence: "鐵礦 / 銅 / 鋁"},
				{Factor: "中國需求", Weight: 0.30, Source: "中國 PMI / 地產", Evidence: "基礎建設 + 房地產"},
				{Factor: "匯率", Weight: 0.20, Source: "TWD/USD", Evidence: "出口競爭力"},
				{Factor: "ESG / 碳關稅", Weight: 0.10, Source: "CBAM 進度", Evidence: "高碳排產業成本上升"},
			},
			"industrials": {
				{Factor: "資本支出循環", Weight: 0.35, Source: "全球半導體 capex", Evidence: "廠房 / 設備 / 自動化"},
				{Factor: "自動化需求", Weight: 0.25, Source: "工業機器人出貨", Evidence: "缺工 + 智慧製造"},
				{Factor: "航運 / 物流", Weight: 0.25, Source: "SCFI / BDI / 運價", Evidence: "全球貿易量"},
				{Factor: "軍工 / 政府支出", Weight: 0.15, Source: "國防預算", Evidence: "地緣政治升溫"},
			},
			"consumer": {
				{Factor: "內需景氣", Weight: 0.40, Source: "台灣零售 / 餐飲營收", Evidence: "可支配所得 / 消費信心"},
				{Factor: "觀光復甦", Weight: 0.25, Source: "來台旅客", Evidence: "跨境消費回升"},
				{Factor: "電商 / 跨境", Weight: 0.20, Source: "momo / PChome / 蝦皮", Evidence: "滲透率提升"},
				{Factor: "薪資成長", Weight: 0.15, Source: "主計處經常性薪資", Evidence: "基本工資調升"},
			},
			"healthcare": {
				{Factor: "人口老化", Weight: 0.35, Source: "國發會人口推估", Evidence: "65+ 比例突破 20%"},
				{Factor: "新藥 / CDMO", Weight: 0.30, Source: "TFDA / FDA 核准", Evidence: "海外授權金收入"},
				{Factor: "醫材出口", Weight: 0.20, Source: "醫材公會", Evidence: "高值化醫材成長"},
				{Factor: "健保政策", Weight: 0.15, Source: "健保署", Evidence: "給付 / 核價制度"},
			},
			"energy": {
				{Factor: "國際油價", Weight: 0.40, Source: "Brent / WTI", Evidence: "OPEC+ 產量政策"},
				{Factor: "綠能轉型", Weight: 0.30, Source: "離岸風電 / 太陽能裝置量", Evidence: "RE100 企業需求"},
				{Factor: "地緣政治", Weight: 0.20, Source: "中東局勢 / 俄烏", Evidence: "能源安全溢價"},
				{Factor: "碳權 / 碳交易", Weight: 0.10, Source: "國際碳價", Evidence: "減碳成本內部化"},
			},
			"telecom": {
				{Factor: "5G / 寬頻滲透", Weight: 0.40, Source: "NCC", Evidence: "FTTH / 行動上網"},
				{Factor: "資費競爭", Weight: 0.25, Source: "NCC 資費審查", Evidence: "ARPU 變化"},
				{Factor: "企業專網 / IDC", Weight: 0.20, Source: "企業用戶", Evidence: "AI / 雲端需求"},
				{Factor: "股利穩定度", Weight: 0.15, Source: "現金股利殖利率", Evidence: "防禦性配置"},
			},
			"utilities": {
				{Factor: "電價 / 費率", Weight: 0.40, Source: "經濟部電價審議", Evidence: "成本反映機制"},
				{Factor: "綠能投資", Weight: 0.25, Source: "再生能源占比", Evidence: "2025 再生能源 20% 目標"},
				{Factor: "資本支出", Weight: 0.20, Source: "台電 / 民營電廠", Evidence: "電網強韌 + 儲能"},
				{Factor: "防禦性需求", Weight: 0.15, Source: "高股息 / 低波動", Evidence: "退休 / 收益型配置"},
			},
			"real_estate": {
				{Factor: "利率敏感度", Weight: 0.40, Source: "央行利率 / 房貸", Evidence: "房貸利率 >2.5% 衝擊"},
				{Factor: "政策調控", Weight: 0.25, Source: "信用管制 / 囤房稅", Evidence: "選擇性信用管制"},
				{Factor: "供需結構", Weight: 0.20, Source: "使照 / 開工", Evidence: "人口集中度"},
				{Factor: "商辦 / 廠辦", Weight: 0.15, Source: "空置率 / 租金", Evidence: "科技業擴廠需求"},
			},
		},
		CycleWeight:     1.0,
		SeasonalWeight:  1.0,
		LinkageWeight:   1.0,
		NarrativeWeight: 1.0,
		MacroWeight:     1.0,
		FactorWeight:    1.0,
		WeightFloor:     0.01,
	}
}

func deriveDefaultReportingConfig() ReportingParameters {
	return ReportingParameters{
		WinRateThreshold: ParameterMetadata[float64]{
			Value:     0.002,
			Rationale: "0.2% cost-adjusted threshold for win-rate classification. Covers transaction cost (~0.15% in TW market) + slippage buffer. ForwardReturn must exceed this to count as a win.",
			Source:    SourceHeuristic,
		},
		SharpeMinSamples: ParameterMetadata[int]{
			Value:     5,
			Rationale: "Minimum 5 per-agent forward-return samples before SharpeLike is reported. Below this, statistical significance is poor and the value is shown as N/A on the frontend.",
			Source:    SourceHeuristic,
		},
	}
}
