package risk

import (
	"math"
	"math/rand"
	"testing"
)

const (
	dccTestSeed      = 0xDCCB1A7E
	dccTestTolerance = 0.20
)

func makeCorrelatedNormals(n int, rho float64, seed int64) ([]float64, []float64) {
	rng := rand.New(rand.NewSource(seed))
	a := make([]float64, n)
	b := make([]float64, n)
	sqrtTerm := math.Sqrt(1.0 - rho*rho)
	for i := range n {
		z1 := rng.NormFloat64()
		z2 := rng.NormFloat64()
		a[i] = z1
		b[i] = rho*z1 + sqrtTerm*z2
	}
	return a, b
}

func meanOfSlice(s []float64) float64 {
	if len(s) == 0 {
		return 0
	}
	var sum float64
	for _, v := range s {
		sum += v
	}
	return sum / float64(len(s))
}

func TestDCCGARCH_KnownCorrelation(t *testing.T) {
	const (
		n   = 1000
		rho = 0.5
	)
	a, b := makeCorrelatedNormals(n, rho, dccTestSeed)
	model := &DCCGARCH{}
	result, err := model.Fit(a, b)
	if err != nil {
		t.Fatalf("Fit returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Fit returned nil result")
	}
	rhat := meanOfSlice(result.Rho)
	if math.Abs(rhat-rho) > dccTestTolerance {
		t.Errorf("estimated mean rho = %.4f, want within %.2f of true rho = %.2f", rhat, dccTestTolerance, rho)
	}
	t.Logf("known-corr test: n=%d true=%.2f rhat=%.4f dccA=%.4f dccB=%.4f", n, rho, rhat, model.dccA, model.dccB)
}

func TestDCCGARCH_RegimeSwitch(t *testing.T) {
	calmA, calmB := makeCorrelatedNormals(600, 0.10, dccTestSeed)
	stressA, stressB := makeCorrelatedNormals(600, 0.60, dccTestSeed+1)
	jointA := append(append([]float64{}, calmA...), stressA...)
	jointB := append(append([]float64{}, calmB...), stressB...)
	model := &DCCGARCH{}
	result, err := model.Fit(jointA, jointB)
	if err != nil {
		t.Fatalf("Fit returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Fit returned nil result")
	}
	mid := len(result.Rho) / 2
	meanCalm := meanOfSlice(result.Rho[:mid])
	meanStress := meanOfSlice(result.Rho[mid:])
	if meanStress <= meanCalm {
		t.Errorf("expected stress mean rho > calm mean rho; got calm=%.4f stress=%.4f", meanCalm, meanStress)
	}
	t.Logf("regime-switch test: calm mean rho=%.4f stress mean rho=%.4f", meanCalm, meanStress)
}

func TestDCCGARCH_NumericalStability(t *testing.T) {
	rng := rand.New(rand.NewSource(dccTestSeed + 2))
	n := 500
	a := make([]float64, n)
	b := make([]float64, n)
	for i := range n {
		if i%50 == 0 {
			a[i] = (rng.Float64() - 0.5) * 1e6
			b[i] = (rng.Float64() - 0.5) * 1e6
		} else {
			a[i] = rng.NormFloat64() * 0.01
			b[i] = rng.NormFloat64() * 0.01
		}
	}
	model := &DCCGARCH{}
	result, err := model.Fit(a, b)
	if err != nil {
		t.Fatalf("Fit returned error on extreme inputs: %v", err)
	}
	if result == nil {
		t.Fatal("Fit returned nil result")
	}
	for i, v := range result.Rho {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("rho[%d] is non-finite: %v", i, v)
		}
	}
	for i, v := range result.SigmaA2 {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("sigmaA2[%d] is non-finite: %v", i, v)
		}
	}
	for i, v := range result.SigmaB2 {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Errorf("sigmaB2[%d] is non-finite: %v", i, v)
		}
	}
}

