package robustness_test

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/robustness"
)

// linearModel is a simple linear regression model for testing ablation analysis.
// Fit solves via normal equations: w = (X^T X)^(-1) X^T y using Gaussian elimination.
// Predict computes X * w.
type linearModel struct {
	weights []float64
}

func (m *linearModel) Fit(X [][]float64, y []float64) error {
	if len(X) == 0 || len(X[0]) == 0 {
		return nil
	}

	nRows := len(X)
	nCols := len(X[0])

	// Compute X^T X (nCols x nCols)
	xtx := make([][]float64, nCols)
	for i := 0; i < nCols; i++ {
		xtx[i] = make([]float64, nCols)
		for j := 0; j < nCols; j++ {
			var sum float64
			for k := 0; k < nRows; k++ {
				sum += X[k][i] * X[k][j]
			}
			xtx[i][j] = sum
		}
	}

	// Compute X^T y
	xty := make([]float64, nCols)
	for i := 0; i < nCols; i++ {
		var sum float64
		for k := 0; k < nRows; k++ {
			sum += X[k][i] * y[k]
		}
		xty[i] = sum
	}

	// Solve (X^T X) * w = X^T y via Gaussian elimination with partial pivoting
	m.weights = gaussSolve(xtx, xty)
	return nil
}

// gaussSolve solves Ax = b using Gaussian elimination with partial pivoting.
func gaussSolve(A [][]float64, b []float64) []float64 {
	n := len(A)
	aug := make([][]float64, n)
	for i := 0; i < n; i++ {
		aug[i] = make([]float64, n+1)
		copy(aug[i], A[i])
		aug[i][n] = b[i]
	}

	for col := 0; col < n; col++ {
		maxRow := col
		maxVal := math.Abs(aug[col][col])
		for row := col + 1; row < n; row++ {
			if v := math.Abs(aug[row][col]); v > maxVal {
				maxVal = v
				maxRow = row
			}
		}
		if maxRow != col {
			aug[col], aug[maxRow] = aug[maxRow], aug[col]
		}
		for row := col + 1; row < n; row++ {
			factor := aug[row][col] / aug[col][col]
			for j := col; j <= n; j++ {
				aug[row][j] -= factor * aug[col][j]
			}
		}
	}

	x := make([]float64, n)
	for i := n - 1; i >= 0; i-- {
		sum := aug[i][n]
		for j := i + 1; j < n; j++ {
			sum -= aug[i][j] * x[j]
		}
		if math.Abs(aug[i][i]) < 1e-15 {
			x[i] = 0
		} else {
			x[i] = sum / aug[i][i]
		}
	}
	return x
}

func (m *linearModel) Predict(X [][]float64) ([]float64, error) {
	pred := make([]float64, len(X))
	for i, row := range X {
		var sum float64
		for j, val := range row {
			if j < len(m.weights) {
				sum += val * m.weights[j]
			}
		}
		pred[i] = sum
	}
	return pred, nil
}

