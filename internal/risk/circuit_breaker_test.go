package risk

import (
	"strings"
	"testing"
	"time"
)

func TestNewCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker(0.05, 3)
	if cb == nil {
		t.Fatal("NewCircuitBreaker returned nil")
	}
	if cb.IsTripped() {
		t.Error("new breaker should not be tripped")
	}
	if cb.ConsecutiveLosses() != 0 {
		t.Error("new breaker should have 0 consecutive losses")
	}
	if !cb.TrippedAt().IsZero() {
		t.Error("new breaker TrippedAt should be zero time")
	}
}

func TestCircuitBreaker_Check_SingleDayLossTrip(t *testing.T) {
	cb := NewCircuitBreaker(0.05, 3)

	tripped, reason := cb.Check(-0.06) // -6% exceeds 5% threshold
	if !tripped {
		t.Error("expected breaker to trip on -6% daily loss")
	}
	if reason == "" {
		t.Error("expected reason string when tripped")
	}
	if !strings.Contains(reason, "6.00%") {
		t.Errorf("reason should mention 6.00%%, got %q", reason)
	}
}

func TestCircuitBreaker_Check_SingleDayLossBelowThreshold(t *testing.T) {
	cb := NewCircuitBreaker(0.05, 3)

	tripped, _ := cb.Check(-0.04) // -4% does not exceed 5%
	if tripped {
		t.Error("breaker should not trip on -4% when threshold is 5%")
	}
}

func TestCircuitBreaker_Check_SingleDayLossAtThreshold(t *testing.T) {
	cb := NewCircuitBreaker(0.05, 3)

	tripped, _ := cb.Check(-0.05) // exactly -5%
	if !tripped {
		t.Error("breaker should trip at exact threshold")
	}
}

func TestCircuitBreaker_Check_ConsecutiveLossesTrip(t *testing.T) {
	cb := NewCircuitBreaker(0.05, 3)

	// 3 consecutive losing days, each below threshold
	cb.Check(-0.01)
	cb.Check(-0.02)
	tripped, reason := cb.Check(-0.01)

	if !tripped {
		t.Error("expected breaker to trip after 3 consecutive losses")
	}
	if !strings.Contains(reason, "consecutive") {
		t.Errorf("reason should mention consecutive, got %q", reason)
	}
}

func TestCircuitBreaker_Check_ConsecutiveLossesReset(t *testing.T) {
	cb := NewCircuitBreaker(0.05, 3)

	// 2 losses then a gain — should reset counter
	cb.Check(-0.01)
	cb.Check(-0.02)
	cb.Check(0.01) // gain day resets

	// Only 2 consecutive after reset
	cb.Check(-0.01)
	cb.Check(-0.02)

	// After gain reset, count should be 2 (not 3), not tripped
	if cb.ConsecutiveLosses() != 2 {
		t.Errorf("expected 2 consecutive losses after reset, got %d", cb.ConsecutiveLosses())
	}
	if cb.IsTripped() {
		t.Error("breaker should not be tripped after only 2 consecutive losses post-reset")
	}
}

func TestCircuitBreaker_Check_GainDay(t *testing.T) {
	cb := NewCircuitBreaker(0.05, 3)

	tripped, _ := cb.Check(0.03) // 3% gain
	if tripped {
		t.Error("breaker should not trip on gain day")
	}
	if cb.ConsecutiveLosses() != 0 {
		t.Error("gain day should not increase consecutive losses")
	}
}

func TestCircuitBreaker_Check_ZeroDay(t *testing.T) {
	cb := NewCircuitBreaker(0.05, 3)

	tripped, _ := cb.Check(0.0)
	if tripped {
		t.Error("breaker should not trip on zero return")
	}
	if cb.ConsecutiveLosses() != 0 {
		t.Error("zero return should not count as loss")
	}
}

func TestCircuitBreaker_AlreadyTripped(t *testing.T) {
	cb := NewCircuitBreaker(0.05, 3)

	cb.Check(-0.06) // trip it
	if !cb.IsTripped() {
		t.Fatal("precondition: breaker should be tripped")
	}

	// Subsequent checks should return tripped without further evaluation
	tripped, reason := cb.Check(0.10)
	if !tripped {
		t.Error("already-tripped breaker should return tripped")
	}
	if !strings.Contains(reason, "remains tripped") {
		t.Errorf("reason should mention 'remains tripped', got %q", reason)
	}
}

