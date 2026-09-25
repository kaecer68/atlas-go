package industry

import (
	"testing"
	"time"
)

// This file is the regression guard for issue #1973: three date defects in the
// event calendar produced semantically wrong dates, and nothing failed when they
// did. Every test here would have failed before the fix.
//
// D1 - buildMonthlyEvent reused rule.ComputePeakDate(year) for every month, so
//      11/12 futures_settlement and 3/4 investor_conference occurrences carried
//      a peak that belonged to another month.
// D2 - buildPositionBuildingEvent derived StartDate and EndDate from two
//      different week anchors, so EndDate was always BEFORE StartDate and
//      DetectActiveEvents could never report the occurrence active.
// D3 - the lunar tables only covered 2023-2040, and years outside that range
//      were answered with conventional placeholder dates (2021 春節 = 02-01
//      instead of 02-12).

// refreshYear returns the event set of one calendar year.
func refreshYear(t *testing.T, year int) []CalendarEvent {
	t.Helper()
	cal := NewEventCalendar()
	cal.RefreshEvents(time.Date(year, time.June, 1, 0, 0, 0, 0, time.UTC))
	return cal.GetAllEvents()
}

// TestCheckDateInvariantsDetectsBothDefectShapes proves the guard itself fails
// on the two shapes the issue reports. Without this, an empty violation list in
// the tests below could just mean a vacuous check.
func TestCheckDateInvariantsDetectsBothDefectShapes(t *testing.T) {
	inverted := CalendarEvent{
		ID:        "position_building_2021_06",
		StartDate: time.Date(2021, 6, 24, 0, 0, 0, 0, time.UTC),
		PeakDate:  time.Date(2021, 6, 26, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2021, 6, 16, 0, 0, 0, 0, time.UTC),
	}
	peakOutside := CalendarEvent{
		ID:        "futures_settlement_2021_01",
		StartDate: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
		PeakDate:  time.Date(2021, 6, 16, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2021, 1, 31, 0, 0, 0, 0, time.UTC),
	}
	healthy := CalendarEvent{
		ID:        "monthly_revenue_2021_01",
		StartDate: time.Date(2021, 1, 7, 0, 0, 0, 0, time.UTC),
		PeakDate:  time.Date(2021, 1, 10, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2021, 1, 13, 0, 0, 0, 0, time.UTC),
	}

	violations := CheckDateInvariants([]CalendarEvent{inverted, peakOutside, healthy})
	if len(violations) != 2 {
		t.Fatalf("violations = %d, want 2 (%+v)", len(violations), violations)
	}
	if violations[0].Reason != InvariantReasonInvertedWindow {
		t.Errorf("first violation reason = %q, want %q", violations[0].Reason, InvariantReasonInvertedWindow)
	}
	if violations[1].Reason != InvariantReasonPeakOutsideWindow {
		t.Errorf("second violation reason = %q, want %q", violations[1].Reason, InvariantReasonPeakOutsideWindow)
	}
}

// TestEventOccurrenceDateInvariants is the core #1973 guard: for every supported
// year, every generated occurrence must satisfy StartDate <= PeakDate <= EndDate.
func TestEventOccurrenceDateInvariants(t *testing.T) {
	for _, year := range []int{2021, 2022, 2023, 2024, 2026, 2030, 2040} {
		t.Run(time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006"), func(t *testing.T) {
			events := refreshYear(t, year)
			if len(events) == 0 {
				t.Fatal("expected events")
			}
			for _, v := range CheckDateInvariants(events) {
				t.Errorf("%s: %s (start=%s peak=%s end=%s)",
					v.EventID, v.Reason,
					v.Start.Format("2006-01-02"), v.Peak.Format("2006-01-02"), v.End.Format("2006-01-02"))
			}
			// Assert the property directly too, so the test is meaningful even if
			// CheckDateInvariants is ever changed.
			for _, evt := range events {
				if evt.EndDate.Before(evt.StartDate) {
					t.Errorf("%s: StartDate %s after EndDate %s",
						evt.ID, evt.StartDate.Format("2006-01-02"), evt.EndDate.Format("2006-01-02"))
				}
				if evt.PeakDate.Before(evt.StartDate) || evt.PeakDate.After(evt.EndDate) {
					t.Errorf("%s: PeakDate %s outside [%s, %s]",
						evt.ID, evt.PeakDate.Format("2006-01-02"),
						evt.StartDate.Format("2006-01-02"), evt.EndDate.Format("2006-01-02"))
				}
			}
		})
	}
}

