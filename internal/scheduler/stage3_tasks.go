package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// maxStage3Retries is the maximum number of retry attempts after the initial
// execution for a Stage 3 scheduled task. 3 retries gives 4 total attempts and
// aligns with the 2s / 4s / 8s exponential backoff delays.
const maxStage3Retries = 3

// stage3RetryDelays provides exponential backoff between retry attempts.
var stage3RetryDelays = []time.Duration{
	2 * time.Second,
	4 * time.Second,
	8 * time.Second,
}

// timeNow is a seam that allows tests to control the current time. In
// production it always delegates to time.Now.
var timeNow = time.Now

// Stage3TaskDeps groups the dependencies required by Stage 3 scheduled tasks.
// It is intentionally decoupled from concrete service types so that the task
// wrappers can be unit-tested without importing the whole application graph.
type Stage3TaskDeps struct {
	// TimeZone is used for all schedule checks. If nil, UTC is assumed.
	TimeZone *time.Location

	// OncestampStore persists once-per-period claims across process restarts.
	// Optional; when nil the once-guards keep all state in-memory.
	OncestampStore OncestampStore

	// OnTaskComplete is invoked after each task run (success or failure)
	// with the static taskID. Optional; nil means no callback. Used by the
	// production wiring to emit atlas_stage3_task_runs_total.
	OnTaskComplete func(taskID string, err error)

	// RefreshEventCalendar updates the in-memory event calendar for the
	// requested local date. The task passes today and today+1.
	RefreshEventCalendar func(now time.Time) error

	// RefreshMacroSnapshot refreshes the on-disk macro snapshot cache.
	RefreshMacroSnapshot func(ctx context.Context) error

	// RefreshCapitalFlow refreshes the market-level capital flow / 三法人買賣超
	// aggregation for the current trading day.
	RefreshCapitalFlow func(ctx context.Context) error

	// UpdateRegimeHistory refreshes the regime historical summary.
	UpdateRegimeHistory func(ctx context.Context, lookbackDays int) error

	// RecalculateTemplateHitRates recalculates narrative template hit rates.
	RecalculateTemplateHitRates func() error

	// LoadPrevDayPrediction returns the event-flow prediction captured for the
	// previous trading day. Returns false when no prediction exists (e.g. the
	// prediction store is empty or the previous day had no prediction).
	LoadPrevDayPrediction func() (ledger.EventFlowPredictionRecord, bool)

	// LoadPrevDayActual returns the realized foreign net buy/sell (T86) for the
	// previous trading day as a signed magnitude in hundred-million shares
	// (positive = foreign net buy). Returns false when the actual is not yet
	// available.
	LoadPrevDayActual func() (float64, bool)

	// UpdatePrevDayActual persists the realized actual onto the prediction
	// record captured at predictedAt (matched by Taipei date).
	UpdatePrevDayActual func(predictedAt time.Time, actualSign float64, source string) error
}

// SyncEventsDailyTaskFunc returns a BackgroundTaskManager-compatible task that
// refreshes the event calendar for today and tomorrow at 06:00 local time.
func SyncEventsDailyTaskFunc(deps Stage3TaskDeps) func(context.Context) error {
	const taskID = "sync-events-daily"
	shouldRun := dailyGuardFor(deps, 6, 0)
	return func(ctx context.Context) error {
		if !shouldRun() {
			return nil
		}
		err := runWithRetryAndAudit(ctx, taskID, func() error {
			if deps.RefreshEventCalendar == nil {
				return fmt.Errorf("RefreshEventCalendar dependency is nil")
			}
			now := timeNow().In(orTZ(deps.TimeZone))
			if err := deps.RefreshEventCalendar(now); err != nil {
				return err
			}
			return deps.RefreshEventCalendar(now.Add(24 * time.Hour))
		})
		if deps.OnTaskComplete != nil {
			deps.OnTaskComplete(taskID, err)
		}
		return err
	}
}

