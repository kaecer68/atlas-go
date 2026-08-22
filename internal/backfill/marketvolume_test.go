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

// stubVolumeFetcher returns a fixed result or error per requested date.
type stubVolumeFetcher struct {
	result *marketdata.MarketVolumeResult
	err    error
	// failTimes makes the first N calls fail (retry test).
	failTimes int
	calls     int
}

func (s *stubVolumeFetcher) FetchDate(_ context.Context, dateStr string) (*marketdata.MarketVolumeResult, error) {
	s.calls++
	if s.failTimes > 0 {
		s.failTimes--
		return nil, errors.New("stub fetch error")
	}
	if s.err != nil {
		return nil, s.err
	}
	return &marketdata.MarketVolumeResult{MarketVolume: 5200.25, Date: dateStr}, nil
}

// weekdayOnlyStub fails on weekends, like the real TWSE endpoint, so the
// backscan falls through to the previous trading day.
type weekdayOnlyStub struct {
	calls     []string // requested dates in order
	weekendOK bool     // when true, weekends also return data
}

func (s *weekdayOnlyStub) FetchDate(_ context.Context, dateStr string) (*marketdata.MarketVolumeResult, error) {
	s.calls = append(s.calls, dateStr)
	d, err := time.Parse("20060102", dateStr)
	if err != nil {
		return nil, err
	}
	if !s.weekendOK && (d.Weekday() == time.Saturday || d.Weekday() == time.Sunday) {
		return nil, errors.New("no TWSE data for weekend")
	}
	return &marketdata.MarketVolumeResult{MarketVolume: 5200.25, Date: dateStr}, nil
}

func writeMacroSnapshot(t *testing.T, dir, name string, withVolume bool) {
	t.Helper()
	body := "{\n  \"taiex\": {\"symbol\":\"^TWII\",\"value\":43000,\"change_pct\":0,\"timestamp\":0}\n"
	if withVolume {
		body += "  ,\"market_volume\": {\"symbol\":\"TSE_VOLUME\",\"value\":6600,\"change_pct\":0,\"timestamp\":1780000000}\n"
	}
	body += "}\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readVolumePoint(t *testing.T, path string) (marketdata.MacroDataPoint, bool) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var snap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatal(err)
	}
	rawPt, ok := snap["market_volume"]
	if !ok {
		return marketdata.MacroDataPoint{}, false
	}
	var pt marketdata.MacroDataPoint
	if err := json.Unmarshal(rawPt, &pt); err != nil {
		t.Fatal(err)
	}
	return pt, true
}

func TestBackfillMarketVolume_MergesPointIntoExistingSnapshot(t *testing.T) {
	dir := t.TempDir()
	writeMacroSnapshot(t, dir, "2026-04-27.json", false)
	writeMacroSnapshot(t, dir, "2026-04-28.json", false)

	stub := &stubVolumeFetcher{result: &marketdata.MarketVolumeResult{MarketVolume: 5200.25, Date: "20260427"}}
	result, err := BackfillMarketVolume(context.Background(), stub, dir, "2026-04-27", "2026-04-28", false)
	if err != nil {
		t.Fatalf("BackfillMarketVolume: %v", err)
	}
	if result.Scanned != 2 {
		t.Errorf("Scanned = %d, want 2", result.Scanned)
	}
	if result.Backfilled != 2 {
		t.Errorf("Backfilled = %d, want 2", result.Backfilled)
	}

	pt, ok := readVolumePoint(t, filepath.Join(dir, "2026-04-27.json"))
	if !ok {
		t.Fatal("market_volume key not present after backfill")
	}
	if pt.Symbol != "TSE_VOLUME" {
		t.Errorf("symbol = %q, want TSE_VOLUME", pt.Symbol)
	}
	if pt.Value != 5200.25 {
		t.Errorf("value = %v, want 5200.25", pt.Value)
	}
	if pt.ChangePct != 0 {
		t.Errorf("change_pct = %v, want 0", pt.ChangePct)
	}
	wantTs := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC).Unix()
	if pt.Timestamp != wantTs {
		t.Errorf("timestamp = %d, want %d (UTC midnight)", pt.Timestamp, wantTs)
	}
}

