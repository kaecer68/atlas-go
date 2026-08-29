package portfolio

import (
	"encoding/json"
	"log"
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

	// B5 Batch 2: foreign capital
	MinDaysForeignNet5DayAvg    = 5
	MinDaysForeignNet10DayAvg   = 10
	MinDaysForeignNetPeakSell   = 10 // need at least 10 days for meaningful peak sell
	MinDaysForeignBuySellDays   = 10
	MinDaysForeignConsecDays    = 10 // need 10 days to establish consecutive buy/sell
	MinDaysForeignFuturesOIPrev = 2  // need today + yesterday
	MinDaysForeignFuturesDelta3 = 4  // need today + 3 prior days

	// B5 Batch 2: margin
	MinDaysMarginBalancePeak     = 30 // need 30 days of margin history for meaningful peak
	MinDaysMarginBalanceChange5D = 6  // need today + 5 prior days

	// B5 Batch 3: sector rotation + public bank
	MinDaysSectorRotationFlag      = 10 // need 10 days of sector index history (5d for current + 5d for prior)
	MinDaysPublicBankConsecBuyDays = 5  // need 5 days of public-bank history for meaningful consecutive-buy signal
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
// calculator consumption. It carries fields needed by Batch 1 and Batch 2.
type SnapshotEntry struct {
	TradingDate         string
	TAIEX               float64
	SOX                 float64
	TSMADR              float64
	USDTWD              float64
	MarketVolume        float64
	ForeignInvestorNet  float64 // B5-2: 外資現貨買賣超
	ForeignFuturesOINet float64 // B5-2: 外資期貨未平倉淨額
}

// MarginEntry is a simplified margin history entry for calculator consumption.
type MarginEntry struct {
	Date          string
	MarginBalance float64
}

// EntriesFromSnapshots extracts the minimal fields from a slice of
// MacroDataSnapshot. The caller must provide snapshots sorted by trading
// date ascending (oldest first), with the current day as the last element.
func EntriesFromSnapshots(snapshots []marketdata.MacroDataSnapshot) []SnapshotEntry {
	entries := make([]SnapshotEntry, len(snapshots))
	for i, s := range snapshots {
		entries[i] = SnapshotEntry{
			TAIEX:               s.TAIEX.Value,
			SOX:                 s.SOXIndex.Value,
			TSMADR:              s.TSMADR.Value,
			USDTWD:              s.USD_TWD.Value,
			MarketVolume:        s.MarketVolume.Value,
			ForeignInvestorNet:  s.ForeignInvestorNet.Value,
			ForeignFuturesOINet: s.ForeignFuturesOINet.Value,
		}
	}
	return entries
}

// Enrich fills computed fields on the PeriodIndicators using
// historical entries. `entries` must be sorted trading date ascending
// with the current day as the last element.
//
// Fields not computable due to insufficient history are left at zero.
func (c *PeriodIndicatorsCalculator) Enrich(ind *PeriodIndicators, entries []SnapshotEntry) {
	n := len(entries)

	// TAIEX moving averages
	if n >= MinDaysTAIEXMA5 {
		ind.TAIEXMA5 = computeSMA(entries, n-MinDaysTAIEXMA5, n-1, MinDaysTAIEXMA5, func(e SnapshotEntry) float64 { return e.TAIEX })
	}
	if n >= MinDaysTAIEXMA20 {
		ind.TAIEXMA20 = computeSMA(entries, n-MinDaysTAIEXMA20, n-1, MinDaysTAIEXMA20, func(e SnapshotEntry) float64 { return e.TAIEX })
	}
	// TAIEXMA20Slope: linear regression of MA20 values over the last 5 days
	if n >= MinDaysTAIEXMA20Slope {
		ind.TAIEXMA20Slope = computeSlope(entries, n-MinDaysTAIEXMA20Slope, func(e SnapshotEntry) float64 { return e.TAIEX })
	}

	// SOX moving averages
	if n >= MinDaysSOXMA20 {
		ind.SOXMA20 = computeSMA(entries, n-MinDaysSOXMA20, n-1, MinDaysSOXMA20, func(e SnapshotEntry) float64 { return e.SOX })
	}
	if n >= MinDaysSOXMA50 {
		ind.SOXMA50 = computeSMA(entries, n-MinDaysSOXMA50, n-1, MinDaysSOXMA50, func(e SnapshotEntry) float64 { return e.SOX })
	}

	// TSM ADR 5-day high
	if n >= MinDaysTSMADRHigh5 {
		ind.TSMADRHigh5 = computeMax(entries, n-MinDaysTSMADRHigh5, n-1, MinDaysTSMADRHigh5, func(e SnapshotEntry) float64 { return e.TSMADR })
	}

	// Market volume 20-day MA
	if n >= MinDaysMarketVolumeMA20 {
		ind.MarketVolumeMA20 = computeSMA(entries, n-MinDaysMarketVolumeMA20, n-1, MinDaysMarketVolumeMA20, func(e SnapshotEntry) float64 { return e.MarketVolume })
	}

	// TWD moving average
	if n >= MinDaysTWDMA20 {
		ind.TWDMA20 = computeSMA(entries, n-MinDaysTWDMA20, n-1, MinDaysTWDMA20, func(e SnapshotEntry) float64 { return e.USDTWD })
	}

	// TWD changes: (today - N ago) / N ago * 100; positive = depreciation
	if n >= MinDaysTWDChange1D {
		ind.TWDChange1D = computeChangePct(entries, n-MinDaysTWDChange1D, n-1, MinDaysTWDChange1D, func(e SnapshotEntry) float64 { return e.USDTWD })
	}
	if n >= MinDaysTWDChange3D {
		ind.TWDChange3D = computeChangePct(entries, n-MinDaysTWDChange3D, n-1, MinDaysTWDChange3D, func(e SnapshotEntry) float64 { return e.USDTWD })
	}
	if n >= MinDaysTWDChange5D {
		ind.TWDChange5D = computeChangePct(entries, n-MinDaysTWDChange5D, n-1, MinDaysTWDChange5D, func(e SnapshotEntry) float64 { return e.USDTWD })
	}

	// ── B5 Batch 2: Foreign capital ──
	if n >= MinDaysForeignNet5DayAvg {
		ind.ForeignNet5DayAvg = computeSMA(entries, n-MinDaysForeignNet5DayAvg, n-1, MinDaysForeignNet5DayAvg,
			func(e SnapshotEntry) float64 { return e.ForeignInvestorNet })
	}
	if n >= MinDaysForeignNet10DayAvg {
		ind.ForeignNet10DayAvg = computeSMA(entries, n-MinDaysForeignNet10DayAvg, n-1, MinDaysForeignNet10DayAvg,
			func(e SnapshotEntry) float64 { return e.ForeignInvestorNet })
	}
	if n >= MinDaysForeignNetPeakSell {
		// Peak sell = most negative net value (minimum) in the window.
		// computeMinSell returns the minimum (most negative) ForeignInvestorNet.
		ind.ForeignNetPeakSell = computeMinSell(entries, n-MinDaysForeignNetPeakSell, n-1, MinDaysForeignNetPeakSell,
			func(e SnapshotEntry) float64 { return e.ForeignInvestorNet })
	}
	if n >= MinDaysForeignBuySellDays {
		ind.ForeignBuyDays10 = computePositiveDays(entries, n-MinDaysForeignBuySellDays, n-1,
			func(e SnapshotEntry) float64 { return e.ForeignInvestorNet })
		ind.ForeignSellDays10 = computeNegativeDays(entries, n-MinDaysForeignBuySellDays, n-1,
			func(e SnapshotEntry) float64 { return e.ForeignInvestorNet })
	}
	if n >= MinDaysForeignConsecDays {
		ind.ForeignConsecBuyDays = computeConsecutiveDays(entries, n-MinDaysForeignConsecDays, n-1, true,
			func(e SnapshotEntry) float64 { return e.ForeignInvestorNet })
		ind.ForeignConsecSellDays = computeConsecutiveDays(entries, n-MinDaysForeignConsecDays, n-1, false,
			func(e SnapshotEntry) float64 { return e.ForeignInvestorNet })
	}

	// ── B5 Batch 2: Futures ──
	if n >= MinDaysForeignFuturesOIPrev && n >= 2 {
		// Previous day futures OI = entry at n-2
		//nolint:gosec // n>=2 guaranteed by guard above (MinDaysForeignFuturesOIPrev=2)
		ind.ForeignFuturesOIPrev = entries[n-2].ForeignFuturesOINet
	}
	if n >= MinDaysForeignFuturesDelta3 {
		// OI delta direction over 3 days (count day-over-day increases minus decreases)
		ind.ForeignFuturesOIDelta3 = computeFuturesDeltaDirection(entries, n-MinDaysForeignFuturesDelta3, n-1)
	}
}

// EnrichMargin computes margin-related indicators from margin history entries.
// marginEntries must be sorted by date ascending, with the current day as the
// last element.
//
// Fields not computable due to insufficient history are left at zero.
func (c *PeriodIndicatorsCalculator) EnrichMargin(ind *PeriodIndicators, marginEntries []MarginEntry) {
	n := len(marginEntries)

	// ── B5 Batch 2: Margin ──
	if n >= MinDaysMarginBalancePeak {
		ind.MarginBalancePeak = computeMarginPeak(marginEntries, n-MinDaysMarginBalancePeak, n-1, MinDaysMarginBalancePeak)
	}
	if n >= MinDaysMarginBalanceChange5D {
		ind.MarginBalanceChange5D = computeMarginChangePct(marginEntries, n-MinDaysMarginBalanceChange5D, n-1, MinDaysMarginBalanceChange5D)
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
		MinDaysForeignNet5DayAvg, MinDaysForeignNet10DayAvg,
		MinDaysForeignNetPeakSell, MinDaysForeignBuySellDays,
		MinDaysForeignConsecDays, MinDaysForeignFuturesOIPrev,
		MinDaysForeignFuturesDelta3,
		MinDaysMarginBalancePeak, MinDaysMarginBalanceChange5D,
		MinDaysSectorRotationFlag, MinDaysPublicBankConsecBuyDays,
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
			TradingDate:         date,
			TAIEX:               snap.TAIEX.Value,
			SOX:                 snap.SOXIndex.Value,
			TSMADR:              snap.TSMADR.Value,
			USDTWD:              snap.USD_TWD.Value,
			MarketVolume:        snap.MarketVolume.Value,
			ForeignInvestorNet:  snap.ForeignInvestorNet.Value,
			ForeignFuturesOINet: snap.ForeignFuturesOINet.Value,
		})
	}
	return result, nil
}

