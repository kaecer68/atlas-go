package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
)

type routeRegistrar interface {
	RegisterRoutes(mux *http.ServeMux)
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
	brokerMode := flags.String("broker-mode", "", "override broker mode: dry-run|paper|live")
	brokerAdapter := flags.String("broker-adapter", "", "override broker adapter: guarded|mock|http")
	brokerMaxRetries := flags.Int("broker-max-retries", -1, "override broker max retries (>=0)")
	allowLiveBroker := flags.Bool("allow-live-broker", false, "allow live broker mode (default false)")
	allowHTTPBroker := flags.Bool("allow-http-broker", false, "allow http broker adapter in live mode (default false)")
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
	if *brokerMaxRetries >= 0 {
		cfg.BrokerMaxRetries = *brokerMaxRetries
	}
	if err := validateBrokerRuntimeConfig(&cfg, *allowLiveBroker, *allowHTTPBroker); err != nil {
		return err
	}

	if *apiMode {
		mux := http.NewServeMux()
		dashboard := deps.newDashboardAPI(cfg.LedgerDir)
		dashboard.RegisterRoutes(mux)
		log.Printf("dashboard api listening on %s", *apiAddr)
		if err := deps.listenAndServe(*apiAddr, mux); err != nil {
			return fmt.Errorf("dashboard api server failed: %w", err)
		}
		return nil
	}

	return runSimulation(cfg)
}

func validateBrokerRuntimeConfig(cfg *config.Config, allowLiveBroker bool, allowHTTPBroker bool) error {
	cfg.BrokerMode = strings.TrimSpace(strings.ToLower(cfg.BrokerMode))
	if cfg.BrokerMode == "" {
		cfg.BrokerMode = "dry-run"
	}
	cfg.BrokerAdapter = strings.TrimSpace(strings.ToLower(cfg.BrokerAdapter))
	if cfg.BrokerAdapter == "" {
		cfg.BrokerAdapter = "guarded"
	}
	if cfg.BrokerAdapter != "guarded" && cfg.BrokerAdapter != "mock" && cfg.BrokerAdapter != "http" {
		return fmt.Errorf("unsupported broker adapter %q (allowed: guarded, mock, http)", cfg.BrokerAdapter)
	}

	switch cfg.BrokerMode {
	case "dry-run", "paper":
		// allowed by default
	case "live":
		if !allowLiveBroker {
			return fmt.Errorf("broker mode %q is disabled by default; pass -allow-live-broker to enable", cfg.BrokerMode)
		}
		if cfg.BrokerAdapter == "http" && !allowHTTPBroker {
			return fmt.Errorf("broker adapter %q is disabled by default in live mode; pass -allow-http-broker to enable", cfg.BrokerAdapter)
		}
	default:
		return fmt.Errorf("unsupported broker mode %q (allowed: dry-run, paper, live)", cfg.BrokerMode)
	}

	if cfg.BrokerMaxRetries < 0 {
		return fmt.Errorf("broker max retries must be >= 0, got %d", cfg.BrokerMaxRetries)
	}

	return nil
}

func runSimulation(cfg config.Config) error {
	system := orchestrator.NewSystem(cfg)

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
