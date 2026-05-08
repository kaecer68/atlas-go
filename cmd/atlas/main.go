package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/kaecer68/atlas-go/internal/bootstrap"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/live"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/repository"
	"github.com/kaecer68/atlas-go/internal/risk"
)

type routeRegistrar interface {
	RegisterRoutes(mux *http.ServeMux)
	RegisterSwaggerRoutes(mux *http.ServeMux)
	RegisterNarrativeRoutes(mux *http.ServeMux)
	RegisterControlRoutes(mux *http.ServeMux)
	RegisterMacroRoutes(mux *http.ServeMux)
	RegisterExperimentRoutes(mux *http.ServeMux)
	RegisterLiveRoutes(mux *http.ServeMux)
}

// registerCommonDashboardRoutes registers the shared set of dashboard API routes
// that are common between apiMode and liveMode. Backtest routes are only registered
// in apiMode, and swagger routes are conditional on the swaggerMode flag.
func registerCommonDashboardRoutes(
	dashboard routeRegistrar,
	mux *http.ServeMux,
	swaggerMode bool,
	includeBacktest bool,
) {
	dashboard.RegisterNarrativeRoutes(mux)
	dashboard.RegisterControlRoutes(mux)
	dashboard.RegisterMacroRoutes(mux)
	dashboard.RegisterExperimentRoutes(mux)
	if d, ok := dashboard.(*monitoring.DashboardAPI); ok {
		if includeBacktest {
			d.RegisterBacktestRoutes(mux)
		}
		if swaggerMode {
			d.RegisterSwaggerRoutes(mux)
		}
		d.RegisterIndustryRoutes(mux)
		d.RegisterLiveRoutes(mux)
	}
}

type appDeps struct {
	loadConfig      func() config.Config
	newDashboardAPI func(string, string, *monitoring.MetricsCollector) routeRegistrar
	listenAndServe  func(*http.Server) error
	shutdown        chan struct{}
}

func defaultAppDeps() appDeps {
	return appDeps{
		loadConfig: config.Load,
		newDashboardAPI: func(workDir, ledgerDir string, collector *monitoring.MetricsCollector) routeRegistrar {
			return monitoring.NewDashboardAPI(workDir, ledgerDir, collector)
		},
		listenAndServe: func(srv *http.Server) error { return srv.ListenAndServe() },
		shutdown:       make(chan struct{}),
	}
}

func main() {
	if err := run(os.Args[1:], defaultAppDeps()); err != nil {
		log.Fatalf("%v", err)
	}
}

