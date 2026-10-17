// Package reporoot locates the Go module root from an arbitrary working
// directory.
//
// Why it exists (FU-20261016-20)
// -----------------------------
// Tests used to resolve the module root with a fixed relative depth:
// `filepath.Join(wd, "../..")`, `filepath.Abs("../..")`, `os.Chdir("../..")`.
// That encodes two hidden assumptions — the process always runs from exactly
// the package directory, and the package always sits exactly two levels below
// the module root. Both broke in practice: the CI failure
// `open /home/runner/work/configs/parameters.json: no such file or directory`
// came from resolving "../.." after a t.Chdir, and it was invisible on the
// developer machine because a lane worktree sits inside the main checkout, so
// the wrong answer still landed on a directory that had configs/parameters.json
// (local green, CI red).
//
// Walking up to go.mod answers the same question at any package depth, in a
// lane worktree, in the main checkout, and on a CI runner.
package reporoot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrNotFound is returned when the walk reaches the filesystem root without
// meeting a go.mod.
var ErrNotFound = errors.New("no go.mod found")

// Find walks up from startDir (inclusive) and returns the first directory that
// contains a go.mod, i.e. the module root.
//
// The error names both the starting directory and the topmost directory the
// walk reached, so a failure says WHICH path was searched instead of only
// reporting the file that could not be read later.
func Find(startDir string) (string, error) {
	abs, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolve %q to an absolute path: %w", startDir, err)
	}
	dir := abs
	for {
		if info, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil && !info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%w above %s (search stopped at %s)", ErrNotFound, abs, dir)
		}
		dir = parent
	}
}

// FromCwd is Find for the current working directory.
//
// Use it where the previous code read "../.." or filed the answer into a
// variable that was then treated as the module root.
func FromCwd() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return Find(wd)
}

// RequireEntries checks that the module root contains the given relative paths
// (files or directories). The error names the resolved root and the missing
// entry, so a test that fails because it resolved the WRONG directory says so
// instead of reporting a confusing "no such file" from the path it built.
func RequireEntries(root string, entries ...string) error {
	for _, entry := range entries {
		if _, err := os.Stat(filepath.Join(root, entry)); err != nil {
			return fmt.Errorf("module root resolved to %s, which has no %s: %w", root, entry, err)
		}
	}
	return nil
}
