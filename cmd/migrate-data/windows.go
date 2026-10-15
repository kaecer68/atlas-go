package main

// Window-family backfill (-windows): copy the file-only backtest-window
// artifacts into the PostgreSQL tables created by migration 000015
// (window_summaries, mutation_briefs).
//
// Why this exists: the window family was file-only on both sides — the producer
// wrote <LedgerDir>/windows/<windowID>.json plus
// <windowID>-mutation-brief.json, and every reader scanned that directory. Once
// ATLAS_STORE_BACKEND=postgres the store-backed readers
// (internal/ledger.WindowSummaryReader) answer from PG, so the pre-migration
// history has to be moved into PG or it becomes invisible.
//
// Contract:
//   - The file name is the classifier (internal/ledger/ledger.go):
//     "<windowID>.json" is a summary, "<windowID>-mutation-brief.json" is a
//     brief. A brief file must never be parsed as a summary — it carries a
//     window_id field, so it would unmarshal cleanly into a half-empty summary
//     instead of failing.
//   - Idempotent. The summary half goes through
//     PostgresLedgerStore.RecordWindowSummary (INSERT ... ON CONFLICT
//     (window_id) DO UPDATE). mutation_briefs has NO unique key (only a
//     non-unique index), so the brief half deletes the window's existing rows
//     first: a re-run replaces, it never accumulates duplicates.
//   - One bad file never aborts the run: unreadable / corrupt artifacts are
//     warned about and skipped, the rest still migrates.
//   - Read-only with respect to the filesystem: nothing is written back.
//   - Fail-closed on the target: -windows refuses to run without -expect-db
//     (see requireMigrateTargetAssertion).

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

const (
	// windowArtifactSuffix is the summary half's file suffix.
	windowArtifactSuffix = ".json"
	// windowMutationBriefSuffix mirrors the unexported
	// ledger.mutationBriefSuffix: the brief half of the same family. It is
	// tested BEFORE windowArtifactSuffix, so a brief name is never classified
	// as a summary name.
	windowMutationBriefSuffix = "-mutation-brief.json"
)

// windowFileKind is the classification of one directory entry name.
type windowFileKind int

const (
	// windowFileIgnored covers everything outside the family: non-JSON files,
	// ".json.tmp" write leftovers, and names that do not yield a usable window
	// id ("" or dot-prefixed).
	windowFileIgnored windowFileKind = iota
	windowFileSummary
	windowFileMutationBrief
)

// windowArtifact is one classified, parsed file of the windows directory.
type windowArtifact struct {
	FileName string // base name inside the windows directory
	WindowID string // row key used for the PG write
	Summary  domain.BacktestWindowSummary
	Brief    domain.MutationBrief
}

// windowSkip records a file that ranked as part of the family but could not be
// used. The caller logs these; the scan itself never fails on them.
type windowSkip struct {
	FileName string
	Reason   string
}

// windowScan is the classification result of one directory scan.
type windowScan struct {
	Summaries []windowArtifact
	Briefs    []windowArtifact
	Skipped   []windowSkip
}

// classifyWindowFile maps one directory entry name to its artifact kind and
// window id. Anything outside the family comes back as windowFileIgnored.
func classifyWindowFile(name string) (windowFileKind, string) {
	if !strings.HasSuffix(name, windowArtifactSuffix) {
		return windowFileIgnored, ""
	}
	if id, ok := strings.CutSuffix(name, windowMutationBriefSuffix); ok {
		return usableWindowID(windowFileMutationBrief, id)
	}
	return usableWindowID(windowFileSummary, strings.TrimSuffix(name, windowArtifactSuffix))
}

