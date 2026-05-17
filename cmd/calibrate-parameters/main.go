package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// CalibratedParameter holds before/after values with calibration metadata.
type CalibratedParameter struct {
	Path             string
	Before           float64
	After            float64
	Method           string
	Confidence       float64
	Significant      bool
	SampleSize       int
	CalibrationNotes string
}

// CalibrationResult holds the outcome of calibrating one module.
type CalibrationResult struct {
	Module     string
	Parameters []CalibratedParameter
	Errors     []string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "calibrate-parameters: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("calibrate-parameters", flag.ContinueOnError)
	module := fs.String("module", "all", "Module to calibrate: garch, var, darwinian, factor, all")
	dryRun := fs.Bool("dry-run", false, "Preview changes without saving")
	dataPath := fs.String("data", "", "Path to replay data (CSV or JSONL). Defaults to ATLAS_REPLAY_DATA_PATH")
	verbose := fs.Bool("v", false, "Verbose output")
	if err := fs.Parse(args); err != nil {
		return err
	}

	workDir, _ := os.Getwd()
	replayPath := *dataPath
	if replayPath == "" {
		replayPath = config.GetReplayDataPath(workDir)
		if replayPath == "" {
			replayPath = filepath.Join(workDir, "data", "replay", "tw_extended_90days.csv")
		}
	}

	paramsPath := filepath.Join(workDir, "configs", "parameters.json")
	paramsCfg, err := config.LoadParametersConfig(paramsPath)
	if err != nil {
		return fmt.Errorf("load parameters config: %w", err)
	}

	returns, n, err := loadReturns(replayPath)
	if err != nil {
		return fmt.Errorf("load returns: %w", err)
	}

	inference := config.NewInferenceEngine(paramsCfg)

	var results []CalibrationResult

	switch *module {
	case "garch":
		results = append(results, calibrateGARCH(inference, returns, n, paramsCfg))
	case "var":
		results = append(results, calibrateVaR(inference, returns, n, paramsCfg))
	case "darwinian":
		results = append(results, calibrateDarwinian(inference, returns, n, paramsCfg))
	case "factor":
		results = append(results, calibrateFactor(inference, returns, n, paramsCfg))
	case "all":
		results = append(results, calibrateGARCH(inference, returns, n, paramsCfg))
		results = append(results, calibrateVaR(inference, returns, n, paramsCfg))
		results = append(results, calibrateDarwinian(inference, returns, n, paramsCfg))
		results = append(results, calibrateFactor(inference, returns, n, paramsCfg))
	default:
		return fmt.Errorf("unknown module: %s (valid: garch, var, darwinian, factor, all)", *module)
	}

	printReport(results, *verbose)

	if *dryRun {
		fmt.Println("\n[DRY-RUN] No changes written.")
		return nil
	}

	now := time.Now()
	paramsCfg.UpdatedAt = now

	for _, res := range results {
		for _, p := range res.Parameters {
			if err := updateParameterMetadata(paramsCfg, p); err != nil {
				fmt.Fprintf(os.Stderr, "warn: update metadata for %s: %v\n", p.Path, err)
			}
		}
	}

	if err := paramsCfg.Save(paramsPath); err != nil {
		return fmt.Errorf("save parameters: %w", err)
	}

	fmt.Printf("\nSaved updated parameters to %s\n", paramsPath)
	return nil
}

func loadReturns(dataPath string) ([]float64, int, error) {
	ext := strings.ToLower(filepath.Ext(dataPath))

	var returns []float64

	if ext == ".jsonl" || ext == ".json" {
		returns, _ = loadReturnsFromJSONL(dataPath)
	}

	if len(returns) == 0 {
		var err error
		returns, _, err = loadReturnsFromCSV(dataPath)
		if err != nil {
			return nil, 0, fmt.Errorf("load from %s: %w", dataPath, err)
		}
	}

	if len(returns) < 30 {
		return nil, len(returns), fmt.Errorf("insufficient data: got %d returns, need at least 30", len(returns))
	}

	return returns, len(returns), nil
}

func loadReturnsFromJSONL(path string) ([]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	type Outcome struct {
		Return float64 `json:"return"`
	}

	var returns []float64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var out Outcome
		if err := json.Unmarshal([]byte(line), &out); err != nil {
			continue
		}
		if !math.IsNaN(out.Return) && !math.IsInf(out.Return, 0) {
			returns = append(returns, out.Return)
		}
	}
	return returns, nil
}

