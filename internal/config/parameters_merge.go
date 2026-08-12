package config

// mergeNarrativeDefaults fills zero-valued narrative fields with defaults.
// Last field (CalibrationEnabled) is unconditional — the default is false
// and we always want it set from defaults, even when loaded config is true.
func mergeNarrativeDefaults(cfg *ParametersConfig) {
	def := DefaultParametersConfig().Narrative
	n := &cfg.Narrative

	if n.MinTrendStrength.Value == 0 {
		n.MinTrendStrength = def.MinTrendStrength
	}
	if n.MinConfidence.Value == 0 {
		n.MinConfidence = def.MinConfidence
	}
	if n.MinHitRate.Value == 0 {
		n.MinHitRate = def.MinHitRate
	}
	if n.OverrideThreshold.Value == 0 {
		n.OverrideThreshold = def.OverrideThreshold
	}
	if n.AIRevenueGrowthThreshold.Value == 0 {
		n.AIRevenueGrowthThreshold = def.AIRevenueGrowthThreshold
	}
	if n.CoWoSUtilizationThreshold.Value == 0 {
		n.CoWoSUtilizationThreshold = def.CoWoSUtilizationThreshold
	}
	if n.CapexGrowthThreshold.Value == 0 {
		n.CapexGrowthThreshold = def.CapexGrowthThreshold
	}
	if n.US10YChangeBpsThreshold.Value == 0 {
		n.US10YChangeBpsThreshold = def.US10YChangeBpsThreshold
	}
	if n.DXYChangePctThreshold.Value == 0 {
		n.DXYChangePctThreshold = def.DXYChangePctThreshold
	}
	if n.GeopoliticalGPRThreshold.Value == 0 {
		n.GeopoliticalGPRThreshold = def.GeopoliticalGPRThreshold
	}
	if n.OilChangePctThreshold.Value == 0 {
		n.OilChangePctThreshold = def.OilChangePctThreshold
	}
	if n.JPYChangePctThreshold.Value == 0 {
		n.JPYChangePctThreshold = def.JPYChangePctThreshold
	}
	if n.VIXLevelThreshold.Value == 0 {
		n.VIXLevelThreshold = def.VIXLevelThreshold
	}
	if n.ModelLookbackDays.Value == 0 {
		n.ModelLookbackDays = def.ModelLookbackDays
	}
	if n.ModelHoldWindowDays.Value == 0 {
		n.ModelHoldWindowDays = def.ModelHoldWindowDays
	}
	if n.InflationEstimate.Value == 0 {
		n.InflationEstimate = def.InflationEstimate
	}
	if len(n.EventTTLMultiplier.Value) == 0 {
		n.EventTTLMultiplier = def.EventTTLMultiplier
	}
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
	if n.TaiwanStressDXYWeight.Value == 0 {
		n.TaiwanStressDXYWeight = def.TaiwanStressDXYWeight
	}
	if n.TaiwanStressUS10YWeight.Value == 0 {
		n.TaiwanStressUS10YWeight = def.TaiwanStressUS10YWeight
	}
	if n.TaiwanStressForeignWeight.Value == 0 {
		n.TaiwanStressForeignWeight = def.TaiwanStressForeignWeight
	}
	if n.TaiwanStressVIXWeight.Value == 0 {
		n.TaiwanStressVIXWeight = def.TaiwanStressVIXWeight
	}
	if n.TaiwanStressJPYWeight.Value == 0 {
		n.TaiwanStressJPYWeight = def.TaiwanStressJPYWeight
	}
	if n.TaiwanStressGeoWeight.Value == 0 {
		n.TaiwanStressGeoWeight = def.TaiwanStressGeoWeight
	}
	if n.TaiwanStressOilWeight.Value == 0 {
		n.TaiwanStressOilWeight = def.TaiwanStressOilWeight
	}
	if n.TaiwanStressGoldWeight.Value == 0 {
		n.TaiwanStressGoldWeight = def.TaiwanStressGoldWeight
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
	if a.SlippageErrorBps.Value == 0 {
		a.SlippageErrorBps = def.SlippageErrorBps
	}
	if a.SlippageWarningBps.Value == 0 {
		a.SlippageWarningBps = def.SlippageWarningBps
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
	if a.HeartbeatTTLMinutes.Value == 0 {
		a.HeartbeatTTLMinutes = def.HeartbeatTTLMinutes
	}
	if a.AlertSLACriticalSec.Value == 0 {
		a.AlertSLACriticalSec = def.AlertSLACriticalSec
	}
	if a.AlertSLAErrorSec.Value == 0 {
		a.AlertSLAErrorSec = def.AlertSLAErrorSec
	}
	if a.AlertSLAWarningSec.Value == 0 {
		a.AlertSLAWarningSec = def.AlertSLAWarningSec
	}
	// SLAViolationMetaAlert is a bool; fall back to default only if Value
	// is false AND Source is empty (unset). This distinguishes "explicitly
	// false" from "not set".
	if !a.SLAViolationMetaAlert.Value && a.SLAViolationMetaAlert.Source == "" {
		a.SLAViolationMetaAlert = def.SLAViolationMetaAlert
	}
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
	if i.CustomerConcentrationLimit.Value == 0 {
		i.CustomerConcentrationLimit = def.CustomerConcentrationLimit
	}
	if i.GeographicExposureLimit.Value == 0 {
		i.GeographicExposureLimit = def.GeographicExposureLimit
	}
	if i.ConfidenceSignal.Value.SignalBase == 0 {
		i.ConfidenceSignal = def.ConfidenceSignal
	}
	if i.ConfidenceMix.Value.WeightBoundary == 0 {
		i.ConfidenceMix = def.ConfidenceMix
	}
	if len(i.SeasonalPatterns.Value) == 0 {
		i.SeasonalPatterns = def.SeasonalPatterns
	}
	if i.PhaseScores.Value.ScoreExpansion == 0 {
		i.PhaseScores = def.PhaseScores
	}
	if len(i.SkillToIndustry.Value) == 0 {
		i.SkillToIndustry = def.SkillToIndustry
	}
	if i.LinkageParams.Value.DownstreamDecayFactor == 0 {
		i.LinkageParams = def.LinkageParams
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
	if r.C1FallbackScore.Value == 0 {
		r.C1FallbackScore = def.C1FallbackScore
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
