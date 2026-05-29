package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ml"
	"github.com/kaecer68/atlas-go/internal/replay"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "backtest-pipeline: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("backtest-pipeline", flag.ContinueOnError)
	dataPath := fs.String("data", "", "CSV replay data path (required, TWSE format)")
	startStr := fs.String("start", "", "training start date YYYY-MM-DD (optional; earliest data date if unset)")
	symbolFilter := fs.String("symbol", "", "filter to single symbol (e.g. '2330.TW'; optional)")
	featuresStr := fs.String("features", "close,volume,return_1d", "comma-separated feature names: close, volume, return_1d, return_5d, hl_ratio")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *dataPath == "" {
		return fmt.Errorf("-data is required")
	}

	// ── 1. Load CSV replay data ──────────────────────────────────────────────
	ds, err := replay.LoadTWSEOpenDataCSV(*dataPath)
	if err != nil {
		return fmt.Errorf("load CSV: %w", err)
	}

	bars := flattenDataset(ds)
	sort.Slice(bars, func(i, j int) bool { return bars[i].Date.Before(bars[j].Date) })

	if len(bars) == 0 {
		return fmt.Errorf("no bars loaded from %s", *dataPath)
	}

	// ── 2. Detect multi-symbol or filter by symbol ──────────────────────────
	symbols := uniqueSymbols(bars)
	if *symbolFilter == "" && len(symbols) > 1 {
		return fmt.Errorf("data contains %d symbols; use -symbol to select one (e.g. -symbol %s)",
			len(symbols), symbols[0])
	}
	if *symbolFilter != "" {
		bars = filterBySymbol(bars, *symbolFilter)
		if len(bars) == 0 {
			return fmt.Errorf("no bars for symbol %s", *symbolFilter)
		}
	}

	// ── 3. Filter by start date (optional) ───────────────────────────────────
	if *startStr != "" {
		startDate, err := time.Parse("2006-01-02", *startStr)
		if err != nil {
			return fmt.Errorf("parse -start: %w", err)
		}
		bars = filterByStart(bars, startDate)
		if len(bars) == 0 {
			return fmt.Errorf("no bars on or after %s", *startStr)
		}
	}

	fmt.Fprintf(os.Stderr, "loaded %d bars from %s to %s",
		len(bars),
		bars[0].Date.Format("2006-01-02"),
		bars[len(bars)-1].Date.Format("2006-01-02"),
	)
	if *symbolFilter != "" {
		fmt.Fprintf(os.Stderr, " (symbol=%s)", *symbolFilter)
	}
	fmt.Fprintln(os.Stderr)

	// ── 4. Parse feature selection ───────────────────────────────────────────
	featureNames := parseFeatureNames(*featuresStr)
	if len(featureNames) == 0 {
		return fmt.Errorf("no features selected; use -features to specify at least one")
	}

	unknown := validateFeatures(featureNames)
	if len(unknown) > 0 {
		return fmt.Errorf("unknown feature(s): %s (available: %s)",
			strings.Join(unknown, ", "), strings.Join(availableFeatures(), ", "))
	}

	// ── 5. Build extractors ──────────────────────────────────────────────────
	fe := makeFeatureExtractor(featureNames)
	le := makeForwardReturnLabel()

	// ── 6. Create pipeline and model ─────────────────────────────────────────
	pipeline := backtest.NewBacktestPipeline(bars)
	pipeline.ExtractFeatures = fe
	pipeline.ExtractLabels = le

	model := ml.NewOLS()

	// ── 7. Run ───────────────────────────────────────────────────────────────
	results, err := pipeline.Run(model)
	if err != nil {
		return fmt.Errorf("pipeline run: %w", err)
	}

	// ── 8. Print results ─────────────────────────────────────────────────────
	printResults(results, featureNames)

	return nil
}

// ── Dataset helpers ────────────────────────────────────────────────────────────

// flattenDataset converts a replay.Dataset into a flat, unsorted slice of DailyBar.
func flattenDataset(ds *replay.Dataset) []domain.DailyBar {
	var out []domain.DailyBar
	for _, date := range ds.Dates {
		day := ds.ByDate[date.Format("2006-01-02")]
		for _, bar := range day {
			out = append(out, bar)
		}
	}
	return out
}

