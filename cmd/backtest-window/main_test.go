package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunBacktestWindow(t *testing.T) {
	origReplay := os.Getenv("ATLAS_REPLAY_DATA_PATH")
	origLedger := os.Getenv("ATLAS_LEDGER_DIR")
	defer func() {
		os.Setenv("ATLAS_REPLAY_DATA_PATH", origReplay)
		os.Setenv("ATLAS_LEDGER_DIR", origLedger)
	}()

	// Use sample replay data
	os.Setenv("ATLAS_REPLAY_DATA_PATH", filepath.Join("..", "..", "samples", "replay", "twse_stock_day_all_sample.csv"))
	dir := t.TempDir()
	os.Setenv("ATLAS_LEDGER_DIR", dir)

	if err := run([]string{"-start", "2026-03-26", "-end", "2026-03-27"}); err != nil {
		t.Fatalf("run backtest-window: %v", err)
	}
}

func TestRunRejectsInvalidDate(t *testing.T) {
	if err := run([]string{"-start", "invalid", "-end", "2026-03-27"}); err == nil {
		t.Fatalf("expected error for invalid start date")
	}
}
