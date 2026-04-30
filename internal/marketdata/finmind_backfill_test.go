package marketdata

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiter_WaitAndRecord(t *testing.T) {
	limiter := newRateLimiter(600) // 600 req/hr
	ctx := context.Background()

	// Should not block on first call
	err := limiter.Wait(ctx)
	if err != nil {
		t.Fatalf("first wait failed: %v", err)
	}

	remaining := limiter.Remaining()
	if remaining >= 600 {
		t.Fatalf("remaining should decrease after wait, got %d", remaining)
	}
}

func TestRateLimiter_DecrementsOnUse(t *testing.T) {
	limiter := newRateLimiter(600)
	limiter.RecordUse()
	if limiter.Remaining() != 599 {
		t.Fatalf("expected 599 remaining, got %d", limiter.Remaining())
	}
}

func TestRateLimiter_429Handling(t *testing.T) {
	limiter := newRateLimiter(600)
	// Simulate hitting 429 - should compute correct wait time
	resetAt := time.Now().Add(30 * time.Second)
	waitDuration := limiter.WaitForReset(resetAt)
	if waitDuration < 29*time.Second || waitDuration > 31*time.Second {
		t.Fatalf("expected ~30s wait, got %v", waitDuration)
	}
}
