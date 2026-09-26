package main

import (
	"encoding/json"
	"flag"
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
	workback := flag.String("writeback", string(configpkg.WritebackSSOT), configpkg.WritebackFlagUsage)
	flag.Parse()
	mode, err := configpkg.ParseCalibrationWriteback(*workback)
	if err != nil {
		return err
	}

	workDir, _ := os.Getwd()
	revenuePath := filepath.Join(workDir, "data", "replay", "month_revenue.jsonl")
	configPath := filepath.Join(workDir, "configs", "parameters.json")
	if err := mode.RegisterForWorkDir(workDir); err != nil {
		return err
	}

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

	if mode.IsOverlay() {
		fmt.Printf("\nWriting calibrated thresholds to %s (configs/parameters.json untouched)...\n", configpkg.GetCalibratedOverlayPath())
	} else {
		fmt.Println("\nWriting calibrated thresholds to parameters.json...")
	}
	if err := writeConfig(configPath, results, mode); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	fmt.Println("Done.")
	return nil
}

func writeConfig(configPath string, results []industry.CalibrationResult, mode configpkg.CalibrationWriteback) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// Overlay mode diffs the document as loaded against the updated one: only the
	// calibrated leaves are persisted, and configs/parameters.json is never
	// written (it is not bind-mounted inside a container).
	var base map[string]any
	if mode.IsOverlay() {
		if err := json.Unmarshal(data, &base); err != nil {
			return fmt.Errorf("parse config (baseline): %w", err)
		}
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
	if mode.IsOverlay() {
		_, err := configpkg.WriteDocumentOverlay("calibrate_thresholds", base, config, time.Now())
		return err
	}

	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	out = append(out, '\n')
	return configpkg.LockedWriteFileWithRollback(configPath, out)
}
