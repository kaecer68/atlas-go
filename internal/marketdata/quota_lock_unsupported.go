//go:build !unix

package marketdata

import (
	"errors"
	"os"
)

// errLockUnsupported is returned on platforms without flock. The tracker fails
// closed on such platforms (see DailyQuotaTracker): advertising a cross-process
// ceiling that cannot actually be enforced is the defect #2014 fixed, so the
// honest answer is "cannot guarantee, therefore do not spend".
var errLockUnsupported = errors.New("advisory file locking (flock) is not available on this platform")

func tryLockFile(*os.File) (bool, error) { return false, errLockUnsupported }

func unlockFile(*os.File) error { return nil }
