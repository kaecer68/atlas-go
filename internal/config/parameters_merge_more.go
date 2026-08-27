package config

// This file contains merge helpers for sub-structs whose default fields would
// otherwise stay zero when an empty or partial JSON config is loaded, causing
// Validate() to fail. Each helper mirrors the per-field conditional pattern of
// mergeNarrativeDefaults (parameters_merge.go) so that:
//   - If the loaded JSON has a non-zero value for a field, it is preserved.
//   - If the loaded JSON has a zero value (i.e. the field was not specified),
//     the field is replaced with the default value.
//
// Together, these helpers guarantee that LoadParametersConfig("empty.json")
// returns a config that is deep-equal to DefaultParametersConfig() modulo
// UpdatedAt (see TestParametersConfig_RoundTrip_EmptyJSON_KnownGap).

func mergeDarwinianDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Darwinian
	d := &cfg.Darwinian
	if d.WeightMin.Value == 0 {
		d.WeightMin = def.WeightMin
	}
	if d.WeightMax.Value == 0 {
		d.WeightMax = def.WeightMax
	}
	if d.WeightNeutral.Value == 0 {
		d.WeightNeutral = def.WeightNeutral
	}
	if d.TopQuartileMultiplier.Value == 0 {
		d.TopQuartileMultiplier = def.TopQuartileMultiplier
	}
	if d.BottomQuartileMultiplier.Value == 0 {
		d.BottomQuartileMultiplier = def.BottomQuartileMultiplier
	}
	if d.DailyAdjustmentCooldown.Value == "" {
		d.DailyAdjustmentCooldown = def.DailyAdjustmentCooldown
	}
	if d.LookbackDays.Value == 0 {
		d.LookbackDays = def.LookbackDays
	}
	if d.EMAAlpha.Value == 0 {
		d.EMAAlpha = def.EMAAlpha
	}
	if d.SharpeNormalizeDenom.Value == 0 {
		d.SharpeNormalizeDenom = def.SharpeNormalizeDenom
	}
	if d.MaxPerformanceBonusPct.Value == 0 {
		d.MaxPerformanceBonusPct = def.MaxPerformanceBonusPct
	}
	if d.VolatilityPenaltyThreshold.Value == 0 {
		d.VolatilityPenaltyThreshold = def.VolatilityPenaltyThreshold
	}
	if d.VolatilityPenaltyMultiplier.Value == 0 {
		d.VolatilityPenaltyMultiplier = def.VolatilityPenaltyMultiplier
	}
	if d.RiskVolatilityThreshold.Value == 0 {
		d.RiskVolatilityThreshold = def.RiskVolatilityThreshold
	}
	if d.RiskMultiplier.Value == 0 {
		d.RiskMultiplier = def.RiskMultiplier
	}
	if d.HitRateHighThreshold.Value == 0 {
		d.HitRateHighThreshold = def.HitRateHighThreshold
	}
	if d.HitRateLowThreshold.Value == 0 {
		d.HitRateLowThreshold = def.HitRateLowThreshold
	}
	if d.MiddleTierBoostMultiplier.Value == 0 {
		d.MiddleTierBoostMultiplier = def.MiddleTierBoostMultiplier
	}
	if d.MiddleTierCutMultiplier.Value == 0 {
		d.MiddleTierCutMultiplier = def.MiddleTierCutMultiplier
	}
	if d.SharpeMinSampleSize.Value == 0 {
		d.SharpeMinSampleSize = def.SharpeMinSampleSize
	}
	if d.MinUniqueReturnsForSharpe.Value == 0 {
		d.MinUniqueReturnsForSharpe = def.MinUniqueReturnsForSharpe
	}
	if d.StdDevMeanRatioThreshold.Value == 0 {
		d.StdDevMeanRatioThreshold = def.StdDevMeanRatioThreshold
	}
	if d.ConvictionClampMin.Value == 0 {
		d.ConvictionClampMin = def.ConvictionClampMin
	}
	if d.ConvictionClampMax.Value == 0 {
		d.ConvictionClampMax = def.ConvictionClampMax
	}
	if d.ZeroSignalPenaltyMultiplier.Value == 0 {
		d.ZeroSignalPenaltyMultiplier = def.ZeroSignalPenaltyMultiplier
	}
	if d.ZeroSignalPenaltyAfterDays.Value == 0 {
		d.ZeroSignalPenaltyAfterDays = def.ZeroSignalPenaltyAfterDays
	}
	if d.LossPenaltyMultiplier.Value == 0 {
		d.LossPenaltyMultiplier = def.LossPenaltyMultiplier
	}
	if d.WeightChangeAlertThreshold.Value == 0 {
		d.WeightChangeAlertThreshold = def.WeightChangeAlertThreshold
	}
}

func mergeFactorDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Factor
	f := &cfg.Factor
	if f.MomentumLookbackDays.Value == 0 {
		f.MomentumLookbackDays = def.MomentumLookbackDays
	}
	if f.MomentumStdDevDivisor.Value == 0 {
		f.MomentumStdDevDivisor = def.MomentumStdDevDivisor
	}
	if f.MomentumIntradayDiscount.Value == 0 {
		f.MomentumIntradayDiscount = def.MomentumIntradayDiscount
	}
	if f.MomentumIntradayThreshold.Value == 0 {
		f.MomentumIntradayThreshold = def.MomentumIntradayThreshold
	}
	if f.ValuePERangeCenter.Value == 0 {
		f.ValuePERangeCenter = def.ValuePERangeCenter
	}
	if f.ValuePERangeWidth.Value == 0 {
		f.ValuePERangeWidth = def.ValuePERangeWidth
	}
	if f.ValuePBRangeCenter.Value == 0 {
		f.ValuePBRangeCenter = def.ValuePBRangeCenter
	}
	if f.ValuePBRangeWidth.Value == 0 {
		f.ValuePBRangeWidth = def.ValuePBRangeWidth
	}
	if f.ValuePSRangeCenter.Value == 0 {
		f.ValuePSRangeCenter = def.ValuePSRangeCenter
	}
	if f.ValuePSRangeWidth.Value == 0 {
		f.ValuePSRangeWidth = def.ValuePSRangeWidth
	}
	if f.QualityDividendYieldCap.Value == 0 {
		f.QualityDividendYieldCap = def.QualityDividendYieldCap
	}
	if f.QualityVolatilityStd.Value == 0 {
		f.QualityVolatilityStd = def.QualityVolatilityStd
	}
	if f.QualityFallbackScore.Value == 0 {
		f.QualityFallbackScore = def.QualityFallbackScore
	}
	if f.ValueFallbackScore.Value == 0 {
		f.ValueFallbackScore = def.ValueFallbackScore
	}
	if len(f.InstitutionalSentimentWeights.Value) == 0 {
		f.InstitutionalSentimentWeights = def.InstitutionalSentimentWeights
	}
	if f.FallbackWeightReduction.Value == 0 {
		f.FallbackWeightReduction = def.FallbackWeightReduction
	}
}

func mergeOptimizerDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Optimizer
	o := &cfg.Optimizer
	if o.MaxPositionPct.Value == 0 {
		o.MaxPositionPct = def.MaxPositionPct
	}
	if o.MaxSectorPct.Value == 0 {
		o.MaxSectorPct = def.MaxSectorPct
	}
	if o.MaxTurnoverDaily.Value == 0 {
		o.MaxTurnoverDaily = def.MaxTurnoverDaily
	}
	if o.TargetBeta.Value == 0 {
		o.TargetBeta = def.TargetBeta
	}
	if o.BetaRangeMin.Value == 0 {
		o.BetaRangeMin = def.BetaRangeMin
	}
	if o.BetaRangeMax.Value == 0 {
		o.BetaRangeMax = def.BetaRangeMax
	}
	if o.MinTradeSize.Value == 0 {
		o.MinTradeSize = def.MinTradeSize
	}
	if o.CashReserve.Value == 0 {
		o.CashReserve = def.CashReserve
	}
	if len(o.FactorWeights.Value) == 0 {
		o.FactorWeights = def.FactorWeights
	}
}

func mergeSizingDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Sizing
	s := &cfg.Sizing
	if s.KellyFraction.Value == 0 {
		s.KellyFraction = def.KellyFraction
	}
	if s.VolLookbackDays.Value == 0 {
		s.VolLookbackDays = def.VolLookbackDays
	}
	if s.MaxPositionByADV.Value == 0 {
		s.MaxPositionByADV = def.MaxPositionByADV
	}
	if s.MaxDrawdownLimit.Value == 0 {
		s.MaxDrawdownLimit = def.MaxDrawdownLimit
	}
	if s.ATRMultiplier.Value == 0 {
		s.ATRMultiplier = def.ATRMultiplier
	}
	if s.CorrelationPenalty.Value == 0 {
		s.CorrelationPenalty = def.CorrelationPenalty
	}
	if s.CorrelationThreshold.Value == 0 {
		s.CorrelationThreshold = def.CorrelationThreshold
	}
	if s.DefaultWinRate.Value == 0 {
		s.DefaultWinRate = def.DefaultWinRate
	}
	if s.DefaultPayoffRatio.Value == 0 {
		s.DefaultPayoffRatio = def.DefaultPayoffRatio
	}
	if s.TargetVolatility.Value == 0 {
		s.TargetVolatility = def.TargetVolatility
	}
	if s.VolAdjustmentMin.Value == 0 {
		s.VolAdjustmentMin = def.VolAdjustmentMin
	}
	if s.VolAdjustmentMax.Value == 0 {
		s.VolAdjustmentMax = def.VolAdjustmentMax
	}
	if s.ATRTargetRisk.Value == 0 {
		s.ATRTargetRisk = def.ATRTargetRisk
	}
	if s.ATRAdjustmentMin.Value == 0 {
		s.ATRAdjustmentMin = def.ATRAdjustmentMin
	}
	if s.ATRAdjustmentMax.Value == 0 {
		s.ATRAdjustmentMax = def.ATRAdjustmentMax
	}
	if s.CorrelationPenaltyFactor.Value == 0 {
		s.CorrelationPenaltyFactor = def.CorrelationPenaltyFactor
	}
	if s.MaxCorrelationPenalty.Value == 0 {
		s.MaxCorrelationPenalty = def.MaxCorrelationPenalty
	}
	if s.DefaultVolatility.Value == 0 {
		s.DefaultVolatility = def.DefaultVolatility
	}
	if s.DefaultADV.Value == 0 {
		s.DefaultADV = def.DefaultADV
	}
	if s.DefaultATR.Value == 0 {
		s.DefaultATR = def.DefaultATR
	}
}

func mergeExperimentDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Experiment
	e := &cfg.Experiment
	if e.MaturityLevel1Observations.Value == 0 {
		e.MaturityLevel1Observations = def.MaturityLevel1Observations
	}
	if e.MaturityLevel2Observations.Value == 0 {
		e.MaturityLevel2Observations = def.MaturityLevel2Observations
	}
	if e.MaturityLevel3Observations.Value == 0 {
		e.MaturityLevel3Observations = def.MaturityLevel3Observations
	}
	if e.ImprovementThreshold.Value == 0 {
		e.ImprovementThreshold = def.ImprovementThreshold
	}
	if e.WelchTTestThreshold.Value == 0 {
		e.WelchTTestThreshold = def.WelchTTestThreshold
	}
	if e.DrawdownProtectionRatio.Value == 0 {
		e.DrawdownProtectionRatio = def.DrawdownProtectionRatio
	}
	if e.VolatilityToleranceRatio.Value == 0 {
		e.VolatilityToleranceRatio = def.VolatilityToleranceRatio
	}
	if e.OOSWindowDays.Value == 0 {
		e.OOSWindowDays = def.OOSWindowDays
	}
	if e.SharpeStabilityThreshold.Value == 0 {
		e.SharpeStabilityThreshold = def.SharpeStabilityThreshold
	}
	if e.MaxFallbackRatio.Value == 0 {
		e.MaxFallbackRatio = def.MaxFallbackRatio
	}
	if e.FactorWeightDriftThreshold.Value == 0 {
		e.FactorWeightDriftThreshold = def.FactorWeightDriftThreshold
	}
	if e.WalkForwardEmbargoDays.Value == 0 {
		e.WalkForwardEmbargoDays = def.WalkForwardEmbargoDays
	}
}

func mergeBaselineDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Baseline
	b := &cfg.Baseline
	if b.StartingCash.Value == 0 {
		b.StartingCash = def.StartingCash
	}
	if b.MaxPositionWeight.Value == 0 {
		b.MaxPositionWeight = def.MaxPositionWeight
	}
	if b.MaxOpenPositions.Value == 0 {
		b.MaxOpenPositions = def.MaxOpenPositions
	}
	if b.MinTradableVolume.Value == 0 {
		b.MinTradableVolume = def.MinTradableVolume
	}
	if b.MinRecommendationConviction.Value == 0 {
		b.MinRecommendationConviction = def.MinRecommendationConviction
	}
	if b.TransactionCostBPS.Value == 0 {
		b.TransactionCostBPS = def.TransactionCostBPS
	}
	if b.DiscountedCommissionBps.Value == 0 {
		b.DiscountedCommissionBps = def.DiscountedCommissionBps
	}
	if b.CommissionDiscountThreshold.Value == 0 {
		b.CommissionDiscountThreshold = def.CommissionDiscountThreshold
	}
	if b.SlippageBPS.Value == 0 {
		b.SlippageBPS = def.SlippageBPS
	}
	if b.AvgTradingCost.Value == 0 {
		b.AvgTradingCost = def.AvgTradingCost
	}
	if b.ReserveCashFraction.Value == 0 {
		b.ReserveCashFraction = def.ReserveCashFraction
	}
	if !b.RequireCROPass.Value {
		b.RequireCROPass = def.RequireCROPass
	}
}

func mergeRiskDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Risk
	r := &cfg.Risk
	if r.VaRConfidenceLevel.Value == 0 {
		r.VaRConfidenceLevel = def.VaRConfidenceLevel
	}
	if r.VaRSecondaryConfidence.Value == 0 {
		r.VaRSecondaryConfidence = def.VaRSecondaryConfidence
	}
	if r.VaRAlertThreshold.Value == 0 {
		r.VaRAlertThreshold = def.VaRAlertThreshold
	}
	if r.VaRCriticalThreshold.Value == 0 {
		r.VaRCriticalThreshold = def.VaRCriticalThreshold
	}
	if r.ConsecutiveLossLimit.Value == 0 {
		r.ConsecutiveLossLimit = def.ConsecutiveLossLimit
	}
	if len(r.SectorConstraintsRiskOff.Value) == 0 {
		r.SectorConstraintsRiskOff = def.SectorConstraintsRiskOff
	}
	if len(r.SectorConstraintsCarryTrade.Value) == 0 {
		r.SectorConstraintsCarryTrade = def.SectorConstraintsCarryTrade
	}
	if len(r.SectorConstraintsSectorRotation.Value) == 0 {
		r.SectorConstraintsSectorRotation = def.SectorConstraintsSectorRotation
	}
	if r.MaxDrawdownPct.Value == 0 {
		r.MaxDrawdownPct = def.MaxDrawdownPct
	}
	if r.MaxPositionSize.Value == 0 {
		r.MaxPositionSize = def.MaxPositionSize
	}
	if r.MaxDailyLossPct.Value == 0 {
		r.MaxDailyLossPct = def.MaxDailyLossPct
	}
	if r.StopLoss.Value == 0 {
		r.StopLoss = def.StopLoss
	}
	if r.TakeProfit.Value == 0 {
		r.TakeProfit = def.TakeProfit
	}
	if r.MaxLossPerTrade.Value == 0 {
		r.MaxLossPerTrade = def.MaxLossPerTrade
	}
	if r.MaxTotalExposure.Value == 0 {
		r.MaxTotalExposure = def.MaxTotalExposure
	}
}

func mergeFactorWeightDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().FactorWeight
	fw := &cfg.FactorWeight
	if len(fw.BaseWeights.Value) == 0 {
		fw.BaseWeights = def.BaseWeights
	}
	if fw.RegimeBullMomentum.Value == 0 {
		fw.RegimeBullMomentum = def.RegimeBullMomentum
	}
	if fw.RegimeBullQuality.Value == 0 {
		fw.RegimeBullQuality = def.RegimeBullQuality
	}
	if fw.RegimeBullValue.Value == 0 {
		fw.RegimeBullValue = def.RegimeBullValue
	}
	if fw.RegimeBearQuality.Value == 0 {
		fw.RegimeBearQuality = def.RegimeBearQuality
	}
	if fw.RegimeBearValue.Value == 0 {
		fw.RegimeBearValue = def.RegimeBearValue
	}
	if fw.RegimeBearMomentum.Value == 0 {
		fw.RegimeBearMomentum = def.RegimeBearMomentum
	}
	if fw.RegimeHighVolLiquidity.Value == 0 {
		fw.RegimeHighVolLiquidity = def.RegimeHighVolLiquidity
	}
	if fw.RegimeHighVolMomentum.Value == 0 {
		fw.RegimeHighVolMomentum = def.RegimeHighVolMomentum
	}
	if fw.RegimeHighVolInstSent.Value == 0 {
		fw.RegimeHighVolInstSent = def.RegimeHighVolInstSent
	}
	if fw.SeverityCritical.Value == 0 {
		fw.SeverityCritical = def.SeverityCritical
	}
	if fw.SeverityHigh.Value == 0 {
		fw.SeverityHigh = def.SeverityHigh
	}
	if fw.SeverityMedium.Value == 0 {
		fw.SeverityMedium = def.SeverityMedium
	}
	if fw.SeverityLow.Value == 0 {
		fw.SeverityLow = def.SeverityLow
	}
	if fw.ClampMin.Value == 0 {
		fw.ClampMin = def.ClampMin
	}
	if fw.ClampMax.Value == 0 {
		fw.ClampMax = def.ClampMax
	}
	if fw.RiskOnMomentum.Value == 0 {
		fw.RiskOnMomentum = def.RiskOnMomentum
	}
	if fw.RiskOnQuality.Value == 0 {
		fw.RiskOnQuality = def.RiskOnQuality
	}
	if fw.RiskOffMomentum.Value == 0 {
		fw.RiskOffMomentum = def.RiskOffMomentum
	}
	if fw.RiskOffQuality.Value == 0 {
		fw.RiskOffQuality = def.RiskOffQuality
	}
	if fw.RiskOffLiquidity.Value == 0 {
		fw.RiskOffLiquidity = def.RiskOffLiquidity
	}
	if fw.ConservativeValue.Value == 0 {
		fw.ConservativeValue = def.ConservativeValue
	}
	if fw.ConservativeQuality.Value == 0 {
		fw.ConservativeQuality = def.ConservativeQuality
	}
	if fw.ConservativeMomentum.Value == 0 {
		fw.ConservativeMomentum = def.ConservativeMomentum
	}
	if fw.AggressiveMomentum.Value == 0 {
		fw.AggressiveMomentum = def.AggressiveMomentum
	}
	if fw.AggressiveInstSent.Value == 0 {
		fw.AggressiveInstSent = def.AggressiveInstSent
	}
	if fw.AggressiveValue.Value == 0 {
		fw.AggressiveValue = def.AggressiveValue
	}
	if fw.AggressiveQuality.Value == 0 {
		fw.AggressiveQuality = def.AggressiveQuality
	}
}

func mergeHealthDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Health
	h := &cfg.Health
	if h.MuteThreshold.Value == 0 {
		h.MuteThreshold = def.MuteThreshold
	}
	if h.UnmuteThreshold.Value == 0 {
		h.UnmuteThreshold = def.UnmuteThreshold
	}
	if h.AutoRecoverDays.Value == 0 {
		h.AutoRecoverDays = def.AutoRecoverDays
	}
	if h.MinSampleSize.Value == 0 {
		h.MinSampleSize = def.MinSampleSize
	}
	if h.NegativeSharpeThreshold.Value == 0 {
		h.NegativeSharpeThreshold = def.NegativeSharpeThreshold
	}
	if h.SharpeWeight.Value == 0 {
		h.SharpeWeight = def.SharpeWeight
	}
	if h.HitRateWeight.Value == 0 {
		h.HitRateWeight = def.HitRateWeight
	}
	if h.StreakWeight.Value == 0 {
		h.StreakWeight = def.StreakWeight
	}
	if h.MaxSharpe.Value == 0 {
		h.MaxSharpe = def.MaxSharpe
	}
	if h.MinSharpe.Value == 0 {
		h.MinSharpe = def.MinSharpe
	}
	if h.StreakMax.Value == 0 {
		h.StreakMax = def.StreakMax
	}
}

func mergeGARCHDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().GARCH
	g := &cfg.GARCH
	if g.Omega.Value == 0 {
		g.Omega = def.Omega
	}
	if g.Alpha.Value == 0 {
		g.Alpha = def.Alpha
	}
	if g.Beta.Value == 0 {
		g.Beta = def.Beta
	}
	if g.MaxHistory.Value == 0 {
		g.MaxHistory = def.MaxHistory
	}
	if g.CorrelationMinDays.Value == 0 {
		g.CorrelationMinDays = def.CorrelationMinDays
	}
	if g.SmoothingFactor.Value == 0 {
		g.SmoothingFactor = def.SmoothingFactor
	}
	if g.RebalanceThreshold.Value == 0 {
		g.RebalanceThreshold = def.RebalanceThreshold
	}
	if g.MinForecastDays.Value == 0 {
		g.MinForecastDays = def.MinForecastDays
	}
	if g.MaxHistoryPoints.Value == 0 {
		g.MaxHistoryPoints = def.MaxHistoryPoints
	}
	if g.HighVolThreshold.Value == 0 {
		g.HighVolThreshold = def.HighVolThreshold
	}
	if g.LowVolThreshold.Value == 0 {
		g.LowVolThreshold = def.LowVolThreshold
	}
	if g.ReduceMagnitude.Value == 0 {
		g.ReduceMagnitude = def.ReduceMagnitude
	}
	if g.IncreaseMagnitude.Value == 0 {
		g.IncreaseMagnitude = def.IncreaseMagnitude
	}
	if g.WeeklyRebalanceDays.Value == 0 {
		g.WeeklyRebalanceDays = def.WeeklyRebalanceDays
	}
}

func mergeOrchestratorDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Orchestrator
	o := &cfg.Orchestrator
	if o.ConvictionFloorDefault.Value == 0 {
		o.ConvictionFloorDefault = def.ConvictionFloorDefault
	}
	if o.SuperinvestorMinConviction.Value == 0 {
		o.SuperinvestorMinConviction = def.SuperinvestorMinConviction
	}
	if o.SuperinvestorConvictionBase.Value == 0 {
		o.SuperinvestorConvictionBase = def.SuperinvestorConvictionBase
	}
	if o.CROZScoreThreshold.Value == 0 {
		o.CROZScoreThreshold = def.CROZScoreThreshold
	}
	if o.SectorConcentrationThreshold.Value == 0 {
		o.SectorConcentrationThreshold = def.SectorConcentrationThreshold
	}
	if o.SectorConcentrationThresholdHigh.Value == 0 {
		o.SectorConcentrationThresholdHigh = def.SectorConcentrationThresholdHigh
	}
	if o.SectorConvictionMultiplier.Value == 0 {
		o.SectorConvictionMultiplier = def.SectorConvictionMultiplier
	}
	if o.CrowdedConvictionMultiplier.Value == 0 {
		o.CrowdedConvictionMultiplier = def.CrowdedConvictionMultiplier
	}
	if o.FactorWeightMomentum.Value == 0 {
		o.FactorWeightMomentum = def.FactorWeightMomentum
	}
	if o.FactorWeightValue.Value == 0 {
		o.FactorWeightValue = def.FactorWeightValue
	}
	if o.FactorWeightQuality.Value == 0 {
		o.FactorWeightQuality = def.FactorWeightQuality
	}
	if o.FactorWeightAgent.Value == 0 {
		o.FactorWeightAgent = def.FactorWeightAgent
	}
	if o.PRISMBoostMultiplier.Value == 0 {
		o.PRISMBoostMultiplier = def.PRISMBoostMultiplier
	}
	if o.PRISMBoostMin.Value == 0 {
		o.PRISMBoostMin = def.PRISMBoostMin
	}
	if o.PRISMBoostMax.Value == 0 {
		o.PRISMBoostMax = def.PRISMBoostMax
	}
	if o.PromotionMinObservations.Value == 0 {
		o.PromotionMinObservations = def.PromotionMinObservations
	}
	if o.PromotionSharpeThreshold.Value == 0 {
		o.PromotionSharpeThreshold = def.PromotionSharpeThreshold
	}
	if o.PromotionHitRateThreshold.Value == 0 {
		o.PromotionHitRateThreshold = def.PromotionHitRateThreshold
	}
	if o.RejectionSharpeThreshold.Value == 0 {
		o.RejectionSharpeThreshold = def.RejectionSharpeThreshold
	}
	if o.RejectionHitRateThreshold.Value == 0 {
		o.RejectionHitRateThreshold = def.RejectionHitRateThreshold
	}
	if len(o.SectorRotationMacroAdjustments.Value) == 0 {
		o.SectorRotationMacroAdjustments = def.SectorRotationMacroAdjustments
	}
	if len(o.SectorRotationFlowAdjustments.Value) == 0 {
		o.SectorRotationFlowAdjustments = def.SectorRotationFlowAdjustments
	}
	if !o.UseMLScoring.Value {
		o.UseMLScoring = def.UseMLScoring
	}
}

func mergeStrategyDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Strategy
	s := &cfg.Strategy
	if s.MinSwitchIntervalDays.Value == 0 {
		s.MinSwitchIntervalDays = def.MinSwitchIntervalDays
	}
	if s.SwitchThreshold.Value == 0 {
		s.SwitchThreshold = def.SwitchThreshold
	}
	if s.ScoreLookbackDays.Value == 0 {
		s.ScoreLookbackDays = def.ScoreLookbackDays
	}
	if s.FallbackStrategy.Value == "" {
		s.FallbackStrategy = def.FallbackStrategy
	}
}

func mergeJanusDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Janus
	j := &cfg.Janus
	if j.ShortWindowDays.Value == 0 {
		j.ShortWindowDays = def.ShortWindowDays
	}
	if j.MediumWindowDays.Value == 0 {
		j.MediumWindowDays = def.MediumWindowDays
	}
	if j.LongWindowDays.Value == 0 {
		j.LongWindowDays = def.LongWindowDays
	}
	if j.MaxHistoryDays.Value == 0 {
		j.MaxHistoryDays = def.MaxHistoryDays
	}
	if j.MinWeight.Value == 0 {
		j.MinWeight = def.MinWeight
	}
	if j.MaxWeight.Value == 0 {
		j.MaxWeight = def.MaxWeight
	}
	if j.NovelThreshold.Value == 0 {
		j.NovelThreshold = def.NovelThreshold
	}
	if j.HistoricalThreshold.Value == 0 {
		j.HistoricalThreshold = def.HistoricalThreshold
	}
	if j.EpsilonWeight.Value == 0 {
		j.EpsilonWeight = def.EpsilonWeight
	}
	if j.ShortWindowBlend.Value == 0 {
		j.ShortWindowBlend = def.ShortWindowBlend
	}
	if j.MediumWindowBlend.Value == 0 {
		j.MediumWindowBlend = def.MediumWindowBlend
	}
	if j.LongWindowBlend.Value == 0 {
		j.LongWindowBlend = def.LongWindowBlend
	}
	if j.HealthStaleHours.Value == 0 {
		j.HealthStaleHours = def.HealthStaleHours
	}
	if j.HealthWarnHours.Value == 0 {
		j.HealthWarnHours = def.HealthWarnHours
	}
}

func mergeMarketdataDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Marketdata
	m := &cfg.Marketdata
	if m.TWSEAPIRateLimit.Value == 0 {
		m.TWSEAPIRateLimit = def.TWSEAPIRateLimit
	}
	if m.TWSEAPIRateBurst.Value == 0 {
		m.TWSEAPIRateBurst = def.TWSEAPIRateBurst
	}
	if m.TWSEAPITimeoutSec.Value == 0 {
		m.TWSEAPITimeoutSec = def.TWSEAPITimeoutSec
	}
	if m.FubonIntradayLimit.Value == 0 {
		m.FubonIntradayLimit = def.FubonIntradayLimit
	}
	if m.FubonHistoricalLimit.Value == 0 {
		m.FubonHistoricalLimit = def.FubonHistoricalLimit
	}
	if m.FubonAPITimeoutSec.Value == 0 {
		m.FubonAPITimeoutSec = def.FubonAPITimeoutSec
	}
	if m.TEJCallsPerSecond.Value == 0 {
		m.TEJCallsPerSecond = def.TEJCallsPerSecond
	}
	if m.TEJAPITimeoutSec.Value == 0 {
		m.TEJAPITimeoutSec = def.TEJAPITimeoutSec
	}
	if m.FugleRateLimit.Value == 0 {
		m.FugleRateLimit = def.FugleRateLimit
	}
	if m.FugleAPITimeoutSec.Value == 0 {
		m.FugleAPITimeoutSec = def.FugleAPITimeoutSec
	}
	if m.BDIAPITimeoutSec.Value == 0 {
		m.BDIAPITimeoutSec = def.BDIAPITimeoutSec
	}
	if m.BDIEndpoint.Value == "" {
		m.BDIEndpoint = def.BDIEndpoint
	}
	if m.MaxRetryAttempts.Value == 0 {
		m.MaxRetryAttempts = def.MaxRetryAttempts
	}
	if m.RetryBackoffMs.Value == 0 {
		m.RetryBackoffMs = def.RetryBackoffMs
	}
}

func mergeRealtimeDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Realtime
	r := &cfg.Realtime
	if r.VolatilityThreshold.Value == 0 {
		r.VolatilityThreshold = def.VolatilityThreshold
	}
	if r.VolumeSpikeThreshold.Value == 0 {
		r.VolumeSpikeThreshold = def.VolumeSpikeThreshold
	}
	if r.PriceChangeThreshold.Value == 0 {
		r.PriceChangeThreshold = def.PriceChangeThreshold
	}
	if r.MinConfidence.Value == 0 {
		r.MinConfidence = def.MinConfidence
	}
	if r.WeightAdjustmentRate.Value == 0 {
		r.WeightAdjustmentRate = def.WeightAdjustmentRate
	}
	if r.MaxWeightChange.Value == 0 {
		r.MaxWeightChange = def.MaxWeightChange
	}
	if r.MinWeight.Value == 0 {
		r.MinWeight = def.MinWeight
	}
	if r.UpdateIntervalMs.Value == 0 {
		r.UpdateIntervalMs = def.UpdateIntervalMs
	}
}

func mergeNarrativeConvictionDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().NarrativeConviction
	nc := &cfg.NarrativeConviction
	if len(nc.ThemeHitRates.Value) == 0 {
		nc.ThemeHitRates = def.ThemeHitRates
	}
	if len(nc.SkillToTheme.Value) == 0 {
		nc.SkillToTheme = def.SkillToTheme
	}
}

func mergePreciousMetalsDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().PreciousMetals
	pm := &cfg.PreciousMetals
	if pm.CentralBankBuyingTrend.Value == "" {
		pm.CentralBankBuyingTrend = def.CentralBankBuyingTrend
	}
	if pm.CentralBankNetBuy.Value == 0 {
		pm.CentralBankNetBuy = def.CentralBankNetBuy
	}
	if pm.IndiaGoldImportsYoY.Value == 0 {
		pm.IndiaGoldImportsYoY = def.IndiaGoldImportsYoY
	}
	if pm.ChinaGoldImportsYoY.Value == 0 {
		pm.ChinaGoldImportsYoY = def.ChinaGoldImportsYoY
	}
	if pm.COMEXDefaultNetLong.Value == 0 {
		pm.COMEXDefaultNetLong = def.COMEXDefaultNetLong
	}
}

func mergeForwardReturnDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().ForwardReturn
	fr := &cfg.ForwardReturn
	if fr.RiskOnMean.Value == 0 {
		fr.RiskOnMean = def.RiskOnMean
	}
	if fr.RiskOffMean.Value == 0 {
		fr.RiskOffMean = def.RiskOffMean
	}
	if fr.RiskOnStdDev.Value == 0 {
		fr.RiskOnStdDev = def.RiskOnStdDev
	}
	if fr.RiskOffStdDev.Value == 0 {
		fr.RiskOffStdDev = def.RiskOffStdDev
	}
}

func mergeTaxDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Tax
	t := &cfg.Tax
	if t.DividendTaxRate.Value == 0 {
		t.DividendTaxRate = def.DividendTaxRate
	}
	if t.TransactionTaxRate.Value == 0 {
		t.TransactionTaxRate = def.TransactionTaxRate
	}
	if t.NHISurchargeRate.Value == 0 {
		t.NHISurchargeRate = def.NHISurchargeRate
	}
}

func mergeSectorAllocationDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().SectorAllocation
	sa := &cfg.SectorAllocation
	if sa.WeightFloor == 0 && sa.CycleWeight == 0 && sa.SeasonalWeight == 0 &&
		sa.LinkageWeight == 0 && sa.NarrativeWeight == 0 && sa.MacroWeight == 0 &&
		sa.FactorWeight == 0 && len(sa.BaseWeights) == 0 && len(sa.DerivationFactors) == 0 &&
		sa.Rationale == "" {
		*sa = def
	}
}

func mergeSmartUniverseDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().SmartUniverse
	su := &cfg.SmartUniverse
	if su.TopN.Value == 0 {
		su.TopN = def.TopN
	}
	if su.PEWeight.Value == 0 {
		su.PEWeight = def.PEWeight
	}
	if su.PBWeight.Value == 0 {
		su.PBWeight = def.PBWeight
	}
	if su.VolumeWeight.Value == 0 {
		su.VolumeWeight = def.VolumeWeight
	}
	if su.MomentumWeight.Value == 0 {
		su.MomentumWeight = def.MomentumWeight
	}
	if su.QualityWeight.Value == 0 {
		su.QualityWeight = def.QualityWeight
	}
	if su.ForeignFlowWeight.Value == 0 {
		su.ForeignFlowWeight = def.ForeignFlowWeight
	}
	if su.VolumeFloorTWD.Value == 0 {
		su.VolumeFloorTWD = def.VolumeFloorTWD
	}
	if su.MinDailyAmountTWD.Value == 0 {
		su.MinDailyAmountTWD = def.MinDailyAmountTWD
	}
	if su.MaxIndustryConcentration.Value == 0 {
		su.MaxIndustryConcentration = def.MaxIndustryConcentration
	}
	if su.PriceMinimum.Value == 0 {
		su.PriceMinimum = def.PriceMinimum
	}
	if su.FactorScoreMaxAgeDays.Value == 0 {
		su.FactorScoreMaxAgeDays = def.FactorScoreMaxAgeDays
	}
	if su.D6ExpiryTradingDays.Value == 0 {
		su.D6ExpiryTradingDays = def.D6ExpiryTradingDays
	}
	if su.VaRContributionMultiplier.Value == 0 {
		su.VaRContributionMultiplier = def.VaRContributionMultiplier
	}
	if su.VolatilityMultiplier.Value == 0 {
		su.VolatilityMultiplier = def.VolatilityMultiplier
	}
	if su.DrawdownWindow.Value == 0 {
		su.DrawdownWindow = def.DrawdownWindow
	}
	if su.DrawdownThreshold.Value == 0 {
		su.DrawdownThreshold = def.DrawdownThreshold
	}
	if su.ConfidenceThreshold.Value == 0 {
		su.ConfidenceThreshold = def.ConfidenceThreshold
	}
	if su.SupplyChainExpandDepth.Value == 0 {
		su.SupplyChainExpandDepth = def.SupplyChainExpandDepth
	}
}
