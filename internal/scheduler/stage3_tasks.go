package scheduler

import (
	"context"
	"fmt"
	"time"

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
}

// SyncEventsDailyTaskFunc returns a BackgroundTaskManager-compatible task that
// refreshes the event calendar for today and tomorrow at 06:00 local time.
func SyncEventsDailyTaskFunc(deps Stage3TaskDeps) func(context.Context) error {
	shouldRun := dailyOnceGuard(deps.TimeZone, 6, 0)
	return func(ctx context.Context) error {
		if !shouldRun() {
			return nil
		}
		return runWithRetryAndAudit(ctx, "sync-events-daily", func() error {
			if deps.RefreshEventCalendar == nil {
				return fmt.Errorf("RefreshEventCalendar dependency is nil")
			}
			now := timeNow().In(orTZ(deps.TimeZone))
			if err := deps.RefreshEventCalendar(now); err != nil {
				return err
			}
			return deps.RefreshEventCalendar(now.Add(24 * time.Hour))
		})
	}
}

// SyncMacroDailyTaskFunc returns a BackgroundTaskManager-compatible task that
// refreshes the macro snapshot after the US market close at 06:00 local time.
func SyncMacroDailyTaskFunc(deps Stage3TaskDeps) func(context.Context) error {
	shouldRun := dailyOnceGuard(deps.TimeZone, 6, 0)
	return func(ctx context.Context) error {
		if !shouldRun() {
			return nil
		}
		return runWithRetryAndAudit(ctx, "sync-macro-daily", func() error {
			if deps.RefreshMacroSnapshot == nil {
				return fmt.Errorf("RefreshMacroSnapshot dependency is nil")
			}
			return deps.RefreshMacroSnapshot(ctx)
		})
	}
}

// SyncCapitalDailyTaskFunc returns a BackgroundTaskManager-compatible task that
// refreshes the TWSE capital-flow / 三法人買賣超 aggregation at 13:30 local time.
func SyncCapitalDailyTaskFunc(deps Stage3TaskDeps) func(context.Context) error {
	shouldRun := dailyOnceGuard(deps.TimeZone, 13, 30)
	return func(ctx context.Context) error {
		if !shouldRun() {
			return nil
		}
		return runWithRetryAndAudit(ctx, "sync-capital-daily", func() error {
			if deps.RefreshCapitalFlow == nil {
				return fmt.Errorf("RefreshCapitalFlow dependency is nil")
			}
			return deps.RefreshCapitalFlow(ctx)
		})
	}
}

// SyncRegimeWeeklyTaskFunc returns a BackgroundTaskManager-compatible task that
// refreshes the regime historical summary every Monday at 08:00 local time.
func SyncRegimeWeeklyTaskFunc(deps Stage3TaskDeps) func(context.Context) error {
	shouldRun := weeklyOnceGuard(deps.TimeZone, time.Monday, 8, 0)
	return func(ctx context.Context) error {
		if !shouldRun() {
			return nil
		}
		return runWithRetryAndAudit(ctx, "sync-regime-weekly", func() error {
			if deps.UpdateRegimeHistory == nil {
				return fmt.Errorf("UpdateRegimeHistory dependency is nil")
			}
			return deps.UpdateRegimeHistory(ctx, 90)
		})
	}
}

// RecalibrateTemplatesMonthlyTaskFunc returns a BackgroundTaskManager-compatible
// task that recalculates narrative template hit rates on the 1st of every month
// at 08:00 local time.
func RecalibrateTemplatesMonthlyTaskFunc(deps Stage3TaskDeps) func(context.Context) error {
	shouldRun := monthlyOnceGuard(deps.TimeZone, 1, 8, 0)
	return func(ctx context.Context) error {
		if !shouldRun() {
			return nil
		}
		return runWithRetryAndAudit(ctx, "recalibrate-templates-monthly", func() error {
			if deps.RecalculateTemplateHitRates == nil {
				return fmt.Errorf("RecalculateTemplateHitRates dependency is nil")
			}
			return deps.RecalculateTemplateHitRates()
		})
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
	logging.Info("stage3_task_audit", "task_executed",
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
