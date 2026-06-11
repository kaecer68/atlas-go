package marketdata

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newTestHybridProvider() *HybridProvider {
	return &HybridProvider{
		fubonProvider:   nil,
		finmindProvider: nil,
		fugleProvider:   nil,
		twseClient:      nil,
		cbState:         ProviderCircuitClosed,
		cbConfig:        defaultCircuitBreakerConfig(),
		fubonCbState:    ProviderCircuitClosed,
	}
}

func TestHybridProvider_FubonCircuit_Closed_AllowsFubon(t *testing.T) {
	p := newTestHybridProvider()
	p.fubonCbState = ProviderCircuitClosed
	if !p.shouldTryFubon() {
		t.Fatal("expected closed circuit to allow fubon attempt")
	}
}

func TestHybridProvider_FubonCircuit_Open_BlocksFubon(t *testing.T) {
	p := newTestHybridProvider()
	p.fubonCbState = ProviderCircuitOpen
	p.fubonCbLastFailure = time.Now()
	if p.shouldTryFubon() {
		t.Fatal("expected open circuit to block fubon attempt within recovery window")
	}
}

func TestHybridProvider_FubonCircuit_RecordFailureIncrementsCounter(t *testing.T) {
	p := newTestHybridProvider()
	if p.fubonCbFailureCount != 0 {
		t.Fatalf("expected initial failure count 0, got %d", p.fubonCbFailureCount)
	}
	p.RecordFubonFailure(errors.New("boom"))
	if p.fubonCbFailureCount != 1 {
		t.Fatalf("expected failure count 1 after first failure, got %d", p.fubonCbFailureCount)
	}
	if p.fubonCbState != ProviderCircuitClosed {
		t.Fatalf("expected circuit to stay closed after 1 failure, got %s", p.fubonCbState)
	}
}

func TestHybridProvider_FubonCircuit_OpensAfterThresholdFailures(t *testing.T) {
	p := newTestHybridProvider()
	for i := 0; i < p.cbConfig.failureThreshold; i++ {
		p.RecordFubonFailure(errors.New("boom"))
	}
	if p.fubonCbState != ProviderCircuitOpen {
		t.Fatalf("expected circuit to open after %d failures, got %s", p.cbConfig.failureThreshold, p.fubonCbState)
	}
}

func TestHybridProvider_FubonCircuit_RecoveryFromHalfOpen(t *testing.T) {
	p := newTestHybridProvider()
	p.fubonCbState = ProviderCircuitOpen
	p.fubonCbLastFailure = time.Now().Add(-2 * p.cbConfig.recoveryTimeout)
	if !p.shouldTryFubon() {
		t.Fatal("expected circuit to enter half-open after recovery timeout elapsed")
	}
	if p.fubonCbState != ProviderCircuitHalfOpen {
		t.Fatalf("expected state half-open after timeout recovery, got %s", p.fubonCbState)
	}
	p.RecordFubonSuccess()
	if p.fubonCbState != ProviderCircuitClosed {
		t.Fatalf("expected circuit to close after success, got %s", p.fubonCbState)
	}
	if p.fubonCbFailureCount != 0 {
		t.Fatalf("expected failure count reset to 0, got %d", p.fubonCbFailureCount)
	}
}

var _ = context.Background
