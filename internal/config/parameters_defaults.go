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
		FlowGateway: defaultFlowGatewayParameters(),
	}
}

// defaultFlowGatewayParameters returns the canonical defaults for the
// stockpicker.flow_gateway section (PR 2b two-level capital-flow gate).
// Foreign is a per-symbol layer: min_abs_net is the minimum |foreign net
// buy| of the checked symbol in 億股 (the evaluator converts
// FlowPoint.ForeignNet 千股 → 億股 by ÷1e5). Institutional and retail are
// market-regime layers gated by the market-wide capitalflow ForceScore —
// 無個股層級資料，僅供市場 regime 參考. Values mirror configs/parameters.json
// → stockpicker.flow_gateway; the gate evaluator never reads these literals
// directly.
func defaultFlowGatewayParameters() FlowGatewayParameters {
	return FlowGatewayParameters{
		FailClosedWhenAllMissing: ParameterMetadata[bool]{
			Value:     true,
			Rationale: "When every enforced gateway layer is skipped due to missing data, fail closed (no-decision): the live path must never trade on a fully-missing flow picture. Backtests may set false to keep evaluating on partial data (PR 2b review fix).",
			Source:    SourceHeuristic,
		},
		Layers: FlowGatewayLayers{
			Foreign: FlowGatewayForeignThreshold{
				MinAbsNet: ParameterMetadata[float64]{
					Value:     0.1,
					Rationale: "Minimum |foreign net buy| of the CHECKED SYMBOL (億股) for the per-symbol foreign layer; ≈1 萬張 net buy on one symbol marks meaningful foreign interest (PR 2b review fix: per-symbol granularity).",
					Source:    SourceHeuristic,
					Todo:      "Calibrate: tune per-symbol magnitude against backtest win rates once OOS samples accumulate.",
				},
			},
			Institutional: FlowGatewayMarketThreshold{
				MinAbsRaw: ParameterMetadata[float64]{
					Value:     0.3,
					Rationale: "Minimum |investment-trust net buy| (億股, market regime) for the institutional layer; 投信 daily magnitudes are smaller than 外資, so the raw gate is lower (PR 2b). No per-symbol source exists — 市場 regime 參考 only.",
					Source:    SourceHeuristic,
				},
				MinAbsZ: ParameterMetadata[float64]{
					Value:     0.5,
					Rationale: "Minimum |investment-trust z-score| (capitalflow 60-day rolling) for the institutional market-regime layer; 0.5 aligns with capitalflow's bullish/bearish trend boundary.",
					Source:    SourceHeuristic,
				},
			},
			Retail: FlowGatewayMarketThreshold{
				MinAbsRaw: ParameterMetadata[float64]{
					Value:     1.0,
					Rationale: "Minimum |retail composite| (融資+融券/借券 change pct, capitalflow ForceRetail RawValue) for the retail market-regime layer; a >1pct composite move marks meaningful retail positioning (PR 2b).",
					Source:    SourceHeuristic,
				},
				MinAbsZ: ParameterMetadata[float64]{
					Value:     0.5,
					Rationale: "Minimum |retail z-score| for the retail market-regime layer; 0.5 aligns with capitalflow's bullish/bearish trend boundary.",
					Source:    SourceHeuristic,
				},
			},
		},
		Conditions: FlowGatewayConditions{
			Foreign3DNetBuy: FlowGatewayCondition{
				Layers: ParameterMetadata[[]string]{
					Value:     []string{"foreign", "institutional", "retail"},
					Rationale: "foreign-3d-net-buy enforces the full two-level gate: per-symbol foreign magnitude + both market-regime layers (PR 2b).",
					Source:    SourceHeuristic,
				},
			},
			Momentum20DPosit: FlowGatewayCondition{
				Layers: ParameterMetadata[[]string]{
					Value:     []string{"foreign", "institutional"},
					Rationale: "momentum-20d-positive is a price condition; its gate narrows to the two institution-driven layers (per-symbol foreign + institutional market regime) and skips the retail regime (PR 2b review fix: real narrowing, not a no-op all-three copy).",
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
	// flow_gateway (PR 2b): fill each sub-block whose provenance is missing
	// so old saved configs (predating the section) resolve to the documented
	// two-level gate defaults.
	if cfg.Stockpicker.FlowGateway.FailClosedWhenAllMissing.Rationale == "" {
		cfg.Stockpicker.FlowGateway.FailClosedWhenAllMissing = def.FlowGateway.FailClosedWhenAllMissing
	}
	if cfg.Stockpicker.FlowGateway.Layers.Foreign.MinAbsNet.Rationale == "" {
		cfg.Stockpicker.FlowGateway.Layers.Foreign = def.FlowGateway.Layers.Foreign
	}
	if cfg.Stockpicker.FlowGateway.Layers.Institutional.MinAbsRaw.Rationale == "" {
		cfg.Stockpicker.FlowGateway.Layers.Institutional = def.FlowGateway.Layers.Institutional
	}
	if cfg.Stockpicker.FlowGateway.Layers.Retail.MinAbsRaw.Rationale == "" {
		cfg.Stockpicker.FlowGateway.Layers.Retail = def.FlowGateway.Layers.Retail
	}
	if cfg.Stockpicker.FlowGateway.Conditions.Foreign3DNetBuy.Layers.Rationale == "" {
		cfg.Stockpicker.FlowGateway.Conditions.Foreign3DNetBuy = def.FlowGateway.Conditions.Foreign3DNetBuy
	}
	if cfg.Stockpicker.FlowGateway.Conditions.Momentum20DPosit.Layers.Rationale == "" {
		cfg.Stockpicker.FlowGateway.Conditions.Momentum20DPosit = def.FlowGateway.Conditions.Momentum20DPosit
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
		TrendBullishThreshold:   TrendBullishThresholdMetadata,
		TrendBearishThreshold:   TrendBearishThresholdMetadata,
		PeriodWeightedQuality:   PeriodWeightedQualityMetadata,
		ActionObservationMode:   ActionObservationModeMetadata,
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
	if cfg.Capitalflow.TrendBullishThreshold.Rationale == "" {
		cfg.Capitalflow.TrendBullishThreshold = def.TrendBullishThreshold
	}
	if cfg.Capitalflow.TrendBearishThreshold.Rationale == "" {
		cfg.Capitalflow.TrendBearishThreshold = def.TrendBearishThreshold
	}
	if cfg.Capitalflow.PeriodWeightedQuality.Rationale == "" {
		cfg.Capitalflow.PeriodWeightedQuality = def.PeriodWeightedQuality
	}
	if cfg.Capitalflow.ActionObservationMode.Rationale == "" {
		cfg.Capitalflow.ActionObservationMode = def.ActionObservationMode
	}
}
