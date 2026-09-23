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
	ErrNoData   = errors.New("no data available")
	ErrUpstream = errors.New("upstream failure")
	ErrSchema   = errors.New("schema mismatch")
)

// ErrTWSEEmptyData is returned by TWSEClient.GetQuotes when TWSE responds
// OK but delivers no quote rows (stat=OK with empty data, or an empty CSV
// payload). It wraps ErrNoData so errors.Is(err, ErrNoData) classifies it
// as a no-data condition (holiday / after-hours) rather than an outage.
var ErrTWSEEmptyData = fmt.Errorf("twse: empty data response: %w", ErrNoData)

// ErrEmptyQuote is returned by quote-style providers when the upstream
// answered with a structurally valid payload that carries no price for the
// requested symbol — the CNBC ".BADI" case (2026-09-20T08:35Z onward): HTTP
// 200, JSON shape unchanged, QuickQuote present, `last` absent, open/high/low
// all "0.00", provider "CNBC Quote Cache".
//
// Why a fourth sentinel instead of reusing the existing three:
//
//	ErrUpstream    — wrong: there is no transport error and no HTTP 4xx/5xx.
//	ErrSchema      — wrong: the response still parses into the expected shape,
//	                 so the failure is data-side, not schema-side.
//	ErrNoData      — wrong FOR A DAILY CHANNEL: ErrNoData means "nothing for
//	                 the requested day yet" (holiday/weekend/not-yet-published)
//	                 and maps to the ok/waiting state. A channel whose upstream
//	                 normally prints every business day would then look "ok"
//	                 while it has been dark for days (bdi 2026-09-20 → 09-23),
//	                 and the staleness backstop does not catch it either
//	                 (staleness compares LastFetchAt, which a waiting record
//	                 refreshes on every tick).
//
// Consumers therefore treat ErrEmptyQuote as an ABNORMAL upstream condition:
//
//	gateway            — records the channel as "warn" (visible on the channel
//	                     page, no ChannelHealthStatusError page) and keeps the
//	                     last-known-good value downstream (narrative
//	                     mergeWithPrev carries the previous Bdi data point).
//	circuit breakers   — expected-empty is a NO-OP for failure accounting: it
//	                     must not accumulate failures, and it must not reset a
//	                     real failure streak either (see
//	                     marketdata/circuit_breaker.go recordSuccess note).
//	monitoring         — severity "warn" (classifyErrorSeverity).
var ErrEmptyQuote = errors.New("upstream returned an empty quote")
