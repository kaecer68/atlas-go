package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/live"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
)

func TestCircuitBreakerService_GetCircuitBreakerState_Uninitialized(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewCircuitBreakerService(tmpDir)

	state, err := svc.GetCircuitBreakerState()
	if err != nil {
		t.Fatalf("GetCircuitBreakerState: %v", err)
	}
	if state == nil {
		t.Fatal("expected state response")
	}
	if state.Initialized {
		t.Errorf("expected uninitialized, got initialized")
	}
	if state.State != "uninitialized" {
		t.Errorf("expected state uninitialized, got %q", state.State)
	}
	if state.StateChangedAt != "" {
		t.Errorf("expected empty StateChangedAt, got %q", state.StateChangedAt)
	}
	if state.CooldownUntil != "" {
		t.Errorf("expected empty CooldownUntil, got %q", state.CooldownUntil)
	}
	if len(state.Events) != 0 {
		t.Errorf("expected 0 events, got %d", len(state.Events))
	}
}

func TestCircuitBreakerService_GetCircuitBreakerState_Initialized(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "data", "state", "circuit_breaker_state.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	stateData := map[string]any{
		"state":            "paused",
		"state_changed_at": time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC),
		"consecutive_sl":   2,
		"cooldown_until":   time.Date(2026, 5, 12, 10, 15, 0, 0, time.UTC),
		"intraday_peak":    3050000.0,
		"day_start_value":  3000000.0,
	}
	data, _ := json.Marshal(stateData)
	if err := os.WriteFile(statePath, data, 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	svc := NewCircuitBreakerService(tmpDir)
	state, err := svc.GetCircuitBreakerState()
	if err != nil {
		t.Fatalf("GetCircuitBreakerState: %v", err)
	}
	if !state.Initialized {
		t.Error("expected initialized")
	}
	if state.State != "paused" {
		t.Errorf("expected state paused, got %q", state.State)
	}
	if state.ConsecutiveSL != 2 {
		t.Errorf("expected ConsecutiveSL 2, got %d", state.ConsecutiveSL)
	}
	if state.IntradayPeak != 3050000.0 {
		t.Errorf("expected IntradayPeak 3050000, got %f", state.IntradayPeak)
	}
	if state.DayStartValue != 3000000.0 {
		t.Errorf("expected DayStartValue 3000000, got %f", state.DayStartValue)
	}
}

func TestCircuitBreakerService_GetCircuitBreakerEvents(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewCircuitBreakerService(tmpDir)

	// No log file yet - should return nil
	events, err := svc.GetCircuitBreakerEvents()
	if err != nil {
		t.Fatalf("GetCircuitBreakerEvents: %v", err)
	}
	if events != nil {
		t.Errorf("expected nil events, got %v", events)
	}

	// Reset from normal to normal is a no-op; first drive to paused, then reset
	svc.cb.SetRules([]live.CircuitBreakerRule{
		{Name: "test", Enabled: true, ConsecutiveSL: 1, CooldownMinutes: 0},
	})
	svc.cb.RecordStopLoss()
	if svc.cb.State() != live.CircuitPaused {
		t.Fatalf("expected paused, got %q", svc.cb.State())
	}

	if err := svc.ResetCircuitBreaker("test_reset"); err != nil {
		t.Fatalf("ResetCircuitBreaker: %v", err)
	}

	events, err = svc.GetCircuitBreakerEvents()
	if err != nil {
		t.Fatalf("GetCircuitBreakerEvents after reset: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected events after state transition")
	}
	foundNormal := false
	for _, ev := range events {
		if ev.ToState == live.CircuitNormal && ev.Reason == "test_reset" {
			foundNormal = true
			break
		}
	}
	if !foundNormal {
		t.Errorf("expected reset to normal event, got %+v", events)
	}
}

