package main

// PR10a: Data sync + Health background task registration.
// Extracted from main.go run() to reduce file size and improve testability.
// Tasks here are periodic data fetches / health probes that run independently
// of the realtime / live trading paths.
//
// Tasks (9 total):
//   1. channel_health_sync    — sync in-memory channel health to Postgres (5m)
//   2. us_market_refresh      — batch-refresh 7 US market channels (5m)
//   3. seasonal_calibration   — optional seasonal binary (7d, binary-presence-gated)
//   4. health_check           — HealthChecker.RunOnce (30s)
//   5. channel_health_fugle   — third-party health probe (1h)
//   6. channel_health_fubon   — third-party health probe (1h)
//   7. channel_health_finmind — third-party health probe (1h)
//   8. channel_health_twse_replay — always-on local CSV probe (1h)
//   9. tsmc_revenue           — Gateway.Fetch tsmc_revenue (24h)
//
// Note: calibration_cycle (narrative weight calibration, 24h, maturity-gated)
// belongs to PR10b and stays in main.go between seasonal_calibration and
// health_check to preserve original ordering.

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/scheduler"
)

// registerDataSyncAndHealthTasks wires the data-sync + channel-health probes
// into the BackgroundTaskManager. All tasks here are fire-and-register: a
// Register error is logged and the task is silently dropped (matches the
// existing pattern in main.go for non-critical background work).
//
// Required deps:
//   - taskMgr: task scheduler (must be non-nil; caller already created it)
//   - cfg:     for API-key-gated tasks (Fugle/Fubon/FinMind) and WorkDir
//   - gateway: for Fetch-based health probes and US market refresh
//   - monitor: required for health_check; nil → task skipped
//   - pool:    required for channel_health_sync; nil → task skipped
func registerDataSyncAndHealthTasks(
	taskMgr *apigateway.BackgroundTaskManager,
	cfg config.Config,
	gateway *apigateway.Gateway,
	monitor *monitoring.Monitor,
	pool *pgxpool.Pool,
	collector *monitoring.MetricsCollector,
) {
	// Register channel_health_sync task (DB sync, not a data fetcher).
	if pool != nil {
		_ = taskMgr.Register(&apigateway.ScheduledTask{
			Name:     "channel_health_sync",
			Interval: 5 * time.Minute,
			Enabled:  true,
			Task: func(ctx context.Context) error {
				healthStore := monitoring.NewChannelHealthStoreWithPool(filepath.Join(cfg.WorkDir, "data/state"), pool)
				return healthStore.SyncAllToDB()
			},
		})
		log.Printf("[Gateway] registered channel_health_sync background task (5m interval)")
	}

	// Register us_market_refresh — batch-refresh 7 US market channels
	// (spx, ndx, dji, nvda, aapl, msft, tsm_adr) every 5 minutes.
	// These channels share yahooSharedLimiter; Gateway.Fetch handles
	// both rate limiting and circuit breaking per channel. Per-channel
	// errors are logged but do not fail the batch (transient errors on
	// one channel should not block the others).
	_ = taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "us_market_refresh",
		Interval: 5 * time.Minute,
		Enabled:  true,
		Task:     apigateway.NewUSMarketRefreshTask(gateway),
	})
	log.Printf("[Gateway] registered us_market_refresh background task (5m interval)")

	// Register seasonal_calibration background task. Guard: skip silently if
	// the calibrate-seasonal binary is not co-located with the current binary
	// (production deploys without it stay clean; no live-trading impact).
	exePath, exeErr := os.Executable()
	if exeErr == nil {
		seasonalBin := filepath.Join(filepath.Dir(exePath), "calibrate-seasonal")
		if _, statErr := os.Stat(seasonalBin); statErr == nil {
			replayPath := filepath.Join(cfg.WorkDir, "data", "replay", "finmind_2020_2024.jsonl")
			if _, rpErr := os.Stat(replayPath); rpErr != nil {
				replayPath = ""
			}
			_ = taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "seasonal_calibration",
				Interval: scheduler.SeasonalCalibrationDefaults.Interval,
				Jitter:   30 * time.Minute,
				Enabled:  true,
				Task:     scheduler.SeasonalCalibrationTaskFuncWithReplay(seasonalBin, replayPath),
			})
			log.Printf("[Gateway] registered seasonal_calibration background task (7d interval)")
		} else {
			log.Printf("[Gateway] seasonal_calibration skipped: binary not found at %s", seasonalBin)
		}
	} else {
		log.Printf("[Gateway] seasonal_calibration skipped: os.Executable failed: %v", exeErr)
	}

	// Register health_check via HealthChecker.RunOnce (stateStore is nil in API mode).
	if monitor != nil {
		healthChecker := monitoring.NewHealthChecker(monitor, nil)
		if gateway != nil {
			healthChecker.SetGateway(gateway)
		}
		if collector != nil {
			healthChecker.SetCollector(collector)
		}
		_ = taskMgr.Register(&apigateway.ScheduledTask{
			Name:     "health_check",
			Interval: 30 * time.Second,
			Enabled:  true,
			Task: func(ctx context.Context) error {
				return healthChecker.RunOnce(ctx)
			},
		})
		log.Printf("[Gateway] registered health_check background task (30s interval)")
	}

	// Register channel health checks for third-party data providers.
	// These tasks populate the Gateway health store so the frontend
	// "信息通道" page can show actual status instead of "未知".
	if cfg.FugleAPIKey != "" {
		_ = taskMgr.Register(&apigateway.ScheduledTask{
			Name:      "channel_health_fugle",
			ChannelID: "fugle",
			Interval:  1 * time.Hour,
			Enabled:   true,
			Task: func(ctx context.Context) error {
				_, err := gateway.Fetch(ctx, "fugle")
				return err
			},
		})
		log.Printf("[Gateway] registered channel_health_fugle background task (1h interval)")
	}

	if cfg.FubonAPIKey != "" {
		_ = taskMgr.Register(&apigateway.ScheduledTask{
			Name:      "channel_health_fubon",
			ChannelID: "fubon",
			Interval:  1 * time.Hour,
			Enabled:   true,
			Task: func(ctx context.Context) error {
				_, err := gateway.Fetch(ctx, "fubon")
				return err
			},
		})
		log.Printf("[Gateway] registered channel_health_fubon background task (1h interval)")
	}

	if cfg.FinMindAPIKey != "" {
		_ = taskMgr.Register(&apigateway.ScheduledTask{
			Name:      "channel_health_finmind",
			ChannelID: "finmind",
			Interval:  1 * time.Hour,
			Enabled:   true,
			Task: func(ctx context.Context) error {
				_, err := gateway.Fetch(ctx, "finmind")
				return err
			},
		})
		log.Printf("[Gateway] registered channel_health_finmind background task (1h interval)")
	}

	// Register TWSE replay health check (always available, reads from local CSV).
	{
		_ = taskMgr.Register(&apigateway.ScheduledTask{
			Name:      "channel_health_twse_replay",
			ChannelID: "twse_replay",
			Interval:  1 * time.Hour,
			Enabled:   true,
			Task: func(ctx context.Context) error {
				_, err := gateway.Fetch(ctx, "twse_replay")
				return err
			},
		})
		log.Printf("[Gateway] registered channel_health_twse_replay background task (1h interval)")
	}

	// Register TSMC Revenue task via Gateway.
	if cfg.FinMindAPIKey != "" {
		_ = taskMgr.Register(&apigateway.ScheduledTask{
			Name:      "tsmc_revenue",
			ChannelID: "tsmc_revenue",
			Interval:  24 * time.Hour,
			Enabled:   true,
			Task: func(ctx context.Context) error {
				_, err := gateway.Fetch(ctx, "tsmc_revenue")
				return err
			},
		})
		log.Printf("[Gateway] registered tsmc_revenue background task (24h interval)")
	}

	// H06: Register e2e_chain_probe — daily data-freshness probe.
	dataCheck := func(ctx context.Context) error {
		rec := gateway.Health().Get("twse_capital_flow")
		if rec == nil {
			return fmt.Errorf("channel twse_capital_flow not found")
		}
		ts := rec.LastSuccessAt
		if ts == "" {
			ts = rec.LastFetchAt
		}
		if ts == "" {
			return fmt.Errorf("channel twse_capital_flow never fetched")
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return fmt.Errorf("channel twse_capital_flow bad timestamp: %w", err)
		}
		if time.Since(t) > 2*time.Hour {
			return fmt.Errorf("channel twse_capital_flow stale: %s ago",
				time.Since(t).Round(time.Minute))
		}
		return nil
	}
	probeDeps := monitoring.E2EProbeDeps{DataLayerCheck: dataCheck}
	_ = taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "e2e_chain_probe",
		Interval: 6 * time.Hour,
		Enabled:  true,
		Task:     monitoring.E2EProbeTaskFunc(probeDeps),
	})
	log.Printf("[Gateway] registered e2e_chain_probe background task (6h interval)")
}
