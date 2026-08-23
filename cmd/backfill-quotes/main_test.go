package main

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func bar(date string, close float64) domain.DailyBar {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		panic(err)
	}
	return domain.DailyBar{Date: d, Close: close, Symbol: "2330.TW"}
}

func TestFilterTradingDays(t *testing.T) {
	input := []domain.DailyBar{
		bar("2026-07-01", 100), // Wed
		bar("2026-07-02", 101), // Thu
		bar("2026-07-03", 102), // Fri
		bar("2026-07-04", 103), // Sat — weekend, skip
		bar("2026-07-05", 104), // Sun — weekend, skip
		bar("2026-07-06", 105), // Mon
	}
	kept, skipped := filterTradingDays(input)
	if skipped != 2 {
		t.Errorf("expected 2 skipped weekend rows, got %d", skipped)
	}
	if len(kept) != 4 {
		t.Fatalf("expected 4 kept bars, got %d", len(kept))
	}
	for i, want := range []string{"2026-07-01", "2026-07-02", "2026-07-03", "2026-07-06"} {
		if got := kept[i].Date.Format("2006-01-02"); got != want {
			t.Errorf("kept[%d] date = %s, want %s", i, got, want)
		}
	}
}

func TestFilterTradingDays_TaiwanHoliday(t *testing.T) {
	input := []domain.DailyBar{
		bar("2026-02-16", 200), // Mon (spring festival eve, listed in table only for 02-17)
		bar("2026-02-17", 201), // Tue — 春節初一, Taiwan holiday, skip
		bar("2026-02-18", 202), // Wed
	}
	kept, skipped := filterTradingDays(input)
	if skipped != 1 {
		t.Errorf("expected 1 skipped holiday row, got %d", skipped)
	}
	if len(kept) != 2 {
		t.Fatalf("expected 2 kept bars, got %d", len(kept))
	}
	if got := kept[0].Date.Format("2006-01-02"); got != "2026-02-16" {
		t.Errorf("kept[0] date = %s, want 2026-02-16", got)
	}
	if got := kept[1].Date.Format("2006-01-02"); got != "2026-02-18" {
		t.Errorf("kept[1] date = %s, want 2026-02-18", got)
	}
}

func TestFilterTradingDays_Empty(t *testing.T) {
	kept, skipped := filterTradingDays(nil)
	if len(kept) != 0 || skipped != 0 {
		t.Errorf("expected empty result, got kept=%d skipped=%d", len(kept), skipped)
	}
}