func TestCircuitBreakerService_StateMachine_ClosedToOpen(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewCircuitBreakerService(tmpDir)

	// Initially normal
	state, _ := svc.GetCircuitBreakerState()
	if state.State != "uninitialized" {
		t.Fatalf("expected uninitialized, got %q", state.State)
	}

	// Reset to initialize day state
	svc.ResetCircuitBreaker("market_open")
	if svc.cb.State() != live.CircuitNormal {
		t.Fatalf("expected normal after reset, got %q", svc.cb.State())
	}

	// Record 3 stop losses to trigger paused state
	svc.cb.RecordStopLoss()
	svc.cb.RecordStopLoss()
	svc.cb.RecordStopLoss()

	if svc.cb.State() != live.CircuitPaused {
		t.Errorf("expected paused after 3 stop losses, got %q", svc.cb.State())
	}

	// Verify events were logged
	events, _ := svc.GetCircuitBreakerEvents()
	if len(events) == 0 {
		t.Fatal("expected events after state transition")
	}
	foundPause := false
	for _, ev := range events {
		if ev.ToState == live.CircuitPaused {
			foundPause = true
			break
		}
	}
	if !foundPause {
		t.Error("expected at least one paused transition event")
	}
}

func TestCircuitBreakerService_StateMachine_HalfOpenToClosed(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewCircuitBreakerService(tmpDir)

	// Set a rule with a very short cooldown so we can test half-open recovery
	svc.cb.SetRules([]live.CircuitBreakerRule{
		{
			Name:            "test_rule",
			Enabled:         true,
			ConsecutiveSL:   1,
			CooldownMinutes: 0,
		},
	})

	// Reset to normal first
	svc.ResetCircuitBreaker("market_open")

	// One stop loss triggers pause with 0-minute cooldown
	svc.cb.RecordStopLoss()
	if svc.cb.State() != live.CircuitPaused {
		t.Fatalf("expected paused after stop loss, got %q", svc.cb.State())
	}

	// Evaluate to trigger auto-recovery from cooldown (0 minutes means immediate)
	svc.cb.Evaluate(livestore.PortfolioState{Cash: 3000000}, nil, nil)

	if svc.cb.State() != live.CircuitNormal {
		t.Errorf("expected normal after cooldown expired, got %q", svc.cb.State())
	}

	// Verify state response reflects normal
	state, _ := svc.GetCircuitBreakerState()
	if state.State != "normal" {
		t.Errorf("expected state normal in response, got %q", state.State)
	}
}

func TestCircuitBreakerService_GetCircuitBreakerState_MalformedFile(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "data", "state", "circuit_breaker_state.json")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write invalid JSON
	if err := os.WriteFile(statePath, []byte("not json"), 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	svc := NewCircuitBreakerService(tmpDir)
	state, err := svc.GetCircuitBreakerState()
	if err != nil {
		t.Fatalf("GetCircuitBreakerState: %v", err)
	}
	// Malformed data should be treated as uninitialized
	if state.Initialized {
		t.Error("expected uninitialized for malformed state file")
	}
	if state.State != "uninitialized" {
		t.Errorf("expected uninitialized state, got %q", state.State)
	}
}

func TestCircuitBreakerService_ResetCircuitBreaker(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewCircuitBreakerService(tmpDir)

	// Reset from already-normal is a no-op in live.CircuitBreaker; drive to paused first.
	svc.cb.SetRules([]live.CircuitBreakerRule{
		{Name: "test", Enabled: true, ConsecutiveSL: 1, CooldownMinutes: 0},
	})
	svc.cb.RecordStopLoss()
	if svc.cb.State() != live.CircuitPaused {
		t.Fatalf("expected paused, got %q", svc.cb.State())
	}

	// Reset with explicit reason
	if err := svc.ResetCircuitBreaker("manual_reset"); err != nil {
		t.Fatalf("ResetCircuitBreaker: %v", err)
	}
	if svc.cb.State() != live.CircuitNormal {
		t.Errorf("expected normal after reset, got %q", svc.cb.State())
	}
	state, _ := svc.GetCircuitBreakerState()
	if state.State != "normal" {
		t.Errorf("expected service state normal after reset, got %q", state.State)
	}

	// Reset with empty reason defaults to manual_reset
	svc.cb.RecordStopLoss()
	if svc.cb.State() != live.CircuitPaused {
		t.Fatalf("expected paused, got %q", svc.cb.State())
	}
	if err := svc.ResetCircuitBreaker(""); err != nil {
		t.Fatalf("ResetCircuitBreaker empty: %v", err)
	}
	if svc.cb.State() != live.CircuitNormal {
		t.Errorf("expected normal after empty reset, got %q", svc.cb.State())
	}
}
