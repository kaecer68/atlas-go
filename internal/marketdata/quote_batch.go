package marketdata

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// QuoteOutcome is the per-symbol verdict of one provider call (issue #1986).
//
// The universe pipeline must be able to tell apart three different reasons a
// requested symbol came back without a quote, because only one of them is an
// acquisition failure:
//
//   - QuoteOutcomeNoData — the source answered authoritatively and there is no
//     tradable data for the symbol on that date (suspended, no trades that day,
//     delisted). This is a market fact, not a failure.
//   - QuoteOutcomeNotCovered — the source answered authoritatively for a whole
//     venue and the symbol is outside that venue's scope (TWSE's daily table
//     never contains 上櫃/TPEx names). Also a fact about the source's scope,
//     not an acquisition failure.
//   - QuoteOutcomeError — the call did not produce an answer: transport error,
//     non-2xx, rate limit, quota exhaustion, circuit breaker, timeout. This is
//     the only outcome that means "we do not know".
//
// Before #1986 the hybrid chain collapsed all three into "the whole batch is
// unusable", which both amplified one bad symbol into a chunk-wide per-symbol
// fallback and made partial coverage indistinguishable from a provider outage.
type QuoteOutcome string

const (
	// QuoteOutcomeOK means a complete domain.Quote was returned.
	QuoteOutcomeOK QuoteOutcome = "ok"
	// QuoteOutcomeNoData means the source has no tradable data for the symbol.
	QuoteOutcomeNoData QuoteOutcome = "no_data"
	// QuoteOutcomeNotCovered means the symbol is outside the source's venue scope.
	QuoteOutcomeNotCovered QuoteOutcome = "not_covered"
	// QuoteOutcomeError means the symbol could not be resolved by this source.
	QuoteOutcomeError QuoteOutcome = "error"
	// QuoteOutcomeNotAttempted means a provider never asked for the symbol
	// (its request budget was already spent). It is an acquisition gap, so it
	// must not be reported as a market fact.
	QuoteOutcomeNotAttempted QuoteOutcome = "not_attempted"
)

// IsBenign reports whether the outcome describes a market/source fact rather
// than a failed acquisition. Callers may only treat a missing quote as benign
// when every source that could have answered either answered it or declared it
// out of scope.
func (o QuoteOutcome) IsBenign() bool {
	return o == QuoteOutcomeOK || o == QuoteOutcomeNoData || o == QuoteOutcomeNotCovered
}

// QuoteBatch is one provider call's result plus a per-symbol verdict.
//
// Outcomes always carries one entry for every requested symbol: a symbol with
// no entry would be indistinguishable from a symbol the caller forgot to ask
// about, which is exactly the ambiguity #1986 was filed about.
type QuoteBatch struct {
	Quotes   []domain.Quote
	Outcomes map[string]QuoteOutcome
}

// NewQuoteBatch returns an empty batch whose outcomes are
// QuoteOutcomeNotAttempted for every requested symbol: the caller must
// explicitly record what happened, and a symbol that is still "not attempted"
// at the end of a chain is reported as an acquisition gap rather than as a
// market fact.
func NewQuoteBatch(symbols []string) QuoteBatch {
	b := QuoteBatch{Outcomes: make(map[string]QuoteOutcome, len(symbols))}
	for _, s := range symbols {
		b.Outcomes[s] = QuoteOutcomeNotAttempted
	}
	return b
}

// Record stores a quote and marks its symbol resolved.
func (b *QuoteBatch) Record(q domain.Quote) {
	b.RecordFor(q.Symbol, q)
}

// RecordFor stores a quote under the *requested* symbol spelling. Outcomes are
// keyed by the symbol the caller asked for, so a provider that answers with a
// differently spelled symbol (".TW" suffix, surrounding spaces) still resolves
// the request.
func (b *QuoteBatch) RecordFor(symbol string, q domain.Quote) {
	if b.Outcomes == nil {
		b.Outcomes = make(map[string]QuoteOutcome)
	}
	b.Quotes = append(b.Quotes, q)
	b.Outcomes[symbol] = QuoteOutcomeOK
}

// Resolve marks every still-unresolved symbol in symbols with outcome.
//
// "Unresolved" means the symbol has no outcome yet or its outcome is
// QuoteOutcomeError. A symbol that already has a resolved outcome
// (ok/no_data/not_covered) is left alone so a later source cannot overwrite a
// definitive answer with a weaker one.
func (b *QuoteBatch) Resolve(symbols []string, outcome QuoteOutcome) {
	if b.Outcomes == nil {
		b.Outcomes = make(map[string]QuoteOutcome, len(symbols))
	}
	for _, s := range symbols {
		if cur, ok := b.Outcomes[s]; ok && cur.IsBenign() {
			continue
		}
		b.Outcomes[s] = outcome
	}
}

