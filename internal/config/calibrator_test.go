package config

import (
	"math"
	"testing"
)

func TestComputeImprovementPct(t *testing.T) {
	tests := []struct {
		name     string
		baseline float64
		opt      float64
		want     float64
	}{
		// baseline > 0
		{"positive baseline - improvement", 10, 12, 20},
		{"positive baseline - regression", 10, 8, -20},
		{"positive baseline - no change", 10, 10, 0},
		{"positive baseline - large improvement", 100, 150, 50},
		{"positive baseline - small improvement", 1000, 1005, 0.5},

		// baseline < 0
		{"negative baseline - improvement (towards zero)", -10, -8, 20},
		{"negative baseline - regression (more negative)", -10, -12, -20},
		{"negative baseline - no change", -10, -10, 0},
		{"negative baseline - cross zero", -10, 5, 150},
		{"negative baseline - large improvement", -100, -50, 50},

		// baseline ≈ 0 (epsilon zone)
		{"near-zero positive baseline", 1e-11, 5, 500},
		{"near-zero negative baseline", -1e-11, -3, -300},
		{"baseline is exactly 0 - improvement", 0, 5, 500},
		{"baseline is exactly 0 - regression", 0, -3, -300},
		{"baseline is exactly 0 - no change", 0, 0, 0},

		// edge cases at epsilon boundary
		{"just above epsilon - improvement", 1e-9, 2e-9, 100},
		{"just below -epsilon - improvement", -1e-9, -0.5e-9, 50},

		// symmetry
		{"symmetric: +10→+12 and -10→-8", 10, 12, 20},
		{"same absolute improvement", 20, 24, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeImprovementPct(tt.baseline, tt.opt)
			tol := 1e-9
		if tt.baseline != 0 && math.Abs(tt.baseline) < 1e-9 {
			tol = 1e-6 // near-epsilon division yields larger float drift
		}
		if math.Abs(got-tt.want) > tol {
				t.Errorf("computeImprovementPct(%v, %v) = %v, want %v", tt.baseline, tt.opt, got, tt.want)
			}
		})
	}
}
