package marketdata

// coverage_completing_provider_test.go — issue #1986 requirement 3.
//
// The coverage layer is what turns "the TWSE table does not publish 上櫃 names"
// from a fake acquisition failure into an auditable source-scope fact, and what
// fills those symbols from the first-party TPEx table when it is up.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// stubMarketWideSource is a whole-market table stub.
type stubMarketWideSource struct {
	name   string
	quotes []domain.Quote
	err    error
	calls  int
}

func (s *stubMarketWideSource) Name() string { return s.name }

func (s *stubMarketWideSource) Snapshot(context.Context) ([]domain.Quote, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.quotes, nil
}

// stubPartialProvider answers with a fixed batch.
type stubPartialProvider struct {
	name  string
	batch func(symbols []string) QuoteBatch
	err   error
	calls int
}

func (s *stubPartialProvider) Name() string { return s.name }

func (s *stubPartialProvider) GetQuotes(context.Context, time.Time, []string) ([]domain.Quote, error) {
	return nil, s.err
}

func (s *stubPartialProvider) GetQuotesBatch(_ context.Context, _ time.Time, symbols []string) (QuoteBatch, error) {
	s.calls++
	if s.err != nil {
		return NewQuoteBatch(symbols), s.err
	}
	return s.batch(symbols), nil
}

func quoteFor(symbol string) domain.Quote {
	return domain.Quote{
		Symbol: symbol, Last: 100, Open: 99, High: 101, Low: 98,
		Volume: 1_000_000, Market: "TW", AsOf: time.Now(), Source: "tpex_daily_close",
	}
}

func TestCoverageCompleting_FillsResidualFromTheWholeMarketTable(t *testing.T) {
	symbols := []string{"2330", "1259", "4804"}
	primary := &stubPartialProvider{
		name: "primary",
		batch: func(syms []string) QuoteBatch {
			b := NewQuoteBatch(syms)
			b.Record(quoteFor("2330"))
			// The primary chain has no source covering TPEx, exactly like the
			// TWSE arm.
			b.Resolve(syms, QuoteOutcomeNotCovered)
			return b
		},
	}
	source := &stubMarketWideSource{
		name: "tpex_daily_close",
		quotes: []domain.Quote{
			quoteFor("1259"),
			// Present but with no usable price: the source publishes it, so it
			// is not an acquisition failure — but it also has no quote.
			{Symbol: "4804", Volume: 0},
		},
	}

	p := NewCoverageCompletingProvider(primary, source)
	batch, err := p.GetQuotesBatch(context.Background(), time.Now(), symbols)
	if err != nil {
		t.Fatalf("GetQuotesBatch: %v", err)
	}
	if got := batch.Outcomes["1259"]; got != QuoteOutcomeOK {
		t.Errorf("outcome for 1259 = %q, want ok (filled from the TPEx table)", got)
	}
	if len(batch.Quotes) != 2 {
		t.Errorf("quotes = %d, want 2", len(batch.Quotes))
	}
	// 4804 is still without a quote and every source answered, so it is a
	// source-scope fact rather than an acquisition gap.
	//
	// It stays not_covered rather than being refined to no_data: the TPEx row
	// proves the source publishes the symbol, but the primary chain already
	// declared the symbol out of scope and a definitive verdict is never
	// overwritten. Both are benign, so the gate is unaffected; the split between
	// them is what the snapshot's quotes_missing_* counters audit.
	if got := batch.Outcomes["4804"]; got != QuoteOutcomeNotCovered {
		t.Errorf("outcome for 4804 = %q, want not_covered", got)
	}
	if source.calls != 1 {
		t.Errorf("source calls = %d, want 1", source.calls)
	}
}

