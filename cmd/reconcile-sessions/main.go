// Command reconcile-sessions reconciles session summaries between the two
// dual-write backends (Phase B6): PostgreSQL (session_summaries table) and the
// JSONL ledger (sessions/<id>/summary.json).
//
// It loads both sides, computes the symmetric difference, and prints a diff
// list. The default is a DRY-RUN (report only); pass -apply to backfill the
// one-sided gaps (JSONL-missing into PG, PG-missing into JSONL). Sessions
// present on both sides with drifted content are reported as conflicts and
// never auto-overwritten.
//
// Usage:
//
//	go run ./cmd/reconcile-sessions                          # dry-run report
//	go run ./cmd/reconcile-sessions -apply                   # backfill gaps
//	go run ./cmd/reconcile-sessions -ledger-dir /path        # override ledger
//	go run ./cmd/reconcile-sessions -db-url postgres://...   # override PG DSN
//	go run ./cmd/reconcile-sessions -json                    # machine output
//
// Env: DATABASE_URL (required, or -db-url), ATLAS_LEDGER_DIR (default
// data/state). Exit code: 0 = consistent (no one-sided gaps after this run);
// 1 = error, backfill errors, or one-sided gaps remain (useful for ops
// alerting — a dry-run that finds gaps exits 1).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/reconcile"
	"github.com/kaecer68/atlas-go/internal/repository"
)

var (
	apply     = flag.Bool("apply", false, "backfill one-sided gaps (default: dry-run report only)")
	jsonOut   = flag.Bool("json", false, "emit machine-readable JSON instead of a text report")
	ledgerDir = flag.String("ledger-dir", "", "override ledger directory (default: from config)")
	dbURL     = flag.String("db-url", "", "override PostgreSQL DSN (default: DATABASE_URL)")
)

func main() {
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cfg := config.Load()

	resolvedLedger := *ledgerDir
	if resolvedLedger == "" {
		resolvedLedger = cfg.LedgerDir
	}

	dsn := *dbURL
	if dsn == "" {
		dsn = cfg.DatabaseURL
	}
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "reconcile-sessions: DATABASE_URL (or -db-url) is required")
		os.Exit(1)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "reconcile-sessions: create pool: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "reconcile-sessions: ping PG: %v\n", err)
		os.Exit(1)
	}

	jsonlStore, err := ledger.NewSessionStore(config.Config{LedgerDir: resolvedLedger})
	if err != nil {
		fmt.Fprintf(os.Stderr, "reconcile-sessions: open ledger: %v\n", err)
		os.Exit(1)
	}
	pgRepo := repository.NewPostgresRepository(pool)

	res, err := reconcile.Run(ctx, pgRepo, jsonlStore, reconcile.Options{Apply: *apply})
	if err != nil {
		fmt.Fprintf(os.Stderr, "reconcile-sessions: %v\n", err)
		os.Exit(1)
	}

	// converged = no one-sided gaps remain. In dry-run mode this is simply
	// whether the first pass found any; after an apply we re-run the read side
	// to confirm the backfill actually converged the two backends.
	converged := res.Diff.Empty()
	if *apply {
		res2, err2 := reconcile.Run(ctx, pgRepo, jsonlStore, reconcile.Options{Apply: false})
		if err2 != nil {
			fmt.Fprintf(os.Stderr, "reconcile-sessions: post-apply verify: %v\n", err2)
			os.Exit(1)
		}
		converged = res2.Diff.Empty()
	}

	if *jsonOut {
		emitJSON(res, *apply, converged, resolvedLedger)
	} else {
		emitText(res, *apply, converged)
	}

	if len(res.Errors) > 0 {
		os.Exit(1)
	}
	if !converged {
		fmt.Fprintln(os.Stderr, "reconcile-sessions: one-sided gaps remain; re-run after investigating errors")
		os.Exit(1)
	}
}

