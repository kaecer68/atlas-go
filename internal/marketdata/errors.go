package marketdata

import "errors"

// ErrRateLimited is returned when an API rate limit is encountered.
var ErrRateLimited = errors.New("rate limited")

// ErrTWSEQuoteNotFound is returned by TWSEClient.GetQuote when the symbol
// is absent from the TWSE STOCK_DAY_ALL snapshot. stocktools.HandleQuote
// uses errors.Is to distinguish "symbol is out of scope for every provider"
// (200 + coverage_note policy signal) from a genuine upstream failure (503).
var ErrTWSEQuoteNotFound = errors.New("twse: quote symbol not found")
