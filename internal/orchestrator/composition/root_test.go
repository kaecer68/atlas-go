package composition

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

func TestCompositionPath_AllowsSectorRotation(t *testing.T) {
	tests := []struct {
		path     CompositionPath
		expected bool
	}{
		{PathAdminManual, true},
		{PathAutoDaily, true},
		{PathStressTestDaily, true},
		{PathCLISimulation, true},
		{PathAutoExperiment, false},
		{PathLiveTrading, false},
	}

	for _, tt := range tests {
		got := tt.path.AllowsSectorRotation()
		if got != tt.expected {
			t.Errorf("%s.AllowsSectorRotation() = %v, want %v", tt.path, got, tt.expected)
		}
	}
}

func TestCompositionPath_String(t *testing.T) {
	// Basic sanity: six distinct values.
	paths := []CompositionPath{
		PathAdminManual, PathAutoDaily, PathStressTestDaily,
		PathCLISimulation, PathAutoExperiment, PathLiveTrading,
	}
	seen := map[string]bool{}
	for _, p := range paths {
		s := string(p)
		if seen[s] {
			t.Errorf("duplicate path: %s", s)
		}
		seen[s] = true
		if s == "" {
			t.Errorf("empty string for path value: %q", p)
		}
	}
}

func TestRoot_NewRoot(t *testing.T) {
	cfg := config.Config{WorkDir: "/tmp/atlas-test", LedgerDir: "/tmp/atlas-test/ledger"}
	root, err := NewRoot(cfg)
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}
	if root == nil {
		t.Fatal("root is nil")
	}
	if root.Mapper == nil {
		t.Fatal("Mapper is nil")
	}
	if root.Calc == nil {
		t.Fatal("Calc is nil")
	}
	if root.WeightEngine() != nil {
		t.Fatal("WeightEngine should be nil before wiring")
	}
	if root.Cfg.WorkDir != cfg.WorkDir {
		t.Errorf("WorkDir mismatch: got %q, want %q", root.Cfg.WorkDir, cfg.WorkDir)
	}
}

func TestRoot_WithWeightEngine(t *testing.T) {
	cfg := config.Config{}
	root, err := NewRoot(cfg)
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}

	want := "wired" // dummy placeholder — real engine requires providers
	_ = want

	// Verify the setter chain works
	root2 := root.WithWeightEngine(nil) // nil engine is accepted (for cleanup/disable)
	if root2 != root {
		t.Fatal("WithWeightEngine should return same instance")
	}
	if root.WeightEngine() != nil {
		t.Fatal("WeightEngine should be nil after setting nil")
	}
}
