// Package scheduler — stockpicker 每日盤後勝率重算排程（PR 2e）。
//
// RegisterStockpickerUpdateSchedule 以 narrative_update 同模式註冊 BTM
// 任務（btm.Register + ScheduledTask）：
//
//   - Interval 1h + TimeGated（daily_report_generate 模式）：每小時 tick，
//     僅在 Asia/Taipei 當地時間 >= 18:00（台股 13:30 收盤 + 資料同步/回補
//     餘裕）且當天是台股交易日（internal/taiwanholidays）時執行。
//     TimeGated 讓 liveness 不依 interval 判斷 stale（同 daily_report_generate）。
//
//   - 執行體是 stockpicker.RunDailyUpdate（與 cmd/run-stockpicker-backtest
//     共用同一函式，禁止複製貼上邏輯），重算 rolling 120d 個股勝率
//     （outcomes → stock_win_rate + data/state/stock_win_rate.json）。
//
//   - 冪等（IdempotencyDay）：該 run 會產出的「最新 trigger date」
//     （= AsOf 往前數 DefaultForwardDays 個交易日的日期）已有 outcomes →
//     skip。失敗不會留下該日 outcome → 下一小時 tick 自動重試，BTM 記錄
//     consecutive_failures 供監控。row-level 還有 ON CONFLICT DO NOTHING
//     兜底，即使 gate 被繞過也不會重複寫入。
//
//   - enabled 預設 true：對齊本模組既有任務（narrative_weight_update /
//     template_detector_scan / daily_report_generate 皆 Enabled: true），
//     且本任務有「交易日 + 時段 + 交易日冪等 + row-level upsert」多重保護；
//     資料只寫 job-local SQLite artifact，不碰 postgres target（除非
//     ATLAS_STORE_BACKEND=postgres + 顯式 ExpectDB，該路徑有 M12 guard）。
package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
	"github.com/kaecer68/atlas-go/internal/taiwanholidays"
)

// StockpickerUpdateTaskName is the BackgroundTaskManager registration name.
const StockpickerUpdateTaskName = "stockpicker_daily_update"

// stockpickerUpdateRunHour is the earliest Asia/Taipei hour the daily update
// may run: TWSE closes at 13:30, data sync + quote backfill finish well
// before 18:00, so an 18:00+ window always sees the completed trading day.
const stockpickerUpdateRunHour = 18

// StockpickerUpdateDeps groups the dependencies for the daily update task.
// WorkDir is required; everything else has a safe default (see
// StockpickerUpdateTaskFunc).
type StockpickerUpdateDeps struct {
	WorkDir    string                                                                                          // atlas repo root; data/state lives under it
	TimeZone   *time.Location                                                                                  // nil → Asia/Taipei (fallback UTC+8)
	Backend    string                                                                                          // "" → ATLAS_STORE_BACKEND env → job-local sqlite
	ExpectDB   string                                                                                          // postgres migration-target guard (M12); "" default
	Universe   string                                                                                          // comma-separated symbols (default: all quote symbols)
	Conditions string                                                                                          // comma-separated condition IDs (default: parameters.json defaults)
	DryRun     bool                                                                                            // compute coverage without persisting (observability/debug)
	Runner     func(ctx context.Context, opts stockpicker.RunDailyOptions) (stockpicker.RunDailyResult, error) // test seam
	Now        func() time.Time                                                                                // test seam; nil → time.Now
}

// RegisterStockpickerUpdateSchedule registers the daily post-close stockpicker
// win-rate update with the BackgroundTaskManager. Fire-and-register
// convention: Register errors (duplicate name) are ignored, aligning with
// RegisterNarrativeWeightUpdateSchedule / cmd/atlas/capital_tasks.go.
func RegisterStockpickerUpdateSchedule(btm *apigateway.BackgroundTaskManager, deps StockpickerUpdateDeps) {
	if btm == nil {
		return
	}
	_ = btm.Register(&apigateway.ScheduledTask{
		Name:      StockpickerUpdateTaskName,
		Interval:  time.Hour,
		TimeGated: true,
		Enabled:   true,
		Task:      StockpickerUpdateTaskFunc(deps),
	})
}

// StockpickerUpdateTaskFunc returns the BackgroundTaskManager-compatible task
// closure. Gates, in order:
//
//  1. Asia/Taipei hour >= 18 → otherwise ErrTaskSkipped (window not open).
//  2. today is a Taiwan trading day → otherwise ErrTaskSkipped (weekend /
//     holiday: no new market data, nothing to recompute).
//  3. RunDailyUpdate with IdempotencyDay — the runner skips (res.Skipped)
//     when the day's increment is already recorded; a skip returns
//     ErrTaskSkipped so the failure counter is untouched (daily_report_generate
//     pattern).
//
// Any runner error is returned so BackgroundTaskManager records the failure
// and the next hourly tick retries.
func StockpickerUpdateTaskFunc(deps StockpickerUpdateDeps) func(context.Context) error {
	nowFn := deps.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	tz := deps.TimeZone
	if tz == nil {
		tz = taipeiLocation()
	}
	runner := deps.Runner
	if runner == nil {
		runner = stockpicker.RunDailyUpdate
	}
	return func(ctx context.Context) error {
		now := nowFn().In(tz)
		if now.Hour() < stockpickerUpdateRunHour {
			return apigateway.ErrTaskSkipped
		}
		if !taiwanholidays.IsTradingDay(now) {
			logging.Info("stockpicker_daily_update", "skip_non_trading_day",
				"date", now.Format("2006-01-02"))
			return apigateway.ErrTaskSkipped
		}
		if deps.WorkDir == "" {
			return fmt.Errorf("stockpicker update: WorkDir is empty")
		}

		res, err := runner(ctx, stockpicker.RunDailyOptions{
			WorkDir:     deps.WorkDir,
			Backend:     deps.Backend,
			ExpectDB:    deps.ExpectDB,
			Idempotency: stockpicker.IdempotencyDay,
			DryRun:      deps.DryRun,
			Universe:    deps.Universe,
			Conditions:  deps.Conditions,
			AsOf:        stockpicker.DateOnlyUTC(now),
		})
		if err != nil {
			return fmt.Errorf("stockpicker update: %w", err)
		}
		if res.Skipped {
			// Already recorded for this trading day (a previous tick or a
			// manual CLI run finished it): no-op tick, not a failure.
			logging.Info("stockpicker_daily_update", "skip_day_done",
				"asof", res.AsOf.Format("2006-01-02"),
				"existing", res.Existing)
			return apigateway.ErrTaskSkipped
		}
		logging.Info("stockpicker_daily_update", "run_completed",
			"asof", res.AsOf.Format("2006-01-02"),
			"outcomes", res.Outcomes,
			"keys", res.Keys,
			"eligible", res.Eligible)
		return nil
	}
}

// taipeiLocation loads Asia/Taipei, falling back to a fixed UTC+8 zone
// (same pattern as cmd/atlas daily_report_generate).
func taipeiLocation() *time.Location {
	if loc, err := time.LoadLocation("Asia/Taipei"); err == nil {
		return loc
	}
	return time.FixedZone("CST", 8*3600)
}
