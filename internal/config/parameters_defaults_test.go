package config

import (
	"testing"
)

// ---------------------------------------------------------------------------
// mergeNarrativeDefaults (line 2354)
// Fills zero-valued Narrative fields with defaults.
// ---------------------------------------------------------------------------

func TestMergeNarrativeDefaults(t *testing.T) {
	def := DefaultParametersConfig().Narrative

	t.Run("case_A_empty_input", func(t *testing.T) {
		cfg := &ParametersConfig{}
		mergeNarrativeDefaults(cfg)

		n := cfg.Narrative
		if n.GoldChangePctThreshold.Value == 0 {
			t.Errorf("GoldChangePctThreshold not populated from defaults")
		}
		if n.USDTWDChangePctThreshold.Value == 0 {
			t.Errorf("USDTWDChangePctThreshold not populated from defaults")
		}
		if n.SemiconductorExportDropThreshold.Value == 0 {
			t.Errorf("SemiconductorExportDropThreshold not populated from defaults")
		}
		if n.RetailMarginZScoreThreshold.Value == 0 {
			t.Errorf("RetailMarginZScoreThreshold not populated from defaults")
		}
		if n.AICapexSentimentThreshold.Value == 0 {
			t.Errorf("AICapexSentimentThreshold not populated from defaults")
		}
		if n.ConfidenceBaseGeopolitical.Value == 0 {
			t.Errorf("ConfidenceBaseGeopolitical not populated from defaults")
		}
		if n.GoldChangePctThreshold.Value != def.GoldChangePctThreshold.Value {
			t.Errorf("GoldChangePctThreshold = %v; want %v", n.GoldChangePctThreshold.Value, def.GoldChangePctThreshold.Value)
		}
		if n.CalibrationEnabled.Value != def.CalibrationEnabled.Value {
			t.Errorf("CalibrationEnabled = %v; want %v", n.CalibrationEnabled.Value, def.CalibrationEnabled.Value)
		}
	})

	t.Run("case_B_partial_override", func(t *testing.T) {
		cfg := &ParametersConfig{}
		cfg.Narrative.GoldChangePctThreshold.Value = 0.99
		cfg.Narrative.ConfidenceBaseUSRates.Value = 0.88

		mergeNarrativeDefaults(cfg)

		n := cfg.Narrative
		if n.GoldChangePctThreshold.Value != 0.99 {
			t.Errorf("GoldChangePctThreshold was overwritten: got %v, want 0.99", n.GoldChangePctThreshold.Value)
		}
		if n.ConfidenceBaseUSRates.Value != 0.88 {
			t.Errorf("ConfidenceBaseUSRates was overwritten: got %v, want 0.88", n.ConfidenceBaseUSRates.Value)
		}
		if n.USDTWDChangePctThreshold.Value == 0 {
			t.Errorf("USDTWDChangePctThreshold still zero after merge")
		}
		if n.SemiconductorExportDropThreshold.Value == 0 {
			t.Errorf("SemiconductorExportDropThreshold still zero after merge")
		}
	})

	t.Run("case_C_full_override", func(t *testing.T) {
		cfg := &ParametersConfig{}
		cfg.Narrative.GoldChangePctThreshold.Value = 1.1
		cfg.Narrative.USDTWDChangePctThreshold.Value = 2.2
		cfg.Narrative.SemiconductorExportDropThreshold.Value = 3.3
		cfg.Narrative.RetailMarginZScoreThreshold.Value = 4.4
		cfg.Narrative.AICapexSentimentThreshold.Value = 5.5
		cfg.Narrative.TSMCRevenueYoYThreshold.Value = 6.6
		cfg.Narrative.TaiwanStressUSDTWDThreshold.Value = 7.7
		cfg.Narrative.RetailInstitutionalDivergenceThreshold.Value = 8.8
		cfg.Narrative.AICapexNegativeSentimentThreshold.Value = 9.9
		cfg.Narrative.AICapexFallbackSentiment.Value = 10.1
		cfg.Narrative.TSMCRevenuePositiveThreshold.Value = 11.2
		cfg.Narrative.ConfidenceBaseUSRates.Value = 12.3
		cfg.Narrative.ConfidenceBaseJPYCarry.Value = 13.4
		cfg.Narrative.ConfidenceBaseGeopolitical.Value = 14.5
		cfg.Narrative.ConfidenceBaseOilShock.Value = 15.6
		cfg.Narrative.ConfidenceBaseAICapex.Value = 16.7
		cfg.Narrative.ConfidenceBaseTSMCRevenue.Value = 17.8
		cfg.Narrative.ConfidenceBaseTaiwanStress.Value = 18.9
		cfg.Narrative.ConfidenceDeviationCeiling.Value = 19.0
		cfg.Narrative.SOXIndexDropThreshold.Value = 20.1
		cfg.Narrative.RetailFrenzyPercentileThreshold.Value = 21.2
		cfg.Narrative.RetailFearPercentileThreshold.Value = 22.3
		cfg.Narrative.RetailAccelerationWindowDays.Value = 23
		cfg.Narrative.SpringFestivalConfidence.Value = 24.4
		cfg.Narrative.ElectionCycleConfidence.Value = 25.5
		cfg.Narrative.EarningsBlackoutConfidence.Value = 26.6
		cfg.Narrative.TechPeakSeasonConfidence.Value = 27.7
		cfg.Narrative.YearEndWindowDressingConfidence.Value = 28.8
		cfg.Narrative.EarningsSurpriseConfidence.Value = 29.9
		cfg.Narrative.EarningsSurpriseThreshold.Value = 30.0
		cfg.Narrative.TaiwanStressDXYScale.Value = 31.1
		cfg.Narrative.TaiwanStressUS10YScale.Value = 32.2
		cfg.Narrative.TaiwanStressForeignScale.Value = 33.3
		cfg.Narrative.TaiwanStressVIXScale.Value = 34.4
		cfg.Narrative.TaiwanStressJPYScale.Value = 35.5
		cfg.Narrative.TaiwanStressGeoScale.Value = 36.6
		cfg.Narrative.TaiwanStressOilScale.Value = 37.7
		cfg.Narrative.TaiwanStressGoldScale.Value = 38.8
		cfg.Narrative.TaiwanStressAlertThreshold.Value = 39.9
		cfg.Narrative.TaiwanStressHighThreshold.Value = 40.0
		cfg.Narrative.TaiwanStressCrisisThreshold.Value = 41.1
		cfg.Narrative.CalibrationBaselineWindow.Value = 42
		cfg.Narrative.CalibrationTargetMedian.Value = 43.3
		cfg.Narrative.CalibrationValidationPct.Value = 44.4
		cfg.Narrative.CalibrationMinRecords.Value = 45
		cfg.Narrative.CalibrationEnabled.Value = true

		mergeNarrativeDefaults(cfg)

		n := cfg.Narrative
		if n.GoldChangePctThreshold.Value != 1.1 {
			t.Errorf("GoldChangePctThreshold overwritten: got %v, want 1.1", n.GoldChangePctThreshold.Value)
		}
		if n.USDTWDChangePctThreshold.Value != 2.2 {
			t.Errorf("USDTWDChangePctThreshold overwritten: got %v, want 2.2", n.USDTWDChangePctThreshold.Value)
		}
		if n.CalibrationEnabled.Value != false {
			t.Errorf("CalibrationEnabled not overwritten to default: got %v, want false", n.CalibrationEnabled.Value)
		}
	})
}

