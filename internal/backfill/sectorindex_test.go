package backfill

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// stubSectorFetcher returns canned sector data or empty maps per day.
type stubSectorFetcher struct {
	failTimes int // first N calls fail
	calls     int
}

func (s *stubSectorFetcher) FetchSectorIndices(_ context.Context, startDate, endDate time.Time) (map[string][]marketdata.SectorIndexData, error) {
	s.calls++
	if s.failTimes > 0 {
		s.failTimes--
		return nil, errors.New("stub fetch error")
	}
	if startDate.Weekday() == time.Saturday || startDate.Weekday() == time.Sunday {
		return map[string][]marketdata.SectorIndexData{}, nil
	}
	return map[string][]marketdata.SectorIndexData{
		"semiconductor": {
			{Date: startDate.Format("2006-01-02"), Industry: "semiconductor", Index: 1548.97, ReturnPct: 0.62},
		},
		"shipping": {
			{Date: startDate.Format("2006-01-02"), Industry: "shipping", Index: 189.89, ReturnPct: 0.12},
		},
	}, nil
}

func TestBackfillSectorIndex_WritesPerDayFile(t *testing.T) {
	dir := t.TempDir()
	stub := &stubSectorFetcher{}
	result, err := BackfillSectorIndex(context.Background(), stub, dir, "2026-04-27", "2026-04-27", false)
	if err != nil {
		t.Fatalf("BackfillSectorIndex: %v", err)
	}
	if result.Backfilled != 1 {
		t.Errorf("Backfilled = %d, want 1", result.Backfilled)
	}
	path := filepath.Join(dir, "sector_indices_20260427_20260427.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var data map[string][]marketdata.SectorIndexData
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(data) != 2 {
		t.Errorf("industry count = %d, want 2", len(data))
	}
	if data["semiconductor"][0].Date != "2026-04-27" || data["semiconductor"][0].Industry != "semiconductor" {
		t.Errorf("semiconductor entry = %+v", data["semiconductor"][0])
	}
}

func TestBackfillSectorIndex_SkipsWeekendAndHoliday(t *testing.T) {
	dir := t.TempDir()
	stub := &stubSectorFetcher{}
	// 2026-04-25 Sat, 04-26 Sun, 04-27 Mon
	result, err := BackfillSectorIndex(context.Background(), stub, dir, "2026-04-25", "2026-04-27", false)
	if err != nil {
		t.Fatalf("BackfillSectorIndex: %v", err)
	}
	if result.Scanned != 3 {
		t.Errorf("Scanned = %d, want 3", result.Scanned)
	}
	if result.Backfilled != 1 {
		t.Errorf("Backfilled = %d, want 1", result.Backfilled)
	}
	if result.SkippedNoData != 2 {
		t.Errorf("SkippedNoData = %d, want 2 (Sat+Sun)", result.SkippedNoData)
	}
}

func TestBackfillSectorIndex_RetriesThenSucceeds(t *testing.T) {
	dir := t.TempDir()
	stub := &stubSectorFetcher{failTimes: 2}
	result, err := BackfillSectorIndex(context.Background(), stub, dir, "2026-04-27", "2026-04-27", false)
	if err != nil {
		t.Fatalf("BackfillSectorIndex: %v", err)
	}
	if result.Backfilled != 1 {
		t.Errorf("Backfilled = %d, want 1", result.Backfilled)
	}
	if stub.calls != 3 {
		t.Errorf("fetcher calls = %d, want 3", stub.calls)
	}
}

func TestBackfillSectorIndex_SkipsExistingFile(t *testing.T) {
	dir := t.TempDir()
	// Pre-existing file for 04-27 (like the 2026-06-03+ ones).
	existing := `{
  "semiconductor": [
    {
      "date": "2026-04-27",
      "industry": "semiconductor",
      "index": 1500,
      "return_pct": 1
    }
  ]
}
`
	if err := os.WriteFile(filepath.Join(dir, "sector_indices_20260427_20260427.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	stub := &stubSectorFetcher{}
	result, err := BackfillSectorIndex(context.Background(), stub, dir, "2026-04-27", "2026-04-27", false)
	if err != nil {
		t.Fatalf("BackfillSectorIndex: %v", err)
	}
	if result.SkippedExists != 1 {
		t.Errorf("SkippedExists = %d, want 1", result.SkippedExists)
	}
	if result.Backfilled != 0 {
		t.Errorf("Backfilled = %d, want 0", result.Backfilled)
	}
	if stub.calls != 0 {
		t.Errorf("fetcher calls = %d, want 0 (existing file must not refetch)", stub.calls)
	}
}

func TestBackfillSectorIndex_DryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	stub := &stubSectorFetcher{}
	result, err := BackfillSectorIndex(context.Background(), stub, dir, "2026-04-27", "2026-04-27", true)
	if err != nil {
		t.Fatalf("BackfillSectorIndex: %v", err)
	}
	if result.Backfilled != 1 {
		t.Errorf("Backfilled = %d, want 1 (dry-run counts)", result.Backfilled)
	}
	if _, err := os.Stat(filepath.Join(dir, "sector_indices_20260427_20260427.json")); !os.IsNotExist(err) {
		t.Error("dry-run must not create files")
	}
}

func TestBackfillSectorIndex_ParseErrors(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		start string
		end   string
	}{
		{"bad", "2026-08-01"},
		{"2026-08-01", "bad"},
		{"2026-08-10", "2026-08-01"},
		{"", "2026-08-01"},
	}
	for _, tc := range cases {
		_, err := BackfillSectorIndex(context.Background(), &stubSectorFetcher{}, dir, tc.start, tc.end, false)
		if err == nil {
			t.Errorf("BackfillSectorIndex(%q, %q): expected error, got nil", tc.start, tc.end)
		}
	}
}

// TestBackfillSectorIndex_DateRangeParameterized drives the same window logic
// the full backfill uses: only dates inside [start, end] produce files.
func TestBackfillSectorIndex_DateRangeParameterized(t *testing.T) {
	dir := t.TempDir()
	stub := &stubSectorFetcher{}
	result, err := BackfillSectorIndex(context.Background(), stub, dir, "2026-04-27", "2026-04-29", false)
	if err != nil {
		t.Fatalf("BackfillSectorIndex: %v", err)
	}
	if result.Scanned != 3 {
		t.Errorf("Scanned = %d, want 3", result.Scanned)
	}
	if result.Backfilled != 3 {
		t.Errorf("Backfilled = %d, want 3", result.Backfilled)
	}
	for _, d := range []string{"20260427", "20260428", "20260429"} {
		if _, err := os.Stat(filepath.Join(dir, "sector_indices_"+d+"_"+d+".json")); err != nil {
			t.Errorf("missing file for %s: %v", d, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "sector_indices_20260430_20260430.json")); !os.IsNotExist(err) {
		t.Error("2026-04-30 is outside the window and must not exist")
	}
}
