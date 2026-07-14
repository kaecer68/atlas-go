// File: cmd/backtest-event-flow/main_test.go
// Package: main
//
// Tests for the prediction backtest engine. Each test uses staging JSONL
// fixtures under t.TempDir() so they never touch real atlas.db.
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

func mustWrite(t *testing.T, path string, lines []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() { _ = f.Close() }()
	for _, ln := range lines {
		if _, err := f.WriteString(ln + "\n"); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
}

func makeRegimeLine(date, regime string) string {
	b, _ := json.Marshal(regimeRow{Date: date, Regime: regime, RecordedAt: "2026-07-14T01:00:00Z"})
	return string(b)
}

func makeStressLine(date string, score float64) string {
	b, _ := json.Marshal(stressRow{Date: date, Score: score, Source: "raw"})
	return string(b)
}

func makeActualLine(date string, total, hit int, winRate, meanRet, capFlowProxy float64) string {
	b, _ := json.Marshal(actualRow{
		Date: date, TotalOutcomes: total, HitOutcomesCount: hit,
		WinRate: winRate, MeanForwardReturn: meanRet, CapitalFlowChangeProxy: capFlowProxy,
	})
	return string(b)
}

func makeEventLine(date, eid, theme string) string {
	b, _ := json.Marshal(map[string]any{"date": date, "event_ids": []string{eid}, "active_themes": []string{theme}})
	return string(b)
}

func makeSampleStaging(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "regime_history_90d.jsonl"), []string{
		makeRegimeLine("2026-04-15", "RISK_ON"),
		makeRegimeLine("2026-05-05", "NEUTRAL"),
		makeRegimeLine("2026-05-06", "RISK_OFF"),
	})
	mustWrite(t, filepath.Join(dir, "stress_index_history_90d.jsonl"), []string{
		makeStressLine("2026-04-15", 4.0),
		makeStressLine("2026-05-05", 0.0),
		makeStressLine("2026-05-06", 8.0),
	})
	mustWrite(t, filepath.Join(dir, "prediction_actual_90d.jsonl"), []string{
		makeActualLine("2026-04-15", 38, 30, 0.79, 0.0167, 0.012),
		makeActualLine("2026-05-05", 0, 0, 0, 0, 0),
		makeActualLine("2026-05-06", 26, 5, 0.19, -0.012, -0.018),
	})
	mustWrite(t, filepath.Join(dir, "event_calendar_90d.jsonl"), []string{
		makeEventLine("2026-04-15", "evt-msci-202604", "msci_rebalance"),
	})
	return dir
}

// ------------------------------------------------------------------
// parseFlags / validateRun
// ------------------------------------------------------------------

func TestParseFlags_Defaults(t *testing.T) {
	opts, err := parseFlags(nil)
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if opts.StagingDir != "./data/staging" {
		t.Errorf("StagingDir = %q", opts.StagingDir)
	}
	if opts.DBPath != "./data/state/atlas.db" {
		t.Errorf("DBPath = %q", opts.DBPath)
	}
	if opts.Model != "stage4-v1" {
		t.Errorf("Model = %q", opts.Model)
	}
	if !opts.Now.After(time.Now().Add(-time.Hour)) {
		t.Errorf("Now = %v should be recent", opts.Now)
	}
}

