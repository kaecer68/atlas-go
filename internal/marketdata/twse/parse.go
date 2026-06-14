// Package twse provides utility functions for parsing TWSE (Taiwan Stock Exchange)
// data formats, including numeric and date parsing from string fields.
package twse

import (
	"strconv"
	"strings"
	"time"
)

// ParseFloat parses a string as a float64, trimming whitespace and removing commas.
// Returns 0 for empty strings, "--", or "-".
func ParseFloat(s string) float64 {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" || s == "--" || s == "-" {
		return 0
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// ParseInt64 parses a string as an int64, trimming whitespace and removing commas.
// Returns 0 for empty strings, "--", or "-".
func ParseInt64(s string) int64 {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if s == "" || s == "--" || s == "-" {
		return 0
	}
	i, _ := strconv.ParseInt(s, 10, 64)
	return i
}

// ParseDate parses a date string in YYYY-MM-DD format and returns the time.Time value.
func ParseDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", s)
}

// TradingDates returns all weekday (Mon-Fri) dates from start to end inclusive.
// Weekends are filtered out. Caller is responsible for holiday calendar.
func TradingDates(start, end time.Time) []time.Time {
	var dates []time.Time
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if d.Weekday() != time.Saturday && d.Weekday() != time.Sunday {
			dates = append(dates, d)
		}
	}
	return dates
}
