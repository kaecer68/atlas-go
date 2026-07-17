package main

// PR10c-2: Operations + data-ingest background task registration.
// Extracted from main.go run() to reduce file size and improve testability.
// Tasks here are periodic operational probes, data ingest, and metrics
// snapshot work that runs independently of the realtime / live trading
// paths.
//
// Tasks (9 total):
//   1. system_health_monitor    — healthMonitor.RunDaily (24h)
//   2. auto_backfill            — daily-replay-sync binary (24h)
//   3. fundamentals_staleness_check — monitor.Alert on >90d (24h)
//   4. storage_cleanup          — LifecycleManager.Run (24h)
//   5. auto_calendar_refresh    — TWSE calendar provider (24h)
//   6. macro_ingest             — dashboard.IngestAndUpdateMacro + VIX crisis (5m)
//   7. realtime_feed            — realtimeAdapter.IngestData (30s)
//   8. silicon_cycle_update     — industrySvc.UpdateSiliconIndicators (10m)
//   9. metrics_snapshot         — repo.SaveSnapshot (60s)

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/importer"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/prism"
	"github.com/kaecer68/atlas-go/internal/realtime"
	"github.com/kaecer68/atlas-go/internal/repository"
	"github.com/kaecer68/atlas-go/internal/scheduler"
	"github.com/kaecer68/atlas-go/internal/storage"
)

// operationsDeps groups the dependencies needed by all 9 operations tasks.
// Passed as a struct so the function signature stays compact as new
// tasks are added.
type operationsDeps struct {
	taskMgr         *apigateway.BackgroundTaskManager
	cfg             config.Config
	monitor         *monitoring.Monitor
	gateway         *apigateway.Gateway
	healthMonitor   *scheduler.SystemHealthMonitor
	lifecycleMgr    *storage.LifecycleManager
	dashboard       *monitoring.DashboardAPI
	realtimeAdapter *realtime.RealTimeAdapter
	repo            *repository.DualWriteRepository
	collector       *monitoring.MetricsCollector
	// eventCalendar 服務 eventdriven.RegisterRoutes(/api/events/*)。
	eventCalendar      *industry.EventCalendar
	capitalFlow        *capitalflow.Service
	janusEngine        *janus.Engine
	prismMgr           *prism.PRISMManager
	vixBaselineTracker *marketdata.VIXBaselineTracker
}

