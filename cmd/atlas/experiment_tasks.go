package main

// PR10c-3: Experiment + simulation + ML + universe + swarm background
// task registration. Extracted from main.go run() to reduce file size
// and improve testability. Tasks here are periodic experiment, backtest,
// ML retrain, universe build, and swarm simulation work that runs
// independently of the realtime / live trading paths.
//
// Tasks (11 total):
//   1. auto_daily_simulation     — system.RunDailySimulation (24h)
//   2. etf_nav_refresh           — gateway.Fetch twse_replay (24h)
//   3. stress_test_daily         — system.RunDailyStressTests (24h)
//   4. auto_experiment           — experiment.AutoExperiment (7d)
//   5. window_backtest           — backtest.Runner 20d window (7d)
//   6. rule_engine_check         — ruleEngine.EvaluateRules (configurable)
//   7. ml_retrain                — scheduler.NewMLRetrainScheduler (24h)
//   8. auto_universe_refresh     — monitoring.NewDailyUniverseRefreshTask (1m, 06:00 TW)
//   9. auto_universe_full_rebuild — monitoring.NewWeeklyUniverseRebuildTask (1m, Mon 06:00 TW)
//  10. universe_coverage_check   — snapshot vs classification coverage (1m, 06:00 TW)
//  11. auto_swarm_simulation     — ctrl.RunSwarmCycle (30m + 3m jitter)
//
// Out of scope (stays in main.go):
//   - autobacktest_daily: depends on btRunner declared AFTER the
//     `if gateway != nil` block, so it must be registered separately.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/experiment"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/metalearning"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/monitoring/metrics"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/repository"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/scheduler"
	"github.com/kaecer68/atlas-go/internal/swarm"
)

// experimentDeps groups the dependencies needed by all 11 experiment tasks.
// Passed as a struct so the function signature stays compact as new
// tasks are added.
type experimentDeps struct {
	taskMgr        *apigateway.BackgroundTaskManager
	cfg            config.Config
	monitor        *monitoring.Monitor
	dashboard      *monitoring.DashboardAPI
	repo           *repository.DualWriteRepository
	collector      *monitoring.MetricsCollector
	janusEngine    *janus.Engine
	dashEventBus   *eventbus.ChannelEventBus
	gateway        *apigateway.Gateway
	gatewayFetcher monitoring.DataFetcher
	ruleEngine     *monitoring.RuleEngine
	classTree      monitoring.ClassificationTreeAccessor
	um             *metrics.UniverseMetrics
}

