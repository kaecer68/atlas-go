package monitoring

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
}

func NewDashboardAPI(workDir, ledgerDir string, metricsCollector *MetricsCollector) *DashboardAPI {
	provider := marketdata.NewCompositeMacroProvider(
		marketdata.NewYahooFinanceMacroProvider(),
		marketdata.NewTWSECapitalFlowProvider(filepath.Join(workDir, "data/state/capital_flow")),
		marketdata.NewExportStatisticsProvider(filepath.Join(workDir, "data/state/export")),
	)
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
	return &DashboardAPI{
		workDir:            workDir,
		ledgerDir:          ledgerDir,
		storeBackend:       os.Getenv("ATLAS_STORE_BACKEND"),
		sqlitePath:         os.Getenv("ATLAS_SQLITE_PATH"),
		baselinePath:       filepath.Join(workDir, "data/state/baseline_policy.json"),
		narrativeEngine:    narrative.NewNarrativeEngine(),
		macroIngestor:      narrative.NewMacroIngestor(provider, filepath.Join(workDir, "data/state/macro")),
		geoProvider:        geoProvider,
		taiwanGeoProvider:  taiwanGeoProvider,
		taiwanStressCalc:   narrative.NewTaiwanStressCalculator(geoProvider),
		reportGenerator:    narrative.NewReportGenerator(),
		industryService:    service.NewIndustryService(industry.DefaultClassification(), industry.NewSeasonalEngine(), industry.NewCycleTracker(), industry.NewLinkageAnalyzer(), industry.NewRiskMonitor()),
		metricsCollector:   metricsCollector,
		metricsHistory:     NewMetricsHistory(1000),
		healthManager:      portfolio.NewAgentHealthManager(),
		dataQualityChecker: NewDataQualityChecker(workDir, ledgerDir),
	}
}

func (a *DashboardAPI) SetEventBus(eventBus *eventbus.ChannelEventBus) {
	a.eventBus = eventBus
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
	metricsHandlers.RegisterRoutes(mux)

	systemSvc := service.NewSystemService(a.workDir, a.ledgerDir, a.baselinePath, outcomeStore)
	systemHandlers := apisystem.NewHandlers(systemSvc)
	systemHandlers.RegisterRoutes(mux)

	handlers := &apihealth.Handlers{}
	handlers.RegisterRoutes(mux)

	mux.HandleFunc("/api/health/data-integrity", apihealth.HandleDataIntegrity(a.workDir, a.ledgerDir))

	riskHandlers := apirisk.NewHandlers(a.ledgerDir)
	riskHandlers.RegisterRoutes(mux)

	taxHandlers := apitax.NewHandlers(a.ledgerDir)
	taxHandlers.RegisterRoutes(mux)

	paramHandlers := apiparameters.NewHandlers(filepath.Join(a.workDir, "configs/parameters.json"))
	paramHandlers.RegisterRoutes(mux)

	// Data channels endpoint — reuses system health's data_channel building logic.
	mux.HandleFunc("/api/dashboard/data-channels", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		health, err := systemSvc.LoadSystemHealth()
		if err != nil {
			shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load system health: %v", err))
			return
		}
		shared.WriteJSON(w, http.StatusOK, map[string]any{
			"channels":  health.DataChannels,
			"alerts":    []any{},
			"generated": time.Now().Format(time.RFC3339),
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
	svc := service.NewBacktestService()
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

func (a *DashboardAPI) RegisterOrderRoutes(mux *http.ServeMux) {
	orderSvc := service.NewOrderService(a.orderMgr)
	handlers := &apiorders.Handlers{
		Svc: orderSvc,
	}
	handlers.RegisterRoutes(mux)
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
