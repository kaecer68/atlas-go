package orchestrator

// ExecutorLoader defines the interface for loading executors.
type ExecutorLoader interface {
	LoadRegimeExecutors() ([]RegimeExecutor, error)
	LoadAgentExecutors() ([]AgentExecutor, error)
	LoadControlExecutors() ([]ControlExecutor, error)
}

// StaticLoader returns hardcoded executors for backward compatibility.
type StaticLoader struct{}

func (StaticLoader) LoadRegimeExecutors() ([]RegimeExecutor, error) {
	return []RegimeExecutor{
		TaiwanMacroRegimeExecutor{},
		ForeignFlowRegimeExecutor{},
	}, nil
}

func (StaticLoader) LoadAgentExecutors() ([]AgentExecutor, error) {
	return []AgentExecutor{
		SemiconductorExecutor{},
		AISupplyChainExecutor{},
		ETFRotationExecutor{},
		FinancialsExecutor{},
		ShippingExecutor{},
		GrowthMomentumExecutor{},
		ValueYieldExecutor{},
		EarningsQualityExecutor{},
		TechnicalBreakoutExecutor{},
	}, nil
}

func (StaticLoader) LoadControlExecutors() ([]ControlExecutor, error) {
	return []ControlExecutor{
		NewCRORiskExecutor(),
		CIOPortfolioExecutor{},
	}, nil
}
