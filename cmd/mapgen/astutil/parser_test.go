package astutil

import (
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindRepoRoot(t *testing.T) {
	// Try multiple starting points since test working dir varies by runner.
	var root string
	var err error
	for _, start := range []string{".", "..", "../..", "../../..", "../../../.."} {
		root, err = FindRepoRoot(start)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("FindRepoRoot: all attempts failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Errorf("FindRepoRoot returned %s, but go.mod not found: %v", root, err)
	}
}

func TestFindRepoRoot_nomod(t *testing.T) {
	_, err := FindRepoRoot("/tmp")
	if err == nil {
		t.Error("expected error for /tmp (no go.mod), got nil")
	}
}

func TestGetAllGoFiles(t *testing.T) {
	// Use a temp directory with known files to test reliably.
	tmpDir := t.TempDir()
	os.WriteFile(tmpDir+"/foo.go", []byte("package foo"), 0o644)
	os.WriteFile(tmpDir+"/foo_test.go", []byte("package foo"), 0o644)
	os.WriteFile(tmpDir+"/bar.go", []byte("package bar"), 0o644)
	os.WriteFile(tmpDir+"/data.txt", []byte("data"), 0o644)
	os.Mkdir(tmpDir+"/sub", 0o755)
	os.WriteFile(tmpDir+"/sub/baz.go", []byte("package baz"), 0o644)
	os.WriteFile(tmpDir+"/sub/baz_test.go", []byte("package baz"), 0o644)

	files, err := GetAllGoFiles(tmpDir)
	if err != nil {
		t.Fatalf("GetAllGoFiles: %v", err)
	}

	// Should find foo.go, bar.go, sub/baz.go (3 non-test .go files)
	if len(files) != 3 {
		t.Errorf("expected 3 files, got %d: %v", len(files), files)
	}

	// No test files should be returned.
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			t.Errorf("test file returned: %s", f)
		}
	}
}

func TestGetAllGoFiles_nonexistent(t *testing.T) {
	_, err := GetAllGoFiles("/nonexistent/path")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestParseGoFile(t *testing.T) {
	fset := token.NewFileSet()
	f := ParseGoFile(fset, "parser.go")
	if f == nil {
		t.Fatal("ParseGoFile returned nil for parser.go")
	}
	if f.Name == nil {
		t.Error("parsed file has nil name")
	}
}

func TestParseGoFile_nonexistent(t *testing.T) {
	// Suppress stderr to prevent GitHub Actions ##[error] annotation on parse failures.
	realStderr := os.Stderr
	devNull, openErr := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if openErr == nil {
		os.Stderr = devNull
		defer func() {
			os.Stderr = realStderr
			devNull.Close()
		}()
	}
	fset := token.NewFileSet()
	f := ParseGoFile(fset, "nonexistent.go")
	if f != nil {
		t.Error("expected nil for nonexistent file")
	}
}

func TestCountLines(t *testing.T) {
	n := CountLines("parser.go")
	if n <= 0 {
		t.Errorf("expected positive line count, got %d", n)
	}
}

func TestCountLines_nonexistent(t *testing.T) {
	n := CountLines("/nonexistent")
	if n != 0 {
		t.Errorf("expected 0 for nonexistent, got %d", n)
	}
}

func TestPackagePath(t *testing.T) {
	got := PackagePath("/repo", "/repo/internal/foo")
	expected := "github.com/kaecer68/atlas-go/internal/foo"
	if got != expected {
		t.Errorf("expected %s, got %s", expected, got)
	}
}

func TestPackagePath_noRel(t *testing.T) {
	// When Rel fails, the function returns the original path.
	got := PackagePath("/repo", "/other")
	// filepath.Rel returns "../other" which becomes "github.com/kaecer68/atlas-go/../other"
	if got == "" {
		t.Error("expected non-empty path")
	}
}
