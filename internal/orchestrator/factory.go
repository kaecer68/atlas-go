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
// It preserves the existing chainable-builder API but surfaces cross-package
// constructor calls so that static analysis tools (go-callvis, godepgraph) can
// trace cmd/atlas -> orchestrator -> {prism, swarm, janus, spawning, reflexivity}.
func NewProductionSystem(cfg config.Config) *System {
	system := NewSystem(cfg)

	pm := prism.NewPRISMManager(prism.DefaultPRISMConfig())
	system.WithPRISM(pm)

	sw := swarm.NewMiroFishSwarm(swarm.DefaultSwarmConfig())
	system.WithSwarm(sw)

	je := janus.NewEngineWithConfig(janus.DefaultJANUSConfig())
	system.WithJANUS(je)

	sm := spawning.NewSpawningManager(&system.registry, spawning.DefaultSpawningConfig())
	system.WithSpawning(sm)

	re := reflexivity.NewReflexivityEngine()
	ctrl := NewPhase3Controller(&system.registry, pm, sw, sm, re, system.ledger)
	system.WithPhase3Controller(ctrl)

	return system
}
