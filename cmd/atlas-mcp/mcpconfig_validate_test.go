package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateAllowedRoots_EmptyList_NoError(t *testing.T) {
	if err := validateAllowedRoots(nil); err != nil {
		t.Fatalf("nil roots must pass: %v", err)
	}
	if err := validateAllowedRoots([]string{}); err != nil {
		t.Fatalf("empty roots must pass: %v", err)
	}
}

func TestValidateAllowedRoots_ValidPaths_NoError(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []string{root, sub, filepath.Join(root, "deeper", "still-ok")}
	if err := validateAllowedRoots(cases); err != nil {
		t.Fatalf("valid tempdir paths must pass: %v", err)
	}
}

func TestValidateAllowedRoots_AcceptsNonExistent(t *testing.T) {
	// An admin may declare a mount point that will be created later.
	// Validation should allow non-existent absolute paths; the read
	// will fail at request time if still missing.
	nonExistent := filepath.Join(t.TempDir(), "does-not-exist")
	if err := validateAllowedRoots([]string{nonExistent}); err != nil {
		t.Fatalf("non-existent path must be allowed (mount-later use case): %v", err)
	}
}

func TestValidateAllowedRoots_RejectsRelativePath(t *testing.T) {
	err := validateAllowedRoots([]string{"relative/path"})
	if err == nil {
		t.Fatal("relative path must be rejected")
	}
	if !strings.Contains(err.Error(), "not an absolute path") {
		t.Errorf("error should mention absolute path, got: %v", err)
	}
}

func TestValidateAllowedRoots_RejectsRootSlash(t *testing.T) {
	err := validateAllowedRoots([]string{"/"})
	if err == nil {
		t.Fatal("'/' must be rejected")
	}
	if !strings.Contains(err.Error(), "protected system root") {
		t.Errorf("error should mention protected system root, got: %v", err)
	}
}

func TestValidateAllowedRoots_RejectsSystemPaths(t *testing.T) {
	systemPaths := []string{"/etc", "/proc", "/sys", "/boot", "/dev", "/root", "/sbin"}
	for _, p := range systemPaths {
		err := validateAllowedRoots([]string{p})
		if err == nil {
			t.Errorf("%s must be rejected as dangerous root", p)
			continue
		}
		if !strings.Contains(err.Error(), "protected system root") {
			t.Errorf("%s: error should mention protected system root, got: %v", p, err)
		}
	}
}

func TestValidateAllowedRoots_RejectsSubpathOfSystem(t *testing.T) {
	err := validateAllowedRoots([]string{"/etc/passwd", "/proc/1/status", "/sys/kernel"})
	if err == nil {
		t.Fatal("subpath of system dir must be rejected")
	}
	if !strings.Contains(err.Error(), "protected system root") {
		t.Errorf("error should mention protected system root, got: %v", err)
	}
}

func TestValidateAllowedRoots_RejectsRegularFile(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "afile.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := validateAllowedRoots([]string{filePath})
	if err == nil {
		t.Fatal("regular file must be rejected (mcp_roots_read_file requires directories)")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("error should mention not a directory, got: %v", err)
	}
}

func TestValidateAllowedRoots_RejectsSymlinkToSystem(t *testing.T) {
	// An attacker places a symlink in a safe-looking root that points
	// at /etc. Without EvalSymlinks resolution, validation would let it
	// pass. We require the resolved target to be checked.
	dir := t.TempDir()
	link := filepath.Join(dir, "escape")
	if err := os.Symlink("/etc", link); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}
	err := validateAllowedRoots([]string{link})
	if err == nil {
		t.Fatal("symlink to /etc must be rejected (resolved target is protected)")
	}
	if !strings.Contains(err.Error(), "protected system root") {
		t.Errorf("error should mention protected system root, got: %v", err)
	}
}

func TestValidateAllowedRoots_AcceptsSymlinkToSafeDir(t *testing.T) {
	// A symlink in tempdir pointing to another tempdir (both safe) is OK.
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	link := filepath.Join(dir1, "link")
	if err := os.Symlink(dir2, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	if err := validateAllowedRoots([]string{link}); err != nil {
		t.Errorf("symlink to safe dir should pass: %v", err)
	}
}

func TestValidateAllowedRoots_AccumulatesErrors(t *testing.T) {
	err := validateAllowedRoots([]string{"/", "/etc", "relative"})
	if err == nil {
		t.Fatal("multiple bad entries must produce one error")
	}
	// Expect the error message to mention all three
	for _, want := range []string{`"/"`, `"/etc"`, `"relative"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %s, got: %v", want, err)
		}
	}
}

func TestValidateAllowedRoots_RespectsAllowUnsafeEnv(t *testing.T) {
	t.Setenv("ATLAS_MCP_ROOTS_ALLOW_UNSAFE", "1")
	// All these would normally be rejected, but the env flag bypasses.
	if err := validateAllowedRoots([]string{"/", "/etc", "relative"}); err != nil {
		t.Fatalf("ALLOW_UNSAFE must bypass validation, got: %v", err)
	}
}

func TestResolveRootsConfig_RejectsDangerousMergedConfig(t *testing.T) {
	// End-to-end: write a parameters.json with /etc in mcp.roots,
	// confirm resolveRootsConfig() returns an error.
	dir := t.TempDir()
	paramsPath := filepath.Join(dir, "params.json")
	body := `{"mcp":{"roots":{"allowed_roots":["/etc"]}}}`
	if err := os.WriteFile(paramsPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ATLAS_MCP_PARAMS", paramsPath)
	t.Setenv("ATLAS_MCP_ROOTS_ALLOWED", "")

	// Unset allow-unsafe to make sure validation is on.
	t.Setenv("ATLAS_MCP_ROOTS_ALLOW_UNSAFE", "")

	_, err := resolveRootsConfig()
	if err == nil {
		t.Fatal("resolveRootsConfig must error on /etc in allowed_roots")
	}
	if !strings.Contains(err.Error(), "/etc") {
		t.Errorf("error should mention /etc, got: %v", err)
	}
}
