package service

import (
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// TestNewBacktestService tests the constructor.
func TestNewBacktestService(t *testing.T) {
	cfg := config.Config{
		LedgerDir: "/tmp/test-ledger",
		WorkDir:   "/tmp/test-work",
	}

	svc := NewBacktestService(cfg)

	if svc == nil {
		t.Fatal("NewBacktestService returned nil")
	}
	if svc.IsRunning() {
		t.Error("expected IsRunning() to be false initially")
	}
	if svc.cfg.LedgerDir != "/tmp/test-ledger" {
		t.Errorf("cfg.LedgerDir = %q, want %q", svc.cfg.LedgerDir, "/tmp/test-ledger")
	}
}

// TestBacktestService_WithEventBus tests the builder pattern.
func TestBacktestService_WithEventBus(t *testing.T) {
	cfg := config.Config{}
	svc := NewBacktestService(cfg)

	// WithEventBus should return the same service for chaining
	result := svc.WithEventBus(nil)
	if result != svc {
		t.Error("WithEventBus should return the same service instance")
	}
}

// TestBacktestService_IsRunning tests the running state check.
func TestBacktestService_IsRunning(t *testing.T) {
	cfg := config.Config{}
	svc := NewBacktestService(cfg)

	// Initially not running
	if svc.IsRunning() {
		t.Error("expected IsRunning() to be false initially")
	}
}

// TestBacktestService_GetStatus tests that GetStatus returns a copy of status.
func TestBacktestService_GetStatus(t *testing.T) {
	cfg := config.Config{
		LedgerDir: "/tmp/test-ledger",
		WorkDir:   "/tmp/test-work",
	}
	svc := NewBacktestService(cfg)

	status := svc.GetStatus()
	if status == nil {
		t.Fatal("GetStatus returned nil")
	}

	// Modify returned status - should not affect internal state
	status["test_key"] = "test_value"

	// Get status again - should not have the added key
	status2 := svc.GetStatus()
	if _, exists := status2["test_key"]; exists {
		t.Error("GetStatus returned a reference to internal state, not a copy")
	}
}

// TestBacktestService_Reset tests the reset method.
func TestBacktestService_Reset(t *testing.T) {
	cfg := config.Config{}
	svc := NewBacktestService(cfg)

	// Reset on non-running service should be safe
	svc.Reset()

	if svc.IsRunning() {
		t.Error("expected IsRunning() to be false after Reset")
	}
}

// TestBacktestService_ConcurrentIsRunning tests that IsRunning is thread-safe.
func TestBacktestService_ConcurrentIsRunning(t *testing.T) {
	cfg := config.Config{}
	svc := NewBacktestService(cfg)

	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			_ = svc.IsRunning()
		})
	}
	wg.Wait()
	// No panic means thread-safe
}

// TestBacktestService_ConcurrentReset tests that Reset is thread-safe.
func TestBacktestService_ConcurrentReset(t *testing.T) {
	cfg := config.Config{}
	svc := NewBacktestService(cfg)

	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			svc.Reset()
		})
	}
	wg.Wait()
	// No panic means thread-safe
}

// TestBacktestService_GetStatusWithNoHistory tests GetStatus when no history exists.
func TestBacktestService_GetStatusWithNoHistory(t *testing.T) {
	cfg := config.Config{
		LedgerDir: "/tmp/nonexistent-ledger-dir",
		WorkDir:   "/tmp/test-work",
	}
	svc := NewBacktestService(cfg)

	// Should not panic even if ledger dir doesn't exist
	status := svc.GetStatus()
	if status == nil {
		t.Fatal("GetStatus returned nil")
	}

	// Should have empty running status
	if status["running"] != nil {
		t.Errorf("expected nil running status, got %v", status["running"])
	}
}

// TestBacktestService_StartReturnsErrorIfAlreadyRunning tests that Start detects concurrent runs.
func TestBacktestService_StartReturnsErrorIfAlreadyRunning(t *testing.T) {
	cfg := config.Config{
		LedgerDir: "/tmp/test-ledger",
		WorkDir:   "/tmp/test-work",
	}
	svc := NewBacktestService(cfg)

	// Manually set running to true (simulating already running)
	svc.mu.Lock()
	svc.running = true
	svc.mu.Unlock()

	// Start should return error
	err := svc.Start(time.Now().AddDate(0, 0, -10), time.Now())
	if err == nil {
		t.Error("expected error when starting while already running")
	}
	if err.Error() != "backtest already running" {
		t.Errorf("unexpected error message: %v", err)
	}

	// Clean up
	svc.mu.Lock()
	svc.running = false
	svc.mu.Unlock()
}
