//go:build !unix

package server

import "os"

// readFileNoFollow is the non-Unix fallback for cmd/atlas-mcp/server/roots_open_unix.go.
// Windows does not expose O_NOFOLLOW through Go's syscall package; the leaf symlink
// TOCTOU window remains open on Windows. Production target is Unix-like systems.
func readFileNoFollow(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY, 0)
}
