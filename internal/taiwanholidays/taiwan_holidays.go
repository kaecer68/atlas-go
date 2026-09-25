// Package taiwanholidays is the single source of truth for Taiwan public
// holidays (fixed-date + lunar/seasonal, verified tables — see CoverageYears).
//
// Years outside the verified range are NOT extrapolated with conventional
// placeholder dates: a placeholder is indistinguishable from a real date to
// every consumer, which is how 2021 春節 came to be reported as 02-01 instead of
// the real 02-12 (issue #1973 D3). Out-of-range years omit the moving holidays
// and report unavailability through VerifiedLunarYear / LunarNewYear &c.
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
	"maps"
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

// adjustedHolidays holds ADJUSTED LEAVE days (補假/調整放假) plus any
// non-spring settlement closures — weekday market closures NOT covered by
// fixedHolidays, springFestivalClosures or the lunar tables. Verified against
// the TWSE official 開休市日期 calendar (web, 2026-08-25). Without these,
// e.g. 2026-02-27 (228 補假) was misjudged a trading day and fake quotes
// leaked into replay (2885 Close 788.33 vs 47.25 the day before).
var adjustedHolidays = map[int][]time.Time{
	2023: {
		time.Date(2023, 2, 27, 0, 0, 0, 0, time.UTC), // 和平紀念日補假
		time.Date(2023, 4, 3, 0, 0, 0, 0, time.UTC),  // 兒童節調整放假
		time.Date(2023, 10, 9, 0, 0, 0, 0, time.UTC), // 國慶日補假
	},
	2025: {
		time.Date(2025, 5, 30, 0, 0, 0, 0, time.UTC),  // 端午節補假（5/31 週六）
		time.Date(2025, 9, 29, 0, 0, 0, 0, time.UTC),  // 教師節補假（9/28 週日）
		time.Date(2025, 10, 24, 0, 0, 0, 0, time.UTC), // 光復節補假（10/25 週六）
	},
	2026: {
		time.Date(2026, 2, 27, 0, 0, 0, 0, time.UTC),  // 和平紀念日補假（2/28 週六）
		time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC),   // 民族掃墓節補假（4/5 週日）
		time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC),  // 國慶日補假（10/10 週六）
		time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC), // 光復節補假（10/25 週日）
	},
}

// lunar tables: year → date (verified 2023-2030). This is the canonical
// copy — marketdata/calendar.go and industry/event_calendar.go previously
// each maintained their own duplicate and could drift.
var lunarNewYearDates = map[int]time.Time{
	2021: time.Date(2021, 2, 12, 0, 0, 0, 0, time.UTC), // 春節初一
	2022: time.Date(2022, 2, 1, 0, 0, 0, 0, time.UTC),
	// 2023 was missing from this table although it is inside the declared
	// verified range, so 2023 春節 also fell back to the 02-01 placeholder.
	// 2023-01-22 is the same 初一 the TWSE closure span records below.
	2023: time.Date(2023, 1, 22, 0, 0, 0, 0, time.UTC),
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
	2021: time.Date(2021, 4, 4, 0, 0, 0, 0, time.UTC), // 清明
	2022: time.Date(2022, 4, 5, 0, 0, 0, 0, time.UTC),
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
	2021: time.Date(2021, 6, 14, 0, 0, 0, 0, time.UTC), // 端午
	2022: time.Date(2022, 6, 3, 0, 0, 0, 0, time.UTC),
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
	2021: time.Date(2021, 9, 21, 0, 0, 0, 0, time.UTC), // 中秋
	2022: time.Date(2022, 9, 10, 0, 0, 0, 0, time.UTC),
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

// CoverageYears returns the verified lunar-table range. Every year inside it has
// all four moving holidays (春節/清明/端午/中秋) in the tables — see
// VerifiedLunarYear, and the completeness test that pins it. Years outside it
// are "not determinable" and yield no moving-holiday date at all.
//
// 2023-2030 values came from the pre-existing marketdata/industry tables; the
// 2031-2040 block was computed and cross-validated (lunardate library + ephem
// solar-term astronomy) against the 2023-2030 reference on 2026-08-23.
// 2021-2022 were added on 2026-09-25 (issue #1973 D3): they are the earliest
// years the platform's own TWSE session universe covers, so the lunar dates can
// be checked against first-party market closures, not just computed.
func CoverageYears() (int, int) { return 2021, 2040 }

// VerifiedLunarYear reports whether every moving Taiwan holiday
// (春節/清明/端午/中秋) has a verified entry for year, i.e. whether the lunar
// calendar is determinable for that year. Callers that need a complete holiday
// picture MUST consult this (or CoverageYears) and treat false as "not
// determinable" rather than "no holiday happened".
func VerifiedLunarYear(year int) bool {
	if _, ok := lunarNewYearDates[year]; !ok {
		return false
	}
	if _, ok := tombSweepingDates[year]; !ok {
		return false
	}
	if _, ok := lunarDragonBoatDates[year]; !ok {
		return false
	}
	if _, ok := lunarMidAutumnDates[year]; !ok {
		return false
	}
	return true
}

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
	maps.Copy(out, in)
	return out
}

