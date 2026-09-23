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
// into the BackgroundTaskManager.
//
// Contract (2026-09-23 hardening): every registration goes through
// registerBackgroundTask, which reports a failed Register instead of dropping
// it, and prints the "registered …" line only on success. A dropped
// registration is NOT harmless: a task with a ChannelID whose channel is
// absent from the gateway is rejected, and the channel then has no periodic
// refresh at all (see the 2026-09-21 fubon outage in registerBackgroundTask).
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
		registerBackgroundTask(taskMgr, &apigateway.ScheduledTask{
			Name:     "channel_health_sync",
			Interval: 5 * time.Minute,
			Enabled:  true,
			Task: func(ctx context.Context) error {
				healthStore := monitoring.NewChannelHealthStoreWithPool(filepath.Join(cfg.WorkDir, "data/state"), pool)
				return healthStore.SyncAllToDB()
			},
		}, "registered channel_health_sync background task (5m interval)")
	}

	// Register per-channel US market refresh tasks instead of a single batch
	// closure so each Yahoo-backed channel has its own ChannelID, failure
	// isolation, and BTM failure telemetry. The shared Yahoo limiters
	// (yahooIndexLimiter / yahooTechLimiter / ExportStatisticsRate) inside
	// Gateway.Fetch serialize requests to the same endpoint group, so launching
	// concurrently does not violate rate limits.
	usChannels := apigateway.USMarketChannels()
	usRegistered := 0
	for _, ch := range usChannels {
		if registerBackgroundTask(taskMgr, &apigateway.ScheduledTask{
			Name:      "us_market_refresh_" + ch,
			ChannelID: ch,
			Interval:  5 * time.Minute,
			Enabled:   true,
			Jitter:    5 * time.Second,
			Task:      gatewayChannelFetch(gateway, ch),
		}, "") {
			usRegistered++
		}
	}
	// Honest count: a dropped registration must not be reported as registered
	// (registerBackgroundTask already logged each failure individually).
	log.Printf("[Gateway] registered %d/%d per-channel US market refresh tasks (5m interval)", usRegistered, len(usChannels))

	// P2 C05: Register auto-fetch tasks for channels without periodic refresh.
	// These channels were identified in the 2026-07-25 architecture audit as
	// lacking automated data ingestion.

	// taiex_index — Taiwan weighted index (daily, after TW market close ~14:00 UTC+8).
	registerBackgroundTask(taskMgr, &apigateway.ScheduledTask{
		Name:      "auto_taiex_index",
		ChannelID: "taiex_index",
		Interval:  1 * time.Hour,
		Enabled:   true,
		Task:      gatewayChannelFetch(gateway, "taiex_index"),
	}, "registered auto_taiex_index background task (1h interval)")

	// exchange_rate — USD/TWD and other FX rates (1h, low volatility).
	registerBackgroundTask(taskMgr, &apigateway.ScheduledTask{
		Name:      "auto_exchange_rate",
		ChannelID: "exchange_rate",
		Interval:  1 * time.Hour,
		Enabled:   true,
		Task:      gatewayChannelFetch(gateway, "exchange_rate"),
	}, "registered auto_exchange_rate background task (1h interval)")

	// geopolitical_taiwan — Taiwan news RSS (6h, same as geopolitical).
	registerBackgroundTask(taskMgr, &apigateway.ScheduledTask{
		Name:      "auto_geopolitical_taiwan",
		ChannelID: "geopolitical_taiwan",
		Interval:  6 * time.Hour,
		Enabled:   true,
		Task:      gatewayChannelFetch(gateway, "geopolitical_taiwan"),
	}, "registered auto_geopolitical_taiwan background task (6h interval)")

	// taifex_daily — Taiwan futures market data (1h, after market hours).
	registerBackgroundTask(taskMgr, &apigateway.ScheduledTask{
		Name:      "auto_taifex_daily",
		ChannelID: "taifex_daily",
		Interval:  1 * time.Hour,
		Enabled:   true,
		Task:      gatewayChannelFetch(gateway, "taifex_daily"),
	}, "registered auto_taifex_daily background task (1h interval)")

	// twse_insider — TWSE OpenAPI 內部人持股轉讓 (daily after market close ~18:00).
	registerBackgroundTask(taskMgr, &apigateway.ScheduledTask{
		Name:      "auto_twse_insider",
		ChannelID: "twse_insider",
		Interval:  1 * time.Hour,
		Enabled:   true,
		Task:      gatewayChannelFetch(gateway, "twse_insider"),
	}, "registered auto_twse_insider background task (1h interval)")

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
				registerBackgroundTask(taskMgr, &apigateway.ScheduledTask{
					Name:     "seasonal_calibration",
					Interval: scheduler.SeasonalCalibrationDefaults.Interval,
					Jitter:   30 * time.Minute,
					Enabled:  true,
					Task:     scheduler.SeasonalCalibrationTaskFuncWithReplay(seasonalBin, replayPath),
				}, "registered seasonal_calibration background task (7d interval)")
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
		registerBackgroundTask(taskMgr, &apigateway.ScheduledTask{
			Name:     "health_check",
			Interval: 30 * time.Second,
			Enabled:  true,
			Task: func(ctx context.Context) error {
				return healthChecker.RunOnce(ctx)
			},
		}, "registered health_check background task (30s interval)")
	}

	// Register channel health checks for third-party data providers.
	// These tasks populate the Gateway health store so the frontend
	// "信息通道" page can show actual status instead of "未知".
	if cfg.FugleAPIKey != "" {
		registerBackgroundTask(taskMgr, &apigateway.ScheduledTask{
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
		}, "registered channel_health_fugle background task (1h interval)")
	}

	// Fubon channel health probe + registration self-heal.
	//
	// channel_health_fubon can only be registered once the fubon adapter is in
	// the gateway registry, so a failed startup probe in
	// apigateway.RegisterChannelAdapters makes this registration fail. Two
	// changes keep that from becoming a silent data outage (2026-09-21):
	//  1. registerBackgroundTask reports the failure instead of printing
	//     "registered …" over a dropped error;
	//  2. fubon_adapter_watch retries the proxy probe, and once the adapter is
	//     registered it attaches this same health probe through
	//     apigateway.FubonHealthProbeTask (one definition, so name/ChannelID/
	//     interval cannot drift between the startup and self-heal paths).
	// FubonKeyConfigured (not cfg.FubonAPIKey) keeps the key check in lock-step
	// with the adapter gate in apigateway.RegisterChannelAdapters — a
	// secret-only deployment used to register the adapter but no health task.
	if apigateway.FubonKeyConfigured(cfg) {
		registerBackgroundTask(taskMgr, apigateway.FubonHealthProbeTask(gateway),
			"registered channel_health_fubon background task (1h interval)")
		registerBackgroundTask(taskMgr, apigateway.FubonAdapterWatchTask(taskMgr, gateway),
			"registered fubon_adapter_watch background task (3m interval, self-heals a missed fubon-proxy startup probe)")
	}

	if cfg.FinMindAPIKey != "" {
		registerBackgroundTask(taskMgr, &apigateway.ScheduledTask{
			Name:      "channel_health_finmind",
			ChannelID: "finmind",
			Interval:  1 * time.Hour,
			Enabled:   true,
			Task: func(ctx context.Context) error {
				_, err := gateway.Fetch(ctx, "finmind")
				return err
			},
		}, "registered channel_health_finmind background task (1h interval)")
	}

	// Register TWSE replay health check (always available, reads from local CSV).
	{
		registerBackgroundTask(taskMgr, &apigateway.ScheduledTask{
			Name:      "channel_health_twse_replay",
			ChannelID: "twse_replay",
			Interval:  1 * time.Hour,
			Enabled:   true,
			Task: func(ctx context.Context) error {
				_, err := gateway.Fetch(ctx, "twse_replay")
				return err
			},
		}, "registered channel_health_twse_replay background task (1h interval)")
	}

	// Register TSMC Revenue task via Gateway.
	if cfg.FinMindAPIKey != "" {
		registerBackgroundTask(taskMgr, &apigateway.ScheduledTask{
			Name:      "tsmc_revenue",
			ChannelID: "tsmc_revenue",
			Interval:  24 * time.Hour,
			Enabled:   true,
			Task: func(ctx context.Context) error {
				_, err := gateway.Fetch(ctx, "tsmc_revenue")
				return err
			},
		}, "registered tsmc_revenue background task (24h interval)")
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
	registerBackgroundTask(taskMgr, &apigateway.ScheduledTask{
		Name:     "e2e_chain_probe",
		Interval: 6 * time.Hour,
		Enabled:  true,
		Task:     monitoring.E2EProbeTaskFunc(probeDeps),
	}, "registered e2e_chain_probe background task (6h interval)")
	// SA11: Dark launch observation — daily count of simulation sessions
	// since the F06 real-strategy-rankings deployment (2026-07-12).
	// Logs progress; when ≥20 sessions are accumulated, prints a
	// prominent milestone log so the operator knows it's time to
	// evaluate real-world prediction hit rates.
	sessionsDir := filepath.Join(cfg.WorkDir, "data", "state", "sessions")
	sa11Cutoff := "session-20260712"
	registerBackgroundTask(taskMgr, &apigateway.ScheduledTask{
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
	}, "registered sa11_dark_launch_check background task (24h interval)")
}

