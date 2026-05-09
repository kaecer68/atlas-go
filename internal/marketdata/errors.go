package marketdata

import "errors"

// ErrRateLimited is returned when an API rate limit is encountered.
var ErrRateLimited = errors.New("rate limited")
