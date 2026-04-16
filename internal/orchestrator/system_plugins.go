package orchestrator

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/janus"
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
	s.persistentState = state
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

// WithPhase3Controller attaches the advanced Phase 3 optimization controller.
// If replay data is available, an adversarial scenario runner is automatically wired.
func (s *System) WithPhase3Controller(ctrl *Phase3Controller) *System {
	if s.host == nil {
		s.host = &PluginHost{}
	}
	// Sync sub-managers so existing plugins delegate to the controller when appropriate.
	for _, p := range s.host.plugins {
		switch pp := p.(type) {
		case *swarmPlugin:
			pp.controller = ctrl
		case *prismPlugin:
			pp.controller = ctrl
		case *spawningPlugin:
			pp.controller = ctrl
		}
	}
	s.host.Register(&phase3Plugin{controller: ctrl}, s.SystemCore)
	return s
}

// The following methods are retained for test backward compatibility.
// They look up the specific plugin in the host and delegate to it.

func (s *System) applySwarmConsensus(recs []domain.Recommendation) []domain.Recommendation {
	if s.host == nil {
		return recs
	}
	for _, p := range s.host.plugins {
		if sp, ok := p.(*swarmPlugin); ok {
			return sp.ProcessRecommendations(domain.RegimeNeutral, recs)
		}
	}
	return recs
}

func (s *System) applyPRISMWeights(recs []domain.Recommendation, regime domain.Regime) []domain.Recommendation {
	if s.host == nil {
		return recs
	}
	for _, p := range s.host.plugins {
		if pp, ok := p.(*prismPlugin); ok {
			return pp.ProcessRecommendations(regime, recs)
		}
	}
	return recs
}

func (s *System) runPhase3Optimization(quotes []domain.Quote, regime domain.Regime, asOf time.Time) {
	if s.host == nil {
		return
	}
	for _, p := range s.host.plugins {
		if pp, ok := p.(*phase3Plugin); ok {
			pp.PostSimulation(quotes, regime, asOf)
			return
		}
	}
}

func (s *System) schedulePRISMForRegime(regime domain.Regime, asOf time.Time) {
	if s.host == nil {
		return
	}
	for _, p := range s.host.plugins {
		if pp, ok := p.(*prismPlugin); ok {
			pp.PostSimulation(nil, regime, asOf)
			return
		}
	}
}

func (s *System) runSpawningCycle() {
	if s.host == nil {
		return
	}
	for _, p := range s.host.plugins {
		if sp, ok := p.(*spawningPlugin); ok {
			sp.PostSimulation(nil, domain.RegimeNeutral, time.Time{})
			return
		}
	}
}
