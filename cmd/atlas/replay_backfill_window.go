package main

// Replay backfill window arithmetic — canonical time zone: **Asia/Taipei**.
//
// FU-20260927-09 (pre-existing, registered 2026-09-27, fixed 2026-10-16):
// getLatestReplayDate() parses the CSV "Date" column with
// time.Parse("2006-01-02", …), so the value lands at 00:00 **UTC**, while the
// window's `end` was built from a **Asia/Taipei** midnight. Comparing those two
// instants is off by exactly 8h, and the single-day case
//
//	start = csv_latest + 1 trading day  ==  end
//
// flipped to "gap: none": the log printed a csv_latest that contradicted its own
// target= field and that trading day was not fetched until the CSV was two days
// behind.
//
// Rule for this package: a *date* of the Taiwan trading calendar is represented
// as a midnight **Asia/Taipei** time.Time. Date-only parsers keep returning the
// raw time.Parse result (UTC midnight — for a date-only value the instant is
// meaningless); every comparison first normalizes through recordTaipeiDate(),
// which re-derives the calendar date in Asia/Taipei, so the representation a
// parser happened to choose cannot shift a boundary.
//
// The CSV→JSONL freshness gate is a different comparison and is intentionally
// out of scope: both of its sides go through internal/replay.GetLatestDate and
// it compares *data date strings*, not instants (replayFreshness.conversionNeeded),
// so it has no time-zone phase error.

import "time"

// replayDateZoneName is the trading-calendar zone used by every date in this
// file. It is named in the logs so the phase is auditable from a single line.
const replayDateZoneName = "Asia/Taipei"

// recordTaipeiDate normalizes any time to the midnight that starts its
// Asia/Taipei calendar date. Use it before comparing a date-only value with a
// window boundary.
func recordTaipeiDate(t time.Time) time.Time {
	loc := taipeiLocation()
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

// nextTradingDate rolls d forward off Saturday/Sunday onto the next weekday.
func nextTradingDate(d time.Time) time.Time {
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, 1)
	}
	return d
}

// replayBackfillWindow returns the inclusive [start, end] fetch window as
// Asia/Taipei midnights, given the CSV's newest data date and the current time.
//
//   - end   = the last settled trading day as of now (15:30 Taipei cutoff and
//     weekend rollback — delegated to currentTaipeiTradingDate so both callers
//     share one implementation);
//   - start = the trading day after the CSV's newest data date.
//
// start.After(end) therefore means "nothing to fetch" and is a *same-basis*
// comparison: both values are Asia/Taipei midnights.
func replayBackfillWindow(latestRecordDate, now time.Time) (start, end time.Time) {
	end = currentTaipeiTradingDate(now)
	start = nextTradingDate(recordTaipeiDate(latestRecordDate).AddDate(0, 0, 1))
	return start, end
}
