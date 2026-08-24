package marketdata

import (
	"context"
	"testing"
	"time"
)

func TestMSCIRebalanceCalendarProvider_Name(t *testing.T) {
	p := NewMSCIRebalanceCalendarProvider()
	if p.Name() != "msci_static" {
		t.Fatalf("unexpected name: %s", p.Name())
	}
}

func TestMSCIRebalanceCalendarProvider_FetchEventsByYear(t *testing.T) {
	p := NewMSCIRebalanceCalendarProvider()

	// 2023: 4 quarterly reviews
	events, err := p.FetchEvents(context.Background(), 2023)
	if err != nil {
		t.Fatalf("FetchEvents(2023): %v", err)
	}
	if len(events) != 4 {
		t.Fatalf("expected 4 MSCI events for 2023, got %d", len(events))
	}
	wantDates := []string{"2023-02-24", "2023-05-31", "2023-08-31", "2023-11-30"}
	for i, want := range wantDates {
		if events[i].Date != want {
			t.Errorf("2023 event[%d] date = %s, want %s", i, events[i].Date, want)
		}
		if events[i].EventType != "msci_rebalance" {
			t.Errorf("2023 event[%d] type = %s, want msci_rebalance", i, events[i].EventType)
		}
		if events[i].Source != "msci_static" {
			t.Errorf("2023 event[%d] source = %s, want msci_static", i, events[i].Source)
		}
		if events[i].Weight != 0.9 {
			t.Errorf("2023 event[%d] weight = %v, want 0.9 (config msci_rebalance base)", i, events[i].Weight)
		}
	}

	// 2026: 4 reviews (including the 2026-11-30 scheduled entry)
	events26, err := p.FetchEvents(context.Background(), 2026)
	if err != nil {
		t.Fatalf("FetchEvents(2026): %v", err)
	}
	if len(events26) != 4 {
		t.Fatalf("expected 4 MSCI events for 2026, got %d", len(events26))
	}
	if events26[2].Date != "2026-08-31" {
		t.Errorf("2026-08 effective date = %s, want 2026-08-31", events26[2].Date)
	}
}

func TestMSCIRebalanceCalendarProvider_FetchEventsAllYears(t *testing.T) {
	p := NewMSCIRebalanceCalendarProvider()
	total := 0
	for year := 2022; year <= 2027; year++ {
		events, err := p.FetchEvents(context.Background(), year)
		if err != nil {
			t.Fatalf("FetchEvents(%d): %v", year, err)
		}
		switch year {
		case 2022, 2027:
			if len(events) != 0 {
				t.Errorf("year %d: expected 0 events (outside static table), got %d", year, len(events))
			}
		default:
			if len(events) != 4 {
				t.Errorf("year %d: expected 4 events, got %d", year, len(events))
			}
			total += len(events)
		}
	}
	if total != 16 {
		t.Errorf("expected 16 static MSCI events (2023-2026), got %d", total)
	}
}

func TestMSCIRebalanceCalendarProvider_EffectiveDatesAreLastBusinessDays(t *testing.T) {
	// Every static effective date must be a weekday (the last TWSE business
	// day of its month). This guards against accidental weekend dates.
	p := NewMSCIRebalanceCalendarProvider()
	for year := 2023; year <= 2026; year++ {
		events, err := p.FetchEvents(context.Background(), year)
		if err != nil {
			t.Fatalf("FetchEvents(%d): %v", year, err)
		}
		for _, e := range events {
			day, err := time.Parse("2006-01-02", e.Date)
			if err != nil {
				t.Fatalf("invalid date %s: %v", e.Date, err)
			}
			if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
				t.Errorf("MSCI effective date %s falls on a weekend", e.Date)
			}
		}
	}
}
