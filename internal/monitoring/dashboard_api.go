package monitoring

import (
	"log"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	apibacktest "github.com/kaecer68/atlas-go/internal/monitoring/api/backtest"
	apicontrol "github.com/kaecer68/atlas-go/internal/monitoring/api/control"
	apidata "github.com/kaecer68/atlas-go/internal/monitoring/api/data"
	apiexperiment "github.com/kaecer68/atlas-go/internal/monitoring/api/experiment"
	apihealth "github.com/kaecer68/atlas-go/internal/monitoring/api/health"
	apiindustry "github.com/kaecer68/atlas-go/internal/monitoring/api/industry"
	apilive "github.com/kaecer68/atlas-go/internal/monitoring/api/live"
	apimacro "github.com/kaecer68/atlas-go/internal/monitoring/api/macro"
	apimetrics "github.com/kaecer68/atlas-go/internal/monitoring/api/metrics"
	apinarrative "github.com/kaecer68/atlas-go/internal/monitoring/api/narrative"
	apiparameters "github.com/kaecer68/atlas-go/internal/monitoring/api/parameters"
	apipipeline "github.com/kaecer68/atlas-go/internal/monitoring/api/pipeline"
	apireport "github.com/kaecer68/atlas-go/internal/monitoring/api/report"
	apirisk "github.com/kaecer68/atlas-go/internal/monitoring/api/risk"
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
}

func NewDashboardAPI(workDir, ledgerDir string, metricsCollector *MetricsCollector) *DashboardAPI {
	provider := marketdata.NewCompositeMacroProvider(
		marketdata.NewYahooFinanceMacroProvider(),
		marketdata.NewTWSECapitalFlowProvider(filepath.Join(workDir, "data/state/capital_flow")),
		marketdata.NewTWSEBalanceProvider(filepath.Join(workDir, "data/state/margin")),
		marketdata.NewExportStatisticsProvider(filepath.Join(workDir, "data/state/export")),
		marketdata.NewTSMCRevenueProvider(filepath.Join(workDir, "data/state/tsmc_revenue")),
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

func (a *DashboardAPI) RegisterRoutes(mux *http.ServeMux) {
	pipelineSvc := service.NewPipelineService(a.workDir, a.ledgerDir)
	pipelineHandlers := apipipeline.NewHandlers(pipelineSvc)
	pipelineHandlers.RegisterRoutes(mux)

	reportSvc := service.NewReportService(a.workDir, a.ledgerDir)
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

	systemSvc := service.NewSystemService(a.workDir, a.ledgerDir, a.baselinePath)
	systemHandlers := apisystem.NewHandlers(systemSvc)
	systemHandlers.RegisterRoutes(mux)

	handlers := &apihealth.Handlers{}
	handlers.RegisterRoutes(mux)

	dataHandlers := apidata.NewHandlers(a.workDir, a.pool, a.macroIngestor, a.geoProvider, a.taiwanGeoProvider, a.janusEngine, &channelHealthAdapter{store: NewChannelHealthStoreWithPool(filepath.Join(a.workDir, "data/state"), a.pool)})
	dataHandlers.RegisterRoutes(mux)

	riskHandlers := apirisk.NewHandlers(a.ledgerDir)
	riskHandlers.RegisterRoutes(mux)

	// TODO: marketdata API handlers — NewHandlers not yet implemented
	// marketdataHandlers.RegisterRoutes(mux)

	taxHandlers := apitax.NewHandlers()
	taxHandlers.RegisterRoutes(mux)

	paramHandlers := apiparameters.NewHandlers(filepath.Join(a.workDir, "configs/parameters.json"))
	paramHandlers.RegisterRoutes(mux)
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
		Svc: svc,
	}
	handlers.RegisterRoutes(mux)
}

func (a *DashboardAPI) RegisterControlRoutes(mux *http.ServeMux) {
	svc := service.NewControlService(a.workDir, a.ledgerDir, a.healthManager)
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

func (a *DashboardAPI) RegisterPhase3Routes(mux *http.ServeMux) {
}

func (a *DashboardAPI) RegisterLiveRoutes(mux *http.ServeMux) {
	svc := service.NewLiveService(a.workDir, a.ledgerDir)
	handlers := &apilive.Handlers{
		LedgerDir: a.ledgerDir,
		WorkDir:   a.workDir,
		Svc:       svc,
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
	handlers := apibacktest.NewHandlers(svc)
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

func (a *DashboardAPI) RegisterTaskExecRoutes(mux *http.ServeMux) {
	if a.taskManager == nil {
		log.Printf("[TaskExec] skipping route registration: taskManager is nil")
		return
	}
	log.Printf("[TaskExec] registering task execution routes")
	handlers := apitaskexec.NewHandlers(a.taskManager)
	handlers.RegisterRoutes(mux)
}

type channelHealthAdapter struct {
	mu    sync.Mutex
	store *ChannelHealthStore
}

func (a *channelHealthAdapter) Record(channelID, status, errMsg string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.store.Record(channelID, status, errMsg)
}

func (a *channelHealthAdapter) Get(channelID string) *apidata.ChannelHealthRecord {
	a.mu.Lock()
	defer a.mu.Unlock()
	rec := a.store.Get(channelID)
	if rec == nil {
		return nil
	}
	return &apidata.ChannelHealthRecord{
		Status:        rec.Status,
		LastFetchAt:   rec.LastFetchAt,
		LastError:     rec.LastError,
		LastSuccessAt: rec.LastSuccessAt,
	}
}

func (a *channelHealthAdapter) Alerts() []apidata.ChannelAlert {
	a.mu.Lock()
	defer a.mu.Unlock()
	alerts := a.store.Alerts()
	result := make([]apidata.ChannelAlert, len(alerts))
	for i, al := range alerts {
		result[i] = apidata.ChannelAlert{
			ChannelID: al.ChannelID,
			Status:    al.Status,
			Error:     al.Error,
			FetchAt:   al.FetchAt,
		}
	}
	return result
}

func (a *channelHealthAdapter) SyncAllToDB() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.store.SyncAllToDB()
}
