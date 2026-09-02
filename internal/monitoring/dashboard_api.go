package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/buildinfo"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/liveness"
	llmcapabilities "github.com/kaecer68/atlas-go/internal/llm/capabilities"
	"github.com/kaecer68/atlas-go/internal/llm_annotator"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/methodology"
	apibacktest "github.com/kaecer68/atlas-go/internal/monitoring/api/backtest"
	apicircuitbreaker "github.com/kaecer68/atlas-go/internal/monitoring/api/circuitbreaker"
	apicontrol "github.com/kaecer68/atlas-go/internal/monitoring/api/control"
	apicrossmarket "github.com/kaecer68/atlas-go/internal/monitoring/api/crossmarket"
	apidashboard "github.com/kaecer68/atlas-go/internal/monitoring/api/dashboard"
	apidecision "github.com/kaecer68/atlas-go/internal/monitoring/api/decision"
	apievents "github.com/kaecer68/atlas-go/internal/monitoring/api/events"
	apiexperiment "github.com/kaecer68/atlas-go/internal/monitoring/api/experiment"
	apiindustry "github.com/kaecer68/atlas-go/internal/monitoring/api/industry"
	apilive "github.com/kaecer68/atlas-go/internal/monitoring/api/live"
	apimacro "github.com/kaecer68/atlas-go/internal/monitoring/api/macro"
	apimetrics "github.com/kaecer68/atlas-go/internal/monitoring/api/metrics"
	apinarrative "github.com/kaecer68/atlas-go/internal/monitoring/api/narrative"
	apiparameters "github.com/kaecer68/atlas-go/internal/monitoring/api/parameters"
	apiperformance "github.com/kaecer68/atlas-go/internal/monitoring/api/performance"
	apipipeline "github.com/kaecer68/atlas-go/internal/monitoring/api/pipeline"
	apirisk "github.com/kaecer68/atlas-go/internal/monitoring/api/risk"
	apishared "github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	apistrategies "github.com/kaecer68/atlas-go/internal/monitoring/api/strategies"
	apisystem "github.com/kaecer68/atlas-go/internal/monitoring/api/system"
	apitaskexec "github.com/kaecer68/atlas-go/internal/monitoring/api/taskexec"
	apitax "github.com/kaecer68/atlas-go/internal/monitoring/api/tax"
	"github.com/kaecer68/atlas-go/internal/monitoring/metrics"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/orchestrator/composition"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/repository"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
	"github.com/kaecer68/atlas-go/internal/taskexec"
)

// FetchMeta carries Gateway-side quality metadata that must be surfaced to
// the L2 adapter so channel failures (CB-open stale, fallback, transient
// errors) propagate up to the 4-layer data-visibility safeguard. Without
// this, gateway.Fetch returns stale bytes with nil error on the CB-open
// path (gateway.go:107), and the L2 adapter silently treats them as fresh.
type FetchMeta struct {
	Stale     bool
	Fallback  bool
	LastError string
}

// DataFetcher is a Gateway-compatible data fetch function injected via SetGateway.
// It breaks the import cycle between monitoring and apigateway packages.
type DataFetcher func(ctx context.Context, channelID string) ([]byte, FetchMeta, error)

// dataQualityAdapter converts the monitoring-layer checker to the
// service-layer interface consumed by the metrics handlers.
type dataQualityAdapter struct {
	checker *DataQualityChecker
}

func (d dataQualityAdapter) RunAll(ctx context.Context) *service.DataQualityReport {
	rep := d.checker.RunAll(ctx)
	if rep == nil {
		return nil
	}
	out := &service.DataQualityReport{
		Overall:     service.CheckStatus(rep.Overall),
		Score:       rep.Score,
		GeneratedAt: rep.GeneratedAt,
		Checks:      make([]service.DataQualityCheck, 0, len(rep.Checks)),
	}
	for _, ch := range rep.Checks {
		out.Checks = append(out.Checks, service.DataQualityCheck{
			Name:      ch.Name,
			Status:    service.CheckStatus(ch.Status),
			Message:   ch.Message,
			Details:   ch.Details,
			CheckedAt: ch.CheckedAt,
			Duration:  ch.Duration,
		})
	}
	return out
}

type DashboardAPI struct {
	workDir                    string
	ledgerDir                  string
	storeBackend               string
	sqlitePath                 string
	baselinePath               string
	apiAddr                    string // dashboard HTTP listen address for /health shape
	fubonProxyPort             int    // fubon proxy port for /health shape
	narrativeEngine            *narrative.NarrativeEngine
	macroIngestor              *narrative.MacroIngestor
	macroProvider              marketdata.MacroDataProvider
	lifecycleMgr               *narrative.EventLifecycleManager
	geoProvider                geopolitical.GeopoliticalRiskProvider
	taiwanGeoProvider          geopolitical.GeopoliticalRiskProvider
	taiwanStressCalc           *narrative.TaiwanStressCalculator
	reportGenerator            *narrative.ReportGenerator
	pool                       *pgxpool.Pool
	industryService            *service.IndustryService
	metricsCollector           *MetricsCollector
	metricsHistory             *MetricsHistory
	lastHistoryPush            atomic.Int64 // unix sec; throttles trend snapshot recording
	lastHistoryVal             atomic.Int64 // last recorded ScreeningRate * 1e6 (change gate)
	lastHistoryTotal           atomic.Int64 // last recorded ScreeningTotal (change gate)
	healthManager              *portfolio.AgentHealthManager
	dataQualityChecker         *DataQualityChecker
	janusEngine                *janus.Engine
	repo                       *repository.DualWriteRepository
	taskManager                *taskexec.Manager
	eventBus                   *eventbus.ChannelEventBus
	outcomeStore               *DualWriteOutcomeStoreAdapter
	storageReport              apimetrics.StorageReporter
	dataFetcher                DataFetcher
	riskGate                   *risk.RiskGate
	riskHandlers               *apirisk.Handlers
	latestDrawdown             *portfolio.DrawdownResult
	drawdownMu                 sync.RWMutex
	strategyTechniquesHandlers *apistrategies.Handlers
	historicalStore            ledger.HistoricalStore
	quoteStore                 ledger.QuoteStore
	quoteStoreMu               sync.RWMutex
	fugleAPIKey                string
	fugleAPIKeyMu              sync.RWMutex
	fugleClient                *marketdata.FugleClient

	// RegisteredChannelIDs, when set, is fed to the data-channels endpoint
	// so the admin page lists every registered channel rather than a
	// hand-maintained subset (manifest #G05).
	RegisteredChannelIDs []string
	strategiesAnnotator  llm_annotator.Annotator
	kimiClient           *llm_annotator.KimiClient // concrete handle for cost/health endpoints; nil if strategiesAnnotator is not a KimiClient
	calibrationTask      *narrative.CalibrationTask
	crisisModeSetter     func(active bool) // callback: VIX>=35 → optimizer crisis mode
	correlationSetter    func(rho float64) // callback: dynamic SPX-TWSE ρ → optimizer
	crossMarketSvc       *service.CrossMarketService
	narrativeHandlers    *apinarrative.Handlers

	// Task-liveness (cross-restart heartbeat, Phase 1): late-bound providers
	// for GET /api/dashboard/task-liveness. Set via SetTaskLivenessProvider /
	// SetSchedulerStatusProvider after the BackgroundTaskManager exists
	// (cmd/atlas/main.go wires them once taskMgr is ready).
	taskLivenessProvider apidashboard.TaskLivenessProvider
	schedulerProvider    apidashboard.SchedulerStatusProvider
}

// SetCompositionRoot wires the dashboard's shared WeightEngine into the
// composition root so that simulation paths (SA08+) can consume the same engine.
func (d *DashboardAPI) SetCompositionRoot(root *composition.Root) {
	if d.industryService == nil || root == nil {
		return
	}
	if d.industryService.WeightEngine != nil {
		root.WithWeightEngine(d.industryService.WeightEngine)
	}
}

