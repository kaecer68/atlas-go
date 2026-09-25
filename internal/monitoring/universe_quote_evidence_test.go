package monitoring

// universe_quote_evidence_test.go pins the machine-readable quote-input
// evidence added for issue #1944 Batch 3 (I25 / N-U1 / N-U3 / N-U4 / N-U7).
//
// The production symptom was a snapshot claiming symbols_ranked=0 and
// ranked: [] while the real cause was UniverseBuilderDeps.Quotes == nil. These
// tests make that impossible to reintroduce silently: they assert the result
// and the persisted snapshot always name what happened to the quote input.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// liquidTestQuote / illiquidTestQuote bracket the filter's default liquidity
// threshold (minDailyAmount 5,000,000 TWD) using Volume * Last.
func liquidTestQuote(sym string) domain.Quote {
	return domain.Quote{Symbol: sym, Last: 100, Volume: 1_000_000, AsOf: time.Now()}
}

func illiquidTestQuote(sym string) domain.Quote {
	return domain.Quote{Symbol: sym, Last: 10, Volume: 1_000, AsOf: time.Now()}
}

// staticSubstrate is a minimal industry.SymbolIndustrySubstrate whose
// population is fixed, used to drive the "gathered but filtered away" branch.
type staticSubstrate struct{ symbols []string }

func (s staticSubstrate) ResolveL1(string) (industry.SectorID, bool) { return "", false }
func (s staticSubstrate) Symbols() []string                          { return s.symbols }

