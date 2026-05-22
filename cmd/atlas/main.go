package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
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
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/autobacktest"
	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/bootstrap"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/evolution"
	"github.com/kaecer68/atlas-go/internal/experiment"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/live"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	apievents "github.com/kaecer68/atlas-go/internal/monitoring/api/events"
	apischeduler "github.com/kaecer68/atlas-go/internal/monitoring/api/scheduler"
	apishared "github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/repository"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/storage"
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

// getLatestReplayDate reads the replay CSV and returns the latest date.
func getLatestReplayDate(csvPath string) (time.Time, error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()

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

	var janusEngine *janus.Engine

	if *apiMode {
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
		dashboard := deps.newDashboardAPI(cfg.WorkDir, cfg.LedgerDir, collector)
		if d, ok := dashboard.(*monitoring.DashboardAPI); ok {
			d.SetPool(pool)
			d.SetHealthManager(portfolio.NewAgentHealthManagerWithStore(portfolio.DefaultAgentHealthConfig(), healthStore).WithParameters(runtimeParams))
			janusEngine = janus.NewEngine()
			janusEngine.EnsureAllRegimes()
			janusEngine.Update()
			d.SetJanusEngine(janusEngine)
			log.Printf("[JANUS] engine injected into dashboard API")
		}
		if repo != nil {
			if d, ok := dashboard.(*monitoring.DashboardAPI); ok {
				d.SetRepository(repo)
				log.Printf("[Repository] injected into dashboard API")
			}
		}
		// Inject EventBus for SSE streaming endpoint
		if d, ok := dashboard.(*monitoring.DashboardAPI); ok {
			eventBus := eventbus.NewChannelEventBus(256)
			d.SetEventBus(eventBus)
			d.SetContext(context.Background())
			log.Printf("[EventBus] injected into dashboard API for SSE streaming")
			eventBus.Subscribe(eventbus.EventNarrative, func(ctx context.Context, event eventbus.BusEvent) error {
				apievents.BufferNarrativeEvent(event)
				return nil
			})
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
			_, _, err := d.IngestAndUpdateMacro(ingestCtx)
			ingestCancel()
			if err != nil {
				logging.Warn("main", "initial_macro_ingest_failed", "err", err)
			} else {
				logging.Info("main", "initial_macro_ingest_ok")
			}
		}

		lifecycleMgr := storage.NewLifecycleManager(filepath.Join(cfg.WorkDir, "data/state"))
		if d, ok := dashboard.(*monitoring.DashboardAPI); ok {
			d.SetStorageReporter(lifecycleMgr)
			log.Printf("[Storage] reporter injected into dashboard API")
		}

		dashboard.RegisterRoutes(mux)

		if alertStore != nil {
			alertAPI := monitoring.NewAlertAPI(alertStore)
			alertAPI.RegisterRoutes(mux)
		}

		if taskManager != nil {
			if d, ok := dashboard.(*monitoring.DashboardAPI); ok {
				d.SetTaskManager(taskManager)
				d.RegisterTaskExecRoutes(mux)
				log.Printf("[TaskExec] injected into dashboard API")
			}
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
			cfg, err := config.ReloadEngineConfig()
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to reload config: %v", err), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
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
				fmt.Fprintf(w, `{"error":"%s"}`+"\n", err.Error())
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"status":"ok","message":"thresholds recalibrated"}`+"\n")
		}))
		var monitor *monitoring.Monitor
		mux.HandleFunc("/admin/trigger-simulation", adminHandler(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			system, err := orchestrator.NewProductionSystem(cfg)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, `{"error":"create system: %v"}`+"\n", err)
				return
			}
			if collector != nil {
				system.WithMetricsCollector(collector)
			}
			if repo != nil {
				system.SetRepository(repo)
			}
			capitalCfg := domain.DefaultCapitalPhaseConfig()
			capitalCfg.PhaseStartDate = time.Now().Add(-30 * 24 * time.Hour)
			controller := risk.NewCapitalPhaseController(capitalCfg)
			allocator := portfolio.NewCapitalAllocator()
			workflow, wErr := risk.NewApprovalWorkflow("data/state/approvals")
			if wErr != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, `{"error":"approval workflow: %v"}`+"\n", wErr)
				return
			}
			system.WithCapitalManagement(controller, allocator, workflow)
			result, simErr := system.RunDailySimulation(time.Now())
			if simErr != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
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
			fmt.Fprintf(w, `{"status":"ok","session":"%s","regime":"%s","orders":%d,"positions":%d}`+"\n",
				system.Session().ID, result.Regime, len(result.Orders), len(result.Positions))
		}))
		mux.HandleFunc("/metrics", monitoring.PrometheusHandler(collector))
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

		registerCommonDashboardRoutes(dashboard, mux, *swaggerMode, true)

		fs := http.FileServer(http.Dir(filepath.Join(cfg.WorkDir, "web/static")))
		mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
			fs.ServeHTTP(w, r)
		}))
		mux.Handle("/static/", http.StripPrefix("/static/", fs))
		log.Printf("dashboard api listening on %s", *apiAddr)

		// Initialize API Gateway with channel adapters and background task manager.
		gateway, err := apigateway.NewGateway(cfg.WorkDir, pool)
		var taskMgr *apigateway.BackgroundTaskManager
		if err != nil {
			log.Printf("[Gateway] initialization failed: %v", err)
		} else if err := apigateway.RegisterChannelAdapters(gateway, cfg.WorkDir, cfg, janusEngine); err != nil {
			log.Printf("[Gateway] adapter registration failed: %v", err)
		} else {
			log.Printf("[Gateway] initialized with %d channels + adapters", len(gateway.ChannelIDs()))

			// Inject Gateway data fetcher into DashboardAPI (breaks import cycle via func type).
			if d, ok := dashboard.(*monitoring.DashboardAPI); ok {
				d.SetGateway(func(ctx context.Context, channelID string) ([]byte, error) {
					result, err := gateway.Fetch(ctx, channelID)
					if err != nil {
						return nil, err
					}
					return result.Data, nil
				})
				log.Printf("[Gateway] data fetcher injected into DashboardAPI")
			}

			// BackgroundTaskManager for centralized goroutine lifecycle management.
			taskMgr = apigateway.NewBackgroundTaskManager(gateway)

			// Wire failure alerts for background tasks.
			taskMgr.SetFailureHandler(func(name string, consecutiveFailures int, err error) {
				if consecutiveFailures >= 3 {
					monitor.Alert(monitoring.AlertLevelError, "background_task",
						fmt.Sprintf("Task %s failed %d consecutive times: %v", name, consecutiveFailures, err),
						map[string]any{"task": name, "consecutive_failures": consecutiveFailures})
				}
			})

			// Register channel_health_sync task (DB sync, not a data fetcher).
			if pool != nil {
				taskMgr.Register(&apigateway.ScheduledTask{
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

			// Register health_check via HealthChecker.RunOnce (stateStore is nil in API mode).
			if monitor != nil {
				healthChecker := monitoring.NewHealthChecker(monitor, nil)
				taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "health_check",
					Interval: 30 * time.Second,
					Enabled:  true,
					Task: func(ctx context.Context) error {
						return healthChecker.RunOnce(ctx)
					},
				})
				log.Printf("[Gateway] registered health_check background task (30s interval)")
			}

			// Register TSMC Revenue task via Gateway.
			if cfg.FinMindAPIKey != "" {
				taskMgr.Register(&apigateway.ScheduledTask{
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
			taskMgr.Register(&apigateway.ScheduledTask{
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
					return nil
				},
			})
			log.Printf("[Gateway] registered auto_backfill background task (24h interval)")

			// Register auto_capital_flow via Gateway.
			taskMgr.Register(&apigateway.ScheduledTask{
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
			taskMgr.Register(&apigateway.ScheduledTask{
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
			taskMgr.Register(&apigateway.ScheduledTask{
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
			taskMgr.Register(&apigateway.ScheduledTask{
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
			taskMgr.Register(&apigateway.ScheduledTask{
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

			if d, ok := dashboard.(*monitoring.DashboardAPI); ok {
				if svc := d.GetIndustryService(); svc != nil {
					var finmindClient *marketdata.FinMindClient
					if cfg.FinMindAPIKey != "" {
						finmindClient = marketdata.NewFinMindClient(cfg.FinMindAPIKey)
					}
					cycleAggregator := industry.NewDataAggregator(svc.CycleTracker, svc.Classifier, finmindClient)
					taskMgr.Register(&apigateway.ScheduledTask{
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
				}
			}

			{
				revenuePath := filepath.Join(cfg.WorkDir, "data", "replay", "month_revenue.jsonl")
				configPath := filepath.Join(cfg.WorkDir, "configs", "parameters.json")
				taskMgr.Register(&apigateway.ScheduledTask{
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

			if d, ok := dashboard.(*monitoring.DashboardAPI); ok {
				taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "macro_ingest",
					Interval: 5 * time.Minute,
					Enabled:  true,
					Task: func(ctx context.Context) error {
						ingestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
						defer cancel()
						_, _, err := d.IngestAndUpdateMacro(ingestCtx)
						if err != nil {
							logging.Warn("main", "macro_ingest_failed", "err", err)
						}
						return nil
					},
				})
				log.Printf("[Gateway] registered macro_ingest background task (5m interval)")
			}

			if repo != nil {
				taskMgr.Register(&apigateway.ScheduledTask{
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
			taskMgr.Register(&apigateway.ScheduledTask{
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

					system, err := orchestrator.NewProductionSystem(cfg)
					if err != nil {
						return fmt.Errorf("create system: %w", err)
					}
					if collector != nil {
						system.WithMetricsCollector(collector)
					}
					if repo != nil {
						system.SetRepository(repo)
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

					logging.Info("simulation", "completed",
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

			// Register auto_experiment — weekly strategy evolution cycle.
			taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "auto_experiment",
				Interval: 7 * 24 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					system, err := orchestrator.NewProductionSystem(cfg)
					if err != nil {
						return fmt.Errorf("create system: %w", err)
					}
					if repo != nil {
						system.SetRepository(repo)
					}

					candidate, err := system.NextExperimentCandidate()
					if err != nil {
						return fmt.Errorf("identify candidate: %w", err)
					}
					if candidate == nil {
						logging.Info("experiment", "no_candidate", "all agents currently healthy")
						return nil
					}

					logging.Info("experiment", "candidate_selected",
						"agent", candidate.Agent.ID,
						"skill", candidate.Agent.Skill,
						"sharpe", fmt.Sprintf("%.3f", candidate.Scorecard.SharpeLike),
					)

					windowID := "window-" + time.Now().Add(-7*24*time.Hour).Format("20060102") + "-" + time.Now().Format("20060102")
					brief := evolution.BuildMutationBrief(windowID, candidate)

					briefDir := filepath.Join(cfg.WorkDir, "data", "state", "windows")
					os.MkdirAll(briefDir, 0755)
					briefPath := filepath.Join(briefDir, "auto-brief-"+candidate.Agent.ID+".json")
					briefData, _ := json.MarshalIndent(brief, "", "  ")
					if err := os.WriteFile(briefPath, briefData, 0644); err != nil {
						return fmt.Errorf("write brief: %w", err)
					}

					store := ledger.NewStore(cfg.LedgerDir)
					executor := experiment.NewExecutor(store.(ledger.FullStore), cfg.BaselinePolicyPath)
					result, runErr := executor.Run(briefPath, cfg.ReplayDataPath)
					if runErr != nil {
						monitor.Alert(monitoring.AlertLevelWarning, "experiment",
							fmt.Sprintf("實驗失敗: agent=%s, err=%v", candidate.Agent.ID, runErr),
							map[string]any{"agent": candidate.Agent.ID, "error": runErr.Error()})
						return fmt.Errorf("run experiment: %w", runErr)
					}

					expPath := findLatestExperiment(filepath.Join(cfg.WorkDir, "data", "state", "experiments"))
					if expPath == "" {
						return fmt.Errorf("experiment result not found for %s", result.Experiment.ID)
					}

					judge := experiment.NewJudge(store.(ledger.ExperimentStore), cfg.ReplayDataPath, cfg.BaselinePolicyPath)
					judged, judgeErr := judge.Evaluate(expPath)
					if judgeErr != nil {
						return fmt.Errorf("judge experiment: %w", judgeErr)
					}

					status := judged.Experiment.Status
					logging.Info("experiment", "judged",
						"agent", candidate.Agent.ID,
						"status", status,
						"baseline", fmt.Sprintf("%.3f", judged.Experiment.BaselineValue),
						"candidate", fmt.Sprintf("%.3f", judged.Experiment.CandidateValue),
					)

					if status == domain.ExperimentAccepted {
						mgr := baseline.NewManager(cfg.BaselinePolicyPath)
						if _, err := mgr.PromoteResult(expPath); err != nil {
							monitor.Alert(monitoring.AlertLevelError, "experiment",
								fmt.Sprintf("晉升失敗: agent=%s, err=%v", candidate.Agent.ID, err),
								map[string]any{"agent": candidate.Agent.ID, "error": err.Error()})
							return fmt.Errorf("promote result: %w", err)
						}
						monitor.Alert(monitoring.AlertLevelInfo, "experiment",
							fmt.Sprintf("策略晉升成功: agent=%s (%s), sharpe=%.3f", candidate.Agent.ID, candidate.Agent.Skill, candidate.Scorecard.SharpeLike),
							map[string]any{
								"agent":     candidate.Agent.ID,
								"skill":     candidate.Agent.Skill,
								"status":    string(status),
								"baseline":  judged.Experiment.BaselineValue,
								"candidate": judged.Experiment.CandidateValue,
							})
					} else {
						monitor.Alert(monitoring.AlertLevelInfo, "experiment",
							fmt.Sprintf("實驗未通過: agent=%s (%s), status=%s", candidate.Agent.ID, candidate.Agent.Skill, status),
							map[string]any{
								"agent":     candidate.Agent.ID,
								"skill":     candidate.Agent.Skill,
								"status":    string(status),
								"baseline":  judged.Experiment.BaselineValue,
								"candidate": judged.Experiment.CandidateValue,
							})
					}
					return nil
				},
			})
			log.Printf("[Gateway] registered auto_experiment background task (7-day interval)")

			if ruleEngine != nil {
				params := config.GetParametersConfig().Alert
				taskMgr.Register(&apigateway.ScheduledTask{
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

			taskMgr.Start(sysCtx)
			log.Printf("[Gateway] BackgroundTaskManager started with %d tasks", len(taskMgr.List()))
		}

		if taskMgr != nil {
			apischeduler.NewHandlers(apischeduler.NewSchedulerService(taskMgr)).RegisterRoutes(mux)
			log.Printf("[Gateway] scheduler API routes registered")
		}

		var btRunner *autobacktest.Runner
		if d, ok := dashboard.(*monitoring.DashboardAPI); ok && d.GetEventBus() != nil {
			btRunner = autobacktest.NewRunnerWithEventBus(cfg, d.GetEventBus())
			log.Printf("[AutoBacktest] connected to Dashboard EventBus for SSE streaming")
		} else {
			btRunner = autobacktest.NewRunner(cfg)
			log.Printf("[AutoBacktest] running without EventBus (no SSE events)")
		}
		autobacktest.StartDailyLoop(sysCtx, btRunner)

		authWrappedMux := apishared.AuthMiddleware(mux)
		finalMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")
			if r.URL.Path == "/metrics" || strings.HasPrefix(r.URL.Path, "/static/") {
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
	system, err := orchestrator.NewProductionSystem(cfg)
	if err != nil {
		return fmt.Errorf("create system: %w", err)
	}
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

	return nil
}

func runLiveTrading(cfg config.Config, deps appDeps, collector *monitoring.MetricsCollector, repo *repository.DualWriteRepository) error {
	system, err := orchestrator.NewProductionSystem(cfg)
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
	eventBus := live.NewChannelEventBus(64)
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
	if d, ok := dashboard.(*monitoring.DashboardAPI); ok {
		if repo != nil {
			d.SetRepository(repo)
			log.Printf("[Repository] injected into live trading dashboard API")
		}
		d.SetEventBus(eventBus)
		d.SetContext(ctx)
		logging.SetLogContext(ctx)
		log.Printf("[EventBus] injected into live trading dashboard API for SSE streaming")
	}
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

// findLatestExperiment auto-discovers the most recent experiment JSON file
// by sorting filenames by embedded timestamp (same as judge-experiment CLI).
func findLatestExperiment(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if filepath.Ext(name) == ".json" && name != "test-experiment.json" {
			files = append(files, name)
		}
	}
	if len(files) == 0 {
		return ""
	}
	sort.Slice(files, func(i, j int) bool {
		return extractTimestamp(files[i]) > extractTimestamp(files[j])
	})
	return filepath.Join(dir, files[0])
}

func extractTimestamp(filename string) int64 {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	parts := strings.Split(base, "-")
	if len(parts) > 0 {
		if ts, err := strconv.ParseInt(parts[len(parts)-1], 10, 64); err == nil {
			return ts
		}
	}
	return 0
}
