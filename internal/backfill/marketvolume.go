package backfill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// marketVolumeSymbol matches the gateway's applyMarketVolume point symbol
// (internal/monitoring/gateway_adapter.go), keeping backfilled snapshots
// byte-compatible with live ingestion.
const marketVolumeSymbol = "TSE_VOLUME"

// maxBackfillAttempts bounds retries per date (task contract: retry ≤ 3).
const maxBackfillAttempts = 3

// MarketVolumeFetcher abstracts the TWSE MI_INDEX type=MS fetch so tests can
// inject a stub without network access.
type MarketVolumeFetcher interface {
	FetchDate(ctx context.Context, dateStr string) (*marketdata.MarketVolumeResult, error)
}

// MarketVolumeResult reports the outcome of a market-volume backfill run.
type MarketVolumeResult struct {
	Scanned       int // snapshot files whose date is inside [start, end]
	Backfilled    int // files that received a market_volume point
	SkippedExists int // file already had a non-zero market_volume
	SkippedNoData int // weekend / non-trading day / API failure after retries
	Errors        int // hard errors (unreadable file, etc.)
}

// BackfillMarketVolume merges the 集中市場成交量 (market_volume) field into
// existing macro snapshot files (macrobackfill mode). It is idempotent:
//   - Only files named YYYY-MM-DD.json inside [start, end] are considered.
//   - Existing non-zero market_volume is never overwritten.
//   - Non-trading dates (weekend/holiday) carry the most recent trading day's
//     value via a ≤7-calendar-day backscan, matching MarketVolumeProvider's
//     FetchLatest semantics (the production snapshot writer fills weekend
//     snapshots with the last session's value and timestamp).
//   - dryRun reports what would be done without writing anything.
//
// The merged point uses the same shape as the gateway adapter:
//
//	"market_volume": {"symbol":"TSE_VOLUME","value":<億>,"change_pct":0,"timestamp":<source date UTC midnight>}
func BackfillMarketVolume(ctx context.Context, fetcher MarketVolumeFetcher, dir, start, end string, dryRun bool) (MarketVolumeResult, error) {
	var result MarketVolumeResult
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

	dates, err := snapshotDatesInRange(dir, startDate, endDate)
	if err != nil {
		return result, err
	}
	result.Scanned = len(dates)

	for _, date := range dates {
		snapPath := filepath.Join(dir, date.Format("2006-01-02")+".json")
		raw, err := os.ReadFile(snapPath)
		if err != nil {
			result.Errors++
			continue
		}
		hasValue, err := snapshotHasNonZeroPoint(raw, "market_volume")
		if err != nil {
			result.Errors++
			continue
		}
		if hasValue {
			result.SkippedExists++
			continue
		}

		vol, sourceDate, err := fetchWithBackscan(ctx, fetcher, date)
		if err != nil {
			// No trading-day data within the 7-day window: leave the field empty.
			result.SkippedNoData++
			continue
		}

		if dryRun {
			result.Backfilled++
			continue
		}

		point := marketdata.MacroDataPoint{
			Symbol:    marketVolumeSymbol,
			Value:     round2(vol.MarketVolume),
			ChangePct: 0,
			Timestamp: dateInUTC(sourceDate).Unix(),
		}
		pointBytes, err := json.Marshal(point)
		if err != nil {
			result.Errors++
			continue
		}
		merged, err := appendSnapshotKey(raw, "market_volume", pointBytes)
		if err != nil {
			result.Errors++
			continue
		}
		if err := writeFileAtomic(snapPath, merged); err != nil {
			result.Errors++
			continue
		}
		result.Backfilled++
	}
	return result, nil
}

// fetchWithBackscan returns the market volume for the most recent trading day
// within the 7 calendar days ending at date (inclusive), mirroring
// MarketVolumeProvider.FetchLatest's 7-day scan. Each scanned date is retried
// up to maxBackfillAttempts times for transient errors.
func fetchWithBackscan(ctx context.Context, fetcher MarketVolumeFetcher, date time.Time) (*marketdata.MarketVolumeResult, time.Time, error) {
	for i := range 7 {
		candidate := date.AddDate(0, 0, -i)
		res, err := fetchWithRetry(ctx, fetcher, candidate)
		if err == nil {
			return res, candidate, nil
		}
	}
	return nil, time.Time{}, fmt.Errorf("no market volume data within 7 days of %s", date.Format("2006-01-02"))
}

