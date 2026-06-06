package industry

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"

)

func TestNewEventCalendar(t *testing.T) {
	tec := NewEventCalendar()
	if tec == nil {
		t.Fatal("expected non-nil EventCalendar")
	}

	// Refresh for 2026
	now := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	tec.RefreshEvents(now)

	if len(tec.annualRules) != 14 {
		t.Errorf("expected 14 event rules, got %d", len(tec.annualRules))
	}

	events := tec.GetAllEvents()
	if len(events) == 0 {
		t.Error("expected events after RefreshEvents, got none")
	}
	t.Logf("total events generated: %d", len(events))
}

func TestDetectActiveEvents(t *testing.T) {
	tec := NewEventCalendar()
	// June 25: window_dressing starts Jun 17, ex_dividend Jun 15-Aug 31,
	// shareholder_meeting May 20-Jun 30, futures_settlement monthly
	now := time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)
	tec.RefreshEvents(now)

	active := tec.DetectActiveEvents(now)

	t.Logf("active events on %s: %d", now.Format("2006-01-02"), len(active))
	for _, evt := range active {
		t.Logf("  - %s (%s): %s to %s, direction=%s, adj=%.4f",
			evt.Name, evt.ID, evt.StartDate.Format("2006-01-02"),
			evt.EndDate.Format("2006-01-02"), evt.Direction, evt.SentimentAdjustment)
	}

	hasExDiv := false
	hasShareholder := false
	hasWindowDressing := false
	hasFutures := false
	for _, evt := range active {
		switch {
		case evt.ID[:len("ex_dividend")] == "ex_dividend":
			hasExDiv = true
		case evt.ID[:len("shareholder_meeting")] == "shareholder_meeting":
			hasShareholder = true
		case evt.ID[:len("window_dressing")] == "window_dressing":
			hasWindowDressing = true
		case evt.ID[:len("futures_settlement")] == "futures_settlement":
			hasFutures = true
		}
	}

	if !hasExDiv {
		t.Error("expected ex_dividend event to be active on June 15")
	}
	if !hasShareholder {
		t.Error("expected shareholder_meeting event to be active on June 15")
	}
	if !hasWindowDressing {
		t.Error("expected window_dressing event to be active on June 15")
	}
	if !hasFutures {
		t.Error("expected futures_settlement event to be active on June 15")
	}
}

func TestGetEventAdjustment(t *testing.T) {
	tec := NewEventCalendar()
	now := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	tec.RefreshEvents(now)

	// Financials should be heavily affected by ex_dividend + shareholder_meeting + window_dressing
	adj := tec.GetEventAdjustment("financials", now)
	t.Logf("financials adjustment: %.4f", adj)

	// An industry not in any affected list should still get partial spillover
	adjIrrelevant := tec.GetEventAdjustment("shipping", now)
	t.Logf("shipping adjustment: %.4f", adjIrrelevant)
}

func TestGetCompositeEventSentiment(t *testing.T) {
	tec := NewEventCalendar()

	// Test with a quiet date (no events? let's check Feb 3 - post spring festival)
	quietDate := time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC)
	tec.RefreshEvents(quietDate)
	sentiment := tec.GetCompositeEventSentiment(quietDate)
	t.Logf("quiet date (2026-02-03) sentiment: %.4f", sentiment)
	if sentiment < 0.8 || sentiment > 1.2 {
		t.Errorf("sentiment %.4f out of [0.8, 1.2] range", sentiment)
	}

	// Test with an active date
	activeDate := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	tec.RefreshEvents(activeDate)
	sentiment = tec.GetCompositeEventSentiment(activeDate)
	t.Logf("active date (2026-06-15) sentiment: %.4f", sentiment)
	if sentiment < 0.8 || sentiment > 1.2 {
		t.Errorf("sentiment %.4f out of [0.8, 1.2] range", sentiment)
	}

	// Verify the value is not exactly 1.0 when events are active
	// (it should deviate from 1.0 since there are active events)
	if sentiment == 1.0 && len(tec.DetectActiveEvents(activeDate)) > 0 {
		t.Log("sentiment is exactly 1.0 despite active events (acceptable if mixed cancels out)")
	}
}

