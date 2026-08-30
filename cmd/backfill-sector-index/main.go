// Command backfill-sector-index writes per-day TWSE sector index files into
// data/state/sector_index/ for every date in [--start, --end] that has data,
// filling the S-gap before the daily "twse_sector_index" gateway channel
// started (2026-06-03). It reuses TWSESectorIndexProvider.FetchSectorIndices.
//
// -source twse    TWSE OpenAPI MI_INDEX (latest-only; historical dates are
//
//	hard-blocked by ErrLatestOnly since #1752 — use for recent
//	windows only).
//
// -source finmind FinMind TaiwanStockEvery5SecondsIndex (每5秒指數統計 / 台灣類股
//
//	股價表; Backer/Sponsor tier, history back to 2005). Use for
//	historical backfill (D08: 2021 sector index). The dataset
//	returns the whole market per request; the provider keeps the
//	last 13:30:00 print per twse series as the daily close and
//	maps the 18 series onto the canonical L1 sectors.
//	FINMIND_API_KEY must be set (env or ~/.config/atlas-go/.env).
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
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func main() {
	var (
		dir    = flag.String("dir", "data/state/sector_index", "sector index directory")
		start  = flag.String("start", "2026-04-25", "backfill window start (YYYY-MM-DD, TW local)")
		end    = flag.String("end", time.Now().In(mustTWLoc()).Format("2006-01-02"), "backfill window end (YYYY-MM-DD, TW local)")
		dryRun = flag.Bool("dry-run", false, "report only, do not write")
		source = flag.String("source", "twse", "data source: twse (openapi MI_INDEX, latest-only) or finmind (historical 5s industry index)")
	)
	flag.Parse()

	ctx := context.Background()
	// cacheDir "" → the provider only fetches; the backfill wrapper owns file
	// writing so --dry-run and idempotency stay in one place.
	var fetcher backfill.SectorIndexFetcher
	switch *source {
	case "twse":
		fetcher = marketdata.NewTWSESectorIndexProvider("")
	case "finmind":
		cfg := config.Load()
		if cfg.FinMindAPIKey == "" {
			log.Fatal("FINMIND_API_KEY not set (env or ~/.config/atlas-go/.env); required for -source finmind")
		}
		fetcher = marketdata.NewFinMindSectorIndexProvider(cfg.FinMindAPIKey)
	default:
		log.Fatalf("backfill-sector-index: unknown -source %q (want twse or finmind)", *source)
	}

	result, err := backfill.BackfillSectorIndex(ctx, fetcher, *dir, *start, *end, *dryRun)
	if err != nil {
		log.Fatalf("backfill-sector-index: %v", err)
	}
	fmt.Printf("sector_index backfill [%s → %s] dir=%s source=%s dry-run=%v\n", *start, *end, *dir, *source, *dryRun)
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