// ---------------------------------------------------------------------------
// mergeDrawdownDefaults (line 2497)
// Fills zero-valued Drawdown fields with defaults.
// ---------------------------------------------------------------------------

func TestMergeDrawdownDefaults(t *testing.T) {
	def := DefaultParametersConfig().Drawdown

	t.Run("case_A_empty_input", func(t *testing.T) {
		cfg := &ParametersConfig{}
		mergeDrawdownDefaults(cfg)

		d := cfg.Drawdown
		if d.LightPercentage.Value == 0 {
			t.Errorf("LightPercentage not populated from defaults")
		}
		if d.ModeratePercentage.Value == 0 {
			t.Errorf("ModeratePercentage not populated from defaults")
		}
		if d.SeverePercentage.Value == 0 {
			t.Errorf("SeverePercentage not populated from defaults")
		}
		if d.EmergencyPercentage.Value == 0 {
			t.Errorf("EmergencyPercentage not populated from defaults")
		}
		if d.LightPercentage.Value != def.LightPercentage.Value {
			t.Errorf("LightPercentage = %v; want %v", d.LightPercentage.Value, def.LightPercentage.Value)
		}
	})

	t.Run("case_B_partial_override", func(t *testing.T) {
		cfg := &ParametersConfig{}
		cfg.Drawdown.NonePercentage.Value = 99.0
		cfg.Drawdown.SevereMaxExposure.Value = 0.05

		mergeDrawdownDefaults(cfg)

		d := cfg.Drawdown
		if d.NonePercentage.Value != 99.0 {
			t.Errorf("NonePercentage was overwritten: got %v, want 99.0", d.NonePercentage.Value)
		}
		if d.SevereMaxExposure.Value != 0.05 {
			t.Errorf("SevereMaxExposure was overwritten: got %v, want 0.05", d.SevereMaxExposure.Value)
		}
		if d.LightPercentage.Value == 0 {
			t.Errorf("LightPercentage still zero after merge")
		}
		if d.ModeratePercentage.Value == 0 {
			t.Errorf("ModeratePercentage still zero after merge")
		}
	})

	t.Run("case_C_full_override", func(t *testing.T) {
		cfg := &ParametersConfig{}
		cfg.Drawdown.NonePercentage.Value = 11.1
		cfg.Drawdown.NoneMaxExposure.Value = 22.2
		cfg.Drawdown.LightPercentage.Value = 33.3
		cfg.Drawdown.LightMaxExposure.Value = 44.4
		cfg.Drawdown.ModeratePercentage.Value = 55.5
		cfg.Drawdown.ModerateMaxExposure.Value = 66.6
		cfg.Drawdown.SeverePercentage.Value = 77.7
		cfg.Drawdown.SevereMaxExposure.Value = 88.8
		cfg.Drawdown.EmergencyPercentage.Value = 99.9
		cfg.Drawdown.EmergencyMaxExposure.Value = 100.1
		cfg.Drawdown.OrangeOverrideMinScore.Value = 0.25
		cfg.Drawdown.RedOverrideMinScore.Value = 0.15
		cfg.Drawdown.SectorConstraintsRiskOff.Value = map[string]float64{"TECH": 0.5}
		cfg.Drawdown.SectorConstraintsCarryTradeUnwind.Value = map[string]float64{"FIN": 0.3}
		cfg.Drawdown.SectorConstraintsSectorRotation.Value = map[string]float64{"ENERGY": 0.2}

		mergeDrawdownDefaults(cfg)

		d := cfg.Drawdown
		if d.NonePercentage.Value != 11.1 {
			t.Errorf("NonePercentage overwritten: got %v, want 11.1", d.NonePercentage.Value)
		}
		if d.NoneMaxExposure.Value != 22.2 {
			t.Errorf("NoneMaxExposure overwritten: got %v, want 22.2", d.NoneMaxExposure.Value)
		}
		if d.SectorConstraintsRiskOff.Value["TECH"] != 0.5 {
			t.Errorf("SectorConstraintsRiskOff overwritten: got %v", d.SectorConstraintsRiskOff.Value)
		}
	})
}

