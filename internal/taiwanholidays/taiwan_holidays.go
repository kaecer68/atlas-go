// Package taiwanholidays is the single source of truth for Taiwan public
// holidays (fixed-date + lunar/seasonal, 2023-2040 verified tables).
//
// P1-8: previously the lunar tables lived in TWO places — marketdata/calendar.go
// and industry/event_calendar.go — with a documented "keep them in sync"
// obligation that could drift. This package owns the tables once; both
// consumers delegate to it.
//
// Dates are stored at 00:00 UTC (calendar-date semantics). Taiwan is UTC+8,
// so a UTC midnight is 08:00 the same day in Taipei — month/day comparisons
// are therefore timezone-independent for these date-only values.
package taiwanholidays

import (
	"sort"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// Holiday is one Taiwan public holiday in a given year.
type Holiday struct {
	Name string
	Date time.Time // 00:00 UTC, calendar-date semantics
}

// twLocation is Taiwan Standard Time (UTC+8). Trading-day / holiday checks
// normalize to this zone so a time near midnight still maps to the correct
// Taiwan calendar date (matches the pre-existing marketdata semantics).
var twLocation = time.FixedZone("CST", 8*60*60)

// Fixed holidays (month, day) that never move.
var fixedHolidays = []struct {
	Name  string
	Month time.Month
	Day   int
}{
	{Name: "元旦", Month: time.January, Day: 1},
	{Name: "228和平紀念日", Month: time.February, Day: 28},
	{Name: "勞動節", Month: time.May, Day: 1},
	{Name: "國慶日", Month: time.October, Day: 10},
}

// springFestivalClosures holds the FULL TWSE closure span for the Spring
// Festival (lunar new year), not just 初一: settlement days (市場無交易僅結算),
// 除夕, 春節 初一~初五 and any 補假 (adjusted leave) — verified against the
// TWSE official 開休市日期 calendar (web, 2026-08-25). Without these, weekday
// closures during the holiday window were misjudged as trading days, so
// daily-replay-sync appended fake holiday quotes and clean-replay-weekends
// failed to remove them (2026-02 spring-festival rows in production replay,
// k3-audit follow-up). Years without an entry fall back to 初一-only with a
// warning — acceptable outside the 2023-2026 backtest window.
var springFestivalClosures = map[int][]time.Time{
	2023: {
		time.Date(2023, 1, 18, 0, 0, 0, 0, time.UTC), // 結算 1/18-19
		time.Date(2023, 1, 19, 0, 0, 0, 0, time.UTC),
		time.Date(2023, 1, 20, 0, 0, 0, 0, time.UTC), // 除夕前一日(調整放假)
		time.Date(2023, 1, 21, 0, 0, 0, 0, time.UTC), // 除夕
		time.Date(2023, 1, 22, 0, 0, 0, 0, time.UTC), // 初一
		time.Date(2023, 1, 23, 0, 0, 0, 0, time.UTC), // 初二
		time.Date(2023, 1, 24, 0, 0, 0, 0, time.UTC), // 初三
		time.Date(2023, 1, 25, 0, 0, 0, 0, time.UTC), // 補假
		time.Date(2023, 1, 26, 0, 0, 0, 0, time.UTC), // 補假
		time.Date(2023, 1, 27, 0, 0, 0, 0, time.UTC), // 調整放假
	},
	2024: {
		time.Date(2024, 2, 6, 0, 0, 0, 0, time.UTC), // 結算 2/6-7
		time.Date(2024, 2, 7, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 2, 8, 0, 0, 0, 0, time.UTC),  // 除夕前一日(調整放假)
		time.Date(2024, 2, 9, 0, 0, 0, 0, time.UTC),  // 除夕
		time.Date(2024, 2, 10, 0, 0, 0, 0, time.UTC), // 初一
		time.Date(2024, 2, 11, 0, 0, 0, 0, time.UTC), // 初二
		time.Date(2024, 2, 12, 0, 0, 0, 0, time.UTC), // 初三
		time.Date(2024, 2, 13, 0, 0, 0, 0, time.UTC), // 補假
		time.Date(2024, 2, 14, 0, 0, 0, 0, time.UTC), // 補假
	},
	2025: {
		time.Date(2025, 1, 23, 0, 0, 0, 0, time.UTC), // 結算 1/23-24
		time.Date(2025, 1, 24, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 27, 0, 0, 0, 0, time.UTC), // 除夕前一日(調整放假)
		time.Date(2025, 1, 28, 0, 0, 0, 0, time.UTC), // 除夕
		time.Date(2025, 1, 29, 0, 0, 0, 0, time.UTC), // 初一
		time.Date(2025, 1, 30, 0, 0, 0, 0, time.UTC), // 初二
		time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC), // 初三
	},
	2026: {
		time.Date(2026, 2, 12, 0, 0, 0, 0, time.UTC), // 結算 2/12-13
		time.Date(2026, 2, 13, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 2, 16, 0, 0, 0, 0, time.UTC), // 除夕（2/15 週日）
		time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC), // 初一
		time.Date(2026, 2, 18, 0, 0, 0, 0, time.UTC), // 初二
		time.Date(2026, 2, 19, 0, 0, 0, 0, time.UTC), // 初三
		time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC), // 補假
	},
}

