package experiment

import (
	"os"
	"path/filepath"
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
