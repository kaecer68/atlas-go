package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// tempDir creates a per-test temp directory whose cleanup tolerates
// leftover files written by background simulation goroutines.
//
// Background: runSimulation returns immediately when shutdown fires
// (production reaps the goroutine at process exit), but the background
// RunDailySimulation goroutine keeps writing session/trace files into
// cfg.LedgerDir after the test ends — there is no System cancel mechanism
// to wait on. t.TempDir() treats a non-empty RemoveAll as a test failure
// ("directory not empty: unlinkat"), which made simulation tests flaky on
// slow runners. Best-effort removal (with a settle beat) never fails the
// test; a leaked goroutine is reaped at process exit.
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "atlas-"+strings.ReplaceAll(t.Name(), "/", "_")+"-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() {
		// Give background simulation goroutines a beat to settle before
		// best-effort removal.
		time.Sleep(200 * time.Millisecond)
		_ = os.RemoveAll(dir)
	})
	return dir
}
