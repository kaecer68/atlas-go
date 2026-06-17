package config

import (
	"os"
	"testing"
	"time"
)

func tempLockFile(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "filelock-*.lock")
	if err != nil {
		t.Fatalf("failed to create temp lock file: %v", err)
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		t.Fatalf("failed to close temp lock file: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("failed to remove temp lock file: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(path)
	})
	return path
}

func TestFileLockBasicLockUnlock(t *testing.T) {
	path := tempLockFile(t)
	fl := NewFileLocker(path)

	unlock := fl.Lock()
	if unlock == nil {
		t.Fatal("expected unlock function")
	}

	if _, err := fl.TryLock(50 * time.Millisecond); err == nil {
		t.Fatal("expected TryLock to fail while lock is held")
	}

	unlock()

	unlock2 := fl.Lock()
	if unlock2 == nil {
		t.Fatal("expected unlock function after re-locking")
	}
	unlock2()
}

func TestFileLockTryLockSuccess(t *testing.T) {
	path := tempLockFile(t)
	fl := NewFileLocker(path)

	unlock, err := fl.TryLock(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("first TryLock should succeed: %v", err)
	}
	if unlock == nil {
		t.Fatal("expected unlock function")
	}
	defer unlock()

	if _, err := fl.TryLock(50 * time.Millisecond); err == nil {
		t.Fatal("second TryLock should fail when lock is already held")
	}
}

func TestFileLockTryLockFailAfterTimeout(t *testing.T) {
	path := tempLockFile(t)
	fl := NewFileLocker(path)

	unlock := fl.Lock()
	defer unlock()

	if _, err := fl.TryLock(100 * time.Millisecond); err == nil {
		t.Fatal("expected TryLock to fail while lock is held")
	}
}

func TestFileLockLockWithRetry(t *testing.T) {
	path := tempLockFile(t)
	fl := NewFileLocker(path)

	unlock1 := fl.Lock()
	go func() {
		time.Sleep(150 * time.Millisecond)
		unlock1()
	}()

	start := time.Now()
	unlock2, err := fl.LockWithRetry(10, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("LockWithRetry should succeed after unlock: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 100*time.Millisecond {
		t.Fatalf("LockWithRetry returned too quickly: %v", elapsed)
	}
	unlock2()
}

func TestFileLockDoubleUnlockSafe(t *testing.T) {
	path := tempLockFile(t)
	fl := NewFileLocker(path)

	unlock := fl.Lock()
	unlock()
	unlock()
}

func TestFileLockExclusiveGoroutines(t *testing.T) {
	path := tempLockFile(t)
	fl := NewFileLocker(path)

	unlock1 := fl.Lock()
	acquired := make(chan struct{})

	go func() {
		unlock2 := fl.Lock()
		close(acquired)
		unlock2()
	}()

	time.Sleep(50 * time.Millisecond)
	select {
	case <-acquired:
		t.Fatal("second goroutine should still be waiting for the lock")
	default:
	}

	unlock1()

	select {
	case <-acquired:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("second goroutine should have acquired the lock after unlock")
	}
}
