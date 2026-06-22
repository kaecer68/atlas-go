package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/alerting"
	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/autobacktest"
	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/bootstrap"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/experiment"
	"github.com/kaecer68/atlas-go/internal/fubonproxy"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/live"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
	"github.com/kaecer68/atlas-go/internal/llm"
	llmAdapters "github.com/kaecer68/atlas-go/internal/llm/adapters"
	"github.com/kaecer68/atlas-go/internal/llm/capabilities"
	"github.com/kaecer68/atlas-go/internal/llm/clients"
	"github.com/kaecer68/atlas-go/internal/llm/schemas"
	"github.com/kaecer68/atlas-go/internal/llm_annotator"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	apievents "github.com/kaecer68/atlas-go/internal/monitoring/api/events"
	llmHealth "github.com/kaecer68/atlas-go/internal/monitoring/api/llm"
	apischeduler "github.com/kaecer68/atlas-go/internal/monitoring/api/scheduler"
	apishared "github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	apistrategies "github.com/kaecer68/atlas-go/internal/monitoring/api/strategies"
	"github.com/kaecer68/atlas-go/internal/monitoring/metrics"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/realtime"
	"github.com/kaecer68/atlas-go/internal/repository"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/scheduler"
	"github.com/kaecer68/atlas-go/internal/storage"
	"github.com/kaecer68/atlas-go/internal/strategy_techniques"
	"github.com/kaecer68/atlas-go/web"
)