// ── internal helpers ──

// computeSMA returns the simple moving average of values extracted from
// entries[start..end] (inclusive). Returns 0 if the number of non-zero
// values in the window is less than minNonZero (W1 honest degradation).
func computeSMA(entries []SnapshotEntry, start, end int, minNonZero int, fn func(SnapshotEntry) float64) float64 {
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
	if count < minNonZero {
		return 0
	}
	return sum / float64(count)
}

// computeMax returns the maximum value extracted from entries[start..end] (inclusive).
// Returns 0 if the number of non-zero values in the window is less than minNonZero.
func computeMax(entries []SnapshotEntry, start, end int, minNonZero int, fn func(SnapshotEntry) float64) float64 {
	count := 0
	maxVal := 0.0
	for i := start; i <= end; i++ {
		v := fn(entries[i])
		if v == 0 {
			continue
		}
		count++
		if v > maxVal {
			maxVal = v
		}
	}
	if count < minNonZero {
		return 0
	}
	return maxVal
}

// computeChangePct returns (value at end - value at start) / value at start * 100.
// Returns 0 if any value is zero or if the non-zero count < minNonZero.
func computeChangePct(entries []SnapshotEntry, start, end int, minNonZero int, fn func(SnapshotEntry) float64) float64 {
	// Count non-zero values in the range
	count := 0
	for i := start; i <= end; i++ {
		if fn(entries[i]) != 0 {
			count++
		}
	}
	if count < minNonZero {
		return 0
	}
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
// ── B5 Batch 2 helpers ──

// computeMinSell returns the minimum (most negative) value in entries[start..end].
// Returns 0 if the number of non-zero values is less than minNonZero.
func computeMinSell(entries []SnapshotEntry, start, end int, minNonZero int, fn func(SnapshotEntry) float64) float64 {
	count := 0
	minVal := 0.0
	for i := start; i <= end; i++ {
		v := fn(entries[i])
		if v == 0 {
			continue
		}
		count++
		if v < minVal {
			minVal = v
		}
	}
	if count < minNonZero {
		return 0
	}
	return minVal
}

// computePositiveDays counts the number of entries in entries[start..end] where fn > 0.
func computePositiveDays(entries []SnapshotEntry, start, end int, fn func(SnapshotEntry) float64) int {
	count := 0
	for i := start; i <= end; i++ {
		v := fn(entries[i])
		if v > 0 {
			count++
		}
	}
	return count
}

// computeNegativeDays counts the number of entries in entries[start..end] where fn < 0.
func computeNegativeDays(entries []SnapshotEntry, start, end int, fn func(SnapshotEntry) float64) int {
	count := 0
	for i := start; i <= end; i++ {
		v := fn(entries[i])
		if v < 0 {
			count++
		}
	}
	return count
}

// computeConsecutiveDays counts consecutive days from the end of the range
// where fn values are positive (if wantPositive=true) or negative (if false).
func computeConsecutiveDays(entries []SnapshotEntry, start, end int, wantPositive bool, fn func(SnapshotEntry) float64) int {
	count := 0
	for i := end; i >= start; i-- {
		v := fn(entries[i])
		if v == 0 {
			break // zero breaks the chain
		}
		if wantPositive && v > 0 {
			count++
		} else if !wantPositive && v < 0 {
			count++
		} else {
			break
		}
	}
	return count
}

// computeFuturesDeltaDirection counts the net change direction of ForeignFuturesOINet.
// Iterates from end backwards, counting day-over-day increases minus decreases.
func computeFuturesDeltaDirection(entries []SnapshotEntry, start, end int) int {
	delta := 0
	for i := end; i > start; i-- {
		diff := entries[i].ForeignFuturesOINet - entries[i-1].ForeignFuturesOINet
		if diff > 0 {
			delta++
		} else if diff < 0 {
			delta--
		}
	}
	return delta
}

// computeMarginPeak returns the maximum margin balance in entries[start..end].
// Returns 0 if the non-zero count < minNonZero.
func computeMarginPeak(entries []MarginEntry, start, end int, minNonZero int) float64 {
	count := 0
	maxVal := 0.0
	for i := start; i <= end; i++ {
		v := entries[i].MarginBalance
		if v == 0 {
			continue
		}
		count++
		if v > maxVal {
			maxVal = v
		}
	}
	if count < minNonZero {
		return 0
	}
	return maxVal
}

// computeMarginChangePct returns (value at end - value at start) / value at start * 100.
// Returns 0 if non-zero count < minNonZero or either endpoint is zero.
func computeMarginChangePct(entries []MarginEntry, start, end int, minNonZero int) float64 {
	count := 0
	for i := start; i <= end; i++ {
		if entries[i].MarginBalance != 0 {
			count++
		}
	}
	if count < minNonZero {
		return 0
	}
	old := entries[start].MarginBalance
	new := entries[end].MarginBalance
	if old == 0 || new == 0 {
		return 0
	}
	return (new - old) / old * 100
}

func computeSlope(entries []SnapshotEntry, start int, fn func(SnapshotEntry) float64) float64 {
	// We have at least MinDaysTAIEXMA20Slope (24) entries.
	// MA20 for day 19 (index start+19) uses days start+0..start+19
	// MA20 for day 20 uses start+1..start+20, ..., day 23 uses start+4..start+23 (=end)
	ma20vals := make([]float64, 5)
	for i := range 5 {
		// Each MA20 requires 20 non-zero entries in its 20-day window.
		ma20vals[i] = computeSMA(entries, start+i, start+i+19, 20, fn)
	}

	// Linear regression: x = [0,1,2,3,4], y = ma20vals
	// slope = (n*Σxy - Σx*Σy) / (n*Σx² - (Σx)²)
	n := float64(5)
	sumX := 0.0
	sumY := 0.0
	sumXY := 0.0
	sumXX := 0.0
	for i := range 5 {
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

// ── B5 Batch 3: Sector rotation + Public bank consecutive buy ──

// SectorTopN returns the top-N industries by cumulative return over the
// given return map (industry -> return_pct), sorted descending. Ties are
// broken by industry name (alphabetical) for determinism.
func sectorTopN(returns map[string]float64, n int) []string {
	type kv struct {
		k string
		v float64
	}
	var arr []kv
	for k, v := range returns {
		arr = append(arr, kv{k, v})
	}
	sort.Slice(arr, func(i, j int) bool {
		if arr[i].v != arr[j].v {
			return arr[i].v > arr[j].v
		}
		return arr[i].k < arr[j].k
	})
	if len(arr) < n {
		n = len(arr)
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = arr[i].k
	}
	return out
}

// setsEqual returns true if two string sets contain the same elements
// (order-insensitive). Empty sets are equal.
func setsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]struct{}, len(a))
	for _, s := range a {
		m[s] = struct{}{}
	}
	for _, s := range b {
		if _, ok := m[s]; !ok {
			return false
		}
	}
	return true
}

