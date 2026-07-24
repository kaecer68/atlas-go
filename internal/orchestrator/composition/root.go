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
	"context"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

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

	// SA08: closure store and session resolver for StrategyEvolver.
	// Set via WithClosureStore / WithSessionResolver before BuildSystem.
	closureStore    sectorallocation.ClosureStore
	sessionResolver orchestrator.TradingSessionResolver
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

// WithClosureStore sets the closure store for SA08 StrategyEvolver wiring.
func (r *Root) WithClosureStore(store sectorallocation.ClosureStore) *Root {
	r.closureStore = store
	return r
}

// WithSessionResolver sets the trading session resolver for SA08 StrategyEvolver wiring.
func (r *Root) WithSessionResolver(resolver orchestrator.TradingSessionResolver) *Root {
	r.sessionResolver = resolver
	return r
}

// InjectSectorDeps wires the mapper and exposure calculator into a
// previously-constructed System. Callers should invoke this after
// system construction but before the first simulation run.
func (r *Root) InjectSectorDeps(sys *orchestrator.System) {
	sys.WithSectorL1Mapper(r.Mapper).WithSectorExposureCalculator(r.Calc)
}

// buildWeightEngine lazily constructs a fully-wired default WeightEngine using
// NewDefaultEngineWithProjector (SA04 path). Adapters are extracted from the
// industry service infrastructure (cycle/seasonal/linkage); narrative/macro/factor
// use nil until SA08 completes the full adapter wiring.
func (r *Root) buildWeightEngine() sectorallocation.WeightEngine {
	params := config.GetParametersConfig()

	// prior — loaded from ParametersConfig; nil on pre-SA02 config.
	prior, err := sectorallocation.LoadStrategicPrior(params)
	if err != nil {
		// Cannot construct a functional engine without prior; degrade.
		return nil
	}

	// projector — SA04 default constraints (0.5 min / 0.005 max / 1e-9 sum tolerance).
	projector := sectorallocation.NewDefaultProjector()

	// Extract industry adapters from the shared L1 mapper's classification tree.

	seasonalEngine := industry.NewSeasonalEngine()
	cycleTracker := industry.NewCycleTracker()
	linkageAnalyzer := industry.NewLinkageAnalyzer()

	// Wire basic supply-chain graph into seasonal engine (no narrative provider yet).
	seasonalEngine.SetLinkageGraph(linkageAnalyzer.GetSupplyChainGraph())

	// Wire validators into cycle tracker for multi-dimensional confidence.
	cycleTracker.SetExternalValidators(seasonalEngine, linkageAnalyzer)

	// weight bounds from Darwinian parameters.
	weightMin := params.Darwinian.WeightMin.Value
	weightMax := params.Darwinian.WeightMax.Value

	// narrative adapter — NarrativeEngine is a stateless factory, construct directly.
	// Uses the same hardcoded theme→bias map as dashboard_api.go:416-440.
	narrativeEngine := narrative.NewNarrativeEngine()
	narrativeThemeMap := map[string]map[string]float64{
		"US_rates_up":             {"financials": 0.05, "semiconductor": -0.04, "electronics": -0.03},
		"US_rates_down":           {"financials": -0.05, "semiconductor": 0.04, "electronics": 0.03},
		"USD_strengther":          {"semiconductor": -0.04, "electronics": -0.03, "tourism": -0.03},
		"USD_weaker":              {"semiconductor": 0.03, "electronics": 0.03, "tourism": 0.03},
		"risk_on":                 {"semiconductor": 0.05, "electronics": 0.04, "financials": 0.03},
		"risk_off":                {"semiconductor": -0.05, "electronics": -0.04, "financials": -0.03},
		"JPY_carry_unwind":        {"financials": -0.03, "semiconductor": -0.05, "electronics": -0.03},
		"geopolitical_risk_spike": {"shipping": -0.05, "energy": -0.05, "industrial": -0.03},
		"oil_price_shock":         {"shipping": -0.04, "energy": -0.04, "industrial": -0.03},
		"semiconductor_downturn":  {"semiconductor": -0.08, "ai_supply_chain": -0.06, "electronics": -0.06},
	}
	narrativeAdapter := sectorallocation.NewNarrativeAdapter(
		func(_ context.Context, industryID string) (float64, float64, string, error) {
			events := narrativeEngine.DetectEvents(narrative.MarketNarrativeData{})
			var totalBias float64
			var maxConf float64
			activeTheme := ""
			for _, e := range events {
				if industryBiases, ok := narrativeThemeMap[e.Theme]; ok {
					if bias, ok := industryBiases[industryID]; ok {
						totalBias += bias * e.Confidence * e.HitRate
						if e.Confidence*e.HitRate > maxConf {
							maxConf = e.Confidence * e.HitRate
							activeTheme = e.Theme
						}
					}
				}
			}
			return totalBias, maxConf, activeTheme, nil
		},
	)
	// macro adapter — no-op until macro pipeline provides MacroDataSnapshot (future).
	macroAdapter := sectorallocation.MacroProviderFunc(
		func(_ context.Context, industryID, _, _ string) (float64, error) {
			return 0.0, nil
		},
	)

	// factor adapter — no-op until factor provider is wired (future).
	factorAdapter := sectorallocation.FactorProviderFunc(
		func(_ context.Context, _ string) (float64, error) {
			return 0.0, nil
		},
	)

	return sectorallocation.NewDefaultEngineWithProjector(
		params.SectorAllocation,
		prior,
		projector,
		sectorallocation.NewCycleAdapter(cycleTracker),
		sectorallocation.NewSeasonalAdapter(seasonalEngine),
		sectorallocation.NewLinkageAdapter(linkageAnalyzer, nil),
		narrativeAdapter,
		macroAdapter,
		factorAdapter,
		weightMin,
		weightMax,
	)
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
	// SA08 Gap A+B: lazily construct weightEngine if nil (path.AllowsSectorRotation only).
	if path.AllowsSectorRotation() {
		if r.weightEngine == nil {
			r.weightEngine = r.buildWeightEngine()
		}
		if r.weightEngine != nil {
			// SA06 establishes the plumbing; actual consumption goes live
			// when SA08 delivers the session policy store + allocator.
			system.WithSectorWeightEngine(r.weightEngine)
		}
	}
	// SA08: wire StrategyEvolver with closure store, session resolver,
	// and weight engine so ApplySectorRotation is fully functional.
	if evolver := system.GetStrategyEvolver(); evolver != nil {
		evolver.WithClosureStore(r.closureStore).
			WithSessionResolver(r.sessionResolver).
			WithSectorWeightEngine(r.weightEngine)
	}

	return system, nil
}
