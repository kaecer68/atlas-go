package server

import (
	"context"
	"sync"
	"time"
)

// RateLimiter enforces per-tool rate limits per tenant (or anonymous caller).
//
// Key format: tenant_id:tool. An empty tenant_id is mapped to "anonymous"
// for backward compatibility with single-tenant deployments.
type RateLimiter struct {
	mu         sync.Mutex
	buckets    map[string]*bucket
	capacity   float64
	refillPerS float64
	idleEvict  time.Duration
	idleSweep  time.Duration
	now        func() time.Time
}

type bucket struct {
	tokens     float64
	lastRefill time.Time
	lastUsed   time.Time
}

// RateLimiterConfig configures a RateLimiter.
type RateLimiterConfig struct {
	PerMinute int           // per-(tenant, tool) requests per minute; 0 = disabled
	Burst     int           // burst capacity; 0 = defaults to PerMinute
	IdleEvict time.Duration // evict buckets idle this long (default 1h)
	IdleSweep time.Duration // sweep interval (default 10m)
}

// NewRateLimiter builds a RateLimiter. With PerMinute == 0 the limiter is a
// no-op: Allow always returns Allowed=true. With Burst == 0 the burst defaults
// to PerMinute (so a token-bucket equivalent of "constant rate" is implicit).
func NewRateLimiter(cfg RateLimiterConfig) *RateLimiter {
	burst := cfg.Burst
	if burst == 0 {
		burst = cfg.PerMinute
	}
	capacity := float64(burst)
	refillPerS := float64(cfg.PerMinute) / 60.0
	idleEvict := cfg.IdleEvict
	if idleEvict == 0 {
		idleEvict = time.Hour
	}
	idleSweep := cfg.IdleSweep
	if idleSweep == 0 {
		idleSweep = 10 * time.Minute
	}
	return &RateLimiter{
		buckets:    make(map[string]*bucket),
		capacity:   capacity,
		refillPerS: refillPerS,
		idleEvict:  idleEvict,
		idleSweep:  idleSweep,
		now:        time.Now,
	}
}

// Result describes an Allow decision.
type Result struct {
	Allowed    bool
	RetryAfter time.Duration
	Remaining  float64
}

// Allow checks if a request for tool from caller is permitted. On allow, the
// bucket is decremented. On deny, RetryAfter gives the time until the next
// token is available. The caller argument is opaque (any string), so callers
// can use token, IP, or session-id.
func (r *RateLimiter) Allow(tool, tenantID string) Result {
	if r.capacity == 0 || r.refillPerS == 0 {
		return Result{Allowed: true, Remaining: r.capacity}
	}
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()

	// Per-tenant key: tenant_id:tool. Empty tenant maps to "anonymous".
	cid := tenantID
	if cid == "" {
		cid = "anonymous"
	}
	key := cid + ":" + tool
	b, ok := r.buckets[key]
	if !ok {
		b = &bucket{tokens: r.capacity, lastRefill: now, lastUsed: now}
		r.buckets[key] = b
	}
	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * r.refillPerS
		if b.tokens > r.capacity {
			b.tokens = r.capacity
		}
		b.lastRefill = now
	}
	b.lastUsed = now
	if b.tokens >= 1 {
		b.tokens--
		return Result{Allowed: true, Remaining: b.tokens}
	}
	missing := 1 - b.tokens
	retry := time.Duration(missing / r.refillPerS * float64(time.Second))
	return Result{Allowed: false, RetryAfter: retry, Remaining: b.tokens}
}

// SweepIdle removes buckets that have not been used within idleEvict. Returns
// the number evicted. Safe to call concurrently with Allow.
func (r *RateLimiter) SweepIdle() int {
	if r.capacity == 0 || r.refillPerS == 0 {
		return 0
	}
	now := r.now()
	cutoff := now.Add(-r.idleEvict)
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for k, b := range r.buckets {
		if b.lastUsed.Before(cutoff) {
			delete(r.buckets, k)
			n++
		}
	}
	return n
}

// Run starts a background sweeper goroutine. The goroutine runs until ctx
// is cancelled. No-op when the limiter is disabled.
func (r *RateLimiter) Run(ctx context.Context) {
	if r.capacity == 0 || r.refillPerS == 0 {
		return
	}
	go func() {
		t := time.NewTicker(r.idleSweep)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.SweepIdle()
			}
		}
	}()
}

// Size returns the number of tracked buckets (for tests / metrics).
func (r *RateLimiter) Size() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.buckets)
}
