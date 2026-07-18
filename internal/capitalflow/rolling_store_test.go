package capitalflow

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// stubRollingStore is a minimal in-memory implementation of
// RollingSampleStore used by service and extractor tests in this
// package. It records every UpsertDay call so callers can assert that
// read paths (LatestDaily, Summary) never write to the rolling
// window, satisfying spec §8.1.
type stubRollingStore struct {
	upsertCalls int
	samples     []RollingSample
	historyFn   func(dimension ForceName, beforeDate string, limit int) ([]RollingSample, error)
}

func (s *stubRollingStore) History(_ context.Context, dimension ForceName, beforeDate string, limit int) ([]RollingSample, error) {
	if s.historyFn != nil {
		return s.historyFn(dimension, beforeDate, limit)
	}
	return nil, nil
}

func (s *stubRollingStore) UpsertDay(_ context.Context, _ string, samples []RollingSample) error {
	s.upsertCalls++
	s.samples = append(s.samples, samples...)
	return nil
}

// TestFileRollingSampleStore_SameDayLastWriteWins verifies spec §8.2:
// a second UpsertDay for the same trading date replaces the value
// but does not grow the rolling sample count. CF-INV-05 requires that
// each (dimension, trading_date) pair contribute at most one sample.
func TestFileRollingSampleStore_SameDayLastWriteWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rolling.json")
	store := NewFileRollingSampleStore(path, 60)
	ctx := context.Background()

	first := []RollingSample{{
		TradingDate: "2026-07-17",
		Dimension:   ForceForeign,
		RawValue:    1,
		Unit:        "hundred_million_shares",
		SourceID:    SourceTWSET86,
	}}
	second := []RollingSample{{
		TradingDate: "2026-07-17",
		Dimension:   ForceForeign,
		RawValue:    2,
		Unit:        "hundred_million_shares",
		SourceID:    SourceTWSET86,
	}}
	if err := store.UpsertDay(ctx, "2026-07-17", first); err != nil {
		t.Fatalf("first UpsertDay: %v", err)
	}
	if err := store.UpsertDay(ctx, "2026-07-17", second); err != nil {
		t.Fatalf("second UpsertDay: %v", err)
	}
	got, err := store.History(ctx, ForceForeign, "2026-07-18", 60)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("History returned %d samples, want 1 (last-write-wins must collapse duplicates)", len(got))
	}
	if got[0].RawValue != 2 {
		t.Errorf("RawValue=%v, want 2 (last-write-wins)", got[0].RawValue)
	}
	if got[0].SourceID != SourceTWSET86 {
		t.Errorf("SourceID=%q, want %q", got[0].SourceID, SourceTWSET86)
	}
	if got[0].TradingDate != "2026-07-17" {
		t.Errorf("TradingDate=%q, want %q", got[0].TradingDate, "2026-07-17")
	}
	if got[0].Unit != "hundred_million_shares" {
		t.Errorf("Unit=%q, want %q", got[0].Unit, "hundred_million_shares")
	}
}

// TestFileRollingSampleStore_RestartRoundTrip verifies spec §8.5: the
// rolling window must persist across process restart. Constructing a
// second FileRollingSampleStore against the same on-disk file must
// recover the previously written sample with full provenance.
func TestFileRollingSampleStore_RestartRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rolling.json")
	ctx := context.Background()

	first := NewFileRollingSampleStore(path, 60)
	samples := []RollingSample{{
		TradingDate: "2026-07-17",
		Dimension:   ForceForeign,
		RawValue:    42,
		Unit:        "hundred_million_shares",
		SourceID:    SourceTWSET86,
	}}
	if err := first.UpsertDay(ctx, "2026-07-17", samples); err != nil {
		t.Fatalf("first UpsertDay: %v", err)
	}

	// Second instance simulates process restart against the same file.
	second := NewFileRollingSampleStore(path, 60)
	got, err := second.History(ctx, ForceForeign, "2026-07-18", 60)
	if err != nil {
		t.Fatalf("History after restart: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("History after restart returned %d samples, want 1", len(got))
	}
	if got[0].RawValue != 42 {
		t.Errorf("RawValue after restart=%v, want 42", got[0].RawValue)
	}
	if got[0].SourceID != SourceTWSET86 {
		t.Errorf("SourceID after restart=%q, want %q", got[0].SourceID, SourceTWSET86)
	}
	if got[0].Unit != "hundred_million_shares" {
		t.Errorf("Unit after restart=%q, want %q", got[0].Unit, "hundred_million_shares")
	}
	if got[0].TradingDate != "2026-07-17" {
		t.Errorf("TradingDate after restart=%q, want %q", got[0].TradingDate, "2026-07-17")
	}
}