// ---------------------------------------------------------------------------
// mergeAlertDefaults (line 2549)
// Fills zero-valued Alert fields with defaults.
// ---------------------------------------------------------------------------

func TestMergeAlertDefaults(t *testing.T) {
	def := DefaultParametersConfig().Alert

	t.Run("case_A_empty_input", func(t *testing.T) {
		cfg := &ParametersConfig{}
		mergeAlertDefaults(cfg)

		a := cfg.Alert
		if a.MinCashThreshold.Value == 0 {
			t.Errorf("MinCashThreshold not populated from defaults")
		}
		if a.MaxPositionsCount.Value == 0 {
			t.Errorf("MaxPositionsCount not populated from defaults")
		}
		if a.MaxPositionWeightPct.Value == 0 {
			t.Errorf("MaxPositionWeightPct not populated from defaults")
		}
		if a.MinCashThreshold.Value != def.MinCashThreshold.Value {
			t.Errorf("MinCashThreshold = %v; want %v", a.MinCashThreshold.Value, def.MinCashThreshold.Value)
		}
	})

	t.Run("case_B_partial_override", func(t *testing.T) {
		cfg := &ParametersConfig{}
		cfg.Alert.MaxPositionsCount.Value = 25
		cfg.Alert.RuleEngineCooldownSec.Value = 300

		mergeAlertDefaults(cfg)

		a := cfg.Alert
		if a.MaxPositionsCount.Value != 25 {
			t.Errorf("MaxPositionsCount was overwritten: got %v, want 25", a.MaxPositionsCount.Value)
		}
		if a.RuleEngineCooldownSec.Value != 300 {
			t.Errorf("RuleEngineCooldownSec was overwritten: got %v, want 300", a.RuleEngineCooldownSec.Value)
		}
		if a.MinCashThreshold.Value == 0 {
			t.Errorf("MinCashThreshold still zero after merge")
		}
		if a.DailyLossWarningPct.Value == 0 {
			t.Errorf("DailyLossWarningPct still zero after merge")
		}
	})

	t.Run("case_C_full_override", func(t *testing.T) {
		cfg := &ParametersConfig{}
		cfg.Alert.MinCashThreshold.Value = 1000000.0
		cfg.Alert.MaxPositionsCount.Value = 10
		cfg.Alert.MaxPositionWeightPct.Value = 0.20
		cfg.Alert.MaxUnrealizedLossPct.Value = 0.15
		cfg.Alert.DailyLossWarningPct.Value = 0.03
		cfg.Alert.DailyLossCriticalPct.Value = 0.05
		cfg.Alert.RuleEngineIntervalSec.Value = 60
		cfg.Alert.RuleEngineCooldownSec.Value = 600
		cfg.Alert.SystemMetricsIntervalSec.Value = 30

		mergeAlertDefaults(cfg)

		a := cfg.Alert
		if a.MinCashThreshold.Value != 1000000.0 {
			t.Errorf("MinCashThreshold overwritten: got %v, want 1000000.0", a.MinCashThreshold.Value)
		}
		if a.MaxPositionsCount.Value != 10 {
			t.Errorf("MaxPositionsCount overwritten: got %v, want 10", a.MaxPositionsCount.Value)
		}
		if a.RuleEngineIntervalSec.Value != 60 {
			t.Errorf("RuleEngineIntervalSec overwritten: got %v, want 60", a.RuleEngineIntervalSec.Value)
		}
	})
}

// ---------------------------------------------------------------------------
// mergeRiskGateDefaults (line 2768)
// Fills zero-valued RiskGate fields with defaults.
// ---------------------------------------------------------------------------

