package main

// PR10c-1: Capital + judge background task registration.
// Extracted from main.go run() to reduce file size and improve testability.
// Tasks here are periodic capital-flow / margin / export / judge-promo
// background work that runs independently of the realtime / live trading
// paths.
//
// Tasks (8 total):
//   1. auto_rollback          — autoRollback.RunDaily (24h)
//   2. auto_judge_promoter    — experiment promotion (24h)
//   3. auto_capital_flow      — gateway.Fetch twse_capital_flow (30m, market hours)
//   4. auto_margin            — gateway.Fetch twse_margin (30m, market hours)
//   5. margin_history_backfill — narrative.NewMarginHistoryBackfiller (24h)
//   6. auto_export            — gateway.Fetch export_statistics (12h)
//   7. auto_government_flow   — gateway.Fetch government_flow (1h + daily-once, weekday 15:00+ Taipei; fix/20260801-govflow-cadence)
//   8. auto_geopolitical      — gateway.Fetch geopolitical (6h)

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/experiment"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/scheduler"
)

// capitalDeps groups the dependencies needed by all 8 capital tasks.
// Passed as a struct so the function signature stays compact as new
// tasks are added.
type capitalDeps struct {
	taskMgr           *apigateway.BackgroundTaskManager
	cfg               config.Config
	gateway           *apigateway.Gateway
	autoRollback      *scheduler.AutoRollback
	autoJudgePromoter *experiment.AutoJudgePromoter
	// monitor feeds the evolution_health alert channel (B4). May be nil;
	// then the health task still runs but raises no alerts.
	monitor *monitoring.Monitor

	// predictionLedger stores event-flow predictions so the prev-day
	// reconciler can fill T+1 actual onto prior predictions. May be nil
	// when the ledger is not wired (then the reconcile task is skipped).
	predictionLedger ledger.EventFlowPredictionStore
	// capitalFlowStore backs the prev-day actual lookup (ForeignInvestorNet
	// from T86). May be nil when the capital-flow pipeline is not wired.
	capitalFlowStore capitalflow.RollingSampleStore
}

