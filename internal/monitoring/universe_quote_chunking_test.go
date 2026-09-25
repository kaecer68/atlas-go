package monitoring

// universe_quote_chunking_test.go pins the chunked quote fetch added for issue
// #1944 Batch 3 (I25 production-scale follow-up).
//
// Production evidence (2026-09-25 06:00Z): the per-stock substrate made the
// universe the whole listed market (`symbols_gathered count=1599`), and a single
// all-symbols GetQuotes call is unsafe — the fubon-proxy /quotes endpoint issues
// one upstream call per symbol, and HybridProvider falls back to FinMind (one
// HTTP request per symbol) whenever a single quote in the batch is incomplete.
// These tests pin the bounds: chunk size, per-chunk failure isolation, and the
// explicit partial-fetch status.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// chunkRecordingProvider records every GetQuotes call and answers with liquid
// quotes, optionally failing selected chunk indices.
type chunkRecordingProvider struct {
	calls        []int // chunk size per call
	chunkSizes   [][]int
	failChunkNth map[int]bool
	isMock       bool
}

func (p *chunkRecordingProvider) GetQuotes(_ context.Context, _ time.Time, symbols []string) ([]domain.Quote, error) {
	idx := len(p.calls)
	p.calls = append(p.calls, len(symbols))
	p.chunkSizes = append(p.chunkSizes, append([]int(nil), len(symbols)))
	if p.failChunkNth[idx] {
		return nil, fmt.Errorf("chunk %d: provider offline", idx)
	}
	out := make([]domain.Quote, 0, len(symbols))
	for _, s := range symbols {
		out = append(out, domain.Quote{Symbol: s, Last: 100, Volume: 1_000_000, AsOf: time.Now()})
	}
	return out, nil
}

func (p *chunkRecordingProvider) IsMock() bool { return p.isMock }

// constantFactorEng scores every symbol identically. The production
// portfolio.FactorEngine always returns momentum/value/quality/agent for any
// symbol (it needs no stored data), so the pipeline can rank every symbol whose
// quote arrived; the chunking tests must not accidentally depend on the test
// stub's per-symbol table.
type constantFactorEng struct{}

func (constantFactorEng) CalculateAllScores(string, map[string]domain.Quote, ...any) map[string]float64 {
	return map[string]float64{"pe": 60, "pb": 55, "volume": 70, "momentum": 65, "quality": 50, "foreign_flow": 40}
}

// scaleSymbols returns n deterministic TWSE-shaped symbols.
func scaleSymbols(n int) []string {
	out := make([]string, 0, n)
	for i := range n {
		out = append(out, fmt.Sprintf("%04d", 1000+i))
	}
	return out
}

// buildScaleDeps wires the pipeline for n symbols from a single industry.
func buildScaleDeps(t *testing.T, n int) UniverseBuilderDeps {
	t.Helper()
	symbols := scaleSymbols(n)
	classifications := make(map[string]*IndustryClassification, n)
	for _, s := range symbols {
		classifications[s] = &IndustryClassification{
			Symbol: s,
			Level1: IndustrySegment{ID: "semiconductor", Name: "半導體"},
		}
	}
	mapper := &mockMapper{
		classifications: classifications,
		byIndustry:      map[string][]string{"semiconductor": symbols},
	}
	tree := newFakeTree([]IndustrySegment{{ID: "semiconductor", Name: "半導體", Level: 1, Weight: 1.0}})

	deps := buildDepsFixture(t, tempDir(t))
	deps.Mapper = mapper
	deps.Tree = tree
	deps.FactorEng = constantFactorEng{}
	deps.RiskFilter = nil
	return deps
}

