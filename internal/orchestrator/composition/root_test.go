package composition

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/narrative"
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

func TestRoot_WithNarrativeEngine(t *testing.T) {
	cfg := config.Config{}
	root, err := NewRoot(cfg)
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}

	eng := narrative.NewNarrativeEngine()
	root2 := root.WithNarrativeEngine(eng)
	if root2 != root {
		t.Fatal("WithNarrativeEngine should return same instance")
	}
	if root.narrativeEngine != eng {
		t.Fatal("narrativeEngine not set on root")
	}

	called := false
	root.WithNarrativeDataFn(func() narrative.MarketNarrativeData {
		called = true
		return narrative.MarketNarrativeData{}
	})
	if root.narrativeDataFn == nil {
		t.Fatal("narrativeDataFn not set on root")
	}
	root.narrativeDataFn()
	if !called {
		t.Fatal("narrativeDataFn did not fire")
	}
}

// TestBuildWeightEngine_NilEngineFallback verifies buildWeightEngine
// constructs a functional engine even when no narrative engine was wired
// (test fallback path) — and that the hardcoded theme→bias maps are gone
// (single source of truth = SectorBias over active models).
func TestBuildWeightEngine_NilEngineFallback(t *testing.T) {
	cfg := config.Config{}
	root, err := NewRoot(cfg)
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}

	eng := root.buildWeightEngine()
	if eng == nil {
		t.Fatal("buildWeightEngine returned nil with nil narrative engine")
	}

	// Regression guard: the inline narrativeThemeMap and its pseudo-theme
	// keys must never come back — SectorBias only knows real detector
	// themes, so any inline map literal re-introduces the split-brain.
	src, err := os.ReadFile("root.go")
	if err != nil {
		t.Fatalf("read root.go: %v", err)
	}
	if bytes.Contains(src, []byte("narrativeThemeMap")) {
		t.Fatal("hardcoded narrativeThemeMap re-introduced in root.go")
	}
	for _, pseudo := range []string{"USD_strengther", "USD_weaker", `"risk_on"`, `"risk_off"`} {
		if bytes.Contains(src, []byte(pseudo)) {
			t.Fatalf("hardcoded pseudo-theme %s leaked into root.go", pseudo)
		}
	}
}

// TestBuildWeightEngine_SectorBiasDrivesNarrativeAdapter verifies the
// production wiring path: an injected engine + real narrative data fn
// produce a multiplier > 1 (favored sector) from model-derived bias.
func TestBuildWeightEngine_SectorBiasDrivesNarrativeAdapter(t *testing.T) {
	cfg := config.Config{}
	root, err := NewRoot(cfg)
	if err != nil {
		t.Fatalf("NewRoot: %v", err)
	}

	eng := narrative.NewNarrativeEngine()
	root.WithNarrativeEngine(eng)
	root.WithNarrativeDataFn(func() narrative.MarketNarrativeData {
		// Trigger AI_capex_surge detector: ai_supercycle_model favors
		// semiconductor (+), avoids consumer (−).
		return narrative.MarketNarrativeData{AICapexSentiment: 0.9}
	})

	w := root.buildWeightEngine()
	if w == nil {
		t.Fatal("buildWeightEngine returned nil")
	}
	sw, err := w.ComputeWeight(context.Background(), "semiconductor", time.Now())
	if err != nil {
		t.Fatalf("ComputeWeight: %v", err)
	}
	narrativeLog, ok := findLogEntry(sw.AdjustmentLog, "narrative")
	if !ok {
		t.Fatalf("expected narrative entry in adjustment log, got %v", sw.AdjustmentLog)
	}
	if narrativeLog <= 1.0 {
		t.Fatalf("expected narrative multiplier > 1 for favored semiconductor, got %f", narrativeLog)
	}
}

// findLogEntry extracts a "name=value" entry from an adjustment log.
func findLogEntry(log []string, name string) (float64, bool) {
	for _, l := range log {
		if strings.HasPrefix(l, name+"=") {
			var v float64
			if _, err := fmt.Sscanf(strings.TrimPrefix(l, name+"="), "%f", &v); err == nil {
				return v, true
			}
		}
	}
	return 0, false
}