// registerOperationsTasks wires the operational probes / data ingest /
// metrics snapshot tasks into the BackgroundTaskManager. All tasks here
// are fire-and-register: a Register error is logged and the task is
// silently dropped (matches the existing pattern in main.go for
// non-critical background work).
func registerOperationsTasks(d operationsDeps) {
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "system_health_monitor",
		Interval: 24 * time.Hour,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			_, err := d.healthMonitor.RunDaily(ctx)
			return err
		},
	})
	log.Printf("[Gateway] registered system_health_monitor background task (24h interval)")

	// Register auto_backfill via Gateway.
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "auto_backfill",
		ChannelID: "twse_replay",
		Interval:  24 * time.Hour,
		Enabled:   true,
		Task: func(ctx context.Context) error {
			absWorkDir, err := filepath.Abs(d.cfg.WorkDir)
			if err != nil {
				absWorkDir = d.cfg.WorkDir
			}
			latestDate, err := getLatestReplayDate(d.cfg.ReplayDataPath)
			if err != nil {
				return fmt.Errorf("backfill replay read: %w", err)
			}
			now := time.Now()
			if tz, err := time.LoadLocation("Asia/Taipei"); err == nil {
				now = now.In(tz)
			}
			end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			if now.Hour() < 15 || (now.Hour() == 15 && now.Minute() < 30) {
				end = end.AddDate(0, 0, -1)
			}
			start := latestDate.AddDate(0, 0, 1)
			for start.Weekday() == time.Saturday || start.Weekday() == time.Sunday {
				start = start.AddDate(0, 0, 1)
			}
			for end.Weekday() == time.Saturday || end.Weekday() == time.Sunday {
				end = end.AddDate(0, 0, -1)
			}
			if start.After(end) {
				return nil
			}
			startStr := start.Format("2006-01-02")
			endStr := end.Format("2006-01-02")
			log.Printf("[Gateway] backfill gap detected: %s to %s", startStr, endStr)
			bgCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			var cmd *exec.Cmd
			binaryPath := filepath.Join(absWorkDir, "daily-replay-sync")
			if _, err := os.Stat(binaryPath); err == nil {
				cmd = exec.CommandContext(bgCtx, binaryPath, "-csv", d.cfg.ReplayDataPath, "-backfill-start", startStr, "-backfill-end", endStr)
				cmd.Dir = absWorkDir
			} else if _, err := exec.LookPath("go"); err == nil {
				cmd = exec.CommandContext(bgCtx, "go", "run", "./cmd/daily-replay-sync", "-csv", d.cfg.ReplayDataPath, "-backfill-start", startStr, "-backfill-end", endStr)
				cmd.Dir = absWorkDir
			} else {
				return fmt.Errorf("backfill binary not found")
			}
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("backfill failed: %w, output: %s", err, string(out))
			}
			log.Printf("[Gateway] backfill success: %s", string(out))

			// Auto-convert CSV to JSONL so the system's replay pipeline
			// (tw_extended_90days.jsonl) stays in sync with the CSV that
			// daily-replay-sync appends to. JSONL is the canonical format
			// consumed by FactorEngine (composition.go:67).
			absCSV := d.cfg.ReplayDataPath
			absJSONL := strings.TrimSuffix(d.cfg.ReplayDataPath, ".csv") + ".jsonl"
			if !filepath.IsAbs(absCSV) {
				absCSV = filepath.Join(absWorkDir, absCSV)
				absJSONL = filepath.Join(absWorkDir, absJSONL)
			}
			if convErr := importer.ImportTWOpenDataCSVToJSONL(absCSV, absJSONL); convErr != nil {
				log.Printf("[Gateway] backfill CSV→JSONL conversion warning (non-fatal): %v", convErr)
			} else {
				log.Printf("[Gateway] backfill CSV→JSONL conversion: %s", absJSONL)
			}
			return nil
		},
	})
	log.Printf("[Gateway] registered auto_backfill background task (24h interval)")

	// Register fundamentals_staleness_check: fundamentals.json is reference
	// data (PE/PB/DividendYield for 1070 stocks) loaded by FactorEngine at
	// startup. It does not change daily—quarterly refresh is appropriate.
	// This task alerts when the file exceeds 90 days without an update.
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "fundamentals_staleness_check",
		Interval: 24 * time.Hour,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			path := filepath.Join(d.cfg.WorkDir, "data", "fundamentals.json")
			info, err := os.Stat(path)
			if err != nil {
				d.monitor.Alert(monitoring.AlertLevelWarning, "data_staleness",
					fmt.Sprintf("fundamentals.json not accessible: %v", err),
					map[string]any{"file": path})
				return nil
			}
			ageDays := int(time.Since(info.ModTime()).Hours() / 24)
			if ageDays > 90 {
				d.monitor.Alert(monitoring.AlertLevelWarning, "data_staleness",
					fmt.Sprintf("fundamentals.json is %d days old — run: go run ./cmd/backfill-financial-statements", ageDays),
					map[string]any{"file": path, "age_days": ageDays})
			}
			return nil
		},
	})
	log.Printf("[Gateway] registered fundamentals_staleness_check background task (24h interval)")

	// Register storage_cleanup via LifecycleManager.
	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "storage_cleanup",
		Interval: 24 * time.Hour,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			report, err := d.lifecycleMgr.Run(ctx, false)
			if err != nil {
				return fmt.Errorf("storage cleanup: %w", err)
			}
			log.Printf("[StorageCleanup] processed %d policies: %d files deleted, %d kept",
				len(report.Policies), report.TotalDeleted, report.TotalKept)
			return nil
		},
	})
	log.Printf("[Gateway] registered storage_cleanup background task (24h interval)")

	var dashboardEC *industry.EventCalendar
	if d.dashboard != nil {
		if svc := d.dashboard.GetIndustryService(); svc != nil {
			dashboardEC = svc.EventCalendar
		}
	}
	if dashboardEC != nil || d.eventCalendar != nil {
		calendarProvider := marketdata.NewTWSECalendarProvider()
		_ = d.taskMgr.Register(&apigateway.ScheduledTask{
			Name:     "auto_calendar_refresh",
			Interval: 24 * time.Hour,
			Enabled:  true,
			Task: func(ctx context.Context) error {
				bgCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
				defer cancel()
				refreshOne := func(ec *industry.EventCalendar, label string) {
					if ec == nil {
						return
					}
					ec.UpdateFromProvider(bgCtx, calendarProvider)
					ec.RefreshEvents(time.Now())
					logging.Info("calendar", "auto_calendar_refresh_instance",
						logging.FStr("instance", label))
				}
				refreshOne(dashboardEC, "dashboard_industry")
				refreshOne(d.eventCalendar, "eventdriven_mcp")
				return nil
			},
		})
		log.Printf("[Gateway] registered auto_calendar_refresh background task (24h interval)")
	}

	{
		dashRef := d.dashboard
		_ = d.taskMgr.Register(&apigateway.ScheduledTask{
			Name:     "macro_ingest",
			Interval: 5 * time.Minute,
			Enabled:  true,
			Task: func(ctx context.Context) error {
				ingestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				defer cancel()
				_, snap, err := dashRef.IngestAndUpdateMacro(ingestCtx)
				if err != nil {
					logging.Warn("main", "macro_ingest_failed", "err", err)
					return err
				}
				// VIXBaseline: 252-day rolling median from history tracker.
				// Stored separately from the macro snapshot and injected here so
				// JANUS gets a meaningful panic threshold (legacy fallback: 20).
				if d.vixBaselineTracker != nil {
					d.vixBaselineTracker.Update(snap.VIX.Value)
					snap.VIXBaseline = d.vixBaselineTracker.Value()
				}
				if d.janusEngine != nil {
					d.janusEngine.UpdateFromMacro(snap)
				}
				// Crisis circuit break: VIX >= 35 triggers force-open on live channels.
				if d.gateway != nil && snap.VIX.Value >= 35.0 {
					liveChannels := []string{"fugle", "fubon", "finmind"}
					for _, ch := range liveChannels {
						if err := d.gateway.ForceOpenChannel(ch); err != nil {
							logging.Warn("main", "crisis_force_open_failed", "channel", ch, "err", err)
						} else {
							logging.Info("main", "crisis_force_open", "channel", ch, "vix", snap.VIX.Value)
						}
					}
				}
				// Propagate VIX signal to optimizer crisis mode.
				dashRef.InvokeCrisisModeSetter(snap.VIX.Value >= 35.0)
				// Feed daily returns into all six rolling correlation engines
				// (SPX-TWSE legacy + NDX/DJI/TSM/NVDA-TWSE + SPX-VIX).
				if svc := dashRef.GetCrossMarketService(); svc != nil {
					svc.UpdateAllCorrelations(snap)
				}

				return nil
			},
		})
		log.Printf("[Gateway] registered macro_ingest background task (5m interval)")
	}

	// RealTimeAdapter feed: periodically ingest market data points from
	// the latest macro snapshot for sub-second regime detection.
	if d.realtimeAdapter != nil {
		dashRef := d.dashboard
		_ = d.taskMgr.Register(&apigateway.ScheduledTask{
			Name:     "realtime_feed",
			Interval: 30 * time.Second,
			Enabled:  true,
			Task: func(ctx context.Context) error {
				snap, ok := dashRef.GetLatestMacroSnapshot()
				if !ok {
					return nil
				}
				now := time.Now()
				points := []realtime.MarketDataPoint{
					{Symbol: "SOX", Price: snap.SOXIndex.Value, Timestamp: now},
					{Symbol: "VIX", Price: snap.VIX.Value, Timestamp: now},
				}
				if snap.SPXIndex.Value > 0 {
					points = append(
						points,
						realtime.MarketDataPoint{Symbol: "SPX", Price: snap.SPXIndex.Value, Timestamp: now},
						realtime.MarketDataPoint{Symbol: "NDX", Price: snap.NDXIndex.Value, Timestamp: now},
					)
				}
				for _, p := range points {
					if p.Price > 0 {
						d.realtimeAdapter.IngestData(p)
					}
				}
				return nil
			},
		})
		log.Printf("[Gateway] registered realtime_feed background task (30s interval)")
	}

	// Silicon cycle indicator update (10m, offset from macro_ingest 5m
	// to ensure fresh TSMC/SOX data). Uses the macro data pipeline already
	// maintained by macro_ingest — no additional external API calls.
	if d.dashboard != nil {
		if industrySvc := d.dashboard.GetIndustryService(); industrySvc != nil && industrySvc.SiliconTracker != nil {
			_ = d.taskMgr.Register(&apigateway.ScheduledTask{
				Name:     "silicon_cycle_update",
				Interval: 10 * time.Minute,
				Enabled:  true,
				Task: func(ctx context.Context) error {
					return industrySvc.UpdateSiliconIndicators(ctx)
				},
			})
			log.Printf("[Gateway] registered silicon_cycle_update background task (10m interval)")
		}
	}

	if d.repo != nil {
		_ = d.taskMgr.Register(&apigateway.ScheduledTask{
			Name:     "metrics_snapshot",
			Interval: 60 * time.Second,
			Enabled:  true,
			Task: func(ctx context.Context) error {
				snap := d.collector.GetMetricsSnapshot()
				repoSnap := repository.MetricsSnapshot{
					ScreeningTotal:     snap.ScreeningTotal,
					ScreeningPassed:    snap.ScreeningPassed,
					ScreeningRate:      snap.ScreeningRate,
					AlertsTriggered:    snap.AlertsTriggered,
					AlertsAcknowledged: snap.AlertsAcknowledged,
					AlertsByType:       snap.AlertsByType,
					Timestamp:          snap.Timestamp,
				}
				return d.repo.SaveSnapshot(ctx, &repoSnap)
			},
		})
		log.Printf("[Gateway] registered metrics_snapshot background task (60s interval)")
	}

	if d.capitalFlow != nil {
		_ = d.taskMgr.Register(&apigateway.ScheduledTask{
			Name:     "capital_flow_refresh",
			Interval: 5 * time.Minute,
			Enabled:  true,
			Task: func(ctx context.Context) error {
				// BK-15: call Refresh(ctx, tradingDate) — the only
				// writer to the shared RollingSampleStore. The
				// scheduler hands us a bare context (no trading
				// date), so derive the current trading date here
				// from Asia/Taipei and roll back across weekends
				// and before the post-close cutoff. Tasks running
				// after 15:30 Taipei attribute the snapshot to
				// today's date; earlier ticks attribute to the
				// previous weekday's settled close.
				tradingDate := currentTaipeiTradingDate(time.Now())
				if err := d.capitalFlow.Refresh(ctx, tradingDate); err != nil {
					return fmt.Errorf("capital_flow_refresh: %w", err)
				}
				return nil
			},
		})
		log.Printf("[Gateway] registered capital_flow_refresh background task (5m interval)")
	}

	if d.janusEngine != nil {
		_ = d.taskMgr.Register(&apigateway.ScheduledTask{
			Name:     "janus_regime_refresh",
			Interval: 6 * time.Hour,
			Enabled:  true,
			Task: func(_ context.Context) error {
				d.janusEngine.Update()
				return nil
			},
		})
		log.Printf("[Gateway] registered janus_regime_refresh background task (6h interval)")
	}

	if d.prismMgr != nil && d.janusEngine != nil {
		// Event-driven: feed completed results to JANUS immediately instead of
		// waiting for the 6h cron. This propagates training improvements to
		// regime detection in near-real-time.
		d.prismMgr.SetOnCompleted(func(result prism.CompletedTrainingResult) {
			if result.Result.Error == "" && !result.Result.Synthetic {
				d.janusEngine.RecordTrainingResult(result.Regime, result.Result)
			}
		})
		_ = d.taskMgr.Register(&apigateway.ScheduledTask{
			Name:     "prism_training",
			Interval: 6 * time.Hour,
			Enabled:  true,
			Task: func(_ context.Context) error {
				if d.prismMgr == nil || d.janusEngine == nil {
					return nil
				}
				results := d.prismMgr.GetCompletedResults()
				for _, r := range results {
					if r.Result.Error != "" || r.Result.Synthetic {
						continue
					}
					d.janusEngine.RecordTrainingResult(r.Regime, r.Result)
				}
				now := time.Now()
				for _, reg := range []prism.RegimeType{prism.RegimeRiskOn, prism.RegimeRiskOff, prism.RegimeHighVolatility, prism.RegimeLowVolatility, prism.RegimeTransition} {
					_ = d.prismMgr.ScheduleTraining(domain.AgentSpec{
						ID:      "system-" + reg.String(),
						Enabled: true,
					}, []prism.TrainingWindow{{
						Start:     now.AddDate(0, 0, -30),
						End:       now,
						Regime:    reg,
						RegimeSet: true,
					}})
				}
				return nil
			},
		})
		log.Printf("[Gateway] registered prism_training background task (6h interval)")
	}
}

