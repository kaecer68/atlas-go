package swarm

import (
	"context"
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// SwarmState is a lightweight state container replacing the full MiroFishSwarm
// simulation engine (GARCH, copula, 100-fish simulation). The simulation engine
// was demoted in PR #963; this type exists for backward compatibility with
// existing wiring: Phase3Controller, swarmPlugin, monitoring APIs.

type SwarmState struct {
	config domain.SwarmConfig
	status string
	mu     sync.RWMutex
}

// NewSwarmState creates a new state container. The config parameter is accepted
// for API compatibility but the simulation engine no longer runs.
func NewSwarmState(config domain.SwarmConfig) *SwarmState {
	return &SwarmState{
		config: config,
		status: "deprecated",
	}
}

// DeprecatedNewMiroFishSwarm is provided for backward compatibility.
// Use NewSwarmState instead.
var DeprecatedNewMiroFishSwarm = NewSwarmState

// Start is a no-op kept for API compatibility with BTM auto_swarm_simulation.
func (s *SwarmState) Start(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	logging.Debug("swarm", "start_skipped", "swarm simulation engine demoted in PR #963")
	return nil
}

// Stop is a no-op kept for API compatibility.
func (s *SwarmState) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return nil
}

// IsRunning always returns false since the simulation engine is disabled.
func (s *SwarmState) IsRunning() bool {
	return false
}

// GetLatestStatus returns a human-readable status string.
func (s *SwarmState) GetLatestStatus() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// GetLatestResult returns an empty result. Consumers should check ok=false
// and treat this as "no swarm data available".
func (s *SwarmState) GetLatestResult() (domain.SwarmSimulationResult, bool) {
	return domain.SwarmSimulationResult{}, false
}

// UpdateScenario is a no-op kept for API compatibility.
func (s *SwarmState) UpdateScenario() {}
