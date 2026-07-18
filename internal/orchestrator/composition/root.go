// Package composition provides the centralized dependency-wiring root for
// atlas-go simulation infrastructure. It owns singleton construction of shared
// components (classification tree, L1 mapper, exposure calculator) that were
// previously built per-callsite.
package composition

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/portfolio"
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
}

// NewRoot constructs the shared dependency root.
// It creates the L1 mapper from the default industry classification tree.
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

// InjectSectorDeps wires the mapper and exposure calculator into a
// previously-constructed System. Callers should invoke this after
// system construction but before the first simulation run.
func (r *Root) InjectSectorDeps(sys *orchestrator.System) {
	sys.WithSectorL1Mapper(r.Mapper).WithSectorExposureCalculator(r.Calc)
}
