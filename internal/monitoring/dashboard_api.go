package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	apibacktest "github.com/kaecer68/atlas-go/internal/monitoring/api/backtest"
	apicircuitbreaker "github.com/kaecer68/atlas-go/internal/monitoring/api/circuitbreaker"
	apicontrol "github.com/kaecer68/atlas-go/internal/monitoring/api/control"
	apidashboard "github.com/kaecer68/atlas-go/internal/monitoring/api/dashboard"
	apidecision "github.com/kaecer68/atlas-go/internal/monitoring/api/decision"
	apieventlogic "github.com/kaecer68/atlas-go/internal/monitoring/api/eventlogic"
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
	apiswarm "github.com/kaecer68/atlas-go/internal/monitoring/api/swarm"
	apisystem "github.com/kaecer68/atlas-go/internal/monitoring/api/system"
	apitaskexec "github.com/kaecer68/atlas-go/internal/monitoring/api/taskexec"
	apitax "github.com/kaecer68/atlas-go/internal/monitoring/api/tax"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/repository"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/taskexec"
)

// DataFetcher is a Gateway-compatible data fetch function injected via SetGateway.
// It breaks the import cycle between monitoring and apigateway packages.
type DataFetcher func(ctx context.Context, channelID string) ([]byte, error)

type DashboardAPI struct {
	workDir            string
	ledgerDir          string
	storeBackend       string
	sqlitePath         string
	baselinePath       string
	narrativeEngine    *narrative.NarrativeEngine
	macroIngestor      *narrative.MacroIngestor
	macroProvider      marketdata.MacroDataProvider
	geoProvider        narrative.GeopoliticalRiskProvider
	taiwanGeoProvider  narrative.GeopoliticalRiskProvider
	taiwanStressCalc   *narrative.TaiwanStressCalculator
	reportGenerator    *narrative.ReportGenerator
	pool               *pgxpool.Pool
	industryService    *service.IndustryService
	metricsCollector   *MetricsCollector
	metricsHistory     *MetricsHistory
	healthManager      *portfolio.AgentHealthManager
	dataQualityChecker *DataQualityChecker
	janusEngine        *janus.Engine
	repo               *repository.DualWriteRepository
	taskManager        *taskexec.Manager
	eventBus           *eventbus.ChannelEventBus
	outcomeStore       *DualWriteOutcomeStoreAdapter
	storageReport      apimetrics.StorageReporter
	dataFetcher        DataFetcher
	riskGate           *risk.RiskGate
	latestDrawdown     *portfolio.DrawdownResult
	drawdownMu         sync.RWMutex
	eventLogicHandlers *apieventlogic.Handlers
}

