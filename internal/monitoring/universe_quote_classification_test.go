package monitoring

// universe_quote_classification_test.go — issue #1986 requirement 3, at the
// pipeline boundary.
//
// The pipeline must report WHY a requested symbol came back without a quote, and
// must only mark the ranking untrustworthy for the reasons that are acquisition
// failures. The production failure this replaces: 1,599 requested, 1,301
// returned, 2 chunks failed → quotes_status=partial, ranked_trustworthy=false,
// with no way to tell how much of the 298-symbol gap was "the market has no such
// quote" and how much was "we failed to fetch it".

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// classifyingProvider answers with a fixed per-symbol verdict.
type classifyingProvider struct {
	bySymbol map[string]marketdata.QuoteOutcome
}

func (p *classifyingProvider) GetQuotes(ctx context.Context, asOf time.Time, symbols []string) ([]domain.Quote, error) {
	batch, err := p.GetQuotesBatch(ctx, asOf, symbols)
	return batch.Quotes, err
}

func (p *classifyingProvider) GetQuotesBatch(_ context.Context, _ time.Time, symbols []string) (marketdata.QuoteBatch, error) {
	batch := marketdata.NewQuoteBatch(symbols)
	for _, s := range symbols {
		switch p.bySymbol[s] {
		case marketdata.QuoteOutcomeOK:
			batch.Record(liquidTestQuote(s))
		case marketdata.QuoteOutcomeNoData:
			batch.SetOutcome(s, marketdata.QuoteOutcomeNoData)
		case marketdata.QuoteOutcomeNotCovered:
			batch.SetOutcome(s, marketdata.QuoteOutcomeNotCovered)
		case marketdata.QuoteOutcomeNotAttempted:
			// leave as-is
		default:
			batch.SetOutcome(s, marketdata.QuoteOutcomeError)
		}
	}
	return batch, nil
}

// plainProviderOnly implements GetQuotes and nothing else, i.e. a provider that
// cannot say why a symbol is absent.
type plainProviderOnly struct{ symbol string }

func (p plainProviderOnly) GetQuotes(_ context.Context, _ time.Time, symbols []string) ([]domain.Quote, error) {
	out := make([]domain.Quote, 0, len(symbols))
	for _, s := range symbols {
		if s == p.symbol {
			out = append(out, liquidTestQuote(s))
		}
	}
	return out, nil
}

func TestFetchQuotesChunked_ClassifiesMissingSymbols(t *testing.T) {
	symbols := []string{"1101", "1102", "1103", "1104", "1105"}
	provider := &classifyingProvider{bySymbol: map[string]marketdata.QuoteOutcome{
		"1101": marketdata.QuoteOutcomeOK,
		"1102": marketdata.QuoteOutcomeNoData,
		"1103": marketdata.QuoteOutcomeNotCovered,
		"1104": marketdata.QuoteOutcomeError,
		"1105": marketdata.QuoteOutcomeNotAttempted,
	}}

	quotes, stats, err := fetchQuotesChunked(context.Background(), provider, symbols, QuoteFetchPolicy{ChunkSize: 2})
	if err != nil {
		t.Fatalf("fetchQuotesChunked: %v", err)
	}
	if len(quotes) != 1 {
		t.Errorf("quotes = %d, want 1", len(quotes))
	}
	if stats.Requested != 5 || stats.Chunks != 3 || stats.ChunksFailed != 0 {
		t.Errorf("stats = %+v, want requested 5 / 3 chunks / 0 failed", stats)
	}
	if stats.Resolved != 1 || stats.NoData != 1 || stats.NotCovered != 1 {
		t.Errorf("classification = resolved %d / no_data %d / not_covered %d, want 1/1/1",
			stats.Resolved, stats.NoData, stats.NotCovered)
	}
	if stats.FetchError != 1 || stats.NotAttempted != 1 {
		t.Errorf("failure counts = fetch_error %d / not_attempted %d, want 1/1",
			stats.FetchError, stats.NotAttempted)
	}
	if got := stats.UnresolvedFailures(); got != 2 {
		t.Errorf("UnresolvedFailures = %d, want 2 (error + not_attempted)", got)
	}
}

// TestFetchQuotesChunked_NonClassifyingProviderIsAFailure pins the conservative
// default: a provider that cannot say why a symbol is absent must not have its
// gaps reported as a market fact.
func TestFetchQuotesChunked_NonClassifyingProviderIsAFailure(t *testing.T) {
	symbols := []string{"1101", "1102", "1103"}
	_, stats, err := fetchQuotesChunked(context.Background(), plainProviderOnly{symbol: "1101"}, symbols,
		QuoteFetchPolicy{ChunkSize: 2})
	if err != nil {
		t.Fatalf("fetchQuotesChunked: %v", err)
	}
	if stats.Resolved != 1 || stats.FetchError != 2 {
		t.Errorf("stats = %+v, want resolved 1 / fetch_error 2", stats)
	}
	if stats.NotCovered != 0 || stats.NoData != 0 {
		t.Errorf("a non-classifying provider must not yield benign categories: %+v", stats)
	}
}

