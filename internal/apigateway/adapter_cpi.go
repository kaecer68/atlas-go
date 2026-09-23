package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// cpiLimiterMaxWait bounds how long Fetch may block waiting for the channel's
// 1-request/hour token.
//
// Why a bound exists: the runtime macro fan-out
// (monitoring.macroDataGatewayAdapter.fetchFresh) has no per-channel timeout —
// it launches one goroutine per channel and blocks on wg.Wait() until the
// CALLER's context deadline (30s for the 5-minute macro_ingest task). An
// unbounded limiter.Wait(ctx) on a 1/hour limiter would therefore park the
// whole batch on this single channel. Real upstream calls are paced by the
// gateway cache TTL for us_cpi instead (24h, see NewGateway); the limiter only
// has to absorb bursts (manual refresh, a second consumer, or a failed fetch
// that already consumed the token).
const cpiLimiterMaxWait = 2 * time.Second

// cpiSnapshotProvider is the minimal provider surface the adapter needs. It is
// an interface rather than *marketdata.BLSCPIProvider so tests can inject a
// stub without touching the provider's unexported endpoint/client fields.
type cpiSnapshotProvider interface {
	FetchSnapshot(ctx context.Context) (marketdata.MacroDataSnapshot, error)
}

// USCPIChannelAdapter adapts the BLS CPI provider to the DataProvider
// interface (channel id "us_cpi"). CPI-U is published monthly, so the limiter
// is deliberately slow (one request per hour) — the channel exists to seed
// MacroDataSnapshot.CPIYoY for the narrative inflation detectors, not for
// per-tick freshness.
type USCPIChannelAdapter struct {
	provider cpiSnapshotProvider
	limiter  *rate.Limiter

	mu   sync.Mutex
	last *FetchResult // last successful payload (last-known-good)
}

func NewUSCPIChannelAdapter(p *marketdata.BLSCPIProvider) *USCPIChannelAdapter {
	return &USCPIChannelAdapter{
		provider: p,
		limiter:  rate.NewLimiter(rate.Every(time.Hour), 1),
	}
}

func (a *USCPIChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	delay, err := a.acquire(ctx)
	if err != nil {
		return nil, err
	}
	if delay > 0 {
		// Throttled: the next token is further away than cpiLimiterMaxWait.
		// Serve the last-known-good payload rather than stalling the fan-out.
		// CPI-U is monthly data, so a payload fetched hours ago is still the
		// current print; it is marked Stale/Fallback so downstream reports it
		// as cached data instead of a fresh upstream fetch.
		if last := a.lastKnownGood(); last != nil {
			return last, nil
		}
		// No payload to fall back on (e.g. the previous fetch consumed the
		// token and then failed). ErrNoData keeps this non-alarming at the
		// gateway: it records "waiting" instead of a channel error.
		return nil, fmt.Errorf("us_cpi throttled by rate limiter (%s until next token): %w",
			delay.Round(time.Millisecond), marketdata.ErrNoData)
	}
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("us_cpi marshal: %w", err)
	}
	result := &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID:          "us_cpi",
		LatencyMs:          time.Since(start).Milliseconds(),
		RateLimitRemaining: int(a.limiter.Tokens()),
		Timestamp:          time.Now(),
	}}
	a.remember(result)
	return result, nil
}

// acquire reserves a limiter token without ever blocking for a full refill
// window. It returns (0, nil) when a token was taken (the caller may fetch),
// (delay, nil) when the token is further away than cpiLimiterMaxWait (the
// reservation was released and the caller must not fetch), or an error when
// ctx was already done or expired while waiting for a short delay.
func (a *USCPIChannelAdapter) acquire(ctx context.Context) (time.Duration, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	r := a.limiter.Reserve()
	delay := r.Delay()
	switch {
	case delay <= 0:
		return 0, nil
	case delay > cpiLimiterMaxWait:
		// Cancel releases the reserved token so this probe does not consume
		// the channel's hourly budget.
		r.Cancel()
		return delay, nil
	default:
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
			return 0, nil
		case <-ctx.Done():
			r.Cancel()
			return 0, ctx.Err()
		}
	}
}

