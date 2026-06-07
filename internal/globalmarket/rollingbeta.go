package globalmarket

import (
	"math"
	"sync"
)

// RollingBeta computes a rolling OLS beta (slope) between two return series
// using a ring buffer. Default window is 60 observations.
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
	for i := 0; i < n; i++ {
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

	return rb.currentBeta, rb.alpha, rb.r2
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
