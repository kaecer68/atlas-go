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
	"github.com/kaecer68/atlas-go/internal/marketdata"
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
	macroProvider    marketdata.MacroDataProvider
	predictionLedger ledger.EventFlowPredictionStore
	metricsCollector *monitoring.MetricsCollector
	historicalStore  ledger.HistoricalStore
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

	alertDeps := monitoring.Stage3AlertDeps{
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
		LatestCapitalFlowActual: func() (monitoring.CapitalFlowSignal, bool) {
			if d.macroProvider == nil {
				return monitoring.CapitalFlowSignal{}, false
			}
			svc := capitalflow.NewService(d.macroProvider, 0, nil)
			report, err := svc.LatestDaily(context.Background())
			if err != nil {
				return monitoring.CapitalFlowSignal{}, false
			}
			return monitoring.CapitalFlowSignal{
				Direction: monitoring.ClassifyDirection(report.QualityScore, 0.5, -0.5),
				Value:     report.QualityScore,
			}, true
		},
	}

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
