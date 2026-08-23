package marketdata

import (
	"errors"
	"fmt"
)

// ErrRateLimited is returned when an API rate limit is encountered.
var ErrRateLimited = errors.New("rate limited")

// ErrTWSEQuoteNotFound is returned by TWSEClient.GetQuote when the symbol
// is absent from the TWSE STOCK_DAY_ALL snapshot. stocktools.HandleQuote
// uses errors.Is to distinguish "symbol is out of scope for every provider"
// (200 + coverage_note policy signal) from a genuine upstream failure (503).
var ErrTWSEQuoteNotFound = errors.New("twse: quote symbol not found")


// ─── P1-9: three-way error taxonomy ─────────────────────────────────────────
//
// Every provider-facing error should wrap exactly one of these three
// sentinels so adapters and the monitoring classifier can branch on typed
// errors instead of brittle string matching:
//
//	ErrNoData    — upstream answered but has no data for the requested day
//	               (holiday / weekend / not-yet-published). Expected; must
//	               NOT trip a circuit breaker and maps to "info" severity.
//	ErrUpstream  — the upstream failed: transport error, HTTP 4xx/5xx,
//	               timeout. Actionable; trips breakers, maps to "error".
//	ErrSchema    — the response was parseable transport-wise but does not
//	               match the expected schema (renamed/missing columns,
//	               HTML instead of JSON). Actionable; trips breakers, maps
//	               to "error".
var (
	ErrNoData    = errors.New("no data available")
	ErrUpstream  = errors.New("upstream failure")
	ErrSchema    = errors.New("schema mismatch")
)

// ErrTWSEEmptyData is returned by TWSEClient.GetQuotes when TWSE responds
// OK but delivers no quote rows (stat=OK with empty data, or an empty CSV
// payload). It wraps ErrNoData so errors.Is(err, ErrNoData) classifies it
// as a no-data condition (holiday / after-hours) rather than an outage.
var ErrTWSEEmptyData = fmt.Errorf("twse: empty data response: %w", ErrNoData)
