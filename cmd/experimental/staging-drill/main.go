package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/live"
	"github.com/kaecer68/atlas-go/internal/live/store"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
)

func main() {
	var help bool
	flag.BoolVar(&help, "help", false, "show help")
	flag.Parse()
	if help {
		fmt.Println("Usage: staging-drill [--help]")
		fmt.Println("Runs a 12-second live trading smoke test in dry-run mode with isolated temp state.")
		os.Exit(0)
	}

	cfg := config.Load()
	cfg.BrokerMode = "paper" // force paper trading for staging

	log.Println("[Staging Drill] Starting live trading smoke test...")

	// Use isolated temp directories so repeated drills don't pollute production state
	tempDir, err := os.MkdirTemp("", "atlas-staging-drill-*")
	if err != nil {
		log.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	liveStateDir := filepath.Join(tempDir, "live")
	circuitLogPath := filepath.Join(tempDir, "circuit_breaker_log.jsonl")
	circuitStatePath := filepath.Join(tempDir, "circuit_breaker_state.json")

	system := orchestrator.NewProductionSystem(cfg)
	stateStore := store.NewStateStore(liveStateDir)
	eventBus := live.NewChannelEventBus(64)
	provider := marketdata.NewMockProvider()

	liveCfg := live.DefaultOrchestratorConfig()
	liveCfg.MarketOpenTime = "00:00"
	liveCfg.MarketCloseTime = "23:59"
	liveCfg.IntradayInterval = 3 * time.Second
	liveCfg.QuotePollInterval = 2 * time.Second
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
		context.Background(),
		stateStore,
		eventBus,
		provider,
		system.Registry(),
		orchestrator.NewAdapterProducer(provider, system),
		liveCfg,
	)
	o.SetWatchlist([]string{"0050", "2330", "2317"})

	// Metrics & alerting
	collector := monitoring.NewMetricsCollector()
	monitor := monitoring.NewMonitor()
	tradingMetrics := monitoring.NewTradingMetrics(collector, monitor)
	o.SetTradingMetrics(tradingMetrics)

	ruleEngine := monitoring.NewRuleEngine(monitor)
	for _, rule := range monitoring.LiveTradingRules() {
		ruleEngine.RegisterRule(rule)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ruleEngine.Start(ctx, stateStore)

	// Inject custom circuit breaker paths so we can inspect them after the drill
	cb := live.NewCircuitBreaker(circuitLogPath, circuitStatePath)
	cb.ResetDayState(1_000_000)
	o.SetCircuitBreaker(cb)

	if err := o.Start(); err != nil {
		log.Fatalf("[Staging Drill] FAILED to start orchestrator: %v", err)
	}

	log.Println("[Staging Drill] Orchestrator running, observing for 12 seconds...")
	time.Sleep(12 * time.Second)

	if err := o.Stop(); err != nil {
		log.Fatalf("[Staging Drill] FAILED to stop orchestrator cleanly: %v", err)
	}

	// Verification
	report := make(map[string]interface{})
	passed := true

	status := o.Status()
	report["orchestrator_status"] = status
	if !status["is_running"].(bool) {
		report["stop_clean"] = true
	} else {
		report["stop_clean"] = false
		passed = false
	}

	metrics := collector.GetAllMetrics()
	report["metrics_count"] = len(metrics)
	if len(metrics) == 0 {
		log.Println("[Staging Drill] WARNING: no trading metrics recorded")
		passed = false
	} else {
		log.Printf("[Staging Drill] Recorded %d metrics", len(metrics))
	}

	// Check circuit breaker state file from the injected CB
	cbData, cbErr := os.ReadFile(circuitStatePath)
	if cbErr == nil {
		var cbState map[string]interface{}
		if err := json.Unmarshal(cbData, &cbState); err != nil {
			log.Printf("[Staging Drill] WARNING: failed to unmarshal circuit breaker state: %v", err)
			report["circuit_breaker_state"] = "unknown (invalid json)"
		} else {
			report["circuit_breaker_state"] = cbState["state"]
			if cbState["state"] != "normal" {
				log.Printf("[Staging Drill] WARNING: circuit breaker state is %v", cbState["state"])
			}
		}
		if cbState["state"] != "normal" {
			log.Printf("[Staging Drill] WARNING: circuit breaker state is %v", cbState["state"])
		}
	} else {
		report["circuit_breaker_state"] = "unknown (file not found)"
	}

	// Check live-status endpoint via lightweight in-memory check (or just file inspection)
	portfolio := stateStore.GetPortfolio()
	report["portfolio_cash"] = portfolio.Cash
	report["portfolio_day_pnl"] = portfolio.DayPnL
	report["positions_count"] = len(stateStore.GetPositions())

	reportBytes, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(reportBytes))

	if passed {
		log.Println("[Staging Drill] PASSED")
		os.Exit(0)
	}
	log.Println("[Staging Drill] FAILED")
	os.Exit(1)
}