// currentTaipeiTradingDate returns the trading-day boundary as of now,
// computed in Asia/Taipei with weekend rollback and a 15:30 cutoff
// (after TWSE close at 13:30 + 2h settlement). Used by the
// capital_flow_refresh background task to derive the tradingDate
// argument to capitalflow.Service.Refresh, since the scheduler hands
// the task a bare context.Context with no trading-date payload.
//
// Behaviour:
//   - On a weekday before 15:30 Taipei, returns the previous weekday's
//     date (the last fully settled trading day).
//   - On a weekday at/after 15:30 Taipei, returns today's date.
//   - On Saturday/Sunday, rolls back to the preceding Friday.
//
// The function never returns zero time and never panics on missing
// tzdata (falls back to UTC, which still produces a valid date).
//
// Order of operations matters: pre-close cutoff is applied first so
// that weekend rollbacks don't double-subtract when the original day
// is already a non-trading day.
func currentTaipeiTradingDate(now time.Time) time.Time {
	taipei, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		taipei = time.UTC
	}
	local := now.In(taipei)
	d := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, taipei)
	// Pre-close cutoff: 15:30 Taipei (TWSE close 13:30 + 2h settlement).
	// Before this, today's data has not settled, so attribute the
	// snapshot to the previous day. Applied first so the weekend
	// rollback below does not double-subtract on Sat/Sun.
	if local.Hour() < 15 || (local.Hour() == 15 && local.Minute() < 30) {
		d = d.AddDate(0, 0, -1)
	}
	// Weekend rollback: Saturday → Friday, Sunday → Friday.
	switch d.Weekday() {
	case time.Saturday:
		d = d.AddDate(0, 0, -1)
	case time.Sunday:
		d = d.AddDate(0, 0, -2)
	}
	return d
}
