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

// --- channel-dir contract (2026-10-01) -------------------------------------
//
// The channel id and the on-disk directory under data/state are two different
// names, and only the producer (the channel adapter's SetStorageDir) decides the
// directory. Rows below lock the two that diverged in production; before the
// DataDir field existed the scanner read a directory that is never written and
// reported a permanent false "missing coverage" gap.

// TestDefaultChannelCoverageExpectations_DataDirIsProducerDir locks the mapping
// against the producer's own path in internal/apigateway/register_adapters.go.
func TestDefaultChannelCoverageExpectations_DataDirIsProducerDir(t *testing.T) {
	want := map[string]string{
		"tdcc_equity_dispersion": "tdcc_dispersion",
		"twse_sbl":               "sbl",
	}

	seen := map[string]bool{}
	for _, exp := range defaultChannelCoverageExpectations() {
		dir, ok := want[exp.ChannelID]
		if !ok {
			continue
		}
		seen[exp.ChannelID] = true
		if got := exp.dir(); got != dir {
			t.Errorf("channel %s reads data/state/%s, want data/state/%s (producer directory)", exp.ChannelID, got, dir)
		}
	}
	for channelID := range want {
		if !seen[channelID] {
			t.Errorf("channel %s missing from defaultChannelCoverageExpectations", channelID)
		}
	}
}

// TestGapDetector_ProducerDir_NoFalseGap is the behavioural regression: with the
// data present under the PRODUCER directories only (as in production), the
// default expectations must report no gap and no error for both channels. Both
// assertions fail before the DataDir fix — the scanner looked under
// data/state/tdcc_equity_dispersion and data/state/twse_sbl, which do not exist.
func TestGapDetector_ProducerDir_NoFalseGap(t *testing.T) {
	dir := t.TempDir()
	reference := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC) // Friday

	// G01: a fresh latest snapshot under the producer directory.
	tdccDir := filepath.Join(dir, "data", "state", "tdcc_dispersion")
	if err := os.MkdirAll(tdccDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tdccDir, "latest.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// G02: one per-day report file for every weekday in the 30-day window.
	sblDir := filepath.Join(dir, "data", "state", "sbl")
	if err := os.MkdirAll(sblDir, 0o755); err != nil {
		t.Fatal(err)
	}
	end := reference.AddDate(0, 0, -1)
	for d := reference.AddDate(0, 0, -30); !d.After(end); d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd == time.Saturday || wd == time.Sunday {
			continue
		}
		name := d.Format("20060102") + "_sbl.json"
		if err := os.WriteFile(filepath.Join(sblDir, name), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	detector := newGapDetector(dir, mockTradingDayCalendar{})
	report := detector.detect(reference)

	byChannel := map[string]ChannelGapReport{}
	for _, ch := range report.Channels {
		byChannel[ch.ChannelID] = ch
	}
	for _, channelID := range []string{"tdcc_equity_dispersion", "twse_sbl"} {
		ch, ok := byChannel[channelID]
		if !ok {
			t.Fatalf("expected a %s report, got %+v", channelID, report.Channels)
		}
		if ch.Error != "" {
			t.Errorf("%s: unexpected error %q (producer directory not read?)", channelID, ch.Error)
		}
		if ch.MissingCount != 0 {
			t.Errorf("%s: missing_count = %d, want 0 (files exist under the producer directory)", channelID, ch.MissingCount)
		}
	}
}