// TestMonthlyEventPeaksAreInTheirOwnMonth pins D1: each monthly occurrence of
// futures_settlement / investor_conference must peak inside its own window.
func TestMonthlyEventPeaksAreInTheirOwnMonth(t *testing.T) {
	const year = 2021
	events := refreshYear(t, year)

	byType := map[string][]CalendarEvent{}
	for _, evt := range events {
		byType[evt.EventType] = append(byType[evt.EventType], evt)
	}

	settlements := byType["futures_settlement"]
	if len(settlements) != 12 {
		t.Fatalf("futures_settlement occurrences = %d, want 12", len(settlements))
	}
	seen := map[string]bool{}
	for _, evt := range settlements {
		// The rule is "3rd Wednesday of each month" (期貨結算日).
		want := thirdWednesday(year, evt.StartDate.Month())
		if !evt.PeakDate.Equal(want) {
			t.Errorf("%s: PeakDate = %s, want the month's 3rd Wednesday %s",
				evt.ID, evt.PeakDate.Format("2006-01-02"), want.Format("2006-01-02"))
		}
		if evt.PeakDate.Month() != evt.StartDate.Month() {
			t.Errorf("%s: PeakDate %s is not in the occurrence's own month %s",
				evt.ID, evt.PeakDate.Format("2006-01-02"), evt.StartDate.Month())
		}
		key := evt.PeakDate.Format("2006-01-02")
		if seen[key] {
			t.Errorf("%s: PeakDate %s is shared by two occurrences", evt.ID, key)
		}
		seen[key] = true
	}

	conferences := byType["investor_conference"]
	if len(conferences) != 4 {
		t.Fatalf("investor_conference occurrences = %d, want 4", len(conferences))
	}
	seen = map[string]bool{}
	for _, evt := range conferences {
		if evt.PeakDate.Month() != evt.StartDate.Month() {
			t.Errorf("%s: PeakDate %s is not in the occurrence's own month %s",
				evt.ID, evt.PeakDate.Format("2006-01-02"), evt.StartDate.Month())
		}
		key := evt.PeakDate.Format("2006-01-02")
		if seen[key] {
			t.Errorf("%s: PeakDate %s is shared by two occurrences", evt.ID, key)
		}
		seen[key] = true
	}
	// The January occurrence used to peak on 2021-07-15 (the year-anchored peak).
	for _, evt := range conferences {
		if evt.StartDate.Month() == time.January && evt.PeakDate.Format("2006-01-02") != "2021-01-15" {
			t.Errorf("%s: January peak = %s, want 2021-01-15", evt.ID, evt.PeakDate.Format("2006-01-02"))
		}
	}
}

// TestPositionBuildingWindowIsNotInverted pins D2 and the window semantics: the
// occurrence is the week immediately before that month's window-dressing window.
func TestPositionBuildingWindowIsNotInverted(t *testing.T) {
	cases := []struct {
		year  int
		month time.Month
		start string
		peak  string
		end   string
	}{
		{2021, time.March, "2021-03-11", "2021-03-13", "2021-03-17"},
		{2021, time.June, "2021-06-10", "2021-06-12", "2021-06-16"},
		{2021, time.December, "2021-12-11", "2021-12-13", "2021-12-17"},
		{2026, time.June, "2026-06-10", "2026-06-12", "2026-06-16"},
	}
	for _, tc := range cases {
		cal := NewEventCalendar()
		cal.RefreshEvents(time.Date(tc.year, tc.month, 15, 0, 0, 0, 0, time.UTC))

		var found *CalendarEvent
		for _, evt := range cal.GetAllEvents() {
			if evt.EventType == "position_building" && evt.StartDate.Month() == tc.month {
				e := evt
				found = &e
			}
		}
		if found == nil {
			t.Fatalf("%s %d: no position_building occurrence", tc.month, tc.year)
		}
		if got := found.StartDate.Format("2006-01-02"); got != tc.start {
			t.Errorf("%s %d: StartDate = %s, want %s", tc.month, tc.year, got, tc.start)
		}
		if got := found.PeakDate.Format("2006-01-02"); got != tc.peak {
			t.Errorf("%s %d: PeakDate = %s, want %s", tc.month, tc.year, got, tc.peak)
		}
		if got := found.EndDate.Format("2006-01-02"); got != tc.end {
			t.Errorf("%s %d: EndDate = %s, want %s", tc.month, tc.year, got, tc.end)
		}
		if found.StartDate.After(found.EndDate) {
			t.Fatalf("%s %d: window is inverted: %s > %s",
				tc.month, tc.year, found.StartDate.Format("2006-01-02"), found.EndDate.Format("2006-01-02"))
		}
		// The window must close exactly when the window-dressing window opens.
		dressing := lastTwoWeekStart(tc.year, tc.month)
		if !found.EndDate.Equal(dressing.AddDate(0, 0, -1)) {
			t.Errorf("%s %d: EndDate = %s, want the day before window_dressing starts (%s)",
				tc.month, tc.year, found.EndDate.Format("2006-01-02"), dressing.Format("2006-01-02"))
		}
	}
}

