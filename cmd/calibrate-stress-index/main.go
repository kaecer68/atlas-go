package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/narrative"
)

func main() {
	workDir := flag.String("workdir", ".", "working directory")
	window := flag.Int("window", 252, "historical window in days")
	output := flag.String("output", "configs/stress_index_weights.json", "output config path")
	flag.Parse()

	if err := run(*workDir, *window, *output, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(workDir string, window int, outputPath string, stdout, stderr io.Writer) error {
	engine := &narrative.WeightCalibrationEngine{}
	records, err := engine.LoadHistoricalData(workDir, window)
	if err != nil {
		return fmt.Errorf("load historical data: %w", err)
	}

	accuracies := engine.ComputeFactorAccuracy(records)
	weights := engine.CalibrateWeights(accuracies)
	scaling := baselineScaling()
	thresholds := baselineThresholds()
	if err := engine.ExportConfig(workDir, weights, scaling, thresholds); err != nil {
		return fmt.Errorf("export config: %w", err)
	}

	if err := copyOutputIfNeeded(workDir, outputPath); err != nil {
		return fmt.Errorf("sync output: %w", err)
	}

	baseline := baselineWeights()
	printSummary(stdout, baseline, weights, accuracies)
	return nil
}

func printSummary(stdout io.Writer, baseline, weights narrative.StressIndexWeights, accuracies map[string]float64) {
	_, _ = fmt.Fprintln(stdout, "Stress index calibration summary")
	_, _ = fmt.Fprintf(stdout, "Records used: %d\n", len(accuracies))
	_, _ = fmt.Fprintln(stdout, "")
	_, _ = fmt.Fprintln(stdout, "Factor weights:")
	for _, row := range factorSummaryRows(baseline, weights, accuracies) {
		_, _ = fmt.Fprintf(stdout, "- %s: baseline=%.2f calibrated=%.2f accuracy=%.1f%%\n", row.name, row.baseline, row.calibrated, row.accuracy*100)
	}
	_, _ = fmt.Fprintf(stdout, "\nTotal accuracy improvement: %.2f%%\n", (weightedAccuracy(weights, accuracies)-weightedAccuracy(baseline, accuracies))*100)
	if below := factorsBelowFloor(weights, 0.05); len(below) > 0 {
		_, _ = fmt.Fprintln(stdout, "Factors below 5% floor:")
		for _, name := range below {
			_, _ = fmt.Fprintf(stdout, "- %s\n", name)
		}
	}
}

type factorRow struct {
	name       string
	baseline   float64
	calibrated float64
	accuracy   float64
}

func factorSummaryRows(baseline, calibrated narrative.StressIndexWeights, accuracies map[string]float64) []factorRow {
	return []factorRow{
		{name: "dxy", baseline: baseline.DXY, calibrated: calibrated.DXY, accuracy: accuracies["dxy"]},
		{name: "us10y", baseline: baseline.US10Y, calibrated: calibrated.US10Y, accuracy: accuracies["us10y"]},
		{name: "foreign_flow", baseline: baseline.ForeignFlow, calibrated: calibrated.ForeignFlow, accuracy: accuracies["foreign_flow"]},
		{name: "vix", baseline: baseline.VIX, calibrated: calibrated.VIX, accuracy: accuracies["vix"]},
		{name: "jpy", baseline: baseline.JPY, calibrated: calibrated.JPY, accuracy: accuracies["jpy"]},
		{name: "geopolitical", baseline: baseline.Geopolitical, calibrated: calibrated.Geopolitical, accuracy: accuracies["geopolitical"]},
		{name: "oil", baseline: baseline.Oil, calibrated: calibrated.Oil, accuracy: accuracies["oil"]},
		{name: "gold", baseline: baseline.Gold, calibrated: calibrated.Gold, accuracy: accuracies["gold"]},
	}
}

func weightedAccuracy(weights narrative.StressIndexWeights, accuracies map[string]float64) float64 {
	return weights.DXY*accuracies["dxy"] + weights.US10Y*accuracies["us10y"] + weights.ForeignFlow*accuracies["foreign_flow"] +
		weights.VIX*accuracies["vix"] + weights.JPY*accuracies["jpy"] + weights.Geopolitical*accuracies["geopolitical"] +
		weights.Oil*accuracies["oil"] + weights.Gold*accuracies["gold"]
}

func factorsBelowFloor(weights narrative.StressIndexWeights, floor float64) []string {
	items := []struct {
		name  string
		value float64
	}{
		{name: "dxy", value: weights.DXY},
		{name: "us10y", value: weights.US10Y},
		{name: "foreign_flow", value: weights.ForeignFlow},
		{name: "vix", value: weights.VIX},
		{name: "jpy", value: weights.JPY},
		{name: "geopolitical", value: weights.Geopolitical},
		{name: "oil", value: weights.Oil},
		{name: "gold", value: weights.Gold},
	}
	var out []string
	for _, item := range items {
		if item.value < floor {
			out = append(out, item.name)
		}
	}
	return out
}

func baselineWeights() narrative.StressIndexWeights {
	return narrative.StressIndexWeights{DXY: 0.13, US10Y: 0.18, ForeignFlow: 0.22, VIX: 0.13, JPY: 0.08, Geopolitical: 0.13, Oil: 0.07, Gold: 0.06}
}

func baselineScaling() narrative.StressIndexScaling {
	return narrative.StressIndexScaling{DXY: 5.0, US10Y: 2.0, ForeignFlow: 10.0, VIX: 100.0 / 40.0, JPY: 10.0, Geopolitical: 1.0, Oil: 2.0, Gold: 2.0}
}

func baselineThresholds() narrative.StressIndexThresholds {
	return narrative.StressIndexThresholds{Crisis: 70.0, High: 50.0, Alert: 30.0}
}

func copyOutputIfNeeded(workDir, outputPath string) error {
	defaultPath := filepath.Join(workDir, "configs", "stress_index_weights.json")
	if filepath.Clean(outputPath) == filepath.Clean(filepath.Join("configs", "stress_index_weights.json")) || filepath.Clean(outputPath) == filepath.Clean(defaultPath) {
		return nil
	}
	data, err := os.ReadFile(defaultPath)
	if err != nil {
		return fmt.Errorf("read default config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("mkdir output dir: %w", err)
	}
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return fmt.Errorf("write output config: %w", err)
	}
	return nil
}
