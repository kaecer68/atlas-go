package experiment

import (
	"fmt"
	"math"
)

const DefaultSharpeStabilityThreshold = 0.5

// SharpeStabilityCheck reports whether a Sharpe ratio series is stably estimated.
// A series is considered stable when BOTH conditions hold:
//   - stderr < threshold  (precise estimation of the mean)
//   - |mean| / stddev >= 0.5  (mean is meaningful relative to dispersion; not just noise near zero)
//
// The second condition prevents flagging a series as "stable" when mean≈0 and stddev≈1,
// which in financial terms is not a reliable signal.
func SharpeStabilityCheck(sharpeSeries []float64, threshold float64) (stable bool, stderr float64, err error) {
	if len(sharpeSeries) < 2 {
		return false, 0, fmt.Errorf("insufficient data: need at least 2 data points")
	}

	mean := 0.0
	for _, v := range sharpeSeries {
		mean += v
	}
	mean /= float64(len(sharpeSeries))

	var sumSqDev float64
	for _, v := range sharpeSeries {
		diff := v - mean
		sumSqDev += diff * diff
	}

	stddev := math.Sqrt(sumSqDev / float64(len(sharpeSeries)-1))
	stderr = stddev / math.Sqrt(float64(len(sharpeSeries)))

	// Condition 1: precise estimation
	if stderr >= threshold {
		return false, stderr, nil
	}

	// Condition 2: mean is meaningfully large relative to dispersion
	// A series with |mean|/stddev < 0.5 is essentially noise around zero.
	if stddev > 0 && math.Abs(mean)/stddev < 0.5 {
		return false, stderr, nil
	}

	return true, stderr, nil
}
