package main

import (
	"encoding/csv"
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
	startStr := fs.String("start", "", "training start date YYYY-MM-DD (optional)")
	symbolFilter := fs.String("symbol", "", "filter to single symbol (e.g. 2330.TW)")
	featuresStr := fs.String("features", "close,volume,return_1d", "comma-separated feature names")
	modelStr := fs.String("model", "ols", "ML model: ols, pcr, pls, elasticnet")
	firstTrainEndStr := fs.String("first-train-end", "2007-12-31", "first train end date YYYY-MM-DD")
	validYears := fs.Int("valid-years", 2, "validation window years")
	stepYears := fs.Int("step-years", 1, "step size in years")
	testEndStr := fs.String("test-end", "2022-04-30", "test end date YYYY-MM-DD")
	outPath := fs.String("out", "", "write results CSV (optional)")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dataPath == "" {
		return fmt.Errorf("-data is required")
	}

	ds, err := replay.LoadTWSEOpenDataCSV(*dataPath)
	if err != nil {
		return fmt.Errorf("load CSV: %w", err)
	}

	bars := flattenDataset(ds)
	sort.Slice(bars, func(i, j int) bool { return bars[i].Date.Before(bars[j].Date) })
	if len(bars) == 0 {
		return fmt.Errorf("no bars loaded from %s", *dataPath)
	}

	symbols := uniqueSymbols(bars)
	if *symbolFilter == "" && len(symbols) > 1 {
		return fmt.Errorf("data contains %d symbols; use -symbol (e.g. -symbol %s)", len(symbols), symbols[0])
	}
	if *symbolFilter != "" {
		bars = filterBySymbol(bars, *symbolFilter)
		if len(bars) == 0 {
			return fmt.Errorf("no bars for symbol %s", *symbolFilter)
		}
	}
	if *startStr != "" {
		d, err := time.Parse("2006-01-02", *startStr)
		if err != nil {
			return fmt.Errorf("parse -start: %w", err)
		}
		bars = filterByStart(bars, d)
		if len(bars) == 0 {
			return fmt.Errorf("no bars on or after %s", *startStr)
		}
	}

	fmt.Fprintf(os.Stderr, "loaded %d bars from %s to %s",
		len(bars), bars[0].Date.Format("2006-01-02"), bars[len(bars)-1].Date.Format("2006-01-02"))
	if *symbolFilter != "" {
		fmt.Fprintf(os.Stderr, " (symbol=%s)", *symbolFilter)
	}
	fmt.Fprintln(os.Stderr)

	featureNames := parseFeatureNames(*featuresStr)
	if len(featureNames) == 0 {
		return fmt.Errorf("no features; use -features")
	}
	unknown := validateFeatures(featureNames)
	if len(unknown) > 0 {
		return fmt.Errorf("unknown feature(s): %s", strings.Join(unknown, ", "))
	}

	fe := makeFeatureExtractor(featureNames)
	le := makeForwardReturnLabel()

	model, err := newModel(*modelStr)
	if err != nil {
		return err
	}

	pipeline := backtest.NewBacktestPipeline(bars)
	pipeline.ExtractFeatures = fe
	pipeline.ExtractLabels = le

	if ft, err := time.Parse("2006-01-02", *firstTrainEndStr); err != nil {
		return fmt.Errorf("parse -first-train-end: %w", err)
	} else {
		pipeline.Split.FirstTrainEnd = ft
	}
	if *validYears < 1 {
		return fmt.Errorf("-valid-years must be >= 1")
	}
	pipeline.Split.ValidLengthYears = *validYears
	if *stepYears < 1 {
		return fmt.Errorf("-step-years must be >= 1")
	}
	pipeline.Split.StepYears = *stepYears
	if te, err := time.Parse("2006-01-02", *testEndStr); err != nil {
		return fmt.Errorf("parse -test-end: %w", err)
	} else {
		pipeline.Split.TestEnd = te
	}

	results, err := pipeline.Run(model)
	if err != nil {
		return fmt.Errorf("pipeline run: %w", err)
	}

	printResults(results, featureNames)

	if *outPath != "" {
		if err := writeResultsCSV(results, featureNames, *outPath); err != nil {
			return fmt.Errorf("write CSV: %w", err)
		}
		fmt.Fprintf(os.Stderr, "results written to %s\n", *outPath)
	}

	return nil
}