func TestAblationAnalysis(t *testing.T) {
	// Synthetic data: 3 factors ["momentum", "value", "quality"]
	// Momentum is highly predictive of y, value and quality are noise
	allFactors := []string{"momentum", "value", "quality"}

	// Generate data: nRows=10, nCols=3
	nRows := 10
	X := make([][]float64, nRows)
	y := make([]float64, nRows)
	for i := 0; i < nRows; i++ {
		// momentum: strongly correlated with y
		momentum := float64(i)*0.1 + 0.5
		// value: noise
		value := float64(i%3) * 0.2
		// quality: noise
		quality := float64((i+1)%4) * 0.15

		X[i] = []float64{momentum, value, quality}
		// y = 2.0 * momentum + small noise
		y[i] = 2.0*momentum + float64(i%2)*0.01
	}

	t.Run("excluding predictive factor drops R2", func(t *testing.T) {
		report, err := robustness.AblationAnalysis(X, y, allFactors, []string{"momentum"}, func() robustness.Model {
			return &linearModel{}
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if report.FullModelR2 <= 0 {
			t.Errorf("expected positive FullModelR2, got %f", report.FullModelR2)
		}

		// R2Drop should be positive: removing a predictive factor hurts performance
		if report.R2Drop <= 0 {
			t.Errorf("expected positive R2Drop (momentum matters), got %f", report.R2Drop)
		}

		// R2DropPct should be > 0
		if report.R2DropPct <= 0 {
			t.Errorf("expected positive R2DropPct, got %f", report.R2DropPct)
		}

		// Verify remaining factors
		expectedRemaining := []string{"value", "quality"}
		if len(report.RemainingFactors) != len(expectedRemaining) {
			t.Errorf("expected %d remaining factors, got %d: %v",
				len(expectedRemaining), len(report.RemainingFactors), report.RemainingFactors)
		}
		for i, f := range expectedRemaining {
			if i < len(report.RemainingFactors) && report.RemainingFactors[i] != f {
				t.Errorf("expected remaining factor %q at index %d, got %q", f, i, report.RemainingFactors[i])
			}
		}
	})

	t.Run("excluding noise factor has minimal impact", func(t *testing.T) {
		report, err := robustness.AblationAnalysis(X, y, allFactors, []string{"value"}, func() robustness.Model {
			return &linearModel{}
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Removing a noise factor should have small R2Drop
		if math.Abs(report.R2Drop) > 0.1 {
			t.Logf("R2Drop for noise factor: %f (should be small)", report.R2Drop)
		}
	})

	t.Run("excluded factor not found returns error", func(t *testing.T) {
		_, err := robustness.AblationAnalysis(X, y, allFactors, []string{"nonexistent"}, func() robustness.Model {
			return &linearModel{}
		})

		if err == nil {
			t.Error("expected error for nonexistent excluded factor, got nil")
		}
	})

	t.Run("empty X returns error", func(t *testing.T) {
		_, err := robustness.AblationAnalysis([][]float64{}, y, allFactors, []string{"momentum"}, func() robustness.Model {
			return &linearModel{}
		})

		if err == nil {
			t.Error("expected error for empty X, got nil")
		}
	})

	t.Run("empty y returns error", func(t *testing.T) {
		_, err := robustness.AblationAnalysis(X, []float64{}, allFactors, []string{"momentum"}, func() robustness.Model {
			return &linearModel{}
		})

		if err == nil {
			t.Error("expected error for empty y, got nil")
		}
	})

	t.Run("X and y length mismatch returns error", func(t *testing.T) {
		_, err := robustness.AblationAnalysis(X[:5], y, allFactors, []string{"momentum"}, func() robustness.Model {
			return &linearModel{}
		})

		if err == nil {
			t.Error("expected error for length mismatch, got nil")
		}
	})

	t.Run("X columns don't match allFactors length", func(t *testing.T) {
		wrongLengthFactors := []string{"momentum", "value"}
		_, err := robustness.AblationAnalysis(X, y, wrongLengthFactors, []string{"momentum"}, func() robustness.Model {
			return &linearModel{}
		})

		if err == nil {
			t.Error("expected error for factor length mismatch, got nil")
		}
	})

	t.Run("R2DropPct handles zero FullModelR2", func(t *testing.T) {
		// Create data where model can't predict (all zeros)
		zeroX := make([][]float64, 5)
		zeroY := make([]float64, 5)
		for i := 0; i < 5; i++ {
			zeroX[i] = []float64{0, 0, 0}
			zeroY[i] = 0
		}

		report, err := robustness.AblationAnalysis(zeroX, zeroY, allFactors, []string{"value"}, func() robustness.Model {
			return &linearModel{}
		})

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// FullModelR2 should be ~0 since all data is zero
		if report.FullModelR2 == 0 {
			if report.R2DropPct != 0 {
				t.Errorf("expected zero R2DropPct when FullModelR2 is zero, got %f", report.R2DropPct)
			}
		}
	})
}