// NewDashboardAPI creates a DashboardAPI backed by CompositeMacroProvider.
//
// Deprecated: production code should use NewDashboardAPIWithGateway with a
// monitoring.DataFetcher so that macro data flows through apigateway.Gateway
// (observability, caching, uniform channel health). This constructor is kept
// only as a fallback for tools that do not have a Gateway available.
func NewDashboardAPI(workDir, ledgerDir string, metricsCollector *MetricsCollector) *DashboardAPI {
	cfg := config.Load()
	var providers []marketdata.MacroDataProvider

	// Yahoo Finance-backed providers — only when enabled.
	// Legacy constructor: production uses NewDashboardAPIWithGateway() instead.
	// See 歷史 gateway-migration 紀錄（已移出公開 docs）。
	if cfg.YahooEnabled {
		providers = append(providers, marketdata.NewYahooFinanceMacroProvider())
		providers = append(providers, marketdata.NewSOXIndexProvider())
		providers = append(providers, marketdata.NewDRAMSpotPriceProvider())
		providers = append(providers, marketdata.NewSPXIndexProvider())
		providers = append(providers, marketdata.NewNDXIndexProvider())
		providers = append(providers, marketdata.NewDJIIndexProvider())
		providers = append(providers, marketdata.NewTSMADRProvider())
		providers = append(providers, marketdata.NewNVDAProvider())
		providers = append(providers, marketdata.NewAAPLProvider())
		providers = append(providers, marketdata.NewMSFTProvider())
		providers = append(providers, marketdata.NewTAIEXIndexProvider())
		providers = append(providers, marketdata.NewTaiwanVolatilityProvider())
	}

	providers = append(providers, marketdata.NewBDIProvider())
	// ExchangeRate-API provides TWD (not in ECB/Frankfurter). DO NOT reorder the
	// next three providers without reading
	// ~/workspace/atlas-notes/05-decisions/2026-07-13-usd-twd-routing-recurring-bug-root-cause.md
	// — this list has been regressed 3+ times by independent fixes.
	//
	// Ordering rationale (last-write-wins mergeSnapshot):
	//   1. ExchangeRate: current value for USD/TWD (free tier, no historical ChangePct)
	//   2. Frankfurter:  current value + daily ChangePct for JPY/EUR (ECB dataset)
	//   3. Yahoo:        MUST be LAST. Yahoo uses range=1mo to compute reliable daily
	//                   ChangePct for ALL tickers incl. forex. Placing Yahoo last ensures
	//                   its (value, ChangePct) wins, and its ChangePct is non-zero.
	//                   Without this, ExchangeRate's ChangePct=0 overwrites Yahoo's.
	providers = append(providers, marketdata.NewExchangeRateProvider())
	providers = append(providers, marketdata.NewFrankfurterFXProvider())
	providers = append(providers, marketdata.NewYahooFinanceMacroProvider())
	providers = append(providers, marketdata.NewTWSECapitalFlowProvider(filepath.Join(workDir, constants.StateCapitalFlow)))
	providers = append(providers, marketdata.NewTWSEMarginBalanceProvider(filepath.Join(workDir, "data/state/margin")))
	providers = append(providers, marketdata.NewExportStatisticsProvider(filepath.Join(workDir, constants.StateExport)))
	// Sector data from local cache (graceful degradation if file missing).
	providers = append(providers, marketdata.NewSectorDataProvider(filepath.Join(workDir, "data/state/sector_data")))
	// TSMC Revenue from FinMind (overwrites cached sector data when available).
	if cfg.FinMindAPIKey != "" {
		providers = append(providers, marketdata.NewTSMCRevenueProvider(cfg.FinMindAPIKey))
	}
	provider := marketdata.NewCompositeMacroProvider(providers...)
	geoProvider := geopolitical.NewCompositeGeopoliticalProvider(
		geopolitical.NewRSSGeopoliticalProvider(),
		geopolitical.NewGDELTGeopoliticalProvider(),
	)
	taiwanGeoProvider := geopolitical.NewCompositeTaiwanGeopoliticalProvider(
		geopolitical.NewTaiwanRSSGeopoliticalProvider(),
	)
	if metricsCollector == nil {
		metricsCollector = NewMetricsCollector()
	}
	lifecycle := narrative.NewEventLifecycleManager()
	ingestor := narrative.NewMacroIngestor(provider, filepath.Join(workDir, constants.StateMacro))
	ingestor.SetLifecycleManager(lifecycle)

	narrativeEng := narrative.NewNarrativeEngine()
	return &DashboardAPI{
		workDir:            workDir,
		ledgerDir:          ledgerDir,
		storeBackend:       os.Getenv("ATLAS_STORE_BACKEND"),
		sqlitePath:         os.Getenv("ATLAS_SQLITE_PATH"),
		baselinePath:       filepath.Join(workDir, constants.StateBaselinePolicy+".json"),
		narrativeEngine:    narrativeEng,
		macroIngestor:      ingestor,
		macroProvider:      provider,
		lifecycleMgr:       lifecycle,
		geoProvider:        geoProvider,
		taiwanGeoProvider:  taiwanGeoProvider,
		taiwanStressCalc:   narrative.NewTaiwanStressCalculator(geoProvider, workDir),
		reportGenerator:    narrative.NewReportGenerator(),
		industryService:    newWiredIndustryService(narrativeEng, provider, workDir),
		metricsCollector:   metricsCollector,
		metricsHistory:     NewMetricsHistory(1000),
		healthManager:      portfolio.NewAgentHealthManager(),
		dataQualityChecker: NewDataQualityChecker(workDir, ledgerDir),
	}
}

// NewDashboardAPIWithGateway creates a DashboardAPI with Gateway-backed data providers.
// Unlike the legacy constructor, this skips direct provider creation and uses
// the Gateway via DataFetcher from the start, complying with the Constitution.
func NewDashboardAPIWithGateway(workDir, ledgerDir string, metricsCollector *MetricsCollector, fetcher DataFetcher) *DashboardAPI {
	if metricsCollector == nil {
		metricsCollector = NewMetricsCollector()
	}

	macroProvider := NewMacroDataGatewayAdapter(fetcher)
	geoProvider := NewGeopoliticalGatewayAdapter(fetcher)
	taiwanGeoProvider := NewTaiwanGeopoliticalGatewayAdapter(fetcher)

	lifecycle := narrative.NewEventLifecycleManager()
	ingestor := narrative.NewMacroIngestor(macroProvider, filepath.Join(workDir, constants.StateMacro))
	ingestor.SetLifecycleManager(lifecycle)

	narrativeEng := narrative.NewNarrativeEngine()
	return &DashboardAPI{
		workDir:            workDir,
		ledgerDir:          ledgerDir,
		storeBackend:       os.Getenv("ATLAS_STORE_BACKEND"),
		sqlitePath:         os.Getenv("ATLAS_SQLITE_PATH"),
		baselinePath:       filepath.Join(workDir, constants.StateBaselinePolicy+".json"),
		narrativeEngine:    narrativeEng,
		macroIngestor:      ingestor,
		macroProvider:      macroProvider,
		lifecycleMgr:       lifecycle,
		geoProvider:        geoProvider,
		taiwanGeoProvider:  taiwanGeoProvider,
		taiwanStressCalc:   narrative.NewTaiwanStressCalculator(geoProvider, workDir),
		reportGenerator:    narrative.NewReportGenerator(),
		industryService:    newWiredIndustryService(narrativeEng, macroProvider, workDir),
		metricsCollector:   metricsCollector,
		metricsHistory:     NewMetricsHistory(1000),
		healthManager:      portfolio.NewAgentHealthManager(),
		dataQualityChecker: NewDataQualityChecker(workDir, ledgerDir),
		dataFetcher:        fetcher,
	}
}

func newWiredIndustryService(narrativeEngine *narrative.NarrativeEngine, macroProvider marketdata.MacroDataProvider, workDir string) *service.IndustryService {
	seasonalEngine := industry.NewSeasonalEngine()
	cycleTracker := industry.NewCycleTracker()
	// B01：載入持久化的 cycle positions/history（restart 後不退回 seeds）。
	// 檔案不存在/損壞時為 no-op。
	if err := cycleTracker.LoadFromFile(filepath.Join(workDir, "data/state", "cycle_tracker.json")); err != nil {
		logging.Warn("industry_service", "cycle_state_load_failed", "err", err.Error())
	}
	linkageAnalyzer := industry.NewLinkageAnalyzer()

	// Wire narrative provider
	bridge := narrative.NewSeasonalBridge(narrativeEngine)
	bridge.SetActiveEvents(narrativeEngine.DetectEvents(narrative.MarketNarrativeData{}))
	seasonalEngine.SetNarrativeProvider(bridge)
	cycleTracker.SetNarrativeProvider(func() float64 {
		events := narrativeEngine.DetectEvents(narrative.MarketNarrativeData{})
		hitRate := 0.0
		for _, e := range events {
			if e.HitRate > hitRate {
				hitRate = e.HitRate
			}
		}
		return hitRate
	})
	cycleTracker.SetNarrativeAdjuster(func(industryID string) industry.NarrativeAdjustment {
		events := narrativeEngine.DetectEvents(narrative.MarketNarrativeData{})
		// theme → {affected industries → baseRevenueBias}
		type themeImpact struct {
			industryBias map[string]float64
		}
		themeMap := map[string]themeImpact{
			"AI_capex_surge":          {map[string]float64{"semiconductor": 0.08, "ai_supply_chain": 0.08, "electronics": 0.05}},
			"US_rates_up":             {map[string]float64{"financials": -0.04}},
			"JPY_carry_unwind":        {map[string]float64{"financials": -0.03, "semiconductor": -0.05, "electronics": -0.03}},
			"geopolitical_risk_spike": {map[string]float64{"shipping": -0.05, "energy": -0.05, "industrial": -0.03}},
			"oil_price_shock":         {map[string]float64{"shipping": -0.04, "energy": -0.04, "industrial": -0.03}},
			"semiconductor_downturn":  {map[string]float64{"semiconductor": -0.08, "ai_supply_chain": -0.06, "electronics": -0.06}},
		}
		totalBias := 0.0
		maxConf := 0.0
		activeTheme := ""
		for _, e := range events {
			ti, ok := themeMap[e.Theme]
			if !ok {
				continue
			}
			bias, ok := ti.industryBias[industryID]
			if !ok {
				continue
			}
			weighted := bias * e.Confidence * e.HitRate
			totalBias += weighted
			if e.Confidence*e.HitRate > maxConf {
				maxConf = e.Confidence * e.HitRate
				activeTheme = e.Theme
			}
		}
		if maxConf == 0 {
			return industry.NarrativeAdjustment{}
		}
		return industry.NarrativeAdjustment{
			RevenueBias: totalBias,
			ProfitBias:  totalBias * 0.8,
			Confidence:  maxConf,
			ActiveTheme: activeTheme,
		}
	})

	// Wire supply chain graph into seasonal engine
	seasonalEngine.SetLinkageGraph(linkageAnalyzer.GetSupplyChainGraph())

	// Wire external validators into cycle tracker for multi-dimensional confidence
	cycleTracker.SetExternalValidators(seasonalEngine, linkageAnalyzer)

	// Create DynamicEnvModulator with real-time macro data (baseline uses neutral values)
	// The update will happen when macro ingestor fetches new data.
	baseline := marketdata.MacroDataSnapshot{
		Oil: marketdata.MacroDataPoint{Value: 75.0},  // Historical WTI average
		DXY: marketdata.MacroDataPoint{Value: 103.0}, // Historical DXY average
	}
	modulator := industry.NewDynamicEnvModulator(baseline, baseline)
	modulator.RecordSnapshot(baseline) // seed history for rolling baseline
	// Bootstrap DynamicEnvModulator asynchronously — don't block API startup.
	// DynamicEnvModulator methods are safe for concurrent use.
	if macroProvider != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			if snap, err := macroProvider.FetchSnapshot(ctx); err == nil {
				modulator.UpdateCurrent(snap)
				modulator.RecordSnapshot(snap)
				modulator.UpdateRollingBaseline() // compute rolling median baseline
			}
		}()
	}
	seasonalEngine.SetDynamicEnv(modulator)

	// Wire narrative provider into linkage analyzer for dynamic supply chain correlations
	linkageAnalyzer.SetNarrativeProvider(bridge)

	// Wire cycle tracker into linkage analyzer for regime-aware correlation adjustment
	// During recession, correlations rise (Ang & Chen 2002)
	linkageAnalyzer.SetCycleProvider(cycleTracker)

	siliconTracker := industry.NewSiliconCycleTracker()

	svc := service.NewIndustryService(
		industry.DefaultClassification(),
		seasonalEngine,
		cycleTracker,
		linkageAnalyzer,
		industry.NewRiskMonitor(),
		siliconTracker,
		// R7 (2026-08-24, k3 adjudication): TWSE rwd API deprecated 2026-06 —
		// the old TWSECalendarProvider returns nothing for any year, leaving
		// the live event calendar empty. Switch to the composite of the new
		// OpenAPI snapshot provider (current-year ex-dividend/shareholder
		// meetings) + MSCI static rebalance dates. Composite fails open:
		// only errors when ALL providers fail (CalendarEventProvider contract),
		// so a provider outage degrades to an empty calendar instead of
		// blocking the dashboard. Announced (future) events are inert —
		// DetectActiveEvents only fires when now ∈ [start, end].
		newWiredEventCalendar(marketdata.NewCompositeCalendarProvider(
			marketdata.NewTWSEOpenAPICalendarProvider(),
			marketdata.NewMSCIRebalanceCalendarProvider(),
		)),
		nil,                                // odmChannel (optional, not wired in default dashboard)
		nil,                                // dataAggregator (optional, not wired in default dashboard)
		config.Load().ParametersConfigPath, // paramsPath
	)

	// Wire the macro provider into the silicon cycle aggregator so that
	// scheduled silicon_cycle_update tasks can pull real TSMC/SOX data.
	svc.SetMacroProvider(macroProvider)

	// Bootstrap silicon tracker with the initial macro snapshot so the
	// cycle status card has non-zero indicators from the first request.
	// SiliconTracker.DetectPhase is safe for concurrent use.
	if macroProvider != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			if snap, err := macroProvider.FetchSnapshot(ctx); err == nil {
				indicators := industry.ExtractSiliconIndicators(snap)
				siliconTracker.DetectPhase(time.Now(), indicators)
			} else {
				logging.Warn("monitoring", "silicon_bootstrap_failed", "err", err)
			}
		}()
	}

	replayPath := config.Load().ReplayDataPath
	if replayPath != "" {
		sectorSymbolsPath := filepath.Join(config.Load().WorkDir, "configs", "sector_symbols.json")
		if returns, err := industry.LoadIndustryReturnsFromReplay(replayPath, sectorSymbolsPath); err == nil {
			svc.RebuildCorrelations(returns)
		} else {
			logging.Warn("monitoring", "correlation_rebuild_failed", "err", err)
		}
	}

	params := config.GetParametersConfig()
	calCfg := params.Industry.CycleCalibration.Value
	cal := industry.NewCycleCalibration(calCfg)
	svc.SetCycleCalibration(cal)

	narrativeFn := func(ctx context.Context, industryID string) (float64, float64, string, error) {
		// Use the real macro snapshot when available (single source of
		// truth for sector bias = models' FavoredSectors/AvoidedSectors).
		data := narrative.MarketNarrativeData{}
		if macroProvider != nil {
			if snap, err := macroProvider.FetchSnapshot(ctx); err == nil {
				data = narrative.MarketNarrativeDataFromSnapshot(snap)
			}
		}
		events := narrativeEngine.DetectEvents(data)
		// Engine consumes narrative as a multiplicative factor (1.0 =
		// neutral; safeGetNarrative clamps ≤0 to 1.0), so shift the
		// signed bias into a multiplier ≥ 1.
		multiplier := 1.0 + narrativeEngine.SectorBias(industryID, events)
		if multiplier <= 0 {
			multiplier = 1.0
		}
		maxConf := 0.0
		activeTheme := ""
		for _, e := range events {
			if e.Confidence*e.HitRate > maxConf {
				maxConf = e.Confidence * e.HitRate
				activeTheme = e.Theme
			}
		}
		return multiplier, maxConf, activeTheme, nil
	}
	macroFn := func(_ context.Context, industryID, _, _ string) (float64, error) {
		return modulator.SeasonalModulation(industryID) - 1.0, nil
	}
	factorFn := func(_ context.Context, _ string) (float64, error) {
		return 0.0, nil
	}
	svc.WeightEngine = sectorallocation.NewDefaultEngine(
		params.SectorAllocation,
		sectorallocation.NewCycleAdapter(cycleTracker),
		sectorallocation.NewSeasonalAdapter(seasonalEngine),
		sectorallocation.NewLinkageAdapter(linkageAnalyzer, nil),
		sectorallocation.NewNarrativeAdapter(narrativeFn),
		sectorallocation.MacroProviderFunc(macroFn),
		sectorallocation.FactorProviderFunc(factorFn),
		params.Darwinian.WeightMin.Value,
		params.Darwinian.WeightMax.Value,
	)

	svc.WithSnapshotReader(sectorallocation.NewFileClosureStore(filepath.Join(workDir, "data", "sector", "allocation")))

	return svc
}

