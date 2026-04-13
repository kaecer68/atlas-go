package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/live"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
)

type routeRegistrar interface {
	RegisterRoutes(mux *http.ServeMux)
	RegisterSwaggerRoutes(mux *http.ServeMux)
	RegisterNarrativeRoutes(mux *http.ServeMux)
	RegisterControlRoutes(mux *http.ServeMux)
	RegisterMacroRoutes(mux *http.ServeMux)
}

type appDeps struct {
	loadConfig      func() config.Config
	newDashboardAPI func(string) routeRegistrar
	listenAndServe  func(string, http.Handler) error
}

func defaultAppDeps() appDeps {
	return appDeps{
		loadConfig:      config.Load,
		newDashboardAPI: func(ledgerDir string) routeRegistrar { return monitoring.NewDashboardAPI(ledgerDir) },
		listenAndServe:  http.ListenAndServe,
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
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	cfg := deps.loadConfig()
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

	if *apiMode {
		mux := http.NewServeMux()
		dashboard := deps.newDashboardAPI(cfg.LedgerDir)
		dashboard.RegisterRoutes(mux)
		dashboard.RegisterNarrativeRoutes(mux)
		dashboard.RegisterControlRoutes(mux)
		dashboard.RegisterMacroRoutes(mux)
		if d, ok := dashboard.(*monitoring.DashboardAPI); ok {
			d.RegisterPhase3Routes(mux)
		}
		if *swaggerMode {
			dashboard.RegisterSwaggerRoutes(mux)
		}
		log.Printf("dashboard api listening on %s", *apiAddr)
		if err := deps.listenAndServe(*apiAddr, mux); err != nil {
			return fmt.Errorf("dashboard api server failed: %w", err)
		}
		return nil
	}

	if *liveMode {
		return runLiveTrading(cfg, deps)
	}
	return runSimulation(cfg)
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

func runSimulation(cfg config.Config) error {
	system := orchestrator.NewProductionSystem(cfg)

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

func runLiveTrading(cfg config.Config, deps appDeps) error {
	system := orchestrator.NewProductionSystem(cfg)

	stateStore := live.NewStateStore("data/state/live")
	eventBus := live.NewChannelEventBus(64)
	provider := marketdata.NewTWSEProvider()

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

	o := live.NewOrchestrator(
		stateStore,
		eventBus,
		provider,
		system.Registry(),
		system,
		liveCfg,
	)

	// Metrics collector for live trading observability
	collector := monitoring.NewMetricsCollector()
	monitor := monitoring.NewMonitor()
	tradingMetrics := monitoring.NewTradingMetrics(collector, monitor)
	o.SetTradingMetrics(tradingMetrics)

	// Alert rule engine for live trading safety
	ruleEngine := monitoring.NewRuleEngine(monitor)
	for _, rule := range monitoring.LiveTradingRules() {
		ruleEngine.RegisterRule(rule)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ruleEngine.Start(ctx, stateStore)

	// Start dashboard API server for live status endpoint
	mux := http.NewServeMux()
	dashboard := deps.newDashboardAPI(cfg.LedgerDir)
	dashboard.RegisterRoutes(mux)
	dashboard.RegisterNarrativeRoutes(mux)
	dashboard.RegisterControlRoutes(mux)
	dashboard.RegisterMacroRoutes(mux)
	if d, ok := dashboard.(*monitoring.DashboardAPI); ok {
		d.RegisterSwaggerRoutes(mux)
		d.RegisterPhase3Routes(mux)
		d.RegisterLiveRoutes(mux)
	}
	apiAddr := ":8080"
	go func() {
		log.Printf("dashboard api listening on %s", apiAddr)
		if err := deps.listenAndServe(apiAddr, mux); err != nil {
			log.Printf("dashboard api server failed: %v", err)
		}
	}()

	log.Printf("starting live trading orchestrator (broker_mode=%s)", liveCfg.BrokerMode)
	if err := o.Start(); err != nil {
		return fmt.Errorf("start live orchestrator: %w", err)
	}

	// Graceful shutdown after a brief run for CLI demonstration.
	select {
	case <-time.After(5 * time.Second):
	case <-ctx.Done():
	}
	if err := o.Stop(); err != nil {
		return fmt.Errorf("stop live orchestrator: %w", err)
	}
	log.Println("live trading orchestrator stopped")
	return nil
}
