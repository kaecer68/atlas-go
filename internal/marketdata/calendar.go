// Package marketdata provides shared calendar utilities used by market data
// providers and channel adapters. These are intentionally simple: TW/US holiday
// detection is left to upstream data sources (Yahoo Finance already returns the
// last close containing non-zero data; TWSE returns empty when market is closed).
//
// P1-8: the Taiwan holiday tables (fixed + lunar 2023-2030) previously lived
// here AND in industry/event_calendar.go as two copies that could drift. The
// canonical tables now live in internal/taiwanholidays — this file delegates,
// and industry/event_calendar.go sources its lunar dates from the same package.
package marketdata

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/taiwanholidays"
)

// PreviousTradingDay rolls back daysBack calendar days, then walks backwards
// until it lands on a Taiwan trading day (skipping weekends AND public
// holidays — P1-8: previously only weekends were skipped, so a holiday
// Tuesday lookup returned the holiday Monday, which hit
// adapter_government_broker in production).
//
// This is used by providers that need a "most recent business day" date for
// constructing API URLs or checking file freshness.
func PreviousTradingDay(now time.Time, daysBack int) time.Time {
	return taiwanholidays.PreviousTradingDay(now, daysBack)
}

// isTaiwanHoliday reports whether t falls on a Taiwan public holiday
// (fixed-date or lunar/seasonal). P1-8: delegated to the single-source
// internal/taiwanholidays package (previously a local copy of the lunar
// tables with a sync obligation to industry/event_calendar.go).
func isTaiwanHoliday(t time.Time) bool {
	return taiwanholidays.IsHoliday(t)
}

// isTaiwanTradingDay reports whether t is a Taiwan trading day:
// a weekday that is not a public holiday. tw_vol freshness、FinMind provider、
// taiex fallback 與 taiwan_index_cache 使用。
func isTaiwanTradingDay(t time.Time) bool {
	return taiwanholidays.IsTradingDay(t)
}

// IsTaiwanTradingDay 是 isTaiwanTradingDay 的 exported 版本，供 stocktools、
// dailyreport 等外部 package 使用（manifest Phase C — quote/technical 路徑
// 非交易日標記）。呼叫端應傳入台灣本地時間（時區只影響 weekday 判定，
// 日期部分與 taiwanholidays 表一致）。
func IsTaiwanTradingDay(t time.Time) bool {
	return taiwanholidays.IsTradingDay(t)
}

// RecentTradingDays returns up to n most recent expected Taiwan trading days
// (inclusive of now's own day when that is a trading day), most recent first.
// Data-source scan loops use this instead of blind calendar-day walks so
// weekend/holiday queries never burn rate-limiter tokens (#1767).
func RecentTradingDays(now time.Time, n int) []time.Time {
	days := make([]time.Time, 0, n)
	day := taiwanholidays.PreviousTradingDay(now, 0)
	for len(days) < n {
		days = append(days, day)
		day = taiwanholidays.PreviousTradingDay(day.AddDate(0, 0, -1), 0)
	}
	return days
}
