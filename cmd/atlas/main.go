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

	"github.com/kaecer68/atlas-go/admin_web"
	"github.com/kaecer68/atlas-go/client_web"
	"github.com/kaecer68/atlas-go/internal/alerting"
	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/autobacktest"
	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/bootstrap"
	"github.com/kaecer68/atlas-go/internal/buildinfo"
	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/dailyreport"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/eventdriven"
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
	"github.com/kaecer68/atlas-go/internal/marketexplain"
	"github.com/kaecer68/atlas-go/internal/methodology"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/monitoring/alertscanner"
	apievents "github.com/kaecer68/atlas-go/internal/monitoring/api/events"
	llmHealth "github.com/kaecer68/atlas-go/internal/monitoring/api/llm"
	apipipeline "github.com/kaecer68/atlas-go/internal/monitoring/api/pipeline"
	apischeduler "github.com/kaecer68/atlas-go/internal/monitoring/api/scheduler"
	apishared "github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	apistrategies "github.com/kaecer68/atlas-go/internal/monitoring/api/strategies"
	"github.com/kaecer68/atlas-go/internal/monitoring/metrics"
	monitoringservice "github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/narrative"
	obsotel "github.com/kaecer68/atlas-go/internal/observability/otel"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/orchestrator/composition"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/realtime"
	"github.com/kaecer68/atlas-go/internal/recommender"
	"github.com/kaecer68/atlas-go/internal/repository"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/scheduler"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
	"github.com/kaecer68/atlas-go/internal/startup"
	"github.com/kaecer68/atlas-go/internal/stocktools"
	"github.com/kaecer68/atlas-go/internal/storage"
	strategyRanker "github.com/kaecer68/atlas-go/internal/strategy_ranker"
	"github.com/kaecer68/atlas-go/internal/strategy_techniques"
	"github.com/kaecer68/atlas-go/internal/subscription"
	"github.com/kaecer68/atlas-go/internal/userstate"
)

// predictionHistoryAdapter bridges ledger.EventFlowPredictionStore to the
// local eventdriven.PredictionHistoryStore interface so the prediction
// handler can persist predictions and compute a realized hit rate without
// importing ledger.
type predictionHistoryAdapter struct {
	inner ledger.EventFlowPredictionStore
}

func (a *predictionHistoryAdapter) AppendPrediction(rec eventdriven.PredictionRecord) error {
	if a == nil || a.inner == nil {
		return nil
	}
	return a.inner.AppendPrediction(ledger.EventFlowPredictionRecord{
		PredictedAt:   rec.PredictedAt,
		DirectionSign: rec.DirectionSign,
		Confidence:    abs(rec.DirectionSign),
		Direction:     directionFromSign(rec.DirectionSign),
	})
}

func (a *predictionHistoryAdapter) HasPredictionOn(t time.Time) (bool, error) {
	if a == nil || a.inner == nil {
		return false, nil
	}
	_, err := a.inner.LoadByDate(t)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ledger.ErrPredictionNotFound) {
		return false, nil
	}
	return false, err
}

func (a *predictionHistoryAdapter) LoadRecentPredictions(limit int) ([]eventdriven.PredictionRecord, error) {
	if a == nil || a.inner == nil {
		return nil, nil
	}
	rows, err := a.inner.LoadRecentPredictions(limit)
	if err != nil {
		return nil, err
	}
	out := make([]eventdriven.PredictionRecord, len(rows))
	for i, r := range rows {
		out[i] = eventdriven.PredictionRecord{
			PredictedAt:      r.PredictedAt,
			DirectionSign:    r.DirectionSign,
			ActualSign:       r.ActualSign,
			ActualCapturedAt: r.ActualCapturedAt,
		}
	}
	return out, nil
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func directionFromSign(sign float64) string {
	switch {
	case sign > 0:
		return "inflow"
	case sign < 0:
		return "outflow"
	default:
		return "neutral"
	}
}

// scanStoreAdapter bridges ledger.DetectorScanStore to the local
// eventdriven.DetectorScanStore interface, avoiding a direct import of
// ledger (which would create a package dependency cycle through
// narrative→type_theme_mapping→eventdriven).
type scanStoreAdapter struct {
	inner ledger.DetectorScanStore
}

func (a *scanStoreAdapter) LoadRecentScans(ctx context.Context, limit int) ([]eventdriven.ScanResult, error) {
	if a == nil || a.inner == nil {
		return nil, nil
	}
	rows, err := a.inner.LoadRecentScans(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]eventdriven.ScanResult, len(rows))
	for i, r := range rows {
		out[i] = eventdriven.ScanResult{
			Theme:      r.Theme,
			Severity:   string(r.Severity),
			Confidence: r.Confidence,
			DetectedAt: r.DetectedAt,
		}
	}
	return out, nil
}

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

// isPrismWorkerCmd reports whether the given positional args (post flag
// parsing) represent the "prism worker" subcommand. This is the only
// subcommand supported by the atlas CLI; all other entry points are
// flag-based (-api, -live, -simulate, -build-universe).
func isPrismWorkerCmd(args []string) bool {
	return len(args) >= 2 && args[0] == "prism" && args[1] == "worker"
}

// janusEngine is the global janus.Engine instance, populated by run() when
// -api mode is enabled, then injected into dashboard via SetJanusEngine.
// The /api/janus/regime-score endpoint is served by the existing
// dashboard.Handlers.HandleJanusRegimeScore handler (see
// internal/monitoring/api/dashboard/handlers.go:116).
var janusEngine *janus.Engine

// strategyFeedbackStore is the FeedbackStore wired by main.go at startup
// and consumed by the autobacktest attribution pipeline (#1259).
var strategyFeedbackStore *apistrategies.FeedbackStore

// isPublicPath determines whether a request bypasses the API-key
// AuthMiddleware. Web UI pages and their static assets under /admin/
// and /client/, plus probing endpoints, are loaded by the browser
// without credentials and must not require an API key. Adding a new
// path here is a security boundary change.
func isPublicPath(p string) bool {
	switch {
	case p == "/" || p == "/health" || p == "/ready" || p == "/metrics":
		return true
	case p == "/api/health" || p == "/api/health/":
		// Kept public: external probes may hit the /api/ prefix; handler is a
		// redirect to the canonical /health endpoint.
		return true
	case p == "/api/llm/health", p == "/api/v1/alerts": // Alertmanager webhook inbound — only POST from Alertmanager container
		return true
	case p == "/api/health/aggregate": // Stage 6 PR#1: 4-tier health aggregation for frontend banner
		return true
	case p == "/api/version":
		return true
	case strings.HasPrefix(p, "/api/routes"):
		return true
	case p == "/api/dashboard" || strings.HasPrefix(p, "/api/dashboard/"):
		return true
	case p == "/api/dashboard/sessions" || strings.HasPrefix(p, "/api/dashboard/sessions/"):
		return true
	case p == "/api/taiwan" || strings.HasPrefix(p, "/api/taiwan/"):
		return true
	case p == "/api/narrative" || strings.HasPrefix(p, "/api/narrative/"):
		return true
	case p == "/api/macro" || strings.HasPrefix(p, "/api/macro/"):
		return true
	case p == "/api/alerts" || strings.HasPrefix(p, "/api/alerts/"):
		return true
	case p == "/api/synergy" || strings.HasPrefix(p, "/api/synergy/"):
		return true
	case p == "/api/cross-market" || strings.HasPrefix(p, "/api/cross-market/"):
		return true
	case p == "/api/detector" || strings.HasPrefix(p, "/api/detector/"):
		return true
	case p == "/api/capital-flow" || strings.HasPrefix(p, "/api/capital-flow/"):
		return true
	case p == "/api/events" || strings.HasPrefix(p, "/api/events/"):
		return true
	case p == "/api/industry" || strings.HasPrefix(p, "/api/industry/"):
		return true
	case p == "/api/recommendations" || strings.HasPrefix(p, "/api/recommendations/"):
		return true
	case p == "/api/reports" || strings.HasPrefix(p, "/api/reports/"):
		return true
	case p == "/api/stock" || strings.HasPrefix(p, "/api/stock/"):
		return true
	case p == "/api/strategy-ranker" || strings.HasPrefix(p, "/api/strategy-ranker/"):
		return true
	case p == "/api/parameters" || strings.HasPrefix(p, "/api/parameters/"):
		return true
	case p == "/api/backtest" || strings.HasPrefix(p, "/api/backtest/"):
		return true
	case p == "/api/auth" || strings.HasPrefix(p, "/api/auth/"):
		return true
	case p == "/api/user" || strings.HasPrefix(p, "/api/user/"):
		return true
	case p == "/api/strategies" || strings.HasPrefix(p, "/api/strategies/"):
		return true
	case p == "/api/risk" || strings.HasPrefix(p, "/api/risk/"):
		return true
	case p == "/api/janus" || strings.HasPrefix(p, "/api/janus/"):
		return true
	case p == "/api/regime" || strings.HasPrefix(p, "/api/regime/"):
		return true
	case p == "/api/geopolitical" || strings.HasPrefix(p, "/api/geopolitical/"):
		return true
	case p == "/api/scheduler" || strings.HasPrefix(p, "/api/scheduler/"):
		return true
	case p == "/api/tasks" || strings.HasPrefix(p, "/api/tasks/"):
		return true
	case p == "/api/traces" || strings.HasPrefix(p, "/api/traces/"):
		return true
	case strings.HasPrefix(p, "/api/llm/"):
		return true
	case strings.HasPrefix(p, "/api/llm_annotator/"):
		return true
	case strings.HasPrefix(p, "/api/prism/"):
		return true
	case p == "/api/field-contract":
		return true
	case p == "/api/control/audit-log" || p == "/api/control/active-overrides":
		return true
	case p == "/api/experiment/history" || p == "/api/experiment/diff":
		return true
	case p == "/admin" || strings.HasPrefix(p, "/admin/"):
		return true
	case p == "/client" || strings.HasPrefix(p, "/client/"):
		return true
	case strings.HasSuffix(p, ".js"):
		// Hashed frontend chunks (stock-quote-*.js, portfolio-*.js, etc.)
		// are served at root level (not under /client/) because main.js
		// at /client/js/main.js imports them with a relative ../ path.
		// Standard API routes never end in .js, so this catch-all is safe.
		return true
	default:
		return false
	}
}

