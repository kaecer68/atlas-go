package capitalflow

// Issue #1947 regression tests: the CF-INV-16 trading-day gate must use the
// authoritative holiday table (marketdata.IsTaiwanTradingDay →
// taiwanholidays.IsTradingDay), not industry.EventCalendar.IsTaiwanTradingDay.
//
// Production evidence (2026-09-24, Mac Mini): capital_flow_rolling.json froze
// at mtime 2026-09-22 07:59 with `msg=skip_non_trading_day
// date=2026-09-24 component=capitalflow` every 5 minutes, while TWSE was
// trading (taiex ts=2026-09-24 12:55, market_volume ts=2026-09-23,
// data/state/capital_flow/20260923_capital_flow.json present). 2026 中秋 is
// 09-25, so 09-23 (Wed) and 09-24 (Thu) are ordinary trading days.
//
// Mechanism: industry.EventCalendar.IsTaiwanTradingDay returns false for any
// date inside a long_holiday event window, and buildHolidayEvent defines every
// public holiday as [holiday-3d, holiday+2d] — 2026-09-22..2026-09-27 for
// 09-25. The gate therefore suppressed a week of trading days.

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// taipeiDay parses a Taipei wall-clock instant used as a snapshot RecordedAt.
func taipeiDay(date, hhmm string) time.Time {
	ts, err := time.ParseInLocation("2006-01-02 15:04", date+" "+hhmm, taipeiZone)
	if err != nil {
		panic(err)
	}
	return ts
}

// TestTradingDayTable_Issue1947Dates locks the authoritative judgement for the
// dates the production freeze covered.
func TestTradingDayTable_Issue1947Dates(t *testing.T) {
	cases := []struct {
		date      string
		wantTrade bool
		why       string
	}{
		{"2026-09-22", true, "Tuesday"},
		{"2026-09-23", true, "Wednesday — radar skipped it in production"},
		{"2026-09-24", true, "Thursday — radar skipped it in production"},
		{"2026-09-25", false, "Mid-Autumn Festival (中秋)"},
		{"2026-09-26", false, "Saturday"},
		{"2026-09-27", false, "Sunday"},
		{"2026-09-28", true, "Monday — first trading day after the holiday"},
	}
	for _, c := range cases {
		t.Run(c.date, func(t *testing.T) {
			if got := marketdata.IsTaiwanTradingDay(taipeiDay(c.date, "13:00")); got != c.wantTrade {
				t.Errorf("marketdata.IsTaiwanTradingDay(%s) = %v, want %v (%s)", c.date, got, c.wantTrade, c.why)
			}
		})
	}
}

// TestEventCalendarLongHolidayWindow_Issue1947Premise documents WHY the gate
// was wrong: the event calendar's long_holiday window spans trading days.
//
// If this test ever fails, industry.EventCalendar no longer reports the
// 中秋 window over 09-23/09-24 — the premise of issue #1947 changed and this
// file's comments (and the divergent-gate warning in service.go) should be
// revisited. The behaviour assertions in the other tests stay valid either way.
func TestEventCalendarLongHolidayWindow_Issue1947Premise(t *testing.T) {
	cal := industry.NewEventCalendar()
	cal.RefreshEvents(taipeiDay("2026-09-24", "12:00")) // generate 2026 events

	for _, date := range []string{"2026-09-23", "2026-09-24"} {
		if cal.IsTaiwanTradingDay(taipeiDay(date, "13:00")) {
			t.Errorf("premise changed: industry.EventCalendar.IsTaiwanTradingDay(%s) = true; "+
				"the 中秋 long_holiday window no longer covers this trading day (issue #1947 premise)", date)
		}
	}
	if cal.IsTaiwanTradingDay(taipeiDay("2026-09-25", "13:00")) {
		t.Error("industry.EventCalendar must still treat 2026-09-25 (中秋) as non-trading")
	}
}

// TestRefresh_WritesOnTradingDaysInsideLongHolidayWindow is the end-to-end
// regression: with the production-shaped event calendar wired in, Refresh must
// still persist samples for 2026-09-23 and 2026-09-24 (the frozen week).
func TestRefresh_WritesOnTradingDaysInsideLongHolidayWindow(t *testing.T) {
	ctx := context.Background()
	cal := industry.NewEventCalendar()
	cal.RefreshEvents(taipeiDay("2026-09-24", "12:00"))

	store := NewMemoryRollingSampleStore(252)
	for _, date := range []string{"2026-09-22", "2026-09-23", "2026-09-24"} {
		provider := &stubProvider{snap: marketdata.MacroDataSnapshot{
			RecordedAt:         taipeiDay(date, "13:00").Unix(),
			ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 120},
			DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DomesticFundNet", Value: 30},
			DealerNet:          marketdata.MacroDataPoint{Symbol: "DealerNet", Value: -40},
		}}
		svc := NewServiceWithStore(provider, 0, store, cal)
		if err := svc.Refresh(ctx); err != nil {
			t.Fatalf("Refresh(%s): %v", date, err)
		}
	}

	samples, err := store.History(ctx, ForceForeign, "2099-12-31", 10)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(samples) != 3 {
		t.Fatalf("foreign samples = %v, want one per trading day 09-22/23/24 (issue #1947: the long-holiday window must not suppress trading days)", samples)
	}
	want := []string{"2026-09-22", "2026-09-23", "2026-09-24"}
	for i, w := range want {
		if samples[i].TradingDate != w {
			t.Errorf("samples[%d].TradingDate = %q, want %q", i, samples[i].TradingDate, w)
		}
	}
}