// TestPositionBuildingIsActiveInItsWindow is the D2 consequence check: the
// occurrence must actually become active inside its window (it could not before,
// because dateInRange never matched an inverted window).
func TestPositionBuildingIsActiveInItsWindow(t *testing.T) {
	cal := NewEventCalendar()
	cal.RefreshEvents(time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC))

	active := func(date string) bool {
		d, err := time.Parse("2006-01-02", date)
		if err != nil {
			t.Fatal(err)
		}
		for _, evt := range cal.DetectActiveEvents(d) {
			if evt.EventType == "position_building" {
				return true
			}
		}
		return false
	}

	for _, in := range []string{"2026-06-10", "2026-06-12", "2026-06-16"} {
		if !active(in) {
			t.Errorf("position_building should be active on %s", in)
		}
	}
	for _, out := range []string{"2026-06-01", "2026-06-17", "2026-06-24"} {
		if active(out) {
			t.Errorf("position_building should not be active on %s", out)
		}
	}
}

// TestLunarUnavailableProducesNoOccurrenceAndNoFabricatedDate pins D3: for a
// year outside the verified lunar range the moving holidays are "not
// determinable", so the calendar omits those occurrences instead of dating them
// with the conventional placeholder.
func TestLunarUnavailableProducesNoOccurrenceAndNoFabricatedDate(t *testing.T) {
	const year = 2019
	if LunarYearDeterminable(year) {
		t.Fatalf("%d must not be lunar-determinable", year)
	}
	minYear, _ := GetLunarCoverageYears()
	if year >= minYear {
		t.Fatalf("test premise broken: %d is inside the declared coverage", year)
	}

	events := refreshYear(t, year)
	if len(events) == 0 {
		t.Fatal("expected the non-lunar events to still be generated")
	}

	// Placeholder dates the old fallback produced for 2019.
	placeholders := map[string]string{
		"2019-02-01": "春節 02-01",
		"2019-04-05": "清明 04-05",
		"2019-06-10": "端午 06-10",
		"2019-09-20": "中秋 09-20",
	}

	longHolidays := 0
	for _, evt := range events {
		if evt.EventType == string(EventSpringFestival) {
			t.Errorf("spring_festival must be omitted for %d, got %s", year, evt.ID)
		}
		if evt.EventType == string(EventLongHoliday) {
			longHolidays++
			for _, lunar := range []string{"春節", "清明節", "端午節", "中秋節"} {
				if evt.Name == "連假 - "+lunar {
					t.Errorf("lunar long_holiday %q must be omitted for %d, got %s", lunar, year, evt.ID)
				}
			}
			if _, isPlaceholder := placeholders[evt.PeakDate.Format("2006-01-02")]; isPlaceholder {
				t.Errorf("%s: PeakDate %s is a placeholder for an undeterminable year",
					evt.ID, evt.PeakDate.Format("2006-01-02"))
			}
		}
	}
	if longHolidays != 4 {
		t.Errorf("long_holiday occurrences = %d, want the 4 fixed-date holidays only", longHolidays)
	}
}

