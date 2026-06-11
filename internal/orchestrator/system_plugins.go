package orchestrator

import (
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/spawning"
	"github.com/kaecer68/atlas-go/internal/strategy_techniques"
	"github.com/kaecer68/atlas-go/internal/swarm"
)

// WithJANUS attaches a JANUS engine to the system for backtest validation.
func (s *System) WithJANUS(j *janus.Engine, pm *prism.PRISMManager) *System {
	if s.host == nil {
		s.host = &PluginHost{}
	}
	s.host.Register(&janusPlugin{engine: j, prismManager: pm}, s.SystemCore)
	return s
}

// WithPersistentState enables cross-day simulation state carry-over for backtests.
func (s *System) WithPersistentState(state *domain.SimulationState) *System {
	s.Sim().persistentState = state
	return s
}

// WithPRISM attaches a PRISM training manager to the system.
// If replay data is available, a real training executor is automatically wired.
func (s *System) WithPRISM(pm *prism.PRISMManager) *System {
	if s.host == nil {
		s.host = &PluginHost{}
	}
	s.host.Register(&prismPlugin{manager: pm}, s.SystemCore)
	return s
}

// WithSwarm attaches a MiroFish swarm simulator to the system.
func (s *System) WithSwarm(sw *swarm.MiroFishSwarm) *System {
	if s.host == nil {
		s.host = &PluginHost{}
	}
	s.host.Register(&swarmPlugin{swarm: sw}, s.SystemCore)
	return s
}

// WithSpawning attaches a spawning manager for automated agent creation.
func (s *System) WithSpawning(sm *spawning.SpawningManager) *System {
	if s.host == nil {
		s.host = &PluginHost{}
	}
	s.host.Register(&spawningPlugin{manager: sm}, s.SystemCore)
	return s
}

// WithDarwinian attaches a Darwinian weight manager to the system for dynamic
// agent weight adjustment based on performance.
func (s *System) WithDarwinian(dw *portfolio.DarwinianWeightManager) *System {
	s.Port().darwinian = dw
	return s
}

// WithPhase3Controller attaches the advanced Phase 3 optimization controller.
// If replay data is available, an adversarial scenario runner is automatically wired.
func (s *System) WithPhase3Controller(ctrl *Phase3Controller) *System {
	if s.host == nil {
		s.host = &PluginHost{}
	}
	for _, p := range s.host.plugins {
		if ca, ok := p.(ControllerAware); ok {
			ca.SetController(ctrl)
		}
	}
	s.host.Register(&phase3Plugin{controller: ctrl}, s.SystemCore)
	s.phase3Ctrl = ctrl
	return s
}

// WithStrategyTechniques wires the new 5-layer strategy techniques library
// into the plugin host. This is the StrategyFrame-based replacement for the
// legacy eventlogic plugin.
//
// Framework:
//   - L1 Global Liquidity, L2 Foreign Capital Behavior, L3 Industry Catalysts,
//     L4 FX & Chips, L5 Geopolitics
//   - 4 core leading indicators: ForeignInvestorNet, TSMADR, NVDA, DXY
//   - Hybrid self-correction: rule-based attribution + LLM annotation
//     (LLM path is filled in by Wave 4; this scaffold is event-loop safe)
//
// Migration: this is the new code path. Existing eventlogicPlugin remains
// wired only for backward compatibility during the migration window
// (eventlogic is retired in Wave 5 cleanup after the production seeds
// migrate and the eventbus references move).
//
// Parameter savePath is the on-disk location for the 9 production seeds
// (data/seeds/strategy_techniques.json). It is read once at boot via
// strategy_techniques.LoadFromFile; live writes are queued but flushed by
// the registry's own background task, not by this plugin.
func (s *System) WithStrategyTechniques(
	registry *strategy_techniques.Registry,
	savePath string,
) *System {
	if s.host == nil {
		s.host = &PluginHost{}
	}
	s.host.Register(&strategyTechniquesPlugin{
		registry: registry,
		savePath: savePath,
	}, s.SystemCore)
	return s
}
