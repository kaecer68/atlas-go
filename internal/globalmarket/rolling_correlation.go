package globalmarket

import (
	"math"
	"sync"
)

// RollingCorrelation computes a rolling Pearson correlation between two return
// series using a ring buffer. Default window is 20 observations (~1 trading month).
type RollingCorrelation struct {
	mu             sync.RWMutex
	window         int
	xs             []float64
	ys             []float64
	pos            int
	count          int
	currentRho     float64
	isFallback     bool   // true when currentRho is a sentinel, not a real Pearson
	fallbackReason string // "insufficient_samples" | "zero_variance" | "non_finite" | ""
}

// NewRollingCorrelation creates a rolling correlation tracker with the given
// window size. Default 20 if window <= 0.
func NewRollingCorrelation(window int) *RollingCorrelation {
	if window <= 0 {
		window = 20
	}
	return &RollingCorrelation{
		window: window,
		xs:     make([]float64, window),
		ys:     make([]float64, window),
	}
}

// SeedWith pre-fills the correlation buffer with historical paired returns.
// Each pair (xs[i], ys[i]) is pushed into the ring buffer as if Update had
// been called sequentially, oldest first. The buffer wraps if len(xs) > window.
// If the arrays are unequal length, only min(len(xs), len(ys)) pairs are used.
// After seeding, the caller can call Update normally for live data.
func (rc *RollingCorrelation) SeedWith(xs, ys []float64) {
	if len(xs) == 0 || len(ys) == 0 {
		return
	}
	n := min(len(xs), len(ys))
	for i := range n {
		rc.Update(xs[i], ys[i])
	}
}

// Update pushes a new observation pair and recomputes the correlation.
// The isFallback and fallbackReason fields are set so callers can
// distinguish real Pearson rho from sentinel values without reverse-
// engineering the rho value (see #1264).
func (rc *RollingCorrelation) Update(x, y float64) float64 {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.xs[rc.pos] = x
	rc.ys[rc.pos] = y
	rc.pos = (rc.pos + 1) % rc.window
	if rc.count < rc.window {
		rc.count++
	}

	n := rc.count
	if n < 3 {
		rc.currentRho = 0.5
		rc.isFallback = true
		rc.fallbackReason = "insufficient_samples"
		return rc.currentRho
	}

	var sumX, sumY, sumXY, sumX2, sumY2 float64
	for i := range n {
		xi := rc.xs[i]
		yi := rc.ys[i]
		sumX += xi
		sumY += yi
		sumXY += xi * yi
		sumX2 += xi * xi
		sumY2 += yi * yi
	}

	num := float64(n)*sumXY - sumX*sumY
	denX := float64(n)*sumX2 - sumX*sumX
	denY := float64(n)*sumY2 - sumY*sumY

	if denX <= 0 || denY <= 0 {
		rc.currentRho = 0.5
		rc.isFallback = true
		rc.fallbackReason = "zero_variance"
		return rc.currentRho
	}

	rc.currentRho = num / math.Sqrt(denX*denY)
	if math.IsNaN(rc.currentRho) || math.IsInf(rc.currentRho, 0) {
		rc.currentRho = 0.5
		rc.isFallback = true
		rc.fallbackReason = "non_finite"
	} else {
		rc.isFallback = false
		rc.fallbackReason = ""
	}
	return rc.currentRho
}

// IsFallback reports whether the current correlation value is a sentinel
// (insufficient data, zero variance, or non-finite result) rather than
// a genuine Pearson rho from real data.
func (rc *RollingCorrelation) IsFallback() bool {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.isFallback
}

// FallbackReason returns the reason the current value is a sentinel.
// Empty string when IsFallback() is false.
func (rc *RollingCorrelation) FallbackReason() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.fallbackReason
}

// GetCurrent returns the latest rolling correlation value.
func (rc *RollingCorrelation) GetCurrent() float64 {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.currentRho
}

// Observations returns the number of paired observations collected so far.
func (rc *RollingCorrelation) Observations() int {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.count
}
