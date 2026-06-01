package retail

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// CalibrationMetadata tracks a unit of calibration evidence.
type CalibrationMetadata struct {
	Parameter         string    `json:"parameter"`
	Before            float64   `json:"before"`
	After             float64   `json:"after"`
	EvidenceQuality   string    `json:"evidence_quality"`
	SampleSize        int       `json:"sample_size"`
	CalibrationMethod string    `json:"calibration_method"`
	ImprovementPct    float64   `json:"improvement_pct"`
	Timestamp         time.Time `json:"timestamp"`
}

// CalibrationReport records the result of an autonomous RSI-tw calibration run.
type CalibrationReport struct {
	Timestamp   time.Time             `json:"timestamp"`
	SampleCount int                   `json:"sample_count"`
	Score       float64               `json:"score"`
	Changes     []CalibrationMetadata `json:"changes"`
	Verdict     string                `json:"verdict"`
	Summary     string                `json:"summary"`
}

// CalibrationResult contains the raw data used for a calibration pass.
type calibrationResult struct {
	Timestamp        time.Time
	PartAScore       float64
	PartCScore       float64
	Score            float64
	AdjustmentFactor float64
	ScoreVariance    float64 // inter-indicator variance
	NonZeroCount     int     // count of sub-indicators with ZScore ≠ 0.5
}

const (
	calibrationHistoryFile = "rsi_tw_calibration.json"
	maxCalibrationHistory  = 3
	maxChangePct           = 0.20 // safety: max 20% change per parameter per run
	minImprovementPct      = 0.02 // minimum improvement to apply changes
)

