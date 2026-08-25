package taiwanholidays

import (
	"testing"
	"time"
)

func TestCoverageYears(t *testing.T) {
	minY, maxY := CoverageYears()
	if minY != 2023 || maxY != 2040 {
		t.Fatalf("coverage = %d-%d, want 2023-2040", minY, maxY)
	}
}

// TestLunarTables_2031_2040 pins the extended table values (computed 2026-08-23
// and cross-validated: lunardate for lunar dates, ephem for the 清明 solar term).
func TestLunarTables_2031_2040(t *testing.T) {
	want := map[string]string{
		"2031-01-23": "chunjie", "2032-02-11": "chunjie", "2033-01-31": "chunjie",
		"2034-02-19": "chunjie", "2035-02-08": "chunjie", "2036-01-28": "chunjie",
		"2037-02-15": "chunjie", "2038-02-04": "chunjie", "2039-01-24": "chunjie",
		"2040-02-12": "chunjie",
		"2031-04-05": "qingming", "2032-04-04": "qingming", "2033-04-04": "qingming",
		"2034-04-05": "qingming", "2035-04-05": "qingming", "2036-04-04": "qingming",
		"2037-04-04": "qingming", "2038-04-05": "qingming", "2039-04-05": "qingming",
		"2040-04-04": "qingming",
		"2031-06-24": "duanwu", "2032-06-12": "duanwu", "2033-06-01": "duanwu",
		"2034-06-20": "duanwu", "2035-06-10": "duanwu", "2036-05-30": "duanwu",
		"2037-06-18": "duanwu", "2038-06-07": "duanwu", "2039-05-27": "duanwu",
		"2040-06-14": "duanwu",
		"2031-10-01": "zhongqiu", "2032-09-19": "zhongqiu", "2033-09-08": "zhongqiu",
		"2034-09-27": "zhongqiu", "2035-09-16": "zhongqiu", "2036-10-04": "zhongqiu",
		"2037-09-24": "zhongqiu", "2038-09-13": "zhongqiu", "2039-10-02": "zhongqiu",
		"2040-09-20": "zhongqiu",
	}
	// Lunar accessor checks
	type acc struct {
		name string
		f    func(int) (time.Time, bool)
	}
	for _, a := range []acc{
		{"chunjie", LunarNewYear},
		{"qingming", TombSweeping},
		{"duanwu", LunarDragonBoat},
		{"zhongqiu", LunarMidAutumn},
	} {
		for y := 2031; y <= 2040; y++ {
			d, ok := a.f(y)
			if !ok {
				t.Errorf("%s %d: missing from table", a.name, y)
				continue
			}
			key := d.Format("2006-01-02")
			kind, exists := want[key]
			if !exists || kind != a.name {
				t.Errorf("%s %d = %s (want %s entry)", a.name, y, key, a.name)
			}
		}
	}
}

// TestHolidaysInYear_FullCoverage pins the complete holiday set for every
// verified year: 8 base holidays (4 fixed + 4 lunar) plus the TWSE spring
// festival closure span (settlement days + 除夕~初五 + 補假) for 2023-2026,
// and the lunar dates must match the canonical accessors.
func TestHolidaysInYear_FullCoverage(t *testing.T) {
	for y := 2023; y <= 2040; y++ {
		hs := HolidaysInYear(y)
		want := 8
		if closures, ok := springFestivalClosures[y]; ok {
			want += len(closures) - 1 // -1: 初一 already counted in the 4 lunar
		}
		if len(hs) != want {
			t.Fatalf("%d: holidays = %d, want %d (got %+v)", y, len(hs), want, hs)
		}
		// 清明 must match the canonical tomb-sweeping accessor.
		for _, h := range hs {
			if h.Name == "清明節" {
				want, _ := TombSweeping(y)
				if !h.Date.Equal(want) {
					t.Errorf("%d 清明節 = %s, want %s", y, h.Date.Format("2006-01-02"), want.Format("2006-01-02"))
				}
			}
		}
		// sorted
		for i := 1; i < len(hs); i++ {
			if hs[i].Date.Before(hs[i-1].Date) {
				t.Fatalf("%d: holidays not sorted: %v before %v", y, hs[i].Date, hs[i-1].Date)
			}
		}
	}
}