func TestValidateRun(t *testing.T) {
	cases := []struct {
		in     RunOptions
		errSub string
	}{
		{RunOptions{StagingDir: "x", DBPath: "y"}, ""},
		{RunOptions{StagingDir: "", DBPath: "y"}, "-staging"},
		{RunOptions{StagingDir: "x", DBPath: ""}, "-db"},
		{RunOptions{StagingDir: "x", DBPath: "y", Since: "bad"}, "-since"},
		{RunOptions{StagingDir: "x", DBPath: "y", Until: "bad"}, "-until"},
		{RunOptions{StagingDir: "x", DBPath: "y", Since: "2026-05-01", Until: "2026-04-01"}, "after -until"},
	}
	for _, tc := range cases {
		err := validateRun(tc.in)
		if tc.errSub == "" {
			if err != nil {
				t.Errorf("validateRun(%+v): unexpected err %v", tc.in, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.errSub) {
			t.Errorf("validateRun(%+v): err = %v, want substring %q", tc.in, err, tc.errSub)
		}
	}
}

// ------------------------------------------------------------------
// helpers
// ------------------------------------------------------------------

func TestInRange(t *testing.T) {
	cases := []struct {
		d, lo, hi string
		want      bool
	}{
		{"2026-04-15", "", "", true},
		{"2026-04-15", "2026-04-10", "2026-04-20", true},
		{"2026-04-15", "2026-04-16", "", false},
		{"2026-04-15", "", "2026-04-14", false},
		{"", "", "", false},
		{"2026-04-15", "2026-04-15", "2026-04-15", true},
	}
	for _, tc := range cases {
		if got := inRange(tc.d, tc.lo, tc.hi); got != tc.want {
			t.Errorf("inRange(%q,%q,%q) = %v, want %v", tc.d, tc.lo, tc.hi, got, tc.want)
		}
	}
}

func TestNormalizeDir(t *testing.T) {
	cases := map[string]string{
		"inflow":    "inflow",
		"  INFLOW ": "inflow",
		"bullish":   "inflow",
		"up":        "inflow",
		"positive":  "inflow",
		"outflow":   "outflow",
		"bearish":   "outflow",
		"down":      "outflow",
		"neutral":   "neutral",
		"":          "neutral",
		"unknown":   "neutral",
	}
	for in, want := range cases {
		if got := normalizeDir(in); got != want {
			t.Errorf("normalizeDir(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDirectionFromReturn(t *testing.T) {
	cases := []struct {
		r    float64
		want string
	}{
		{0.05, "inflow"},
		{0.006, "inflow"},
		{0.005, "neutral"},
		{0, "neutral"},
		{-0.001, "neutral"},
		{-0.006, "outflow"},
		{-0.05, "outflow"},
	}
	for _, tc := range cases {
		if got := directionFromReturn(tc.r); got != tc.want {
			t.Errorf("directionFromReturn(%v) = %q, want %q", tc.r, got, tc.want)
		}
	}
}

func TestDeriveFromRegime(t *testing.T) {
	cases := map[string][2]string{
		"RISK_ON":  {"inflow", ">0.01"},
		"risk_on":  {"inflow", ">0.01"},
		"RISK_OFF": {"outflow", "<-0.01"},
		"NEUTRAL":  {"neutral", "==0"},
		"":         {"neutral", "==0"},
	}
	for in, want := range cases {
		dir, flow := deriveFromRegime(in)
		if dir != want[0] {
			t.Errorf("deriveFromRegime(%q) dir = %q, want %q", in, dir, want[0])
		}
		switch want[1] {
		case ">0.01":
			if flow <= 0.01 {
				t.Errorf("deriveFromRegime(%q) flow = %v, want > 0.01", in, flow)
			}
		case "<-0.01":
			if flow >= -0.01 {
				t.Errorf("deriveFromRegime(%q) flow = %v, want < -0.01", in, flow)
			}
		case "==0":
			if flow != 0 {
				t.Errorf("deriveFromRegime(%q) flow = %v, want == 0", in, flow)
			}
		}
	}
}

func TestClampScore(t *testing.T) {
	cases := map[float64]float64{
		0.5:   0.5,
		1.5:   1.0,
		-1.5:  -1.0,
		-0.3:  -0.3,
		0:     0,
		1:     1,
		-1:    -1,
		0.001: 0.001,
	}
	for in, want := range cases {
		if got := clampScore(in); got != want {
			t.Errorf("clampScore(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestLabelForScore(t *testing.T) {
	cases := map[float64]string{
		0.5:   "inflow",
		0.21:  "inflow",
		0.2:   "neutral",
		0:     "neutral",
		-0.2:  "neutral",
		-0.21: "outflow",
		-0.5:  "outflow",
	}
	for in, want := range cases {
		if got := labelForScore(in); got != want {
			t.Errorf("labelForScore(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestSortedDates(t *testing.T) {
	m := map[string]regimeRow{
		"2026-05-06": {},
		"2026-04-15": {},
		"2026-05-05": {},
	}
	got := sortedDates(m)
	want := []string{"2026-04-15", "2026-05-05", "2026-05-06"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, d := range want {
		if got[i] != d {
			t.Errorf("[%d] = %q, want %q", i, got[i], d)
		}
	}
}

// ------------------------------------------------------------------
// End-to-end with real SQLite
// ------------------------------------------------------------------

func TestRun_NoStaging_NoOps(t *testing.T) {
	dir := t.TempDir()
	stats, err := Run(RunOptions{
		StagingDir: dir,
		DBPath:     filepath.Join(dir, "atlas.db"),
		InitSchema: true,
		Now:        time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
		Logger:     nil,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.PredictionsWritten != 0 || stats.DaysProcessed != 0 {
		t.Errorf("stats = %+v, want zero counters", stats)
	}
}

func TestRun_DryRun_DoesNotCreateDB(t *testing.T) {
	dir := makeSampleStaging(t)
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	stats, err := Run(RunOptions{
		StagingDir: dir,
		DBPath:     dbPath,
		DryRun:     true,
		Now:        time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.PredictionsWritten == 0 {
		t.Errorf("stats = %+v, want predictions > 0", stats)
	}
	if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
		t.Errorf("dbPath should not exist in dry-run; stat err = %v", err)
	}
}

func TestRun_E2E_WithSampleData(t *testing.T) {
	dir := makeSampleStaging(t)
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	stats, err := Run(RunOptions{
		StagingDir: dir,
		DBPath:     dbPath,
		InitSchema: true,
		Now:        time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.PredictionsWritten == 0 {
		t.Fatalf("stats = %+v, want > 0", stats)
	}

	store := openStore(t, dbPath)
	rows, err := store.LoadPredictionBacktestRange(context.Background(), "", "", 100)
	if err != nil {
		t.Fatalf("LoadPredictionBacktestRange: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("LoadPredictionBacktestRange returned 0 rows")
	}
	for _, r := range rows {
		if r.ModelVersion != "stage4-v1" {
			t.Errorf("ModelVersion = %q", r.ModelVersion)
		}
		if r.IsSynthetic != 1 {
			t.Errorf("IsSynthetic = %d", r.IsSynthetic)
		}
		if r.PredictedDirection != "inflow" && r.PredictedDirection != "outflow" && r.PredictedDirection != "neutral" {
			t.Errorf("PredictedDirection = %q", r.PredictedDirection)
		}
		if r.ActualDirection != "inflow" && r.ActualDirection != "outflow" && r.ActualDirection != "neutral" {
			t.Errorf("ActualDirection = %q", r.ActualDirection)
		}
	}
}

func TestRun_FiltersByDateRange(t *testing.T) {
	dir := makeSampleStaging(t)
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	stats, err := Run(RunOptions{
		StagingDir: dir,
		DBPath:     dbPath,
		InitSchema: true,
		Since:      "2026-05-01",
		Until:      "2026-05-31",
		Now:        time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	store := openStore(t, dbPath)
	rows, err := store.LoadPredictionBacktestRange(context.Background(), "2026-05-01", "2026-05-31", 100)
	if err != nil {
		t.Fatalf("LoadPredictionBacktestRange: %v", err)
	}
	for _, r := range rows {
		if r.Date < "2026-05-01" || r.Date > "2026-05-31" {
			t.Errorf("date %q outside [2026-05-01,2026-05-31]", r.Date)
		}
	}
	if stats.DaysSkipped == 0 {
		t.Errorf("expected DaysSkipped > 0 (April 15 row should be skipped)")
	}
}

func TestRun_FallbackToRegimeWhenActualMissing(t *testing.T) {
	// Stage only regimes + stress (no actuals) — every row should still get
	// a prediction via the regime-derived fallback.
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "regime_history_90d.jsonl"), []string{
		makeRegimeLine("2026-04-15", "RISK_OFF"),
	})
	mustWrite(t, filepath.Join(dir, "stress_index_history_90d.jsonl"), []string{
		makeStressLine("2026-04-15", 0),
	})
	dbPath := filepath.Join(t.TempDir(), "atlas.db")
	stats, err := Run(RunOptions{
		StagingDir: dir,
		DBPath:     dbPath,
		InitSchema: true,
		Now:        time.Date(2026, 7, 14, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.PredictionsWritten == 0 {
		t.Errorf("expected at least 1 row from fallback path; stats = %+v", stats)
	}
	if stats.PredictionsSkippedNoActual != 0 {
		t.Errorf("fallback should produce rows without counting as skipped; got %d", stats.PredictionsSkippedNoActual)
	}
}

func openStore(t *testing.T, dbPath string) ledger.HistoricalStore {
	t.Helper()
	db, err := ledger.OpenSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return ledger.NewSQLiteHistoricalStore(db)
}
