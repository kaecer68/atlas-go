package monitoring

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// This file is the regression net for the holiday-blind universe gate.
//
// The scheduler used to answer "is today a trading day?" from a local
// weekday-only predicate, and the weekly rebuild only asked "is it Monday?".
// Both therefore ran the pipeline on weekday public holidays. The case that
// exposed it: 2026-09-28 is a Monday AND 孔子誕辰紀念日/教師節 (TWSE 115 年
// 開休市日期 calendar) — the market is closed, so a rebuild could only
// re-snapshot the previous session's quotes.
//
// Every fixture below asserts its own calendar premise against
// marketdata.IsTaiwanTradingDay before it is used, so a table change fails
// loudly here instead of silently weakening the assertions.

// captureLogs installs a Debug-level text logger into the returned buffer for
// the duration of the test and restores the previous logger afterwards.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := logging.Default()
	logging.SetLogger(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { logging.SetLogger(prev) })
	return buf
}

// freezeClock pins the scheduler clock for the duration of the test.
func freezeClock(t *testing.T, instant time.Time) {
	t.Helper()
	prev := clockFunc
	clockFunc = func() time.Time { return instant }
	t.Cleanup(func() { clockFunc = prev })
}

// universeSnapshotPath is the artifact both tasks write when they run.
func universeSnapshotPath(workDir string) string {
	return filepath.Join(workDir, "data", "state", "universe_snapshot.json")
}

// requireCalendarPremise fails the test when the authoritative calendar does not
// agree with the expected judgement — the fixtures must be calendar facts, not
// assumptions.
func requireCalendarPremise(t *testing.T, day time.Time, wantTrading bool) {
	t.Helper()
	if got := marketdata.IsTaiwanTradingDay(day); got != wantTrading {
		t.Fatalf("fixture drift: marketdata.IsTaiwanTradingDay(%s %s) = %v, want %v",
			day.Format("2006-01-02"), day.Weekday(), got, wantTrading)
	}
}

// TestDailyRefresh_HolidayAwareGate verifies the incremental task against the
// authoritative Taiwan calendar: it skips holidays and weekends, and runs on
// ordinary Tue-Fri sessions.
func TestDailyRefresh_HolidayAwareGate(t *testing.T) {
	cases := []struct {
		name        string
		date        time.Time
		wantTrading bool
		why         string
	}{
		{
			name:        "holiday_monday_teacher_day",
			date:        time.Date(2026, 9, 28, 0, 0, 0, 0, universeLocation()),
			wantTrading: false,
			why:         "Monday 孔子誕辰紀念日/教師節 (休市) — the weekday-only gate ran here",
		},
		{
			name:        "holiday_friday_mid_autumn",
			date:        time.Date(2026, 9, 25, 0, 0, 0, 0, universeLocation()),
			wantTrading: false,
			why:         "Friday 中秋節 (休市)",
		},
		{
			name:        "saturday",
			date:        time.Date(2026, 9, 26, 0, 0, 0, 0, universeLocation()),
			wantTrading: false,
			why:         "weekend",
		},
		{
			name:        "sunday",
			date:        time.Date(2026, 9, 27, 0, 0, 0, 0, universeLocation()),
			wantTrading: false,
			why:         "weekend",
		},
		{
			name:        "tuesday_after_the_holidays",
			date:        time.Date(2026, 9, 29, 0, 0, 0, 0, universeLocation()),
			wantTrading: true,
			why:         "ordinary Tuesday session",
		},
		{
			name:        "wednesday",
			date:        time.Date(2026, 9, 30, 0, 0, 0, 0, universeLocation()),
			wantTrading: true,
			why:         "ordinary Wednesday session",
		},
		{
			name:        "thursday",
			date:        time.Date(2026, 10, 1, 0, 0, 0, 0, universeLocation()),
			wantTrading: true,
			why:         "ordinary Thursday session",
		},
		{
			name:        "friday",
			date:        time.Date(2026, 10, 2, 0, 0, 0, 0, universeLocation()),
			wantTrading: true,
			why:         "ordinary Friday session",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireCalendarPremise(t, tc.date, tc.wantTrading)

			workDir := tempDir(t)
			deps := buildDepsFixture(t, workDir)
			freezeClock(t, triggerAt(tc.date.Year(), tc.date.Month(), tc.date.Day(), time.UTC))
			buf := captureLogs(t)
			defer func() { t.Logf("%s @ %s: %s", tc.name, tc.date.Format("2006-01-02"), strings.TrimSpace(buf.String())) }()

			if err := NewDailyUniverseRefreshTask(deps)(context.Background()); err != nil {
				t.Fatalf("daily refresh (%s): %v", tc.why, err)
			}

			_, statErr := os.Stat(universeSnapshotPath(workDir))
			if tc.wantTrading {
				if statErr != nil {
					t.Errorf("no snapshot after a %s: %v", tc.why, statErr)
				}
				return
			}
			if statErr == nil {
				t.Errorf("snapshot written on a non-trading day (%s): the gate must skip", tc.why)
			}
			if !strings.Contains(buf.String(), "daily_skip_non_trading") {
				t.Errorf("expected a daily_skip_non_trading log for %s, got: %s", tc.why, buf.String())
			}
			if !strings.Contains(buf.String(), tc.date.Format("2006-01-02")) {
				t.Errorf("the skip log must carry the judged date %s, got: %s", tc.date.Format("2006-01-02"), buf.String())
			}
			if !strings.Contains(buf.String(), "criterion=marketdata.IsTaiwanTradingDay") {
				t.Errorf("the skip log must name the criterion, got: %s", buf.String())
			}
		})
	}
}

