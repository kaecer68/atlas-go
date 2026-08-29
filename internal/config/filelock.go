package config

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

// FileLocker wraps gofrs/flock with a process-level mutex so that the same
// FileLocker instance cannot be locked twice within a single process.
type FileLocker struct {
	path      string
	mu        sync.Mutex
	flock     *flock.Flock
	lockCount int
}

// NewFileLocker creates a new FileLocker for the given path.
func NewFileLocker(path string) *FileLocker {
	return &FileLocker{
		path:  path,
		flock: flock.New(path),
	}
}

// Lock acquires the file lock and returns an idempotent unlock function.
// The process-level mutex is held for the whole critical section, preventing
// double-locking from the same process.
func (fl *FileLocker) Lock() func() {
	fl.mu.Lock()

	if err := fl.flock.Lock(); err != nil {
		fl.mu.Unlock()
		panic(fmt.Sprintf("file lock failed: %v", err))
	}

	fl.lockCount = 1

	return func() {
		if fl.lockCount == 0 {
			return
		}

		fl.lockCount = 0
		_ = fl.flock.Unlock()
		_ = os.Remove(fl.path)
		fl.mu.Unlock()
	}
}

// TryLock attempts to acquire the lock within the given timeout. It retries
// every 10 milliseconds using the underlying flock's TryLockContext.
func (fl *FileLocker) TryLock(timeout time.Duration) (func(), error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for !fl.mu.TryLock() {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("failed to acquire lock within %v: %w", timeout, ctx.Err())
		case <-ticker.C:
		}
	}

	ok, err := fl.flock.TryLockContext(ctx, 10*time.Millisecond)
	if err != nil || !ok {
		fl.mu.Unlock()
		if err == nil {
			err = fmt.Errorf("failed to acquire file lock within %v", timeout)
		}
		return nil, err
	}

	fl.lockCount = 1

	return func() {
		if fl.lockCount == 0 {
			return
		}

		fl.lockCount = 0
		_ = fl.flock.Unlock()
		_ = os.Remove(fl.path)
		fl.mu.Unlock()
	}, nil
}

// LockWithRetry retries TryLock up to maxRetries times, waiting retryDelay
// between attempts, and returns an idempotent unlock function on success.
func (fl *FileLocker) LockWithRetry(maxRetries int, retryDelay time.Duration) (func(), error) {
	for i := range maxRetries {
		unlock, err := fl.TryLock(retryDelay)
		if err == nil {
			return unlock, nil
		}
		if i < maxRetries-1 {
			time.Sleep(retryDelay)
		}
	}
	return nil, fmt.Errorf("failed to acquire lock after %d retries", maxRetries)
}
