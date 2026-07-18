package orchestrator

import (
	"context"
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/importer"
	"github.com/kaecer68/atlas-go/internal/industry"
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
func buildSimulationCore(cfg config.Config, registry domain.AgentRegistry, policy baseline.Policy, ds *replay.Dataset, optimizer *portfolio.Optimizer, store ledger.OutcomeStore) SimulationCore {
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

// SetProvider replaces the default provider created by selectProvider().
// Used by main.go to inject a GatewayBackedProvider with independent rate limiting.
func (sc *SimulationCore) SetProvider(p marketdata.Provider) {
	sc.provider = p
}

// RefreshETFNAV refreshes ETF NAV for all tracked symbols.
//
// Uses a tiered strategy via TWSEETFNAVScraper:
//  1. Attempts TWSE NAV scraping (Tier 1 — deferred until a working endpoint exists).
//  2. Falls back to close-price proxy via the market data provider (Tier 2).
//
// Returns the number of symbols whose NAV was updated.
func (sc *SimulationCore) RefreshETFNAV(ctx context.Context) int {
	if sc.factorEngine == nil || sc.provider == nil {
		return 0
	}

	etfAnalyzer := sc.factorEngine.GetETFAnalyzer()
	if etfAnalyzer == nil {
		return 0
	}

	// Build scraper wrapping the current provider as Tier-2 fallback.
	scraper := marketdata.NewTWSEETFNAVScraper(sc.provider)
	navProv := marketdata.NewETFNAVProvider(sc.provider).WithScraper(scraper)

	return etfAnalyzer.RefreshNAVFromFetcher(ctx, navProv, false)
}

func buildSimEngine(policy baseline.Policy, optimizer *portfolio.Optimizer) *sim.Engine {
	return sim.NewEngine(policy.Constraints).
		WithOptimizer(optimizer).
		WithThresholdEngine(sim.NewDynamicThresholdEngine()).
		WithTaxCalculator(tax.NewTaiwanTaxCalculator(config.GetParametersConfig().Tax.ToConfig())).
		WithPreTradeGate(risk.NewPreTradeGate()).
		WithReflexivityRules(
			reflexivity.PriceToFundamentalsRule{},
			reflexivity.PnLBehaviorRule{},
			reflexivity.NarrativeFlowsRule{Threshold: 3},
			reflexivity.MarketPolicyRule{Threshold: 0.03},
			reflexivity.NewReversalDetectionRule(),
		)
}

// buildFactorEngine constructs the factor computation pipeline.
// macroSnap is a pointer to a MacroDataSnapshot that gets updated before each simulation
// run; the PM provider closure reads from it at scoring time. nil is safe (PM scores = 0).
func buildFactorEngine(runtimeParams *portfolio.RuntimeParameters, macroSnap *marketdata.MacroDataSnapshot, replayCSVPath string) (*portfolio.FactorEngine, *portfolio.HistoricalPrices, *portfolio.FundamentalProvider) {
	// Derive JSONL path from CSV path to match conversion target (P2).
	ext := filepath.Ext(replayCSVPath)
	jsonlPath := replayCSVPath[:len(replayCSVPath)-len(ext)] + ".jsonl"

	// Derive fundamentals.json path from the replay CSV path. Both files live
	// in the same data/ directory; this avoids relying on process cwd (which
	// breaks when atlas is launched from elsewhere, e.g. IDE or container).
	fundamentalsPath := filepath.Join(filepath.Dir(replayCSVPath), "fundamentals.json")

	hp := portfolio.NewHistoricalPrices()

	// Auto-convert CSV→JSONL if JSONL is missing but CSV exists (P1).
	if _, err := os.Stat(jsonlPath); os.IsNotExist(err) {
		if _, csvErr := os.Stat(replayCSVPath); csvErr == nil {
			logging.Info("composition", "replay JSONL missing, converting from CSV", "csv", replayCSVPath, "jsonl", jsonlPath)
			if convertErr := importer.ImportTWOpenDataCSVToJSONL(replayCSVPath, jsonlPath); convertErr != nil {
				logging.Warn("composition", "auto-convert CSV→JSONL failed", "err", convertErr)
			}
		}
	}

	if err := hp.LoadFromExtendedJSONL(jsonlPath); err != nil {
		logging.Warn("composition", "failed to load historical prices", "err", err)
	}
	fp := portfolio.NewFundamentalProvider()
	if err := fp.LoadFromJSON(fundamentalsPath); err != nil {
		logging.Warn("composition", "failed to load fundamentals", "err", err)
	}

	// P3-1: Wire precious metals macro context provider using live snapshot data.
	// CPIYoY uses live snapshot data when available; falls back to config parameter.
	// New PM fields populated from ParametersConfig.PreciousMetals and macro snapshot.
	pmProvider := func(symbol string) *portfolio.PreciousMetalsContext {
		if macroSnap == nil || macroSnap.RecordedAt == 0 {
			return nil
		}
		pc := config.GetParametersConfig()
		cpiYoY := pc.Narrative.InflationEstimate.Value
		if macroSnap.CPIYoY.Symbol != "" && macroSnap.CPIYoY.Value != 0 {
			cpiYoY = macroSnap.CPIYoY.Value
		}
		realRate := macroSnap.US10Y.Value - cpiYoY

		cbTrend := pc.PreciousMetals.CentralBankBuyingTrend.Value
		var cbReserveTrend float64
		switch cbTrend {
		case "accelerating":
			cbReserveTrend = 0.5
		case "decelerating":
			cbReserveTrend = -0.3
		default:
			cbReserveTrend = 0.0
		}

		comexNetLong := pc.PreciousMetals.COMEXDefaultNetLong.Value
		if macroSnap.Gold.Symbol != "" {
			comexNetLong = macroSnap.Gold.Value
		}

		gsRatioZ := computeGoldSilverRatioZ(macroSnap)

		return &portfolio.PreciousMetalsContext{
			RealRate:            realRate,
			VIX:                 macroSnap.VIX.Value,
			DXY:                 macroSnap.DXY.Value,
			CPIYoY:              cpiYoY,
			CentralBankNetBuy:   pc.PreciousMetals.CentralBankNetBuy.Value,
			CBReserveTrend:      cbReserveTrend,
			IndiaGoldImportsYoY: pc.PreciousMetals.IndiaGoldImportsYoY.Value,
			ChinaGoldImportsYoY: pc.PreciousMetals.ChinaGoldImportsYoY.Value,
			COMEXNetLong:        comexNetLong,
			GoldSilverRatioZ:    gsRatioZ,
		}
	}

	// Wire ETF analyzer with metadata for Taiwan ETF universe.
	ea := portfolio.NewETFAnalyzer()
	ea.LoadMetadata(map[string]portfolio.ETFMetadata{
		// Existing
		"0050.TW":  {Name: "元大台灣50", NAV: 195.50, ExpenseRatio: 0.0032, Benchmark: "TW50"},
		"0056.TW":  {Name: "元大高股息", NAV: 42.80, ExpenseRatio: 0.0043, Benchmark: "TWHDividend"},
		"00878.TW": {Name: "國泰永續高股息", NAV: 25.30, ExpenseRatio: 0.0045, Benchmark: "MSCITWESG"},
		// New — broad market
		"006208.TW": {Name: "富邦台50", NAV: 0, ExpenseRatio: 0.0032, Benchmark: "TW50"},
		"00692.TW":  {Name: "富邦公司治理", NAV: 0, ExpenseRatio: 0.0035, Benchmark: "TWCG"},
		// New — defensive
		"00713.TW": {Name: "元大高股息低波動", NAV: 0, ExpenseRatio: 0.0035, Benchmark: "TWHDivLowVol"},
		// New — sector
		"00881.TW": {Name: "國泰台灣5G+", NAV: 0, ExpenseRatio: 0.0045, Benchmark: "TW5G"},
		"00891.TW": {Name: "中信關鍵半導體", NAV: 0, ExpenseRatio: 0.0045, Benchmark: "TWSemi"},
		// New — dividend
		"00919.TW": {Name: "群益台灣精選高息", NAV: 0, ExpenseRatio: 0.0040, Benchmark: "TWHDivSelect"},
		"00929.TW": {Name: "復華台灣科技優息", NAV: 0, ExpenseRatio: 0.0040, Benchmark: "TWTechDiv"},
		"00940.TW": {Name: "元大台灣價值高息", NAV: 0, ExpenseRatio: 0.0043, Benchmark: "TWValDiv"},
	})
	ea.WithHistoricalPrices(hp)

	fe := portfolio.NewFactorEngine().
		WithParameters(runtimeParams).
		WithHistoricalPrices(hp).
		WithFundamentalProvider(fp).
		WithPreciousMetalsProvider(pmProvider).
		WithETFAnalyzer(ea)

	// Wire industry cycle provider from CycleTracker with default positions.
	cycleTracker := industry.NewCycleTracker()
	fe.WithIndustryCycleProvider(func(symbol string) *domain.IndustryCycleFactorScore {
		industryID := symbolToIndustryID(symbol)
		if industryID == "" {
			return nil
		}
		pos, ok := cycleTracker.GetPosition(industryID)
		if !ok {
			return nil
		}
		return &domain.IndustryCycleFactorScore{
			Score:      pos.GetPhaseScore(),
			Phase:      string(pos.BusinessCycle),
			PhaseScore: pos.GetPhaseScore(),
			Confidence: pos.Confidence,
			IndustryID: industryID,
		}
	})

	linkageAnalyzer := industry.NewLinkageAnalyzer()
	fe.WithLinkageProvider(func(symbol string) *domain.LinkageFactorScore {
		industryID := symbolToIndustryID(symbol)
		if industryID == "" {
			return nil
		}
		score := linkageAnalyzer.CalculateLinkageScore(industryID)
		return &domain.LinkageFactorScore{
			Score:              score.SystemicImportance * score.AvgCorrelation,
			SystemicImportance: score.SystemicImportance,
			ShockPropagation:   score.ShockPropagationSpeed,
			AvgCorrelation:     score.AvgCorrelation,
			IndustryID:         industryID,
		}
	})

	return fe, hp, fp
}

// buildPortfolioManager constructs the portfolio management subsystem.
// Darwinian weight/history paths are anchored to ledgerDir (the injected
// state dir, ATLAS_LEDGER_DIR) instead of CWD-relative literals so the
// writer and the monitoring readers always resolve the same files.
func buildPortfolioManager(runtimeParams *portfolio.RuntimeParameters, registry domain.AgentRegistry, eventBus *eventbus.ChannelEventBus, factorEngine *portfolio.FactorEngine, ledgerDir string) PortfolioManager {
	darwinian := portfolio.NewDarwinianWeightManager(filepath.Join(ledgerDir, "darwinian_weights.json")).
		WithHistoryPath(filepath.Join(ledgerDir, "darwinian_history.jsonl")).
		WithParameters(runtimeParams)
	_ = darwinian.Load()
	darwinian.InitializeFromRegistry(registry)
	_ = darwinian.Save()
	darwinian.WithEventBus(eventBus)

	factorWeightEngine := portfolio.NewFactorWeightEngine()

	// Wire narrative factor provider from active events.
	factorEngine.WithNarrativeProvider(func(symbol string) *domain.NarrativeFactorScore {
		events := factorWeightEngine.GetActiveEvents()
		if len(events) == 0 {
			return nil
		}
		var totalConf, totalHit float64
		themes := make([]string, 0, len(events))
		eventIDs := make([]string, 0, len(events))
		for _, ev := range events {
			totalConf += ev.Confidence
			totalHit += ev.HitRate
			themes = append(themes, ev.Theme)
			eventIDs = append(eventIDs, ev.ID)
		}
		n := float64(len(events))
		score := n * (totalConf / n) * (totalHit / n)
		if score > 1.0 {
			score = 1.0
		}
		return &domain.NarrativeFactorScore{
			Score:      score,
			Theme:      themes[0],
			HitRate:    totalHit / n,
			Confidence: totalConf / n,
			EventIDs:   eventIDs,
		}
	})

	return PortfolioManager{
		darwinian:          darwinian,
		alphaDiscovery:     NewAlphaDiscoveryEngine(factorEngine),
		factorWeightEngine: factorWeightEngine,
	}
}

// buildStrategyLayer constructs the strategy subsystem.
func buildStrategyLayer(thresholdEngine *sim.DynamicThresholdEngine) StrategyLayer {
	strategyRegistry := strategy.NewRegistryWithDefaults()
	comparisonEngine := strategy.NewComparisonEngine(20, nil)
	strategySelector := strategy.NewSelector(strategyRegistry, comparisonEngine)
	strategyAllocator := strategy.NewStrategyAllocator(strategyRegistry)

	return StrategyLayer{
		strategyRegistry:  strategyRegistry,
		strategySelector:  strategySelector,
		comparisonEngine:  comparisonEngine,
		thresholdEngine:   thresholdEngine,
		strategyAllocator: strategyAllocator,
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

// computeGoldSilverRatioZ returns the z-score of the gold/silver ratio.
// Phase 1 default: returns 0 (no signal) since silver price is not yet tracked
// in MacroDataSnapshot. A future phase will add Silver to the snapshot.
func computeGoldSilverRatioZ(snap *marketdata.MacroDataSnapshot) float64 {
	_ = snap
	return 0
}

// buildPluginRegistry constructs the plugin registry with screener and factor engine.
// If loader is non-nil, it is passed to NewPluginRegistry; otherwise StaticLoader{} is used.
func buildPluginRegistry(factorEngine *portfolio.FactorEngine, fp *portfolio.FundamentalProvider, loader ExecutorLoader) *PluginRegistry {
	screenerEngine := screener.NewEngine(factorEngine, fp)
	reg := NewPluginRegistry()
	if loader != nil {
		reg = NewPluginRegistry(loader)
	}
	// Register all agent executors that also implement PositionEvaluator.
	// This wires position rotation into the recommendation pipeline. Executors
	// that do not implement PositionEvaluator are silently skipped.
	// Uses reg.agentExecutors (already loaded by NewPluginRegistry) so it
	// works with both the default StaticLoader and custom loaders.
	for _, exec := range reg.agentExecutors {
		if pe, ok := exec.(PositionEvaluator); ok {
			reg.RegisterPositionEvaluators(pe)
		}
	}
	return reg.WithScreener(screenerEngine).WithFactorEngine(factorEngine)
}

func loadRuntimeParamsOrDefault(parametersConfigPath string) *portfolio.RuntimeParameters {
	paramsCfg, err := config.LoadParametersConfig(parametersConfigPath)
	if err != nil || paramsCfg == nil {
		paramsCfg = config.DefaultParametersConfig()
	}
	return portfolio.ToRuntimeParameters(paramsCfg)
}

// symbolToIndustryID maps a stock symbol to an industry ID using Taiwan market conventions.
func symbolToIndustryID(symbol string) string {
	switch symbol {
	case "2330.TW", "2454.TW", "2303.TW":
		return "semiconductor"
	case "2317.TW", "2382.TW":
		return "electronics"
	case "2881.TW", "2882.TW", "2891.TW":
		return "financials"
	case "2603.TW", "2609.TW", "2615.TW":
		return "shipping"
	default:
		return ""
	}
}
