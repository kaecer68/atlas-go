package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/live"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/portfolio"
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

type appDeps struct {
	loadConfig                  func() config.Config
	newDashboardAPI             func(string, string, *monitoring.MetricsCollector) routeRegistrar
	listenAndServe              func(*http.Server) error
	shutdown                    chan struct{}
	runAutoCapitalFlowOnStartup func(string)
	runAutoBackfillOnStartup    func(string)
}

func defaultAppDeps() appDeps {
	return appDeps{
		loadConfig: config.Load,
		newDashboardAPI: func(workDir, ledgerDir string, collector *monitoring.MetricsCollector) routeRegistrar {
			return monitoring.NewDashboardAPI(workDir, ledgerDir, collector)
		},
		listenAndServe:              func(srv *http.Server) error { return srv.ListenAndServe() },
		shutdown:                    make(chan struct{}),
		runAutoCapitalFlowOnStartup: runAutoCapitalFlowFetchOnStartup,
		runAutoBackfillOnStartup:    runAutoBackfillOnStartup,
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
	if *brokerMode != "" {
		cfg.BrokerMode = *brokerMode
	}
	if *brokerAdapter != "" {
		cfg.BrokerAdapter = *brokerAdapter
	}
	if *brokerSigner != "" {
		cfg.BrokerSigner = *brokerSigner
	}
	if *brokerKeyID != "" {
		cfg.BrokerKeyID = *brokerKeyID
	}
	if *brokerRetryStatusCodes != "" {
		cfg.BrokerHTTPRetryStatusCodes = parseStatusCodeCSV(*brokerRetryStatusCodes, cfg.BrokerHTTPRetryStatusCodes)
	}
	if *brokerMaxRetries >= 0 {
		cfg.BrokerMaxRetries = *brokerMaxRetries
	}
	if *brokerMaxClockSkew >= 0 {
		cfg.BrokerMaxClockSkewS = *brokerMaxClockSkew
	}
	if *brokerNonceTTL >= 0 {
		cfg.BrokerNonceTTLS = *brokerNonceTTL
	}
	if *brokerNonceStore != "" {
		cfg.BrokerNonceStore = *brokerNonceStore
	}
	if *brokerNonceStorePath != "" {
		cfg.BrokerNonceStorePath = *brokerNonceStorePath
	}
	if *brokerNonceRedisURL != "" {
		cfg.BrokerNonceRedisURL = *brokerNonceRedisURL
	}
	if *brokerNonceRedisKeyPrefix != "" {
		cfg.BrokerNonceRedisKeyPrefix = *brokerNonceRedisKeyPrefix
	}
	if err := validateBrokerRuntimeConfig(&cfg, *allowLiveBroker, *allowHTTPBroker, *allowRealSigner); err != nil {
		return err
	}

	collector := monitoring.NewMetricsCollector()

	if *apiMode {
		var pool *pgxpool.Pool
		if dsn := os.Getenv("DATABASE_URL"); dsn != "" {
			migrationsPath := filepath.Join(cfg.WorkDir, "sql/migrations")
			if _, err := os.Stat(migrationsPath); err == nil {
				var dbErr error
				pool, dbErr = db.Init(context.Background(), dsn, migrationsPath)
				if dbErr != nil {
					log.Printf("[DB] failed to initialize database: %v", dbErr)
				} else {
					log.Printf("[DB] connected and migrations applied")
					defer pool.Close()
				}
			}
		}

		mux := http.NewServeMux()
		healthStore, err := portfolio.NewAgentHealthStore(filepath.Join(cfg.WorkDir, "data/state"))
		if err != nil {
			log.Printf("[AgentHealth] failed to create health store: %v", err)
		}
		dashboard := deps.newDashboardAPI(cfg.WorkDir, cfg.LedgerDir, collector)
		if d, ok := dashboard.(*monitoring.DashboardAPI); ok {
			d.SetPool(pool)
			d.SetHealthManager(portfolio.NewAgentHealthManagerWithStore(portfolio.DefaultAgentHealthConfig(), healthStore))
			janusEngine := janus.NewEngine()
			janusEngine.EnsureAllRegimes()
			d.SetJanusEngine(janusEngine)
			log.Printf("[JANUS] engine injected into dashboard API")
		}
		dashboard.RegisterRoutes(mux)

		alertStore, err := monitoring.NewAlertStore(filepath.Join(cfg.WorkDir, "data/state/alerts"))
		if err != nil {
			log.Printf("[Alerts] failed to create alert store: %v", err)
		} else {
			alertAPI := monitoring.NewAlertAPI(alertStore)
			alertAPI.RegisterRoutes(mux)
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
		dashboard.RegisterNarrativeRoutes(mux)
		dashboard.RegisterControlRoutes(mux)
		dashboard.RegisterMacroRoutes(mux)
		dashboard.RegisterExperimentRoutes(mux)
		if d, ok := dashboard.(*monitoring.DashboardAPI); ok {
			d.RegisterPhase3Routes(mux)
			d.RegisterBacktestRoutes(mux)
			d.RegisterIndustryRoutes(mux)
			d.RegisterLiveRoutes(mux)
		}
		if *swaggerMode {
			dashboard.RegisterSwaggerRoutes(mux)
		}
		mux.Handle("/", http.FileServer(http.Dir(filepath.Join(cfg.WorkDir, "web/static"))))
		log.Printf("dashboard api listening on %s", *apiAddr)
		// Trigger automatic backfill if replay data has gaps.
		deps.runAutoBackfillOnStartup(cfg.WorkDir)
		deps.runAutoCapitalFlowOnStartup(cfg.WorkDir)

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
		return runLiveTrading(cfg, deps, collector)
	}
	return runSimulation(cfg, collector)
}

func validateBrokerRuntimeConfig(cfg *config.Config, allowLiveBroker bool, allowHTTPBroker bool, allowRealSigner bool) error {
	normalizeBrokerStrings(cfg)
	if err := validateBrokerEnums(cfg); err != nil {
		return err
	}
	if err := validateBrokerLiveMode(cfg, allowLiveBroker, allowHTTPBroker, allowRealSigner); err != nil {
		return err
	}
	if err := validateBrokerRetryConfig(cfg); err != nil {
		return err
	}
	if err := validateBrokerNonceConfig(cfg); err != nil {
		return err
	}
	return nil
}

func normalizeBrokerStrings(cfg *config.Config) {
	cfg.BrokerMode = strings.TrimSpace(strings.ToLower(cfg.BrokerMode))
	if cfg.BrokerMode == "" {
		cfg.BrokerMode = "dry-run"
	}
	cfg.BrokerAdapter = strings.TrimSpace(strings.ToLower(cfg.BrokerAdapter))
	if cfg.BrokerAdapter == "" {
		cfg.BrokerAdapter = "guarded"
	}
	cfg.BrokerSigner = strings.TrimSpace(strings.ToLower(cfg.BrokerSigner))
	if cfg.BrokerSigner == "" {
		cfg.BrokerSigner = "placeholder"
	}
	cfg.BrokerKeyID = strings.TrimSpace(cfg.BrokerKeyID)
}

func validateBrokerEnums(cfg *config.Config) error {
	if cfg.BrokerAdapter != "guarded" && cfg.BrokerAdapter != "mock" && cfg.BrokerAdapter != "http" {
		return fmt.Errorf("unsupported broker adapter %q (allowed: guarded, mock, http)", cfg.BrokerAdapter)
	}
	if cfg.BrokerSigner != "placeholder" && cfg.BrokerSigner != "hmac-sha256" {
		return fmt.Errorf("unsupported broker signer %q (allowed: placeholder, hmac-sha256)", cfg.BrokerSigner)
	}
	return nil
}

func validateBrokerLiveMode(cfg *config.Config, allowLiveBroker bool, allowHTTPBroker bool, allowRealSigner bool) error {
	switch cfg.BrokerMode {
	case "dry-run", "paper":
		return nil
	case "live":
		if !allowLiveBroker {
			return fmt.Errorf("broker mode %q is disabled by default; pass -allow-live-broker to enable", cfg.BrokerMode)
		}
		if cfg.BrokerAdapter == "http" && !allowHTTPBroker {
			return fmt.Errorf("broker adapter %q is disabled by default in live mode; pass -allow-http-broker to enable", cfg.BrokerAdapter)
		}
		if cfg.BrokerAdapter == "http" && cfg.BrokerSigner != "placeholder" && !allowRealSigner {
			return fmt.Errorf("broker signer %q is disabled by default for http adapter; pass -allow-real-signer to enable", cfg.BrokerSigner)
		}
		if cfg.BrokerAdapter == "http" && cfg.BrokerSigner != "placeholder" && cfg.BrokerKeyID == "" {
			return fmt.Errorf("broker key id is required when using signer %q with http adapter", cfg.BrokerSigner)
		}
		return nil
	default:
		return fmt.Errorf("unsupported broker mode %q (allowed: dry-run, paper, live)", cfg.BrokerMode)
	}
}

func validateBrokerRetryConfig(cfg *config.Config) error {
	if cfg.BrokerMaxRetries < 0 {
		return fmt.Errorf("broker max retries must be >= 0, got %d", cfg.BrokerMaxRetries)
	}
	if len(cfg.BrokerHTTPRetryStatusCodes) == 0 {
		cfg.BrokerHTTPRetryStatusCodes = []int{408, 425, 429, 500, 502, 503, 504}
	}
	for _, code := range cfg.BrokerHTTPRetryStatusCodes {
		if code < 400 || code > 599 {
			return fmt.Errorf("broker retry status code must be 4xx/5xx, got %d", code)
		}
	}
	if cfg.BrokerMaxClockSkewS < 0 {
		return fmt.Errorf("broker max clock skew must be >= 0, got %d", cfg.BrokerMaxClockSkewS)
	}
	return nil
}

func validateBrokerNonceConfig(cfg *config.Config) error {
	if cfg.BrokerNonceTTLS == 0 {
		cfg.BrokerNonceTTLS = 300
	}
	if cfg.BrokerNonceTTLS < 0 {
		return fmt.Errorf("broker nonce ttl must be >= 0, got %d", cfg.BrokerNonceTTLS)
	}
	cfg.BrokerNonceStore = strings.TrimSpace(strings.ToLower(cfg.BrokerNonceStore))
	if cfg.BrokerNonceStore == "" {
		cfg.BrokerNonceStore = "memory"
	}
	if cfg.BrokerNonceStore != "memory" && cfg.BrokerNonceStore != "file" && cfg.BrokerNonceStore != "redis" {
		return fmt.Errorf("unsupported broker nonce store %q (allowed: memory, file, redis)", cfg.BrokerNonceStore)
	}
	cfg.BrokerNonceStorePath = strings.TrimSpace(cfg.BrokerNonceStorePath)
	defaultedNonceStorePath := false
	if cfg.BrokerNonceStore == "file" && cfg.BrokerNonceStorePath == "" {
		ledgerDir := strings.TrimSpace(cfg.LedgerDir)
		if ledgerDir == "" {
			ledgerDir = "data/state"
		}
		cfg.BrokerNonceStorePath = filepath.Join(ledgerDir, "broker-nonce-replay.json")
		defaultedNonceStorePath = true
	}
	if cfg.BrokerNonceStore == "file" && !defaultedNonceStorePath && !filepath.IsAbs(cfg.BrokerNonceStorePath) {
		ledgerDir := strings.TrimSpace(cfg.LedgerDir)
		if ledgerDir == "" {
			ledgerDir = "data/state"
		}
		cfg.BrokerNonceStorePath = filepath.Join(ledgerDir, cfg.BrokerNonceStorePath)
	}
	cfg.BrokerNonceRedisURL = strings.TrimSpace(cfg.BrokerNonceRedisURL)
	cfg.BrokerNonceRedisKeyPrefix = strings.TrimSpace(cfg.BrokerNonceRedisKeyPrefix)
	if cfg.BrokerNonceRedisKeyPrefix == "" {
		cfg.BrokerNonceRedisKeyPrefix = "atlas:nonce:"
	}
	if cfg.BrokerNonceStore == "redis" && cfg.BrokerNonceRedisURL == "" {
		return fmt.Errorf("broker nonce redis url is required when broker nonce store is redis")
	}
	return nil
}

func parseStatusCodeCSV(raw string, fallback []int) []int {
	parts := strings.Split(raw, ",")
	parsed := make([]int, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		parsed = append(parsed, n)
	}
	if len(parsed) == 0 {
		return append([]int(nil), fallback...)
	}
	return parsed
}

func runSimulation(cfg config.Config, collector *monitoring.MetricsCollector) error {
	system := orchestrator.NewProductionSystem(cfg)
	if collector != nil {
		system.WithMetricsCollector(collector)
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

func runLiveTrading(cfg config.Config, deps appDeps, collector *monitoring.MetricsCollector) error {
	system := orchestrator.NewProductionSystem(cfg)
	if collector != nil {
		system.WithMetricsCollector(collector)
	}

	stateStore := live.NewStateStore("data/state/live")
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

	o := live.NewOrchestrator(
		ctx,
		stateStore,
		eventBus,
		provider,
		system.Registry(),
		system,
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
	dashboard.RegisterNarrativeRoutes(mux)
	dashboard.RegisterControlRoutes(mux)
	dashboard.RegisterMacroRoutes(mux)
	dashboard.RegisterExperimentRoutes(mux)
	if d, ok := dashboard.(*monitoring.DashboardAPI); ok {
		d.RegisterSwaggerRoutes(mux)
		d.RegisterPhase3Routes(mux)
		d.RegisterLiveRoutes(mux)
		d.RegisterIndustryRoutes(mux)
	}
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

// runAutoBackfillOnStartup checks for missing trading days in the replay CSV
// and automatically triggers FinMind backfill in a background goroutine.
func runAutoBackfillOnStartup(workDir string) {
	go func() {
		// Give the server a moment to start before doing I/O.
		time.Sleep(3 * time.Second)

		csvPath := filepath.Join(workDir, "data/replay/tw_extended_90days.csv")
		latestDate, err := getLatestReplayDate(csvPath)
		if err != nil {
			log.Printf("[AutoBackfill] cannot read replay csv: %v", err)
			return
		}

		now := time.Now()
		if tz, err := time.LoadLocation("Asia/Taipei"); err == nil {
			now = now.In(tz)
		}

		// Determine backfill end: yesterday before 15:30 TW, else today.
		end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		if now.Hour() < 15 || (now.Hour() == 15 && now.Minute() < 30) {
			end = end.AddDate(0, 0, -1)
		}

		start := latestDate.AddDate(0, 0, 1)
		// Fast-forward through weekends so we don't backfill Saturdays/Sundays.
		for start.Weekday() == time.Saturday || start.Weekday() == time.Sunday {
			start = start.AddDate(0, 0, 1)
		}
		for end.Weekday() == time.Saturday || end.Weekday() == time.Sunday {
			end = end.AddDate(0, 0, -1)
		}

		if start.After(end) {
			log.Printf("[AutoBackfill] no gap detected (latest=%s, target=%s)", latestDate.Format("2006-01-02"), end.Format("2006-01-02"))
			return
		}

		startStr := start.Format("2006-01-02")
		endStr := end.Format("2006-01-02")
		log.Printf("[AutoBackfill] detected gap %s ~ %s. Triggering backfill...", startStr, endStr)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		var cmd *exec.Cmd
		binaryPath := filepath.Join(workDir, "backfill-replay")
		if _, err := os.Stat(binaryPath); err == nil {
			cmd = exec.CommandContext(ctx, binaryPath, "-csv", csvPath, "-start", startStr, "-end", endStr)
		} else if _, err := exec.LookPath("go"); err == nil {
			cmd = exec.CommandContext(ctx, "go", "run", "./cmd/backfill-replay", "-csv", csvPath, "-start", startStr, "-end", endStr)
			cmd.Dir = workDir
		} else {
			log.Printf("[AutoBackfill] backfill-replay binary not found and go not in PATH; skipping auto-backfill")
			return
		}

		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("[AutoBackfill] backfill failed: %v\noutput: %s", err, string(out))
			return
		}
		log.Printf("[AutoBackfill] success:\n%s", string(out))
	}()
}

func getLatestReplayDate(csvPath string) (time.Time, error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	var latest time.Time
	_, _ = reader.Read() // skip header
	for {
		row, err := reader.Read()
		if err != nil {
			break
		}
		if len(row) == 0 {
			continue
		}
		d, err := time.Parse("2006-01-02", strings.TrimSpace(row[0]))
		if err != nil {
			continue
		}
		if d.After(latest) {
			latest = d
		}
	}
	if latest.IsZero() {
		return time.Time{}, fmt.Errorf("no valid dates found")
	}
	return latest, nil
}

// runAutoCapitalFlowFetchOnStartup fetches capital flow data from TWSE in a background
// goroutine after a short delay to allow the server to fully start.
func runAutoCapitalFlowFetchOnStartup(workDir string) {
	go func() {
		time.Sleep(5 * time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		capFlowProvider := marketdata.NewTWSECapitalFlowProvider(filepath.Join(workDir, "data/state/capital_flow"))
		_, err := capFlowProvider.FetchSnapshot(ctx)
		if err != nil {
			log.Printf("[AutoCapitalFlow] fetch failed: %v", err)
			return
		}
		log.Printf("[AutoCapitalFlow] fetch succeeded")
	}()
}
