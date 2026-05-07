package marketdata

import (
	"testing"
	"time"
)

func TestCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker(defaultCircuitBreakerConfig())
	if cb.State() != ProviderCircuitClosed {
		t.Errorf("initial state = %q, want %q", cb.State(), ProviderCircuitClosed)
	}
	if !cb.Allow() {
		t.Error("expected Allow()=true when closed")
	}
}

func TestCircuitBreaker_OpenAfterFailures(t *testing.T) {
	cb := NewCircuitBreaker(circuitBreakerConfig{
		failureThreshold: 2,
		recoveryTimeout:  5 * time.Minute,
		halfOpenMaxCalls: 1,
	})

	cb.RecordFailure()
	if cb.State() != ProviderCircuitClosed {
		t.Errorf("after 1 failure: state = %q, want %q", cb.State(), ProviderCircuitClosed)
	}

	cb.RecordFailure()
	if cb.State() != ProviderCircuitOpen {
		t.Errorf("after 2 failures: state = %q, want %q", cb.State(), ProviderCircuitOpen)
	}

	if cb.Allow() {
		t.Error("expected Allow()=false when open")
	}
}

func TestCircuitBreaker_RecoveryTimeout(t *testing.T) {
	cb := NewCircuitBreaker(circuitBreakerConfig{
		failureThreshold: 2,
		recoveryTimeout:  50 * time.Millisecond,
		halfOpenMaxCalls: 1,
	})

	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != ProviderCircuitOpen {
		t.Fatalf("expected state=open, got %q", cb.State())
	}

	time.Sleep(100 * time.Millisecond)

	if !cb.Allow() {
		t.Error("expected Allow()=true after recovery timeout")
	}

	if cb.State() != ProviderCircuitHalfOpen {
		t.Errorf("after recovery: state = %q, want %q", cb.State(), ProviderCircuitHalfOpen)
	}
}

func TestCircuitBreaker_HalfOpenLimit(t *testing.T) {
	cb := NewCircuitBreaker(circuitBreakerConfig{
		failureThreshold: 2,
		recoveryTimeout:  50 * time.Millisecond,
		halfOpenMaxCalls: 2,
	})

	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(100 * time.Millisecond)

	if !cb.Allow() {
		t.Error("expected Allow()=true for first half-open call")
	}
	if !cb.Allow() {
		t.Error("expected Allow()=true for second half-open call")
	}
	if cb.Allow() {
		t.Error("expected Allow()=false for third half-open call (exceeds limit)")
	}
}

func TestCircuitBreaker_HalfOpenToClosed(t *testing.T) {
	cb := NewCircuitBreaker(circuitBreakerConfig{
		failureThreshold: 2,
		recoveryTimeout:  50 * time.Millisecond,
		halfOpenMaxCalls: 2,
	})

	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(100 * time.Millisecond)

	cb.Allow()
	cb.Allow()
	cb.RecordSuccess()
	cb.RecordSuccess()

	if cb.State() != ProviderCircuitClosed {
		t.Errorf("after recovery: state = %q, want %q", cb.State(), ProviderCircuitClosed)
	}
}

func TestCircuitBreaker_HalfOpenToOpenOnFailure(t *testing.T) {
	cb := NewCircuitBreaker(circuitBreakerConfig{
		failureThreshold: 2,
		recoveryTimeout:  50 * time.Millisecond,
		halfOpenMaxCalls: 2,
	})

	cb.RecordFailure()
	cb.RecordFailure()
	time.Sleep(100 * time.Millisecond)

	cb.Allow()
	cb.RecordFailure()

	if cb.State() != ProviderCircuitOpen {
		t.Errorf("after failure in half-open: state = %q, want %q", cb.State(), ProviderCircuitOpen)
	}

	if cb.Allow() {
		t.Error("expected Allow()=false after re-opening")
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(circuitBreakerConfig{
		failureThreshold: 1,
		recoveryTimeout:  5 * time.Minute,
		halfOpenMaxCalls: 1,
	})

	cb.RecordFailure()
	if cb.State() != ProviderCircuitOpen {
		t.Fatal("expected open after failure")
	}

	cb.Reset()
	if cb.State() != ProviderCircuitClosed {
		t.Errorf("after reset: state = %q, want %q", cb.State(), ProviderCircuitClosed)
	}
	if !cb.Allow() {
		t.Error("expected Allow()=true after reset")
	}
}

func TestCircuitBreaker_ForceOpen(t *testing.T) {
	cb := NewCircuitBreaker(defaultCircuitBreakerConfig())
	cb.ForceOpen()

	if cb.State() != ProviderCircuitOpen {
		t.Errorf("after ForceOpen: state = %q, want %q", cb.State(), ProviderCircuitOpen)
	}
	if cb.Allow() {
		t.Error("expected Allow()=false after ForceOpen")
	}
}

func TestCircuitBreaker_Stats(t *testing.T) {
	cb := NewCircuitBreaker(circuitBreakerConfig{
		failureThreshold: 3,
		recoveryTimeout:  5 * time.Minute,
		halfOpenMaxCalls: 2,
	})

	stats := cb.Stats()
	if stats["state"] != string(ProviderCircuitClosed) {
		t.Errorf("stats.state = %q, want %q", stats["state"], ProviderCircuitClosed)
	}
	if stats["failure_count"] != 0 {
		t.Errorf("stats.failure_count = %d, want 0", stats["failure_count"])
	}
	if stats["failure_threshold"] != 3 {
		t.Errorf("stats.failure_threshold = %d, want 3", stats["failure_threshold"])
	}

	cb.RecordFailure()
	stats = cb.Stats()
	if stats["failure_count"] != 1 {
		t.Errorf("after failure: stats.failure_count = %d, want 1", stats["failure_count"])
	}
}

func TestCircuitBreaker_ClosedResetsFailureCount(t *testing.T) {
	cb := NewCircuitBreaker(circuitBreakerConfig{
		failureThreshold: 5,
		recoveryTimeout:  5 * time.Minute,
		halfOpenMaxCalls: 1,
	})

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.State() != ProviderCircuitClosed {
		t.Fatalf("expected state=closed, got %q", cb.State())
	}

	cb.RecordSuccess()

	stats := cb.Stats()
	if stats["failure_count"] != 0 {
		t.Errorf("after success in closed: failure_count = %d, want 0", stats["failure_count"])
	}
}

func TestCircuitBreaker_RecordSuccessInHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(circuitBreakerConfig{
		failureThreshold: 1,
		recoveryTimeout:  50 * time.Millisecond,
		halfOpenMaxCalls: 3,
	})

	cb.RecordFailure()
	time.Sleep(100 * time.Millisecond)

	cb.Allow()
	cb.Allow()
	cb.Allow()

	if cb.State() != ProviderCircuitHalfOpen {
		t.Fatalf("expected state=half-open, got %q", cb.State())
	}

	cb.RecordSuccess()
	if cb.State() != ProviderCircuitClosed {
		t.Errorf("after success reaching halfOpenMaxCalls: state = %q, want %q", cb.State(), ProviderCircuitClosed)
	}
}
