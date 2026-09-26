//go:build unix

package marketdata

import (
	"errors"
	"os"
	"syscall"
)

// tryLockFile attempts a non-blocking exclusive (flock) lock on f. It reports
// (true, nil) when the lock was acquired, (false, nil) when another process
// currently holds it, and (false, err) for real failures.
//
// Non-blocking + a bounded retry loop (see quotaLockTimeout) is deliberate:
// flock has no timeout of its own, so a holder that is alive but wedged —
// paused container, stalled write, blocked stderr — would otherwise block
// every caller forever. flock is also NOT recursive across file descriptors,
// so a process must never take this lock twice on the same file.
//
// The lock dies with the process (or with the file descriptor), so a crashed
// container cannot wedge the counter.
func tryLockFile(f *os.File) (bool, error) {
	err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return false, nil
	}
	return false, err
}

// unlockFile releases a lock taken by tryLockFile.
func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