// lunar tables: year → date (verified 2023-2030). This is the canonical
// copy — marketdata/calendar.go and industry/event_calendar.go previously
// each maintained their own duplicate and could drift.
var lunarNewYearDates = map[int]time.Time{
	2024: time.Date(2024, 2, 10, 0, 0, 0, 0, time.UTC), // 春節初一
	2025: time.Date(2025, 1, 29, 0, 0, 0, 0, time.UTC),
	2026: time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC),
	2027: time.Date(2027, 2, 6, 0, 0, 0, 0, time.UTC),
	2028: time.Date(2028, 1, 26, 0, 0, 0, 0, time.UTC),
	2029: time.Date(2029, 2, 13, 0, 0, 0, 0, time.UTC),
	2030: time.Date(2030, 2, 3, 0, 0, 0, 0, time.UTC),
	2031: time.Date(2031, 1, 23, 0, 0, 0, 0, time.UTC),
	2032: time.Date(2032, 2, 11, 0, 0, 0, 0, time.UTC),
	2033: time.Date(2033, 1, 31, 0, 0, 0, 0, time.UTC),
	2034: time.Date(2034, 2, 19, 0, 0, 0, 0, time.UTC),
	2035: time.Date(2035, 2, 8, 0, 0, 0, 0, time.UTC),
	2036: time.Date(2036, 1, 28, 0, 0, 0, 0, time.UTC),
	2037: time.Date(2037, 2, 15, 0, 0, 0, 0, time.UTC),
	2038: time.Date(2038, 2, 4, 0, 0, 0, 0, time.UTC),
	2039: time.Date(2039, 1, 24, 0, 0, 0, 0, time.UTC),
	2040: time.Date(2040, 2, 12, 0, 0, 0, 0, time.UTC),
}

var tombSweepingDates = map[int]time.Time{
	2023: time.Date(2023, 4, 5, 0, 0, 0, 0, time.UTC), // 清明
	2024: time.Date(2024, 4, 4, 0, 0, 0, 0, time.UTC),
	2025: time.Date(2025, 4, 4, 0, 0, 0, 0, time.UTC),
	2026: time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC),
	2027: time.Date(2027, 4, 5, 0, 0, 0, 0, time.UTC),
	2028: time.Date(2028, 4, 4, 0, 0, 0, 0, time.UTC),
	2029: time.Date(2029, 4, 4, 0, 0, 0, 0, time.UTC),
	2030: time.Date(2030, 4, 5, 0, 0, 0, 0, time.UTC),
	2031: time.Date(2031, 4, 5, 0, 0, 0, 0, time.UTC),
	2032: time.Date(2032, 4, 4, 0, 0, 0, 0, time.UTC),
	2033: time.Date(2033, 4, 4, 0, 0, 0, 0, time.UTC),
	2034: time.Date(2034, 4, 5, 0, 0, 0, 0, time.UTC),
	2035: time.Date(2035, 4, 5, 0, 0, 0, 0, time.UTC),
	2036: time.Date(2036, 4, 4, 0, 0, 0, 0, time.UTC),
	2037: time.Date(2037, 4, 4, 0, 0, 0, 0, time.UTC),
	2038: time.Date(2038, 4, 5, 0, 0, 0, 0, time.UTC),
	2039: time.Date(2039, 4, 5, 0, 0, 0, 0, time.UTC),
	2040: time.Date(2040, 4, 4, 0, 0, 0, 0, time.UTC),
}

