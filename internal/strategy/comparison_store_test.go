package strategy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileComparisonStore_New(t *testing.T) {
	s := NewFileComparisonStore("/tmp/test.json", 100)
	if s.path != "/tmp/test.json" {
		t.Errorf("path = %s, want /tmp/test.json", s.path)
	}
	if s.maxDays != 100 {
		t.Errorf("maxDays = %d, want 100", s.maxDays)
	}
}

func TestFileComparisonStore_Load_NonExistent(t *testing.T) {
	dir := t.TempDir()
	s := NewFileComparisonStore(filepath.Join(dir, "nonexistent.json"), 100)
	ctx := context.Background()

	days, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("Load of non-existent file: %v", err)
	}
	if days != nil {
		t.Errorf("Load = %v, want nil for non-existent file", days)
	}
}

func TestFileComparisonStore_UpsertAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "comparisons.json")
	s := NewFileComparisonStore(path, 100)
	ctx := context.Background()

	day := ComparisonDay{
		TradingDate: "2026-07-01",
		Benchmark: BenchmarkObservation{
			TradingDate: "2026-07-01",
			SourceID:    "TAIEX",
			ReasonCode:  "test",
			Return:      0.01,
			Available:   true,
		},
		Observations: []StrategyDailyObservation{
			{
				TradingDate:     "2026-07-01",
				StrategyID:      "growth",
				EvaluationMode:  EvaluationModeShadow,
				DailyReturn:     0.02,
				BenchmarkReturn: 0.01,
				Outperformance:  0.01,
				OutcomeCount:    1,
			},
		},
	}

	if err := s.Upsert(ctx, day); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	// Verify file exists on disk
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("Upsert did not create file")
	}

	loaded, err := s.Load(ctx)
	if err != nil {
		t.Fatalf("Load after upsert: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("loaded %d days, want 1", len(loaded))
	}
	if loaded[0].TradingDate != "2026-07-01" {
		t.Errorf("TradingDate = %s, want 2026-07-01", loaded[0].TradingDate)
	}
	if len(loaded[0].Observations) != 1 {
		t.Errorf("len(Observations) = %d, want 1", len(loaded[0].Observations))
	}
}

func TestFileComparisonStore_UpsertReplaceExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replace.json")
	s := NewFileComparisonStore(path, 100)
	ctx := context.Background()

	// Insert day 1
	s.Upsert(ctx, ComparisonDay{
		TradingDate: "2026-07-01",
		Observations: []StrategyDailyObservation{
			{StrategyID: "old", DailyReturn: 0.01},
		},
	})

	// Replace same date with different data
	s.Upsert(ctx, ComparisonDay{
		TradingDate: "2026-07-01",
		Observations: []StrategyDailyObservation{
			{StrategyID: "new", DailyReturn: 0.02},
		},
	})

	loaded, _ := s.Load(ctx)
	if len(loaded) != 1 {
		t.Fatalf("loaded %d days, want 1 after replace", len(loaded))
	}
	if len(loaded[0].Observations) != 1 || loaded[0].Observations[0].StrategyID != "new" {
		t.Error("Upsert did not replace existing day data")
	}
}

func TestFileComparisonStore_MultipleDays(t *testing.T) {
	dir := t.TempDir()
	s := NewFileComparisonStore(filepath.Join(dir, "multi.json"), 100)
	ctx := context.Background()

	dates := []string{"2026-07-01", "2026-07-02", "2026-07-03"}
	for _, d := range dates {
		s.Upsert(ctx, ComparisonDay{TradingDate: d})
	}

	loaded, _ := s.Load(ctx)
	if len(loaded) != 3 {
		t.Fatalf("loaded %d days, want 3", len(loaded))
	}

	// Verify sorted order
	for i, d := range dates {
		if loaded[i].TradingDate != d {
			t.Errorf("loaded[%d].TradingDate = %s, want %s", i, loaded[i].TradingDate, d)
		}
	}
}

func TestFileComparisonStore_MaxDaysPruning(t *testing.T) {
	dir := t.TempDir()
	s := NewFileComparisonStore(filepath.Join(dir, "prune.json"), 2) // max 2 days
	ctx := context.Background()

	s.Upsert(ctx, ComparisonDay{TradingDate: "2026-07-01"})
	s.Upsert(ctx, ComparisonDay{TradingDate: "2026-07-02"})
	s.Upsert(ctx, ComparisonDay{TradingDate: "2026-07-03"})

	loaded, _ := s.Load(ctx)
	if len(loaded) != 2 {
		t.Fatalf("loaded %d days, want 2 (maxDays=2)", len(loaded))
	}
	if loaded[0].TradingDate != "2026-07-02" {
		t.Errorf("oldest day = %s, want 2026-07-02", loaded[0].TradingDate)
	}
	if loaded[1].TradingDate != "2026-07-03" {
		t.Errorf("newest day = %s, want 2026-07-03", loaded[1].TradingDate)
	}
}

func TestFileComparisonStore_LoadCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.json")
	os.WriteFile(path, []byte("{invalid json"), 0o644)

	s := NewFileComparisonStore(path, 100)
	ctx := context.Background()

	_, err := s.Load(ctx)
	if err == nil {
		t.Error("Load of corrupt file should return error")
	}
}

func TestFileComparisonStore_ZeroMaxDays(t *testing.T) {
	dir := t.TempDir()
	s := NewFileComparisonStore(filepath.Join(dir, "nolimit.json"), 0) // unlimited
	ctx := context.Background()

	dates := []string{
		"2026-07-01", "2026-07-02", "2026-07-03", "2026-07-04", "2026-07-05",
		"2026-07-06", "2026-07-07", "2026-07-08", "2026-07-09", "2026-07-10",
	}
	for _, d := range dates {
		s.Upsert(ctx, ComparisonDay{TradingDate: d})
	}

	// With maxDays=0, no pruning should happen
	loaded, _ := s.Load(ctx)
	if len(loaded) != 10 {
		t.Fatalf("loaded %d days, want 10 (unlimited)", len(loaded))
	}
}
