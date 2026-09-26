package marketdata

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/logging"
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

// TestDailyQuotaTracker_LockTimeoutFailsClosed pins the bounded wait for the
// cross-process lock. flock has no timeout of its own, so without
// quotaLockTimeout a holder that is alive but wedged (paused container,
// stalled write, blocked stderr) would block every caller — including the
// 5-second index endpoint — indefinitely. The tracker must instead give up and
// fail closed.
func TestDailyQuotaTracker_LockTimeoutFailsClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("advisory file locking is unavailable on windows; the tracker fails closed there")
	}
	dir := t.TempDir()
	provider := "timeout"
	lockFile := filepath.Join(dir, provider+"_daily_quota.json.lock")

	holder, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open lock file: %v", err)
	}
	defer func() { _ = holder.Close() }()
	locked, err := tryLockFile(holder)
	if err != nil || !locked {
		t.Fatalf("test could not take the lock itself (locked=%v err=%v)", locked, err)
	}

	tracker := NewDailyQuotaTracker(provider, dir, 10)
	start := time.Now()
	if tracker.AllowCall() {
		t.Fatal("AllowCall() must not hand out budget while the lock is held by another process")
	}
	elapsed := time.Since(start)
	if elapsed < quotaLockTimeout/2 {
		t.Errorf("AllowCall() gave up after %s, want it to wait for the lock up to ~%s", elapsed, quotaLockTimeout)
	}
	if elapsed > 8*quotaLockTimeout {
		t.Errorf("AllowCall() waited %s, want a bounded wait around %s", elapsed, quotaLockTimeout)
	}
	if err := tracker.StateErr(); !errors.Is(err, ErrQuotaStateUnavailable) {
		t.Errorf("StateErr() = %v, want ErrQuotaStateUnavailable", err)
	}
	if got := tracker.Remaining(); got != 0 {
		t.Errorf("Remaining() = %d while the lock is unavailable, want 0", got)
	}

	// Once the holder releases, the tracker must recover on its own.
	if err := unlockFile(holder); err != nil {
		t.Fatalf("release lock: %v", err)
	}
	if !tracker.AllowCall() {
		t.Fatalf("AllowCall() must succeed again after the lock is released, StateErr=%v", tracker.StateErr())
	}
	if err := tracker.StateErr(); err != nil {
		t.Errorf("StateErr() = %v after recovery, want nil", err)
	}
}

// TestDailyQuotaTracker_CorruptStateBackupFailureKeepsDayClosed covers the
// ordering guarantee in quarantineCorruptStateLocked: the fail-closed marker
// is written BEFORE the forensic copy is taken. A failed copy must therefore
// leave the day closed — never reopen it by leaving no state file behind
// (readStateLocked reads "no file" as a fresh day with a full budget).
func TestDailyQuotaTracker_CorruptStateBackupFailureKeepsDayClosed(t *testing.T) {
	original := quotaWriteBackup
	quotaWriteBackup = func(string, []byte) error { return errors.New("backup device is full") }
	t.Cleanup(func() { quotaWriteBackup = original })

	dir := t.TempDir()
	provider := "corrupt_backup_fail"
	stateFile := filepath.Join(dir, provider+"_daily_quota.json")
	if err := os.WriteFile(stateFile, []byte(`{"calls_today": `), 0o644); err != nil {
		t.Fatalf("seed corrupt state: %v", err)
	}

	tracker := NewDailyQuotaTracker(provider, dir, 100)
	if tracker.AllowCall() {
		t.Fatal("AllowCall() must refuse: the counter is corrupt and the backup failed")
	}
	if err := tracker.StateErr(); !errors.Is(err, ErrQuotaStateCorrupt) {
		t.Errorf("StateErr() = %v, want the corrupt reason", err)
	}

	marker, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("the state file must still exist after a failed backup: %v", err)
	}
	var parsed QuotaState
	if err := json.Unmarshal(marker, &parsed); err != nil {
		t.Fatalf("state file is not the marker (%s): %v", marker, err)
	}
	if !parsed.QuotaUnknown {
		t.Errorf("state file = %s, want quota_unknown=true", marker)
	}

	// And the day must stay closed across a restart.
	restarted := NewDailyQuotaTracker(provider, dir, 100)
	if restarted.AllowCall() {
		t.Error("a restart must not reopen the day after a failed backup")
	}
}

// TestDailyQuotaTracker_WriteFailureRefusesCall proves the "spend but cannot
// record" case fails closed. Handing out budget that was never persisted would
// re-create exactly the cross-process hole #2014 is about.
func TestDailyQuotaTracker_WriteFailureRefusesCall(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	tracker := NewDailyQuotaTracker("write_fail", dir, 100)
	if !tracker.AllowCall() {
		t.Fatalf("seed call must succeed, StateErr=%v", tracker.StateErr())
	}

	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if tracker.AllowCall() {
		t.Fatal("AllowCall() must refuse when the increment cannot be persisted")
	}
	if err := tracker.StateErr(); !errors.Is(err, ErrQuotaStateUnavailable) {
		t.Errorf("StateErr() = %v, want ErrQuotaStateUnavailable", err)
	}
	if got := tracker.Remaining(); got != 0 {
		t.Errorf("Remaining() = %d after a failed write, want 0", got)
	}
}