// TestFileRollingSampleStore_BeforeDateExcludes verifies the
// beforeDate parameter of History: only samples with TradingDate
// strictly before the given date are returned, in ascending order.
// This is the contract the production LatestDaily / Refresh paths
// rely on to keep current trading date out of the reference window.
func TestFileRollingSampleStore_BeforeDateExcludes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rolling.json")
	store := NewFileRollingSampleStore(path, 60)
	ctx := context.Background()

	for i, date := range []string{"2026-07-15", "2026-07-16", "2026-07-17"} {
		if err := store.UpsertDay(ctx, date, []RollingSample{{
			TradingDate: date,
			Dimension:   ForceForeign,
			RawValue:    float64(i + 1),
			Unit:        "hundred_million_shares",
			SourceID:    SourceTWSET86,
		}}); err != nil {
			t.Fatalf("UpsertDay %s: %v", date, err)
		}
	}

	got, err := store.History(ctx, ForceForeign, "2026-07-17", 60)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("History returned %d samples, want 2 (07-17 must be excluded)", len(got))
	}
	for i, s := range got {
		if s.TradingDate >= "2026-07-17" {
			t.Errorf("got[%d].TradingDate=%q, want strictly before 2026-07-17", i, s.TradingDate)
		}
		if i > 0 && got[i-1].TradingDate >= s.TradingDate {
			t.Errorf("history not in ascending order: %q then %q", got[i-1].TradingDate, s.TradingDate)
		}
	}
}

// TestFileRollingSampleStore_TrimsToCapacity verifies that writing 61
// distinct trading dates collapses the rolling window to the 60
// newest entries. The oldest written sample must be discarded while
// ascending order is preserved.
func TestFileRollingSampleStore_TrimsToCapacity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rolling.json")
	store := NewFileRollingSampleStore(path, 60)
	ctx := context.Background()

	base, err := time.Parse("2006-01-02", "2026-05-17")
	if err != nil {
		t.Fatalf("parse base date: %v", err)
	}
	const total = 61
	for i := 0; i < total; i++ {
		date := base.AddDate(0, 0, i).Format("2006-01-02")
		if err := store.UpsertDay(ctx, date, []RollingSample{{
			TradingDate: date,
			Dimension:   ForceForeign,
			RawValue:    float64(i),
			Unit:        "hundred_million_shares",
			SourceID:    SourceTWSET86,
		}}); err != nil {
			t.Fatalf("UpsertDay %s: %v", date, err)
		}
	}

	// Far-future beforeDate so every persisted sample is in scope.
	got, err := store.History(ctx, ForceForeign, "2099-12-31", 100)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(got) != 60 {
		t.Fatalf("History returned %d samples, want 60 (capacity must trim oldest)", len(got))
	}
	wantOldest := base.AddDate(0, 0, 1).Format("2006-01-02")
	if got[0].TradingDate != wantOldest {
		t.Errorf("oldest retained=%q, want %q (oldest date %q should have been trimmed)", got[0].TradingDate, wantOldest, "2026-05-17")
	}
	wantNewest := base.AddDate(0, 0, total-1).Format("2006-01-02")
	if got[59].TradingDate != wantNewest {
		t.Errorf("newest retained=%q, want %q", got[59].TradingDate, wantNewest)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].TradingDate >= got[i].TradingDate {
			t.Errorf("history not ascending at %d: %q >= %q", i, got[i-1].TradingDate, got[i].TradingDate)
		}
	}
}

// TestFileRollingSampleStore_UnavailableDimensionsAbsent verifies
// spec §8.3 / CF-INV-06: a dimension whose data was not supplied
// must be absent from History, never represented by a zero-valued
// placeholder sample.
func TestFileRollingSampleStore_UnavailableDimensionsAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rolling.json")
	store := NewFileRollingSampleStore(path, 60)
	ctx := context.Background()

	if err := store.UpsertDay(ctx, "2026-07-17", []RollingSample{{
		TradingDate: "2026-07-17",
		Dimension:   ForceForeign,
		RawValue:    1,
		Unit:        "hundred_million_shares",
		SourceID:    SourceTWSET86,
	}}); err != nil {
		t.Fatalf("UpsertDay: %v", err)
	}

	// Government dimension had no data on 2026-07-17; it must be
	// absent, not a zero-valued placeholder.
	got, err := store.History(ctx, ForceGovernment, "2026-07-18", 60)
	if err != nil {
		t.Fatalf("History government: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("government history returned %d samples, want 0 (unavailable dimensions must be absent, not zero-valued)", len(got))
	}
	for _, s := range got {
		if s.RawValue == 0 {
			t.Errorf("found zero-valued sample for dimension %q (CF-INV-06)", s.Dimension)
		}
	}
}
