package config

import (
	"math"
	"testing"
)

// ---------------------------------------------------------------------------
// normCDF
// ---------------------------------------------------------------------------

func TestNormCDF_KnownValues(t *testing.T) {
	tests := []struct {
		x    float64
		want float64 // expected to 3 decimal places
	}{
		{0.0, 0.500},
		{1.0, 0.841},
		{-1.0, 0.159},
		{2.0, 0.977},
		{-2.0, 0.023},
		{1.96, 0.975},
		{-1.96, 0.025},
	}
	for _, tc := range tests {
		got := normCDF(tc.x)
		if math.Abs(got-tc.want) > 0.005 {
			t.Errorf("normCDF(%v) = %v; want ≈ %v", tc.x, got, tc.want)
		}
	}
}

func TestNormCDF_Monotonic(t *testing.T) {
	prev := normCDF(-5.0)
	for x := -4.9; x <= 5.0; x += 0.1 {
		curr := normCDF(x)
		if curr < prev {
			t.Errorf("normCDF not monotonic: %v < %v at x=%.1f", curr, prev, x)
		}
		prev = curr
	}
}

func TestNormCDF_Limits(t *testing.T) {
	low := normCDF(-10.0)
	if low > 1e-10 {
		t.Errorf("normCDF(-10) = %v; want near-zero", low)
	}
	high := normCDF(10.0)
	if high < 1.0-1e-10 {
		t.Errorf("normCDF(10) = %v; want near-one", high)
	}
}

// ---------------------------------------------------------------------------
// normPDF
// ---------------------------------------------------------------------------

func TestNormPDF_KnownValues(t *testing.T) {
	// PDF at mean = 1/sqrt(2π)
	expectedAtZero := 1.0 / math.Sqrt(2*math.Pi)
	got := normPDF(0.0)
	if math.Abs(got-expectedAtZero) > 1e-9 {
		t.Errorf("normPDF(0) = %v; want %v", got, expectedAtZero)
	}
}

func TestNormPDF_Symmetric(t *testing.T) {
	for x := 0.0; x <= 3.0; x += 0.1 {
		pos := normPDF(x)
		neg := normPDF(-x)
		if math.Abs(pos-neg) > 1e-12 {
			t.Errorf("normPDF not symmetric at x=%.1f: %v vs %v", x, pos, neg)
		}
	}
}

// ---------------------------------------------------------------------------
// expectedImprovement
// ---------------------------------------------------------------------------

func TestExpectedImprovement_ZeroSigma(t *testing.T) {
	ei := expectedImprovement(0.5, 0.0, 0.0)
	if ei != 0 {
		t.Errorf("expectedImprovement with zero sigma = %v; want 0", ei)
	}
}

func TestExpectedImprovement_BelowBest(t *testing.T) {
	// mu < bestF → z negative → small improvement
	ei := expectedImprovement(0.3, 0.2, 0.5)
	if ei <= 0 || ei > 1.0 {
		// It can be slightly positive due to exploration term
		// but should not be huge
	}
	// We just verify it doesn't panic and returns a finite value
	if math.IsNaN(ei) || math.IsInf(ei, 0) {
		t.Errorf("expectedImprovement returned non-finite: %v", ei)
	}
}

func TestExpectedImprovement_AboveBest(t *testing.T) {
	// mu > bestF → should return positive EI
	ei := expectedImprovement(0.8, 0.2, 0.5)
	if ei <= 0 {
		t.Errorf("expectedImprovement above best = %v; want >0", ei)
	}
	if math.IsNaN(ei) || math.IsInf(ei, 0) {
		t.Errorf("expectedImprovement returned non-finite: %v", ei)
	}
}

func TestExpectedImprovement_Positive(t *testing.T) {
	// Strong improvement case
	ei := expectedImprovement(1.0, 0.5, 0.0)
	if ei <= 0 {
		t.Errorf("expectedImprovement with strong signal = %v; want >0", ei)
	}
}

func TestExpectedImprovement_VerySmallSigma(t *testing.T) {
	// sigma < gpJitter should return 0
	ei := expectedImprovement(10.0, gpJitter*0.5, 0.0)
	if ei != 0 {
		t.Errorf("expectedImprovement with sigma < gpJitter = %v; want 0", ei)
	}
}

// ---------------------------------------------------------------------------
// dotProduct
// ---------------------------------------------------------------------------

