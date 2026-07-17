package apigateway

import (
	"context"
	"time"

	"golang.org/x/time/rate"
)

// waitForLimiter is the apigateway-internal variant of marketdata.WaitForLimiter.
// Decouples the rate-limiter wait from the caller's deadline so a short fetch
// context (e.g. 5s health probe) does not kill a politeness wait and surface
// as "rate limit wait: context canceled" (BK-10). The HTTP layer that follows
// still uses the caller's context for real cancellation.
func waitForLimiter(ctx context.Context, l *rate.Limiter) error {
	waitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	return l.Wait(waitCtx)
}