func TestCircuitBreaker_Check_AutoReset(t *testing.T) {
	cb := NewCircuitBreaker(0.05, 3)
	cb.WithAutoResetAfter(50 * time.Millisecond)

	cb.Check(-0.06) // trip it
	if !cb.IsTripped() {
		t.Fatal("precondition: breaker should be tripped")
	}

	time.Sleep(100 * time.Millisecond) // wait for auto-reset

	// Check should auto-recover and evaluate normally
	tripped, _ := cb.Check(0.01)
	if tripped {
		t.Error("after auto-reset, gain day should not trip")
	}
	if cb.IsTripped() {
		t.Error("breaker should be reset after auto-reset elapsed")
	}
}

func TestCircuitBreaker_Check_AutoResetNotYet(t *testing.T) {
	cb := NewCircuitBreaker(0.05, 3)
	cb.WithAutoResetAfter(1 * time.Hour) // far in future

	cb.Check(-0.06) // trip it
	if !cb.IsTripped() {
		t.Fatal("precondition: breaker should be tripped")
	}

	tripped, _ := cb.Check(0.10)
	if !tripped {
		t.Error("breaker should still be tripped when auto-reset hasn't elapsed")
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(0.05, 3)

	cb.Check(-0.06) // trip it
	if !cb.IsTripped() {
		t.Fatal("precondition: breaker should be tripped")
	}

	cb.Reset()

	if cb.IsTripped() {
		t.Error("breaker should not be tripped after Reset")
	}
	if cb.ConsecutiveLosses() != 0 {
		t.Error("consecutive losses should be 0 after Reset")
	}
	if !cb.TrippedAt().IsZero() {
		t.Error("TrippedAt should be zero after Reset")
	}
}

func TestCircuitBreaker_WithAutoResetAfter(t *testing.T) {
	cb := NewCircuitBreaker(0.05, 3)
	d := 30 * time.Minute
	result := cb.WithAutoResetAfter(d)

	if result != cb {
		t.Error("WithAutoResetAfter should return self for chaining")
	}

	// Verify it works by tripping then checking reset
	cb.WithAutoResetAfter(1 * time.Millisecond)
	cb.Check(-0.06)
	time.Sleep(10 * time.Millisecond)
	cb.Check(0.01)
	if cb.IsTripped() {
		t.Error("auto-reset should have cleared after short duration")
	}
}

func TestCircuitBreaker_IsTripped(t *testing.T) {
	cb := NewCircuitBreaker(0.05, 3)

	if cb.IsTripped() {
		t.Error("new breaker should not be tripped")
	}

	cb.Check(-0.06)
	if !cb.IsTripped() {
		t.Error("breaker should be tripped after large loss")
	}

	cb.Reset()
	if cb.IsTripped() {
		t.Error("breaker should not be tripped after Reset")
	}
}

func TestCircuitBreaker_TrippedAt(t *testing.T) {
	cb := NewCircuitBreaker(0.05, 3)

	if !cb.TrippedAt().IsZero() {
		t.Error("TrippedAt should be zero before tripping")
	}

	before := time.Now()
	cb.Check(-0.06)
	after := time.Now()

	trippedAt := cb.TrippedAt()
	if trippedAt.Before(before) || trippedAt.After(after) {
		t.Errorf("TrippedAt %v should be between %v and %v", trippedAt, before, after)
	}
}

func TestCircuitBreaker_ConsecutiveLosses(t *testing.T) {
	cb := NewCircuitBreaker(0.05, 5)

	if cb.ConsecutiveLosses() != 0 {
		t.Error("initial count should be 0")
	}

	cb.Check(-0.01)
	if cb.ConsecutiveLosses() != 1 {
		t.Error("count should be 1 after one loss")
	}

	cb.Check(-0.02)
	if cb.ConsecutiveLosses() != 2 {
		t.Error("count should be 2 after two losses")
	}

	cb.Check(0.01)
	if cb.ConsecutiveLosses() != 0 {
		t.Error("count should reset to 0 after gain")
	}
}

func TestCircuitBreaker_ConsecutiveLossesResetsOnTrip(t *testing.T) {
	cb := NewCircuitBreaker(0.05, 3)

	cb.Check(-0.01)
	cb.Check(-0.02)
	cb.Check(-0.01) // 3rd consecutive loss trips

	if cb.ConsecutiveLosses() != 0 {
		t.Error("consecutive losses should reset to 0 after trip")
	}
}
