package narrative

import (
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative/calibration"
)

// Facade re-exports calibration types and helpers that originally lived in the
// narrative package. Existing consumers can keep using the narrative import
// path while the canonical implementations live in narrative/calibration.

// Type aliases for stress-index calibration types.
type (
	StressIndexWeightsConfig = calibration.StressIndexWeightsConfig
	StressIndexScaling       = calibration.StressIndexScaling
	StressIndexWeights       = calibration.StressIndexWeights
	StressIndexThresholds    = calibration.StressIndexThresholds
	BaselineConfig           = calibration.BaselineConfig
	RegimeCalibratedConfig   = calibration.RegimeCalibratedConfig
	SignalStrategy           = calibration.SignalStrategy
	CalibrationRecord        = calibration.CalibrationRecord
	FactorBaseline           = calibration.FactorBaseline
	CalibrationValidation    = calibration.CalibrationValidation
	WeightCalibrationEngine  = calibration.WeightCalibrationEngine
	CalibrationTask          = calibration.CalibrationTask
)

// SignalStrategy constants.
const (
	SignalChange = calibration.SignalChange
	SignalLevel  = calibration.SignalLevel
	SignalHybrid = calibration.SignalHybrid
)

// Stress-index calibration constants.
const (
	StressScaleDXY          = calibration.StressScaleDXY
	StressScaleUS10Y        = calibration.StressScaleUS10Y
	StressScaleForeignFlow  = calibration.StressScaleForeignFlow
	StressScaleVIX          = calibration.StressScaleVIX
	StressScaleJPY          = calibration.StressScaleJPY
	StressScaleGeopolitical = calibration.StressScaleGeopolitical
	StressScaleOil          = calibration.StressScaleOil
	StressScaleGold         = calibration.StressScaleGold

	StressWeightDXY          = calibration.StressWeightDXY
	StressWeightUS10Y        = calibration.StressWeightUS10Y
	StressWeightForeignFlow  = calibration.StressWeightForeignFlow
	StressWeightVIX          = calibration.StressWeightVIX
	StressWeightJPY          = calibration.StressWeightJPY
	StressWeightGeopolitical = calibration.StressWeightGeopolitical
	StressWeightOil          = calibration.StressWeightOil
	StressWeightGold         = calibration.StressWeightGold

	StressThresholdCrisis = calibration.StressThresholdCrisis
	StressThresholdHigh   = calibration.StressThresholdHigh
	StressThresholdAlert  = calibration.StressThresholdAlert
)

// LoadBaselines loads persisted baselines from disk.
func LoadBaselines(workDir string) (*BaselineConfig, error) {
	return calibration.LoadBaselines(workDir)
}

// SaveBaselines persists baselines to disk.
func SaveBaselines(workDir string, cfg *BaselineConfig) error {
	return calibration.SaveBaselines(workDir, cfg)
}

// ComputeBaselines computes rolling baselines from calibration records.
func ComputeBaselines(records []CalibrationRecord, cfg *BaselineConfig) *BaselineConfig {
	return calibration.ComputeBaselines(records, cfg)
}

// ComputeLevelSignal returns the level-based z-score signal for a factor.
func ComputeLevelSignal(factor string, snap marketdata.MacroDataSnapshot, foreignNet float64, cfg *BaselineConfig) float64 {
	return calibration.ComputeLevelSignal(factor, snap, foreignNet, cfg)
}

// ComputeHybridSignal returns the stronger of change-based and level-based signals.
func ComputeHybridSignal(factor string, snap marketdata.MacroDataSnapshot, foreignNet float64, cfg *BaselineConfig) float64 {
	return calibration.ComputeHybridSignal(factor, snap, foreignNet, cfg)
}

// DefaultRegimeConfig returns a RegimeCalibratedConfig with default weights.
func DefaultRegimeConfig() *RegimeCalibratedConfig {
	return calibration.DefaultRegimeConfig()
}

// Path constants for baseline storage.
const (
	BaselinesDir      = calibration.BaselinesDir
	BaselinesFileName = calibration.BaselinesFileName
)

// LoadWeightsConfig reads stress index weights from the centralized parameters system.
// Deprecated: Use config.GetParametersConfig().Narrative for authoritative stress index parameters.
func LoadWeightsConfig(workDir string) *StressIndexWeightsConfig {
	return calibration.LoadWeightsConfig(workDir)
}

// DefaultCalibrationWeights returns the compile-time default stress index weights.
func DefaultCalibrationWeights() StressIndexWeights {
	return calibration.DefaultCalibrationWeights()
}

// ClassifyRegime determines the market regime from VIX value.
func ClassifyRegime(vix float64) calibration.MarketRegime {
	return calibration.ClassifyRegime(vix)
}

// NewCalibrationTask creates a new calibration task (legacy facade).
func NewCalibrationTask(workDir string) *calibration.CalibrationTask {
	return calibration.NewCalibrationTask(workDir)
}
