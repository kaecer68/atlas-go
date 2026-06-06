// Command calibrate-baselines triggers one calibration cycle to bootstrap
// the rolling calibration framework's persisted baselines file.
//
// Purpose
// -------
// NewTaiwanStressCalculator (the production constructor) auto-enables
// hybrid signal computation when a baselines file exists at
// <workdir>/data/state/calibration/baselines.json. The hybrid path is
// required for DXY/JPY/US10Y/Oil/Gold to produce non-zero stress
// contributions on flat days (when ChangePct=0) but the level is
// extreme relative to history.
//
// On a fresh deployment, no baselines file exists. The background
// `calibration_cycle` task (PR #362) only runs every 24h AND requires
// `calibration_enabled=true` in parameters.json. This CLI provides a
// one-shot bootstrap so production deployments can activate hybrid
// signal immediately rather than waiting up to 24h.
//
// Usage
// -----
//
//	go run ./cmd/calibrate-baselines -workdir=/path/to/atlas
//
// On success: prints path to baselines.json and a summary of the
// computed baselines (mean/stddev/count per factor). The next server
// restart (or any process that calls NewTaiwanStressCalculator) will
// auto-load the file and enable hybrid signal.
//
// On failure: prints the underlying error (typically "load historical
// data: no paired macro/flow records found" if the server has never
// collected any data). Run `cmd/atlas` first to populate historical
// snapshots and capital flow records, then re-run this CLI.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/narrative"
)

func main() {
	workDir := flag.String("workdir", ".", "working directory (where data/state lives)")
	flag.Parse()

	if err := run(*workDir); err != nil {
		fmt.Fprintf(os.Stderr, "calibrate-baselines: %v\n", err)
		os.Exit(1)
	}
}

func run(workDir string) error {
	if workDir == "" {
		return fmt.Errorf("workdir is required")
	}

	baselinesPath := filepath.Join(workDir, narrative.BaselinesDir, narrative.BaselinesFileName)
	fmt.Printf("workdir:        %s\n", workDir)
	fmt.Printf("baselines path: %s\n", baselinesPath)
	fmt.Println()
	fmt.Println("Running one calibration cycle...")

	task := narrative.NewCalibrationTask(workDir)
	validation, err := task.RunCalibrationCycle()
	if err != nil {
		return fmt.Errorf("calibration cycle failed: %w", err)
	}

	if validation == nil {
		return fmt.Errorf("calibration cycle returned nil validation")
	}

	fmt.Println()
	fmt.Println("Calibration cycle complete.")
	fmt.Printf("  Old accuracy:  %.4f\n", validation.OldAccuracy)
	fmt.Printf("  New accuracy:  %.4f\n", validation.NewAccuracy)
	fmt.Printf("  Improvement:   %.4f\n", validation.Improvement)
	fmt.Printf("  IsDegradation: %t\n", validation.IsDegradation)
	fmt.Printf("  Training size: %d\n", validation.TrainingSize)
	fmt.Printf("  Validation size: %d\n", validation.ValidationSize)
	fmt.Println()

	bl, loadErr := narrative.LoadBaselines(workDir)
	if loadErr != nil {
		return fmt.Errorf("load baselines after save (should never happen): %w", loadErr)
	}
	if bl == nil {
		return fmt.Errorf("baselines file not found after save (SaveBaselines silently failed)")
	}

	fmt.Printf("Loaded baselines (window=%d):\n", bl.Window)
	for _, factor := range []string{"dxy", "us10y", "foreign_flow", "vix", "jpy", "geopolitical", "oil", "gold"} {
		entry, ok := bl.Baselines[factor]
		if !ok {
			fmt.Printf("  %-15s (no data)\n", factor)
			continue
		}
		if entry.Count == 0 {
			fmt.Printf("  %-15s count=0 (insufficient history)\n", factor)
			continue
		}
		fmt.Printf("  %-15s mean=%9.4f stddev=%9.4f count=%d\n",
			factor, entry.Mean, entry.StdDev, entry.Count)
	}
	fmt.Println()
	fmt.Printf("Next step: restart the atlas server so NewTaiwanStressCalculator\n")
	fmt.Printf("           auto-loads %s and enables hybrid signal.\n", baselinesPath)
	return nil
}
