package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/reflexivity"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/spawning"
	"github.com/kaecer68/atlas-go/internal/swarm"
)

func main() {
	var help bool
	flag.BoolVar(&help, "help", false, "show help")
	flag.Parse()
	if help {
		fmt.Println("Usage: validate-phase3-integration [--help]")
		fmt.Println("Runs a Phase-3 integration smoke test (PRISM, Swarm, Spawning, Reflexivity)")
		fmt.Println("over a short replay window (2026-01-01 to 2026-01-08).")
		os.Exit(0)
	}

	cfg := config.Config{
		ReplayDataPath:     "data/replay/tw_extended_90days.csv",
		BaselinePolicyPath: "data/state/baseline_policy.json",
		AgentRegistryPath:  "configs/agents.json",
		LedgerDir:          "data/state",
		ReplayMode:         "window",
		PrimaryMarket:      "TW",
		MarketDataProvider: "replay",
	}

	ds, err := replay.LoadTWSEOpenDataCSV(cfg.ReplayDataPath)
	if err != nil {
		log.Fatalf("Failed to load replay data: %v", err)
	}
	if len(ds.Dates) < 10 {
		log.Fatalf("Insufficient replay data: %d dates", len(ds.Dates))
	}

	policy, err := baseline.Load(cfg.BaselinePolicyPath)
	if err != nil {
		policy = baseline.DefaultPolicy()
	}

	registry, _ := orchestrator.LoadRegistry(cfg.AgentRegistryPath)
	if len(registry.Agents) == 0 {
		registry = orchestrator.SeedRegistry()
	}

	pm := prism.NewPRISMManager(prism.DefaultPRISMConfig())
	sw := swarm.NewMiroFishSwarm(swarm.DefaultSwarmConfig())
	sm := spawning.NewSpawningManager(&registry, spawning.DefaultSpawningConfig())
	reflex := reflexivity.NewReflexivityEngine()
	store := ledger.NewStore(cfg.LedgerDir)

	ctrl := orchestrator.NewPhase3Controller(&registry, pm, sw, sm, reflex, store)
	pm.Start()
	defer pm.Stop()

	startDate := ds.Dates[0]
	endDate := ds.Dates[min(5, len(ds.Dates)-1)]

	fmt.Printf("=== Phase III Integration Validation ===\n")
	fmt.Printf("Window: %s -> %s\n\n", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	persistentState := domain.NewSimulationState(policy.Constraints.StartingCash)
	for _, date := range ds.Dates {
		if date.Before(startDate) || date.After(endDate) {
			continue
		}
		nextDate, ok := ds.NextDate(date, 1)
		if !ok || nextDate.After(endDate) {
			continue
		}

		dayCfg := cfg
		dayCfg.ReplaySessionDate = date.Format("2006-01-02")
		system := orchestrator.NewSystem(dayCfg)
		system.WithPersistentState(&persistentState)
		system.WithPhase3Controller(ctrl)

		result, err := system.RunDailySimulation(date)
		if err != nil {
			log.Fatalf("Simulation failed on %s: %v", date.Format("2006-01-02"), err)
		}
		fmt.Printf("[%s] regime=%s cash=%.0f portfolio=%.0f positions=%d\n",
			date.Format("2006-01-02"), result.Regime, result.EndingCash, result.PortfolioValue, len(result.Positions))
	}

	baseState := swarm.MarketState{
		Timestamp: endDate,
		Prices:    map[string]float64{"2330.TW": 850},
		Volumes:   map[string]float64{"2330.TW": 1000000},
	}
	ctrl.RunParallelOptimization(baseState, domain.RegimeRiskOn)

	// Give PRISM workers time to process queued tasks
	time.Sleep(500 * time.Millisecond)

	metrics, err := orchestrator.LoadPhase3Metrics("")
	if err != nil {
		log.Fatalf("Failed to load phase3 metrics: %v", err)
	}

	fmt.Println("\n--- 5-Track Metrics ---")
	fmt.Printf("A. Swarm        : running=%v consensus_symbols=%d\n", metrics.SwarmRunning, metrics.SwarmConsensusSymbols)
	fmt.Printf("B. PRISM        : queued=%d completed=%d top_agent=%s (sharpe %.3f)\n",
		metrics.PRISMQueuedTasks, metrics.PRISMCompletedResults, metrics.PRISMTopAgentID, metrics.PRISMTopAgentSharpe)
	fmt.Printf("C. Spawning     : active=%d candidates=%d\n", metrics.SpawningActive, metrics.SpawningCandidates)
	fmt.Printf("D. Reflexivity  : active_loops=%d\n", metrics.ReflexivityActiveLoops)
	fmt.Printf("E. Adversarial  : last_score=%.2f vulnerabilities=%v\n", metrics.AdversarialLastScore, metrics.AdversarialVulnerabilities)

	passed := true
	if metrics.PRISMCompletedResults == 0 {
		fmt.Println("\n⚠️  WARNING: PRISM produced no completed results")
		passed = false
	}
	// Swarm consensus accumulates on a 1-minute ticker; zero symbols at short
	// horizon is expected and not treated as failure.
	if passed {
		fmt.Println("\n✅ Phase III integration validation PASSED")
	} else {
		fmt.Println("\n⚠️  Phase III integration validation finished with warnings")
		os.Exit(1)
	}
}
