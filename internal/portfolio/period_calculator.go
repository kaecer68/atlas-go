package portfolio

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// Minimum trading days required for each computed indicator.
// If historical data is shorter than the requirement, the field
// is left at its zero value (honest degradation).
const (
	MinDaysTAIEXMA5         = 5
	MinDaysTAIEXMA20        = 20
	MinDaysTAIEXMA20Slope   = 24 // 20 for MA20 + 5 MA20 values for regression
	MinDaysSOXMA50          = 50
	MinDaysSOXMA20          = 20
	MinDaysTSMADRHigh5      = 5
	MinDaysMarketVolumeMA20 = 20
	MinDaysTWDMA20          = 20
	MinDaysTWDChange1D      = 2 // need today + yesterday to compute 1-day change
	MinDaysTWDChange3D      = 4 // need today + 3 prior days
	MinDaysTWDChange5D      = 6 // need today + 5 prior days
)

// PeriodIndicatorsCalculator enriches a base PeriodIndicators (already
// populated with single-day snapshot fields) with rolling-window computed
// indicators from historical macro snapshots.
//
// Zero values are returned when historical data is insufficient — the
// detector's existing >0 guards in InputAvailable will naturally treat
// the field as unavailable.
type PeriodIndicatorsCalculator struct{}

// NewCalculator returns a ready-to-use calculator. The calculator is
// stateless — all state comes from the snapshot history passed to Enrich.
func NewCalculator() *PeriodIndicatorsCalculator {
	return &PeriodIndicatorsCalculator{}
}

// SnapshotEntry is a minimal view of a historical macro snapshot for
// calculator consumption. It carries only the fields needed by Batch 1.
type SnapshotEntry struct {
	TradingDate  string
	TAIEX        float64
	SOX          float64
	TSMADR       float64
	USDTWD       float64
	MarketVolume float64
}

// EntriesFromSnapshots extracts the minimal fields from a slice of
// MacroDataSnapshot. The caller must provide snapshots sorted by trading
// date ascending (oldest first), with the current day as the last element.
func EntriesFromSnapshots(snapshots []marketdata.MacroDataSnapshot) []SnapshotEntry {
	entries := make([]SnapshotEntry, len(snapshots))
	for i, s := range snapshots {
		entries[i] = SnapshotEntry{
			TAIEX:        s.TAIEX.Value,
			SOX:          s.SOXIndex.Value,
			TSMADR:       s.TSMADR.Value,
			USDTWD:       s.USD_TWD.Value,
			MarketVolume: s.MarketVolume.Value,
		}
	}
	return entries
}

