package orchestrator

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestDefaultSymbols_MergesFromReplayCSV verifies H4: the replay CSV written
// by daily-replay-sync uses a "Code" header (not "symbol"), and CSV symbols
// are normalized to ".TW"-suffixed form before merging so the universe
// actually expands instead of double-listing bare codes.
func TestDefaultSymbols_MergesFromReplayCSV(t *testing.T) {
	csvPath := filepath.Join(t.TempDir(), "replay.csv")
	content := "Date,Code,Name,TradeVolume,Open,High,Low,Close\n" +
		"2026-07-01,2330,台積電,100,900,910,890,905\n" +
		"2026-07-01,0056,元大高股息,100,40,41,39,40\n"
	if err := os.WriteFile(csvPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write fixture csv: %v", err)
	}

	got := loadSymbolsFromCSV(csvPath)
	want := []string{"0056", "2330"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loadSymbolsFromCSV with Code header: got %v want %v", got, want)
	}

	merged := mergeReplaySymbols([]string{"2330.TW", "0050.TW"}, got)
	if !contains(merged, "2330.TW") || !contains(merged, "0056.TW") {
		t.Fatalf("merged universe must contain normalized .TW symbols, got %v", merged)
	}
	for _, s := range merged {
		if s == "2330" || s == "0056" {
			t.Fatalf("bare code leaked into merged universe: %v", merged)
		}
	}

	// Empty CSV symbols leave the base untouched.
	unchanged := mergeReplaySymbols([]string{"2330.TW"}, nil)
	if !reflect.DeepEqual(unchanged, []string{"2330.TW"}) {
		t.Fatalf("nil CSV symbols must not change base, got %v", unchanged)
	}
}

// TestDefaultSymbols_FallsBackToBase verifies DefaultSymbols still returns
// the hardcoded base universe when the replay CSV is unavailable (the const
// path does not exist in the test working directory).
func TestDefaultSymbols_FallsBackToBase(t *testing.T) {
	got := DefaultSymbols()
	if len(got) < 41 {
		t.Fatalf("base universe should have at least 41 symbols, got %d", len(got))
	}
	if !contains(got, "2330.TW") {
		t.Errorf("base universe must contain 2330.TW, got %v", got)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