func TestMergeRiskGateDefaults(t *testing.T) {
	def := DefaultParametersConfig().RiskGate

	t.Run("case_A_empty_input", func(t *testing.T) {
		cfg := &ParametersConfig{}
		mergeRiskGateDefaults(cfg)

		r := cfg.RiskGate
		if r.PreTrade.MaxPositionPct.Value == 0 {
			t.Errorf("PreTrade.MaxPositionPct not populated from defaults")
		}
		if r.PreTrade.VarLimitPct.Value == 0 {
			t.Errorf("PreTrade.VarLimitPct not populated from defaults")
		}
		if r.InTrade.StopLossPct.Value == 0 {
			t.Errorf("InTrade.StopLossPct not populated from defaults")
		}
		if r.PostTrade.MaxDrawdownHaltPct.Value == 0 {
			t.Errorf("PostTrade.MaxDrawdownHaltPct not populated from defaults")
		}
		if r.PreTrade.MaxPositionPct.Value != def.PreTrade.MaxPositionPct.Value {
			t.Errorf("MaxPositionPct = %v; want %v", r.PreTrade.MaxPositionPct.Value, def.PreTrade.MaxPositionPct.Value)
		}
	})

	t.Run("case_B_partial_override", func(t *testing.T) {
		cfg := &ParametersConfig{}
		cfg.RiskGate.PreTrade.MaxPositionPct.Value = 0.05
		cfg.RiskGate.InTrade.TakeProfitPct.Value = 0.25

		mergeRiskGateDefaults(cfg)

		r := cfg.RiskGate
		if r.PreTrade.MaxPositionPct.Value != 0.05 {
			t.Errorf("PreTrade.MaxPositionPct was overwritten: got %v, want 0.05", r.PreTrade.MaxPositionPct.Value)
		}
		if r.InTrade.TakeProfitPct.Value != 0.25 {
			t.Errorf("InTrade.TakeProfitPct was overwritten: got %v, want 0.25", r.InTrade.TakeProfitPct.Value)
		}
		if r.PreTrade.VarLimitPct.Value == 0 {
			t.Errorf("PreTrade.VarLimitPct still zero after merge")
		}
		if r.PostTrade.MaxDrawdownHaltPct.Value == 0 {
			t.Errorf("PostTrade.MaxDrawdownHaltPct still zero after merge")
		}
	})

	t.Run("case_C_full_override", func(t *testing.T) {
		cfg := &ParametersConfig{}
		cfg.RiskGate.PreTrade.MaxPositionPct.Value = 0.08
		cfg.RiskGate.PreTrade.MaxSectorExposurePct.Value = 0.30
		cfg.RiskGate.PreTrade.VaRConfidenceLevel.Value = 0.95
		cfg.RiskGate.PreTrade.VarLimitPct.Value = 0.02
		cfg.RiskGate.PreTrade.MinCashBufferPct.Value = 0.10
		cfg.RiskGate.PreTrade.MaxCorrelation.Value = 0.60
		cfg.RiskGate.PreTrade.MinADVRatio.Value = 0.05
		cfg.RiskGate.PreTrade.MaxOpenPositions.Value = 20
		cfg.RiskGate.InTrade.MonitorIntervalSec.Value = 120
		cfg.RiskGate.InTrade.StopLossPct.Value = 0.05
		cfg.RiskGate.InTrade.TakeProfitPct.Value = 0.20
		cfg.RiskGate.InTrade.TrailingStopATRMult.Value = 2.5
		cfg.RiskGate.InTrade.VolatilitySpikeMult.Value = 1.8
		cfg.RiskGate.InTrade.CircuitBreakerDailyLossPct.Value = 0.04
		cfg.RiskGate.PostTrade.MaxDrawdownHaltPct.Value = 0.15
		cfg.RiskGate.PostTrade.MaxDrawdownDefensivePct.Value = 0.10
		cfg.RiskGate.PostTrade.MinRollingSharpe.Value = 1.5
		cfg.RiskGate.PostTrade.ConsecutiveLossDays.Value = 5
		cfg.RiskGate.PostTrade.EvaluationIntervalHours.Value = 24

		mergeRiskGateDefaults(cfg)

		r := cfg.RiskGate
		if r.PreTrade.MaxPositionPct.Value != 0.08 {
			t.Errorf("PreTrade.MaxPositionPct overwritten: got %v, want 0.08", r.PreTrade.MaxPositionPct.Value)
		}
		if r.InTrade.StopLossPct.Value != 0.05 {
			t.Errorf("InTrade.StopLossPct overwritten: got %v, want 0.05", r.InTrade.StopLossPct.Value)
		}
		if r.PostTrade.EvaluationIntervalHours.Value != 24 {
			t.Errorf("PostTrade.EvaluationIntervalHours overwritten: got %v, want 24", r.PostTrade.EvaluationIntervalHours.Value)
		}
	})
}

// ---------------------------------------------------------------------------
// mergeEngineDefaults (line 2831)
// Fills zero-valued Engine fields with defaults.
// Uses compound structs: fills entire sub-struct when one field is zero.
// ---------------------------------------------------------------------------