// TestSpringFestival2021CoversTheRealHoliday is the D3 regression that motivated
// the table extension: 2021 春節 is 02-12, and the calendar's own window must
// contain it. Previously the placeholder peak 02-01 produced the window
// 01-27..02-11, which missed the real holiday completely.
func TestSpringFestival2021CoversTheRealHoliday(t *testing.T) {
	const year = 2021
	if !LunarYearDeterminable(year) {
		t.Fatalf("%d must be lunar-determinable (verified range)", year)
	}

	var spring *CalendarEvent
	events := refreshYear(t, year)
	for _, evt := range events {
		if evt.EventType == string(EventSpringFestival) {
			e := evt
			spring = &e
		}
	}
	if spring == nil {
		t.Fatalf("no spring_festival occurrence for %d", year)
	}

	wantPeak := time.Date(2021, 2, 12, 0, 0, 0, 0, time.UTC)
	if !spring.PeakDate.Equal(wantPeak) {
		t.Errorf("spring_festival peak = %s, want the real lunar new year %s",
			spring.PeakDate.Format("2006-01-02"), wantPeak.Format("2006-01-02"))
	}
	if !dateInRange(wantPeak, spring.StartDate, spring.EndDate) {
		t.Errorf("the window [%s, %s] does not contain the real 春節 %s",
			spring.StartDate.Format("2006-01-02"), spring.EndDate.Format("2006-01-02"),
			wantPeak.Format("2006-01-02"))
	}

	// The real 2021 TWSE spring-festival closure (02-08..02-16, first-party
	// session data) must therefore be covered around its核心 day.
	cal := NewEventCalendar()
	cal.RefreshEvents(time.Date(year, time.June, 1, 0, 0, 0, 0, time.UTC))
	active := false
	for _, evt := range cal.DetectActiveEvents(wantPeak) {
		if evt.EventType == string(EventSpringFestival) {
			active = true
		}
	}
	if !active {
		t.Errorf("spring_festival must be active on the real lunar new year %s", wantPeak.Format("2006-01-02"))
	}
}

// TestPeakDateConsumersAgreeWithTheWindow checks the two consumers the issue
// calls out. Both read PeakDate as "the day of this occurrence", so a
// peak-outside-window or inverted-window occurrence made them wrong (D1/D2).
//
//   - toRawEvent feeds EffectiveDate (= PeakDate) into the Stage 2 quality gate.
//   - GetEventTimeline marks evt.Active from dateInRange(now, Start, End).
func TestPeakDateConsumersAgreeWithTheWindow(t *testing.T) {
	for _, year := range []int{2021, 2026} {
		events := refreshYear(t, year)
		if len(events) == 0 {
			t.Fatalf("%d: expected events", year)
		}
		for _, evt := range events {
			raw := toRawEvent(evt)
			if !dateInRange(raw.EffectiveDate, evt.StartDate, evt.EndDate) {
				t.Errorf("%s: toRawEvent EffectiveDate %s outside window [%s, %s]",
					evt.ID, raw.EffectiveDate.Format("2006-01-02"),
					evt.StartDate.Format("2006-01-02"), evt.EndDate.Format("2006-01-02"))
			}
		}
	}

	// GetEventTimeline: Active must be exactly "now is inside the window", and
	// every returned occurrence must actually overlap the window.
	now := time.Date(2021, 6, 12, 0, 0, 0, 0, time.UTC)
	cal := NewEventCalendar()
	cal.RefreshEvents(time.Date(2021, 6, 1, 0, 0, 0, 0, time.UTC))
	timeline := cal.GetEventTimeline(now, 10)
	if len(timeline) == 0 {
		t.Fatal("expected events in the 2021-06-12..06-22 timeline")
	}
	hasFutures := false
	for _, evt := range timeline {
		want := dateInRange(now, evt.StartDate, evt.EndDate)
		if evt.Active != want {
			t.Errorf("%s: Active = %v, want %v for now=%s window=[%s, %s]",
				evt.ID, evt.Active, want, now.Format("2006-01-02"),
				evt.StartDate.Format("2006-01-02"), evt.EndDate.Format("2006-01-02"))
		}
		if evt.EventType == "futures_settlement" {
			hasFutures = true
			// 2021-06 settlement is the 3rd Wednesday, 06-16.
			if evt.PeakDate.Format("2006-01-02") != "2021-06-16" {
				t.Errorf("%s: peak = %s, want 2021-06-16", evt.ID, evt.PeakDate.Format("2006-01-02"))
			}
		}
	}
	if !hasFutures {
		t.Error("expected the June 2021 futures_settlement occurrence in the timeline")
	}
}
