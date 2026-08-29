package marketdata

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDailyQuotaTracker_Remaining tests the Remaining method.
func TestDailyQuotaTracker_Remaining(t *testing.T) {
	// Create a temp state dir
	tmpDir := t.TempDir()

	t.Run("full quota on fresh start", func(t *testing.T) {
		tracker := NewDailyQuotaTracker("test_fresh", tmpDir, 100)
		remaining := tracker.Remaining()
		if remaining != 100 {
			t.Errorf("expected 100 remaining, got %d", remaining)
		}
	})

	t.Run("remaining decreases after AllowCall", func(t *testing.T) {
		tracker := NewDailyQuotaTracker("test_decrease", tmpDir, 50)
		tracker.AllowCall()
		tracker.AllowCall()
		remaining := tracker.Remaining()
		if remaining != 48 {
			t.Errorf("expected 48 remaining after 2 calls, got %d", remaining)
		}
	})

	t.Run("zero remaining when exhausted", func(t *testing.T) {
		tracker := NewDailyQuotaTracker("test_exhausted", tmpDir, 3)
		tracker.AllowCall()
		tracker.AllowCall()
		tracker.AllowCall()
		remaining := tracker.Remaining()
		if remaining != 0 {
			t.Errorf("expected 0 remaining after 3 calls, got %d", remaining)
		}
	})
}

// TestDailyQuotaTracker_CallsToday tests the CallsToday method.
func TestDailyQuotaTracker_CallsToday(t *testing.T) {
	tmpDir := t.TempDir()

	tracker := NewDailyQuotaTracker("test_calls", tmpDir, 100)
	if got := tracker.CallsToday(); got != 0 {
		t.Errorf("expected 0 calls on fresh start, got %d", got)
	}

	tracker.AllowCall()
	tracker.AllowCall()
	tracker.AllowCall()

	if got := tracker.CallsToday(); got != 3 {
		t.Errorf("expected 3 calls today, got %d", got)
	}
}

// TestDailyQuotaTracker_SetLimit tests the SetLimit method.
func TestDailyQuotaTracker_SetLimit(t *testing.T) {
	tmpDir := t.TempDir()

	tracker := NewDailyQuotaTracker("test_limit", tmpDir, 50)
	tracker.AllowCall()
	tracker.AllowCall()

	// Set a new limit
	tracker.SetLimit(10)

	// Remaining should now reflect new limit minus existing calls
	remaining := tracker.Remaining()
	if remaining != 8 {
		t.Errorf("expected 8 remaining (10 - 2 calls), got %d", remaining)
	}
}

// TestDailyQuotaTracker_StatePersistence tests that state persists across instances.
func TestDailyQuotaTracker_StatePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	provider := "test_persist"

	// First instance: make some calls
	tracker1 := NewDailyQuotaTracker(provider, tmpDir, 100)
	tracker1.AllowCall()
	tracker1.AllowCall()

	// Second instance with same provider should see the calls
	tracker2 := NewDailyQuotaTracker(provider, tmpDir, 100)
	if got := tracker2.CallsToday(); got != 2 {
		t.Errorf("expected 2 calls from persisted state, got %d", got)
	}
	if got := tracker2.Remaining(); got != 98 {
		t.Errorf("expected 98 remaining from persisted state, got %d", got)
	}
}

// TestDailyQuotaTracker_DifferentProvidersIndependent tests that different providers have independent state.
func TestDailyQuotaTracker_DifferentProvidersIndependent(t *testing.T) {
	tmpDir := t.TempDir()

	trackerA := NewDailyQuotaTracker("provider_a", tmpDir, 50)
	trackerB := NewDailyQuotaTracker("provider_b", tmpDir, 50)

	trackerA.AllowCall()
	trackerA.AllowCall()
	trackerB.AllowCall()

	if got := trackerA.CallsToday(); got != 2 {
		t.Errorf("provider_a: expected 2 calls, got %d", got)
	}
	if got := trackerB.CallsToday(); got != 1 {
		t.Errorf("provider_b: expected 1 call, got %d", got)
	}
}

// TestDailyQuotaTracker_AllowCallReturnsFalseWhenExhausted tests that AllowCall returns false when quota is exhausted.
func TestDailyQuotaTracker_AllowCallReturnsFalseWhenExhausted(t *testing.T) {
	tmpDir := t.TempDir()

	tracker := NewDailyQuotaTracker("test_exhaust", tmpDir, 2)
	if ok := tracker.AllowCall(); !ok {
		t.Error("first call should succeed")
	}
	if ok := tracker.AllowCall(); !ok {
		t.Error("second call should succeed")
	}
	if ok := tracker.AllowCall(); ok {
		t.Error("third call should fail when exhausted")
	}
}

