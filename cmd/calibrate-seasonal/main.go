package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/replay"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "calibrate-seasonal: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("calibrate-seasonal", flag.ContinueOnError)
	startYear := fs.Int("start", 2021, "Start year for backtest window")
	endYear := fs.Int("end", 2026, "End year for backtest window")
	outputJSON := fs.Bool("json", false, "Output results as JSON")
	replayPath := fs.String("replay", "", "Path to replay data (CSV/JSONL). When set, uses actual stock returns aggregated by industry instead of synthetic data.")
	update := fs.Bool("update", false, "Write calibration results back to configs/parameters.json")
	updateThreshold := fs.Int("update-threshold", 3, "Minimum observations required to update a pattern")
	if err := fs.Parse(args); err != nil {
		return err
	}

	seasonalEngine := industry.NewSeasonalEngineFromConfig(config.GetParametersConfig())

	var industryReturns map[string]map[string]float64
	if *replayPath != "" {
		fmt.Fprintf(os.Stderr, "Loading replay data from %s...\n", *replayPath)
		dataset, err := loadReplayDataset(*replayPath)
		if err != nil {
			return fmt.Errorf("load replay: %w", err)
		}
		// Convert to industry returns using the sector symbols mapping
		industryReturns = aggregateIndustryReturns(dataset)
		fmt.Fprintf(os.Stderr, "Loaded %d industries from replay data\n", len(industryReturns))
	} else {
		industryReturns = buildSyntheticReturns()
	}

	results, err := industry.CalibratePatterns(seasonalEngine, industryReturns, *startYear, *endYear)
	if err != nil {
		return fmt.Errorf("calibration failed: %w", err)
	}

	if *update {
		if *replayPath == "" {
			return fmt.Errorf("calibrate-seasonal: refusing --update with synthetic 2024 fallback data; rerun with --replay <path> to supply real stock returns before writing to configs/parameters.json")
		}
		if err := updateParametersFile(results, *updateThreshold, *replayPath); err != nil {
			return fmt.Errorf("update parameters: %w", err)
		}
	}

	if *outputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(results)
	}

	report := industry.CalibrationReport(results)
	fmt.Print(report)

	missing := industry.ValidateIndustryIDs(seasonalEngine.GetAllPatterns(), industryReturns)
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "Warning: missing industry return data for: %v\n", missing)
	}

	return nil
}

