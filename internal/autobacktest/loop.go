package autobacktest

import (
	"context"
	"log"
	"time"
)

func StartDailyLoop(ctx context.Context, runner *Runner) {
	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}

		log.Printf("[Autobacktest] daily loop started")

		for {
			now := timeNow()
			next := next13_30(now)

			wait := next.Sub(now)
			log.Printf("[Autobacktest] next scheduled run: %s (in %s)", next.Format("2006-01-02 15:04"), wait.Round(time.Second))

			select {
			case <-ctx.Done():
				log.Printf("[Autobacktest] loop stopped")
				return
			case <-time.After(wait):
				if t := timeNow(); t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
					log.Printf("[Autobacktest] 13:30 falls on weekend; skipping")
					continue
				}

				log.Printf("[Autobacktest] triggering daily backtest")
				if err := runner.RunAndStore(); err != nil {
					log.Printf("[Autobacktest] run failed: %v", err)
				} else {
					log.Printf("[Autobacktest] daily backtest completed successfully")
				}
			}
		}
	}()
}

func next13_30(from time.Time) time.Time {
	taipei, _ := time.LoadLocation("Asia/Taipei")
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