var lunarDragonBoatDates = map[int]time.Time{
	2023: time.Date(2023, 6, 22, 0, 0, 0, 0, time.UTC), // 端午
	2024: time.Date(2024, 6, 10, 0, 0, 0, 0, time.UTC),
	2025: time.Date(2025, 5, 31, 0, 0, 0, 0, time.UTC),
	2026: time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC),
	2027: time.Date(2027, 6, 9, 0, 0, 0, 0, time.UTC),
	2028: time.Date(2028, 5, 28, 0, 0, 0, 0, time.UTC),
	2029: time.Date(2029, 6, 16, 0, 0, 0, 0, time.UTC),
	2030: time.Date(2030, 6, 5, 0, 0, 0, 0, time.UTC),
	2031: time.Date(2031, 6, 24, 0, 0, 0, 0, time.UTC),
	2032: time.Date(2032, 6, 12, 0, 0, 0, 0, time.UTC),
	2033: time.Date(2033, 6, 1, 0, 0, 0, 0, time.UTC),
	2034: time.Date(2034, 6, 20, 0, 0, 0, 0, time.UTC),
	2035: time.Date(2035, 6, 10, 0, 0, 0, 0, time.UTC),
	2036: time.Date(2036, 5, 30, 0, 0, 0, 0, time.UTC),
	2037: time.Date(2037, 6, 18, 0, 0, 0, 0, time.UTC),
	2038: time.Date(2038, 6, 7, 0, 0, 0, 0, time.UTC),
	2039: time.Date(2039, 5, 27, 0, 0, 0, 0, time.UTC),
	2040: time.Date(2040, 6, 14, 0, 0, 0, 0, time.UTC),
}

var lunarMidAutumnDates = map[int]time.Time{
	2023: time.Date(2023, 9, 29, 0, 0, 0, 0, time.UTC), // 中秋
	2024: time.Date(2024, 9, 17, 0, 0, 0, 0, time.UTC),
	2025: time.Date(2025, 10, 6, 0, 0, 0, 0, time.UTC),
	2026: time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC),
	2027: time.Date(2027, 9, 15, 0, 0, 0, 0, time.UTC),
	2028: time.Date(2028, 10, 3, 0, 0, 0, 0, time.UTC),
	2029: time.Date(2029, 9, 22, 0, 0, 0, 0, time.UTC),
	2030: time.Date(2030, 9, 12, 0, 0, 0, 0, time.UTC),
	2031: time.Date(2031, 10, 1, 0, 0, 0, 0, time.UTC),
	2032: time.Date(2032, 9, 19, 0, 0, 0, 0, time.UTC),
	2033: time.Date(2033, 9, 8, 0, 0, 0, 0, time.UTC),
	2034: time.Date(2034, 9, 27, 0, 0, 0, 0, time.UTC),
	2035: time.Date(2035, 9, 16, 0, 0, 0, 0, time.UTC),
	2036: time.Date(2036, 10, 4, 0, 0, 0, 0, time.UTC),
	2037: time.Date(2037, 9, 24, 0, 0, 0, 0, time.UTC),
	2038: time.Date(2038, 9, 13, 0, 0, 0, 0, time.UTC),
	2039: time.Date(2039, 10, 2, 0, 0, 0, 0, time.UTC),
	2040: time.Date(2040, 9, 20, 0, 0, 0, 0, time.UTC),
}

