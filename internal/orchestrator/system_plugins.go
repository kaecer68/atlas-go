package orchestrator

import (
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventlogic"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/spawning"
	"github.com/kaecer68/atlas-go/internal/swarm"
)

// WithJANUS attaches a JANUS engine to the system for backtest validation.
func (s *System) WithJANUS(j *janus.Engine) *System {
	if s.host == nil {
		s.host = &PluginHost{}
	}
	s.host.Register(&janusPlugin{engine: j}, s.SystemCore)
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

func (s *System) WithEventLogic(
	detector *eventlogic.PatternDetector,
	corrector *eventlogic.SelfCorrector,
	saveRulesPath string,
	historyRecorder *eventlogic.HistoryRecorder,
) *System {
	if s.host == nil {
		s.host = &PluginHost{}
	}
	s.host.Register(&eventlogicPlugin{
		detector: detector, corrector: corrector,
		saveRulesPath: saveRulesPath, historyRecorder: historyRecorder,
	}, s.SystemCore)
	return s
}
