package orchestrator

import (
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/reflexivity"
	"github.com/kaecer68/atlas-go/internal/spawning"
	"github.com/kaecer68/atlas-go/internal/swarm"
)

// NewProductionSystem builds a fully-wired System for dependency-graph visibility
// with an internally-created EventBus.
func NewProductionSystem(cfg config.Config, opts ...SystemOption) (*System, error) {
	return NewProductionSystemWithEventBus(cfg, nil, opts...)
}

// NewProductionSystemWithEventBus builds a fully-wired System, passing the
// provided EventBus to NewSystemWithEventBus. If eventBus is nil, an internal
// EventBus is created (backward-compatible).
func NewProductionSystemWithEventBus(cfg config.Config, eventBus *eventbus.ChannelEventBus, opts ...SystemOption) (*System, error) {
	system, err := NewSystemWithEventBus(cfg, eventBus, opts...)
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

	spawnCfg := spawning.DefaultSpawningConfig()
	spawnCfg.PromptsDir = filepath.Join(cfg.WorkDir, "prompts")
	sm := spawning.NewSpawningManager(&system.Sim().registry, spawnCfg)
	system.WithSpawning(sm)

	re := reflexivity.NewReflexivityEngine()
	ctrl := NewPhase3Controller(&system.Sim().registry, pm, sw, sm, re, system.Sim().ledger)
	system.WithPhase3Controller(ctrl)

	system.WithStrategyEvolver(NewStrategyEvolver())

	return system, nil
}