// TestDailyQuotaTracker_ConcurrentGoroutinesStayWithinCeiling covers the
// in-process half of the same guarantee: many goroutines spending through one
// tracker must still stop at the ceiling.
func TestDailyQuotaTracker_ConcurrentGoroutinesStayWithinCeiling(t *testing.T) {
	const (
		limit     = 60
		goroutine = 8
		attempts  = 30
	)
	dir := t.TempDir()
	tracker := NewDailyQuotaTracker("concurrent", dir, limit)

	var wg sync.WaitGroup
	var mu sync.Mutex
	granted := 0
	for range goroutine {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range attempts {
				if tracker.AllowCall() {
					mu.Lock()
					granted++
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()

	if granted != limit {
		t.Fatalf("granted = %d, want exactly %d (ceiling %d, %d goroutines x %d attempts)",
			granted, limit, limit, goroutine, attempts)
	}
	if got := tracker.CallsToday(); got != limit {
		t.Errorf("CallsToday() = %d, want %d", got, limit)
	}
}

// TestQuotaRegistry_EntryCarriesStateError makes the "used" number honest: a
// tracker whose state is unusable reports used=0 with exhausted=true, and the
// snapshot must say WHY, otherwise the dashboard shows a broken counter as an
// idle one.
func TestQuotaRegistry_EntryCarriesStateError(t *testing.T) {
	registry := NewQuotaRegistry()
	broken := NewDailyQuotaTracker("broken", "/nonexistent/path", 100)
	registry.Register("broken", broken)
	// Force the fail-closed state before the snapshot is taken.
	if broken.AllowCall() {
		t.Fatal("a tracker with an unusable state dir must refuse")
	}

	snapshot := registry.Snapshot()
	if len(snapshot.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(snapshot.Entries))
	}
	entry := snapshot.Entries[0]
	if entry.StateError == "" {
		t.Error("StateError is empty: the snapshot cannot distinguish a broken counter from an idle one")
	}
	if !strings.Contains(entry.StateError, "state unavailable") {
		t.Errorf("StateError = %q, want it to name the state error", entry.StateError)
	}
	if !entry.Exhausted || entry.Remaining != 0 {
		t.Errorf("entry = %+v, want exhausted with 0 remaining", entry)
	}
}

// TestDailyQuotaTracker_MissingStateDirIsCreated pins the documented (and
// warned) boundary: if the state directory does not exist, the tracker creates
// it and counts from zero. That is a private counter, not the shared one — the
// tracker emits daily_quota_state_dir_created to make it visible, because the
// ceiling silently degrades to per-process in that case (#2014).
func TestDailyQuotaTracker_MissingStateDirIsCreated(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "state", "not", "yet", "there")

	tracker := NewDailyQuotaTracker("missing_dir", dir, 10)
	if !tracker.AllowCall() {
		t.Fatalf("a fresh counter dir must be usable, StateErr=%v", tracker.StateErr())
	}
	if _, err := os.Stat(filepath.Join(dir, "missing_dir_daily_quota.json")); err != nil {
		t.Errorf("state file was not created: %v", err)
	}
	if got := tracker.Remaining(); got != 9 {
		t.Errorf("Remaining() = %d, want 9", got)
	}
}

// TestDailyQuotaTracker_LockIsNotRecursive documents the invariant every method
// relies on: flock is per file descriptor, so a second attempt in the same
// process does NOT succeed. update/refresh must therefore never call another
// tracker method while they hold the lock.
func TestDailyQuotaTracker_LockIsNotRecursive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("advisory file locking is unavailable on windows")
	}
	dir := t.TempDir()
	lockFile := filepath.Join(dir, "recursive_daily_quota.json.lock")

	first, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = first.Close() }()
	second, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open second: %v", err)
	}
	defer func() { _ = second.Close() }()

	locked, err := tryLockFile(first)
	if err != nil || !locked {
		t.Fatalf("first lock: locked=%v err=%v", locked, err)
	}
	locked, err = tryLockFile(second)
	if err != nil {
		t.Fatalf("second lock errored: %v", err)
	}
	if locked {
		t.Error("a second fd in the same process acquired the lock: flock is not recursive, so the tracker must never re-enter it")
	}
}

// TestDailyQuotaTracker_CorruptStateEmitsBothLogLines guards the deferred-log
// queue: the corruption report (which carries the operator's repair hint) and
// the fail-closed reason are produced by the same operation, so a single-slot
// queue silently swallowed the actionable one. Logging must also happen after
// the locks are released, which this exercises end to end.
func TestDailyQuotaTracker_CorruptStateEmitsBothLogLines(t *testing.T) {
	var buf bytes.Buffer
	previous := logging.Default()
	logging.SetLogger(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { logging.SetLogger(previous) })

	dir := t.TempDir()
	stateFile := filepath.Join(dir, "loglines_daily_quota.json")
	if err := os.WriteFile(stateFile, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("seed corrupt state: %v", err)
	}

	tracker := NewDailyQuotaTracker("loglines", dir, 100)
	if tracker.AllowCall() {
		t.Fatal("corrupt state must refuse the call")
	}

	emitted := buf.String()
	if !strings.Contains(emitted, "daily_quota_state_corrupt") {
		t.Errorf("the corruption report was not emitted (it carries the repair hint):\n%s", emitted)
	}
	if !strings.Contains(emitted, "repair target") {
		t.Errorf("the repair hint is missing from the emitted logs:\n%s", emitted)
	}
	if !strings.Contains(emitted, "daily_quota_state_unusable") {
		t.Errorf("the fail-closed reason was not emitted:\n%s", emitted)
	}
}