// Enrich fills Batch 1 computed fields on the PeriodIndicators using
// historical entries. `entries` must be sorted trading date ascending
// with the current day as the last element.
//
// Fields not computable due to insufficient history are left at zero.
func (c *PeriodIndicatorsCalculator) Enrich(ind *PeriodIndicators, entries []SnapshotEntry) {
	n := len(entries)

	// TAIEX moving averages
	if n >= MinDaysTAIEXMA5 {
		ind.TAIEXMA5 = computeSMA(entries, n-MinDaysTAIEXMA5, n-1, func(e SnapshotEntry) float64 { return e.TAIEX })
	}
	if n >= MinDaysTAIEXMA20 {
		ind.TAIEXMA20 = computeSMA(entries, n-MinDaysTAIEXMA20, n-1, func(e SnapshotEntry) float64 { return e.TAIEX })
	}
	// TAIEXMA20Slope: linear regression of MA20 values over the last 5 days
	if n >= MinDaysTAIEXMA20Slope {
		ind.TAIEXMA20Slope = computeSlope(entries, n-MinDaysTAIEXMA20Slope, func(e SnapshotEntry) float64 { return e.TAIEX })
	}

	// SOX moving averages
	if n >= MinDaysSOXMA20 {
		ind.SOXMA20 = computeSMA(entries, n-MinDaysSOXMA20, n-1, func(e SnapshotEntry) float64 { return e.SOX })
	}
	if n >= MinDaysSOXMA50 {
		ind.SOXMA50 = computeSMA(entries, n-MinDaysSOXMA50, n-1, func(e SnapshotEntry) float64 { return e.SOX })
	}

	// TSM ADR 5-day high
	if n >= MinDaysTSMADRHigh5 {
		ind.TSMADRHigh5 = computeMax(entries, n-MinDaysTSMADRHigh5, n-1, func(e SnapshotEntry) float64 { return e.TSMADR })
	}

	// Market volume 20-day MA
	if n >= MinDaysMarketVolumeMA20 {
		ind.MarketVolumeMA20 = computeSMA(entries, n-MinDaysMarketVolumeMA20, n-1, func(e SnapshotEntry) float64 { return e.MarketVolume })
	}

	// TWD moving average
	if n >= MinDaysTWDMA20 {
		ind.TWDMA20 = computeSMA(entries, n-MinDaysTWDMA20, n-1, func(e SnapshotEntry) float64 { return e.USDTWD })
	}

	// TWD changes: (today - N ago) / N ago * 100; positive = depreciation
	if n >= MinDaysTWDChange1D {
		ind.TWDChange1D = computeChangePct(entries, n-MinDaysTWDChange1D, n-1, func(e SnapshotEntry) float64 { return e.USDTWD })
	}
	if n >= MinDaysTWDChange3D {
		ind.TWDChange3D = computeChangePct(entries, n-MinDaysTWDChange3D, n-1, func(e SnapshotEntry) float64 { return e.USDTWD })
	}
	if n >= MinDaysTWDChange5D {
		ind.TWDChange5D = computeChangePct(entries, n-MinDaysTWDChange5D, n-1, func(e SnapshotEntry) float64 { return e.USDTWD })
	}
}

// EnrichFromDir reads the most recent snapshot files from snapshotDir,
// extracts the minimal fields, and enriches the PeriodIndicators.
//
// tradingDate is the date for which we are computing indicators.
// It reads up to minDaysNeeded snapshots dated ≤ tradingDate, sorted
// ascending. If the directory has fewer than minDaysNeeded snapshots,
// some fields will remain at zero (honest degradation).
//
// The minDaysNeeded is the maximum of all MinDays constants listed above.
func (c *PeriodIndicatorsCalculator) EnrichFromDir(ind *PeriodIndicators, tradingDate string, snapshotDir string) (err error) {
	entries, err := loadRecentSnapshots(snapshotDir, tradingDate, maxRequiredDays())
	if err != nil {
		return err
	}
	c.Enrich(ind, entries)
	return nil
}

// maxRequiredDays returns the maximum of all MinDays constants.
func maxRequiredDays() int {
	max := MinDaysTAIEXMA5
	for _, d := range []int{
		MinDaysTAIEXMA20, MinDaysTAIEXMA20Slope,
		MinDaysSOXMA50, MinDaysSOXMA20,
		MinDaysTSMADRHigh5,
		MinDaysMarketVolumeMA20,
		MinDaysTWDMA20,
		MinDaysTWDChange1D, MinDaysTWDChange3D, MinDaysTWDChange5D,
	} {
		if d > max {
			max = d
		}
	}
	return max
}