func TestBackfillMarketVolume_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	writeMacroSnapshot(t, dir, "2026-07-28.json", true)
	before, _ := os.ReadFile(filepath.Join(dir, "2026-07-28.json"))

	stub := &stubVolumeFetcher{result: &marketdata.MarketVolumeResult{MarketVolume: 9999, Date: "20260728"}}
	result, err := BackfillMarketVolume(context.Background(), stub, dir, "2026-07-28", "2026-07-28", false)
	if err != nil {
		t.Fatalf("BackfillMarketVolume: %v", err)
	}
	if result.SkippedExists != 1 {
		t.Errorf("SkippedExists = %d, want 1", result.SkippedExists)
	}
	if result.Backfilled != 0 {
		t.Errorf("Backfilled = %d, want 0", result.Backfilled)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "2026-07-28.json"))
	if string(before) != string(after) {
		t.Error("file changed despite refuse-overwrite")
	}
}

func TestBackfillMarketVolume_WeekendGetsPreviousTradingDay(t *testing.T) {
	dir := t.TempDir()
	writeMacroSnapshot(t, dir, "2026-04-25.json", false) // Saturday

	stub := &weekdayOnlyStub{}
	result, err := BackfillMarketVolume(context.Background(), stub, dir, "2026-04-25", "2026-04-25", false)
	if err != nil {
		t.Fatalf("BackfillMarketVolume: %v", err)
	}
	if result.Backfilled != 1 {
		t.Errorf("Backfilled = %d, want 1 (weekend carries last trading day)", result.Backfilled)
	}
	// Backscan: 04-25 (Sat, fails) → 04-24 (Fri, succeeds).
	wantCalls := []string{"20260425", "20260425", "20260425", "20260424"}
	if len(stub.calls) != len(wantCalls) {
		t.Fatalf("fetcher calls = %v, want %v", stub.calls, wantCalls)
	}
	for i, c := range wantCalls {
		if stub.calls[i] != c {
			t.Errorf("call %d = %q, want %q", i, stub.calls[i], c)
		}
	}
	pt, ok := readVolumePoint(t, filepath.Join(dir, "2026-04-25.json"))
	if !ok {
		t.Fatal("market_volume key not present after weekend backfill")
	}
	// Timestamp must be the source trading day (04-24), matching FetchLatest.
	wantTs := time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC).Unix()
	if pt.Timestamp != wantTs {
		t.Errorf("timestamp = %d, want %d (source trading day UTC midnight)", pt.Timestamp, wantTs)
	}
}

func TestBackfillMarketVolume_SkipsWhenNoDataInWindow(t *testing.T) {
	dir := t.TempDir()
	writeMacroSnapshot(t, dir, "2026-04-27.json", false)

	stub := &stubVolumeFetcher{err: errors.New("no data")}
	result, err := BackfillMarketVolume(context.Background(), stub, dir, "2026-04-27", "2026-04-27", false)
	if err != nil {
		t.Fatalf("BackfillMarketVolume: %v", err)
	}
	if result.SkippedNoData != 1 {
		t.Errorf("SkippedNoData = %d, want 1", result.SkippedNoData)
	}
	if result.Backfilled != 0 {
		t.Errorf("Backfilled = %d, want 0", result.Backfilled)
	}
	// 7 scanned days × maxBackfillAttempts retries each.
	if stub.calls != 7*maxBackfillAttempts {
		t.Errorf("fetcher calls = %d, want %d (7-day backscan × retry ≤ 3)", stub.calls, 7*maxBackfillAttempts)
	}
}

func TestBackfillMarketVolume_RetriesThenSucceeds(t *testing.T) {
	dir := t.TempDir()
	writeMacroSnapshot(t, dir, "2026-04-27.json", false)

	stub := &stubVolumeFetcher{failTimes: 2}
	result, err := BackfillMarketVolume(context.Background(), stub, dir, "2026-04-27", "2026-04-27", false)
	if err != nil {
		t.Fatalf("BackfillMarketVolume: %v", err)
	}
	if result.Backfilled != 1 {
		t.Errorf("Backfilled = %d, want 1 after transient failures", result.Backfilled)
	}
	if stub.calls != 3 {
		t.Errorf("fetcher calls = %d, want 3", stub.calls)
	}
}

