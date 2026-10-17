package reporoot_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/testutil/reporoot"
)

// writeModule lays out <root>/go.mod plus the nested directories, all inside a
// t.TempDir(), so every case here is hermetic (no dependency on the real
// checkout layout).
func writeModule(t *testing.T, nested ...string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/fixture\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	for _, rel := range nested {
		if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
	}
	return root
}

func TestFind_FromNestedDirectory(t *testing.T) {
	root := writeModule(t, "a/b/c")
	got, err := reporoot.Find(filepath.Join(root, "a", "b", "c"))
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if want := filepath.Clean(root); got != want {
		t.Errorf("Find(nested) = %s, want %s", got, want)
	}
}

func TestFind_FromRootItself(t *testing.T) {
	root := writeModule(t, "a")
	got, err := reporoot.Find(root)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if want := filepath.Clean(root); got != want {
		t.Errorf("Find(root) = %s, want %s", got, want)
	}
}

func TestFind_NearestModuleWins(t *testing.T) {
	// The inner module must win: a repository may vendor a module that itself
	// carries a go.mod, and the walk must stop at the closest one.
	outer := writeModule(t, "inner/pkg")
	inner := filepath.Join(outer, "inner")
	if err := os.WriteFile(filepath.Join(inner, "go.mod"), []byte("module example.test/inner\n"), 0o644); err != nil {
		t.Fatalf("write inner go.mod: %v", err)
	}
	got, err := reporoot.Find(filepath.Join(inner, "pkg"))
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got != filepath.Clean(inner) {
		t.Errorf("Find(inner/pkg) = %s, want the nearest module %s", got, filepath.Clean(inner))
	}
}

func TestFind_NotFoundNamesTheSearchedPath(t *testing.T) {
	// t.TempDir() has no go.mod in it or above it (the OS temp tree is not a
	// module), so the walk must fail and say where it looked.
	dir := t.TempDir()
	_, err := reporoot.Find(dir)
	if err == nil {
		t.Fatal("Find on a directory with no go.mod above it returned nil error")
	}
	if !errors.Is(err, reporoot.ErrNotFound) {
		t.Errorf("error does not wrap ErrNotFound: %v", err)
	}
	if !strings.Contains(err.Error(), dir) {
		t.Errorf("error message must name the searched path %q, got: %v", dir, err)
	}
}

func TestFromCwd_FollowsChdir(t *testing.T) {
	root := writeModule(t, "a/b")
	nested := filepath.Join(root, "a", "b")
	t.Chdir(nested)
	got, err := reporoot.FromCwd()
	if err != nil {
		t.Fatalf("FromCwd: %v", err)
	}
	if want := filepath.Clean(root); got != want {
		t.Errorf("FromCwd from %s = %s, want %s", nested, got, want)
	}
}

func TestRequireEntries(t *testing.T) {
	root := writeModule(t, "configs")
	if err := reporoot.RequireEntries(root, "go.mod", "configs"); err != nil {
		t.Errorf("RequireEntries on existing entries = %v, want nil", err)
	}
	err := reporoot.RequireEntries(root, "configs/parameters.json")
	if err == nil {
		t.Fatal("RequireEntries on a missing entry returned nil error")
	}
	// The message must name the resolved root, otherwise a wrong-root bug reads
	// like a missing-file bug (the CI symptom this package removes).
	if !strings.Contains(err.Error(), filepath.Clean(root)) {
		t.Errorf("error must name the resolved root %q, got: %v", root, err)
	}
	if !strings.Contains(err.Error(), "configs/parameters.json") {
		t.Errorf("error must name the missing entry, got: %v", err)
	}
}