func run(args []string, deps appDeps) error {
	flags := flag.NewFlagSet("atlas", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	apiMode := flags.Bool("api", false, "start dashboard api server")
	apiAddr := flags.String("addr", constants.AdminHTTPPort, "dashboard api listen address")
	swaggerMode := flags.Bool("swagger", false, "enable swagger docs endpoints")
	// L2.4 sector-agents CLI flag (delivered via PR #828 merged 2026-06-29).
	// String flag (not Bool) so empty = no-override, "true"/"false" = explicit override.
	useLLMSectorAgents := flags.String("use-llm-sector-agents", "",
		"override L2.4 LLM-driven sector agent (true|false|1|0, empty=no-override)")
	brokerMode := flags.String("broker-mode", "", "override broker mode: dry-run|paper|live")
	brokerAdapter := flags.String("broker-adapter", "", "override broker adapter: guarded|mock|http")
	brokerSigner := flags.String("broker-signer", "", "override broker signer: placeholder|hmac-sha256")
	brokerKeyID := flags.String("broker-key-id", "", "override broker key id for signer key rotation")
	brokerRetryStatusCodes := flags.String("broker-retry-status-codes", "", "override broker retry status codes csv, e.g. 408,429,503")
	brokerMaxRetries := flags.Int("broker-max-retries", -1, "override broker max retries (>=0)")
	brokerMaxClockSkew := flags.Int("broker-max-clock-skew-sec", -1, "override broker max clock skew seconds (>=0, 0 disables check)")
	brokerNonceTTL := flags.Int("broker-nonce-ttl-sec", -1, "override broker nonce replay ttl seconds (>=1)")
	brokerNonceStore := flags.String("broker-nonce-store", "", "override nonce replay store: memory|file|redis")
	versionFlag := flags.Bool("version", false, "print version and exit")
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
	fubonProxyPort := flags.Int("fubon-port", constants.FubonProxyPort, "fubon-proxy Python 服務 listen port(同時決定 /health URL 與 FubonClient proxy URL)")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if *versionFlag {
		info := buildinfo.Current()
		fmt.Printf("atlas %s (commit %s, built %s, %s)\n", info.Version, info.Commit, info.BuildTime, info.GoVersion)
		return nil
	}
	// -fubon-port flag 注入 fubonproxy 內部 listen port(單一真相來源);
	// PR #837 user prompt A1 root cause:之前 marketdata 與 fubonproxy 各自有
	// package-level port var,任一處忘記同步就 drift → channel recurring failure。
	// 此處直接寫 fubonproxy,移除中間的 marketdata.SetFubonProxyPort deprecated wrapper。
	fubonproxy.SetFubonProxyPort(*fubonProxyPort)

	cfg := deps.loadConfig()

	// SA06: composition root for shared dependency wiring.
	// Elevated to run() level so it is visible to -live/-simulate paths.
	var compositionRoot *composition.Root
	// historicalStore is elevated to run() scope so registerCalibrationTasks
	// (line ~2175) can pass it to a prediction_backtest reverse-write task
	// (Gap 2-D) without opening a second SQLite connection.
	var historicalStore ledger.HistoricalStore
	var err error
	compositionRoot, err = composition.NewRoot(cfg)
	if err != nil {
		log.Printf("[Composition] failed to create root: %v", err)
	} else {
		// SA08 Gap D: wire closure store, session resolver, and
		// SAC closure state manager for StrategyEvolver.
		//
		// The closure store is rooted under <WorkDir>/data/sector/allocation
		// so it lives in the same persistent mount as the rest of the
		// data/state tree (data/state/atlas.db, data/state/...). Without
		// the "data" prefix the file would land in the container's
		// /app/sector/allocation, which is not mounted from host and
		// vanishes on container restart.
		//
		// The session resolver prefers the replay CSV (deterministic,
		// fail-closed per spec §8.2). When the CSV is missing or
		// unparseable we fall back to NoOpNextSessionResolver so the
		// production container never blocks on a missing dataset.
		closureStore := sectorallocation.NewFileClosureStore(
			filepath.Join(cfg.WorkDir, "data", "sector", "allocation"),
		)
		replayPath := os.Getenv("ATLAS_REPLAY_DATA_PATH")
		if replayPath == "" {
			replayPath = filepath.Join(cfg.WorkDir, "data", "replay", "tw_extended_90days.csv")
		}
		sessionResolver, resolverErr := orchestrator.NewReplaySessionResolverFromCSV(replayPath)
		if resolverErr != nil {
			log.Printf("[Composition] replay session resolver unavailable: %v — falling back to NoOp (spec §8.2 fail-closed)", resolverErr)
			sessionResolver = &orchestrator.NoOpNextSessionResolver{}
		}
		// SA11.A: closure state manager tracks the observation window
		// (session count, invariant violations) required for the
		// Gate 5 promotion check.
		closureStateMgr := sectorallocation.NewSACClosureStateManager(cfg.WorkDir)
		compositionRoot.
			WithClosureStore(closureStore).
			WithSessionResolver(sessionResolver).
			WithSACClosureStateManager(closureStateMgr)
	}

	// --use-llm-sector-agents CLI override (delivered scaffold via PR #828,
	// wired here). Tri-state flag: empty = no override (env var / parameters.json
	// value preserved), explicit true/false = override cfg.LLMSectorAgentsEnabled.
	if *useLLMSectorAgents != "" {
		parsed, explicit, err := parseTriStateFlag(*useLLMSectorAgents)
		if err != nil {
			return fmt.Errorf("parse --use-llm-sector-agents: %w", err)
		}
		if explicit {
			prev := cfg.LLMSectorAgentsEnabled
			cfg.LLMSectorAgentsEnabled = parsed
			logging.Info("main", "cli_override_applied",
				"flag", "use-llm-sector-agents",
				"from", prev,
				"to", parsed,
				"source", "cli")
		}
	}

	logging.Init(*logFormat, slog.LevelInfo)

	// Subcommand dispatch: "prism worker" is a lightweight daemon that
	// does not need the full runtime (no API server, no live trading
	// bootstrap, no Postgres pre-flight). Route it before the heavy
	// init so a misconfigured DB does not block worker startup.
	if isPrismWorkerCmd(flags.Args()) {
		return runPrismWorker(cfg, deps)
	}

	otelShutdown, otelErr := obsotel.Init(context.Background())
	if otelErr != nil {
		logging.Warn("main", "otel_init_failed", "err", otelErr)
	} else {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := otelShutdown(shutdownCtx); err != nil {
				logging.Warn("main", "otel_shutdown_failed", "err", err)
			}
		}()
	}

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
	if diag := ensurePostgres(); diag != "" {
		logging.Warn(
			"main", "postgres_startup_diag",
			"message", "ensurePostgres did not fully succeed; bootstrap may fail",
			"diagnostics", diag,
		)
	} else {
		logging.Info("main", "postgres_ready")
	}

	// Security gate: live broker requires both the CLI flag AND the
	// ATLAS_ALLOW_LIVE_BROKER=true env var. This prevents accidental
	// live-broker activation from a mistyped CLI flag or stale alias.
	if *allowLiveBroker && !cfg.AllowLiveBroker {
		return fmt.Errorf("live broker requires ATLAS_ALLOW_LIVE_BROKER=true env var in addition to --allow-live-broker flag")
	}
	if *allowHTTPBroker && !cfg.AllowHTTPBroker {
		return fmt.Errorf("http broker requires ATLAS_ALLOW_HTTP_BROKER=true env var in addition to --allow-http-broker flag")
	}
	if *allowRealSigner && !cfg.AllowRealSigner {
		return fmt.Errorf("real signer requires ATLAS_ALLOW_REAL_SIGNER=true env var in addition to --allow-real-signer flag")
	}

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

	baselineMgr := baseline.NewManager(cfg.BaselinePolicyPath)

	// Handle --simulate mode: run one-shot daily simulation and exit
	if *simulateMode {
		return runSimulationMode(rt, cfg, compositionRoot, *verboseMode, *dateOverride)
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

		// Stage 8.5 follow-up: use wired factory so default events load on
		// startup (was empty events slice before stage3 schedule fired).
		// See internal/industry/event_calendar.go:1326-1328 "PR#1 root cause".
		eventCalendar := industry.NewEventCalendarWithProvider(nil)

		// Initialize MaturityTracker for burn-in / calibrating / full-auto gating.
		maturityTracker, _ := domain.NewMaturityTracker(filepath.Join(cfg.WorkDir, "data/state/maturity_tracker.json"))
		if maturityTracker != nil {
			logging.Info("bootstrap", "maturity_tracker_ready",
				"maturity", string(maturityTracker.Current()),
				"days_since_start", maturityTracker.DaysSinceStart())
		}

		var lifecycleMgr *storage.LifecycleManager

		var stRegistry *strategy_techniques.Registry
		var stHandlers *apistrategies.Handlers
		var stSeedsPath string
		var methodologyAdvisor *methodology.Advisor

		// Exclusive atlas-http claim always runs first so a healthy Docker
		// (or leftover native) instance fails fast before expensive bootstrap.
		// fubon-proxy is shared and only claimed when we intend to manage it.
		claims := []startup.PortClaim{
			{Component: "atlas-http", Addr: *apiAddr},
		}
		if shouldStartFubonProxy(cfg.BrokerMode, cfg.FubonAPIKey) {
			claims = append(claims, startup.PortClaim{
				Component:       "fubon-proxy",
				Addr:            fmt.Sprintf("127.0.0.1:%d", *fubonProxyPort),
				AllowZombieKill: true,
			})
		}
		if err := startup.Preflight(claims); err != nil {
			return fmt.Errorf("preflight failed: %w", err)
		}

		// Start fubon-proxy process manager BEFORE Gateway adapter registration,
		// so the fubon TCP probe in RegisterChannelAdapters finds fubon-port already running.
		var fubonMgr *fubonproxy.ProcessManager
		if shouldStartFubonProxy(cfg.BrokerMode, cfg.FubonAPIKey) {
			fubonMgr = fubonproxy.NewManager(cfg.WorkDir, *fubonProxyPort)
			if err := fubonMgr.Start(context.Background()); err != nil {
				log.Printf("[FubonProxy] start warning (non-fatal): %v", err)
			} else {
				log.Printf("[FubonProxy] process manager started")
			}
			defer fubonMgr.Stop()
		}

		// Readiness state is populated during startup and consumed by the
		// GET /ready handler installed by registerSimpleRoutes.
		rc := readyChecker{
			dbDSN:      os.Getenv("DATABASE_URL"),
			replayPath: config.GetReplayDataPath(cfg.WorkDir),
		}

		// Initialize Gateway BEFORE DashboardAPI so data providers use Gateway from the start.
		var gateway *apigateway.Gateway
		var detectorRegistry *narrative.DetectorRegistry
		var detectorScanStore ledger.DetectorScanStore
		var scanStoreErr error
		var gatewayFetcher monitoring.DataFetcher
		var narrativeEngine *narrative.NarrativeEngine
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
				gatewayFetcher = func(ctx context.Context, channelID string) ([]byte, monitoring.FetchMeta, error) {
					result, err := gateway.Fetch(ctx, channelID)
					if err != nil {
						return nil, monitoring.FetchMeta{}, err
					}
					return result.Data, monitoring.FetchMeta{
						Stale:     result.Stale,
						Fallback:  result.Fallback,
						LastError: result.LastError,
					}, nil
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
		// Manifest #G05: feed the full ChannelRegistry into the admin data-channels
		// page so it lists every registered adapter (not just the hand-maintained
		// subset). The list is queried at request time so new adapters picked up
		// after startup appear on the next refresh.
		if gateway != nil {
			dashboard.RegisteredChannelIDs = gateway.ChannelIDs()
		}
		agentHealthMgr := portfolio.NewAgentHealthManagerWithStore(portfolio.DefaultAgentHealthConfig(), healthStore).WithParameters(runtimeParams)
		dashboard.SetHealthManager(agentHealthMgr)
		prismMgr := prism.NewPRISMManager(prism.DefaultPRISMConfig())
		// PRISM 訓練結果 API 已移除（admin 頁面刪除，2026-08-12）。
		// prismMgr 保留供 operations_tasks.go 的 prism_training BTM 使用
		// （該 task 現為 disabled — 見 operations_tasks.go）。
		// Start the PRISM training-queue workers. Previously the manager
		// was created but never started, so tasks scheduled via the
		// dashboard API would queue up without ever being processed.
		// Workers idle on an empty in-memory queue and exit cleanly via
		// the deferred Stop() when the apiMode block returns.
		prismMgr.Start()
		defer prismMgr.Stop()
		dwMgr := portfolio.NewDarwinianWeightManager(filepath.Join(cfg.WorkDir, "data/state/darwinian_weights.json"))
		l24Mgr := apipipeline.NewL24StateManager(cfg.WorkDir)
		// Seed the schedule config from parameters.json so a fresh
		// state file does not start with DefaultPeriodDays=0, which
		// would create an immediately-expired observation window.
		p := config.GetL2_4Schedule()
		if err := l24Mgr.SetConfig(apipipeline.L24ScheduleConfig{
			DefaultStartTime:  p.DefaultStartTime.Value,
			DefaultPeriodDays: p.DefaultPeriodDays.Value,
			AutoEnabled:       p.AutoEnabled.Value,
		}); err != nil {
			logging.Warn("main", "l24_seed_failed", logging.Err(err))
		}
		autoRollback := scheduler.NewAutoRollback(nil, dwMgr, agentHealthMgr)
		healthMonitor := scheduler.NewSystemHealthMonitor(dwMgr, agentHealthMgr)
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
		defer func() {
			if err := dashEventBus.Close(); err != nil {
				logging.Warn("main", "dash_event_bus_close_failed", logging.Err(err))
			}
		}()

		// Inject EventBus for SSE streaming endpoint
		dashboard.SetEventBus(dashEventBus)
		autoRollback.WithEventBus(dashEventBus)
		dashboard.SetContext(context.Background())
		log.Printf("[EventBus] injected into dashboard API for SSE streaming")
		apievents.RegisterDashboardBufferSubs(dashEventBus)
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

		// Register static routes and simple probes *before* DashboardAPI so that
		// DashboardAPI's dynamic /health (which reflects -addr and -fubon-port)
		// wins on the same mux pattern. registerSimpleRoutes also provides the
		// fallback /health used by live/simulation modes.
		adminSubFS, err := fs.Sub(admin_web.DistFS, "dist")
		if err != nil {
			log.Fatalf("failed to get admin dist sub FS: %v", err)
		}
		clientSubFS, err := fs.Sub(client_web.DistFS, "dist")
		if err != nil {
			log.Fatalf("failed to get client dist sub FS: %v", err)
		}
		registerSimpleRoutes(mux, collector, adminSubFS, clientSubFS, rc, *apiAddr)

		// Per-symbol stock endpoints for atlas-mcp.
		var quoteStore ledger.QuoteStore
		var fugleClient *marketdata.FugleClient
		var finmindClient *marketdata.FinMindClient
		stockDeps := stocktools.Deps{}
		if cfg.FugleAPIKey != "" {
			fugleClient = marketdata.GetSharedFugleClient(cfg.FugleAPIKey)
			stockDeps.FugleClient = fugleClient
		}
		if cfg.FinMindAPIKey != "" {
			finmindClient = marketdata.GetSharedFinMindClient(cfg.FinMindAPIKey, cfg.WorkDir)
		}
		if fp := portfolio.NewFundamentalProvider(); true {
			fundamentalsPath := filepath.Join(cfg.WorkDir, "data", "fundamentals.json")
			if err := fp.LoadFromJSON(fundamentalsPath); err == nil {
				stockDeps.Fundamentals = fp
			} else {
				log.Printf("[StockTools] fundamentals load failed: %v", err)
			}
		}
		if qs, err := ledger.NewQuoteStore(cfg); err == nil {
			quoteStore = qs
			stockDeps.QuoteStore = qs
			if dashboard != nil {
				dashboard.SetQuoteStore(qs)
				if fugleClient != nil {
					dashboard.SetFugleClient(fugleClient)
				}
			}
		} else {
			log.Printf("[StockTools] quote store init failed: %v", err)
		}
		// Postgres historical backend: inject the shared pgxpool so
		// regime/stress/period history live in the multi-process-friendly
		// PostgreSQL database instead of the SQLite atlas.db shared by
		// multiple containers (avoids SQLITE_IOERR WAL contention).
		if cfg.StoreBackend == "postgres" {
			ledger.SetPostgresPool(pool)
		}
		if historicalStore, err = ledger.NewHistoricalStore(cfg); err == nil {
			log.Printf("[HistoricalStore] initialized (backend=%s)", cfg.StoreBackend)
			// CL-3 A03: wire into DashboardAPI so /api/dashboard/regime-history
			// reads the regime_history SQLite table (true time-series) instead
			// of simulation session summaries. Late-binding after stocktools
			// registration because the store is opened here.
			if dashboard != nil {
				dashboard.WithHistoricalStore(historicalStore)
			}
		} else {
			log.Printf("[HistoricalStore] init failed: %v", err)
		}
		stockDeps.CapitalFlow = marketdata.NewTWSECapitalFlowProvider(filepath.Join(cfg.WorkDir, constants.StateCapitalFlow))

		stockDeps.TWSEQuote = marketdata.NewTWSEOpenAPIProvider()
		// Monthly revenue endpoint (stock_get_monthly_revenue MCP tool) —
		// reuse the same TSMCRevenueProvider used by the tsmc_revenue
		// macro channel (register_adapters.go:163) so both share the
		// FinMind singleton client + 14400/day QuotaRegistry tracker.
		// Without this wiring the endpoint 503s in production.
		if cfg.FinMindAPIKey != "" {
			stockDeps.Revenue = marketdata.NewTSMCRevenueProviderWithStorage(cfg.FinMindAPIKey, filepath.Join(cfg.WorkDir, "data/state/tsmc_revenue"))
		}
		stocktools.RegisterRoutes(mux, stockDeps)
		log.Printf("[StockTools] registered /api/stock/* routes")

		dashboard.SetHealthAddrs(*apiAddr, *fubonProxyPort)
		dashboard.RegisterAllRoutes(mux, monitoring.RouteOptions{IncludeBacktest: true, IncludeSwagger: *swaggerMode})
		apipipeline.RegisterL24Routes(mux, apipipeline.L24RouteDeps{Manager: l24Mgr, GetParam: config.GetL2_4Schedule})

		alertWebhook := alerting.NewAlertWebhookHandler(1000)
		mux.Handle("/api/v1/alerts", alertWebhook)
		log.Printf("[Alerting] registered /api/v1/alerts webhook handler (cap=1000)")

		if alertStore != nil {
			alertAPI := monitoring.NewAlertAPI(alertStore)
			alertAPI.WithAlertSources(alertscanner.NewWebhookSource(alertWebhook))
			// Narrative source: surfaces macro narrative detections as
			// alerts (表裡閉環 Wave 5).
			narrativeSrc := alertscanner.NewNarrativeSource(dashEventBus, 256)
			if err := narrativeSrc.Start(); err != nil {
				log.Printf("[AlertScanner] narrative source start failed: %v", err)
			}
			alertAPI.WithAlertSources(narrativeSrc)
			alertAPI.RegisterRoutes(mux)

			// Route C: scan for in-flight alerts after process restart.
			scanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			scanner := alertscanner.New(alertStore)
			if alerts, err := scanner.Scan(scanCtx); err != nil {
				log.Printf("[AlertScanner] startup scan failed: %v", err)
			} else if len(alerts) > 0 {
				log.Printf("[AlertScanner] %d unacknowledged alerts loaded from store", len(alerts))
			}
		}

		dailyRptGen := dailyreport.NewGenerator(cfg.WorkDir)
		var macroProvider marketdata.MacroDataProvider
		var eventPredictor orchestrator.EventFlowPredictor // F04
		// BK-15: capitalFlowStore is the production-side handle for the
		// date-keyed rolling sample store (spec §8.5). Construction is
		// infallible today (NewFileRollingSampleStore only returns the
		// store, no error path); the underlying persistLocked call may
		// still fail at write time, which is why we keep a memory
		// fallback available if cfg.LedgerDir becomes read-only.
		var capitalFlowStore capitalflow.RollingSampleStore
		// BK-15: capitalFlowService is the shared *capitalflow.Service
		// wired inside the `gatewayFetcher != nil` block below and
		// passed to registerOperationsTasks so the capital_flow_refresh
		// closure can call Refresh(ctx, tradingDate) — the only writer
		// to the shared rolling sample store. Stays nil when the
		// gateway is unavailable; operations_tasks gates the refresh
		// task registration on this being non-nil.
		var capitalFlowService *capitalflow.Service
		var edHandler *eventdriven.Handler
		var recommenderEventPredictor recommender.EventPredictor

		if gatewayFetcher != nil {
			macroProvider = monitoring.NewMacroDataGatewayAdapter(gatewayFetcher)
			// BK-15: build one shared rolling sample store for the
			// capitalflow subsystem. The file store survives process
			// restart (spec §8.5) and is the only writer through
			// Service.Refresh. Construction is currently infallible
			// (NewFileRollingSampleStore takes path + capacity only);
			// if cfg.LedgerDir becomes read-only at runtime the
			// Refresh path will log the UpsertDay failure and the
			// next 5-minute tick will retry. The store is reused by
			// the HTTP handler, the eventdriven adapter, the
			// recommender HandlerDeps, and the operations_tasks
			// refresh closure so the date-keyed window is identical
			// across all consumers.
			capitalFlowStore = capitalflow.NewFileRollingSampleStore(filepath.Join(cfg.LedgerDir, "capital_flow_rolling.json"), 252)
			cfHandler := capitalflow.NewHandlerWithStore(macroProvider, capitalFlowStore, eventCalendar)
			mux.Handle("GET /api/capital-flow/daily", apishared.Get(cfHandler.HandleDaily))
			mux.Handle("GET /api/capital-flow/summary", apishared.Get(cfHandler.HandleSummary))
			mux.Handle("GET /api/capital-flow/history", apishared.Get(cfHandler.HandleHistory))
			// BL-CL5b (spec §18.3.2): point-in-time snapshot by trading date.
			mux.Handle("GET /api/capital-flow/historical-snapshot/{trading_date}", apishared.Get(cfHandler.HandleHistoricalSnapshot))
			// CL-3 B01 (deprecated duplicate): /api/janus/regime-score is
			// already registered by dashboard.RegisterAllRoutes via the
			// dashboard.Handlers.HandleJanusRegimeScore handler (see
			// internal/monitoring/api/dashboard/handlers.go:116). B01 was
			// originally implemented as a main.go inline handler before
			// the audit revealed the existing implementation.
			// H03: market explain endpoint for retail "為什麼漲跌" button.
			// BK-15: hoist the capitalflow.Service handle BEFORE
			// marketexplain.NewHandler so the handler gets a non-nil
			// Service pointer. Previously these two lines were in
			// the wrong order (L777 was below L770), causing
			// marketexplain.Handler to be constructed with a nil
			// *capitalflow.Service and panic with nil-pointer
			// dereference at every /api/market/explain request
			// (curl 52 Empty reply, server logs show
			// "panic ... capitalflow.(*Service).LatestDaily" — bug
			// independently confirmed by Hermes on 2026-07-22,
			// missed in PR #1269 audit which only fixed the auth
			// path, not the handler init order). Moving the
			// ServiceFromHandler call up two statements fixes the
			// init order.
			capitalFlowService = capitalflow.ServiceFromHandler(cfHandler)
			// H03: market explain endpoint for retail "為什麼漲跌" button.
			explainHandler := marketexplain.NewHandler(macroProvider, capitalFlowService)
			mux.Handle("GET /api/market/explain", apishared.Get(explainHandler.HandleExplain))
			log.Printf("[CapitalFlow] registered /api/capital-flow/* routes")
			// Wire the daily report to the same live sources (fix manifest #B06).
			dailyRptGen.SetProvider(newLiveDailyReportProvider(macroProvider, capitalFlowService, eventCalendar))

			// Stage 5 PR#4 Stage B: register detector scan routes BEFORE event routes
			// so the scan store is available for injection.
			detectorRegistry = narrative.NewDefaultDetectorRegistry()
			narrativeEngine = narrative.NewNarrativeEngine()
			detectorScanStore, scanStoreErr = ledger.NewDetectorScanStore(cfg)
			if scanStoreErr != nil {
				log.Printf("[TemplateDetector] scan store unavailable (%v); routes still registered for reachability", scanStoreErr)
			}
			RegisterTemplateDetectorRoutes(mux, detectorRegistry, detectorScanStore)
			log.Printf("[TemplateDetector] registered /api/detector/* routes (24 detectors + scan store=%v)", detectorScanStore != nil)

			// Wrap detector scan store in adapter to break the
			// ledger→narrative→eventdriven import cycle.
			eventScanStore := &scanStoreAdapter{inner: detectorScanStore}
			narrativeAdapter := &eventdriven.NarrativeProvider{Engine: narrativeEngine}
			// ⚠️  DO NOT add a second RegisterRoutesWith* call for /api/events/*
			// below this line — duplicate mux.Handle will panic on startup.
			// See PR #1173 (commit 7d93e754) for the bug this comment prevents.
			// All event/* routes are owned by RegisterRoutesWithDetectors.
			edHandler = eventdriven.RegisterRoutesWithDetectors(mux, eventCalendar, capitalflow.ServiceFromHandler(cfHandler), narrativeAdapter, eventScanStore)
			// Wire the prediction history store so /api/events/prediction
			// surfaces historical_hit_rate (product positioning §6/§9: show
			// realized accuracy alongside the forecast, "校準中" until enough
			// T+1 samples). The adapter bridges ledger's record type to the
			// handler's local PredictionRecord projection.
			edHandler.SetPredictionStore(&predictionHistoryAdapter{inner: ledger.NewJSONLEventFlowPredictionStore(cfg.LedgerDir)})
			if cfg.SectorPredictionEnabled {
				edHandler.SetMacroProvider(macroProvider)
				log.Printf("[EventDriven] sector predictions enabled with macro provider")
			} else {
				log.Printf("[EventDriven] sector predictions disabled (set SECTOR_PREDICTION_ENABLED=true to enable)")
			}
			log.Printf("[EventDriven] registered /api/events/* routes (wired with capital flow + narrative models + detector scans)")
			log.Printf("[Narrative] wired %d InvestmentModels into predictor", len(narrativeAdapter.ListModels()))

			// F04: wire event-driven prediction into orchestrator simulation tilt.
			if cfg.EventPredictionEnabled {
				eventPredictor = orchestrator.NewEventFlowAdapter(func() (string, float64) {
					report := edHandler.Predictor().Predict(time.Now())
					if len(report.Predictions) == 0 {
						return "neutral", 0
					}
					p := report.Predictions[0]
					return p.Direction, p.Confidence
				})
				log.Printf("[EventDriven] prediction tilt enabled for orchestrator simulation (F04)")
			}
		}

		subStore, err := subscription.NewStore(cfg.WorkDir)
		if err != nil {
			log.Printf("[Subscription] store init failed: %v", err)
		} else {
			jwtSecret := config.GetSecret("ATLAS_JWT_SECRET")
			if jwtSecret == "" {
				jwtSecret = "atlas-dev-secret-change-in-prod"
			}
			jwtMgr := subscription.NewJWTManager(jwtSecret)
			subHandler := subscription.NewHandler(subStore, jwtMgr).
				WithWaitlist(filepath.Join(cfg.LedgerDir, "waitlist.jsonl"))
			// ATLAS_REQUIRE_USER_AUTH=true forces JWT; default is guest TierFree.
			allowGuest := config.GetSecret("ATLAS_REQUIRE_USER_AUTH") != "true"
			subHandler.RegisterRoutes(mux, allowGuest)
			log.Printf("[Subscription] registered /api/auth/* + /api/user/* routes (guest=%v)", allowGuest)

			// Gap 3-R3: per-user signal-state HTTP API. Reuses the same
			// JWT manager + allowGuest flag so R3 auth mirrors the rest of
			// /api/user/*. nil StoreInit → endpoint stays un-registered
			// (defensive: a misconfigured process never exposes the endpoint
			// without a backing store).
			userSignalStore := userstate.NewJSONLStore(filepath.Join(cfg.WorkDir, "data/state"))
			userstate.NewHandler(userSignalStore).RegisterRoutes(mux, subscription.NewAuthMiddleware(jwtMgr, allowGuest))
			log.Printf("[UserState] registered /api/user/signals/* routes (guest=%v)", allowGuest)
			devMode := config.GetSecret("ATLAS_DEV_MODE") == "true"
			deps := WireRecommenderDeps(WireDeps{
				WorkDir:          cfg.WorkDir,
				MacroProvider:    macroProvider,
				EventCalendar:    eventCalendar,
				CapitalFlowStore: capitalFlowStore,
			})
			recommenderEventPredictor = deps.EventPredictor
			methodologyAdvisor = deps.MethodologyAdvisor
			recommender.RegisterRoutesWithDeps(mux, *subStore, jwtMgr, deps, devMode)
			log.Printf("[Recommender] registered /api/recommendations route (real services: %v)",
				anyDepsWired(deps))
		}

		// Startup eager warmup: fill TTL caches before the first user request
		// arrives. Runs in a background goroutine; does NOT block server listen.
		// Set ATLAS_WARMUP=0 to disable.
		go monitoring.RunWarmup(monitoring.WarmupDeps{
			MacroProvider:    macroProvider,
			CapitalFlowSvc:   capitalFlowService,
			EventHandler:     edHandler,
			NarrativeHandler: dashboard.NarrativeHandlers(),
			EventPredictor:   recommenderEventPredictor,
		})

		dailyRptGen.SetRegimeProvider(func() domain.Regime {
			summary, _ := monitoringservice.FindLatestSessionSummary(ledger.NewStore(cfg.LedgerDir), cfg.LedgerDir)
			if summary != nil {
				return summary.Regime
			}
			return domain.RegimeNeutral
		})
		dailyRptGen.SetPeriodProvider(func() *dailyreport.PeriodInfo {
			if macroProvider == nil {
				return nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			snap, err := macroProvider.FetchSnapshot(ctx)
			if err != nil {
				return nil
			}
			ind := monitoring.SnapshotToPeriodIndicators(snap)
			assessment, _ := portfolio.NewPeriodDetectorWithDefaults().DetectAssessment(ind)
			period := assessment.MarketPeriod
			advisor := methodology.NewAdvisor(nil)
			indicators := make([]dailyreport.IndicatorHit, len(assessment.TriggeredIndicators))
			for i, ti := range assessment.TriggeredIndicators {
				indicators[i] = dailyreport.IndicatorHit{
					Name: ti.Name, Value: ti.Value, Threshold: ti.Threshold,
					Relation: ti.Relation, Hit: ti.Hit, InputAvailable: ti.InputAvailable,
				}
			}
			return &dailyreport.PeriodInfo{
				MarketPeriod:        string(period),
				PeriodNameZH:        period.PeriodNameZH(),
				CashLevel:           advisor.CashReserve(period),
				AllowedStrategies:   advisor.AllowedStrategies(period),
				StrategiesDetail:    advisor.StrategiesWithCategory(period),
				Confidence:          assessment.Confidence,
				ConditionsHit:       assessment.ConditionsHit,
				ConditionsTotal:     assessment.ConditionsTotal,
				TriggeredIndicators: indicators,
			}
		})
		dailyreport.RegisterRoutes(mux, dailyRptGen)
		log.Printf("[DailyReport] registered /api/reports/* routes")

		if taskManager != nil {
			dashboard.SetTaskManager(taskManager)
			dashboard.RegisterTaskExecRoutes(mux)
			log.Printf("[TaskExec] injected into dashboard API")
		}

		RegisterAdminRoutes(mux, cfg, fubonMgr)
		var monitor *monitoring.Monitor
		mux.HandleFunc("/admin/trigger-simulation", wrapAdminAuth(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			system, err := buildSystemOrFallback(compositionRoot, composition.PathAdminManual, cfg, dashEventBus, janusEngine, eventPredictor, capitalFlowService)
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
				monitor.Alert(monitoring.AlertLevelInfo, "simulation",
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
					if status == "degraded" {
						if len(failed) == 0 {
							return
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
						return
					}
					// status == "stale" or status == "ok": both mean no
					// channel is currently failing (failed list is empty).
					// PR-F2 (kaecer 2026-08-05 production verification):
					// extend the recovery path to cover "stale" too. The
					// original PR-F (4251d7f8) only handled "ok", which
					// was insufficient for the actual production case
					// where the macro snapshot was stuck at "stale" with
					// an empty failed list — us10y / vix stayed degraded
					// for 9 days because the recovery never fired.
					// Recovery path (PR-F, kaecer 2026-08-05): when status is
					// "ok", clear any previously-recorded "degraded" status
					// for the macro channels. The original 2026-08-04 code
					// iterated the callback's `failed` parameter, but on
					// recovery that list is empty (snapshot is healthy, so
					// no channels are currently failing) — the loop was a
					// no-op and us10y / vix stayed flagged degraded for 9
					// days even after the upstream data recovered.
					//
					// PR-F fix: iterate USMacroFields (the canonical
					// detector list) directly, so EVERY monitored channel
					// is cleared regardless of what `failed` contains.
					// This also handles the first-OK-after-restart case
					// where the process is loading stale "degraded"
					// records from channel_health.json.
					cleared := 0
					for _, ch := range monitoringservice.USMacroFields {
						_ = gateway.Health().Record(ch, "ok", "crossmarket snapshot recovered")
						cleared++
					}
					log.Printf("[CrossMarket] recovery: cleared %d macro channels (status=ok)", cleared)
				})
				log.Printf("[CrossMarket] degraded-data callback wired to UnifiedHealthStore + Monitor")
			}
		}

		log.Printf("dashboard api listening on %s", *apiAddr)

		// Publish bootstrap events so the dashboard SSE stream shows system status immediately.
		publishBootstrapEvents(dashEventBus, replayPath, baselinePath)

		// Gateway already initialized before DashboardAPI. Create BackgroundTaskManager.
		var realtimeAdapter *realtime.RealTimeAdapter
		taskMgr := setupBackgroundTaskManager(gateway, monitor, autoHandler)
		// PRISM 5-min queue rebalancer: migrated from rogue ticker goroutine to
		// BTM (Issue #1447, docs/manifests/2026-08-06-btm-ticker-migration.md).
		// prismMgr is only non-nil in apiMode; AutoBalanceEnabled gates the task.
		if prismMgr.AutoBalanceEnabled() {
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "prism_auto_balancer",
				Interval: 5 * time.Minute,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					prismMgr.Rebalance()
					return nil
				},
			})
			log.Printf("[Gateway] registered prism_auto_balancer background task (5m interval)")
		}
		if gateway != nil {

			// RealTimeAdapter: sub-second regime detection and agent weight
			// adaptation during live market sessions. Gated behind -allow-realtime
			// (mirrors -allow-live-broker pattern).
			if *allowRealtime {
				realtimeAdapter = realtime.NewRealTimeAdapter(&paramsCfg.Realtime)
				log.Printf("[RealTime] adapter created (cadence=%dms, window=%d)",
					paramsCfg.Realtime.UpdateIntervalMs.Value, 60)
			}

			registerDataSyncAndHealthTasks(taskMgr, cfg, gateway, monitor, pool, collector)

			registerCapitalTasks(capitalDeps{
				taskMgr:           taskMgr,
				cfg:               cfg,
				gateway:           gateway,
				autoRollback:      autoRollback,
				autoJudgePromoter: autoJudgePromoter,
				predictionLedger:  ledger.NewJSONLEventFlowPredictionStore(cfg.LedgerDir),
				capitalFlowStore:  capitalFlowStore,
			})

			if detectorRegistry != nil && detectorScanStore != nil {
				macroProvider := func() marketdata.MacroDataSnapshot {
					snap, _ := dashboard.GetLatestMacroSnapshot()
					return snap
				}
				marketProvider := func() narrative.MarketNarrativeData {
					return narrativeEngine.MarketNarrativeData()
				}
				scheduler.RegisterTemplateDetectorScanTasks(taskMgr, detectorRegistry, detectorScanStore, macroProvider, marketProvider)
			}
			if narrativeEngine != nil {
				scheduler.RegisterNarrativeWeightUpdateSchedule(taskMgr, narrativeEngine)
			}
			// fix/20260731-govflow-cadence: shared in-memory CAPTCHA
			// cooldown. One instance per process, injected into the
			// two gov-related background tasks (BTM aggregate +
			// gateway read-refresh). Process restart resets the
			// cooldown — intentional, the upstream's view of
			// bot-shaped traffic is what we are protecting, not
			// our own retry history.
			govCaptchaCooldown := marketdata.NewCaptchaCooldown()

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
				janusEngine:     janusEngine,
				prismMgr:        prismMgr,
				// BK-15: plumb the shared capitalflow.Service so the
				// 5-minute capital_flow_refresh closure can call
				// Refresh(ctx, tradingDate) against the persisted
				// rolling store built above.
				governmentFlowDir: filepath.Join(cfg.WorkDir, "data/state/government_flow"),
				capitalFlow:       capitalFlowService,
				captchaCooldown:   govCaptchaCooldown,
			})

			registerBackfillTasks(backfillDeps{
				taskMgr:   taskMgr,
				cfg:       cfg,
				monitor:   monitor,
				calendar:  eventCalendar,
				collector: collector,
			})

			// Schedule daily report generation after market close (14:00–14:59
			// Taipei), once per day; other ticks skip via ErrTaskSkipped so the
			// failure counter is untouched (fix manifest #B10).
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "daily_report_generate",
				Interval: 1 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					taipei, tzErr := time.LoadLocation("Asia/Taipei")
					if tzErr != nil {
						taipei = time.FixedZone("CST", 8*3600)
					}
					now := time.Now().In(taipei)
					if now.Hour() != 14 {
						return apigateway.ErrTaskSkipped
					}
					if dailyRptGen.GetByDate(now.Format("2006-01-02")) != nil {
						return apigateway.ErrTaskSkipped
					}
					dailyRptGen.Generate()
					return nil
				},
			})
			log.Printf("[DailyReport] registered daily_report_generate background task (1h interval, gated 14:00 Taipei)")

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

					system, err := buildSystemOrFallback(compositionRoot, composition.PathAutoDaily, cfg, dashEventBus, janusEngine, eventPredictor, capitalFlowService)
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
						monitor.Alert(monitoring.AlertLevelInfo, "simulation",
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
					system, err := buildSystemOrFallback(compositionRoot, composition.PathStressTestDaily, cfg, dashEventBus, janusEngine, eventPredictor, capitalFlowService)
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
					system, err := buildSystemOrFallback(compositionRoot, composition.PathAutoExperiment, cfg, dashEventBus, janusEngine, eventPredictor, capitalFlowService)
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

			// Register auto_propose — degradation-triggered experiment proposals
			// (fix manifest #D02: AutoProposer existed but was never wired).
			autoProposer := experiment.NewAutoProposer(dwMgr, agentHealthMgr)
			if maturityTracker != nil {
				autoProposer.WithMaturityTracker(maturityTracker)
			}
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "auto_propose",
				Interval: 24 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					// Backlog gate: don't pile proposals onto a full queue (#D01 cap).
					if n := ledger.CountUnresolvedPlanned(cfg.LedgerDir); n >= 100 {
						logging.Info("auto_propose", "backlog_full_skip", "unresolved", n)
						return nil
					}
					// Reload weights so proposals use fresh rolling metrics.
					_ = dwMgr.Load()
					proposals, err := autoProposer.CheckAndPropose(ctx)
					if err != nil {
						return err
					}
					store := ledger.NewStore(cfg.LedgerDir)
					for _, p := range proposals {
						if err := store.RecordExperiment(domain.ExperimentRecord{
							ID:            fmt.Sprintf("auto-propose-%s-%d", p.AgentID, time.Now().Unix()),
							TargetAgentID: p.AgentID,
							Skill:         p.Brief.TargetSkill,
							MutationType:  p.MutationType,
							Status:        domain.ExperimentPlanned,
							Hypothesis:    p.TriggerReason,
						}); err != nil {
							return fmt.Errorf("record proposal: %w", err)
						}
					}
					if len(proposals) > 0 {
						logging.Info("auto_propose", "proposals_recorded", "count", len(proposals))
					}
					return nil
				},
			})
			log.Printf("[Gateway] registered auto_propose background task (24h interval)")

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

			// Register auto_universe_refresh — daily SmartUniverseBuilder pipeline (06:00 TW, trading days).
			// Fires every minute but only executes when alignToTarget(06:00) and isTradingDay() both pass.
			// The task closure is raw func(ctx context.Context) error to avoid monitoring ↔ apigateway
			// circular import; callers assign directly to apigateway.ScheduledTask.Task.
			um := metrics.NewUniverseMetrics()
			um.SetOnInc(func(name string, labels []string, value float64) {
				labelMap := make(map[string]string, len(labels)/2)
				for i := 0; i+1 < len(labels); i += 2 {
					labelMap[labels[i]] = labels[i+1]
				}
				collector.RecordCounter(name, value, labelMap)
			})
			classTreeAdapter := monitoring.AdaptClassificationTree(industry.DefaultClassification())
			{
				suCfg := config.GetParametersConfig().SmartUniverse
				suDeps := newUniverseBuilderDeps(cfg, classTreeAdapter, gateway, um, suCfg)
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "auto_universe_refresh",
					Interval: 1 * time.Minute,
					Enabled:  true,
					Task:     monitoring.NewDailyUniverseRefreshTask(suDeps),
				})
				log.Printf("[Gateway] registered auto_universe_refresh background task (1m interval, 06:00 TW trigger)")
			}
			{
				suCfg := config.GetParametersConfig().SmartUniverse
				suDeps := newUniverseBuilderDeps(cfg, classTreeAdapter, gateway, um, suCfg)
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "auto_universe_full_rebuild",
					Interval: 1 * time.Minute,
					Enabled:  true,
					Task:     monitoring.NewWeeklyUniverseRebuildTask(suDeps),
				})
				log.Printf("[Gateway] registered auto_universe_full_rebuild background task (1m interval, Mon 06:00 TW trigger)")
			}

			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "universe_coverage_check",
				Interval: 1 * time.Minute,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					tz, err := time.LoadLocation("Asia/Taipei")
					if err != nil {
						return nil
					}
					now := time.Now().In(tz)
					target := time.Date(now.Year(), now.Month(), now.Day(), 6, 0, 0, 0, tz)
					diff := now.Sub(target)
					if diff < -1*time.Minute || diff > 1*time.Minute {
						return nil
					}
					wd := now.Weekday()
					if wd == time.Saturday || wd == time.Sunday {
						return nil
					}
					// Count total symbols from the shared classification tree.
					totalSymbols := monitoring.TotalClassifiedSymbols(classTreeAdapter)
					// Load universe snapshot and count built symbols.
					snapshotPath := filepath.Join(cfg.WorkDir, "data", "state", "universe_snapshot.json")
					snapshotSymbols := 0
					if data, rErr := os.ReadFile(snapshotPath); rErr == nil {
						var snapshot struct {
							Result monitoring.UniverseBuildResult `json:"result"`
						}
						if err := json.Unmarshal(data, &snapshot); err == nil {
							snapshotSymbols = snapshot.Result.SymbolsBuilt
						}
					}
					if totalSymbols > 0 {
						coveragePct := float64(snapshotSymbols) / float64(totalSymbols) * 100
						if snapshotSymbols > 0 && coveragePct < 90 {
							monitor.Alert(monitoring.AlertLevelWarning, "universe_coverage",
								fmt.Sprintf("Universe coverage %.1f%% (%d/%d symbols) — snapshot may be stale",
									coveragePct, snapshotSymbols, totalSymbols),
								map[string]any{
									"snapshot_symbols": snapshotSymbols,
									"total_symbols":    totalSymbols,
									"coverage_pct":     coveragePct,
								})
						}
						um.CoverageMapped.WithLabelValues("coverage_check", "all").Add(int64(snapshotSymbols))
						um.CoverageTotal.WithLabelValues("coverage_check", "all").Add(int64(totalSymbols))
					}
					// Check D6 watchlist size.
					watchlistPath := filepath.Join(cfg.WorkDir, "data", "state", "universe_watchlist.json")
					if wlData, rErr := os.ReadFile(watchlistPath); rErr == nil {
						var wl monitoring.Watchlist
						if err := json.Unmarshal(wlData, &wl); err == nil && len(wl.Symbols) > 20 {
							monitor.Alert(monitoring.AlertLevelWarning, "universe_watchlist",
								fmt.Sprintf("D6 watchlist has %d symbols (threshold: 20)",
									len(wl.Symbols)),
								map[string]any{
									"watchlist_size": len(wl.Symbols),
									"threshold":      20,
								})
						}
					}
					return nil
				},
			})
			log.Printf("[Gateway] registered universe_coverage_check background task (1m interval, 06:00 TW trigger)")

			// Register auto_quote_backfill — daily incremental backfill of historical
			// daily bars for all stocks in fundamentals.json with PE>0. Stocks already
			// covered (≥90 days) are skipped. Uses FinMind (per-day TaiwanStockPrice)
			// to preserve Fugle free-tier quota for on-demand queries.
			//
			// Data source priority: TWSE → Fubon → FinMind → Fugle.
			// TWSE/Fubon lack historical range APIs; FinMind is used for background
			// (tolerates per-day latency); Fugle is reserved for on-demand fallback.
			if quoteStore != nil && finmindClient != nil {
				backfillDeps := monitoring.QuoteBackfillDeps{
					FinMindClient: finmindClient,
					QuoteStore:    quoteStore,
					WorkDir:       cfg.WorkDir,
				}
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "auto_quote_backfill",
					Interval: 24 * time.Hour,
					Enabled:  true,
					Task:     monitoring.NewQuoteBackfillRunner(backfillDeps),
				})
				log.Printf("[Gateway] registered auto_quote_backfill background task (24h, FinMind)")
			}

			// Register tej_refresh — daily TEJ data refresh after market close (15:00 TW).
			// Issue #1086: TEJ channel data was stale for 66 days because no periodic fetch existed.
			// Fires every hour but only executes at 15:00 TW to avoid daily API quota exhaustion.
			// Reference: daily_report_generate pattern (lines ~1047) for time-gated ErrTaskSkipped.
			// DISABLED 2026-08-03 — TEJ free trial API key (AAA003) expired on 2026-07-31.
			// Source-layer audit (PR chore/20260803-disable-tej) confirmed no mission-required
			// endpoint consumes TEJ data; the system has been running with tej dead for days.
			// Scheduler registration is gated on the same TEJ_API_KEY secret that gates
			// channel registration in internal/apigateway/register_adapters.go (~L122) so
			// channel and scheduler stay in lock-step (T3-A47 fix). To re-enable, set
			// TEJ_API_KEY (and TEJ_TIER=paid if upgraded) in the atlas env — the channel
			// adapter and tej_refresh scheduler will both come back online.
			//
			// Health-record side effect: when TEJ_API_KEY is unset, register_adapters.go
			// writes status="inactive" so dashboard + Alerts() stop surfacing the stale
			// AAA003 error. This scheduler branch is intentionally a no-op (log only) —
			// the health-record write is centralized in the adapter layer to avoid
			// double-writes between the two gate sites.
			if tejAPIKey := config.GetSecret("TEJ_API_KEY"); tejAPIKey != "" {
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:      "tej_refresh",
					ChannelID: "tej",
					Interval:  1 * time.Hour,
					Enabled:   true,
					Task: func(ctx context.Context) error {
						taipei, err := time.LoadLocation("Asia/Taipei")
						if err != nil {
							taipei = time.FixedZone("CST", 8*3600)
						}
						now := time.Now().In(taipei)
						// Only fetch at 15:00 TW to avoid quota exhaustion.
						if now.Hour() != 15 {
							return apigateway.ErrTaskSkipped
						}
						_, fetchErr := gateway.Fetch(ctx, "tej")
						if fetchErr != nil {
							return fmt.Errorf("tej_refresh fetch: %w", fetchErr)
						}
						logging.Info("tej_refresh", "completed", "channel", "tej")
						return nil
					},
				})
				log.Printf("[Gateway] registered tej_refresh background task (1h interval, 15:00 TW trigger)")
			} else {
				log.Printf("[Gateway] tej_refresh SKIPPED: TEJ_API_KEY not configured (channel disabled; see PR chore/20260803-disable-tej)")
			}

			// Register janus_regime_refresh — periodic JANUS regime data refresh (6h interval).
			// Issue #1086: janus_regime channel data was stale because no periodic fetch existed.
			// JANUS engine.Update() early-returns if no new PRISM snapshot exists (internal/janus/engine.go ~L71),
			// so no TW time gating is needed. 6h interval allows max 4 fetches/day without excessive load.
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:      "janus_regime_refresh",
				ChannelID: "janus_regime",
				Interval:  6 * time.Hour,
				Enabled:   true,
				Task: func(ctx context.Context) error {
					_, fetchErr := gateway.Fetch(ctx, "janus_regime")
					if fetchErr != nil {
						return fmt.Errorf("janus_regime_refresh fetch: %w", fetchErr)
					}
					logging.Info("janus_regime_refresh", "completed", "channel", "janus_regime")
					return nil
				},
			})
			log.Printf("[Gateway] registered janus_regime_refresh background task (6h interval)")

			mux.HandleFunc("/universe/metrics", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(um.Snapshot())
			})

			riskGate := risk.NewRiskGate(risk.NewPreTradeGate(), risk.NewInTradeGate(), risk.NewPostTradeGate())
			if maturityTracker != nil {
				riskGate.WithMaturityTracker(maturityTracker)
			}
			dashboard.SetRiskGate(riskGate)

			// Forward every risk gate decision to the shared EventBus so SSE
			// clients and audit subscribers receive real-time risk events.
			riskGate.Subscribe(func(dec risk.RiskDecision) {
				dashEventBus.PublishRiskGateEvent(eventbus.RiskGateEventPayload{
					Phase:                string(dec.Phase),
					Verdict:              string(dec.Verdict),
					Reason:               dec.Reason,
					ActionType:           string(dec.Action.Type),
					ActionDescription:    dec.Action.Description,
					Mode:                 dec.Mode,
					Symbol:               dec.Symbol,
					Timestamp:            dec.Recorded,
					ConfidenceCommentary: dec.ConfidenceCommentary,
				})
			})

			if params := config.GetParametersConfig(); params != nil && params.RSITw.LastCalibratedScore.Value > 0 {
				riskGate.SetPreTradeRSITwScore(params.RSITw.LastCalibratedScore.Value)
				log.Printf("[RiskGate] restored RSI-tw calibration score: %.4f", params.RSITw.LastCalibratedScore.Value)
			}

			stSeedsPath = filepath.Join(cfg.WorkDir, "data/seeds/strategy_techniques.json")
			if stReg, err := strategy_techniques.LoadFromFile(stSeedsPath); err == nil {
				stRegistry = stReg
				// Manifest #F07: persist validate-API results so hit_rate
				// accumulates across backtest runs.
				stHandlers = apistrategies.NewHandlers(stRegistry,
					apistrategies.NewFeedbackStore(filepath.Join(cfg.LedgerDir, "strategy_feedback")))
				dashboard.SetStrategiesHandlers(stHandlers)
				if methodologyAdvisor != nil {
					dashboard.SetStrategiesMethodologyAdvisor(methodologyAdvisor)
				}
				// Re-register: RegisterAllRoutes ran before SetStrategiesHandlers,
				strategyFeedbackStore = stHandlers.FeedbackStore
				// so the original call encountered a nil handler. nil-safe.
				dashboard.RegisterStrategiesRoutes(mux)
				logging.Info("main", "strategy_techniques_loaded", "count", stRegistry.Count(), "path", stSeedsPath)
				strategyRanker.RegisterRoutesWithMetrics(mux, stRegistry, filepath.Join(cfg.WorkDir, "data/state/macro"))
				log.Printf("[StrategyRanker] registered /api/strategy-ranker/* routes with metrics")
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

			// Wire strategy summary handler (opt-in; 503 until wired — #1280).
			if stRegistry != nil {
				dashboard.SetStrategiesSummaryHandler(capabilities.NewStrategySummaryHandler(llmRouter))
			}

			// =============================================================================
			// Phase 2: LLM Capability Wiring (opt-in via LLM_*_ENABLED flags)
			// =============================================================================

			// Mirrors buildRouter pattern from cmd/lint-pr/lint-prompts.
			// Only *llm.ProviderImpl escapes the closure; *clients.XxxClient
			// never lives in this scope (constitution: clients/* usage must
			// be adapter-bounded).
			registerProvider := func(secret string, build func(string) llm.ProviderImpl) {
				apiKey := config.GetSecret(secret)
				if apiKey == "" {
					return
				}
				impl := build(apiKey)
				if impl == nil {
					return
				}
				_ = llmRouter.Register(impl)
			}

			registerProvider("LLM_DEEPSEEK_API_KEY", func(apiKey string) llm.ProviderImpl {
				c := clients.NewDeepSeekClient(apiKey, nil)
				if collector != nil {
					c.Metrics = collector
				}
				return llmAdapters.NewDeepSeekAdapter(c, "deepseek-v4-pro")
			})
			registerProvider("LLM_MINIMAX_API_KEY", func(apiKey string) llm.ProviderImpl {
				c := clients.NewMiniMaxClient(apiKey, nil)
				if collector != nil {
					c.Metrics = collector
				}
				return llmAdapters.NewMiniMaxAdapter(c)
			})
			registerProvider("LLM_KIMI_API_KEY", func(apiKey string) llm.ProviderImpl {
				c := clients.NewKimiClient(apiKey, nil)
				if collector != nil {
					c.Metrics = collector
				}
				return llmAdapters.NewKimiAdapter(c)
			})

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
				dashEventBus.Subscribe(eventbus.EventDrawdownBreach, func(ctx context.Context, ev eventbus.BusEvent) error {
					logging.Error("monitor", "drawdown_breach",
						"id", ev.ID, "payload", fmt.Sprintf("%+v", ev.Payload),
						"description", ev.Description)
					return nil
				})
				log.Printf("[Monitor] subscribed to regime/drawdown events")
			}

			log.Printf("[RiskGate] injected into DashboardAPI for calibration reports")

			if realtimeAdapter != nil {
				go realtimeAdapter.Start(sysCtx)
				log.Printf("[RealTime] adapter started (cadence=%dms)", paramsCfg.Realtime.UpdateIntervalMs.Value)
			}

			var btRunner *autobacktest.Runner
			if dashboard.GetEventBus() != nil {
				btRunner = autobacktest.NewRunnerWithEventBus(cfg, dashboard.GetEventBus())
				log.Printf("[AutoBacktest] connected to Dashboard EventBus for SSE streaming")
			} else {
				btRunner = autobacktest.NewRunner(cfg)
				log.Printf("[AutoBacktest] running without EventBus (no SSE events)")
			}
			// Wire attribution pipeline (#1259): every backtest writes a
			// measured-attribution marker per active strategy so listActive
			// can distinguish 'never validated' from 'validated, no hits yet'.
			if stRegistry != nil && strategyFeedbackStore != nil {
				btRunner.WithAttribution(
					strategyAttributionAdapter{fb: strategyFeedbackStore},
					stRegistry,
				)
			}
			// Wire condition evaluator (Phase 2 of #1259): evaluate
			// strategy conditions against historical macro data to
			// compute actual hit_rate.
			if stRegistry != nil {
				snapDir := filepath.Join(cfg.WorkDir, constants.StateMacro)
				snapshots, err := strategy_techniques.LoadSnapshotsFromDir(snapDir)
				if err != nil {
					log.Printf("[AutoBacktest] condition evaluator skipped: load snapshots from %s: %v", snapDir, err)
				} else if len(snapshots) > 0 {
					evaluator := strategy_techniques.NewMacroSnapshotEvaluator(snapshots, 1)
					btRunner.WithConditionEvaluator(evaluator)
					// Also wire the evaluator into the strategies API so
					// GET /api/strategies reflects current backtest logic
					// rather than stale seed values.
					if stHandlers != nil {
						stHandlers.SnapshotEvaluator = evaluator
					}
					log.Printf("[AutoBacktest] condition evaluator wired with %d snapshots", len(snapshots))
				} else {
					log.Printf("[AutoBacktest] condition evaluator skipped: no dated snapshots in %s", snapDir)
				}
			}
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "autobacktest_daily",
				Interval: 1 * time.Hour,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					err := autobacktest.RunScheduledBacktest(ctx, btRunner)
					if errors.Is(err, autobacktest.ErrNotInWindow) {
						return apigateway.ErrTaskSkipped
					}
					if err != nil {
						return err
					}
					// Manifest #F08: consume the autobacktest CIRCUIT_BREAKER
					// signal (previously orphan). Only fires when the daily
					// backtest actually ran, so SignalEngine has fresh
					// outcomes to evaluate against.
					if signalErr := autobacktest.SignalApply(ctx, cfg.LedgerDir, gateway); signalErr != nil {
						logging.Warn("autobacktest", "signal_apply_failed", "err", signalErr)
					}
					return nil
				},
			})
			log.Printf("[Gateway] registered autobacktest_daily background task (1h interval)")

			registerCalibrationTasks(calibrationDeps{
				TaskMgr:          taskMgr,
				Cfg:              cfg,
				ParamsCfg:        paramsCfg,
				RiskGate:         riskGate,
				JanusEngine:      janusEngine,
				Dashboard:        dashboard,
				FinMindClient:    nil,
				MaturityTracker:  maturityTracker,
				CalProvider:      monitoring.NewSessionCalibrationProvider(filepath.Join(cfg.WorkDir, "data/state")),
				Collector:        collector,
				PredictionLedger: ledger.NewJSONLEventFlowPredictionStore(cfg.LedgerDir),
				HistoricalStore:  historicalStore,
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
		// /api/health is kept as an alias for /health so external probes that
		// expect an /api/ prefix still succeed. It does not reimplement a
		// separate health schema: the canonical implementation lives at /health
		// in cmd/atlas/api_routes.go (newHealthHandler). The legacy dual

		// implementation (C02 follow-up) and the silent 200-on-degraded behavior
		// have been removed.
		mux.Handle("GET /api/health", http.RedirectHandler("/health", http.StatusMovedPermanently))
		mux.Handle("GET /api/version", http.HandlerFunc(dashboard.HandleVersion))
		// F-02: API route discovery — serve known endpoint mappings as JSON.
		// This is a curated list of top-level routes; for the full MCP tool
		// inventory, connect via MCP protocol (atlas-mcp stdio server).
		routesJSON := `{"routes":[{"pattern":"GET /api/routes","description":"This route list","auth_required":false},{"pattern":"GET /api/health","description":"Alias for /health (redirects)","auth_required":false},{"pattern":"GET /health","description":"Detailed health with port probes","auth_required":false},{"pattern":"GET /api/market/explain","description":"今日台股解說","auth_required":true},{"pattern":"GET /api/capital-flow/daily","description":"七維錢潮雷達 daily","auth_required":false},{"pattern":"GET /api/capital-flow/summary","description":"錢潮摘要","auth_required":false},{"pattern":"GET /api/capital-flow/history","description":"歷史資金流向","auth_required":false},{"pattern":"GET /api/regime/history","description":"市場體質歷史","auth_required":false},{"pattern":"GET /api/dashboard/system-health","description":"完整系統健康","auth_required":false},{"pattern":"GET /api/narrative/stress-index/current","description":"壓力指數","auth_required":false},{"pattern":"GET /api/taiwan/stress-index","description":"壓力指數（前端用）","auth_required":false},{"pattern":"GET /api/cross-market/status","description":"跨市場狀態","auth_required":false},{"pattern":"GET /api/dashboard/risk","description":"風險指標 VaR","auth_required":false},{"pattern":"MCP tools (97 total)","description":"使用 MCP 協定連接 atlas-mcp stdio server","auth_required":true}],"count":14}`
		mux.Handle("GET /api/routes", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(routesJSON))
		}))

		// C-04: catch-all 404 for unmatched /api/* routes (any method).
		// Registered last — mux matches most specific pattern first.
		mux.Handle("/api/{rest...}", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "route not found",
				"code":  "404",
				"path":  r.URL.Path,
			})
		}))

		authWrappedMux := apishared.AuthMiddleware(mux)
		finalMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")
			if isPublicPath(r.URL.Path) {
				mux.ServeHTTP(w, r)
				return
			}
			authWrappedMux.ServeHTTP(w, r)
		})
		srv := &http.Server{
			Addr:              *apiAddr,
			Handler:           finalMux,
			ReadTimeout:       5 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       120 * time.Second,
			ReadHeaderTimeout: 10 * time.Second,
		}
		srvErr := make(chan error, 1)
		go func() {
			if err := deps.listenAndServe(srv); err != nil && !errors.Is(err, http.ErrServerClosed) {
				srvErr <- fmt.Errorf("dashboard api server failed: %w", err)
			}
		}()

		// Startup deadline: if the server goroutine hasn't reported an error
		// within 10 seconds, assume startup succeeded and proceed to the
		// graceful shutdown select loop. If srvErr fires during this window,
		// treat it as a startup failure.
		startupTimer := time.NewTimer(10 * time.Second)
		sigCh := registerShutdownSignal()
		select {
		case <-startupTimer.C:
			// Server started successfully — proceed to long-running shutdown loop
			logging.Info("main", "server_startup_ok", "addr", *apiAddr)
		case err := <-srvErr:
			startupTimer.Stop()
			sysCancel()
			return err
		case <-sigCh:
			startupTimer.Stop()
			logging.Info("main", "shutdown_during_startup", "addr", *apiAddr)
			sysCancel()
			return nil
		case <-deps.shutdown:
			startupTimer.Stop()
			logging.Info("main", "deps_shutdown_during_startup", "addr", *apiAddr)
			sysCancel()
			return nil
		}

		// Server is running — enter long-running graceful shutdown select
		select {
		case <-sigCh:
			logging.Info("main", "shutdown_signal_received", "mode", "api")
		case err := <-srvErr:
			sysCancel()
			return err
		case <-deps.shutdown:
			logging.Info("main", "shutdown_deps_triggered", "mode", "api")
		}

		sysCancel()
		if realtimeAdapter != nil {
			realtimeAdapter.Stop()
			log.Printf("[RealTime] adapter stopped")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			logging.Error("main", "graceful_shutdown_failed", "mode", "api", logging.Err(err))
		} else {
			logging.Info("main", "server_stopped", "mode", "api")
		}
		return nil
	}
	if *liveMode {
		return runLiveTrading(cfg, compositionRoot, deps, collector, repo, baselineMgr, *apiAddr, *forceIntradayCycles)
	}
	return runSimulation(cfg, compositionRoot, false, collector, repo, deps.shutdown)
}

