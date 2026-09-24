// Package stockpicker — per-symbol flow-file freshness contract (issue #1945).
//
// data/state/stock_flows/<symbol>.json is the per-symbol foreign-flow store
// (written by cmd/backfill-stockpicker-flows) that two consumers read:
//
//  1. the panel backtest (real_panel.go → conditions.go evalForeign3DNetBuy),
//     which turns triggers into stock_signal_outcomes rows and therefore into
//     the persisted win rates; and
//  2. the decision-path gate (internal/orchestrator StockpickerWinrateExecutor),
//     which reads the newest FlowPoint as "the latest foreign net flow".
//
// Both consumers only look at points dated <= t, so a frozen file is never a
// point-in-time violation — but it does let a stale window masquerade as a
// fresh one for every later date. The file stopped being refreshed after
// 2026-08-27 while these consumers kept running, so both call the single
// freshness predicate below instead of trusting the file blindly.
package stockpicker

import "time"

const (
	// DefaultMaxFlowAgeDays is the fallback freshness limit in calendar days
	// for a per-symbol flow point: data older than this cannot back a trigger
	// or a gate decision. 7 days covers a weekend plus the Taiwan holiday
	// blocks while still failing closed on a feed that died.
	DefaultMaxFlowAgeDays = 7

	// flowDateLayout is the on-disk date layout of FlowPoint.Date.
	flowDateLayout = "2006-01-02"
)

// FlowDate parses a flow-file date (YYYY-MM-DD) into a UTC midnight instant.
// ok is false for an empty or malformed date.
func FlowDate(date string) (time.Time, bool) {
	if date == "" {
		return time.Time{}, false
	}
	d, err := time.Parse(flowDateLayout, date)
	if err != nil {
		return time.Time{}, false
	}
	return d, true
}

// FlowStale reports whether a reading dated `date` (YYYY-MM-DD, the newest
// flow point available) is too old to back an evaluation dated asOf. An
// empty or malformed date is stale (fail closed). maxAgeDays <= 0 falls back
// to DefaultMaxFlowAgeDays.
func FlowStale(date string, asOf time.Time, maxAgeDays int) bool {
	maxAge := maxAgeDays
	if maxAge <= 0 {
		maxAge = DefaultMaxFlowAgeDays
	}
	d, ok := FlowDate(date)
	if !ok {
		return true
	}
	return dateOnlyUTC(asOf).Sub(d) > time.Duration(maxAge)*24*time.Hour
}

// dateOnlyUTC truncates t to its calendar date at UTC midnight, matching the
// UTC-midnight instants FlowDate returns (the repo's session-date convention).
func dateOnlyUTC(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
