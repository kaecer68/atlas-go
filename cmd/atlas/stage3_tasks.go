package main

// Stage 3: scheduling + alerting wiring.
// Registers 5 periodic tasks and 3 alert evaluation wrappers into the
// BackgroundTaskManager. All dependencies are resolved from the main() scope.

import (
	"context"
	"log"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/eventdriven"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	monitoringservice "github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/scheduler"
)

// stage3Deps groups the dependencies needed by Stage 3 tasks and alerts.
type stage3Deps struct {
	taskMgr          *apigateway.BackgroundTaskManager
	cfg              config.Config
	gateway          *apigateway.Gateway
	monitor          *monitoring.Monitor
	dashboard        *monitoring.DashboardAPI
	eventCalendar    *industry.EventCalendar
	predictionLedger ledger.EventFlowPredictionStore
	metricsCollector *monitoring.MetricsCollector
	historicalStore  ledger.HistoricalStore
	// capitalFlow is the process-wide shared *capitalflow.Service built in
	// main.go around the file-backed rolling sample store. Issue #1941: the
	// drift rule's LatestCapitalFlowActual MUST read through it. The previous
	// implementation built a throwaway capitalflow.NewService(macroProvider, 0,
	// nil) per call, so the rolling window was empty, every dimension Z was 0
	// and the prediction-vs-actual comparison was meaningless.
	capitalFlow *capitalflow.Service
}

// registerStage3Tasks wires the 5 Stage 3 scheduled tasks into BTM.
// All tasks run at 1-minute interval; the task wrappers contain daily/weekly/
// monthly once-guards so they only execute at the scheduled time.
func registerStage3Tasks(d stage3Deps) {
	if !d.cfg.Stage3TasksEnabled {
		log.Printf("[Stage3] tasks disabled via STAGE3_TASKS_ENABLED=false; skipping 5 task registrations")
		return
	}
	tz, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		log.Printf("[Stage3] failed to load Asia/Taipei tz, falling back to UTC: %v", err)
		tz = time.UTC
	}

	pipelineSvc := monitoringservice.NewPipelineService(d.cfg.WorkDir, d.cfg.LedgerDir, ledger.NewStore(d.cfg.LedgerDir)).
		WithHistoricalStore(d.historicalStore)

	oncestore, oncestoreErr := scheduler.NewFileOncestampStore(d.cfg.LedgerDir)
	if oncestoreErr != nil {
		log.Printf("[Stage3] oncestamp store unavailable, falling back to in-memory: %v", oncestoreErr)
		oncestore = nil
	}

	deps := scheduler.Stage3TaskDeps{
		TimeZone:       tz,
		OncestampStore: oncestore,
		OnTaskComplete: func(taskID string, err error) {
			result := "success"
			if err != nil {
				result = "failed"
			}
			monitoring.RecordStage3TaskRun(d.metricsCollector, taskID, result)
			monitoring.RecordStage3LedgerRecords(d.metricsCollector, d.predictionLedger)
		},
		RefreshEventCalendar: func(now time.Time) error {
			d.eventCalendar.RefreshEvents(now)
			return nil
		},
		RefreshMacroSnapshot: func(ctx context.Context) error {
			_, _, err := d.dashboard.IngestAndUpdateMacro(ctx)
			return err
		},
		RefreshCapitalFlow: func(ctx context.Context) error {
			_, err := d.gateway.Fetch(ctx, "twse_capital_flow")
			return err
		},
		UpdateRegimeHistory: func(ctx context.Context, lookbackDays int) error {
			_, err := pipelineSvc.LoadRegimeHistory(lookbackDays)
			return err
		},
		RecalculateTemplateHitRates: func() error {
			eng := d.dashboard.NarrativeEngine()
			if eng == nil {
				return nil
			}
			eng.RecalculateTemplateHitRates()
			return nil
		},
	}

	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "sync-events-daily",
		Interval: 1 * time.Minute,
		Enabled:  true,
		Task:     scheduler.SyncEventsDailyTaskFunc(deps),
	})
	log.Printf("[Gateway] registered sync-events-daily background task (1m interval, fires 06:00)")

	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "sync-macro-daily",
		Interval: 1 * time.Minute,
		Enabled:  true,
		Task:     scheduler.SyncMacroDailyTaskFunc(deps),
	})
	log.Printf("[Gateway] registered sync-macro-daily background task (1m interval, fires 06:00)")

	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "sync-capital-daily",
		Interval: 1 * time.Minute,
		Enabled:  true,
		Task:     scheduler.SyncCapitalDailyTaskFunc(deps),
	})
	log.Printf("[Gateway] registered sync-capital-daily background task (1m interval, fires 13:30)")

	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "sync-regime-weekly",
		Interval: 1 * time.Minute,
		Enabled:  true,
		Task:     scheduler.SyncRegimeWeeklyTaskFunc(deps),
	})
	log.Printf("[Gateway] registered sync-regime-weekly background task (1m interval, fires Mon 08:00)")

	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "recalibrate-templates-monthly",
		Interval: 1 * time.Minute,
		Enabled:  true,
		Task:     scheduler.RecalibrateTemplatesMonthlyTaskFunc(deps),
	})
	log.Printf("[Gateway] registered recalibrate-templates-monthly background task (1m interval, fires 1st 08:00)")
}

