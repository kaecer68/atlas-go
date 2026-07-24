package calibration

import (
	"math"
)

// CalibrationValidation holds the comparison results between old and new
// calibration configs. It is the output of the Phase 5 validation gate.
type CalibrationValidation struct {
	OldConfig StressIndexWeightsConfig `json:"old_config"`
	NewConfig StressIndexWeightsConfig `json:"new_config"`

	OldAccuracy float64 `json:"old_accuracy"` // direction accuracy using old config, [0, 1]
	NewAccuracy float64 `json:"new_accuracy"` // direction accuracy using new config, [0, 1]

	Improvement float64 `json:"improvement"` // NewAccuracy - OldAccuracy, can be negative

	IsDegradation  bool   `json:"is_degradation"`            // true when Improvement < -0.05
	DegradationMsg string `json:"degradation_msg,omitempty"` // human-readable warning

	ValidationSize int `json:"validation_size"` // number of records in validation set
	TrainingSize   int `json:"training_size"`   // number of records in training set
}

// ValidateCalibration performs out-of-sample validation comparing old and new
// configs. The approach:
//  1. Split records into training (first 80%) and validation (last 20%).
//  2. If records < 10: return validation with DegradationMsg "insufficient
//     data" and IsDegradation=true.
//  3. For each validation record, compute stress index score with old and new
//     configs using training records for baselines.
//  4. Compare each score direction against actual Outflow direction.
//  5. Compute accuracy and determine if the new config degrades performance.
func ValidateCalibration(
	records []CalibrationRecord,
	oldConfig StressIndexWeightsConfig,
	newConfig StressIndexWeightsConfig,
) CalibrationValidation {
	result := CalibrationValidation{
		OldConfig: oldConfig,
		NewConfig: newConfig,
	}

	if len(records) < 10 {
		result.IsDegradation = true
		result.DegradationMsg = "insufficient data: fewer than 10 records provided"
		result.TrainingSize = len(records)
		result.ValidationSize = 0
		return result
	}

	splitPoint := int(float64(len(records)) * 0.8)
	training := records[:splitPoint]
	validation := records[splitPoint:]

	result.TrainingSize = len(training)
	result.ValidationSize = len(validation)

	// Compute baselines from training set with window=60.
	trainingBaselines := ComputeBaselines(training, &BaselineConfig{Window: 60})

	var correctOld, correctNew int

	for _, rec := range validation {
		oldScore := computeStressFromConfig(rec, oldConfig, trainingBaselines)
		newScore := computeStressFromConfig(rec, newConfig, trainingBaselines)

		predictedOld := oldScore > 50.0
		predictedNew := newScore > 50.0
		actualOutflow := rec.Outflow > 0

		if predictedOld == actualOutflow {
			correctOld++
		}
		if predictedNew == actualOutflow {
			correctNew++
		}
	}

	if len(validation) > 0 {
		result.OldAccuracy = float64(correctOld) / float64(len(validation))
		result.NewAccuracy = float64(correctNew) / float64(len(validation))
	}

	result.Improvement = result.NewAccuracy - result.OldAccuracy

	if result.Improvement < -0.05 {
		result.IsDegradation = true
		result.DegradationMsg = "new config degrades accuracy beyond 5% threshold"
	}

	return result
}

// computeStressFromConfig computes a simplified stress score for a single
// snapshot using the given config. This is a lightweight version that doesn't
// need a full TaiwanStressCalculator — it just computes the score algebra.
//
// For each factor:
//
//	component = abs(raw_signal * scale) capped to [0, 100]
//	score = sum(component * weight for all 8 factors)
//
// When baselines is non-nil, ComputeHybridSignal is used for DXY/JPY/Oil/Gold.
// Otherwise, simple change-based signals are used.
// The geopolitical component is excluded from validation since CalibrationRecord
// doesn't carry geo scores.
func computeStressFromConfig(
	record CalibrationRecord,
	config StressIndexWeightsConfig,
	baselines *BaselineConfig,
) float64 {
	type factorEntry struct {
		name   string
		raw    float64
		scale  float64
		weight float64
	}

	// Build raw signals for each factor.
	// For DXY/JPY/Oil/Gold: use hybrid signal when baselines are available.
	entries := []factorEntry{
		{
			name: "dxy", scale: config.Scaling.DXY, weight: config.Weights.DXY,
			raw: changeOrHybrid("dxy", record, baselines),
		},
		{
			name: "us10y", scale: config.Scaling.US10Y, weight: config.Weights.US10Y,
			raw: math.Abs(record.Snapshot.US10Y.Value),
		},
		{
			name: "foreign_flow", scale: config.Scaling.ForeignFlow, weight: config.Weights.ForeignFlow,
			raw: -record.Snapshot.ForeignInvestorNet.Value,
		},
		{
			name: "vix", scale: config.Scaling.VIX, weight: config.Weights.VIX,
			raw: record.Snapshot.VIX.Value,
		},
		{
			name: "jpy", scale: config.Scaling.JPY, weight: config.Weights.JPY,
			raw: changeOrHybrid("jpy", record, baselines),
		},
		{
			name: "geopolitical", scale: config.Scaling.Geopolitical, weight: config.Weights.Geopolitical,
			raw: 0, // no geoScore available in CalibrationRecord
		},
		{
			name: "oil", scale: config.Scaling.Oil, weight: config.Weights.Oil,
			raw: changeOrHybrid("oil", record, baselines),
		},
		{
			name: "gold", scale: config.Scaling.Gold, weight: config.Weights.Gold,
			raw: changeOrHybrid("gold", record, baselines),
		},
	}

	var score float64
	for _, e := range entries {
		component := math.Abs(e.raw * e.scale)
		if component > 100.0 {
			component = 100.0
		}
		score += component * e.weight
	}

	return score
}

// changeOrHybrid returns the absolute signal for a change-pct factor.
// When baselines are provided and contain an entry for the factor, it uses
// ComputeHybridSignal. Otherwise it falls back to the simple absolute change-pct.
func changeOrHybrid(factor string, record CalibrationRecord, baselines *BaselineConfig) float64 {
	if baselines != nil && baselines.GetBaseline(factor) != nil {
		return ComputeHybridSignal(factor, record.Snapshot, record.ForeignNet, baselines)
	}
	// Fallback: simple absolute change-pct or value signal.
	return math.Abs(factorSignal(factor, record.Snapshot, record.ForeignNet))
}