// registerBackgroundTask registers a background task and reports the outcome
// honestly: the operator-facing "registered …" line is printed ONLY when
// RegisterAndStart returned nil.
//
// Why this helper exists (2026-09-21 fubon outage): every registration in this
// file used to discard the error with `_ =` while still logging success.
// Register rejects a task whose ChannelID is not present in the gateway
// registry, so when the fubon adapter was skipped by a startup probe race the
// channel_health_fubon task was dropped — and the log still claimed
// "registered channel_health_fubon background task (1h interval)". The gap
// stayed invisible until the 48h freshness window flipped the channel to
// stale. NEVER reintroduce `_ = taskMgr.Register(...)` here: a dropped
// registration means the channel has no periodic refresh at all.
//
// RegisterAndStart is used instead of Register so a task registered while the
// manager is already running (self-heal path: fubon_adapter_watch attaching
// channel_health_fubon after the proxy recovered) is actually scheduled
// instead of sitting in the registry map unrun.
//
// registeredMsg is the historical operator-facing line, kept verbatim so log
// greps and runbooks keep working. Pass "" for tasks whose success is
// summarized by the caller (per-channel loops).
func registerBackgroundTask(taskMgr *apigateway.BackgroundTaskManager, task *apigateway.ScheduledTask, registeredMsg string) bool {
	if err := taskMgr.RegisterAndStart(task); err != nil {
		log.Printf("[Gateway] FAILED to register background task %s (channel %q): %v — task dropped, this channel now has NO periodic refresh",
			task.Name, task.ChannelID, err)
		return false
	}
	if registeredMsg != "" {
		log.Printf("[Gateway] %s", registeredMsg)
	}
	return true
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