// SyncMacroDailyTaskFunc returns a BackgroundTaskManager-compatible task that
// refreshes the macro snapshot after the US market close at 06:00 local time.
func SyncMacroDailyTaskFunc(deps Stage3TaskDeps) func(context.Context) error {
	const taskID = "sync-macro-daily"
	shouldRun := dailyGuardFor(deps, 6, 0)
	return func(ctx context.Context) error {
		if !shouldRun() {
			return nil
		}
		err := runWithRetryAndAudit(ctx, taskID, func() error {
			if deps.RefreshMacroSnapshot == nil {
				return fmt.Errorf("RefreshMacroSnapshot dependency is nil")
			}
			return deps.RefreshMacroSnapshot(ctx)
		})
		if deps.OnTaskComplete != nil {
			deps.OnTaskComplete(taskID, err)
		}
		return err
	}
}

// SyncCapitalDailyTaskFunc returns a BackgroundTaskManager-compatible task that
// refreshes the TWSE capital-flow / 三法人買賣超 aggregation at 13:30 local time.
func SyncCapitalDailyTaskFunc(deps Stage3TaskDeps) func(context.Context) error {
	const taskID = "sync-capital-daily"
	shouldRun := dailyGuardFor(deps, 13, 30)
	return func(ctx context.Context) error {
		if !shouldRun() {
			return nil
		}
		err := runWithRetryAndAudit(ctx, taskID, func() error {
			if deps.RefreshCapitalFlow == nil {
				return fmt.Errorf("RefreshCapitalFlow dependency is nil")
			}
			return deps.RefreshCapitalFlow(ctx)
		})
		if deps.OnTaskComplete != nil {
			deps.OnTaskComplete(taskID, err)
		}
		return err
	}
}

// SyncRegimeWeeklyTaskFunc returns a BackgroundTaskManager-compatible task that
// refreshes the regime historical summary every Monday at 08:00 local time.
func SyncRegimeWeeklyTaskFunc(deps Stage3TaskDeps) func(context.Context) error {
	const taskID = "sync-regime-weekly"
	shouldRun := weeklyGuardFor(deps, time.Monday, 8, 0)
	return func(ctx context.Context) error {
		if !shouldRun() {
			return nil
		}
		err := runWithRetryAndAudit(ctx, taskID, func() error {
			if deps.UpdateRegimeHistory == nil {
				return fmt.Errorf("UpdateRegimeHistory dependency is nil")
			}
			return deps.UpdateRegimeHistory(ctx, 90)
		})
		if deps.OnTaskComplete != nil {
			deps.OnTaskComplete(taskID, err)
		}
		return err
	}
}

// RecalibrateTemplatesMonthlyTaskFunc returns a BackgroundTaskManager-compatible
// task that recalculates narrative template hit rates on the 1st of every month
// at 08:00 local time.
func RecalibrateTemplatesMonthlyTaskFunc(deps Stage3TaskDeps) func(context.Context) error {
	const taskID = "recalibrate-templates-monthly"
	shouldRun := monthlyGuardFor(deps, 1, 8, 0)
	return func(ctx context.Context) error {
		if !shouldRun() {
			return nil
		}
		err := runWithRetryAndAudit(ctx, taskID, func() error {
			if deps.RecalculateTemplateHitRates == nil {
				return fmt.Errorf("RecalculateTemplateHitRates dependency is nil")
			}
			return deps.RecalculateTemplateHitRates()
		})
		if deps.OnTaskComplete != nil {
			deps.OnTaskComplete(taskID, err)
		}
		return err
	}
}