type jsonReport struct {
	Mode              string         `json:"mode"`
	LedgerDir         string         `json:"ledger_dir"`
	PGCount           int            `json:"pg_count"`
	JSONLCount        int            `json:"jsonl_count"`
	OnlyPG            []string       `json:"only_pg,omitempty"`
	OnlyJSONL         []string       `json:"only_jsonl,omitempty"`
	Conflicts         []jsonConflict `json:"conflicts,omitempty"`
	BackfilledToPG    int            `json:"backfilled_to_pg,omitempty"`
	BackfilledToJSONL int            `json:"backfilled_to_jsonl,omitempty"`
	Errors            []string       `json:"errors,omitempty"`
	Converged         bool           `json:"converged"`
}

type jsonConflict struct {
	SessionID string                `json:"session_id"`
	PG        domain.SessionSummary `json:"pg"`
	JSONL     domain.SessionSummary `json:"jsonl"`
}

func buildReport(res reconcile.Result, apply, converged bool, ledgerDir string) jsonReport {
	mode := "dry-run"
	if apply {
		mode = "apply"
	}
	conflicts := make([]jsonConflict, 0, len(res.Diff.Conflicts))
	for _, c := range res.Diff.Conflicts {
		conflicts = append(conflicts, jsonConflict{SessionID: c.SessionID, PG: c.PG, JSONL: c.JSONL})
	}
	return jsonReport{
		Mode:              mode,
		LedgerDir:         ledgerDir,
		PGCount:           res.Diff.PGCount,
		JSONLCount:        res.Diff.JSONLCount,
		OnlyPG:            res.Diff.OnlyPG,
		OnlyJSONL:         res.Diff.OnlyJSONL,
		Conflicts:         conflicts,
		BackfilledToPG:    res.BackfilledToPG,
		BackfilledToJSONL: res.BackfilledToJSONL,
		Errors:            res.Errors,
		Converged:         converged,
	}
}

func emitJSON(res reconcile.Result, apply, converged bool, ledgerDir string) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(buildReport(res, apply, converged, ledgerDir)); err != nil {
		fmt.Fprintf(os.Stderr, "reconcile-sessions: encode report: %v\n", err)
		os.Exit(1)
	}
}

func emitText(res reconcile.Result, apply, converged bool) {
	mode := "DRY-RUN (no writes)"
	if apply {
		mode = "APPLY (backfill)"
	}
	fmt.Printf("reconcile-sessions: mode=%s\n", mode)
	fmt.Printf("  PG summaries:   %d\n", res.Diff.PGCount)
	fmt.Printf("  JSONL summaries:%d\n", res.Diff.JSONLCount)

	if res.Diff.Empty() {
		fmt.Println("  result: CONSISTENT — no one-sided gaps, no content conflicts")
		return
	}

	if len(res.Diff.OnlyJSONL) > 0 {
		fmt.Printf("  MISSING IN PG (%d):\n", len(res.Diff.OnlyJSONL))
		for _, id := range res.Diff.OnlyJSONL {
			fmt.Printf("    %s\n", id)
		}
	}
	if len(res.Diff.OnlyPG) > 0 {
		fmt.Printf("  MISSING IN JSONL (%d):\n", len(res.Diff.OnlyPG))
		for _, id := range res.Diff.OnlyPG {
			fmt.Printf("    %s\n", id)
		}
	}
	if len(res.Diff.Conflicts) > 0 {
		fmt.Printf("  CONFLICTS (both sides, content drifted; manual review — never auto-fixed) (%d):\n", len(res.Diff.Conflicts))
		for _, c := range res.Diff.Conflicts {
			fmt.Printf("    %s (PG ts=%s / JSONL ts=%s)\n", c.SessionID, c.PG.RecordedAt.Format(time.RFC3339), c.JSONL.RecordedAt.Format(time.RFC3339))
		}
	}
	if apply {
		fmt.Printf("  backfilled: PG<-JSONL %d, JSONL<-PG %d\n", res.BackfilledToPG, res.BackfilledToJSONL)
	} else {
		fmt.Println("  backfill: none (dry-run). Re-run with -apply to fill one-sided gaps.")
	}
	if len(res.Errors) > 0 {
		fmt.Printf("  errors (%d):\n", len(res.Errors))
		for _, e := range res.Errors {
			fmt.Printf("    %s\n", e)
		}
	}
	if converged {
		fmt.Println("  converged: YES (no one-sided gaps remain)")
	} else {
		fmt.Println("  converged: NO (one-sided gaps remain)")
	}
}