func newWiredEventCalendar(provider marketdata.CalendarEventProvider) *industry.EventCalendar {
	// Stage 1 PR#1 改用 industry 公開 factory，確保 RefreshEvents 一定會被呼叫。
	// provider 非 nil 時另外排 background refresh（避免 startup block）。
	ec := industry.NewEventCalendarWithProvider(provider)
	if provider != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			defer cancel()
			ec.UpdateFromProvider(ctx, provider)
		}()
	}
	return ec
}

// SetHealthAddrs records the API listen address and Fubon proxy port so that
// the /health endpoint can report accurate per-port status.
func (a *DashboardAPI) SetHealthAddrs(apiAddr string, fubonProxyPort int) {
	a.apiAddr = apiAddr
	a.fubonProxyPort = fubonProxyPort
}

func (a *DashboardAPI) SetEventBus(eventBus *eventbus.ChannelEventBus) {
	a.eventBus = eventBus
	if a.macroIngestor != nil {
		a.macroIngestor.SetEventBus(eventBus)
	}
	// Subscribe to narrative events to keep DynamicEnvModulator current.
	if eventBus != nil {
		eventBus.Subscribe(eventbus.EventNarrative, func(ctx context.Context, event eventbus.BusEvent) error {
			snap, err := a.macroProvider.FetchSnapshot(ctx)
			if err != nil {
				logging.Warn("dashboardapi", "dynamic_env_update_failed", "err", err)
				return nil
			}
			a.industryService.UpdateDynamicEnv(snap)
			return nil
		})
	}
}

func (a *DashboardAPI) GetEventBus() *eventbus.ChannelEventBus {
	return a.eventBus
}

func (a *DashboardAPI) GetMacroIngestor() *narrative.MacroIngestor {
	return a.macroIngestor
}

// GetEventLifecycleManager returns the narrative event lifecycle manager.
func (a *DashboardAPI) GetEventLifecycleManager() *narrative.EventLifecycleManager {
	return a.lifecycleMgr
}

// IngestAndUpdateMacro performs macro ingestion and updates the narrative engine state.
// This ensures GetCurrentStressIndex() has valid data instead of zero values.
//
// D08 fix: on the error-fallback path (when macroIngestor.Ingest returns
// error but a disk snapshot is available), we previously only fed
// narrativeEngine with the snapshot and skipped ledger persistence.
// That left stress_index_history.captured_at frozen at the first
// successful tick's timestamp — subsequent ticks that happened to hit
// transient upstream errors never advanced the row. Both branches now
// route through applyMacroUpdate so the ledger row refreshes on every
// tick, success or fallback.
func (a *DashboardAPI) IngestAndUpdateMacro(ctx context.Context) ([]narrative.NarrativeEvent, marketdata.MacroDataSnapshot, error) {
	events, snap, err := a.macroIngestor.Ingest(ctx)
	geoScore := a.resolveGeoScore(ctx)
	if err != nil {
		// On ingest failure, also feed silicon tracker from the on-disk snapshot
		// (regression: otherwise the 矽循環時鐘 panel renders all zeros).
		if diskSnap, ok := a.GetLatestMacroSnapshot(); ok {
			a.applyMacroUpdate(ctx, diskSnap, geoScore)
			if a.industryService != nil && a.industryService.SiliconTracker != nil {
				indicators := industry.ExtractSiliconIndicators(diskSnap)
				a.industryService.SiliconTracker.DetectPhase(time.Now(), indicators)
			}
		}
		return events, snap, err
	}
	a.applyMacroUpdate(ctx, snap, geoScore)
	// Also update the industry seasonal engine's dynamic environment (oil, DXY, BDI)
	// so that seasonal adjustments reflect real-time macro conditions.
	if a.industryService != nil && a.industryService.SeasonalEngine != nil {
		a.industryService.SeasonalEngine.UpdateDynamicEnv(snap)
	}

	// Update silicon cycle tracker with fresh TSMC revenue and SOX index data
	// so the cycle status card reflects the latest macro snapshot.
	if a.industryService != nil && a.industryService.SiliconTracker != nil {
		indicators := industry.ExtractSiliconIndicators(snap)
		a.industryService.SiliconTracker.DetectPhase(time.Now(), indicators)
	}
	return events, snap, err
}

// applyMacroUpdate routes the snapshot through narrativeEngine + both
// ledgers in lockstep; shared by success and error-fallback paths so
// stress_index_history.captured_at advances on every tick.
func (a *DashboardAPI) applyMacroUpdate(ctx context.Context, snap marketdata.MacroDataSnapshot, geoScore geopolitical.GeopoliticalRiskScore) {
	if a.narrativeEngine == nil {
		return
	}
	a.narrativeEngine.UpdateMacro(snap, geoScore)
	a.persistStressIndex(ctx)
	a.persistGeopolitical(ctx, geoScore)
	a.persistPeriodHistory(ctx, snap, geoScore)
	a.persistRegimeHistory(ctx)
}

// persistStressIndex writes the current TaiwanStressIndex to the historical
// ledger so that /api/narrative/stress-index/history can serve persistent data
// instead of the process-local ring buffer. It is called after every successful
// macro ingestion; the SQLite ON CONFLICT(date) upsert makes repeated calls
// idempotent for the same trading day.
func (a *DashboardAPI) persistStressIndex(ctx context.Context) {
	if a.historicalStore == nil || a.narrativeEngine == nil {
		return
	}
	idx := a.narrativeEngine.GetCurrentStressIndex()
	if idx.Timestamp == 0 {
		return
	}
	row := ledger.StressRow{
		Date:        time.Unix(idx.Timestamp, 0).UTC().Format("2006-01-02"),
		Score:       idx.Score,
		Regime:      idx.Regime,
		Components:  stressComponentsToMap(idx.Components),
		Source:      "macro_ingest",
		CapturedAt:  time.Unix(idx.Timestamp, 0).UTC(),
		IsSynthetic: 0,
	}
	if err := a.historicalStore.UpsertStress(ctx, row); err != nil {
		logging.Warn("dashboard_api", "persist_stress_failed",
			logging.FStr("date", row.Date),
			logging.Err(err))
	}
}

