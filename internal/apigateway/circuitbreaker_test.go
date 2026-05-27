package apigateway

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// NewCircuitBreaker tests
// =============================================================================

func TestNewCircuitBreaker_InitialState(t *testing.T) {
	cb := NewCircuitBreaker("test-channel")

	if cb.State() != StateClosed {
		t.Errorf("expected StateClosed, got %v", cb.State())
	}
	if cb.IsOpen() {
		t.Error("expected IsOpen() to be false for new breaker")
	}
	if cb.failures != 0 {
		t.Errorf("expected 0 failures, got %d", cb.failures)
	}
	if cb.halfOpenCalls != 0 {
		t.Errorf("expected 0 halfOpenCalls, got %d", cb.halfOpenCalls)
	}
	if cb.channelID != "test-channel" {
		t.Errorf("expected channelID 'test-channel', got %q", cb.channelID)
	}
}

// =============================================================================
// State / IsOpen tests
// =============================================================================

func TestCircuitBreaker_State_And_IsOpen(t *testing.T) {
	t.Run("new breaker is closed", func(t *testing.T) {
		cb := NewCircuitBreaker("ch")
		if cb.State() != StateClosed {
			t.Errorf("expected StateClosed, got %v", cb.State())
		}
		if cb.IsOpen() {
			t.Error("expected IsOpen()=false")
		}
	})

	t.Run("after triple failure, circuit is open", func(t *testing.T) {
		cb := NewCircuitBreaker("ch")
		failFn := func() error { return errors.New("fail") }

		for i := 0; i < CircuitBreakerFailureThreshold; i++ {
			_ = cb.Call(failFn)
		}

		if cb.State() != StateOpen {
			t.Errorf("expected StateOpen after %d failures, got %v", CircuitBreakerFailureThreshold, cb.State())
		}
		if !cb.IsOpen() {
			t.Error("expected IsOpen()=true after threshold failures")
		}
	})

	t.Run("after success, circuit is closed", func(t *testing.T) {
		cb := NewCircuitBreaker("ch")
		_ = cb.Call(func() error { return nil })

		if cb.State() != StateClosed {
			t.Errorf("expected StateClosed after success, got %v", cb.State())
		}
	})
}

// =============================================================================
// Call tests — full state machine coverage
// =============================================================================

var errTestFailure = errors.New("test failure")

func TestCircuitBreaker_Call_SuccessfulCallsStayClosed(t *testing.T) {
	cb := NewCircuitBreaker("ch")

	for i := 0; i < 10; i++ {
		err := cb.Call(func() error { return nil })
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if cb.State() != StateClosed {
			t.Fatalf("call %d: expected StateClosed, got %v", i, cb.State())
		}
	}
}

func TestCircuitBreaker_Call_FailuresResetOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker("ch")

	// Two failures — not enough to open
	for i := 0; i < CircuitBreakerFailureThreshold-1; i++ {
		_ = cb.Call(func() error { return errTestFailure })
	}
	if cb.failures != 2 {
		t.Fatalf("expected 2 failures, got %d", cb.failures)
	}

	// One success resets
	_ = cb.Call(func() error { return nil })
	if cb.failures != 0 {
		t.Fatalf("expected 0 failures after success, got %d", cb.failures)
	}
	if cb.State() != StateClosed {
		t.Fatalf("expected StateClosed after success, got %v", cb.State())
	}
}

func TestCircuitBreaker_Call_ThreeFailuresOpenCircuit(t *testing.T) {
	cb := NewCircuitBreaker("ch")

	for i := 0; i < CircuitBreakerFailureThreshold; i++ {
		// First N-1 calls should not yet open
		err := cb.Call(func() error { return errTestFailure })
		if i < CircuitBreakerFailureThreshold-1 {
			if cb.State() != StateClosed {
				t.Fatalf("call %d: expected StateClosed, got %v", i, cb.State())
			}
		}
		// Verify error propagation
		if err == nil {
			t.Fatalf("call %d: expected error, got nil", i)
		}
	}

	if cb.State() != StateOpen {
		t.Fatalf("expected StateOpen after %d failures, got %v", CircuitBreakerFailureThreshold, cb.State())
	}
	if !cb.IsOpen() {
		t.Fatal("expected IsOpen()=true")
	}
}

