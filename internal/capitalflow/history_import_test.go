package capitalflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// genDates returns n consecutive YYYY-MM-DD strings starting at base.
func genDates(base string, n int) []string {
	start, err := time.Parse("2006-01-02", base)
	if err != nil {
		panic(err)
	}
	out := make([]string, n)
	for i := range out {
		out[i] = start.AddDate(0, 0, i).Format("2006-01-02")
	}
	return out
}

// genT86Samples builds 3 samples (foreign / institutional / dealer)
// per date with distinct per-date raw values.
func genT86Samples(dates []string) []RollingSample {
	var out []RollingSample
	for i, d := range dates {
		out = append(out,
			RollingSample{TradingDate: d, Dimension: ForceForeign, RawValue: float64(i) + 0.5, Unit: "hundred_million_shares", SourceID: SourceTWSET86},
			RollingSample{TradingDate: d, Dimension: ForceInstitutional, RawValue: float64(i) + 0.25, Unit: "hundred_million_shares", SourceID: SourceTWSET86},
			RollingSample{TradingDate: d, Dimension: ForceDealer, RawValue: float64(i) - 0.25, Unit: "hundred_million_shares", SourceID: SourceTWSET86},
		)
	}
	return out
}

// writeReplayFixture writes a replay-style CSV for the given dates.
func writeReplayFixture(t *testing.T, path string, dates []string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create replay fixture: %v", err)
	}
	defer f.Close()
	fmt.Fprintln(f, "Date,Code,Name,TradeVolume,Open,High,Low,Close")
	for _, d := range dates {
		fmt.Fprintf(f, "%s,2330,台積電,100,100,101,99,100.5\n", d)
	}
}

