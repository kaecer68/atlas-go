package taiwanholidays

import (
	"testing"
	"time"
)

// TestTradingDaysMatchTWSEOfficialCalendar pins the weekday market closures of
// the 114 (2025) and 115 (2026) TWSE 開休市日期 calendars against this package,
// so a hole in the tables fails loudly here instead of showing up as a pipeline
// that ran on a 休市日.
//
// Source (fetched 2026-09-26):
// https://www.twse.com.tw/rwd/zh/holidaySchedule/holidaySchedule?response=json&date=YYYY
//
// Rows whose 名稱 ends in 交易日 are the "market (re)opens" markers, not
// closures — they are asserted to BE trading days below. The 2026 list is the
// one that caught the missing entries this test now locks: 09-28
// (孔子誕辰紀念日/教師節, a Monday) and 12-25 (行憲紀念日, a Friday) were both
// treated as trading days because the tables only carried the 2025 補假 rows.
func TestTradingDaysMatchTWSEOfficialCalendar(t *testing.T) {
	closures2025 := []string{
		"2025-01-01", "2025-01-23", "2025-01-24", "2025-01-27", "2025-01-28",
		"2025-01-29", "2025-01-30", "2025-01-31", "2025-02-28", "2025-04-03",
		"2025-04-04", "2025-05-01", "2025-05-30", "2025-05-31", "2025-09-28",
		"2025-09-29", "2025-10-06", "2025-10-10", "2025-10-24", "2025-10-25",
		"2025-12-25",
	}
	closures2026 := []string{
		"2026-01-01", "2026-02-12", "2026-02-13", "2026-02-15", "2026-02-16",
		"2026-02-17", "2026-02-18", "2026-02-19", "2026-02-20", "2026-02-27",
		"2026-02-28", "2026-04-03", "2026-04-04", "2026-04-05", "2026-04-06",
		"2026-05-01", "2026-06-19", "2026-09-25", "2026-09-28", "2026-10-09",
		"2026-10-10", "2026-10-25", "2026-10-26", "2026-12-25",
	}
	// 交易日 rows: the calendar explicitly marks these as sessions.
	reopenMarkers := []string{
		"2025-01-02", "2025-01-22", "2025-02-03",
		"2026-01-02", "2026-02-11", "2026-02-23",
	}

	for _, date := range append(append([]string{}, closures2025...), closures2026...) {
		d := mustParseDate(t, date)
		if IsTradingDay(d) {
			t.Errorf("IsTradingDay(%s %s) = true, but the TWSE 開休市日 calendar lists it as a closure", date, d.Weekday())
		}
		// Weekend closures need no table row (IsTradingDay already says false), so
		// the holiday-table assertion only applies to weekday closures.
		if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday && !IsHoliday(d) {
			t.Errorf("IsHoliday(%s %s) = false, but the TWSE 開休市日 calendar lists it as a closure", date, wd)
		}
	}
	for _, date := range reopenMarkers {
		d := mustParseDate(t, date)
		if !IsTradingDay(d) {
			t.Errorf("IsTradingDay(%s %s) = false, but the TWSE calendar marks it as a 交易日", date, d.Weekday())
		}
	}
}

// TestRestoredNationalHolidaysAreYearScoped pins the two holidays that came
// back as 放假日 in 2025 (教師節 09-28, 行憲紀念日 12-25) and therefore cannot
// live in fixedHolidays, which has no year axis: 2021-2024 had neither, so a
// recurring entry would close real sessions in those years.
//
// 2026-09-28 is the date that exposed the universe scheduler's weekday-only
// gate: the block below is what makes "Monday" not equal "trading day".
func TestRestoredNationalHolidaysAreYearScoped(t *testing.T) {
	notClosed := []string{"2023-09-28", "2024-09-27", "2023-12-25", "2024-12-25"}
	for _, date := range notClosed {
		d := mustParseDate(t, date)
		if IsHoliday(d) {
			t.Errorf("IsHoliday(%s %s) = true: 教師節/行憲紀念日 were not 放假日 before 2025", date, d.Weekday())
		}
	}
	closed := map[string]string{
		"2025-12-25": "行憲紀念日 (restored in 2025)",
		"2026-09-28": "孔子誕辰紀念日/教師節 (Monday; the scheduler gate case)",
		"2026-12-25": "行憲紀念日",
	}
	for date, why := range closed {
		d := mustParseDate(t, date)
		if IsTradingDay(d) {
			t.Errorf("IsTradingDay(%s) = true, want false: %s", date, why)
		}
	}
}

func mustParseDate(t *testing.T, date string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		t.Fatalf("bad fixture %q: %v", date, err)
	}
	return d
}