// resolveGeoScore returns the best available geopolitical risk score for the
// current macro tick. It prefers a live fetch from the configured geo provider,
// falls back to the most recent non-zero row in the historical ledger, and
// finally to the on-disk geopolitical store. This prevents a transient live
// fetch failure from producing a zero stress component while a stale-but-valid
// historical score is available.
func (a *DashboardAPI) resolveGeoScore(ctx context.Context) geopolitical.GeopoliticalRiskScore {
	if a.geoProvider != nil {
		geoCtx, geoCancel := context.WithTimeout(ctx, 15*time.Second)
		defer geoCancel()
		if score, err := a.geoProvider.FetchScore(geoCtx); err == nil && score.Intensity != 0 && !score.Timestamp.IsZero() {
			return score
		}
	}

	if a.historicalStore != nil {
		rows, err := a.historicalStore.LoadGeopoliticalHistoryAll(ctx, 1)
		if err == nil && len(rows) > 0 && rows[0].Intensity != 0 && !rows[0].CapturedAt.IsZero() {
			return geopolitical.GeopoliticalRiskScore{
				Intensity: rows[0].Intensity,
				Timestamp: rows[0].CapturedAt,
				Sources:   rows[0].Sources,
			}
		}
	}

	store := geopolitical.NewGeopoliticalStore(filepath.Join(a.workDir, constants.StateGeopolitical))
	if fallback, err := store.Load(); err == nil && fallback.Intensity != 0 && !fallback.Timestamp.IsZero() {
		return fallback
	}
	return geopolitical.GeopoliticalRiskScore{}
}

// geoIntensityChange5D computes the 5-calendar-day change of geopolitical
// intensity vs the given date from the historical ledger. Returns 0 when
// history is unavailable (honest degradation — the 地緣升溫 condition in the
// detector then falls back to intensity-only, since Change5D==0 passes >= 0).
func (a *DashboardAPI) geoIntensityChange5D(ctx context.Context, date string, current float64) float64 {
	if a.historicalStore == nil || current == 0 {
		return 0
	}
	rows, err := a.historicalStore.LoadGeopoliticalHistoryAll(ctx, 30)
	if err != nil || len(rows) == 0 {
		return 0
	}
	target, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0
	}
	base := target.AddDate(0, 0, -5).Format("2006-01-02")
	for _, row := range rows {
		if row.Date == base {
			return current - row.Intensity
		}
	}
	return 0
}

// toGeoEventRows converts provider-level geo events to ledger rows (G5-4).
func toGeoEventRows(evs []geopolitical.GeoEvent) []ledger.GeoEventRow {
	if len(evs) == 0 {
		return nil
	}
	out := make([]ledger.GeoEventRow, 0, len(evs))
	for _, ev := range evs {
		out = append(out, ledger.GeoEventRow{Title: ev.Title, Keyword: ev.Keyword, Source: ev.Source})
	}
	return out
}

// persistGeopolitical writes the latest geopolitical risk score to the ledger
// so historical reconstruction can read real geo values instead of defaulting
// to zero. The SQLite ON CONFLICT(date) upsert makes repeated calls idempotent.
// It also mirrors the score to the on-disk geopolitical store so that the
// on-demand /api/taiwan/stress-index calculator and the macro-ingest path use
// the same fallback source.
func (a *DashboardAPI) persistGeopolitical(ctx context.Context, geo geopolitical.GeopoliticalRiskScore) {
	if a.historicalStore == nil || geo.Timestamp.IsZero() {
		return
	}
	row := ledger.GeopoliticalRow{
		Date:        geo.Timestamp.UTC().Format("2006-01-02"),
		Intensity:   geo.Intensity,
		Sources:     geo.Sources,
		Events:      toGeoEventRows(geo.Events),
		Source:      "macro_ingest",
		CapturedAt:  geo.Timestamp.UTC(),
		IsSynthetic: 0,
	}
	if err := a.historicalStore.UpsertGeopolitical(ctx, row); err != nil {
		logging.Warn("dashboard_api", "persist_geopolitical_failed",
			logging.FStr("date", row.Date),
			logging.Err(err))
		return
	}

	store := geopolitical.NewGeopoliticalStore(filepath.Join(a.workDir, constants.StateGeopolitical))
	if err := store.Save(geo); err != nil {
		logging.Warn("dashboard_api", "mirror_geopolitical_store_failed",
			logging.FStr("date", row.Date),
			logging.Err(err))
	}
}

// persistRegimeHistory derives a canonical regime row from the current stress
// index and upserts it into regime_history. This closes the live-pipeline gap
// where stress and geo were persisted but regime_history only contained stage-4
// synthetic backfill rows. The mapping is the same bidirectional mapping used
// by /api/narrative/regime-mapping and is intentionally approximate.
func (a *DashboardAPI) persistRegimeHistory(ctx context.Context) {
	if a.historicalStore == nil || a.narrativeEngine == nil {
		return
	}
	idx := a.narrativeEngine.GetCurrentStressIndex()
	if idx.Timestamp == 0 {
		return
	}
	date := time.Unix(idx.Timestamp, 0).UTC().Format("2006-01-02")
	row := ledger.RegimeRow{
		Date:            date,
		Regime:          narrative.NormalizeRegime(idx.Regime),
		Source:          "macro_ingest",
		RecordedAt:      time.Unix(idx.Timestamp, 0).UTC(),
		CapturedAt:      time.Unix(idx.Timestamp, 0).UTC(),
		IsSynthetic:     0,
		SourceSessionID: "macro_ingest:" + date,
	}
	if err := a.historicalStore.UpsertRegime(ctx, row); err != nil {
		logging.Warn("dashboard_api", "persist_regime_history_failed",
			logging.FStr("date", row.Date),
			logging.Err(err))
	}
}

// persistPeriodHistory derives the current seven-period classification from
// the macro snapshot via PeriodDetector and upserts it into period_history.
func (a *DashboardAPI) persistPeriodHistory(ctx context.Context, snap marketdata.MacroDataSnapshot, geoScore geopolitical.GeopoliticalRiskScore) {
	if a.historicalStore == nil {
		return
	}
	date := time.Now().UTC().Format("2006-01-02")
	if snap.RecordedAt > 0 {
		date = time.Unix(snap.RecordedAt, 0).UTC().Format("2006-01-02")
	}
	ind := SnapshotToPeriodIndicators(snap)
	// G5: 台海危機訊號進時期判別（憲章 §3 黑天鵝/轉折下壓地緣條件）。
	// geoScore 由 applyMacroUpdate 的 resolveGeoScore() 提供（live → history → file）。
	ind.GeoIntensity = geoScore.Intensity
	// 地緣 5 日趨勢（憲章 v1.1：轉折下壓「地緣緊張升溫」需 5 日非下降；
	// 無歷史資料回傳 0 → detector 退為僅以當日強度判定）。
	ind.GeoIntensityChange5D = a.geoIntensityChange5D(ctx, date, geoScore.Intensity)
	// A1（R8 接線）：國安基金護盤期間 → 黑天鵝條件 4「國安基金宣布進場護盤」。
	// 使用 snapshot 自身日期判定（static 表涵蓋 2000-2026，backfill 亦正確）。
	ind.NationalFundActive = marketdata.NewNationalStabilizationProvider().IsInterventionActive(date)
	// B5 Batch 1 & 2: enrich with computed fields from historical snapshots.
	// Errors are swallowed — if historical data is unavailable, indicators
	// stay at zero (honest degradation, detector guards handle it).
	if a.macroIngestor != nil {
		calc := portfolio.NewCalculator()
		_ = calc.EnrichFromDir(&ind, date, a.macroIngestor.SnapshotDir())

		// B5 Batch 2: margin history enrichment
		marginDir := filepath.Join(a.workDir, "data", "state", "margin")
		marginEntries, err := narrative.LoadMarginHistory(marginDir)
		if err == nil && len(marginEntries) > 0 {
			// Convert narrative.MarginHistoryEntry → portfolio.MarginEntry
			pmEntries := make([]portfolio.MarginEntry, len(marginEntries))
			for i, me := range marginEntries {
				pmEntries[i] = portfolio.MarginEntry{
					Date:          me.Date,
					MarginBalance: me.MarginBalance,
				}
			}
			calc.EnrichMargin(&ind, pmEntries)
		}

		// B5 Batch 3: sector rotation + public-bank consecutive buy
		sectorIndexDir := filepath.Join(a.workDir, "data", "state", "sector_index")
		govFlowDir := filepath.Join(a.workDir, "data", "state", "government_flow")
		calc.EnrichBatch3(&ind, date, sectorIndexDir, govFlowDir)
	}
	period := portfolio.NewPeriodDetectorWithDefaults().DetectPeriod(ind)
	now := time.Now().UTC()
	row := ledger.PeriodRow{
		Date:        date,
		Period:      string(period),
		RecordedAt:  now,
		CapturedAt:  now,
		IsSynthetic: 0,
		Source:      "macro_ingest",
	}
	if err := a.historicalStore.UpsertPeriod(ctx, row); err != nil {
		logging.Warn("dashboard_api", "persist_period_history_failed",
			logging.FStr("date", row.Date),
			logging.Err(err))
	}
}

func stressComponentsToMap(comps map[string]float64) map[string]any {
	if comps == nil {
		return nil
	}
	out := make(map[string]any, len(comps))
	for k, v := range comps {
		out[k] = v
	}
	return out
}

// CalibrateNarrative evaluates model performance against replay data and updates
// model weights and template hit rates. Returns the calibration report or error.
func (a *DashboardAPI) CalibrateNarrative(replayPath string) (*narrative.NarrativeCalibrationReport, error) {
	if a.narrativeEngine == nil {
		return nil, fmt.Errorf("narrative calibrate: no narrative engine")
	}
	return a.narrativeEngine.SelfCalibrate(replayPath)
}

// NarrativeEngine returns the underlying narrative engine for callers that
// need to invoke methods not yet exposed via the DashboardAPI surface
// (e.g. Stage 3 scheduling of template-hit-rate recalculation).
func (a *DashboardAPI) NarrativeEngine() *narrative.NarrativeEngine {
	return a.narrativeEngine
}

// GetLatestMacroSnapshot reads the macro ingestor's latest.json from disk.
// Returns (zero, false) when unavailable. Public accessor for consumers that
// need the raw snapshot without triggering a full ingestion (e.g. RealTimeAdapter).
func (a *DashboardAPI) GetLatestMacroSnapshot() (marketdata.MacroDataSnapshot, bool) {
	if a.macroIngestor == nil {
		return marketdata.MacroDataSnapshot{}, false
	}
	snapDir := filepath.Clean(a.macroIngestor.SnapshotDir())
	latestPath := filepath.Join(snapDir, "latest.json")
	data, err := os.ReadFile(latestPath)
	if err != nil {
		return marketdata.MacroDataSnapshot{}, false
	}
	var snap marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return marketdata.MacroDataSnapshot{}, false
	}
	return snap, true
}

