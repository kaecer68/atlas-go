package main

// Experiment-family backfill (-experiments): copy the JSONL-side experiment
// artifacts into the PostgreSQL tables created by migration 000015
// (experiments, prompt_experiment_results).
//
// Why this exists: on the JSONL side the experiment family is file-only. The
// producer appends every status transition of an experiment to
// <LedgerDir>/experiments.jsonl and writes one result file per evaluated
// candidate to <LedgerDir>/experiments/<experiment_id>.json
// (internal/ledger.Store.RecordPromptExperimentResult). Once
// ATLAS_STORE_BACKEND=postgres the store-backed readers answer from PG, so the
// pre-migration history has to be moved into PG or it becomes invisible. The
// SQLite half is already covered by -experiments-sqlite; this is the JSONL half
// (the two id spaces are disjoint, evidence C1: 984 SQLite rows vs 1,153 JSONL
// rows with no overlap).
//
// Contract:
//   - experiments.jsonl is append-only, so one experiment_id appears many times
//     (planned -> running -> accepted/rejected/expired). The backfill folds the
//     stream to ONE row per id with LAST write wins — the same semantics as
//     ledger.LatestExperimentStatusByID. Writing every line would both collide
//     on experiments.experiment_id UNIQUE and leave an arbitrary (first) status
//     in the table.
//   - session_id is written as NULL: experiments.jsonl carries no session
//     information, so nothing is invented. The upsert's
//     COALESCE(experiments.session_id, EXCLUDED.session_id) keeps a session_id
//     that a live run (or -experiments-sqlite) already stored, and
//     RecordSessionExperiment's targeted upsert (FU-20261016-03) keeps working
//     the same way afterwards.
//   - Idempotent: both halves are INSERT ... ON CONFLICT (experiment_id) DO
//     UPDATE — the SQL shapes already in production use in
//     internal/ledger/postgres_ledger.go (RecordSessionExperiment and
//     UpdatePromptExperimentResult) — so a re-run refreshes instead of
//     duplicating, and the freshest folded status is the one that survives.
//     DO NOTHING is not enough: the folded state must be able to overwrite an
//     older row written by a previous run.
//   - One bad line or one bad file never aborts the run: it is warned about and
//     skipped, the rest still migrates.
//   - Read-only with respect to the filesystem: nothing is written back.
//   - Fail-closed on the target: -experiments refuses to run without
//     -expect-db (requireExperimentsTargetAssertion). This is an explicit
//     production backfill, so it is deliberately NOT part of -all.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/domain"
)

const (
	// experimentResultFileSuffix is the only accepted result-file suffix.
	// ".json.tmp" (the writer's temp name before os.Rename) does NOT end in it,
	// so the leftover of a crashed write is ignored rather than parsed as a
	// half-written result.
	experimentResultFileSuffix = ".json"

	// experimentJSONLName is the append-only experiment record stream inside
	// the ledger directory (internal/ledger.ledger.go).
	experimentJSONLName = "experiments.jsonl"

	// experimentJSONLMaxLine bounds one experiments.jsonl line. The default
	// bufio.Scanner limit is 64 KiB and a too-long token aborts the whole scan;
	// one oversized (or corrupted) line must not kill the backfill.
	experimentJSONLMaxLine = 4 * 1024 * 1024
)

// experimentSkip records a source that carried experiment-family data but could
// not be used. The caller logs these; the scan itself never fails on them.
type experimentSkip struct {
	Source string // "experiments.jsonl line 12" or the result file name
	Reason string
}

// experimentLineRecord is one parsed experiments.jsonl line together with its
// physical line number, so the folding step can name the exact line it drops.
type experimentLineRecord struct {
	Line   int
	Record domain.ExperimentRecord
}

// experimentJSONLScan is the parse result of experiments.jsonl.
type experimentJSONLScan struct {
	Found   bool // the file exists (missing is the normal state, not an error)
	Lines   int  // non-blank lines read
	Records []experimentLineRecord
	Skipped []experimentSkip
}

// experimentArtifact is one parsed <experiment_id>.json result file.
type experimentArtifact struct {
	FileName     string
	ExperimentID string    // row key: derived from the file name
	ModTime      time.Time // file mtime; zero when it could not be read
	Result       domain.PromptExperimentResult
}

// experimentScan is the whole plan of one -experiments run: everything the PG
// writer needs, plus the counters the final report prints.
type experimentScan struct {
	JSONLFound     bool
	JSONLLines     int
	JSONLSkipped   int
	Experiments    []domain.ExperimentRecord // folded: one per id, last write wins
	ResultFiles    int
	ResultsSkipped int
	Results        []experimentArtifact
	Skipped        []experimentSkip // per-source detail for the warnings
}