func run(args []string, deps appDeps) error {
	flags := flag.NewFlagSet("atlas", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	apiMode := flags.Bool("api", false, "start dashboard api server")
	apiAddr := flags.String("addr", ":8080", "dashboard api listen address")
	swaggerMode := flags.Bool("swagger", false, "enable swagger docs endpoints")
	brokerMode := flags.String("broker-mode", "", "override broker mode: dry-run|paper|live")
	brokerAdapter := flags.String("broker-adapter", "", "override broker adapter: guarded|mock|http")
	brokerSigner := flags.String("broker-signer", "", "override broker signer: placeholder|hmac-sha256")
	brokerKeyID := flags.String("broker-key-id", "", "override broker key id for signer key rotation")
	brokerRetryStatusCodes := flags.String("broker-retry-status-codes", "", "override broker retry status codes csv, e.g. 408,429,503")
	brokerMaxRetries := flags.Int("broker-max-retries", -1, "override broker max retries (>=0)")
	brokerMaxClockSkew := flags.Int("broker-max-clock-skew-sec", -1, "override broker max clock skew seconds (>=0, 0 disables check)")
	brokerNonceTTL := flags.Int("broker-nonce-ttl-sec", -1, "override broker nonce replay ttl seconds (>=1)")
	brokerNonceStore := flags.String("broker-nonce-store", "", "override nonce replay store: memory|file|redis")
	brokerNonceStorePath := flags.String("broker-nonce-store-path", "", "override nonce replay file store path (required when store=file)")
	brokerNonceRedisURL := flags.String("broker-nonce-redis-url", "", "override nonce replay redis url (required when store=redis)")
	brokerNonceRedisKeyPrefix := flags.String("broker-nonce-redis-key-prefix", "", "override nonce replay redis key prefix")
	allowLiveBroker := flags.Bool("allow-live-broker", false, "allow live broker mode (default false)")
	allowHTTPBroker := flags.Bool("allow-http-broker", false, "allow http broker adapter in live mode (default false)")
	allowRealSigner := flags.Bool("allow-real-signer", false, "allow non-placeholder signer for http broker adapter")
	liveMode := flags.Bool("live", false, "start live trading orchestrator")
	logFormat := flags.String("log-format", "text", "log format: text or json")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	cfg := deps.loadConfig()
	logging.Init(*logFormat, slog.LevelInfo)
	if err := bootstrap.ApplyBrokerConfig(&cfg, bootstrap.BrokerOverrides{
		Mode:                *brokerMode,
		Adapter:             *brokerAdapter,
		Signer:              *brokerSigner,
		KeyID:               *brokerKeyID,
		RetryStatusCodes:    *brokerRetryStatusCodes,
		MaxRetries:          *brokerMaxRetries,
		MaxClockSkewSec:     *brokerMaxClockSkew,
		NonceTTLSec:         *brokerNonceTTL,
		NonceStore:          *brokerNonceStore,
		NonceStorePath:      *brokerNonceStorePath,
		NonceRedisURL:       *brokerNonceRedisURL,
		NonceRedisKeyPrefix: *brokerNonceRedisKeyPrefix,
		AllowLiveBroker:     *allowLiveBroker,
		AllowHTTPBroker:     *allowHTTPBroker,
		AllowRealSigner:     *allowRealSigner,
	}); err != nil {
		return err
	}

	// Initialize runtime using bootstrap package
	rt, err := bootstrap.InitRuntime(context.Background(), bootstrap.Config{
		WorkDir:   cfg.WorkDir,
		LedgerDir: cfg.LedgerDir,
	})
	if err != nil {
		return fmt.Errorf("initialize runtime: %w", err)
	}
	defer rt.Close()

	collector := rt.MetricsCollector
	pool := rt.Pool
	alertStore := rt.Stores.AlertStore
	repo := rt.Repository
	taskManager := rt.TaskManager

	if *apiMode {
		mux := http.NewServeMux()
		healthStore, err := portfolio.NewAgentHealthStore(filepath.Join(cfg.WorkDir, "data/state"))
		if err != nil {
			log.Printf("[AgentHealth] failed to create health store: %v", err)
		}
		paramsCfg, err := config.LoadParametersConfig(cfg.ParametersConfigPath)
		if err != nil {
			log.Printf("[Parameters] failed to load parameters config: %v", err)
		}
		runtimeParams := portfolio.ToRuntimeParameters(paramsCfg)
		dashboard := deps.newDashboardAPI(cfg.WorkDir, cfg.LedgerDir, collector)
		if d, ok := dashboard.(*monitoring.DashboardAPI); ok {
			d.SetPool(pool)
			d.SetHealthManager(portfolio.NewAgentHealthManagerWithStore(portfolio.DefaultAgentHealthConfig(), healthStore).WithParameters(runtimeParams))
			janusEngine := janus.NewEngine()
			janusEngine.EnsureAllRegimes()
			d.SetJanusEngine(janusEngine)
			log.Printf("[JANUS] engine injected into dashboard API")
		}
		dashboard.RegisterRoutes(mux)

		if alertStore != nil {
			alertAPI := monitoring.NewAlertAPI(alertStore)
			alertAPI.RegisterRoutes(mux)
		}

		if repo != nil {
			if d, ok := dashboard.(*monitoring.DashboardAPI); ok {
				d.SetRepository(repo)
				log.Printf("[Repository] injected into dashboard API")
			}
		}

		if taskManager != nil {
			if d, ok := dashboard.(*monitoring.DashboardAPI); ok {
				d.SetTaskManager(taskManager)
				d.RegisterTaskExecRoutes(mux)
				log.Printf("[TaskExec] injected into dashboard API")
			}
		}

		mux.HandleFunc("/admin/reload-config", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			cfg, err := config.ReloadEngineConfig()
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to reload config: %v", err), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"status":"ok","version":"%s"}`+"\n", cfg.Version)
		})
		mux.HandleFunc("/metrics", monitoring.PrometheusHandler(collector))
		monitor := monitoring.NewMonitor()
		if alertStore != nil {
			monitor.SetAlertStore(alertStore)
		}
		sysMetrics := monitoring.NewSystemMetrics(collector, monitor)
		sysCtx, sysCancel := context.WithCancel(context.Background())
		go sysMetrics.Start(sysCtx)

		// Periodic metrics snapshot save
		if repo != nil {
			go func() {
				ticker := time.NewTicker(60 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-sysCtx.Done():
						return
					case <-ticker.C:
						snap := collector.GetMetricsSnapshot()
						repoSnap := repository.MetricsSnapshot{
							ScreeningTotal:     snap.ScreeningTotal,
							ScreeningPassed:    snap.ScreeningPassed,
							ScreeningRate:      snap.ScreeningRate,
							AlertsTriggered:    snap.AlertsTriggered,
							AlertsAcknowledged: snap.AlertsAcknowledged,
							AlertsByType:       snap.AlertsByType,
							Timestamp:          snap.Timestamp,
						}
						if err := repo.SaveSnapshot(sysCtx, &repoSnap); err != nil {
							log.Printf("[Metrics] snapshot save failed: %v", err)
						}
					}
				}
			}()
		}
		registerCommonDashboardRoutes(dashboard, mux, *swaggerMode, true)

		mux.Handle("/", http.FileServer(http.Dir(filepath.Join(cfg.WorkDir, "web/static"))))
		log.Printf("dashboard api listening on %s", *apiAddr)
		bootstrap.StartChannelHealthSyncLoop(sysCtx, cfg.WorkDir, pool)
		bootstrap.StartAutoBackfill(sysCtx, cfg.WorkDir)
		bootstrap.StartAutoCapitalFlowFetch(sysCtx, cfg.WorkDir)
		bootstrap.StartAutoBacktestLoop(sysCtx, cfg)

		srv := &http.Server{Addr: *apiAddr, Handler: mux}
		srvErr := make(chan error, 1)
		go func() {
			if err := deps.listenAndServe(srv); err != nil && err != http.ErrServerClosed {
				srvErr <- fmt.Errorf("dashboard api server failed: %w", err)
			}
		}()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-sigCh:
			log.Printf("received signal, shutting down api server...")
		case err := <-srvErr:
			sysCancel()
			return err
		case <-deps.shutdown:
			log.Printf("shutdown signal received, shutting down api server...")
		}

		sysCancel()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("api server graceful shutdown failed: %v", err)
		} else {
			log.Printf("api server stopped")
		}
		return nil
	}

	if *liveMode {
		return runLiveTrading(cfg, deps, collector, repo)
	}
	return runSimulation(cfg, collector, repo)
}

