package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/live"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	apibacktest "github.com/kaecer68/atlas-go/internal/monitoring/api/backtest"
	apicircuitbreaker "github.com/kaecer68/atlas-go/internal/monitoring/api/circuitbreaker"
	apicontrol "github.com/kaecer68/atlas-go/internal/monitoring/api/control"
	apievents "github.com/kaecer68/atlas-go/internal/monitoring/api/events"
	apiexperiment "github.com/kaecer68/atlas-go/internal/monitoring/api/experiment"
	apihealth "github.com/kaecer68/atlas-go/internal/monitoring/api/health"
	apiindustry "github.com/kaecer68/atlas-go/internal/monitoring/api/industry"
	apilive "github.com/kaecer68/atlas-go/internal/monitoring/api/live"
	apimacro "github.com/kaecer68/atlas-go/internal/monitoring/api/macro"
	apimetrics "github.com/kaecer68/atlas-go/internal/monitoring/api/metrics"
	apinarrative "github.com/kaecer68/atlas-go/internal/monitoring/api/narrative"
	apiorders "github.com/kaecer68/atlas-go/internal/monitoring/api/orders"
	apiparameters "github.com/kaecer68/atlas-go/internal/monitoring/api/parameters"
	apiperformance "github.com/kaecer68/atlas-go/internal/monitoring/api/performance"
	apipipeline "github.com/kaecer68/atlas-go/internal/monitoring/api/pipeline"
	apireport "github.com/kaecer68/atlas-go/internal/monitoring/api/report"
	apirisk "github.com/kaecer68/atlas-go/internal/monitoring/api/risk"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	apiswagger "github.com/kaecer68/atlas-go/internal/monitoring/api/swagger"
	apisystem "github.com/kaecer68/atlas-go/internal/monitoring/api/system"
	apitaskexec "github.com/kaecer68/atlas-go/internal/monitoring/api/taskexec"
	apitax "github.com/kaecer68/atlas-go/internal/monitoring/api/tax"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/repository"
	"github.com/kaecer68/atlas-go/internal/taskexec"
)

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
	taiwanGeoProvider  *narrative.CompositeTaiwanGeopoliticalProvider
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
	orderMgr           *live.OrderManager
	storageReport      apimetrics.StorageReporter
}

// channelState tracks enable/disable status for each channel.
type channelState struct {
	Enabled   bool      `json:"enabled"`
	UpdatedAt time.Time `json:"updated_at"`
}

var (
	channelStates   = make(map[string]channelState)
	channelStatesMu sync.RWMutex
)

func loadChannelStates(workDir string) {
	channelStatesMu.Lock()
	defer channelStatesMu.Unlock()

	path := filepath.Join(workDir, "data/state/channel_states.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return // file may not exist yet
	}
	_ = json.Unmarshal(data, &channelStates)
}