// CalibrateRSITw runs autonomous RSI-tw parameter calibration using historical
// macro snapshots. It evaluates the current parameter set against grid-search
// alternatives and applies changes within safety limits.
func CalibrateRSITw(workDir string) (*CalibrationReport, error) {
	calData, err := loadCalibrationData(workDir)
	if err != nil {
		return nil, fmt.Errorf("calibrate_rsi_tw: load data: %w", err)
	}
	if len(calData) < 10 {
		return &CalibrationReport{
			Timestamp:   time.Now(),
			SampleCount: len(calData),
			Verdict:     "insufficient_data",
			Summary:     fmt.Sprintf("need ≥10 snapshots, got %d", len(calData)),
		}, nil
	}

	// Compute scores with current params
	calculator := NewCalculator()
	currentParams := config.GetParametersConfig().RSITw
	calculator.SetParams(currentParams)
	currentResults := computeCalibrationResults(calculator, &currentParams, calData)

	baselineScore := avgScore(currentResults)

	// Grid search: perturb weights by ±10% and evaluate
	variants := []struct {
		name   string
		value  float64
		getter func(*config.RSITwParameters) float64
		setter func(*config.RSITwParameters, float64)
	}{
		{"a1_weight", currentParams.A1Weight.Value, func(p *config.RSITwParameters) float64 { return p.A1Weight.Value }, func(p *config.RSITwParameters, v float64) { p.A1Weight.Value = v }},
		{"a2_weight", currentParams.A2Weight.Value, func(p *config.RSITwParameters) float64 { return p.A2Weight.Value }, func(p *config.RSITwParameters, v float64) { p.A2Weight.Value = v }},
		{"a3_weight", currentParams.A3Weight.Value, func(p *config.RSITwParameters) float64 { return p.A3Weight.Value }, func(p *config.RSITwParameters, v float64) { p.A3Weight.Value = v }},
		{"a4_weight", currentParams.A4Weight.Value, func(p *config.RSITwParameters) float64 { return p.A4Weight.Value }, func(p *config.RSITwParameters, v float64) { p.A4Weight.Value = v }},
		{"a5_weight", currentParams.A5Weight.Value, func(p *config.RSITwParameters) float64 { return p.A5Weight.Value }, func(p *config.RSITwParameters, v float64) { p.A5Weight.Value = v }},
		{"a6_weight", currentParams.A6Weight.Value, func(p *config.RSITwParameters) float64 { return p.A6Weight.Value }, func(p *config.RSITwParameters, v float64) { p.A6Weight.Value = v }},
		{"a_part_weight", currentParams.APartWeight.Value, func(p *config.RSITwParameters) float64 { return p.APartWeight.Value }, func(p *config.RSITwParameters, v float64) { p.APartWeight.Value = v }},
		{"c_part_weight", currentParams.CPartWeight.Value, func(p *config.RSITwParameters) float64 { return p.CPartWeight.Value }, func(p *config.RSITwParameters, v float64) { p.CPartWeight.Value = v }},
	}

	var bestChanges []CalibrationMetadata
	bestScore := baselineScore

	for _, variant := range variants {
		original := variant.getter(&currentParams)
		for _, delta := range []float64{1.10, 0.90} { // ±10%
			candidate := original * delta
			if candidate <= 0 {
				continue
			}

			pcopy := currentParams
			variant.setter(&pcopy, candidate)
			cResults := computeCalibrationResults(calculator, &pcopy, calData)
			cScore := avgScore(cResults)

			improvement := 0.0
			if baselineScore > 0 {
				improvement = (cScore - baselineScore) / baselineScore
			}

			if improvement > minImprovementPct && cScore > bestScore {
				// Check safety limit
				changePct := (candidate - original) / original
				if changePct > maxChangePct || changePct < -maxChangePct {
					continue
				}
				bestScore = cScore
				bestChanges = append(bestChanges, CalibrationMetadata{
					Parameter:         variant.name,
					Before:            original,
					After:             candidate,
					EvidenceQuality:   "medium",
					SampleSize:        len(calData),
					CalibrationMethod: "grid_search_10pct",
					ImprovementPct:    improvement,
					Timestamp:         time.Now(),
				})
			}
		}
	}

	report := &CalibrationReport{
		Timestamp:   time.Now(),
		SampleCount: len(calData),
		Score:       baselineScore,
		Changes:     bestChanges,
	}

	if len(bestChanges) == 0 {
		report.Verdict = "no_improvement"
		report.Summary = fmt.Sprintf("grid search yielded no improvement over baseline score %.4f", baselineScore)
	} else {
		report.Verdict = "calibrated"
		report.Score = bestScore
		report.Summary = fmt.Sprintf("%d parameter(s) adjusted, best score %.4f (baseline %.4f)", len(bestChanges), bestScore, baselineScore)

		// Apply changes to the global parameter config
		cfg := config.GetParametersConfig()
		for _, ch := range bestChanges {
			applyChange(&cfg.RSITw, ch)
			logging.Info("rsi_tw_calibrate", "parameter_adjusted",
				"param", ch.Parameter,
				"before", ch.Before,
				"after", ch.After,
				"improvement", fmt.Sprintf("%.2f%%", ch.ImprovementPct*100))
		}
		if err := config.GetParametersConfig().SaveWithRollback(filepath.Join(workDir, "configs", "parameters.json")); err != nil {
			logging.Error("rsi_tw_calibrate", "save_params_failed", "err", err.Error())
			return report, fmt.Errorf("save calibrated params: %w", err)
		}

		// Update calculator instance with new params
		GetCalculator().SetParams(cfg.RSITw)
	}

	// Persist calibration report for API/history
	saveCalibrationReport(workDir, report)
	return report, nil
}

func applyChange(p *config.RSITwParameters, ch CalibrationMetadata) {
	switch ch.Parameter {
	case "a1_weight":
		p.A1Weight.Value = ch.After
	case "a2_weight":
		p.A2Weight.Value = ch.After
	case "a3_weight":
		p.A3Weight.Value = ch.After
	case "a4_weight":
		p.A4Weight.Value = ch.After
	case "a5_weight":
		p.A5Weight.Value = ch.After
	case "a6_weight":
		p.A6Weight.Value = ch.After
	case "a_part_weight":
		p.APartWeight.Value = ch.After
	case "c_part_weight":
		p.CPartWeight.Value = ch.After
	}
}