// loadSnapshotIntoNarrativeEngine loads the latest snapshot from disk into the narrative engine.
// Used as fallback when live ingestion fails to ensure stress index has data.
func (a *DashboardAPI) SetContext(ctx context.Context) {
	if a.outcomeStore != nil && ctx != nil {
		a.outcomeStore.SetContext(ctx)
	}
}

// WithHistoricalStore injects the regime_history SQLite store into the
// dashboard API so /api/dashboard/regime-history reads true regime time
// series instead of simulation session metadata. Builder pattern preserves
// nil-default behavior (see CL-3 A03 docs/manifests/2026-07-20-cl3-regime-history.md).
func (a *DashboardAPI) WithHistoricalStore(hs ledger.HistoricalStore) *DashboardAPI {
	a.historicalStore = hs
	// E04: warm up stock quotes from Fugle for technical indicators.
	go a.warmupQuotes()
	return a
}

// SetQuoteStore injects the QuoteStore for stock quote warmup.
func (a *DashboardAPI) SetQuoteStore(qs ledger.QuoteStore) {
	a.quoteStoreMu.Lock()
	defer a.quoteStoreMu.Unlock()
	a.quoteStore = qs
}

// SetFugleClient injects the shared FugleClient so warmup and on-demand
// fetches use the singleton rate limiter instead of raw HTTP calls.
func (a *DashboardAPI) SetFugleClient(c *marketdata.FugleClient) {
	a.fugleClient = c
}

// SetFugleAPIKey injects the Fugle API key so warmup can fetch candles
// from the same config source as the rest of the application.
func (a *DashboardAPI) SetFugleAPIKey(key string) {
	a.fugleAPIKeyMu.Lock()
	defer a.fugleAPIKeyMu.Unlock()
	a.fugleAPIKey = key
	// Also use the shared singleton client so warmup shares one rate limiter
	// with the rest of the app. Fallback only for tests/direct callers that
	// never injected a client.
	if a.fugleClient == nil && key != "" {
		a.fugleClient = marketdata.GetSharedFugleClient(key)
	}
}

// warmupQuotes fetches historical daily bars from Fugle for all representative
// stocks in the 20-sector taxonomy and inserts them into the quotes table.
// Called once on startup via WithHistoricalStore to ensure technical indicators
// work immediately.
//
// Rate limit: Fugle free tier is ~30 req/min. We serialize requests with a
// 2-second delay between symbols to stay under the limit. Timeout is 300s
// for the full warmup (~96 symbols * ~2s).
func (a *DashboardAPI) warmupQuotes() {
	a.quoteStoreMu.RLock()
	qs := a.quoteStore
	a.quoteStoreMu.RUnlock()
	if qs == nil {
		logging.Warn("dashboard_api", "quote_warmup_skipped", "reason", "quote store nil")
		return
	}

	if a.fugleClient == nil {
		logging.Warn("dashboard_api", "quote_warmup_skipped", "reason", "FugleClient not configured")
		return
	}
	// Collect unique symbols from DefaultRepresentativeStocks and DefaultSymbols.
	// DefaultSymbols includes ETFs (0050, 0056, etc.) not covered by the
	// industry-sector representative list.
	symbolSet := make(map[string]struct{})
	for _, stocks := range industry.DefaultRepresentativeStocks() {
		for _, s := range stocks {
			symbolSet[s] = struct{}{}
		}
	}
	for _, s := range orchestrator.DefaultSymbols() {
		// DefaultSymbols uses ".TW" suffix; Fugle API uses bare symbols.
		symbolSet[strings.TrimSuffix(s, ".TW")] = struct{}{}
	}
	symbols := make([]string, 0, len(symbolSet))
	for s := range symbolSet {
		symbols = append(symbols, s)
	}

	from := "2026-01-01"
	to := time.Now().Format("2006-01-02")
	ctx := context.Background()
	totalBars := 0
	fetched := 0
	failed := 0

	for _, sym := range symbols {
		bars, err := a.fugleClient.GetHistoricalCandles(ctx, sym, from, to)
		if err != nil {
			failed++
			continue
		}
		if len(bars) == 0 {
			continue
		}

		if err := qs.RecordQuotes(bars); err != nil {
			logging.Warn("dashboard_api", "quote_warmup_insert_failed", "symbol", sym, logging.Err(err))
			failed++
			continue
		}
		totalBars += len(bars)
		fetched++
	}

	logging.Info(
		"dashboard_api", "quote_warmup_complete",
		"symbols_fetched", fetched,
		"symbols_failed", failed,
		"total_bars", totalBars,
	)
}