// experimentRow is one experiments table row, ready to bind. It is built by a
// pure function so the column mapping stays unit-testable without a database.
type experimentRow struct {
	ExperimentID      string
	MutationBriefJSON string
	Accepted          int    // 1 iff Status == ExperimentAccepted (boolToInt)
	Timestamp         string // RFC3339, mirroring RecordExperiment
}

// scanExperimentJSONL reads the append-only experiment record stream. A missing
// file is not an error (a deployment that never ran an experiment has none);
// only a file that cannot be read at all returns an error.
func scanExperimentJSONL(path string) (experimentJSONLScan, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return experimentJSONLScan{}, nil
		}
		return experimentJSONLScan{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	scan := experimentJSONLScan{Found: true}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), experimentJSONLMaxLine)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		scan.Lines++
		var rec domain.ExperimentRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			scan.Skipped = append(scan.Skipped, experimentSkip{
				Source: fmt.Sprintf("%s line %d", path, lineNo),
				Reason: fmt.Sprintf("unmarshal experiment record: %v", err),
			})
			continue
		}
		scan.Records = append(scan.Records, experimentLineRecord{Line: lineNo, Record: rec})
	}
	if err := scanner.Err(); err != nil {
		return experimentJSONLScan{}, fmt.Errorf("scan %s: %w", path, err)
	}
	return scan, nil
}

// foldExperimentRecords folds the append-only stream into one record per
// experiment id, last write wins (mirror of
// ledger.LatestExperimentStatusByID). The first appearance fixes the row's
// position so the output order is the stream's first-appearance order and a
// re-run of the same input is byte-identical.
//
// A record without an id cannot address a row (experiments.experiment_id is
// NOT NULL UNIQUE — every such record would fight over the same empty key), so
// it is reported as skipped instead of written. The skip labels name
// experiments.jsonl because this fold is only ever fed from that stream.
func foldExperimentRecords(records []experimentLineRecord) ([]domain.ExperimentRecord, []experimentSkip) {
	folded := make([]domain.ExperimentRecord, 0, len(records))
	index := make(map[string]int, len(records))
	var skipped []experimentSkip
	for _, lr := range records {
		if lr.Record.ID == "" {
			skipped = append(skipped, experimentSkip{
				Source: fmt.Sprintf("experiments.jsonl line %d", lr.Line),
				Reason: "experiment record without an id (cannot address a unique row)",
			})
			continue
		}
		if pos, ok := index[lr.Record.ID]; ok {
			folded[pos] = lr.Record // last write wins
			continue
		}
		index[lr.Record.ID] = len(folded)
		folded = append(folded, lr.Record)
	}
	return folded, skipped
}