// appDeps is the central dependency-injection struct for run().
// Construction (defaultAppDeps) lives in bootstrap_helpers.go.
type appDeps struct {
	loadConfig      func() config.Config
	newDashboardAPI func(string, string, *monitoring.MetricsCollector) *monitoring.DashboardAPI
	listenAndServe  func(*http.Server) error
	shutdown        chan struct{}
	dataFetcher     monitoring.DataFetcher // when non-nil, skips Gateway init and uses this fetcher
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
	allowRealtime := flags.Bool("allow-realtime", false, "enable real-time regime detection adapter (default false)")
	allowHTTPBroker := flags.Bool("allow-http-broker", false, "allow http broker adapter in live mode (default false)")
	allowRealSigner := flags.Bool("allow-real-signer", false, "allow non-placeholder signer for http broker adapter")
	liveMode := flags.Bool("live", false, "start live trading orchestrator")
	forceIntradayCycles := flags.Bool("force-intraday-cycles", false, "bypass market hours check for off-hours testing")
	logFormat := flags.String("log-format", "text", "log format: text or json")
	simulateMode := flags.Bool("simulate", false, "run one-shot daily simulation and exit (skip api server)")
	verboseMode := flags.Bool("verbose", false, "enable color-coded terminal trace output during simulation")
	dateOverride := flags.String("date", "", "override simulation session date (format: 2006-01-02)")
	checkIntegrity := flags.Bool("check-integrity", false, "check configs/parameters.json integrity and exit")
	buildUniverseMode := flags.String("build-universe", "", "run SmartUniverseBuilder pipeline: run|map|scrape|status")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	cfg := deps.loadConfig()
	logging.Init(*logFormat, slog.LevelInfo)

	if *checkIntegrity {
		paramsPath := config.GetParametersConfigPath()
		errs := config.CheckParamsIntegrity(paramsPath)
		if len(errs) > 0 {
			for _, err := range errs {
				logging.Error("integrity_check", "integrity_check: "+err.Error())
			}
			os.Exit(1)
		}
		logging.Info("integrity_check", fmt.Sprintf("integrity_check: %s is valid", paramsPath))
		os.Exit(0)
	}

	// Ensure PostgreSQL is reachable before we try to connect.
	// If DATABASE_URL is unset or postgres is already running, this is a no-op.
	// On failure, the app continues without DB (bootstrap handles graceful degradation).
	ensurePostgres()

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

	// Phase A3: Clean up stale gateway heartbeat alerts on startup.
	if alertStore != nil {
		cutoff := time.Now().Add(-24 * time.Hour)
		deleted, err := alertStore.DeleteWhere(func(r *domain.AlertRecord) bool {
			return r.Rule == "gateway" && r.Timestamp.Before(cutoff)
		})
		if err != nil {
			log.Printf("[AlertCleanup] cleanup failed: %v", err)
		} else if deleted > 0 {
			log.Printf("[AlertCleanup] deleted %d stale gateway heartbeats (>24h)", deleted)
		}

		// Auto-acknowledge existing gateway INFO alerts (noise — humans should never see them).
		acked, err := alertStore.AcknowledgeWhere(func(r *domain.AlertRecord) bool {
			return r.Rule == "gateway" && r.Severity == "INFO" && !r.Acknowledged
		}, "auto-handler-startup")
		if err != nil {
			log.Printf("[AlertCleanup] auto-acknowledge failed: %v", err)
		} else if acked > 0 {
			log.Printf("[AlertCleanup] auto-acknowledged %d gateway INFO alerts", acked)
		}
	}

	var janusEngine *janus.Engine

	// Handle --simulate mode: run one-shot daily simulation and exit
	if *simulateMode {
		return runSimulationMode(rt, cfg, *verboseMode, *dateOverride)
	}

	// Handle --build-universe: run SmartUniverseBuilder pipeline and exit.
	if *buildUniverseMode != "" {
		return runBuildUniverse(rt, cfg, *verboseMode, *dateOverride, *buildUniverseMode)
	}

	if *apiMode {
		// Pre-initialize janus engine for Gateway channel adapters.
		janusEngine = janus.NewEngine()
		janusEngine.EnsureAllRegimes()
		janusEngine.Update()

		// Initialize MaturityTracker for burn-in / calibrating / full-auto gating.
		maturityTracker, _ := domain.NewMaturityTracker(filepath.Join(cfg.WorkDir, "data/state/maturity_tracker.json"))
		if maturityTracker != nil {
			logging.Info("bootstrap", "maturity_tracker_ready",
				"maturity", string(maturityTracker.Current()),
				"days_since_start", maturityTracker.DaysSinceStart())
		}

		var lifecycleMgr *storage.LifecycleManager

		var stRegistry *strategy_techniques.Registry
		var stSeedsPath string

		// Start fubon-proxy process manager BEFORE Gateway adapter registration,
		// so the fubon TCP probe in RegisterChannelAdapters finds :8081 already running.
		if shouldStartFubonProxy(cfg.BrokerMode, cfg.FubonAPIKey) {
			fubonMgr := fubonproxy.NewManager(cfg.WorkDir)
			if err := fubonMgr.Start(context.Background()); err != nil {
				log.Printf("[FubonProxy] start warning (non-fatal): %v", err)
			} else {
				log.Printf("[FubonProxy] process manager started")
			}
			defer fubonMgr.Stop()
		}

		// Initialize Gateway BEFORE DashboardAPI so data providers use Gateway from the start.
		var gateway *apigateway.Gateway
		var gatewayFetcher monitoring.DataFetcher
		if deps.dataFetcher != nil {
			// Test override: skip real Gateway initialization, use injected fetcher.
			gatewayFetcher = deps.dataFetcher
			log.Printf("[Gateway] using injected data fetcher (test mode)")
		} else {
			gw, gwErr := apigateway.NewGateway(cfg.WorkDir, pool)
			if gwErr != nil {
				log.Printf("[Gateway] initialization failed: %v", gwErr)
			} else if err := apigateway.RegisterChannelAdapters(gw, cfg.WorkDir, cfg, janusEngine); err != nil {
				log.Printf("[Gateway] adapter registration failed: %v", err)
			} else {
				gateway = gw
				log.Printf("[Gateway] initialized with %d channels + adapters", len(gateway.ChannelIDs()))
				gatewayFetcher = func(ctx context.Context, channelID string) ([]byte, error) {
					result, err := gateway.Fetch(ctx, channelID)
					if err != nil {
						return nil, err
					}
					return result.Data, nil
				}
				log.Printf("[Gateway] data fetcher prepared for DashboardAPI")
			}
		}

		mux := http.NewServeMux()
		log.Printf("[Auth] API key authentication %s", map[bool]string{true: "ENABLED", false: "DISABLED (no ATLAS_API_KEY set)"}[os.Getenv("ATLAS_API_KEY") != ""])
		healthStore, err := portfolio.NewAgentHealthStore(filepath.Join(cfg.WorkDir, "data/state"))
		if err != nil {
			log.Printf("[AgentHealth] failed to create health store: %v", err)
		}
		paramsCfg, err := config.LoadParametersConfig(cfg.ParametersConfigPath)
		if err != nil {
			log.Printf("[Parameters] failed to load parameters config: %v", err)
		}
		runtimeParams := portfolio.ToRuntimeParameters(paramsCfg)
		var dashboard *monitoring.DashboardAPI
		if gatewayFetcher != nil {
			dashboard = monitoring.NewDashboardAPIWithGateway(cfg.WorkDir, cfg.LedgerDir, collector, gatewayFetcher)
		} else {
			dashboard = deps.newDashboardAPI(cfg.WorkDir, cfg.LedgerDir, collector)
		}
		dashboard.SetPool(pool)
		agentHealthMgr := portfolio.NewAgentHealthManagerWithStore(portfolio.DefaultAgentHealthConfig(), healthStore).WithParameters(runtimeParams)
		dashboard.SetHealthManager(agentHealthMgr)
		dwMgr := portfolio.NewDarwinianWeightManager(filepath.Join(cfg.WorkDir, "data/state/darwinian_weights.json"))
		autoRollback := scheduler.NewAutoRollback(nil, dwMgr, agentHealthMgr)
		healthMonitor := scheduler.NewSystemHealthMonitor(dwMgr, agentHealthMgr)
		baselineMgr := baseline.NewManager(cfg.BaselinePolicyPath)
		judge := experiment.NewJudge(ledger.NewStore(cfg.LedgerDir).(ledger.ExperimentStore), cfg.ReplayDataPath, cfg.BaselinePolicyPath)
		autoJudgePromoter := experiment.NewAutoJudgePromoter(judge, baselineMgr).
			WithMaturityTracker(maturityTracker).
			WithPromotionRecorder(autoRollback)
		dashboard.SetJanusEngine(janusEngine)
		log.Printf("[JANUS] engine injected into dashboard API")
		if repo != nil {
			dashboard.SetRepository(repo)
			log.Printf("[Repository] injected into dashboard API")
		}
		// Create shared EventBus for SSE streaming AND simulation orchestration.
		// Both the Dashboard API and all simulation-triggered Systems use the SAME bus,
		// so simulation events (start, regime change, recommendations, guard outcomes)
		// flow to SSE clients in real time.
		dashEventBus := eventbus.NewChannelEventBus(256)

		// Inject EventBus for SSE streaming endpoint
		dashboard.SetEventBus(dashEventBus)
		autoRollback.WithEventBus(dashEventBus)
		dashboard.SetContext(context.Background())
		log.Printf("[EventBus] injected into dashboard API for SSE streaming")
		dashEventBus.Subscribe(eventbus.EventNarrative, func(ctx context.Context, event eventbus.BusEvent) error {
			apievents.BufferNarrativeEvent(event)
			return nil
		})
		dashEventBus.Subscribe(eventbus.EventPromotionRecorded, func(ctx context.Context, event eventbus.BusEvent) error {
			apievents.BufferPromotionRecordedEvent(event)
			return nil
		})
		dashEventBus.Subscribe(eventbus.EventHealthAlert, func(ctx context.Context, event eventbus.BusEvent) error {
			apievents.BufferHealthAlertEvent(event)
			return nil
		})
		dashEventBus.Subscribe(eventbus.EventRiskGateRejected, func(ctx context.Context, event eventbus.BusEvent) error {
			apievents.BufferRiskGateEvent(event)
			return nil
		})
		dashEventBus.Subscribe(eventbus.EventRiskGateAllowed, func(ctx context.Context, event eventbus.BusEvent) error {
			apievents.BufferRiskGateEvent(event)
			return nil
		})
		dashEventBus.Subscribe(eventbus.EventRiskGateOverridden, func(ctx context.Context, event eventbus.BusEvent) error {
			apievents.BufferRiskGateEvent(event)
			return nil
		})
		dashEventBus.Subscribe(eventbus.EventIndustryCalendar, func(ctx context.Context, event eventbus.BusEvent) error {
			apievents.BufferIndustryCalendarEvent(event)
			return nil
		})
		dashEventBus.Subscribe(eventbus.EventBacktestCompleted, func(ctx context.Context, event eventbus.BusEvent) error {
			apievents.BufferBacktestCompletedEvent(event)
			return nil
		})
		dashEventBus.Subscribe(eventbus.EventCalibrationCompleted, func(ctx context.Context, event eventbus.BusEvent) error {
			apievents.BufferCalibrationCompletedEvent(event)
			return nil
		})
		dashEventBus.Subscribe(eventbus.EventTradeSlippage, func(ctx context.Context, event eventbus.BusEvent) error {
			apievents.BufferTradeSlippageEvent(event)
			return nil
		})
		dashEventBus.Subscribe(eventbus.EventChannelIndividualHealth, func(ctx context.Context, event eventbus.BusEvent) error {
			apievents.BufferChannelIndividualHealthEvent(event)
			return nil
		})
		dashEventBus.Subscribe(eventbus.EventRegimeChangeConfirmed, func(ctx context.Context, event eventbus.BusEvent) error {
			apievents.BufferRegimeChangeConfirmedEvent(event)
			return nil
		})
		dashEventBus.Subscribe(eventbus.EventFactorWeightRegression, func(ctx context.Context, event eventbus.BusEvent) error {
			apievents.BufferFactorWeightRegressionEvent(event)
			return nil
		})
		dashEventBus.Subscribe(eventbus.EventDriftDetected, func(ctx context.Context, event eventbus.BusEvent) error {
			apievents.BufferDriftDetectedEvent(event)
			return nil
		})
		dashEventBus.Subscribe(eventbus.EventIngestionLagSpike, func(ctx context.Context, event eventbus.BusEvent) error {
			apievents.BufferIngestionLagSpikeEvent(event)
			return nil
		})
		risk.NewAuditSubscriber(dashEventBus)
		log.Printf("[Risk] audit subscriber registered on shared event bus")
		// Initial macro ingestion on startup to populate snapshot and publish events.
		ingestCtx, ingestCancel := context.WithTimeout(context.Background(), 60*time.Second)
		// Cancel ingest early if shutdown is signaled to avoid blocking
		// tests that send shutdown after 100ms (otherwise ingest would wait
		// up to 60s for Yahoo/Frankfurter geo timeouts).
		go func() {
			select {
			case <-deps.shutdown:
				ingestCancel()
			case <-ingestCtx.Done():
			}
		}()
		_, _, err = dashboard.IngestAndUpdateMacro(ingestCtx)
		ingestCancel()
		if err != nil {
			logging.Warn("main", "initial_macro_ingest_failed", "err", err)
		} else {
			logging.Info("main", "initial_macro_ingest_ok")
		}

		lifecycleMgr = storage.NewLifecycleManager(filepath.Join(cfg.WorkDir, "data/state"))
		dashboard.SetStorageReporter(lifecycleMgr)
		log.Printf("[Storage] reporter injected into dashboard API")

		// Startup health check: verify critical data files exist and warn if missing.
		replayPath := config.GetReplayDataPath(cfg.WorkDir)
		if _, err := os.Stat(replayPath); os.IsNotExist(err) {
			log.Printf("[Startup] ⚠️  replay data NOT FOUND at: %s", replayPath)
			log.Printf("[Startup]    Dashboard will show degraded data. To fix:")
			log.Printf("[Startup]    go run ./cmd/import-replay -source <csv> -target <jsonl>")
		} else {
			log.Printf("[Startup] replay data found at: %s", replayPath)
		}
		baselinePath := cfg.BaselinePolicyPath
		if baselinePath == "" {
			baselinePath = filepath.Join(cfg.WorkDir, "data", "state", "baseline_policy.json")
		}
		if _, err := os.Stat(baselinePath); os.IsNotExist(err) {
			log.Printf("[Startup] ⚠️  baseline policy NOT FOUND at: %s", baselinePath)
		}

		dashboard.RegisterAllRoutes(mux, monitoring.RouteOptions{IncludeBacktest: true, IncludeSwagger: *swaggerMode})

		if alertStore != nil {
			alertAPI := monitoring.NewAlertAPI(alertStore)
			alertAPI.RegisterRoutes(mux)
		}

		alertWebhook := alerting.NewAlertWebhookHandler(1000)
		mux.Handle("/api/v1/alerts", alertWebhook)
		log.Printf("[Alerting] registered /api/v1/alerts webhook handler (cap=1000)")

		if taskManager != nil {
			dashboard.SetTaskManager(taskManager)
			dashboard.RegisterTaskExecRoutes(mux)
			log.Printf("[TaskExec] injected into dashboard API")
		}

		RegisterAdminRoutes(mux, cfg)
		var monitor *monitoring.Monitor
		mux.HandleFunc("/admin/trigger-simulation", wrapAdminAuth(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			system, err := orchestrator.NewProductionSystemWithEventBus(cfg, dashEventBus, janusEngine)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				//nolint:errcheck
				fmt.Fprintf(w, `{"error":"create system: %v"}`+"\n", err)
				return
			}
			if gatewayFetcher != nil {
				system.Sim().SetProvider(orchestrator.NewGatewayBackedProvider(cfg))
			}
			if collector != nil {
				system.WithMetricsCollector(collector)
			}
			if repo != nil {
				system.SetRepository(repo)
			}

			if stRegistry != nil {
				system.WithStrategyTechniques(stRegistry, stSeedsPath)
			}
			if dashboard != nil {
				system.SetDrawdownReporter(func(d portfolio.DrawdownResult) {
					dashboard.SetLatestDrawdown(&d)
				})
			}
			capitalCfg := domain.DefaultCapitalPhaseConfig()
			capitalCfg.PhaseStartDate = time.Now().Add(-30 * 24 * time.Hour)
			controller := risk.NewCapitalPhaseController(capitalCfg)
			allocator := portfolio.NewCapitalAllocator()
			workflow, wErr := risk.NewApprovalWorkflow("data/state/approvals")
			if wErr != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				//nolint:errcheck
				fmt.Fprintf(w, `{"error":"approval workflow: %v"}`+"\n", wErr)
				return
			}
			system.WithCapitalManagement(controller, allocator, workflow)
			result, simErr := system.RunDailySimulation(time.Now())
			if simErr != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				//nolint:errcheck
				fmt.Fprintf(w, `{"error":"simulation: %v"}`+"\n", simErr)
				return
			}
			candidate, _ := system.NextExperimentCandidate()
			if recErr := system.RecordSessionSummary(result, candidate); recErr != nil {
				logging.Warn("admin", "record_session_failed", "err", recErr.Error())
			}
			if len(result.Orders) == 0 && monitor != nil {
				monitor.Alert(monitoring.AlertLevelWarning, "simulation",
					fmt.Sprintf("手動觸發場次 %s 產生 0 筆訂單（regime=%s）",
						system.Session().ID, result.Regime),
					map[string]any{
						"session":   system.Session().ID,
						"regime":    string(result.Regime),
						"orders":    0,
						"positions": len(result.Positions),
					})
			}
			w.Header().Set("Content-Type", "application/json")
			//nolint:errcheck
			fmt.Fprintf(w, `{"status":"ok","session":"%s","regime":"%s","orders":%d,"positions":%d}`+"\n",
				system.Session().ID, result.Regime, len(result.Orders), len(result.Positions))
		}))
		monitor, autoHandler := setupMonitor(alertStore, paramsCfg.Alert.SuppressCategories.Value)
		sysCtx, sysCancel := context.WithCancel(context.Background())

		var ruleEngine *monitoring.RuleEngine
		if monitor != nil {
			ruleEngine = monitoring.NewRuleEngine(monitor)
			for _, rule := range monitoring.DefaultRules() {
				ruleEngine.RegisterRule(rule)
			}
			for _, rule := range monitoring.LiveTradingRules() {
				ruleEngine.RegisterRule(rule)
			}
			params := config.GetParametersConfig().Alert
			ruleEngine.SetCheckInterval(time.Duration(params.RuleEngineIntervalSec.Value) * time.Second)
			log.Printf("[RuleEngine] initialized with %d rules, interval=%ds",
				len(monitoring.DefaultRules())+len(monitoring.LiveTradingRules()),
				params.RuleEngineIntervalSec.Value)
		}

		// Wire cross-market degraded-data detection into the unified alert+health
		// pipeline (Fix 5 — Option B full alerting). The callback avoids circular
		// imports by running in main.go where both gateway.Health() and monitor
		// are accessible.
		if gateway != nil && monitor != nil {
			if svc := dashboard.GetCrossMarketService(); svc != nil {
				svc.SetDegradedCallback(func(status string, failed []string) {
					if status != "degraded" || len(failed) == 0 {
						return // recovery or false alarm; no action needed
					}
					// Record per-channel degraded status in the same
					// UnifiedHealthStore used by Gateway.Fetch.
					for _, ch := range failed {
						_ = gateway.Health().Record(ch, "degraded",
							"detectDegradedUSStatus: data missing or zero-value in macro snapshot")
					}
					// Surface as user-visible alert.
					monitor.Warning("crossmarket",
						fmt.Sprintf("美台連動數據降級: %d 個通道失敗 (%v)", len(failed), failed),
						map[string]any{
							"data_status":     status,
							"failed_channels": failed,
						})
				})
				log.Printf("[CrossMarket] degraded-data callback wired to UnifiedHealthStore + Monitor")
			}
		}

		subFS, err := fs.Sub(web.DistFS, "dist")
		if err != nil {
			log.Fatalf("failed to get dist sub FS: %v", err)
		}
		registerSimpleRoutes(mux, collector, subFS)
		log.Printf("dashboard api listening on %s", *apiAddr)

		// Publish bootstrap events so the dashboard SSE stream shows system status immediately.
		publishBootstrapEvents(dashEventBus, replayPath, baselinePath)

		// Gateway already initialized before DashboardAPI. Create BackgroundTaskManager.
		var realtimeAdapter *realtime.RealTimeAdapter
		taskMgr := setupBackgroundTaskManager(gateway, monitor, autoHandler)
		if gateway != nil {

			// RealTimeAdapter: sub-second regime detection and agent weight
			// adaptation during live market sessions. Gated behind -allow-realtime
			// (mirrors -allow-live-broker pattern).
			if *allowRealtime {
				realtimeAdapter = realtime.NewRealTimeAdapter(&paramsCfg.Realtime)
				log.Printf("[RealTime] adapter created (cadence=%dms, window=%d)",
					paramsCfg.Realtime.UpdateIntervalMs.Value, 60)
			}

			registerDataSyncAndHealthTasks(taskMgr, cfg, gateway, monitor, pool)

			registerCapitalTasks(capitalDeps{
				taskMgr:           taskMgr,
				cfg:               cfg,
				gateway:           gateway,
				autoRollback:      autoRollback,
				autoJudgePromoter: autoJudgePromoter,
			})

			registerOperationsTasks(operationsDeps{
				taskMgr:         taskMgr,
				cfg:             cfg,
				monitor:         monitor,
				gateway:         gateway,
				healthMonitor:   healthMonitor,
				lifecycleMgr:    lifecycleMgr,
				dashboard:       dashboard,
				realtimeAdapter: realtimeAdapter,
				repo:            repo,
				collector:       collector,
			})

			um := metrics.NewUniverseMetrics()
			um.SetOnInc(func(name string, labels []string, value float64) {
				labelMap := make(map[string]string, len(labels)/2)
				for i := 0; i+1 < len(labels); i += 2 {
					labelMap[labels[i]] = labels[i+1]
				}
				collector.RecordCounter(name, value, labelMap)
			})
			classTreeAdapter := monitoring.AdaptClassificationTree(industry.DefaultClassification())
			registerExperimentTasks(experimentDeps{
				taskMgr:        taskMgr,
				cfg:            cfg,
				monitor:        monitor,
				dashboard:      dashboard,
				repo:           repo,
				collector:      collector,
				janusEngine:    janusEngine,
				dashEventBus:   dashEventBus,
				gateway:        gateway,
				gatewayFetcher: gatewayFetcher,
				ruleEngine:     ruleEngine,
				classTree:      classTreeAdapter,
				um:             um,
			})

			mux.HandleFunc("/universe/metrics", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(um.Snapshot())
			})

			riskGate := risk.NewRiskGate(risk.NewPreTradeGate(), risk.NewInTradeGate(), risk.NewPostTradeGate())
			if maturityTracker != nil {
				riskGate.WithMaturityTracker(maturityTracker)
			}
			dashboard.SetRiskGate(riskGate)

			if params := config.GetParametersConfig(); params != nil && params.RSITw.LastCalibratedScore.Value > 0 {
				riskGate.SetPreTradeRSITwScore(params.RSITw.LastCalibratedScore.Value)
				log.Printf("[RiskGate] restored RSI-tw calibration score: %.4f", params.RSITw.LastCalibratedScore.Value)
			}

			stSeedsPath = filepath.Join(cfg.WorkDir, "data/seeds/strategy_techniques.json")
			if stReg, err := strategy_techniques.LoadFromFile(stSeedsPath); err == nil {
				stRegistry = stReg
				stHandlers := apistrategies.NewHandlers(stRegistry)
				dashboard.SetStrategiesHandlers(stHandlers)
				// Re-register: RegisterAllRoutes ran before SetStrategiesHandlers,
				// so the original call encountered a nil handler. nil-safe.
				dashboard.RegisterStrategiesRoutes(mux)
				logging.Info("main", "strategy_techniques_loaded", "count", stRegistry.Count(), "path", stSeedsPath)
			} else {
				logging.Warn("main", "strategy_techniques_load_failed", "path", stSeedsPath, "err", err.Error())
			}

			// LLM annotator is opt-in via env var. Without
			// LLM_MINIMAX_API_KEY (or LLM_ANNOTATOR_API_KEY for backward
			// compatibility) the /annotate endpoint returns 503; this is
			// the explicit signal that the on-demand attribution path is not
			// configured. Init failure is Warn, not Fatal: rule_based
			// attribution remains authoritative.
			//
			// Phase 4 fix: LLM_ANNOTATOR_API_KEY has always held a MiniMax
			// coding plan key (sk-cp- prefix). The env var name was misleading.
			// We now read LLM_MINIMAX_API_KEY first, falling back to
			// LLM_ANNOTATOR_API_KEY for backward compatibility.
			apiKey := config.GetSecret("LLM_MINIMAX_API_KEY")
			if apiKey == "" {
				apiKey = config.GetSecret("LLM_ANNOTATOR_API_KEY")
			}
			var kimi *llm_annotator.KimiClient
			if apiKey != "" {
				var err error
				kimi, err = llm_annotator.NewKimiClient(llm_annotator.Config{APIKey: apiKey, Metrics: collector})
				if err != nil {
					logging.Warn("main", "kimi_init_failed", "err", err.Error())
				} else {
					if store, storeErr := llm_annotator.NewJSONLStore(
						filepath.Join(cfg.WorkDir, "data/state/llm_annotations", "annotations.jsonl"),
					); storeErr != nil {
						logging.Warn("main", "annotation_store_init_failed", "err", storeErr.Error())
					} else {
						kimi.SetAnnotationStore(store)
						defer func() { _ = store.Close() }()
					}
					dashboard.SetStrategiesAnnotator(kimi)
					logging.Info("main", "kimi_annotator_loaded", "backend", kimi.Name())
				}
			} else {
				logging.Info("main", "kimi_annotator_disabled", "hint", "set LLM_ANNOTATOR_API_KEY to enable on-demand attribution")
			}

			// Phase 1: LLM Router (experimental, X-level). Wraps existing KimiClient via adapter.
			// Does NOT replace dashboard.SetStrategiesAnnotator(kimi) above — that still receives
			// the raw *KimiClient required by dashboard_api.go:880 type assertion.
			var llmRouter llm.Router
			if kimi != nil {
				llmRouter = llm.NewDefaultRouter(
					llmAdapters.NewAnnotatorAdapter(kimi, "moonshot-v1-8k"),
				)
			} else {
				// No API key — Router still exists but all Supports() return false.
				llmRouter = llm.NewDefaultRouter()
			}
			llmHealthHandler := llmHealth.NewHandler(llmRouter)
			llmHealthHandler.RegisterRoutes(mux)

			// =============================================================================
			// Phase 2: LLM Capability Wiring (opt-in via LLM_*_ENABLED flags)
			// =============================================================================

			// Provider clients (created only if API keys are set)
			var (
				deepseekClient *clients.DeepSeekClient
				minimaxClient  *clients.MiniMaxClient
				kimiClient     *clients.KimiClient
			)

			if apiKey := config.GetSecret("LLM_DEEPSEEK_API_KEY"); apiKey != "" {
				deepseekClient = clients.NewDeepSeekClient(apiKey, nil)
			}
			if apiKey := config.GetSecret("LLM_MINIMAX_API_KEY"); apiKey != "" {
				minimaxClient = clients.NewMiniMaxClient(apiKey, nil)
			}
			if apiKey := config.GetSecret("LLM_KIMI_API_KEY"); apiKey != "" {
				kimiClient = clients.NewKimiClient(apiKey, nil)
			}

			// ProviderImpl adapters (created only if client exists)
			var (
				deepseekAdapter *llmAdapters.DeepSeekAdapter
				minimaxAdapter  *llmAdapters.MiniMaxAdapter
				kimiPhase2      *llmAdapters.KimiAdapter
			)
			if deepseekClient != nil {
				deepseekAdapter = llmAdapters.NewDeepSeekAdapter(deepseekClient, "deepseek-v4-pro")
			}
			if minimaxClient != nil {
				minimaxAdapter = llmAdapters.NewMiniMaxAdapter(minimaxClient)
			}
			if kimiClient != nil {
				kimiPhase2 = llmAdapters.NewKimiAdapter(kimiClient)
			}

			// Register adapters with Router (llmRouter is always non-nil after Phase 1)
			if deepseekAdapter != nil {
				_ = llmRouter.Register(deepseekAdapter)
			}
			if minimaxAdapter != nil {
				_ = llmRouter.Register(minimaxAdapter)
			}
			if kimiPhase2 != nil {
				_ = llmRouter.Register(kimiPhase2)
			}

			// Wire 4 module hooks (only if flag enabled AND Router exists)
			if cfg.LLMRationaleTranslationEnabled {
				narrative.RationaleTranslator = func(ctx context.Context, englishText string, dataClass string) (string, error) {
					h := capabilities.NewRationaleGenerationHandler(llmRouter)
					input := schemas.RationaleGenerationInput{
						EnglishText: englishText,
						DataClass:   llm.DataClassNonRegulated,
					}
					output, err := h.Handle(ctx, input)
					return output.TranslatedText, err
				}
			}
			if cfg.LLMPrismScenarioEnabled {
				orchestrator.ScenarioExplainer = func(ctx context.Context, result interface{}) (string, error) {
					h := capabilities.NewScenarioSimulationHandler(llmRouter)
					tr, ok := result.(prism.TrainingResult)
					if !ok {
						return "", fmt.Errorf("scenario: unexpected result type %T", result)
					}
					input := schemas.ScenarioSimulationInput{Result: tr}
					output, err := h.Handle(ctx, input)
					return output.Insight, err
				}
			}
			if cfg.LLMNarrativeExplainEnabled {
				narrative.RegimeExplainer = func(ctx context.Context, event interface{}) (string, error) {
					h := capabilities.NewRegimeExplanationHandler(llmRouter)
					ne, ok := event.(*narrative.NarrativeEvent)
					if !ok {
						return "", fmt.Errorf("regime: unexpected event type %T", event)
					}
					input := schemas.RegimeExplanationInput{Event: *ne}
					output, err := h.Handle(ctx, input)
					return output.Headline, err
				}
				narrative.SentimentExplainer = func(ctx context.Context, event interface{}) (string, error) {
					h := capabilities.NewSentimentExplanationHandler(llmRouter)
					ne, ok := event.(*narrative.NarrativeEvent)
					if !ok {
						return "", fmt.Errorf("sentiment: unexpected event type %T", event)
					}
					input := schemas.SentimentExplanationInput{Event: *ne}
					output, err := h.Handle(ctx, input)
					return output.Explanation, err
				}
			}
			if cfg.LLMRiskForensicsEnabled {
				risk.PerformanceForensics = func(ctx context.Context, snapshot interface{}) (string, error) {
					h := capabilities.NewPerformanceForensicsHandler(llmRouter)
					rs, ok := snapshot.(domain.RiskSnapshot)
					if !ok {
						return "", fmt.Errorf("forensics: unexpected snapshot type %T", snapshot)
					}
					input := schemas.PerformanceForensicsInput{Snapshot: rs}
					output, err := h.Handle(ctx, input)
					return output.Commentary, err
				}
			}
			if cfg.LLMConfidenceCommentaryEnabled {
				risk.ConfidenceCommentary = func(ctx context.Context, decision interface{}) (string, error) {
					h := capabilities.NewConfidenceCommentaryHandler(llmRouter)
					rd, ok := decision.(risk.RiskDecision)
					if !ok {
						return "", fmt.Errorf("confidence: unexpected decision type %T", decision)
					}
					input := schemas.ConfidenceCommentaryInput{
						Decision:  rd,
						DataClass: llm.DataClassNonRegulated,
					}
					output, err := h.Handle(ctx, input)
					return output.Commentary, err
				}
			}

			if dashEventBus != nil {
				dashEventBus.Subscribe(eventbus.EventSimulationComplete, func(ctx context.Context, ev eventbus.BusEvent) error {
					log.Printf("[EventLogic] simulation complete: %s", ev.ID)
					return nil
				})
				log.Printf("[EventLogic] subscribed to EventSimulationComplete")

				dashEventBus.Subscribe(eventbus.EventRegimeChange, func(ctx context.Context, ev eventbus.BusEvent) error {
					if p, ok := ev.Payload.(eventbus.RegimeEventPayload); ok {
						logging.Info("monitor", "regime_change",
							"old", string(p.OldRegime), "new", string(p.NewRegime),
							"confidence", fmt.Sprintf("%.2f", p.Confidence), "by", p.DeterminedBy)
					} else {
						logging.Info("monitor", "regime_change_event", "id", ev.ID, "payload", fmt.Sprintf("%+v", ev.Payload))
					}
					return nil
				})
				dashEventBus.Subscribe(eventbus.EventSharpeDegradation, func(ctx context.Context, ev eventbus.BusEvent) error {
					logging.Warn("monitor", "sharpe_degradation",
						"id", ev.ID, "payload", fmt.Sprintf("%+v", ev.Payload),
						"description", ev.Description)
					return nil
				})
				dashEventBus.Subscribe(eventbus.EventDrawdownBreach, func(ctx context.Context, ev eventbus.BusEvent) error {
					logging.Error("monitor", "drawdown_breach",
						"id", ev.ID, "payload", fmt.Sprintf("%+v", ev.Payload),
						"description", ev.Description)
					return nil
				})
				log.Printf("[Monitor] subscribed to regime/sharpe/drawdown events")
			}

			log.Printf("[RiskGate] injected into DashboardAPI for calibration reports")

			if realtimeAdapter != nil {
				go realtimeAdapter.Start(sysCtx)
				log.Printf("[RealTime] adapter started (cadence=%dms)", paramsCfg.Realtime.UpdateIntervalMs.Value)
			}

			registerCalibrationTasks(calibrationDeps{
				TaskMgr:         taskMgr,
				Cfg:             cfg,
				ParamsCfg:       paramsCfg,
				RiskGate:        riskGate,
				JanusEngine:     janusEngine,
				Dashboard:       dashboard,
				FinMindClient:   nil,
				MaturityTracker: maturityTracker,
				CalProvider:     monitoring.NewSessionCalibrationProvider(filepath.Join(cfg.WorkDir, "data/state")),
			})

			taskMgr.Start(sysCtx)
			log.Printf("[Gateway] BackgroundTaskManager started with %d tasks", len(taskMgr.List()))
			dashEventBus.Publish(eventbus.BusEvent{
				ID:          "schedule-" + time.Now().Format("150405"),
				Type:        eventbus.EventSystemStart,
				Timestamp:   time.Now(),
				Description: fmt.Sprintf("排程已就緒 · %d 個背景任務已註冊（含每日模擬、實驗、風控校準）", len(taskMgr.List())),
				Severity:    "info",
			})
		}

		if taskMgr != nil {
			apischeduler.NewHandlers(apischeduler.NewSchedulerService(taskMgr)).RegisterRoutes(mux)
			log.Printf("[Gateway] scheduler API routes registered")
		}

		var btRunner *autobacktest.Runner
		if dashboard.GetEventBus() != nil {
			btRunner = autobacktest.NewRunnerWithEventBus(cfg, dashboard.GetEventBus())
			log.Printf("[AutoBacktest] connected to Dashboard EventBus for SSE streaming")
		} else {
			btRunner = autobacktest.NewRunner(cfg)
			log.Printf("[AutoBacktest] running without EventBus (no SSE events)")
		}
		_ = taskMgr.Register(&apigateway.ScheduledTask{
			Name:     "autobacktest_daily",
			Interval: 1 * time.Hour,
			Enabled:  true,
			Task: func(ctx context.Context) error {
				return autobacktest.RunScheduledBacktest(ctx, btRunner)
			},
		})
		log.Printf("[Gateway] registered autobacktest_daily background task (1h interval)")

		authWrappedMux := apishared.AuthMiddleware(mux)
		finalMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")
			if r.URL.Path == "/metrics" || r.URL.Path == "/health" || strings.HasPrefix(r.URL.Path, "/static/") {
				mux.ServeHTTP(w, r)
				return
			}
			authWrappedMux.ServeHTTP(w, r)
		})
		srv := &http.Server{
			Addr:              *apiAddr,
			Handler:           finalMux,
			ReadHeaderTimeout: 10 * time.Second,
		}
		srvErr := make(chan error, 1)
		go func() {
			if err := deps.listenAndServe(srv); err != nil && !errors.Is(err, http.ErrServerClosed) {
				srvErr <- fmt.Errorf("dashboard api server failed: %w", err)
			}
		}()

		sigCh := registerShutdownSignal()
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
		if realtimeAdapter != nil {
			realtimeAdapter.Stop()
			log.Printf("[RealTime] adapter stopped")
		}
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
		return runLiveTrading(cfg, deps, collector, repo, *forceIntradayCycles)
	}
	return runSimulation(cfg, false, collector, repo, deps.shutdown)
}