func TestHolidaysInYear_2026(t *testing.T) {
	hs := HolidaysInYear(2026)
	// 4 fixed + 4 lunar + 6 extra spring-closure days (2/12,13,16,18,19,20)
	if len(hs) != 14 {
		t.Fatalf("2026 holidays = %d, want 14 (got %+v)", len(hs), hs)
	}
	dates := map[string]bool{}
	for _, h := range hs {
		dates[h.Date.Format("2006-01-02")] = true
	}
	want := []string{
		"2026-01-01", // 元旦
		"2026-02-17", // 春節
		"2026-02-28", // 228
		"2026-04-05", // 清明
		"2026-05-01", // 勞動節
		"2026-06-19", // 端午
		"2026-09-25", // 中秋
		"2026-10-10", // 國慶
	}
	for _, w := range want {
		if !dates[w] {
			t.Errorf("2026 missing holiday %s (got %v)", w, dates)
		}
	}
	// sorted by date
	for i := 1; i < len(hs); i++ {
		if hs[i].Date.Before(hs[i-1].Date) {
			t.Fatalf("holidays not sorted: %v before %v", hs[i].Date, hs[i-1].Date)
		}
	}
}

func TestIsHoliday_FixedAndLunar(t *testing.T) {
	cases := []time.Time{
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),   // 元旦
		time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC),  // 春節
		time.Date(2026, 4, 5, 0, 0, 0, 0, time.UTC),   // 清明
		time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC), // 國慶
	}
	for _, c := range cases {
		if !IsHoliday(c) {
			t.Errorf("%s should be a holiday", c.Format("2006-01-02"))
		}
		if IsTradingDay(c) {
			t.Errorf("%s should NOT be a trading day", c.Format("2006-01-02"))
		}
	}
	notHoliday := []time.Time{
		time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC), // ordinary Friday
		time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC), // ordinary Friday
	}
	for _, c := range notHoliday {
		if IsHoliday(c) {
			t.Errorf("%s should not be a holiday", c.Format("2006-01-02"))
		}
		if !IsTradingDay(c) {
			t.Errorf("%s should be a trading day", c.Format("2006-01-02"))
		}
	}
}

func TestIsTradingDay_Weekend(t *testing.T) {
	sat := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
	sun := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	if IsTradingDay(sat) || IsTradingDay(sun) {
		t.Fatal("weekend must not be a trading day")
	}
}

func TestPreviousTradingDay_SkipsWeekend(t *testing.T) {
	// Friday 2026-07-24 → Thursday 2026-07-23
	got := PreviousTradingDay(time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC), 1)
	if got.Format("2006-01-02") != "2026-07-23" {
		t.Errorf("got %s, want 2026-07-23", got.Format("2006-01-02"))
	}
	// Monday 2026-07-27 → previous trading day = Friday 2026-07-24
	got = PreviousTradingDay(time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC), 1)
	if got.Format("2006-01-02") != "2026-07-24" {
		t.Errorf("got %s, want 2026-07-24", got.Format("2006-01-02"))
	}
}

func TestPreviousTradingDay_SkipsHoliday(t *testing.T) {
	// 2026-10-10 (Sat) is 國慶日; 2026-10-09 (Fri) is a regular trading day.
	// From Monday 2026-10-12, one day back must land on Friday 10-09.
	got := PreviousTradingDay(time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC), 1)
	if got.Format("2006-01-02") != "2026-10-09" {
		t.Errorf("got %s, want 2026-10-09 (holiday Monday rollback)", got.Format("2006-01-02"))
	}

	// 2026 春節休市: 2/12-2/20 (settlement + 除夕~初五 + 補假). From 2/23
	// (Mon, reopening) one day back must skip the entire closure span and
	// land on 2/11 (Wed, last trading day), NOT a closure day.
	got = PreviousTradingDay(time.Date(2026, 2, 23, 12, 0, 0, 0, time.UTC), 1)
	if got.Format("2006-01-02") != "2026-02-11" {
		t.Errorf("got %s, want 2026-02-11 (spring festival closure rollback)", got.Format("2006-01-02"))
	}
}