// writeT86Fixture writes one T86 snapshot JSON per date into dir,
// alternating the two date shapes seen in production.
func writeT86Fixture(t *testing.T, dir string, dates []string) {
	t.Helper()
	for i, d := range dates {
		compact := d[0:4] + d[5:7] + d[8:10]
		name := compact + ".json"
		if i%2 == 1 {
			name = compact + "_capital_flow.json"
		}
		rec := map[string]any{
			"date":                 d,
			"foreign_investor_net": float64(i) + 0.5,
			"domestic_fund_net":    float64(i) + 0.25,
			"dealer_net":           float64(i) - 0.25,
			"total_net":            float64(i) + 0.5,
		}
		data, err := json.Marshal(rec)
		if err != nil {
			t.Fatalf("marshal t86 fixture: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatalf("write t86 fixture %s: %v", name, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "_metadata.json"), []byte(`{"generated_at":"test"}`), 0o644); err != nil {
		t.Fatalf("write metadata fixture: %v", err)
	}
}

// TestImportHistory verifies the CAL-1 contract: importing 30 days of
// T86 samples into an empty store makes History return exactly 30
// samples per dimension (sample_count=30), re-importing the same
// dates collapses to the same 30 (last-write-wins dedup), and the
// capacity trim still applies.
func TestImportHistory(t *testing.T) {
	ctx := context.Background()
	dates := genDates("2026-05-01", 30)
	samples := genT86Samples(dates)

	store := NewMemoryRollingSampleStore(60)
	if err := store.ImportHistory(ctx, samples); err != nil {
		t.Fatalf("ImportHistory: %v", err)
	}

	const sentinel = "9999-12-31"
	for _, dim := range []ForceName{ForceForeign, ForceInstitutional, ForceDealer} {
		got, err := store.History(ctx, dim, sentinel, 100)
		if err != nil {
			t.Fatalf("History(%s): %v", dim, err)
		}
		if len(got) != 30 {
			t.Fatalf("History(%s) returned %d samples, want 30", dim, len(got))
		}
		// ascending order + provenance intact
		for i, s := range got {
			if s.TradingDate != dates[i] {
				t.Fatalf("History(%s)[%d].TradingDate=%q, want %q", dim, i, s.TradingDate, dates[i])
			}
			if s.SourceID != SourceTWSET86 || s.Unit != "hundred_million_shares" {
				t.Fatalf("History(%s)[%d] provenance=%q/%q, want SRC-TWSE-T86/hundred_million_shares", dim, i, s.SourceID, s.Unit)
			}
		}
	}

	// Re-importing the same dates must not grow the store (CF-INV-05).
	if err := store.ImportHistory(ctx, samples); err != nil {
		t.Fatalf("second ImportHistory: %v", err)
	}
	for _, dim := range []ForceName{ForceForeign, ForceInstitutional, ForceDealer} {
		got, _ := store.History(ctx, dim, sentinel, 100)
		if len(got) != 30 {
			t.Fatalf("after re-import History(%s) returned %d samples, want 30 (last-write-wins)", dim, len(got))
		}
	}

	// Capacity trim: a 10-capacity store keeps only the newest 10.
	trimmed := NewMemoryRollingSampleStore(10)
	if err := trimmed.ImportHistory(ctx, samples); err != nil {
		t.Fatalf("trimmed ImportHistory: %v", err)
	}
	for _, dim := range []ForceName{ForceForeign, ForceInstitutional, ForceDealer} {
		got, _ := trimmed.History(ctx, dim, sentinel, 100)
		if len(got) != 10 {
			t.Fatalf("trimmed History(%s) returned %d samples, want 10", dim, len(got))
		}
		if got[0].TradingDate != dates[20] || got[9].TradingDate != dates[29] {
			t.Fatalf("trimmed History(%s) kept wrong window: %q..%q", dim, got[0].TradingDate, got[9].TradingDate)
		}
	}
}

// TestImportHistory_FileStorePersists verifies ImportHistory writes
// atomically to disk: a second FileRollingSampleStore against the same
// file (process-restart equivalent) reads back all 30 samples.
func TestImportHistory_FileStorePersists(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rolling.json")
	dates := genDates("2026-05-01", 30)
	samples := genT86Samples(dates)

	store := NewFileRollingSampleStore(path, 60)
	if err := store.ImportHistory(ctx, samples); err != nil {
		t.Fatalf("ImportHistory: %v", err)
	}

	reloaded := NewFileRollingSampleStore(path, 60)
	for _, dim := range []ForceName{ForceForeign, ForceInstitutional, ForceDealer} {
		got, err := reloaded.History(ctx, dim, "9999-12-31", 100)
		if err != nil {
			t.Fatalf("reloaded History(%s): %v", dim, err)
		}
		if len(got) != 30 {
			t.Fatalf("reloaded History(%s) returned %d samples, want 30", dim, len(got))
		}
	}
}

// TestImportHistory_Validation verifies a malformed batch fails before
// any write lands (atomicity): the store file stays empty.
func TestImportHistory_Validation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "rolling.json")
	store := NewFileRollingSampleStore(path, 60)

	bad := []RollingSample{
		{TradingDate: "not-a-date", Dimension: ForceForeign, RawValue: 1, Unit: "hundred_million_shares", SourceID: SourceTWSET86},
	}
	if err := store.ImportHistory(ctx, bad); err == nil {
		t.Fatal("ImportHistory with bad date: want error, got nil")
	}
	unknownDim := []RollingSample{
		{TradingDate: "2026-05-01", Dimension: ForceName("bogus"), RawValue: 1, Unit: "x", SourceID: "y"},
	}
	if err := store.ImportHistory(ctx, unknownDim); err == nil {
		t.Fatal("ImportHistory with unknown dimension: want error, got nil")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("store file must not exist after failed imports (atomicity); stat err=%v", err)
	}
	// empty batch is a no-op
	if err := store.ImportHistory(ctx, nil); err != nil {
		t.Fatalf("ImportHistory(nil): %v", err)
	}
}

