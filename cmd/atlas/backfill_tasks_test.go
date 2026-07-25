package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/config"
)

// mockTradingDayCalendar treats every weekday as a trading day and weekends as
// non-trading. It is sufficient for unit testing the gap detector without a
// full holiday database.
type mockTradingDayCalendar struct{}

func (mockTradingDayCalendar) IsTaiwanTradingDay(date time.Time) bool {
	w := date.Weekday()
	return w != time.Saturday && w != time.Sunday
}

func TestGapDetector_DetectDailyFiles_MissingDates(t *testing.T) {
	dir := t.TempDir()
	capitalDir := filepath.Join(dir, "data", "state", "capital_flow")
	if err := os.MkdirAll(capitalDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Friday.
	reference := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC) // Friday
	// Custom expectation with 7-day lookback for a focused test.
	exp := ChannelCoverageExpectation{
		ChannelID: "capital_flow", CoverageType: CoverageDailyFiles,
		FilePattern: "20060102.json", LookbackDays: 7, Enabled: true,
	}
	// reference = 2026-07-24 Fri; end = yesterday 2026-07-23 Thu.
	// start = 2026-07-17 Fri. Trading days in window:
	// Fri 17, Mon 20, Tue 21, Wed 22, Thu 23.
	// Create files for Fri 17, Mon 20, Thu 23 -> expect Tue 21 and Wed 22 missing.
	dates := []string{"20260717", "20260720", "20260723"}
	for _, d := range dates {
		if err := os.WriteFile(filepath.Join(capitalDir, d+".json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	detector := newGapDetector(dir, mockTradingDayCalendar{})
	detector.expectations = []ChannelCoverageExpectation{exp}
	report := detector.detect(reference)

	var capitalReport *ChannelGapReport
	for i := range report.Channels {
		if report.Channels[i].ChannelID == "capital_flow" {
			capitalReport = &report.Channels[i]
			break
		}
	}
	if capitalReport == nil {
		t.Fatalf("expected capital_flow report, got %+v", report.Channels)
	}
	if capitalReport.MissingCount != 2 {
		t.Fatalf("expected 2 missing dates, got %d: %v", capitalReport.MissingCount, capitalReport.MissingDates)
	}
	want := map[string]bool{"2026-07-21": true, "2026-07-22": true}
	got := map[string]bool{}
	for _, d := range capitalReport.MissingDates {
		got[d] = true
	}
	for k := range want {
		if !got[k] {
			t.Fatalf("missing expected date %s in %v", k, capitalReport.MissingDates)
		}
	}
}

func TestGapDetector_DetectLatestFile_Stale(t *testing.T) {
	dir := t.TempDir()
	fugleDir := filepath.Join(dir, "data", "state", "fugle")
	if err := os.MkdirAll(fugleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if err := os.WriteFile(filepath.Join(fugleDir, "latest.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(fugleDir, "latest.json"), stale, stale); err != nil {
		t.Fatal(err)
	}

	reference := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC) // 23 days later
	detector := newGapDetector(dir, mockTradingDayCalendar{})
	report := detector.detect(reference)

	var fugleReport *ChannelGapReport
	for i := range report.Channels {
		if report.Channels[i].ChannelID == "fugle" {
			fugleReport = &report.Channels[i]
			break
		}
	}
	if fugleReport == nil {
		t.Fatalf("expected fugle report")
	}
	if fugleReport.MissingCount != 1 {
		t.Fatalf("expected stale latest file, got missing_count=%d", fugleReport.MissingCount)
	}
	if fugleReport.LatestDate != "2026-07-01" {
		t.Fatalf("expected latest_date 2026-07-01, got %s", fugleReport.LatestDate)
	}
}

func TestGapDetector_WriteReport(t *testing.T) {
	dir := t.TempDir()
	detector := newGapDetector(dir, mockTradingDayCalendar{})
	report := GapReport{GeneratedAt: time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC)}
	if err := detector.writeReport(report); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "data", "state", "gap_report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed GapReport
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if !parsed.GeneratedAt.Equal(report.GeneratedAt) {
		t.Fatalf("expected generated_at %v, got %v", report.GeneratedAt, parsed.GeneratedAt)
	}
}

func TestRegisterBackfillTasks_TaskRegistered(t *testing.T) {
	mgr := apigateway.NewBackgroundTaskManager(nil)
	registerBackfillTasks(backfillDeps{
		taskMgr: mgr,
		cfg:     config.Config{WorkDir: t.TempDir()},
	})
	if _, ok := mgr.Get("auto_gap_detection"); !ok {
		t.Fatal("auto_gap_detection task was not registered")
	}
}
