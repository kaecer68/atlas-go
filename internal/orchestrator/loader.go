package orchestrator

// ExecutorLoader defines the interface for loading executors.
type ExecutorLoader interface {
	LoadRegimeExecutors() ([]RegimeExecutor, error)
	LoadAgentExecutors() ([]AgentExecutor, error)
	LoadControlExecutors() ([]ControlExecutor, error)
}

// ── PLUGIN BOUNDARY: DO NOT REMOVE ──────────────────────────────────
//
// StaticLoader.RegisterAgent / RegisterRegime / RegisterControl are the
// injection point for proprietary strategy modules. They appear unused in
// the open-source codebase, but proprietary repos inject custom executors
// through these methods before passing the loader to NewPluginRegistry.
//
// Architectural intent: open-source core ships builtin executor list;
// proprietary plugins extend it via Register*. This is the sole coupling
// point between the open-source engine and private strategy IP.
//
// When refactoring: keep the Register* methods and the extra* fields.
// They are NOT dead code — they are the plugin boundary contract.
// ─────────────────────────────────────────────────────────────────────

// StaticLoader loads executors from a configurable list. Use
// RegisterAgent/RegisterRegime/RegisterControl to add custom executors
// before passing the loader to NewPluginRegistry.
// The zero value (StaticLoader{}) produces the built-in executor list
// via the builtin* functions for full backward compatibility.
type StaticLoader struct {
	extraAgents   []AgentExecutor   // populated by RegisterAgent — plugin boundary
	extraRegimes  []RegimeExecutor  // populated by RegisterRegime — plugin boundary
	extraControls []ControlExecutor // populated by RegisterControl — plugin boundary
}

// RegisterAgent is the injection point for proprietary AgentExecutor implementations.
// Called by private strategy modules before NewPluginRegistry.
// DO NOT REMOVE — appears unused in open-source core but is the plugin contract.
func (l *StaticLoader) RegisterAgent(exec AgentExecutor) {
	l.extraAgents = append(l.extraAgents, exec)
}

// RegisterRegime is the injection point for proprietary RegimeExecutor implementations.
// DO NOT REMOVE — appears unused in open-source core but is the plugin contract.
func (l *StaticLoader) RegisterRegime(exec RegimeExecutor) {
	l.extraRegimes = append(l.extraRegimes, exec)
}

// RegisterControl is the injection point for proprietary ControlExecutor implementations.
// DO NOT REMOVE — appears unused in open-source core but is the plugin contract.
func (l *StaticLoader) RegisterControl(exec ControlExecutor) {
	l.extraControls = append(l.extraControls, exec)
}

func (l StaticLoader) LoadRegimeExecutors() ([]RegimeExecutor, error) {
	base := builtinRegimeExecutors()
	return append(base, l.extraRegimes...), nil
}

func (l StaticLoader) LoadAgentExecutors() ([]AgentExecutor, error) {
	base := builtinAgentExecutors()
	return append(base, l.extraAgents...), nil
}

func (l StaticLoader) LoadControlExecutors() ([]ControlExecutor, error) {
	base := builtinControlExecutors()
	return append(base, l.extraControls...), nil
}

// builtinAgentExecutors returns the default set of agent executors.
// This is the canonical registration point for all built-in strategies.
// To add custom strategies, use StaticLoader.RegisterAgent instead of
// modifying this list.
func builtinAgentExecutors() []AgentExecutor {
	return []AgentExecutor{
		SemiconductorExecutor{},
		AISupplyChainExecutor{},
		LEOSatelliteExecutor{},
		ETFRotationExecutor{},
		FinancialsExecutor{},
		ShippingExecutor{},
		GrowthMomentumExecutor{},
		ValueYieldExecutor{},
		EarningsQualityExecutor{},
		TechnicalBreakoutExecutor{},
	}
}

func builtinRegimeExecutors() []RegimeExecutor {
	return []RegimeExecutor{
		TaiwanMacroRegimeExecutor{},
		ForeignFlowRegimeExecutor{},
	}
}

func builtinControlExecutors() []ControlExecutor {
	return []ControlExecutor{
		NewCRORiskExecutor(),
		CIOPortfolioExecutor{},
	}
}
