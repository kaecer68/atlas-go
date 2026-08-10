// Package marketdata provides shared calendar utilities used by market data
// providers and channel adapters. These are intentionally simple: TW/US holiday
// detection is left to upstream data sources (Yahoo Finance already returns the
// last close containing non-zero data; TWSE returns empty when market is closed).
package marketdata

import "time"

// PreviousTradingDay rolls back daysBack calendar days, then skips weekends
// to land on the most recent weekday. For example, if today is Monday,
// PreviousTradingDay(now, 1) returns the previous Friday.
//
// This is used by providers that need a "most recent business day" date for
// constructing API URLs or checking file freshness.
func PreviousTradingDay(now time.Time, daysBack int) time.Time {
	d := now.AddDate(0, 0, -daysBack)
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, -1)
	}
	return d
}

// ─── Taiwan trading calendar ────────────────────────────────────────────────
//
// B05（2026-08-10 audit）：isTaiwanTradingDay 原本只排除週末，國定假日被當成
// 交易日 → tw_vol freshness 在假日誤判 stale、FinMind GetQuotes 假日誤發請求。
// 此處加入台灣國定假日判定（固定日期 + 農曆/節氣）。
//
// ⚠️ 同步義務：lunar 日期表與 internal/industry/event_calendar.go 的
// taiwanPublicHolidays / lunarNewYearDates / lunarDragonBoatDates /
// lunarMidAutumnDates / tombSweepingDates 對齊（2023-2030）。兩 package 各自
// 維護（marketdata 不能 import industry，避免 import cycle）。更新一側需同步
// 另一側；超出 2030 年需擴充 lunar 表（fallback 為慣例日期）。

// taiwanFixedHoliday 是固定日期（月/日）的台灣國定假日。
type taiwanFixedHoliday struct {
	month time.Month
	day   int
}

var taiwanFixedHolidays = []taiwanFixedHoliday{
	{time.January, 1},   // 元旦
	{time.February, 28}, // 228 和平紀念日
	{time.May, 1},       // 勞動節
	{time.October, 10},  // 國慶日
}

// taiwanLunarHolidays 是農曆/節氣假日（年份 → 日期列表）。
// 與 industry/event_calendar.go 的 lunar 表同步（2023-2030）。
var taiwanLunarHolidays = map[int][]time.Time{
	2023: {
		time.Date(2023, 4, 5, 0, 0, 0, 0, time.UTC),  // 清明
		time.Date(2023, 6, 22, 0, 0, 0, 0, time.UTC), // 端午
		time.Date(2023, 9, 29, 0, 0, 0, 0, time.UTC), // 中秋
	},
	2024: {
		time.Date(2024, 2, 10, 0, 0, 0, 0, time.UTC), // 春節初一
		time.Date(2024, 4, 4, 0, 0, 0, 0, time.UTC),  // 清明
		time.Date(2024, 6, 10, 0, 0, 0, 0, time.UTC), // 端午
		time.Date(2024, 9, 17, 0, 0, 0, 0, time.UTC), // 中秋
	},
	2025: {
		time.Date(2025, 1, 29, 0, 0, 0, 0, time.UTC), // 春節初一
		time.Date(2025, 4, 4, 0, 0, 0, 0, time.UTC),  // 清明
		time.Date(2025, 5, 31, 0, 0, 0, 0, time.UTC), // 端午
		time.Date(2025, 10, 6, 0, 0, 0, 0, time.UTC), // 中秋
	},
	2026: {
		time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC), // 春節初一
		time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC),  // 清明
		time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC), // 端午
		time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC), // 中秋
	},
	2027: {
		time.Date(2027, 2, 6, 0, 0, 0, 0, time.UTC),  // 春節初一
		time.Date(2027, 4, 5, 0, 0, 0, 0, time.UTC),  // 清明
		time.Date(2027, 6, 9, 0, 0, 0, 0, time.UTC),  // 端午
		time.Date(2027, 9, 15, 0, 0, 0, 0, time.UTC), // 中秋
	},
	2028: {
		time.Date(2028, 1, 26, 0, 0, 0, 0, time.UTC), // 春節初一
		time.Date(2028, 4, 4, 0, 0, 0, 0, time.UTC),  // 清明
		time.Date(2028, 5, 28, 0, 0, 0, 0, time.UTC), // 端午
		time.Date(2028, 10, 3, 0, 0, 0, 0, time.UTC), // 中秋
	},
	2029: {
		time.Date(2029, 2, 13, 0, 0, 0, 0, time.UTC), // 春節初一
		time.Date(2029, 4, 4, 0, 0, 0, 0, time.UTC),  // 清明
		time.Date(2029, 6, 16, 0, 0, 0, 0, time.UTC), // 端午
		time.Date(2029, 9, 22, 0, 0, 0, 0, time.UTC), // 中秋
	},
	2030: {
		time.Date(2030, 2, 3, 0, 0, 0, 0, time.UTC),  // 春節初一
		time.Date(2030, 4, 5, 0, 0, 0, 0, time.UTC),  // 清明
		time.Date(2030, 6, 5, 0, 0, 0, 0, time.UTC),  // 端午
		time.Date(2030, 9, 12, 0, 0, 0, 0, time.UTC), // 中秋
	},
}

// isTaiwanHoliday reports whether t falls on a Taiwan public holiday
// (fixed-date or lunar/seasonal). Timezone-independent after normalizing
// to twseLocation.
func isTaiwanHoliday(t time.Time) bool {
	local := t.In(twseLocation)
	for _, h := range taiwanFixedHolidays {
		if local.Month() == h.month && local.Day() == h.day {
			return true
		}
	}
	for _, d := range taiwanLunarHolidays[local.Year()] {
		dLocal := d.In(twseLocation)
		if dLocal.Month() == local.Month() && dLocal.Day() == local.Day() {
			return true
		}
	}
	return false
}

// isTaiwanTradingDay reports whether t is a Taiwan trading day:
// a weekday that is not a public holiday.
//
// B05：原本只排除週末；國定假日（春節/清明/端午/中秋/228/勞動節/國慶/元旦）
// 現在也會被排除，使 tw_vol freshness 與 FinMind GetQuotes 在假日正確回退，
// 不再把假日當交易日誤判 stale。
func isTaiwanTradingDay(t time.Time) bool {
	w := t.Weekday()
	if w == time.Saturday || w == time.Sunday {
		return false
	}
	return !isTaiwanHoliday(t)
}
