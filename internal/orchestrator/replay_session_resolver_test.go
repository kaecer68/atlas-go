package orchestrator

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeReplayCSV is a small helper that writes a fixture CSV with the
// production schema (Date,Code,Name,...) and returns the absolute path.
func writeReplayCSV(t *testing.T, dates []string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.csv")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.WriteString("Date,Code, Name, TradeVolume, Open, High, Low, Close\n"); err != nil {
		t.Fatalf("write header: %v", err)
	}
	for _, d := range dates {
		// Pad the row so the parser sees the expected column count.
		if _, err := f.WriteString(d + ", 2330, TSMC, 1000, 100, 101, 99, 100.5\n"); err != nil {
			t.Fatalf("write row %s: %v", d, err)
		}
	}
	return path
}

func TestReplaySessionResolverFromCSV_NextAfterAsOf(t *testing.T) {
	// 5 distinct trading dates, all 2026-07 weekdays.
	csvPath := writeReplayCSV(t, []string{
		"2026-07-01", "2026-07-02", "2026-07-03", "2026-07-06", "2026-07-07",
	})
	r, err := NewReplaySessionResolverFromCSV(csvPath)
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}

	asOf, _ := time.Parse("2006-01-02", "2026-07-02")
	got, err := r.NextTradingSession(asOf)
	if err != nil {
		t.Fatalf("NextTradingSession: %v", err)
	}
	if got.Format("2006-01-02") != "2026-07-03" {
		t.Errorf("expected 2026-07-03, got %s", got.Format("2006-01-02"))
	}
}

func TestReplaySessionResolverFromCSV_NextFromFirstRow(t *testing.T) {
	csvPath := writeReplayCSV(t, []string{
		"2026-07-01", "2026-07-02", "2026-07-03",
	})
	r, err := NewReplaySessionResolverFromCSV(csvPath)
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}

	// asOf is BEFORE the first date — first row is the next session.
	asOf, _ := time.Parse("2006-01-02", "2026-06-30")
	got, err := r.NextTradingSession(asOf)
	if err != nil {
		t.Fatalf("NextTradingSession: %v", err)
	}
	if got.Format("2006-01-02") != "2026-07-01" {
		t.Errorf("expected 2026-07-01, got %s", got.Format("2006-01-02"))
	}
}

func TestReplaySessionResolverFromCSV_Exhausted(t *testing.T) {
	csvPath := writeReplayCSV(t, []string{
		"2026-07-01", "2026-07-02",
	})
	r, err := NewReplaySessionResolverFromCSV(csvPath)
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}

	asOf, _ := time.Parse("2006-01-02", "2026-12-31")
	_, err = r.NextTradingSession(asOf)
	if err == nil {
		t.Fatal("expected error for exhausted dataset")
	}
	if !errors.Is(err, ErrSessionUnavailable) {
		t.Errorf("expected ErrSessionUnavailable, got %v", err)
	}
}

func TestReplaySessionResolverFromCSV_DedupesAndSorts(t *testing.T) {
	// Out-of-order duplicates must not produce a wrong next date.
	csvPath := writeReplayCSV(t, []string{
		"2026-07-03", "2026-07-01", "2026-07-02", "2026-07-01", "2026-07-03",
	})
	r, err := NewReplaySessionResolverFromCSV(csvPath)
	if err != nil {
		t.Fatalf("constructor: %v", err)
	}

	asOf, _ := time.Parse("2006-01-02", "2026-07-01")
	got, err := r.NextTradingSession(asOf)
	if err != nil {
		t.Fatalf("NextTradingSession: %v", err)
	}
	if got.Format("2006-01-02") != "2026-07-02" {
		t.Errorf("expected 2026-07-02 (next after deduped+sort), got %s", got.Format("2006-01-02"))
	}
}

func TestReplaySessionResolverFromCSV_SkipsMalformedRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.csv")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	defer func() { _ = f.Close() }()

	// Header + 1 good row + 2 bad rows.
	if _, err := f.WriteString("Date,Code,Name,TradeVolume,Open,High,Low,Close\n"); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := f.WriteString("not-a-date,2330,TSMC,1,1,1,1,1\n"); err != nil {
		t.Fatalf("write bad row: %v", err)
	}
	if _, err := f.WriteString("2026-07-02,2330,TSMC,1,1,1,1,1\n"); err != nil {
		t.Fatalf("write good row: %v", err)
	}
	if _, err := f.WriteString(",2330,TSMC,1,1,1,1,1\n"); err != nil {
		t.Fatalf("write empty-date row: %v", err)
	}

	r, err := NewReplaySessionResolverFromCSV(path)
	if err != nil {
		t.Fatalf("constructor should succeed with skip-malformed, got: %v", err)
	}

	asOf, _ := time.Parse("2006-01-02", "2026-07-01")
	got, err := r.NextTradingSession(asOf)
	if err != nil {
		t.Fatalf("NextTradingSession: %v", err)
	}
	if got.Format("2006-01-02") != "2026-07-02" {
		t.Errorf("expected 2026-07-02 (only valid row), got %s", got.Format("2006-01-02"))
	}
}

func TestReplaySessionResolverFromCSV_MissingFile(t *testing.T) {
	_, err := NewReplaySessionResolverFromCSV("/nonexistent/replay.csv")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReplaySessionResolverFromCSV_HeaderOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.csv")
	if err := os.WriteFile(path, []byte("Date,Code,Name,TradeVolume,Open,High,Low,Close\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := NewReplaySessionResolverFromCSV(path)
	if err == nil {
		t.Fatal("expected error for header-only CSV")
	}
}
