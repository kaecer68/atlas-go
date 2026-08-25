package marketdata

import (
	"context"
	"testing"
)

func TestNationalStabilizationProvider_FetchEvents(t *testing.T) {
	p := NewNationalStabilizationProvider()

	// 2025：進場（2025-04-09）+ 退場（2026-01-12，跨年）
	evs, err := p.FetchEvents(context.Background(), 2025)
	if err != nil {
		t.Fatalf("FetchEvents(2025): %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("2025 events = %d, want 1 (entry only; exit is 2026)", len(evs))
	}
	e := evs[0]
	if e.Date != "2025-04-09" || e.EventType != "national_stabilization" {
		t.Errorf("2025 event = %s/%s, want 2025-04-09/national_stabilization", e.Date, e.EventType)
	}
	if e.Direction != "bullish" || e.Weight < 0.9 {
		t.Errorf("entry event should be high-weight bullish: dir=%s w=%.2f", e.Direction, e.Weight)
	}

	// 2026：退場事件（2026-01-12）
	evs26, err := p.FetchEvents(context.Background(), 2026)
	if err != nil {
		t.Fatalf("FetchEvents(2026): %v", err)
	}
	if len(evs26) != 1 {
		t.Fatalf("2026 events = %d, want 1 (exit)", len(evs26))
	}
	if evs26[0].Date != "2026-01-12" || evs26[0].EventType != "national_stabilization_exit" {
		t.Errorf("2026 event = %s/%s, want 2026-01-12/national_stabilization_exit", evs26[0].Date, evs26[0].EventType)
	}
}

func TestNationalStabilizationProvider_NoEventsBefore2000(t *testing.T) {
	p := NewNationalStabilizationProvider()
	evs, err := p.FetchEvents(context.Background(), 1999)
	if err != nil {
		t.Fatalf("FetchEvents(1999): %v", err)
	}
	if len(evs) != 0 {
		t.Errorf("1999 events = %d, want 0", len(evs))
	}
}

func TestNationalStabilizationProvider_IsInterventionActive(t *testing.T) {
	p := NewNationalStabilizationProvider()
	cases := []struct {
		date string
		want bool
	}{
		{"2025-04-08", false}, // 進場前一日
		{"2025-04-09", true},  // 進場日
		{"2025-08-01", true},  // 護盤中
		{"2026-01-12", true},  // 退場日（含）
		{"2026-01-13", false}, // 退場後
		{"2022-07-12", false}, // 2022 進場前
		{"2022-07-13", true},  // 2022 進場
		{"2023-04-13", true},  // 2022 退場日
		{"2008-12-17", true},  // 2008 退場日
		{"2008-12-18", false}, // 2008 退場後
	}
	for _, c := range cases {
		if got := p.IsInterventionActive(c.date); got != c.want {
			t.Errorf("IsInterventionActive(%s) = %v, want %v", c.date, got, c.want)
		}
	}
}

func TestNationalStabilizationProvider_AllPeriodsValid(t *testing.T) {
	// 所有期間必須 start <= end，且 2018（授權未進場）不得出現
	for _, pr := range nsfInterventionPeriods {
		if pr.start >= pr.end {
			t.Errorf("period %s..%s: start must be < end", pr.start, pr.end)
		}
		if pr.start >= "2018-01-01" && pr.start < "2019-01-01" {
			t.Errorf("2018 authorization (no entry) must not be listed: %s", pr.start)
		}
	}
}
