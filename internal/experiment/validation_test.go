package experiment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeTestCSV writes a TWSE-format CSV with the given rows (header added automatically).
func writeTestCSV(t *testing.T, path string, rows string) {
	t.Helper()
	content := "Date,Code,Name,TradeVolume,Open,High,Low,Close\n" + rows
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test CSV: %v", err)
	}
}

func TestValidateReplayData_Success(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "replay.csv")
	writeTestCSV(t, csvPath, `2026-03-20,0050,Test,1000000,100,110,90,105
2026-03-25,0050,Test,1000000,105,115,95,110
2026-03-27,0050,Test,1000000,110,120,100,115
`)

	windowStart := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	meta, err := ValidateReplayData(windowStart, windowEnd, csvPath)
	if err != nil {
		t.Fatalf("ValidateReplayData returned error: %v", err)
	}
	if meta == nil {
		t.Fatal("expected non-nil meta")
	}
	if meta.RecordCount != 3 {
		t.Errorf("expected RecordCount=3, got %d", meta.RecordCount)
	}
	if !meta.CoversWindow {
		t.Error("expected CoversWindow=true")
	}
	if meta.SourcePath != csvPath {
		t.Errorf("expected SourcePath=%s, got %s", csvPath, meta.SourcePath)
	}
}

func TestValidateReplayData_FileNotFound(t *testing.T) {
	windowStart := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	meta, err := ValidateReplayData(windowStart, windowEnd, "/nonexistent/path/replay.csv")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
	if meta != nil {
		t.Error("expected nil meta on file not found")
	}
}

func TestValidateReplayData_EmptyData(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "empty.csv")
	// Write CSV with only header — no data rows
	writeTestCSV(t, csvPath, "")

	windowStart := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	meta, err := ValidateReplayData(windowStart, windowEnd, csvPath)
	if err == nil {
		t.Fatal("expected error for empty data")
	}
	if meta != nil {
		t.Error("expected nil meta on empty data")
	}
}

func TestValidateReplayData_InsufficientData(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "short.csv")
	// Data ends at 2026-03-22, but window ends at 2026-03-27
	writeTestCSV(t, csvPath, `2026-03-20,0050,Test,1000000,100,110,90,105
2026-03-22,0050,Test,1000000,105,115,95,110
`)

	windowStart := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	windowEnd := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)

	meta, err := ValidateReplayData(windowStart, windowEnd, csvPath)
	if err == nil {
		t.Fatal("expected error for insufficient data")
	}
	// meta should still be returned with partial info
	if meta == nil {
		t.Fatal("expected non-nil meta even on insufficient data error")
	}
	if meta.RecordCount != 2 {
		t.Errorf("expected RecordCount=2, got %d", meta.RecordCount)
	}
}

func TestReplayLatestDate_ReturnsLatestDate(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "replay.csv")
	writeTestCSV(t, csvPath, `2026-03-20,0050,Test,1000000,100,110,90,105
2026-03-25,0050,Test,1000000,105,115,95,110
2026-03-27,0050,Test,1000000,110,120,100,115
`)

	latest, err := ReplayLatestDate(csvPath)
	if err != nil {
		t.Fatalf("ReplayLatestDate returned error: %v", err)
	}
	expected := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	if !latest.Equal(expected) {
		t.Errorf("expected latest=%v, got %v", expected, latest)
	}
}

func TestReplayLatestDate_FileNotFound(t *testing.T) {
	_, err := ReplayLatestDate("/nonexistent/path/replay.csv")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
}

func TestReplayLatestDate_EmptyData(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "empty.csv")
	// Header only — no data rows
	writeTestCSV(t, csvPath, "")
	if _, err := ReplayLatestDate(csvPath); err == nil {
		t.Fatal("expected error for empty data")
	}
}

// TestResolveExperimentWindow_OneDayStaleDefers — 缺 1 天：窗口整窗平移對齊
// latestDate（[latestDate-6d, latestDate]，長度恆 7 天不縮短），不失敗。
func TestResolveExperimentWindow_OneDayStaleDefers(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "replay.csv")
	// replay 最後日期 = 2026-03-27，now = 2026-03-28 → 缺 1 天
	writeTestCSV(t, csvPath, `2026-03-20,0050,Test,1000000,100,110,90,105
2026-03-25,0050,Test,1000000,105,115,95,110
2026-03-27,0050,Test,1000000,110,120,100,115
`)
	now := time.Date(2026, 3, 28, 14, 0, 0, 0, time.UTC)

	start, end, deferred, err := resolveExperimentWindow(now, csvPath)
	if err != nil {
		t.Fatalf("resolveExperimentWindow returned error for 1-day stale replay: %v", err)
	}
	if !deferred {
		t.Fatal("expected deferred=true for 1-day stale replay")
	}
	expectedEnd := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	expectedStart := time.Date(2026, 3, 21, 0, 0, 0, 0, time.UTC)
	if !end.Equal(expectedEnd) {
		t.Errorf("expected window end=%v, got %v", expectedEnd, end)
	}
	if !start.Equal(expectedStart) {
		t.Errorf("expected window start=%v, got %v", expectedStart, start)
	}
}

