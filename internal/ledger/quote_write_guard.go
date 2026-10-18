package ledger

import (
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// Reason label values carried by the write-path rejection WARN line
// (`quote_write_rejected`). They name WHY a row was refused, so an operator can
// tell the documented close<=0 bad value from a non-finite one without reading
// the numeric fields.
const (
	// QuoteInvalidReasonNonPositive means a price column was <= 0 (the
	// documented close<=0 bad value, FU-20260930-03).
	QuoteInvalidReasonNonPositive = "non_positive"
	// QuoteInvalidReasonNonFinite means a price column was NaN or +/-Inf.
	QuoteInvalidReasonNonFinite = "non_finite"
)

// Backend names carried by the write-path rejection WARN line, one per
// QuoteStore implementation.
const (
	// QuoteBackendPostgres is the production quote backend.
	QuoteBackendPostgres = "postgres"
	// QuoteBackendSQLite is the opt-in sqlite mirror / local backend.
	QuoteBackendSQLite = "sqlite"
	// QuoteBackendJSONL is the legacy JSONL backend.
	QuoteBackendJSONL = "jsonl"
)

// ErrAllQuotesRejected is the sentinel wrapped by *QuoteRejectionsError: a
// non-empty batch reached RecordQuotes and the write-path guard refused every
// row, so nothing was persisted. Callers that treat a non-nil RecordQuotes
// error as "this symbol failed" keep working; a partially rejected batch does
// NOT return an error (the good rows are written), so one bad row can no longer
// abort a whole backfill.
var ErrAllQuotesRejected = errors.New("all quotes rejected by write-path validity guard")

// QuoteRejectionsError reports the all-rejected case with the identity of the
// first refused row, so an operator can find it without grepping the WARN log.
type QuoteRejectionsError struct {
	Backend  string // backend name (postgres / sqlite / jsonl)
	Total    int    // rows handed to RecordQuotes
	Rejected int    // rows refused by the guard (== Total when this is returned)
	Reason   string // reason of the first refused row
	Symbol   string // symbol of the first refused row
	Date     string // YYYY-MM-DD of the first refused row
}

// Error implements the error interface.
func (e *QuoteRejectionsError) Error() string {
	return fmt.Sprintf("%s quote store rejected %d/%d rows (first: %s %s, reason=%s): %v",
		e.Backend, e.Rejected, e.Total, e.Symbol, e.Date, e.Reason, ErrAllQuotesRejected)
}

// Unwrap exposes ErrAllQuotesRejected for errors.Is.
func (e *QuoteRejectionsError) Unwrap() error { return ErrAllQuotesRejected }

// quoteRejectionWarnCap bounds the per-row WARN lines this process emits.
//
// A single bad feed can carry thousands of close=0 rows (a suspended symbol
// backfilled over a year), and an unbounded per-row WARN would turn the guard
// into a log flood. One extra line announces the suppression, so the audit
// trail never claims there were no further rejects — same discipline as
// pricegate's warn cap.
const quoteRejectionWarnCap = 100

var quoteRejectionWarn struct {
	mu   sync.Mutex
	seen int
}

// quoteRejectionSummary tallies the rows of one batch that the guard refused.
type quoteRejectionSummary struct {
	Count  int
	Reason string
	Symbol string
	Date   string
}

// filterWritableQuotes returns the rows that may be persisted and the tally of
// the refused ones. Every refusal is reported (WARN) and never aborts the batch:
// a bad row is a missing observation, not a reason to drop the good ones.
func filterWritableQuotes(backend string, quotes []domain.DailyBar) ([]domain.DailyBar, quoteRejectionSummary) {
	kept := make([]domain.DailyBar, 0, len(quotes))
	var sum quoteRejectionSummary
	for _, bar := range quotes {
		reason, ok := quoteWriteRejectReason(bar)
		if ok {
			kept = append(kept, bar)
			continue
		}
		if sum.Count == 0 {
			sum.Reason = reason
			sum.Symbol = bar.Symbol
			sum.Date = bar.Date.UTC().Format("2006-01-02")
		}
		sum.Count++
		logQuoteWriteRejection(backend, reason, bar)
	}
	return kept, sum
}

// quoteWriteRejectReason reports why bar must not be persisted; ok=false means
// "write it".
//
// close carries the documented bad value (FU-20260930-03): a Close of 0 is
// written for days with no closing price, and every price->return consumer then
// manufactures a -100% return out of it. The read side already treats close<=0
// as missing (shared.IsUsablePrice); refusing it at the write side stops new
// rows of that shape instead of re-detecting them daily.
//
// open/high/low are guarded against non-finite values only. A zero
// Open/High/Low with a usable Close is a legitimate "intraday columns unknown"
// row in this codebase: cmd/cron-quote-backfill builds
// DailyBar{Symbol, Date, Close, Volume} only, so Open/High/Low are 0 by
// construction on that production path, and several providers fill them for a
// subset of days. Refusing those rows would throw away a usable close — data
// loss strictly worse than the defect this guard fixes. NaN/Inf can never mean a
// price and poisons every downstream aggregate, so they are refused everywhere.
func quoteWriteRejectReason(bar domain.DailyBar) (reason string, ok bool) {
	if !shared.IsUsablePrice(bar.Close) {
		if math.IsNaN(bar.Close) || math.IsInf(bar.Close, 0) {
			return QuoteInvalidReasonNonFinite, false
		}
		return QuoteInvalidReasonNonPositive, false
	}
	for _, v := range [3]float64{bar.Open, bar.High, bar.Low} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return QuoteInvalidReasonNonFinite, false
		}
	}
	return "", true
}

// logQuoteWriteRejection emits the per-row WARN, capped at
// quoteRejectionWarnCap lines per process. It is the only signal the guard
// produces: alerting on stored bad values is a database-level export concern
// (a2a-dev's postgres_exporter extended query), not a per-write counter, so no
// duplicate metric source exists in this repo.
func logQuoteWriteRejection(backend, reason string, bar domain.DailyBar) {
	quoteRejectionWarn.mu.Lock()
	quoteRejectionWarn.seen++
	n := quoteRejectionWarn.seen
	quoteRejectionWarn.mu.Unlock()

	if n > quoteRejectionWarnCap {
		if n == quoteRejectionWarnCap+1 {
			logging.Warn("ledger_quote_write_guard", "quote_write_rejection_suppressed",
				logging.FStr("reason", "warn_cap_reached"),
				logging.FInt("cap", quoteRejectionWarnCap),
				logging.FStr("hint", "further quote write rejections are not logged individually"))
		}
		return
	}
	logging.Warn("ledger_quote_write_guard", "quote_write_rejected",
		logging.FStr("backend", backend),
		logging.FStr("reason", reason),
		logging.FStr("symbol", bar.Symbol),
		logging.FStr("date", bar.Date.UTC().Format("2006-01-02")),
		logging.FStr("source", bar.Source),
		logging.FFloat64("open", bar.Open),
		logging.FFloat64("high", bar.High),
		logging.FFloat64("low", bar.Low),
		logging.FFloat64("close", bar.Close),
		logging.FInt("volume", int(bar.Volume)))
}

// allRejectedError builds the sentinel error for a batch whose rows were all
// refused, or nil when at least one row is writable.
func allRejectedError(backend string, total int, sum quoteRejectionSummary) error {
	if total == 0 || sum.Count < total {
		return nil
	}
	return &QuoteRejectionsError{
		Backend:  backend,
		Total:    total,
		Rejected: sum.Count,
		Reason:   sum.Reason,
		Symbol:   sum.Symbol,
		Date:     sum.Date,
	}
}
