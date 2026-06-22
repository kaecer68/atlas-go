package orchestrator

import (
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/sim"
	"github.com/kaecer68/atlas-go/internal/strategy"
)

// Plugin/strategy accessors extracted from system.go (PR1 of #611
// sub-issue-4). These are thin pass-throughs to internal state used by
// the API layer and BackgroundTaskManager tasks.

func (s *System) Registry() domain.AgentRegistry {
	return s.Sim().registry
}

func (s *System) GetPlugins() *PluginRegistry {
	return s.plugins
}

func (s *System) GetExecutionPolicy() domain.ExecutionPolicy {
	return s.Sim().policy.ExecutionPolicy
}

func (s *System) GetCurrentStrategy() *strategy.Strategy {
	if s.strat.strategySelector == nil {
		return nil
	}
	return s.strat.strategySelector.GetCurrentStrategy()
}

func (s *System) GetStrategySelector() *strategy.Selector {
	return s.strat.strategySelector
}

// GetStrategyAllocator returns the multi-strategy allocator (nil if not attached).
func (s *System) GetStrategyAllocator() *strategy.StrategyAllocator {
	return s.strat.strategyAllocator
}

// WithStrategyAllocator attaches a risk-parity strategy allocator (P2).
// When attached, sessions can use multi-strategy allocation instead of single-strategy selection.
// nil-safe: if nil, Selector path is used (backward compatible).
func (s *System) WithStrategyAllocator(sa *strategy.StrategyAllocator) *System {
	s.strat.strategyAllocator = sa
	return s
}

// GetStrategyEvolver returns the strategy evolver (nil if not attached).
func (s *System) GetStrategyEvolver() *StrategyEvolver {
	return s.strat.strategyEvolver
}

// WithStrategyEvolver attaches a strategy evolver for macro-driven state transitions.
// When attached, the macro pipeline evaluates strategy evolution after drawdown assessment.
// nil-safe: if nil, no strategy evolution occurs (backward compatible).
func (s *System) WithStrategyEvolver(ev *StrategyEvolver) *System {
	s.strat.strategyEvolver = ev
	return s
}

func (s *System) GetThresholdEngine() *sim.DynamicThresholdEngine {
	return s.strat.thresholdEngine
}
