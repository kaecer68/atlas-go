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