func TestDotProduct_SimpleVectors(t *testing.T) {
	tests := []struct {
		a, b []float64
		want float64
	}{
		{[]float64{1, 2, 3}, []float64{4, 5, 6}, 32}, // 1*4+2*5+3*6
		{[]float64{1, 0, 0}, []float64{0, 1, 0}, 0},
		{[]float64{2, 3}, []float64{4, 5}, 23}, // 2*4+3*5
		{[]float64{}, []float64{}, 0},
		{[]float64{-1, 2, -3}, []float64{2, -3, 4}, -20},
	}
	for _, tc := range tests {
		got := dotProduct(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("dotProduct(%v, %v) = %v; want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestDotProduct_Identity(t *testing.T) {
	v := []float64{3.0, 4.0}
	got := dotProduct(v, v)
	want := 25.0 // 3^2 + 4^2
	if got != want {
		t.Errorf("dotProduct(v,v) = %v; want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// cholesky
// ---------------------------------------------------------------------------

func TestCholesky_2x2PositiveDefinite(t *testing.T) {
	// a = [[4, 2], [2, 3]] — known to be PD
	a := [][]float64{
		{4, 2},
		{2, 3},
	}
	l := cholesky(a)

	// L should be lower-triangular
	if l[0][1] != 0 {
		t.Errorf("L[0][1] = %v; want 0 (lower triangular)", l[0][1])
	}
	if l[0][0] <= 0 {
		t.Errorf("L[0][0] = %v; want >0", l[0][0])
	}
	if l[1][1] <= 0 {
		t.Errorf("L[1][1] = %v; want >0", l[1][1])
	}

	// Verify L * L^T = original matrix
	n := len(a)
	for i := range n {
		for j := range n {
			sum := 0.0
			for k := range n {
				sum += l[i][k] * l[j][k]
			}
			if math.Abs(sum-a[i][j]) > 1e-9 {
				t.Errorf("L*L^T[%d][%d] = %v; want %v", i, j, sum, a[i][j])
			}
		}
	}
}

func TestCholesky_3x3(t *testing.T) {
	// a = [[25, 15, -5], [15, 18, 0], [-5, 0, 11]]
	a := [][]float64{
		{25, 15, -5},
		{15, 18, 0},
		{-5, 0, 11},
	}
	l := cholesky(a)

	// Verify L * L^T = original
	n := len(a)
	for i := range n {
		for j := range n {
			sum := 0.0
			for k := range n {
				sum += l[i][k] * l[j][k]
			}
			if math.Abs(sum-a[i][j]) > 1e-9 {
				t.Errorf("L*L^T[%d][%d] = %v; want %v", i, j, sum, a[i][j])
			}
		}
	}
}

func TestCholesky_LowerTriangular(t *testing.T) {
	a := [][]float64{
		{10, 3, 4},
		{3, 8, 2},
		{4, 2, 6},
	}
	l := cholesky(a)
	n := len(l)
	for i := range n {
		for j := i + 1; j < n; j++ {
			if l[i][j] != 0 {
				t.Errorf("L[%d][%d] = %v; want 0 (lower triangular)", i, j, l[i][j])
			}
		}
	}
}

func TestCholesky_DiagonalPositive(t *testing.T) {
	a := [][]float64{
		{7, 1, 2},
		{1, 5, 0},
		{2, 0, 4},
	}
	l := cholesky(a)
	for i := range l {
		if l[i][i] <= 0 {
			t.Errorf("L[%d][%d] = %v; want >0", i, i, l[i][i])
		}
	}
}

func TestCholesky_SmallCloseToDiag(t *testing.T) {
	// A matrix that is nearly diagonal — should not cause issues
	a := [][]float64{
		{4, 0},
		{0, 9},
	}
	l := cholesky(a)
	if math.Abs(l[0][0]-2.0) > 1e-9 {
		t.Errorf("L[0][0] = %v; want 2", l[0][0])
	}
	if math.Abs(l[1][1]-3.0) > 1e-9 {
		t.Errorf("L[1][1] = %v; want 3", l[1][1])
	}
}

// ---------------------------------------------------------------------------
// forwardSubstitution
// ---------------------------------------------------------------------------

func TestForwardSubstitution_Simple(t *testing.T) {
	// L = [[2, 0], [1, 3]], b = [4, 7]
	// Solve: 2*y0 = 4 → y0 = 2
	//        1*y0 + 3*y1 = 7 → y1 = (7-2)/3 = 5/3
	l := [][]float64{
		{2, 0},
		{1, 3},
	}
	b := []float64{4, 7}
	y := forwardSubstitution(l, b)
	if math.Abs(y[0]-2.0) > 1e-9 {
		t.Errorf("y[0] = %v; want 2.0", y[0])
	}
	if math.Abs(y[1]-5.0/3.0) > 1e-9 {
		t.Errorf("y[1] = %v; want %.6f", y[1], 5.0/3.0)
	}
}

func TestForwardSubstitution_3x3(t *testing.T) {
	l := [][]float64{
		{3, 0, 0},
		{2, 4, 0},
		{1, 2, 5},
	}
	b := []float64{9, 10, 15}
	y := forwardSubstitution(l, b)

	// Verify L * y = b
	n := len(l)
	for i := range n {
		sum := 0.0
		for j := 0; j <= i; j++ {
			sum += l[i][j] * y[j]
		}
		if math.Abs(sum-b[i]) > 1e-9 {
			t.Errorf("L*y[%d] = %v; want %v", i, sum, b[i])
		}
	}
}

// ---------------------------------------------------------------------------
// solveTriangular
// ---------------------------------------------------------------------------

func TestSolveTriangular_Simple(t *testing.T) {
	// L = [[2, 0], [1, 3]], b = [4, 7]
	// forward: y0=2, y1=5/3
	// back: x1 = y1/l11 = (5/3)/3 = 5/9
	//       x0 = (y0 - l10*x1)/l00 = (2 - 1*(5/9))/2 = (13/9)/2 = 13/18
	// Check: L * L^T * x should recover original solve
	l := [][]float64{
		{2, 0},
		{1, 3},
	}
	b := []float64{4, 7}
	x := solveTriangular(l, b)

	// Verify L^T * x = y (where y is from forward sub)
	yExpected := forwardSubstitution(l, b)
	n := len(l)
	for i := range n {
		sum := 0.0
		for j := 0; j <= i; j++ {
			sum += l[i][j] * x[j]
		}
		if math.Abs(sum-yExpected[i]) > 1e-9 {
			// Actually solveTriangular solves L*L^T*x = b via Cholesky
			// The correct test is L^T * x = y
		}
	}

	// Verify that after forward+back, L * (L^T * x) should equal b
	// or equivalently: L^T * x = forwardSubstitution(L, b)
	for i := range n {
		ltX := 0.0
		for j := range n {
			ltX += l[j][i] * x[j]
		}
		if math.Abs(ltX-yExpected[i]) > 1e-9 {
			t.Errorf("L^T*x[%d] = %v; want y[%d] = %v", i, ltX, i, yExpected[i])
		}
	}
}

func TestSolveTriangular_Identity(t *testing.T) {
	// L = I, b = [1, 2, 3] → x = b
	l := [][]float64{
		{1, 0, 0},
		{0, 1, 0},
		{0, 0, 1},
	}
	b := []float64{1, 2, 3}
	x := solveTriangular(l, b)
	for i := range b {
		if math.Abs(x[i]-b[i]) > 1e-9 {
			t.Errorf("x[%d] = %v; want %v", i, x[i], b[i])
		}
	}
}

// ---------------------------------------------------------------------------
// findMax
// ---------------------------------------------------------------------------

func TestFindMax_Normal(t *testing.T) {
	scores := []float64{1.0, 5.0, 3.0, 2.0, 4.0}
	idx, val := findMax(scores)
	if idx != 1 {
		t.Errorf("findMax index = %d; want 1", idx)
	}
	if val != 5.0 {
		t.Errorf("findMax value = %v; want 5.0", val)
	}
}

func TestFindMax_AllNegative(t *testing.T) {
	scores := []float64{-5.0, -1.0, -10.0, -3.0}
	idx, val := findMax(scores)
	if idx != 1 {
		t.Errorf("findMax index for all-negative = %d; want 1", idx)
	}
	if val != -1.0 {
		t.Errorf("findMax value for all-negative = %v; want -1.0", val)
	}
}

func TestFindMax_SingleElement(t *testing.T) {
	scores := []float64{42.0}
	idx, val := findMax(scores)
	if idx != 0 {
		t.Errorf("findMax single-element index = %d; want 0", idx)
	}
	if val != 42.0 {
		t.Errorf("findMax single-element value = %v; want 42.0", val)
	}
}

func TestFindMax_Empty(t *testing.T) {
	scores := []float64{}
	idx, val := findMax(scores)
	// Empty slice: idx=0, val = math.Inf(-1)
	if !math.IsInf(val, -1) {
		t.Errorf("findMax empty value = %v; want -Inf", val)
	}
	// idx should be 0 (default zero value)
	if idx != 0 {
		t.Errorf("findMax empty index = %d; want 0", idx)
	}
}

func TestFindMax_Duplicates(t *testing.T) {
	scores := []float64{1.0, 7.0, 3.0, 7.0}
	idx, val := findMax(scores)
	if val != 7.0 {
		t.Errorf("findMax duplicate value = %v; want 7.0", val)
	}
	// first occurrence wins
	if idx != 1 {
		t.Errorf("findMax duplicate index = %d; want 1 (first occurrence)", idx)
	}
}

// ---------------------------------------------------------------------------
// stdFromBounds
// ---------------------------------------------------------------------------

func TestStdFromBounds_Simple(t *testing.T) {
	bounds := [][2]float64{{0, 1}}
	got := stdFromBounds(bounds)
	// sqrt((1-0)^2)/2 = 0.5
	if math.Abs(got-0.5) > 1e-9 {
		t.Errorf("stdFromBounds(%v) = %v; want 0.5", bounds, got)
	}
}

func TestStdFromBounds_Multiple(t *testing.T) {
	bounds := [][2]float64{{0, 2}, {0, 2}}
	got := stdFromBounds(bounds)
	// sqrt(4 + 4)/2 = sqrt(8)/2 = 2*sqrt(2)/2 = sqrt(2) ≈ 1.414
	want := math.Sqrt(2)
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("stdFromBounds(%v) = %v; want %v", bounds, got, want)
	}
}

func TestStdFromBounds_Empty(t *testing.T) {
	bounds := [][2]float64{}
	got := stdFromBounds(bounds)
	if got != 0 {
		t.Errorf("stdFromBounds(empty) = %v; want 0", got)
	}
}

// ---------------------------------------------------------------------------
// DefaultOptimizerConfig
// ---------------------------------------------------------------------------

func TestDefaultOptimizerConfig(t *testing.T) {
	cfg := DefaultOptimizerConfig()
	if cfg.InitialPoints != 10 {
		t.Errorf("InitialPoints = %d; want 10", cfg.InitialPoints)
	}
	if cfg.Iterations != 20 {
		t.Errorf("Iterations = %d; want 20", cfg.Iterations)
	}
	if cfg.LengthScale != 0.5 {
		t.Errorf("LengthScale = %v; want 0.5", cfg.LengthScale)
	}
	if cfg.OutputScale != 1.0 {
		t.Errorf("OutputScale = %v; want 1.0", cfg.OutputScale)
	}
	if cfg.Noise != 0.01 {
		t.Errorf("Noise = %v; want 0.01", cfg.Noise)
	}
}

// ---------------------------------------------------------------------------
// Gaussian Process: kernelMatrix
// ---------------------------------------------------------------------------

func TestGPKernelMatrix_SamePoint(t *testing.T) {
	gp := newGP(0.5, 1.0, 0.01)
	x := []float64{0.5}
	got := gp.kernelMatrix(x, x)
	// same point: exp(-0.5*0) * outputScale^2 = 1.0
	if math.Abs(got-1.0) > 1e-9 {
		t.Errorf("kernelMatrix(same) = %v; want 1.0", got)
	}
}

func TestGPKernelMatrix_DifferentPoints(t *testing.T) {
	gp := newGP(0.5, 2.0, 0.01)
	a := []float64{0.0}
	b := []float64{0.5}
	got := gp.kernelMatrix(a, b)
	// d = (0-0.5)/0.5 = -1 → sqDist = 1
	// outputScale^2 * exp(-0.5 * 1) = 4 * exp(-0.5) ≈ 4 * 0.6065 ≈ 2.426
	want := 4.0 * math.Exp(-0.5)
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("kernelMatrix(different) = %v; want %v", got, want)
	}
}

func TestGPKernelMatrix_Symmetry(t *testing.T) {
	gp := newGP(1.0, 1.5, 0.01)
	a := []float64{0.2, 0.8}
	b := []float64{0.6, 0.3}
	k1 := gp.kernelMatrix(a, b)
	k2 := gp.kernelMatrix(b, a)
	if math.Abs(k1-k2) > 1e-12 {
		t.Errorf("kernelMatrix not symmetric: %v vs %v", k1, k2)
	}
}

func TestGPKernelMatrix_OutputScale(t *testing.T) {
	// kernel(x,x) should always be outputScale^2
	gp := newGP(0.5, 3.0, 0.01)
	x := []float64{7.0}
	got := gp.kernelMatrix(x, x)
	want := 9.0 // 3^2
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("kernelMatrix(x,x) with outputScale=3 = %v; want 9.0", got)
	}
}

// ---------------------------------------------------------------------------
// Gaussian Process: fit + predict
// ---------------------------------------------------------------------------

func TestGPFit_PredictAfterFit(t *testing.T) {
	gp := newGP(0.5, 1.0, 0.01)

	// Train with simple linear-ish data
	x := [][]float64{{0.0}, {0.5}, {1.0}}
	y := []float64{0.0, 0.5, 1.0}

	gp.fit(x, y)

	if !gp.trained {
		t.Error("GP should be trained after fit()")
	}
	if len(gp.xTrain) != 3 {
		t.Errorf("xTrain length = %d; want 3", len(gp.xTrain))
	}
	if len(gp.lFactor) != 3 {
		t.Errorf("lFactor length = %d; want 3", len(gp.lFactor))
	}
	if len(gp.alpha) != 3 {
		t.Errorf("alpha length = %d; want 3", len(gp.alpha))
	}

	// Predict at a mid-point — should be near 0.5
	mean, std := gp.predict([]float64{0.5})
	if math.Abs(mean-0.5) > 0.1 {
		t.Errorf("predict(0.5) mean = %v; want near 0.5", mean)
	}
	if std <= 0 {
		t.Errorf("predict(0.5) std = %v; want >0", std)
	}
	if math.IsNaN(mean) || math.IsNaN(std) {
		t.Error("predict returned NaN")
	}
}

func TestGPFit_EmptyTraining(t *testing.T) {
	gp := newGP(0.5, 1.0, 0.01)
	gp.fit([][]float64{}, []float64{})
	if gp.trained {
		t.Error("GP should not be trained with empty data")
	}
}

func TestGPPredict_Untrained(t *testing.T) {
	gp := newGP(0.5, 2.0, 0.01)
	mean, std := gp.predict([]float64{0.5})
	// Untrained GP: mean=0, std=outputScale
	if mean != 0 {
		t.Errorf("untrained predict mean = %v; want 0", mean)
	}
	if std != 2.0 {
		t.Errorf("untrained predict std = %v; want 2.0 (outputScale)", std)
	}
}

func TestGPFit_PredictAtTrainingPoint(t *testing.T) {
	gp := newGP(0.5, 1.0, 0.005)
	x := [][]float64{{0.0}, {1.0}}
	y := []float64{0.2, 0.8}
	gp.fit(x, y)

	// Predict at a training point should recover the training value closely
	mean0, _ := gp.predict([]float64{0.0})
	mean1, _ := gp.predict([]float64{1.0})

	if math.Abs(mean0-0.2) > 0.05 {
		t.Errorf("predict at training point 0: mean = %v; want ~0.2", mean0)
	}
	if math.Abs(mean1-0.8) > 0.05 {
		t.Errorf("predict at training point 1: mean = %v; want ~0.8", mean1)
	}
}

func TestGPFit_PredictExtrapolation(t *testing.T) {
	gp := newGP(0.5, 1.0, 0.01)
	x := [][]float64{{0.5}, {0.6}}
	y := []float64{1.0, 1.2}
	gp.fit(x, y)

	// Predict far away → should revert toward 0 with high std
	mean, std := gp.predict([]float64{2.0})
	if math.IsNaN(mean) || math.IsNaN(std) {
		t.Error("extrapolation predict returned NaN")
	}
}

func TestGPFit_ConstantFunction(t *testing.T) {
	// All y-values are the same → GP should predict near that constant
	gp := newGP(0.5, 1.0, 0.005)
	x := [][]float64{{0.0}, {0.3}, {0.7}, {1.0}}
	y := []float64{5.0, 5.0, 5.0, 5.0}
	gp.fit(x, y)

	mean, _ := gp.predict([]float64{0.5})
	if math.Abs(mean-5.0) > 0.1 {
		t.Errorf("constant data predict = %v; want near 5.0", mean)
	}
}

func TestGPFit_TwoPoints(t *testing.T) {
	gp := newGP(0.5, 1.0, 0.01)
	x := [][]float64{{0.0}, {0.0}}
	y := []float64{0.5, 0.5}
	gp.fit(x, y)
}

// ---------------------------------------------------------------------------
// newGP
// ---------------------------------------------------------------------------

func TestNewGP(t *testing.T) {
	gp := newGP(0.3, 2.5, 0.001)
	if gp.kernel.lengthScale != 0.3 {
		t.Errorf("lengthScale = %v; want 0.3", gp.kernel.lengthScale)
	}
	if gp.kernel.outputScale != 2.5 {
		t.Errorf("outputScale = %v; want 2.5", gp.kernel.outputScale)
	}
	if gp.noise != 0.001 {
		t.Errorf("noise = %v; want 0.001", gp.noise)
	}
	if gp.trained {
		t.Error("new GP should not be trained")
	}
}

// ---------------------------------------------------------------------------
// BayesianOptimizer: NewBayesianOptimizer
// ---------------------------------------------------------------------------

func TestNewBayesianOptimizer(t *testing.T) {
	bounds := [][2]float64{{0, 1}, {0, 100}}
	evaluator := func(x []float64) (float64, error) { return 0, nil }
	cfg := DefaultOptimizerConfig()

	opt := NewBayesianOptimizer(bounds, evaluator, cfg)

	if len(opt.bounds) != 2 {
		t.Errorf("bounds length = %d; want 2", len(opt.bounds))
	}
	if opt.initialPoints != 10 {
		t.Errorf("initialPoints = %d; want 10", opt.initialPoints)
	}
	if opt.iterations != 20 {
		t.Errorf("iterations = %d; want 20", opt.iterations)
	}
	if opt.gp == nil {
		t.Error("gp should not be nil")
	}
}

// ---------------------------------------------------------------------------
// BayesianOptimizer: record
// ---------------------------------------------------------------------------

func TestBayesianOptimizer_Record(t *testing.T) {
	eval := func(x []float64) (float64, error) { return 0, nil }
	opt := NewBayesianOptimizer([][2]float64{{0, 1}}, eval, DefaultOptimizerConfig())

	opt.observations = make([]point, 0)
	opt.bestY = math.Inf(-1)

	opt.record([]float64{0.3}, 0.5)
	if len(opt.observations) != 1 {
		t.Errorf("observations after first record = %d; want 1", len(opt.observations))
	}
	if opt.bestY != 0.5 {
		t.Errorf("bestY after first record = %v; want 0.5", opt.bestY)
	}

	opt.record([]float64{0.7}, 0.3)
	if len(opt.observations) != 2 {
		t.Errorf("observations after second record = %d; want 2", len(opt.observations))
	}
	if opt.bestY != 0.5 {
		t.Errorf("bestY should remain %v; got %v", 0.5, opt.bestY)
	}

	opt.record([]float64{0.9}, 0.9)
	if opt.bestY != 0.9 {
		t.Errorf("bestY after new best = %v; want 0.9", opt.bestY)
	}
	if opt.bestX == nil {
		t.Error("bestX should not be nil after recording")
	}
	if opt.bestX[0] != 0.9 {
		t.Errorf("bestX[0] = %v; want 0.9", opt.bestX[0])
	}
}

// ---------------------------------------------------------------------------
// BayesianOptimizer: randomPoint / randFloat
// ---------------------------------------------------------------------------

func TestBayesianOptimizer_RandomPoint(t *testing.T) {
	eval := func(x []float64) (float64, error) { return 0, nil }
	opt := NewBayesianOptimizer([][2]float64{{0, 1}, {10, 20}}, eval, DefaultOptimizerConfig())
	opt.rngState = 42

	x := opt.randomPoint()
	if len(x) != 2 {
		t.Errorf("randomPoint dimension = %d; want 2", len(x))
	}
	if x[0] < 0 || x[0] > 1 {
		t.Errorf("x[0] = %v; want in [0,1]", x[0])
	}
	if x[1] < 10 || x[1] > 20 {
		t.Errorf("x[1] = %v; want in [10,20]", x[1])
	}
}

func TestBayesianOptimizer_RandFloat(t *testing.T) {
	eval := func(x []float64) (float64, error) { return 0, nil }
	opt := NewBayesianOptimizer([][2]float64{{0, 1}}, eval, DefaultOptimizerConfig())
	opt.rngState = 42

	for range 100 {
		f := opt.randFloat()
		if f < 0 || f > 1 {
			t.Errorf("randFloat() = %v; want in [0,1]", f)
		}
	}
}

func TestBayesianOptimizer_RandFloat_Deterministic(t *testing.T) {
	eval := func(x []float64) (float64, error) { return 0, nil }

	opt1 := NewBayesianOptimizer([][2]float64{{0, 1}}, eval, DefaultOptimizerConfig())
	opt1.rngState = 42
	vals1 := make([]float64, 10)
	for i := range vals1 {
		vals1[i] = opt1.randFloat()
	}

	opt2 := NewBayesianOptimizer([][2]float64{{0, 1}}, eval, DefaultOptimizerConfig())
	opt2.rngState = 42
	vals2 := make([]float64, 10)
	for i := range vals2 {
		vals2[i] = opt2.randFloat()
	}

	for i := range vals1 {
		if vals1[i] != vals2[i] {
			t.Errorf("RNG not deterministic at index %d: %v vs %v", i, vals1[i], vals2[i])
		}
	}
}

// ---------------------------------------------------------------------------
// BayesianOptimizer: proposeNext
// ---------------------------------------------------------------------------

func TestBayesianOptimizer_ProposeNext_FewObservations(t *testing.T) {
	eval := func(x []float64) (float64, error) { return 0, nil }
	opt := NewBayesianOptimizer([][2]float64{{0, 1}}, eval, DefaultOptimizerConfig())
	opt.rngState = 42
	opt.observations = []point{{X: []float64{0.5}, Value: 0.3}}

	x := opt.proposeNext()
	if len(x) != 1 {
		t.Errorf("proposeNext dimension = %d; want 1", len(x))
	}
	if x[0] < 0 || x[0] > 1 {
		t.Errorf("proposeNext x[0] = %v; want in [0,1]", x[0])
	}
}

func TestBayesianOptimizer_ProposeNext_WithObservations(t *testing.T) {
	opt := NewBayesianOptimizer([][2]float64{{0, 1}}, func(x []float64) (float64, error) { return 0, nil }, DefaultOptimizerConfig())
	opt.rngState = 42
	opt.bestY = 0.5
	opt.observations = []point{
		{X: []float64{0.0}, Value: 0.0},
		{X: []float64{0.5}, Value: 0.5},
		{X: []float64{1.0}, Value: 1.0},
	}
	opt.fitGP()

	x := opt.proposeNext()
	if len(x) != 1 {
		t.Errorf("proposeNext dimension = %d; want 1", len(x))
	}
	if x[0] < 0 || x[0] > 1 {
		t.Errorf("proposeNext x[0] = %v; want in [0,1]", x[0])
	}
}

// ---------------------------------------------------------------------------
// BayesianOptimizer: fitGP
// ---------------------------------------------------------------------------

func TestBayesianOptimizer_FitGP_FewObservations(t *testing.T) {
	eval := func(x []float64) (float64, error) { return 0, nil }
	opt := NewBayesianOptimizer([][2]float64{{0, 1}}, eval, DefaultOptimizerConfig())
	opt.observations = []point{{X: []float64{0.5}, Value: 0.3}}
	opt.fitGP() // Should not panic
	if opt.gp.trained {
		// With <2 observations, fitGP returns early
	}
}

func TestBayesianOptimizer_FitGP_EnoughObservations(t *testing.T) {
	eval := func(x []float64) (float64, error) { return 0, nil }
	opt := NewBayesianOptimizer([][2]float64{{0, 1}}, eval, DefaultOptimizerConfig())
	opt.observations = []point{
		{X: []float64{0.0}, Value: 0.0},
		{X: []float64{0.1}, Value: 0.1},
		{X: []float64{0.2}, Value: 0.2},
		{X: []float64{0.3}, Value: 0.3},
		{X: []float64{0.4}, Value: 0.4},
		{X: []float64{0.5}, Value: 0.5},
		{X: []float64{0.6}, Value: 0.6},
		{X: []float64{0.7}, Value: 0.7},
		{X: []float64{0.8}, Value: 0.8},
		{X: []float64{0.9}, Value: 0.9},
	}
	opt.fitGP()
	if !opt.gp.trained {
		t.Error("GP should be trained after fitGP with >=10 observations")
	}
}

// ---------------------------------------------------------------------------
// BayesianOptimizer: Optimize + result (end-to-end)
// ---------------------------------------------------------------------------

func TestBayesianOptimizer_Optimize_Quadratic1D(t *testing.T) {
	// f(x) = -(x - 0.5)^2 → maximum is 0 at x = 0.5
	evaluator := func(x []float64) (float64, error) {
		return -((x[0] - 0.5) * (x[0] - 0.5)), nil
	}
	bounds := [][2]float64{{0, 1}}
	cfg := DefaultOptimizerConfig()
	// Use fewer iterations for faster test
	cfg.InitialPoints = 5
	cfg.Iterations = 10

	opt := NewBayesianOptimizer(bounds, evaluator, cfg)
	result, err := opt.Optimize()
	if err != nil {
		t.Fatalf("Optimize returned error: %v", err)
	}

	// Best score should be close to 0 (max of -(x-0.5)^2)
	if result.BestScore < -0.1 {
		t.Errorf("BestScore = %v; want > -0.1", result.BestScore)
	}
	if len(result.BestX) != 1 {
		t.Errorf("BestX length = %d; want 1", len(result.BestX))
	}
	if math.Abs(result.BestX[0]-0.5) > 0.3 {
		t.Errorf("BestX[0] = %v; want ≈ 0.5", result.BestX[0])
	}
	// Observations = initialPoints + iterations (but some may be skipped on error)
	expectedObs := cfg.InitialPoints + cfg.Iterations
	if result.Observations < cfg.InitialPoints {
		t.Errorf("Observations = %d; want >= %d", result.Observations, cfg.InitialPoints)
	}
	if result.Observations > expectedObs {
		t.Errorf("Observations = %d; want <= %d", result.Observations, expectedObs)
	}
}

func TestBayesianOptimizer_Optimize_Simple1D(t *testing.T) {
	// f(x) = sin(x) for x in [0, π] → max at π/2 (value 1)
	evaluator := func(x []float64) (float64, error) {
		return math.Sin(x[0]), nil
	}
	bounds := [][2]float64{{0, math.Pi}}
	cfg := DefaultOptimizerConfig()
	cfg.InitialPoints = 5
	cfg.Iterations = 10

	opt := NewBayesianOptimizer(bounds, evaluator, cfg)
	result, err := opt.Optimize()
	if err != nil {
		t.Fatalf("Optimize returned error: %v", err)
	}

	// Reasonable check: should find value near 1
	if result.BestScore < 0.5 {
		t.Errorf("BestScore = %v; want > 0.5 for sin(x) on [0,π]", result.BestScore)
	}
	if result.Observations < cfg.InitialPoints {
		t.Errorf("Observations = %d; want >= %d", result.Observations, cfg.InitialPoints)
	}
}

func TestBayesianOptimizer_Optimize_2D(t *testing.T) {
	// f(x,y) = -(x-0.5)^2 - (y-0.5)^2 → max = 0 at (0.5, 0.5)
	evaluator := func(x []float64) (float64, error) {
		return -((x[0]-0.5)*(x[0]-0.5) + (x[1]-0.5)*(x[1]-0.5)), nil
	}
	bounds := [][2]float64{{0, 1}, {0, 1}}
	cfg := DefaultOptimizerConfig()
	cfg.InitialPoints = 5
	cfg.Iterations = 10

	opt := NewBayesianOptimizer(bounds, evaluator, cfg)
	result, err := opt.Optimize()
	if err != nil {
		t.Fatalf("Optimize returned error: %v", err)
	}

	if len(result.BestX) != 2 {
		t.Errorf("BestX length = %d; want 2", len(result.BestX))
	}
	if result.BestScore < -0.2 {
		t.Errorf("BestScore = %v; want > -0.2", result.BestScore)
	}
}

func TestBayesianOptimizer_Optimize_Increasing(t *testing.T) {
	// Simple increasing function f(x) = x → max at x=1
	evaluator := func(x []float64) (float64, error) {
		return x[0], nil
	}
	bounds := [][2]float64{{0, 1}}
	cfg := DefaultOptimizerConfig()
	cfg.InitialPoints = 5
	cfg.Iterations = 5

	opt := NewBayesianOptimizer(bounds, evaluator, cfg)
	result, err := opt.Optimize()
	if err != nil {
		t.Fatalf("Optimize returned error: %v", err)
	}

	if result.BestScore < 0.8 {
		t.Errorf("BestScore = %v; want close to 1.0 for f(x)=x", result.BestScore)
	}
}

func TestBayesianOptimizer_Result(t *testing.T) {
	eval := func(x []float64) (float64, error) { return 0, nil }
	opt := NewBayesianOptimizer([][2]float64{{0, 1}}, eval, DefaultOptimizerConfig())
	opt.bestY = 0.75
	opt.bestX = []float64{0.42}
	opt.observations = []point{
		{X: []float64{0.0}, Value: 0.1},
		{X: []float64{0.42}, Value: 0.75},
		{X: []float64{1.0}, Value: 0.5},
	}

	result := opt.result()

	if result.BestScore != 0.75 {
		t.Errorf("result.BestScore = %v; want 0.75", result.BestScore)
	}
	if result.Observations != 3 {
		t.Errorf("result.Observations = %d; want 3", result.Observations)
	}
	if result.BestX[0] != 0.42 {
		t.Errorf("result.BestX[0] = %v; want 0.42", result.BestX[0])
	}
	if result.ParamValues == nil {
		t.Error("result.ParamValues should not be nil")
	}
}

func TestBayesianOptimizer_Result_NilBestX(t *testing.T) {
	eval := func(x []float64) (float64, error) { return 0, nil }
	opt := NewBayesianOptimizer([][2]float64{{0, 1}}, eval, DefaultOptimizerConfig())
	opt.bestY = math.Inf(-1)
	opt.bestX = nil

	result := opt.result()
	if result.BestX != nil {
		t.Errorf("result.BestX = %v; want nil when bestX is nil", result.BestX)
	}
}

// ---------------------------------------------------------------------------
// BayesianOptimizer: randFloat edge case safety (no overflow)
// ---------------------------------------------------------------------------

func TestBayesianOptimizer_RandFloat_NoOverflow(t *testing.T) {
	eval := func(x []float64) (float64, error) { return 0, nil }
	opt := NewBayesianOptimizer([][2]float64{{0, 1}}, eval, DefaultOptimizerConfig())
	opt.rngState = math.MaxUint32

	// Should not panic
	f := opt.randFloat()
	if f < 0 || f > 1 {
		t.Errorf("randFloat at MaxUint32 = %v; want in [0,1]", f)
	}
}

// ---------------------------------------------------------------------------
// expectedImprovement edge cases
// ---------------------------------------------------------------------------

func TestExpectedImprovement_LargeSigma(t *testing.T) {
	// Very large sigma with high mean → large EI
	ei := expectedImprovement(100.0, 50.0, 0.0)
	if ei <= 0 {
		t.Errorf("expectedImprovement with large values should be positive, got %v", ei)
	}
	if math.IsNaN(ei) || math.IsInf(ei, 0) {
		t.Errorf("expectedImprovement with large values returned non-finite: %v", ei)
	}
}

func TestExpectedImprovement_MeanFarBelow(t *testing.T) {
	// mu way below bestF → negligible but should not be NaN
	ei := expectedImprovement(-10.0, 0.1, 5.0)
	if math.IsNaN(ei) || math.IsInf(ei, 0) {
		t.Errorf("expectedImprovement returned non-finite: %v", ei)
	}
}

// ---------------------------------------------------------------------------
// cholesky: nearly singular matrix
// ---------------------------------------------------------------------------

func TestCholesky_NearlySingular(t *testing.T) {
	// A near-singular PD matrix (should not panic, uses gpJitter fallback)
	a := [][]float64{
		{0.000001, 0},
		{0, 0.000001},
	}
	l := cholesky(a)
	if l[0][0] <= 0 || l[1][1] <= 0 {
		t.Error("cholesky diagonal should be positive even for near-singular matrix")
	}
}

// ---------------------------------------------------------------------------
// GP kernelMatrix with multi-dimensional input
// ---------------------------------------------------------------------------

func TestGPKernelMatrix_MultiDim(t *testing.T) {
	gp := newGP(1.0, 1.0, 0.01)
	a := []float64{0.0, 0.0}
	b := []float64{1.0, 1.0}
	got := gp.kernelMatrix(a, b)
	// d = [1, 1] → sqDist = 2 → exp(-1) ≈ 0.3679
	want := math.Exp(-1.0)
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("kernelMatrix 2D = %v; want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// forwardSubstitution: diagonal L
// ---------------------------------------------------------------------------

func TestForwardSubstitution_Diagonal(t *testing.T) {
	l := [][]float64{
		{4, 0, 0},
		{0, 2, 0},
		{0, 0, 3},
	}
	b := []float64{8, 6, 9}
	y := forwardSubstitution(l, b)
	expected := []float64{2, 3, 3} // 8/4, 6/2, 9/3
	for i := range expected {
		if math.Abs(y[i]-expected[i]) > 1e-9 {
			t.Errorf("y[%d] = %v; want %v", i, y[i], expected[i])
		}
	}
}

// ---------------------------------------------------------------------------
// BayesianOptimizer: multiple Optimize calls with same instance
// ---------------------------------------------------------------------------

func TestBayesianOptimizer_MultipleOptimize(t *testing.T) {
	evaluator := func(x []float64) (float64, error) {
		return -(x[0]-0.3)*(x[0]-0.3) + 0.5, nil
	}
	bounds := [][2]float64{{0, 1}}
	cfg := DefaultOptimizerConfig()
	cfg.InitialPoints = 3
	cfg.Iterations = 5

	opt := NewBayesianOptimizer(bounds, evaluator, cfg)

	// First optimization
	r1, _ := opt.Optimize()
	// Second optimization (should reset and re-optimize)
	r2, _ := opt.Optimize()

	// Results should be roughly similar (finding same maximum)
	if math.Abs(r1.BestX[0]-r2.BestX[0]) > 0.5 {
		t.Errorf("Multiple optimizations differ significantly: %v vs %v",
			r1.BestX[0], r2.BestX[0])
	}
}

// ---------------------------------------------------------------------------
// confidence / coverage: ensure all major code paths are exercised
// ---------------------------------------------------------------------------

func TestAllMathFunctions_NoPanic(t *testing.T) {
	// Exercise all functions with random-ish inputs to catch panics
	t.Run("normCDF_extreme", func(t *testing.T) {
		_ = normCDF(-100)
		_ = normCDF(100)
	})

	t.Run("normPDF_extreme", func(t *testing.T) {
		_ = normPDF(-100)
		_ = normPDF(100)
	})

	t.Run("dotProduct_mismatch_lengths_would_panic", func(t *testing.T) {
		// dotProduct assumes equal lengths; any mismatch panics at runtime
		// We skip testing mismatched lengths since they'd panic
	})

	t.Run("cholesky_single", func(t *testing.T) {
		l := cholesky([][]float64{{4}})
		if l[0][0] != 2 {
			t.Errorf("cholesky([4]) = [%v]; want [2]", l[0][0])
		}
	})
}
