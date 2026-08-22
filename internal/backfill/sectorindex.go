package backfill

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// SectorIndexFetcher abstracts the TWSE sector index fetch so tests can inject
// a stub without network access.
type SectorIndexFetcher interface {
	FetchSectorIndices(ctx context.Context, startDate, endDate time.Time) (map[string][]marketdata.SectorIndexData, error)
}

// SectorIndexResult reports the outcome of a sector-index backfill run.
type SectorIndexResult struct {
	Scanned       int // calendar days in [start, end]
	Backfilled    int // per-day sector_indices_YYYYMMDD_YYYYMMDD.json files written
	SkippedExists int // per-day file already present
	SkippedNoData int // weekend / non-trading day / persistent fetch failure
	Errors        int // hard errors (unreadable dir, etc.)
}

// BackfillSectorIndex writes per-day sector index files into dir for every
// date in [start, end] that has TWSE data. It is idempotent:
//   - Existing sector_indices_YYYYMMDD_YYYYMMDD.json files are never overwritten.
//   - Weekend and non-trading dates are skipped.
//   - dryRun reports what would be done without writing anything.
//
// File format matches TWSESectorIndexProvider.saveToCache:
// map[industry][]SectorIndexData, indented with 2 spaces.
func BackfillSectorIndex(ctx context.Context, fetcher SectorIndexFetcher, dir, start, end string, dryRun bool) (SectorIndexResult, error) {
	var result SectorIndexResult
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		return result, fmt.Errorf("load Asia/Taipei: %w", err)
	}
	startDate, err := parseDateArg(start, loc)
	if err != nil {
		return result, fmt.Errorf("parse --start: %w", err)
	}
	endDate, err := parseDateArg(end, loc)
	if err != nil {
		return result, fmt.Errorf("parse --end: %w", err)
	}
	if endDate.Before(startDate) {
		return result, fmt.Errorf("--end %s is before --start %s", end, start)
	}

	if !dryRun {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return result, fmt.Errorf("create sector dir %s: %w", dir, err)
		}
	}

	current := startDate
	for !current.After(endDate) {
		result.Scanned++
		day := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, loc)

		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			result.SkippedNoData++
			current = current.AddDate(0, 0, 1)
			continue
		}

		compact := day.Format("20060102")
		target := filepath.Join(dir, fmt.Sprintf("sector_indices_%s_%s.json", compact, compact))
		if _, err := os.Stat(target); err == nil {
			result.SkippedExists++
			current = current.AddDate(0, 0, 1)
			continue
		}

		data, err := sectorFetchWithRetry(ctx, fetcher, day)
		if err != nil || len(data) == 0 {
			// Non-trading day (holiday) or persistent failure: skip.
			result.SkippedNoData++
			current = current.AddDate(0, 0, 1)
			continue
		}

		if dryRun {
			result.Backfilled++
			current = current.AddDate(0, 0, 1)
			continue
		}

		encoded, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			result.Errors++
			current = current.AddDate(0, 0, 1)
			continue
		}
		encoded = append(encoded, '\n')
		if err := writeFileAtomic(target, encoded); err != nil {
			result.Errors++
			current = current.AddDate(0, 0, 1)
			continue
		}
		result.Backfilled++
		current = current.AddDate(0, 0, 1)
	}
	return result, nil
}

// sectorFetchWithRetry attempts a single-day fetch up to maxBackfillAttempts
// times. An empty result (no industry mapped / holiday) is reported as an
// error so callers treat it as "no data for this day".
func sectorFetchWithRetry(ctx context.Context, fetcher SectorIndexFetcher, day time.Time) (map[string][]marketdata.SectorIndexData, error) {
	var lastErr error
	for attempt := 1; attempt <= maxBackfillAttempts; attempt++ {
		res, err := fetcher.FetchSectorIndices(ctx, day, day)
		if err == nil {
			if len(res) == 0 {
				lastErr = fmt.Errorf("no sector data for %s", day.Format("2006-01-02"))
			} else {
				return res, nil
			}
		} else {
			lastErr = err
		}
		if attempt < maxBackfillAttempts {
			select {
			case <-time.After(time.Duration(attempt) * time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return nil, lastErr
}