func runSimulation(cfg config.Config, collector *monitoring.MetricsCollector, repo *repository.DualWriteRepository) error {
	system := orchestrator.NewProductionSystem(cfg)
	if collector != nil {
		system.WithMetricsCollector(collector)
	}
	if repo != nil {
		system.SetRepository(repo)
		log.Printf("[Repository] injected into simulation system")
	}

	capitalCfg := domain.DefaultCapitalPhaseConfig()
	capitalCfg.PhaseStartDate = time.Now().Add(-30 * 24 * time.Hour)
	controller := risk.NewCapitalPhaseController(capitalCfg)
	allocator := portfolio.NewCapitalAllocator()
	workflow, err := risk.NewApprovalWorkflow("data/state/approvals")
	if err != nil {
		return fmt.Errorf("create approval workflow: %w", err)
	}
	system.WithCapitalManagement(controller, allocator, workflow)

	result, err := system.RunDailySimulation(time.Now())
	if err != nil {
		return fmt.Errorf("simulation failed: %w", err)
	}

	registry := system.Registry()
	session := system.Session()

	fmt.Printf("atlas-go daily simulation\n")
	fmt.Printf("provider: %s\n", cfg.MarketDataProvider)
	fmt.Printf("broker_mode: %s\n", cfg.BrokerMode)
	fmt.Printf("broker_adapter: %s\n", cfg.BrokerAdapter)
	fmt.Printf("broker_signer: %s\n", cfg.BrokerSigner)
	fmt.Printf("broker_key_id: %s\n", cfg.BrokerKeyID)
	fmt.Printf("broker_retry_status_codes: %v\n", cfg.BrokerHTTPRetryStatusCodes)
	fmt.Printf("broker_max_clock_skew_sec: %d\n", cfg.BrokerMaxClockSkewS)
	fmt.Printf("broker_nonce_ttl_sec: %d\n", cfg.BrokerNonceTTLS)
	fmt.Printf("broker_nonce_store: %s\n", cfg.BrokerNonceStore)
	fmt.Printf("broker_nonce_store_path: %s\n", cfg.BrokerNonceStorePath)
	fmt.Printf("broker_nonce_redis_url: %s\n", cfg.BrokerNonceRedisURL)
	fmt.Printf("broker_nonce_redis_key_prefix: %s\n", cfg.BrokerNonceRedisKeyPrefix)
	fmt.Printf("broker_max_retries: %d\n", cfg.BrokerMaxRetries)
	fmt.Printf("session: %s\n", session.ID)
	fmt.Printf("agents: %d\n", len(registry.Agents))
	fmt.Printf("regime: %s\n", result.Regime)
	fmt.Printf("orders: %d\n", len(result.Orders))
	fmt.Printf("cash: %.2f\n", result.EndingCash)
	fmt.Printf("positions: %d\n", len(result.Positions))

	candidate, err := system.NextExperimentCandidate()
	if err != nil {
		return fmt.Errorf("candidate selection failed: %w", err)
	}
	if candidate != nil {
		fmt.Printf("next_experiment_agent: %s\n", candidate.Agent.ID)
		fmt.Printf("next_experiment_skill: %s\n", candidate.Agent.Skill)
		fmt.Printf("baseline_sharpe_like: %.6f\n", candidate.Scorecard.SharpeLike)
	}

	if err := system.RecordSessionSummary(result, candidate); err != nil {
		return fmt.Errorf("record session summary failed: %w", err)
	}

	return nil
}

