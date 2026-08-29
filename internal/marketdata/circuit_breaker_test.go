package marketdata

import (
	"testing"
	"time"
)

func fastTestConfig() circuitBreakerConfig {
	return circuitBreakerConfig{
		failureThreshold: 3,
		recoveryTimeout:  100 * time.Millisecond,
		halfOpenMaxCalls: 2,
	}
}

func TestProviderBreaker_ClosedByDefault(t *testing.T) {
	b := newProviderBreaker("test", fastTestConfig())
	snap := b.stateSnapshot()
	if snap.State != ProviderCircuitClosed {
		t.Fatalf("expected Closed, got %s", snap.State)
	}
	if snap.Name != "test" {
		t.Fatalf("expected name 'test', got %s", snap.Name)
	}
}

func TestProviderBreaker_OpensAfterThresholdFailures(t *testing.T) {
	b := newProviderBreaker("fugle", fastTestConfig())

	// 3 consecutive failures should trip to Open
	for range 3 {
		b.recordFailure()
	}

	snap := b.stateSnapshot()
	if snap.State != ProviderCircuitOpen {
		t.Fatalf("expected Open after 3 failures, got %s", snap.State)
	}
	if snap.FailureCount != 3 {
		t.Fatalf("expected FailureCount=3, got %d", snap.FailureCount)
	}

	// shouldTry should return false when Open and no recovery timeout reached
	if b.shouldTry() {
		t.Fatal("expected shouldTry()=false when Open without recovery timeout")
	}
}

func TestProviderBreaker_StaysOpenBeforeRecoveryTimeout(t *testing.T) {
	b := newProviderBreaker("fugle", fastTestConfig())
	for range 3 {
		b.recordFailure()
	}
	// immediately after opening, before any timeout
	if b.shouldTry() {
		t.Fatal("expected shouldTry()=false when Open and before recovery timeout")
	}
}

func TestProviderBreaker_HalfOpenAfterRecoveryTimeout(t *testing.T) {
	b := newProviderBreaker("fugle", fastTestConfig())
	for range 3 {
		b.recordFailure()
	}

	// sleep past recovery timeout
	time.Sleep(150 * time.Millisecond)

	// shouldTry transitions to HalfOpen
	if !b.shouldTry() {
		t.Fatal("expected shouldTry()=true after recovery timeout (HalfOpen)")
	}

	snap := b.stateSnapshot()
	if snap.State != ProviderCircuitHalfOpen {
		t.Fatalf("expected HalfOpen after recovery, got %s", snap.State)
	}
}

func TestProviderBreaker_HalfOpenSuccessCloses(t *testing.T) {
	b := newProviderBreaker("fugle", fastTestConfig())
	for range 3 {
		b.recordFailure()
	}
	time.Sleep(150 * time.Millisecond)
	b.shouldTry() // transition to HalfOpen

	b.recordSuccess()

	snap := b.stateSnapshot()
	if snap.State != ProviderCircuitClosed {
		t.Fatalf("expected Closed after success in HalfOpen, got %s", snap.State)
	}
	if snap.FailureCount != 0 {
		t.Fatalf("expected FailureCount=0 after reset, got %d", snap.FailureCount)
	}
}

func TestProviderBreaker_HalfOpenFailureReopens(t *testing.T) {
	b := newProviderBreaker("fugle", fastTestConfig())
	for range 3 {
		b.recordFailure()
	}
	time.Sleep(150 * time.Millisecond)
	b.shouldTry() // transition to HalfOpen

	b.recordFailure()

	snap := b.stateSnapshot()
	if snap.State != ProviderCircuitOpen {
		t.Fatalf("expected Open after failure in HalfOpen, got %s", snap.State)
	}
}