func runSimulation(cfg config.Config, root *composition.Root, verbose bool, collector *monitoring.MetricsCollector, repo *repository.DualWriteRepository, shutdown <-chan struct{}) error {
	eventBus := eventbus.NewChannelEventBus(64)
	var system *orchestrator.System
	var err error
	if root != nil {
		system, err = root.BuildSystem(composition.PathCLISimulation, eventBus, nil)
	} else {
		system, err = orchestrator.NewProductionSystemWithEventBus(cfg, eventBus, nil)
	}
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

		// SA08: When sector allocation closure is enabled, skip the
		// legacy live-store sync. Simulation positions are simulation
		// artifacts; they must not contaminate live state.
		// The closure policy file is the authoritative source of truth
		// for next-session sector allocation.
		if os.Getenv("ATLAS_SECTOR_ALLOCATION_CLOSURE_ENABLED") == "" {
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
		} else {
			logging.Info("main", "live_state_sync_skipped",
				"reason", "sector_allocation_closure_enabled")
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

func runLiveTrading(cfg config.Config, root *composition.Root, deps appDeps, collector *monitoring.MetricsCollector, repo *repository.DualWriteRepository, baselineMgr *baseline.Manager, apiAddr string, forceIntradayCycles bool) error {
	eventBus := live.NewChannelEventBus(64)
	var system *orchestrator.System
	var err error
	if root != nil {
		system, err = root.BuildSystem(composition.PathLiveTrading, eventBus, nil)
	} else {
		system, err = orchestrator.NewProductionSystemWithEventBus(cfg, eventBus, nil)
	}
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

	stateStore := livestore.NewStateStore(constants.StateLive)
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
	// Register dashboard buffer catchup hooks on the LIVE bus.  The earlier
	// registration in `run()` wired the same hooks on the simulation bus,
	// but in runLiveTrading every Wave 9 detector (and the rest of the live
	// system) publishes to this bus, so the catchup buffer must follow.
	apievents.RegisterDashboardBufferSubs(eventBus)
	risk.NewAuditSubscriber(eventBus)
	log.Printf("[EventBus] buffer catchup and risk audit subscriber re-registered on live bus")
	dashboard.RegisterAllRoutes(mux, monitoring.RouteOptions{IncludeBacktest: false, IncludeSwagger: true})
	alertStore, err := monitoring.NewAlertStore(filepath.Join(cfg.WorkDir, "data/state/alerts"))
	if err != nil {
		log.Printf("[Alerts] failed to create alert store: %v", err)
	} else {
		alertAPI := monitoring.NewAlertAPI(alertStore)
		wave9Src := alertscanner.NewWave9Source(eventBus, 256)
		if err := wave9Src.Start(); err != nil {
			log.Printf("[AlertScanner] wave9 source start failed: %v", err)
		}
		alertAPI.WithAlertSources(wave9Src)
		// Narrative source: surfaces high-severity macro narrative detections
		// as alerts so detected themes actively warn (表裡閉環 Wave 5).
		narrativeSrc := alertscanner.NewNarrativeSource(eventBus, 256)
		if err := narrativeSrc.Start(); err != nil {
			log.Printf("[AlertScanner] narrative source start failed: %v", err)
		}
		alertAPI.WithAlertSources(narrativeSrc)
		alertAPI.RegisterRoutes(mux)
		// Route C: scan for in-flight alerts after process restart.
		scanCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		scanner := alertscanner.New(alertStore)
		if alerts, err := scanner.Scan(scanCtx); err != nil {
			log.Printf("[AlertScanner] startup scan failed: %v", err)
		} else if len(alerts) > 0 {
			log.Printf("[AlertScanner] %d unacknowledged alerts loaded from store", len(alerts))
		}
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

	adminSubFS, err := fs.Sub(admin_web.DistFS, "dist")
	if err != nil {
		log.Fatalf("failed to get admin dist sub FS: %v", err)
	}
	clientSubFS, err := fs.Sub(client_web.DistFS, "dist")
	if err != nil {
		log.Fatalf("failed to get client dist sub FS: %v", err)
	}
	// Static routes and basic probes are registered through the same helper
	// used by api-mode so live trading and simulation behave identically.
	rc := readyChecker{
		dbDSN:       os.Getenv("DATABASE_URL"),
		replayPath:  config.GetReplayDataPath(cfg.WorkDir),
		skipGateway: true, // live mode does not initialize apigateway.Gateway
	}
	registerSimpleRoutes(mux, collector, adminSubFS, clientSubFS, rc, apiAddr)
	srv := &http.Server{
		Addr:              apiAddr,
		Handler:           mux,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("dashboard api listening on %s", apiAddr)
		if err := deps.listenAndServe(srv); err != nil && err != http.ErrServerClosed {
			logging.Error("main", "server_failed", "mode", "live", logging.Err(err))
		}
	}()

	// Wave 9 observability detectors (event-driven; managed via defer, not BTM).
	healthStore := apigateway.NewUnifiedHealthStore(filepath.Join(cfg.WorkDir, "data/state"), nil)
	var weightProvider monitoringservice.WeightProvider
	if engine := system.Port().FactorWeightEngine(); engine != nil {
		weightProvider = monitoringservice.NewFactorWeightEngineWeightProvider(engine)
	}
	wave9, err := monitoring.NewWave9Observability(
		eventBus,
		monitoring.WithWeightProvider(weightProvider),
		monitoring.WithChannelHealthProvider(monitoring.NewChannelHealthProviderFromStore(healthStore)),
		monitoring.WithIngestionLagProvider(monitoringservice.NewChannelHealthIngestionLagProvider(healthStore)),
	)
	if err != nil {
		return fmt.Errorf("create wave9 observability: %w", err)
	}
	defer func() {
		if err := wave9.Stop(); err != nil {
			logging.Warn("main", "wave9_stop_failed", logging.Err(err))
		}
	}()
	if err := wave9.Start(ctx); err != nil {
		return fmt.Errorf("start wave9 observability: %w", err)
	}

	// Baseline trigger: evaluates position updates against the current baseline policy constraints.
	// Wired as a standalone lifecycle component (independent of Wave 9 detectors).
	baselineTrigger := baseline.NewTrigger(baselineMgr, eventBus)
	if err := baselineTrigger.Start(ctx); err != nil {
		logging.Error("main", "baseline_trigger_start_failed", "error", err.Error())
		return err
	}
	defer func() {
		if err := baselineTrigger.Stop(); err != nil {
			logging.Warn("main", "baseline_trigger_stop_failed", logging.Err(err))
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
		logging.Error("main", "graceful_shutdown_failed", "mode", "live", logging.Err(err))
	}
	if err := o.Stop(); err != nil {
		return fmt.Errorf("stop live orchestrator: %w", err)
	}
	log.Println("live trading orchestrator stopped")
	return nil
}

// runPrismWorker runs the PRISM training-queue worker as a long-lived
// daemon. The PRISM manager spawns worker goroutines per regime queue
// and processes tasks as they are enqueued. Without a TrainingExecutor
// wired up here, tasks complete in synthetic mode (placeholder metrics);
// a real executor is wired by the atlas API service when it is running
// alongside this container and shares the same Postgres/Redis backend.
//
// Shutdown: SIGINT/SIGTERM trigger a graceful stop of the manager. The
// docker restart policy is "unless-stopped", so the container will be
// restarted automatically after clean exit.
//
// Regression context: this function exists because the docker-compose
// `prism-worker` service invokes `atlas-go prism worker`. Before this
// handler was added, the positional args were ignored and run() fell
// through to runSimulation() (a one-shot), producing a 60-second
// restart loop that the user perceived as the system "abnormally
// closing after running for a while".
// runPrismWorker runs the PRISM training-queue worker as a long-lived
// daemon. The error return is intentionally always nil today
// (prism.NewPRISMManager().Start()/Stop() do not return error), but the
// signature is kept consistent with the other run* dispatch targets
// (runLiveTrading, runSimulation) so run() can route subcommands
// without special-casing each one. If Start/Stop ever grow error
// returns, wire them through here.
func runPrismWorker(cfg config.Config, deps appDeps) error { //nolint:unparam
	cfgPrism := prism.DefaultPRISMConfig()
	logging.Info(
		"prism_worker", "starting",
		"workdir", cfg.WorkDir,
		"replay_session_date", cfg.ReplaySessionDate,
		"queue_size", cfgPrism.QueueSize,
		"workers_per_queue", cfgPrism.WorkersPerQueue,
		"regimes", int(prism.RegimeCount),
	)

	mgr := prism.NewPRISMManager(cfgPrism)
	mgr.Start()
	logging.Info("prism_worker", "started", "regime_queues", int(prism.RegimeCount))

	sigCh := registerShutdownSignal()
	select {
	case sig := <-sigCh:
		logging.Info("prism_worker", "shutdown_signal", "signal", sig.String())
	case <-deps.shutdown:
		logging.Info("prism_worker", "deps_shutdown")
	}

	mgr.Stop()
	logging.Info("prism_worker", "stopped")
	return nil
}

func runSimulationMode(rt *bootstrap.Runtime, cfg config.Config, root *composition.Root, verbose bool, dateOverride string) error {
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
	if err := runSimulation(cfg, root, verbose, collector, repo, nil); err != nil {
		return fmt.Errorf("simulation failed: %w", err)
	}

	if verbose {
		log.Printf("[SIMULATE] simulation completed for %s", sessionDate)
	}

	return nil
}

// buildSystemOrFallback creates a System through the composition root when
// available (six-path matrix), falling back to the legacy production factory
// when the root is nil (e.g. SA06 not yet wired or pre-bootstrap paths).
func buildSystemOrFallback(
	root *composition.Root,
	path composition.CompositionPath,
	cfg config.Config,
	eventBus *eventbus.ChannelEventBus,
	janusEngine *janus.Engine,
	eventPredictor orchestrator.EventFlowPredictor,
	capitalFlowService *capitalflow.Service,
) (*orchestrator.System, error) {
	var sys *orchestrator.System
	var err error
	if root != nil {
		sys, err = root.BuildSystem(path, eventBus, janusEngine)
	} else {
		sys, err = orchestrator.NewProductionSystemWithEventBus(cfg, eventBus, janusEngine)
	}
	if err != nil {
		return nil, err
	}
	if eventPredictor != nil {
		sys.WithEventPredictor(eventPredictor)
	}
	if capitalFlowService != nil {
		sys.WithCapitalFlowAssessmentProvider(orchestrator.NewCapitalFlowServiceAdapter(capitalFlowService))
	}
	return sys, nil
}
