package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	cfg := deps.loadConfig()
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
