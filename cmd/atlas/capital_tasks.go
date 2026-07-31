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
	"log"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/experiment"
	"github.com/kaecer68/atlas-go/internal/logging"
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

	// Register auto_twse_sbl — daily fetch of TWSE SBL (借券賣出餘額) data (G02).
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "auto_twse_sbl",
		ChannelID: "twse_sbl",
		Interval:  1 * time.Hour,
		Enabled:   false, // G02: stub adapter, channel not yet active
		Task: func(ctx context.Context) error {
			// Only fetch on weekdays after market close (15:00+).
			if now := time.Now(); now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
				return nil
			} else if now.Hour() < 15 {
				return nil
			}
			_, err := d.gateway.Fetch(ctx, "twse_sbl")
			return err
		},
	})
	log.Printf("[Gateway] registered auto_twse_sbl background task (1h interval, 15:00+ Taipei, G02)")

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
}
