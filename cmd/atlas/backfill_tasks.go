package main

// Background gap-detection task for data channels that store per-date files.
//
// This task does not replace the existing channel-specific backfill tasks
// (auto_backfill for twse_replay, margin_history_backfill for margin, etc.).
// It adds a generic scanner that:
//   1. knows the desired coverage for each channel (daily file, latest file, none)
//   2. scans data/state/<channel> for missing dates
//   3. writes a JSON gap report to data/state/gap_report.json
//   4. emits Monitor.Alert records so missing data is visible instead of silent
//
// Future PRs can add per-channel backfill command hooks once a reliable source
// is wired for each channel.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// CoverageType values.
const (
	CoverageDailyFiles = "daily_files"
	CoverageLatestFile = "latest_file"
	CoverageNone       = "none"
)

// ChannelCoverageExpectation describes how we expect a channel's data to be
// represented on disk and how far back we care about.
type ChannelCoverageExpectation struct {
	ChannelID    string `json:"channel_id"`
	CoverageType string `json:"coverage_type"` // daily_files | latest_file | none
	// FilePattern is a Go time.Format layout string, e.g. "20060102.json".
	FilePattern  string `json:"file_pattern,omitempty"`
	LookbackDays int    `json:"lookback_days"`
	Enabled      bool   `json:"enabled"`
}

// defaultChannelCoverageExpectations is the initial, hard-coded coverage map.
// It intentionally covers only channels with a known, stable on-disk format so
// the scanner does not produce false positives on channels whose storage layout
// is not yet finalized.
func defaultChannelCoverageExpectations() []ChannelCoverageExpectation {
	return []ChannelCoverageExpectation{
		// #1780 audit: files were renamed upstream to <date>_capital_flow.json
		// (2026-06); the old bare-date pattern reported 13 false "missing
		// coverage" warnings daily for data that existed under the new name.
		{ChannelID: "capital_flow", CoverageType: CoverageDailyFiles, FilePattern: "20060102_capital_flow.json", LookbackDays: 30, Enabled: true},
		{ChannelID: "margin", CoverageType: CoverageDailyFiles, FilePattern: "20060102_margin.json", LookbackDays: 30, Enabled: true},
		{ChannelID: "government_flow", CoverageType: CoverageDailyFiles, FilePattern: "20060102.json", LookbackDays: 30, Enabled: true},
		// Channels that only publish a "latest" snapshot.
		{ChannelID: "finmind", CoverageType: CoverageLatestFile, FilePattern: "latest.json", LookbackDays: 90, Enabled: true},
		{ChannelID: "fugle", CoverageType: CoverageLatestFile, FilePattern: "latest.json", LookbackDays: 7, Enabled: true},
		{ChannelID: "fubon", CoverageType: CoverageLatestFile, FilePattern: "latest.json", LookbackDays: 7, Enabled: true},
		{ChannelID: "geopolitical", CoverageType: CoverageLatestFile, FilePattern: "latest.json", LookbackDays: 7, Enabled: true},
		// Stubs / file-backed / conditional channels with no uniform date layout.
		{ChannelID: "tdcc_equity_dispersion", CoverageType: CoverageNone, Enabled: false},
		{ChannelID: "twse_sbl", CoverageType: CoverageNone, Enabled: false},
		{ChannelID: "janus_regime", CoverageType: CoverageNone, Enabled: false},
		{ChannelID: "sector_data", CoverageType: CoverageNone, Enabled: false},
	}
}

// tradingDayChecker is the subset of industry.EventCalendar used by the gap
// detector. It keeps the implementation testable without a full calendar
// provider setup.
type tradingDayChecker interface {
	IsTaiwanTradingDay(date time.Time) bool
}

// ChannelGapReport is the per-channel result produced by a scan.
type ChannelGapReport struct {
	ChannelID     string   `json:"channel_id"`
	ExpectedCount int      `json:"expected_count"`
	MissingCount  int      `json:"missing_count"`
	MissingDates  []string `json:"missing_dates"`
	LatestDate    string   `json:"latest_date,omitempty"`
	Error         string   `json:"error,omitempty"`
}

// GapReport is the top-level report written to disk.
type GapReport struct {
	GeneratedAt time.Time          `json:"generated_at"`
	Channels    []ChannelGapReport `json:"channels"`
}

// gapDetector scans data/state directories and produces a gap report.
type gapDetector struct {
	workDir      string
	calendar     tradingDayChecker
	expectations []ChannelCoverageExpectation
}

// newGapDetector creates a detector with the default expectations.
func newGapDetector(workDir string, calendar tradingDayChecker) *gapDetector {
	return &gapDetector{
		workDir:      workDir,
		calendar:     calendar,
		expectations: defaultChannelCoverageExpectations(),
	}
}

// detect scans all enabled expectations and returns the gap report.
func (g *gapDetector) detect(reference time.Time) GapReport {
	report := GapReport{GeneratedAt: reference, Channels: make([]ChannelGapReport, 0, len(g.expectations))}
	for _, exp := range g.expectations {
		if !exp.Enabled {
			continue
		}
		report.Channels = append(report.Channels, g.detectChannel(reference, exp))
	}
	return report
}

func (g *gapDetector) detectChannel(reference time.Time, exp ChannelCoverageExpectation) ChannelGapReport {
	switch exp.CoverageType {
	case CoverageDailyFiles:
		return g.detectDailyFiles(reference, exp)
	case CoverageLatestFile:
		return g.detectLatestFile(reference, exp)
	case CoverageNone:
		return ChannelGapReport{ChannelID: exp.ChannelID, ExpectedCount: 0, MissingCount: 0}
	default:
		return ChannelGapReport{ChannelID: exp.ChannelID, Error: fmt.Sprintf("unknown coverage type %q", exp.CoverageType)}
	}
}