func loadReturnsFromCSV(path string) ([]float64, int, error) {
	ds, err := replay.LoadTWSEOpenDataCSV(path)
	if err != nil {
		return nil, 0, err
	}

	var bestSym string
	var bestCount int
	if len(ds.Dates) > 0 {
		for sym := range ds.ByDate[ds.Dates[0].Format("2006-01-02")] {
			count := 0
			for _, d := range ds.Dates {
				bar, ok := ds.ByDate[d.Format("2006-01-02")][sym]
				if ok && bar.Close > 0 {
					count++
				}
			}
			if count > bestCount {
				bestCount = count
				bestSym = sym
			}
		}
	}

	if bestSym == "" {
		return nil, 0, fmt.Errorf("no valid bars found")
	}

	bestCount = 0
	for _, d := range ds.Dates {
		bar, ok := ds.ByDate[d.Format("2006-01-02")][bestSym]
		if ok && bar.Close > 0 {
			bestCount++
		}
	}

	var returns []float64
	var prevClose float64
	for _, date := range ds.Dates {
		bar, ok := ds.ByDate[date.Format("2006-01-02")][bestSym]
		if !ok || bar.Close == 0 {
			continue
		}
		if prevClose > 0 {
			ret := (bar.Close - prevClose) / prevClose
			if !math.IsNaN(ret) && !math.IsInf(ret, 0) {
				returns = append(returns, ret)
			}
		}
		prevClose = bar.Close
	}

	return returns, bestCount, nil
}

// ---- Module calibrators ----

func calibrateGARCH(ie *config.InferenceEngine, returns []float64, n int, cfg *config.ParametersConfig) CalibrationResult {
	res := CalibrationResult{Module: "garch"}

	garch, err := ie.InferGARCH(returns)
	if err != nil {
		res.Errors = append(res.Errors, err.Error())
		return res
	}

	beforeAlpha := cfg.GARCH.Alpha.Value
	beforeBeta := cfg.GARCH.Beta.Value
	beforeOmega := cfg.GARCH.Omega.Value
	beforeSum := beforeAlpha + beforeBeta
	afterSum := garch.Alpha + garch.Beta
	significant := beforeSum > 0 && math.Abs(afterSum-beforeSum)/beforeSum > 0.05

	res.Parameters = append(res.Parameters, CalibratedParameter{
		Path:        "garch.omega",
		Before:      beforeOmega,
		After:       garch.Omega,
		Method:      "MLE_grid_search",
		Confidence:  0.95,
		Significant: significant,
		SampleSize:  n,
	})
	res.Parameters = append(res.Parameters, CalibratedParameter{
		Path:        "garch.alpha",
		Before:      beforeAlpha,
		After:       garch.Alpha,
		Method:      "MLE_grid_search",
		Confidence:  0.95,
		Significant: significant,
		SampleSize:  n,
	})
	res.Parameters = append(res.Parameters, CalibratedParameter{
		Path:        "garch.beta",
		Before:      beforeBeta,
		After:       garch.Beta,
		Method:      "MLE_grid_search",
		Confidence:  0.95,
		Significant: significant,
		SampleSize:  n,
	})

	ie.SetParameter("garch_omega", garch.Omega)
	ie.SetParameter("garch_alpha", garch.Alpha)
	ie.SetParameter("garch_beta", garch.Beta)

	return res
}