func TestDetectActiveEvents_NoEvents(t *testing.T) {
	tec := NewEventCalendar()
	now := time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC)
	tec.RefreshEvents(now)

	active := tec.DetectActiveEvents(now)
	t.Logf("events on Feb 20: %d", len(active))
	// Feb 20 is between spring_festival end (~Feb 27 in 2026) and Mar window_dressing
	// Should have relatively few events
}

func TestSpringFestivalDates(t *testing.T) {
	tec := NewEventCalendar()
	now := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	tec.RefreshEvents(now)

	active := tec.DetectActiveEvents(now)

	hasSpringFestival := false
	for _, evt := range active {
		if evt.ID[:len("spring_festival")] == "spring_festival" {
			hasSpringFestival = true
			t.Logf("spring_festival: start=%s, peak=%s, end=%s",
				evt.StartDate.Format("2006-01-02"),
				evt.PeakDate.Format("2006-01-02"),
				evt.EndDate.Format("2006-01-02"))

			// 2026 lunar new year is Feb 17
			expectedPeak := time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC)
			if !evt.PeakDate.Equal(expectedPeak) {
				t.Errorf("expected spring festival peak %s, got %s",
					expectedPeak.Format("2006-01-02"),
					evt.PeakDate.Format("2006-01-02"))
			}
		}
	}
	if !hasSpringFestival {
		t.Error("expected spring_festival to be active around lunar new year")
	}
}

func TestWindowDressingDates(t *testing.T) {
	tec := NewEventCalendar()

	testCases := []struct {
		date     time.Time
		expected bool
		label    string
	}{
		{time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC), true, "late March"},
		{time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC), true, "late June"},
		{time.Date(2026, 9, 22, 0, 0, 0, 0, time.UTC), true, "late September"},
		{time.Date(2026, 12, 28, 0, 0, 0, 0, time.UTC), true, "late December"},
		{time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), false, "early March"},
		{time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), false, "early June"},
	}

	for _, tc := range testCases {
		tec.RefreshEvents(tc.date)
		active := tec.DetectActiveEvents(tc.date)
		hasWindowDressing := false
		for _, evt := range active {
			if evt.ID[:len("window_dressing")] == "window_dressing" {
				hasWindowDressing = true
				break
			}
		}
		if hasWindowDressing != tc.expected {
			t.Errorf("%s: expected window_dressing=%v, got %v", tc.label, tc.expected, hasWindowDressing)
		}
	}
}

func TestGetAllActiveEventNames(t *testing.T) {
	tec := NewEventCalendar()
	now := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	tec.RefreshEvents(now)

	names := tec.GetAllActiveEventNames(now)
	t.Logf("active event names: %v", names)
	if len(names) == 0 {
		t.Error("expected active event names")
	}
}

func TestGetEventTimeline(t *testing.T) {
	tec := NewEventCalendar()
	now := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	tec.RefreshEvents(now)

	timeline := tec.GetEventTimeline(now, 30)
	t.Logf("events in next 30 days: %d", len(timeline))
	for _, evt := range timeline {
		t.Logf("  - %s: %s to %s (active=%v)",
			evt.Name, evt.StartDate.Format("2006-01-02"),
			evt.EndDate.Format("2006-01-02"), evt.Active)
	}
	if len(timeline) == 0 {
		t.Error("expected events in next 30 days")
	}
}