// TestBuildUniverseQuotesStatusOK verifies the trustworthy case: a wired
// provider that answers produces a non-empty ranked list with an explicit OK
// status and no fallback reason.
func TestBuildUniverseQuotesStatusOK(t *testing.T) {
	workDir := tempDir(t)
	deps := buildDepsFixture(t, workDir)

	result, ranked, err := BuildUniverse(context.Background(), deps, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.QuotesStatus != QuotesStatusOK {
		t.Errorf("QuotesStatus = %q, want %q", result.QuotesStatus, QuotesStatusOK)
	}
	if !result.RankedTrustworthy {
		t.Error("RankedTrustworthy = false, want true with a wired provider")
	}
	if result.RankedFallbackReason != "" {
		t.Errorf("RankedFallbackReason = %q, want empty", result.RankedFallbackReason)
	}
	if result.QuotesReturned != 2 {
		t.Errorf("QuotesReturned = %d, want 2", result.QuotesReturned)
	}
	if len(ranked) == 0 {
		t.Fatal("expected ranked > 0 when the quote provider is wired (I25 regression)")
	}
	if result.SymbolsRanked != len(ranked) {
		t.Errorf("SymbolsRanked = %d, len(ranked) = %d", result.SymbolsRanked, len(ranked))
	}
}

// TestBuildUniverseNilQuoteProviderIsExplicit verifies that a nil provider can
// no longer produce an unexplained zero: the result carries a machine-readable
// reason and the same fields land in the persisted snapshot.
func TestBuildUniverseNilQuoteProviderIsExplicit(t *testing.T) {
	workDir := tempDir(t)
	deps := buildDepsFixture(t, workDir)
	deps.Quotes = nil

	result, ranked, err := BuildUniverse(context.Background(), deps, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.QuotesStatus != QuotesStatusProviderUnavailable {
		t.Errorf("QuotesStatus = %q, want %q", result.QuotesStatus, QuotesStatusProviderUnavailable)
	}
	if result.RankedFallbackReason != RankedFallbackQuoteProviderUnavailable {
		t.Errorf("RankedFallbackReason = %q, want %q",
			result.RankedFallbackReason, RankedFallbackQuoteProviderUnavailable)
	}
	if result.RankedTrustworthy {
		t.Error("RankedTrustworthy = true with a nil quote provider; the zero would be misread as a market verdict")
	}
	if result.QuotesReturned != 0 {
		t.Errorf("QuotesReturned = %d, want 0", result.QuotesReturned)
	}
	if len(ranked) != 0 {
		t.Fatalf("expected ranked=0 without quotes, got %d", len(ranked))
	}

	// The snapshot must carry the same evidence, so an operator reading only
	// the file can tell the difference between "no provider" and "no market".
	snap, loadErr := LoadUniverseSnapshot(workDir)
	if loadErr != nil {
		t.Fatalf("LoadUniverseSnapshot: %v", loadErr)
	}
	if snap.Result == nil {
		t.Fatal("snapshot has no result block")
	}
	if snap.Result.QuotesStatus != QuotesStatusProviderUnavailable {
		t.Errorf("snapshot quotes_status = %q, want %q",
			snap.Result.QuotesStatus, QuotesStatusProviderUnavailable)
	}
	if snap.Result.RankedFallbackReason != RankedFallbackQuoteProviderUnavailable {
		t.Errorf("snapshot ranked_fallback_reason = %q, want %q",
			snap.Result.RankedFallbackReason, RankedFallbackQuoteProviderUnavailable)
	}
	if snap.Result.RankedTrustworthy {
		t.Error("snapshot ranked_trustworthy = true, want false")
	}

	// Raw JSON keys are part of the contract too: other readers (atlas-mcp,
	// operational scripts) parse the file, not the Go struct.
	raw, readErr := os.ReadFile(UniverseSnapshotPath(workDir))
	if readErr != nil {
		t.Fatalf("read snapshot: %v", readErr)
	}
	for _, key := range []string{`"quotes_status"`, `"quotes_returned"`, `"ranked_trustworthy"`, `"ranked_fallback_reason"`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("snapshot JSON missing %s:\n%s", key, raw)
		}
	}

	// A previous ranked list that was never a market verdict must not be
	// replayed as "previously ranked", which would fabricate D6 failures for
	// every symbol.
	if prev := loadPreviousRankedSymbols(workDir); prev != nil {
		t.Errorf("loadPreviousRankedSymbols = %v, want nil for an untrustworthy snapshot", prev)
	}
}

// TestBuildUniverseQuoteFetchDegradationsIsExplicit covers the two remaining
// degraded quote paths: a provider error and an empty answer.
func TestBuildUniverseQuoteFetchDegradationsIsExplicit(t *testing.T) {
	t.Run("fetch_error", func(t *testing.T) {
		workDir := tempDir(t)
		deps := buildDepsFixture(t, workDir)
		deps.Quotes = &mockQuoteProv{err: errors.New("provider offline")}

		result, ranked, err := BuildUniverse(context.Background(), deps, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.QuotesStatus != QuotesStatusFetchError {
			t.Errorf("QuotesStatus = %q, want %q", result.QuotesStatus, QuotesStatusFetchError)
		}
		if result.RankedFallbackReason != RankedFallbackQuoteFetchError {
			t.Errorf("RankedFallbackReason = %q, want %q",
				result.RankedFallbackReason, RankedFallbackQuoteFetchError)
		}
		if result.RankedTrustworthy {
			t.Error("RankedTrustworthy = true after a quote fetch error")
		}
		if len(ranked) != 0 {
			t.Errorf("expected ranked=0 on fetch error, got %d", len(ranked))
		}
	})

	t.Run("empty_quote_set", func(t *testing.T) {
		workDir := tempDir(t)
		deps := buildDepsFixture(t, workDir)
		deps.Quotes = &mockQuoteProv{quotes: map[string]domain.Quote{}}

		result, ranked, err := BuildUniverse(context.Background(), deps, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.QuotesStatus != QuotesStatusEmpty {
			t.Errorf("QuotesStatus = %q, want %q", result.QuotesStatus, QuotesStatusEmpty)
		}
		if result.RankedFallbackReason != RankedFallbackQuoteFetchEmpty {
			t.Errorf("RankedFallbackReason = %q, want %q",
				result.RankedFallbackReason, RankedFallbackQuoteFetchEmpty)
		}
		if result.RankedTrustworthy {
			t.Error("RankedTrustworthy = true for an empty quote set")
		}
		if len(ranked) != 0 {
			t.Errorf("expected ranked=0 with no quotes, got %d", len(ranked))
		}
	})
}

// TestBuildUniverseEarlyReturnReasonsIsExplicit verifies the two early returns
// also explain themselves instead of publishing an unattributed zero.
func TestBuildUniverseEarlyReturnReasonsIsExplicit(t *testing.T) {
	t.Run("empty_universe", func(t *testing.T) {
		workDir := tempDir(t)
		deps := buildDepsFixture(t, workDir)
		deps.Tree = nil
		deps.Mapper = nil
		deps.Substrate = nil

		result, _, err := BuildUniverse(context.Background(), deps, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.SymbolsBuilt != 0 {
			t.Fatalf("SymbolsBuilt = %d, want 0", result.SymbolsBuilt)
		}
		if result.RankedFallbackReason != RankedFallbackEmptyUniverse {
			t.Errorf("RankedFallbackReason = %q, want %q",
				result.RankedFallbackReason, RankedFallbackEmptyUniverse)
		}
		if result.QuotesStatus != QuotesStatusNotAttempted {
			t.Errorf("QuotesStatus = %q, want %q", result.QuotesStatus, QuotesStatusNotAttempted)
		}
		if result.RankedTrustworthy {
			t.Error("RankedTrustworthy = true for an empty universe")
		}
	})

	t.Run("empty_filtered", func(t *testing.T) {
		workDir := tempDir(t)
		deps := buildDepsFixture(t, workDir)
		// Substrate supplies the population, but the mapper cannot classify
		// it, so Layer 1 drops every symbol.
		deps.Substrate = staticSubstrate{symbols: []string{"9999"}}

		result, _, err := BuildUniverse(context.Background(), deps, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.SymbolsBuilt != 1 {
			t.Fatalf("SymbolsBuilt = %d, want 1 (substrate population)", result.SymbolsBuilt)
		}
		if result.SymbolsFiltered != 0 {
			t.Fatalf("SymbolsFiltered = %d, want 0", result.SymbolsFiltered)
		}
		if result.RankedFallbackReason != RankedFallbackEmptyFiltered {
			t.Errorf("RankedFallbackReason = %q, want %q",
				result.RankedFallbackReason, RankedFallbackEmptyFiltered)
		}
		if result.RankedTrustworthy {
			t.Error("RankedTrustworthy = true when the industry filter removed everything")
		}
	})
}

// TestSnapshotSchemaIsSingleAndCanonical pins N-U3: there is exactly one
// snapshot schema, and a snapshot written by the retired CLI schema is not a
// usable snapshot.
func TestSnapshotSchemaIsSingleAndCanonical(t *testing.T) {
	t.Run("roundtrip_preserves_evidence", func(t *testing.T) {
		workDir := tempDir(t)
		result := sampleUniverseBuildResult()
		result.QuotesStatus = QuotesStatusOK
		result.QuotesReturned = 7
		result.RankedTrustworthy = true
		ranked := sampleRankedSymbols(3)

		if err := SaveUniverseSnapshot(workDir, result, ranked); err != nil {
			t.Fatalf("SaveUniverseSnapshot: %v", err)
		}
		snap, err := LoadUniverseSnapshot(workDir)
		if err != nil {
			t.Fatalf("LoadUniverseSnapshot: %v", err)
		}
		if snap.Result == nil || snap.Result.QuotesStatus != QuotesStatusOK ||
			snap.Result.QuotesReturned != 7 || !snap.Result.RankedTrustworthy {
			t.Fatalf("evidence fields lost in roundtrip: %+v", snap.Result)
		}
		if len(snap.Ranked) != 3 {
			t.Fatalf("len(ranked) = %d, want 3", len(snap.Ranked))
		}
	})

	t.Run("legacy_cli_schema_is_not_a_snapshot", func(t *testing.T) {
		workDir := tempDir(t)
		dir := filepath.Join(workDir, "data", "state")
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		// The retired `-build-universe run` schema. It unmarshals into the
		// canonical struct without error, which is exactly why N-U3 was silent.
		legacy := `{"build_time":"2026-09-24T06:00:23Z","total_symbols":27,"ranked_count":0,"top_symbols":[]}`
		if err := os.WriteFile(UniverseSnapshotPath(workDir), []byte(legacy), 0o640); err != nil {
			t.Fatalf("write legacy snapshot: %v", err)
		}

		snap, err := LoadUniverseSnapshot(workDir)
		if err != nil {
			t.Fatalf("LoadUniverseSnapshot: %v", err)
		}
		if snap.Result != nil {
			t.Fatalf("legacy schema produced a non-nil result: %+v", snap.Result)
		}
		if len(snap.Ranked) != 0 {
			t.Fatalf("legacy schema produced ranked symbols: %+v", snap.Ranked)
		}

		// The retired reader classified this JSON as a valid universeSnapshot
		// and reported zeros. Assert the fixture really is the legacy shape so
		// the assertion above keeps testing what it claims to test.
		raw, readErr := os.ReadFile(UniverseSnapshotPath(workDir))
		if readErr != nil {
			t.Fatalf("read: %v", readErr)
		}
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(raw, &probe); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if _, ok := probe["result"]; ok {
			t.Fatal("legacy fixture unexpectedly carries a result block")
		}
		if _, ok := probe["build_time"]; !ok {
			t.Fatal("legacy fixture is not in the retired schema")
		}
	})
}

// TestD6WatchlistChainReachable is the N-U4 regression: the D6 consumer in
// cmd/atlas/main.go selects watchlist entries with ConsecutiveFailures >= 60,
// which could never happen while the ranked list was always empty. With quotes
// wired the pipeline produces a real previous-ranked list, a dropped symbol
// enters the watchlist, and repeated misses reach the threshold.
func TestD6WatchlistChainReachable(t *testing.T) {
	workDir := tempDir(t)
	deps := buildDepsFixture(t, workDir)

	// First run: both symbols ranked, so the snapshot holds a real ranked list.
	first, firstRanked, err := BuildUniverse(context.Background(), deps, false)
	if err != nil {
		t.Fatalf("first BuildUniverse: %v", err)
	}
	if !first.RankedTrustworthy || len(firstRanked) == 0 {
		t.Fatalf("first run produced no ranked list (trustworthy=%t ranked=%d)",
			first.RankedTrustworthy, len(firstRanked))
	}
	prev := loadPreviousRankedSymbols(workDir)
	if len(prev) == 0 {
		t.Fatal("loadPreviousRankedSymbols returned nothing after a trustworthy run")
	}

	// Second run: 2317 loses its quote, so it drops out of the ranked list.
	deps.Quotes = &mockQuoteProv{quotes: map[string]domain.Quote{
		"2330": {Symbol: "2330", Last: 500, Volume: 50_000_000, AsOf: time.Now()},
	}}
	_, secondRanked, err := BuildUniverse(context.Background(), deps, false)
	if err != nil {
		t.Fatalf("second BuildUniverse: %v", err)
	}

	const expiryDays = 60
	for i := range expiryDays {
		if err := CheckD6Expiry(workDir, secondRanked, prev, deps.Mapper, expiryDays); err != nil {
			t.Fatalf("CheckD6Expiry iteration %d: %v", i, err)
		}
	}

	raw, err := os.ReadFile(filepath.Join(workDir, "data", "state", "universe_watchlist.json"))
	if err != nil {
		t.Fatalf("read watchlist: %v", err)
	}
	var wl Watchlist
	if err := json.Unmarshal(raw, &wl); err != nil {
		t.Fatalf("unmarshal watchlist: %v", err)
	}
	// Same predicate as the D6 consumer in cmd/atlas/main.go.
	var expired []string
	for _, entry := range wl.Symbols {
		if entry.ConsecutiveFailures >= 60 {
			expired = append(expired, entry.Symbol)
		}
	}
	if len(expired) == 0 {
		t.Fatalf("no symbol reached the D6 consumer threshold; watchlist=%+v", wl.Symbols)
	}
}

// TestRiskExclusionLiquidityEvidence pins N-U7: the Layer 2.5 liquidity check
// used to return nothing when it had no quote for a symbol, so a nil
// QuoteProvider made the rule inert with no trace in the results. Every skip
// now records an INFO RuleDetail, and the real check still fails on a low
// daily amount.
func TestRiskExclusionLiquidityEvidence(t *testing.T) {
	findRule := func(t *testing.T, results []RiskExclusionResult, symbol, rule string) RuleDetail {
		t.Helper()
		for _, r := range results {
			if r.Symbol != symbol {
				continue
			}
			for _, rr := range r.RuleResults {
				if rr.RuleName == rule {
					return rr
				}
			}
		}
		t.Fatalf("no %q RuleDetail for %s (results=%+v)", rule, symbol, results)
		return RuleDetail{}
	}

	t.Run("no_provider_records_skip", func(t *testing.T) {
		rf := NewRiskExclusionFilter(nil, nil, nil)
		results, err := rf.Filter([]string{"2330"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rd := findRule(t, results, "2330", "liquidity")
		if !rd.Passed {
			t.Errorf("skipped liquidity rule must stay a pass (no fail-closed change), got Passed=false")
		}
		if rd.Severity != "INFO" {
			t.Errorf("Severity = %q, want INFO", rd.Severity)
		}
		if !strings.Contains(rd.Message, "skipped: quote provider not configured") {
			t.Errorf("Message = %q, want the not-configured skip reason", rd.Message)
		}
		if results[0].Passed != true {
			t.Error("a skipped liquidity check must not exclude the symbol")
		}
	})

	t.Run("provider_without_quote_for_symbol_records_skip", func(t *testing.T) {
		rf := NewRiskExclusionFilter(nil, &mockQuoteProv{quotes: map[string]domain.Quote{
			"2330": liquidTestQuote("2330"),
		}}, nil)
		results, err := rf.Filter([]string{"2330", "2454"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rd := findRule(t, results, "2454", "liquidity")
		if rd.Severity != "INFO" || !strings.Contains(rd.Message, "skipped: no quote returned") {
			t.Errorf("RuleDetail = %+v, want an INFO no-quote skip", rd)
		}
		// The covered symbol still gets a real evaluation.
		real := findRule(t, results, "2330", "liquidity")
		if strings.Contains(real.Message, "skipped") {
			t.Errorf("2330 has a quote, so the rule must run: %+v", real)
		}
	})

	t.Run("illiquid_symbol_fails_with_reason", func(t *testing.T) {
		rf := NewRiskExclusionFilter(nil, &mockQuoteProv{quotes: map[string]domain.Quote{
			"LIQ":   liquidTestQuote("LIQ"),
			"ILLIQ": illiquidTestQuote("ILLIQ"),
		}}, nil)
		results, err := rf.Filter([]string{"LIQ", "ILLIQ"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		rd := findRule(t, results, "ILLIQ", "liquidity")
		if rd.Passed {
			t.Errorf("illiquid symbol passed the liquidity rule: %+v", rd)
		}
		for _, r := range results {
			if r.Symbol != "ILLIQ" {
				continue
			}
			if r.Passed {
				t.Error("ILLIQ must not pass the filter")
			}
			found := false
			for _, reason := range r.FailReasons {
				if reason == "liquidity" {
					found = true
				}
			}
			if !found {
				t.Errorf("FailReasons = %v, want it to contain liquidity", r.FailReasons)
			}
		}
	})
}
