package risk

import (
	"strings"
	"testing"
	"time"
)

func TestCircuitBreaker_DailyLossTrip(t *testing.T) {
	cb := NewCircuitBreaker(0.05, 3)

	tripped, reason := cb.Check(-0.06)
	if !tripped {
		t.Error("expected circuit breaker to trip on -6% daily loss")
	}
	if !strings.Contains(reason, "daily loss") {
		t.Errorf("expected daily-loss reason, got: %s", reason)
	}
	if !cb.IsTripped() {
		t.Error("expected IsTripped() true after trip")
	}
	if cb.TrippedAt().IsZero() {
		t.Error("expected TrippedAt to be set")
	}
	if cb.ConsecutiveLosses() != 1 {
		t.Errorf("expected consecutive losses to reflect the loss day, got %d", cb.ConsecutiveLosses())
	}
}

func TestCircuitBreaker_ConsecutiveLossesTrip(t *testing.T) {
	cb := NewCircuitBreaker(0.10, 3)

	if tripped, _ := cb.Check(-0.01); tripped {
		t.Fatal("unexpected trip on first loss")
	}
	if tripped, _ := cb.Check(-0.005); tripped {
		t.Fatal("unexpected trip on second loss")
	}

	tripped, reason := cb.Check(-0.02)
	if !tripped {
		t.Error("expected circuit breaker to trip after 3 consecutive losses")
	}
	if !strings.Contains(reason, "consecutive losing days") {
		t.Errorf("expected consecutive-loss reason, got: %s", reason)
	}
	if !cb.IsTripped() {
		t.Error("expected breaker to remain tripped")
	}
	if cb.ConsecutiveLosses() != 0 {
		t.Errorf("expected consecutive losses reset after trip, got %d", cb.ConsecutiveLosses())
	}
}

func TestCircuitBreaker_AutoReset(t *testing.T) {
	cb := NewCircuitBreaker(0.05, 3).WithAutoResetAfter(10 * time.Millisecond)

	cb.Check(-0.10)
	if !cb.IsTripped() {
		t.Fatal("expected breaker to be tripped")
	}

	time.Sleep(20 * time.Millisecond)

	tripped, _ := cb.Check(-0.01)
	if tripped {
		t.Error("expected breaker to auto-reset and evaluate the new day")
	}
	if cb.IsTripped() {
		t.Error("expected breaker to be reset after auto-reset duration")
	}
	if cb.ConsecutiveLosses() != 1 {
		t.Errorf("expected consecutive losses to count the post-reset loss, got %d", cb.ConsecutiveLosses())
	}
}

func TestCircuitBreaker_ResetManual(t *testing.T) {
	cb := NewCircuitBreaker(0.05, 3)
	cb.Check(-0.06)
	if !cb.IsTripped() {
		t.Fatal("expected breaker to be tripped")
	}

	cb.Reset()

	if cb.IsTripped() {
		t.Error("expected IsTripped() false after Reset()")
	}
	if cb.ConsecutiveLosses() != 0 {
		t.Errorf("expected ConsecutiveLosses() 0 after Reset(), got %d", cb.ConsecutiveLosses())
	}
	if !cb.TrippedAt().IsZero() {
		t.Error("expected TrippedAt zeroed after Reset()")
	}
}

func TestCircuitBreaker_WinResetsConsecutiveLosses(t *testing.T) {
	cb := NewCircuitBreaker(0.10, 3)
	cb.Check(-0.01)
	cb.Check(-0.02)
	if cb.ConsecutiveLosses() != 2 {
		t.Fatalf("expected 2 consecutive losses, got %d", cb.ConsecutiveLosses())
	}

	cb.Check(0.005)
	if cb.ConsecutiveLosses() != 0 {
		t.Errorf("expected consecutive losses reset after winning day, got %d", cb.ConsecutiveLosses())
	}

	cb.Check(-0.01)
	if cb.ConsecutiveLosses() != 1 {
		t.Errorf("expected consecutive losses to restart at 1, got %d", cb.ConsecutiveLosses())
	}
}
