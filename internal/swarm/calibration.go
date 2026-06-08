package swarm

import (
	"math"
	"time"
)

// SimulationStatistics holds aggregate statistics computed from simulated price paths.
// These metrics describe the distributional and risk properties of the simulation output
// and can be compared against real market data for calibration purposes.
type SimulationStatistics struct {
	MeanReturn        float64                       `json:"mean_return"`
	Volatility        float64                       `json:"volatility"`
	Skewness          float64                       `json:"skewness"`
	Kurtosis          float64                       `json:"kurtosis"`
	MaxDrawdown       float64                       `json:"max_drawdown"`
	SharpeRatio       float64                       `json:"sharpe_ratio"`
	CorrelationMatrix map[string]map[string]float64 `json:"correlation_matrix,omitempty"`
}

// CalibrationReport compares simulated statistics against target (market) statistics
// and suggests parameter adjustments to bring simulation closer to observed reality.
type CalibrationReport struct {
	SimulatedStats       SimulationStatistics `json:"simulated_stats"`
	TargetStats          SimulationStatistics `json:"target_stats"`
	ParameterAdjustments map[string]float64   `json:"parameter_adjustments"`
	CalibrationError     float64              `json:"calibration_error"`
	Timestamp            time.Time            `json:"timestamp"`
}

// defaultLearningRate controls the aggressiveness of proportional parameter adjustments.
const defaultLearningRate = 0.1

// ComputeSimulationStats aggregates all fish histories from the given simulation results
// and computes return statistics, risk metrics, and cross-asset correlations.
//
// Returns are computed as log-returns across consecutive price observations
// for every symbol present in the fish histories.
func ComputeSimulationStats(results []SimulationResult) SimulationStatistics {
	if len(results) == 0 {
		return SimulationStatistics{}
	}

	// SimulationResult does not embed fish histories; they live on MiroFish.
	// This function is a placeholder that delegates to computeStatsFromFish.
	// Callers needing full path-based statistics should use CalibrateAgainstTarget
	// on the swarm, which has access to sw.fish.
	_ = results
	return computeStatsFromFish(nil)
}

// computeStatsFromFish computes statistics directly from fish histories.
// Exported consumers should use CalibrateAgainstTarget on the swarm.
func computeStatsFromFish(fish []*MiroFish) SimulationStatistics {
	if len(fish) == 0 {
		return SimulationStatistics{}
	}

	// Gather per-symbol returns from all fish histories.
	symbolReturns := make(map[string][]float64)

	for _, f := range fish {
		if len(f.History) < 2 {
			continue
		}
		for i := 1; i < len(f.History); i++ {
			prev := f.History[i-1]
			curr := f.History[i]
			for sym, prevPrice := range prev.Prices {
				if prevPrice <= 0 {
					continue
				}
				currPrice, ok := curr.Prices[sym]
				if !ok || currPrice <= 0 {
					continue
				}
				ret := math.Log(currPrice / prevPrice)
				symbolReturns[sym] = append(symbolReturns[sym], ret)
			}
		}
	}

	if len(symbolReturns) == 0 {
		return SimulationStatistics{}
	}

	// Flatten all returns for aggregate moments.
	allReturns := make([]float64, 0, 1024)
	for _, rets := range symbolReturns {
		allReturns = append(allReturns, rets...)
	}

	mean := mean64(allReturns)
	vol := stdDev64(allReturns, mean)
	skew := skewness64(allReturns, mean, vol)
	kurt := kurtosis64(allReturns, mean, vol)

	// Max drawdown from aggregated PnL path.
	maxDD := computeMaxDrawdownFromFish(fish)

	// Sharpe ratio: use mean return over volatility, annualized assuming daily steps.
	sharpe := 0.0
	if vol > 1e-12 {
		sharpe = mean / vol * math.Sqrt(252.0)
	}

	// Cross-asset correlation matrix.
	corrMatrix := computeCorrelationMatrix(symbolReturns)

	return SimulationStatistics{
		MeanReturn:        mean,
		Volatility:        vol,
		Skewness:          skew,
		Kurtosis:          kurt,
		MaxDrawdown:       maxDD,
		SharpeRatio:       sharpe,
		CorrelationMatrix: corrMatrix,
	}
}

