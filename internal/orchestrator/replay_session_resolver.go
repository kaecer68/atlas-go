package orchestrator

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"time"
)

// NewReplaySessionResolverFromCSV parses a replay CSV file whose first column
// is a YYYY-MM-DD trading date and returns a TradingSessionResolver that
// yields the next trading date strictly after asOf.
//
// The CSV is expected to follow the same schema as
// data/replay/tw_extended_90days.csv:
//
//	Date,Code,Name,TradeVolume,Open,High,Low,Close
//
// The full set of unique trading dates is loaded once at construction time
// and sorted ascending. Subsequent calls to NextTradingSession do an O(N)
// linear scan to find the first date strictly after asOf. N is bounded by
// the replay window size (≤90 days for the production dataset) so this is
// fine; if the dataset grows much larger, replace with a bsearch or cursor.
//
// Construction is fail-fast: malformed date rows are skipped, but a missing
// file, an unreadable file, a header-only file, or zero parsed dates all
// return an error. Callers in main.go fall back to NoOpNextSessionResolver
// when this constructor returns an error so that spec §8.2 fail-closed
// semantics are preserved.
func NewReplaySessionResolverFromCSV(csvPath string) (TradingSessionResolver, error) {
	f, err := os.Open(csvPath)
	if err != nil {
		return nil, fmt.Errorf("open replay csv %s: %w", csvPath, err)
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(f)
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse replay csv %s: %w", csvPath, err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("replay csv %s has <2 rows (header + data)", csvPath)
	}

	seen := map[string]struct{}{}
	var dates []time.Time
	for i, row := range rows[1:] {
		if len(row) == 0 {
			continue
		}
		d, perr := time.Parse("2006-01-02", row[0])
		if perr != nil {
			// Skip malformed rows; do not abort the whole load.
			continue
		}
		key := row[0]
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		dates = append(dates, d)
		_ = i // keep linter happy; reserved for future per-row debug logging
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })

	if len(dates) == 0 {
		return nil, fmt.Errorf("replay csv %s has 0 valid dates", csvPath)
	}

	return &csvReplayResolver{dates: dates}, nil
}

// csvReplayResolver is the concrete TradingSessionResolver backed by a
// pre-parsed slice of trading dates. It is unexported because callers should
// always go through NewReplaySessionResolverFromCSV, which handles CSV
// parsing and validation.
type csvReplayResolver struct {
	dates []time.Time
}

// NextTradingSession returns the first date in the underlying slice that is
// strictly after asOf. When asOf is at or past the last known trading date
// (i.e. the replay dataset is exhausted), it returns ErrSessionUnavailable
// so spec §8.2 fail-closed semantics are preserved.
func (r *csvReplayResolver) NextTradingSession(asOf time.Time) (time.Time, error) {
	for _, d := range r.dates {
		if d.After(asOf) {
			return d, nil
		}
	}
	return time.Time{}, fmt.Errorf(
		"%w: no date after %s in replay csv (have %d dates, last %s)",
		ErrSessionUnavailable,
		asOf.Format("2006-01-02"),
		len(r.dates),
		lastDateOrEmpty(r.dates),
	)
}

// lastDateOrEmpty formats the last date in the slice for the error message,
// or "<empty>" when the slice is empty. Kept tiny to avoid pulling in a
// date-formatting library in the hot error path.
func lastDateOrEmpty(dates []time.Time) string {
	if len(dates) == 0 {
		return "<empty>"
	}
	return dates[len(dates)-1].Format("2006-01-02")
}