func TestBackfillMarketVolume_DryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	writeMacroSnapshot(t, dir, "2026-04-27.json", false)
	before, _ := os.ReadFile(filepath.Join(dir, "2026-04-27.json"))

	stub := &stubVolumeFetcher{}
	result, err := BackfillMarketVolume(context.Background(), stub, dir, "2026-04-27", "2026-04-27", true)
	if err != nil {
		t.Fatalf("BackfillMarketVolume: %v", err)
	}
	if result.Backfilled != 1 {
		t.Errorf("Backfilled = %d, want 1 (dry-run counts)", result.Backfilled)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "2026-04-27.json"))
	if string(before) != string(after) {
		t.Error("dry-run must not modify the snapshot file")
	}
}

func TestBackfillMarketVolume_RespectsRange(t *testing.T) {
	dir := t.TempDir()
	writeMacroSnapshot(t, dir, "2026-04-27.json", false) // inside
	writeMacroSnapshot(t, dir, "2026-05-04.json", false) // inside
	writeMacroSnapshot(t, dir, "2026-06-01.json", false) // outside range

	stub := &stubVolumeFetcher{}
	result, err := BackfillMarketVolume(context.Background(), stub, dir, "2026-04-27", "2026-05-04", false)
	if err != nil {
		t.Fatalf("BackfillMarketVolume: %v", err)
	}
	if result.Scanned != 2 {
		t.Errorf("Scanned = %d, want 2 (out-of-range file untouched)", result.Scanned)
	}
	if _, ok := readVolumePoint(t, filepath.Join(dir, "2026-06-01.json")); ok {
		t.Error("out-of-range snapshot must not be backfilled")
	}
}

func TestBackfillMarketVolume_ParseErrors(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		start string
		end   string
	}{
		{"not-a-date", "2026-08-01"},
		{"2026-08-01", "not-a-date"},
		{"2026-08-10", "2026-08-01"}, // end before start
		{"", "2026-08-01"},
	}
	for _, tc := range cases {
		_, err := BackfillMarketVolume(context.Background(), &stubVolumeFetcher{}, dir, tc.start, tc.end, false)
		if err == nil {
			t.Errorf("BackfillMarketVolume(%q, %q): expected error, got nil", tc.start, tc.end)
		}
	}
}

func TestBackfillMarketVolume_IgnoresSpecialFiles(t *testing.T) {
	dir := t.TempDir()
	writeMacroSnapshot(t, dir, "2026-04-27.json", false)
	for _, name := range []string{"_metadata.json", "latest.json", "previous.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stub := &stubVolumeFetcher{}
	result, err := BackfillMarketVolume(context.Background(), stub, dir, "2026-04-27", "2026-04-27", false)
	if err != nil {
		t.Fatalf("BackfillMarketVolume: %v", err)
	}
	if result.Scanned != 1 {
		t.Errorf("Scanned = %d, want 1 (special files ignored)", result.Scanned)
	}
}

// TestBackfillMarketVolume_DateRangeParameterized is the contract test:
// the same range drives which dates are backfilled, and parsing is validated.
func TestBackfillMarketVolume_DateRangeParameterized(t *testing.T) {
	dir := t.TempDir()
	dates := []string{"2026-04-27", "2026-04-28", "2026-04-29", "2026-04-30", "2026-05-01", "2026-05-04"}
	for _, d := range dates {
		writeMacroSnapshot(t, dir, d+".json", false)
	}
	stub := &stubVolumeFetcher{}
	result, err := BackfillMarketVolume(context.Background(), stub, dir, "2026-04-28", "2026-04-30", false)
	if err != nil {
		t.Fatalf("BackfillMarketVolume: %v", err)
	}
	if result.Scanned != 3 {
		t.Errorf("Scanned = %d, want 3 (window 04-28..04-30)", result.Scanned)
	}
	if result.Backfilled != 3 {
		t.Errorf("Backfilled = %d, want 3", result.Backfilled)
	}
	if _, ok := readVolumePoint(t, filepath.Join(dir, "2026-04-27.json")); ok {
		t.Error("2026-04-27 is before the window and must not be backfilled")
	}
	if _, ok := readVolumePoint(t, filepath.Join(dir, "2026-05-01.json")); ok {
		t.Error("2026-05-01 is after the window and must not be backfilled")
	}
}