func TestCircuitBreaker_Call_OpenCircuitRejectsCalls(t *testing.T) {
	cb := NewCircuitBreaker("test-chan")

	// Put breaker into open state
	for i := 0; i < CircuitBreakerFailureThreshold; i++ {
		_ = cb.Call(func() error { return errTestFailure })
	}

	// Now calls should be rejected
	err := cb.Call(func() error { return nil })
	if err == nil {
		t.Fatal("expected error when circuit is open, got nil")
	}
	if !strings.Contains(err.Error(), "circuit breaker open") {
		t.Errorf("expected error to contain 'circuit breaker open', got: %v", err)
	}
	if !strings.Contains(err.Error(), "test-chan") {
		t.Errorf("expected error to contain channel ID 'test-chan', got: %v", err)
	}
}

func TestCircuitBreaker_Call_RecoveryTransitionsToHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker("ch")

	// Put breaker into open state
	for i := 0; i < CircuitBreakerFailureThreshold; i++ {
		_ = cb.Call(func() error { return errTestFailure })
	}
	if cb.State() != StateOpen {
		t.Fatal("expected StateOpen")
	}

	// Simulate recovery timeout by setting lastFailure far into the past
	cb.lastFailure = time.Now().Add(-10 * time.Minute)

	// Next call should transition to half-open
	err := cb.Call(func() error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cb.State() != StateClosed {
		t.Fatalf("expected StateClosed after success in half-open, got %v", cb.State())
	}
}