// registerStage3AlertTasks wires the Stage 3 alert evaluator into BTM.
// Three wrappers are registered:
//   - staleness: every 5 minutes (aligned with .omo/plans/Atlas 錢潮方向預測實作規劃.md § Stage 3.2 spec)
//   - daily:     every 1 minute (fires 06:30 via internal guard)
//   - market-close: every 1 minute (fires 13:45 via internal guard)
func registerStage3AlertTasks(d stage3Deps) {
	if !d.cfg.Stage3AlertsEnabled {
		log.Printf("[Stage3] alerts disabled via STAGE3_ALERTS_ENABLED=false; skipping 3 alert registrations")
		return
	}
	tz, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		log.Printf("[Stage3] failed to load Asia/Taipei tz, falling back to UTC: %v", err)
		tz = time.UTC
	}

	alertDeps := buildStage3AlertDeps(d, tz)

	evaluator := monitoring.NewStage3AlertEvaluator(d.monitor, alertDeps)

	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "stage3-alert-staleness",
		Interval: 5 * time.Minute,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			evaluator.EvaluateStaleness()
			return nil
		},
	})
	log.Printf("[Gateway] registered stage3-alert-staleness background task (5m interval; aligned with .omo/plans/Atlas 錢潮方向預測實作規劃.md § Stage 3.2 spec)")

	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "stage3-alert-daily",
		Interval: 1 * time.Minute,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			now := time.Now().In(tz)
			if now.Hour() == 6 && now.Minute() == 30 {
				evaluator.EvaluateDaily()
			}
			return nil
		},
	})
	log.Printf("[Gateway] registered stage3-alert-daily background task (1m interval, fires 06:30)")

	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "stage3-alert-market-close",
		Interval: 1 * time.Minute,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			now := time.Now().In(tz)
			if now.Hour() == 13 && now.Minute() == 45 {
				evaluator.EvaluateMarketClose()
			}
			return nil
		},
	})
	log.Printf("[Gateway] registered stage3-alert-market-close background task (1m interval, fires 13:45)")
}

// wireStage3 registers the Stage 3 scheduled tasks and alert evaluators for the
// production process. It is the single call site main.go uses.
//
// Issue #1941: registerStage3Tasks / registerStage3AlertTasks were fully
// implemented but only ever called from tests, so the config flags
// (STAGE3_TASKS_ENABLED / STAGE3_ALERTS_ENABLED, both default true) gated
// nothing and the Stage-3 sync + alert tasks never ran in production.
//
// NewStage3AlertEvaluator panics on a nil monitor, so the alert wiring is
// skipped with a loud log line instead of crashing startup when no monitor is
// available.
func wireStage3(d stage3Deps) {
	registerStage3Tasks(d)
	if d.monitor == nil {
		log.Printf("[Stage3] alerts skipped: monitor unavailable (STAGE3_ALERTS_ENABLED=%v)", d.cfg.Stage3AlertsEnabled)
		return
	}
	registerStage3AlertTasks(d)
}

