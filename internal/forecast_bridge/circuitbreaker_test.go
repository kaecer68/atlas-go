package forecast_bridge

import (
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedByDefault(t *testing.T) {
	cb := DefaultCircuitBreaker()
	if cb.IsOpen() {
		t.Fatal("expected breaker to be closed by default")
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := DefaultCircuitBreaker()
	for i := 0; i < 10; i++ {
		cb.RecordFailure()
	}
	if !cb.IsOpen() {
		t.Fatal("expected breaker to open after 10 failures")
	}
}

func TestCircuitBreaker_AutoResetAfterCooldown(t *testing.T) {
	cb := NewCircuitBreaker(2, 100*time.Millisecond)
	cb.RecordFailure()
	cb.RecordFailure()
	if !cb.IsOpen() {
		t.Fatal("expected breaker to open after 2 failures")
	}
	time.Sleep(150 * time.Millisecond)
	if cb.IsOpen() {
		t.Fatal("expected breaker to reset after cooldown")
	}
}

func TestCircuitBreaker_SuccessResets(t *testing.T) {
	cb := NewCircuitBreaker(3, 1*time.Hour)
	for i := 0; i < 2; i++ {
		cb.RecordFailure()
	}
	cb.RecordSuccess()
	cb.RecordFailure()
	cb.RecordFailure()
	if cb.IsOpen() {
		t.Fatal("expected breaker still closed after 2 failures (success reset counter)")
	}
	cb.RecordFailure()
	if !cb.IsOpen() {
		t.Fatal("expected breaker to open after 3rd failure post-reset")
	}
}
