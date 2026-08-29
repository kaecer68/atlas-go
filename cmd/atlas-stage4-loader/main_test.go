// File: main_test.go
// Package: main (cmd/atlas-stage4-loader)
//
// End-to-end tests for the loader CLI. Each test writes fixture JSONLs
// to t.TempDir() and then exercises Run with a temp SQLite DB.
package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/ledger"
)

// ------------------------------------------------------------------
// Test fixtures
// ------------------------------------------------------------------

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeRegimeJSONL(t *testing.T, path string, rows int) {
	t.Helper()
	var b strings.Builder
	for i := range rows {
		date := time.Date(2026, 4, i+1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		r := loaderRegime{
			Date:            date,
			Regime:          "RISK_ON",
			SourceSessionID: "session-" + date + "-daily",
			RecordedAt:      time.Date(2026, 4, i+1, 0, 0, 0, 0, time.UTC),
			CapturedAt:      time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
			IsSynthetic:     1,
		}
		b.WriteString(mustJSON(t, r))
		b.WriteString("\n")
	}
	mustWriteFile(t, path, b.String())
}

func writeStressJSONL(t *testing.T, path string, rows int) {
	t.Helper()
	var b strings.Builder
	for i := range rows {
		date := time.Date(2026, 4, i+1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		r := loaderStress{
			Date:        date,
			Score:       float64(i+1) / 10.0,
			Regime:      "low",
			Source:      "macro-file",
			CapturedAt:  time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
			IsSynthetic: 1,
		}
		b.WriteString(mustJSON(t, r))
		b.WriteString("\n")
	}
	mustWriteFile(t, path, b.String())
}

func writeEventJSONL(t *testing.T, path string, rows int) {
	t.Helper()
	var b strings.Builder
	for i := range rows {
		date := time.Date(2026, 4, i+1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		r := loaderEvent{
			Date:        date,
			EventIDs:    []string{"evt-tech-" + date, "evt-dividend-" + date},
			Source:      "session-derive",
			ActiveTheme: []string{"tech-peak", "dividend"},
			CapturedAt:  time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
			IsSynthetic: 1,
		}
		b.WriteString(mustJSON(t, r))
		b.WriteString("\n")
	}
	mustWriteFile(t, path, b.String())
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func makeStaging(t *testing.T, dir string, regimeN, stressN, eventN int) {
	t.Helper()
	writeRegimeJSONL(t, filepath.Join(dir, "regime_history_90d.jsonl"), regimeN)
	writeStressJSONL(t, filepath.Join(dir, "stress_index_history_90d.jsonl"), stressN)
	writeEventJSONL(t, filepath.Join(dir, "event_calendar_90d.jsonl"), eventN)
	// prediction_actual_90d.jsonl is intentionally NOT written —
	// PR#3 will populate prediction_backtest from the loader output.
}

// ------------------------------------------------------------------
// Run / parseFlags
// ------------------------------------------------------------------

func TestParseFlags_Defaults(t *testing.T) {
	opts, err := parseFlags(nil)
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if opts.StagingDir != "./data/staging" {
		t.Errorf("default StagingDir = %q", opts.StagingDir)
	}
	if opts.DBPath != "./data/state/atlas.db" {
		t.Errorf("default DBPath = %q", opts.DBPath)
	}
	if opts.Tables != "all" {
		t.Errorf("default Tables = %q, want all", opts.Tables)
	}
	if !opts.Now.After(time.Now().Add(-time.Hour)) {
		t.Errorf("default Now = %v, want recent", opts.Now)
	}
}

func TestParseTables(t *testing.T) {
	cases := []struct {
		in     string
		want   map[string]bool
		errSub string
	}{
		{"", map[string]bool{"regime": true, "stress": true, "events": true, "prediction": true}, ""},
		{"all", map[string]bool{"regime": true, "stress": true, "events": true, "prediction": true}, ""},
		{"regime,stress", map[string]bool{"regime": true, "stress": true}, ""},
		{"regime,", map[string]bool{"regime": true}, ""},
		{"junk", nil, "unknown table"},
		{",,,", nil, "no valid tables"},
	}
	for _, tc := range cases {
		got, err := parseTables(tc.in)
		if tc.errSub != "" {
			if err == nil || !strings.Contains(err.Error(), tc.errSub) {
				t.Errorf("parseTables(%q) err = %v, want substring %q", tc.in, err, tc.errSub)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseTables(%q): unexpected err %v", tc.in, err)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("parseTables(%q) len = %d, want %d", tc.in, len(got), len(tc.want))
		}
		for k := range tc.want {
			if !got[k] {
				t.Errorf("parseTables(%q) missing %q", tc.in, k)
			}
		}
	}
}

func TestInRange(t *testing.T) {
	cases := []struct {
		date, since, until string
		want               bool
	}{
		{"2026-04-15", "", "", true},
		{"2026-04-15", "2026-04-10", "2026-04-20", true},
		{"2026-04-15", "2026-04-16", "", false},
		{"2026-04-15", "", "2026-04-14", false},
		{"2026-04-15", "2026-04-15", "2026-04-15", true}, // inclusive
		{"", "", "", false},
	}
	for _, tc := range cases {
		got := inRange(tc.date, tc.since, tc.until)
		if got != tc.want {
			t.Errorf("inRange(%q,%q,%q) = %v, want %v", tc.date, tc.since, tc.until, got, tc.want)
		}
	}
}

// ------------------------------------------------------------------
// End-to-end
// ------------------------------------------------------------------

func TestRun_EndToEnd_AllTablesPresent(t *testing.T) {
	staging := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	makeStaging(t, staging, 5, 5, 3) // 5 regime + 5 stress + 3 events

	stats, err := Run(RunOptions{
		StagingDir: staging,
		DBPath:     dbPath,
		InitSchema: true,
		Now:        time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.RegimeWritten != 5 || stats.StressWritten != 5 || stats.EventWritten != 6 {
		// 3 events × 2 IDs each = 6 rows.
		t.Errorf("stats = %+v, want regime=5 stress=5 events=6", stats)
	}
	if stats.PredictionWritten != 0 {
		t.Errorf("prediction writer count = %d, want 0 (PR#3 territory)", stats.PredictionWritten)
	}
}

func TestRun_DryRun_DoesNotWrite(t *testing.T) {
	staging := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	makeStaging(t, staging, 3, 3, 2)

	_, err := Run(RunOptions{
		StagingDir: staging,
		DBPath:     dbPath,
		DryRun:     true,
	})
	if err != nil {
		t.Fatalf("Run dry: %v", err)
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Errorf("dry-run should not create %s, stat err = %v", dbPath, err)
	}
}

func TestRun_FiltersByTables(t *testing.T) {
	staging := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	makeStaging(t, staging, 3, 3, 2)

	stats, err := Run(RunOptions{
		StagingDir: staging,
		DBPath:     dbPath,
		InitSchema: true,
		Tables:     "regime",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.RegimeWritten != 3 {
		t.Errorf("regime writes = %d, want 3", stats.RegimeWritten)
	}
	if stats.StressWritten != 0 || stats.EventWritten != 0 {
		t.Errorf("non-regime writes should be 0, got stress=%d event=%d", stats.StressWritten, stats.EventWritten)
	}
}

func TestRun_FiltersByDateRange(t *testing.T) {
	staging := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	makeStaging(t, staging, 5, 5, 0)
	stats, err := Run(RunOptions{
		StagingDir: staging,
		DBPath:     dbPath,
		InitSchema: true,
		Since:      "2026-04-02",
		Until:      "2026-04-04",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.RegimeWritten != 3 {
		t.Errorf("in-range regime = %d, want 3", stats.RegimeWritten)
	}
	if stats.StressWritten != 3 {
		t.Errorf("in-range stress = %d, want 3", stats.StressWritten)
	}
	if stats.OutOfRange != 4 {
		t.Errorf("OutOfRange = %d, want 4 (2 regime + 2 stress excluded)", stats.OutOfRange)
	}
}

func TestRun_MissingTables_ErrorsWhenNotInitialised(t *testing.T) {
	staging := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	makeStaging(t, staging, 1, 1, 1)

	// Create the DB but skip InitSchema.
	db, err := ledger.OpenSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_ = db.Close()

	_, err = Run(RunOptions{
		StagingDir: staging,
		DBPath:     dbPath,
		// InitSchema: false — expect error.
	})
	if err == nil {
		t.Fatal("expected error when tables are missing")
	}
	if !strings.Contains(err.Error(), "missing table") {
		t.Errorf("error message = %v, want 'missing table'", err)
	}
}

func TestRun_EmptyStaging_NoOp(t *testing.T) {
	staging := t.TempDir() // empty directory
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	stats, err := Run(RunOptions{
		StagingDir: staging,
		DBPath:     dbPath,
		InitSchema: true,
	})
	if err != nil {
		t.Fatalf("Run on empty staging: %v", err)
	}
	total := stats.RegimeRead + stats.StressRead + stats.EventRead + stats.PredictionRead
	if total != 0 {
		t.Errorf("reads = %d, want 0", total)
	}
}

func TestRun_VerifyIdempotentReRun(t *testing.T) {
	staging := t.TempDir()
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	makeStaging(t, staging, 3, 3, 2)

	for i := range 2 {
		_, err := Run(RunOptions{
			StagingDir: staging,
			DBPath:     dbPath,
			InitSchema: i == 0, // only initialise once
		})
		if err != nil {
			t.Fatalf("Run #%d: %v", i, err)
		}
	}
	// Verify via HistoricalStore read.
	db, err := ledger.OpenSQLiteDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err := ledger.InitSchema(db); err != nil {
		t.Fatal(err)
	}
	store := ledger.NewSQLiteHistoricalStore(db)
	got, err := store.LoadRegimeHistoryAll(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("regime rows after 2 re-runs = %d, want 3 (UPSERT dedup)", len(got))
	}
}

func TestScanJSONL_MalformedLinesSkipped(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "regime.jsonl")
	mustWriteFile(t, path, `not json`+"\n"+
		`{"date":"2026-04-15","regime":"RISK_ON","is_synthetic":1}`+"\n"+
		`{"date":"2026-04-16","regime":"RISK_OFF","is_synthetic":1}`+"\n")
	var rows []loaderRegime
	read, bad, _ := scanJSONL(path, &rows)
	if read != 3 {
		t.Errorf("read = %d, want 3", read)
	}
	if bad != 1 {
		t.Errorf("bad = %d, want 1", bad)
	}
	if len(rows) != 2 {
		t.Errorf("rows = %d, want 2 (1 malformed skipped)", len(rows))
	}
}

func TestRun_ValidatesFlags(t *testing.T) {
	cases := []struct {
		name string
		opts RunOptions
		bad  string
	}{
		{"empty staging", RunOptions{DBPath: "/x"}, "-staging"},
		{"empty db", RunOptions{StagingDir: "/x"}, "-db"},
		{"bad since", RunOptions{StagingDir: "/x", DBPath: "/y", Since: "yesterday"}, "-since"},
		{"bad until", RunOptions{StagingDir: "/x", DBPath: "/y", Until: "tomorrow"}, "-until"},
	}
	for _, tc := range cases {
		_, err := Run(tc.opts)
		if err == nil {
			t.Errorf("%s: expected error", tc.name)
			continue
		}
		if !strings.Contains(err.Error(), tc.bad) {
			t.Errorf("%s: err = %v, want substring %q", tc.name, err, tc.bad)
		}
	}
}