// EnrichSectorRotation computes the SectorRotationFlag from sector index history.
// It calls sectorIndexReader for two windows: the most-recent 5 trading days
// (current top 3) vs the prior 5 trading days (prior top 3). If the two sets
// differ, rotation is considered active.
//
// Honesty rule (P0-1c): the indicator is filled ONLY when the reader reports
// >= MinDaysSectorRotationFlag effective dates; otherwise the field stays at
// its zero value (false = unavailable per detector contract).
//
// A non-empty but insufficient window (< MinDays) results in zero + Warn log.
func (c *PeriodIndicatorsCalculator) EnrichSectorRotation(ind *PeriodIndicators, tradingDate string, sectorIndexDir string) {
	if sectorIndexDir == "" {
		return
	}
	reader := marketdata.NewSectorIndexReader(sectorIndexDir)
	dates, err := reader.AvailableDates()
	if err != nil {
		log.Printf("warn: sector index reader failed: %v", err)
		return
	}
	// Filter dates <= tradingDate, sorted ascending.
	filtered := make([]string, 0, len(dates))
	for _, d := range dates {
		if d <= tradingDate {
			filtered = append(filtered, d)
		}
	}
	if len(filtered) < MinDaysSectorRotationFlag {
		log.Printf("warn: sector rotation insufficient dates=%d need=%d (tradingDate=%s)", len(filtered), MinDaysSectorRotationFlag, tradingDate)
		return
	}
	// Use the most recent 5 dates for current window, prior 5 for baseline.
	// Requires at least 10 effective dates to avoid double-counting.
	currentWindow := filtered[len(filtered)-5:]
	priorWindow := filtered[len(filtered)-10 : len(filtered)-5]
	curReturns, err := sumReturnsInWindow(reader, currentWindow)
	if err != nil {
		log.Printf("warn: sector rotation current window read failed: %v", err)
		return
	}
	priorReturns, err := sumReturnsInWindow(reader, priorWindow)
	if err != nil {
		log.Printf("warn: sector rotation prior window read failed: %v", err)
		return
	}
	curTop := sectorTopN(curReturns, 3)
	priorTop := sectorTopN(priorReturns, 3)
	ind.SectorRotationFlag = !setsEqual(curTop, priorTop)
}

