package main

import (
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

// TestBuildPrismTrainingExecutor_WiresReplayBackedExecutor verifies the
// PRISM Phase A wiring: when a replay dataset is resolvable, the executor is
// non-nil and carries a non-empty agent registry (so the prism_training BTM
// can schedule real per-agent training and produce real cohort metrics).
func TestBuildPrismTrainingExecutor_WiresReplayBackedExecutor(t *testing.T) {
	sample, err := filepath.Abs(filepath.Join("..", "..", "samples", "replay", "twse_stock_day_all_sample.csv"))
	if err != nil {
		t.Fatalf("abs sample path: %v", err)
	}
	t.Setenv("ATLAS_REPLAY_DATA_PATH", sample)

	cfg := config.Config{
		WorkDir:           ".",
		AgentRegistryPath: filepath.Join("..", "..", "configs", "agents.json"),
	}

	exec, registry := buildPrismTrainingExecutor(cfg)
	if exec == nil {
		t.Fatal("expected non-nil executor when a replay dataset is loadable (PRISM Phase A wiring)")
	}
	if len(registry.Agents) == 0 {
		t.Fatal("expected non-empty agent registry to schedule per-agent training")
	}
}

// TestBuildPrismTrainingExecutor_NilWhenDatasetMissing verifies the
// Synthetic fallback stays intact: when the replay dataset cannot be loaded,
// the executor is nil (prism_training BTM falls back to Synthetic instead of
// surfacing per-task errors), while the registry is still resolved.
func TestBuildPrismTrainingExecutor_NilWhenDatasetMissing(t *testing.T) {
	t.Setenv("ATLAS_REPLAY_DATA_PATH", filepath.Join(t.TempDir(), "missing.csv"))

	cfg := config.Config{
		WorkDir:           ".",
		AgentRegistryPath: filepath.Join("..", "..", "configs", "agents.json"),
	}

	exec, registry := buildPrismTrainingExecutor(cfg)
	if exec != nil {
		t.Fatal("expected nil executor when replay dataset is missing (Synthetic fallback)")
	}
	if len(registry.Agents) == 0 {
		t.Fatal("expected fallback registry even when replay dataset is missing")
	}
}

// TestBuildPrismTrainingExecutor_PrefersEnvPath verifies the executor uses
// the ATLAS_REPLAY_DATA_PATH env override (config.GetReplayDataPath priority
// #1), matching the production iMac setting
// ATLAS_REPLAY_DATA_PATH=data/replay/tw_extended_90days.csv.
func TestBuildPrismTrainingExecutor_PrefersEnvPath(t *testing.T) {
	sample, err := filepath.Abs(filepath.Join("..", "..", "samples", "replay", "twse_stock_day_all_sample.csv"))
	if err != nil {
		t.Fatalf("abs sample path: %v", err)
	}
	t.Setenv("ATLAS_REPLAY_DATA_PATH", sample)

	// A VERSION-file / default path would resolve to the extended CSV (which
	// is not checked into the worktree); the env override must win so the
	// executor loads the provided dataset.
	cfg := config.Config{
		WorkDir:           ".",
		AgentRegistryPath: filepath.Join("..", "..", "configs", "agents.json"),
		ReplayDataPath:    "unused/fallback.csv",
	}

	exec, _ := buildPrismTrainingExecutor(cfg)
	if exec == nil {
		t.Fatal("expected non-nil executor when ATLAS_REPLAY_DATA_PATH is set")
	}
}