// registerExperimentTasks wires the experiment / simulation / backtest /
// ML / universe / swarm tasks into the BackgroundTaskManager. All tasks
// here are fire-and-register: a Register error is logged and the task
// is silently dropped (matches the existing pattern in main.go for
// non-critical background work).
func registerExperimentTasks(d experimentDeps) {
	// Register auto_daily_simulation — runs daily simulation at market close.
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
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

			system, err := orchestrator.NewProductionSystemWithEventBus(d.cfg, d.dashEventBus, d.janusEngine)
			if err != nil {
				return fmt.Errorf("create system: %w", err)
			}
			if d.gatewayFetcher != nil {
				system.Sim().SetProvider(orchestrator.NewGatewayBackedProvider(d.cfg))
			}
			if d.collector != nil {
				system.WithMetricsCollector(d.collector)
			}
			if d.repo != nil {
				system.SetRepository(d.repo)
			}
			if d.dashboard != nil {
				system.SetDrawdownReporter(func(dr portfolio.DrawdownResult) {
					d.dashboard.SetLatestDrawdown(&dr)
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
			if d.dashboard != nil && d.dashboard.GetIndustryService() != nil {
				card, cardErr := d.dashboard.GetIndustryService().BuildCycleStatusCard(nextClose)
				if cardErr == nil && card != nil {
					signals := map[string]float64{
						"silicon":        card.SiliconScore,
						"business_cycle": card.CycleConfidence,
						"seasonal":       card.SeasonalAdjustment,
						"events":         card.EventSentiment,
						"supply_chain":   card.SupplyChainSignal,
					}
					d.dashboard.RecordCycleCalibrationOutcome(
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
				d.monitor.Alert(monitoring.AlertLevelWarning, "simulation",
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
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "etf_nav_refresh",
		ChannelID: "twse_replay",
		Interval:  24 * time.Hour,
		Enabled:   true,
		Task: func(ctx context.Context) error {
			_, err := d.gateway.Fetch(ctx, "twse_replay")
			if err != nil {
				d.monitor.Alert(monitoring.AlertLevelWarning, "etf_nav",
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
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "stress_test_daily",
		Interval: 24 * time.Hour,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			system, err := orchestrator.NewProductionSystemWithEventBus(d.cfg, d.dashEventBus, d.janusEngine)
			if err != nil {
				return fmt.Errorf("create system for stress test: %w", err)
			}
			if d.gatewayFetcher != nil {
				system.Sim().SetProvider(orchestrator.NewGatewayBackedProvider(d.cfg))
			}
			if d.repo != nil {
				system.SetRepository(d.repo)
			}
			if d.dashboard != nil {
				system.SetDrawdownReporter(func(dr portfolio.DrawdownResult) {
					d.dashboard.SetLatestDrawdown(&dr)
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
	monitorAdapter := &experimentMonitorAdapter{m: d.monitor}

	// Register auto_experiment — weekly strategy evolution cycle.
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "auto_experiment",
		Interval: 7 * 24 * time.Hour,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			system, err := orchestrator.NewProductionSystemWithEventBus(d.cfg, d.dashEventBus, d.janusEngine)
			if err != nil {
				return fmt.Errorf("create system: %w", err)
			}
			if d.gatewayFetcher != nil {
				system.Sim().SetProvider(orchestrator.NewGatewayBackedProvider(d.cfg))
			}
			if d.repo != nil {
				system.SetRepository(d.repo)
			}
			if d.dashboard != nil {
				system.SetDrawdownReporter(func(dr portfolio.DrawdownResult) {
					d.dashboard.SetLatestDrawdown(&dr)
				})
			}
			return experiment.AutoExperiment(ctx, experiment.AutoExperimentConfig{
				System:  system,
				Config:  d.cfg,
				Monitor: monitorAdapter,
			})
		},
	})
	log.Printf("[Gateway] registered auto_experiment background task (7-day interval)")

	// Register window_backtest — periodic 20-day scoring window (7-day interval, offset 3d).
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "window_backtest",
		Interval: 7 * 24 * time.Hour,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			store := ledger.NewStore(d.cfg.LedgerDir)
			btRunner := backtest.NewRunner(d.cfg, store)
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

	if d.ruleEngine != nil {
		params := config.GetParametersConfig().Alert
		_ = d.taskMgr.Register(&apigateway.ScheduledTask{
			Name:     "rule_engine_check",
			Interval: time.Duration(params.RuleEngineIntervalSec.Value) * time.Second,
			Enabled:  true,
			Task: func(ctx context.Context) error {
				d.ruleEngine.EvaluateRules(nil)
				return nil
			},
		})
		log.Printf("[Gateway] registered rule_engine_check background task (%ds interval)", params.RuleEngineIntervalSec.Value)
	}

	{
		mlScheduler := scheduler.NewMLRetrainScheduler(d.cfg.ReplayDataPath)
		mlScheduler.SetWorkDir(d.cfg.WorkDir)
		_ = d.taskMgr.Register(&apigateway.ScheduledTask{
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
	{
		suCfg := config.GetParametersConfig().SmartUniverse
		suDeps := newUniverseBuilderDeps(d.cfg, d.classTree, d.gateway, d.um, suCfg)
		_ = d.taskMgr.Register(&apigateway.ScheduledTask{
			Name:     "auto_universe_refresh",
			Interval: 1 * time.Minute,
			Enabled:  true,
			Task:     monitoring.NewDailyUniverseRefreshTask(suDeps),
		})
		log.Printf("[Gateway] registered auto_universe_refresh background task (1m interval, 06:00 TW trigger)")
	}
	{
		suCfg := config.GetParametersConfig().SmartUniverse
		suDeps := newUniverseBuilderDeps(d.cfg, d.classTree, d.gateway, d.um, suCfg)
		_ = d.taskMgr.Register(&apigateway.ScheduledTask{
			Name:     "auto_universe_full_rebuild",
			Interval: 1 * time.Minute,
			Enabled:  true,
			Task:     monitoring.NewWeeklyUniverseRebuildTask(suDeps),
		})
		log.Printf("[Gateway] registered auto_universe_full_rebuild background task (1m interval, Mon 06:00 TW trigger)")
	}

	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
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
			totalSymbols := monitoring.TotalClassifiedSymbols(d.classTree)
			// Load universe snapshot and count built symbols.
			snapshotPath := filepath.Join(d.cfg.WorkDir, "data", "state", "universe_snapshot.json")
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
					d.monitor.Alert(monitoring.AlertLevelWarning, "universe_coverage",
						fmt.Sprintf("Universe coverage %.1f%% (%d/%d symbols) — snapshot may be stale",
							coveragePct, snapshotSymbols, totalSymbols),
						map[string]any{
							"snapshot_symbols": snapshotSymbols,
							"total_symbols":    totalSymbols,
							"coverage_pct":     coveragePct,
						})
				}
				d.um.CoverageMapped.WithLabelValues("coverage_check", "all").Add(int64(snapshotSymbols))
				d.um.CoverageTotal.WithLabelValues("coverage_check", "all").Add(int64(totalSymbols))
			}
			// Check D6 watchlist size.
			watchlistPath := filepath.Join(d.cfg.WorkDir, "data", "state", "universe_watchlist.json")
			if wlData, rErr := os.ReadFile(watchlistPath); rErr == nil {
				var wl monitoring.Watchlist
				if err := json.Unmarshal(wlData, &wl); err == nil && len(wl.Symbols) > 20 {
					d.monitor.Alert(monitoring.AlertLevelWarning, "universe_watchlist",
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

	// Register auto_swarm_simulation — periodic swarm simulation
	// for training data generation and scenario monitoring.
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "auto_swarm_simulation",
		Interval: 30 * time.Minute,
		Jitter:   3 * time.Minute,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			sys, err := orchestrator.NewProductionSystemWithEventBus(d.cfg, d.dashEventBus, d.janusEngine)
			if err != nil {
				return fmt.Errorf("create system for swarm: %w", err)
			}
			provider := orchestrator.NewGatewayBackedProvider(d.cfg)
			if d.gatewayFetcher != nil {
				sys.Sim().SetProvider(provider)
			}
			ctrl := sys.Phase3Controller()
			if ctrl == nil {
				return nil
			}
			trainingDir := filepath.Join(d.cfg.WorkDir, "data/state/swarm_training")
			ctrl.SetTrainingStore(swarm.NewTrainingStore(trainingDir))
			ctrl.SetSnapshotPath(filepath.Join(d.cfg.WorkDir, "data/state/swarm_latest.json"))
			ctrl.SetMetaLearner(metalearning.NewMetaLearner(metalearning.DefaultMetaLearningConfig()),
				filepath.Join(d.cfg.WorkDir, "data/state/metalearner_state.json"))

			baseState := buildBaseState(provider, []string{"2330.TW", "2317.TW", "2454.TW", "2412.TW", "2308.TW"})
			ctrl.RunSwarmCycle(baseState)
			logging.Info("swarm_btm", "cycle_completed", "symbols", len(baseState.Prices))
			return nil
		},
	})
	log.Printf("[Gateway] registered auto_swarm_simulation background task (30m interval)")
}