func flattenDataset(ds *replay.Dataset) []domain.DailyBar {
	var out []domain.DailyBar
	for _, date := range ds.Dates {
		for _, bar := range ds.ByDate[date.Format("2006-01-02")] {
			out = append(out, bar)
		}
	}
	return out
}

func filterBySymbol(bars []domain.DailyBar, sym string) []domain.DailyBar {
	out := make([]domain.DailyBar, 0, len(bars))
	for _, b := range bars {
		if b.Symbol == sym {
			out = append(out, b)
		}
	}
	return out
}

func filterByStart(bars []domain.DailyBar, start time.Time) []domain.DailyBar {
	out := make([]domain.DailyBar, 0, len(bars))
	for _, b := range bars {
		if !b.Date.Before(start) {
			out = append(out, b)
		}
	}
	return out
}

func uniqueSymbols(bars []domain.DailyBar) []string {
	seen := map[string]struct{}{}
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

type featureFunc func(bar domain.DailyBar, idx int, bars []domain.DailyBar) float64

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
	"ma_ratio": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx < 19 {
			return 1.0
		}
		sum := 0.0
		for j := idx - 19; j <= idx; j++ {
			sum += bars[j].Close
		}
		if sum > 0 {
			return b.Close / (sum / 20.0)
		}
		return 1.0
	},
	"volume_ratio": func(b domain.DailyBar, idx int, bars []domain.DailyBar) float64 {
		if idx < 19 || b.Volume <= 0 {
			return 1.0
		}
		sum := 0.0
		for j := idx - 19; j <= idx; j++ {
			sum += float64(bars[j].Volume)
		}
		avg := sum / 20.0
		if avg > 0 {
			return float64(b.Volume) / avg
		}
		return 1.0
	},
}

func availableFeatures() []string {
	n := make([]string, 0, len(featureRegistry))
	for k := range featureRegistry {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

func validateFeatures(names []string) []string {
	var u []string
	for _, n := range names {
		if _, ok := featureRegistry[n]; !ok {
			u = append(u, n)
		}
	}
	return u
}

func parseFeatureNames(r string) []string {
	parts := strings.Split(r, ",")
	names := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			names = append(names, p)
		}
	}
	return names
}

func makeFeatureExtractor(names []string) backtest.FeatureExtractor {
	return func(bars []domain.DailyBar) [][]float64 {
		f := make([][]float64, len(bars))
		for i, bar := range bars {
			row := make([]float64, len(names))
			for j, n := range names {
				row[j] = featureRegistry[n](bar, i, bars)
			}
			f[i] = row
		}
		return f
	}
}

func makeForwardReturnLabel() backtest.LabelExtractor {
	return func(bars []domain.DailyBar) []float64 {
		l := make([]float64, len(bars))
		for i := 0; i < len(bars)-1; i++ {
			if bars[i].Close > 0 {
				l[i] = (bars[i+1].Close - bars[i].Close) / bars[i].Close
			}
		}
		if len(bars) > 0 {
			l[len(bars)-1] = 0
		}
		return l
	}
}

func newModel(name string) (backtest.Model, error) {
	switch name {
	case "ols":
		return ml.NewOLS(), nil
	case "pcr":
		return &ml.PCR{NComponents: 5, VarianceThreshold: 0.95}, nil
	case "pls":
		return &ml.PLS{NComponents: 3}, nil
	case "elasticnet":
		return &ml.ElasticNet{L1Ratio: 0.5, Alpha: 1.0, AlphaAuto: false, MaxIter: 1000, Tol: 1e-4}, nil
	default:
		return nil, fmt.Errorf("unknown model %q; choose: ols, pcr, pls, elasticnet", name)
	}
}

func oosR2(pred, act []float64) float64 {
	n := len(pred)
	if n <= 1 {
		return math.NaN()
	}
	mean := 0.0
	for _, a := range act {
		mean += a
	}
	mean /= float64(n)
	sst, sse := 0.0, 0.0
	for i := 0; i < n; i++ {
		dA := act[i] - mean
		sst += dA * dA
		dE := act[i] - pred[i]
		sse += dE * dE
	}
	if sst == 0 {
		return math.NaN()
	}
	return 1.0 - sse/sst
}