func runSimulation(cfg config.Config, verbose bool, collector *monitoring.MetricsCollector, repo *repository.DualWriteRepository, shutdown <-chan struct{}) error {
	system, err := orchestrator.NewProductionSystem(cfg)
	if err != nil {
		return fmt.Errorf("create system: %w", err)
	}
	system.SetVerboseTrace(verbose)
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

	// Run simulation in a goroutine so we can listen for shutdown signals.
	done := make(chan error, 1)
	go func() {
		result, simErr := system.RunDailySimulation(time.Now())
		if simErr != nil {
			done <- fmt.Errorf("simulation failed: %w", simErr)
			return
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
			done <- fmt.Errorf("candidate selection failed: %w", err)
			return
		}
		if candidate != nil {
			fmt.Printf("next_experiment_agent: %s\n", candidate.Agent.ID)
			fmt.Printf("next_experiment_skill: %s\n", candidate.Agent.Skill)
			fmt.Printf("baseline_sharpe_like: %.6f\n", candidate.Scorecard.SharpeLike)
		}

		if err := system.RecordSessionSummary(result, candidate); err != nil {
			done <- fmt.Errorf("record session summary failed: %w", err)
			return
		}

		stateStore := livestore.NewStateStore(livestore.DefaultLiveStateBasePath)
		if err := stateStore.Load(); err != nil {
			logging.Warn("main", "load_live_state_failed", "err", err.Error())
		}
		for symbol := range stateStore.GetPositions() {
			stateStore.RemovePosition(symbol)
		}
		var totalExposure, totalUnrealizedPnL float64
		for _, pos := range result.Positions {
			totalExposure += pos.MarketValue
			totalUnrealizedPnL += pos.UnrealizedPnL
			stateStore.UpdatePosition(pos)
		}
		stateStore.UpdatePortfolio(livestore.PortfolioState{
			Cash:          result.EndingCash,
			TotalExposure: totalExposure,
			AvailableCash: result.EndingCash,
			DayPnL:        result.BeforeTaxPnL,
			UnrealizedPnL: totalUnrealizedPnL,
			LastUpdated:   time.Now(),
		})
		stateStore.UpdateRegime(result.Regime, 0.5, "simulation")
		if err := stateStore.Save(); err != nil {
			logging.Warn("main", "sync_live_state_failed", "err", err.Error())
		} else {
			logging.Info("main", "synced_simulation_to_live_store",
				"positions", len(result.Positions),
				"exposure", totalExposure,
				"cash", result.EndingCash)
		}

		done <- nil
	}()

	select {
	case <-shutdown:
		return fmt.Errorf("simulation: shutdown")
	case err := <-done:
		return err
	}
}