// sumReturnsInWindow sums per-industry return_pct over the given date window.
func sumReturnsInWindow(reader *marketdata.SectorIndexReader, dates []string) (map[string]float64, error) {
	out := make(map[string]float64)
	for _, d := range dates {
		returns, ok, err := reader.Get(d)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		for industry, ret := range returns {
			out[industry] += ret
		}
	}
	return out, nil
}

// isValidGovernmentFlowDate returns true if the date has both the legacy
// YYYYMMDD.json total file and the new-format YYYYMMDD_brokers.json per-broker
// detail file (P0-3 rule: legacy zero-files from the broken parser era are
// excluded as valid data days).
func isValidGovernmentFlowDate(date, flowDir string) bool {
	dateCompact := strings.ReplaceAll(date, "-", "")
	legacyPath := filepath.Join(flowDir, dateCompact+".json")
	brokersPath := filepath.Join(flowDir, dateCompact+"_brokers.json")
	if _, err := os.Stat(legacyPath); err != nil {
		return false
	}
	if _, err := os.Stat(brokersPath); err != nil {
		return false
	}
	return true
}

// EnrichGovernmentBroker computes the consecutive-buy-day count for the
// public-bank (government) broker channel. It reads both the legacy
// YYYYMMDD.json and the per-broker YYYYMMDD_brokers.json files in
// flowDir; per P0-3, a date is only valid if BOTH files exist (legacy-only
// zero files from the broken parser era are excluded).
//
// Honesty rule (P0-1c): the indicator is filled ONLY when at least
// MinDaysPublicBankConsecBuyDays valid dates are present; otherwise the
// field stays at its zero value (= unavailable per detector contract).
//
// A non-empty but insufficient window results in zero + Warn log.
func (c *PeriodIndicatorsCalculator) EnrichGovernmentBroker(ind *PeriodIndicators, tradingDate string, flowDir string) {
	if flowDir == "" {
		return
	}
	entries, err := os.ReadDir(flowDir)
	if err != nil {
		log.Printf("warn: government flow dir not accessible: %v", err)
		return
	}
	// Collect valid dates <= tradingDate, sorted ascending.
	var validDates []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		if !strings.HasSuffix(name, "_brokers.json") {
			continue
		}
		// Extract YYYYMMDD from name.
		base := strings.TrimSuffix(name, "_brokers.json")
		if len(base) != 8 {
			continue
		}
		dateCanonical := base[:4] + "-" + base[4:6] + "-" + base[6:8]
		if dateCanonical > tradingDate {
			continue
		}
		if isValidGovernmentFlowDate(dateCanonical, flowDir) {
			validDates = append(validDates, dateCanonical)
		}
	}
	sort.Strings(validDates)
	if len(validDates) < MinDaysPublicBankConsecBuyDays {
		log.Printf("warn: government broker insufficient valid dates=%d need=%d (tradingDate=%s)", len(validDates), MinDaysPublicBankConsecBuyDays, tradingDate)
		return
	}
	// Read net flow per date from per-broker files; use total if per-broker
	// is not granular, or sum public-bank rows if structured. We use the
	// legacy total_net for simplicity (the public-bank net is the total of
	// all public banks; both formats expose the same scalar).
	streak := 0
	consecutiveBuy := true
	// Walk from most recent backward; count consecutive buy days.
	for i := len(validDates) - 1; i >= 0; i-- {
		dateCanonical := validDates[i]
		dateCompact := strings.ReplaceAll(dateCanonical, "-", "")
		legacyPath := filepath.Join(flowDir, dateCompact+".json")
		data, err := os.ReadFile(legacyPath)
		if err != nil {
			// Missing legacy file: stop the streak.
			break
		}
		var legacy struct {
			TotalNet float64 `json:"total_net"`
		}
		if err := json.Unmarshal(data, &legacy); err != nil {
			break
		}
		if legacy.TotalNet > 0 && consecutiveBuy {
			streak++
		} else {
			break
		}
	}
	ind.PublicBankConsecBuyDays = streak
}

// EnrichBatch3 wires EnrichSectorRotation + EnrichGovernmentBroker onto an
// already-Enriched PeriodIndicators. Pass empty strings to skip either
// source. All operations are best-effort: failures or insufficient data
// leave the field at its zero value (= unavailable per detector contract).
func (c *PeriodIndicatorsCalculator) EnrichBatch3(ind *PeriodIndicators, tradingDate, sectorIndexDir, govFlowDir string) {
	c.EnrichSectorRotation(ind, tradingDate, sectorIndexDir)
	c.EnrichGovernmentBroker(ind, tradingDate, govFlowDir)
}
