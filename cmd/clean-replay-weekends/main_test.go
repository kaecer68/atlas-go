package main

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixtureJSONL builds a replay jsonl with trading days (Fri 2026-07-03, Mon
// 2026-07-06), weekend rows (Sat 2026-07-04, Sun 2026-07-05), and a weekday
// Taiwan holiday (Tue 2026-02-17, 春節初一), for two symbols.
const fixtureJSONL = `{"date":"2026-07-03T00:00:00Z","symbol":"2330.TW","name":"台積電","open":1000,"high":1010,"low":990,"close":1005,"volume":100000,"source":"twse_open_data_csv"}
{"date":"2026-07-04T00:00:00Z","symbol":"2330.TW","name":"台積電","open":1005,"high":1015,"low":995,"close":1008,"volume":99999,"source":"twse_open_data_csv"}
{"date":"2026-07-05T00:00:00Z","symbol":"2330.TW","name":"台積電","open":1008,"high":1018,"low":998,"close":1010,"volume":99998,"source":"twse_open_data_csv"}
{"date":"2026-07-06T00:00:00Z","symbol":"2330.TW","name":"台積電","open":1010,"high":1020,"low":1000,"close":1015,"volume":110000,"source":"twse_open_data_csv"}
{"date":"2026-02-17","symbol":"0050.TW","name":"0050","open":190,"high":191,"low":189,"close":190.5,"volume":12345,"source":"finmind"}
{"date":"2026-07-03","symbol":"0050.TW","name":"0050","open":191,"high":192,"low":190,"close":191.5,"volume":13000,"source":"finmind"}
{"date":"2026-07-04","symbol":"0050.TW","name":"0050","open":191.5,"high":192.5,"low":190.5,"close":192,"volume":13001,"source":"finmind"}
`

func writeFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestClassifyDate(t *testing.T) {
	cases := []struct {
		date string
		want string
	}{
		{"2026-07-03", "trading"}, // Fri
		{"2026-07-04", "weekend"}, // Sat
		{"2026-07-05", "weekend"}, // Sun
		{"2026-07-06", "trading"}, // Mon
		{"2026-02-17", "holiday"}, // Tue 春節初一
		{"2026-06-19", "holiday"}, // Fri 端午
	}
	for _, c := range cases {
		d, err := time.Parse("2006-01-02", c.date)
		if err != nil {
			t.Fatal(err)
		}
		if got := classifyDate(d); got != c.want {
			t.Errorf("classifyDate(%s) = %q, want %q", c.date, got, c.want)
		}
	}
}

func TestCleanJSONL_RemovesWeekendsAndHolidays(t *testing.T) {
	dir := t.TempDir()
	p := writeFixture(t, dir, "input.jsonl", fixtureJSONL)

	keptRows, keptLines, st, err := cleanJSONL(p)
	if err != nil {
		t.Fatalf("cleanJSONL: %v", err)
	}
	if st.Scanned != 7 {
		t.Errorf("scanned = %d, want 7", st.Scanned)
	}
	if st.Removed != 4 || st.RemovedWeekend != 3 || st.RemovedHoliday != 1 {
		t.Errorf("removed = %d (weekend=%d holiday=%d), want 4 (3,1)",
			st.Removed, st.RemovedWeekend, st.RemovedHoliday)
	}
	if st.Kept != 3 {
		t.Errorf("kept = %d, want 3", st.Kept)
	}
	// trading days preserved, holiday + weekends gone
	if len(keptRows) != 3 || len(keptLines) != 3 {
		t.Fatalf("keptRows=%d keptLines=%d, want 3 each", len(keptRows), len(keptLines))
	}
	gotDates := map[string]int{}
	for _, r := range keptRows {
		gotDates[r.Date[:10]]++
	}
	if gotDates["2026-07-04"] != 0 || gotDates["2026-07-05"] != 0 || gotDates["2026-02-17"] != 0 {
		t.Errorf("non-trading rows survived: %v", gotDates)
	}
	if gotDates["2026-07-03"] != 2 || gotDates["2026-07-06"] != 1 {
		t.Errorf("trading rows wrong: %v", gotDates)
	}
}

func TestRun_EndToEnd_RegeneratesMergedCSV(t *testing.T) {
	dir := t.TempDir()
	in := writeFixture(t, dir, "input.jsonl", fixtureJSONL)
	mergedPath := filepath.Join(dir, "merged.csv")

	args := []string{"-inputs", in, "-output", mergedPath}
	if err := run(args); err != nil {
		t.Fatalf("run: %v", err)
	}

	// merged.csv exists with 3 unique bars, no weekend/holiday dates
	f, err := os.Open(mergedPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	header, _ := r.Read()
	if strings.Join(header, ",") != "Date,Code,Name,TradeVolume,Open,High,Low,Close" {
		t.Errorf("unexpected header: %v", header)
	}
	records, err := r.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("merged.csv rows = %d, want 3", len(records))
	}
	for _, rec := range records {
		d, _ := time.Parse("2006-01-02", rec[0])
		if classifyDate(d) != "trading" {
			t.Errorf("merged.csv contains non-trading row: %v", rec)
		}
	}
	// original jsonl untouched without -rewrite
	src, _ := os.ReadFile(in)
	if !strings.Contains(string(src), "2026-07-04") {
		t.Errorf("input jsonl was modified without -rewrite")
	}
}

func TestRun_Rewrite_RemovesRowsInPlace(t *testing.T) {
	dir := t.TempDir()
	in := writeFixture(t, dir, "input.jsonl", fixtureJSONL)
	mergedPath := filepath.Join(dir, "merged.csv")

	args := []string{"-inputs", in, "-output", mergedPath, "-rewrite"}
	if err := run(args); err != nil {
		t.Fatalf("run: %v", err)
	}
	after, _ := os.ReadFile(in)
	for _, bad := range []string{"2026-07-04", "2026-07-05", "2026-02-17"} {
		if strings.Contains(string(after), bad) {
			t.Errorf("rewritten jsonl still contains %s", bad)
		}
	}
	// trading rows still present
	for _, good := range []string{"2026-07-03", "2026-07-06"} {
		if !strings.Contains(string(after), good) {
			t.Errorf("rewritten jsonl lost trading day %s", good)
		}
	}
	if n := strings.Count(string(after), "\n"); n != 3 {
		t.Errorf("rewritten jsonl lines = %d, want 3", n)
	}
}

func TestRun_CSVInput(t *testing.T) {
	dir := t.TempDir()
	csvContent := "Date,Code,Name,TradeVolume,Open,High,Low,Close\n" +
		"2026-07-03,2330,台積電,100000,1000,1010,990,1005\n" +
		"2026-07-04,2330,台積電,99999,1005,1015,995,1008\n" +
		"2026-07-06,2330,台積電,110000,1010,1020,1000,1015\n"
	in := writeFixture(t, dir, "input.csv", csvContent)
	mergedPath := filepath.Join(dir, "merged.csv")

	if err := run([]string{"-inputs", in, "-output", mergedPath}); err != nil {
		t.Fatalf("run csv: %v", err)
	}
	f, err := os.Open(mergedPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	_, _ = r.Read()
	records, _ := r.ReadAll()
	if len(records) != 2 {
		t.Fatalf("merged rows = %d, want 2", len(records))
	}
	for _, rec := range records {
		d, _ := time.Parse("2006-01-02", rec[0])
		if classifyDate(d) != "trading" {
			t.Errorf("merged.csv contains non-trading row: %v", rec)
		}
	}
}

func TestRun_RequiresInputs(t *testing.T) {
	if err := run([]string{"-output", filepath.Join(t.TempDir(), "m.csv")}); err == nil {
		t.Fatal("expected error when -inputs empty, got nil")
	}
}

func TestRun_DryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	in := writeFixture(t, dir, "input.jsonl", fixtureJSONL)
	mergedPath := filepath.Join(dir, "merged.csv")
	if err := run([]string{"-inputs", in, "-output", mergedPath, "-dry-run", "-rewrite"}); err != nil {
		t.Fatalf("run dry-run: %v", err)
	}
	if _, err := os.Stat(mergedPath); !os.IsNotExist(err) {
		t.Error("dry-run wrote merged CSV")
	}
	src, _ := os.ReadFile(in)
	if strings.Count(string(src), "\n") != 7 {
		t.Error("dry-run modified input")
	}
}
