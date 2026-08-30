package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveSeasonalReplayPath verifies the seasonal_calibration
// registration guard (#1757): the replay dataset path resolves only when
// finmind_2020_2024.jsonl exists under workDir/data/replay. An empty result
// means the task must be skipped — calibrate-seasonal hard-refuses -update
// without real replay data, so registering would guarantee a task_failed
// every 7d tick.
func TestResolveSeasonalReplayPath(t *testing.T) {
	t.Run("missing dataset returns empty", func(t *testing.T) {
		dir := t.TempDir()
		if got := resolveSeasonalReplayPath(dir); got != "" {
			t.Errorf("resolveSeasonalReplayPath(%q) = %q, want empty", dir, got)
		}
	})

	t.Run("empty replay dir returns empty", func(t *testing.T) {
		dir := t.TempDir()
		replayDir := filepath.Join(dir, "data", "replay")
		if err := os.MkdirAll(replayDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if got := resolveSeasonalReplayPath(dir); got != "" {
			t.Errorf("resolveSeasonalReplayPath(%q) = %q, want empty", dir, got)
		}
	})

	t.Run("existing dataset returns absolute path", func(t *testing.T) {
		dir := t.TempDir()
		replayDir := filepath.Join(dir, "data", "replay")
		if err := os.MkdirAll(replayDir, 0o755); err != nil {
			t.Fatal(err)
		}
		dataset := filepath.Join(replayDir, "finmind_2020_2024.jsonl")
		if err := os.WriteFile(dataset, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		got := resolveSeasonalReplayPath(dir)
		if got == "" {
			t.Fatalf("resolveSeasonalReplayPath(%q) = empty, want %q", dir, dataset)
		}
		if want, err := filepath.Abs(got); err == nil && want != got {
			t.Errorf("path not absolute-normalized: %q", got)
		}
		if filepath.Base(got) != "finmind_2020_2024.jsonl" {
			t.Errorf("unexpected dataset filename: %q", got)
		}
	})
}