// TestDailyQuotaTracker_AllowCallIncrementsCounter tests that AllowCall increments counter.
func TestDailyQuotaTracker_AllowCallIncrementsCounter(t *testing.T) {
	tmpDir := t.TempDir()

	tracker := NewDailyQuotaTracker("test_increment", tmpDir, 10)
	for range 5 {
		tracker.AllowCall()
	}

	if got := tracker.CallsToday(); got != 5 {
		t.Errorf("expected 5 calls, got %d", got)
	}
}

// TestDailyQuotaTracker_NewWithInvalidStateDir tests creation with an invalid state directory.
func TestDailyQuotaTracker_NewWithInvalidStateDir(t *testing.T) {
	// Should not panic even if dir is invalid; it will just not persist
	tracker := NewDailyQuotaTracker("test_invalid", "/nonexistent/path", 100)
	if tracker == nil {
		t.Fatal("tracker should not be nil")
	}
	if got := tracker.Remaining(); got != 100 {
		t.Errorf("expected 100 remaining on new tracker, got %d", got)
	}
}

// TestDailyQuotaTracker_LoadSkipsOldState tests that old day state is not loaded.
func TestDailyQuotaTracker_LoadSkipsOldState(t *testing.T) {
	tmpDir := t.TempDir()
	provider := "test_old_day"

	// Create a tracker and write old day state directly
	stateFile := filepath.Join(tmpDir, provider+"_daily_quota.json")
	oldState := `{"calls_today": 80, "last_reset": "2020-01-01T00:00:00Z"}`
	if err := os.WriteFile(stateFile, []byte(oldState), 0o644); err != nil {
		t.Fatalf("failed to write old state: %v", err)
	}

	// New tracker should not load old day state
	tracker := NewDailyQuotaTracker(provider, tmpDir, 100)
	if got := tracker.CallsToday(); got != 0 {
		t.Errorf("expected 0 calls for new day, got %d", got)
	}
	if got := tracker.Remaining(); got != 100 {
		t.Errorf("expected 100 remaining for new day, got %d", got)
	}
}

// TestDailyQuotaTracker_LoadLoadsTodayState tests that same-day state is loaded.
func TestDailyQuotaTracker_LoadLoadsTodayState(t *testing.T) {
	tmpDir := t.TempDir()
	provider := "test_today"

	// Create a tracker first to set up state
	tracker := NewDailyQuotaTracker(provider, tmpDir, 100)
	tracker.AllowCall()
	tracker.AllowCall()

	// Create a new instance - it should load today's state
	tracker2 := NewDailyQuotaTracker(provider, tmpDir, 100)
	if got := tracker2.CallsToday(); got != 2 {
		t.Errorf("expected 2 calls from today's persisted state, got %d", got)
	}
	if got := tracker2.Remaining(); got != 98 {
		t.Errorf("expected 98 remaining, got %d", got)
	}
}

// TestDailyQuotaTracker_SetLimitDoesNotAffectExistingCalls tests that changing limit doesn't retroactively reduce calls.
func TestDailyQuotaTracker_SetLimitDoesNotAffectExistingCalls(t *testing.T) {
	tmpDir := t.TempDir()

	tracker := NewDailyQuotaTracker("test_limit_change", tmpDir, 100)
	tracker.AllowCall()
	tracker.AllowCall()

	// Reduce limit to 1
	tracker.SetLimit(1)

	// Already used 2 calls, so remaining = max(0, 1-2) = 0
	if got := tracker.Remaining(); got != 0 {
		t.Errorf("expected 0 remaining (1 limit - 2 calls, floored at 0), got %d", got)
	}
}

// TestDailyQuotaTracker_CallsTodayReturnsExactCount tests CallsToday returns exact count.
func TestDailyQuotaTracker_CallsTodayReturnsExactCount(t *testing.T) {
	tmpDir := t.TempDir()

	tracker := NewDailyQuotaTracker("test_exact", tmpDir, 1000)
	for range 123 {
		tracker.AllowCall()
	}

	if got := tracker.CallsToday(); got != 123 {
		t.Errorf("expected 123 calls, got %d", got)
	}
}

// TestDailyQuotaTracker_RemainingFlooredAtZero tests that remaining never goes negative.
func TestDailyQuotaTracker_RemainingFlooredAtZero(t *testing.T) {
	tmpDir := t.TempDir()

	tracker := NewDailyQuotaTracker("test_floor", tmpDir, 5)
	for range 10 {
		tracker.AllowCall()
	}

	// Should be floored at 0, not negative
	if got := tracker.Remaining(); got != 0 {
		t.Errorf("expected 0 remaining (floored), got %d", got)
	}
}
