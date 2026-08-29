package portfolio

import (
	"math"

	"gonum.org/v1/gonum/mat"
)

// GetEfficientFrontier computes the mean-variance efficient frontier (20 points).
func (o *Optimizer) GetEfficientFrontier() []struct{ Return, Risk float64 } {
	o.mu.RLock()
	hp := o.history
	wMax := o.constraints.MaxPositionPct
	o.mu.RUnlock()

	if hp == nil {
		return nil
	}

	symbols := []string{
		"2330.TW", "2317.TW", "2454.TW", "2308.TW", "2881.TW",
		"2882.TW", "1301.TW", "1303.TW", "2412.TW", "2002.TW",
	}
	rm := o.extractReturnMatrix(symbols)
	if rm == nil || len(rm.assets) < 2 {
		return nil
	}

	sample := o.sampleCov(rm)
	if sample == nil {
		return nil
	}
	sigma := o.ledoitWolfShrink(rm, sample)

	N := len(rm.assets)
	lb := make([]float64, N)
	ub := make([]float64, N)
	for i := range N {
		ub[i] = wMax
	}

	minRet := rm.means[0]
	maxRet := rm.means[0]
	for _, m := range rm.means {
		if m < minRet {
			minRet = m
		}
		if m > maxRet {
			maxRet = m
		}
	}

	daysPerYear := 252.0
	const numPoints = 20

	Aeq := mat.NewDense(2, N, nil)
	for j := range N {
		Aeq.Set(0, j, 1.0)
	}

	frontier := make([]struct{ Return, Risk float64 }, numPoints)
	for k := range numPoints {
		frac := float64(k) / float64(numPoints-1)
		rTarget := minRet + frac*(maxRet-minRet)
		for j := range N {
			Aeq.Set(1, j, rm.means[j])
		}
		beq := []float64{1.0, rTarget}

		wInit := make([]float64, N)
		for i := range N {
			wInit[i] = 1.0 / float64(N)
		}

		wOpt := o.activeSetQP(sigma, Aeq, beq, lb, ub, wInit)

		var portVar float64
		for i := range N {
			for j := range N {
				portVar += wOpt[i] * sigma.At(i, j) * wOpt[j]
			}
		}
		portRisk := math.Sqrt(portVar) * math.Sqrt(daysPerYear)
		annRet := rTarget * daysPerYear

		frontier[k] = struct{ Return, Risk float64 }{Return: annRet, Risk: portRisk}
	}

	return frontier
}
