package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/autobacktest"
	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/bootstrap"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/eventlogic"
	"github.com/kaecer68/atlas-go/internal/experiment"
	"github.com/kaecer68/atlas-go/internal/fubonproxy"
	"github.com/kaecer68/atlas-go/internal/importer"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/live"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/metalearning"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	apieventlogic "github.com/kaecer68/atlas-go/internal/monitoring/api/eventlogic"
	apievents "github.com/kaecer68/atlas-go/internal/monitoring/api/events"
	apischeduler "github.com/kaecer68/atlas-go/internal/monitoring/api/scheduler"
	apishared "github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/realtime"
	"github.com/kaecer68/atlas-go/internal/repository"
	"github.com/kaecer68/atlas-go/internal/retail"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/scheduler"
	"github.com/kaecer68/atlas-go/internal/storage"
	"github.com/kaecer68/atlas-go/internal/swarm"
	"github.com/kaecer68/atlas-go/web"
)

// experimentMonitorAdapter wraps *monitoring.Monitor to match experiment.AutoExperimentMonitor interface.
type experimentMonitorAdapter struct {
	m *monitoring.Monitor
}

func (a *experimentMonitorAdapter) Alert(level string, category, message string, details map[string]any) {
	if a.m != nil {
		var al monitoring.AlertLevel
		switch level {
		case "error":
			al = monitoring.AlertLevelError
		case "warning":
			al = monitoring.AlertLevelWarning
		default:
			al = monitoring.AlertLevelInfo
		}
		a.m.Alert(al, category, message, details)
	}
}

type appDeps struct {
	loadConfig      func() config.Config
	newDashboardAPI func(string, string, *monitoring.MetricsCollector) *monitoring.DashboardAPI
	listenAndServe  func(*http.Server) error
	shutdown        chan struct{}
	dataFetcher     monitoring.DataFetcher // when non-nil, skips Gateway init and uses this fetcher
}

func defaultAppDeps() appDeps {
	return appDeps{
		loadConfig: config.Load,
		newDashboardAPI: func(workDir, ledgerDir string, collector *monitoring.MetricsCollector) *monitoring.DashboardAPI {
			return monitoring.NewDashboardAPI(workDir, ledgerDir, collector)
		},
		listenAndServe: func(srv *http.Server) error { return srv.ListenAndServe() },
		shutdown:       make(chan struct{}),
	}
}

// getLatestReplayDate reads the replay CSV and returns the latest date.
func getLatestReplayDate(csvPath string) (time.Time, error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = f.Close() }()

	reader := csv.NewReader(f)
	var latest time.Time
	_, _ = reader.Read()
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

