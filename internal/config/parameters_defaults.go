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