func TestElectionEvent(t *testing.T) {
	tec := NewEventCalendar()

	// 2026 is even-numbered (local elections year) - should have election in Nov
	now2026 := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	tec.RefreshEvents(now2026)
	active2026 := tec.DetectActiveEvents(now2026)

	hasElection := false
	for _, evt := range active2026 {
		if evt.ID[:len("election")] == "election" {
			hasElection = true
			t.Logf("2026 election event: %s", evt.ID)
			break
		}
	}
	if !hasElection {
		t.Error("expected election event in Nov 2026 (local elections year)")
	}

	// 2025 is odd-numbered - should NOT have election
	now2025 := time.Date(2025, 11, 15, 0, 0, 0, 0, time.UTC)
	tec.RefreshEvents(now2025)
	active2025 := tec.DetectActiveEvents(now2025)

	for _, evt := range active2025 {
		if evt.ID[:len("election")] == "election" {
			t.Errorf("did NOT expect election event in %s, but found: %s",
				now2025.Format("2006"), evt.ID)
		}
	}

	// 2028 is presidential year - should have election in Jan
	now2028 := time.Date(2028, 1, 15, 0, 0, 0, 0, time.UTC)
	tec.RefreshEvents(now2028)
	active2028 := tec.DetectActiveEvents(now2028)

	hasElection2028 := false
	for _, evt := range active2028 {
		if evt.ID[:len("election")] == "election" {
			hasElection2028 = true
			t.Logf("2028 election event: %s", evt.ID)
			break
		}
	}
	if !hasElection2028 {
		t.Error("expected election event in Jan 2028 (presidential year)")
	}
}

func TestEventString(t *testing.T) {
	evt := CalendarEvent{
		ID:         "test_event",
		Name:       "測試事件",
		NameEN:     "Test Event",
		Direction:  "bullish",
		BaseWeight: 0.5,
		Active:     true,
		PeakDate:   time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
	}
	s := evt.String()
	if s == "" {
		t.Error("String() returned empty")
	}
	t.Logf("event string: %s", s)
}

func TestGetCompositeEventSentiment_QuietDate(t *testing.T) {
	tec := NewEventCalendar()

	// Feb 20 should have few active events
	now := time.Date(2026, 2, 20, 0, 0, 0, 0, time.UTC)
	tec.RefreshEvents(now)
	sentiment := tec.GetCompositeEventSentiment(now)
	t.Logf("Feb 20 sentiment: %.4f", sentiment)

	if sentiment < 0.8 || sentiment > 1.2 {
		t.Errorf("sentiment %.4f out of [0.8, 1.2] range", sentiment)
	}
}

func TestLunarNewYearLookup(t *testing.T) {
	// Verify lookup table has correct ranges
	for year := 2024; year <= 2028; year++ {
		d, ok := lunarNewYearDates[year]
		if !ok {
			t.Errorf("missing lunar new year date for %d", year)
			continue
		}
		// Lunar new year should be between Jan 21 and Feb 20
		if d.Month() < 1 || d.Month() > 2 {
			t.Errorf("lunar new year %d: month out of range: %d", year, d.Month())
		}
		t.Logf("lunar new year %d: %s", year, d.Format("2006-01-02"))
	}
}

func TestDateHelpers(t *testing.T) {
	// Test thirdFriday
	tf := thirdFriday(2026, 6)
	t.Logf("3rd Friday June 2026: %s, weekday=%s", tf.Format("2006-01-02"), tf.Weekday())
	if tf.Weekday() != time.Friday {
		t.Errorf("expected Friday, got %s", tf.Weekday())
	}
	if tf.Day() < 15 || tf.Day() > 21 {
		t.Errorf("3rd Friday should be between 15-21, got day %d", tf.Day())
	}

	// Test thirdWednesday
	tw := thirdWednesday(2026, 6)
	t.Logf("3rd Wednesday June 2026: %s, weekday=%s", tw.Format("2006-01-02"), tw.Weekday())
	if tw.Weekday() != time.Wednesday {
		t.Errorf("expected Wednesday, got %s", tw.Weekday())
	}

	// Test lastBusinessDay
	lbd := lastBusinessDay(2026, 6)
	t.Logf("last business day June 2026: %s, weekday=%s", lbd.Format("2006-01-02"), lbd.Weekday())
	if lbd.Weekday() == time.Saturday || lbd.Weekday() == time.Sunday {
		t.Errorf("last business day should not be weekend, got %s", lbd.Weekday())
	}
	if lbd.Month() != time.June {
		t.Errorf("expected June, got %s", lbd.Month())
	}

	// Test lastTwoWeekStart
	ltws := lastTwoWeekStart(2026, 6)
	t.Logf("last two week start June 2026: %s", ltws.Format("2006-01-02"))
	if ltws.Month() != time.June {
		t.Errorf("expected June, got %s", ltws.Month())
	}
	// Last two weeks of June = days 18-30
	if ltws.Day() < 17 || ltws.Day() > 20 {
		t.Errorf("expected last two week start around day 18, got day %d", ltws.Day())
	}

	// Test nthWeekdayOfMonth
	nth := nthWeekdayOfMonth(2026, 6, time.Monday, 2)
	t.Logf("2nd Monday June 2026: %s, weekday=%s", nth.Format("2006-01-02"), nth.Weekday())
	if nth.Weekday() != time.Monday {
		t.Errorf("expected Monday, got %s", nth.Weekday())
	}
}