// buildBaseState queries the provider for current market state.
// Falls back to placeholder values if provider fails (with warning log).
func runLiveTrading(cfg config.Config, deps appDeps, collector *monitoring.MetricsCollector, repo *repository.DualWriteRepository, forceIntradayCycles bool) error {
	eventBus := live.NewChannelEventBus(64)
	system, err := orchestrator.NewProductionSystemWithEventBus(cfg, eventBus, nil)
	if err != nil {
		return fmt.Errorf("create system: %w", err)
	}
	if collector != nil {
		system.WithMetricsCollector(collector)
	}
	if repo != nil {
		system.SetRepository(repo)
		log.Printf("[Repository] injected into live trading system")
	}

	stateStore := livestore.NewStateStore("data/state/live")
	provider := marketdata.NewHybridProvider(cfg.FinMindAPIKey, cfg.FugleAPIKey)

	liveCfg := live.DefaultOrchestratorConfig()
	liveCfg.ForceIntradayCycles = forceIntradayCycles
	liveCfg.BrokerMode = cfg.BrokerMode
	liveCfg.BrokerAdapter = cfg.BrokerAdapter
	liveCfg.BrokerSigner = cfg.BrokerSigner
	liveCfg.BrokerKeyID = cfg.BrokerKeyID
	liveCfg.TWSEAPIURL = cfg.TWSEAPIURL
	liveCfg.TWSEAPIKey = cfg.TWSEAPIKey
	liveCfg.TWSEAPISecret = cfg.TWSEAPISecret
	liveCfg.TWSEAccountID = cfg.TWSEAccountID
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

	d6WatchlistPath := filepath.Join(cfg.WorkDir, "data", "state", "universe_watchlist.json")
	if d6Data, rErr := os.ReadFile(d6WatchlistPath); rErr == nil {
		var wl monitoring.Watchlist
		if uErr := json.Unmarshal(d6Data, &wl); uErr == nil {
			var d6Symbols []string
			for _, entry := range wl.Symbols {
				if entry.ConsecutiveFailures >= 60 {
					d6Symbols = append(d6Symbols, entry.Symbol)
				}
			}
			if len(d6Symbols) > 0 {
				o.SetWatchlist(d6Symbols)
				log.Printf("[D6 Watchlist] wired %d expired symbols to live scheduler: %v", len(d6Symbols), d6Symbols)
			} else {
				log.Printf("[D6 Watchlist] no D6-expired symbols found in watchlist")
			}
		} else {
			log.Printf("[D6 Watchlist] failed to parse watchlist file: %v", uErr)
		}
	} else {
		log.Printf("[D6 Watchlist] no watchlist file found — skipping")
	}

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
	if repo != nil {
		dashboard.SetRepository(repo)
		log.Printf("[Repository] injected into live trading dashboard API")
	}
	dashboard.SetEventBus(eventBus)
	dashboard.SetContext(ctx)
	logging.SetLogContext(ctx)
	log.Printf("[EventBus] injected into live trading dashboard API for SSE streaming")
	dashboard.RegisterAllRoutes(mux, monitoring.RouteOptions{IncludeBacktest: false, IncludeSwagger: true})
	alertStore, err := monitoring.NewAlertStore(filepath.Join(cfg.WorkDir, "data/state/alerts"))
	if err != nil {
		log.Printf("[Alerts] failed to create alert store: %v", err)
	} else {
		alertAPI := monitoring.NewAlertAPI(alertStore)
		alertAPI.RegisterRoutes(mux)
		monitor.SetAlertStore(alertStore)
		// Phase 2A: dedup, auto-handler, console output
		alertDeduplicator := monitoring.NewAlertDeduplicator(5*time.Minute, alertStore)
		var suppressRules []monitoring.SuppressRule
		if p := config.GetParametersConfig(); p != nil {
			for _, cat := range p.Alert.SuppressCategories.Value {
				suppressRules = append(suppressRules, monitoring.SuppressRule{
					Category: cat,
					Duration: 24 * time.Hour,
				})
			}
		}
		autoHandler := monitoring.NewAutoHandler(alertStore, suppressRules)
		monitor.SetDeduplicator(alertDeduplicator)
		monitor.SetAutoHandler(autoHandler)
		monitor.RegisterHandler(monitoring.ConsoleHandler)
	}

	subFS, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		log.Fatalf("failed to get dist sub FS: %v", err)
	}
	mux.Handle("/", staticHandler(subFS))
	apiAddr := ":8080"
	srv := &http.Server{
		Addr:              apiAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
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

	sigCh := registerShutdownSignal()
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

func runSimulationMode(rt *bootstrap.Runtime, cfg config.Config, verbose bool, dateOverride string) error {
	if verbose {
		log.Println("[SIMULATE] verbose mode enabled")
	}

	sessionDate := time.Now().Format("2006-01-02")
	if dateOverride != "" {
		if _, err := time.Parse("2006-01-02", dateOverride); err != nil {
			return fmt.Errorf("invalid date format (expected 2006-01-02): %w", err)
		}
		sessionDate = dateOverride
		// Thread date override through config so ReplaySessionDate resolves correctly.
		cfg.ReplaySessionDate = dateOverride
		if verbose {
			log.Printf("[SIMULATE] date override: %s", sessionDate)
		}
	}

	if verbose {
		log.Printf("[SIMULATE] running one-shot daily simulation for %s", sessionDate)
	}

	collector := rt.MetricsCollector
	repo := rt.Repository

	if err := runSimulation(cfg, verbose, collector, repo, nil); err != nil {
		return fmt.Errorf("simulation failed: %w", err)
	}

	if verbose {
		log.Printf("[SIMULATE] simulation completed for %s", sessionDate)
	}

	return nil
}