// TestCoverageCompleting_ReachableNoDataPath pins the case where the coverage
// layer is the first source able to say anything at all about a symbol: the
// primary chain reported an acquisition error, the TPEx table publishes the
// symbol with no usable price, so the honest verdict is no_data.
func TestCoverageCompleting_ReachableNoDataPath(t *testing.T) {
	symbols := []string{"4804"}
	primary := &stubPartialProvider{
		name: "primary",
		batch: func(syms []string) QuoteBatch {
			b := NewQuoteBatch(syms)
			b.Resolve(syms, QuoteOutcomeError)
			return b
		},
	}
	source := &stubMarketWideSource{
		name:   "tpex_daily_close",
		quotes: []domain.Quote{{Symbol: "4804", Volume: 0}},
	}
	p := NewCoverageCompletingProvider(primary, source)
	batch, err := p.GetQuotesBatch(context.Background(), time.Now(), symbols)
	if err != nil {
		t.Fatalf("GetQuotesBatch: %v", err)
	}
	if got := batch.Outcomes["4804"]; got != QuoteOutcomeNoData {
		t.Fatalf("outcome = %q, want no_data (the table publishes the symbol without a usable price)", got)
	}
	if !batch.Outcomes["4804"].IsBenign() {
		t.Error("no_data must be benign: a suspended stock is a market fact")
	}
}

func TestCoverageCompleting_FailedSourceDowngradesNotCoveredToError(t *testing.T) {
	symbols := []string{"2330", "1259"}
	primary := &stubPartialProvider{
		name: "primary",
		batch: func(syms []string) QuoteBatch {
			b := NewQuoteBatch(syms)
			b.Record(quoteFor("2330"))
			b.Resolve(syms, QuoteOutcomeNotCovered)
			return b
		},
	}
	source := &stubMarketWideSource{name: "tpex_daily_close", err: errors.New("upstream 503")}

	p := NewCoverageCompletingProvider(primary, source)
	batch, err := p.GetQuotesBatch(context.Background(), time.Now(), symbols)
	if err != nil {
		t.Fatalf("GetQuotesBatch: %v", err)
	}
	// "Outside every covered venue" is only a fact while every venue answered.
	if got := batch.Outcomes["1259"]; got != QuoteOutcomeError {
		t.Fatalf("outcome for 1259 = %q, want error (a coverage source was down)", got)
	}
	if batch.Outcomes["1259"].IsBenign() {
		t.Error("a downgraded outcome must not be benign")
	}
	if got := batch.Outcomes["2330"]; got != QuoteOutcomeOK {
		t.Errorf("outcome for 2330 = %q, want ok (a resolved symbol is untouched)", got)
	}
}

func TestCoverageCompleting_DeclaresNotCoveredOnlyWhenEverySourceAnswered(t *testing.T) {
	symbols := []string{"1259"}
	primary := &stubPartialProvider{
		name: "primary",
		batch: func(syms []string) QuoteBatch {
			b := NewQuoteBatch(syms)
			b.Resolve(syms, QuoteOutcomeNotCovered)
			return b
		},
	}
	// A second source that also answers, and also does not carry the symbol.
	second := &stubMarketWideSource{name: "other_venue", quotes: nil}

	p := NewCoverageCompletingProvider(primary, &stubMarketWideSource{name: "tpex_daily_close"}, second)
	batch, err := p.GetQuotesBatch(context.Background(), time.Now(), symbols)
	if err != nil {
		t.Fatalf("GetQuotesBatch: %v", err)
	}
	if got := batch.Outcomes["1259"]; got != QuoteOutcomeNotCovered {
		t.Fatalf("outcome = %q, want not_covered once every covered venue answered without it", got)
	}
}

func TestCoverageCompleting_NoSourceConfiguredPassesThePrimaryThrough(t *testing.T) {
	symbols := []string{"2330"}
	primary := &stubPartialProvider{
		name: "primary",
		batch: func(syms []string) QuoteBatch {
			b := NewQuoteBatch(syms)
			b.Record(quoteFor("2330"))
			return b
		},
	}
	p := NewCoverageCompletingProvider(primary)
	batch, err := p.GetQuotesBatch(context.Background(), time.Now(), symbols)
	if err != nil {
		t.Fatalf("GetQuotesBatch: %v", err)
	}
	if got := batch.Outcomes["2330"]; got != QuoteOutcomeOK {
		t.Errorf("outcome = %q, want ok", got)
	}
	if got := p.Name(); got != "coverage-completing-primary" {
		t.Errorf("Name() = %q", got)
	}
}

