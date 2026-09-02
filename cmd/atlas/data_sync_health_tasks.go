package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
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

	// Register per-channel US market refresh tasks instead of a single batch
	// closure so each Yahoo-backed channel has its own ChannelID, failure
	// isolation, and BTM failure telemetry. The shared Yahoo limiters
	// (yahooIndexLimiter / yahooTechLimiter / ExportStatisticsRate) inside
	// Gateway.Fetch serialize requests to the same endpoint group, so launching
	// concurrently does not violate rate limits.
	for _, ch := range apigateway.USMarketChannels() {
		_ = taskMgr.Register(&apigateway.ScheduledTask{
			Name:      "us_market_refresh_" + ch,
			ChannelID: ch,
			Interval:  5 * time.Minute,
			Enabled:   true,
			Jitter:    5 * time.Second,
			Task:      gatewayChannelFetch(gateway, ch),
		})
	}
	log.Printf("[Gateway] registered %d per-channel US market refresh tasks (5m interval)", len(apigateway.USMarketChannels()))

	// P2 C05: Register auto-fetch tasks for channels without periodic refresh.
	// These channels were identified in the 2026-07-25 architecture audit as
	// lacking automated data ingestion.

	// taiex_index — Taiwan weighted index (daily, after TW market close ~14:00 UTC+8).
	_ = taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "auto_taiex_index",
		ChannelID: "taiex_index",
		Interval:  1 * time.Hour,
		Enabled:   true,
		Task:      gatewayChannelFetch(gateway, "taiex_index"),
	})
	log.Printf("[Gateway] registered auto_taiex_index background task (1h interval)")

	// exchange_rate — USD/TWD and other FX rates (1h, low volatility).
	_ = taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "auto_exchange_rate",
		ChannelID: "exchange_rate",
		Interval:  1 * time.Hour,
		Enabled:   true,
		Task:      gatewayChannelFetch(gateway, "exchange_rate"),
	})
	log.Printf("[Gateway] registered auto_exchange_rate background task (1h interval)")

	// geopolitical_taiwan — Taiwan news RSS (6h, same as geopolitical).
	_ = taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "auto_geopolitical_taiwan",
		ChannelID: "geopolitical_taiwan",
		Interval:  6 * time.Hour,
		Enabled:   true,
		Task:      gatewayChannelFetch(gateway, "geopolitical_taiwan"),
	})
	log.Printf("[Gateway] registered auto_geopolitical_taiwan background task (6h interval)")

	// taifex_daily — Taiwan futures market data (1h, after market hours).
	_ = taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "auto_taifex_daily",
		ChannelID: "taifex_daily",
		Interval:  1 * time.Hour,
		Enabled:   true,
		Task:      gatewayChannelFetch(gateway, "taifex_daily"),
	})
	log.Printf("[Gateway] registered auto_taifex_daily background task (1h interval)")

	// twse_insider — TWSE OpenAPI 內部人持股轉讓 (daily after market close ~18:00).
	_ = taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "auto_twse_insider",
		ChannelID: "twse_insider",
		Interval:  1 * time.Hour,
		Enabled:   true,
		Task:      gatewayChannelFetch(gateway, "twse_insider"),
	})
	log.Printf("[Gateway] registered auto_twse_insider background task (1h interval)")

	// Register seasonal_calibration background task. Guard: skip silently if
	// the calibrate-seasonal binary is not co-located with the current binary
	// (production deploys without it stay clean; no live-trading impact).
	exePath, exeErr := os.Executable()
	if exeErr == nil {
		seasonalBin := filepath.Join(filepath.Dir(exePath), "calibrate-seasonal")
		if _, statErr := os.Stat(seasonalBin); statErr == nil {
			replayPath := resolveSeasonalReplayPath(cfg.WorkDir)
			if replayPath == "" {
				// calibrate-seasonal hard-refuses `-update` without real replay
				// data (synthetic-fallback guard), so registering the task
				// without a replay dataset guarantees a task_failed every 7d
				// tick (pure noise — #1757). Skip registration instead and
				// surface a clear WARN, mirroring the binary-missing guard.
				log.Printf("[Gateway] seasonal_calibration skipped: replay dataset not found under %s (sync data/replay/finmind_2020_2024.jsonl to enable)", filepath.Join(cfg.WorkDir, "data", "replay"))
			} else {
				_ = taskMgr.Register(&apigateway.ScheduledTask{
					Name:     "seasonal_calibration",
					Interval: scheduler.SeasonalCalibrationDefaults.Interval,
					Jitter:   30 * time.Minute,
					Enabled:  true,
					Task:     scheduler.SeasonalCalibrationTaskFuncWithReplay(seasonalBin, replayPath),
				})
				log.Printf("[Gateway] registered seasonal_calibration background task (7d interval)")
			}
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
				if err != nil && errors.Is(err, marketdata.ErrFugleBreakerOpen) {
					// Breaker open 是熔斷狀態不是任務失敗——頻道頁已呈現
					// 熔斷狀態，每小時 task_failed 警報只是噪音
					// （observed 2026-09-03: fugle breaker open 連環 task_failed）。
					log.Printf("[Gateway] channel_health_fugle skipped: fugle breaker open")
					return nil
				}
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
	// SA11: Dark launch observation — daily count of simulation sessions
	// since the F06 real-strategy-rankings deployment (2026-07-12).
	// Logs progress; when ≥20 sessions are accumulated, prints a
	// prominent milestone log so the operator knows it's time to
	// evaluate real-world prediction hit rates.
	sessionsDir := filepath.Join(cfg.WorkDir, "data", "state", "sessions")
	sa11Cutoff := "session-20260712"
	_ = taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "sa11_dark_launch_check",
		Interval: 24 * time.Hour,
		Enabled:  true,
		Task: func(_ context.Context) error {
			entries, err := os.ReadDir(sessionsDir)
			if err != nil {
				return fmt.Errorf("sa11: read sessions dir: %w", err)
			}
			count := 0
			for _, e := range entries {
				if e.IsDir() && e.Name() >= sa11Cutoff {
					count++
				}
			}
			if count >= 20 {
				log.Printf("[SA11] DARK LAUNCH MILESTONE: %d/20 sessions. Evaluate hit rates and decide on Predicted Trade Cycle.", count)
			} else {
				log.Printf("[SA11] dark launch progress: %d/20 sessions (need %d more)", count, 20-count)
			}
			return nil
		},
	})
	log.Printf("[Gateway] registered sa11_dark_launch_check background task (24h interval)")
}

// gatewayChannelFetch returns a BackgroundTaskFunc that calls gateway.Fetch
// for the given channel. Used by P2 C05 auto-fetch tasks to avoid repeating
// the same closure pattern for each channel.
func gatewayChannelFetch(g *apigateway.Gateway, channelID string) apigateway.BackgroundTaskFunc {
	return func(ctx context.Context) error {
		_, err := g.Fetch(ctx, channelID)
		return err
	}
}

// resolveSeasonalReplayPath returns the absolute replay dataset path used by
// the seasonal_calibration background task when it exists under workDir, or
// "" when missing. calibrate-seasonal hard-refuses `-update` without real
// replay data, so an empty result means the task must not be registered
// (registering it would produce a guaranteed task_failed every 7d tick —
// see #1757).
func resolveSeasonalReplayPath(workDir string) string {
	p := filepath.Join(workDir, "data", "replay", "finmind_2020_2024.jsonl")
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}