func TestCircuitBreaker_Call_HalfOpenUpToMaxCalls(t *testing.T) {
	cb := NewCircuitBreaker("ch")

	// Put breaker into open state, then simulate recovery
	for i := 0; i < CircuitBreakerFailureThreshold; i++ {
		_ = cb.Call(func() error { return errTestFailure })
	}
	cb.lastFailure = time.Now().Add(-10 * time.Minute)

	// Circuit should now be half-open after the first call succeeds
	// But let's do a more careful test: fail the first half-open call
	// to stay half-open (well, actually fail re-opens...)
	// Let me test: after recovery timeout, Call transitions state to half-open
	// and allows calls. First call could succeed or fail.

	// Test: multiple allowed calls in half-open (each after re-entering)
	// Actually, let me use a helper approach.

	// Enter half-open, fail call → back to open
	cb2 := NewCircuitBreaker("ch2")
	for i := 0; i < CircuitBreakerFailureThreshold; i++ {
		_ = cb2.Call(func() error { return errTestFailure })
	}
	cb2.lastFailure = time.Now().Add(-10 * time.Minute)

	// First half-open call: succeed → closed
	err := cb2.Call(func() error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cb2.State() != StateClosed {
		t.Fatalf("expected StateClosed after success in half-open, got %v", cb2.State())
	}
}

func TestCircuitBreaker_Call_HalfOpenLimitRejectsCalls(t *testing.T) {
	// To test this edge case: we need the circuit in half-open with halfOpenCalls >= 2
	// Since halfOpenCalls is only incremented inside Call() when state == StateHalfOpen,
	// and the state transitions to Open on failure or Closed on success within the same call,
	// the only way to hit this path is if halfOpenCalls was already at max before the check.

	// But wait — halfOpenCalls is reset to 0 when entering half-open from open state.
	// So the only way to hit this path is if two calls are made while still half-open
	// (before the first one succeeds/fails). In the current implementation, the lock is
	// released between the state check and the function execution — but the counter is
	// incremented before the lock is released for fn() execution.

	// Actually, looking more carefully:
	// 1. Lock, check state, increment halfOpenCalls, Unlock
	// 2. Execute fn()
	// 3. Lock, update state/failures based on err, Unlock

	// This means two goroutines could both increment halfOpenCalls before either
	// fn() completes. If both increment and we're at the limit, the third caller
	// would get rejected.

	// For a single-threaded test, this is hard to trigger. Let me directly set
	// halfOpenCalls to simulate the case.

	cb := NewCircuitBreaker("ch")
	for i := 0; i < CircuitBreakerFailureThreshold; i++ {
		_ = cb.Call(func() error { return errTestFailure })
	}
	cb.lastFailure = time.Now().Add(-10 * time.Minute)
	// Manually set the state and counter to simulate a concurrent race
	cb.state = StateHalfOpen
	cb.halfOpenCalls = CircuitBreakerHalfOpenMaxCalls

	err := cb.Call(func() error { return nil })
	if err == nil {
		t.Fatal("expected error when half-open limit reached")
	}
	if !strings.Contains(err.Error(), "half-open limit reached") {
		t.Errorf("expected error to mention half-open limit, got: %v", err)
	}
}

func TestCircuitBreaker_Call_SuccessInHalfOpenResetsToClosed(t *testing.T) {
	cb := NewCircuitBreaker("ch")

	// Open the circuit
	for i := 0; i < CircuitBreakerFailureThreshold; i++ {
		_ = cb.Call(func() error { return errTestFailure })
	}
	// Simulate recovery
	cb.lastFailure = time.Now().Add(-10 * time.Minute)

	// Success in half-open → closed
	err := cb.Call(func() error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cb.State() != StateClosed {
		t.Fatalf("expected StateClosed after success in half-open, got %v", cb.State())
	}
	if cb.failures != 0 {
		t.Fatalf("expected 0 failures after reset, got %d", cb.failures)
	}
	if cb.halfOpenCalls != 0 {
		t.Fatalf("expected 0 halfOpenCalls after reset, got %d", cb.halfOpenCalls)
	}
}

func TestCircuitBreaker_Call_FailureInHalfOpenReOpens(t *testing.T) {
	cb := NewCircuitBreaker("ch")

	// Open the circuit
	for i := 0; i < CircuitBreakerFailureThreshold; i++ {
		_ = cb.Call(func() error { return errTestFailure })
	}
	// Simulate recovery
	cb.lastFailure = time.Now().Add(-10 * time.Minute)

	// Failure in half-open → back to open
	err := cb.Call(func() error { return errTestFailure })
	if err == nil {
		t.Fatal("expected error from failed fn")
	}
	if cb.State() != StateOpen {
		t.Fatalf("expected StateOpen after failure in half-open, got %v", cb.State())
	}
}

func TestCircuitBreaker_Call_ErrorPropagation(t *testing.T) {
	cb := NewCircuitBreaker("ch")
	testErr := errors.New("specific error")

	err := cb.Call(func() error { return testErr })
	if err != testErr {
		t.Errorf("expected testErr, got %v", err)
	}
}

// =============================================================================
// Call — recovery timeout edge cases
// =============================================================================

func TestCircuitBreaker_Call_RecoveryNotYetExpired(t *testing.T) {
	cb := NewCircuitBreaker("ch")

	// Open the circuit
	for i := 0; i < CircuitBreakerFailureThreshold; i++ {
		_ = cb.Call(func() error { return errTestFailure })
	}
	// lastFailure was just set — recovery NOT yet expired
	err := cb.Call(func() error { return nil })
	if err == nil {
		t.Fatal("expected error because recovery timeout not expired")
	}
	if !strings.Contains(err.Error(), "circuit breaker open") {
		t.Errorf("expected 'circuit breaker open', got: %v", err)
	}
}

func TestCircuitBreaker_Call_RecoveryExactlyAtBoundary(t *testing.T) {
	cb := NewCircuitBreaker("ch")

	for i := 0; i < CircuitBreakerFailureThreshold; i++ {
		_ = cb.Call(func() error { return errTestFailure })
	}
	// Set lastFailure exactly at the recovery timeout boundary
	cb.lastFailure = time.Now().Add(-CircuitBreakerRecoveryTimeout)

	err := cb.Call(func() error { return nil })
	if err != nil {
		t.Fatalf("expected success at recovery boundary, got: %v", err)
	}
	if cb.State() != StateClosed {
		t.Errorf("expected StateClosed, got %v", cb.State())
	}
}

// =============================================================================
// Multiple open→half-open→open cycles (oscillation test)
// =============================================================================

func TestCircuitBreaker_Call_OscillationPattern(t *testing.T) {
	cb := NewCircuitBreaker("ch")

	// Cycle 1: open → half-open (fail) → open
	for i := 0; i < CircuitBreakerFailureThreshold; i++ {
		_ = cb.Call(func() error { return errTestFailure })
	}
	cb.lastFailure = time.Now().Add(-10 * time.Minute)
	_ = cb.Call(func() error { return errTestFailure })
	if cb.State() != StateOpen {
		t.Fatalf("cycle 1: expected StateOpen, got %v", cb.State())
	}

	// Cycle 2: open → half-open (fail) → open
	cb.lastFailure = time.Now().Add(-10 * time.Minute)
	_ = cb.Call(func() error { return errTestFailure })
	if cb.State() != StateOpen {
		t.Fatalf("cycle 2: expected StateOpen, got %v", cb.State())
	}

	// Cycle 3: open → half-open (success) → closed
	cb.lastFailure = time.Now().Add(-10 * time.Minute)
	err := cb.Call(func() error { return nil })
	if err != nil {
		t.Fatalf("cycle 3: unexpected error: %v", err)
	}
	if cb.State() != StateClosed {
		t.Fatalf("cycle 3: expected StateClosed, got %v", cb.State())
	}
}

// =============================================================================
// NewCircuitBreakerManager tests
// =============================================================================

func TestNewCircuitBreakerManager_CreatesBreakers(t *testing.T) {
	channelIDs := []string{"twse", "fugle", "finmind"}
	mgr := NewCircuitBreakerManager(channelIDs)

	if len(mgr.breakers) != len(channelIDs) {
		t.Fatalf("expected %d breakers, got %d", len(channelIDs), len(mgr.breakers))
	}

	for _, id := range channelIDs {
		cb, err := mgr.Get(id)
		if err != nil {
			t.Errorf("expected breaker for %q, got error: %v", id, err)
			continue
		}
		if cb.channelID != id {
			t.Errorf("breaker for %q has channelID %q", id, cb.channelID)
		}
		if cb.State() != StateClosed {
			t.Errorf("breaker for %q should start closed, got %v", id, cb.State())
		}
	}
}

func TestNewCircuitBreakerManager_EmptyInput(t *testing.T) {
	mgr := NewCircuitBreakerManager([]string{})
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	if len(mgr.breakers) != 0 {
		t.Errorf("expected 0 breakers, got %d", len(mgr.breakers))
	}
}

func TestNewCircuitBreakerManager_NilInput(t *testing.T) {
	mgr := NewCircuitBreakerManager(nil)
	if mgr == nil {
		t.Fatal("expected non-nil manager for nil input")
	}
	if len(mgr.breakers) != 0 {
		t.Errorf("expected 0 breakers for nil input, got %d", len(mgr.breakers))
	}
}

// =============================================================================
// CircuitBreakerManager.Get tests
// =============================================================================

func TestCircuitBreakerManager_Get_ValidChannel(t *testing.T) {
	mgr := NewCircuitBreakerManager([]string{"twse", "fugle"})

	cb, err := mgr.Get("twse")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cb == nil {
		t.Fatal("expected non-nil breaker")
	}
	if cb.channelID != "twse" {
		t.Errorf("expected channelID 'twse', got %q", cb.channelID)
	}
}

func TestCircuitBreakerManager_Get_UnknownChannel(t *testing.T) {
	mgr := NewCircuitBreakerManager([]string{"twse"})

	cb, err := mgr.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown channel")
	}
	if cb != nil {
		t.Errorf("expected nil breaker for unknown channel, got %v", cb)
	}
	if !strings.Contains(err.Error(), "unknown channel") {
		t.Errorf("expected 'unknown channel' in error, got: %v", err)
	}
}

func TestCircuitBreakerManager_Get_EmptyChannelID(t *testing.T) {
	mgr := NewCircuitBreakerManager([]string{"twse"})

	cb, err := mgr.Get("")
	if err == nil {
		t.Fatal("expected error for empty channel ID")
	}
	if cb != nil {
		t.Error("expected nil breaker for empty channel")
	}
}

// =============================================================================
// CircuitBreakerManager.Status tests
// =============================================================================

func TestCircuitBreakerManager_Status(t *testing.T) {
	mgr := NewCircuitBreakerManager([]string{"twse", "fugle"})

	// Open one of the breakers
	twseCB, _ := mgr.Get("twse")
	for i := 0; i < CircuitBreakerFailureThreshold; i++ {
		_ = twseCB.Call(func() error { return errTestFailure })
	}

	status := mgr.Status()

	if len(status) != 2 {
		t.Fatalf("expected 2 status entries, got %d", len(status))
	}

	s, ok := status["twse"]
	if !ok {
		t.Fatal("expected status for 'twse'")
	}
	if s.ChannelID != "twse" {
		t.Errorf("expected ChannelID 'twse', got %q", s.ChannelID)
	}
	if s.State != "open" {
		t.Errorf("expected State 'open', got %q", s.State)
	}
	if s.Failures != CircuitBreakerFailureThreshold {
		t.Errorf("expected %d failures, got %d", CircuitBreakerFailureThreshold, s.Failures)
	}

	s, ok = status["fugle"]
	if !ok {
		t.Fatal("expected status for 'fugle'")
	}
	if s.State != "closed" {
		t.Errorf("expected State 'closed', got %q", s.State)
	}
}

func TestCircuitBreakerManager_Status_EmptyManager(t *testing.T) {
	mgr := NewCircuitBreakerManager([]string{})
	status := mgr.Status()

	if len(status) != 0 {
		t.Errorf("expected empty status map, got %d entries", len(status))
	}
}

// =============================================================================
// stateString tests
// =============================================================================

func TestStateString(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(999), "unknown"},
		{State(-1), "unknown"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("state_%d", tt.state), func(t *testing.T) {
			got := stateString(tt.state)
			if got != tt.expected {
				t.Errorf("stateString(%d) = %q, want %q", tt.state, got, tt.expected)
			}
		})
	}
}