// TestResolveExperimentWindow_TwoDaysStaleFails — 缺 ≥2 天：仍失敗，錯誤提示
// 檢查 daily-replay-sync。
func TestResolveExperimentWindow_TwoDaysStaleFails(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "replay.csv")
	// replay 最後日期 = 2026-03-26，now = 2026-03-28 → 缺 2 天
	writeTestCSV(t, csvPath, `2026-03-20,0050,Test,1000000,100,110,90,105
2026-03-26,0050,Test,1000000,105,115,95,110
`)
	now := time.Date(2026, 3, 28, 14, 0, 0, 0, time.UTC)

	_, _, deferred, err := resolveExperimentWindow(now, csvPath)
	if err == nil {
		t.Fatal("expected error for 2-day stale replay")
	}
	if deferred {
		t.Error("expected deferred=false for 2-day stale replay")
	}
	if !strings.Contains(err.Error(), "daily-replay-sync") {
		t.Errorf("expected error to mention daily-replay-sync, got: %v", err)
	}
}

// TestResolveExperimentWindow_FreshDataKeepsDefaultWindow — 資料新鮮（同一天）
// 時維持原窗口 [now-7d, now]，不順延。
func TestResolveExperimentWindow_FreshDataKeepsDefaultWindow(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "replay.csv")
	// replay 最後日期 = 2026-03-28 = now 的日期 → 新鮮
	writeTestCSV(t, csvPath, `2026-03-20,0050,Test,1000000,100,110,90,105
2026-03-28,0050,Test,1000000,110,120,100,115
`)
	now := time.Date(2026, 3, 28, 14, 0, 0, 0, time.UTC)

	start, end, deferred, err := resolveExperimentWindow(now, csvPath)
	if err != nil {
		t.Fatalf("resolveExperimentWindow returned error for fresh replay: %v", err)
	}
	if deferred {
		t.Error("expected deferred=false for fresh replay")
	}
	expectedStart := now.Add(-7 * 24 * time.Hour)
	if !start.Equal(expectedStart) {
		t.Errorf("expected start=%v, got %v", expectedStart, start)
	}
	if !end.Equal(now) {
		t.Errorf("expected end=%v, got %v", now, end)
	}
}

// TestResolveExperimentWindow_EmptyPathSkipsGate — replay 路徑未設定時不做
// 新鮮度檢查，維持原窗口（由 executor 的驗證照常處理）。
func TestResolveExperimentWindow_EmptyPathSkipsGate(t *testing.T) {
	now := time.Date(2026, 3, 28, 14, 0, 0, 0, time.UTC)

	start, end, deferred, err := resolveExperimentWindow(now, "")
	if err != nil {
		t.Fatalf("resolveExperimentWindow returned error for empty path: %v", err)
	}
	if deferred {
		t.Error("expected deferred=false when replay path is empty")
	}
	expectedStart := now.Add(-7 * 24 * time.Hour)
	if !start.Equal(expectedStart) || !end.Equal(now) {
		t.Errorf("expected default window [%v, %v], got [%v, %v]", expectedStart, now, start, end)
	}
}

func TestParseWindowDates_Valid(t *testing.T) {
	start, end, err := parseWindowDates("window-20260320-20260327")
	if err != nil {
		t.Fatalf("parseWindowDates returned error: %v", err)
	}
	expectedStart := time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)
	expectedEnd := time.Date(2026, 3, 27, 0, 0, 0, 0, time.UTC)
	if !start.Equal(expectedStart) {
		t.Errorf("expected start=%v, got %v", expectedStart, start)
	}
	if !end.Equal(expectedEnd) {
		t.Errorf("expected end=%v, got %v", expectedEnd, end)
	}
}

func TestParseWindowDates_Invalid(t *testing.T) {
	_, _, err := parseWindowDates("bad-format")
	if err == nil {
		t.Fatal("expected error for invalid window ID format")
	}

	_, _, err = parseWindowDates("window-2026ABCD-20260327")
	if err == nil {
		t.Fatal("expected error for invalid date format in start")
	}

	_, _, err = parseWindowDates("window-20260320-2026ABCD")
	if err == nil {
		t.Fatal("expected error for invalid date format in end")
	}
}