// ReconcilePrevDayPredictionTaskFunc returns a BackgroundTaskManager-compatible
// task that fills the previous trading day's event-flow prediction with the
// realized T+1 capital-flow actual. Runs at 14:30 local time (after TWSE T86
// publish, which lands ~14:00). It is a no-op (no error) when:
//   - the previous day had no prediction (LoadPrevDayPrediction=false), or
//   - the actual is not yet available (LoadPrevDayActual=false).
//
// The task only writes via UpdateActual when both sides exist, matching the
// "prediction → actual → hit" loop described in product positioning §6
// (T+1 same-unit error tracking).
func ReconcilePrevDayPredictionTaskFunc(deps Stage3TaskDeps) func(context.Context) error {
	const taskID = "reconcile-prev-day-prediction"
	shouldRun := dailyGuardFor(deps, 14, 30)
	return func(ctx context.Context) error {
		if !shouldRun() {
			return nil
		}
		err := runWithRetryAndAudit(ctx, taskID, func() error {
			if deps.LoadPrevDayPrediction == nil || deps.LoadPrevDayActual == nil {
				return fmt.Errorf("LoadPrevDayPrediction/LoadPrevDayActual dependency is nil")
			}
			rec, ok := deps.LoadPrevDayPrediction()
			if !ok {
				logging.Info("scheduler", "reconcile_prev_day_no_prediction")
				return nil
			}
			actual, ok := deps.LoadPrevDayActual()
			if !ok {
				logging.Info("scheduler", "reconcile_prev_day_actual_unavailable")
				return nil
			}
			if deps.UpdatePrevDayActual == nil {
				return fmt.Errorf("UpdatePrevDayActual dependency is nil")
			}
			return deps.UpdatePrevDayActual(rec.PredictedAt, actual, "twse_t86")
		})
		if deps.OnTaskComplete != nil {
			deps.OnTaskComplete(taskID, err)
		}
		return err
	}
}

// runWithRetryAndAudit executes work with exponential backoff and writes an
// audit log entry for success or failure.
func runWithRetryAndAudit(ctx context.Context, taskID string, work func() error) error {
	start := timeNow()
	var err error
	retryCount := 0

	for attempt := 0; attempt <= maxStage3Retries; attempt++ {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			logTaskAudit(taskID, start, err, retryCount)
			return err
		default:
		}

		err = work()
		if err == nil {
			break
		}

		if attempt < maxStage3Retries {
			retryCount = attempt + 1
			select {
			case <-ctx.Done():
				err = ctx.Err()
				break
			case <-time.After(stage3RetryDelays[attempt]):
			}
		}
	}

	logTaskAudit(taskID, start, err, retryCount)
	return err
}

// logTaskAudit writes a structured audit log entry for a task execution.
func logTaskAudit(taskID string, scheduledAt time.Time, err error, retryCount int) {
	status := "success"
	errMsg := ""
	if err != nil {
		status = "failed"
		errMsg = err.Error()
	}
	logging.Info(
		"stage3_task_audit", "task_executed",
		"task_id", taskID,
		"scheduled_at", scheduledAt.Format(time.RFC3339),
		"executed_at", timeNow().Format(time.RFC3339),
		"status", status,
		"retry_count", retryCount,
		"error", errMsg,
	)
}

// dailyOnceGuard returns a function that returns true once per day at the
// requested local time. It is safe to call from the BTM goroutine every minute.
func dailyOnceGuard(tz *time.Location, hour, minute int) func() bool {
	var lastRun time.Time
	return func() bool {
		now := timeNow().In(orTZ(tz))
		if now.Hour() != hour || now.Minute() != minute {
			return false
		}
		if lastRun.Year() == now.Year() && lastRun.YearDay() == now.YearDay() {
			return false
		}
		lastRun = now
		return true
	}
}

// weeklyOnceGuard returns a function that returns true once per week on the
// requested weekday and local time.
func weeklyOnceGuard(tz *time.Location, weekday time.Weekday, hour, minute int) func() bool {
	var lastRun time.Time
	return func() bool {
		now := timeNow().In(orTZ(tz))
		if now.Weekday() != weekday || now.Hour() != hour || now.Minute() != minute {
			return false
		}
		weekStart := now.Add(-time.Duration((now.Weekday()+6)%7) * 24 * time.Hour)
		weekStart = time.Date(weekStart.Year(), weekStart.Month(), weekStart.Day(), 0, 0, 0, 0, orTZ(tz))
		lastWeekStart := time.Date(lastRun.Year(), lastRun.Month(), lastRun.Day(), 0, 0, 0, 0, orTZ(tz))
		lastWeekStart = lastWeekStart.Add(-time.Duration((lastRun.Weekday()+6)%7) * 24 * time.Hour)
		if weekStart.Equal(lastWeekStart) {
			return false
		}
		lastRun = now
		return true
	}
}

