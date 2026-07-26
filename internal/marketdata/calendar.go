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
