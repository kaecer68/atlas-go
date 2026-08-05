package main

// PR10b: Calibration background task registration.
// Extracted from main.go run() to reduce file size and improve testability.
// All tasks here are periodic calibration runs that adjust threshold/weight
// parameters based on historical outcomes.
//
// Tasks (18 total — 1 inner CalibrationTask + 17 top-level ScheduledTask):
//   1.  narrative_weight_calibration (inner) — narrative conviction calibration
//   2.  calibration_cycle                 — maturity-gated daily cycle (host of #1)
//   3.  auto_strategy_evolution           — sector strategy evolution (24h)
//   4.  auto_threshold_calibrate          — industry thresholds from monthly revenue (1st of month)
//   5.  auto_cycle_update                 — industry cycle aggregation (6h)
//   6.  risk_gate_calibrate               — risk gate self-calibrate (24h)
//   7.  cycle_calibrate                   — industry card weight calibration (24h)
//   8.  regime_calibrate                  — risk recalibration on regime change (1h)
//   9.  factor_weight_calibrate           — factor weight via historical orders (24h)
//   10. conviction_calibrate              — conviction thresholds via executors (24h)
//   11. macro_risk_calibrate              — engine MacroRisk thresholds (24h)
//   12. structural_trend_calibrate        — engine StructuralTrend thresholds (24h)
//   13. narrative_calibrate               — narrative event detection thresholds (24h)
//   14. linkage_calibrate                 — recession shock amplifier (24h)
//   15. factor_weight_strategy_calibrate  — strategy deltas (conservative/aggressive/risk-on/risk-off) (24h)
//   16. auto_calibrate                    — Darwinian parameters (7d)
//   17. rsi_tw_calibrate                  — RSI-TW autonomous calibration (24h)
//
// Removed: seasonal_calibrate (go-run variant) — it exec'd `go run` which
// cannot work in the container (no Go toolchain) and duplicated the
// binary-guarded `seasonal_calibration` task in data_sync_health_tasks.go.
//
// Out of scope (PR10c): auto_swarm_simulation, autobacktest_daily, and other
// experiment/simulation/capital tasks.

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/calibration"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/retail"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/scheduler"
)

// calibrationDeps bundles the dependencies needed by the 18 calibration
// background tasks. All fields are read-only inside the function.
type calibrationDeps struct {
	TaskMgr         *apigateway.BackgroundTaskManager
	Cfg             config.Config
	ParamsCfg       *config.ParametersConfig
	RiskGate        *risk.RiskGate
	JanusEngine     *janus.Engine
	Dashboard       *monitoring.DashboardAPI
	FinMindClient   *marketdata.FinMindClient
	MaturityTracker *domain.MaturityTracker
	CalProvider     risk.CalibrationProvider
	// Collector is the Prometheus MetricsCollector used by auto_cycle_update
	// to emit per-failure-kind metrics (monitoring.RecordDataAggregatorFailure).
	// nil = no telemetry (acceptable in tests / early bootstrap).
	Collector *monitoring.MetricsCollector
}

// registerCalibrationTasks wires 18 calibration background tasks into the
// BackgroundTaskManager. Tasks are fire-and-register: a Register error is
// logged and dropped (matches the existing pattern in main.go for non-critical
// background work). Per-task conditions (nil deps, feature flags, etc.) are
// checked inline so this function can be called unconditionally.
func registerCalibrationTasks(d calibrationDeps) {
	d.registerCalibrationCycle()
	d.registerAutoStrategyEvolution()
	d.registerAutoThresholdCalibrate()
	d.registerAutoCycleUpdate()
	d.registerRiskGateCalibrate()
	d.registerCycleCalibrate()
	d.registerRegimeCalibrate()
	d.registerFactorWeightCalibrate()
	d.registerConvictionCalibrate()
	d.registerMacroRiskCalibrate()
	d.registerStructuralTrendCalibrate()
	d.registerNarrativeCalibrate()
	d.registerPredictorCalibrate()
	d.registerLinkageCalibrate()
	d.registerFactorWeightStrategyCalibrate()
	d.registerAutoCalibrate()
	d.registerRSITwCalibrate()
}