// monthlyOnceGuard returns a function that returns true once per month on the
// requested day and local time.
func monthlyOnceGuard(tz *time.Location, day, hour, minute int) func() bool {
	var lastRun time.Time
	return func() bool {
		now := timeNow().In(orTZ(tz))
		if now.Day() != day || now.Hour() != hour || now.Minute() != minute {
			return false
		}
		if lastRun.Year() == now.Year() && lastRun.Month() == now.Month() {
			return false
		}
		lastRun = now
		return true
	}
}

// orTZ returns tz if non-nil, otherwise UTC.
func orTZ(tz *time.Location) *time.Location {
	if tz != nil {
		return tz
	}
	return time.UTC
}

// dailyGuardFor returns a daily once-guard that delegates to OncestampStore
// when deps.OncestampStore is non-nil, otherwise falls back to the in-memory
// closure used by the pre-Stage 3.1 wrappers.
func dailyGuardFor(deps Stage3TaskDeps, hour, minute int) func() bool {
	if deps.OncestampStore != nil {
		return dailyOnceGuardWithStore(deps.TimeZone, hour, minute, "stage3.daily", deps.OncestampStore, sameDay)
	}
	return dailyOnceGuard(deps.TimeZone, hour, minute)
}

// weeklyGuardFor returns a weekly once-guard with optional persistence.
func weeklyGuardFor(deps Stage3TaskDeps, weekday time.Weekday, hour, minute int) func() bool {
	if deps.OncestampStore != nil {
		return weeklyOnceGuardWithStore(deps.TimeZone, weekday, hour, minute, "stage3.weekly.monday", deps.OncestampStore, sameWeek)
	}
	return weeklyOnceGuard(deps.TimeZone, weekday, hour, minute)
}

// monthlyGuardFor returns a monthly once-guard with optional persistence.
func monthlyGuardFor(deps Stage3TaskDeps, day, hour, minute int) func() bool {
	if deps.OncestampStore != nil {
		return monthlyOnceGuardWithStore(deps.TimeZone, day, hour, minute, "stage3.monthly.first", deps.OncestampStore, sameMonth)
	}
	return monthlyOnceGuard(deps.TimeZone, day, hour, minute)
}

// dailyOnceGuardWithStore is the persistent analog of dailyOnceGuard.
// On a hit (run=false) the in-memory state is left untouched so the on-disk
// record remains the single source of truth for the period.
func dailyOnceGuardWithStore(tz *time.Location, hour, minute int, key string, store OncestampStore, samePeriod func(a, b time.Time) bool) func() bool {
	return func() bool {
		now := timeNow().In(orTZ(tz))
		if now.Hour() != hour || now.Minute() != minute {
			return false
		}
		run, ok := store.TryClaim(key, now, samePeriod)
		return ok && run
	}
}

// weeklyOnceGuardWithStore is the persistent analog of weeklyOnceGuard.
// The store key is fixed per (tz, weekday, hour, minute) tuple; the samePeriod
// comparator is anchored to Monday in tz.
func weeklyOnceGuardWithStore(tz *time.Location, weekday time.Weekday, hour, minute int, key string, store OncestampStore, samePeriod func(tz *time.Location, a, b time.Time) bool) func() bool {
	loc := orTZ(tz)
	comparator := func(a, b time.Time) bool { return samePeriod(loc, a, b) }
	return func() bool {
		now := timeNow().In(loc)
		if now.Weekday() != weekday || now.Hour() != hour || now.Minute() != minute {
			return false
		}
		run, ok := store.TryClaim(key, now, comparator)
		return ok && run
	}
}

// monthlyOnceGuardWithStore is the persistent analog of monthlyOnceGuard.
func monthlyOnceGuardWithStore(tz *time.Location, day, hour, minute int, key string, store OncestampStore, samePeriod func(tz *time.Location, a, b time.Time) bool) func() bool {
	loc := orTZ(tz)
	comparator := func(a, b time.Time) bool { return samePeriod(loc, a, b) }
	return func() bool {
		now := timeNow().In(loc)
		if now.Day() != day || now.Hour() != hour || now.Minute() != minute {
			return false
		}
		run, ok := store.TryClaim(key, now, comparator)
		return ok && run
	}
}
