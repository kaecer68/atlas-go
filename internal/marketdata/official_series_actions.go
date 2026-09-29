package marketdata

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// This file derives backward-adjustment corporate actions from an OFFICIAL
// adjusted price series (FinMind `TaiwanStockPriceAdj`) instead of from a
// dividend-event feed.
//
// Why derived actions (evidence, 2026-09-30, ticket #2151):
//   - The replay/extended price series (data/replay/tw_extended_90days.jsonl)
//     is UNADJUSTED. Index-style consumers (MomentumReturn, Volatility) inherit
//     the result: `6669.TW` shows a -66.5% single-day cliff on 2026-09-02
//     (7800 -> 2610, a stock dividend), `0050.TW` -74.8% on 2025-06-18 and
//     `0052.TW` -85.6% on 2025-11-26 (ETF splits).
//   - The dividend-event feed (FinMind `TaiwanStockDividend`) carries the
//     individual-stock stock dividend but NOT the 2025 ETF splits: with 14 cash
//     actions only, `0050.TW` still shows a 74.78% cliff after adjustment.
//   - The official adjusted series smooths every one of those events
//     (max |daily return| = 10.0% = the Taiwan limit) and, on the common window,
//     reproduces the raw series' returns exactly apart from the events.
//
// The derivation keeps the existing (audited) adjustment semantics: the LATEST
// close is preserved and only pre-event prices are rewritten. Actions are fed
// to portfolio.HistoricalPrices.AdjustForCorporateActions, which computes
//     factor = ReferencePrice / postEventRawPrice
// for each action, so the emitted ReferencePrice is the raw post-event close
// scaled by the ratio change observed in the official series. Applying the
// emitted actions in ExDate order telescopes to
//     adjusted[d] = raw[d] * (ratio[d] / ratio[last])
// which is the official series rescaled so that the newest bar keeps its raw
// value.

// OfficialSeriesActionSource is the provenance tag for actions derived from an
// official adjusted price series.
const OfficialSeriesActionSource = "official_series_derived"

// DefaultOfficialAdjustedRatioTolerance is the relative tolerance used when
// deciding that the official/raw price ratio changed between two trading days.
//
// The ratio is piecewise constant between corporate actions, so the tolerance
// only needs to absorb rounding noise. Calibration (2026-09-30, replay window
// 2020-01-02..2026-09-04): the largest day-to-day ratio drift on non-event days
// was 1.2e-7 (0050.TW: 4.8e-8, 0052.TW: 1.2e-7, 0056.TW: 5.6e-8, 6669.TW:
// 6.5e-10), while the smallest real event observed was +2.2% (0050.TW
// 2021-01-22 cash dividend) and the largest was +600% (0052.TW 2025-11-26
// split). 1e-6 therefore separates noise from events by five orders of
// magnitude.
const DefaultOfficialAdjustedRatioTolerance = 1e-6

// DatedClose is a (date, close) pair.
type DatedClose struct {
	Date  time.Time
	Close float64
}

// LoadDatedClosesJSONL reads a FinMind-shaped JSONL file where each line carries
// at least `date` (YYYY-MM-DD) and `close`. Lines that are malformed, that carry
// a non-positive close, or that repeat a date (the last one wins) are tolerated.
func LoadDatedClosesJSONL(path string) ([]DatedClose, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open dated closes: %w", err)
	}
	defer func() { _ = f.Close() }()

	byDate := make(map[time.Time]float64)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var rec struct {
			Date  string  `json:"date"`
			Close float64 `json:"close"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue // skip malformed lines
		}
		t, err := time.Parse("2006-01-02", strings.TrimSpace(rec.Date))
		if err != nil {
			continue
		}
		if rec.Close <= 0 {
			continue
		}
		byDate[t] = rec.Close
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan dated closes: %w", err)
	}

	out := make([]DatedClose, 0, len(byDate))
	for t, c := range byDate {
		out = append(out, DatedClose{Date: t, Close: c})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out, nil
}

// LoadOfficialAdjustedSeriesDir loads every `<SYMBOL>.jsonl` under dir and
// returns the series keyed by the file name without its extension (so the key
// matches the symbol spelling used by the caller, e.g. "0050.TW").
//
// A missing directory is reported as an error; callers treat that as
// "adjustment unavailable" and must keep the unadjusted prices (this is the
// documented rollback path).
func LoadOfficialAdjustedSeriesDir(dir string) (map[string][]DatedClose, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read official adjusted dir %s: %w", dir, err)
	}
	out := make(map[string][]DatedClose)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		series, err := LoadDatedClosesJSONL(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if len(series) == 0 {
			continue
		}
		out[strings.TrimSuffix(entry.Name(), ".jsonl")] = series
	}
	if len(out) == 0 {
		return nil, errors.New("official adjusted dir holds no usable series: " + dir)
	}
	return out, nil
}

// ActionsFromOfficialSeries derives the backward-adjustment actions for one
// symbol by comparing the official adjusted series (`official`) with the raw
// series (`raw`).
//
// Rule: walk the raw series in date order; on every raw date that also exists in
// the official series, form ratio = official/raw. Whenever that ratio changes
// (relative change above tolerance), emit one action dated at that raw date with
//
//	ReferencePrice = raw[date] * previousRatio / ratio
//
// so that AdjustForCorporateActions rewrites every earlier price by
// previousRatio/ratio. Raw dates missing from the official series are skipped
// entirely (they never produce an action, and they do not advance the ratio), so
// a missing official bar cannot fabricate an event.
//
// The returned slice is sorted by ExDate ascending, as AdjustForCorporateActions
// requires. No action is emitted for a symbol whose ratio never changes.
func ActionsFromOfficialSeries(symbol string, raw, official []DatedClose, tolerance float64) []domain.CorporateAction {
	if len(raw) == 0 || len(official) == 0 {
		return nil
	}
	if tolerance <= 0 {
		tolerance = DefaultOfficialAdjustedRatioTolerance
	}

	officialByDate := make(map[time.Time]float64, len(official))
	for _, o := range official {
		if o.Close > 0 {
			officialByDate[o.Date] = o.Close
		}
	}

	rawCopy := make([]DatedClose, 0, len(raw))
	for _, r := range raw {
		if r.Close > 0 {
			rawCopy = append(rawCopy, r)
		}
	}
	sort.Slice(rawCopy, func(i, j int) bool { return rawCopy[i].Date.Before(rawCopy[j].Date) })

	var actions []domain.CorporateAction
	previousRatio := 0.0
	for _, r := range rawCopy {
		officialClose, ok := officialByDate[r.Date]
		if !ok {
			continue
		}
		ratio := officialClose / r.Close
		if previousRatio == 0 {
			previousRatio = ratio
			continue
		}
		if relChange := (ratio - previousRatio) / previousRatio; relChange > tolerance || relChange < -tolerance {
			referencePrice := r.Close * (previousRatio / ratio)
			if referencePrice > 0 {
				actions = append(actions, domain.CorporateAction{
					Symbol:         symbol,
					ExDate:         r.Date,
					ReferencePrice: referencePrice,
					Source:         OfficialSeriesActionSource,
				})
			}
			previousRatio = ratio
			continue
		}
		previousRatio = ratio
	}
	return actions
}
