// Package aggregate.go — PR 1c win-rate aggregation.
//
// Groups raw SignalOutcome rows by (symbol, source) into StockWinRateSummary
// and persists them into stock_win_rate (upsert) plus a state JSON snapshot
// (data/state/stock_win_rate.json), mirroring the SQLite-facts + JSON-snapshot
// dual-track convention used by darwinian_weights.json.
//
// The grouping uses the internal aggregateWinRate helper (PR 1b) which skips
// the single-symbol/single-source homogeneity checks, because per-key groups
// are already homogeneous by construction.
package stockpicker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// GroupAndSummarize groups outcomes by (symbol, source) and computes one
// StockWinRateSummary per key. The window label (e.g. "120d") is carried into
// the summary key; it does not filter the input (callers filter with
// LoadOutcomesAsOf). Empty input returns an empty slice.
func GroupAndSummarize(outcomes []SignalOutcome, window string, costRate float64, minSamples int, confidence float64) []StockWinRateSummary {
	byKey := make(map[string][]SignalOutcome)
	keys := make([]string, 0)
	for _, o := range outcomes {
		key := o.Symbol + "\x00" + o.Source
		if _, ok := byKey[key]; !ok {
			keys = append(keys, key)
		}
		byKey[key] = append(byKey[key], o)
	}
	sort.Strings(keys)

	summaries := make([]StockWinRateSummary, 0, len(keys))
	for _, key := range keys {
		parts := splitKey(key)
		group := byKey[key]
		agg := aggregateWinRate(group, costRate, minSamples, confidence)
		summaries = append(summaries, StockWinRateSummary{
			Symbol:            parts[0],
			Source:            parts[1],
			Window:            window,
			Observations:      agg.Observations,
			Hits:              agg.Hits,
			WinRate:           agg.WinRate,
			WilsonLower:       agg.WilsonLower,
			WilsonUpper:       agg.WilsonUpper,
			Confidence:        agg.Confidence,
			CalibrationStatus: agg.CalibrationStatus,
			NetCostRate:       agg.NetCostRate,
			AvgForwardReturn:  agg.AvgForwardReturn,
		})
	}
	return summaries
}

// splitKey reverses the "\x00"-joined group key. Symbols are ASCII stock
// codes, so the separator cannot appear inside either part.
func splitKey(key string) [2]string {
	for i := 0; i < len(key); i++ {
		if key[i] == 0 {
			return [2]string{key[:i], key[i+1:]}
		}
	}
	return [2]string{key, ""}
}

// AggregateFromStore loads all outcomes (optionally restricted to a rolling
// window relative to asOf), groups them, upserts the per-key summaries into
// the win-rate store, and returns the summaries.
func AggregateFromStore(ctx context.Context, outcomesStore *SignalOutcomeStore, winStore *WinRateStore, window string, costRate float64, minSamples int, confidence float64, asOf time.Time) ([]StockWinRateSummary, error) {
	outcomes, err := LoadOutcomesAsOf(ctx, outcomesStore.db, "", "", window, asOf)
	if err != nil {
		return nil, fmt.Errorf("stockpicker: aggregate load outcomes: %w", err)
	}
	summaries := GroupAndSummarize(outcomes, window, costRate, minSamples, confidence)
	for _, s := range summaries {
		if err := winStore.SaveWinRate(ctx, s); err != nil {
			return nil, fmt.Errorf("stockpicker: aggregate save %s/%s/%s: %w", s.Symbol, s.Source, s.Window, err)
		}
	}
	return summaries, nil
}

// StateJSON is the on-disk snapshot shape of data/state/stock_win_rate.json.
type StateJSON struct {
	UpdatedAt string                `json:"updated_at"`
	AsOf      string                `json:"as_of"`
	Window    string                `json:"window"`
	Eligible  int                   `json:"eligible_keys"`
	Summaries []StockWinRateSummary `json:"summaries"`
}

// WriteStateJSON writes summaries to path (data/state/stock_win_rate.json)
// following the darwinian_weights.json snapshot convention. asOf is the PIT
// cutoff of the aggregation run.
func WriteStateJSON(path string, summaries []StockWinRateSummary, asOf time.Time) error {
	eligible := 0
	for _, s := range summaries {
		if s.CalibrationStatus == CalibrationEligible {
			eligible++
		}
	}
	snap := StateJSON{
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		AsOf:      asOf.Format("2006-01-02"),
		Eligible:  eligible,
		Summaries: summaries,
	}
	if len(summaries) > 0 {
		snap.Window = summaries[0].Window
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("stockpicker: marshal state json: %w", err)
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("stockpicker: mkdir state dir: %w", err)
		}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("stockpicker: write state json: %w", err)
	}
	return nil
}