func TestMergeEngineDefaults(t *testing.T) {
	def := DefaultParametersConfig().Engine

	t.Run("case_A_empty_input", func(t *testing.T) {
		cfg := &ParametersConfig{}
		mergeEngineDefaults(cfg)

		e := cfg.Engine
		if e.MacroRisk.VIXThreshold.Value == 0 {
			t.Errorf("MacroRisk.VIXThreshold not populated from defaults")
		}
		if e.StructuralTrend.MinConfidence.Value == 0 {
			t.Errorf("StructuralTrend.MinConfidence not populated from defaults")
		}
		if len(e.Drawdown.Levels.Value) == 0 {
			t.Errorf("Engine.Drawdown.Levels not populated from defaults")
		}
		if e.MacroRisk.VIXThreshold.Value != def.MacroRisk.VIXThreshold.Value {
			t.Errorf("MacroRisk.VIXThreshold = %v; want %v", e.MacroRisk.VIXThreshold.Value, def.MacroRisk.VIXThreshold.Value)
		}
	})

	t.Run("case_B_partial_override", func(t *testing.T) {
		cfg := &ParametersConfig{}
		cfg.Engine.MacroRisk.VIXThreshold.Value = 35.0

		mergeEngineDefaults(cfg)

		e := cfg.Engine
		if e.MacroRisk.VIXThreshold.Value != 35.0 {
			t.Errorf("MacroRisk.VIXThreshold was overwritten: got %v, want 35.0", e.MacroRisk.VIXThreshold.Value)
		}
		if e.StructuralTrend.MinConfidence.Value == 0 {
			t.Errorf("StructuralTrend.MinConfidence still zero after merge")
		}
		if len(e.Drawdown.Levels.Value) == 0 {
			t.Errorf("Engine.Drawdown.Levels still empty after merge")
		}
	})

	t.Run("case_C_full_override", func(t *testing.T) {
		cfg := &ParametersConfig{}
		cfg.Engine.MacroRisk.VIXThreshold.Value = 40.0
		cfg.Engine.MacroRisk.US10YThreshold.Value = 0.05
		cfg.Engine.StructuralTrend.MinConfidence.Value = 0.75
		cfg.Engine.StructuralTrend.MinHitRate.Value = 0.60
		cfg.Engine.Drawdown.Levels.Value = map[string]DrawdownLevel{"none": {Percentage: 0.01, MaxExposure: 0.90}}
		cfg.Engine.SectorRotation.BaseAllocations.Value = map[string]float64{"TECH": 0.40}
		cfg.Engine.StrategyEvolution.CooldownPeriodHours.Value = 168
		cfg.Engine.Executors.VIXMomentumCrashThreshold.Value = 15.0
		cfg.Engine.Simulation.NeutralRegimeSizingFactor.Value = 0.85

		mergeEngineDefaults(cfg)

		e := cfg.Engine
		if e.MacroRisk.VIXThreshold.Value != 40.0 {
			t.Errorf("MacroRisk.VIXThreshold overwritten: got %v, want 40.0", e.MacroRisk.VIXThreshold.Value)
		}
		if e.Simulation.NeutralRegimeSizingFactor.Value != 0.85 {
			t.Errorf("Simulation.NeutralRegimeSizingFactor overwritten: got %v, want 0.85", e.Simulation.NeutralRegimeSizingFactor.Value)
		}
	})
}

// ---------------------------------------------------------------------------
// mergeSectorExecutorDefaults (line 2858)
// Fills zero-valued SectorExecutor sub structs with defaults.
// ---------------------------------------------------------------------------

func TestMergeSectorExecutorDefaults(t *testing.T) {
	def := DefaultParametersConfig().SectorExecutor

	t.Run("case_A_empty_input", func(t *testing.T) {
		cfg := &ParametersConfig{}
		mergeSectorExecutorDefaults(cfg)

		s := cfg.SectorExecutor
		if s.LEOSatellite.ConvictionBase.Value == 0 {
			t.Errorf("LEOSatellite.ConvictionBase not populated from defaults")
		}
		if s.Financials.DividendBoost.Value == 0 {
			t.Errorf("Financials.DividendBoost not populated from defaults")
		}
		if s.GrowthMomentum.ConvictionBase.Value == 0 {
			t.Errorf("GrowthMomentum.ConvictionBase not populated from defaults")
		}
		if s.LEOSatellite.ConvictionBase.Value != def.LEOSatellite.ConvictionBase.Value {
			t.Errorf("LEOSatellite.ConvictionBase = %v; want %v", s.LEOSatellite.ConvictionBase.Value, def.LEOSatellite.ConvictionBase.Value)
		}
	})

	t.Run("case_B_partial_override", func(t *testing.T) {
		cfg := &ParametersConfig{}
		cfg.SectorExecutor.LEOSatellite.ConvictionBase.Value = 80
		cfg.SectorExecutor.Shipping.TacticalBoost.Value = 15

		mergeSectorExecutorDefaults(cfg)

		s := cfg.SectorExecutor
		if s.LEOSatellite.ConvictionBase.Value != 80 {
			t.Errorf("LEOSatellite.ConvictionBase was overwritten: got %v, want 80", s.LEOSatellite.ConvictionBase.Value)
		}
		if s.Shipping.TacticalBoost.Value != 15 {
			t.Errorf("Shipping.TacticalBoost was overwritten: got %v, want 15", s.Shipping.TacticalBoost.Value)
		}
		if s.Financials.DividendBoost.Value == 0 {
			t.Errorf("Financials.DividendBoost still zero after merge")
		}
		if s.GrowthMomentum.ConvictionBase.Value == 0 {
			t.Errorf("GrowthMomentum.ConvictionBase still zero after merge")
		}
	})

	t.Run("case_C_full_override", func(t *testing.T) {
		cfg := &ParametersConfig{}
		cfg.SectorExecutor.LEOSatellite.ConvictionBase.Value = 91
		cfg.SectorExecutor.Financials.DividendBoost.Value = 12
		cfg.SectorExecutor.Shipping.TacticalBoost.Value = 13
		cfg.SectorExecutor.ValueYield.CashFlowBoost.Value = 14
		cfg.SectorExecutor.EarningsQuality.RepeatableBoost.Value = 15
		cfg.SectorExecutor.TechnicalBreakout.DefaultVolumeFloor.Value = 5000000
		cfg.SectorExecutor.GrowthMomentum.ConvictionBase.Value = 65

		mergeSectorExecutorDefaults(cfg)

		s := cfg.SectorExecutor
		if s.LEOSatellite.ConvictionBase.Value != 91 {
			t.Errorf("LEOSatellite.ConvictionBase overwritten: got %v, want 91", s.LEOSatellite.ConvictionBase.Value)
		}
		if s.Financials.DividendBoost.Value != 12 {
			t.Errorf("Financials.DividendBoost overwritten: got %v, want 12", s.Financials.DividendBoost.Value)
		}
		if s.GrowthMomentum.ConvictionBase.Value != 65 {
			t.Errorf("GrowthMomentum.ConvictionBase overwritten: got %v, want 65", s.GrowthMomentum.ConvictionBase.Value)
		}
	})
}