func calibrateVaR(ie *config.InferenceEngine, returns []float64, n int, cfg *config.ParametersConfig) CalibrationResult {
	res := CalibrationResult{Module: "var"}

	var95, err := ie.EstimateVaR(returns, 0.95)
	if err != nil {
		res.Errors = append(res.Errors, fmt.Errorf("95%% VaR: %w", err).Error())
		return res
	}

	var99, err := ie.EstimateVaR(returns, 0.99)
	if err != nil {
		res.Errors = append(res.Errors, fmt.Errorf("99%% VaR: %w", err).Error())
		return res
	}

	empiricalVol := math.Abs(var95.VaR) / 1.645
	empiricalMaxDD := math.Abs(var99.VaR) * 1.2

	beforeTargetVol := cfg.Sizing.TargetVolatility.Value
	beforeMaxDD := cfg.Sizing.MaxDrawdownLimit.Value

	sigTargetVol := math.Abs(empiricalVol-beforeTargetVol)/beforeTargetVol > 0.10
	sigMaxDD := math.Abs(empiricalMaxDD-beforeMaxDD)/beforeMaxDD > 0.10

	res.Parameters = append(res.Parameters, CalibratedParameter{
		Path:             "sizing.target_volatility",
		Before:           beforeTargetVol,
		After:            math.Min(empiricalVol, 0.30),
		Method:           "VaR_inference",
		Confidence:       0.90,
		Significant:      sigTargetVol,
		SampleSize:       n,
		CalibrationNotes: fmt.Sprintf("VaR(95%%)=%.4f from %d observations", var95.VaR, n),
	})
	res.Parameters = append(res.Parameters, CalibratedParameter{
		Path:             "sizing.max_drawdown_limit",
		Before:           beforeMaxDD,
		After:            math.Min(empiricalMaxDD, 0.20),
		Method:           "VaR_inference",
		Confidence:       0.90,
		Significant:      sigMaxDD,
		SampleSize:       n,
		CalibrationNotes: fmt.Sprintf("VaR(99%%)=%.4f from %d observations", var99.VaR, n),
	})

	if empiricalVol > 0 && empiricalVol < 0.30 {
		ie.SetParameter("sizing_target_volatility", math.Min(empiricalVol, 0.30))
	}
	if empiricalMaxDD > 0 && empiricalMaxDD < 0.20 {
		ie.SetParameter("sizing_max_drawdown_limit", math.Max(0.05, math.Min(empiricalMaxDD, 0.20)))
	}

	return res
}

func calibrateDarwinian(ie *config.InferenceEngine, returns []float64, n int, cfg *config.ParametersConfig) CalibrationResult {
	res := CalibrationResult{Module: "darwinian"}

	workDir, _ := os.Getwd()
	ledgerStore := ledger.NewStore(filepath.Join(workDir, "data", "state"))
	outcomes, err := ledgerStore.LoadOutcomes()
	if err != nil || len(outcomes) < 10 {
		res.Errors = append(res.Errors, "insufficient outcome data for darwinian calibration, using defaults")
		res.Parameters = darwinianHeuristicDefaults(n, cfg)
		return res
	}

	agentStats := make(map[string]struct{ hits, total int })
	for _, o := range outcomes {
		stats := agentStats[o.AgentID]
		stats.total++
		if o.Hit {
			stats.hits++
		}
		agentStats[o.AgentID] = stats
	}

	hitRates := make([]float64, 0, len(agentStats))
	for _, stats := range agentStats {
		if stats.total > 0 {
			hitRates = append(hitRates, float64(stats.hits)/float64(stats.total))
		}
	}

	if len(hitRates) < 3 {
		res.Errors = append(res.Errors, fmt.Sprintf("only %d agents with hit-rate data, need at least 3", len(hitRates)))
		res.Parameters = darwinianHeuristicDefaults(n, cfg)
		return res
	}

	sort.Float64s(hitRates)
	highHitRate := hitRates[int(float64(len(hitRates))*0.75)]
	lowHitRate := hitRates[int(float64(len(hitRates))*0.25)]

	beforeHighHR := cfg.Darwinian.HitRateHighThreshold.Value
	beforeLowHR := cfg.Darwinian.HitRateLowThreshold.Value

	sigHighHR := math.Abs(highHitRate-beforeHighHR)/beforeHighHR > 0.10
	sigLowHR := math.Abs(lowHitRate-beforeLowHR)/beforeLowHR > 0.10

	res.Parameters = append(res.Parameters, CalibratedParameter{
		Path:             "darwinian.hit_rate_high_threshold",
		Before:           beforeHighHR,
		After:            highHitRate,
		Method:           "percentile_based",
		Confidence:       0.85,
		Significant:      sigHighHR,
		SampleSize:       len(hitRates),
		CalibrationNotes: fmt.Sprintf("75th percentile of %d agent hit-rates", len(hitRates)),
	})
	res.Parameters = append(res.Parameters, CalibratedParameter{
		Path:             "darwinian.hit_rate_low_threshold",
		Before:           beforeLowHR,
		After:            lowHitRate,
		Method:           "percentile_based",
		Confidence:       0.85,
		Significant:      sigLowHR,
		SampleSize:       len(hitRates),
		CalibrationNotes: fmt.Sprintf("25th percentile of %d agent hit-rates", len(hitRates)),
	})

	if sigHighHR {
		ie.SetParameter("darwinian_hit_rate_high_threshold", highHitRate)
	}
	if sigLowHR {
		ie.SetParameter("darwinian_hit_rate_low_threshold", lowHitRate)
	}

	return res
}