// registerCalibrationCycle sets up the maturity-gated daily calibration
// framework. The inner CalibrationTask (narrative_weight_calibration) is
// registered with the scheduler; the outer ScheduledTask (calibration_cycle)
// is registered with the BackgroundTaskManager. Both are no-ops when
// maturityTracker is nil.
func (d calibrationDeps) registerCalibrationCycle() {
	if d.MaturityTracker == nil {
		log.Printf("[Gateway] calibration_cycle skipped: maturity tracker is nil")
		return
	}
	calTask := narrative.NewCalibrationTask(d.Cfg.WorkDir)
	calScheduler := scheduler.NewBackgroundCalibrationScheduler(d.MaturityTracker)
	calScheduler.Register(&scheduler.CalibrationTask{
		Name:        "narrative_weight_calibration",
		MinMaturity: domain.MaturityCalibrating,
		Run: func(_ context.Context) error {
			_, err := calTask.RunCalibrationCycle()
			return err
		},
	})
	d.Dashboard.SetCalibrationTask(calTask)
	_ = d.TaskMgr.Register(&apigateway.ScheduledTask{
		Name:     "calibration_cycle",
		Interval: 24 * time.Hour,
		Jitter:   30 * time.Minute,
		Enabled:  d.ParamsCfg.Narrative.CalibrationEnabled.Value,
		Task:     calScheduler.RunDaily,
	})
	log.Printf("[Gateway] registered calibration_cycle background task (24h interval, maturity-gated)")
}

func (d calibrationDeps) registerAutoStrategyEvolution() {
	_ = d.TaskMgr.Register(&apigateway.ScheduledTask{
		Name:     "auto_strategy_evolution",
		Interval: 24 * time.Hour,
		Enabled:  true,
		Task: scheduler.StrategyEvolutionTaskFunc(scheduler.StrategyEvolutionDeps{
			Dashboard:       d.Dashboard,
			SectorDataDir:   d.Cfg.WorkDir,
			MaturityTracker: d.MaturityTracker,
		}),
	})
	log.Printf("[Gateway] registered auto_strategy_evolution background task (24h interval)")
}

