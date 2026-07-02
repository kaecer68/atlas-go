package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// dangerousRoots are absolute paths that should never be granted to
// mcp_roots_read_file. Misconfiguring these would expose system internals
// (/etc/passwd, kernel state, etc.) to the connected MCP client.
//
// Scope: matches the audit's stated concern (issue #870/903 — "grant /etc
// or / access"). Conservative additions for system state. Does NOT
// include /var, /usr, /opt, /lib, /private — these are either legitimate
// user-app dirs (e.g. /var/folders/... on macOS for temp dirs) or aliases
// (e.g. /private on macOS is a transparent wrapper over /etc, /var, /tmp).
// The checkDangerousPath helper unwraps /private/ before matching so
// /private/etc is caught without blocking /private/var/folders.
var dangerousRoots = map[string]bool{
	"/":     true,
	"/etc":  true,
	"/proc": true,
	"/sys":  true,
	"/boot": true,
	"/dev":  true,
	"/root": true,
	"/sbin": true,
}

// validateAllowedRoots checks that every entry in roots is suitable for
// granting file-read access via mcp_roots_read_file. It accumulates all
// failures (does not short-circuit) so the operator sees every problem
// in one go.
//
// Set ATLAS_MCP_ROOTS_ALLOW_UNSAFE=1 to bypass validation entirely. This
// is intended for advanced power users who understand the implications;
// the audit (issue #870, #903) flagged the missing check as a real risk.
func validateAllowedRoots(roots []string) error {
	if os.Getenv("ATLAS_MCP_ROOTS_ALLOW_UNSAFE") == "1" {
		return nil
	}
	if len(roots) == 0 {
		return nil
	}
	var errs []string
	for _, raw := range roots {
		if err := validateOneAllowedRoot(raw); err != nil {
			errs = append(errs, fmt.Sprintf("%q: %s", raw, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("allowed_roots contains %d invalid entr%s:\n  - %s",
			len(errs), plural(len(errs)),
			strings.Join(errs, "\n  - "))
	}
	return nil
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// checkDangerousPath returns a non-nil error if path is, or is contained
// within, a protected system root. Used twice in validateOneAllowedRoot:
// once on the raw user-supplied path (catches explicit /etc even when it
// is a symlink to /private/etc on macOS), and once on the symlink-resolved
// target (catches a safe-looking name that symlinks to /etc).
//
// macOS alias handling: /private is a transparent wrapper over /etc, /var,
// /tmp. We recursively check the unwrapped path so /private/etc is caught
// without blocking /private/var/folders (a legitimate temp dir).
func checkDangerousPath(path string) error {
	if dangerousRoots[path] {
		return fmt.Errorf("path is a protected system root (%s); refusing to grant filesystem access", path)
	}
	if unwrapped, ok := strings.CutPrefix(path, "/private"); ok {
		if err := checkDangerousPath(unwrapped); err != nil {
			return err
		}
	}
	sep := string(filepath.Separator)
	for sys := range dangerousRoots {
		if strings.HasPrefix(path+sep, sys+sep) {
			return fmt.Errorf("path is under protected system root %s; refusing to grant filesystem access", sys)
		}
	}
	return nil
}

func validateOneAllowedRoot(raw string) error {
	if raw == "" {
		return fmt.Errorf("empty path")
	}
	if !filepath.IsAbs(raw) {
		return fmt.Errorf("not an absolute path (relative paths are ambiguous and refused)")
	}
	cleaned := filepath.Clean(raw)

	// Reject dangerous roots on the user-supplied path BEFORE resolving
	// symlinks. This catches explicit /etc even when the kernel resolves
	// it to /private/etc on macOS.
	if err := checkDangerousPath(cleaned); err != nil {
		return err
	}

	// Resolve symlinks to catch a safe-looking name that symlinks into a
	// dangerous root. EvalSymlinks errors on non-existent paths — we
	// allow those (mount-later use case) and trust the read call to fail
	// later if the path still doesn't exist.
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("cannot resolve symlinks: %w", err)
	}
	resolved = filepath.Clean(resolved)
	if err := checkDangerousPath(resolved); err != nil {
		return err
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory (file roots are not supported by mcp_roots_read_file)")
	}
	return nil
}
