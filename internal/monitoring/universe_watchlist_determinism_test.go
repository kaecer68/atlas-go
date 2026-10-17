package monitoring

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

// watchlistClockField matches the only wall-clock field in the persisted
// watchlist (updated_at). It is normalized out of the determinism comparison so
// that the assertion is about ordering, not about how fast the test runs.
var watchlistClockField = regexp.MustCompile(`"updated_at":\s*"[^"]*"`)

func readWatchlistBytes(t *testing.T, workDir string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(workDir, "data", "state", "universe_watchlist.json"))
	if err != nil {
		t.Fatalf("read watchlist: %v", err)
	}
	return raw
}

func watchlistSHA(raw []byte) string {
	sum := sha256.Sum256(watchlistClockField.ReplaceAll(raw, []byte(`"updated_at": "<normalized>"`)))
	return hex.EncodeToString(sum[:])
}

func watchlistSymbols(t *testing.T, raw []byte) []string {
	t.Helper()
	var wl Watchlist
	if err := json.Unmarshal(raw, &wl); err != nil {
		t.Fatalf("unmarshal watchlist: %v", err)
	}
	out := make([]string, 0, len(wl.Symbols))
	for _, e := range wl.Symbols {
		out = append(out, e.Symbol)
	}
	return out
}

// TestCheckD6Expiry_DeterministicWatchlistOrder pins the persistence contract of
// <workDir>/data/state/universe_watchlist.json: wl.Symbols is persisted in
// ascending symbol order, so the same input always produces byte-identical
// output regardless of Go's randomized map iteration order.
//
// Before the fix the newly watched symbols were appended in the iteration order
// of the previouslyRankedSet map, so two runs of the same input produced a
// different file (FU-20261016-33).
func TestCheckD6Expiry_DeterministicWatchlistOrder(t *testing.T) {
	mapper := &mockMapper{classifications: map[string]*IndustryClassification{}}

	// 40 symbols that were ranked in the previous run and are absent today:
	// every one of them becomes a brand-new watchlist entry in one call.
	prev := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		prev = append(prev, fmt.Sprintf("%04d", 1000+i))
	}

	dirA := t.TempDir()
	dirB := t.TempDir()
	if err := CheckD6Expiry(dirA, nil, prev, mapper, 60); err != nil {
		t.Fatalf("CheckD6Expiry A: %v", err)
	}
	if err := CheckD6Expiry(dirB, nil, prev, mapper, 60); err != nil {
		t.Fatalf("CheckD6Expiry B: %v", err)
	}

	rawA := readWatchlistBytes(t, dirA)
	rawB := readWatchlistBytes(t, dirB)
	shaA := watchlistSHA(rawA)
	shaB := watchlistSHA(rawB)
	t.Logf("sha256 A (updated_at normalized) = %s", shaA)
	t.Logf("sha256 B (updated_at normalized) = %s", shaB)

	if string(rawA) != string(rawB) && shaA != shaB {
		t.Fatalf("watchlist is not byte-identical across two runs of the same input (sha256 %s vs %s)\nfirst symbols A: %v\nfirst symbols B: %v",
			shaA, shaB, watchlistSymbols(t, rawA)[:5], watchlistSymbols(t, rawB)[:5])
	}

	got := watchlistSymbols(t, rawA)
	want := slices.Clone(prev)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("watchlist symbols are not in canonical ascending order\n got: %v\nwant: %v", got, want)
	}
}