func (g *gapDetector) detectDailyFiles(reference time.Time, exp ChannelCoverageExpectation) ChannelGapReport {
	report := ChannelGapReport{ChannelID: exp.ChannelID}
	end := reference.AddDate(0, 0, -1) // yesterday
	start := reference.AddDate(0, 0, -exp.LookbackDays)

	var expected, missing []string
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if !g.calendar.IsTaiwanTradingDay(d) {
			continue
		}
		dateStr := d.Format("2006-01-02")
		fileName := d.Format(exp.FilePattern)
		filePath := filepath.Join(g.workDir, "data", "state", exp.ChannelID, fileName)
		expected = append(expected, dateStr)
		if _, err := os.Stat(filePath); err != nil {
			missing = append(missing, dateStr)
		}
	}
	report.ExpectedCount = len(expected)
	report.MissingCount = len(missing)
	report.MissingDates = missing
	return report
}

func (g *gapDetector) detectLatestFile(reference time.Time, exp ChannelCoverageExpectation) ChannelGapReport {
	report := ChannelGapReport{ChannelID: exp.ChannelID}
	filePath := filepath.Join(g.workDir, "data", "state", exp.ChannelID, exp.FilePattern)
	info, err := os.Stat(filePath)
	if err != nil {
		report.Error = fmt.Sprintf("%s not accessible: %v", exp.FilePattern, err)
		report.MissingCount = 1
		report.MissingDates = []string{reference.Format("2006-01-02")}
		return report
	}
	ageDays := int(reference.Sub(info.ModTime()).Hours() / 24)
	report.LatestDate = info.ModTime().Format("2006-01-02")
	if ageDays > exp.LookbackDays {
		report.MissingCount = 1
		report.MissingDates = []string{report.LatestDate}
		report.Error = fmt.Sprintf("latest file is %d days old (threshold %d)", ageDays, exp.LookbackDays)
	}
	return report
}

// writeReport writes the report to data/state/gap_report.json.
func (g *gapDetector) writeReport(report GapReport) error {
	reportPath := filepath.Join(g.workDir, "data", "state", "gap_report.json")
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		return fmt.Errorf("create gap report dir: %w", err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal gap report: %w", err)
	}
	if err := os.WriteFile(reportPath, data, 0o644); err != nil {
		return fmt.Errorf("write gap report: %w", err)
	}
	return nil
}

// emitAlerts raises a single consolidated alert per channel with missing data.
func (g *gapDetector) emitAlerts(monitor *monitoring.Monitor, report GapReport) {
	if monitor == nil {
		return
	}
	for _, ch := range report.Channels {
		if ch.MissingCount == 0 && ch.Error == "" {
			continue
		}
		msg := fmt.Sprintf("channel %s has %d missing coverage date(s)", ch.ChannelID, ch.MissingCount)
		if ch.Error != "" {
			msg = fmt.Sprintf("channel %s coverage check failed: %s", ch.ChannelID, ch.Error)
		}
		monitor.Alert(monitoring.AlertLevelWarning, "data_gap",
			msg,
			map[string]any{
				"channel":       ch.ChannelID,
				"missing_count": ch.MissingCount,
				"missing_dates": ch.MissingDates,
				"error":         ch.Error,
			})
	}
}

// backfillDeps groups the dependencies needed by backfill tasks.
type backfillDeps struct {
	taskMgr   *apigateway.BackgroundTaskManager
	cfg       config.Config
	monitor   *monitoring.Monitor
	calendar  tradingDayChecker
	collector *monitoring.MetricsCollector
}

// registerBackfillTasks wires the gap-detection background task into the
// BackgroundTaskManager. Errors are logged and the task is silently dropped,
// matching the existing pattern in other register*Tasks functions.
func registerBackfillTasks(d backfillDeps) {
	calendar := d.calendar
	if calendar == nil {
		calendar = industry.NewEventCalendar()
	}
	detector := newGapDetector(d.cfg.WorkDir, calendar)

	_ = d.taskMgr.Register(&apigateway.ScheduledTask{
		Name:      "auto_gap_detection",
		ChannelID: "",
		Interval:  24 * time.Hour,
		Enabled:   true,
		Task: func(ctx context.Context) error {
			reference := time.Now()
			if tz, err := time.LoadLocation("Asia/Taipei"); err == nil {
				reference = reference.In(tz)
			}
			report := detector.detect(reference)
			if err := detector.writeReport(report); err != nil {
				logMsg := fmt.Sprintf("[Gateway] auto_gap_detection report failed: %v", err)
				if d.monitor != nil {
					d.monitor.Alert(monitoring.AlertLevelError, "data_gap", logMsg, map[string]any{"error": err.Error()})
				}
				return fmt.Errorf("write gap report: %w", err)
			}
			detector.emitAlerts(d.monitor, report)

			var sb strings.Builder
			totalMissing := 0
			for _, ch := range report.Channels {
				totalMissing += ch.MissingCount
				if ch.MissingCount > 0 {
					fmt.Fprintf(&sb, " %s:%d", ch.ChannelID, ch.MissingCount)
				}
			}
			if totalMissing > 0 {
				log.Printf("[Gateway] auto_gap_detection found %d missing date(s):%s", totalMissing, sb.String())
			} else {
				log.Printf("[Gateway] auto_gap_detection found no gaps")
			}
			return nil
		},
	})
	log.Printf("[Gateway] registered auto_gap_detection background task (24h interval)")

	registerChannelHealthMetricsTask(d)
}