func NewDashboardAPI(workDir, ledgerDir string, metricsCollector *MetricsCollector) *DashboardAPI {
	cfg := config.Load()
	var providers []marketdata.MacroDataProvider

	// Yahoo Finance-backed providers — only when enabled.
	// Legacy constructor: production uses NewDashboardAPIWithGateway() instead.
	// See docs/GATEWAY_MIGRATION_TRACKING.md.
	if cfg.YahooEnabled {
		providers = append(providers, marketdata.NewYahooFinanceMacroProvider())
		providers = append(providers, marketdata.NewSOXIndexProvider())
			providers = append(providers, marketdata.NewDRAMSpotPriceProvider())
	}

	providers = append(providers, marketdata.NewFrankfurterFXProvider())
	providers = append(providers, marketdata.NewBDIProvider())
	// ExchangeRate-API provides TWD (not available in ECB/Frankfurter dataset).
	providers = append(providers, marketdata.NewExchangeRateProvider())
	providers = append(providers, marketdata.NewTWSECapitalFlowProvider(filepath.Join(workDir, "data/state/capital_flow")))
	providers = append(providers, marketdata.NewTWSEMarginBalanceProvider(filepath.Join(workDir, "data/state/margin")))
	providers = append(providers, marketdata.NewExportStatisticsProvider(filepath.Join(workDir, "data/state/export")))
	// Sector data from local cache (graceful degradation if file missing).
	providers = append(providers, marketdata.NewSectorDataProvider(filepath.Join(workDir, "data/state/sector_data")))
	// TSMC Revenue from FinMind (overwrites cached sector data when available).
	if cfg.FinMindAPIKey != "" {
		providers = append(providers, marketdata.NewTSMCRevenueProvider(cfg.FinMindAPIKey))
	}
	provider := marketdata.NewCompositeMacroProvider(providers...)
	geoProvider := narrative.NewCompositeGeopoliticalProvider(
		narrative.NewRSSGeopoliticalProvider(),
		narrative.NewGDELTGeopoliticalProvider(),
	)
	taiwanGeoProvider := narrative.NewCompositeTaiwanGeopoliticalProvider(
		narrative.NewTaiwanRSSGeopoliticalProvider(),
	)
	if metricsCollector == nil {
		metricsCollector = NewMetricsCollector()
	}
	lifecycle := narrative.NewEventLifecycleManager()
	ingestor := narrative.NewMacroIngestor(provider, filepath.Join(workDir, "data/state/macro"))
	ingestor.SetLifecycleManager(lifecycle)

	narrativeEng := narrative.NewNarrativeEngine()
	return &DashboardAPI{
		workDir:            workDir,
		ledgerDir:          ledgerDir,
		storeBackend:       os.Getenv("ATLAS_STORE_BACKEND"),
		sqlitePath:         os.Getenv("ATLAS_SQLITE_PATH"),
		baselinePath:       filepath.Join(workDir, "data/state/baseline_policy.json"),
		narrativeEngine:    narrativeEng,
		macroIngestor:      ingestor,
		macroProvider:      provider,
		geoProvider:        geoProvider,
		taiwanGeoProvider:  taiwanGeoProvider,
		taiwanStressCalc:   narrative.NewTaiwanStressCalculator(geoProvider, workDir),
		reportGenerator:    narrative.NewReportGenerator(),
		industryService:    newWiredIndustryService(narrativeEng, provider),
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
	ingestor := narrative.NewMacroIngestor(macroProvider, filepath.Join(workDir, "data/state/macro"))
	ingestor.SetLifecycleManager(lifecycle)

	narrativeEng := narrative.NewNarrativeEngine()
	return &DashboardAPI{
		workDir:            workDir,
		ledgerDir:          ledgerDir,
		storeBackend:       os.Getenv("ATLAS_STORE_BACKEND"),
		sqlitePath:         os.Getenv("ATLAS_SQLITE_PATH"),
		baselinePath:       filepath.Join(workDir, "data/state/baseline_policy.json"),
		narrativeEngine:    narrativeEng,
		macroIngestor:      ingestor,
		macroProvider:      macroProvider,
		geoProvider:        geoProvider,
		taiwanGeoProvider:  taiwanGeoProvider,
		taiwanStressCalc:   narrative.NewTaiwanStressCalculator(geoProvider, workDir),
		reportGenerator:    narrative.NewReportGenerator(),
		industryService:    newWiredIndustryService(narrativeEng, macroProvider),
		metricsCollector:   metricsCollector,
		metricsHistory:     NewMetricsHistory(1000),
		healthManager:      portfolio.NewAgentHealthManager(),
		dataQualityChecker: NewDataQualityChecker(workDir, ledgerDir),
		dataFetcher:        fetcher,
	}
}

func newWiredIndustryService(narrativeEngine *narrative.NarrativeEngine, macroProvider marketdata.MacroDataProvider) *service.IndustryService {
	seasonalEngine := industry.NewSeasonalEngine()
	cycleTracker := industry.NewCycleTracker()
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
	if macroProvider != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if snap, err := macroProvider.FetchSnapshot(ctx); err == nil {
			modulator.UpdateCurrent(snap)
			modulator.RecordSnapshot(snap)
			modulator.UpdateRollingBaseline() // compute rolling median baseline
		}
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
		newWiredEventCalendar(marketdata.NewTWSECalendarProvider()), // eventCalendar with TWSE provider
	)

	// Wire the macro provider into the silicon cycle aggregator so that
	// scheduled silicon_cycle_update tasks can pull real TSMC/SOX data.
	svc.SetMacroProvider(macroProvider)

	// Bootstrap silicon tracker with the initial macro snapshot so the
	// cycle status card has non-zero indicators from the first request.
	if macroProvider != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if snap, err := macroProvider.FetchSnapshot(ctx); err == nil {
			indicators := industry.ExtractSiliconIndicators(snap)
			siliconTracker.DetectPhase(time.Now(), indicators)
			cancel()
		} else {
			cancel()
			logging.Warn("monitoring", "silicon_bootstrap_failed", "err", err)
		}
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

	return svc
}