// Conventional fallback dates for years beyond the verified 2023-2040 range.
// These are rough approximations (matching the pre-existing industry fallback
// conventions) so callers degrade gracefully instead of panicking.
const (
	fallbackLunarNewYearMonth = time.February
	fallbackLunarNewYearDay   = 1
	fallbackTombSweepingMonth = time.April
	fallbackTombSweepingDay   = 5
	fallbackDragonBoatMonth   = time.June
	fallbackDragonBoatDay     = 10
	fallbackMidAutumnMonth    = time.September
	fallbackMidAutumnDay      = 20
)

// CoverageYears returns the verified hardcoded lunar-table range.
// 2023-2030 values came from the pre-existing marketdata/industry tables;
// 2031-2040 were computed and cross-validated (lunardate library + ephem
// solar-term astronomy) against the 2023-2030 reference on 2026-08-23.
func CoverageYears() (int, int) { return 2023, 2040 }

// LunarNewYear returns the 春節 (lunar new year) date for year. ok=false when
// the year is outside the verified range (callers should use a fallback).
func LunarNewYear(year int) (time.Time, bool) {
	d, ok := lunarNewYearDates[year]
	return d, ok
}

// TombSweeping returns the 清明節 date for year. ok=false when out of range.
func TombSweeping(year int) (time.Time, bool) {
	d, ok := tombSweepingDates[year]
	return d, ok
}

// LunarDragonBoat returns the 端午節 date for year. ok=false when out of range.
func LunarDragonBoat(year int) (time.Time, bool) {
	d, ok := lunarDragonBoatDates[year]
	return d, ok
}

// LunarMidAutumn returns the 中秋節 date for year. ok=false when out of range.
func LunarMidAutumn(year int) (time.Time, bool) {
	d, ok := lunarMidAutumnDates[year]
	return d, ok
}

// LunarNewYearDates returns a copy of the full lunar new year table
// (compat helper for consumers that index by year map-style).
func LunarNewYearDates() map[int]time.Time {
	return copyTable(lunarNewYearDates)
}

// TombSweepingDates returns a copy of the tomb-sweeping table.
func TombSweepingDates() map[int]time.Time { return copyTable(tombSweepingDates) }

// LunarDragonBoatDates returns a copy of the dragon-boat table.
func LunarDragonBoatDates() map[int]time.Time { return copyTable(lunarDragonBoatDates) }

// LunarMidAutumnDates returns a copy of the mid-autumn table.
func LunarMidAutumnDates() map[int]time.Time { return copyTable(lunarMidAutumnDates) }

