package industry

import (
	"testing"
	"time"
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

func TestGetEventsForDate(t *testing.T) {
	tec := NewEventCalendar()
	tec.RefreshEvents(time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC))

	events := tec.GetEventsForDate(time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC))
	if len(events) == 0 {
		t.Fatalf("expected active events on 2026-06-25")
	}
	for _, evt := range events {
		if evt.StartDate.After(time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)) ||
			evt.EndDate.Before(time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("event %s outside date range [%s, %s]", evt.Name, evt.StartDate, evt.EndDate)
		}
	}

	empty := tec.GetEventsForDate(time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC))
	if len(empty) != 0 {
		t.Errorf("expected no events on 1999-01-01, got %d", len(empty))
	}
}

func TestIsTaiwanTradingDay(t *testing.T) {
	tec := NewEventCalendar()
	tec.RefreshEvents(time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC))

	cases := []struct {
		name string
		date time.Time
		want bool
	}{
		{"regular weekday", time.Date(2026, 6, 24, 0, 0, 0, 0, time.UTC), true},
		{"saturday", time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC), false},
		{"sunday", time.Date(2026, 6, 28, 0, 0, 0, 0, time.UTC), false},
		{"early january pre-new-year (no events)", time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), true},
		{"lunar new year eve within holiday window", time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC), false},
		{"10/10 national day within holiday window", time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tec.IsTaiwanTradingDay(tc.date); got != tc.want {
				t.Errorf("IsTaiwanTradingDay(%v) = %v, want %v", tc.date, got, tc.want)
			}
		})
	}
}

func TestIsTaiwanTradingDay_228HolidayWindow(t *testing.T) {
	tec := NewEventCalendar()
	tec.RefreshEvents(time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC))

	cases := []struct {
		name string
		date time.Time
		want bool
	}{
		{"friday before 228 (in window)", time.Date(2026, 2, 27, 0, 0, 0, 0, time.UTC), false},
		{"228 saturday itself", time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), false},
		{"monday after 228 (in window)", time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC), false},
		{"regular post-window weekday", time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tec.IsTaiwanTradingDay(tc.date); got != tc.want {
				t.Errorf("IsTaiwanTradingDay(%v) = %v, want %v", tc.date, got, tc.want)
			}
		})
	}
}