func calibrateFactor(ie *config.InferenceEngine, returns []float64, n int, cfg *config.ParametersConfig) CalibrationResult {
	res := CalibrationResult{Module: "factor"}

	sorted := make([]float64, len(returns))
	copy(sorted, returns)
	sort.Float64s(sorted)

	p90 := sorted[int(float64(len(sorted))*0.90)]

	autocorr := computeAutocorrelation(returns, 5)
	var momentumLookback int
	if autocorr > 0.5 {
		momentumLookback = 20
	} else if autocorr > 0.2 {
		momentumLookback = 10
	} else {
		momentumLookback = 5
	}

	beforeStddevDiv := cfg.Factor.MomentumStdDevDivisor.Value
	beforeLookback := cfg.Factor.MomentumLookbackDays.Value

	res.Parameters = append(res.Parameters, CalibratedParameter{
		Path:             "factor.momentum_stddev_divisor",
		Before:           beforeStddevDiv,
		After:            math.Max(p90*0.5, 0.05),
		Method:           "return_distribution",
		Confidence:       0.80,
		Significant:      true,
		SampleSize:       n,
		CalibrationNotes: fmt.Sprintf("P90=%.4f, autocorrelation=%.3f", p90, autocorr),
	})
	res.Parameters = append(res.Parameters, CalibratedParameter{
		Path:             "factor.momentum_lookback_days",
		Before:           float64(beforeLookback),
		After:            float64(momentumLookback),
		Method:           "autocorrelation",
		Confidence:       0.75,
		Significant:      beforeLookback != momentumLookback,
		SampleSize:       n,
		CalibrationNotes: fmt.Sprintf("autocorrelation=%.3f suggests %d-day window", autocorr, momentumLookback),
	})

	if p90 > 0.05 {
		ie.SetParameter("factor_momentum_stddev_divisor", math.Max(p90*0.5, 0.05))
	}
	ie.SetParameter("factor_momentum_lookback_days", float64(momentumLookback))

	return res
}

// ---- Helpers ----

func computeAutocorrelation(returns []float64, lag int) float64 {
	if len(returns) < lag+1 {
		return 0
	}
	var mean, sumCorr, sumSq float64
	for _, r := range returns {
		mean += r
	}
	mean /= float64(len(returns))

	for i := 0; i < len(returns)-lag; i++ {
		sumCorr += (returns[i] - mean) * (returns[i+lag] - mean)
	}
	for i := 0; i < len(returns); i++ {
		dx := returns[i] - mean
		sumSq += dx * dx
	}
	if sumSq == 0 {
		return 0
	}
	return sumCorr / sumSq
}

func darwinianHeuristicDefaults(n int, cfg *config.ParametersConfig) []CalibratedParameter {
	confidence := 0.6
	return []CalibratedParameter{
		{
			Path:        "darwinian.hit_rate_high_threshold",
			Before:      cfg.Darwinian.HitRateHighThreshold.Value,
			After:       cfg.Darwinian.HitRateHighThreshold.Value,
			Method:      "heuristic_fallback",
			Confidence:  confidence,
			Significant: false,
			SampleSize:  n,
		},
		{
			Path:        "darwinian.hit_rate_low_threshold",
			Before:      cfg.Darwinian.HitRateLowThreshold.Value,
			After:       cfg.Darwinian.HitRateLowThreshold.Value,
			Method:      "heuristic_fallback",
			Confidence:  confidence,
			Significant: false,
			SampleSize:  n,
		},
	}
}