func copyTable(in map[int]time.Time) map[int]time.Time {
	out := make(map[int]time.Time, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// fallbackWarned tracks years (beyond the verified range) for which we have
// already logged the fallback warning, so the log does not spam every call.
var fallbackWarned sync.Map

// warnFallback logs once per out-of-range year that the lunar table is being
// extrapolated with conventional dates instead of verified values.
func warnFallback(year int) {
	if _, loaded := fallbackWarned.LoadOrStore(year, true); loaded {
		return
	}
	logging.Warn("taiwanholidays", "lunar_fallback",
		"year", year,
		"note", "beyond verified 2023-2040 range; using conventional fallback dates — extend the lunar tables")
}

// lunarDatesInYear returns the lunar/seasonal holidays for year, using the
// verified table when present and conventional fallbacks (with a warning)
// for out-of-range years.
func lunarDatesInYear(year int) []Holiday {
	holidays := make([]Holiday, 0, 4)
	if closures, ok := springFestivalClosures[year]; ok {
		// Full TWSE closure span (settlement days + 除夕~初五 + 補假),
		// verified against the official calendar — see springFestivalClosures.
		for _, d := range closures {
			holidays = append(holidays, Holiday{Name: "春節休市", Date: d})
		}
	} else if d, ok := lunarNewYearDates[year]; ok {
		// Fallback: 初一 only (pre-2023 / beyond-2026 years) — a weekday
		// closure inside the spring window may be missed; acceptable outside
		// the verified 2023-2026 span.
		holidays = append(holidays, Holiday{Name: "春節", Date: d})
	} else {
		warnFallback(year)
		holidays = append(holidays, Holiday{Name: "春節", Date: time.Date(year, fallbackLunarNewYearMonth, fallbackLunarNewYearDay, 0, 0, 0, 0, time.UTC)})
	}
	if d, ok := tombSweepingDates[year]; ok {
		holidays = append(holidays, Holiday{Name: "清明節", Date: d})
	} else {
		warnFallback(year)
		holidays = append(holidays, Holiday{Name: "清明節", Date: time.Date(year, fallbackTombSweepingMonth, fallbackTombSweepingDay, 0, 0, 0, 0, time.UTC)})
	}
	if d, ok := lunarDragonBoatDates[year]; ok {
		holidays = append(holidays, Holiday{Name: "端午節", Date: d})
	} else {
		warnFallback(year)
		holidays = append(holidays, Holiday{Name: "端午節", Date: time.Date(year, fallbackDragonBoatMonth, fallbackDragonBoatDay, 0, 0, 0, 0, time.UTC)})
	}
	if d, ok := lunarMidAutumnDates[year]; ok {
		holidays = append(holidays, Holiday{Name: "中秋節", Date: d})
	} else {
		warnFallback(year)
		holidays = append(holidays, Holiday{Name: "中秋節", Date: time.Date(year, fallbackMidAutumnMonth, fallbackMidAutumnDay, 0, 0, 0, 0, time.UTC)})
	}
	return holidays
}

// HolidaysInYear returns every Taiwan public holiday for year (fixed-date and
// lunar/seasonal), sorted by date. Out-of-range years use conventional
// fallback dates and log a one-time warning.
func HolidaysInYear(year int) []Holiday {
	holidays := make([]Holiday, 0, 8)
	for _, f := range fixedHolidays {
		holidays = append(holidays, Holiday{Name: f.Name, Date: time.Date(year, f.Month, f.Day, 0, 0, 0, 0, time.UTC)})
	}
	holidays = append(holidays, lunarDatesInYear(year)...)
	sort.Slice(holidays, func(i, j int) bool { return holidays[i].Date.Before(holidays[j].Date) })
	return holidays
}

// IsHoliday reports whether t falls on a Taiwan public holiday. The check is
// calendar-date based (UTC normalized) — the input's time-of-day and zone are
// irrelevant, matching the pre-existing marketdata semantics.
func IsHoliday(t time.Time) bool {
	t = t.In(twLocation)
	year := t.Year()
	if year > 2040 {
		warnFallback(year)
	}
	for _, h := range HolidaysInYear(year) {
		if h.Date.Year() == t.Year() && h.Date.Month() == t.Month() && h.Date.Day() == t.Day() {
			return true
		}
	}
	return false
}

// IsTradingDay reports whether t is a Taiwan trading day: a weekday that is
// not a public holiday.
func IsTradingDay(t time.Time) bool {
	w := t.Weekday()
	if w == time.Saturday || w == time.Sunday {
		return false
	}
	return !IsHoliday(t)
}

// PreviousTradingDay rolls back daysBack calendar days from now, then walks
// backwards until it lands on a Taiwan trading day (skipping weekends AND
// public holidays — P1-8: the old helper only skipped weekends, so a
// Tuesday-after-holiday-Monday lookup returned the holiday Monday).
func PreviousTradingDay(now time.Time, daysBack int) time.Time {
	d := now.AddDate(0, 0, -daysBack)
	for !IsTradingDay(d) {
		d = d.AddDate(0, 0, -1)
	}
	return d
}