func TestDCCGARCH_Stationarity(t *testing.T) {
	rng := rand.New(rand.NewSource(dccTestSeed + 3))
	n := 1000
	a := make([]float64, n)
	b := make([]float64, n)
	rho := 0.9
	sqrtTerm := math.Sqrt(1.0 - rho*rho)
	for i := range n {
		z1 := rng.NormFloat64() * 2.0
		z2 := rng.NormFloat64() * 2.0
		a[i] = z1
		b[i] = rho*z1 + sqrtTerm*z2
	}
	model := &DCCGARCH{}
	result, err := model.Fit(a, b)
	if err != nil {
		t.Fatalf("Fit returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Fit returned nil result")
	}
	const slack = 1e-3
	if model.dccA+model.dccB >= 1.0+slack {
		t.Errorf("stationarity violated: dccA=%.6f dccB=%.6f sum=%.6f >= 1.0", model.dccA, model.dccB, model.dccA+model.dccB)
	}
	if model.dccA < 0 || model.dccB < 0 {
		t.Errorf("DCC parameters must be non-negative: dccA=%.6f dccB=%.6f", model.dccA, model.dccB)
	}
}

func TestDCCGARCH_OutputLength(t *testing.T) {
	const n = 250
	a, b := makeCorrelatedNormals(n, 0.3, dccTestSeed+4)
	model := &DCCGARCH{}
	result, err := model.Fit(a, b)
	if err != nil {
		t.Fatalf("Fit returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Fit returned nil result")
	}
	if len(result.SigmaA2) != n {
		t.Errorf("len(SigmaA2) = %d, want %d", len(result.SigmaA2), n)
	}
	if len(result.SigmaB2) != n {
		t.Errorf("len(SigmaB2) = %d, want %d", len(result.SigmaB2), n)
	}
	if len(result.Rho) != n {
		t.Errorf("len(Rho) = %d, want %d", len(result.Rho), n)
	}
	if len(result.StdResidA) != n {
		t.Errorf("len(StdResidA) = %d, want %d", len(result.StdResidA), n)
	}
	if len(result.StdResidB) != n {
		t.Errorf("len(StdResidB) = %d, want %d", len(result.StdResidB), n)
	}
}

func TestDCCGARCH_InputValidation(t *testing.T) {
	model := &DCCGARCH{}
	if _, err := model.Fit(nil, nil); err == nil {
		t.Error("expected error on nil input, got nil")
	}
	if _, err := model.Fit([]float64{0.1, 0.2}, []float64{0.1}); err == nil {
		t.Error("expected error on length mismatch, got nil")
	}
	short := make([]float64, dccMinObservations-1)
	for i := range short {
		short[i] = 0.01 * float64(i+1)
	}
	if _, err := model.Fit(short, short); err == nil {
		t.Error("expected error on too-short input, got nil")
	}
}

func TestFitGARCH11Fallback_KnownValues(t *testing.T) {
	mean := 0.001
	demean := []float64{-0.01, 0.02, -0.005, 0.015, -0.02}
	uncond := sampleVariance(demean)
	if uncond <= 0 {
		t.Fatal("unconditional variance must be positive for fallback test")
	}

	fit := fitGARCH11Fallback(demean, uncond, mean)

	if len(fit.sigma2) != len(demean) {
		t.Errorf("sigma2 length %d, want %d", len(fit.sigma2), len(demean))
	}
	if len(fit.stdResid) != len(demean) {
		t.Errorf("stdResid length %d, want %d", len(fit.stdResid), len(demean))
	}
	for i, v := range fit.sigma2 {
		if math.Abs(v-uncond) > 1e-12 {
			t.Errorf("sigma2[%d] = %.12g, want unconditional variance %.12g", i, v, uncond)
		}
	}
	for i, v := range fit.stdResid {
		want := demean[i] / math.Sqrt(uncond)
		if math.Abs(v-want) > 1e-12 {
			t.Errorf("stdResid[%d] = %.12g, want %.12g", i, v, want)
		}
	}
	if fit.mean != mean {
		t.Errorf("mean = %.6g, want %.6g", fit.mean, mean)
	}
	if fit.omega != uncond {
		t.Errorf("omega = %.12g, want %.12g", fit.omega, uncond)
	}
	if fit.alpha != 0 || fit.beta != 0 {
		t.Errorf("fallback must have alpha=beta=0, got alpha=%.12g beta=%.12g", fit.alpha, fit.beta)
	}
}