func TestFetchQuotesChunked_BoundsProviderCallSize(t *testing.T) {
	provider := &chunkRecordingProvider{}
	symbols := scaleSymbols(1599)

	quotes, stats, err := fetchQuotesChunked(context.Background(), provider, symbols,
		QuoteFetchPolicy{ChunkSize: 50, Pause: time.Microsecond})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.Requested != 1599 {
		t.Errorf("Requested = %d, want 1599", stats.Requested)
	}
	if want := 32; stats.Chunks != want {
		t.Errorf("Chunks = %d, want %d (1599/50 rounded up)", stats.Chunks, want)
	}
	if stats.ChunksFailed != 0 {
		t.Errorf("ChunksFailed = %d, want 0", stats.ChunksFailed)
	}
	if len(quotes) != 1599 {
		t.Errorf("quotes = %d, want 1599", len(quotes))
	}
	for i, size := range provider.calls {
		if size > 50 {
			t.Fatalf("chunk %d asked for %d symbols, want <= 50 (one provider call must stay bounded)", i, size)
		}
	}
	if got := provider.calls[len(provider.calls)-1]; got != 49 {
		t.Errorf("last chunk size = %d, want 49", got)
	}
}

func TestFetchQuotesChunked_PartialFailureKeepsOtherChunks(t *testing.T) {
	provider := &chunkRecordingProvider{failChunkNth: map[int]bool{1: true, 4: true}}
	quotes, stats, err := fetchQuotesChunked(context.Background(), provider, scaleSymbols(300),
		QuoteFetchPolicy{ChunkSize: 50, Pause: time.Microsecond})
	if err != nil {
		t.Fatalf("partial failure must not surface as a fetch error, got %v", err)
	}
	if stats.Chunks != 6 || stats.ChunksFailed != 2 {
		t.Fatalf("stats = %+v, want 6 chunks / 2 failed", stats)
	}
	if len(quotes) != 200 {
		t.Errorf("quotes = %d, want 200 (4 surviving chunks)", len(quotes))
	}
}

func TestFetchQuotesChunked_AllChunksFailIsError(t *testing.T) {
	provider := &chunkRecordingProvider{failChunkNth: map[int]bool{0: true, 1: true, 2: true}}
	quotes, stats, err := fetchQuotesChunked(context.Background(), provider, scaleSymbols(150),
		QuoteFetchPolicy{ChunkSize: 50, Pause: time.Microsecond})
	if err == nil {
		t.Fatal("expected an error when every chunk fails")
	}
	if len(quotes) != 0 || stats.ChunksFailed != 3 {
		t.Fatalf("quotes=%d stats=%+v, want 0 quotes / 3 failed chunks", len(quotes), stats)
	}
}

func TestBuildUniverse_ProductionScaleChunkedFetchRanksSymbols(t *testing.T) {
	deps := buildScaleDeps(t, 1599)
	provider := &chunkRecordingProvider{}
	deps.Quotes = provider
	deps.QuotePolicy = QuoteFetchPolicy{ChunkSize: 50, Pause: time.Microsecond}

	result, ranked, err := BuildUniverse(context.Background(), deps, false)
	if err != nil {
		t.Fatalf("BuildUniverse: %v", err)
	}
	t.Logf("input=%d ranked=%d chunks=%d chunks_failed=%d quotes_returned=%d quotes_status=%s",
		result.SymbolsFiltered, result.SymbolsRanked, result.QuotesChunks,
		result.QuotesChunksFailed, result.QuotesReturned, result.QuotesStatus)

	if result.SymbolsBuilt != 1599 {
		t.Fatalf("SymbolsBuilt = %d, want 1599", result.SymbolsBuilt)
	}
	if result.SymbolsFiltered != 1599 {
		t.Fatalf("SymbolsFiltered = %d, want 1599", result.SymbolsFiltered)
	}
	if result.SymbolsRanked <= 0 {
		t.Fatalf("SymbolsRanked = 0 at production scale (input=1599) — ranking is still blocked")
	}
	if len(ranked) != result.SymbolsRanked {
		t.Errorf("len(ranked) = %d, want %d", len(ranked), result.SymbolsRanked)
	}
	if result.QuotesStatus != QuotesStatusOK {
		t.Errorf("QuotesStatus = %q, want %q", result.QuotesStatus, QuotesStatusOK)
	}
	if !result.RankedTrustworthy {
		t.Errorf("RankedTrustworthy = false (reason %q), want true", result.RankedFallbackReason)
	}
	if result.QuotesRequested != 1599 {
		t.Errorf("QuotesRequested = %d, want 1599", result.QuotesRequested)
	}
	if result.QuotesChunks != 32 {
		t.Errorf("QuotesChunks = %d, want 32", result.QuotesChunks)
	}
	if result.QuotesReturned != 1599 {
		t.Errorf("QuotesReturned = %d, want 1599", result.QuotesReturned)
	}
	if provider.calls[0] > DefaultQuoteChunkSize {
		t.Errorf("first provider call asked for %d symbols, want <= %d", provider.calls[0], DefaultQuoteChunkSize)
	}
}