func (a *DashboardAPI) RegisterRoutes(mux *http.ServeMux) {
	var outcomeStore ledger.OutcomeStore
	if a.repo != nil {
		a.outcomeStore = NewDualWriteOutcomeStoreAdapter(a.repo)
		outcomeStore = a.outcomeStore
	} else {
		cfg := config.Config{
			LedgerDir:    a.ledgerDir,
			StoreBackend: a.storeBackend,
			SQLitePath:   a.sqlitePath,
		}
		var err error
		outcomeStore, err = ledger.NewOutcomeStore(cfg)
		if err != nil {
			logging.Error("dashboardapi", "create_outcome_store_failed", "err", err)
			outcomeStore = nil
		}
	}

	// Register SSE event stream endpoint.
	if a.eventBus != nil {
		sseHandler := apievents.NewSSEHandler(a.eventBus)
		mux.HandleFunc("/api/events/stream", sseHandler.ServeHTTP)
	}

	pipelineSvc := service.NewPipelineService(a.workDir, a.ledgerDir, outcomeStore).
		WithHistoricalStore(a.historicalStore).
		WithNarrativeProvider(func(eventIDs []string) *service.NarrativeContextData {
			if a.narrativeEngine == nil {
				return nil
			}
			events := a.narrativeEngine.DetectEvents(narrative.MarketNarrativeData{})
			var allowedIDs map[string]struct{}
			if len(eventIDs) > 0 {
				allowedIDs = make(map[string]struct{}, len(eventIDs))
				for _, id := range eventIDs {
					allowedIDs[id] = struct{}{}
				}
			}
			var activeThemes []string
			var primaryTheme string
			var primaryHitRate float64
			var directionHint string
			for _, event := range events {
				if event.Status != "active" && event.Status != "confirmed" {
					continue
				}
				if allowedIDs != nil {
					if _, ok := allowedIDs[event.ID]; !ok {
						continue
					}
				}
				activeThemes = append(activeThemes, event.Theme)
				if primaryTheme == "" {
					primaryTheme = event.Theme
					primaryHitRate = event.HitRate
					if event.Sentiment > 0.3 {
						directionHint = "positive"
					} else if event.Sentiment < -0.3 {
						directionHint = "negative"
					} else {
						directionHint = "neutral"
					}
				}
			}
			return &service.NarrativeContextData{
				ActiveThemes:   activeThemes,
				PrimaryTheme:   primaryTheme,
				PrimaryHitRate: primaryHitRate,
				DirectionHint:  directionHint,
			}
		}).
		WithCycleProvider(func(skill string) *service.IndustryContextData {
			if a.industryService == nil {
				return nil
			}
			tracker := a.industryService.CycleTracker
			if tracker == nil {
				return nil
			}
			pos, ok := tracker.GetPosition(skill)
			if !ok {
				return nil
			}
			var seasonalMultiplier float64
			if se := a.industryService.SeasonalEngine; se != nil {
				seasonalMultiplier = se.GetPatternAdjustment(skill, time.Now())
			}
			var systemicImportance float64
			if la := a.industryService.LinkageAnalyzer; la != nil {
				if score := la.CalculateLinkageScore(skill); score != nil {
					systemicImportance = score.SystemicImportance
				}
			}
			return &service.IndustryContextData{
				IndustryID:         skill,
				BusinessCycle:      string(pos.BusinessCycle),
				CycleConfidence:    pos.Confidence,
				SeasonalMultiplier: seasonalMultiplier,
				SystemicImportance: systemicImportance,
			}
		}).
		WithCycleCardProvider(func() *industry.CycleStatusCard {
			if a.industryService == nil || a.industryService.CardBuilder == nil {
				return nil
			}
			card, err := a.industryService.CardBuilder.BuildCompositeCard(time.Now())
			if err != nil {
				return nil
			}
			return card
		})
	pipelineHandlers := apipipeline.NewHandlers(pipelineSvc)
	pipelineHandlers.ReasoningHandler = &apipipeline.ReasoningHandler{BaseDir: a.ledgerDir}
	pipelineHandlers.RegisterRoutes(mux)

	// Decision-chain aggregate endpoint: combines narrative events, event logic
	// rules, sector heatmap, recommendations, and exit alerts.
	decisionHandlers := &apidecision.Handlers{
		NarrativeEng:  a.narrativeEngine,
		IndustrySvc:   a.industryService,
		PipelineSvc:   pipelineSvc,
		MacroProvider: a.macroProvider,
		MacroIngestor: a.macroIngestor,
		WorkDir:       a.workDir,
		LedgerDir:     a.ledgerDir,
	}

	decisionHandlers.RegisterRoutes(mux)

	reportSvc := service.NewReportService(a.workDir, a.ledgerDir, outcomeStore)
	if a.macroProvider != nil {
		detector := portfolio.NewPeriodDetectorWithDefaults()
		advisor := methodology.NewAdvisor(nil)
		reportSvc.SetPeriodProvider(func() *service.PeriodInfo {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			snap, err := a.macroProvider.FetchSnapshot(ctx)
			if err != nil {
				return nil
			}
			ind := SnapshotToPeriodIndicators(snap)
			// B5 Batch 1: enrich with historical MA/change fields.
			if a.macroIngestor != nil {
				calc := portfolio.NewCalculator()
				_ = calc.EnrichFromDir(&ind, time.Now().UTC().Format("2006-01-02"), a.macroIngestor.SnapshotDir())
				// B5 Batch 3: sector rotation + public-bank consecutive buy
				sectorIndexDir := filepath.Join(a.workDir, "data", "state", "sector_index")
				govFlowDir := filepath.Join(a.workDir, "data", "state", "government_flow")
				calc.EnrichBatch3(&ind, time.Now().UTC().Format("2006-01-02"), sectorIndexDir, govFlowDir)
			}
			// G5: 台海危機訊號進時期判別（與 persistPeriodHistory 同源 resolveGeoScore）。
			ind.GeoIntensity = a.resolveGeoScore(ctx).Intensity
			ind.GeoIntensityChange5D = a.geoIntensityChange5D(ctx,
				time.Now().UTC().Format("2006-01-02"), ind.GeoIntensity)
			// A1（R8 接線）：國安基金護盤期間 → 黑天鵝條件 4。
			ind.NationalFundActive = marketdata.NewNationalStabilizationProvider().
				IsInterventionActive(time.Now().UTC().Format("2006-01-02"))
			assessment, _ := detector.DetectAssessment(ind)
			indicators := make([]service.IndicatorHit, len(assessment.TriggeredIndicators))
			for i, ti := range assessment.TriggeredIndicators {
				indicators[i] = service.IndicatorHit{
					Name: ti.Name, Value: ti.Value, Threshold: ti.Threshold,
					Relation: ti.Relation, Hit: ti.Hit, InputAvailable: ti.InputAvailable,
				}
			}
			return &service.PeriodInfo{
				MarketPeriod:        string(assessment.MarketPeriod),
				CashLevel:           advisor.CashReserve(assessment.MarketPeriod),
				Confidence:          assessment.Confidence,
				ConditionsHit:       assessment.ConditionsHit,
				ConditionsTotal:     assessment.ConditionsTotal,
				TriggeredIndicators: indicators,
			}
		})
	}
	reportHandlers := apipipeline.NewReportHandlers(reportSvc)
	reportHandlers.RegisterRoutes(mux)

	metricsSvc := service.NewMetricsService(
		&service.MetricsCollectorAdapter{
			GetScreeningRateFunc: a.metricsCollector.GetScreeningRate,
			GetMetricsSnapshotFunc: func() service.MetricsSnapshot {
				snap := a.metricsCollector.GetMetricsSnapshot()
				// Record the snapshot into the trend history (throttled to
				// one point per minute, and only when the counters actually
				// moved) so the trend chart shows real changes instead of a
				// flat plateau between simulation runs.
				if now := time.Now().Unix(); now-a.lastHistoryPush.Load() >= 60 {
					a.lastHistoryPush.Store(now)
					rate := int64(snap.ScreeningRate * 1e6)
					changed := rate != a.lastHistoryVal.Load() || snap.ScreeningTotal != a.lastHistoryTotal.Load()
					a.lastHistoryVal.Store(rate)
					a.lastHistoryTotal.Store(snap.ScreeningTotal)
					if changed {
						a.metricsHistory.AddSnapshot(MetricsSnapshot{
							ScreeningRate:      snap.ScreeningRate,
							AlertsTriggered:    snap.AlertsTriggered,
							AlertsAcknowledged: snap.AlertsAcknowledged,
							Timestamp:          snap.Timestamp,
						})
					}
				}
				return service.MetricsSnapshot{
					ScreeningTotal:     snap.ScreeningTotal,
					ScreeningPassed:    snap.ScreeningPassed,
					ScreeningRate:      snap.ScreeningRate,
					AlertsTriggered:    snap.AlertsTriggered,
					AlertsAcknowledged: snap.AlertsAcknowledged,
					AlertsByType:       snap.AlertsByType,
					Timestamp:          snap.Timestamp,
				}
			},
			CheckThresholdsFunc: func(threshold service.AlertThreshold) []service.ThresholdViolation {
				mt := AlertThreshold{
					MinScreeningRate:        threshold.MinScreeningRate,
					MaxAlertTriggerRate:     threshold.MaxAlertTriggerRate,
					MaxUnacknowledgedAlerts: threshold.MaxUnacknowledgedAlerts,
				}
				raw := a.metricsCollector.CheckThresholds(mt)
				out := make([]service.ThresholdViolation, len(raw))
				for i, v := range raw {
					out[i] = service.ThresholdViolation{
						Metric:    v.Metric,
						Current:   v.Current,
						Threshold: v.Threshold,
						Severity:  v.Severity,
						Message:   v.Message,
					}
				}
				return out
			},
		},
		&service.MetricsHistoryAdapter{
			GetTrendFunc: func(metric string) []service.TrendPoint {
				points := a.metricsHistory.GetTrend(metric)
				result := make([]service.TrendPoint, len(points))
				for i, p := range points {
					result[i] = service.TrendPoint{
						Timestamp: p.Timestamp,
						Value:     p.Value,
					}
				}
				return result
			},
		},
	)
	metricsHandlers := apimetrics.NewHandlers(metricsSvc)
	if a.storageReport != nil {
		metricsHandlers.WithStorageReporter(a.storageReport)
	}
	if a.dataQualityChecker != nil {
		metricsHandlers.WithQualityChecker(dataQualityAdapter{a.dataQualityChecker})
	}
	metricsHandlers.RegisterRoutes(mux)

	systemSvc := service.NewSystemService(
		a.workDir,
		a.ledgerDir,
		a.baselinePath,
		outcomeStore,
		a.janusEngine,
		apigateway.NewChannelHealthStoreWithPool(filepath.Join(a.workDir, "data/state"), a.pool),
	)
	if a.pool != nil {
		systemSvc.SetHistoricalStore(ledger.NewPostgresHistoricalStore(a.pool))
	}
	if a.industryService != nil {
		systemSvc.SetCycleTracker(a.industryService.CycleTracker)
	}
	systemHandlers := apisystem.NewHandlers(systemSvc)
	if a.dataFetcher != nil {
		systemHandlers.DayTradingFetcher = apisystem.DayTradingFetcher(
			NewDayTradingFetcher(a.dataFetcher),
		)
		systemHandlers.TaifexFetcher = NewTaifexFetcher(a.dataFetcher)
		systemHandlers.OddLotFetcher = NewOddLotFetcher(a.dataFetcher)
		systemHandlers.ETFFetcher = NewETFFetcher()
	}
	if a.geoProvider != nil || a.taiwanGeoProvider != nil {
		systemHandlers.GeopoliticalRiskFetcher = newGeopoliticalRiskFetcher(a.geoProvider, a.taiwanGeoProvider)
	}
	systemHandlers.RegisterRoutes(mux)

	healthStore := apigateway.NewChannelHealthStore(filepath.Join(a.workDir, "data/state"))
	handlers := &apisystem.HealthHandlers{
		ChannelHealth: healthStore,
	}
	if a.apiAddr != "" {
		handlers.APIAddr = a.apiAddr
	}
	if a.fubonProxyPort != 0 {
		handlers.FubonAddr = fmt.Sprintf("127.0.0.1:%d", a.fubonProxyPort)
	}
	// /api/health/aggregate — Stage 6 PR#1: frontend 單一呼叫即可取得 4-tier 健康聚合。
	// Per P2 C02: canonical /health is registered by cmd/atlas/api_routes.go newHealthHandler.
	// HealthHandlers.RegisterRoutes (which registered a duplicate GET /health) is intentionally
	// not called here to avoid two competing /health handlers.

	handlers.RegisterAggregateRoute(mux)
	// Channel health summary endpoint for the alerts page dashboard.
	mux.HandleFunc("/api/dashboard/channel-health", func(w http.ResponseWriter, r *http.Request) {
		// Uses ChannelHealthStore (same source as /api/health/aggregate Tier 2)
		// for freshness-aware status, instead of reading channel_health.json directly.
		//
		// PR-C: each channel response also includes a `known_issue` field
		// (populated from the static registry in known_issues.go) so the
		// dashboard UI can distinguish "atlas broke" (red error) from
		// "we know about this, it's an external problem" (gray known-issue
		// badge with link to the tracking issue).
		type channelHealthResp struct {
			ChannelID          string      `json:"channel_id"`
			Status             string      `json:"status"`
			UpdatedAt          string      `json:"updated_at,omitempty"`
			LastDataAt         string      `json:"last_data_at,omitempty"`
			LastError          string      `json:"last_error,omitempty"`
			LastSuccessAt      string      `json:"last_success_at,omitempty"`
			LatencyMs          int64       `json:"latency_ms,omitempty"`
			RateLimitRemaining int         `json:"rate_limit_remaining,omitempty"`
			RecordsFetched     int         `json:"records_fetched,omitempty"`
			SymbolsProcessed   int         `json:"symbols_processed,omitempty"`
			Errors             []string    `json:"errors,omitempty"`
			KnownIssue         *KnownIssue `json:"known_issue,omitempty"`
		}
		allRecs := healthStore.All()
		channels := make([]channelHealthResp, 0, len(allRecs))
		for id, rec := range allRecs {
			channels = append(channels, channelHealthResp{
				ChannelID:          id,
				Status:             rec.Status,
				UpdatedAt:          rec.LastFetchAt,
				LastDataAt:         rec.LastDataAt,
				LastError:          rec.LastError,
				LastSuccessAt:      rec.LastSuccessAt,
				LatencyMs:          rec.LatencyMs,
				RateLimitRemaining: rec.RateLimitRemaining,
				RecordsFetched:     rec.RecordsFetched,
				SymbolsProcessed:   rec.SymbolsProcessed,
				Errors:             rec.Errors,
				KnownIssue:         LookupKnownIssue(id),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"channels":   channels,
			"updated_at": "", // no longer file-based
		})
	})

	// Agent display-name registry endpoint. Single source of truth for the
	// performance-report frontend (replaces two competing static maps in
	// shared_web/static/js/names.js and shared_web/static/js/shared/constants.js).
	mux.HandleFunc("/api/dashboard/agent-names", a.handleAgentNames)

	// Deprecated: dev-only health check; not for web UI or MCP. See docs/operations/tier-boundary.md.
	mux.HandleFunc("/api/health/data-integrity", apisystem.HandleDataIntegrity(a.workDir, a.ledgerDir))

	// Swarm routes removed — simulation engine demoted in PR #963, cleaned in PR #964.

	riskHandlers := apirisk.NewHandlers(a.ledgerDir)
	if a.riskGate != nil {
		riskHandlers.WithRiskGate(a.riskGate)
	}
	if a.industryService != nil {
		if a.industryService.LinkageAnalyzer != nil {
			riskHandlers.WithCorrelationMatrix(a.industryService.LinkageAnalyzer.GetCorrelationMatrix())
		}
		if a.industryService.Classifier != nil {
			// Inject the canonical industry classification tree so the
			// /api/dashboard/correlation-matrix labels resolve to the
			// Chinese `name` field from configs/parameters.json rather
			// than the legacy 13-entry hardcoded map (PR fix: crossmarket
			// matrix showed English IDs for newer sectors like defensive,
			// high_dividend, leo_satellite, etf_rotation, pcb, etc.).
			riskHandlers.WithClassificationTree(a.industryService.Classifier)
		}
	}
	a.riskHandlers = riskHandlers
	riskHandlers.RegisterRoutes(mux)

	var dividendProvider apitax.DividendProvider
	cfg := config.Load()
	if cfg.FinMindAPIKey != "" {
		// FinMind dividend provider is tax-utility, not a data channel.
		// Gateway migration deferred — 見歷史紀錄（已移出公開 docs）。
		finMindClient := marketdata.GetSharedFinMindClient(cfg.FinMindAPIKey, a.workDir)
		cacheDir := filepath.Join(a.workDir, "data", "cache", "dividends")
		dividendProvider = marketdata.NewFinMindDividendProvider(finMindClient, cacheDir)
	}
	taxHandlers := apitax.NewHandlers(a.ledgerDir, dividendProvider)
	taxHandlers.RegisterRoutes(mux)

	paramHandlers := apiparameters.NewHandlers(filepath.Join(a.workDir, constants.ParametersFile))
	paramHandlers.RegisterRoutes(mux)

	mux.Handle("GET /api/config", configHandler())

	// Dashboard management center handlers (data-channels, data-pipeline,
	// drawdown, sim-trace, channel toggle, api-keys, etc.)
	dashboardHandlers := apidashboard.NewHandlers(a.workDir, a.ledgerDir)
	dashboardHandlers.RegisteredChannelIDs = a.RegisteredChannelIDs
	if a.pool != nil {
		dashboardHandlers.Pool = a.pool
	}
	if a.macroIngestor != nil {
		dashboardHandlers.MacroIngestor = a.macroIngestor
	}
	if a.geoProvider != nil {
		dashboardHandlers.GeoProvider = a.geoProvider
	}
	if a.taiwanGeoProvider != nil {
		dashboardHandlers.TaiwanGeoProvider = a.taiwanGeoProvider
	}
	if a.janusEngine != nil {
		dashboardHandlers.JanusEngine = a.janusEngine
	}
	dashboardHandlers.DrawdownProvider = a     // DashboardAPI satisfies DrawdownProvider
	dashboardHandlers.TaskLivenessProvider = a // DashboardAPI satisfies TaskLivenessProvider (late-bound)
	dashboardHandlers.SchedulerStatus = a      // DashboardAPI satisfies SchedulerStatusProvider (late-bound)
	dashboardHandlers.RegisterRoutes(mux)

	a.RegisterPerformanceRoutes(mux)
	a.RegisterCircuitBreakerRoutes(mux)
}

func (a *DashboardAPI) SetStrategiesHandlers(h *apistrategies.Handlers) {
	a.strategyTechniquesHandlers = h
	if a.strategiesAnnotator != nil {
		h.SetAnnotator(a.strategiesAnnotator)
	}
}

func (a *DashboardAPI) SetStrategiesAnnotator(ann llm_annotator.Annotator) {
	a.strategiesAnnotator = ann
	if kc, ok := ann.(*llm_annotator.KimiClient); ok {
		a.kimiClient = kc
	}
	if a.strategyTechniquesHandlers != nil {
		a.strategyTechniquesHandlers.SetAnnotator(ann)
	}
}

// SetStrategiesSummaryHandler wires the LLM strategy summary handler into
// the strategies API. The handler is opt-in; the /summary endpoint returns
// 503 until a real handler is wired in.
func (a *DashboardAPI) SetStrategiesSummaryHandler(sh *llmcapabilities.StrategySummaryHandler) {
	if a.strategyTechniquesHandlers != nil {
		a.strategyTechniquesHandlers.SetSummaryHandler(sh)
	}
}

// SetStrategiesMethodologyAdvisor wires the methodology advisor into the
// strategies API so StrategyFrameSummary can include category badges (E5b).
func (a *DashboardAPI) SetStrategiesMethodologyAdvisor(advisor *methodology.Advisor) {
	if a.strategyTechniquesHandlers != nil {
		a.strategyTechniquesHandlers.SetMethodologyAdvisor(advisor)
	}
}

func (a *DashboardAPI) RegisterStrategiesRoutes(mux *http.ServeMux) {
	if a.strategyTechniquesHandlers != nil {
		a.strategyTechniquesHandlers.RegisterRoutes(mux)
	}
}

func (a *DashboardAPI) SetCalibrationTask(task *narrative.CalibrationTask) {
	a.calibrationTask = task
}

func (a *DashboardAPI) RunCalibration() (*narrative.CalibrationValidation, error) {
	if a.calibrationTask == nil {
		return nil, fmt.Errorf("run calibration: no calibration task set")
	}
	return a.calibrationTask.RunCalibrationCycle()
}

func (a *DashboardAPI) RegisterIndustryRoutes(mux *http.ServeMux) {
	handlers := &apiindustry.Handlers{
		Svc:             a.industryService,
		SectorAllocator: a.industryService.WeightEngine,
	}
	handlers.RegisterRoutes(mux)
}

func (a *DashboardAPI) RegisterSwaggerRoutes(mux *http.ServeMux) {
	swaggerHandlers := apisystem.NewSwaggerHandlers(a.workDir)
	swaggerHandlers.RegisterRoutes(mux)
}

func (a *DashboardAPI) RegisterNarrativeRoutes(mux *http.ServeMux) {
	svc := service.NewNarrativeService(a.workDir, a.narrativeEngine, a.reportGenerator).
		WithHistoricalStore(a.historicalStore)
	svc.SetMacroProvider(a.macroProvider)
	a.narrativeHandlers = &apinarrative.Handlers{
		Svc:             svc,
		IndustryService: a.industryService,
		EventCalendar:   a.industryService.EventCalendar,
	}
	a.narrativeHandlers.RegisterRoutes(mux)
}

// NarrativeHandlers returns the narrative API handlers so warmup can call
// BuildBundle directly through the same code path as the HTTP handler.
func (a *DashboardAPI) NarrativeHandlers() *apinarrative.Handlers {
	return a.narrativeHandlers
}

func (a *DashboardAPI) RegisterControlRoutes(mux *http.ServeMux) {
	var outcomeStore ledger.OutcomeStore
	if a.repo != nil {
		outcomeStore = NewDualWriteOutcomeStoreAdapter(a.repo)
	} else {
		cfg := config.Config{
			LedgerDir:    a.ledgerDir,
			StoreBackend: a.storeBackend,
			SQLitePath:   a.sqlitePath,
		}
		var err error
		outcomeStore, err = ledger.NewOutcomeStore(cfg)
		if err != nil {
			logging.Error("dashboardapi", "create_control_outcome_store_failed", "err", err)
			outcomeStore = nil
		}
	}
	svc := service.NewControlService(a.workDir, a.ledgerDir, a.healthManager, outcomeStore)
	handlers := &apicontrol.Handlers{
		Svc: svc,
	}
	handlers.RegisterRoutes(mux)
}

func (a *DashboardAPI) RegisterMacroRoutes(mux *http.ServeMux) {
	svc := service.NewMacroService(a.workDir, a.macroIngestor, a.taiwanStressCalc).
		WithGeoProvider(a.geoProvider).
		WithHistoricalStore(a.historicalStore)
	handlers := &apimacro.Handlers{
		Service: svc,
	}
	handlers.RegisterRoutes(mux)
}

func (a *DashboardAPI) RegisterCrossMarketRoutes(mux *http.ServeMux) {
	a.crossMarketSvc = service.NewCrossMarketService(a.macroProvider)

	// E05: warm up rolling correlation engines with historical snapshots
	// so the correlation API returns meaningful data immediately instead
	// of waiting 20 trading days for the window to fill.
	snapshotDir := filepath.Join(a.workDir, "data", "state", "macro")
	if entries, err := os.ReadDir(snapshotDir); err == nil {
		var snapshots []marketdata.MacroDataSnapshot
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			if entry.Name() == "latest.json" || entry.Name() == "previous.json" || entry.Name() == "_metadata.json" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(snapshotDir, entry.Name()))
			if err != nil {
				continue
			}
			var snap marketdata.MacroDataSnapshot
			if err := json.Unmarshal(data, &snap); err == nil {
				snapshots = append(snapshots, snap)
			}
		}
		if len(snapshots) > 0 {
			a.crossMarketSvc.WarmupFromHistory(snapshots)
			logging.Info("dashboard_api", "correlation_warmup_complete",
				"snapshots", len(snapshots))
		}
	}

	dm := metrics.NewDegradedMetrics()
	dm.SetOnInc(func(name string, labels []string, value float64) {
		labelMap := make(map[string]string, len(labels)/2)
		for i := 0; i+1 < len(labels); i += 2 {
			labelMap[labels[i]] = labels[i+1]
		}
		a.metricsCollector.RecordCounter(name, value, labelMap)
	})
	a.crossMarketSvc.SetDegradedMetrics(dm)
	mux.HandleFunc("/api/degraded", metrics.HandleDegraded(dm))
	mux.HandleFunc("/api/llm_annotator/cost", metrics.HandleCost(func() *llm_annotator.KimiClient { return a.kimiClient }, 0.001))
	handlers := &apicrossmarket.Handlers{
		Svc: a.crossMarketSvc,
	}
	handlers.RegisterRoutes(mux)
}

