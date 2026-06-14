package replay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWindowDates_FullRange(t *testing.T) {
	path := twseCSVFixture(t, "window.csv")
	ds, err := LoadTWSEOpenDataCSV(path)
	if err != nil {
		t.Fatal(err)
	}

	start, _ := time.Parse("2006-01-02", "2026-03-20")
	end, _ := time.Parse("2006-01-02", "2026-03-21")
	dates := ds.WindowDates(start, end, 1)
	if len(dates) != 1 {
		t.Fatalf("expected 1 date (only first date has forward window of 1), got %d", len(dates))
	}
	if dates[0].Format("2006-01-02") != "2026-03-20" {
		t.Fatalf("expected 2026-03-20, got %s", dates[0].Format("2006-01-02"))
	}
}

func TestWindowDates_NoDatesInRange(t *testing.T) {
	path := twseCSVFixture(t, "window.csv")
	ds, err := LoadTWSEOpenDataCSV(path)
	if err != nil {
		t.Fatal(err)
	}

	start, _ := time.Parse("2006-01-02", "2025-01-01")
	end, _ := time.Parse("2006-01-02", "2025-01-31")
	dates := ds.WindowDates(start, end, 1)
	if len(dates) != 0 {
		t.Fatalf("expected 0 dates, got %d", len(dates))
	}
}

func TestWindowDates_ForwardUnavailable(t *testing.T) {
	path := twseCSVFixture(t, "window.csv")
	ds, err := LoadTWSEOpenDataCSV(path)
	if err != nil {
		t.Fatal(err)
	}

	start, _ := time.Parse("2006-01-02", "2026-03-21")
	end, _ := time.Parse("2006-01-02", "2026-03-21")
	// Window=1 on the last date has no next date → should be filtered out
	dates := ds.WindowDates(start, end, 1)
	if len(dates) != 0 {
		t.Fatalf("expected 0 dates (no forward window for last date), got %d", len(dates))
	}
}

func TestWindowDates_EmptyDataset(t *testing.T) {
	dir := t.TempDir()
	csv := strings.Join([]string{
		"Date,Code,Name,TradeVolume,TradeValue,Open,High,Low,Close,Change,Transaction",
	}, "\n") + "\n"
	path := filepath.Join(dir, "empty.csv")
	if err := os.WriteFile(path, []byte(csv), 0o644); err != nil {
		t.Fatal(err)
	}

	ds, err := LoadTWSEOpenDataCSV(path)
	if err != nil {
		t.Fatal(err)
	}

	start, _ := time.Parse("2006-01-02", "2026-01-01")
	end, _ := time.Parse("2006-01-02", "2026-12-31")
	dates := ds.WindowDates(start, end, 1)
	if len(dates) != 0 {
		t.Fatalf("expected 0 dates for empty dataset, got %d", len(dates))
	}
}