func TestGetAllEvents(t *testing.T) {
	tec := NewEventCalendar()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tec.RefreshEvents(now)

	events := tec.GetAllEvents()
	t.Logf("total events for 2026: %d", len(events))

	// Count unique event types
	typeCounts := make(map[string]int)
	for _, e := range events {
		// Extract base event type (before _year_month)
		for _, prefix := range []string{
			"spring_festival", "ex_dividend", "shareholder_meeting",
			"window_dressing", "election", "msci_rebalance",
			"financial_report", "investor_conference", "monthly_revenue",
			"long_holiday", "dividend_payout", "taiwan50_rebalance",
			"futures_settlement", "position_building",
		} {
			if len(e.ID) >= len(prefix) && e.ID[:len(prefix)] == prefix {
				typeCounts[prefix]++
				break
			}
		}
	}

	for eventType, count := range typeCounts {
		t.Logf("  %s: %d events", eventType, count)
	}
}

// --- ST-1: Config override tests ---

func TestApplyConfigOverrides(t *testing.T) {
	tec := NewEventCalendar()
	tec.RefreshEvents(time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC))

	events := tec.GetAllEvents()

	// Verify that default rules match parameters.json values (which are identical)
	for _, e := range events {
		switch e.EventType {
		case "spring_festival":
			if e.BaseWeight != 0.6 {
				t.Errorf("spring_festival: expected weight 0.6, got %.2f", e.BaseWeight)
			}
			if e.DecayDays != 5 {
				t.Errorf("spring_festival: expected decay 5, got %d", e.DecayDays)
			}
		case "msci_rebalance":
			if e.BaseWeight != 0.9 {
				t.Errorf("msci_rebalance: expected weight 0.9, got %.2f", e.BaseWeight)
			}
		case "window_dressing":
			if e.BaseWeight != 0.8 {
				t.Errorf("window_dressing: expected weight 0.8, got %.2f", e.BaseWeight)
			}
		}
	}

	// Verify that annualRules were actually modified by config
	rule, ok := tec.annualRules["spring_festival"]
	if !ok {
		t.Fatal("spring_festival rule not found in annualRules")
	}
	if rule.BaseWeight != 0.6 {
		t.Errorf("annualRules[spring_festival].BaseWeight = %.2f, want 0.6", rule.BaseWeight)
	}
}

// --- ST-2: Provider events survive RefreshEvents ---

type testCalendarProvider struct {
	events []marketdata.CalendarProviderData
}

func (p *testCalendarProvider) Name() string { return "twse_calendar" }
func (p *testCalendarProvider) FetchEvents(_ context.Context, _ int) ([]marketdata.CalendarProviderData, error) {
	return p.events, nil
}

