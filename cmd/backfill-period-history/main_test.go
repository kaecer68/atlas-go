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
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// ---------------------------------------------------------------------------
// fake store
// ---------------------------------------------------------------------------

// fakeStore implements ledger.HistoricalStore with recording upserts. The
// embedded nil interface satisfies the unused methods; only the four methods
// the backfill touches are implemented.
type fakeStore struct {
	ledger.HistoricalStore
	periodRows []ledger.PeriodRow
	regimeRows []ledger.RegimeRow
	// livePeriod / liveRegime mark dates that already hold a live
	// (is_synthetic=0) row — the backfill must skip those dates.
	livePeriod map[string]bool
	liveRegime map[string]bool
}

func (f *fakeStore) UpsertPeriod(_ context.Context, row ledger.PeriodRow) error {
	f.periodRows = append(f.periodRows, row)
	return nil
}

func (f *fakeStore) LoadPeriodByDateAll(_ context.Context, date string) (ledger.PeriodRow, bool, error) {
	if f.livePeriod[date] {
		return ledger.PeriodRow{Date: date, IsSynthetic: 0}, true, nil
	}
	return ledger.PeriodRow{}, false, nil
}

func (f *fakeStore) UpsertRegime(_ context.Context, row ledger.RegimeRow) error {
	f.regimeRows = append(f.regimeRows, row)
	return nil
}

func (f *fakeStore) LoadRegimeByDateAll(_ context.Context, date string) (ledger.RegimeRow, bool, error) {
	if f.liveRegime[date] {
		return ledger.RegimeRow{Date: date, IsSynthetic: 0}, true, nil
	}
	return ledger.RegimeRow{}, false, nil
}

// ---------------------------------------------------------------------------
// fixture helpers
// ---------------------------------------------------------------------------

func writeSnapshot(t *testing.T, dir, date string, snap marketdata.MacroDataSnapshot) {
	t.Helper()
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal snapshot %s: %v", date, err)
	}
	if err := os.WriteFile(filepath.Join(dir, date+".json"), data, 0o644); err != nil {
		t.Fatalf("write snapshot %s: %v", date, err)
	}
}

// fixtureTree builds data/state/macro with 3 dated snapshots plus the
// meta files that must be skipped (latest/previous/_metadata).
func fixtureTree(t *testing.T) (workDir, macroDir string) {
	t.Helper()
	workDir = t.TempDir()
	macroDir = filepath.Join(workDir, "data", "state", "macro")
	if err := os.MkdirAll(macroDir, 0o755); err != nil {
		t.Fatal(err)
	}
	base := unixDate("2026-05-01")
	writeSnapshot(t, macroDir, "2026-05-01", marketdata.MacroDataSnapshot{
		VIX:                marketdata.MacroDataPoint{Symbol: "^VIX", Value: 14},
		DXY:                marketdata.MacroDataPoint{Symbol: "DX-Y.NYB", Value: 100},
		TAIEX:              marketdata.MacroDataPoint{Symbol: "TAIEX", Value: 25000},
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "TAIWAN_FOREIGN", Value: 8},
		RecordedAt:         base,
	})
	writeSnapshot(t, macroDir, "2026-05-02", marketdata.MacroDataSnapshot{
		VIX:                marketdata.MacroDataPoint{Symbol: "^VIX", Value: 15},
		DXY:                marketdata.MacroDataPoint{Symbol: "DX-Y.NYB", Value: 101},
		TAIEX:              marketdata.MacroDataPoint{Symbol: "TAIEX", Value: 25100},
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "TAIWAN_FOREIGN", Value: 12},
		RecordedAt:         base + 86400,
	})
	// 2026-05-03: all-zero snapshot → period=consolidation, fallback=true,
	// stress=0 → regime=RISK_ON (deterministic no-data path).
	writeSnapshot(t, macroDir, "2026-05-03", marketdata.MacroDataSnapshot{RecordedAt: base + 2*86400})
	for _, meta := range []string{"latest.json", "previous.json", "_metadata.json"} {
		if err := os.WriteFile(filepath.Join(macroDir, meta), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return workDir, macroDir
}

func unixDate(s string) int64 {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t.Unix()
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	_ = w.Close()
	os.Stdout = old
	buf := make([]byte, 64*1024)
	n, _ := r.Read(buf)
	return string(buf[:n])
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

func TestRunDryRun_ProcessesDatedSnapshotsSkipsMeta(t *testing.T) {
	workDir, _ := fixtureTree(t)
	out := captureStdout(t, func() {
		stats, err := run(context.Background(), runConfig{
			workDir: workDir,
			dryRun:  true,
			now:     func() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) },
		})
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if stats.totalInDir != 3 {
			t.Errorf("totalInDir = %d, want 3 (meta files must be skipped)", stats.totalInDir)
		}
		if stats.inRange != 3 {
			t.Errorf("inRange = %d, want 3", stats.inRange)
		}
		if stats.processed != 3 {
			t.Errorf("processed = %d, want 3", stats.processed)
		}
		if stats.errors != 0 {
			t.Errorf("errors = %d, want 0", stats.errors)
		}
		if stats.upsertPeriod != 0 || stats.upsertRegime != 0 {
			t.Errorf("dry-run must not upsert: period=%d regime=%d", stats.upsertPeriod, stats.upsertRegime)
		}
	})
	for _, want := range []string{"2026-05-01", "2026-05-02", "2026-05-03", "period=", "regime=", "fallback="} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q", want)
		}
	}
	// No-data day must be consolidation + fallback + RISK_ON (stress 0 → low).
	if !strings.Contains(out, "[2026-05-03] DRY-RUN period=consolidation") {
		t.Errorf("2026-05-03 should be consolidation, got:\n%s", out)
	}
	if !strings.Contains(out, "regime=RISK_ON") {
		t.Errorf("no-data day should map to RISK_ON, got:\n%s", out)
	}
}