// GetCrossMarketService returns the cross-market service for live correlation updates.
func (a *DashboardAPI) GetCrossMarketService() *service.CrossMarketService {
	return a.crossMarketSvc
}

func (a *DashboardAPI) RegisterLiveRoutes(mux *http.ServeMux) {
	svc := service.NewLiveService(a.workDir, a.ledgerDir)
	handlers := &apilive.Handlers{
		LedgerDir:     a.ledgerDir,
		WorkDir:       a.workDir,
		Svc:           svc,
		Classifier:    a.industryService.Classifier,
		AgentLayerMap: apilive.BuildAgentLayerMap(a.workDir),
	}
	handlers.RegisterRoutes(mux)
}

func (a *DashboardAPI) RegisterExperimentRoutes(mux *http.ServeMux) {
	handlers := &apiexperiment.Handlers{
		BaselinePath: a.baselinePath,
		LedgerDir:    a.ledgerDir,
		WorkDir:      a.workDir,
	}
	handlers.RegisterRoutes(mux)
}

func (a *DashboardAPI) RegisterBacktestRoutes(mux *http.ServeMux) {
	cfg := config.Normalize(config.Load())
	svc := service.NewBacktestService(cfg)
	if a.eventBus != nil {
		svc.WithEventBus(a.eventBus)
	}
	handlers := apibacktest.NewHandlers(svc, a.ledgerDir)
	handlers.RegisterRoutes(mux)
}

