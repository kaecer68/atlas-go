package marketdata

import (
	"context"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// TestWaitForLimiter_ParentCanceled verifies the helper ignores a canceled
// parent context (BK-10 fix: politeness wait must not consume caller's deadline).
func TestWaitForLimiter_ParentCanceled(t *testing.T) {
	l := rate.NewLimiter(rate.Every(5*time.Second), 1)
	if err := l.Wait(context.Background()); err != nil { // consume the burst token
		t.Fatalf("setup: %v", err)
	}
	parentCtx, cancel := context.WithCancel(context.Background())
	cancel() // caller is already dead

	done := make(chan error, 1)
	go func() {
		done <- WaitForLimiter(parentCtx, l)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected nil (token will be replenished), got %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("WaitForLimiter blocked longer than 15s cap")
	}
}

// TestWaitForLimiter_RespectsCap verifies the helper bounds the wait at 15s
// even when the limiter would otherwise block longer (30s here).
func TestWaitForLimiter_RespectsCap(t *testing.T) {
	l := rate.NewLimiter(rate.Every(30*time.Second), 1)
	if err := l.Wait(context.Background()); err != nil {
		t.Fatalf("setup: %v", err)
	}
	start := time.Now()
	err := WaitForLimiter(context.Background(), l)
	if err == nil {
		t.Fatal("expected an error when limiter interval exceeds the helper cap")
	}
	// rate package returns its own preflight error ("would exceed context
	// deadline") rather than context.DeadlineExceeded — either is acceptable;
	// what matters is that the helper does NOT let the call block 30s.
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("elapsed=%v, helper should have bailed well before the 30s limiter interval", elapsed)
	}
}
