package marketdata

import (
	"testing"
	"time"
)

func TestPreviousTradingDay_Weekday(t *testing.T) {
	// Thursday → Wednesday
	got := PreviousTradingDay(time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC), 1)
	if got.Format("2006-01-02") != "2026-07-23" {
		t.Errorf("got %s, want 2026-07-23", got.Format("2006-01-02"))
	}
}

func TestPreviousTradingDay_SaturdayRollsToFriday(t *testing.T) {
	got := PreviousTradingDay(time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC), 1)
	if got.Format("2006-01-02") != "2026-07-24" {
		t.Errorf("got %s, want 2026-07-24", got.Format("2006-01-02"))
	}
}

func TestPreviousTradingDay_SundayRollsToFriday(t *testing.T) {
	got := PreviousTradingDay(time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC), 1)
	if got.Format("2006-01-02") != "2026-07-24" {
		t.Errorf("got %s, want 2026-07-24", got.Format("2006-01-02"))
	}
}

func TestPreviousTradingDay_MultiDay(t *testing.T) {
	// Monday with daysBack=7 → previous Monday
	got := PreviousTradingDay(time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC), 7)
	if got.Format("2006-01-02") != "2026-07-20" {
		t.Errorf("got %s, want 2026-07-20", got.Format("2006-01-02"))
	}
}

func TestPreviousTradingDay_NoWeekend(t *testing.T) {
	got := PreviousTradingDay(time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC), 1)
	if got.Weekday() == time.Saturday || got.Weekday() == time.Sunday {
		t.Errorf("returned day is weekend: %s", got.Weekday())
	}
}