// TestRefresh_SkipsRealHolidayAndWeekend keeps CF-INV-16 honest in the other
// direction: a holiday and a weekend are still skipped, with no samples.
func TestRefresh_SkipsRealHolidayAndWeekend(t *testing.T) {
	ctx := context.Background()
	cal := industry.NewEventCalendar()
	cal.RefreshEvents(taipeiDay("2026-09-24", "12:00"))

	for _, date := range []string{"2026-09-25", "2026-09-26", "2026-09-27"} {
		t.Run(date, func(t *testing.T) {
			store := NewMemoryRollingSampleStore(252)
			provider := &stubProvider{snap: marketdata.MacroDataSnapshot{
				RecordedAt:         taipeiDay(date, "13:00").Unix(),
				ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 120},
			}}
			svc := NewServiceWithStore(provider, 0, store, cal)
			if err := svc.Refresh(ctx); err != nil {
				t.Fatalf("Refresh(%s) must skip-and-log without error, got %v", date, err)
			}
			samples, err := store.History(ctx, ForceForeign, "2099-12-31", 10)
			if err != nil {
				t.Fatalf("History: %v", err)
			}
			if len(samples) != 0 {
				t.Errorf("Refresh(%s) wrote %v, want nothing (CF-INV-16)", date, samples)
			}
			if got := svc.skipCount(); got != 1 {
				t.Errorf("consecutiveSkips = %d, want 1 after one skip", got)
			}
		})
	}
}

// TestRefresh_SkipCounterResetsOnTradingDay locks the "frozen store is
// visible" signal: consecutive skips accumulate and reset on the next
// trading-day refresh.
func TestRefresh_SkipCounterResetsOnTradingDay(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryRollingSampleStore(252)
	snap := func(date string) marketdata.MacroDataSnapshot {
		return marketdata.MacroDataSnapshot{
			RecordedAt:         taipeiDay(date, "13:00").Unix(),
			ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet", Value: 120},
		}
	}
	// 2026-09-25 (holiday) then 09-26/27 (weekend) then 09-28 (trading day).
	for _, date := range []string{"2026-09-25", "2026-09-26"} {
		svc := NewServiceWithStore(&stubProvider{snap: snap(date)}, 0, store, nil)
		if err := svc.Refresh(ctx); err != nil {
			t.Fatalf("Refresh(%s): %v", date, err)
		}
		if got, want := svc.skipCount(), 1; got != want {
			t.Errorf("after one skip on %s consecutiveSkips = %d, want %d", date, got, want)
		}
	}
	svc := NewServiceWithStore(&stubProvider{snap: snap("2026-09-28")}, 0, store, nil)
	if err := svc.Refresh(ctx); err != nil {
		t.Fatalf("Refresh(2026-09-28): %v", err)
	}
	if got := svc.skipCount(); got != 0 {
		t.Errorf("consecutiveSkips = %d after a trading-day refresh, want 0", got)
	}
}

// TestSkipAlertLevel_EscalationPolicy pins the observability policy: a
// contradictory skip (upstream has data) is always WARN, a silent streak is
// WARN exactly once when it crosses the threshold, everything else is INFO.
func TestSkipAlertLevel_EscalationPolicy(t *testing.T) {
	cases := []struct {
		present string
		streak  int
		want    string
	}{
		{present: "", streak: 1, want: "info"},
		{present: "", streak: 2, want: "info"},
		{present: "", streak: nonTradingSkipStreakWarn, want: "warn"},
		{present: "", streak: nonTradingSkipStreakWarn + 1, want: "info"},
		{present: "foreign,institutional", streak: 1, want: "warn"},
		{present: "foreign,institutional", streak: nonTradingSkipStreakWarn, want: "warn"},
	}
	for _, c := range cases {
		if got := skipAlertLevel(c.present, c.streak); got != c.want {
			t.Errorf("skipAlertLevel(%q, %d) = %q, want %q", c.present, c.streak, got, c.want)
		}
	}
}

// TestPresentCapitalFlowInputs pins the contradiction detector used by the
// escalation policy.
func TestPresentCapitalFlowInputs(t *testing.T) {
	if got := presentCapitalFlowInputs(marketdata.MacroDataSnapshot{}); got != "" {
		t.Errorf("empty snapshot = %q, want \"\"", got)
	}
	snap := marketdata.MacroDataSnapshot{
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "ForeignInvestorNet"},
		GovernmentNet:      marketdata.MacroDataPoint{Symbol: "GOV_FLOW_NET"},
	}
	if got := presentCapitalFlowInputs(snap); got != "foreign,government" {
		t.Errorf("presentCapitalFlowInputs = %q, want foreign,government", got)
	}
}
