// Package main implements the staging smoke drill for strategy_techniques
// plugin + seed loading flow.
//
// Phase 0: Pre-flight (isolated temp workspace)
// Phase 1: Boot-time seed loading (assert Count==12, all Validate() pass)
//
// Reference plan: .omo/plans/staging-smoke-strategy-techniques.md
package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/strategy_techniques"
)

// run executes Phase 0 (pre-flight) + Phase 1 (boot-time seed loading).
// It returns the loaded Registry for downstream phases to reuse.
func run() (*strategy_techniques.Registry, error) {
	// --- Phase 0: Pre-flight ---
	repoRoot, err := findRepoRoot()
	if err != nil {
		return nil, fmt.Errorf("locate repo root: %w", err)
	}
	productionSeedPath := filepath.Join(repoRoot, "data", "seeds", "strategy_techniques.json")

	// Isolate all writes to a temp dir (per cmd/experimental/AGENTS.md).
	tempDir, err := os.MkdirTemp("", "staging-drill-strategy-techniques-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Copy production seeds into the isolated temp dir.
	tempSeedPath := filepath.Join(tempDir, "strategy_techniques.json")
	if err := copyFile(productionSeedPath, tempSeedPath); err != nil {
		return nil, fmt.Errorf("copy production seeds: %w", err)
	}

	// --- Phase 1: Boot-time seed loading ---
	// LoadFromFile reads the JSON, unmarshals frames, and validates each.
	// An invalid frame causes LoadFromFile to return an error.
	reg, err := strategy_techniques.LoadFromFile(tempSeedPath)
	if err != nil {
		return nil, fmt.Errorf("load seeds: %w", err)
	}

	// Emit the boot log line that dashboards / Phase 1 assertions match on.
	logging.Default().Info("strategy_techniques_loaded",
		slog.String("path", tempSeedPath),
		slog.Int("count", reg.Count()),
	)

	return reg, nil
}

// findRepoRoot walks up from this file's directory until it finds go.mod.
// This makes run() resolve the production seed file regardless of the
// caller's working directory (notably when go test runs from the package dir).
func findRepoRoot() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("runtime.Caller failed")
	}
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", filepath.Dir(thisFile))
		}
		dir = parent
	}
}

// copyFile copies src to dst. Used to seed the isolated temp workspace
// from the read-only production seed file.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy %s -> %s: %w", src, dst, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close %s: %w", dst, err)
	}
	return nil
}

func runPhase2(reg *strategy_techniques.Registry, savePath string) (*orchestrator.System, error) {
	cfg := config.Load()
	cfg.BrokerMode = "paper"

	system, err := orchestrator.NewProductionSystem(cfg)
	if err != nil {
		return nil, fmt.Errorf("construct production system: %w", err)
	}

	system.WithStrategyTechniques(reg, savePath)
	return system, nil
}

func main() {
	reg, err := run()
	if err != nil {
		panic(err)
	}

	tempDir, err := os.MkdirTemp("", "staging-drill-strategy-techniques-main-*")
	if err != nil {
		panic(fmt.Errorf("create temp dir: %w", err))
	}
	defer os.RemoveAll(tempDir)
	savePath := filepath.Join(tempDir, "strategy_techniques_save.json")

	if _, err := runPhase2(reg, savePath); err != nil {
		panic(err)
	}
}
