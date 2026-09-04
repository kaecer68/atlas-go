package capitalflow

import (
	"testing"
	"time"
)

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse %s: %v", s, d)
	}
	return d
}

func TestTAIFEXMonthlyExpiry_ThirdWednesday(t *testing.T) {
	// 2026-01 Wednesdays: 7, 14, 21, 28 → 3rd = 2026-01-21 (R1 rollover anchor).
	got := TAIFEXMonthlyExpiry(2026, time.January, DefaultWeekdayHoliday)
	if !got.Equal(mustDate(t, "2026-01-21")) {
		t.Fatalf("expiry = %s, want 2026-01-21", got.Format("2006-01-02"))
	}
}

func TestTAIFEXMonthlyExpiry_HolidayFallsBackToPreviousTradingDay(t *testing.T) {
	// 2026-06 Wednesdays: 3, 10, 17, 24 → 3rd = 2026-06-17. If 06-17 is a
	// holiday, TAIFEX takes the PREVIOUS trading day (06-16), not the next.
	holidays := map[string]bool{"2026-06-17": true}
	isHoliday := func(d time.Time) bool {
		return DefaultWeekdayHoliday(d) || holidays[d.Format("2006-01-02")]
	}
	got := TAIFEXMonthlyExpiry(2026, time.June, isHoliday)
	if !got.Equal(mustDate(t, "2026-06-16")) {
		t.Fatalf("expiry = %s, want 2026-06-16 (previous trading day)", got.Format("2006-01-02"))
	}
}

func TestTAIFEXMonthlyExpiry_MultiDayHolidayFallback(t *testing.T) {
	// 3rd Wed + the day before both holidays → walk back to Tuesday-1.
	holidays := map[string]bool{"2026-06-17": true, "2026-06-16": true}
	isHoliday := func(d time.Time) bool {
		return DefaultWeekdayHoliday(d) || holidays[d.Format("2006-01-02")]
	}
	got := TAIFEXMonthlyExpiry(2026, time.June, isHoliday)
	if !got.Equal(mustDate(t, "2026-06-15")) {
		t.Fatalf("expiry = %s, want 2026-06-15", got.Format("2006-01-02"))
	}
}

func TestIsRolloverWindow_MarksExpiryAndNextTradingDay(t *testing.T) {
	isHoliday := DefaultWeekdayHoliday
	cases := map[string]bool{
		"2026-01-21": true,  // settlement day
		"2026-01-22": true,  // next trading day
		"2026-01-20": false, // day before
		"2026-01-23": false, // two days after
		"2026-01-24": false, // Saturday (weekend is never a window day)
	}
	for d, want := range cases {
		if got := IsRolloverWindow(mustDate(t, d), isHoliday); got != want {
			t.Errorf("IsRolloverWindow(%s) = %v, want %v", d, got, want)
		}
	}
}

func TestIsRolloverWindow_HolidayShiftedWindow(t *testing.T) {
	holidays := map[string]bool{"2026-06-17": true, "2026-06-18": true}
	isHoliday := func(d time.Time) bool {
		return DefaultWeekdayHoliday(d) || holidays[d.Format("2006-01-02")]
	}
	cases := map[string]bool{
		"2026-06-16": true,  // fallback settlement (prev trading day)
		"2026-06-19": true,  // next trading day after the fallback expiry
		"2026-06-17": false, // holiday itself is never marked
		"2026-06-18": false,
		"2026-06-22": false, // two trading days after
	}
	for d, want := range cases {
		if got := IsRolloverWindow(mustDate(t, d), isHoliday); got != want {
			t.Errorf("IsRolloverWindow(%s) = %v, want %v", d, got, want)
		}
	}
}

func TestRolloverWindowDates(t *testing.T) {
	start := mustDate(t, "2026-01-01")
	end := mustDate(t, "2026-02-28")
	got := RolloverWindowDates(start, end, DefaultWeekdayHoliday)
	// 2026-01 settlement 21 + next trading day 22; 2026-02 Wednesdays:
	// 4, 11, 18, 25 → settlement 18 + next trading day 19.
	want := []string{"2026-01-21", "2026-01-22", "2026-02-18", "2026-02-19"}
	if len(got) != len(want) {
		t.Fatalf("RolloverWindowDates = %v, want %v", got, want)
	}
	for _, d := range want {
		if !got[d] {
			t.Errorf("missing rollover date %s in %v", d, got)
		}
	}
}
