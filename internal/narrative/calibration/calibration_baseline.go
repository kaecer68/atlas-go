package calibration

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// FactorBaseline holds the rolling mean and standard deviation for a single
// macro factor. It represents the "normal range" against which current
// readings are compared to produce level-based signals.
type FactorBaseline struct {
	Factor string  `json:"factor"`
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"std_dev"`
	Count  int     `json:"count"`
}

// BaselineConfig holds rolling baselines for all macro factors used in the
// stress index. When provided to calculators, it enables level-aware signal
// computation in addition to the standard change-pct signal.
type BaselineConfig struct {
	Baselines map[string]*FactorBaseline `json:"baselines"`
	Window    int                        `json:"window"` // rolling window in days
}

// GetBaseline returns the baseline for a given factor, or nil if not available.
func (c *BaselineConfig) GetBaseline(factor string) *FactorBaseline {
	if c == nil || c.Baselines == nil {
		return nil
	}
	return c.Baselines[factor]
}

// WindowSize returns the configured rolling window size, defaulting to 30.
func (c *BaselineConfig) WindowSize() int {
	if c == nil || c.Window <= 0 {
		return 30
	}
	return c.Window
}

// ComputeBaselines computes rolling mean and standard deviation for each
// factor from the provided calibration records. Only the most recent
// BaselineConfig.Window records are used.
func ComputeBaselines(records []CalibrationRecord, cfg *BaselineConfig) *BaselineConfig {
	window := 30
	if cfg != nil && cfg.Window > 0 {
		window = cfg.Window
	}

	if len(records) == 0 {
		return &BaselineConfig{Baselines: map[string]*FactorBaseline{}, Window: window}
	}

	// Use the most recent `window` records.
	if len(records) > window {
		records = records[len(records)-window:]
	}

	factors := []string{"dxy", "us10y", "foreign_flow", "vix", "jpy", "geopolitical", "oil", "gold"}
	baselines := make(map[string]*FactorBaseline, len(factors))

	for _, factor := range factors {
		values := extractFactorValues(factor, records)
		if len(values) == 0 {
			baselines[factor] = &FactorBaseline{Factor: factor, Count: 0}
			continue
		}
		m := mean(values)
		sd := stdDev(values)
		baselines[factor] = &FactorBaseline{
			Factor: factor,
			Mean:   m,
			StdDev: sd,
			Count:  len(values),
		}
	}

	return &BaselineConfig{Baselines: baselines, Window: window}
}

// extractFactorValues extracts the raw signal value for a factor from each record.
func extractFactorValues(factor string, records []CalibrationRecord) []float64 {
	values := make([]float64, 0, len(records))
	for _, r := range records {
		v := factorSignal(factor, r.Snapshot, r.ForeignNet)
		values = append(values, v)
	}
	return values
}

// ComputeLevelSignal returns a level-based signal: the z-score deviation
// from the baseline mean. Returns 0 if the baseline is nil or has zero
// standard deviation (degenerate case — no signal possible from level alone).
func ComputeLevelSignal(factor string, snap marketdata.MacroDataSnapshot, foreignNet float64, cfg *BaselineConfig) float64 {
	bl := cfg.GetBaseline(factor)
	if bl == nil || bl.Count == 0 {
		return 0
	}
	current := factorSignal(factor, snap, foreignNet)
	if bl.StdDev == 0 {
		return 0
	}
	return (current - bl.Mean) / bl.StdDev
}

// ComputeHybridSignal returns the signal with the larger absolute magnitude
// between the change-based signal and the level-based z-score signal.
// This ensures that even on stable days (ChangePct=0), extreme absolute levels
// still generate meaningful signal.
func ComputeHybridSignal(factor string, snap marketdata.MacroDataSnapshot, foreignNet float64, cfg *BaselineConfig) float64 {
	changeSignal := factorSignal(factor, snap, foreignNet)
	levelSignal := ComputeLevelSignal(factor, snap, foreignNet, cfg)

	if math.Abs(levelSignal) > math.Abs(changeSignal) {
		return levelSignal
	}
	return changeSignal
}

// stdDev computes the population standard deviation of a float64 slice.
func stdDev(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	m := mean(values)
	sum := 0.0
	for _, v := range values {
		d := v - m
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(values)))
}

const BaselinesFileName = "baselines.json"

// BaselinesDir is an alias for constants.StateCalibration to preserve
// backward compatibility for external callers.
const BaselinesDir = constants.StateCalibration

func SaveBaselines(workDir string, cfg *BaselineConfig) error {
	if cfg == nil {
		return fmt.Errorf("save baselines: nil config")
	}
	if workDir == "" {
		return fmt.Errorf("save baselines: empty workDir")
	}
	path := filepath.Join(workDir, BaselinesDir, BaselinesFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("save baselines: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("save baselines: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("save baselines: write: %w", err)
	}
	return nil
}

func LoadBaselines(workDir string) (*BaselineConfig, error) {
	if workDir == "" {
		return nil, fmt.Errorf("load baselines: empty workDir")
	}
	path := filepath.Join(workDir, BaselinesDir, BaselinesFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("load baselines: read: %w", err)
	}
	var cfg BaselineConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("load baselines: unmarshal: %w", err)
	}
	return &cfg, nil
}
