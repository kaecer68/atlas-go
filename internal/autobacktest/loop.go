package autobacktest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// ErrNotInWindow marks a scheduled-backtest tick that falls outside the
// 13:30±30m Taipei window (or on a weekend). The task wrapper maps it to
// apigateway.ErrTaskSkipped so no-op ticks do not reset failure counters.
var ErrNotInWindow = errors.New("autobacktest: outside scheduled window")

func StartDailyLoop(ctx context.Context, runner *Runner) {
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}

		logging.Info("autobacktest", "daily_loop_started")

		for {
			now := timeNow()
			next := next13_30(now)

			wait := next.Sub(now)
			logging.Info("autobacktest", "next_scheduled_run", "scheduled_time", next.Format("2006-01-02 15:04"), "wait_seconds", wait.Round(time.Second).String())

			select {
			case <-ctx.Done():
				logging.Info("autobacktest", "loop_stopped")
				return
			case <-time.After(wait):
				if t := timeNow(); t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
					logging.Info("autobacktest", "weekend_skip")
					continue
				}

				logging.Info("autobacktest", "triggering_daily_backtest")
				if err := runner.RunAndStore(); err != nil {
					logging.Error("autobacktest", "backtest_run_failed", "err", err.Error())
				} else {
					logging.Info("autobacktest", "daily_backtest_completed")
				}
			}
		}
	}()
}

func next13_30(from time.Time) time.Time {
	taipei, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		logging.Warn("autobacktest", "timezone_load_failed", "err", err.Error())
		taipei = time.FixedZone("CST", 8*3600)
	}
	today := from.In(taipei)

	scheduled := time.Date(today.Year(), today.Month(), today.Day(), 13, 30, 0, 0, taipei)

	if from.After(scheduled) || from.Equal(scheduled) {
		scheduled = scheduled.AddDate(0, 0, 1)
	}

	for scheduled.Weekday() == time.Saturday || scheduled.Weekday() == time.Sunday {
		scheduled = scheduled.AddDate(0, 0, 1)
	}

	return scheduled
}

var timeNow = time.Now

func RunScheduledBacktest(ctx context.Context, runner *Runner) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	taipei, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		taipei = time.FixedZone("CST", 8*3600)
	}
	now := time.Now().In(taipei)
	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		return ErrNotInWindow
	}
	scheduled := time.Date(now.Year(), now.Month(), now.Day(), 13, 30, 0, 0, taipei)
	diff := now.Sub(scheduled)
	if diff < -30*time.Minute || diff > 30*time.Minute {
		return ErrNotInWindow
	}
	logging.Info("autobacktest", "triggering_scheduled_backtest")
	if err := runner.RunAndStore(); err != nil {
		return fmt.Errorf("autobacktest RunAndStore: %w", err)
	}
	logging.Info("autobacktest", "scheduled_backtest_completed")
	return nil
}
