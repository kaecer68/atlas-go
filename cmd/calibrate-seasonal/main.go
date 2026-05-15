package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/kaecer68/atlas-go/internal/config"
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

// buildSyntheticReturns creates placeholder industry returns for testing.
func buildSyntheticReturns() map[string]map[string]float64 {
	return map[string]map[string]float64{
		"semiconductor":   {"2024-01-15": 0.02, "2024-02-15": 0.03, "2024-07-01": 0.08, "2024-09-15": -0.02, "2024-11-01": 0.01, "2024-12-31": 0.04},
		"ai_supply_chain": {"2024-01-15": 0.03, "2024-02-15": 0.04, "2024-07-01": 0.10, "2024-09-15": -0.01, "2024-11-01": 0.02, "2024-12-31": 0.05},
		"financials":      {"2024-01-15": 0.01, "2024-02-15": 0.02, "2024-07-01": 0.01, "2024-09-15": 0.00, "2024-11-01": 0.03, "2024-12-31": 0.02},
	}
}

// loadReplayDataset loads TWSE CSV replay data from a file or directory.
func loadReplayDataset(path string) (*replay.Dataset, error) {
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
	stockIndustryMap := loadSectorSymbols()

	return industry.IndustryReturnAggregator(stockReturns, stockIndustryMap)
}

// loadSectorSymbols reads the sector-to-stock mapping from config.
func loadSectorSymbols() map[string]string {
	type sectorEntry struct {
		Sector  string   `json:"sector"`
		Symbols []string `json:"symbols"`
	}

	mapping := make(map[string]string)
	data, err := os.ReadFile("configs/sector_symbols.json")
	if err != nil {
		return mapping
	}

	var raw map[string][]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return mapping
	}

	for sector, symbols := range raw {
		for _, sym := range symbols {
			mapping[sym] = sector
		}
	}
	return mapping
}
