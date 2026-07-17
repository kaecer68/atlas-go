package marketdata

import (
	"context"
	"time"

	"golang.org/x/time/rate"
)

// WaitForLimiter waits for the rate limiter with a budget decoupled from
// the caller's deadline: the politeness wait must not consume the fetch
// deadline (BK-10: "rate limit wait: context canceled"). Bounded at 15s;
// the actual HTTP call still runs under the caller's context.
//
// During process shutdown, the real ctx will be canceled and the helper
// returns within the 15s cap (it does not propagate the parent cancel,
// only the timeout). The HTTP layer that follows still respects shutdown.
func WaitForLimiter(ctx context.Context, l *rate.Limiter) error {
	waitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	return l.Wait(waitCtx)
}
