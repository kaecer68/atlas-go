package calibration

import (
	"math"
	"sort"
)

// ScaleCalibrator calibrates stress index scale factors from historical data.
// The goal is to normalize each factor so that under normal conditions,
// all factors contribute roughly equally to the total stress score.
//
// Formula: scale = TargetMedianContribution / median_abs_signal
// This ensures factors with large natural signals get small scales,
// and factors with tiny natural signals get large scales.
// All factors end up contributing ~TargetMedianContribution in median conditions.
type ScaleCalibrator struct {
	TargetMedianContribution float64 // default 20.0
}

// NewScaleCalibrator creates a ScaleCalibrator with default target of 20.0.
func NewScaleCalibrator() *ScaleCalibrator {
	return &ScaleCalibrator{
		TargetMedianContribution: 20.0,
	}
}

// WithTarget sets a custom target median contribution and returns the calibrator (fluent pattern).
func (c *ScaleCalibrator) WithTarget(target float64) *ScaleCalibrator {
	c.TargetMedianContribution = target
	return c
}

// CalibrateScales computes optimal scale factors from historical calibration records.
// For each factor:
//  1. Extract raw signal values from records (see signalForScaling)
//  2. Compute median absolute signal
//  3. scale = TargetMedianContribution / median_abs_signal
//  4. Clamp scale to [0.1, 100.0] to prevent division-by-zero and extreme values
//
// Returns StressIndexScaling with all 8 factors calibrated.
// If records are empty, returns StressIndexScaling with defaults (all 1.0).
func (c *ScaleCalibrator) CalibrateScales(records []CalibrationRecord) StressIndexScaling {
	if len(records) == 0 {
		return StressIndexScaling{
			DXY:          1.0,
			US10Y:        1.0,
			ForeignFlow:  1.0,
			VIX:          1.0,
			JPY:          1.0,
			Geopolitical: 1.0,
			Oil:          1.0,
			Gold:         1.0,
		}
	}

	factors := []string{"dxy", "us10y", "foreign_flow", "vix", "jpy", "geopolitical", "oil", "gold"}
	scales := make(map[string]float64, len(factors))

	for _, factor := range factors {
		signals := signalForScaling(factor, records)
		medianAbs := medianAbsValue(signals)
		if medianAbs == 0 {
			scales[factor] = 1.0
			continue
		}
		scale := c.TargetMedianContribution / medianAbs
		scales[factor] = clampScale(scale)
	}

	return StressIndexScaling{
		DXY:          scales["dxy"],
		US10Y:        scales["us10y"],
		ForeignFlow:  scales["foreign_flow"],
		VIX:          scales["vix"],
		JPY:          scales["jpy"],
		Geopolitical: scales["geopolitical"],
		Oil:          scales["oil"],
		Gold:         scales["gold"],
	}
}

// signalForScaling extracts the raw signal value used for scale calibration.
// For DXY/JPY/Oil/Gold: median of |hybrid_signal| where hybrid = max(|level_deviation|, |change_pct/100|)
//
//	If baselines are not available, fall back to |change_pct|.
//
// For US10Y: median of |snap.US10Y.Value| (already absolute yield)
// For ForeignFlow: median of |-snap.ForeignInvestorNet.Value| (absolute net flow in 億)
// For VIX: median of snap.VIX.Value (raw VIX, not scaled)
// For Geopolitical: median of geoScore (but we only have snap, so use 0 — skipped)
func signalForScaling(factor string, records []CalibrationRecord) []float64 {
	signals := make([]float64, 0, len(records))
	for _, r := range records {
		var v float64
		switch factor {
		case "dxy":
			v = absSignalForChangeFactor("dxy", r)
		case "jpy":
			v = absSignalForChangeFactor("jpy", r)
		case "oil":
			v = absSignalForChangeFactor("oil", r)
		case "gold":
			v = absSignalForChangeFactor("gold", r)
		case "us10y":
			v = math.Abs(r.Snapshot.US10Y.Value)
		case "foreign_flow":
			v = math.Abs(-r.Snapshot.ForeignInvestorNet.Value)
		case "vix":
			v = r.Snapshot.VIX.Value
		case "geopolitical":
			// Geopolitical score is not available in CalibrationRecord snapshots
			v = 0
		}
		// Filter out zero/NaN signals — they dilute the median
		if v != 0 && !math.IsNaN(v) && !math.IsInf(v, 0) {
			signals = append(signals, v)
		}
	}
	return signals
}

// absSignalForChangeFactor returns the absolute change-pct signal for change-pct-based
// factors (DXY, JPY, Oil, Gold).
func absSignalForChangeFactor(factor string, r CalibrationRecord) float64 {
	return math.Abs(factorChangePctFromSnapshot(factor, r))
}

// factorChangePctFromSnapshot extracts the change percentage for a factor from a record's snapshot.
func factorChangePctFromSnapshot(factor string, r CalibrationRecord) float64 {
	switch factor {
	case "dxy":
		return r.Snapshot.DXY.ChangePct
	case "jpy":
		return r.Snapshot.JPY.ChangePct
	case "oil":
		return r.Snapshot.Oil.ChangePct
	case "gold":
		return r.Snapshot.Gold.ChangePct
	default:
		return 0
	}
}

// medianAbsValue computes the median of absolute values in a float64 slice.
// Returns 0 if the slice is empty.
func medianAbsValue(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	abs := make([]float64, len(values))
	for i, v := range values {
		abs[i] = math.Abs(v)
	}
	sort.Float64s(abs)

	n := len(abs)
	if n%2 == 0 {
		return (abs[n/2-1] + abs[n/2]) / 2.0
	}
	return abs[n/2]
}

// clampScale restricts a scale factor to [0.1, 100.0] to prevent extreme values.
func clampScale(scale float64) float64 {
	const minScale = 0.1
	const maxScale = 100.0
	if scale < minScale {
		return minScale
	}
	if scale > maxScale {
		return maxScale
	}
	return scale
}
