package industry

// Regression guard for the trading-day judgement of EventCalendar (issue #1947
// follow-up, 2026-10-01).
//
// IsTaiwanTradingDay previously reported "non-trading" for every weekday inside
// a long_holiday EVENT window, where buildHolidayEvent defines each public
// holiday as [holiday-3d, holiday+2d]. That window is a market-sentiment window
// ("連假前後交易淡季"), not a market closure, and it also misses the adjusted
// holidays (補假/調整放假) that produce no long_holiday occurrence at all
// (2026-09-28 教師節). Both directions were wrong:
//
//   - false "closed" for real trading days -> coverage/gap gates skipped them,
//     so a genuine data gap went unreported (a check that misses real gaps);
//   - false "open" for 2026-09-28 -> a weekday-only gate ran on a market closure.
//
// The judgement is now date-exact and delegated to internal/taiwanholidays, the
// same table marketdata.IsTaiwanTradingDay uses.

import (
	"testing"
	"time"
)

// tradingDayContractDates is the frozen week from the 2026 中秋/教師節
// production incident plus the adjusted-holiday dates the long_holiday windows
// cannot cover.
var tradingDayContractDates = []struct {
	date string
	want bool
	why  string
}{
	{"2026-09-22", true, "Tuesday — real trading day inside the 中秋 long_holiday window"},
	{"2026-09-23", true, "Wednesday — real trading day inside the 中秋 long_holiday window"},
	{"2026-09-24", true, "Thursday — real trading day inside the 中秋 long_holiday window"},
	{"2026-09-25", false, "中秋節 (中秋) — market closure"},
	{"2026-09-26", false, "Saturday"},
	{"2026-09-27", false, "Sunday"},
	{"2026-09-28", false, "教師節/孔子誕辰紀念日 — adjusted holiday (adjustedHolidays), no long_holiday occurrence"},
	{"2026-09-29", true, "Tuesday — first trading day after the 中秋/教師節 weekend"},
	{"2026-10-09", false, "國慶日補假 (10-10 is a Saturday)"},
	{"2026-12-25", false, "行憲紀念日 — 2025+ restored holiday, adjustedHolidays"},
	{"2026-06-24", true, "plain Wednesday"},
}

func TestEventCalendarIsTaiwanTradingDay_IsDateExact(t *testing.T) {
	tec := NewEventCalendar()
	tec.RefreshEvents(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))

	for _, tc := range tradingDayContractDates {
		t.Run(tc.date, func(t *testing.T) {
			date, err := time.Parse("2006-01-02", tc.date)
			if err != nil {
				t.Fatalf("parse %s: %v", tc.date, err)
			}
			// 13:00 Taipei == 05:00 UTC; the judgement is calendar-date based.
			instant := date.Add(5 * time.Hour)
			if got := tec.IsTaiwanTradingDay(instant); got != tc.want {
				t.Errorf("IsTaiwanTradingDay(%s) = %v, want %v (%s)", tc.date, got, tc.want, tc.why)
			}
		})
	}
}

// TestEventCalendarLongHolidayWindowStillExists pins the part that must NOT
// change: the long_holiday event is a sentiment window around each holiday and
// keeps spanning the surrounding trading days. Only the trading-day judgement
// became date-exact; the event layer feeding event-sentiment rules is untouched.
func TestEventCalendarLongHolidayWindowStillExists(t *testing.T) {
	tec := NewEventCalendar()
	tec.RefreshEvents(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC))

	midAutumn := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)
	for _, offset := range []int{-3, -2, -1, 0} {
		date := midAutumn.AddDate(0, 0, offset)
		found := false
		for _, evt := range tec.GetEventsForDate(date) {
			if evt.EventType == string(EventLongHoliday) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected a long_holiday sentiment event on %s (中秋 window [holiday-3d, holiday+2d])",
				date.Format("2006-01-02"))
		}
	}
}