// loadRecentSnapshots reads at most `maxFiles` snapshot files from
// `snapshotDir`, sorted ascending by date, with date ≤ tradingDate.
// Excludes latest.json, previous.json, and _metadata.json.
func loadRecentSnapshots(snapshotDir, tradingDate string, maxFiles int) ([]SnapshotEntry, error) {
	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		return nil, err
	}

	// Collect dated snapshot files.
	var dates []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "20") || !strings.HasSuffix(name, ".json") {
			continue
		}
		// Skip non-dated meta files.
		if name == "latest.json" || name == "previous.json" || name == "_metadata.json" {
			continue
		}
		date := strings.TrimSuffix(name, ".json")
		if len(date) != 10 || date[4] != '-' || date[7] != '-' {
			continue // not a valid YYYY-MM-DD name
		}
		if date > tradingDate {
			continue
		}
		dates = append(dates, date)
	}

	if len(dates) == 0 {
		return nil, nil
	}

	// Sort descending, then take the most recent maxFiles.
	sort.Slice(dates, func(i, j int) bool { return dates[i] > dates[j] })
	if len(dates) > maxFiles {
		dates = dates[:maxFiles]
	}

	// Sort ascending for the calculator.
	sort.Strings(dates)

	result := make([]SnapshotEntry, 0, len(dates))
	for _, date := range dates {
		path := filepath.Join(snapshotDir, date+".json")
		data, err := os.ReadFile(path)
		if err != nil {
			continue // skip unreadable files (per CF-MS-03)
		}
		var snap marketdata.MacroDataSnapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			continue
		}
		result = append(result, SnapshotEntry{
			TradingDate:  date,
			TAIEX:        snap.TAIEX.Value,
			SOX:          snap.SOXIndex.Value,
			TSMADR:       snap.TSMADR.Value,
			USDTWD:       snap.USD_TWD.Value,
			MarketVolume: snap.MarketVolume.Value,
		})
	}
	return result, nil
}

// ── internal helpers ──

// computeSMA returns the simple moving average of values extracted from
// entries[start..end] (inclusive).
func computeSMA(entries []SnapshotEntry, start, end int, fn func(SnapshotEntry) float64) float64 {
	sum := 0.0
	count := 0
	for i := start; i <= end; i++ {
		v := fn(entries[i])
		if v == 0 {
			continue // skip missing data points
		}
		sum += v
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

// computeMax returns the maximum value extracted from entries[start..end] (inclusive).
func computeMax(entries []SnapshotEntry, start, end int, fn func(SnapshotEntry) float64) float64 {
	maxVal := 0.0
	for i := start; i <= end; i++ {
		v := fn(entries[i])
		if v > maxVal {
			maxVal = v
		}
	}
	return maxVal
}

// computeChangePct returns (value at end - value at start) / value at start * 100.
// Returns 0 if either value is zero.
func computeChangePct(entries []SnapshotEntry, start, end int, fn func(SnapshotEntry) float64) float64 {
	old := fn(entries[start])
	new := fn(entries[end])
	if old == 0 || new == 0 {
		return 0
	}
	return (new - old) / old * 100
}

// computeSlope calculates the linear regression slope of MA20 values derived
// from a rolling window. For each of the last 5 MA20 windows (days
// 19,18,17,16,15 prior to end), we compute the MA20 of the preceding 20 days
// and fit a line to those 5 MA20 values.
//
// This is a simplified approach: we extract 5 MA20 data points from the
// window and run linear regression. Each MA20 point requires 20 days, and
// we need 5 such points → 24 days total (20 + 4 forward offsets).
func computeSlope(entries []SnapshotEntry, start int, fn func(SnapshotEntry) float64) float64 {
	// We have at least MinDaysTAIEXMA20Slope (24) entries.
	// MA20 for day 19 (index start+19) uses days start+0..start+19
	// MA20 for day 20 uses start+1..start+20, ..., day 23 uses start+4..start+23 (=end)
	ma20vals := make([]float64, 5)
	for i := 0; i < 5; i++ {
		ma20vals[i] = computeSMA(entries, start+i, start+i+19, fn)
	}

	// Linear regression: x = [0,1,2,3,4], y = ma20vals
	// slope = (n*Σxy - Σx*Σy) / (n*Σx² - (Σx)²)
	n := float64(5)
	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumXX := 0.0
	for i := 0; i < 5; i++ {
		x := float64(i)
		y := ma20vals[i]
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	denom := n*sumXX - sumX*sumX
	if math.Abs(denom) < 1e-12 {
		return 0
	}
	return (n*sumXY - sumX*sumY) / denom
}
