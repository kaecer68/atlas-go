package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFundamentals(t *testing.T, dir string, data map[string]rawFundamental) {
	t.Helper()
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal fundamentals: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		t.Fatalf("mkdir data: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data", "fundamentals.json"), b, 0o644); err != nil {
		t.Fatalf("write fundamentals: %v", err)
	}
}

func runApp(t *testing.T, dir, date string, force bool, now func() time.Time) {
	t.Helper()
	a := &app{workDir: dir, date: date, force: force, now: now}
	if err := a.run(); err != nil {
		t.Fatalf("run failed: %v", err)
	}
}

func countHistoryLines(t *testing.T, dir string) int {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, "data", "fundamentals_history.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("open history: %v", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	n := 0
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			n++
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan history: %v", err)
	}
	return n
}

func readHistory(t *testing.T, dir string) []historyRecord {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, "data", "fundamentals_history.jsonl"))
	if err != nil {
		t.Fatalf("open history: %v", err)
	}
	defer f.Close()
	var records []historyRecord
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec historyRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("unmarshal history line %q: %v", line, err)
		}
		records = append(records, rec)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan history: %v", err)
	}
	return records
}

func TestAppend_Idempotent(t *testing.T) {
	dir := t.TempDir()
	writeFundamentals(t, dir, map[string]rawFundamental{
		"2330.TW": {PE: new(18.5), PB: new(2.1), DividendYield: new(2.5), Sector: new("半導體業")},
		"2317.TW": {PE: new(12.3), PB: new(1.5), DividendYield: new(4.0), Sector: new("電子零組件業")},
	})

	runApp(t, dir, "2026-08-27", false, nil)
	first := countHistoryLines(t, dir)

	runApp(t, dir, "2026-08-27", false, nil)
	second := countHistoryLines(t, dir)

	if first != 2 || second != 2 || first != second {
		t.Fatalf("expected 2 lines after each run, got first=%d second=%d", first, second)
	}
}

func TestAppend_MultipleDates(t *testing.T) {
	dir := t.TempDir()
	writeFundamentals(t, dir, map[string]rawFundamental{
		"2330.TW": {PE: new(18.5), PB: new(2.1), DividendYield: new(2.5), Sector: new("半導體業")},
	})

	runApp(t, dir, "2026-08-25", false, nil)
	if got := countHistoryLines(t, dir); got != 1 {
		t.Fatalf("after first date expected 1 line, got %d", got)
	}

	runApp(t, dir, "2026-08-26", false, nil)
	if got := countHistoryLines(t, dir); got != 2 {
		t.Fatalf("after second date expected 2 lines, got %d", got)
	}

	records := readHistory(t, dir)
	if records[0].Date != "2026-08-25" || records[1].Date != "2026-08-26" {
		t.Fatalf("unexpected dates: %v", records)
	}
}

func TestAppend_ForceOverwrite(t *testing.T) {
	dir := t.TempDir()
	writeFundamentals(t, dir, map[string]rawFundamental{
		"2330.TW": {PE: new(18.5), PB: new(2.1), DividendYield: new(2.5), Sector: new("半導體業")},
	})
	runApp(t, dir, "2026-08-27", false, nil)

	writeFundamentals(t, dir, map[string]rawFundamental{
		"2330.TW": {PE: new(19.0), PB: new(2.2), DividendYield: new(2.6), Sector: new("半導體業")},
	})
	runApp(t, dir, "2026-08-27", true, nil)

	records := readHistory(t, dir)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	if records[0].PE != 19.0 {
		t.Fatalf("expected overwritten PE=19.0, got %v", records[0].PE)
	}
}

