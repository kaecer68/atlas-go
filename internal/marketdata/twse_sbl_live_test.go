package marketdata

// Env-gated live equivalence check for the first-party SBL source
// (fix/20260924-finmind-quota).
//
// Unlike the mock-based tests, this one hits the real official endpoints
// (免 key) and — when a FinMind-derived artifact is supplied — asserts that
// every row matches it value-for-value. It exists so the 2026-09-24
// equivalence claim can be re-run at any time (e.g. before/after an upstream
// table change), not just trusted from a one-off manual comparison.
//
//	ATLAS_TEST_SBL_LIVE=1 \
//	ATLAS_TEST_SBL_DATE=20260923 \
//	ATLAS_TEST_SBL_PROD_FILE=/path/to/data/state/sbl/20260923_sbl.json \
//	go test ./internal/marketdata -run LiveFirstParty -v
//
// Skipped by default: CI must not depend on upstream availability.

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"golang.org/x/time/rate"
)

func TestTWSESBL_LiveFirstParty_EquivalentToFinMindArtifact(t *testing.T) {
	if os.Getenv("ATLAS_TEST_SBL_LIVE") == "" {
		t.Skip("set ATLAS_TEST_SBL_LIVE=1 to hit the official TWSE/TPEx endpoints")
	}
	date := os.Getenv("ATLAS_TEST_SBL_DATE")
	if date == "" {
		date = "20260923"
	}

	oldTPEx := SetTPExSharedLimiterForTest(rate.NewLimiter(rate.Inf, 1))
	t.Cleanup(func() { SetTPExSharedLimiterForTest(oldTPEx) })

	p := NewTWSESBLProvider(0.5)
	p.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	p.SetStorageDir(t.TempDir())

	stats, err := p.FetchSBLSummary(context.Background(), date)
	if err != nil {
		t.Fatalf("live first-party fetch %s: %v", date, err)
	}
	// Baseline: 上市 ~1.3k + 上櫃 ~0.9k ≈ 2.2k symbols on a trading day.
	if len(stats) < 2000 {
		t.Fatalf("live rows = %d, want >= 2000 (both exchanges must answer; a single-host result = missing 上櫃 half)", len(stats))
	}
	if got := p.LastSource(); got != "TWSE:TWT93U+TPEx:margin/sbl" {
		t.Errorf("LastSource = %q, want both official hosts", got)
	}
	for _, s := range stats {
		if s.Date == "" || s.Symbol == "" {
			t.Fatalf("row missing date/symbol: %+v", s)
		}
	}

	prodFile := os.Getenv("ATLAS_TEST_SBL_PROD_FILE")
	if prodFile == "" {
		t.Logf("live rows = %d (%s); no artifact supplied, skipping value comparison", len(stats), p.LastSource())
		return
	}
	raw, err := os.ReadFile(prodFile)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	var want []SBLStats
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("parse artifact %s: %v", prodFile, err)
	}
	got := make(map[string]SBLStats, len(stats))
	for _, s := range stats {
		got[s.Symbol] = s
	}
	missing, mismatched := 0, 0
	for _, w := range want {
		g, ok := got[w.Symbol]
		if !ok {
			missing++
			if missing <= 5 {
				t.Errorf("symbol %s present in FinMind artifact but missing from first-party result", w.Symbol)
			}
			continue
		}
		if g.SBLShortBalance != w.SBLShortBalance || g.SBLShortVolume != w.SBLShortVolume || g.SBLReturnVolume != w.SBLReturnVolume {
			mismatched++
			if mismatched <= 5 {
				t.Errorf("symbol %s: first-party %+v != FinMind %+v", w.Symbol, g, w)
			}
		}
	}
	t.Logf("equivalence %s: artifact rows=%d, first-party rows=%d, mismatched=%d, missing=%d",
		date, len(want), len(stats), mismatched, missing)
	if mismatched != 0 || missing != 0 {
		t.Fatalf("first-party source differs from the FinMind artifact (mismatched=%d missing=%d)", mismatched, missing)
	}
}
