package narrative

import (
	"math"
)

type DivergenceDetector struct {
	marginHistory  []float64
	foreignHistory []float64
}

func NewDivergenceDetector() *DivergenceDetector {
	return &DivergenceDetector{
		marginHistory:  make([]float64, 0),
		foreignHistory: make([]float64, 0),
	}
}

func (d *DivergenceDetector) Update(marginBalance, prevMargin, foreignNet, prevForeignNet float64) {
	d.marginHistory = append(d.marginHistory, marginBalance-prevMargin)
	d.foreignHistory = append(d.foreignHistory, foreignNet-prevForeignNet)
	const maxHistory = 60
	if len(d.marginHistory) > maxHistory {
		d.marginHistory = d.marginHistory[len(d.marginHistory)-maxHistory:]
	}
	if len(d.foreignHistory) > maxHistory {
		d.foreignHistory = d.foreignHistory[len(d.foreignHistory)-maxHistory:]
	}
}

func (d *DivergenceDetector) RetailDivergenceAndMarginZScore(currentMargin, currentForeignNet float64) (float64, float64) {
	if len(d.marginHistory) < 60 || len(d.foreignHistory) < 60 {
		return 0, 0
	}
	if currentMargin == currentForeignNet {
		return 0, 0
	}
	marginMean := mean(d.marginHistory)
	marginStd := stddev(d.marginHistory)
	foreignMean := mean(d.foreignHistory)
	foreignStd := stddev(d.foreignHistory)
	marginZScore := 0.0
	foreignZScore := 0.0
	if marginStd > 0 {
		marginZScore = (currentMargin - marginMean) / marginStd
	}
	if foreignStd > 0 {
		foreignZScore = (currentForeignNet - foreignMean) / foreignStd
	}
	if marginZScore == 0 || foreignZScore == 0 {
		return 0, marginZScore
	}
	if marginZScore > 0 && foreignZScore > 0 {
		return 0, marginZScore
	}
	if marginZScore < 0 && foreignZScore < 0 {
		return 0, marginZScore
	}
	strength := math.Min(math.Abs(marginZScore), math.Abs(foreignZScore))
	if marginZScore > 0 && foreignZScore < 0 {
		return strength, marginZScore
	}
	return -strength, marginZScore
}

func mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var s float64
	for _, v := range vals {
		s += v
	}
	return s / float64(len(vals))
}

func stddev(vals []float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	m := mean(vals)
	var sumSq float64
	for _, v := range vals {
		dx := v - m
		sumSq += dx * dx
	}
	return math.Sqrt(sumSq / float64(len(vals)-1))
}