func publishBootstrapEvents(bus eventbus.EventBus, replayPath, baselinePath string) {
	now := time.Now()

	// Check data status
	replayStatus := "已載入"
	replayDate := ""
	if _, err := os.Stat(replayPath); os.IsNotExist(err) {
		replayStatus = "未找到"
	} else if d, err := getLatestReplayDate(replayPath); err == nil {
		replayDate = d.Format("2006-01-02")
	} else {
		replayStatus = "載入失敗"
	}

	baselineStatus := "已載入"
	if _, err := os.Stat(baselinePath); os.IsNotExist(err) {
		baselineStatus = "未找到"
	}

	bus.Publish(eventbus.BusEvent{
		ID:        "bootstrap-" + now.Format("150405"),
		Type:      eventbus.EventSystemStart,
		Timestamp: now,
		Description: "Atlas 系統啟動完成 · replay 資料 " + replayStatus + func() string {
			if replayDate != "" {
				return "（" + replayDate + "）"
			}
			return ""
		}() + " · 基線策略 " + baselineStatus,
		Severity: "info",
		Payload: map[string]any{
			"replay_status":   replayStatus,
			"replay_date":     replayDate,
			"baseline_status": baselineStatus,
		},
	})
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
	logFormat := flags.String("log-format", "text", "log format: text or json")
	simulateMode := flags.Bool("simulate", false, "run one-shot daily simulation and exit (skip api server)")
	verboseMode := flags.Bool("verbose", false, "enable color-coded terminal trace output during simulation")
	dateOverride := flags.String("date", "", "override simulation session date (format: 2006-01-02)")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	cfg := deps.loadConfig()
	logging.Init(*logFormat, slog.LevelInfo)

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

	var janusEngine *janus.Engine

	// Handle --simulate mode: run one-shot daily simulation and exit
	if *simulateMode {
		return runSimulationMode(rt, cfg, *verboseMode, *dateOverride)
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

		var elDetector *eventlogic.PatternDetector
		var elCorrector *eventlogic.SelfCorrector
		var elValidator *eventlogic.RuleValidator
		var narLifecycleMgr *narrative.EventLifecycleManager
		var lifecycleMgr *storage.LifecycleManager
		var elRulesPath string
		var elHistoryRecorder *eventlogic.HistoryRecorder

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

		// Start fubon-proxy process manager (non-fatal on failure).
		// In dry-run mode, fubon-proxy is skipped — it requires live broker credentials.
		if *brokerMode != "dry-run" {
			fubonMgr := fubonproxy.NewManager(cfg.WorkDir)
			if err := fubonMgr.Start(context.Background()); err != nil {
				log.Printf("[FubonProxy] start warning (non-fatal): %v", err)
			} else {
				log.Printf("[FubonProxy] process manager started")
			}
			defer fubonMgr.Stop()
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
		dashboard.SetHealthManager(portfolio.NewAgentHealthManagerWithStore(portfolio.DefaultAgentHealthConfig(), healthStore).WithParameters(runtimeParams))
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
		dashboard.SetContext(context.Background())
		log.Printf("[EventBus] injected into dashboard API for SSE streaming")
		dashEventBus.Subscribe(eventbus.EventNarrative, func(ctx context.Context, event eventbus.BusEvent) error {
			apievents.BufferNarrativeEvent(event)
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
		narLifecycleMgr = dashboard.GetEventLifecycleManager()

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

		if taskManager != nil {
			dashboard.SetTaskManager(taskManager)
			dashboard.RegisterTaskExecRoutes(mux)
			log.Printf("[TaskExec] injected into dashboard API")
		}

		adminHandler := func(h http.HandlerFunc) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				apiKey := os.Getenv("ATLAS_API_KEY")
				if apiKey != "" {
					provided := r.Header.Get("X-API-Key")
					if provided == "" {
						auth := r.Header.Get("Authorization")
						if strings.HasPrefix(auth, "Bearer ") {
							provided = strings.TrimPrefix(auth, "Bearer ")
						}
					}
					if provided != apiKey {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusUnauthorized)
						//nolint:errcheck
						fmt.Fprintf(w, `{"error":"unauthorized"}`+"\n")
						return
					}
				}
				h(w, r)
			}
		}
		mux.HandleFunc("/admin/reload-config", adminHandler(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			if err := config.ReloadParametersConfig(); err != nil {
				http.Error(w, fmt.Sprintf("Failed to reload config: %v", err), http.StatusInternalServerError)
				return
			}
			cfg := config.GetParametersConfig()
			w.Header().Set("Content-Type", "application/json")
			//nolint:errcheck
			fmt.Fprintf(w, `{"status":"ok","version":"%s"}`+"\n", cfg.Version)
		}))
		mux.HandleFunc("/api/admin/calibrate-thresholds", adminHandler(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			revenuePath := filepath.Join(cfg.WorkDir, "data", "replay", "month_revenue.jsonl")
			configPath := filepath.Join(cfg.WorkDir, "configs", "parameters.json")
			if err := industry.RecalibrateThresholds(revenuePath, configPath); err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				//nolint:errcheck
				fmt.Fprintf(w, `{"error":"%s"}`+"\n", err.Error())
				return
			}
			w.Header().Set("Content-Type", "application/json")
			//nolint:errcheck
			fmt.Fprintf(w, `{"status":"ok","message":"thresholds recalibrated"}`+"\n")
		}))
		var monitor *monitoring.Monitor
		mux.HandleFunc("/admin/trigger-simulation", adminHandler(func(w http.ResponseWriter, r *http.Request) {
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
			if elDetector != nil && elCorrector != nil {
				system.WithEventLogic(elDetector, elCorrector, elRulesPath, elHistoryRecorder)
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
		mux.HandleFunc("/metrics", monitoring.PrometheusHandler(collector))
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		})
		monitor = monitoring.NewMonitor()
		if alertStore != nil {
			monitor.SetAlertStore(alertStore)
		}
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

		subFS, err := fs.Sub(web.DistFS, "dist")
		if err != nil {
			log.Fatalf("failed to get dist sub FS: %v", err)
		}
		handler := staticHandler(subFS)
		mux.Handle("/", handler)
		mux.Handle("/static/", http.StripPrefix("/static/", handler))
		log.Printf("dashboard api listening on %s", *apiAddr)

		// Publish bootstrap events so the dashboard SSE stream shows system status immediately.
		publishBootstrapEvents(dashEventBus, replayPath, baselinePath)

		// Gateway already initialized before DashboardAPI. Create BackgroundTaskManager.
		var taskMgr *apigateway.BackgroundTaskManager
		var realtimeAdapter *realtime.RealTimeAdapter
		if gateway != nil {
			taskMgr = apigateway.NewBackgroundTaskManager(gateway)
		} else {
			taskMgr = apigateway.NewBackgroundTaskManager(nil)
		}
		if gateway != nil {

			// Wire failure alerts for background tasks.
			taskMgr.SetFailureHandler(func(name string, consecutiveFailures int, err error) {
				if consecutiveFailures >= 3 {
					monitor.Alert(monitoring.AlertLevelError, "background_task",
						fmt.Sprintf("Task %s failed %d consecutive times: %v", name, consecutiveFailures, err),
						map[string]any{"task": name, "consecutive_failures": consecutiveFailures})
				}
			})

			// RealTimeAdapter: sub-second regime detection and agent weight
			// adaptation during live market sessions. Gated behind -allow-realtime
			// (mirrors -allow-live-broker pattern).
			if *allowRealtime {
				realtimeAdapter = realtime.NewRealTimeAdapter(&paramsCfg.Realtime)
				log.Printf("[RealTime] adapter created (cadence=%dms, window=%d)",
					paramsCfg.Realtime.UpdateIntervalMs.Value, 60)
			}

			// Register channel_health_sync task (DB sync, not a data fetcher).
			if pool != nil {
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "channel_health_sync",
					Interval: 5 * time.Minute,
					Enabled:  true,
					Task: func(ctx context.Context) error {
						healthStore := monitoring.NewChannelHealthStoreWithPool(filepath.Join(cfg.WorkDir, "data/state"), pool)
						return healthStore.SyncAllToDB()
					},
				})
				log.Printf("[Gateway] registered channel_health_sync background task (5m interval)")
			}

			// Register us_market_refresh — batch-refresh 7 US market channels
			// (spx, ndx, dji, nvda, aapl, msft, tsm_adr) every 5 minutes.
			// These channels share yahooSharedLimiter; Gateway.Fetch handles
			// both rate limiting and circuit breaking per channel. Per-channel
			// errors are logged but do not fail the batch (transient errors on
			// one channel should not block the others).
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "us_market_refresh",
				Interval: 5 * time.Minute,
				Enabled:  true,
				Task:     apigateway.NewUSMarketRefreshTask(gateway),
			})
			log.Printf("[Gateway] registered us_market_refresh background task (5m interval)")

			// Register seasonal_calibration background task. Guard: skip silently if
			// the calibrate-seasonal binary is not co-located with the current binary
			// (production deploys without it stay clean; no live-trading impact).
			exePath, exeErr := os.Executable()
			if exeErr == nil {
				seasonalBin := filepath.Join(filepath.Dir(exePath), "calibrate-seasonal")
				if _, statErr := os.Stat(seasonalBin); statErr == nil {
					_ = taskMgr.Register(&apigateway.ScheduledTask{
						Name:     "seasonal_calibration",
						Interval: scheduler.SeasonalCalibrationDefaults.Interval,
						Jitter:   30 * time.Minute,
						Enabled:  true,
						Task:     scheduler.SeasonalCalibrationTaskFunc(seasonalBin),
					})
					log.Printf("[Gateway] registered seasonal_calibration background task (7d interval)")
				} else {
					log.Printf("[Gateway] seasonal_calibration skipped: binary not found at %s", seasonalBin)
				}
			} else {
				log.Printf("[Gateway] seasonal_calibration skipped: os.Executable failed: %v", exeErr)
			}

			// Register calibration_cycle background task (rolling calibration framework).
			// Maturity-gated: BackgroundCalibrationScheduler.RunDaily checks maturityTracker
			// and skips gracefully in BURN_IN mode (no validation, no false signals).
			if maturityTracker != nil {
				calTask := narrative.NewCalibrationTask(cfg.WorkDir)
				calScheduler := scheduler.NewBackgroundCalibrationScheduler(maturityTracker)
				calScheduler.Register(&scheduler.CalibrationTask{
					Name:        "narrative_weight_calibration",
					MinMaturity: domain.MaturityCalibrating,
					Run: func(_ context.Context) error {
						_, err := calTask.RunCalibrationCycle()
						return err
					},
				})
				dashboard.SetCalibrationTask(calTask)
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "calibration_cycle",
					Interval: 24 * time.Hour,
					Jitter:   30 * time.Minute,
					Enabled:  paramsCfg.Narrative.CalibrationEnabled.Value,
					Task:     calScheduler.RunDaily,
				})
				log.Printf("[Gateway] registered calibration_cycle background task (24h interval, maturity-gated)")
			} else {
				log.Printf("[Gateway] calibration_cycle skipped: maturity tracker is nil")
			}
			// Register health_check via HealthChecker.RunOnce (stateStore is nil in API mode).
			if monitor != nil {
				healthChecker := monitoring.NewHealthChecker(monitor, nil)
				if gateway != nil {
					healthChecker.SetGateway(gateway)
				}
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "health_check",
					Interval: 30 * time.Second,
					Enabled:  true,
					Task: func(ctx context.Context) error {
						return healthChecker.RunOnce(ctx)
					},
				})
				log.Printf("[Gateway] registered health_check background task (30s interval)")
			}

			// Register channel health checks for third-party data providers.
			// These tasks populate the Gateway health store so the frontend
			// "信息通道" page can show actual status instead of "未知".
			if cfg.FugleAPIKey != "" {
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:      "channel_health_fugle",
					ChannelID: "fugle",
					Interval:  1 * time.Hour,
					Enabled:   true,
					Task: func(ctx context.Context) error {
						_, err := gateway.Fetch(ctx, "fugle")
						return err
					},
				})
				log.Printf("[Gateway] registered channel_health_fugle background task (1h interval)")
			}

			if cfg.FubonAPIKey != "" {
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:      "channel_health_fubon",
					ChannelID: "fubon",
					Interval:  1 * time.Hour,
					Enabled:   true,
					Task: func(ctx context.Context) error {
						_, err := gateway.Fetch(ctx, "fubon")
						return err
					},
				})
				log.Printf("[Gateway] registered channel_health_fubon background task (1h interval)")
			}

			if cfg.FinMindAPIKey != "" {
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:      "channel_health_finmind",
					ChannelID: "finmind",
					Interval:  1 * time.Hour,
					Enabled:   true,
					Task: func(ctx context.Context) error {
						_, err := gateway.Fetch(ctx, "finmind")
						return err
					},
				})
				log.Printf("[Gateway] registered channel_health_finmind background task (1h interval)")
			}

			// Register TWSE replay health check (always available, reads from local CSV).
			{
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:      "channel_health_twse_replay",
					ChannelID: "twse_replay",
					Interval:  1 * time.Hour,
					Enabled:   true,
					Task: func(ctx context.Context) error {
						_, err := gateway.Fetch(ctx, "twse_replay")
						return err
					},
				})
				log.Printf("[Gateway] registered channel_health_twse_replay background task (1h interval)")
			}

			// Register TSMC Revenue task via Gateway.
			if cfg.FinMindAPIKey != "" {
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:      "tsmc_revenue",
					ChannelID: "tsmc_revenue",
					Interval:  24 * time.Hour,
					Enabled:   true,
					Task: func(ctx context.Context) error {
						_, err := gateway.Fetch(ctx, "tsmc_revenue")
						return err
					},
				})
				log.Printf("[Gateway] registered tsmc_revenue background task (24h interval)")
			}

			// Register auto_backfill via Gateway.
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:      "auto_backfill",
				ChannelID: "twse_replay",
				Interval:  24 * time.Hour,
				Enabled:   true,
				Task: func(ctx context.Context) error {
					absWorkDir, err := filepath.Abs(cfg.WorkDir)
					if err != nil {
						absWorkDir = cfg.WorkDir
					}
					latestDate, err := getLatestReplayDate(cfg.ReplayDataPath)
					if err != nil {
						return fmt.Errorf("backfill replay read: %w", err)
					}
					now := time.Now()
					if tz, err := time.LoadLocation("Asia/Taipei"); err == nil {
						now = now.In(tz)
					}
					end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
					if now.Hour() < 15 || (now.Hour() == 15 && now.Minute() < 30) {
						end = end.AddDate(0, 0, -1)
					}
					start := latestDate.AddDate(0, 0, 1)
					for start.Weekday() == time.Saturday || start.Weekday() == time.Sunday {
						start = start.AddDate(0, 0, 1)
					}
					for end.Weekday() == time.Saturday || end.Weekday() == time.Sunday {
						end = end.AddDate(0, 0, -1)
					}
					if start.After(end) {
						return nil
					}
					startStr := start.Format("2006-01-02")
					endStr := end.Format("2006-01-02")
					log.Printf("[Gateway] backfill gap detected: %s to %s", startStr, endStr)
					bgCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
					defer cancel()
					var cmd *exec.Cmd
					binaryPath := filepath.Join(absWorkDir, "daily-replay-sync")
					if _, err := os.Stat(binaryPath); err == nil {
						cmd = exec.CommandContext(bgCtx, binaryPath, "-csv", cfg.ReplayDataPath, "-backfill-start", startStr, "-backfill-end", endStr)
						cmd.Dir = absWorkDir
					} else if _, err := exec.LookPath("go"); err == nil {
						cmd = exec.CommandContext(bgCtx, "go", "run", "./cmd/daily-replay-sync", "-csv", cfg.ReplayDataPath, "-backfill-start", startStr, "-backfill-end", endStr)
						cmd.Dir = absWorkDir
					} else {
						return fmt.Errorf("backfill binary not found")
					}
					out, err := cmd.CombinedOutput()
					if err != nil {
						return fmt.Errorf("backfill failed: %w, output: %s", err, string(out))
					}
					log.Printf("[Gateway] backfill success: %s", string(out))

					// Auto-convert CSV to JSONL so the system's replay pipeline
					// (tw_extended_90days.jsonl) stays in sync with the CSV that
					// daily-replay-sync appends to.  JSONL is the canonical format
					// consumed by FactorEngine (composition.go:67).
					absCSV := cfg.ReplayDataPath
					absJSONL := strings.TrimSuffix(cfg.ReplayDataPath, ".csv") + ".jsonl"
					if !filepath.IsAbs(absCSV) {
						absCSV = filepath.Join(absWorkDir, absCSV)
						absJSONL = filepath.Join(absWorkDir, absJSONL)
					}
					if convErr := importer.ImportTWOpenDataCSVToJSONL(absCSV, absJSONL); convErr != nil {
						log.Printf("[Gateway] backfill CSV→JSONL conversion warning (non-fatal): %v", convErr)
					} else {
						log.Printf("[Gateway] backfill CSV→JSONL conversion: %s", absJSONL)
					}
					return nil
				},
			})
			log.Printf("[Gateway] registered auto_backfill background task (24h interval)")

			// Register fundamentals_staleness_check: fundamentals.json is reference
			// data (PE/PB/DividendYield for 1070 stocks) loaded by FactorEngine at
			// startup.  It does not change daily—quarterly refresh is appropriate.
			// This task alerts when the file exceeds 90 days without an update.
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "fundamentals_staleness_check",
				Interval: 24 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					path := filepath.Join(cfg.WorkDir, "data", "fundamentals.json")
					info, err := os.Stat(path)
					if err != nil {
						monitor.Alert(monitoring.AlertLevelWarning, "data_staleness",
							fmt.Sprintf("fundamentals.json not accessible: %v", err),
							map[string]any{"file": path})
						return nil
					}
					ageDays := int(time.Since(info.ModTime()).Hours() / 24)
					if ageDays > 90 {
						monitor.Alert(monitoring.AlertLevelWarning, "data_staleness",
							fmt.Sprintf("fundamentals.json is %d days old — run: go run ./cmd/backfill-financial-statements", ageDays),
							map[string]any{"file": path, "age_days": ageDays})
					}
					return nil
				},
			})
			log.Printf("[Gateway] registered fundamentals_staleness_check background task (24h interval)")

			// Register auto_capital_flow via Gateway.
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:      "auto_capital_flow",
				ChannelID: "twse_capital_flow",
				Interval:  30 * time.Minute,
				Enabled:   true,
				Task: func(ctx context.Context) error {
					now := time.Now()
					if tz, err := time.LoadLocation("Asia/Taipei"); err == nil {
						now = now.In(tz)
					}
					if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
						return nil
					}
					hour := now.Hour()
					if hour < 9 || hour >= 16 {
						return nil
					}
					_, err := gateway.Fetch(ctx, "twse_capital_flow")
					return err
				},
			})
			log.Printf("[Gateway] registered auto_capital_flow background task (30m interval)")

			// Register auto_margin via Gateway.
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:      "auto_margin",
				ChannelID: "twse_margin",
				Interval:  30 * time.Minute,
				Enabled:   true,
				Task: func(ctx context.Context) error {
					now := time.Now()
					if tz, err := time.LoadLocation("Asia/Taipei"); err == nil {
						now = now.In(tz)
					}
					if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
						return nil
					}
					hour := now.Hour()
					if hour < 9 || hour >= 16 {
						return nil
					}
					_, err := gateway.Fetch(ctx, "twse_margin")
					return err
				},
			})
			log.Printf("[Gateway] registered auto_margin background task (30m interval)")

			// Register margin_history_backfill via Gateway.
			if err := taskMgr.Register(&apigateway.ScheduledTask{
				Name:      "margin_history_backfill",
				ChannelID: "twse_margin",
				Interval:  24 * time.Hour,
				Enabled:   true,
				Task: func(ctx context.Context) error {
					backfiller := narrative.NewMarginHistoryBackfiller(cfg.WorkDir)
					return backfiller.Backfill(ctx)
				},
			}); err != nil {
				log.Printf("[Gateway] failed to register margin_history_backfill: %v", err)
			} else {
				log.Printf("[Gateway] registered margin_history_backfill background task (24h interval)")
			}

			// Register auto_export via Gateway.
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:      "auto_export",
				ChannelID: "export_statistics",
				Interval:  12 * time.Hour,
				Enabled:   true,
				Task: func(ctx context.Context) error {
					_, err := gateway.Fetch(ctx, "export_statistics")
					return err
				},
			})
			log.Printf("[Gateway] registered auto_export background task (12h interval)")

			// Register auto_geopolitical via Gateway.
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:      "auto_geopolitical",
				ChannelID: "geopolitical",
				Interval:  6 * time.Hour,
				Enabled:   true,
				Task: func(ctx context.Context) error {
					adapter := apigateway.NewGeopoliticalChannelAdapter(cfg.WorkDir)
					_, err := adapter.Fetch(ctx)
					return err
				},
			})
			log.Printf("[Gateway] registered auto_geopolitical background task (6h interval)")

			// Register storage_cleanup via LifecycleManager.
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "storage_cleanup",
				Interval: 24 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					report, err := lifecycleMgr.Run(ctx, false)
					if err != nil {
						return fmt.Errorf("storage cleanup: %w", err)
					}
					log.Printf("[StorageCleanup] processed %d policies: %d files deleted, %d kept",
						len(report.Policies), report.TotalDeleted, report.TotalKept)
					return nil
				},
			})
			log.Printf("[Gateway] registered storage_cleanup background task (24h interval)")

			if svc := dashboard.GetIndustryService(); svc != nil {
				var finmindClient *marketdata.FinMindClient
				if cfg.FinMindAPIKey != "" {
					finmindClient = marketdata.GetSharedFinMindClient(cfg.FinMindAPIKey)
				}
				cycleAggregator := industry.NewDataAggregator(svc.CycleTracker, svc.Classifier, finmindClient)
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "auto_cycle_update",
					Interval: 6 * time.Hour,
					Enabled:  true,
					Task: func(ctx context.Context) error {
						bgCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
						defer cancel()
						return cycleAggregator.AggregateAllIndustries(bgCtx)
					},
				})
				log.Printf("[Gateway] registered auto_cycle_update background task (6h interval)")

				calendarProvider := marketdata.NewTWSECalendarProvider()
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "auto_calendar_refresh",
					Interval: 24 * time.Hour,
					Enabled:  true,
					Task: func(ctx context.Context) error {
						bgCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
						defer cancel()
						svc.EventCalendar.UpdateFromProvider(bgCtx, calendarProvider)
						svc.EventCalendar.RefreshEvents(time.Now())
						logging.Info("calendar", "auto_calendar_refresh completed")
						return nil
					},
				})
				log.Printf("[Gateway] registered auto_calendar_refresh background task (24h interval)")
			}

			{
				revenuePath := filepath.Join(cfg.WorkDir, "data", "replay", "month_revenue.jsonl")
				configPath := filepath.Join(cfg.WorkDir, "configs", "parameters.json")
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "auto_threshold_calibrate",
					Interval: 24 * time.Hour,
					Enabled:  true,
					Task: func(ctx context.Context) error {
						now := time.Now()
						if tz, err := time.LoadLocation("Asia/Taipei"); err == nil {
							now = now.In(tz)
						}
						if now.Day() != 1 || now.Hour() < 3 {
							return nil
						}
						if _, err := os.Stat(revenuePath); os.IsNotExist(err) {
							return nil
						}
						return industry.RecalibrateThresholds(revenuePath, configPath)
					},
				})
				log.Printf("[Gateway] registered auto_threshold_calibrate background task (24h interval, checks 1st of month)")
			}

			{
				dashRef := dashboard
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "macro_ingest",
					Interval: 5 * time.Minute,
					Enabled:  true,
					Task: func(ctx context.Context) error {
						ingestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
						defer cancel()
						_, snap, err := dashRef.IngestAndUpdateMacro(ingestCtx)
						if err != nil {
							logging.Warn("main", "macro_ingest_failed", "err", err)
							return err
						}
						// Crisis circuit break: VIX >= 35 triggers force-open on live channels.
						if gateway != nil && snap.VIX.Value >= 35.0 {
							liveChannels := []string{"fugle", "fubon", "finmind"}
							for _, ch := range liveChannels {
								if err := gateway.ForceOpenChannel(ch); err != nil {
									logging.Warn("main", "crisis_force_open_failed", "channel", ch, "err", err)
								} else {
									logging.Info("main", "crisis_force_open", "channel", ch, "vix", snap.VIX.Value)
								}
							}
						}
						// Propagate VIX signal to optimizer crisis mode.
						dashRef.InvokeCrisisModeSetter(snap.VIX.Value >= 35.0)
						// Feed SPX/SOX daily returns into rolling correlation engine.
						if svc := dashRef.GetCrossMarketService(); svc != nil {
							svc.UpdateCorrelation(snap.SPXIndex.ChangePct, snap.SOXIndex.ChangePct)
						}
						// EventLogic cross-market rule evaluation against live data.
						if elValidator != nil && snap.RecordedAt > 0 {
							themes := narrativeActiveThemes(narLifecycleMgr)
							fired := eventlogic.EvaluateActiveRules(elValidator, snap, themes)
							if len(fired) > 0 {
								logging.Info("main", "eventlogic_fired", "count", len(fired), "rules", fired)
							}
						}
						return nil
					},
				})
				log.Printf("[Gateway] registered macro_ingest background task (5m interval)")
			}

			// RealTimeAdapter feed: periodically ingest market data points from
			// the latest macro snapshot for sub-second regime detection.
			if realtimeAdapter != nil {
				dashRef := dashboard
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "realtime_feed",
					Interval: 30 * time.Second,
					Enabled:  true,
					Task: func(ctx context.Context) error {
						snap, ok := dashRef.GetLatestMacroSnapshot()
						if !ok {
							return nil
						}
						now := time.Now()
						points := []realtime.MarketDataPoint{
							{Symbol: "SOX", Price: snap.SOXIndex.Value, Timestamp: now},
							{Symbol: "VIX", Price: snap.VIX.Value, Timestamp: now},
						}
						if snap.SPXIndex.Value > 0 {
							points = append(points,
								realtime.MarketDataPoint{Symbol: "SPX", Price: snap.SPXIndex.Value, Timestamp: now},
								realtime.MarketDataPoint{Symbol: "NDX", Price: snap.NDXIndex.Value, Timestamp: now},
							)
						}
						for _, p := range points {
							if p.Price > 0 {
								realtimeAdapter.IngestData(p)
							}
						}
						return nil
					},
				})
				log.Printf("[Gateway] registered realtime_feed background task (30s interval)")
			}

			// Silicon cycle indicator update (10m, offset from macro_ingest 5m
			// to ensure fresh TSMC/SOX data). Uses the macro data pipeline already
			// maintained by macro_ingest — no additional external API calls.
			if industrySvc := dashboard.GetIndustryService(); industrySvc != nil && industrySvc.SiliconTracker != nil {
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "silicon_cycle_update",
					Interval: 10 * time.Minute,
					Enabled:  true,
					Task: func(ctx context.Context) error {
						return industrySvc.UpdateSiliconIndicators(ctx)
					},
				})
				log.Printf("[Gateway] registered silicon_cycle_update background task (10m interval)")
			}

			// Narrative model + template hit-rate self-calibration (24h).
			{
				dashRef := dashboard
				replayCSV := cfg.ReplayDataPath
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "narrative_calibrate",
					Interval: 24 * time.Hour,
					Enabled:  true,
					Task: func(ctx context.Context) error {
						report, err := dashRef.CalibrateNarrative(replayCSV)
						if err != nil {
							logging.Warn("main", "narrative_calibrate_failed", "err", err)
							return nil
						}
						logging.Info(
							"main", "narrative_calibrate_ok",
							"verdict", report.Verdict,
							"models_updated", report.ModelsUpdated,
							"templates_updated", report.TemplatesUpdated,
						)
						return nil
					},
				})
				log.Printf("[Gateway] registered narrative_calibrate background task (24h interval)")
			}

			if repo != nil {
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "metrics_snapshot",
					Interval: 60 * time.Second,
					Enabled:  true,
					Task: func(ctx context.Context) error {
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
						return repo.SaveSnapshot(ctx, &repoSnap)
					},
				})
				log.Printf("[Gateway] registered metrics_snapshot background task (60s interval)")
			}

			// Register auto_daily_simulation — runs daily simulation at market close.
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "auto_daily_simulation",
				Interval: 24 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					// Determine next market-close time (Asia/Taipei 13:30 weekdays).
					now := time.Now()
					if tz, err := time.LoadLocation("Asia/Taipei"); err == nil {
						now = now.In(tz)
					}
					nextClose := time.Date(now.Year(), now.Month(), now.Day(), 13, 30, 0, 0, now.Location())
					if now.Before(nextClose) {
						nextClose = nextClose.Add(-24 * time.Hour)
					}
					for nextClose.Weekday() == time.Saturday || nextClose.Weekday() == time.Sunday {
						nextClose = nextClose.AddDate(0, 0, -1)
					}
					log.Printf("[Simulation] auto trigger: %s", nextClose.Format("2006-01-02"))

					system, err := orchestrator.NewProductionSystemWithEventBus(cfg, dashEventBus, janusEngine)
					if err != nil {
						return fmt.Errorf("create system: %w", err)
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
					if dashboard != nil {
						system.SetDrawdownReporter(func(d portfolio.DrawdownResult) {
							dashboard.SetLatestDrawdown(&d)
						})
					}

					capitalCfg := domain.DefaultCapitalPhaseConfig()
					capitalCfg.PhaseStartDate = nextClose.Add(-30 * 24 * time.Hour)
					controller := risk.NewCapitalPhaseController(capitalCfg)
					allocator := portfolio.NewCapitalAllocator()
					workflow, err := risk.NewApprovalWorkflow("data/state/approvals")
					if err != nil {
						return fmt.Errorf("create approval workflow: %w", err)
					}
					system.WithCapitalManagement(controller, allocator, workflow)

					result, err := system.RunDailySimulation(nextClose)
					if err != nil {
						return fmt.Errorf("simulation failed: %w", err)
					}

					candidate, err := system.NextExperimentCandidate()
					if err != nil {
						logging.Warn("simulation", "candidate_failed", "err", err.Error())
					}
					if err := system.RecordSessionSummary(result, candidate); err != nil {
						return fmt.Errorf("record session: %w", err)
					}

					// Record cycle calibration outcome for layer accuracy tracking.
					// Uses the composite card sentiment signals against the actual
					// portfolio return to measure which layers were directionally correct.
					if dashboard != nil && dashboard.GetIndustryService() != nil {
						card, cardErr := dashboard.GetIndustryService().BuildCycleStatusCard(nextClose)
						if cardErr == nil && card != nil {
							signals := map[string]float64{
								"silicon":        card.SiliconScore,
								"business_cycle": card.CycleConfidence,
								"seasonal":       card.SeasonalAdjustment,
								"events":         card.EventSentiment,
								"supply_chain":   card.SupplyChainSignal,
							}
							dashboard.RecordCycleCalibrationOutcome(
								system.Session().ID, nextClose, signals, result.BeforeTaxPnL,
							)
						}
					}

					logging.Info(
						"simulation", "completed",
						"session", system.Session().ID,
						"regime", result.Regime,
						"orders", len(result.Orders),
						"positions", len(result.Positions),
					)
					// Quality alerts
					if len(result.Orders) == 0 {
						monitor.Alert(monitoring.AlertLevelWarning, "simulation",
							fmt.Sprintf("場次 %s 產生 0 筆訂單（regime=%s, positions=%d）",
								system.Session().ID, result.Regime, len(result.Positions)),
							map[string]any{
								"session":   system.Session().ID,
								"regime":    string(result.Regime),
								"orders":    0,
								"positions": len(result.Positions),
							})
					}
					return nil
				},
			})
			log.Printf("[Gateway] registered auto_daily_simulation background task (24h interval)")

			// Register etf_nav_refresh — verify ETF data freshness in replay after daily sync.
			// ETF NAV calibration from replay data happens automatically at each system startup
			// (see orchestrator/system.go). This task ensures replay data stays fresh and alerts
			// if ETF symbols are missing from the dataset.
			// Data source priority: TWSE OpenAPI (primary) → Fubon → Fugle → FinMind.
			// Compliance: CONSTITUTION.md Article 4 — BackgroundTaskManager registration.
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:      "etf_nav_refresh",
				ChannelID: "twse_replay",
				Interval:  24 * time.Hour,
				Enabled:   true,
				Task: func(ctx context.Context) error {
					_, err := gateway.Fetch(ctx, "twse_replay")
					if err != nil {
						monitor.Alert(monitoring.AlertLevelWarning, "etf_nav",
							fmt.Sprintf("TWSE replay data fetch failed: %v", err),
							map[string]any{"channel": "twse_replay"})
						return fmt.Errorf("etf_nav_refresh fetch: %w", err)
					}
					logging.Info("etf_nav_refresh", "completed",
						"etf_symbols", len(orchestrator.DefaultSymbols()),
						"hint", "ETF NAV is calibrated at next system startup from replay close prices")
					return nil
				},
			})
			log.Printf("[Gateway] registered etf_nav_refresh background task (24h interval)")

			// Register stress_test_daily — run multi-day stress scenarios after market close (P3-5).
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "stress_test_daily",
				Interval: 24 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					system, err := orchestrator.NewProductionSystemWithEventBus(cfg, dashEventBus, janusEngine)
					if err != nil {
						return fmt.Errorf("create system for stress test: %w", err)
					}
					if gatewayFetcher != nil {
						system.Sim().SetProvider(orchestrator.NewGatewayBackedProvider(cfg))
					}
					if repo != nil {
						system.SetRepository(repo)
					}
					if dashboard != nil {
						system.SetDrawdownReporter(func(d portfolio.DrawdownResult) {
							dashboard.SetLatestDrawdown(&d)
						})
					}
					capitalCfg := domain.DefaultCapitalPhaseConfig()
					capitalCfg.PhaseStartDate = time.Now().Add(-30 * 24 * time.Hour)
					ctrl := risk.NewCapitalPhaseController(capitalCfg)
					alloc := portfolio.NewCapitalAllocator()
					wf, _ := risk.NewApprovalWorkflow("data/state/approvals")
					system.WithCapitalManagement(ctrl, alloc, wf)
					if _, simErr := system.RunDailySimulation(time.Now()); simErr != nil {
						logging.Warn("stress_test_daily", "simulation_failed", "err", simErr.Error())
					}
					return system.RunDailyStressTests()
				},
			})
			log.Printf("[Gateway] registered stress_test_daily background task (24h interval)")

			// monitorAdapter wraps *monitoring.Monitor to match AutoExperimentMonitor interface.
			monitorAdapter := &experimentMonitorAdapter{m: monitor}

			// Register auto_experiment — weekly strategy evolution cycle.
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "auto_experiment",
				Interval: 7 * 24 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					system, err := orchestrator.NewProductionSystemWithEventBus(cfg, dashEventBus, janusEngine)
					if err != nil {
						return fmt.Errorf("create system: %w", err)
					}
					if gatewayFetcher != nil {
						system.Sim().SetProvider(orchestrator.NewGatewayBackedProvider(cfg))
					}
					if repo != nil {
						system.SetRepository(repo)
					}
					if dashboard != nil {
						system.SetDrawdownReporter(func(d portfolio.DrawdownResult) {
							dashboard.SetLatestDrawdown(&d)
						})
					}
					return experiment.AutoExperiment(ctx, experiment.AutoExperimentConfig{
						System:  system,
						Config:  cfg,
						Monitor: monitorAdapter,
					})
				},
			})
			log.Printf("[Gateway] registered auto_experiment background task (7-day interval)")

			// Register window_backtest — periodic 20-day scoring window (7-day interval, offset 3d).
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "window_backtest",
				Interval: 7 * 24 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					store := ledger.NewStore(cfg.LedgerDir)
					btRunner := backtest.NewRunner(cfg, store)
					endDate := time.Now().AddDate(0, 0, -3)
					startDate := endDate.AddDate(0, 0, -20)
					logging.Info("window_backtest", "running", "start", startDate.Format("2006-01-02"), "end", endDate.Format("2006-01-02"))
					summary, err := btRunner.Run(startDate, endDate)
					if err != nil {
						return fmt.Errorf("window backtest: %w", err)
					}
					if _, err := btRunner.GenerateReport(summary); err != nil {
						logging.Warn("window_backtest", "report_failed", "err", err.Error())
					}
					logging.Info("window_backtest", "completed",
						"sessions", summary.SessionCount,
						"outcomes", summary.OutcomeCount,
						"worst_agent", summary.WorstAgentID)
					return nil
				},
			})
			log.Printf("[Gateway] registered window_backtest background task (7-day interval)")

			if ruleEngine != nil {
				params := config.GetParametersConfig().Alert
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "rule_engine_check",
					Interval: time.Duration(params.RuleEngineIntervalSec.Value) * time.Second,
					Enabled:  true,
					Task: func(ctx context.Context) error {
						ruleEngine.EvaluateRules(nil)
						return nil
					},
				})
				log.Printf("[Gateway] registered rule_engine_check background task (%ds interval)", params.RuleEngineIntervalSec.Value)
			}

			{
				mlScheduler := scheduler.NewMLRetrainScheduler(cfg.ReplayDataPath)
				mlScheduler.SetWorkDir(cfg.WorkDir)
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "ml_retrain",
					Interval: 24 * time.Hour,
					Enabled:  true,
					Task: func(ctx context.Context) error {
						return mlScheduler.RetrainAll(ctx)
					},
				})
				log.Printf("[Gateway] registered ml_retrain background task (24h interval)")
			}

			riskGate := risk.NewRiskGate(risk.NewPreTradeGate(), risk.NewInTradeGate(), risk.NewPostTradeGate())
			if maturityTracker != nil {
				riskGate.WithMaturityTracker(maturityTracker)
			}
			dashboard.SetRiskGate(riskGate)

			if params := config.GetParametersConfig(); params != nil && params.RSITw.LastCalibratedScore.Value > 0 {
				riskGate.SetPreTradeRSITwScore(params.RSITw.LastCalibratedScore.Value)
				log.Printf("[RiskGate] restored RSI-tw calibration score: %.4f", params.RSITw.LastCalibratedScore.Value)
			}

			elRulesPath = filepath.Join(cfg.WorkDir, "data/state/eventlogic", "rules.json")
			elHistoryRecorder = eventlogic.NewHistoryRecorder(filepath.Join(cfg.WorkDir, "data/state/eventlogic", "history.jsonl"))
			elRegistry := eventlogic.LoadOrDefault(elRulesPath)
			elValidator = eventlogic.NewValidator(elRegistry)
			elDetector = eventlogic.NewDetector(elRegistry)
			elCorrector = eventlogic.NewCorrector(elRegistry)
			elHandlers := apieventlogic.NewHandlers(elRegistry, elValidator, elDetector)
			dashboard.SetEventLogicHandlers(elHandlers)
			elRegistry.MustSave(elRulesPath)
			elHistoryRecorder.SnapshotAll(elRegistry)
			log.Printf("[EventLogic] loaded %d rules from %s", elRegistry.Count(), elRulesPath)

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

			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "eventlogic_auto_discover",
				Interval: 168 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					candidates := elDetector.DiscoverPatterns(nil)
					logging.Info("eventlogic_discover", "completed", "candidates", len(candidates))
					for _, c := range candidates {
						if rule, err := elDetector.PromoteCandidate(&c); err == nil {
							logging.Info("eventlogic_discover", "promoted", "rule_id", rule.ID)
						}
					}
					return nil
				},
			})
			log.Printf("[EventLogic] weekly auto-discover background task registered")

			log.Printf("[RiskGate] injected into DashboardAPI for calibration reports")
			calProvider := monitoring.NewSessionCalibrationProvider(filepath.Join(cfg.WorkDir, "data/state"))
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "risk_gate_calibrate",
				Interval: 24 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					report, err := riskGate.SelfCalibrate(ctx, calProvider, 30)
					if err != nil {
						logging.Error("risk_calibrate", "self_calibrate_failed", "err", err.Error())
						return err
					}
					riskGate.SetLastCalibration(report)
					logging.Info("risk_calibrate", "completed",
						"verdict", report.Verdict,
						"changes", len(report.Changes),
						"summary", report.Summary)
					for _, ch := range report.Changes {
						logging.Info("risk_calibrate", "parameter_change",
							"param", ch.Name,
							"before", ch.Before,
							"after", ch.After,
							"rationale", ch.Rationale,
							"confidence", ch.Confidence)
					}
					return nil
				},
			})
			log.Printf("[Gateway] registered risk_gate_calibrate background task (24h interval)")

			if svc := dashboard.GetIndustryService(); svc != nil && svc.CycleCalibration != nil {
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "cycle_calibrate",
					Interval: 24 * time.Hour,
					Enabled:  true,
					Task: func(ctx context.Context) error {
						defaultCfg := industry.CardConfig{
							LayerWeights: map[string]float64{
								"silicon":        0.25,
								"business_cycle": 0.20,
								"seasonal":       0.15,
								"events":         0.15,
								"supply_chain":   0.10,
							},
						}
						calibrated := svc.CycleCalibration.CalibrateWeights(defaultCfg.LayerWeights)
						metrics := svc.CycleCalibration.GetMetrics()

						logging.Info("cycle_calibrate", "completed",
							"outcomes", svc.CycleCalibration.GetOutcomeCount(),
							"layers", len(calibrated))
						for layer, m := range metrics {
							logging.Info("cycle_calibrate", "layer_accuracy",
								"layer", layer,
								"accuracy", m.Accuracy,
								"signals", m.TotalSignals)
						}
						return nil
					},
				})
				log.Printf("[Gateway] registered cycle_calibrate background task (24h interval)")
			}

			if janusEngine != nil {
				var prevRegime string
				regimeScenario := map[string]string{
					"NOVEL_REGIME":      "ai_bubble_2024",
					"HISTORICAL_REGIME": "normal_market_2024",
					"RISK_OFF":          "covid_crash_2020",
				}
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "regime_calibrate",
					Interval: 1 * time.Hour,
					Enabled:  true,
					Task: func(ctx context.Context) error {
						class := janusEngine.GetRegimeClassification()
						current := string(class)
						if current == "" || current == "MIXED" {
							prevRegime = current
							return nil
						}
						if current != prevRegime && prevRegime != "" {
							scenario := regimeScenario[current]
							if scenario == "" {
								scenario = "fed_hikes_2022"
							}
							logging.Info("regime_calibrate", "regime_change_detected",
								"from", prevRegime, "to", current,
								"suggested_stress_scenario", scenario)
							report, err := riskGate.SelfCalibrate(ctx, calProvider, 20)
							if err != nil {
								logging.Error("regime_calibrate", "calibrate_after_regime_change_failed", "err", err.Error())
								return nil
							}
							riskGate.SetLastCalibration(report)
							logging.Info("regime_calibrate", "calibration_after_regime_change",
								"verdict", report.Verdict, "changes", len(report.Changes))
						}
						prevRegime = current
						return nil
					},
				})
				log.Printf("[Gateway] registered regime_calibrate background task (1h interval, triggers calibration on regime change)")
			}

			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "factor_weight_calibrate",
				Interval: 24 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					orders, err := loadCalibrationOrders(cfg.WorkDir)
					if err != nil {
						return nil
					}
					report, err := portfolio.CalibrateWeights(ctx, orders)
					if err != nil {
						return nil
					}
					logging.Info("fw_calibrate", "completed",
						"verdict", report.Verdict,
						"improvement", report.ImprovementPct,
						"changes", len(report.Changes),
						"orders", report.OrdersEvaluated)
					for _, ch := range report.Changes {
						logging.Info("fw_calibrate", "weight_change",
							"factor", string(ch.Factor),
							"before", ch.Before,
							"after", ch.After,
							"delta", ch.DeltaPct,
							"confidence", ch.Confidence)
					}
					return nil
				},
			})
			log.Printf("[Gateway] registered factor_weight_calibrate background task (24h interval)")

			// Register conviction_calibrate — optimizes factor-driven conviction
			// thresholds and deltas (FactorConvictionParams) using historical
			// recommendation outcomes with forward returns.
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "conviction_calibrate",
				Interval: 24 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					return orchestrator.RunConvictionCalibration(
						cfg.WorkDir,
						orchestrator.SemiconductorExecutor{},
						orchestrator.AISupplyChainExecutor{},
						orchestrator.LEOSatelliteExecutor{},
						orchestrator.ETFRotationExecutor{},
						orchestrator.FinancialsExecutor{},
						orchestrator.ShippingExecutor{},
						orchestrator.ValueYieldExecutor{},
						orchestrator.EarningsQualityExecutor{},
						orchestrator.TechnicalBreakoutExecutor{},
						orchestrator.GrowthMomentumExecutor{},
					)
				},
			})
			log.Printf("[Gateway] registered conviction_calibrate background task (24h interval)")

			// Register macro_risk_calibrate — calibrates Engine MacroRisk thresholds
			// (carry_trade, VIX, US10Y, oil, gold, DXY, TWD, outflow probabilities).
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "macro_risk_calibrate",
				Interval: 24 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					cal := config.NewMacroRiskCalibrator()
					evaluator := cal.BuildEvaluator()
					result, err := config.CalibrateParameters(ctx, cal, evaluator, config.DefaultCalibrateConfig())
					if err != nil {
						logging.Error("macro_risk_calibrate", "failed", "err", err.Error())
						return err
					}
					logging.Info("macro_risk_calibrate", "completed",
						"verdict", result.Verdict,
						"changes", len(result.Changes),
						"summary", result.Summary)
					for _, ch := range result.Changes {
						logging.Info("macro_risk_calibrate", "param_change",
							"param", ch.ParamName,
							"before", ch.Before,
							"after", ch.After,
							"delta", ch.DeltaPct,
							"confidence", ch.Confidence)
					}
					return nil
				},
			})
			log.Printf("[Gateway] registered macro_risk_calibrate background task (24h interval)")

			// Register structural_trend_calibrate — calibrates Engine StructuralTrend
			// thresholds (trend strength, confidence, override, AI/capex/utilization).
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "structural_trend_calibrate",
				Interval: 24 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					cal := &config.StructuralTrendCalibrator{}
					evaluator := cal.BuildEvaluator()
					result, err := config.CalibrateParameters(ctx, cal, evaluator, config.DefaultCalibrateConfig())
					if err != nil {
						logging.Error("structural_trend_calibrate", "failed", "err", err.Error())
						return err
					}
					logging.Info("structural_trend_calibrate", "completed",
						"verdict", result.Verdict,
						"changes", len(result.Changes),
						"summary", result.Summary)
					for _, ch := range result.Changes {
						logging.Info("structural_trend_calibrate", "param_change",
							"param", ch.ParamName,
							"before", ch.Before,
							"after", ch.After,
							"delta", ch.DeltaPct,
							"confidence", ch.Confidence)
					}
					return nil
				},
			})
			log.Printf("[Gateway] registered structural_trend_calibrate background task (24h interval)")

			// Register narrative_calibrate — calibrates Narrative event detection
			// thresholds (AI revenue, CoWoS, capex, US10Y, DXY, GPR, oil, JPY, VIX).
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "narrative_calibrate",
				Interval: 24 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					nc := &config.NarrativeCalibrator{}
					evaluator := config.NewNarrativeEvaluator()
					result, err := config.CalibrateParameters(ctx, nc, evaluator, config.DefaultCalibrateConfig())
					if err != nil {
						logging.Error("narrative_calibrate", "failed", "err", err.Error())
						return err
					}
					logging.Info("narrative_calibrate", "completed",
						"verdict", result.Verdict,
						"changes", len(result.Changes),
						"summary", result.Summary)
					for _, ch := range result.Changes {
						logging.Info("narrative_calibrate", "param_change",
							"param", ch.ParamName,
							"before", ch.Before,
							"after", ch.After,
							"delta", ch.DeltaPct,
							"confidence", ch.Confidence)
					}
					return nil
				},
			})
			log.Printf("[Gateway] registered narrative_calibrate background task (24h interval)")

			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "seasonal_calibrate",
				Interval: 24 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					cmd := exec.CommandContext(ctx, "go", "run", "./cmd/calibrate-seasonal",
						"--replay", "data/replay/finmind_2020_2024.jsonl",
						"--start", "2020", "--end", "2024", "--update", "--update-threshold", "1")
					output, err := cmd.CombinedOutput()
					if err != nil {
						logging.Error("seasonal_calibrate", "failed", "err", err.Error(), "output", string(output))
						return err
					}
					logging.Info("seasonal_calibrate", "completed", "output", string(output))
					return nil
				},
			})
			log.Printf("[Gateway] registered seasonal_calibrate background task (24h interval)")

			// Register linkage_calibrate — calibrates RecessionShockAmplifier
			// for shock propagation amplification during recession regimes.
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "linkage_calibrate",
				Interval: 24 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					calibrator := &config.LinkageAmplifierCalibrator{}
					evaluator := func(cfg *config.ParametersConfig) (float64, error) {
						// TODO: Implement proper recession shock accuracy scoring
						// using historical session data. For now, return a
						// neutral score so the calibration infrastructure runs
						// end-to-end without changing the amplifier value.
						return 0.0, nil
					}
					result, err := config.CalibrateParameters(ctx, calibrator, evaluator, config.DefaultCalibrateConfig())
					if err != nil {
						logging.Error("linkage_calibrate", "calibration_failed", "err", err.Error())
						return err
					}
					logging.Info("linkage_calibrate", "completed",
						"baseline", fmt.Sprintf("%.3f", result.BaselineScore),
						"optimized", fmt.Sprintf("%.3f", result.OptimizedScore))
					return nil
				},
			})
			log.Printf("[Gateway] registered linkage_calibrate background task (24h interval)")

			// Register factor_weight_strategy_calibrate — calibrates FactorWeight
			// strategy deltas (conservative/aggressive/risk-on/risk-off adjustments).
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "factor_weight_strategy_calibrate",
				Interval: 24 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					result, err := config.CalibrateStrategyDeltas(ctx, config.DefaultCalibrateConfig())
					if err != nil {
						logging.Error("fw_strategy_calibrate", "failed", "err", err.Error())
						return err
					}
					logging.Info("fw_strategy_calibrate", "completed",
						"verdict", result.Verdict,
						"changes", len(result.Changes),
						"summary", result.Summary)
					for _, ch := range result.Changes {
						logging.Info("fw_strategy_calibrate", "delta_change",
							"param", ch.ParamName,
							"before", ch.Before,
							"after", ch.After,
							"delta", ch.DeltaPct,
							"confidence", ch.Confidence)
					}
					return nil
				},
			})
			log.Printf("[Gateway] registered factor_weight_strategy_calibrate background task (24h interval)")

			// Register auto_calibrate — runs Darwinian parameter calibration from
			// historical session outcome data (7-day interval).
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "auto_calibrate",
				Interval: 7 * 24 * time.Hour,
				Jitter:   4 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					cmd := exec.CommandContext(ctx, "go", "run", "./cmd/calibrate-parameters",
						"--module=darwinian")
					cmd.Dir = cfg.WorkDir
					out, err := cmd.CombinedOutput()
					if err != nil {
						logging.Warn("auto_calibrate", "failed",
							logging.Err(err),
							logging.FStr("output", string(out)))
						return nil
					}
					logging.Info("auto_calibrate", "completed")
					return nil
				},
			})
			log.Printf("[Gateway] registered auto_calibrate background task (7-day interval)")

			// Register auto_swarm_simulation — periodic swarm simulation
			// for training data generation and scenario monitoring.
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "auto_swarm_simulation",
				Interval: 30 * time.Minute,
				Jitter:   3 * time.Minute,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					sys, err := orchestrator.NewProductionSystemWithEventBus(cfg, dashEventBus, janusEngine)
					if err != nil {
						return fmt.Errorf("create system for swarm: %w", err)
					}
					if gatewayFetcher != nil {
						sys.Sim().SetProvider(orchestrator.NewGatewayBackedProvider(cfg))
					}
					ctrl := sys.Phase3Controller()
					if ctrl == nil {
						return nil
					}
					trainingDir := filepath.Join(cfg.WorkDir, "data/state/swarm_training")
					ctrl.SetTrainingStore(swarm.NewTrainingStore(trainingDir))
					ctrl.SetSnapshotPath(filepath.Join(cfg.WorkDir, "data/state/swarm_latest.json"))
					ctrl.SetMetaLearner(metalearning.NewMetaLearner(metalearning.DefaultMetaLearningConfig()),
						filepath.Join(cfg.WorkDir, "data/state/metalearner_state.json"))

					baseState := swarm.MarketState{
						Timestamp: time.Now(),
						Prices:    make(map[string]float64),
						Volumes:   make(map[string]float64),
					}
					// Seed with common TWSE symbols and placeholder prices.
					for _, sym := range []string{"2330.TW", "2317.TW", "2454.TW", "2412.TW", "2308.TW"} {
						baseState.Prices[sym] = 100.0
						baseState.Volumes[sym] = 5000000
					}
					ctrl.RunSwarmCycle(baseState)
					logging.Info("swarm_btm", "cycle_completed", "symbols", len(baseState.Prices))
					return nil
				},
			})
			log.Printf("[Gateway] registered auto_swarm_simulation background task (30m interval)")

			// RSI-tw autonomous calibration — runs every 24h at market close
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "rsi_tw_calibrate",
				Interval: 24 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					report, err := retail.CalibrateRSITw(cfg.WorkDir)
					if err != nil {
						log.Printf("[RSITw] calibration failed: %v", err)
						return err
					}
					riskGate.SetPreTradeRSITwScore(report.Score)
					log.Printf("[RSITw] calibration complete: %s (score=%.4f, samples=%d, changes=%d)",
						report.Verdict, report.Score, report.SampleCount, len(report.Changes))
					return nil
				},
			})
			log.Printf("[Gateway] registered rsi_tw_calibrate background task (24h interval)")

			if realtimeAdapter != nil {
				go realtimeAdapter.Start(sysCtx)
				log.Printf("[RealTime] adapter started (cadence=%dms)", paramsCfg.Realtime.UpdateIntervalMs.Value)
			}

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
		return runLiveTrading(cfg, deps, collector, repo)
	}
	return runSimulation(cfg, false, collector, repo, deps.shutdown)
}