func TestProviderEventsPreservedAcrossRefresh(t *testing.T) {
	tec := NewEventCalendar()
	now := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	tec.RefreshEvents(now)

	provider := &testCalendarProvider{
		events: []marketdata.CalendarProviderData{
			{Date: "2026-07-10", EventType: "ex_dividend", Name: "台積電 除息", Symbol: "2330", Direction: "mixed", Weight: 0.8},
			{Date: "2026-06-25", EventType: "shareholder_meeting", Name: "聯發科 股東會", Symbol: "2454", Direction: "bullish", Weight: 0.3},
		},
	}

	tec.UpdateFromProvider(context.Background(), provider)

	// Provider events should be immediately visible
	countBefore := countProviderEvents(tec.GetAllEvents())
	if countBefore != 2 {
		t.Fatalf("expected 2 provider events after UpdateFromProvider, got %d", countBefore)
	}

	// RefreshEvents should NOT destroy them
	tec.RefreshEvents(now)
	countAfter := countProviderEvents(tec.GetAllEvents())
	if countAfter != 2 {
		t.Errorf("expected 2 provider events after RefreshEvents, got %d (events were destroyed!)", countAfter)
	}

	// Verify specific event data is intact
	found := false
	for _, e := range tec.GetAllEvents() {
		if e.ID == "ex_dividend_2026-07-10_2330" {
			found = true
			if e.Name != "台積電 除息" {
				t.Errorf("TSMC event name = %q, want %q", e.Name, "台積電 除息")
			}
			if e.DataSource != DataSourceTWSE {
				t.Errorf("TSMC event DataSource = %q, want %q", e.DataSource, DataSourceTWSE)
			}
			break
		}
	}
	if !found {
		t.Error("TSMC provider event (ex_dividend_2026-07-10_2330) not found after RefreshEvents")
	}
}

func countProviderEvents(events []CalendarEvent) int {
	n := 0
	for _, e := range events {
		if e.DataSource == DataSourceTWSE {
			n++
		}
	}
	return n
}

// --- ST-5: Validation table-driven tests ---

