package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"time"

	configpkg "github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "calibrate-thresholds: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	workDir, _ := os.Getwd()
	revenuePath := filepath.Join(workDir, "data", "replay", "month_revenue.jsonl")
	configPath := filepath.Join(workDir, "configs", "parameters.json")

	results, err := industry.CalibrateThresholdsFromFile(revenuePath)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return fmt.Errorf("no industry had enough data for calibration")
	}

	fmt.Println("Threshold calibration results:")
	for _, r := range results {
		fmt.Printf("  %-20s  n=%-4d  P25=%6.1f%%  P50=%6.1f%%  P75=%6.1f%%\n",
			r.IndustryID, r.SampleSize, r.P25*100, r.P50*100, r.P75*100)
	}

	fmt.Println("\nWriting calibrated thresholds to parameters.json...")
	if err := writeConfig(configPath, results); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	fmt.Println("Done.")
	return nil
}

func writeConfig(configPath string, results []industry.CalibrationResult) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	industryCfg, _ := config["industry"].(map[string]any)
	if industryCfg == nil {
		industryCfg = make(map[string]any)
		config["industry"] = industryCfg
	}
	ct, _ := industryCfg["cycle_thresholds"].(map[string]any)
	if ct == nil {
		ct = make(map[string]any)
		industryCfg["cycle_thresholds"] = ct
	}
	ct["rationale"] = "Per-industry thresholds calibrated from historical revenue growth percentiles"
	ct["source"] = "percentile_based"
	ct["calibrated_at"] = time.Now().Format(time.RFC3339)
	delete(ct, "todo")
	value, _ := ct["value"].(map[string]any)
	if value == nil {
		value = make(map[string]any)
		ct["value"] = value
	}
	for _, r := range results {
		value[r.IndustryID] = map[string]float64{
			"expansion_revenue_pct": math.Round(r.P75*10000) / 10000,
			"expansion_profit_pct":  math.Round(r.P75*10000) / 10000,
			"recovery_revenue_pct":  math.Round(r.P50*10000) / 10000,
			"recovery_profit_pct":   math.Round(r.P50*10000) / 10000,
			"mature_revenue_pct":    math.Round(r.P25*10000) / 10000,
			"mature_profit_pct":     math.Round(r.P25*10000) / 10000,
		}
	}
	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	out = append(out, '\n')
	return configpkg.LockedWriteFileWithRollback(configPath, out)
}