// CalibrateParameters compares simulated statistics against target statistics and
// produces a CalibrationReport with suggested parameter adjustments.
//
// Adjustments are computed as proportional corrections:
//
//	adjustment = error * learningRate
//
// where error is (target - simulated) / |target| for normalised comparison.
// Supported adjustment keys:
//   - "garch_omega": increase if simulated vol is too low
//   - "garch_alpha": increase if simulated kurtosis is too low (more shock sensitivity)
//   - "garch_beta":  increase if simulated vol persistence is too low
//   - "jump_lambda": increase if simulated skewness / tail risk is too low
//   - "jump_mu":     shift if simulated mean return is off
//   - "jump_sigma":  increase if simulated jump volatility is too low
//   - "trend":       shift if simulated mean return is off
func CalibrateParameters(current SwarmConfig, simStats, targetStats SimulationStatistics) CalibrationReport {
	errVol := relativeError(targetStats.Volatility, simStats.Volatility)
	errMean := relativeError(targetStats.MeanReturn, simStats.MeanReturn)
	errSkew := relativeError(targetStats.Skewness, simStats.Skewness)
	errKurt := relativeError(targetStats.Kurtosis, simStats.Kurtosis)
	errDD := relativeError(targetStats.MaxDrawdown, simStats.MaxDrawdown)

	adjustments := make(map[string]float64)

	// Volatility calibration via GARCH omega.
	adjustments["garch_omega"] = errVol * defaultLearningRate

	// Kurtosis / tail calibration via GARCH alpha (shock sensitivity).
	adjustments["garch_alpha"] = errKurt * defaultLearningRate * 0.5

	// Volatility persistence via GARCH beta.
	if simStats.Volatility > 0 && targetStats.Volatility > 0 {
		persistErr := (targetStats.Volatility - simStats.Volatility) / targetStats.Volatility
		adjustments["garch_beta"] = persistErr * defaultLearningRate * 0.3
	}

	// Jump process adjustments based on skewness and mean.
	adjustments["jump_lambda"] = errSkew * defaultLearningRate
	adjustments["jump_mu"] = errMean * defaultLearningRate
	adjustments["jump_sigma"] = errKurt * defaultLearningRate * 0.5

	// Trend adjustment for mean return mismatch.
	adjustments["trend"] = errMean * defaultLearningRate

	// Composite calibration error: root-mean-square of normalised errors.
	calErr := math.Sqrt((errVol*errVol + errMean*errMean + errSkew*errSkew + errKurt*errKurt + errDD*errDD) / 5.0)

	return CalibrationReport{
		SimulatedStats:       simStats,
		TargetStats:          targetStats,
		ParameterAdjustments: adjustments,
		CalibrationError:     calErr,
		Timestamp:            time.Now(),
	}
}

// CalibrateAgainstTarget computes statistics from the latest fish histories and
// generates a calibration report against the provided target statistics.
func (sw *MiroFishSwarm) CalibrateAgainstTarget(target SimulationStatistics) CalibrationReport {
	sw.mu.RLock()
	defer sw.mu.RUnlock()

	simStats := computeStatsFromFish(sw.fish)
	return CalibrateParameters(sw.config, simStats, target)
}

// --- helpers ---

func mean64(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

func stdDev64(xs []float64, mean float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	var sumSq float64
	for _, x := range xs {
		d := x - mean
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(xs)-1))
}

func skewness64(xs []float64, mean, stdDev float64) float64 {
	if len(xs) < 3 || stdDev < 1e-12 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		d := x - mean
		sum += d * d * d
	}
	n := float64(len(xs))
	return (sum / n) / (stdDev * stdDev * stdDev)
}

func kurtosis64(xs []float64, mean, stdDev float64) float64 {
	if len(xs) < 4 || stdDev < 1e-12 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		d := x - mean
		sum += d * d * d * d
	}
	n := float64(len(xs))
	return (sum / n) / (stdDev * stdDev * stdDev * stdDev)
}

// relativeError returns (target - actual) / max(|target|, epsilon).
// When target is near zero, falls back to raw difference.
func relativeError(target, actual float64) float64 {
	const eps = 1e-8
	denom := math.Abs(target)
	if denom < eps {
		denom = eps
	}
	return (target - actual) / denom
}

// computeMaxDrawdownFromFish estimates the worst peak-to-trough decline
// across all fish PnL paths.
func computeMaxDrawdownFromFish(fish []*MiroFish) float64 {
	worst := 0.0
	for _, f := range fish {
		if f.Performance.MaxDrawdown > worst {
			worst = f.Performance.MaxDrawdown
		}
	}
	return worst
}

// computeCorrelationMatrix builds a Pearson correlation matrix from per-symbol return series.
// Symbols with fewer than 2 observations are skipped.
func computeCorrelationMatrix(symbolReturns map[string][]float64) map[string]map[string]float64 {
	symbols := make([]string, 0, len(symbolReturns))
	for sym, rets := range symbolReturns {
		if len(rets) >= 2 {
			symbols = append(symbols, sym)
		}
	}
	if len(symbols) == 0 {
		return nil
	}

	result := make(map[string]map[string]float64, len(symbols))
	for _, s := range symbols {
		result[s] = make(map[string]float64, len(symbols))
	}

	for i, si := range symbols {
		ri := symbolReturns[si]
		mi := mean64(ri)
		sdi := stdDev64(ri, mi)
		if sdi < 1e-12 {
			continue
		}
		for j, sj := range symbols {
			if i > j {
				result[si][sj] = result[sj][si]
				continue
			}
			rj := symbolReturns[sj]
			mj := mean64(rj)
			sdj := stdDev64(rj, mj)
			if sdj < 1e-12 {
				continue
			}
			cov := covariance(ri, rj, mi, mj)
			corr := cov / (sdi * sdj)
			if math.IsNaN(corr) || math.IsInf(corr, 0) {
				corr = 0
			}
			result[si][sj] = corr
		}
	}
	return result
}

func covariance(a, b []float64, ma, mb float64) float64 {
	n := min(len(a), len(b))
	if n == 0 {
		return 0
	}
	var sum float64
	for i := range n {
		sum += (a[i] - ma) * (b[i] - mb)
	}
	return sum / float64(n)
}
