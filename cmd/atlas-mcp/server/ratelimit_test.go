package server

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestRateLimiter_DisabledWhenZeroCapacity(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{PerMinute: 0, Burst: 0})
	for i := range 1000 {
		if r := rl.Allow("tool", "caller"); !r.Allowed {
			t.Fatalf("expected allowed when disabled, got deny at i=%d", i)
		}
	}
}

func TestRateLimiter_BurstDefaultsToPerMinute(t *testing.T) {
	now := time.Now()
	rl := &RateLimiter{
		buckets:    make(map[string]*bucket),
		capacity:   60, // burst defaults to PerMinute=60
		refillPerS: 1,
		now:        func() time.Time { return now },
	}
	for i := range 60 {
		if r := rl.Allow("tool", "caller"); !r.Allowed {
			t.Fatalf("burst %d: expected allowed, got deny", i)
		}
	}
	if r := rl.Allow("tool", "caller"); r.Allowed {
		t.Fatal("61st request should be denied (burst exhausted)")
	}
}

func TestRateLimiter_BurstThenDeny(t *testing.T) {
	now := time.Now()
	rl := &RateLimiter{
		buckets:    make(map[string]*bucket),
		capacity:   5,
		refillPerS: 1,
		now:        func() time.Time { return now },
	}
	for i := range 5 {
		if r := rl.Allow("tool", "caller"); !r.Allowed {
			t.Fatalf("burst %d: expected allowed, got deny", i)
		}
	}
	r := rl.Allow("tool", "caller")
	if r.Allowed {
		t.Fatalf("6th request should be denied, got %+v", r)
	}
	if r.RetryAfter <= 0 {
		t.Fatalf("expected positive RetryAfter, got %s", r.RetryAfter)
	}
}

func TestRateLimiter_RefillOverTime(t *testing.T) {
	now := time.Now()
	rl := &RateLimiter{
		buckets:    make(map[string]*bucket),
		capacity:   5,
		refillPerS: 1,
		now:        func() time.Time { return now },
	}
	for range 5 {
		rl.Allow("tool", "caller")
	}
	if r := rl.Allow("tool", "caller"); r.Allowed {
		t.Fatal("expected deny after burst exhausted")
	}
	now = now.Add(2 * time.Second)
	if r := rl.Allow("tool", "caller"); !r.Allowed {
		t.Fatalf("after 2s, expected allow, got %+v", r)
	}
}

func TestRateLimiter_PerToolIsolation(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{PerMinute: 60, Burst: 1})
	if r := rl.Allow("toolA", "caller"); !r.Allowed {
		t.Fatal("toolA first should allow")
	}
	if r := rl.Allow("toolA", "caller"); r.Allowed {
		t.Fatal("toolA second should deny")
	}
	if r := rl.Allow("toolB", "caller"); !r.Allowed {
		t.Fatal("toolB first should allow (separate bucket)")
	}
}

func TestRateLimiter_PerCallerIsolation(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{PerMinute: 60, Burst: 1})
	if r := rl.Allow("tool", "A"); !r.Allowed {
		t.Fatal("A first should allow")
	}
	if r := rl.Allow("tool", "A"); r.Allowed {
		t.Fatal("A second should deny")
	}
	if r := rl.Allow("tool", "B"); !r.Allowed {
		t.Fatal("B first should allow (separate bucket)")
	}
}

func TestRateLimiter_CapAtCapacity(t *testing.T) {
	now := time.Now()
	rl := &RateLimiter{
		buckets:    make(map[string]*bucket),
		capacity:   5,
		refillPerS: 1000, // very fast refill
		now:        func() time.Time { return now },
	}
	now = now.Add(100 * time.Second) // would yield 100000 tokens without cap
	allowed := 0
	for range 20 {
		if r := rl.Allow("tool", "caller"); r.Allowed {
			allowed++
		}
	}
	if allowed != 5 {
		t.Fatalf("expected exactly 5 allowed (capped), got %d", allowed)
	}
}

func TestRateLimiter_SweepIdleEvicts(t *testing.T) {
	now := time.Now()
	rl := &RateLimiter{
		buckets:    make(map[string]*bucket),
		capacity:   5,
		refillPerS: 1,
		idleEvict:  10 * time.Minute,
		now:        func() time.Time { return now },
	}
	rl.Allow("tool", "A")
	rl.Allow("tool", "B")
	if got := rl.Size(); got != 2 {
		t.Fatalf("expected 2 buckets, got %d", got)
	}
	now = now.Add(11 * time.Minute)
	n := rl.SweepIdle()
	if n != 2 {
		t.Fatalf("expected 2 evicted, got %d", n)
	}
	if got := rl.Size(); got != 0 {
		t.Fatalf("expected 0 buckets after sweep, got %d", got)
	}
}

func TestRateLimiter_SweepKeepsActiveBuckets(t *testing.T) {
	now := time.Now()
	rl := &RateLimiter{
		buckets:    make(map[string]*bucket),
		capacity:   5,
		refillPerS: 1,
		idleEvict:  10 * time.Minute,
		now:        func() time.Time { return now },
	}
	rl.Allow("tool", "A")
	now = now.Add(5 * time.Minute)
	rl.Allow("tool", "A")
	now = now.Add(5 * time.Minute)
	n := rl.SweepIdle()
	if n != 0 {
		t.Fatalf("expected 0 evicted (A is active), got %d", n)
	}
	if got := rl.Size(); got != 1 {
		t.Fatalf("expected 1 bucket, got %d", got)
	}
}

func TestRateLimiter_ConcurrentAllow(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{PerMinute: 600, Burst: 10})
	var wg sync.WaitGroup
	var allowed int64
	var mu sync.Mutex
	for range 100 {
		wg.Go(func() {
			if r := rl.Allow("tool", "caller"); r.Allowed {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if allowed > 10 {
		t.Fatalf("expected at most 10 allowed (burst), got %d", allowed)
	}
	if allowed < 1 {
		t.Fatalf("expected at least 1 allowed, got %d", allowed)
	}
}

func TestRateLimiter_RunStopsOnContextCancel(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{PerMinute: 60, Burst: 5, IdleSweep: 50 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	rl.Run(ctx)
	cancel()
	time.Sleep(100 * time.Millisecond)
	if r := rl.Allow("tool", "caller"); !r.Allowed {
		t.Fatal("Allow should still work after Run cancelled")
	}
}