// filterBySymbol keeps only bars matching the given symbol.
func filterBySymbol(bars []domain.DailyBar, symbol string) []domain.DailyBar {
	out := make([]domain.DailyBar, 0, len(bars))
	for _, b := range bars {
		if b.Symbol == symbol {
			out = append(out, b)
		}
	}
	return out
}

// filterByStart keeps only bars on or after the given start date.
func filterByStart(bars []domain.DailyBar, start time.Time) []domain.DailyBar {
	out := make([]domain.DailyBar, 0, len(bars))
	for _, b := range bars {
		if !b.Date.Before(start) {
			out = append(out, b)
		}
	}
	return out
}

// uniqueSymbols returns the sorted unique symbol values in bars.
func uniqueSymbols(bars []domain.DailyBar) []string {
	seen := make(map[string]struct{}, 10)
	for _, b := range bars {
		seen[b.Symbol] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// ── Feature extraction ─────────────────────────────────────────────────────────

// featureFunc computes one feature value from a bar at position idx in a sorted bar slice.
type featureFunc func(bar domain.DailyBar, idx int, bars []domain.DailyBar) float64

// featureRegistry maps feature names to their computation functions.
var featureRegistry = map[string]featureFunc{
	"close": func(b domain.DailyBar, _ int, _ []domain.DailyBar) float64 {
		return b.Close
	},
	"volume": func(b domain.DailyBar, _ int, _ []domain.DailyBar) float64 {
		if b.Volume <= 0 {
			return 0
		}
		return math.Log(float64(b.Volume))
	},
	"return_1d": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx > 0 && bars[idx-1].Close > 0 {
			return (b.Close - bars[idx-1].Close) / bars[idx-1].Close
		}
		return 0
	},
	"return_5d": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx >= 5 && bars[idx-5].Close > 0 {
			return (b.Close - bars[idx-5].Close) / bars[idx-5].Close
		}
		return 0
	},
	"hl_ratio": func(b domain.DailyBar, _ int, _ []domain.DailyBar) float64 {
		if b.Close > 0 {
			return (b.High - b.Low) / b.Close
		}
		return 0
	},
}

// availableFeatures returns all registered feature names in sorted order.
func availableFeatures() []string {
	names := make([]string, 0, len(featureRegistry))
	for k := range featureRegistry {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// validateFeatures returns any feature names not in the registry.
func validateFeatures(names []string) []string {
	var unknown []string
	for _, n := range names {
		if _, ok := featureRegistry[n]; !ok {
			unknown = append(unknown, n)
		}
	}
	return unknown
}

// parseFeatureNames splits a comma-separated string, trimming whitespace.
func parseFeatureNames(raw string) []string {
	parts := strings.Split(raw, ",")
	names := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			names = append(names, p)
		}
	}
	return names
}

// makeFeatureExtractor returns a backtest.FeatureExtractor that computes
// the selected features for each bar.
func makeFeatureExtractor(names []string) backtest.FeatureExtractor {
	return func(bars []domain.DailyBar) [][]float64 {
		features := make([][]float64, len(bars))
		for i, bar := range bars {
			row := make([]float64, len(names))
			for j, name := range names {
				row[j] = featureRegistry[name](bar, i, bars)
			}
			features[i] = row
		}
		return features
	}
}

// ── Label extraction ───────────────────────────────────────────────────────────

// makeForwardReturnLabel returns a backtest.LabelExtractor that uses the
// forward 1-day return as the label. For the last bar (unknown forward
// return), the label is 0.
func makeForwardReturnLabel() backtest.LabelExtractor {
	return func(bars []domain.DailyBar) []float64 {
		labels := make([]float64, len(bars))
		for i := 0; i < len(bars)-1; i++ {
			if bars[i].Close > 0 {
				labels[i] = (bars[i+1].Close - bars[i].Close) / bars[i].Close
			}
		}
		// Last bar: forward return unknown, set to 0.
		if len(bars) > 0 {
			labels[len(bars)-1] = 0
		}
		return labels
	}
}

// ── Metrics ────────────────────────────────────────────────────────────────────

// oosR2 computes out-of-sample R² = 1 - SSE / SST.
// Returns NaN if SST is zero or if n ≤ 1.
func oosR2(predictions, actuals []float64) float64 {
	n := len(predictions)
	if n <= 1 {
		return math.NaN()
	}

	mean := 0.0
	for _, a := range actuals {
		mean += a
	}
	mean /= float64(n)

	sst, sse := 0.0, 0.0
	for i := 0; i < n; i++ {
		diffA := actuals[i] - mean
		sst += diffA * diffA
		diffE := actuals[i] - predictions[i]
		sse += diffE * diffE
	}

	if sst == 0 {
		return math.NaN()
	}
	return 1.0 - sse/sst
}

