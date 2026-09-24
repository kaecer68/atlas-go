// Package scheduler — per-symbol T86 flow store refresh (issue #1945).
//
// RegisterStockpickerFlowsUpdateSchedule 以 stockpicker_daily_update 同模式註冊
// BTM 任務（btm.Register + ScheduledTask）：
//
//   - Interval 1h + TimeGated：每小時 tick，僅在 Asia/Taipei 當地時間 >= 18:00
//     （台股 13:30 收盤、T86 盤後公布）且當天是台股交易日時執行。
//
//   - 執行體是 internal/stockflows.Run，把「上一次已入庫的最新交易日 + 1」
//     到今天的每個平日抓一次全市場 T86 並合併進
//     data/state/stock_flows/<symbol>.json（idempotent merge，重跑不重複）。
//     在此之前該目錄只有手動 CLI（cmd/backfill-stockpicker-flows）會寫，
//     沒有任何排程，因此 2026-08-27 之後完全停更（issue #1945）。
//
//   - 增量視窗以檔案內容（NewestStoredDate）推算而非記事本狀態，所以任何
//     一次失敗都會在下一個 tick 自動補齊；單次視窗上限 MaxSpanDays，避免
//     久停之後一次打爆 TWSE。
//
//   - enabled 預設 true：對齊本模組既有任務（stockpicker_daily_update 等
//     皆 Enabled: true），且本任務有「交易日 + 時段 + 增量視窗 + 每日
//     idempotent merge」多重保護，加上 per-day min-rows 反假成功閘門。
package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/stockflows"
	"github.com/kaecer68/atlas-go/internal/taiwanholidays"
)

// StockpickerFlowsUpdateTaskName is the BackgroundTaskManager registration
// name.
const StockpickerFlowsUpdateTaskName = "stockpicker_flows_update"

// stockpickerFlowsRunHour is the earliest Asia/Taipei hour the per-symbol
// flow refresh may run: T86 is published after the 13:30 close, so 18:00
// guarantees the completed trading day is available.
const stockpickerFlowsRunHour = 18

// stockpickerFlowsMaxSpanDays bounds one catch-up run. A longer gap (months
// without a writer, as in issue #1945) is repaired with the CLI
// (cmd/backfill-stockpicker-flows); the scheduled task logs the clamp and
// works forward from the clamp boundary instead of hammering TWSE.
const stockpickerFlowsMaxSpanDays = 14

// StockpickerFlowsUpdateDeps groups the dependencies for the refresh task.
// WorkDir is required; everything else has a safe default.
type StockpickerFlowsUpdateDeps struct {
	WorkDir  string         // atlas repo root; the flow store lives under it
	TimeZone *time.Location // nil → Asia/Taipei (fallback UTC+8)
	// Runner is the test seam; nil → stockflows.Run.
	Runner func(ctx context.Context, cfg stockflows.Config) (stockflows.Result, error)
	// Now is the test seam; nil → time.Now.
	Now func() time.Time
	// MaxSpanDays bounds one run's window; <= 0 → stockpickerFlowsMaxSpanDays.
	MaxSpanDays int
	// Sleep paces TWSE requests between days; <= 0 → stockflows.DefaultSleep
	// (a multi-day catch-up must not fire unpaced requests at TWSE).
	Sleep time.Duration
}

// RegisterStockpickerFlowsUpdateSchedule registers the daily post-close
// per-symbol flow refresh with the BackgroundTaskManager. Fire-and-register
// convention: Register errors (duplicate name) are ignored, aligning with
// RegisterStockpickerUpdateSchedule.
func RegisterStockpickerFlowsUpdateSchedule(btm *apigateway.BackgroundTaskManager, deps StockpickerFlowsUpdateDeps) {
	if btm == nil {
		return
	}
	_ = btm.Register(&apigateway.ScheduledTask{
		Name:      StockpickerFlowsUpdateTaskName,
		Interval:  time.Hour,
		TimeGated: true,
		Enabled:   true,
		Task:      StockpickerFlowsUpdateTaskFunc(deps),
	})
}

