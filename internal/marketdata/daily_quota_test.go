package marketdata

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// TestDailyQuotaTracker_InvalidStateDirFailsClosed pins the #2014 fail-closed
// contract: an unusable state directory means today's usage is UNKNOWN, so the
// tracker must not hand out budget.
//
// Before #2014 this case silently reported "100 remaining" — a quota guard that
// announces a full budget exactly when it cannot read its own counter is the
// #2009 "empty value passes" failure mode, one level down.
func TestDailyQuotaTracker_InvalidStateDirFailsClosed(t *testing.T) {
	tracker := NewDailyQuotaTracker("test_invalid", "/nonexistent/path", 100)
	if tracker == nil {
		t.Fatal("tracker should not be nil")
	}
	if got := tracker.Remaining(); got != 0 {
		t.Errorf("Remaining() = %d with an unusable state dir, want 0 (fail closed)", got)
	}
	if tracker.AllowCall() {
		t.Error("AllowCall() must refuse when the shared counter cannot be established")
	}
	if err := tracker.StateErr(); !errors.Is(err, ErrQuotaStateUnavailable) {
		t.Errorf("StateErr() = %v, want ErrQuotaStateUnavailable", err)
	}
}

// TestDailyQuotaTracker_CorruptStateFailsClosed covers every shape of "the
// counter file exists but cannot be trusted". Each case must (a) refuse calls,
// (b) report 0 remaining, (c) expose a distinct reason, and (d) keep refusing
// after a restart — a restart must not turn "usage unknown" into "0 used".
func TestDailyQuotaTracker_CorruptStateFailsClosed(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		limit int
	}{
		{name: "truncated json", body: `{"calls_today": 12`, limit: 100},
		{name: "empty file", body: ``, limit: 100},
		{name: "not an object", body: `[]`, limit: 100},
		{name: "missing last_reset", body: `{"calls_today": 7}`, limit: 100},
		{name: "negative calls_today", body: `{"calls_today": -3, "last_reset": "` + time.Now().UTC().Format(time.RFC3339) + `"}`, limit: 100},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			provider := "corrupt"
			stateFile := filepath.Join(dir, provider+"_daily_quota.json")
			if err := os.WriteFile(stateFile, []byte(tc.body), 0o644); err != nil {
				t.Fatalf("write corrupt state: %v", err)
			}

			tracker := NewDailyQuotaTracker(provider, dir, tc.limit)
			if got := tracker.Remaining(); got != 0 {
				t.Errorf("Remaining() = %d on corrupt state, want 0", got)
			}
			if tracker.AllowCall() {
				t.Error("AllowCall() must refuse on corrupt state (no silent 0-and-pass)")
			}
			if err := tracker.StateErr(); !errors.Is(err, ErrQuotaStateCorrupt) {
				t.Errorf("StateErr() = %v, want ErrQuotaStateCorrupt", err)
			}

			// AllowCall is the path that repairs: the bad file is set aside and
			// an explicit marker is written for the current quota day.
			raw, err := os.ReadFile(stateFile)
			if err != nil {
				t.Fatalf("read marker: %v", err)
			}
			var marker QuotaState
			if err := json.Unmarshal(raw, &marker); err != nil {
				t.Fatalf("marker is not valid JSON (%s): %v", raw, err)
			}
			if !marker.QuotaUnknown {
				t.Errorf("marker = %s, want quota_unknown=true", raw)
			}
			if marker.CallsToday != 0 {
				t.Errorf("marker calls_today = %d, want 0 (usage is unknown, not invented)", marker.CallsToday)
			}
			quarantined, err := filepath.Glob(stateFile + ".corrupt-*")
			if err != nil {
				t.Fatalf("glob quarantine: %v", err)
			}
			if len(quarantined) != 1 {
				t.Errorf("quarantine files = %v, want exactly one", quarantined)
			}
			if len(quarantined) == 1 {
				kept, readErr := os.ReadFile(quarantined[0])
				if readErr != nil {
					t.Fatalf("read quarantined file: %v", readErr)
				}
				if string(kept) != tc.body {
					t.Errorf("quarantined content = %q, want the original %q (forensics)", kept, tc.body)
				}
			}

			// Durability: a restart must NOT restore the budget.
			restarted := NewDailyQuotaTracker(provider, dir, tc.limit)
			if restarted.AllowCall() {
				t.Error("a restarted tracker must still refuse: the quota day is latched as unknown")
			}
			if got := restarted.Remaining(); got != 0 {
				t.Errorf("restarted Remaining() = %d, want 0", got)
			}
			if err := restarted.StateErr(); !errors.Is(err, ErrQuotaStateCorrupt) {
				t.Errorf("restarted StateErr() = %v, want ErrQuotaStateCorrupt", err)
			}
			if !strings.Contains(restarted.StateErr().Error(), stateFile) {
				t.Errorf("StateErr() = %q, want it to name the state file", restarted.StateErr())
			}
		})
	}
}

// TestDailyQuotaTracker_CorruptStateRecoversNextQuotaDay proves the fail-closed
// marker is day-scoped like the upstream latch: it must not become a permanent
// kill switch.
func TestDailyQuotaTracker_CorruptStateRecoversNextQuotaDay(t *testing.T) {
	dir := t.TempDir()
	provider := "corrupt_rollover"
	stateFile := filepath.Join(dir, provider+"_daily_quota.json")
	if err := os.WriteFile(stateFile, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write corrupt state: %v", err)
	}

	tracker := NewDailyQuotaTracker(provider, dir, 10)
	if tracker.AllowCall() {
		t.Fatal("corrupt state must refuse")
	}

	// Rewrite the marker with YESTERDAY's quota day, which is what the next day
	// looks like from the tracker's point of view.
	yesterday := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -1)
	rolled := QuotaState{CallsToday: 0, LastReset: yesterday, QuotaUnknown: true, QuotaUnknownReason: "previous corruption"}
	raw, err := json.Marshal(rolled)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(stateFile, raw, 0o644); err != nil {
		t.Fatalf("rewrite state: %v", err)
	}

	if !tracker.AllowCall() {
		t.Fatalf("a new quota day must be usable again, StateErr=%v", tracker.StateErr())
	}
	if got := tracker.CallsToday(); got != 1 {
		t.Errorf("CallsToday() = %d after the rollover, want 1", got)
	}
	if err := tracker.StateErr(); err != nil {
		t.Errorf("StateErr() = %v after the rollover, want nil", err)
	}
}

// TestDailyQuotaTracker_StateIsRereadFromDisk proves the counter is no longer a
// snapshot taken once at construction (#2014): a value written by another
// process after this tracker was built is visible immediately.
func TestDailyQuotaTracker_StateIsRereadFromDisk(t *testing.T) {
	dir := t.TempDir()
	provider := "reread"
	stateFile := filepath.Join(dir, provider+"_daily_quota.json")

	reader := NewDailyQuotaTracker(provider, dir, 100)
	if got := reader.CallsToday(); got != 0 {
		t.Fatalf("fresh CallsToday() = %d, want 0", got)
	}

	// Another process spends 9 calls (written straight to the shared file).
	other := QuotaState{CallsToday: 9, LastReset: time.Now().UTC().Truncate(24 * time.Hour)}
	raw, err := json.Marshal(other)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(stateFile, raw, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := reader.CallsToday(); got != 9 {
		t.Errorf("CallsToday() = %d after another process spent 9, want 9 (read once at construction was the bug)", got)
	}
	if got := reader.Remaining(); got != 91 {
		t.Errorf("Remaining() = %d, want 91", got)
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
