package orchestrator

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/reflexivity"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/screener"
	"github.com/kaecer68/atlas-go/internal/sim"
	"github.com/kaecer68/atlas-go/internal/strategy"
	"github.com/kaecer68/atlas-go/internal/tax"
)

// buildSimulationCore constructs the core simulation subsystem.
func buildSimulationCore(cfg config.Config, registry domain.AgentRegistry, policy baseline.Policy, ds *replay.Dataset) SimulationCore {
	return SimulationCore{
		cfg:      cfg,
		provider: selectProvider(cfg),
		engine:   buildSimEngine(policy, ds),
		registry: registry,
		policy:   policy,
		ledger:   ledger.NewStore(cfg.LedgerDir),
		replay:   ds,
		session:  newSession(cfg, ds),
		ctx:      context.Background(),
	}
}

func buildSimEngine(policy baseline.Policy, ds *replay.Dataset) *sim.Engine {
	optimizer := portfolio.NewOptimizer()
	runtimeParams := loadRuntimeParamsOrDefault()
	factorEngine, hp, fp := buildFactorEngine(runtimeParams)
	optimizer.WithHistoricalPrices(hp).WithFundamentalProvider(fp).WithFactorEngine(factorEngine)

	thresholdEngine := sim.NewDynamicThresholdEngine()

	return sim.NewEngine(policy.Constraints).
		WithOptimizer(optimizer).
		WithThresholdEngine(thresholdEngine).
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
		fmt.Printf("[System] warn: failed to load historical prices: %v\n", err)
	}
	fp := portfolio.NewFundamentalProvider()
	if err := fp.LoadFromJSON("data/fundamentals.json"); err != nil {
		fmt.Printf("[System] warn: failed to load fundamentals: %v\n", err)
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
func buildStrategyLayer() StrategyLayer {
	strategyRegistry := strategy.NewRegistryWithDefaults()
	comparisonEngine := strategy.NewComparisonEngine(20)
	strategySelector := strategy.NewSelector(strategyRegistry, comparisonEngine)
	thresholdEngine := sim.NewDynamicThresholdEngine()

	return StrategyLayer{
		strategyRegistry: strategyRegistry,
		strategySelector: strategySelector,
		comparisonEngine: comparisonEngine,
		thresholdEngine:  thresholdEngine,
	}
}

// buildRiskOps constructs the risk management subsystem.
func buildRiskOps(cfg config.Config, eventBus *eventbus.ChannelEventBus) RiskOps {
	return RiskOps{
		eventBus:       eventBus,
		clampingLogger: newClampingLogger(filepath.Join(cfg.LedgerDir, "clamping_events.jsonl")),
	}
}

// buildPluginRegistry constructs the plugin registry with screener and factor engine.
func buildPluginRegistry(factorEngine *portfolio.FactorEngine, fp *portfolio.FundamentalProvider) *PluginRegistry {
	screenerEngine := screener.NewEngine(factorEngine, fp)
	return NewPluginRegistry().WithScreener(screenerEngine).WithFactorEngine(factorEngine)
}

func loadRuntimeParamsOrDefault() *portfolio.RuntimeParameters {
	paramsCfg, err := config.LoadParametersConfig(config.Load().ParametersConfigPath)
	if err != nil || paramsCfg == nil {
		paramsCfg = config.DefaultParametersConfig()
	}
	return portfolio.ToRuntimeParameters(paramsCfg)
}