// StockpickerFlowsUpdateTaskFunc returns the BackgroundTaskManager-compatible
// task closure. Gates, in order:
//
//  1. Asia/Taipei hour >= 18 → otherwise ErrTaskSkipped (window not open).
//  2. today is a Taiwan trading day → otherwise ErrTaskSkipped.
//  3. the store already covers today → ErrTaskSkipped (nothing to fetch).
//
// Any runner error is returned so BackgroundTaskManager records the failure
// and the next hourly tick retries with the same incremental window.
func StockpickerFlowsUpdateTaskFunc(deps StockpickerFlowsUpdateDeps) func(context.Context) error {
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
		runner = stockflows.Run
	}
	maxSpan := deps.MaxSpanDays
	if maxSpan <= 0 {
		maxSpan = stockpickerFlowsMaxSpanDays
	}
	sleep := deps.Sleep
	if sleep <= 0 {
		sleep = stockflows.DefaultSleep
	}

	return func(ctx context.Context) error {
		now := nowFn().In(tz)
		if now.Hour() < stockpickerFlowsRunHour {
			return apigateway.ErrTaskSkipped
		}
		// Date-only "today" in the task's own zone: the window arithmetic
		// below mixes it with dates parsed from the store (UTC midnight), so
		// both must live on one calendar basis (date-only, same location).
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, tz)
		if !taiwanholidays.IsTradingDay(today) {
			logging.Info("stockpicker_flows_update", "skip_non_trading_day",
				"date", today.Format("2006-01-02"))
			return apigateway.ErrTaskSkipped
		}
		if deps.WorkDir == "" {
			return fmt.Errorf("stockpicker flows update: WorkDir is empty")
		}

		flowsDir := stockflows.Dir(deps.WorkDir)
		start, covered, err := stockpickerFlowsWindow(flowsDir, today, maxSpan)
		if err != nil {
			return fmt.Errorf("stockpicker flows update: %w", err)
		}
		if covered {
			logging.Info("stockpicker_flows_update", "skip_up_to_date",
				"date", today.Format("2006-01-02"))
			return apigateway.ErrTaskSkipped
		}

		logging.Info("stockpicker_flows_update", "run_started",
			"start", start.Format("2006-01-02"),
			"end", today.Format("2006-01-02"),
			"flows_dir", flowsDir)
		res, err := runner(ctx, stockflows.Config{
			WorkDir: deps.WorkDir,
			Start:   start,
			End:     today,
			Sleep:   sleep,
		})
		if err != nil {
			return fmt.Errorf("stockpicker flows update: %w", err)
		}
		logging.Info("stockpicker_flows_update", "run_completed",
			"start", start.Format("2006-01-02"),
			"end", today.Format("2006-01-02"),
			"days", len(res.Days),
			"symbols", len(res.Written),
			"flow_points", res.FlowPoints)
		return nil
	}
}

// stockpickerFlowsWindow resolves the incremental fetch window: the day after
// the newest stored flow date through today. An empty/missing store falls
// back to the last stockflows.DefaultLookbackDays calendar days so a fresh
// deployment does not request a decade of history. Window longer than
// maxSpan is clamped to today-maxSpan (logged) so a long outage cannot turn
// one tick into hundreds of TWSE requests. covered reports that the store
// already reaches today, in which case the tick is a no-op.
func stockpickerFlowsWindow(flowsDir string, today time.Time, maxSpan int) (start time.Time, covered bool, err error) {
	newest, ok, err := stockflows.NewestStoredDate(flowsDir)
	if err != nil {
		return time.Time{}, false, err
	}
	start = today.AddDate(0, 0, -stockflows.DefaultLookbackDays)
	if ok {
		// NewestStoredDate returns a UTC-midnight instant; re-anchor it to
		// today's calendar location before comparing/adding.
		start = time.Date(newest.Year(), newest.Month(), newest.Day(), 0, 0, 0, 0, today.Location()).AddDate(0, 0, 1)
	}
	if start.After(today) {
		return time.Time{}, true, nil
	}
	if clamp := today.AddDate(0, 0, -maxSpan); start.Before(clamp) {
		logging.Warn("stockpicker_flows_update", "window_clamped",
			"requested_start", start.Format("2006-01-02"),
			"clamped_start", clamp.Format("2006-01-02"),
			"note", "use cmd/backfill-stockpicker-flows for a wider historical repair")
		start = clamp
	}
	return start, false, nil
}