func (a *DashboardAPI) RegisterPerformanceRoutes(mux *http.ServeMux) {
	cfg := config.Normalize(config.Load())
	// SSoT read path (2026-08-23): PG-first with JSONL fallback + degraded
	// marker for the postgres backend; other backends keep their semantics.
	store, err := ledger.NewReportOutcomeStore(cfg)
	if err != nil {
		logging.Error("dashboard", "performance_store_init_failed", logging.Err(err))
		store = ledger.NewStore(a.ledgerDir)
	}
	svc := service.NewPerformanceService(store, a.ledgerDir)
	handlers := apiperformance.NewHandlers(svc)
	handlers.RegisterRoutes(mux)
}

func (a *DashboardAPI) RegisterCircuitBreakerRoutes(mux *http.ServeMux) {
	svc := service.NewCircuitBreakerService(a.workDir)
	handlers := &apicircuitbreaker.Handlers{
		Svc: svc,
	}
	handlers.RegisterRoutes(mux)
}

func (a *DashboardAPI) SetPool(pool *pgxpool.Pool) {
	a.pool = pool
}

func (a *DashboardAPI) SetHealthManager(m *portfolio.AgentHealthManager) {
	a.healthManager = m
}

func (a *DashboardAPI) SetJanusEngine(e *janus.Engine) {
	a.janusEngine = e
}

func (a *DashboardAPI) SetRepository(repo *repository.DualWriteRepository) {
	a.repo = repo
}

func (a *DashboardAPI) SetTaskManager(m *taskexec.Manager) {
	a.taskManager = m
}

// SetTaskLivenessProvider wires the cross-restart task-liveness store into
// GET /api/dashboard/task-liveness. May be nil (endpoint then reports 503).
func (a *DashboardAPI) SetTaskLivenessProvider(p apidashboard.TaskLivenessProvider) {
	a.taskLivenessProvider = p
}

// SetSchedulerStatusProvider wires the live BackgroundTaskManager status into
// the task-liveness aggregation (interval / enabled / next run / staleness).
func (a *DashboardAPI) SetSchedulerStatusProvider(p apidashboard.SchedulerStatusProvider) {
	a.schedulerProvider = p
}

// List implements apidashboard.TaskLivenessProvider.
func (a *DashboardAPI) List(ctx context.Context) ([]liveness.Row, error) {
	if a.taskLivenessProvider == nil {
		return nil, errors.New("task liveness provider not configured")
	}
	return a.taskLivenessProvider.List(ctx)
}

// Status implements apidashboard.SchedulerStatusProvider.
func (a *DashboardAPI) Status() []apigateway.TaskStatus {
	if a.schedulerProvider == nil {
		return nil
	}
	return a.schedulerProvider.Status()
}

func (a *DashboardAPI) SetStorageReporter(r apimetrics.StorageReporter) {
	a.storageReport = r
}

func (a *DashboardAPI) SetGateway(g DataFetcher) {
	a.dataFetcher = g
	a.initGatewayProviders()
}

// initGatewayProviders replaces legacy direct providers with Gateway-backed adapters.
// Called once when SetGateway() injects the DataFetcher.
func (a *DashboardAPI) initGatewayProviders() {
	if a.dataFetcher == nil {
		return
	}
	a.macroProvider = NewMacroDataGatewayAdapter(a.dataFetcher)
	a.geoProvider = NewGeopoliticalGatewayAdapter(a.dataFetcher)
	if a.macroIngestor != nil {
		a.macroIngestor = narrative.NewMacroIngestor(a.macroProvider, filepath.Join(a.workDir, constants.StateMacro))
	}
	logging.Info("dashboardapi", "gateway_providers_initialized")
}

// SetRiskGate injects a RiskGate instance for serving calibration reports.
func (a *DashboardAPI) SetRiskGate(g *risk.RiskGate) {
	a.riskGate = g
	if a.riskHandlers != nil {
		a.riskHandlers.WithRiskGate(g)
	}
}

// SetLatestDrawdown stores the latest drawdown result.
func (a *DashboardAPI) SetLatestDrawdown(d *portfolio.DrawdownResult) {
	a.drawdownMu.Lock()
	defer a.drawdownMu.Unlock()
	a.latestDrawdown = d
}

// handleAgentNames serves the agent display-name registry as JSON. Reads
// configs/agents.json from workDir and returns one record per agent with
// id, name, skill, layer. Returns an empty array on missing file so the
// frontend can render gracefully without error-handling per render.
func (a *DashboardAPI) handleAgentNames(w http.ResponseWriter, r *http.Request) {
	type agentNameResp struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Skill string `json:"skill"`
		Layer string `json:"layer"`
	}
	w.Header().Set("Content-Type", "application/json")
	agentsPath := filepath.Join(a.workDir, "configs", "agents.json")
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		if os.IsNotExist(err) {
			_ = json.NewEncoder(w).Encode(map[string]any{"agents": []agentNameResp{}})
			return
		}
		apishared.WriteJSONErrorEx(w, http.StatusInternalServerError, "agent_registry_read_failed", "read agent registry")
		return
	}
	var reg domain.AgentRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		apishared.WriteJSONErrorEx(w, http.StatusInternalServerError, "agent_registry_parse_failed", "parse agent registry")
		return
	}
	agents := make([]agentNameResp, 0, len(reg.Agents))
	for _, ag := range reg.Agents {
		agents = append(agents, agentNameResp{
			ID:    ag.ID,
			Name:  ag.Name,
			Skill: ag.Skill,
			Layer: string(ag.Layer),
		})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"agents": agents})
}

// SetCrisisModeSetter registers a callback invoked when macro ingest detects
// VIX >= 35 so the optimizer can enable crisis mode (covariance inflation).
func (a *DashboardAPI) SetCrisisModeSetter(fn func(active bool)) {
	a.crisisModeSetter = fn
}

// InvokeCrisisModeSetter calls the registered crisis mode setter with the given
// active flag. Safe no-op if no setter is registered.
func (a *DashboardAPI) InvokeCrisisModeSetter(active bool) {
	if a.crisisModeSetter != nil {
		a.crisisModeSetter(active)
	}
}

// SetCorrelationSetter registers a callback invoked by macro ingest to update
// the optimizer's SPX-TWSE dynamic correlation for covariance inflation.
func (a *DashboardAPI) SetCorrelationSetter(fn func(rho float64)) {
	a.correlationSetter = fn
}

// InvokeCorrelationSetter calls the registered correlation setter. No-op if none.
func (a *DashboardAPI) InvokeCorrelationSetter(rho float64) {
	if a.correlationSetter != nil {
		a.correlationSetter(rho)
	}
}

// GetLatestDrawdown returns the latest drawdown result, or nil.
func (a *DashboardAPI) GetLatestDrawdown() *portfolio.DrawdownResult {
	a.drawdownMu.RLock()
	defer a.drawdownMu.RUnlock()
	return a.latestDrawdown
}

func (a *DashboardAPI) GetIndustryService() *service.IndustryService {
	return a.industryService
}

// HandleVersion serves the build info as JSON at GET /api/version.
func (a *DashboardAPI) HandleVersion(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(buildinfo.Current()); err != nil {
		log.Printf("[api/version] encode error: %v", err)
	}
}

// RecordCycleCalibrationOutcome stores a calibration data point for the
// cycle layer accuracy tracker. Called by the backtest pipeline after
// daily returns are computed. Safe when industryService or CycleCalibration
// is nil — the call is silently dropped.
func (a *DashboardAPI) RecordCycleCalibrationOutcome(
	sessionID string, date time.Time, layerSignals map[string]float64, actualReturn float64,
) {
	if a.industryService != nil {
		a.industryService.RecordCycleCalibrationOutcome(sessionID, date, layerSignals, actualReturn)
	}
}

func (a *DashboardAPI) RegisterTaskExecRoutes(mux *http.ServeMux) {
	if a.taskManager == nil {
		logging.Warn("dashboardapi", "taskexec_skip_registration", "reason", "taskManager is nil")
		return
	}
	logging.Info("dashboardapi", "taskexec_registering_routes")
	handlers := apitaskexec.NewHandlers(a.taskManager)
	handlers.RegisterRoutes(mux)
}

// RouteOptions controls which optional route groups are registered.
type RouteOptions struct {
	IncludeBacktest bool // backtest and experiment routes
	IncludeSwagger  bool // Swagger API documentation
}

// RegisterAllRoutes registers all DashboardAPI routes in one call.
// This replaces the scattered registration pattern in cmd/atlas/main.go
// with a single options-driven entry point, ensuring route consistency
// across API, simulation, and live trading modes.
func (a *DashboardAPI) RegisterAllRoutes(mux *http.ServeMux, opts RouteOptions) {
	a.RegisterRoutes(mux)
	a.RegisterNarrativeRoutes(mux)
	a.RegisterControlRoutes(mux)
	a.RegisterMacroRoutes(mux)
	a.RegisterCrossMarketRoutes(mux)
	a.RegisterExperimentRoutes(mux)
	a.RegisterIndustryRoutes(mux)
	a.RegisterStrategiesRoutes(mux)
	a.RegisterLiveRoutes(mux)

	if opts.IncludeBacktest {
		a.RegisterBacktestRoutes(mux)
	}
	if opts.IncludeSwagger {
		a.RegisterSwaggerRoutes(mux)
	}
}

func configHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := config.Load()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cfg)
	})
}

// SnapshotToPeriodIndicators maps a raw macro snapshot to the indicator set
// consumed by the seven-period detector.
func SnapshotToPeriodIndicators(snap marketdata.MacroDataSnapshot) portfolio.PeriodIndicators {
	return portfolio.PeriodIndicators{
		VIX:                    snap.VIX.Value,
		DXY:                    snap.DXY.Value,
		US10Y:                  snap.US10Y.Value,
		SOXPrice:               snap.SOXIndex.Value,
		TSMADRPrice:            snap.TSMADR.Value,
		TAIEXPrice:             snap.TAIEX.Value,
		ForeignSingleDayNet:    snap.ForeignInvestorNet.Value,
		ForeignFuturesOI:       snap.ForeignFuturesOINet.Value,
		MarginBalance:          snap.RetailMarginBalance.Value,
		MarginMaintenanceRatio: snap.MarginMaintenanceRatio.Value,
		MarketVolume:           snap.MarketVolume.Value,
		DayTradeRatio:          snap.DayTradeRatio.Value,
	}
}