// ---------------------------------------------------------------------------
// mergeIndustryDefaults (line 2888)
// Fills zero-valued Industry fields with defaults.
// ---------------------------------------------------------------------------

func TestMergeIndustryDefaults(t *testing.T) {
	def := DefaultParametersConfig().Industry

	t.Run("case_A_empty_input", func(t *testing.T) {
		cfg := &ParametersConfig{}
		mergeIndustryDefaults(cfg)

		i := cfg.Industry
		if i.AsymmetricDropCritical.Value == 0 {
			t.Errorf("AsymmetricDropCritical not populated from defaults")
		}
		if i.NewsImpactMultiplier.Value == 0 {
			t.Errorf("NewsImpactMultiplier not populated from defaults")
		}
		if i.DynamicEnv.Value.HistoryWindowDays == 0 {
			t.Errorf("DynamicEnv.HistoryWindowDays not populated from defaults")
		}
		if i.SiliconCycle.Value.RevenueYoYThreshold == 0 {
			t.Errorf("SiliconCycle.RevenueYoYThreshold not populated from defaults")
		}
		if i.AsymmetricDropCritical.Value != def.AsymmetricDropCritical.Value {
			t.Errorf("AsymmetricDropCritical = %v; want %v", i.AsymmetricDropCritical.Value, def.AsymmetricDropCritical.Value)
		}
	})

	t.Run("case_B_partial_override", func(t *testing.T) {
		cfg := &ParametersConfig{}
		cfg.Industry.AsymmetricDropCritical.Value = 0.12
		cfg.Industry.BoundaryFallback.Value = 0.30

		mergeIndustryDefaults(cfg)

		i := cfg.Industry
		if i.AsymmetricDropCritical.Value != 0.12 {
			t.Errorf("AsymmetricDropCritical was overwritten: got %v, want 0.12", i.AsymmetricDropCritical.Value)
		}
		if i.BoundaryFallback.Value != 0.30 {
			t.Errorf("BoundaryFallback was overwritten: got %v, want 0.30", i.BoundaryFallback.Value)
		}
		if i.NewsImpactMultiplier.Value == 0 {
			t.Errorf("NewsImpactMultiplier still zero after merge")
		}
		if i.SiliconCycle.Value.RevenueYoYThreshold == 0 {
			t.Errorf("SiliconCycle.RevenueYoYThreshold still zero after merge")
		}
	})

	t.Run("case_C_full_override", func(t *testing.T) {
		cfg := &ParametersConfig{}
		cfg.Industry.AsymmetricDropCritical.Value = 0.10
		cfg.Industry.AsymmetricDropHigh.Value = 0.15
		cfg.Industry.AsymmetricDropMedium.Value = 0.20
		cfg.Industry.NewsImpactMultiplier.Value = 0.25
		cfg.Industry.BoundaryFallback.Value = 0.35
		cfg.Industry.AdjustmentFloor.Value = 0.05
		cfg.Industry.DynamicEnv.Value.HistoryWindowDays = 60
		cfg.Industry.HistoryRetentionDays.Value = 180
		cfg.Industry.SiliconCycle.Value.RevenueYoYThreshold = 0.30
		cfg.Industry.SiliconCycle.Value.BillingsYoYThreshold = 0.25
		cfg.Industry.SiliconCycle.Value.IndexMAPercentThreshold = 0.15
		cfg.Industry.EventSentimentCap.Value = 0.80
		cfg.Industry.ClassificationTree.Value.Segments = []IndustrySegmentConfig{{ID: "seg1", Name: "Technology", Level: 1, Cyclicality: "High", TechnologyIntensity: "High"}}

		mergeIndustryDefaults(cfg)

		i := cfg.Industry
		if i.AsymmetricDropCritical.Value != 0.10 {
			t.Errorf("AsymmetricDropCritical overwritten: got %v, want 0.10", i.AsymmetricDropCritical.Value)
		}
		if i.SiliconCycle.Value.RevenueYoYThreshold != 0.30 {
			t.Errorf("SiliconCycle.RevenueYoYThreshold overwritten: got %v, want 0.30", i.SiliconCycle.Value.RevenueYoYThreshold)
		}
		if len(i.ClassificationTree.Value.Segments) == 0 || i.ClassificationTree.Value.Segments[0].ID != "seg1" {
			t.Errorf("ClassificationTree overwritten: got %v", i.ClassificationTree.Value)
		}
	})
}

// ---------------------------------------------------------------------------
// mergeRSITwDefaults (line 2930)
// Fills zero-valued RSITwParameters fields with defaults.
// ---------------------------------------------------------------------------