// SetOutcomeFor records one verdict for many symbols, honoring the same
// "definitive answers win" rule as Resolve.
func (b *QuoteBatch) SetOutcomeFor(symbols []string, outcome QuoteOutcome) {
	for _, s := range symbols {
		b.SetOutcome(s, outcome)
	}
}

// Downgrade replaces a not_covered verdict with outcome.
//
// It exists for one case (issue #1986): a whole-market source declared a symbol
// out of scope, and then another source that also covers the population failed.
// "Outside every covered venue" is only a fact while every venue answered, so
// the verdict must fall back to an acquisition error.
func (b *QuoteBatch) Downgrade(symbols []string, outcome QuoteOutcome) {
	if b.Outcomes == nil {
		return
	}
	for _, s := range symbols {
		if b.Outcomes[s] == QuoteOutcomeNotCovered {
			b.Outcomes[s] = outcome
		}
	}
}

// SetOutcome records a single symbol's verdict, honoring the same
// "definitive answers win" rule as Resolve.
func (b *QuoteBatch) SetOutcome(symbol string, outcome QuoteOutcome) {
	if b.Outcomes == nil {
		b.Outcomes = make(map[string]QuoteOutcome)
	}
	if cur, ok := b.Outcomes[symbol]; ok && cur.IsBenign() {
		return
	}
	b.Outcomes[symbol] = outcome
}

// Missing returns the requested symbols that are not resolved yet, in input
// order. It is the residual a fallback arm must be asked for: asking an arm for
// the whole batch when one symbol is missing is the amplification #1986 fixed.
func (b QuoteBatch) Missing(symbols []string) []string {
	out := make([]string, 0, len(symbols))
	for _, s := range symbols {
		if o, ok := b.Outcomes[s]; ok && o.IsBenign() {
			continue
		}
		out = append(out, s)
	}
	return out
}

// MissingQuote returns the requested symbols that still have NO quote, in input
// order: outcomes ok and no_data are excluded, not_covered is NOT.
//
// The two notions of "missing" are deliberately separate:
//
//   - Missing (a chain asks each successive arm only for the residual the chain
//     has not resolved — not_covered counts as resolved, because for the chain
//     that declared it the symbol is out of scope).
//   - MissingQuote (a layer that has ANOTHER source covering the same
//     population asks for every symbol it still has no quote for — for the
//     coverage layer a not_covered verdict from a narrower source is not the
//     end of the story: the 上櫃 table may well publish it).
func (b QuoteBatch) MissingQuote(symbols []string) []string {
	out := make([]string, 0, len(symbols))
	for _, s := range symbols {
		switch b.Outcomes[s] {
		case QuoteOutcomeOK, QuoteOutcomeNoData:
			continue
		}
		out = append(out, s)
	}
	return out
}

// UnresolvedCount counts symbols whose outcome is not benign.
func (b QuoteBatch) UnresolvedCount(symbols []string) int {
	return len(b.Missing(symbols))
}

// MergeBatch folds src into dst: quotes are appended and src's per-symbol
// verdicts are copied with the "definitive answers win" rule.
func MergeBatch(dst *QuoteBatch, src QuoteBatch) {
	for _, q := range src.Quotes {
		dst.Record(q)
	}
	for sym, outcome := range src.Outcomes {
		dst.SetOutcome(sym, outcome)
	}
}

// PartialBatchProvider is the optional capability a market-data provider
// exposes when it can report per-symbol outcomes.
//
// The pipeline keeps marketdata.Provider minimal (GetQuotes) so every existing
// implementation stays valid; callers that need the classification type-assert
// to this interface (see GetQuotesBatch) and fall back to a caller-supplied
// default outcome for the symbols a non-aware provider dropped.
type PartialBatchProvider interface {
	GetQuotesBatch(ctx context.Context, asOf time.Time, symbols []string) (QuoteBatch, error)
}

// GetQuotesBatch asks p for quotes and returns the per-symbol verdict.
//
// missingOutcome is used for symbols a provider returned no quote for and that
// cannot classify them; callers pass QuoteOutcomeNotCovered for a source whose
// answer covers a whole venue (TWSE/TPEx daily tables) and QuoteOutcomeError
// for a per-symbol source where absence means the request failed.
func GetQuotesBatch(ctx context.Context, p Provider, asOf time.Time, symbols []string, missingOutcome QuoteOutcome) (QuoteBatch, error) {
	if p == nil {
		b := NewQuoteBatch(symbols)
		b.Resolve(symbols, missingOutcome)
		return b, nil
	}
	if aware, ok := p.(PartialBatchProvider); ok {
		return aware.GetQuotesBatch(ctx, asOf, symbols)
	}
	quotes, err := p.GetQuotes(ctx, asOf, symbols)
	batch := NewQuoteBatch(symbols)
	for _, q := range quotes {
		batch.Record(q)
	}
	batch.Resolve(symbols, missingOutcome)
	return batch, err
}