// TestWeeklyRebuild_HolidayAwareGate verifies the full-rebuild task: it runs on
// an ordinary Monday and skips a Monday public holiday, with the holiday skip
// visible as weekly_skip_holiday (date + criterion).
func TestWeeklyRebuild_HolidayAwareGate(t *testing.T) {
	cases := []struct {
		name        string
		date        time.Time
		wantTrading bool
		why         string
	}{
		{
			name:        "holiday_monday_teacher_day",
			date:        time.Date(2026, 9, 28, 0, 0, 0, 0, universeLocation()),
			wantTrading: false,
			why:         "Friday-holiday weekend plus Monday 教師節; the market is closed",
		},
		{
			name:        "holiday_monday_retrocession_makeup",
			date:        time.Date(2026, 10, 26, 0, 0, 0, 0, universeLocation()),
			wantTrading: false,
			why:         "Monday 光復節補假 (10/25 was a Sunday)",
		},
		{
			name:        "ordinary_monday_after_double_ten",
			date:        time.Date(2026, 10, 12, 0, 0, 0, 0, universeLocation()),
			wantTrading: true,
			why:         "ordinary Monday session, the first after the 國慶 holiday",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireCalendarPremise(t, tc.date, tc.wantTrading)

			workDir := tempDir(t)
			deps := buildDepsFixture(t, workDir)
			freezeClock(t, triggerAt(tc.date.Year(), tc.date.Month(), tc.date.Day(), time.UTC))
			buf := captureLogs(t)
			defer func() { t.Logf("%s @ %s: %s", tc.name, tc.date.Format("2006-01-02"), strings.TrimSpace(buf.String())) }()

			if err := NewWeeklyUniverseRebuildTask(deps)(context.Background()); err != nil {
				t.Fatalf("weekly rebuild (%s): %v", tc.why, err)
			}

			_, statErr := os.Stat(universeSnapshotPath(workDir))
			if tc.wantTrading {
				if statErr != nil {
					t.Errorf("no snapshot after an ordinary Monday: %v", statErr)
				}
				if !strings.Contains(buf.String(), "weekly_rebuild_start") {
					t.Errorf("expected weekly_rebuild_start, got: %s", buf.String())
				}
				return
			}
			if statErr == nil {
				t.Errorf("full rebuild ran on a 休市日 (%s): a rebuild can only re-snapshot stale quotes", tc.why)
			}
			if !strings.Contains(buf.String(), "weekly_skip_holiday") {
				t.Errorf("expected a weekly_skip_holiday log, got: %s", buf.String())
			}
			log := buf.String()
			for _, want := range []string{
				tc.date.Format("2006-01-02"),
				"criterion=marketdata.IsTaiwanTradingDay",
				"weekday=Monday",
			} {
				if !strings.Contains(log, want) {
					t.Errorf("weekly_skip_holiday log must carry %q, got: %s", want, log)
				}
			}
			if strings.Contains(log, "weekly_rebuild_start") {
				t.Errorf("the rebuild must not start on a 休市日, got: %s", log)
			}
		})
	}
}

// TestWeeklyRebuild_WeekdayOnlyPredicateWouldHaveRun is the negative control
// kept as a permanent guard: it pins the exact input where the removed
// weekday-only predicate and the authoritative calendar disagree. Reverting the
// gate to "is it Monday?" makes this test red, because the snapshot assertion
// below would then find a rebuild artifact on a 休市日.
func TestWeeklyRebuild_WeekdayOnlyPredicateWouldHaveRun(t *testing.T) {
	instant := time.Date(2026, 9, 28, universeTriggerHourTW, 0, 0, 0, universeLocation())

	// The predicate the scheduler used to apply.
	weekdayOnly := func(t time.Time) bool {
		switch t.Weekday() {
		case time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday:
			return true
		default:
			return false
		}
	}
	if !weekdayOnly(instant) {
		t.Fatal("fixture requires 2026-09-28 to be a weekday — that is what the removed gate saw")
	}
	if !alignToTarget(instant) {
		t.Fatal("fixture requires 2026-09-28 14:00 Taipei to be the trigger instant")
	}
	requireCalendarPremise(t, instant, false)

	workDir := tempDir(t)
	deps := buildDepsFixture(t, workDir)
	freezeClock(t, instant)
	buf := captureLogs(t)

	if err := NewWeeklyUniverseRebuildTask(deps)(context.Background()); err != nil {
		t.Fatalf("weekly rebuild: %v", err)
	}
	if _, err := os.Stat(universeSnapshotPath(workDir)); err == nil {
		t.Fatalf("2026-09-28 is a 休市日 but the rebuild ran (weekday-only predicate?) — log: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "weekly_skip_holiday") {
		t.Errorf("expected weekly_skip_holiday, got: %s", buf.String())
	}
}