func newWiredEventCalendar(provider marketdata.CalendarEventProvider) *industry.EventCalendar {
	ec := industry.NewEventCalendar()
	if provider == nil {
		return ec
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	ec.UpdateFromProvider(ctx, provider)
	return ec
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

// IngestAndUpdateMacro performs macro ingestion and updates the narrative engine state.
// This ensures GetCurrentStressIndex() has valid data instead of zero values.
func (a *DashboardAPI) IngestAndUpdateMacro(ctx context.Context) ([]narrative.NarrativeEvent, marketdata.MacroDataSnapshot, error) {
	events, snap, err := a.macroIngestor.Ingest(ctx)
	if err != nil {
		// Even if ingestion fails, try to load existing snapshot for stress index
		if a.narrativeEngine != nil {
			a.loadSnapshotIntoNarrativeEngine()
		}
		return events, snap, err
	}
	if a.narrativeEngine != nil {
		geoScore := narrative.GeopoliticalRiskScore{}
		if a.geoProvider != nil {
			// Use a separate short timeout for geo fetch to avoid blocking startup
			// when GDELT RSS feeds are slow (can take 55s+ in CI).
			geoCtx, geoCancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer geoCancel()
			if score, err := a.geoProvider.FetchScore(geoCtx); err == nil {
				geoScore = score
			}
		}
		a.narrativeEngine.UpdateMacro(snap, geoScore)
	}
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

// CalibrateNarrative evaluates model performance against replay data and updates
// model weights and template hit rates. Returns the calibration report or error.
func (a *DashboardAPI) CalibrateNarrative(replayPath string) (*narrative.NarrativeCalibrationReport, error) {
	if a.narrativeEngine == nil {
		return nil, fmt.Errorf("narrative calibrate: no narrative engine")
	}
	return a.narrativeEngine.SelfCalibrate(replayPath)
}

// loadSnapshotIntoNarrativeEngine loads the latest snapshot from disk into the narrative engine.
// Used as fallback when live ingestion fails to ensure stress index has data.
func (a *DashboardAPI) loadSnapshotIntoNarrativeEngine() {
	snapDir := filepath.Clean(a.macroIngestor.SnapshotDir())
	latestPath := filepath.Join(snapDir, "latest.json")
	data, err := os.ReadFile(latestPath)
	if err != nil {
		return
	}
	var snap marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return
	}
	geoScore := narrative.GeopoliticalRiskScore{}
	if a.geoProvider != nil {
		geoCtx, geoCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer geoCancel()
		if score, err := a.geoProvider.FetchScore(geoCtx); err == nil {
			geoScore = score
		}
	}
	a.narrativeEngine.UpdateMacro(snap, geoScore)
}

func (a *DashboardAPI) SetContext(ctx context.Context) {
	if a.outcomeStore != nil && ctx != nil {
		a.outcomeStore.SetContext(ctx)
	}
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
		WithNarrativeProvider(func(eventIDs []string) *service.NarrativeContextData {
			if a.narrativeEngine == nil {
				return nil
			}
			events := a.narrativeEngine.DetectEvents(narrative.MarketNarrativeData{})
			var activeThemes []string
			var primaryTheme string
			var primaryHitRate float64
			var directionHint string
			for _, event := range events {
				if event.Status == "active" || event.Status == "confirmed" {
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
			return &service.IndustryContextData{
				IndustryID:      skill,
				BusinessCycle:   string(pos.BusinessCycle),
				CycleConfidence: pos.Confidence,
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
		WorkDir:       a.workDir,
		LedgerDir:     a.ledgerDir,
	}
	if a.eventLogicHandlers != nil {
		decisionHandlers.Registry = a.eventLogicHandlers.Registry()
	}
	decisionHandlers.RegisterRoutes(mux)

	reportSvc := service.NewReportService(a.workDir, a.ledgerDir, outcomeStore)
	reportHandlers := apipipeline.NewReportHandlers(reportSvc)
	reportHandlers.RegisterRoutes(mux)

	metricsSvc := service.NewMetricsService(
		&service.MetricsCollectorAdapter{
			GetScreeningRateFunc: a.metricsCollector.GetScreeningRate,
			GetMetricsSnapshotFunc: func() service.MetricsSnapshot {
				snap := a.metricsCollector.GetMetricsSnapshot()
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
	metricsHandlers.RegisterRoutes(mux)

	systemSvc := service.NewSystemService(a.workDir, a.ledgerDir, a.baselinePath, outcomeStore, a.janusEngine)
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
		systemHandlers.ETFFetcher = NewETFFetcher(a.dataFetcher)
	}
	if a.geoProvider != nil || a.taiwanGeoProvider != nil {
		systemHandlers.GeopoliticalRiskFetcher = newGeopoliticalRiskFetcher(a.geoProvider, a.taiwanGeoProvider)
	}
	systemHandlers.RegisterRoutes(mux)

	handlers := &apisystem.HealthHandlers{}
	handlers.RegisterRoutes(mux)

	mux.HandleFunc("/api/health/data-integrity", apisystem.HandleDataIntegrity(a.workDir, a.ledgerDir))

	swarmSvc := service.NewSwarmService(filepath.Join(a.workDir, "data/state/swarm_latest.json"))
	swarmSvc.SetTrainingDir(filepath.Join(a.workDir, "data/state/swarm_training"))
	swarmHandlers := apiswarm.NewHandlers(swarmSvc)
	swarmHandlers.RegisterRoutes(mux)

	riskHandlers := apirisk.NewHandlers(a.ledgerDir)
	riskHandlers.WithRiskGate(a.riskGate)
	if a.industryService != nil && a.industryService.LinkageAnalyzer != nil {
		riskHandlers.WithCorrelationMatrix(a.industryService.LinkageAnalyzer.GetCorrelationMatrix())
	}
	riskHandlers.RegisterRoutes(mux)

	var dividendProvider apitax.DividendProvider
	cfg := config.Load()
	if cfg.FinMindAPIKey != "" {
		// FinMind dividend provider is tax-utility, not a data channel.
		// Gateway migration deferred — see docs/GATEWAY_MIGRATION_TRACKING.md.
		finMindClient := marketdata.GetSharedFinMindClient(cfg.FinMindAPIKey)
		cacheDir := filepath.Join(a.workDir, "data", "cache", "dividends")
		dividendProvider = marketdata.NewFinMindDividendProvider(finMindClient, cacheDir)
	}
	taxHandlers := apitax.NewHandlers(a.ledgerDir, dividendProvider)
	taxHandlers.RegisterRoutes(mux)

	paramHandlers := apiparameters.NewHandlers(filepath.Join(a.workDir, "configs/parameters.json"))
	paramHandlers.RegisterRoutes(mux)

	mux.Handle("GET /api/config", configHandler())

	// Dashboard management center handlers (data-channels, data-pipeline,
	// drawdown, sim-trace, channel toggle, api-keys, etc.)
	dashboardHandlers := apidashboard.NewHandlers(a.workDir, a.ledgerDir)
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
	dashboardHandlers.DrawdownProvider = a // DashboardAPI satisfies DrawdownProvider
	dashboardHandlers.RegisterRoutes(mux)

	a.RegisterPerformanceRoutes(mux)
	a.RegisterCircuitBreakerRoutes(mux)
}

func (a *DashboardAPI) SetEventLogicHandlers(h *apieventlogic.Handlers) { a.eventLogicHandlers = h }
func (a *DashboardAPI) RegisterEventLogicRoutes(mux *http.ServeMux) {
	if a.eventLogicHandlers != nil {
		a.eventLogicHandlers.RegisterRoutes(mux)
	}
}

func (a *DashboardAPI) RegisterIndustryRoutes(mux *http.ServeMux) {
	handlers := &apiindustry.Handlers{
		Svc: a.industryService,
	}
	handlers.RegisterRoutes(mux)
}

func (a *DashboardAPI) RegisterSwaggerRoutes(mux *http.ServeMux) {
	swaggerHandlers := apisystem.NewSwaggerHandlers(a.workDir)
	swaggerHandlers.RegisterRoutes(mux)
}

func (a *DashboardAPI) RegisterNarrativeRoutes(mux *http.ServeMux) {
	svc := service.NewNarrativeService(a.workDir, a.narrativeEngine, a.reportGenerator)
	handlers := &apinarrative.Handlers{
		Svc:             svc,
		IndustryService: a.industryService,
	}
	handlers.RegisterRoutes(mux)
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
	svc := service.NewMacroService(a.workDir, a.macroIngestor, a.taiwanStressCalc)
	handlers := &apimacro.Handlers{
		Service: svc,
	}
	handlers.RegisterRoutes(mux)
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
	svc := service.NewPerformanceService(a.ledgerDir)
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
		a.macroIngestor = narrative.NewMacroIngestor(a.macroProvider, filepath.Join(a.workDir, "data/state/macro"))
	}
	logging.Info("dashboardapi", "gateway_providers_initialized")
}

// SetRiskGate injects a RiskGate instance for serving calibration reports.
func (a *DashboardAPI) SetRiskGate(g *risk.RiskGate) {
	a.riskGate = g
}

// SetLatestDrawdown stores the latest drawdown result.
func (a *DashboardAPI) SetLatestDrawdown(d *portfolio.DrawdownResult) {
	a.drawdownMu.Lock()
	defer a.drawdownMu.Unlock()
	a.latestDrawdown = d
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
	a.RegisterExperimentRoutes(mux)
	a.RegisterIndustryRoutes(mux)
	a.RegisterEventLogicRoutes(mux)
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