// staticHandler returns an http.Handler that serves static assets from the given fs.FS.
// It applies Cache-Control headers (immutable for hashed assets, no-cache for others)
// and implements SPA fallback (serves index.html for paths not matching any file).
func staticHandler(assets fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		cleanPath := filepath.Clean(r.URL.Path)
		// Serve hashed assets with long-lived cache
		if strings.Contains(cleanPath, "-") && (strings.HasSuffix(cleanPath, ".js") || strings.HasSuffix(cleanPath, ".css")) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			w.Header().Del("Pragma")
			w.Header().Del("Expires")
		}
		// SPA fallback: serve index.html for paths that don't match static files
		if _, err := fs.Stat(assets, strings.TrimPrefix(cleanPath, "/")); err != nil {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
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

func runLiveTrading(cfg config.Config, deps appDeps, collector *monitoring.MetricsCollector, repo *repository.DualWriteRepository) error {
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
	provider := marketdata.NewMockProvider()

	liveCfg := live.DefaultOrchestratorConfig()
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

func loadCalibrationOrders(workDir string) ([]portfolio.CalibratedOrder, error) {
	sessionsDir := filepath.Join(workDir, "data", "state", "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, err
	}
	var all []portfolio.CalibratedOrder
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		orders, err := portfolio.LoadOrdersFromJSONL(filepath.Join(sessionsDir, e.Name(), "recommendation_outcomes.jsonl"))
		if err != nil {
			continue
		}
		all = append(all, orders...)
	}
	return all, nil
}

// narrativeActiveThemes returns the active narrative themes from the lifecycle
// manager. Returns nil when the manager is not initialized.
func narrativeActiveThemes(mgr *narrative.EventLifecycleManager) []string {
	if mgr == nil {
		return nil
	}
	events := mgr.GetActiveEvents()
	themes := make([]string, len(events))
	for i, ev := range events {
		themes[i] = ev.Theme
	}
	return themes
}