func TestMergeRSITwDefaults(t *testing.T) {
	def := DefaultParametersConfig().RSITw

	t.Run("case_A_empty_input", func(t *testing.T) {
		cfg := &ParametersConfig{}
		mergeRSITwDefaults(cfg)

		r := cfg.RSITw
		if r.A1Weight.Value == 0 {
			t.Errorf("A1Weight not populated from defaults")
		}
		if r.A3Midpoint.Value == 0 {
			t.Errorf("A3Midpoint not populated from defaults")
		}
		if len(r.A4VixThresholds.Value) == 0 {
			t.Errorf("A4VixThresholds not populated from defaults")
		}
		if r.C1VeryBullishThreshold.Value == 0 {
			t.Errorf("C1VeryBullishThreshold not populated from defaults")
		}
		if r.A1Weight.Value != def.A1Weight.Value {
			t.Errorf("A1Weight = %v; want %v", r.A1Weight.Value, def.A1Weight.Value)
		}
	})

	t.Run("case_B_partial_override", func(t *testing.T) {
		cfg := &ParametersConfig{}
		cfg.RSITw.A1Weight.Value = 0.20
		cfg.RSITw.C1VeryBullishThreshold.Value = 0.85

		mergeRSITwDefaults(cfg)

		r := cfg.RSITw
		if r.A1Weight.Value != 0.20 {
			t.Errorf("A1Weight was overwritten: got %v, want 0.20", r.A1Weight.Value)
		}
		if r.C1VeryBullishThreshold.Value != 0.85 {
			t.Errorf("C1VeryBullishThreshold was overwritten: got %v, want 0.85", r.C1VeryBullishThreshold.Value)
		}
		if r.A3Midpoint.Value == 0 {
			t.Errorf("A3Midpoint still zero after merge")
		}
		if len(r.A4VixThresholds.Value) == 0 {
			t.Errorf("A4VixThresholds still empty after merge")
		}
	})

	t.Run("case_C_full_override", func(t *testing.T) {
		cfg := &ParametersConfig{}
		cfg.RSITw.A1Weight.Value = 0.15
		cfg.RSITw.A2Weight.Value = 0.15
		cfg.RSITw.A3Weight.Value = 0.15
		cfg.RSITw.A4Weight.Value = 0.15
		cfg.RSITw.A5Weight.Value = 0.10
		cfg.RSITw.A6Weight.Value = 0.10
		cfg.RSITw.APartWeight.Value = 0.70
		cfg.RSITw.CPartWeight.Value = 0.30
		cfg.RSITw.A3Midpoint.Value = 50.0
		cfg.RSITw.A3Scale.Value = 1.2
		cfg.RSITw.A4VixThresholds.Value = []float64{15.0, 20.0, 25.0}
		cfg.RSITw.A4VixScores.Value = []float64{1.0, 0.5, 0.0}
		cfg.RSITw.A5PcrThresholds.Value = []float64{0.8, 1.0, 1.2}
		cfg.RSITw.A5PcrScores.Value = []float64{1.0, 0.5, 0.0}
		cfg.RSITw.A5PcrFallback.Value = 0.25
		cfg.RSITw.A6OddLotThresholds.Value = []float64{50.0, 100.0}
		cfg.RSITw.A6OddLotScores.Value = []float64{1.0, 0.0}
		cfg.RSITw.A6OddLotFallback.Value = 0.10
		cfg.RSITw.C1Weight.Value = 0.40
		cfg.RSITw.C2Weight.Value = 0.35
		cfg.RSITw.C3Weight.Value = 0.25
		cfg.RSITw.C1VeryBullishThreshold.Value = 0.90
		cfg.RSITw.C1BullishThreshold.Value = 0.70
		cfg.RSITw.C1BearishThreshold.Value = 0.30
		cfg.RSITw.C1VeryBearishThreshold.Value = 0.10
		cfg.RSITw.C2NeutralMidpoint.Value = 0.50
		cfg.RSITw.C2NetflowScalingFactor.Value = 1.5
		cfg.RSITw.C3VeryBullishThreshold.Value = 0.88
		cfg.RSITw.C3BullishThreshold.Value = 0.65
		cfg.RSITw.C3BearishThreshold.Value = 0.35
		cfg.RSITw.DGeoPoliticalRiskThreshold.Value = 0.60
		cfg.RSITw.DGeoPoliticalRiskMultiplier.Value = 1.3
		cfg.RSITw.DVIXSpikeThreshold.Value = 25.0
		cfg.RSITw.DVIXSpikeMultiplier.Value = 1.4
		cfg.RSITw.DCreditTighteningMultiplier.Value = 1.2

		mergeRSITwDefaults(cfg)

		r := cfg.RSITw
		if r.A1Weight.Value != 0.15 {
			t.Errorf("A1Weight overwritten: got %v, want 0.15", r.A1Weight.Value)
		}
		if r.C1VeryBullishThreshold.Value != 0.90 {
			t.Errorf("C1VeryBullishThreshold overwritten: got %v, want 0.90", r.C1VeryBullishThreshold.Value)
		}
		if len(r.A4VixThresholds.Value) != 3 {
			t.Errorf("A4VixThresholds overwritten: got %v", r.A4VixThresholds.Value)
		}
	})
}

// ---------------------------------------------------------------------------
// mergeFallbackPriceTargetsDefaults (line 3058)
// Ensures every skill key in defaults is present in the loaded config,
// and fills zero-valued target/stop-loss multipliers.
// ---------------------------------------------------------------------------

