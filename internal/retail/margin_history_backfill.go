// Package retail provides RSI-tw (Retail Sentiment Index — Taiwan).
package retail

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// marginHistoryBackfillMax is the number of most-recent persisted margin
// balance entries loaded into marginHistory at backfill time (the A1 z-score
// window is 30, so 30 historical trading days is the useful baseline).
const marginHistoryBackfillMax = 30

// marginHistoryFile mirrors the on-disk margin cache written by
// marketdata.TWSEMarginBalanceProvider.saveMargin
// (data/state/margin/2026MMDD_margin.json).
type marginHistoryFile struct {
	Date          string  `json:"date"`
	MarginBalance float64 `json:"margin_balance"`
}

// LoadMarginHistoryFromDisk reads margin balance history from persisted margin
// cache files (*_margin.json) under dir and returns at most maxEntries values
// ordered by date ascending (most recent last). Files that fail to read/parse
// are skipped so one corrupt file cannot break the whole backfill. A missing or
// unreadable dir returns an error so callers can log a warning and keep the
// existing in-memory fallback behavior.
func LoadMarginHistoryFromDisk(dir string, maxEntries int) ([]float64, error) {
	if maxEntries <= 0 {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read margin history dir: %w", err)
	}

	var files []marginHistoryFile
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_margin.json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name())) // #nosec G304 — path from dir listing
		if err != nil {
			continue
		}
		var mf marginHistoryFile
		if err := json.Unmarshal(data, &mf); err != nil || mf.Date == "" {
			continue
		}
		files = append(files, mf)
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Date < files[j].Date })
	if len(files) > maxEntries {
		files = files[len(files)-maxEntries:]
	}

	values := make([]float64, 0, len(files))
	for _, f := range files {
		values = append(values, f.MarginBalance)
	}
	return values, nil
}
