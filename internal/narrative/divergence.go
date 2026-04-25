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

func (d *DivergenceDetector) Update(marginBalance, foreignNet float64) {
	d.marginHistory = append(d.marginHistory, marginBalance)
	d.foreignHistory = append(d.foreignHistory, foreignNet)
	const maxHistory = 60
	if len(d.marginHistory) > maxHistory {
		d.marginHistory = d.marginHistory[len(d.marginHistory)-maxHistory:]
	}
	if len(d.foreignHistory) > maxHistory {
		d.foreignHistory = d.foreignHistory[len(d.foreignHistory)-maxHistory:]
	}
}

func (d *DivergenceDetector) RetailDivergenceAndMarginZScore(currentMargin, currentForeignNet float64) (float64, float64) {
	if len(d.marginHistory) < 10 {
		return 0, 0
	}
	marginMean := mean(d.marginHistory[:len(d.marginHistory)-1])
	marginStd := stddev(d.marginHistory[:len(d.marginHistory)-1])
	marginZScore := 0.0
	if marginStd > 0 {
		marginZScore = (currentMargin - marginMean) / marginStd
	}
	divergence := currentMargin*1e9 + currentForeignNet
	return divergence, marginZScore
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