func updateParametersFile(results []industry.SeasonalCalibration, threshold int, dataSource string) error {
	paramsPath := constants.ParametersFile

	data, err := os.ReadFile(paramsPath)
	if err != nil {
		return fmt.Errorf("read parameters.json: %w", err)
	}

	var params map[string]any
	if err := json.Unmarshal(data, &params); err != nil {
		return fmt.Errorf("parse parameters.json: %w", err)
	}

	industrySection, ok := params["industry"].(map[string]any)
	if !ok {
		return fmt.Errorf("industry section not found in parameters.json")
	}

	seasonalPatternsObj, ok := industrySection["seasonal_patterns"]
	if !ok {
		return fmt.Errorf("seasonal_patterns not found in parameters.json")
	}

	seasonalPatterns, ok := seasonalPatternsObj.(map[string]any)
	if !ok {
		return fmt.Errorf("seasonal_patterns is not an object")
	}

	patternsArrayObj, ok := seasonalPatterns["value"]
	if !ok {
		return fmt.Errorf("seasonal_patterns.value not found")
	}

	patternsArray, ok := patternsArrayObj.([]any)
	if !ok {
		return fmt.Errorf("seasonal_patterns.value is not an array")
	}

	resultByID := make(map[string]industry.SeasonalCalibration)
	for _, r := range results {
		resultByID[r.PatternID] = r
	}

	var updated []string
	var skipped []string

	for i, patternObj := range patternsArray {
		pattern, ok := patternObj.(map[string]any)
		if !ok {
			continue
		}

		patternID, ok := pattern["id"].(string)
		if !ok || patternID == "" {
			continue
		}

		result, found := resultByID[patternID]
		if !found {
			skipped = append(skipped, fmt.Sprintf("%s (no calibration result)", patternID))
			continue
		}

		if result.ObservationCount == 0 {
			skipped = append(skipped, fmt.Sprintf("%s (zero observations)", patternID))
			continue
		}

		if result.ObservationCount < threshold {
			skipped = append(skipped, fmt.Sprintf("%s (observations %d < threshold %d)", patternID, result.ObservationCount, threshold))
			continue
		}

		if violations := validateCalibrationResult(result); len(violations) > 0 {
			skipped = append(skipped, fmt.Sprintf("%s (calibration out of range: %v)", patternID, violations))
			continue
		}

		pattern["historical_accuracy"] = result.ObservedAccuracy
		pattern["avg_market_return"] = result.ObservedAvgReturn
		pattern["adjustment_factor"] = result.ObservedAdjustment
		patternsArray[i] = pattern
		updated = append(updated, patternID)
	}

	now := time.Now().UTC()
	seasonalPatterns["calibration_timestamp"] = now.Format(time.RFC3339)
	seasonalPatterns["last_calibrated"] = now.Format(time.RFC3339)
	if dataSource != "" {
		seasonalPatterns["calibration_data_source"] = dataSource
	} else {
		seasonalPatterns["calibration_data_source"] = "synthetic"
	}
	// Update citation evidence_quality based on observation counts
	avgObs := 0
	for _, r := range results {
		avgObs += r.ObservationCount
	}
	if len(results) > 0 {
		avgObs /= len(results)
	}
	cite, ok := seasonalPatterns["citation"].(map[string]any)
	if !ok {
		cite = make(map[string]any)
	}
	if avgObs >= 5 {
		cite["evidence_quality"] = "high"
	} else if avgObs >= 3 {
		cite["evidence_quality"] = "medium"
	}
	cite["calibration_method"] = "backtest_empirical"
	seasonalPatterns["citation"] = cite

	out, err := json.MarshalIndent(params, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal updated parameters: %w", err)
	}

	if err := config.LockedWriteFileWithRollback(paramsPath, out); err != nil {
		return fmt.Errorf("write parameters.json: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\nUpdated parameters.json:\n")
	for _, id := range updated {
		fmt.Fprintf(os.Stderr, "  ✓ %s\n", id)
	}
	for _, reason := range skipped {
		fmt.Fprintf(os.Stderr, "  ✗ %s\n", reason)
	}

	return nil
}

// buildSyntheticReturns creates placeholder industry returns for testing.
func buildSyntheticReturns() map[string]map[string]float64 {
	return map[string]map[string]float64{
		"semiconductor":   {"2024-01-15": 0.02, "2024-02-15": 0.03, "2024-07-01": 0.08, "2024-09-15": -0.02, "2024-11-01": 0.01, "2024-12-31": 0.04},
		"ai_supply_chain": {"2024-01-15": 0.03, "2024-02-15": 0.04, "2024-07-01": 0.10, "2024-09-15": -0.01, "2024-11-01": 0.02, "2024-12-31": 0.05},
		"financials":      {"2024-01-15": 0.01, "2024-02-15": 0.02, "2024-07-01": 0.01, "2024-09-15": 0.00, "2024-11-01": 0.03, "2024-12-31": 0.02},
	}
}

// loadReplayDataset loads TWSE replay data from a CSV or JSONL file.
func loadReplayDataset(path string) (*replay.Dataset, error) {
	if strings.HasSuffix(path, ".jsonl") {
		return loadReplayDatasetJSONL(path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		return replay.LoadTWSEOpenDataCSV(filepath.Join(path, "replay.csv"))
	}
	return replay.LoadTWSEOpenDataCSV(path)
}

// aggregateIndustryReturns converts a replay dataset into per-industry daily returns
// using equal-weight averaging of constituent stock returns.
func aggregateIndustryReturns(dataset *replay.Dataset) map[string]map[string]float64 {
	stockReturns := make(map[string]map[string]float64)

	for dateStr, stocks := range dataset.ByDate {
		for symbol, bar := range stocks {
			if bar.Close <= 0 {
				continue
			}
			if stockReturns[symbol] == nil {
				stockReturns[symbol] = make(map[string]float64)
			}
			stockReturns[symbol][dateStr] = bar.Close
		}
	}

	// Convert close prices to daily returns
	for symbol, datePrices := range stockReturns {
		sortedDates := make([]string, 0, len(datePrices))
		for d := range datePrices {
			sortedDates = append(sortedDates, d)
		}
		sort.Strings(sortedDates)

		for i := len(sortedDates) - 1; i > 0; i-- {
			curr := sortedDates[i]
			prev := sortedDates[i-1]
			currPrice := datePrices[curr]
			prevPrice := datePrices[prev]
			if prevPrice > 0 {
				stockReturns[symbol][curr] = (currPrice - prevPrice) / prevPrice
			} else {
				delete(stockReturns[symbol], curr)
			}
		}
		delete(stockReturns[symbol], sortedDates[0])
	}

	// Map stocks to industries using sector_symbols.json
	cfg := config.Load()
	sectorSymbolsPath := filepath.Join(cfg.WorkDir, "configs", "sector_symbols.json")
	stockIndustryMap, err := loadSectorSymbols(sectorSymbolsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}

	return industry.IndustryReturnAggregator(stockReturns, stockIndustryMap)
}

// loadSectorSymbols reads the sector-to-stock mapping from the given path.
// Returns a map from stock symbol to ALL sectors it belongs to (many-to-many).
func loadSectorSymbols(path string) (map[string][]string, error) {
	mapping := make(map[string][]string)
	data, err := os.ReadFile(path)
	if err != nil {
		return mapping, fmt.Errorf("read sector symbols from %s: %w", path, err)
	}

	var raw map[string][]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return mapping, fmt.Errorf("parse sector symbols from %s: %w", path, err)
	}

	for sector, symbols := range raw {
		for _, sym := range symbols {
			mapping[sym] = append(mapping[sym], sector)
		}
	}
	return mapping, nil
}

type jsonlRow struct {
	Date   string  `json:"date"`
	Symbol string  `json:"symbol"`
	Name   string  `json:"name"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume int64   `json:"volume"`
	Source string  `json:"source"`
}

// loadReplayDatasetJSONL loads a JSONL replay file into a replay.Dataset.
func loadReplayDatasetJSONL(path string) (*replay.Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	ds := &replay.Dataset{
		ByDate: map[string]map[string]domain.DailyBar{},
		Dates:  make([]time.Time, 0),
	}
	seenDates := map[string]time.Time{}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] != '{' {
			continue
		}
		var row jsonlRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("unmarshal jsonl line: %w", err)
		}
		date, err := time.Parse("2006-01-02", row.Date)
		if err != nil {
			return nil, fmt.Errorf("parse date %s: %w", row.Date, err)
		}
		dateKey := date.Format("2006-01-02")
		if _, ok := ds.ByDate[dateKey]; !ok {
			ds.ByDate[dateKey] = map[string]domain.DailyBar{}
			seenDates[dateKey] = date
		}
		ds.ByDate[dateKey][row.Symbol] = domain.DailyBar{
			Date:   date,
			Symbol: row.Symbol,
			Name:   row.Name,
			Open:   row.Open,
			High:   row.High,
			Low:    row.Low,
			Close:  row.Close,
			Volume: row.Volume,
			Source: row.Source,
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}

	for _, date := range seenDates {
		ds.Dates = append(ds.Dates, date)
	}
	sort.Slice(ds.Dates, func(i, j int) bool {
		return ds.Dates[i].Before(ds.Dates[j])
	})
	return ds, nil
}

func validateCalibrationResult(r industry.SeasonalCalibration) []string {
	var violations []string
	if r.ObservedAdjustment < industry.DarwinianMinAdjustment || r.ObservedAdjustment > industry.DarwinianMaxAdjustment {
		violations = append(violations, fmt.Sprintf(
			"adjustment_factor=%.3f outside Darwinian [%.1f, %.1f]",
			r.ObservedAdjustment,
			industry.DarwinianMinAdjustment,
			industry.DarwinianMaxAdjustment,
		))
	}
	if r.ObservedAccuracy < 0.0 || r.ObservedAccuracy > 1.0 {
		violations = append(violations, fmt.Sprintf("historical_accuracy=%.3f outside [0, 1]", r.ObservedAccuracy))
	}
	if r.ObservedAvgReturn < -1.0 || r.ObservedAvgReturn > 1.0 {
		violations = append(violations, fmt.Sprintf("avg_market_return=%.3f outside [-1, 1]", r.ObservedAvgReturn))
	}
	return violations
}