func saveChannelStates(workDir string) error {
	channelStatesMu.RLock()
	defer channelStatesMu.RUnlock()

	path := filepath.Join(workDir, "data/state/channel_states.json")
	data, err := json.MarshalIndent(channelStates, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func setChannelEnabled(workDir, channelID string, enabled bool) error {
	channelStatesMu.Lock()
	channelStates[channelID] = channelState{Enabled: enabled, UpdatedAt: time.Now()}
	channelStatesMu.Unlock()
	return saveChannelStates(workDir)
}

func updateEnvFile(envPath, key, value string) error {
	data, err := os.ReadFile(envPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := strings.Split(string(data), "\n")
	found := false
	prefix := key + "="
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			lines[i] = prefix + value
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, prefix+value)
	}
	return os.WriteFile(envPath, []byte(strings.Join(lines, "\n")), 0644)
}

func NewDashboardAPI(workDir, ledgerDir string, metricsCollector *MetricsCollector) *DashboardAPI {
	loadChannelStates(workDir)

	providers := []marketdata.MacroDataProvider{
		// TODO: Migrate to Gateway for direct Yahoo Finance macro provider instantiation.
		marketdata.NewYahooFinanceMacroProvider(),
		// TODO: Migrate to Gateway for direct Frankfurter FX provider instantiation.
		marketdata.NewFrankfurterFXProvider(),
		// TODO: Migrate to Gateway for direct SOX index provider instantiation.
		marketdata.NewSOXIndexProvider(),
		// TODO: Migrate to Gateway for direct TWSE capital flow provider instantiation.
		marketdata.NewTWSECapitalFlowProvider(filepath.Join(workDir, "data/state/capital_flow")),
		// TODO: Migrate to Gateway for direct TWSE margin balance provider instantiation.
		marketdata.NewTWSEMarginBalanceProvider(""),
		// TODO: Migrate to Gateway for direct export statistics provider instantiation.
		marketdata.NewExportStatisticsProvider(filepath.Join(workDir, "data/state/export")),
	}
	// Sector data from local cache (graceful degradation if file missing).
	// TODO: Migrate to Gateway for direct sector data provider instantiation.
	providers = append(providers, marketdata.NewSectorDataProvider(filepath.Join(workDir, "data/sector_data")))
	// TSMC Revenue from FinMind (overwrites cached sector data when available).
	cfg := config.Load()
	if cfg.FinMindAPIKey != "" {
		// TODO: Migrate to Gateway for direct TSMC revenue provider instantiation.
		providers = append(providers, marketdata.NewTSMCRevenueProvider(cfg.FinMindAPIKey))
	}
	// TODO: Migrate to Gateway for direct composite macro provider instantiation.
	provider := marketdata.NewCompositeMacroProvider(providers...)
	// TODO: Migrate to Gateway for direct geopolitical composite provider instantiation.
	geoProvider := narrative.NewCompositeGeopoliticalProvider(
		// TODO: Migrate to Gateway for direct RSS geopolitical provider instantiation.
		narrative.NewRSSGeopoliticalProvider(),
		// TODO: Migrate to Gateway for direct GDELT geopolitical provider instantiation.
		narrative.NewGDELTGeopoliticalProvider(),
	)
	// TODO: Migrate to Gateway for direct Taiwan geopolitical composite provider instantiation.
	taiwanGeoProvider := narrative.NewCompositeTaiwanGeopoliticalProvider(
		// TODO: Migrate to Gateway for direct Taiwan RSS geopolitical provider instantiation.
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
		Oil: marketdata.MacroDataPoint{Value: 75.0},   // Historical WTI average
		DXY: marketdata.MacroDataPoint{Value: 103.0},  // Historical DXY average
		BDI: marketdata.MacroDataPoint{Value: 1500.0}, // Historical BDI average
	}
	modulator := industry.NewDynamicEnvModulator(baseline, baseline)
	if macroProvider != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if snap, err := macroProvider.FetchSnapshot(ctx); err == nil {
			modulator.UpdateCurrent(snap)
		}
	}
	seasonalEngine.SetDynamicEnv(modulator)

	// Wire narrative provider into linkage analyzer for dynamic supply chain correlations
	linkageAnalyzer.SetNarrativeProvider(bridge)

	svc := service.NewIndustryService(
		industry.DefaultClassification(),
		seasonalEngine,
		cycleTracker,
		linkageAnalyzer,
		industry.NewRiskMonitor(),
	)

	replayPath := config.Load().ReplayDataPath
	if replayPath != "" {
		sectorSymbolsPath := filepath.Join(config.Load().WorkDir, "configs", "sector_symbols.json")
		if returns, err := industry.LoadIndustryReturnsFromReplay(replayPath, sectorSymbolsPath); err == nil {
			svc.RebuildCorrelations(returns)
		} else {
			logging.Warn("monitoring", "correlation_rebuild_failed", "err", err)
		}
	}

	return svc
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

	pipelineSvc := service.NewPipelineService(a.workDir, a.ledgerDir, outcomeStore)
	pipelineHandlers := apipipeline.NewHandlers(pipelineSvc)
	pipelineHandlers.ReasoningHandler = &apipipeline.ReasoningHandler{BaseDir: a.ledgerDir}
	pipelineHandlers.RegisterRoutes(mux)

	reportSvc := service.NewReportService(a.workDir, a.ledgerDir, outcomeStore)
	reportHandlers := apireport.NewHandlers(reportSvc)
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
	systemHandlers.RegisterRoutes(mux)

	handlers := &apihealth.Handlers{}
	handlers.RegisterRoutes(mux)

	mux.HandleFunc("/api/health/data-integrity", apihealth.HandleDataIntegrity(a.workDir, a.ledgerDir))

	riskHandlers := apirisk.NewHandlers(a.ledgerDir)
	riskHandlers.RegisterRoutes(mux)

	var dividendProvider apitax.DividendProvider
	cfg := config.Load()
	if cfg.FinMindAPIKey != "" {
		// TODO: Migrate to Gateway for direct FinMind client instantiation.
		finMindClient := marketdata.NewFinMindClient(cfg.FinMindAPIKey)
		cacheDir := filepath.Join(a.workDir, "data", "cache", "dividends")
		// TODO: Migrate to Gateway for direct FinMind dividend provider instantiation.
		dividendProvider = marketdata.NewFinMindDividendProvider(finMindClient, cacheDir)
	}
	taxHandlers := apitax.NewHandlers(a.ledgerDir, dividendProvider)
	taxHandlers.RegisterRoutes(mux)

	paramHandlers := apiparameters.NewHandlers(filepath.Join(a.workDir, "configs/parameters.json"))
	paramHandlers.RegisterRoutes(mux)

	// Data channels endpoint — uses DataChannelService for full channel metadata.
	mux.HandleFunc("/api/dashboard/data-channels", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		fugleKey := config.GetSecret("FUGLE_API_KEY")
		if fugleKey == "" {
			fugleKey = config.GetSecret("ATLAS_FUGLE_API_KEY")
		}
		fubonKey := config.GetSecret("FUBON_API_KEY")
		if fubonKey == "" {
			fubonKey = config.GetSecret("ATLAS_FUBON_API_KEY")
		}
		finmindKey := config.GetSecret("FINMIND_API_KEY")
		if finmindKey == "" {
			finmindKey = config.GetSecret("ATLAS_FINMIND_API_KEY")
		}
		tejKey := config.GetSecret("TEJ_API_KEY")
		channelSvc := service.NewDataChannelService(
			a.workDir,
			a.pool,
			a.macroIngestor,
			a.geoProvider,
			a.taiwanGeoProvider,
			a.janusEngine,
			fugleKey,
			fubonKey,
			finmindKey,
			tejKey,
		)
		channels, err := channelSvc.GetAllChannelStatuses(r.Context())
		if err != nil {
			shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load data channels: %v", err))
			return
		}
		alerts, err := channelSvc.GetAlerts(r.Context())
		if err != nil {
			alerts = []service.ChannelAlert{}
		}
		shared.WriteJSON(w, http.StatusOK, map[string]any{
			"channels":  channels,
			"alerts":    alerts,
			"generated": time.Now().Format(time.RFC3339),
		})
	})

	// Data pipeline endpoint — tracks producer/consumer freshness for all data sources.
	mux.HandleFunc("/api/dashboard/data-pipeline", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		pipelineSvc := service.NewDataPipelineService(a.workDir, a.ledgerDir)
		sources, err := pipelineSvc.GetPipelineStatus()
		if err != nil {
			shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load data pipeline: %v", err))
			return
		}
		shared.WriteJSON(w, http.StatusOK, map[string]any{
			"sources":   sources,
			"generated": time.Now().Format(time.RFC3339),
		})
	})

	// Management center endpoints — channel control and API key management.
	mux.HandleFunc("/api/dashboard/channels/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/dashboard/channels/")
		parts := strings.Split(path, "/")
		if len(parts) < 2 {
			shared.WriteJSONError(w, http.StatusBadRequest, "invalid path")
			return
		}
		channelID := parts[0]
		action := parts[1]

		switch action {
		case "trigger":
			shared.WriteJSON(w, http.StatusOK, map[string]any{
				"channel_id": channelID,
				"action":     "trigger",
				"status":     "ok",
				"note":       "next poll will reflect fresh status",
			})
		case "toggle":
			var req struct {
				Enabled bool `json:"enabled"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				shared.WriteJSONError(w, http.StatusBadRequest, "invalid body")
				return
			}
			if err := setChannelEnabled(a.workDir, channelID, req.Enabled); err != nil {
				shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("save state: %v", err))
				return
			}
			shared.WriteJSON(w, http.StatusOK, map[string]any{
				"channel_id": channelID,
				"enabled":    req.Enabled,
				"status":     "ok",
			})
		default:
			shared.WriteJSONError(w, http.StatusBadRequest, "unknown action")
		}
	})

	mux.HandleFunc("/api/dashboard/api-keys/update", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req struct {
			Provider string `json:"provider"`
			APIKey   string `json:"api_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			shared.WriteJSONError(w, http.StatusBadRequest, "invalid body")
			return
		}
		if req.Provider == "" || req.APIKey == "" {
			shared.WriteJSONError(w, http.StatusBadRequest, "provider and api_key required")
			return
		}
		allowedProviders := map[string]bool{
			"finmind": true,
			"fugle":   true,
			"tej":     true,
			"fubon":   true,
		}
		if !allowedProviders[strings.ToLower(req.Provider)] {
			shared.WriteJSONError(w, http.StatusBadRequest, "invalid provider")
			return
		}
		if len(req.APIKey) < 8 || len(req.APIKey) > 512 {
			shared.WriteJSONError(w, http.StatusBadRequest, "api_key length invalid")
			return
		}
		key := strings.ToUpper(req.Provider) + "_API_KEY"
		os.Setenv(key, req.APIKey)
		shared.WriteJSON(w, http.StatusOK, map[string]any{
			"provider": req.Provider,
			"status":   "ok",
		})
	})

	a.RegisterOrderRoutes(mux)
	a.RegisterPerformanceRoutes(mux)
	a.RegisterCircuitBreakerRoutes(mux)
}

func (a *DashboardAPI) RegisterIndustryRoutes(mux *http.ServeMux) {
	handlers := &apiindustry.Handlers{
		Svc: a.industryService,
	}
	handlers.RegisterRoutes(mux)
}

func (a *DashboardAPI) RegisterSwaggerRoutes(mux *http.ServeMux) {
	swaggerHandlers := apiswagger.NewHandlers(a.workDir)
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
		LedgerDir:  a.ledgerDir,
		WorkDir:    a.workDir,
		Svc:        svc,
		Classifier: a.industryService.Classifier,
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

func (a *DashboardAPI) SetOrderManager(om *live.OrderManager) {
	a.orderMgr = om
}

func (a *DashboardAPI) SetStorageReporter(r apimetrics.StorageReporter) {
	a.storageReport = r
}

func (a *DashboardAPI) RegisterOrderRoutes(mux *http.ServeMux) {
	orderSvc := service.NewOrderService(a.orderMgr)
	handlers := &apiorders.Handlers{
		Svc: orderSvc,
	}
	handlers.RegisterRoutes(mux)
}

func (a *DashboardAPI) GetIndustryService() *service.IndustryService {
	return a.industryService
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