func runLiveTrading(cfg config.Config, deps appDeps, collector *monitoring.MetricsCollector, repo *repository.DualWriteRepository) error {
	system := orchestrator.NewProductionSystem(cfg)
	if collector != nil {
		system.WithMetricsCollector(collector)
	}
	if repo != nil {
		system.SetRepository(repo)
		log.Printf("[Repository] injected into live trading system")
	}

	stateStore := livestore.NewStateStore("data/state/live")
	eventBus := live.NewChannelEventBus(64)
	provider := marketdata.NewMockProvider()

	liveCfg := live.DefaultOrchestratorConfig()
	liveCfg.BrokerMode = cfg.BrokerMode
	liveCfg.BrokerAdapter = cfg.BrokerAdapter
	liveCfg.BrokerSigner = cfg.BrokerSigner
	liveCfg.BrokerKeyID = cfg.BrokerKeyID
	liveCfg.BrokerMaxRetries = cfg.BrokerMaxRetries
	liveCfg.BrokerHTTPTimeoutS = cfg.BrokerHTTPTimeoutS
	liveCfg.BrokerHTTPAttempts = cfg.BrokerHTTPAttempts
	liveCfg.BrokerHTTPRetryStatusCodes = cfg.BrokerHTTPRetryStatusCodes
	liveCfg.BrokerMaxClockSkewS = cfg.BrokerMaxClockSkewS
	liveCfg.BrokerNonceTTLS = cfg.BrokerNonceTTLS
	liveCfg.BrokerNonceStore = cfg.BrokerNonceStore
	liveCfg.BrokerNonceStorePath = cfg.BrokerNonceStorePath
	liveCfg.BrokerNonceRedisURL = cfg.BrokerNonceRedisURL
	liveCfg.BrokerNonceRedisKeyPrefix = cfg.BrokerNonceRedisKeyPrefix

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	adapter := orchestrator.NewAdapterProducer(provider, system)

	o := live.NewOrchestrator(
		ctx,
		stateStore,
		eventBus,
		provider,
		system.Registry(),
		adapter,
		liveCfg,
	)

	// Metrics collector for live trading observability
	monitor := monitoring.NewMonitor()
	tradingMetrics := monitoring.NewTradingMetrics(collector, monitor)
	o.SetTradingMetrics(tradingMetrics)

	// Alert rule engine for live trading safety
	ruleEngine := monitoring.NewRuleEngine(monitor)
	for _, rule := range monitoring.LiveTradingRules() {
		ruleEngine.RegisterRule(rule)
	}
	go ruleEngine.Start(ctx, stateStore)

	// Start dashboard API server for live status endpoint
	mux := http.NewServeMux()
	dashboard := deps.newDashboardAPI(cfg.WorkDir, cfg.LedgerDir, collector)
	dashboard.RegisterRoutes(mux)
	alertStore, err := monitoring.NewAlertStore(filepath.Join(cfg.WorkDir, "data/state/alerts"))
	if err != nil {
		log.Printf("[Alerts] failed to create alert store: %v", err)
	} else {
		alertAPI := monitoring.NewAlertAPI(alertStore)
		alertAPI.RegisterRoutes(mux)
		monitor.SetAlertStore(alertStore)
	}
	registerCommonDashboardRoutes(dashboard, mux, true, false)

	mux.Handle("/", http.FileServer(http.Dir(filepath.Join(cfg.WorkDir, "web/static"))))
	apiAddr := ":8080"
	srv := &http.Server{Addr: apiAddr, Handler: mux}
	go func() {
		log.Printf("dashboard api listening on %s", apiAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("dashboard api server failed: %v", err)
		}
	}()

	log.Printf("starting live trading orchestrator (broker_mode=%s)", liveCfg.BrokerMode)
	if err := o.Start(); err != nil {
		return fmt.Errorf("start live orchestrator: %w", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	log.Printf("live trading orchestrator running; press Ctrl+C to stop")
	select {
	case <-sigCh:
	case <-ctx.Done():
	}
	log.Println("shutting down...")
	ctx2, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx2); err != nil {
		log.Printf("dashboard api graceful shutdown failed: %v", err)
	}
	if err := o.Stop(); err != nil {
		return fmt.Errorf("stop live orchestrator: %w", err)
	}
	log.Println("live trading orchestrator stopped")
	return nil
}