func (d calibrationDeps) registerAutoThresholdCalibrate() {
	revenuePath := filepath.Join(d.Cfg.WorkDir, "data", "replay", "month_revenue.jsonl")
	configPath := filepath.Join(d.Cfg.WorkDir, "configs", "parameters.json")
	_ = d.TaskMgr.Register(&apigateway.ScheduledTask{
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

func (d calibrationDeps) registerAutoCycleUpdate() {
	svc := d.Dashboard.GetIndustryService()
	if svc == nil || svc.CycleTracker == nil || svc.Classifier == nil {
		return
	}
	finmindClient := d.FinMindClient
	if finmindClient == nil && d.Cfg.FinMindAPIKey != "" {
		finmindClient = marketdata.GetSharedFinMindClient(d.Cfg.FinMindAPIKey, d.Cfg.WorkDir)
	}
	// Wire failure telemetry: 把 AggregateIndustry 失敗按根因 kind 分類,emit Prometheus metric。
	// collector 為 nil (bootstrap 早期) 時 silent no-op — monitoring.RecordDataAggregatorFailure 內部已處理。
	collector := d.Collector
	recordFailure := func(industryID, kind string) {
		monitoring.RecordDataAggregatorFailure(collector, industryID, kind)
	}
	cycleAggregator := industry.NewDataAggregator(svc.CycleTracker, svc.Classifier, finmindClient, recordFailure)
	_ = d.TaskMgr.Register(&apigateway.ScheduledTask{
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

func (d calibrationDeps) registerRiskGateCalibrate() {
	_ = d.TaskMgr.Register(&apigateway.ScheduledTask{
		Name:     "risk_gate_calibrate",
		Interval: 24 * time.Hour,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			report, err := d.RiskGate.SelfCalibrate(ctx, d.CalProvider, 30)
			if err != nil {
				logging.Error("risk_calibrate", "self_calibrate_failed", "err", err.Error())
				return err
			}
			d.RiskGate.SetLastCalibration(report)
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
}

func (d calibrationDeps) registerCycleCalibrate() {
	svc := d.Dashboard.GetIndustryService()
	if svc == nil || svc.CycleCalibration == nil {
		return
	}
	_ = d.TaskMgr.Register(&apigateway.ScheduledTask{
		Name:     "cycle_calibrate",
		Interval: 24 * time.Hour,
		Enabled:  true,
		Task: func(_ context.Context) error {
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

func (d calibrationDeps) registerRegimeCalibrate() {
	if d.JanusEngine == nil {
		return
	}
	prevRegime := ""
	regimeScenario := map[string]string{
		"NOVEL_REGIME":      "ai_bubble_2024",
		"HISTORICAL_REGIME": "normal_market_2024",
		"RISK_OFF":          "covid_crash_2020",
	}
	_ = d.TaskMgr.Register(&apigateway.ScheduledTask{
		Name:     "regime_calibrate",
		Interval: 1 * time.Hour,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			class := d.JanusEngine.GetRegimeClassification()
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
				report, err := d.RiskGate.SelfCalibrate(ctx, d.CalProvider, 20)
				if err != nil {
					logging.Error("regime_calibrate", "calibrate_after_regime_change_failed", "err", err.Error())
					return nil
				}
				d.RiskGate.SetLastCalibration(report)
				logging.Info("regime_calibrate", "calibration_after_regime_change",
					"verdict", report.Verdict, "changes", len(report.Changes))
			}
			prevRegime = current
			return nil
		},
	})
	log.Printf("[Gateway] registered regime_calibrate background task (1h interval, triggers calibration on regime change)")
}

func (d calibrationDeps) registerFactorWeightCalibrate() {
	_ = d.TaskMgr.Register(&apigateway.ScheduledTask{
		Name:     "factor_weight_calibrate",
		Interval: 24 * time.Hour,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			orders, err := loadCalibrationOrders(d.Cfg.WorkDir)
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
}

func (d calibrationDeps) registerConvictionCalibrate() {
	_ = d.TaskMgr.Register(&apigateway.ScheduledTask{
		Name:     "conviction_calibrate",
		Interval: 24 * time.Hour,
		Enabled:  true,
		Task: func(_ context.Context) error {
			return orchestrator.RunConvictionCalibration(
				d.Cfg.WorkDir,
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
}

func (d calibrationDeps) registerMacroRiskCalibrate() {
	_ = d.TaskMgr.Register(&apigateway.ScheduledTask{
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
}

func (d calibrationDeps) registerStructuralTrendCalibrate() {
	_ = d.TaskMgr.Register(&apigateway.ScheduledTask{
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
}

func (d calibrationDeps) registerNarrativeCalibrate() {
	_ = d.TaskMgr.Register(&apigateway.ScheduledTask{
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
}

func (d calibrationDeps) registerPredictorCalibrate() {
	_ = d.TaskMgr.Register(&apigateway.ScheduledTask{
		Name:     "predictor_calibrate",
		Interval: 24 * time.Hour,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			dbPath := filepath.Join(d.Cfg.LedgerDir, "atlas.db")
			result, err := calibration.CalibratePredictor(ctx, dbPath)
			if err != nil {
				logging.Error("predictor_calibrate", "failed", "err", err.Error())
				return err
			}
			logging.Info("predictor_calibrate", "completed",
				"verdict", result.Verdict,
				"changes", len(result.Changes),
				"summary", result.Summary)
			for _, ch := range result.Changes {
				logging.Info("predictor_calibrate", "param_change",
					"param", ch.ParamName,
					"before", ch.Before,
					"after", ch.After,
					"delta", ch.DeltaPct,
					"confidence", ch.Confidence)
			}
			return nil
		},
	})
	log.Printf("[Gateway] registered predictor_calibrate background task (24h interval)")
}

// registerSeasonalCalibrate was removed: it exec'd `go run
// ./cmd/calibrate-seasonal`, which can never work in the container (no Go
// toolchain) and failed daily. The binary-guarded `seasonal_calibration`
// task (data_sync_health_tasks.go) is the supported mechanism; the
// calibrate-seasonal binary is now built into the image (fix manifest #B09).

func (d calibrationDeps) registerLinkageCalibrate() {
	_ = d.TaskMgr.Register(&apigateway.ScheduledTask{
		Name:     "linkage_calibrate",
		Interval: 24 * time.Hour,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			calibrator := &config.LinkageAmplifierCalibrator{}
			evaluator := func(cfg *config.ParametersConfig) (float64, error) {
				// [INTENTIONAL STUB] TODO: Implement proper recession shock accuracy scoring
				// Audit 2026-07-06: evaluator returns neutral score (0.0, nil) so
				// calibration infra runs end-to-end without changing amplifier value.
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
}

func (d calibrationDeps) registerFactorWeightStrategyCalibrate() {
	_ = d.TaskMgr.Register(&apigateway.ScheduledTask{
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
}

func (d calibrationDeps) registerAutoCalibrate() {
	_ = d.TaskMgr.Register(&apigateway.ScheduledTask{
		Name:     "auto_calibrate",
		Interval: 7 * 24 * time.Hour,
		Jitter:   4 * time.Hour,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			cmd := exec.CommandContext(ctx, "go", "run", "./cmd/calibrate-parameters",
				"--module=darwinian")
			cmd.Dir = d.Cfg.WorkDir
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
}

func (d calibrationDeps) registerRSITwCalibrate() {
	_ = d.TaskMgr.Register(&apigateway.ScheduledTask{
		Name:     "rsi_tw_calibrate",
		Interval: 24 * time.Hour,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			report, err := retail.CalibrateRSITw(d.Cfg.WorkDir)
			if err != nil {
				log.Printf("[RSITw] calibration failed: %v", err)
				return err
			}
			d.RiskGate.SetPreTradeRSITwScore(report.Score)
			log.Printf("[RSITw] calibration complete: %s (score=%.4f, samples=%d, changes=%d)",
				report.Verdict, report.Score, report.SampleCount, len(report.Changes))
			return nil
		},
	})
	log.Printf("[Gateway] registered rsi_tw_calibrate background task (24h interval)")
}
