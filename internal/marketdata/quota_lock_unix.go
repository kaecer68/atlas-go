//go:build unix

package marketdata

import (
	"os"
	"syscall"
)

// lockFileExclusive takes an exclusive advisory (flock) lock on f, blocking
// until it is granted. It is what makes the daily quota counter a cross-process
// total instead of a per-process guess (issue #2014): every atlas container
// sharing one state directory serialises its read-modify-write here.
//
// The lock dies with the process (or with the file descriptor), so a crashed
// container cannot wedge the counter.
func lockFileExclusive(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

// unlockFile releases a lock taken by lockFileExclusive.
func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