func TestCoverageCompleting_NilPrimaryUsesTheSourcesAlone(t *testing.T) {
	symbols := []string{"1259", "4804"}
	source := &stubMarketWideSource{name: "tpex_daily_close", quotes: []domain.Quote{quoteFor("1259")}}
	p := NewCoverageCompletingProvider(nil, source)
	batch, err := p.GetQuotesBatch(context.Background(), time.Now(), symbols)
	if err != nil {
		t.Fatalf("GetQuotesBatch: %v", err)
	}
	if got := batch.Outcomes["1259"]; got != QuoteOutcomeOK {
		t.Errorf("outcome for 1259 = %q, want ok", got)
	}
	if got := batch.Outcomes["4804"]; got != QuoteOutcomeNotCovered {
		t.Errorf("outcome for 4804 = %q, want not_covered", got)
	}
	if got := p.Name(); got != "coverage-completing-none" {
		t.Errorf("Name() = %q", got)
	}
}

func TestCoverageCompleting_CachesTheWholeMarketTableAcrossChunks(t *testing.T) {
	symbols := []string{"1259"}
	source := &stubMarketWideSource{name: "tpex_daily_close", quotes: []domain.Quote{quoteFor("1259")}}
	primary := &stubPartialProvider{
		name: "primary",
		batch: func(syms []string) QuoteBatch {
			b := NewQuoteBatch(syms)
			b.Resolve(syms, QuoteOutcomeNotCovered)
			return b
		},
	}
	p := NewCoverageCompletingProvider(primary, source)

	// 32 chunks, one call each: the pipeline's production shape.
	for range 32 {
		if _, err := p.GetQuotesBatch(context.Background(), time.Now(), symbols); err != nil {
			t.Fatalf("GetQuotesBatch: %v", err)
		}
	}
	if source.calls != 1 {
		t.Errorf("whole-market downloads = %d, want 1 (the table is fetched once, not per chunk)", source.calls)
	}
}

func TestCoverageCompleting_GetQuotesMatchesTheBatchQuotes(t *testing.T) {
	symbols := []string{"2330", "1259"}
	primary := &stubPartialProvider{
		name: "primary",
		batch: func(syms []string) QuoteBatch {
			b := NewQuoteBatch(syms)
			b.Record(quoteFor("2330"))
			b.Resolve(syms, QuoteOutcomeNotCovered)
			return b
		},
	}
	source := &stubMarketWideSource{name: "tpex_daily_close", quotes: []domain.Quote{quoteFor("1259")}}
	p := NewCoverageCompletingProvider(primary, source)

	quotes, err := p.GetQuotes(context.Background(), time.Now(), symbols)
	if err != nil {
		t.Fatalf("GetQuotes: %v", err)
	}
	if len(quotes) != 2 {
		t.Fatalf("quotes = %d, want 2 (the Provider interface must see the completed set)", len(quotes))
	}
}

func TestCoverageCompleting_SymbolSuffixAndWhitespaceStillMatch(t *testing.T) {
	// The snapshot/pipeline normalizes symbols, but a table row and a request
	// may still differ by the ".TW" suffix; the lookup must not miss.
	primary := &stubPartialProvider{
		name: "primary",
		batch: func(syms []string) QuoteBatch {
			b := NewQuoteBatch(syms)
			b.Resolve(syms, QuoteOutcomeNotCovered)
			return b
		},
	}
	source := &stubMarketWideSource{
		name:   "tpex_daily_close",
		quotes: []domain.Quote{{Symbol: "1259.TW", Last: 12.3, Open: 12.2, High: 12.45, Low: 12.15, Volume: 1000}},
	}
	p := NewCoverageCompletingProvider(primary, source)
	batch, err := p.GetQuotesBatch(context.Background(), time.Now(), []string{"1259"})
	if err != nil {
		t.Fatalf("GetQuotesBatch: %v", err)
	}
	if got := batch.Outcomes["1259"]; got != QuoteOutcomeOK {
		t.Fatalf("outcome = %q, want ok", got)
	}
}
