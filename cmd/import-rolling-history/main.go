// Command import-rolling-history — CAL-1: bulk-load historical
// capital-flow samples into the BK-15 rolling sample store with one
// atomic write.
//
// Background: the rolling window that feeds the Z-score reference only
// grows by one trading day per Service.Refresh call, so a fresh
// production store stays in "calibrating" with sample_count=1 and a
// flat Z-score history for weeks. This tool replays history that
// already exists in the repo — real TWSE T86 三大法人 snapshots under
// data/state/capital_flow/ intersected with the replay trading
// calendar data/replay/tw_extended_90days.csv — into the same store
// file the server reads, bringing foreign / institutional / dealer
// sample counts above 1 immediately.
//
// Usage:
//
//	go run ./cmd/import-rolling-history \
//	    [-store data/state/capital_flow_rolling.json] \
//	    [-replay data/replay/tw_extended_90days.csv] \
//	    [-t86 data/state/capital_flow] \
//	    [-gov data/state/government_flow] \
//	    [-from 2026-05-01] [-capacity 252] [-dry-run]
//
// Since 2026-08-26 the government dimension IS imported when real
// readings exist under data/state/government_flow/ (HiStock media-curated
// daily readings; total_net TWD converted to 億元). The remaining
// dimensions (futures / tsm_adr / retail) are intentionally not
// fabricated: they need their real sources (TAIFEX institutional OI,
// Yahoo TSM ADR, TWSE margin+當沖). See internal/capitalflow/history_import.go.
package main

import (
	"context"
	"flag"
	"log"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
)

const (
	defaultStorePath  = "data/state/capital_flow_rolling.json"
	defaultReplayPath = "data/replay/tw_extended_90days.csv"
	defaultT86Dir     = "data/state/capital_flow"
	defaultGovDir     = "data/state/government_flow"
	defaultCapacity   = 252
	// defaultFrom excludes the 2025-05..2025-09 T86 snapshots that
	// repeat identical values across consecutive days (synthetic
	// placeholder data from an early fetch). Real readings start in
	// 2026-05; widen with -from when real sources cover earlier dates.
	defaultFrom = "2026-05-01"
)

func main() {
	var (
		storePath  = flag.String("store", defaultStorePath, "rolling sample store JSON path (same file the server reads)")
		replayPath = flag.String("replay", defaultReplayPath, "replay CSV trading calendar")
		t86Dir     = flag.String("t86", defaultT86Dir, "directory of TWSE T86 snapshot JSON files")
		govDir     = flag.String("gov", defaultGovDir, "directory of government_flow reading JSON files (YYYYMMDD.json)")
		fromDate   = flag.String("from", defaultFrom, "import only trading dates >= this date (YYYY-MM-DD, inclusive); empty = full replay range")
		capacity   = flag.Int("capacity", defaultCapacity, "per-dimension rolling capacity (must match the server wiring, main.go:939 uses 252)")
		dryRun     = flag.Bool("dry-run", false, "build and report the batch without writing the store")
	)
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	samples, rep, err := capitalflow.BuildHistorySamples(*replayPath, *t86Dir, *govDir, *fromDate)
	if err != nil {
		log.Fatalf("import-rolling-history: %v", err)
	}
	log.Printf("build: %s", rep.String())

	if *dryRun {
		log.Printf("dry-run: store %s untouched (import would write %d samples)", *storePath, len(samples))
		return
	}
	if len(samples) == 0 {
		log.Printf("nothing to import (replay ∩ T86 window is empty); store %s left unchanged", *storePath)
		return
	}

	// Resolve relative to the repo root the same way the server does
	// (ATLAS_LEDGER_DIR defaults to data/state; the server joins it
	// with capital_flow_rolling.json).
	abs, err := filepath.Abs(*storePath)
	if err != nil {
		log.Fatalf("import-rolling-history: resolve store path: %v", err)
	}
	store := capitalflow.NewFileRollingSampleStore(abs, *capacity)
	if err := store.ImportHistory(ctx, samples); err != nil {
		log.Fatalf("import-rolling-history: ImportHistory: %v", err)
	}
	log.Printf("import: wrote %d samples to %s", len(samples), abs)

	// Post-import readback: per-dimension sample counts strictly
	// before today — the exact number /api/capital-flow/summary
	// surfaces as forces[].sample_count (spec §8.4 excludes today's
	// own observation from the reference window).
	const sentinel = "9999-12-31"
	today := time.Now().Format("2006-01-02")
	for _, dim := range []capitalflow.ForceName{
		capitalflow.ForceForeign, capitalflow.ForceInstitutional, capitalflow.ForceDealer,
		capitalflow.ForceFutures, capitalflow.ForceTSMADR, capitalflow.ForceGovernment, capitalflow.ForceRetail,
	} {
		hist, err := store.History(ctx, dim, today, *capacity)
		if err != nil {
			log.Printf("readback %s: %v", dim, err)
			continue
		}
		total, _ := store.History(ctx, dim, sentinel, *capacity)
		log.Printf("readback %-14s sample_count(before %s)=%d total_persisted=%d",
			dim, today, len(hist), len(total))
	}
}
