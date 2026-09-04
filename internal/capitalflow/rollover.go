package capitalflow

// TAIFEX monthly-contract rollover window marker — H-CF-01 data hygiene
// (arbitration report .omo/plans/2026-09-04-hcf01-arbitration.md §1.3/S4,
// 2026-09-04).
//
// TX (臺股期貨) monthly contracts settle on the third Wednesday of the month.
// TAIFEX rule (previously wrong here): when that Wednesday is a non-trading
// day, the expiry falls back to the PREVIOUS trading day (取前一交易日) — the
// window is NOT shifted forward. The rollover window = expiry day + the next
// trading day; ΔOI inside the window is dominated by 轉倉 (contract rolling),
// so lag-sensitive procedures (H-CF-01 v1/v2 family) must exclude it.
//
// Empirical context: the rollover window covers ~9.3% of paired days and
// inflates |ΔOI| by ~25% on those days (R1 §A3), though removing it barely
// moves headline hit rates — it is a hygiene item, not a root cause.

import "time"

// DefaultWeekendHoliday reports non-trading days when no richer calendar is
// available: Saturdays and Sundays. Callers holding a holiday calendar
// should wrap it so weekends are also reported as holidays.
func DefaultWeekdayHoliday(t time.Time) bool {
	return t.Weekday() == time.Saturday || t.Weekday() == time.Sunday
}

// TAIFEXMonthlyExpiry returns the settlement date of the monthly TX contract:
// the third Wednesday of (year, month); if that Wednesday is a non-trading
// day per isHoliday, the PREVIOUS trading day (取前一交易日, TAIFEX rule).
func TAIFEXMonthlyExpiry(year int, month time.Month, isHoliday func(time.Time) bool) time.Time {
	if isHoliday == nil {
		isHoliday = DefaultWeekdayHoliday
	}
	expiry := nthWeekdayOfMonth(year, month, time.Wednesday, 3)
	for isHoliday(expiry) {
		expiry = expiry.AddDate(0, 0, -1)
	}
	return expiry
}

// IsRolloverWindow reports whether date falls in a monthly rollover window:
// a settlement day, or the next TRADING day after one. isHoliday must report
// non-trading days (weekends + market holidays); nil defaults to weekends.
func IsRolloverWindow(date time.Time, isHoliday func(time.Time) bool) bool {
	if isHoliday == nil {
		isHoliday = DefaultWeekdayHoliday
	}
	d := date
	if isHoliday(d) {
		return false
	}
	// Candidate expiries: this month's settlement and the previous month's
	// (a settlement early in the month can have its fallback reach back).
	prev := time.Date(d.Year(), d.Month()-1, 1, 0, 0, 0, 0, time.UTC)
	for _, e := range []time.Time{
		TAIFEXMonthlyExpiry(d.Year(), d.Month(), isHoliday),
		TAIFEXMonthlyExpiry(prev.Year(), prev.Month(), isHoliday),
	} {
		if e.After(d) {
			continue
		}
		if d.Equal(e) {
			return true
		}
		allHolidayBetween := true
		for x := e.AddDate(0, 0, 1); x.Before(d); x = x.AddDate(0, 0, 1) {
			if !isHoliday(x) {
				allHolidayBetween = false
				break
			}
		}
		if allHolidayBetween {
			return true // d is the next trading day after expiry
		}
	}
	return false
}

// RolloverWindowDates returns the set (keyed "YYYY-MM-DD") of rollover-window
// days — monthly settlement day + next trading day — for every month whose
// settlement may intersect [start, end]. Use it to tag OI snapshots whose
// ΔOI jump is contract rolling rather than positioning (換月跳動標註).
func RolloverWindowDates(start, end time.Time, isHoliday func(time.Time) bool) map[string]bool {
	if isHoliday == nil {
		isHoliday = DefaultWeekdayHoliday
	}
	out := map[string]bool{}
	// Scan from the month before start: a settlement's holiday fallback and
	// its next-trading-day can still land inside [start, end].
	cur := time.Date(start.Year(), start.Month()-1, 1, 0, 0, 0, 0, start.Location())
	for !cur.After(end) {
		expiry := TAIFEXMonthlyExpiry(cur.Year(), cur.Month(), isHoliday)
		if !expiry.Before(start) && !expiry.After(end) {
			out[expiry.Format("2006-01-02")] = true
		}
		for next := expiry.AddDate(0, 0, 1); next.Before(expiry.AddDate(0, 0, 10)); next = next.AddDate(0, 0, 1) {
			if isHoliday(next) {
				continue
			}
			if !next.Before(start) && !next.After(end) {
				out[next.Format("2006-01-02")] = true
			}
			break
		}
		cur = cur.AddDate(0, 1, 0)
	}
	return out
}

// nthWeekdayOfMonth returns the nth occurrence of weekday in (year, month).
func nthWeekdayOfMonth(year int, month time.Month, weekday time.Weekday, n int) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	offset := (int(weekday) - int(first.Weekday()) + 7) % 7
	return first.AddDate(0, 0, offset+(n-1)*7)
}