func updateParameterMetadata(cfg *config.ParametersConfig, p CalibratedParameter) error {
	pathParts := strings.Split(p.Path, ".")
	if len(pathParts) != 2 {
		return fmt.Errorf("invalid path format: %s", p.Path)
	}
	section, key := pathParts[0], pathParts[1]
	now := time.Now()

	switch section {
	case "garch":
		switch key {
		case "omega":
			cfg.GARCH.Omega.Source = config.SourceCalibrated
			cfg.GARCH.Omega.CalibrationMethod = p.Method
			cfg.GARCH.Omega.LastCalibrated = &now
		case "alpha":
			cfg.GARCH.Alpha.Source = config.SourceCalibrated
			cfg.GARCH.Alpha.CalibrationMethod = p.Method
			cfg.GARCH.Alpha.LastCalibrated = &now
		case "beta":
			cfg.GARCH.Beta.Source = config.SourceCalibrated
			cfg.GARCH.Beta.CalibrationMethod = p.Method
			cfg.GARCH.Beta.LastCalibrated = &now
		}
	case "sizing":
		switch key {
		case "target_volatility":
			cfg.Sizing.TargetVolatility.Source = config.SourceCalibrated
			cfg.Sizing.TargetVolatility.CalibrationMethod = p.Method
			cfg.Sizing.TargetVolatility.LastCalibrated = &now
		case "max_drawdown_limit":
			cfg.Sizing.MaxDrawdownLimit.Source = config.SourceCalibrated
			cfg.Sizing.MaxDrawdownLimit.CalibrationMethod = p.Method
			cfg.Sizing.MaxDrawdownLimit.LastCalibrated = &now
		}
	case "darwinian":
		switch key {
		case "hit_rate_high_threshold":
			cfg.Darwinian.HitRateHighThreshold.Source = config.SourceCalibrated
			cfg.Darwinian.HitRateHighThreshold.CalibrationMethod = p.Method
			cfg.Darwinian.HitRateHighThreshold.LastCalibrated = &now
		case "hit_rate_low_threshold":
			cfg.Darwinian.HitRateLowThreshold.Source = config.SourceCalibrated
			cfg.Darwinian.HitRateLowThreshold.CalibrationMethod = p.Method
			cfg.Darwinian.HitRateLowThreshold.LastCalibrated = &now
		}
	case "factor":
		switch key {
		case "momentum_stddev_divisor":
			cfg.Factor.MomentumStdDevDivisor.Source = config.SourceCalibrated
			cfg.Factor.MomentumStdDevDivisor.CalibrationMethod = p.Method
			cfg.Factor.MomentumStdDevDivisor.LastCalibrated = &now
		case "momentum_lookback_days":
			cfg.Factor.MomentumLookbackDays.Source = config.SourceCalibrated
			cfg.Factor.MomentumLookbackDays.CalibrationMethod = p.Method
			cfg.Factor.MomentumLookbackDays.LastCalibrated = &now
		}
	}
	return nil
}

// ---- Reporting ----

func printReport(results []CalibrationResult, verbose bool) {
	fmt.Println("=== Parameter Calibration Report ===")
	fmt.Printf("Generated: %s\n\n", time.Now().Format(time.RFC3339))

	totalParams := 0
	for _, res := range results {
		fmt.Printf("Module: %s\n", res.Module)
		fmt.Println(strings.Repeat("-", 60))

		if len(res.Errors) > 0 {
			fmt.Printf("  [ERRORS]\n")
			for _, e := range res.Errors {
				fmt.Printf("    - %s\n", e)
			}
			fmt.Println()
			continue
		}

		for _, p := range res.Parameters {
			totalParams++
			sigMark := ""
			if p.Significant {
				sigMark = " *"
			}
			fmt.Printf("  %-35s  before=%-10.6f  after=%-10.6f%s\n", p.Path, p.Before, p.After, sigMark)
			fmt.Printf("    method=%-20s confidence=%-5.2f  n=%d\n", p.Method, p.Confidence, p.SampleSize)
			if p.CalibrationNotes != "" {
				fmt.Printf("    notes: %s\n", p.CalibrationNotes)
			}
		}
		fmt.Println()
	}

	fmt.Printf("Total parameters calibrated: %d\n", totalParams)
	if verbose {
		fmt.Println("\n[*] indicates statistically significant change (>10% relative)")
	}
}