// fetchWithRetry attempts FetchDate up to maxBackfillAttempts times with a
// short backoff. Only the first error is surfaced after retries are exhausted.
func fetchWithRetry(ctx context.Context, fetcher MarketVolumeFetcher, date time.Time) (*marketdata.MarketVolumeResult, error) {
	var lastErr error
	for attempt := 1; attempt <= maxBackfillAttempts; attempt++ {
		res, err := fetcher.FetchDate(ctx, date.Format("20060102"))
		if err == nil {
			return res, nil
		}
		lastErr = err
		if attempt < maxBackfillAttempts {
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	return nil, lastErr
}

// snapshotDatesInRange returns the snapshot dates (YYYY-MM-DD.json) in dir that
// fall inside [start, end]. Special files (_metadata.json, latest.json,
// previous.json) are ignored.
func snapshotDatesInRange(dir string, start, end time.Time) ([]time.Time, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read snapshot dir %s: %w", dir, err)
	}
	loc, _ := time.LoadLocation("Asia/Taipei")
	var dates []time.Time
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") || !isSnapshotDateName(name) {
			continue
		}
		d, err := time.ParseInLocation("2006-01-02", strings.TrimSuffix(name, ".json"), loc)
		if err != nil {
			continue
		}
		if d.Before(start) || d.After(end) {
			continue
		}
		dates = append(dates, d)
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
	return dates, nil
}

// isSnapshotDateName reports whether name is a dated snapshot like
// "2026-07-28.json" (not "_metadata.json" / "latest.json" / "previous.json").
func isSnapshotDateName(name string) bool {
	if len(name) != len("2006-01-02.json") {
		return false
	}
	if !strings.HasPrefix(name, "20") {
		return false
	}
	return name[4] == '-' && name[7] == '-'
}

// snapshotHasNonZeroPoint reports whether raw contains key with a non-zero
// value (an existing real reading). A zero-value sentinel is treated as missing.
func snapshotHasNonZeroPoint(raw []byte, key string) (bool, error) {
	var snap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &snap); err != nil {
		return false, fmt.Errorf("parse snapshot: %w", err)
	}
	rawPt, ok := snap[key]
	if !ok {
		return false, nil
	}
	var pt marketdata.MacroDataPoint
	if err := json.Unmarshal(rawPt, &pt); err != nil {
		return false, nil
	}
	return pt.Symbol != "" || pt.Value != 0 || pt.Timestamp != 0, nil
}

// appendSnapshotKey inserts key:value at the end of the top-level JSON object,
// preserving the original bytes (indentation and key order) so diffs show only
// the new key. Mirrors cmd/macrobackfill's rewriteMergePreservingOrder.
func appendSnapshotKey(raw []byte, key string, value json.RawMessage) ([]byte, error) {
	end := len(raw)
	for end > 0 {
		if raw[end-1] == '}' {
			break
		}
		if raw[end-1] == ' ' || raw[end-1] == '\t' || raw[end-1] == '\r' || raw[end-1] == '\n' {
			end--
			continue
		}
		return nil, errors.New("expected top-level JSON object")
	}
	if end == 0 {
		return nil, errors.New("empty or non-object input")
	}
	prefix := end - 1
	for prefix > 0 && (raw[prefix-1] == ' ' || raw[prefix-1] == '\t' || raw[prefix-1] == '\r' || raw[prefix-1] == '\n') {
		prefix--
	}
	hasComma := prefix > 0 && raw[prefix-1] == ','
	keyBytes, err := json.Marshal(key)
	if err != nil {
		return nil, err
	}
	indent := detectSnapshotIndent(raw)

	var sb strings.Builder
	sb.Write(raw[:end-1])
	if !hasComma {
		sb.WriteByte(',')
	}
	sb.WriteByte('\n')
	sb.WriteString(indent)
	sb.Write(keyBytes)
	sb.WriteString(": ")
	sb.Write(value)
	sb.WriteByte('\n')
	sb.WriteByte('}')
	return []byte(sb.String()), nil
}

// detectSnapshotIndent returns the leading whitespace used by the first key of
// a pretty-printed JSON object (or "" for single-line objects).
func detectSnapshotIndent(raw []byte) string {
	openIdx := -1
	for i, b := range raw {
		if b == '{' {
			openIdx = i
			break
		}
	}
	if openIdx == -1 {
		return ""
	}
	nlIdx := -1
	for i := openIdx + 1; i < len(raw); i++ {
		if raw[i] == '\n' {
			nlIdx = i
			break
		}
		if raw[i] == '}' {
			return ""
		}
	}
	if nlIdx == -1 {
		return ""
	}
	var indent []byte
	for i := nlIdx + 1; i < len(raw); i++ {
		c := raw[i]
		if c == ' ' || c == '	' {
			indent = append(indent, c)
			continue
		}
		break
	}
	return string(indent)
}

// writeFileAtomic writes data to path via a temp file + rename.
func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write tmp %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename tmp: %w", err)
	}
	return nil
}

// parseDateArg parses YYYY-MM-DD in Asia/Taipei.
func parseDateArg(s string, loc *time.Location) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("empty date")
	}
	return time.ParseInLocation("2006-01-02", s, loc)
}

// dateInUTC returns the UTC midnight of the given (calendar) date, matching
// the gateway's timestamp convention (time.Parse("20060102", ...) → UTC).
func dateInUTC(d time.Time) time.Time {
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
}

// round2 rounds to 2 decimal places (億元 precision used by the snapshot).
func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}