func TestMergeFallbackPriceTargetsDefaults(t *testing.T) {
	def := DefaultParametersConfig().FallbackPriceTargets

	t.Run("case_A_empty_input", func(t *testing.T) {
		cfg := &ParametersConfig{}
		mergeFallbackPriceTargetsDefaults(cfg)

		if cfg.FallbackPriceTargets == nil {
			t.Fatal("FallbackPriceTargets is nil after merge")
		}
		if len(cfg.FallbackPriceTargets) == 0 {
			t.Fatal("FallbackPriceTargets is empty after merge")
		}
		for key := range def {
			if _, ok := cfg.FallbackPriceTargets[key]; !ok {
				t.Errorf("key %q missing from FallbackPriceTargets after merge", key)
			}
		}
		if cfg.FallbackPriceTargets["_default"].TargetMultiplier.Value == 0 {
			t.Errorf("_default.TargetMultiplier not populated from defaults")
		}
	})

	t.Run("case_B_partial_override", func(t *testing.T) {
		cfg := &ParametersConfig{}
		cfg.FallbackPriceTargets = make(map[string]FallbackPriceTarget)
		cfg.FallbackPriceTargets["_default"] = FallbackPriceTarget{
			TargetMultiplier:   ParameterMetadata[float64]{Value: 0.08},
			StopLossMultiplier: ParameterMetadata[float64]{Value: 0.04},
		}
		cfg.FallbackPriceTargets["growth_momentum"] = FallbackPriceTarget{
			TargetMultiplier:   ParameterMetadata[float64]{Value: 0.10},
			StopLossMultiplier: ParameterMetadata[float64]{Value: 0},
		}

		mergeFallbackPriceTargetsDefaults(cfg)

		if cfg.FallbackPriceTargets["_default"].TargetMultiplier.Value != 0.08 {
			t.Errorf("_default.TargetMultiplier was overwritten: got %v, want 0.08",
				cfg.FallbackPriceTargets["_default"].TargetMultiplier.Value)
		}
		if cfg.FallbackPriceTargets["growth_momentum"].TargetMultiplier.Value != 0.10 {
			t.Errorf("growth_momentum.TargetMultiplier was overwritten: got %v, want 0.10",
				cfg.FallbackPriceTargets["growth_momentum"].TargetMultiplier.Value)
		}
		if cfg.FallbackPriceTargets["growth_momentum"].StopLossMultiplier.Value == 0 {
			t.Errorf("growth_momentum.StopLossMultiplier still zero after merge")
		}
		if _, ok := cfg.FallbackPriceTargets["_default"]; !ok {
			t.Errorf("_default key missing after partial-override merge")
		}
	})

	t.Run("case_C_full_override", func(t *testing.T) {
		cfg := &ParametersConfig{}
		cfg.FallbackPriceTargets = make(map[string]FallbackPriceTarget)
		for key, defEntry := range def {
			cfg.FallbackPriceTargets[key] = FallbackPriceTarget{
				TargetMultiplier:   ParameterMetadata[float64]{Value: defEntry.TargetMultiplier.Value + 0.01},
				StopLossMultiplier: ParameterMetadata[float64]{Value: defEntry.StopLossMultiplier.Value + 0.01},
			}
		}

		mergeFallbackPriceTargetsDefaults(cfg)

		for key, entry := range cfg.FallbackPriceTargets {
			defEntry := def[key]
			if entry.TargetMultiplier.Value != defEntry.TargetMultiplier.Value+0.01 {
				t.Errorf("%s.TargetMultiplier overwritten: got %v, want %v",
					key, entry.TargetMultiplier.Value, defEntry.TargetMultiplier.Value+0.01)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// mergeReportingDefaults (line 3080)
// Fills zero-valued Reporting fields with defaults.
// ---------------------------------------------------------------------------

func TestMergeReportingDefaults(t *testing.T) {
	def := DefaultParametersConfig().Reporting

	t.Run("case_A_empty_input", func(t *testing.T) {
		cfg := &ParametersConfig{}
		mergeReportingDefaults(cfg)

		r := cfg.Reporting
		if r.WinRateThreshold.Value == 0 {
			t.Errorf("WinRateThreshold not populated from defaults")
		}
		if r.SharpeMinSamples.Value == 0 {
			t.Errorf("SharpeMinSamples not populated from defaults")
		}
		if r.WinRateThreshold.Value != def.WinRateThreshold.Value {
			t.Errorf("WinRateThreshold = %v; want %v", r.WinRateThreshold.Value, def.WinRateThreshold.Value)
		}
		if r.SharpeMinSamples.Value != def.SharpeMinSamples.Value {
			t.Errorf("SharpeMinSamples = %v; want %v", r.SharpeMinSamples.Value, def.SharpeMinSamples.Value)
		}
	})

	t.Run("case_B_partial_override", func(t *testing.T) {
		cfg := &ParametersConfig{}
		cfg.Reporting.WinRateThreshold.Value = 0.60

		mergeReportingDefaults(cfg)

		r := cfg.Reporting
		if r.WinRateThreshold.Value != 0.60 {
			t.Errorf("WinRateThreshold was overwritten: got %v, want 0.60", r.WinRateThreshold.Value)
		}
		if r.SharpeMinSamples.Value == 0 {
			t.Errorf("SharpeMinSamples still zero after merge")
		}
	})

	t.Run("case_C_full_override", func(t *testing.T) {
		cfg := &ParametersConfig{}
		cfg.Reporting.WinRateThreshold.Value = 0.70
		cfg.Reporting.SharpeMinSamples.Value = 30

		mergeReportingDefaults(cfg)

		r := cfg.Reporting
		if r.WinRateThreshold.Value != 0.70 {
			t.Errorf("WinRateThreshold overwritten: got %v, want 0.70", r.WinRateThreshold.Value)
		}
		if r.SharpeMinSamples.Value != 30 {
			t.Errorf("SharpeMinSamples overwritten: got %v, want 30", r.SharpeMinSamples.Value)
		}
	})
}