// annualizedSharpe computes an annualized Sharpe ratio from prediction-signed
// returns. A positive prediction is treated as a long position (uses actual
// return directly); a negative prediction is treated as a short position
// (inverts the actual return). This assumes actuals are forward returns.
//
// Returns NaN if n ≤ 1 or if variance is zero.
func annualizedSharpe(predictions, actuals []float64) float64 {
	n := len(predictions)
	if n <= 1 {
		return math.NaN()
	}

	strat := make([]float64, n)
	for i := 0; i < n; i++ {
		if predictions[i] >= 0 {
			strat[i] = actuals[i]
		} else {
			strat[i] = -actuals[i]
		}
	}

	mean := 0.0
	for _, r := range strat {
		mean += r
	}
	mean /= float64(n)

	variance := 0.0
	for _, r := range strat {
		diff := r - mean
		variance += diff * diff
	}
	variance /= float64(n - 1)

	if variance <= 0 {
		return math.NaN()
	}
	std := math.Sqrt(variance)
	return mean / std * math.Sqrt(float64(tradingDaysPerYear)) // annualize
}

const tradingDaysPerYear = 252

// ── Output ─────────────────────────────────────────────────────────────────────

func printResults(results []backtest.BacktestResult, featureNames []string) {
	fmt.Printf("%-45s %8s %8s %8s %8s %8s\n",
		"Window ID", "Train", "Test", "OOS R²", "Sharpe", "Features")
	fmt.Println(strings.Repeat("-", 100))

	for _, r := range results {
		trainN, testN := 0, 0
		if v, ok := r.Metrics["train_samples"]; ok {
			trainN = int(v)
		}
		if v, ok := r.Metrics["test_samples"]; ok {
			testN = int(v)
		}
		r2 := oosR2(r.Predictions, r.Actuals)
		sharpe := annualizedSharpe(r.Predictions, r.Actuals)

		r2Str := "   N/A"
		if !math.IsNaN(r2) {
			r2Str = fmt.Sprintf("%+7.4f", r2)
		}

		sharpeStr := "   N/A"
		if !math.IsNaN(sharpe) {
			sharpeStr = fmt.Sprintf("%+7.4f", sharpe)
		}

		trainStr, testStr := "   N/A", "   N/A"
		if _, ok := r.Metrics["train_samples"]; ok {
			trainStr = fmt.Sprintf("%8d", trainN)
		}
		if _, ok := r.Metrics["test_samples"]; ok {
			testStr = fmt.Sprintf("%8d", testN)
		}

		fmt.Printf("%-45s %8s %8s %8s %8s %8d\n",
			r.WindowID, trainStr, testStr, r2Str, sharpeStr, len(featureNames))
	}

	// ── Summary across all windows ───────────────────────────────────────────
	fmt.Println(strings.Repeat("-", 100))
	printSummary(results)
}

func printSummary(results []backtest.BacktestResult) {
	var (
		totalR2     float64
		totalSharpe float64
		r2Count     int
		sharpeCount int
		totalTrain  int
		totalTest   int
	)

	for _, r := range results {
		if v, ok := r.Metrics["train_samples"]; ok {
			totalTrain += int(v)
		}
		if v, ok := r.Metrics["test_samples"]; ok {
			totalTest += int(v)
		}
		r2 := oosR2(r.Predictions, r.Actuals)
		if !math.IsNaN(r2) {
			totalR2 += r2
			r2Count++
		}
		sharpe := annualizedSharpe(r.Predictions, r.Actuals)
		if !math.IsNaN(sharpe) {
			totalSharpe += sharpe
			sharpeCount++
		}
	}

	fmt.Printf("%-45s %8d %8d", "SUMMARY", totalTrain, totalTest)
	if r2Count > 0 {
		fmt.Printf(" %+7.4f", totalR2/float64(r2Count))
	} else {
		fmt.Printf(" %8s", "N/A")
	}
	if sharpeCount > 0 {
		fmt.Printf(" %+7.4f", totalSharpe/float64(sharpeCount))
	} else {
		fmt.Printf(" %8s", "N/A")
	}
	fmt.Printf(" %8s", fmt.Sprintf("%d windows", len(results)))
	fmt.Println()
}
