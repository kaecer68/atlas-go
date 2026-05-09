package orchestrator

import (
	"context"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/reflexivity"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/screener"
	"github.com/kaecer68/atlas-go/internal/sim"
	"github.com/kaecer68/atlas-go/internal/strategy"
	"github.com/kaecer68/atlas-go/internal/tax"
)

// buildSimulationCore constructs the core simulation subsystem.
func buildSimulationCore(cfg config.Config, registry domain.AgentRegistry, policy baseline.Policy, ds *replay.Dataset, optimizer *portfolio.Optimizer, store *ledger.Store) SimulationCore {
	return SimulationCore{
		cfg:      cfg,
		provider: selectProvider(cfg),
		engine:   buildSimEngine(policy, optimizer),
		registry: registry,
		policy:   policy,
		ledger:   store,
		replay:   ds,
		session:  newSession(cfg, ds),
		ctx:      context.Background(),
	}
}

func buildSimEngine(policy baseline.Policy, optimizer *portfolio.Optimizer) *sim.Engine {
	return sim.NewEngine(policy.Constraints).
		WithOptimizer(optimizer).
		WithThresholdEngine(sim.NewDynamicThresholdEngine()).
		WithTaxCalculator(tax.NewTaiwanTaxCalculator(domain.DefaultTaiwanTaxConfig())).
		WithReflexivityRules(
			reflexivity.PriceToFundamentalsRule{},
			reflexivity.PnLBehaviorRule{},
			reflexivity.NarrativeFlowsRule{Threshold: 3},
			reflexivity.MarketPolicyRule{Threshold: 0.03},
			reflexivity.NewReversalDetectionRule(),
		)
}

// buildFactorEngine constructs the factor computation pipeline.
func buildFactorEngine(runtimeParams *portfolio.RuntimeParameters) (*portfolio.FactorEngine, *portfolio.HistoricalPrices, *portfolio.FundamentalProvider) {
	hp := portfolio.NewHistoricalPrices()
	if err := hp.LoadFromExtendedJSONL("data/replay/tw_extended_90days.jsonl"); err != nil {
		logging.Warn("composition", "failed to load historical prices", "err", err)
	}
	fp := portfolio.NewFundamentalProvider()
	if err := fp.LoadFromJSON("data/fundamentals.json"); err != nil {
		logging.Warn("composition", "failed to load fundamentals", "err", err)
	}
	fe := portfolio.NewFactorEngine().
		WithParameters(runtimeParams).
		WithHistoricalPrices(hp).
		WithFundamentalProvider(fp)
	return fe, hp, fp
}

// buildPortfolioManager constructs the portfolio management subsystem.
func buildPortfolioManager(runtimeParams *portfolio.RuntimeParameters, registry domain.AgentRegistry, eventBus *eventbus.ChannelEventBus, factorEngine *portfolio.FactorEngine) PortfolioManager {
	darwinian := portfolio.NewDarwinianWeightManager("data/state/darwinian_weights.json").
		WithHistoryPath("data/state/darwinian_history.jsonl").
		WithParameters(runtimeParams)
	darwinian.InitializeFromRegistry(registry)
	_ = darwinian.Load()
	darwinian.WithEventBus(eventBus)

	factorWeightEngine := portfolio.NewFactorWeightEngine()

	return PortfolioManager{
		darwinian:          darwinian,
		alphaDiscovery:     NewAlphaDiscoveryEngine(factorEngine),
		factorWeightEngine: factorWeightEngine,
	}
}

// buildStrategyLayer constructs the strategy subsystem.
func buildStrategyLayer(thresholdEngine *sim.DynamicThresholdEngine) StrategyLayer {
	strategyRegistry := strategy.NewRegistryWithDefaults()
	comparisonEngine := strategy.NewComparisonEngine(20)
	strategySelector := strategy.NewSelector(strategyRegistry, comparisonEngine)

	return StrategyLayer{
		strategyRegistry: strategyRegistry,
		strategySelector: strategySelector,
		comparisonEngine: comparisonEngine,
		thresholdEngine:  thresholdEngine,
	}
}

// buildMacroEngines constructs the macro assessment engines.
func buildMacroEngines(sectorDataDir string) (macroRiskEngine *narrative.MacroRiskAssessmentEngine, structuralTrendEngine *narrative.StructuralTrendEngine, macroDrawdownEngine *risk.MacroAwareDrawdownEngine, sectorDataProvider *marketdata.SectorDataProvider) {
	sectorDataProvider = marketdata.NewSectorDataProvider(sectorDataDir)
	return narrative.NewMacroRiskAssessmentEngine(), narrative.NewStructuralTrendEngine(), risk.NewMacroAwareDrawdownEngine(), sectorDataProvider
}

// buildRiskOps constructs the risk management subsystem.
func buildRiskOps(cfg config.Config, eventBus *eventbus.ChannelEventBus, macroRiskEngine *narrative.MacroRiskAssessmentEngine, structuralTrendEngine *narrative.StructuralTrendEngine, macroDrawdownEngine *risk.MacroAwareDrawdownEngine, sectorDataProvider *marketdata.SectorDataProvider) RiskOps {
	return RiskOps{
		eventBus:              eventBus,
		clampingLogger:        newClampingLogger(filepath.Join(cfg.LedgerDir, "clamping_events.jsonl")),
		macroRiskEngine:       macroRiskEngine,
		structuralTrendEngine: structuralTrendEngine,
		macroDrawdownEngine:   macroDrawdownEngine,
		sectorDataProvider:    sectorDataProvider,
	}
}

// buildPluginRegistry constructs the plugin registry with screener and factor engine.
func buildPluginRegistry(factorEngine *portfolio.FactorEngine, fp *portfolio.FundamentalProvider) *PluginRegistry {
	screenerEngine := screener.NewEngine(factorEngine, fp)
	return NewPluginRegistry().WithScreener(screenerEngine).WithFactorEngine(factorEngine)
}

func loadRuntimeParamsOrDefault(parametersConfigPath string) *portfolio.RuntimeParameters {
	paramsCfg, err := config.LoadParametersConfig(parametersConfigPath)
	if err != nil || paramsCfg == nil {
		paramsCfg = config.DefaultParametersConfig()
	}
	return portfolio.ToRuntimeParameters(paramsCfg)
}