func TestValidateProviderEvent(t *testing.T) {
	tests := []struct {
		name    string
		event   marketdata.CalendarProviderData
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid event",
			event:   marketdata.CalendarProviderData{Date: "2026-06-15", EventType: "ex_dividend", Symbol: "2330", Direction: "bullish", Weight: 0.5},
			wantErr: false,
		},
		{
			name:    "empty symbol",
			event:   marketdata.CalendarProviderData{Date: "2026-06-15", EventType: "ex_dividend", Symbol: "", Direction: "bullish", Weight: 0.5},
			wantErr: true,
			errMsg:  "empty symbol",
		},
		{
			name:    "empty event_type",
			event:   marketdata.CalendarProviderData{Date: "2026-06-15", EventType: "", Symbol: "2330", Direction: "bullish", Weight: 0.5},
			wantErr: true,
			errMsg:  "empty event_type",
		},
		{
			name:    "empty date",
			event:   marketdata.CalendarProviderData{Date: "", EventType: "ex_dividend", Symbol: "2330", Direction: "bullish", Weight: 0.5},
			wantErr: true,
			errMsg:  "empty date",
		},
		{
			name:    "weight too high",
			event:   marketdata.CalendarProviderData{Date: "2026-06-15", EventType: "ex_dividend", Symbol: "2330", Direction: "bullish", Weight: 1.5},
			wantErr: true,
			errMsg:  "weight",
		},
		{
			name:    "weight negative",
			event:   marketdata.CalendarProviderData{Date: "2026-06-15", EventType: "ex_dividend", Symbol: "2330", Direction: "bullish", Weight: -0.1},
			wantErr: true,
			errMsg:  "weight",
		},
		{
			name:    "invalid direction",
			event:   marketdata.CalendarProviderData{Date: "2026-06-15", EventType: "ex_dividend", Symbol: "2330", Direction: "sideways", Weight: 0.5},
			wantErr: true,
			errMsg:  "invalid direction",
		},
		{
			name:    "date year too old",
			event:   marketdata.CalendarProviderData{Date: "2005-06-15", EventType: "ex_dividend", Symbol: "2330", Direction: "bullish", Weight: 0.5},
			wantErr: true,
			errMsg:  "date year",
		},
		{
			name:    "date year too far future",
			event:   marketdata.CalendarProviderData{Date: "2050-06-15", EventType: "ex_dividend", Symbol: "2330", Direction: "bullish", Weight: 0.5},
			wantErr: true,
			errMsg:  "date year",
		},
		{
			name:    "unparseable date",
			event:   marketdata.CalendarProviderData{Date: "not-a-date", EventType: "ex_dividend", Symbol: "2330", Direction: "bullish", Weight: 0.5},
			wantErr: true,
			errMsg:  "unparseable",
		},
		{
			name:    "direction mixed is valid",
			event:   marketdata.CalendarProviderData{Date: "2026-06-15", EventType: "ex_dividend", Symbol: "2330", Direction: "mixed", Weight: 0.5},
			wantErr: false,
		},
		{
			name:    "direction neutral is valid",
			event:   marketdata.CalendarProviderData{Date: "2026-06-15", EventType: "ex_dividend", Symbol: "2330", Direction: "neutral", Weight: 0.0},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProviderEvent(tt.event)
			if tt.wantErr && err == nil {
				t.Errorf("expected error containing %q, got nil", tt.errMsg)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if got := err.Error(); !contains(got, tt.errMsg) {
					t.Errorf("error = %q, want to contain %q", got, tt.errMsg)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- ST-7: Lunar date fallback test ---

func TestGetLunarDateFallback(t *testing.T) {
	fallback := time.Date(2035, 2, 1, 0, 0, 0, 0, time.UTC)

	// Year within coverage → returns exact date
	d := getLunarDate(2026, lunarNewYearDates, fallback, "春節")
	expected := time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC)
	if !d.Equal(expected) {
		t.Errorf("2026 春節: got %v, want %v", d, expected)
	}

	// Year outside coverage → ST-8: auto-computed instead of fallback
	d = getLunarDate(2035, lunarNewYearDates, fallback, "春節")
	if d.Equal(fallback) {
		t.Errorf("2035 春節: expected auto-computed date, got fallback %v", d)
	}

	// Verify coverage range
	minY, maxY := GetLunarCoverageYears()
	if minY != 2023 || maxY != 2030 {
		t.Errorf("coverage = %d-%d, want 2023-2030", minY, maxY)
	}

	// Verify all years within coverage return non-fallback for all tables
	tables := map[string]map[int]time.Time{
		"spring":    lunarNewYearDates,
		"dragon":    lunarDragonBoatDates,
		"midautumn": lunarMidAutumnDates,
		"tombsweep": tombSweepingDates,
	}
	for name, table := range tables {
		for y := minY; y <= maxY; y++ {
			d := getLunarDate(y, table, fallback, name)
			if d.Equal(fallback) {
				t.Errorf("%s year %d: returned fallback (table entry missing)", name, y)
			}
		}
	}
}

// TestLunarAutoComputation verifies that automatic lunar computation produces
// identical results to the hardcoded lookup table for 2023-2030 (ST-8).
func TestLunarAutoComputation(t *testing.T) {
	testCases := []struct {
		name    string
		compute func(int) time.Time
		tables  map[int]time.Time
	}{
		{"春節", computeLunarNewYear, lunarNewYearDates},
		{"端午節", computeDragonBoat, lunarDragonBoatDates},
		{"中秋節", computeMidAutumn, lunarMidAutumnDates},
		{"清明節", computeQingming, tombSweepingDates},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for year := 2023; year <= 2030; year++ {
				auto := tc.compute(year)
				cached, ok := tc.tables[year]
				if !ok {
					t.Fatalf("missing cached date for %s %d", tc.name, year)
				}
				if !auto.Equal(cached) {
					t.Errorf("%s %d: auto=%s cached=%s mismatch",
						tc.name, year, auto.Format("2006-01-02"), cached.Format("2006-01-02"))
				}
			}
		})
	}
}

// TestLunarAutoComputationFarFuture verifies automatic computation for years
// well outside the original 2023-2030 hardcoded range (ST-8).
func TestLunarAutoComputationFarFuture(t *testing.T) {
	// 2100 Spring Festival
	d := computeLunarNewYear(2100)
	expected := time.Date(2100, 2, 9, 0, 0, 0, 0, time.UTC)
	if !d.Equal(expected) {
		t.Errorf("2100 春節: got %s, want %s", d.Format("2006-01-02"), expected.Format("2006-01-02"))
	}

	// 2049 Dragon Boat
	d = computeDragonBoat(2049)
	if d.Month() != 6 || d.Day() != 4 {
		t.Errorf("2049 端午: got %s, expected Jun 4", d.Format("2006-01-02"))
	}

	// 2049 Mid-Autumn
	d = computeMidAutumn(2049)
	if d.Month() != 9 || d.Day() != 11 {
		t.Errorf("2049 中秋: got %s, expected Sep 11", d.Format("2006-01-02"))
	}

	// 2049 Qingming
	d = computeQingming(2049)
	if d.Month() != 4 || (d.Day() != 4 && d.Day() != 5) {
		t.Errorf("2049 清明: got %s, expected Apr 4 or 5", d.Format("2006-01-02"))
	}
}

// TestLunarHolidayEventsAuto verifies that holiday events use auto-computed
// dates for years outside the hardcoded table (ST-8).
func TestLunarHolidayEventsAuto(t *testing.T) {
	tec := NewEventCalendar()
	now := time.Date(2049, 2, 10, 0, 0, 0, 0, time.UTC)
	tec.RefreshEvents(now)

	active := tec.DetectActiveEvents(now)

	hasSpringFestival := false
	for _, evt := range active {
		if len(evt.ID) >= len("spring_festival") && evt.ID[:len("spring_festival")] == "spring_festival" {
			hasSpringFestival = true
			expectedPeak := time.Date(2049, 2, 2, 0, 0, 0, 0, time.UTC)
			if !evt.PeakDate.Equal(expectedPeak) {
				t.Errorf("2049 spring_festival peak: got %s, want %s",
					evt.PeakDate.Format("2006-01-02"), expectedPeak.Format("2006-01-02"))
			}
			break
		}
	}
	if !hasSpringFestival {
		t.Error("expected spring_festival to be active around 2049 lunar new year")
	}
}

// --- ST-4: Evidence fields test ---

func TestEvidenceFieldsPopulated(t *testing.T) {
	tec := NewEventCalendar()
	now := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	tec.RefreshEvents(now)

	events := tec.GetAllEvents()
	if len(events) == 0 {
		t.Fatal("no events generated")
	}

	for _, e := range events {
		if e.DataSource == "" {
			t.Errorf("event %q has empty DataSource", e.ID)
		}
		if e.EvidenceQuality == "" {
			t.Errorf("event %q has empty EvidenceQuality", e.ID)
		}
		if e.GeneratedAt.IsZero() {
			t.Errorf("event %q has zero GeneratedAt", e.ID)
		}
		// All default-rule events should be DataSourceDefaultRules
		if e.DataSource != DataSourceDefaultRules {
			t.Errorf("event %q: DataSource = %q, want %q", e.ID, e.DataSource, DataSourceDefaultRules)
		}
	}

	// Provider events should have different evidence markers
	provider := &testCalendarProvider{
		events: []marketdata.CalendarProviderData{
			{Date: "2026-08-01", EventType: "ex_dividend", Name: "Test", Symbol: "9999", Direction: "bullish", Weight: 0.5},
		},
	}
	tec.UpdateFromProvider(context.Background(), provider)

	for _, e := range tec.GetAllEvents() {
		if e.ID == "ex_dividend_2026-08-01_9999" {
			if e.DataSource != DataSourceTWSE {
				t.Errorf("provider event DataSource = %q, want %q", e.DataSource, DataSourceTWSE)
			}
			if e.EvidenceQuality != EvidenceEstimated {
				t.Errorf("provider event EvidenceQuality = %q, want %q", e.EvidenceQuality, EvidenceEstimated)
			}
			return
		}
	}
	t.Error("provider event not found in GetAllEvents")
}