// TestAppend_RecordedAt verifies the recorded_at timestamp is derived from the
// trading date (not silently normalized to a wrong month/day due to scanf).
func TestAppend_RecordedAt(t *testing.T) {
	dir := t.TempDir()
	writeFundamentals(t, dir, map[string]rawFundamental{
		"2330.TW": {PE: new(18.5), PB: new(2.1), DividendYield: new(2.5), Sector: new("半導體業")},
	})
	runApp(t, dir, "2026-08-27", false, nil)

	records := readHistory(t, dir)
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	want := "2026-08-27T15:30:00+08:00"
	if records[0].RecordedAt != want {
		t.Fatalf("recorded_at = %s, want %s (date parsing bug?)", records[0].RecordedAt, want)
	}
}

func TestAppend_MissingFundamentals(t *testing.T) {
	dir := t.TempDir()
	a := &app{workDir: dir, date: "2026-08-27", now: time.Now}
	err := a.run()
	if err == nil {
		t.Fatal("expected error when fundamentals.json is missing")
	}
	if !strings.Contains(err.Error(), "fundamentals.json") {
		t.Fatalf("error should mention fundamentals.json, got: %v", err)
	}
}

func TestAppend_SkipsBadSymbol(t *testing.T) {
	dir := t.TempDir()
	writeFundamentals(t, dir, map[string]rawFundamental{
		"2330.TW": {PE: new(18.5), PB: new(2.1), DividendYield: new(2.5), Sector: new("半導體業")},
		"9999.TW": {PE: new(1.0), DividendYield: new(1.0), Sector: new("壞資料")}, // missing PB
	})

	runApp(t, dir, "2026-08-27", false, nil)
	records := readHistory(t, dir)
	if len(records) != 1 || records[0].Symbol != "2330" {
		t.Fatalf("expected only 2330, got %+v", records)
	}
}

func TestAppend_TradingDateAfterClose(t *testing.T) {
	dir := t.TempDir()
	writeFundamentals(t, dir, map[string]rawFundamental{
		"2330.TW": {PE: new(18.5), PB: new(2.1), DividendYield: new(2.5), Sector: new("半導體業")},
	})

	loc := time.FixedZone("Asia/Taipei", taipeiOffsetSeconds)
	afterClose := time.Date(2026, 8, 27, 14, 0, 0, 0, loc)
	runApp(t, dir, "", false, func() time.Time { return afterClose })

	records := readHistory(t, dir)
	if len(records) != 1 || records[0].Date != "2026-08-27" {
		t.Fatalf("expected date 2026-08-27, got %+v", records)
	}
}

func TestAppend_TradingDateBeforeClose(t *testing.T) {
	dir := t.TempDir()
	writeFundamentals(t, dir, map[string]rawFundamental{
		"2330.TW": {PE: new(18.5), PB: new(2.1), DividendYield: new(2.5), Sector: new("半導體業")},
	})

	loc := time.FixedZone("Asia/Taipei", taipeiOffsetSeconds)
	beforeClose := time.Date(2026, 8, 27, 10, 0, 0, 0, loc)
	runApp(t, dir, "", false, func() time.Time { return beforeClose })

	records := readHistory(t, dir)
	if len(records) != 1 || records[0].Date != "2026-08-26" {
		t.Fatalf("expected previous weekday 2026-08-26, got %+v", records)
	}
}

func TestAppend_WeekendRollover(t *testing.T) {
	dir := t.TempDir()
	writeFundamentals(t, dir, map[string]rawFundamental{
		"2330.TW": {PE: new(18.5), PB: new(2.1), DividendYield: new(2.5), Sector: new("半導體業")},
	})

	loc := time.FixedZone("Asia/Taipei", taipeiOffsetSeconds)
	mondayMorning := time.Date(2026, 9, 7, 10, 0, 0, 0, loc)
	runApp(t, dir, "", false, func() time.Time { return mondayMorning })

	records := readHistory(t, dir)
	if len(records) != 1 || records[0].Date != "2026-09-04" {
		t.Fatalf("expected previous Friday 2026-09-04, got %+v", records)
	}
}

// fp returns a pointer to a float64 to simplify test fixture construction.
//
//go:fix inline
func fp(v float64) *float64 { return new(v) }

// sp returns a pointer to a string to simplify test fixture construction.
//
//go:fix inline
func sp(v string) *string { return new(v) }
