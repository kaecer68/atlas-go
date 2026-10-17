package main

// Wiring tests for the ledger_archive background task (FU-20261012-05).
//
// The pre-fix shape: internal/ledger.Archiver existed but NewArchiver had zero
// call sites, so nothing in the running system ever archived a ledger file
// (grep -rn "NewArchiver" --include='*.go' . matched only archiver.go itself).
// These tests pin the lifecycle hook: the task is registered on the existing
// BackgroundTaskManager, honours the enable flag, and its body actually moves an
// aged ledger file into archive/.

import (
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

const taskFixtureRecords = "{\"line\":\"one\"}\n{\"line\":\"two\"}\n"

// writeAgedJSONL writes rel under dir with a backdated mtime.
func writeAgedJSONL(t *testing.T, dir, rel, content string, age time.Duration) string {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	stamp := time.Now().Add(-age)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatalf("chtimes %s: %v", rel, err)
	}
	return path
}

func TestLedgerArchiveDir_ResolvesRelativeToWorkDir(t *testing.T) {
	cfg := config.Config{WorkDir: "/tmp/atlas-work", LedgerDir: "data/state"}
	if got, want := ledgerArchiveDir(cfg), "/tmp/atlas-work/data/state"; got != want {
		t.Errorf("ledgerArchiveDir = %q, want %q", got, want)
	}
	abs := config.Config{WorkDir: "/tmp/atlas-work", LedgerDir: "/var/lib/atlas"}
	if got, want := ledgerArchiveDir(abs), "/var/lib/atlas"; got != want {
		t.Errorf("absolute ledger dir = %q, want %q", got, want)
	}
	empty := config.Config{WorkDir: "/tmp/atlas-work"}
	if got, want := ledgerArchiveDir(empty), "/tmp/atlas-work/data/state"; got != want {
		t.Errorf("default ledger dir = %q, want %q", got, want)
	}
}

func TestRegisterLedgerArchiveTask_GatedByConfig(t *testing.T) {
	mgr := apigateway.NewBackgroundTaskManager(nil)
	if registerLedgerArchiveTask(mgr, config.Config{}) {
		t.Fatal("ledger_archive must not be registered when LedgerArchiveEnabled is false")
	}
	if _, ok := mgr.Get("ledger_archive"); ok {
		t.Fatal("ledger_archive must not appear in the registry when disabled")
	}
}

func TestRegisterLedgerArchiveTask_Registered(t *testing.T) {
	mgr := apigateway.NewBackgroundTaskManager(nil)
	cfg := config.Config{
		WorkDir:                    t.TempDir(),
		LedgerDir:                  "data/state",
		LedgerArchiveEnabled:       true,
		LedgerArchiveRetentionDays: 30,
		LedgerArchiveMaxFileSizeMB: 100,
		LedgerArchiveFilesPerRun:   5,
		LedgerArchiveGlobs:         []string{"*.jsonl"},
	}
	if !registerLedgerArchiveTask(mgr, cfg) {
		t.Fatal("ledger_archive must be registered when enabled")
	}
	task, ok := mgr.Get("ledger_archive")
	if !ok {
		t.Fatal("ledger_archive missing from the registry")
	}
	if task.Interval != ledgerArchiveInterval {
		t.Errorf("interval = %v, want %v", task.Interval, ledgerArchiveInterval)
	}

	// The task body must archive an aged ledger file placed under the
	// configured ledger dir.
	ledgerDir := ledgerArchiveDir(cfg)
	writeAgedJSONL(t, ledgerDir, "metrics.jsonl", taskFixtureRecords, 200*24*time.Hour)
	if err := task.Task(context.Background()); err != nil {
		t.Fatalf("task body: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ledgerDir, "metrics.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("aged ledger file must be moved into archive/ (err=%v)", err)
	}
	entries, err := os.ReadDir(filepath.Join(ledgerDir, "archive"))
	if err != nil {
		t.Fatalf("read archive dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("archive dir holds %d entries, want 1", len(entries))
	}
}

// TestRunLedgerArchive_ArchivesAndKeepsContent drives the task body directly and
// verifies the archive is byte-identical to the moved file.
func TestRunLedgerArchive_ArchivesAndKeepsContent(t *testing.T) {
	dir := t.TempDir()
	writeAgedJSONL(t, dir, "recommendation_outcomes.jsonl", taskFixtureRecords, 400*24*time.Hour)
	fresh := writeAgedJSONL(t, dir, "quotes.jsonl", taskFixtureRecords, time.Minute)

	policy := ledger.ArchivePolicy{RetentionDays: 365, MaxFileSizeMB: 100, MaxFilesPerRun: 10, Globs: []string{"*.jsonl"}}
	if err := runLedgerArchive(context.Background(), dir, policy); err != nil {
		t.Fatalf("runLedgerArchive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "recommendation_outcomes.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("aged file must be archived (err=%v)", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh file must be untouched: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(dir, "archive"))
	if err != nil {
		t.Fatalf("read archive dir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("archive dir holds %d entries, want 1", len(entries))
	}
	f, err := os.Open(filepath.Join(dir, "archive", entries[0].Name()))
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	defer func() { _ = gz.Close() }()
	data, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	if string(data) != taskFixtureRecords {
		t.Errorf("archive content = %q, want %q", string(data), taskFixtureRecords)
	}
}