// registerCapitalTasks wires the capital-flow / margin / export / judge
// tasks into the BackgroundTaskManager. All tasks here are
// fire-and-register: a Register error is logged and the task is silently
// dropped (matches the existing pattern in main.go for non-critical
// background work).
func registerCapitalTasks(d capitalDeps) {
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "auto_rollback",
		Interval: 24 * time.Hour,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			_, err := d.autoRollback.RunDaily(ctx)
			return err
		},
	})
	log.Printf("[Gateway] registered auto_rollback background task (24h interval)")

	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "auto_judge_promoter",
		Interval: 24 * time.Hour,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			pending := experiment.LoadPendingExperiments(d.cfg.WorkDir)
			if len(pending) == 0 {
				return nil
			}
			_, err := d.autoJudgePromoter.RunDaily(ctx, pending)
			return err
		},
	})
	log.Printf("[Gateway] registered auto_judge_promoter background task (24h interval)")

	// Register auto_capital_flow via Gateway.
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "auto_capital_flow",
		ChannelID: "twse_capital_flow",
		Interval:  30 * time.Minute,
		Enabled:   true,
		Task: func(ctx context.Context) error {
			now := time.Now()
			if tz, err := time.LoadLocation("Asia/Taipei"); err == nil {
				now = now.In(tz)
			}
			if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
				return nil
			}
			hour := now.Hour()
			if hour < 9 || hour >= 16 {
				return nil
			}
			_, err := d.gateway.Fetch(ctx, "twse_capital_flow")
			return err
		},
	})
	log.Printf("[Gateway] registered auto_capital_flow background task (30m interval)")

	// Register auto_margin via Gateway.
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "auto_margin",
		ChannelID: "twse_margin",
		Interval:  30 * time.Minute,
		Enabled:   true,
		Task: func(ctx context.Context) error {
			now := time.Now()
			if tz, err := time.LoadLocation("Asia/Taipei"); err == nil {
				now = now.In(tz)
			}
			if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
				return nil
			}
			hour := now.Hour()
			if hour < 9 || hour >= 16 {
				return nil
			}
			_, err := d.gateway.Fetch(ctx, "twse_margin")
			return err
		},
	})
	log.Printf("[Gateway] registered auto_margin background task (30m interval)")

	// Register margin_history_backfill via Gateway.
	if err := d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "margin_history_backfill",
		ChannelID: "twse_margin",
		Interval:  24 * time.Hour,
		Enabled:   true,
		Task: func(ctx context.Context) error {
			backfiller := narrative.NewMarginHistoryBackfiller(d.cfg.WorkDir)
			return backfiller.Backfill(ctx)
		},
	}); err != nil {
		log.Printf("[Gateway] failed to register margin_history_backfill: %v", err)
	} else {
		log.Printf("[Gateway] registered margin_history_backfill background task (24h interval)")
	}

	// Register auto_export via Gateway.
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "auto_export",
		ChannelID: "export_statistics",
		Interval:  12 * time.Hour,
		Enabled:   true,
		Task: func(ctx context.Context) error {
			_, err := d.gateway.Fetch(ctx, "export_statistics")
			return err
		},
	})
	log.Printf("[Gateway] registered auto_export background task (12h interval)")

	// Register auto_taifex_institutional — daily pull of 三大法人 期貨 OI
	// after TAIFEX publishes (15:30 Taipei). 1h interval is a safety net;
	// Gateway.Fetch's cache keeps it idempotent within the same date.
	// 90-day backfill is NOT provided by TAIFEX OpenAPI; backlog BK-12.
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "auto_taifex_institutional",
		ChannelID: "taifex_institutional",
		Interval:  1 * time.Hour,
		Enabled:   true,
		Task: func(ctx context.Context) error {
			now := time.Now()
			if tz, err := time.LoadLocation("Asia/Taipei"); err == nil {
				now = now.In(tz)
			}
			if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
				return nil
			}
			if now.Hour() < 15 {
				return nil
			}
			_, err := d.gateway.Fetch(ctx, "taifex_institutional")
			return err
		},
	})
	log.Printf("[Gateway] registered auto_taifex_institutional background task (1h interval, 15:00+ Taipei)")

	// Register auto_twse_sbl — daily fetch of SBL (借券賣出餘額) balances
	// via FinMind TaiwanDailyShortSaleBalances (G02 live).
	sblLastFetchDay := ""
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "auto_twse_sbl",
		ChannelID: "twse_sbl",
		Interval:  1 * time.Hour,
		Enabled:   true, // G02 live (FinMind TaiwanDailyShortSaleBalances)
		Task: func(ctx context.Context) error {
			// Weekdays after market close (15:00+), once per day.
			now := time.Now()
			if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
				return nil
			}
			if now.Hour() < 15 {
				return nil
			}
			today := now.Format("2006-01-02")
			if sblLastFetchDay == today {
				return nil // hourly tick; already fetched today
			}
			_, err := d.gateway.Fetch(ctx, "twse_sbl")
			if err == nil {
				sblLastFetchDay = today
				return nil
			}
			// 告警降噪（2026-09-03）：FinMind 日額度耗盡是等待態（00:00 TW
			// 自動重置），不是通道故障——health 已在 gateway 分類為 warn（非
			// error）。把今日標記為已嘗試，避免 16:00-23:00 每小時重試白燒額度
			// 並持續製造 task_failed 噪音；隔日 15:00+ 額度重置後自然恢復。
			if errors.Is(err, marketdata.ErrQuotaExhausted) {
				sblLastFetchDay = today
				logging.Warn("capital_tasks", "auto_twse_sbl_quota_skip", "err", err.Error())
				return nil
			}
			return err
		},
	})
	log.Printf("[Gateway] registered auto_twse_sbl background task (1h interval, 15:00+ Taipei, once daily, G02 live)")

	// Register auto_sbl_tdcc_history_backfill — one-time 6-month historical
	// backfill for the two newly wired channels (2026-09-01 user directive:
	// anything backfillable for prediction is backfilled instead of waiting
	// for future data). Self-advancing month cursor: each run fetches the
	// next 30-day chunk for both channels (~2 FinMind calls), then no-ops
	// until the cursor passes today. Idempotent (existing files skipped).
	historyStart, _ := time.Parse("2006-01-02", "2026-03-01")
	cursor := historyStart
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "auto_sbl_tdcc_history_backfill",
		ChannelID: "twse_sbl",
		Interval:  1 * time.Hour,
		Enabled:   true,
		Task: func(ctx context.Context) error {
			if cursor.After(time.Now()) {
				return nil // backfill complete
			}
			chunkEnd := cursor.AddDate(0, 1, 0).AddDate(0, 0, -1)
			if chunkEnd.After(time.Now()) {
				chunkEnd = time.Now()
			}
			// SBL: per-day files via the provider's history walk
			// (gateway.Fetch only returns the newest snapshot — the
			// backfill needs every day in the chunk).
			sblProvider, sblErr := d.gateway.Provider("twse_sbl")
			if sblErr != nil {
				return sblErr
			}
			if sbl, ok := sblProvider.(*apigateway.TWSESBLChannelAdapter); ok {
				if _, err := sbl.Provider().FetchSBLHistory(ctx, cursor, chunkEnd); err != nil {
					log.Printf("[Backfill] twse_sbl history chunk %s..%s deferred: %v", cursor.Format("2006-01-02"), chunkEnd.Format("2006-01-02"), err)
					return err
				}
			}
			// TDCC: monthly chunk via the provider's history method. The
			// gateway Fetch path only returns the newest snapshot, so the
			// chunked history walk goes through the registry provider
			// directly.
			tdccProvider, tdccErr := d.gateway.Provider("tdcc_equity_dispersion")
			if tdccErr != nil {
				return tdccErr
			}
			if tdcc, ok := tdccProvider.(*apigateway.TDCClientChannelAdapter); ok {
				if _, err := tdcc.Provider().FetchDispersionHistory(ctx, cursor, chunkEnd); err != nil {
					log.Printf("[Backfill] tdcc history chunk %s..%s deferred: %v", cursor.Format("2006-01-02"), chunkEnd.Format("2006-01-02"), err)
					return err
				}
			}
			log.Printf("[Backfill] history chunk %s..%s done", cursor.Format("2006-01-02"), chunkEnd.Format("2006-01-02"))
			cursor = chunkEnd.AddDate(0, 0, 1)
			return nil
		},
	})
	log.Printf("[Gateway] registered auto_sbl_tdcc_history_backfill (monthly chunks from %s, self-advancing)", historyStart.Format("2006-01-02"))

	// Register auto_tdcc_dispersion — weekly fetch of the 集保戶股權分散表
	// (G01 live). The table is weekly (data dated Friday, published early
	// the following week): fetch on Tue (primary) and Fri (retry).
	tdccLastFetchDay := ""
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "auto_tdcc_dispersion",
		ChannelID: "tdcc_equity_dispersion",
		Interval:  1 * time.Hour,
		Enabled:   true, // G01 live (FinMind TaiwanStockHoldingSharesPer)
		Task: func(ctx context.Context) error {
			now := time.Now()
			if wd := now.Weekday(); wd != time.Tuesday && wd != time.Friday {
				return nil
			}
			if now.Hour() < 10 {
				return nil
			}
			today := now.Format("2006-01-02")
			if tdccLastFetchDay == today {
				return nil
			}
			_, err := d.gateway.Fetch(ctx, "tdcc_equity_dispersion")
			if err == nil {
				tdccLastFetchDay = today
				return nil
			}
			// 告警降噪（2026-09-03）：TDCC 股權分散是週頻快照，「walk-back 無
			// 資料」= 快照尚未發布（等待態）、FinMind 額度耗盡 = 00:00 TW 重置
			// ——皆非通道故障。health 已在 gateway 分類（ErrNoData→ok 不更新
			// last_success；ErrQuotaExhausted→warn），任務回 nil 避免 task_failed
			// 噪音。不設 tdccLastFetchDay：當日後續 tick 仍會嘗試，捕捉同日晚些
			// 發布的快照。
			if errors.Is(err, marketdata.ErrNoData) || errors.Is(err, marketdata.ErrQuotaExhausted) {
				logging.Warn("capital_tasks", "auto_tdcc_dispersion_waiting", "err", err.Error())
				return nil
			}
			return err
		},
	})
	log.Printf("[Gateway] registered auto_tdcc_dispersion background task (1h interval, Tue/Fri 10:00+ Taipei, G01 live)")

	// Register auto_government_flow — daily refresh of operator-imported
	// 官股行庫 readings (manifest #E04). No upstream HTTP — just reads the
	// state directory that the BTM government_flow_aggregate task writes
	// to (fix/20260731-govflow-cadence, fix/20260801-govflow-cadence).
	//
	// Cadence policy (fix/20260801-govflow-cadence): 1h tick with
	// weekday 15:00+ Taipei gate PLUS daily-once guard (in-memory).
	// The 24h tick pre-this-PR was always anchored at process start
	// (see internal/apigateway/background.go:313 time.NewTicker +
	// immediate executeTask), so a 14:21 container start produced
	// a permanent 14:21 next-tick that the 15:00 gate blocked forever.
	// 1h tick is short enough that within an hour of 15:00 Taipei the
	// task will land inside the gate; the daily-once guard keeps it
	// from re-running once a successful fetch has happened today.
	//
	// No captchaCooldown here: this task does not hit upstream (TWSE
	// bsr). The cooldown lives on the BTM government_flow_aggregate
	// task, which is the one that actually fetches from upstream.
	//
	// lastSuccessDate is captured by the closure: each task body
	// invocation (BTM runTask loop, see background.go:320) reads the
	// same variable, so a successful fetch today sets the guard for
	// the rest of the trading day. The two gov tasks each have their
	// own lastSuccessDate — they are NOT shared.
	// daily-once guard: in-memory, captured by the Task closure. A
	// successful fetch writes today's Asia/Taipei date; subsequent
	// ticks that day short-circuit via shouldRunGovFlow.
	var lastSuccessDate string
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "auto_government_flow",
		ChannelID: "government_flow",
		Interval:  1 * time.Hour,
		Enabled:   true,
		Task: func(ctx context.Context) error {
			now := time.Now()
			if !shouldRunGovFlow(now, lastSuccessDate) {
				reason := classifyGateSkip(now, lastSuccessDate)
				logging.Info("main", "auto_government_flow_skipped", "reason", string(reason))
				return nil
			}
			_, err := d.gateway.Fetch(ctx, "government_flow")
			if err == nil {
				lastSuccessDate = taipeiDateString(now)
			}
			return err
		},
	})
	log.Printf("[Gateway] registered auto_government_flow background task (1h interval, weekday 15:00+ Taipei, daily-once guard)")

	// Register auto_geopolitical via Gateway.
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "auto_geopolitical",
		ChannelID: "geopolitical",
		Interval:  6 * time.Hour,
		Enabled:   true,
		Task: func(ctx context.Context) error {
			_, err := d.gateway.Fetch(ctx, "geopolitical")
			return err
		},
	})
	log.Printf("[Gateway] registered auto_geopolitical background task (6h interval)")

	// Register reconcile-prev-day-prediction: fills the previous trading
	// day's event-flow prediction with the realized T+1 actual (product
	// positioning §6 "prediction vs actual same-unit error tracking").
	// Only registered when both the prediction ledger and the capital-flow
	// store are wired — otherwise the task would be a silent no-op.
	if d.predictionLedger != nil && d.capitalFlowStore != nil {
		_ = d.taskMgr.Register(&apigateway.ScheduledTask{
			Name:     "reconcile-prev-day-prediction",
			Interval: 1 * time.Minute,
			Enabled:  true,
			Task: scheduler.ReconcilePrevDayPredictionTaskFunc(scheduler.Stage3TaskDeps{
				TimeZone:       taipeiLocation(),
				OncestampStore: nil, // 14:30 daily guard is in-memory; fine for reconciliation idempotency
				LoadPrevDayPrediction: func() (ledger.EventFlowPredictionRecord, bool) {
					rec, err := d.predictionLedger.LoadByDate(time.Now().In(taipeiLocation()).AddDate(0, 0, -1))
					if err != nil {
						return ledger.EventFlowPredictionRecord{}, false
					}
					return rec, true
				},
				LoadPrevDayActual: func() (float64, bool) {
					// History returns samples with TradingDate strictly < beforeDate.
					// Reconciling yesterday's prediction (T-1) needs the actual for
					// T-1, which is the latest sample strictly before TODAY (not
					// before yesterday — that would attach the trading day before
					// the prediction's day). Example: Tuesday 14:30, prediction was
					// Monday → History(beforeDate=Tuesday) returns Monday's actual.
					beforeDate := time.Now().In(taipeiLocation()).Format("2006-01-02")
					samples, err := d.capitalFlowStore.History(context.Background(), capitalflow.ForceForeign, beforeDate, 1)
					if err != nil || len(samples) == 0 {
						return 0, false
					}
					return samples[len(samples)-1].RawValue, true
				},
				UpdatePrevDayActual: func(predictedAt time.Time, actualSign float64, source string) error {
					return d.predictionLedger.UpdateActual(predictedAt, actualSign, source)
				},
			}),
		})
		log.Printf("[Gateway] registered reconcile-prev-day-prediction background task (1m interval, fires 14:30; fills T+1 actual onto prior-day prediction)")
	} else {
		log.Printf("[Gateway] reconcile-prev-day-prediction skipped: predictionLedger or capitalFlowStore not wired")
	}

	// Register evolution_health — B4 evolution-loop liveness alerting.
	// Watches the four evolution pillars (proposal / judge / promote /
	// revert) for 24h of silence and raises monitor alerts through the
	// existing alert channel (same adapter as auto_experiment). Missing
	// monitor (nil) keeps the check running but drops alerts.
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "evolution_health",
		Interval: 24 * time.Hour,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			health := experiment.CheckEvolutionHealth(experiment.EvolutionHealthConfig{
				LedgerDir:          d.cfg.LedgerDir,
				BaselinePolicyPath: d.cfg.BaselinePolicyPath,
				ReplayDataPath:     config.GetReplayDataPath(d.cfg.WorkDir),
			})
			if d.monitor != nil {
				experiment.RaiseEvolutionHealthAlerts(&experimentMonitorAdapter{m: d.monitor}, health)
			}
			return nil
		},
	})
	log.Printf("[Gateway] registered evolution_health background task (24h interval)")
}