func annualizedSharpe(pred, act []float64) float64 {
	n := len(pred)
	if n <= 1 {
		return math.NaN()
	}
	s := make([]float64, n)
	for i := 0; i < n; i++ {
		if pred[i] >= 0 {
			s[i] = act[i]
		} else {
			s[i] = -act[i]
		}
	}
	mean := 0.0
	for _, r := range s {
		mean += r
	}
	mean /= float64(n)
	v := 0.0
	for _, r := range s {
		d := r - mean
		v += d * d
	}
	v /= float64(n - 1)
	if v <= 0 {
		return math.NaN()
	}
	return mean / math.Sqrt(v) * math.Sqrt(252)
}

func printResults(results []backtest.BacktestResult, featureNames []string) {
	fmt.Printf("%-45s %8s %8s %8s %8s %8s\n", "Window ID", "Train", "Test", "OOS R2", "Sharpe", "Features")
	fmt.Println(strings.Repeat("-", 100))
	for _, r := range results {
		tn, ts := 0, 0
		if v, ok := r.Metrics["train_samples"]; ok {
			tn = int(v)
		}
		if v, ok := r.Metrics["test_samples"]; ok {
			ts = int(v)
		}
		r2 := oosR2(r.Predictions, r.Actuals)
		sh := annualizedSharpe(r.Predictions, r.Actuals)
		r2s := "   N/A"
		if !math.IsNaN(r2) {
			r2s = fmt.Sprintf("%+7.4f", r2)
		}
		shs := "   N/A"
		if !math.IsNaN(sh) {
			shs = fmt.Sprintf("%+7.4f", sh)
		}
		tns := "   N/A"
		if _, ok := r.Metrics["train_samples"]; ok {
			tns = fmt.Sprintf("%8d", tn)
		}
		tsts := "   N/A"
		if _, ok := r.Metrics["test_samples"]; ok {
			tsts = fmt.Sprintf("%8d", ts)
		}
		fmt.Printf("%-45s %8s %8s %8s %8s %8d\n", r.WindowID, tns, tsts, r2s, shs, len(featureNames))
	}
	fmt.Println(strings.Repeat("-", 100))
	printSummary(results)
}

func printSummary(results []backtest.BacktestResult) {
	var tR2, tSh float64
	var rc, sc, tt, tst int
	for _, r := range results {
		if v, ok := r.Metrics["train_samples"]; ok {
			tt += int(v)
		}
		if v, ok := r.Metrics["test_samples"]; ok {
			tst += int(v)
		}
		if r2 := oosR2(r.Predictions, r.Actuals); !math.IsNaN(r2) {
			tR2 += r2
			rc++
		}
		if sh := annualizedSharpe(r.Predictions, r.Actuals); !math.IsNaN(sh) {
			tSh += sh
			sc++
		}
	}
	fmt.Printf("%-45s %8d %8d", "SUMMARY", tt, tst)
	if rc > 0 {
		fmt.Printf(" %+7.4f", tR2/float64(rc))
	} else {
		fmt.Printf(" %8s", "N/A")
	}
	if sc > 0 {
		fmt.Printf(" %+7.4f", tSh/float64(sc))
	} else {
		fmt.Printf(" %8s", "N/A")
	}
	fmt.Printf(" %8s", fmt.Sprintf("%d windows", len(results)))
	fmt.Println()
}

func writeResultsCSV(results []backtest.BacktestResult, featureNames []string, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	w := csv.NewWriter(f)
	defer w.Flush()
	_ = w.Write([]string{"WindowID", "TrainSamples", "TestSamples", "OOS_R2", "Sharpe", "FeaturesCount"})
	for _, r := range results {
		tn, ts := 0, 0
		if v, ok := r.Metrics["train_samples"]; ok {
			tn = int(v)
		}
		if v, ok := r.Metrics["test_samples"]; ok {
			ts = int(v)
		}
		r2 := oosR2(r.Predictions, r.Actuals)
		sh := annualizedSharpe(r.Predictions, r.Actuals)
		r2s := ""
		if !math.IsNaN(r2) {
			r2s = fmt.Sprintf("%.6f", r2)
		}
		shs := ""
		if !math.IsNaN(sh) {
			shs = fmt.Sprintf("%.6f", sh)
		}
		_ = w.Write([]string{r.WindowID, fmt.Sprintf("%d", tn), fmt.Sprintf("%d", ts), r2s, shs, fmt.Sprintf("%d", len(featureNames))})
	}
	return nil
}
