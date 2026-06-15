// Command backfill-summaries reconstructs minimal summary.json for orphan sessions
// (those with recommendation_outcomes.jsonl but no summary.json). One-shot tool;
// safe to re-run (idempotent: never overwrites existing summary.json).
//
// Usage:
//
//	go run ./cmd/backfill-summaries             # backfill using default paths
//	go run ./cmd/backfill-summaries -dry-run    # print plan without writing
//	go run ./cmd/backfill-summaries -ledger-dir /custom/path/to/ledger
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/backfill"
	"github.com/kaecer68/atlas-go/internal/config"
)

var (
	dryRun    = flag.Bool("dry-run", false, "print plan without writing summary.json")
	ledgerDir = flag.String("ledger-dir", "", "override ledger directory (default: from config)")
	workDir   = flag.String("workdir", "", "override work directory (default: from config)")
)

func main() {
	flag.Parse()

	cfg := config.GetParametersConfig()
	if cfg == nil {
		fmt.Fprintln(os.Stderr, "config not initialized; run from project root with parameters.json available")
		os.Exit(1)
	}

	resolvedLedger := resolveLedgerDir(*ledgerDir, *workDir)
	sessionsDir := filepath.Join(resolvedLedger, "sessions")

	fmt.Printf("Scanning sessions directory: %s\n", sessionsDir)
	if *dryRun {
		fmt.Println("Mode: DRY-RUN (no files will be written)")
	}

	result, err := backfill.BackfillSummaries(sessionsDir, *dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "backfill failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nResults:\n")
	fmt.Printf("  Scanned:        %d session directories\n", result.Scanned)
	fmt.Printf("  Backfilled:     %d (minimal summary.json written)\n", result.Backfilled)
	fmt.Printf("  Skipped (exists): %d (summary.json already present, not overwritten)\n", result.SkippedExists)
	fmt.Printf("  Skipped (empty):  %d (no outcomes.jsonl, no summary to derive)\n", result.SkippedEmpty)

	if result.Backfilled > 0 && !*dryRun {
		fmt.Println("\nNext steps:")
		fmt.Println("  - Re-run pipeline page to verify the previously-orphan session now shows data")
		fmt.Println("  - Investigate the root cause of the missing summary.json (see cmd/orchestrator + cmd/run-experiment write paths)")
	}
}

// resolveLedgerDir 解析 ledger 目錄,優先序: 明確指定 > workdir flag > config
func resolveLedgerDir(ledgerFlag, workdirFlag string) string {
	if ledgerFlag != "" {
		return ledgerFlag
	}
	// 延遲載入 workDir 以避免 config.GetParametersConfig 早期回傳 nil
	if workdirFlag == "" {
		if wd, err := os.Getwd(); err == nil {
			workdirFlag = wd
		}
	}
	return filepath.Join(workdirFlag, "data", "ledger")
}