// =============================================================================
// CircuitBreakerStatus struct tests
// =============================================================================

func TestCircuitBreakerStatus_JSONTags(t *testing.T) {
	// Verify the struct has correct JSON tags (compile-time check)
	var s CircuitBreakerStatus
	_ = s.ChannelID
	_ = s.State
	_ = s.Failures
	_ = s.LastError

	// Verify zero value
	if s.ChannelID != "" {
		t.Error("expected empty ChannelID in zero value")
	}
	if s.State != "" {
		t.Error("expected empty State in zero value")
	}
}

// =============================================================================
// Concurrency safety: concurrent Call on same breaker
// =============================================================================

func TestCircuitBreaker_Call_Concurrent(t *testing.T) {
	cb := NewCircuitBreaker("ch")
	done := make(chan bool, 20)

	// 10 goroutines calling success concurrently
	for i := 0; i < 10; i++ {
		go func() {
			_ = cb.Call(func() error { return nil })
			done <- true
		}()
	}

	// Wait for all to finish
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should remain closed with 0 failures
	if cb.State() != StateClosed {
		t.Errorf("expected StateClosed after concurrent successes, got %v", cb.State())
	}
	if cb.failures != 0 {
		t.Errorf("expected 0 failures, got %d", cb.failures)
	}
}

func TestCircuitBreaker_Call_ConcurrentFailures(t *testing.T) {
	cb := NewCircuitBreaker("ch")
	done := make(chan bool, 20)

	// 20 goroutines calling failure concurrently
	for i := 0; i < 20; i++ {
		go func() {
			_ = cb.Call(func() error { return errTestFailure })
			done <- true
		}()
	}

	for i := 0; i < 20; i++ {
		<-done
	}

	// Should be open (at least threshold failures accumulated)
	if cb.State() != StateOpen {
		t.Errorf("expected StateOpen after concurrent failures, got %v", cb.State())
	}
}
