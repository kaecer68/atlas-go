//go:build unix

package server

import (
	"os"
	"syscall"
)

// readFileNoFollow opens path for reading without following a symlink at the
// leaf. Closes the TOCTOU window between filepath.EvalSymlinks and os.OpenFile
// in cmd/atlas-mcp/server/roots.go's handleMCPRootsReadFile.
//
// Requires a Unix-like kernel (Linux/macOS/BSD). Windows builds fall back to
// plain os.OpenFile in roots_open_other.go, which still follows symlinks —
// accepted trade-off since the production target is Unix.
func readFileNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
}
