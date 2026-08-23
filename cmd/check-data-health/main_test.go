package main

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeReplayCSV writes a replay CSV (production format) containing rows for
// every (date, symbol) pair and returns its path.
func writeReplayCSV(t *testing.T, dates []string, symbols []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "replay.csv")
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"Date", "Code", "Name", "TradeVolume", "Open", "High", "Low", "Close"})
	for _, date := range dates {
		for _, sym := range symbols {
			_ = w.Write([]string{date, sym, "測試股", "15000000", "100.00", "101.00", "99.00", "100.50"})
		}
	}
	w.Flush()
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write replay csv: %v", err)
	}
	return path
}

// tradingDays is the set of Taiwan trading days (per marketdata calendar)
// inside the 14-day lookback window ending 2026-08-21.
var tradingDays = []string{
	"2026-08-10", "2026-08-11", "2026-08-12", "2026-08-13", "2026-08-14",
	"2026-08-17", "2026-08-18", "2026-08-19", "2026-08-20", "2026-08-21",
}

// TestRunReportsMissingDayAndExitError is the core regression test: the CSV's
// latest date is fresh (2026-08-21, delay 1 day — inside the normal range),
// but 2026-08-20 is missing in the middle. The check must report the gap and
// return an error (main() turns it into exit code 1).
func TestRunReportsMissingDayAndExitError(t *testing.T) {
	dates := make([]string, 0, len(tradingDays))
	for _, d := range tradingDays {
		if d != "2026-08-20" {
			dates = append(dates, d)
		}
	}
	path := writeReplayCSV(t, dates, []string{"2330", "0050"})

	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) // Saturday
	var out bytes.Buffer
	err := run(path, now, &out)

	if err == nil {
		t.Fatalf("run() = nil, want error (missing day 2026-08-20)")
	}
	if !strings.Contains(err.Error(), "2026-08-20") {
		t.Errorf("error %q does not mention missing day 2026-08-20", err.Error())
	}
	if !strings.Contains(err.Error(), "缺 1 個交易日") {
		t.Errorf("error %q does not mention gap count", err.Error())
	}
	if !strings.Contains(out.String(), "⚠️  缺日: 2026-08-20") {
		t.Errorf("output missing gap line:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "建議執行: go run ./cmd/daily-replay-sync（它會自動補抓缺日）") {
		t.Errorf("output missing backfill suggestion:\n%s", out.String())
	}
	// Delay is inside the normal range — only the gap drives the failure.
	if !strings.Contains(out.String(), "✅ 數據延遲: 1 天（正常範圍內）") {
		t.Errorf("output missing healthy delay line:\n%s", out.String())
	}
}

// TestRunHealthyNoGap verifies a complete CSV (every trading day in the
// window, fresh latest date) passes with nil error.
func TestRunHealthyNoGap(t *testing.T) {
	path := writeReplayCSV(t, tradingDays, []string{"2330", "0050"})
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)

	var out bytes.Buffer
	if err := run(path, now, &out); err != nil {
		t.Fatalf("run() = %v, want nil", err)
	}
	if !strings.Contains(out.String(), "✅ 缺日檢查: 近 14 日無缺日") {
		t.Errorf("output missing healthy gap line:\n%s", out.String())
	}
}

// TestRunReportsDelayStillFails preserves the existing delay gate: a stale
// latest date (> 2 days) still returns an error.
func TestRunReportsDelayStillFails(t *testing.T) {
	// CSV ends at 2026-08-10; check runs 12 days later.
	path := writeReplayCSV(t, []string{"2026-08-10"}, []string{"2330"})
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)

	var out bytes.Buffer
	err := run(path, now, &out)
	if err == nil {
		t.Fatalf("run() = nil, want error for 12-day delay")
	}
	if !strings.Contains(err.Error(), "延遲 12 天") {
		t.Errorf("error %q does not mention delay", err.Error())
	}
	if !strings.Contains(out.String(), "⚠️  數據延遲: 12 天") {
		t.Errorf("output missing delay warning:\n%s", out.String())
	}
}