// buildStage3AlertDeps assembles the Stage 3 alert evaluator dependencies from
// the production deps. Extracted from registerStage3AlertTasks so tests can
// exercise the individual closures (in particular the capital-flow
// prediction/actual pair) without standing up the BackgroundTaskManager.
func buildStage3AlertDeps(d stage3Deps, tz *time.Location) monitoring.Stage3AlertDeps {
	driftRecorder := newStage3DriftRecorder(d.cfg.LedgerDir)

	return monitoring.Stage3AlertDeps{
		TimeZone: tz,
		OnAlertFired: func(ruleID string, severity monitoring.AlertLevel, metadata map[string]any) {
			monitoring.RecordStage3AlertFired(d.metricsCollector, ruleID, severity)
			monitoring.RecordStage3LedgerRecords(d.metricsCollector, d.predictionLedger)
		},
		ChannelLastDataAt: func() map[string]time.Time {
			out := make(map[string]time.Time)
			if d.gateway == nil {
				return out
			}
			for _, ch := range d.gateway.ChannelIDs() {
				rec := d.gateway.Health().Get(ch)
				if rec == nil || rec.LastDataAt == "" {
					continue
				}
				if t, err := time.Parse(time.RFC3339, rec.LastDataAt); err == nil {
					out[ch] = t
				}
			}
			return out
		},
		IsTradingDay: func(date time.Time) bool {
			if d.eventCalendar == nil {
				wd := date.Weekday()
				return wd != time.Saturday && wd != time.Sunday
			}
			return d.eventCalendar.IsTaiwanTradingDay(date)
		},
		EventCalendarEventCount: func(date time.Time) int {
			return len(d.eventCalendar.GetEventsForDate(date))
		},
		RecentEventFlowPredictions: func(days int) []float64 {
			out := make([]float64, days)
			for i := range out {
				out[i] = 0.5
			}
			if d.predictionLedger == nil {
				return out
			}
			records, err := d.predictionLedger.LoadRecentPredictions(days)
			if err != nil || len(records) == 0 {
				return out
			}
			flows := make([]float64, len(records))
			for i, r := range records {
				flows[i] = r.DirectionSign
			}
			if len(flows) >= days {
				return flows[len(flows)-days:]
			}
			return flows
		},
		RecentEventFlowPredictionsActualCount: func(days int) int {
			if d.predictionLedger == nil {
				return 0
			}
			n := d.predictionLedger.Len()
			if n > days {
				return days
			}
			return n
		},
		LatestCapitalFlowPrediction: func() (monitoring.CapitalFlowSignal, bool) {
			if d.eventCalendar == nil {
				return monitoring.CapitalFlowSignal{}, false
			}
			predictor := eventdriven.NewPredictor(d.eventCalendar)
			report := predictor.Predict(time.Now())
			if len(report.Predictions) == 0 {
				return monitoring.CapitalFlowSignal{}, false
			}
			pred := report.Predictions[0]
			if d.predictionLedger != nil {
				_ = d.predictionLedger.AppendPrediction(ledger.EventFlowPredictionRecord{
					PredictedAt:   time.Now(),
					DirectionSign: ledger.DirectionSign(pred.Direction, pred.Confidence),
					Confidence:    pred.Confidence,
					Direction:     pred.Direction,
				})
			}
			return monitoring.CapitalFlowSignal{
				Direction: monitoring.ClassifyDirection(pred.Confidence, 0.6, 0.4),
				Value:     pred.Confidence,
			}, true
		},
		// Issue #1941: read the realized capital flow through the shared
		// service (same rolling sample store as every other consumer) instead
		// of a throwaway service with an empty window. A nil service is
		// silently unavailable — the drift rule then skips, it never compares
		// against a fabricated 0.
		LatestCapitalFlowActual: func() (monitoring.CapitalFlowSignal, bool) {
			if d.capitalFlow == nil {
				return monitoring.CapitalFlowSignal{}, false
			}
			report, err := d.capitalFlow.LatestDaily(context.Background())
			if err != nil {
				return monitoring.CapitalFlowSignal{}, false
			}
			// A shared store that was never written (or lost) leaves every
			// dimension at SampleCount=0 and every Z at 0, so the
			// prediction-vs-actual comparison would be meaningless again —
			// skip the rule instead of comparing against a fabricated zero
			// (issue #1941).
			if !hasCalibrationEvidence(report.Forces) {
				return monitoring.CapitalFlowSignal{}, false
			}
			return monitoring.CapitalFlowSignal{
				Direction: monitoring.ClassifyDirection(report.QualityScore, 0.5, -0.5),
				Value:     report.QualityScore,
			}, true
		},
		// Issue #1941: durable predicted-vs-actual record produced by the
		// Stage-3 market-close task, written whether the rule alerts or not.
		OnCapitalFlowDriftCompared: func(predicted, actual monitoring.CapitalFlowSignal) {
			if err := driftRecorder.record(time.Now(), predicted, actual); err != nil {
				log.Printf("[Stage3] drift observation record failed: %v", err)
			}
		},
	}
}

// hasCalibrationEvidence reports whether a DailyReport was scored against a
// populated rolling window. Dimensions whose source is missing are excluded
// (DataAvailable=false); at least one available dimension with a non-empty
// reference window is required (issue #1941: an empty shared store must not be
// presented as a real "actual" reading).
func hasCalibrationEvidence(forces []capitalflow.ForceScore) bool {
	for _, f := range forces {
		if f.DataAvailable && f.SampleCount > 0 {
			return true
		}
	}
	return false
}
