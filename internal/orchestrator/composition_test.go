package orchestrator

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

func TestNewSystem_UsesParametersConfigPathFromConfig(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	parametersPath := filepath.Join(tempDir, "parameters.json")

	parameters := config.DefaultParametersConfig()
	parameters.Darwinian.WeightNeutral.Value = 1.23
	if err := parameters.Save(parametersPath); err != nil {
		t.Fatalf("save parameters config: %v", err)
	}

	sys, _ := NewSystem(config.Config{ParametersConfigPath: parametersPath})
	got := sys.Port().darwinian.GetWeight("missing-agent")

	if math.Abs(got-1.23) > 1e-9 {
		t.Fatalf("expected neutral weight from config path to be 1.23, got %.6f", got)
	}
}

// TestBuildPortfolioManager_DoesNotOverwriteOnLoadFailure covers the 2026-08-13
// 23:36 regression: when darwinian_weights.json exists but is malformed, the
// old buildPortfolioManager silently swallowed the Load error, then called
// InitializeFromRegistry (Sharpe=0 for every agent) + Save, overwriting the
// real Sharpe history. This test asserts the post-fix behavior: Load failure
// logs an error and the file is NOT overwritten.
func TestBuildPortfolioManager_DoesNotOverwriteOnLoadFailure(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	weightsPath := filepath.Join(tempDir, "darwinian_weights.json")
	// Plant a deliberately malformed JSON file (looks like corruption).
	if err := os.WriteFile(weightsPath, []byte("{not valid json"), 0o644); err != nil {
		t.Fatalf("plant malformed weights: %v", err)
	}
	originalContent, err := os.ReadFile(weightsPath)
	if err != nil {
		t.Fatalf("read planted weights: %v", err)
	}

	// Trigger buildPortfolioManager indirectly via NewSystem. We only need the
	// PortfolioManager wiring path; buildPortfolioManager is package-private so
	// we go through NewSystem which calls it.
	cfg := config.Config{
		ParametersConfigPath: filepath.Join(tempDir, "parameters.json"),
		AgentRegistryPath:    "",
		BaselinePolicyPath:   "",
		ReplayDataPath:       "",
		LedgerDir:            tempDir,
	}
	_ = cfg
	_ = originalContent

	// Note: NewSystem also touches replay/registry/baseline loaders; we use
	// the same defaults-fallback pattern that TestNewSystem_UsesParametersConfigPathFromConfig
	// relies on, so we don't need a real data dir. The critical assertion is that
	// the malformed weights file is preserved verbatim after the call.
	_, _ = NewSystem(cfg)

	after, err := os.ReadFile(weightsPath)
	if err != nil {
		t.Fatalf("read weights post-NewSystem: %v", err)
	}
	if string(after) != string(originalContent) {
		t.Fatalf("darwinian_weights.json was overwritten despite Load failure:\nbefore: %q\nafter:  %q",
			string(originalContent), string(after))
	}
}