// remember stores a successful payload as last-known-good. The stored copy is
// detached from the caller's slice so later mutations (the gateway marks stale
// results in place) cannot corrupt the memo.
func (a *USCPIChannelAdapter) remember(result *FetchResult) {
	cp := *result
	cp.Data = append([]byte(nil), result.Data...)
	cp.Meta.Stale = false
	cp.Meta.Fallback = false
	cp.Meta.LastError = ""
	a.mu.Lock()
	a.last = &cp
	a.mu.Unlock()
}

// lastKnownGood returns a detached copy of the last successful payload marked
// as cached data, or nil when no successful fetch has happened yet.
func (a *USCPIChannelAdapter) lastKnownGood() *FetchResult {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.last == nil {
		return nil
	}
	cp := *a.last
	cp.Data = append([]byte(nil), a.last.Data...)
	cp.Meta.Cached = true
	cp.Meta.Stale = true
	cp.Meta.Fallback = true
	cp.Meta.LastError = "us_cpi: serving last-known-good snapshot (rate limiter throttled)"
	cp.Meta.RateLimitRemaining = int(a.limiter.Tokens())
	return &cp
}

func (a *USCPIChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *USCPIChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{ChannelID: "us_cpi", Country: "美國", Platform: "BLS", APIFormat: "REST JSON", Path: "api.bls.gov", HasLimiter: true}
}

// HealthCheck reports channel health WITHOUT bypassing the adapter's
// 1-request/hour limiter. It goes through the same acquire() boundary as
// Fetch, so a probe can never steal the channel's hourly token from a real
// fetch — the pre-#1918 behavior pinged BLS directly on every health scan,
// consuming the channel's only token and throttling the next real Fetch.
//
// Status semantics (why not error/warn on throttle):
//   - Token acquired → real liveness ping; failure is "error" + err,
//     unchanged from the pre-existing behavior.
//   - Throttled + last-known-good → "ok": CPI-U is monthly data, so the
//     cached payload is still the current print. The note records that the
//     probe was served from cache instead of a live ping.
//   - Throttled + no last-known-good → "ok" with a "waiting for rate-limit
//     token" note, deliberately NOT "error" and NOT "warn":
//   - Not "error": nothing is known to be broken upstream — the channel
//     simply has not produced data yet (or an earlier attempt consumed
//     the token and then failed). An "error" attempt would increment
//     ConsecutiveFailures and, after GraceFailures, page on a transient
//     cold-start state. The gateway fetch path treats the identical
//     condition (ErrNoData) as RecordWaiting, i.e. status stays "ok".
//   - Not "warn": every non-ok status surfaces in Alerts() and would pin
//     a badge for a condition expected to last up to an hour after each
//     cold start; it also leaves the failure streak alive instead of
//     resetting it like "ok" does.
//
// The throttled branch never consumes a token: acquire() cancels the
// reservation, so the probe is free and the next real fetch keeps its budget.
func (a *USCPIChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	delay, err := a.acquire(ctx)
	if err != nil {
		return HealthStatus{
			Status:    "error",
			LastError: err.Error(),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "liveness",
		}, err
	}
	if delay > 0 {
		if a.lastKnownGood() != nil {
			return HealthStatus{
				Status:    "ok",
				LastError: "us_cpi: rate limiter throttled health probe; serving last-known-good snapshot (cached data still current)",
				UpdatedAt: time.Now().Format(time.RFC3339),
				CheckType: "liveness",
			}, nil
		}
		return HealthStatus{
			Status:    "ok",
			LastError: fmt.Sprintf("us_cpi: rate limiter throttled health probe; no last-known-good yet (next token in %s)", delay.Round(time.Millisecond)),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "liveness",
		}, nil
	}
	if _, err := a.provider.FetchSnapshot(ctx); err != nil {
		return HealthStatus{
			Status:    "error",
			LastError: err.Error(),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "liveness",
		}, err
	}
	return HealthStatus{
		Status:    "ok",
		UpdatedAt: time.Now().Format(time.RFC3339),
		CheckType: "liveness",
	}, nil
}