func TestBuildUniverse_PartialQuoteFetchIsExplicit(t *testing.T) {
	deps := buildScaleDeps(t, 300)
	provider := &chunkRecordingProvider{failChunkNth: map[int]bool{2: true}}
	deps.Quotes = provider
	deps.QuotePolicy = QuoteFetchPolicy{ChunkSize: 50, Pause: time.Microsecond}

	result, ranked, err := BuildUniverse(context.Background(), deps, false)
	if err != nil {
		t.Fatalf("BuildUniverse: %v", err)
	}
	if result.QuotesStatus != QuotesStatusPartial {
		t.Fatalf("QuotesStatus = %q, want %q", result.QuotesStatus, QuotesStatusPartial)
	}
	if result.QuotesChunksFailed != 1 {
		t.Errorf("QuotesChunksFailed = %d, want 1", result.QuotesChunksFailed)
	}
	if result.RankedTrustworthy {
		t.Error("RankedTrustworthy = true on a partial fetch — absence of the failed chunk's symbols would fabricate D6 failures")
	}
	if result.RankedFallbackReason != RankedFallbackQuoteFetchPartial {
		t.Errorf("RankedFallbackReason = %q, want %q", result.RankedFallbackReason, RankedFallbackQuoteFetchPartial)
	}
	if len(ranked) == 0 {
		t.Error("expected the surviving chunks to still produce a ranked list for inspection")
	}

	// The persisted snapshot must carry the same evidence, and a partial
	// snapshot must not be used as the D6 baseline.
	if err := SaveUniverseSnapshot(deps.WorkDir, result, ranked); err != nil {
		t.Fatalf("SaveUniverseSnapshot: %v", err)
	}
	snap, err := LoadUniverseSnapshot(deps.WorkDir)
	if err != nil {
		t.Fatalf("LoadUniverseSnapshot: %v", err)
	}
	if snap.Result == nil || snap.Result.QuotesChunksFailed != 1 {
		t.Fatalf("snapshot lost the chunk evidence: %+v", snap.Result)
	}
	if prev := loadPreviousRankedSymbols(deps.WorkDir); prev != nil {
		t.Errorf("partial snapshot must not become the D6 baseline, got %d symbols", len(prev))
	}
}

func TestBuildUniverse_AllChunksFailIsUntrustworthy(t *testing.T) {
	deps := buildScaleDeps(t, 100)
	failAll := &chunkRecordingProvider{failChunkNth: map[int]bool{0: true, 1: true}}
	deps.Quotes = failAll
	deps.QuotePolicy = QuoteFetchPolicy{ChunkSize: 50, Pause: time.Microsecond}

	result, _, err := BuildUniverse(context.Background(), deps, false)
	if err != nil {
		t.Fatalf("BuildUniverse must not return an error for a provider outage: %v", err)
	}
	if result.QuotesStatus != QuotesStatusFetchError {
		t.Errorf("QuotesStatus = %q, want %q", result.QuotesStatus, QuotesStatusFetchError)
	}
	if result.SymbolsRanked != 0 {
		t.Errorf("SymbolsRanked = %d, want 0 without quotes", result.SymbolsRanked)
	}
	if result.RankedTrustworthy {
		t.Error("RankedTrustworthy = true although no quote arrived")
	}
	if result.RankedFallbackReason != RankedFallbackQuoteFetchError {
		t.Errorf("RankedFallbackReason = %q, want %q", result.RankedFallbackReason, RankedFallbackQuoteFetchError)
	}
}