func avgScore(results []calibrationResult) float64 {
	if len(results) == 0 {
		return 0
	}
	var sum float64
	for _, r := range results {
		sum += math.Abs(r.Score) // prefer scores with higher magnitude (more signal, less 0.5 clustering)
	}
	return sum / float64(len(results))
}

func computeCalibrationResults(calc *Calculator, params *config.RSITwParameters, inputs []RSITwInput) []calibrationResult {
	calc.SetParams(*params)
	results := make([]calibrationResult, 0, len(inputs))
	for _, in := range inputs {
		snap := calc.ComputeFinal(in)
		nonzero := 0
		for _, si := range snap.SubIndicators {
			if si.ZScore != 0.5 && !si.IsFallback {
				nonzero++
			}
		}
		results = append(results, calibrationResult{
			Timestamp:        snap.Timestamp,
			PartAScore:       snap.PartAScore,
			PartCScore:       snap.PartCScore,
			Score:            snap.Score,
			AdjustmentFactor: snap.AdjustmentFactor,
			NonZeroCount:     nonzero,
		})
	}
	return results
}

func loadCalibrationData(workDir string) ([]RSITwInput, error) {
	pattern := filepath.Join(workDir, "data", "state", "macro", "*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	type macroEntry struct {
		ts   time.Time
		data json.RawMessage
	}
	var entries []macroEntry

	for _, path := range matches {
		if filepath.Base(path) == "latest.json" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		// Parse timestamp from filename or content
		var raw struct {
			RecordedAt string `json:"recorded_at"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			continue
		}
		ts, err := time.Parse(time.RFC3339, raw.RecordedAt)
		if err != nil {
			// Try alternate formats
			ts, err = time.Parse("2006-01-02T15:04:05Z07:00", raw.RecordedAt)
			if err != nil {
				continue
			}
		}
		entries = append(entries, macroEntry{ts: ts, data: data})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ts.Before(entries[j].ts)
	})

	var inputs []RSITwInput
	for _, e := range entries {
		var snap struct {
			RetailMarginBalance struct {
				Value float64 `json:"value"`
			} `json:"retail_margin_balance"`
			VIX struct {
				Value float64 `json:"value"`
			} `json:"vix"`
			ForeignInvestorNet struct {
				Value float64 `json:"value"`
			} `json:"foreign_investor_net"`
			DomesticFundNet struct {
				Value float64 `json:"value"`
			} `json:"domestic_fund_net"`
		}
		if err := json.Unmarshal(e.data, &snap); err != nil {
			continue
		}
		if snap.VIX.Value <= 0 || snap.RetailMarginBalance.Value <= 0 {
			continue
		}
		inputs = append(inputs, RSITwInput{
			MarginBalance:      snap.RetailMarginBalance.Value,
			VIXLevel:           snap.VIX.Value,
			ForeignInvestorNet: snap.ForeignInvestorNet.Value,
			DomesticFundNet:    snap.DomesticFundNet.Value,
		})
	}
	return inputs, nil
}

func saveCalibrationReport(workDir string, report *CalibrationReport) {
	path := filepath.Join(workDir, "data", "state", calibrationHistoryFile)
	// Load existing reports
	var reports []*CalibrationReport
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &reports)
	}
	reports = append([]*CalibrationReport{report}, reports...)
	if len(reports) > maxCalibrationHistory {
		reports = reports[:maxCalibrationHistory]
	}
	data, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		logging.Error("rsi_tw_calibrate", "marshal_report_failed", "err", err.Error())
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		logging.Error("rsi_tw_calibrate", "write_report_failed", "err", err.Error())
	}
}

// LoadLastCalibrationReport reads the most recent calibration report from disk.
func LoadLastCalibrationReport(workDir string) (*CalibrationReport, error) {
	path := filepath.Join(workDir, "data", "state", calibrationHistoryFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var reports []*CalibrationReport
	if err := json.Unmarshal(data, &reports); err != nil {
		return nil, err
	}
	if len(reports) == 0 {
		return nil, fmt.Errorf("no calibration reports found")
	}
	return reports[0], nil
}
