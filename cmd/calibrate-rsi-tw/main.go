package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

type calibrationResult struct {
	GeneratedAt     time.Time            `json:"generated_at"`
	SampleSize      int                  `json:"sample_size"`
	DateRange       [2]string            `json:"date_range"`
	Method          string               `json:"method"`
	VIXThresholds   []float64            `json:"vix_thresholds"`
	VIXScores       []float64            `json:"vix_scores"`
	VIXDistribution vixDistributionStats `json:"vix_distribution"`
	MarginStats     marginStats          `json:"margin_stats"`
	Notes           []string             `json:"notes"`
}

type vixDistributionStats struct {
	Mean    float64   `json:"mean"`
	StdDev  float64   `json:"std_dev"`
	Min     float64   `json:"min"`
	Max     float64   `json:"max"`
	P10     float64   `json:"p10"`
	P25     float64   `json:"p25"`
	P50     float64   `json:"p50"`
	P75     float64   `json:"p75"`
	P90     float64   `json:"p90"`
	Samples []float64 `json:"samples,omitempty"`
}

type marginStats struct {
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"std_dev"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	P50    float64 `json:"p50"`
	P90    float64 `json:"p90"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "calibrate-rsi-tw: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("calibrate-rsi-tw", flag.ContinueOnError)
	workDir := fs.String("work-dir", ".", "Working directory (path to atlas root)")
	outputPath := fs.String("output", "", "Output JSON path (default: stdout)")
	dryRun := fs.Bool("dry-run", false, "Dry run: print report, do not update parameters")
	updateParams := fs.Bool("update", false, "Update configs/parameters.json with calibrated values")
	if err := fs.Parse(args); err != nil {
		return err
	}

	macroDir := filepath.Join(*workDir, "data", "state", "macro")
	snapshots, err := loadMacroSnapshots(macroDir)
	if err != nil {
		return fmt.Errorf("load macro snapshots: %w", err)
	}
	if len(snapshots) < 5 {
		return fmt.Errorf("insufficient data: need >=5 snapshots, got %d", len(snapshots))
	}

	result := calibrate(snapshots)

	if *updateParams && !*dryRun {
		if err := applyCalibration(*workDir, result); err != nil {
			return fmt.Errorf("apply calibration: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Updated %s/configs/parameters.json with calibrated VIX values\n", *workDir)
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}

	if *outputPath != "" {
		if err := os.WriteFile(*outputPath, data, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Calibration report written to %s\n", *outputPath)
	} else {
		fmt.Println(string(data))
	}

	return nil
}

func loadMacroSnapshots(dir string) ([]marketdata.MacroDataSnapshot, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var snaps []marketdata.MacroDataSnapshot
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var snap marketdata.MacroDataSnapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			continue
		}
		// Only include snapshots with valid VIX data.
		if snap.VIX.Value > 0 {
			snaps = append(snaps, snap)
		}
	}

	sort.Slice(snaps, func(i, j int) bool {
		return snaps[i].RecordedAt < snaps[j].RecordedAt
	})

	return snaps, nil
}

func calibrate(snapshots []marketdata.MacroDataSnapshot) calibrationResult {
	result := calibrationResult{
		GeneratedAt: time.Now(),
		SampleSize:  len(snapshots),
		DateRange: [2]string{
			time.Unix(snapshots[0].RecordedAt, 0).Format("2006-01-02"),
			time.Unix(snapshots[len(snapshots)-1].RecordedAt, 0).Format("2006-01-02"),
		},
		Method: "percentile_from_historical_distribution",
	}

	// Extract VIX values.
	vixVals := make([]float64, 0, len(snapshots))
	for _, s := range snapshots {
		if s.VIX.Value > 0 {
			vixVals = append(vixVals, s.VIX.Value)
		}
	}
	sort.Float64s(vixVals)
	result.VIXDistribution = computeDistribution(vixVals)

	// Calibrate VIX thresholds as percentiles of the historical distribution.
	result.VIXThresholds = []float64{
		percentile(vixVals, 0.10), // very calm
		percentile(vixVals, 0.30), // calm
		percentile(vixVals, 0.50), // moderate
		percentile(vixVals, 0.70), // elevated
		percentile(vixVals, 0.90), // high
	}

	// Scores ordered: [vix < T0, T0 <= vix < T1, ..., vix >= T4]
	// Higher VIX → bearish → higher score. Monotonic 0.1 → 1.0 across 6 buckets.
	result.VIXScores = []float64{0.1, 0.28, 0.46, 0.64, 0.82, 1.0}

	// Extract margin balances.
	marginVals := make([]float64, 0, len(snapshots))
	for _, s := range snapshots {
		if s.RetailMarginBalance.Value > 0 {
			marginVals = append(marginVals, s.RetailMarginBalance.Value)
		}
	}
	sort.Float64s(marginVals)
	if len(marginVals) > 0 {
		result.MarginStats = marginStats{
			Mean:   mean(marginVals),
			StdDev: stddev(marginVals),
			Min:    marginVals[0],
			Max:    marginVals[len(marginVals)-1],
			P50:    percentile(marginVals, 0.50),
			P90:    percentile(marginVals, 0.90),
		}
	}

	result.Notes = append(
		result.Notes,
		"VIX thresholds derived from historical percentile distribution — replaces heuristic [15,20,25,30,35]",
		"Scores are evenly spaced 0.1→1.0 across 6 VIX buckets for monotonic mapping",
		"Margin stats provided for A1 Z-score calibration evidence",
		"Part C thresholds require futures/flow/ETF data — use gateway channels or external sources",
		"Part D multipliers require event study — calibrate with geopolitical event database",
	)

	return result
}

func computeDistribution(vals []float64) vixDistributionStats {
	if len(vals) == 0 {
		return vixDistributionStats{}
	}
	return vixDistributionStats{
		Mean:    mean(vals),
		StdDev:  stddev(vals),
		Min:     vals[0],
		Max:     vals[len(vals)-1],
		P10:     percentile(vals, 0.10),
		P25:     percentile(vals, 0.25),
		P50:     percentile(vals, 0.50),
		P75:     percentile(vals, 0.75),
		P90:     percentile(vals, 0.90),
		Samples: vals,
	}
}

func applyCalibration(workDir string, result calibrationResult) error {
	paramsPath := filepath.Join(workDir, "configs", "parameters.json")
	cfg, err := config.LoadParametersConfig(paramsPath)
	if err != nil {
		// Load defaults if file doesn't exist yet.
		cfg = config.DefaultParametersConfig()
	}
	cfg.UpdatedAt = time.Now()

	// Only update if we have enough data confidence.
	if result.SampleSize >= 10 {
		cfg.RSITw.A4VixThresholds.Value = result.VIXThresholds
		cfg.RSITw.A4VixScores.Value = result.VIXScores
		cfg.RSITw.A4VixThresholds.Source = config.SourceCalibrated
		cfg.RSITw.A4VixScores.Source = config.SourceCalibrated
		now := time.Now()
		cfg.RSITw.A4VixThresholds.LastCalibrated = &now
		cfg.RSITw.A4VixScores.LastCalibrated = &now
		cfg.RSITw.A4VixThresholds.CalibrationMethod = "percentile_distribution"
		cfg.RSITw.A4VixScores.CalibrationMethod = "even_spacing_6_buckets"
	}

	return cfg.SaveWithRollback(paramsPath)
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p * float64(len(sorted)-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	if lo == hi || hi >= len(sorted) {
		return sorted[lo]
	}
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

func mean(vs []float64) float64 {
	if len(vs) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vs {
		sum += v
	}
	return sum / float64(len(vs))
}

func stddev(vs []float64) float64 {
	if len(vs) < 2 {
		return 0
	}
	m := mean(vs)
	var sumSq float64
	for _, v := range vs {
		d := v - m
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(vs)))
}
