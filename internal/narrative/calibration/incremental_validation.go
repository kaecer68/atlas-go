package calibration

import "math"

type DMStatistic struct {
	Stat float64
	PVal float64
	N    int
}

type IncrementalValidationResult struct {
	BaselineAccuracy   float64
	CandidateAccuracy  float64
	AccuracyGain       float64
	DM                 DMStatistic
	CandidateImproved  bool
	ImprovementPercent float64
}

type IncrementalValidator struct{}

func (v IncrementalValidator) CompareModels(baselineRecords, candidateRecords []CalibrationRecord) IncrementalValidationResult {
	baseCorrect, baseTotal, baseErrors := directionalStats(baselineRecords)
	candCorrect, candTotal, candErrors := directionalStats(candidateRecords)

	baseAcc := accuracyFromCounts(baseCorrect, baseTotal)
	candAcc := accuracyFromCounts(candCorrect, candTotal)
	result := IncrementalValidationResult{
		BaselineAccuracy:  baseAcc,
		CandidateAccuracy: candAcc,
		AccuracyGain:      candAcc - baseAcc,
		DM:                v.ComputeDieboldMariano(baseErrors, candErrors),
	}
	if baseAcc > 0 {
		result.ImprovementPercent = (candAcc - baseAcc) / baseAcc
	}
	result.CandidateImproved = candAcc > baseAcc
	return result
}

func (v IncrementalValidator) ComputeDieboldMariano(baselineErrors, candidateErrors []float64) DMStatistic {
	n := min(len(candidateErrors), len(baselineErrors))
	if n == 0 {
		return DMStatistic{}
	}
	var mean float64
	diffs := make([]float64, n)
	for i := range n {
		diffs[i] = baselineErrors[i] - candidateErrors[i]
		mean += diffs[i]
	}
	mean /= float64(n)
	var varSum float64
	for i := range n {
		delta := diffs[i] - mean
		varSum += delta * delta
	}
	if n < 2 || varSum == 0 {
		return DMStatistic{Stat: mean, PVal: 1, N: n}
	}
	std := math.Sqrt(varSum / float64(n-1))
	if std == 0 {
		return DMStatistic{Stat: mean, PVal: 1, N: n}
	}
	stat := mean / (std / math.Sqrt(float64(n)))
	return DMStatistic{Stat: stat, PVal: math.Erfc(math.Abs(stat) / math.Sqrt2), N: n}
}

func (v IncrementalValidator) ShouldAddFactor(result IncrementalValidationResult) bool {
	return result.CandidateAccuracy > result.BaselineAccuracy+0.05
}

func directionalStats(records []CalibrationRecord) (correct, total int, errors []float64) {
	errors = make([]float64, 0, len(records))
	for _, r := range records {
		if r.Outflow == 0 || r.OutflowTarget == 0 {
			continue
		}
		total++
		if sameDirection(r.Outflow, r.OutflowTarget) {
			correct++
		}
		errors = append(errors, math.Abs(r.Outflow-r.OutflowTarget))
	}
	return correct, total, errors
}

func accuracyFromCounts(correct, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(correct) / float64(total)
}