// lunarUnavailableWarned tracks years for which the unavailability of the lunar
// tables has already been logged, so the log does not spam every call.
var lunarUnavailableWarned sync.Map

// warnLunarUnavailable logs once per out-of-coverage year that the moving
// holidays are not determinable. It deliberately does NOT substitute a
// conventional date: issue #1973 D3 traced a wrong 春節 (02-01 for the real
// 02-12) to exactly that substitution.
func warnLunarUnavailable(year int) {
	if _, loaded := lunarUnavailableWarned.LoadOrStore(year, true); loaded {
		return
	}
	minYear, maxYear := CoverageYears()
	logging.Warn("taiwanholidays", "lunar_unavailable",
		"year", year,
		"verified_from", minYear,
		"verified_to", maxYear,
		"note", "no verified lunar table entry for this year; moving holidays are omitted instead of reported with a placeholder date — extend the lunar tables")
}

// lunarDatesInYear returns the moving (lunar/seasonal) holidays for year that
// the verified tables can determine. A holiday whose year is outside the
// verified range is omitted (with a one-time warning) rather than reported with
// a conventional placeholder date, so no caller can mistake a placeholder for a
// real date.
func lunarDatesInYear(year int) []Holiday {
	holidays := make([]Holiday, 0, 4)
	unavailable := false
	if closures, ok := springFestivalClosures[year]; ok {
		// Full TWSE closure span (settlement days + 除夕~初五 + 補假),
		// verified against the official calendar — see springFestivalClosures.
		for _, d := range closures {
			holidays = append(holidays, Holiday{Name: "春節休市", Date: d})
		}
	} else if d, ok := lunarNewYearDates[year]; ok {
		// 初一 only (years without a verified closure span): a weekday closure
		// inside the spring window cannot be derived from the lunar date alone.
		holidays = append(holidays, Holiday{Name: "春節", Date: d})
	} else {
		unavailable = true
	}
	if d, ok := tombSweepingDates[year]; ok {
		holidays = append(holidays, Holiday{Name: "清明節", Date: d})
	} else {
		unavailable = true
	}
	if d, ok := lunarDragonBoatDates[year]; ok {
		holidays = append(holidays, Holiday{Name: "端午節", Date: d})
	} else {
		unavailable = true
	}
	if d, ok := lunarMidAutumnDates[year]; ok {
		holidays = append(holidays, Holiday{Name: "中秋節", Date: d})
	} else {
		unavailable = true
	}
	if unavailable {
		warnLunarUnavailable(year)
	}
	return holidays
}

// HolidaysInYear returns every Taiwan public holiday for year that the
// platform can determine (fixed-date plus the verified lunar/seasonal ones),
// sorted by date. For a year outside CoverageYears only the fixed-date holidays
// are returned and a one-time warning is logged: the moving holidays are then
// NOT determinable, so omitting them is the honest answer (issue #1973 D3).
func HolidaysInYear(year int) []Holiday {
	holidays := make([]Holiday, 0, 8)
	for _, f := range fixedHolidays {
		holidays = append(holidays, Holiday{Name: f.Name, Date: time.Date(year, f.Month, f.Day, 0, 0, 0, 0, time.UTC)})
	}
	holidays = append(holidays, lunarDatesInYear(year)...)
	if adj, ok := adjustedHolidays[year]; ok {
		for _, d := range adj {
			holidays = append(holidays, Holiday{Name: "調整放假", Date: d})
		}
	}
	sort.Slice(holidays, func(i, j int) bool { return holidays[i].Date.Before(holidays[j].Date) })
	return holidays
}

// IsHoliday reports whether t falls on a Taiwan public holiday. The check is
// calendar-date based (UTC normalized) — the input's time-of-day and zone are
// irrelevant, matching the pre-existing marketdata semantics.
func IsHoliday(t time.Time) bool {
	t = t.In(twLocation)
	year := t.Year()
	if !VerifiedLunarYear(year) {
		// The moving holidays cannot be judged for this year: warn once instead
		// of silently answering "not a holiday" for a date that may well be one.
		warnLunarUnavailable(year)
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
