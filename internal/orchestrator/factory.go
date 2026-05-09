package orchestrator

import (
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/reflexivity"
	"github.com/kaecer68/atlas-go/internal/spawning"
	"github.com/kaecer68/atlas-go/internal/swarm"
)

// NewProductionSystem builds a fully-wired System for dependency-graph visibility.
// It registers each subsystem as a Plugin on the System's PluginHost so that
// cross-package boundaries are explicit and the simulation loop delegates to
// a unified lifecycle interface.
func NewProductionSystem(cfg config.Config) (*System, error) {
	system, err := NewSystem(cfg)
	if err != nil {
		return nil, err
	}

	system.WithDarwinian(system.Port().darwinian)

	pm := prism.NewPRISMManager(prism.DefaultPRISMConfig())
	system.WithPRISM(pm)

	sw := swarm.NewMiroFishSwarm(swarm.DefaultSwarmConfig())
	system.WithSwarm(sw)

	je := janus.NewEngineWithConfig(janus.DefaultJANUSConfig())
	system.WithJANUS(je)

	sm := spawning.NewSpawningManager(&system.Sim().registry, spawning.DefaultSpawningConfig())
	system.WithSpawning(sm)

	re := reflexivity.NewReflexivityEngine()
	ctrl := NewPhase3Controller(&system.Sim().registry, pm, sw, sm, re, system.Sim().ledger)
	system.WithPhase3Controller(ctrl)

	return system, nil
}
