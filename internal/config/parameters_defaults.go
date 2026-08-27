package config

// DefaultParametersConfig returns a configuration that exactly mirrors
// the current hard-coded values in the portfolio, experiment, and baseline
// packages. This ensures zero behavioral drift when no config file exists.
func DefaultParametersConfig() *ParametersConfig {
	return &ParametersConfig{
		Version:              "1.0",
		FallbackPriceTargets: defaultFallbackPriceTargets(),
		Darwinian:            defaultDarwinianParameters(),
		Factor:               defaultFactorParameters(),
		Optimizer:            defaultOptimizerParameters(),
		Sizing:               defaultSizingParameters(),
		Health:               defaultHealthParameters(),
		GARCH:                defaultGARCHParameters(),
		Experiment:           defaultExperimentParameters(),
		Baseline:             defaultBaselineParameters(),
		Orchestrator:         defaultOrchestratorParameters(),
		Risk:                 defaultRiskParameters(),
		Drawdown:             defaultDrawdownParameters(),
		Realtime:             defaultRealtimeParameters(),
		Narrative:            defaultNarrativeParameters(),
		Janus:                defaultJanusParameters(),
		Marketdata:           defaultMarketdataParameters(),
		Industry:             defaultIndustryParameters(),
		Strategy:             defaultStrategyParameters(),
		PreciousMetals:       defaultPreciousMetalsParameters(),
		FactorWeight:         defaultFactorWeightParameters(),
		NarrativeConviction:  defaultNarrativeConvictionParameters(),
		SectorExecutor:       defaultSectorExecutorParameters(),
		Alert:                defaultAlertParameters(),
		Capitalflow:          defaultCapitalflowParameters(),
		RiskGate:             defaultRiskGateParameters(),
		Engine:               defaultEngineParameters(),
		RSITw:                defaultRSITwParameters(),
		Tax:                  defaultTaxParameters(),
		SectorAllocation:     deriveDefaultSectorAllocationConfig(),
		Reporting:            deriveDefaultReportingConfig(),
		SmartUniverse:        defaultSmartUniverseParams(),
		ForwardReturn:        defaultForwardReturnParameters(),
		Stockpicker:          defaultStockpickerParameters(),
	}
}

// defaultStockpickerParameters returns the canonical defaults for the
// stockpicker backtest/aggregation subsystem (PR 1c). The round-trip cost
// follows the Taiwan statutory schedule (commission 0.1425% × 2 + sell-side
// transaction tax 0.3% ≈ 0.585%); min_samples mirrors the capitalflow
// 30-sample calibration gate (forces.go H-CF-02).
func defaultStockpickerParameters() StockpickerParameters {
	return StockpickerParameters{
		Costs: StockpickerCostsParameters{
			RoundTripPct: ParameterMetadata[float64]{
				Value:     0.00585,
				Rationale: "Taiwan round-trip cost: commission 0.1425% ×2 + sell-side transaction tax 0.3% ≈ 0.585%. NetHit uses this to avoid systematically overstating win rates (k3 review R1 / P0-3).",
				Source:    SourceHeuristic,
				Todo:      "Calibrate: after backtest aggregation produces enough samples, fit per-window cost from slippage/impact data.",
			},
		},
		Calibration: StockpickerCalibrationParameters{
			MinSamples: ParameterMetadata[int]{
				Value:     30,
				Rationale: "Minimum samples for eligible calibration status; mirrors capitalflow 30-sample gate (forces.go H-CF-02). Below this the point estimate is not displayed.",
				Source:    SourceHeuristic,
			},
		},
		Conditions: StockpickerConditionsParameters{
			Foreign3DNetBuy: StockpickerConditionWindow{
				WindowDays: ParameterMetadata[float64]{
					Value:     3,
					Rationale: "Foreign net-buy accumulation window in trading days (PR 1c demo condition; configurable since PR 2a).",
					Source:    SourceHeuristic,
					Todo:      "Calibrate: tune window against backtest win rates once OOS samples accumulate.",
				},
				Threshold: ParameterMetadata[float64]{
					Value:     0,
					Rationale: "Cumulative foreign net buy over the window must exceed this value to trigger (PR 2a).",
					Source:    SourceHeuristic,
				},
			},
			Momentum20DPosit: StockpickerConditionWindow{
				WindowDays: ParameterMetadata[float64]{
					Value:     20,
					Rationale: "Momentum lookback in trading days: close[t]/close[t-window] - 1 (PR 1c demo condition; configurable since PR 2a).",
					Source:    SourceHeuristic,
					Todo:      "Calibrate: tune window against backtest win rates once OOS samples accumulate.",
				},
				Threshold: ParameterMetadata[float64]{
					Value:     0,
					Rationale: "Momentum must exceed this value to trigger (PR 2a).",
					Source:    SourceHeuristic,
				},
			},
		},
	}
}

// mergeStockpickerDefaults fills missing Stockpicker fields from the
// canonical defaults after JSON unmarshal so old saved configs (that predate
// the stockpicker section) still resolve to the documented values.
func mergeStockpickerDefaults(cfg *ParametersConfig) {
	def := defaultStockpickerParameters()
	if cfg.Stockpicker.Costs.RoundTripPct.Rationale == "" {
		cfg.Stockpicker.Costs = def.Costs
	}
	if cfg.Stockpicker.Calibration.MinSamples.Rationale == "" {
		cfg.Stockpicker.Calibration = def.Calibration
	}
	if cfg.Stockpicker.Conditions.Foreign3DNetBuy.WindowDays.Rationale == "" {
		cfg.Stockpicker.Conditions.Foreign3DNetBuy = def.Conditions.Foreign3DNetBuy
	}
	if cfg.Stockpicker.Conditions.Momentum20DPosit.WindowDays.Rationale == "" {
		cfg.Stockpicker.Conditions.Momentum20DPosit = def.Conditions.Momentum20DPosit
	}
}

// defaultCapitalflowParameters returns the canonical defaults for capitalflow.
// Bounds are design constants (PR #1007 invariant); values live in the
// top-level *Metadata vars so they are reachable before config load
// (e.g. in test environments that never call LoadParametersConfig).
func defaultCapitalflowParameters() CapitalflowParameters {
	return CapitalflowParameters{
		ResonanceCoefficientMax: ResonanceCoefficientMaxMetadata,
		ResonanceCoefficientMin: ResonanceCoefficientMinMetadata,
	}
}

// mergeCapitalflowDefaults fills any missing Capitalflow subgroup field with
// the canonical default from defaultCapitalflowParameters. Called from
// LoadParametersConfig after JSON unmarshal so old saved configs (that
// predate the capitalflow section) still resolve to the documented bounds.
func mergeCapitalflowDefaults(cfg *ParametersConfig) {
	def := defaultCapitalflowParameters()
	if cfg.Capitalflow.ResonanceCoefficientMax.Rationale == "" {
		cfg.Capitalflow.ResonanceCoefficientMax = def.ResonanceCoefficientMax
	}
	if cfg.Capitalflow.ResonanceCoefficientMin.Rationale == "" {
		cfg.Capitalflow.ResonanceCoefficientMin = def.ResonanceCoefficientMin
	}
}