func TestRunWriteMode_UpsertsPeriodAndRegime(t *testing.T) {
	workDir, _ := fixtureTree(t)
	store := &fakeStore{}
	_, err := run(context.Background(), runConfig{
		workDir: workDir,
		dryRun:  false,
		dbPath:  "atlas.db",
		store:   store,
		now:     func() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(store.periodRows) != 3 {
		t.Fatalf("UpsertPeriod calls = %d, want 3", len(store.periodRows))
	}
	if len(store.regimeRows) != 3 {
		t.Fatalf("UpsertRegime calls = %d, want 3", len(store.regimeRows))
	}
	for i, want := range []string{"2026-05-01", "2026-05-02", "2026-05-03"} {
		pr := store.periodRows[i]
		if pr.Date != want {
			t.Errorf("period row %d date = %q, want %q", i, pr.Date, want)
		}
		if pr.Period == "" {
			t.Errorf("period row %d has empty period", i)
		}
		if pr.IsSynthetic != 1 {
			t.Errorf("period row %d IsSynthetic = %d, want 1 (backfilled)", i, pr.IsSynthetic)
		}
		if pr.Source != sourceName {
			t.Errorf("period row %d Source = %q, want %q", i, pr.Source, sourceName)
		}
		rr := store.regimeRows[i]
		if rr.Date != want {
			t.Errorf("regime row %d date = %q, want %q", i, rr.Date, want)
		}
		if rr.Regime == "" {
			t.Errorf("regime row %d has empty regime", i)
		}
		if rr.IsSynthetic != 1 {
			t.Errorf("regime row %d IsSynthetic = %d, want 1", i, rr.IsSynthetic)
		}
		if rr.SourceSessionID != sourceName+":"+want {
			t.Errorf("regime row %d SourceSessionID = %q, want %q", i, rr.SourceSessionID, sourceName+":"+want)
		}
	}
}

func TestRunWriteMode_SkipsExistingLiveRows(t *testing.T) {
	workDir, _ := fixtureTree(t)
	store := &fakeStore{livePeriod: map[string]bool{"2026-05-02": true}, liveRegime: map[string]bool{"2026-05-02": true}}
	stats, err := run(context.Background(), runConfig{
		workDir: workDir,
		dryRun:  false,
		dbPath:  "atlas.db",
		store:   store,
		now:     func() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(store.periodRows) != 2 {
		t.Errorf("UpsertPeriod calls = %d, want 2 (live date skipped)", len(store.periodRows))
	}
	if len(store.regimeRows) != 2 {
		t.Errorf("UpsertRegime calls = %d, want 2 (live date skipped)", len(store.regimeRows))
	}
	if stats.skippedLive != 1 {
		t.Errorf("skippedLive = %d, want 1", stats.skippedLive)
	}
	for _, pr := range store.periodRows {
		if pr.Date == "2026-05-02" {
			t.Errorf("live date 2026-05-02 must not be overwritten")
		}
	}
}

func TestRun_StartEndFilter(t *testing.T) {
	workDir, _ := fixtureTree(t)
	stats, err := run(context.Background(), runConfig{
		workDir: workDir,
		start:   "2026-05-02",
		end:     "2026-05-02",
		dryRun:  true,
		now:     func() time.Time { return time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if stats.inRange != 1 || stats.processed != 1 {
		t.Errorf("inRange/processed = %d/%d, want 1/1", stats.inRange, stats.processed)
	}
}

func TestStressScoreToRegime(t *testing.T) {
	cases := []struct {
		score float64
		want  string
	}{
		{0, "low"}, {10, "low"}, {29.9, "low"},
		{30, "alert"}, {49.9, "alert"},
		{50, "high"}, {69.9, "high"},
		{70, "crisis"}, {100, "crisis"},
	}
	for _, c := range cases {
		if got := stressScoreToRegime(c.score); got != c.want {
			t.Errorf("stressScoreToRegime(%v) = %q, want %q", c.score, got, c.want)
		}
	}
}
