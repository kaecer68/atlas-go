package retail

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// writeMarginFile writes a margin cache file in the same shape as
// marketdata.TWSEMarginBalanceProvider.saveMargin:
// data/state/margin/<date>_margin.json with a margin_balance field.
func writeMarginFile(t *testing.T, dir, date string, balance float64) {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"date":           date,
		"margin_balance": balance,
	})
	if err != nil {
		t.Fatalf("marshal margin file: %v", err)
	}
	path := filepath.Join(dir, date+"_margin.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// writeMarginFiles writes n consecutive-trading-day margin files ending at
// endDate. Balances increase monotonically over time: the oldest file gets
// startBalance and the newest (endDate) gets startBalance + step*(n-1).
func writeMarginFiles(t *testing.T, dir string, endDate string, startBalance, step float64, n int) {
	t.Helper()
	date := endDate
	for i := 0; i < n; i++ {
		writeMarginFile(t, dir, date, startBalance+step*float64(n-1-i))
		date = prevTradingDate(date)
	}
}

// prevTradingDate walks back to the previous weekday (approximation of a
// trading day; sufficient for ordering tests).
func prevTradingDate(date string) string {
	t, err := parseMarginDate(date)
	if err != nil {
		return ""
	}
	for {
		t = t.AddDate(0, 0, -1)
		if t.Weekday() != 0 && t.Weekday() != 6 { // skip Sun/Sat
			return t.Format("20060102")
		}
	}
}

func parseMarginDate(date string) (time.Time, error) {
	return time.Parse("20060102", date)
}

func TestBackfillMarginHistory_SeedsHistoryFromDisk(t *testing.T) {
	dir := t.TempDir()
	// 5 consecutive trading days, balances rising 5000 → 5400 (oldest first).
	writeMarginFiles(t, dir, "20260817", 5000, 100, 5)

	c := NewCalculator()
	n, err := c.BackfillMarginHistory(dir)
	if err != nil {
		t.Fatalf("BackfillMarginHistory: %v", err)
	}
	if n != 5 {
		t.Fatalf("backfilled %d entries, want 5", n)
	}

	c.mu.RLock()
	got := make([]float64, len(c.marginHistory))
	copy(got, c.marginHistory)
	c.mu.RUnlock()
	if len(got) != 5 {
		t.Fatalf("marginHistory len = %d, want 5", len(got))
	}
	// Ordered ascending by date: 5000, 5100, ..., 5400.
	for i, want := range []float64{5000, 5100, 5200, 5300, 5400} {
		if got[i] != want {
			t.Errorf("marginHistory[%d] = %v, want %v", i, got[i], want)
		}
	}

	// Current value above history → A1 z-score computed, not fallback.
	c.SetParams(config.DefaultParametersConfig().RSITw)
	res := c.ComputeFinal(RSITwInput{MarginBalance: 5500})
	si, ok := res.SubIndicators["a1_margin_z"]
	if !ok {
		t.Fatal("missing a1_margin_z sub-indicator")
	}
	if si.IsFallback {
		t.Error("a1_margin_z should NOT be fallback after disk backfill")
	}
	if si.ZScore == 0 {
		t.Error("a1_margin_z should have a non-zero z-score with history")
	}
}

func TestBackfillMarginHistory_NoFilesKeepsFallback(t *testing.T) {
	// Empty existing dir → graceful: no error, no history, fallback preserved.
	dir := t.TempDir()
	c := NewCalculator()
	n, err := c.BackfillMarginHistory(dir)
	if err != nil {
		t.Fatalf("BackfillMarginHistory on empty dir: %v", err)
	}
	if n != 0 {
		t.Fatalf("backfilled %d entries, want 0", n)
	}

	c.SetParams(config.DefaultParametersConfig().RSITw)
	res := c.ComputeFinal(RSITwInput{MarginBalance: 5000})
	si, ok := res.SubIndicators["a1_margin_z"]
	if !ok {
		t.Fatal("missing a1_margin_z sub-indicator")
	}
	if !si.IsFallback {
		t.Error("a1_margin_z should remain fallback with no history")
	}
}

func TestBackfillMarginHistory_MissingDirKeepsFallback(t *testing.T) {
	// Missing dir → error surfaced (caller logs warn) and fallback preserved.
	c := NewCalculator()
	n, err := c.BackfillMarginHistory(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected error for missing dir, got nil")
	}
	if n != 0 {
		t.Fatalf("backfilled %d entries, want 0", n)
	}

	c.SetParams(config.DefaultParametersConfig().RSITw)
	res := c.ComputeFinal(RSITwInput{MarginBalance: 5000})
	si, ok := res.SubIndicators["a1_margin_z"]
	if !ok {
		t.Fatal("missing a1_margin_z sub-indicator")
	}
	if !si.IsFallback {
		t.Error("a1_margin_z should remain fallback when dir is missing")
	}
}

func TestBackfillMarginHistory_CapsAt30(t *testing.T) {
	dir := t.TempDir()
	// 35 consecutive trading days ending 20260817, balances 5000 → 5340.
	writeMarginFiles(t, dir, "20260817", 5000, 10, 35)

	c := NewCalculator()
	n, err := c.BackfillMarginHistory(dir)
	if err != nil {
		t.Fatalf("BackfillMarginHistory: %v", err)
	}
	if n != marginHistoryBackfillMax {
		t.Fatalf("backfilled %d entries, want %d", n, marginHistoryBackfillMax)
	}

	c.mu.RLock()
	got := make([]float64, len(c.marginHistory))
	copy(got, c.marginHistory)
	c.mu.RUnlock()
	if len(got) != marginHistoryBackfillMax {
		t.Fatalf("marginHistory len = %d, want %d", len(got), marginHistoryBackfillMax)
	}
	// Last 30 of 35 (balances 5000..5340 ascending): the oldest 5 (5000..5040)
	// are dropped, so the kept window is 5050..5340.
	if got[0] != 5050 {
		t.Errorf("marginHistory[0] = %v, want 5050 (oldest of last 30)", got[0])
	}
	if got[len(got)-1] != 5340 {
		t.Errorf("marginHistory[last] = %v, want 5340 (newest)", got[len(got)-1])
	}
}

func TestBackfillMarginHistory_Idempotent(t *testing.T) {
	dir := t.TempDir()
	writeMarginFiles(t, dir, "20260817", 5000, 100, 5)

	c := NewCalculator()
	if n, err := c.BackfillMarginHistory(dir); err != nil || n != 5 {
		t.Fatalf("first backfill: n=%d err=%v, want n=5 err=nil", n, err)
	}

	// Second call is a no-op — no duplicate entries.
	n, err := c.BackfillMarginHistory(dir)
	if err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if n != 0 {
		t.Fatalf("second backfill returned %d, want 0 (no-op)", n)
	}

	c.mu.RLock()
	got := make([]float64, len(c.marginHistory))
	copy(got, c.marginHistory)
	c.mu.RUnlock()
	if len(got) != 5 {
		t.Fatalf("marginHistory len = %d, want 5 (no duplicates)", len(got))
	}
}

func TestBackfillMarginHistory_SkipsMalformedFiles(t *testing.T) {
	dir := t.TempDir()
	writeMarginFile(t, dir, "20260810", 5100)
	// Malformed file must not break the backfill.
	if err := os.WriteFile(filepath.Join(dir, "20260811_margin.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write malformed: %v", err)
	}
	// Unrelated file must be ignored.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write notes: %v", err)
	}
	writeMarginFile(t, dir, "20260812", 5300)

	c := NewCalculator()
	n, err := c.BackfillMarginHistory(dir)
	if err != nil {
		t.Fatalf("BackfillMarginHistory: %v", err)
	}
	if n != 2 {
		t.Fatalf("backfilled %d entries, want 2 (malformed/unrelated skipped)", n)
	}

	c.mu.RLock()
	got := make([]float64, len(c.marginHistory))
	copy(got, c.marginHistory)
	c.mu.RUnlock()
	if len(got) != 2 || got[0] != 5100 || got[1] != 5300 {
		t.Fatalf("marginHistory = %v, want [5100 5300]", got)
	}
}