// TestBuildHistorySamples verifies the replay + T86 → samples builder:
// 30 replay dates with T86 snapshots produce 30 dates × 3 real-source
// dimensions, the four no-source dimensions are reported, fromDate
// filters the window, and replay dates without a T86 snapshot are
// counted as skipped.
func TestBuildHistorySamples(t *testing.T) {
	dir := t.TempDir()
	dates := genDates("2026-05-01", 30)
	replayPath := filepath.Join(dir, "replay.csv")
	writeReplayFixture(t, replayPath, dates)
	t86Dir := filepath.Join(dir, "t86")
	if err := os.MkdirAll(t86Dir, 0o755); err != nil {
		t.Fatalf("mkdir t86: %v", err)
	}
	writeT86Fixture(t, t86Dir, dates)

	samples, rep, err := BuildHistorySamples(replayPath, t86Dir, "")
	if err != nil {
		t.Fatalf("BuildHistorySamples: %v", err)
	}
	if rep.ImportedDates != 30 || rep.ImportedSamples != 90 {
		t.Fatalf("report imported = %d dates / %d samples, want 30/90", rep.ImportedDates, rep.ImportedSamples)
	}
	if rep.DateRange != [2]string{"2026-05-01", "2026-05-30"} {
		t.Fatalf("DateRange=%v, want [2026-05-01 2026-05-30]", rep.DateRange)
	}
	if len(samples) != 90 {
		t.Fatalf("samples len=%d, want 90", len(samples))
	}
	// every sample is one of the three real-source dimensions
	wantDims := map[ForceName]bool{ForceForeign: true, ForceInstitutional: true, ForceDealer: true}
	for _, s := range samples {
		if !wantDims[s.Dimension] {
			t.Fatalf("unexpected dimension %q in import batch", s.Dimension)
		}
		if s.SourceID != SourceTWSET86 {
			t.Fatalf("sample source=%q, want SRC-TWSE-T86", s.SourceID)
		}
	}
	// the four no-source dimensions must be flagged, not fabricated
	if len(rep.NeedsRealSource) != 4 {
		t.Fatalf("NeedsRealSource=%v, want 4 dimensions", rep.NeedsRealSource)
	}

	// fromDate window filter
	samples2, rep2, err := BuildHistorySamples(replayPath, t86Dir, "2026-05-15")
	if err != nil {
		t.Fatalf("BuildHistorySamples(from): %v", err)
	}
	if rep2.ImportedDates != 16 || len(samples2) != 48 {
		t.Fatalf("from-filter imported = %d dates / %d samples, want 16/48", rep2.ImportedDates, len(samples2))
	}
	if rep2.DateRange[0] != "2026-05-15" {
		t.Fatalf("from-filter DateRange[0]=%q, want 2026-05-15", rep2.DateRange[0])
	}

	// replay date without T86 snapshot → skipped, not imported
	extraReplay := filepath.Join(dir, "replay_extra.csv")
	writeReplayFixture(t, extraReplay, append([]string{"2026-04-29", "2026-04-30"}, dates...))
	samples3, rep3, err := BuildHistorySamples(extraReplay, t86Dir, "")
	if err != nil {
		t.Fatalf("BuildHistorySamples(skip): %v", err)
	}
	if rep3.ImportedDates != 30 || rep3.SkippedDatesNoT86 != 2 {
		t.Fatalf("skip report = %d imported / %d skipped, want 30/2", rep3.ImportedDates, rep3.SkippedDatesNoT86)
	}
	if len(samples3) != 90 {
		t.Fatalf("skip samples len=%d, want 90", len(samples3))
	}
}

// TestLoadT86CapitalFlow_NormalizesDates verifies both production date
// shapes ("2026-08-14" and "20260814") normalize to YYYY-MM-DD and
// _metadata.json is ignored.
func TestLoadT86CapitalFlow_NormalizesDates(t *testing.T) {
	dir := t.TempDir()
	writeT86Fixture(t, dir, genDates("2026-08-10", 3))

	got, err := LoadT86CapitalFlow(dir)
	if err != nil {
		t.Fatalf("LoadT86CapitalFlow: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("loaded %d records, want 3", len(got))
	}
	for _, d := range []string{"2026-08-10", "2026-08-11", "2026-08-12"} {
		if _, ok := got[d]; !ok {
			t.Fatalf("missing normalized date %s in %v", d, got)
		}
	}
}

// TestLoadReplayTradingDates verifies header skip, dedup, and sort.
func TestLoadReplayTradingDates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replay.csv")
	dates := []string{"2026-08-12", "2026-08-10", "2026-08-12", "2026-08-11"}
	writeReplayFixture(t, path, dates)

	got, err := LoadReplayTradingDates(path)
	if err != nil {
		t.Fatalf("LoadReplayTradingDates: %v", err)
	}
	want := []string{"2026-08-10", "2026-08-11", "2026-08-12"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("got %v, want %v", got, want)
	}
}
