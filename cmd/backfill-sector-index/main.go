// Command backfill-sector-index writes per-day TWSE sector index files into
// data/state/sector_index/ for every date in [--start, --end] that has data,
// filling the S-gap before the daily "twse_sector_index" gateway channel
// started (2026-06-03). It reuses TWSESectorIndexProvider.FetchSectorIndices.
//
// Behavior:
//   - Writes sector_indices_YYYYMMDD_YYYYMMDD.json in the same format as the
//     provider cache (map[industry][]SectorIndexData, 2-space indent).
//   - Existing per-day files are never overwritten (idempotent).
//   - Weekend / non-trading days are skipped; per-date fetch retries ≤ 3.
//   - --dry-run reports what would be done without writing anything.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/kaecer68/atlas-go/internal/backfill"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func main() {
	var (
		dir    = flag.String("dir", "data/state/sector_index", "sector index directory")
		start  = flag.String("start", "2026-04-25", "backfill window start (YYYY-MM-DD, TW local)")
		end    = flag.String("end", time.Now().In(mustTWLoc()).Format("2006-01-02"), "backfill window end (YYYY-MM-DD, TW local)")
		dryRun = flag.Bool("dry-run", false, "report only, do not write")
	)
	flag.Parse()

	ctx := context.Background()
	// cacheDir "" → the provider only fetches; the backfill wrapper owns file
	// writing so --dry-run and idempotency stay in one place.
	provider := marketdata.NewTWSESectorIndexProvider("")
	result, err := backfill.BackfillSectorIndex(ctx, provider, *dir, *start, *end, *dryRun)
	if err != nil {
		log.Fatalf("backfill-sector-index: %v", err)
	}
	fmt.Printf("sector_index backfill [%s → %s] dir=%s dry-run=%v\n", *start, *end, *dir, *dryRun)
	fmt.Printf("  scanned=%d backfilled=%d skipped_exists=%d skipped_no_data=%d errors=%d\n",
		result.Scanned, result.Backfilled, result.SkippedExists, result.SkippedNoData, result.Errors)
	if result.Errors > 0 {
		log.Fatalf("backfill-sector-index: %d hard errors", result.Errors)
	}
}

func mustTWLoc() *time.Location {
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		panic(err)
	}
	return loc
}
