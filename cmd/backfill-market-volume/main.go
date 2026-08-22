// Command backfill-market-volume merges 集中市場成交量 (market_volume) into
// existing macro snapshot files (data/state/macro/YYYY-MM-DD.json) for the
// dates in [--start, --end]. It is a one-shot R4 backfill: daily continuation
// is handled by the gateway "market_volume" channel.
//
// Behavior (macrobackfill mode):
//   - Only existing snapshot files inside the date range are touched.
//   - Existing non-zero market_volume is never overwritten (idempotent).
//   - Weekend / non-trading days are skipped; per-date fetch retries ≤ 3.
//   - --dry-run reports what would be done without writing anything.
//
// Provenance: backfill is recorded only by the snapshot diff itself (data files
// are gitignored runtime state); no schema mutation is introduced.
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
		dir    = flag.String("dir", "data/state/macro", "macro snapshot directory")
		start  = flag.String("start", "2024-07-01", "backfill window start (YYYY-MM-DD, TW local)")
		end    = flag.String("end", time.Now().In(mustTWLoc()).Format("2006-01-02"), "backfill window end (YYYY-MM-DD, TW local)")
		dryRun = flag.Bool("dry-run", false, "report only, do not write")
	)
	flag.Parse()

	ctx := context.Background()
	result, err := backfill.BackfillMarketVolume(ctx, marketdata.NewMarketVolumeProvider(), *dir, *start, *end, *dryRun)
	if err != nil {
		log.Fatalf("backfill-market-volume: %v", err)
	}
	fmt.Printf("market_volume backfill [%s → %s] dir=%s dry-run=%v\n", *start, *end, *dir, *dryRun)
	fmt.Printf("  scanned=%d backfilled=%d skipped_exists=%d skipped_no_data=%d errors=%d\n",
		result.Scanned, result.Backfilled, result.SkippedExists, result.SkippedNoData, result.Errors)
	if result.Errors > 0 {
		log.Fatalf("backfill-market-volume: %d hard errors", result.Errors)
	}
}

func mustTWLoc() *time.Location {
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		panic(err)
	}
	return loc
}
