package globalmarket

import (
	"math"
	"sync"
)

// RollingBeta computes a rolling OLS beta (slope) between two return series
// using a ring buffer. Default window is 60 observations.
// Also tracks downside (market return < 0) and upside (market return >= 0)
// conditional betas separately, enabling asymmetric risk analysis.
type RollingBeta struct {
	mu          sync.RWMutex
	window      int
	xs          []float64
	ys          []float64
	pos         int
	count       int
	currentBeta float64
	alpha       float64
	r2          float64

	// Downside-only tracking (x < 0)
	dxs           []float64
	dys           []float64
	dpos          int
	dcount        int
	downsideBeta  float64
	downsideAlpha float64
	downsideR2    float64

	// Upside-only tracking (x >= 0)
	uxs         []float64
	uys         []float64
	upos        int
	ucount      int
	upsideBeta  float64
	upsideAlpha float64
	upsideR2    float64
}

// NewRollingBeta creates a rolling beta tracker with the given window size.
// Default 60 if window <= 0.
func NewRollingBeta(window int) *RollingBeta {
	if window <= 0 {
		window = 60
	}
	return &RollingBeta{
		window: window,
		xs:     make([]float64, window),
		ys:     make([]float64, window),
		dxs:    make([]float64, window),
		dys:    make([]float64, window),
		uxs:    make([]float64, window),
		uys:    make([]float64, window),
	}
}

// Update pushes a new observation pair and recomputes the OLS parameters.
func (rb *RollingBeta) Update(x, y float64) (beta, alpha, r2 float64) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.xs[rb.pos] = x
	rb.ys[rb.pos] = y
	rb.pos = (rb.pos + 1) % rb.window
	if rb.count < rb.window {
		rb.count++
	}

	n := rb.count
	if n < 5 {
		rb.currentBeta = 1.0
		rb.alpha = 0.0
		rb.r2 = 0.0
		return rb.currentBeta, rb.alpha, rb.r2
	}

	// One-pass computation of means and sums
	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for i := range n {
		xi := rb.xs[i]
		yi := rb.ys[i]
		sumX += xi
		sumY += yi
		sumXY += xi * yi
		sumX2 += xi * xi
		sumY2 += yi * yi
	}

	// Core OLS algebra
	// β = (n·Σxy - Σx·Σy) / (n·Σx² - (Σx)²)
	num := float64(n)*sumXY - sumX*sumY
	den := float64(n)*sumX2 - sumX*sumX

	if den <= 0 {
		rb.currentBeta = 1.0
		rb.alpha = 0.0
		rb.r2 = 0.0
		return rb.currentBeta, rb.alpha, rb.r2
	}

	rb.currentBeta = num / den

	// α = ȳ - β·x̄
	meanX := sumX / float64(n)
	meanY := sumY / float64(n)
	rb.alpha = meanY - rb.currentBeta*meanX

	// R² from correlation squared
	denY := float64(n)*sumY2 - sumY*sumY
	if denY > 0 {
		r := num / math.Sqrt(den*denY)
		rb.r2 = r * r
	} else {
		rb.r2 = 0.0
	}

	// Numerical stability: guard against NaN/Inf
	if math.IsNaN(rb.currentBeta) || math.IsInf(rb.currentBeta, 0) {
		rb.currentBeta = 1.0
	}
	if math.IsNaN(rb.alpha) || math.IsInf(rb.alpha, 0) {
		rb.alpha = 0.0
	}
	if math.IsNaN(rb.r2) || math.IsInf(rb.r2, 0) {
		rb.r2 = 0.0
	}

	rb.updateConditional(x, y)

	return rb.currentBeta, rb.alpha, rb.r2
}

func (rb *RollingBeta) updateConditional(x, y float64) {
	if x < 0 {
		rb.dxs[rb.dpos] = x
		rb.dys[rb.dpos] = y
		rb.dpos = (rb.dpos + 1) % rb.window
		if rb.dcount < rb.window {
			rb.dcount++
		}
		rb.downsideBeta, rb.downsideAlpha, rb.downsideR2 = rb.computeBeta(rb.dxs, rb.dys, rb.dcount)
	} else {
		rb.uxs[rb.upos] = x
		rb.uys[rb.upos] = y
		rb.upos = (rb.upos + 1) % rb.window
		if rb.ucount < rb.window {
			rb.ucount++
		}
		rb.upsideBeta, rb.upsideAlpha, rb.upsideR2 = rb.computeBeta(rb.uxs, rb.uys, rb.ucount)
	}
}

func (rb *RollingBeta) computeBeta(xs, ys []float64, n int) (beta, alpha, r2 float64) {
	if n < 5 {
		return 1.0, 0.0, 0.0
	}

	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for i := range n {
		xi := xs[i]
		yi := ys[i]
		sumX += xi
		sumY += yi
		sumXY += xi * yi
		sumX2 += xi * xi
		sumY2 += yi * yi
	}

	num := float64(n)*sumXY - sumX*sumY
	den := float64(n)*sumX2 - sumX*sumX

	if den <= 0 {
		return 1.0, 0.0, 0.0
	}

	b := num / den
	meanX := sumX / float64(n)
	meanY := sumY / float64(n)
	a := meanY - b*meanX

	denY := float64(n)*sumY2 - sumY*sumY
	var r2v float64
	if denY > 0 {
		r := num / math.Sqrt(den*denY)
		r2v = r * r
	}

	if math.IsNaN(b) || math.IsInf(b, 0) {
		b = 1.0
	}
	if math.IsNaN(a) || math.IsInf(a, 0) {
		a = 0.0
	}
	if math.IsNaN(r2v) || math.IsInf(r2v, 0) {
		r2v = 0.0
	}

	return b, a, r2v
}

// DownsideBeta returns the OLS beta computed only from observations where
// market return (x) was negative. Represents the stock's sensitivity during
// down-market moves — critical for asymmetric risk assessment.
func (rb *RollingBeta) DownsideBeta() float64 {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.downsideBeta
}

// UpsideBeta returns the OLS beta computed only from observations where
// market return (x) was non-negative. Represents the stock's participation
// during up-market moves.
func (rb *RollingBeta) UpsideBeta() float64 {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.upsideBeta
}

// DualBeta returns both downside and upside conditional betas.
// From the 2024-2025 research: downside beta (1.0-1.5x) > upside beta (0.8-1.0x)
// — the asymmetry is the core transmission risk signal.
func (rb *RollingBeta) DualBeta() (downside, upside float64) {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.downsideBeta, rb.upsideBeta
}

// GetCurrent returns the latest rolling OLS estimates.
func (rb *RollingBeta) GetCurrent() (beta, alpha, r2 float64) {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.currentBeta, rb.alpha, rb.r2
}

// Observations returns the number of paired observations collected so far.
func (rb *RollingBeta) Observations() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()
	return rb.count
}
