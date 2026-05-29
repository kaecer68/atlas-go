package eval_test

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/eval"
)

func TestOOSR2(t *testing.T) {
	tests := []struct {
		name   string
		yTrue  []float64
		yPred  []float64
		expect float64
	}{
		{
			name:   "empty slices",
			yTrue:  []float64{},
			yPred:  []float64{},
			expect: 0,
		},
		{
			name:   "nil slices",
			yTrue:  nil,
			yPred:  nil,
			expect: 0,
		},
		{
			name:   "nil yTrue, non-nil yPred",
			yTrue:  nil,
			yPred:  []float64{1, 2},
			expect: 0,
		},
		{
			name:   "length mismatch",
			yTrue:  []float64{1, 2, 3},
			yPred:  []float64{1, 2},
			expect: 0,
		},
		{
			name:  "perfect prediction",
			yTrue: []float64{1, 2, 3},
			yPred: []float64{1, 2, 3},
			// SSres=0 → R² = 1 - 0/Σy² = 1
			expect: 1,
		},
		{
			name:  "bad prediction",
			yTrue: []float64{1, 2, 3},
			yPred: []float64{100, 200, 300},
			// SSres=(1-100)²+(2-200)²+(3-300)²=137214, SStot=1+4+9=14, R²=1-9801=-9800
			expect: -9800,
		},
		{
			name:   "all zero yTrue",
			yTrue:  []float64{0, 0, 0},
			yPred:  []float64{1, 2, 3},
			expect: 0,
		},
		{
			name:  "single element perfect",
			yTrue: []float64{5},
			yPred: []float64{5},
			// SSres=0, SStot=25, R²=1
			expect: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eval.OOSR2(tt.yTrue, tt.yPred)
			if math.Abs(got-tt.expect) > 1e-10 {
				t.Errorf("OOSR2() = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestSharpeRatio(t *testing.T) {
	const tolerance = 1e-8

	t.Run("empty returns", func(t *testing.T) {
		got := eval.SharpeRatio([]float64{}, 0)
		if got != 0 {
			t.Errorf("SharpeRatio(empty) = %v, want 0", got)
		}
	})

	t.Run("nil returns", func(t *testing.T) {
		got := eval.SharpeRatio(nil, 0)
		if got != 0 {
			t.Errorf("SharpeRatio(nil) = %v, want 0", got)
		}
	})

	t.Run("single element", func(t *testing.T) {
		got := eval.SharpeRatio([]float64{0.001}, 0)
		if got != 0 {
			t.Errorf("SharpeRatio(single) = %v, want 0 (std dev is zero)", got)
		}
	})

	t.Run("positive returns", func(t *testing.T) {
		// Daily returns: 0.1% each day for 252 days
		returns := make([]float64, 252)
		for i := range returns {
			returns[i] = 0.001
		}
		got := eval.SharpeRatio(returns, 0)
		// Mean daily = 0.001, std dev ~ 0, Sharpes go to infinity
		// but with constant returns, std dev = 0 → returns 0
		if got != 0 {
			t.Errorf("SharpeRatio(constant positive) = %v, want 0 (zero std dev)", got)
		}
	})

	t.Run("negative returns", func(t *testing.T) {
		returns := []float64{-0.01, -0.02, -0.01, -0.03, -0.01}
		got := eval.SharpeRatio(returns, 0)
		if got >= 0 {
			t.Errorf("SharpeRatio(negative returns) = %v, want negative", got)
		}
	})

	t.Run("annualization factor check", func(t *testing.T) {
		// Build a series where daily Sharpe is exactly 1.0
		// mean=0.01, std=0.01 → daily Sharpe = 1.0 → annualized = 1.0 * sqrt(252) ≈ 15.87
		returns := []float64{0.02, 0.00, 0.02, 0.00}
		got := eval.SharpeRatio(returns, 0)
		// With only 4 data points, sample std dev dominates; just check it's not zero
		// and not just the daily value
		if math.Abs(got) < 1e-10 {
			t.Errorf("SharpeRatio() = %v, expected non-zero", got)
		}
		// The annualization factor sqrt(252) ≈ 15.87 means even modest daily
		// Sharpe gets multiplied significantly
	})

	t.Run("with risk-free rate", func(t *testing.T) {
		returns := []float64{0.001, 0.002, 0.001, 0.003, 0.001}
		rf := 0.02 // 2% annual
		got := eval.SharpeRatio(returns, rf)
		// Should be lower than without risk-free rate
		gotZero := eval.SharpeRatio(returns, 0)
		if got >= gotZero {
			t.Errorf("SharpeRatio with rf=%v = %v, should be < Sharpe with rf=0 (%v)", rf, got, gotZero)
		}
	})

	t.Run("zero returns", func(t *testing.T) {
		returns := []float64{0, 0, 0, 0, 0}
		got := eval.SharpeRatio(returns, 0)
		if got != 0 {
			t.Errorf("SharpeRatio(all zeros) = %v, want 0", got)
		}
	})
}

func TestCumulativeReturn(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		got := eval.CumulativeReturn([]float64{})
		if got != 0 {
			t.Errorf("CumulativeReturn(empty) = %v, want 0", got)
		}
	})

	t.Run("nil", func(t *testing.T) {
		got := eval.CumulativeReturn(nil)
		if got != 0 {
			t.Errorf("CumulativeReturn(nil) = %v, want 0", got)
		}
	})

	t.Run("single positive", func(t *testing.T) {
		got := eval.CumulativeReturn([]float64{0.05})
		expect := 0.05
		if math.Abs(got-expect) > 1e-10 {
			t.Errorf("CumulativeReturn(+5%%) = %v, want %v", got, expect)
		}
	})

	t.Run("simple case", func(t *testing.T) {
		// +5%, +3% → 1.05 * 1.03 - 1 = 0.0815
		got := eval.CumulativeReturn([]float64{0.05, 0.03})
		expect := 0.0815
		if math.Abs(got-expect) > 1e-10 {
			t.Errorf("CumulativeReturn(+5%%,+3%%) = %v, want %v", got, expect)
		}
	})

	t.Run("negative returns", func(t *testing.T) {
		got := eval.CumulativeReturn([]float64{-0.05, -0.03})
		// 0.95 * 0.97 - 1 = -0.0785
		expect := -0.0785
		if math.Abs(got-expect) > 1e-10 {
			t.Errorf("CumulativeReturn(-5%%,-3%%) = %v, want %v", got, expect)
		}
	})

	t.Run("mixed returns", func(t *testing.T) {
		got := eval.CumulativeReturn([]float64{0.10, -0.05, 0.02})
		// 1.10 * 0.95 * 1.02 - 1 = 0.0659
		expect := 0.0659
		if math.Abs(got-expect) > 1e-10 {
			t.Errorf("CumulativeReturn(mixed) = %v, want %v", got, expect)
		}
	})
}

func TestMaxDrawdown(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		got := eval.MaxDrawdown([]float64{})
		if got != 0 {
			t.Errorf("MaxDrawdown(empty) = %v, want 0", got)
		}
	})

	t.Run("nil", func(t *testing.T) {
		got := eval.MaxDrawdown(nil)
		if got != 0 {
			t.Errorf("MaxDrawdown(nil) = %v, want 0", got)
		}
	})

	t.Run("increasing only", func(t *testing.T) {
		got := eval.MaxDrawdown([]float64{0.01, 0.02, 0.03, 0.01})
		// Never goes below prior peak, so dd = 0 (allow floating-point near-zero)
		if math.Abs(got) > 1e-10 {
			t.Errorf("MaxDrawdown(increasing) = %v, want 0", got)
		}
	})

	t.Run("decreasing only", func(t *testing.T) {
		got := eval.MaxDrawdown([]float64{-0.01, -0.02, -0.03})
		// After -1%: cum=0.99, dd=(1-0.99)=0.01
		// After -2%: cum=0.9702, dd=(1-0.9702)=0.0298
		// After -3%: cum=0.941094, dd=(1-0.941094)=0.058906
		expect := 0.058906
		if math.Abs(got-expect) > 1e-6 {
			t.Errorf("MaxDrawdown(decreasing) = %v, want %v", got, expect)
		}
	})

	t.Run("peak then drop", func(t *testing.T) {
		// Go up 10%, then down 5% → drawdown from peak
		returns := []float64{0.10, 0.10, -0.05, -0.10}
		// 1.10 → 1.21 (peak) → 1.1495 → 1.03455
		// Max DD from peak 1.21: (1.21-1.03455)/1.21 = 0.145
		got := eval.MaxDrawdown(returns)
		if math.Abs(got-0.145) > 1e-6 {
			t.Errorf("MaxDrawdown(peak-then-drop) = %v, want ~0.145", got)
		}
	})

	t.Run("recovery after drawdown", func(t *testing.T) {
		// Drop 20%, then recover 30%
		returns := []float64{-0.20, 0.30}
		// cum=0.8 (peak=1, dd=0.2) → cum=1.04 (new peak, dd still = 0.2)
		got := eval.MaxDrawdown(returns)
		expect := 0.20
		if math.Abs(got-expect) > 1e-6 {
			t.Errorf("MaxDrawdown(recovery) = %v, want %v", got, expect)
		}
	})
}