// usableWindowID rejects ids that cannot address one artifact: empty
// ("-mutation-brief.json", ".json"), dot-prefixed (editor/temp leftovers) or
// path-bearing.
func usableWindowID(kind windowFileKind, id string) (windowFileKind, string) {
	if id == "" || strings.HasPrefix(id, ".") || strings.ContainsAny(id, `/\`) {
		return windowFileIgnored, ""
	}
	return kind, id
}

// scanWindowArtifacts reads and classifies every window-family file in dir.
// It is pure (filesystem in, values out: no PG handle, no logging) so the
// classification rules stay unit-testable without a database.
//
// A missing directory is not an error: it is the normal state of a deployment
// that never ran a backtest window, so the result is empty. Only a directory
// that cannot be read at all (permissions, ENOTDIR) returns an error.
func scanWindowArtifacts(dir string) (windowScan, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return windowScan{}, nil
		}
		return windowScan{}, fmt.Errorf("read windows dir %s: %w", dir, err)
	}

	var scan windowScan
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		kind, fileID := classifyWindowFile(name)
		if kind == windowFileIgnored {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			scan.Skipped = append(scan.Skipped, windowSkip{
				FileName: name,
				Reason:   fmt.Sprintf("read: %v", err),
			})
			continue
		}
		switch kind {
		case windowFileSummary:
			var summary domain.BacktestWindowSummary
			if err := json.Unmarshal(data, &summary); err != nil {
				scan.Skipped = append(scan.Skipped, windowSkip{
					FileName: name,
					Reason:   fmt.Sprintf("unmarshal window summary: %v", err),
				})
				continue
			}
			// The writer derives the file name from summary.WindowID, so for a
			// row written by the producer the two agree and either can key the
			// PG row. An empty data field (hand-written or truncated file)
			// falls back to the file name — the id every file-scanning reader
			// already uses. Without the fallback the row would land as
			// window_id='', unaddressable and silently overwritten by the next
			// such file.
			if summary.WindowID == "" {
				summary.WindowID = fileID
			}
			scan.Summaries = append(scan.Summaries, windowArtifact{
				FileName: name,
				WindowID: summary.WindowID,
				Summary:  summary,
			})
		case windowFileMutationBrief:
			var brief domain.MutationBrief
			if err := json.Unmarshal(data, &brief); err != nil {
				scan.Skipped = append(scan.Skipped, windowSkip{
					FileName: name,
					Reason:   fmt.Sprintf("unmarshal mutation brief: %v", err),
				})
				continue
			}
			// RecordMutationBrief takes the window id as an argument and the
			// JSONL writer built the file name from that same argument, so the
			// file name is the authoritative key for the brief half.
			scan.Briefs = append(scan.Briefs, windowArtifact{
				FileName: name,
				WindowID: fileID,
				Brief:    brief,
			})
		case windowFileIgnored:
			// Unreachable (filtered above); kept so the switch stays exhaustive.
		}
	}
	return scan, nil
}

// migrateWindowsData backfills the window family from windowsDir into PG.
// Idempotent and never aborts on a single bad artifact or write (see the file
// header). windowsDir defaults to <LedgerDir>/windows at the call site.
func migrateWindowsData(ctx context.Context, pool *pgxpool.Pool, windowsDir string) error {
	scan, err := scanWindowArtifacts(windowsDir)
	if err != nil {
		return err
	}
	for _, skip := range scan.Skipped {
		log.Printf("Warning: skipping window file %s: %s", skip.FileName, skip.Reason)
	}
	if len(scan.Summaries) == 0 && len(scan.Briefs) == 0 {
		log.Printf("No window summaries or mutation briefs found in %s, skipping", windowsDir)
		return nil
	}

	store := ledger.NewPostgresLedgerStore(pool)

	var summaryCount int
	for _, artifact := range scan.Summaries {
		// ON CONFLICT (window_id) DO UPDATE: a re-run refreshes the row.
		if err := store.RecordWindowSummary(artifact.Summary); err != nil {
			log.Printf("Warning: failed to upsert window summary %s (%s): %v",
				artifact.WindowID, artifact.FileName, err)
			continue
		}
		summaryCount++
	}

	var briefCount int
	for _, artifact := range scan.Briefs {
		// mutation_briefs carries no unique constraint, so a plain insert on a
		// re-run appends duplicates forever. Delete this window's rows first:
		// the run replaces the brief, it never accumulates copies.
		if _, err := pool.Exec(ctx, `DELETE FROM mutation_briefs WHERE window_id = $1`, artifact.WindowID); err != nil {
			log.Printf("Warning: failed to clear mutation briefs for %s (%s): %v",
				artifact.WindowID, artifact.FileName, err)
			continue
		}
		if err := store.RecordMutationBrief(artifact.WindowID, artifact.Brief); err != nil {
			log.Printf("Warning: failed to insert mutation brief %s (%s): %v",
				artifact.WindowID, artifact.FileName, err)
			continue
		}
		briefCount++
	}

	log.Printf("Migrated %d window summaries and %d mutation briefs", summaryCount, briefCount)
	return nil
}