func TestPreviousTradingDay_MultiDay(t *testing.T) {
	// Monday 2026-07-27 with daysBack=7 → previous Monday 2026-07-20
	got := PreviousTradingDay(time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC), 7)
	if got.Format("2006-01-02") != "2026-07-20" {
		t.Errorf("got %s, want 2026-07-20", got.Format("2006-01-02"))
	}
}

func TestFallbackBeyond2040_ReturnsConventionalDate(t *testing.T) {
	// 2045 is beyond the verified table (extended to 2040): fallback must
	// still return a date (conventional), and the trading-day query must
	// not panic.
	hs := HolidaysInYear(2045)
	if len(hs) != 8 {
		t.Fatalf("2045 holidays = %d, want 8 (conventional fallback)", len(hs))
	}
	found := false
	for _, h := range hs {
		if h.Name == "春節" && h.Date.Format("2006-01-02") == "2045-02-01" {
			found = true
		}
	}
	if !found {
		t.Errorf("2045 spring festival fallback not 2045-02-01: %+v", hs)
	}
	if !IsHoliday(time.Date(2045, 4, 5, 0, 0, 0, 0, time.UTC)) {
		t.Error("2045-04-05 should be a holiday via fallback")
	}
}

func TestLunarNewYearAccessor(t *testing.T) {
	d, ok := LunarNewYear(2026)
	if !ok || d.Format("2006-01-02") != "2026-02-17" {
		t.Fatalf("LunarNewYear(2026) = %v ok=%v", d, ok)
	}
	if _, ok := LunarNewYear(2045); ok {
		t.Fatal("LunarNewYear(2045) should report ok=false (out of coverage)")
	}
	if m := LunarNewYearDates(); len(m) != len(lunarNewYearDates) {
		t.Fatalf("LunarNewYearDates() copy len=%d want %d", len(m), len(lunarNewYearDates))
	}
}

// TestSpringFestivalClosures_TWSEOfficial pins the full TWSE closure span
// (settlement days + 除夕~初五 + 補假) for 2023-2026 — verified against the
// TWSE official 開休市日期 calendar (2026-08-25). Regression guard: a
// weekday closure inside the spring window must NOT be a trading day
// (previously only 初一 was marked, so daily-replay-sync appended fake
// spring-festival quotes and clean-replay-weekends failed to remove them).
func TestSpringFestivalClosures_TWSEOfficial(t *testing.T) {
	closures := map[string][]string{
		"2023": {"2023-01-18", "2023-01-19", "2023-01-20", "2023-01-21", "2023-01-24", "2023-01-25", "2023-01-26", "2023-01-27"},
		"2024": {"2024-02-06", "2024-02-07", "2024-02-08", "2024-02-09", "2024-02-13", "2024-02-14"},
		"2025": {"2025-01-23", "2025-01-24", "2025-01-27", "2025-01-28", "2025-01-30", "2025-01-31"},
		"2026": {"2026-02-12", "2026-02-13", "2026-02-16", "2026-02-18", "2026-02-19", "2026-02-20"},
	}
	for yr, days := range closures {
		for _, ds := range days {
			d, _ := time.Parse("2006-01-02", ds)
			if IsTradingDay(d) {
				t.Errorf("%s must be closed (spring festival closure %s)", ds, yr)
			}
		}
	}
	// Reopening days must be trading days.
	for _, ds := range []string{"2023-01-30", "2024-02-15", "2025-02-03", "2026-02-23"} {
		d, _ := time.Parse("2006-01-02", ds)
		if !IsTradingDay(d) {
			t.Errorf("%s must be a trading day (spring festival reopening)", ds)
		}
	}
}
