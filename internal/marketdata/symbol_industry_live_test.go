package marketdata

// Env-gated live coverage check for the first-party per-stock industry source
// (issue #1943).
//
// Unlike the mock-based tests, this one hits the real official OpenAPI
// endpoints (免 key) and asserts the acceptance thresholds of issue #1943: the
// population must cover the overwhelming majority of the listed market (>= 800
// symbols), reach every canonical L1 sector, and report — never guess — the
// codes the declared vocabulary leaves unmapped.
//
//	ATLAS_TEST_SYMBOL_INDUSTRY_LIVE=1 \
//	go test ./internal/marketdata -run LiveSymbolIndustry -v
//
// It exists so the 2026-09-24 coverage numbers can be re-run at any time (e.g.
// after an upstream table change or when the sectormap table is updated), not
// just trusted from a one-off manual measurement.
//
// Skipped by default: CI must not depend on upstream availability.

import (
	"context"
	"os"
	"slices"
	"testing"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/sectormap"
)

func TestSymbolIndustry_LiveCoverage(t *testing.T) {
	if os.Getenv("ATLAS_TEST_SYMBOL_INDUSTRY_LIVE") == "" {
		t.Skip("set ATLAS_TEST_SYMBOL_INDUSTRY_LIVE=1 to hit the official TWSE/TPEx OpenAPI endpoints")
	}

	oldTWSE := SetTWSESharedLimiterForTest(rate.NewLimiter(rate.Inf, 1))
	t.Cleanup(func() { SetTWSESharedLimiterForTest(oldTWSE) })
	oldTPEx := SetTPExSharedLimiterForTest(rate.NewLimiter(rate.Inf, 1))
	t.Cleanup(func() { SetTPExSharedLimiterForTest(oldTPEx) })

	p := NewSymbolIndustryProvider(t.TempDir())
	snap, err := p.FetchAll(context.Background())
	if err != nil {
		t.Fatalf("live fetch: %v", err)
	}

	// Both exchanges must answer: a single-host result means half the market
	// (上櫃 or 上市) is missing, which is exactly the population bug #1943 is
	// about.
	if len(snap.Sources) != 2 {
		t.Fatalf("sources = %v, want both TWSE:t187ap03_L and TPEx:mopsfin_t187ap03_O", snap.Sources)
	}

	// Issue #1943 acceptance: >= 800 symbols must reach a canonical L1 sector.
	if snap.Counts.Mapped < 800 {
		t.Fatalf("mapped symbols = %d, want >= 800 (issue #1943 acceptance)", snap.Counts.Mapped)
	}
	// All 20 canonical L1 sectors must receive at least one symbol.
	if snap.Counts.CanonicalL1 != 20 {
		t.Errorf("canonical L1 reached = %d, want 20", snap.Counts.CanonicalL1)
	}
	if len(snap.L1Counts) != snap.Counts.CanonicalL1 {
		t.Errorf("l1_counts has %d entries but counts.canonical_l1 = %d", len(snap.L1Counts), snap.Counts.CanonicalL1)
	}

	// Every declared unmapped code must appear with a reason (report, do not
	// guess), and every code the vocabulary does not declare at all must be
	// reported as unknown.
	declaredCodes := map[string]sectormap.Status{}
	for _, c := range sectormap.TWSESIndustryCodes() {
		m := sectormap.Resolve(sectormap.NamespaceTWSESIndustryCode, c.Code)
		declaredCodes[c.Code] = m.Status
	}
	for _, d := range snap.UnmappedCodes {
		if d.Reason == "" {
			t.Errorf("unmapped code %s has no reason", d.Code)
		}
		if declaredCodes[d.Code] != sectormap.StatusUnmapped {
			t.Errorf("code %s reported unmapped but the namespace says %s", d.Code, declaredCodes[d.Code])
		}
	}
	for _, d := range snap.UnknownCodes {
		if _, declared := declaredCodes[d.Code]; declared {
			t.Errorf("code %s reported unknown but IS declared in namespace %s", d.Code, sectormap.NamespaceTWSESIndustryCode)
		}
		if d.Reason == "" {
			t.Errorf("unknown code %s has no reason", d.Code)
		}
	}

	// Entries must be deterministic and complete.
	if snap.Counts.Total != len(snap.Entries) {
		t.Errorf("counts.total = %d but %d entries", snap.Counts.Total, len(snap.Entries))
	}
	symbols := make([]string, 0, len(snap.Entries))
	for _, e := range snap.Entries {
		if e.Symbol == "" {
			t.Fatal("entry with empty symbol")
		}
		if e.MappingStatus == "mapped" && e.CanonicalL1 == "" {
			t.Errorf("symbol %s mapped but canonical_l1 empty", e.Symbol)
		}
		if e.MappingStatus != "mapped" && e.CanonicalL1 != "" {
			t.Errorf("symbol %s status=%s but canonical_l1=%q", e.Symbol, e.MappingStatus, e.CanonicalL1)
		}
		symbols = append(symbols, e.Symbol)
	}
	if !slices.IsSorted(symbols) {
		t.Error("entries are not sorted by symbol (the state file must be diffable)")
	}

	t.Logf("LIVE symbol_industry: total=%d mapped=%d unmapped=%d unknown=%d canonical_l1=%d sources=%v",
		snap.Counts.Total, snap.Counts.Mapped, snap.Counts.Unmapped, snap.Counts.Unknown,
		snap.Counts.CanonicalL1, snap.Sources)
	t.Logf("LIVE per-L1: %v", snap.L1Counts)
	t.Logf("LIVE unmapped codes: %+v", snap.UnmappedCodes)
	t.Logf("LIVE unknown codes: %+v", snap.UnknownCodes)
}