// experimentRowValues maps one folded record to the experiments table columns
// the way internal/ledger.PostgresLedgerStore.RecordExperiment does:
//   - mutation_brief_json = the whole record as JSON
//   - accepted            = 1 iff the status is ExperimentAccepted
//   - timestamp           = WindowStart as RFC3339 ("2006-01-02T15:04:05Z07:00")
//   - session_id          = NULL (supplied by the caller's SQL, not here)
func experimentRowValues(rec domain.ExperimentRecord) (experimentRow, error) {
	briefJSON, err := json.Marshal(rec)
	if err != nil {
		return experimentRow{}, fmt.Errorf("marshal experiment %s: %w", rec.ID, err)
	}
	return experimentRow{
		ExperimentID:      rec.ID,
		MutationBriefJSON: string(briefJSON),
		Accepted:          boolToInt(rec.Status == domain.ExperimentAccepted),
		Timestamp:         rec.WindowStart.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

// upsertExperimentSQL mirrors the shapes the live writer uses today:
// RecordExperiment (Q2 batch 2 unit A, 2026-10-16) upserts
// mutation_brief_json / accepted / timestamp so the table can carry a status
// transition, and RecordSessionExperiment keeps session_id out of the update by
// writing COALESCE(experiments.session_id, EXCLUDED.session_id). This backfill
// needs both: a re-run must refresh the row with the folded latest status, and
// the NULL it writes must never erase a session_id the live pipeline already
// established (FU-20261016-03). Assigning
// COALESCE(experiments.session_id, EXCLUDED.session_id) is order-independent:
// in a DO UPDATE the unqualified table name always refers to the stored row.
const upsertExperimentSQL = `
	INSERT INTO experiments (experiment_id, session_id, mutation_brief_json, accepted, timestamp)
	VALUES ($1, NULL, $2, $3, $4)
	ON CONFLICT (experiment_id) DO UPDATE SET
		mutation_brief_json = EXCLUDED.mutation_brief_json,
		accepted = EXCLUDED.accepted,
		timestamp = EXCLUDED.timestamp,
		session_id = COALESCE(experiments.session_id, EXCLUDED.session_id)
`

// upsertPromptExperimentResultSQL mirrors UpdatePromptExperimentResult: the JSON
// store overwrites <experiment_id>.json, so the PG row must be overwritten too
// (a bare INSERT here would report a duplicate key on every re-run, which is the
// bug -windows' summary half avoids by upserting as well).
const upsertPromptExperimentResultSQL = `
	INSERT INTO prompt_experiment_results (experiment_id, data_json, created_at)
	VALUES ($1, $2, $3)
	ON CONFLICT (experiment_id) DO UPDATE SET
		data_json = EXCLUDED.data_json,
		created_at = EXCLUDED.created_at
`

// classifyExperimentResultFile maps one directory entry name to the experiment
// id its content belongs to. Only "<id>.json" is part of the family: non-JSON
// names, the writer's "<id>.json.tmp" leftovers (which do not end in ".json"),
// dot files and anything that cannot yield a usable id are ignored outright
// rather than reported as broken data.
func classifyExperimentResultFile(name string) (string, bool) {
	id, ok := strings.CutSuffix(name, experimentResultFileSuffix)
	if !ok || id == "" || strings.HasPrefix(id, ".") || strings.ContainsAny(id, `/\`) {
		return "", false
	}
	return id, true
}

// scanExperimentResults reads and parses every result file in dir. Missing
// directory means "nothing recorded yet" and scans empty; an unreadable path
// (permissions, ENOTDIR) is a real error.
//
// The file name is the row key: the producer builds the name from the
// experiment id it is recording (Store.RecordPromptExperimentResult), and every
// file-scanning reader addresses the artifact by name. When the payload carries
// a different non-empty id the caller warns — the file is still ingested under
// its name, never silently re-keyed.
func scanExperimentResults(dir string) ([]experimentArtifact, []experimentSkip, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("read experiments dir %s: %w", dir, err)
	}

	var (
		results []experimentArtifact
		skipped []experimentSkip
	)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		id, ok := classifyExperimentResultFile(name)
		if !ok {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			skipped = append(skipped, experimentSkip{
				Source: name,
				Reason: fmt.Sprintf("read: %v", err),
			})
			continue
		}
		var result domain.PromptExperimentResult
		if err := json.Unmarshal(data, &result); err != nil {
			skipped = append(skipped, experimentSkip{
				Source: name,
				Reason: fmt.Sprintf("unmarshal prompt experiment result: %v", err),
			})
			continue
		}
		artifact := experimentArtifact{FileName: name, ExperimentID: id, Result: result}
		// mtime is the file's real write time; a failure to read it only costs
		// the created_at precision (the writer falls back to now).
		if info, err := entry.Info(); err == nil {
			artifact.ModTime = info.ModTime()
		}
		results = append(results, artifact)
	}
	return results, skipped, nil
}

// scanExperimentFamily is the pure planner for one -experiments run:
// filesystem in, records + counters out (no PG handle, no logging), so the
// folding and classification rules stay unit-testable without a database.
func scanExperimentFamily(jsonlPath, dir string) (experimentScan, error) {
	jsonl, err := scanExperimentJSONL(jsonlPath)
	if err != nil {
		return experimentScan{}, err
	}
	results, resultSkips, err := scanExperimentResults(dir)
	if err != nil {
		return experimentScan{}, err
	}

	folded, foldSkips := foldExperimentRecords(jsonl.Records)
	scan := experimentScan{
		JSONLFound:     jsonl.Found,
		JSONLLines:     jsonl.Lines,
		JSONLSkipped:   len(jsonl.Skipped) + len(foldSkips),
		Experiments:    folded,
		ResultFiles:    len(results),
		ResultsSkipped: len(resultSkips),
		Results:        results,
	}
	scan.Skipped = append(scan.Skipped, jsonl.Skipped...)
	scan.Skipped = append(scan.Skipped, foldSkips...)
	scan.Skipped = append(scan.Skipped, resultSkips...)
	return scan, nil
}

// requireExperimentsTargetAssertion is the fail-closed gate for -experiments, the
// third production-writing backfill in this command (see
// requireMigrateTargetAssertion in stockpicker.go): a forgotten -expect-db must
// abort the run, never backfill whatever DATABASE_URL happens to point at.
//
// It mirrors that helper instead of extending it: the helper's flag set has a
// fixed two-boolean shape and the abort message must name the flags that are
// actually on the command line, which is also why this one exists at all.
func requireExperimentsTargetAssertion(experiments bool, expectDB string) error {
	if !experiments || expectDB != "" {
		return nil
	}
	return fmt.Errorf("-experiments requires -expect-db=<name> (refusing to write without a target assertion)")
}

// experimentSourcePaths resolves the two -experiments source paths from the
// LedgerDir convention (the same way -windows derives <LedgerDir>/windows):
// each explicit flag wins, otherwise the path is relative to the ledger
// directory. Kept separate from the flag plumbing so the defaults are pinned by
// a hermetic test instead of by a manual run.
func experimentSourcePaths(ledgerDir, dirFlag, jsonlFlag string) (dir, jsonlPath string) {
	dir = dirFlag
	if dir == "" {
		dir = filepath.Join(ledgerDir, "experiments")
	}
	jsonlPath = jsonlFlag
	if jsonlPath == "" {
		jsonlPath = filepath.Join(ledgerDir, experimentJSONLName)
	}
	return dir, jsonlPath
}

// migrateExperimentsData backfills the JSONL-side experiment family (jsonlPath =
// <LedgerDir>/experiments.jsonl, dir = <LedgerDir>/experiments) into PG.
// Idempotent, and never aborts on a single bad source (see the file header).
func migrateExperimentsData(ctx context.Context, pool *pgxpool.Pool, dir, jsonlPath string) error {
	scan, err := scanExperimentFamily(jsonlPath, dir)
	if err != nil {
		return err
	}
	for _, skip := range scan.Skipped {
		log.Printf("Warning: skipping %s: %s", skip.Source, skip.Reason)
	}
	if len(scan.Experiments) == 0 && len(scan.Results) == 0 {
		log.Printf("No experiment records in %s and no prompt experiment results in %s, skipping",
			jsonlPath, dir)
		return nil
	}

	var (
		experimentWrites int
		resultWrites     int
		writeFailures    int
	)

	for _, rec := range scan.Experiments {
		row, err := experimentRowValues(rec)
		if err != nil {
			log.Printf("Warning: failed to encode experiment %s: %v", rec.ID, err)
			writeFailures++
			continue
		}
		if _, err := pool.Exec(ctx, upsertExperimentSQL,
			row.ExperimentID, row.MutationBriefJSON, row.Accepted, row.Timestamp); err != nil {
			log.Printf("Warning: failed to upsert experiment %s: %v", row.ExperimentID, err)
			writeFailures++
			continue
		}
		experimentWrites++
	}

	for _, artifact := range scan.Results {
		if dataID := artifact.Result.Experiment.ID; dataID != "" && dataID != artifact.ExperimentID {
			log.Printf("Warning: %s carries experiment id %q but is stored under %q (keyed by file name)",
				artifact.FileName, dataID, artifact.ExperimentID)
		}
		// created_at = the result file's mtime (UTC RFC3339): it is the file's
		// real write time, and unlike time.Now() it keeps a re-run byte-stable
		// instead of restamping every row with the migration time. The fallback
		// is only for a directory entry whose mtime cannot be read.
		var createdAt string
		if artifact.ModTime.IsZero() {
			createdAt = time.Now().UTC().Format(time.RFC3339)
		} else {
			createdAt = artifact.ModTime.UTC().Format(time.RFC3339)
		}
		data, err := json.Marshal(artifact.Result)
		if err != nil {
			log.Printf("Warning: failed to encode prompt experiment result %s: %v", artifact.FileName, err)
			writeFailures++
			continue
		}
		if _, err := pool.Exec(ctx, upsertPromptExperimentResultSQL,
			artifact.ExperimentID, string(data), createdAt); err != nil {
			log.Printf("Warning: failed to upsert prompt experiment result %s: %v", artifact.FileName, err)
			writeFailures++
			continue
		}
		resultWrites++
	}

	log.Printf("experiments: migrated %d ids (from %d jsonl lines, %d skipped)",
		experimentWrites, scan.JSONLLines, scan.JSONLSkipped)
	log.Printf("prompt_experiment_results: migrated %d files (%d skipped)",
		resultWrites, scan.ResultsSkipped)
	if writeFailures > 0 {
		// A single failed row must not abort the backfill, but a run in which
		// NOTHING landed (missing table, permissions, wrong backend) must not
		// report success either.
		if experimentWrites+resultWrites == 0 {
			return fmt.Errorf("all %d experiment-family writes failed (see the warnings above)", writeFailures)
		}
		log.Printf("Warning: %d experiment-family rows failed to write (the rest were migrated)", writeFailures)
	}
	log.Printf("Experiment-family backfill complete")
	return nil
}