// TestBuildUniverseGapClassification is the end-to-end statement of requirement
// 3: a population whose gaps are all source-scope facts produces
// quotes_status=ok and a trustworthy ranking, while the same gaps reported as
// acquisition failures do not.
func TestBuildUniverseGapClassification(t *testing.T) {
	for _, tc := range []struct {
		name            string
		outcome         marketdata.QuoteOutcome
		wantStatus      string
		wantTrustworthy bool
	}{
		{
			name:            "the sources do not publish the symbol",
			outcome:         marketdata.QuoteOutcomeNotCovered,
			wantStatus:      QuotesStatusOK,
			wantTrustworthy: true,
		},
		{
			name:            "the symbol has no tradable data",
			outcome:         marketdata.QuoteOutcomeNoData,
			wantStatus:      QuotesStatusOK,
			wantTrustworthy: true,
		},
		{
			name:            "the acquisition failed",
			outcome:         marketdata.QuoteOutcomeError,
			wantStatus:      QuotesStatusPartial,
			wantTrustworthy: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const population = 8
			deps := buildScaleDeps(t, population)
			symbols := deps.Mapper.GetSymbolsByIndustry("semiconductor")
			bySymbol := make(map[string]marketdata.QuoteOutcome, len(symbols))
			for i, s := range symbols {
				if i < 2 {
					bySymbol[s] = marketdata.QuoteOutcomeOK
					continue
				}
				bySymbol[s] = tc.outcome
			}
			deps.Quotes = &classifyingProvider{bySymbol: bySymbol}

			result, _, err := BuildUniverse(context.Background(), deps, false)
			if err != nil {
				t.Fatalf("BuildUniverse: %v", err)
			}

			if result.QuotesStatus != tc.wantStatus {
				t.Errorf("quotes_status = %q, want %q (reason %q)",
					result.QuotesStatus, tc.wantStatus, result.RankedFallbackReason)
			}
			if result.RankedTrustworthy != tc.wantTrustworthy {
				t.Errorf("ranked_trustworthy = %v, want %v (reason %q)",
					result.RankedTrustworthy, tc.wantTrustworthy, result.RankedFallbackReason)
			}
			if !tc.wantTrustworthy && result.RankedFallbackReason != RankedFallbackQuoteFetchPartial {
				t.Errorf("fallback reason = %q, want %q", result.RankedFallbackReason, RankedFallbackQuoteFetchPartial)
			}
			if tc.wantTrustworthy && result.RankedFallbackReason != "" {
				t.Errorf("fallback reason = %q, want empty for a trustworthy ranking", result.RankedFallbackReason)
			}

			// Every requested symbol must be accounted for exactly once.
			accounted := result.QuotesReturned + result.QuotesMissingNoData +
				result.QuotesMissingNotCovered + result.QuotesMissingFetchError +
				result.QuotesMissingNotAttempted
			if accounted != result.QuotesRequested {
				t.Errorf("returned + missing breakdown = %d, want quotes_requested %d", accounted, result.QuotesRequested)
			}
			switch tc.outcome {
			case marketdata.QuoteOutcomeNotCovered:
				if result.QuotesMissingNotCovered != population-2 {
					t.Errorf("quotes_missing_not_covered = %d, want %d", result.QuotesMissingNotCovered, population-2)
				}
			case marketdata.QuoteOutcomeNoData:
				if result.QuotesMissingNoData != population-2 {
					t.Errorf("quotes_missing_no_data = %d, want %d", result.QuotesMissingNoData, population-2)
				}
			default:
				if result.QuotesMissingFetchError != population-2 {
					t.Errorf("quotes_missing_fetch_error = %d, want %d", result.QuotesMissingFetchError, population-2)
				}
			}
			if result.QuotesChunksFailed != 0 {
				t.Errorf("quotes_chunks_failed = %d, want 0", result.QuotesChunksFailed)
			}
		})
	}
}

// TestBuildUniverseGapClassification_PersistsToSnapshot keeps the auditable
// fields reachable from the file every downstream reader consumes.
func TestBuildUniverseGapClassification_PersistsToSnapshot(t *testing.T) {
	deps := buildScaleDeps(t, 6)
	symbols := deps.Mapper.GetSymbolsByIndustry("semiconductor")
	bySymbol := make(map[string]marketdata.QuoteOutcome, len(symbols))
	for i, s := range symbols {
		if i < 2 {
			bySymbol[s] = marketdata.QuoteOutcomeOK
			continue
		}
		bySymbol[s] = marketdata.QuoteOutcomeNotCovered
	}
	deps.Quotes = &classifyingProvider{bySymbol: bySymbol}

	if _, _, err := BuildUniverse(context.Background(), deps, false); err != nil {
		t.Fatalf("BuildUniverse: %v", err)
	}
	snap, err := LoadUniverseSnapshot(deps.WorkDir)
	if err != nil {
		t.Fatalf("LoadUniverseSnapshot: %v", err)
	}
	if snap.Result == nil {
		t.Fatal("snapshot has no result block")
	}
	if snap.Result.QuotesMissingNotCovered != 4 {
		t.Errorf("snapshot quotes_missing_not_covered = %d, want 4", snap.Result.QuotesMissingNotCovered)
	}
	if !snap.Result.RankedTrustworthy {
		t.Errorf("snapshot ranked_trustworthy = false (reason %q)", snap.Result.RankedFallbackReason)
	}
}

func TestQuoteFetchPolicy_DefaultsUnchanged(t *testing.T) {
	// The chunk parameters are operational guardrails justified by measurement
	// in docs/specs/universe-quote-reliability-spec.md; a change must be
	// deliberate and re-measured.
	if DefaultQuoteChunkSize != 50 {
		t.Errorf("DefaultQuoteChunkSize = %d, want 50", DefaultQuoteChunkSize)
	}
	if DefaultQuoteChunkPause != 100*time.Millisecond {
		t.Errorf("DefaultQuoteChunkPause = %v, want 100ms", DefaultQuoteChunkPause)
	}
	if DefaultQuoteChunkTimeout != 60*time.Second {
		t.Errorf("DefaultQuoteChunkTimeout = %v, want 60s", DefaultQuoteChunkTimeout)
	}
}
