package main

// Ledger JSONL archival background task (FU-20261012-05).
//
// Root cause of the follow-up: internal/ledger.Archiver implemented rotation but
// had zero call sites, so data/state grew without bound (2026-10-16 read-only
// audit: 2,291 JSONL files / 864 MB under data/, of which
// recommendation_outcomes.jsonl alone was 172 MB / 45,668 records). Wiring it
// alone was not enough either: the archiver's fixed 4-name list did not include
// the files that actually grow and named alerts.jsonl, which lives in alerts/.
//
// This task is the lifecycle hook: it runs on the existing BackgroundTaskManager
// (never inside the tick hot path), is bounded per run (ATLAS_LEDGER_ARCHIVE_
// FILES_PER_RUN) and per duration, and only ever *moves* data into
// <LedgerDir>/archive (verified line count) unless pruning is explicitly
// enabled.

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

const (
	// ledgerArchiveInterval is how often a bounded archival pass runs.
	ledgerArchiveInterval = 6 * time.Hour
	// ledgerArchiveRunBudget caps the wall-clock time of a single pass so the
	// task can never turn into an unbounded IO burst.
	ledgerArchiveRunBudget = 15 * time.Minute
)

// ledgerArchiveDir resolves the ledger base directory (same semantics as every
// other ledger consumer: ATLAS_LEDGER_DIR, relative to the work dir).
func ledgerArchiveDir(cfg config.Config) string {
	dir := cfg.LedgerDir
	if dir == "" {
		dir = "data/state"
	}
	if filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(cfg.WorkDir, dir)
}

// ledgerArchivePolicy maps the operator configuration onto the archiver policy.
func ledgerArchivePolicy(cfg config.Config) ledger.ArchivePolicy {
	return ledger.ArchivePolicy{
		RetentionDays:    cfg.LedgerArchiveRetentionDays,
		MaxFileSizeMB:    int64(cfg.LedgerArchiveMaxFileSizeMB),
		MaxFilesPerRun:   cfg.LedgerArchiveFilesPerRun,
		ArchivePruneDays: cfg.LedgerArchivePruneDays,
		Globs:            cfg.LedgerArchiveGlobs,
	}
}

// registerLedgerArchiveTask registers the ledger_archive task. It returns false
// when archival is disabled by configuration, so callers/tests can assert the
// gate without inspecting the registry.
func registerLedgerArchiveTask(taskMgr *apigateway.BackgroundTaskManager, cfg config.Config) bool {
	if taskMgr == nil || !cfg.LedgerArchiveEnabled {
		return false
	}
	policy := ledgerArchivePolicy(cfg)
	dir := ledgerArchiveDir(cfg)

	err := taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "ledger_archive",
		Interval: ledgerArchiveInterval,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			return runLedgerArchive(ctx, dir, policy)
		},
	})
	if err != nil {
		log.Printf("[Gateway] ledger_archive registration failed: %v", err)
		return false
	}
	log.Printf("[Gateway] registered ledger_archive background task (%s interval, dir=%s, retention=%dd, max_size=%dMB)",
		ledgerArchiveInterval, dir, policy.RetentionDays, policy.MaxFileSizeMB)
	return true
}

// runLedgerArchive performs one bounded archival pass over dir.
func runLedgerArchive(ctx context.Context, dir string, policy ledger.ArchivePolicy) error {
	runCtx, cancel := context.WithTimeout(ctx, ledgerArchiveRunBudget)
	defer cancel()

	archiver := ledger.NewArchiver(dir).WithPolicy(policy).WithContext(runCtx)
	report, err := archiver.RunWithReport()
	printLedgerArchiveSummary(dir, report)
	if err != nil {
		return fmt.Errorf("ledger archive: %w", err)
	}
	if len(report.Errors) > 0 {
		// Surface persistent per-file failures through the task liveness path
		// (the BTM alerts after 3 consecutive failures) instead of hiding them
		// in logs only.
		return fmt.Errorf("ledger archive: %d of %d file(s) failed: %s",
			len(report.Errors), report.Scanned, report.Errors[0])
	}
	return nil
}

// printLedgerArchiveSummary logs one auditable line per run.
func printLedgerArchiveSummary(dir string, report ledger.RunReport) {
	log.Printf("[LedgerArchive] dir=%s scanned=%d archived=%d rotated=%d pruned=%d skipped=%d freed_mb=%d errors=%d",
		dir, report.Scanned, len(report.Archived), len(report.Rotated), report.Pruned, report.Skipped,
		report.BytesFreed/1024/1024, len(report.Errors))
	for _, f := range report.Archived {
		log.Printf("[LedgerArchive] moved %s → %s (%d lines, %d bytes)", f.Path, f.Archive, f.Lines, f.Size)
	}
	for _, f := range report.Rotated {
		log.Printf("[LedgerArchive] rotated %s → %s (%d lines, %d bytes)", f.Path, f.Archive, f.Lines, f.Size)
	}
}
