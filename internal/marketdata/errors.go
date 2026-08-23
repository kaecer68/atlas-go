package marketdata

import "errors"

// ErrRateLimited is returned when an API rate limit is encountered.
var ErrRateLimited = errors.New("rate limited")

// ErrTWSEQuoteNotFound is returned by TWSEClient.GetQuote when the symbol
// is absent from the TWSE STOCK_DAY_ALL snapshot. stocktools.HandleQuote
// uses errors.Is to distinguish "symbol is out of scope for every provider"
// (200 + coverage_note policy signal) from a genuine upstream failure (503).
var ErrTWSEQuoteNotFound = errors.New("twse: quote symbol not found")

// ErrTWSEEmptyData is returned by TWSEClient.GetQuotes when TWSE responds
// OK but delivers no quote rows (stat=OK with empty data, or an empty CSV
// payload). P0-4: previously GetQuotes returned an empty success slice, so
// the gateway breaker never tripped and channel-health could not tell a
// holiday/no-data response apart from a real upstream failure. Adapters can
// errors.Is this sentinel to treat it as no-data (non-error) when the
// trading calendar explains it.
var ErrTWSEEmptyData = errors.New("twse: empty data response")
