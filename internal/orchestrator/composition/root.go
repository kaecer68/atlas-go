// Package composition provides the centralized dependency-wiring root for
// atlas-go simulation infrastructure. It owns singleton construction of shared
// components (classification tree, L1 mapper, exposure calculator, WeightEngine)
// that were previously built per-callsite.
//
// SA06: Composition root extends from SA07 worktree B to also hold the shared
// WeightEngine and provides BuildSystem(path) for the six main.go constructor
// matrix: admin_manual, auto_daily, stress_test_daily, cli_simulation (rotation
// allowed), auto_experiment, live_trading (rotation denied). IndustryService no
// longer constructs a nil-provider partial engine; the dashboard wires its
// adapters through Root.
package composition

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// CompositionPath identifies the caller context for wiring decisions.
// Only simulation paths (admin/auto/stress/cli) may enable sector rotation;
// live and auto_experiment paths receive DisableSectorRotation.
type CompositionPath string

const (
	PathAdminManual     CompositionPath = "admin_manual"
	PathAutoDaily       CompositionPath = "auto_daily"
	PathStressTestDaily CompositionPath = "stress_test_daily"
	PathCLISimulation   CompositionPath = "cli_simulation"
	PathAutoExperiment  CompositionPath = "auto_experiment"
	PathLiveTrading     CompositionPath = "live_trading"
)

// AllowsSectorRotation is true only for the four simulation paths.
func (p CompositionPath) AllowsSectorRotation() bool {
	return p == PathAdminManual || p == PathAutoDaily ||
		p == PathStressTestDaily || p == PathCLISimulation
}

// Root holds the shared, singly-constructed dependencies that were
// previously spread across multiple constructor call-sites.
type Root struct {
	Cfg config.Config

	// SA07: symbol→L1 mapper and sector-exposure calculator.
	Mapper *industry.SymbolL1Mapper
	Calc   *portfolio.SectorExposureCalculator

	// SA06: shared WeightEngine. Set via WithWeightEngine by the
	// dashboard (for monitoring/web UI) or by simulation orchestrator.
	// Nil until wired.
	weightEngine sectorallocation.WeightEngine
}

// NewRoot constructs the shared dependency root.
// It creates the L1 mapper from the default industry classification tree.
// The WeightEngine is created lazily — callers must wire it via
// WithWeightEngine before first use.
func NewRoot(cfg config.Config) (*Root, error) {
	tree := industry.DefaultClassification()
	mapper, err := industry.NewSymbolL1Mapper(tree)
	if err != nil {
		return nil, fmt.Errorf("composition: mapper: %w", err)
	}
	return &Root{
		Cfg:    cfg,
		Mapper: mapper,
		Calc:   &portfolio.SectorExposureCalculator{},
	}, nil
}

// WithWeightEngine sets the shared WeightEngine on the root.
// Callers (dashboard or simulation orchestrator) are responsible
// for wiring the appropriate providers before setting the engine.
func (r *Root) WithWeightEngine(eng sectorallocation.WeightEngine) *Root {
	r.weightEngine = eng
	return r
}

// WeightEngine returns the shared engine, or nil if not yet wired.
func (r *Root) WeightEngine() sectorallocation.WeightEngine {
	return r.weightEngine
}

// InjectSectorDeps wires the mapper and exposure calculator into a
// previously-constructed System. Callers should invoke this after
// system construction but before the first simulation run.
func (r *Root) InjectSectorDeps(sys *orchestrator.System) {
	sys.WithSectorL1Mapper(r.Mapper).WithSectorExposureCalculator(r.Calc)
}

// BuildSystem creates a fully-wired System for the given CompositionPath.
// It constructs the system through the production factory (maturity tracker,
// Darwinian, PRISM, JANUS, spawning, reflexivity, Phase3) and injects
// shared dependencies (L1 mapper, exposure calculator, WeightEngine).
//
// Paths that do not allow sector rotation (auto_experiment, live_trading)
// still receive the L1 mapper and calculator but skip WeightEngine wiring.
// Negative test: BuildSystem(PathLiveTrading) → system is constructed but
// sector rotation is disabled.
func (r *Root) BuildSystem(
	path CompositionPath,
	eventBus *eventbus.ChannelEventBus,
	janusEngine *janus.Engine,
	opts ...orchestrator.SystemOption,
) (*orchestrator.System, error) {
	system, err := orchestrator.NewProductionSystemWithEventBus(r.Cfg, eventBus, janusEngine, opts...)
	if err != nil {
		return nil, fmt.Errorf("composition: build system for %s: %w", path, err)
	}

	// Always inject sector deps — mapper and exposure calculator are
	// safe to include on all paths (they are read-only utilities).
	r.InjectSectorDeps(system)

	// For denied paths (auto_experiment, live_trading) we skip wiring
	// to prevent any accidental sector-allocation mutation.
	if path.AllowsSectorRotation() && r.weightEngine != nil {
		// SA06 establishes the plumbing; actual consumption goes live
		// when SA08 delivers the session policy store + allocator.
		system.WithSectorWeightEngine(r.weightEngine)
	}

	return system, nil
}
