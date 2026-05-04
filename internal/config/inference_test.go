package config

import (
	"math"
	"testing"
)

func TestInferenceEngineInferGARCH(t *testing.T) {
	// Generate synthetic GARCH(1,1) returns: omega=0.000001, alpha=0.1, beta=0.85
	returns := generateSyntheticGARCHReturns(200, 0.000001, 0.1, 0.85)

	ie := NewInferenceEngine(nil)
	garch, err := ie.InferGARCH(returns)
	if err != nil {
		t.Fatalf("InferGARCH failed: %v", err)
	}

	// Verify stationarity constraint
	if garch.Alpha+garch.Beta >= 1.0 {
		t.Errorf("alpha+beta=%f violates stationarity", garch.Alpha+garch.Beta)
	}

	// Verify parameters are in reasonable ranges
	if garch.Omega <= 0 {
		t.Errorf("omega=%f must be positive", garch.Omega)
	}
	if garch.Alpha <= 0 || garch.Alpha > 0.5 {
		t.Errorf("alpha=%f out of reasonable range", garch.Alpha)
	}
	if garch.Beta <= 0 || garch.Beta > 0.99 {
		t.Errorf("beta=%f out of reasonable range", garch.Beta)
	}
}

func TestInferenceEngineInferGARCHInsufficientData(t *testing.T) {
	ie := NewInferenceEngine(nil)
	_, err := ie.InferGARCH([]float64{0.01, 0.02})
	if err == nil {
		t.Fatal("expected error for insufficient data")
	}
}

func TestInferenceEngineEstimateVaR(t *testing.T) {
	// Generate returns with known distribution
	returns := make([]float64, 100)
	for i := range returns {
		// Standard normal * 0.02 (2% daily vol)
		returns[i] = randNormal() * 0.02
	}

	ie := NewInferenceEngine(nil)
	var95, err := ie.EstimateVaR(returns, 0.95)
	if err != nil {
		t.Fatalf("EstimateVaR failed: %v", err)
	}

	if var95.VaR >= 0 {
		t.Errorf("95%% VaR should be negative, got %f", var95.VaR)
	}
	if var95.ES >= 0 {
		t.Errorf("95%% ES should be negative, got %f", var95.ES)
	}
	if math.Abs(var95.ES) < math.Abs(var95.VaR) {
		t.Errorf("ES(%f) should be more extreme than VaR(%f)", var95.ES, var95.VaR)
	}
}

func TestInferenceEngineEstimateVaRInsufficientData(t *testing.T) {
	ie := NewInferenceEngine(nil)
	_, err := ie.EstimateVaR([]float64{0.01}, 0.95)
	if err == nil {
		t.Fatal("expected error for insufficient data")
	}
}

func TestInferenceEngineSweepParameter(t *testing.T) {
	ie := NewInferenceEngine(nil)

	// Simple evaluator: score = parameter value
	evaluator := func(params *ParametersConfig) (float64, error) {
		return params.Darwinian.WeightMax.Value, nil
	}

	result, err := ie.SweepParameter(
		"darwinian_weight_max",
		2.5,
		[]float64{1.0, 2.0, 3.0, 4.0},
		evaluator,
	)
	if err != nil {
		t.Fatalf("SweepParameter failed: %v", err)
	}

	if result.BestValue != 4.0 {
		t.Errorf("expected best value 4.0, got %f", result.BestValue)
	}
	if result.BestScore != 4.0 {
		t.Errorf("expected best score 4.0, got %f", result.BestScore)
	}
}

func TestInferenceEngineCalibrateGARCH(t *testing.T) {
	returns := generateSyntheticGARCHReturns(200, 0.000001, 0.1, 0.85)
	cfg := DefaultParametersConfig()
	ie := NewInferenceEngine(cfg)

	err := ie.CalibrateGARCH(returns)
	if err != nil {
		t.Fatalf("CalibrateGARCH failed: %v", err)
	}

	// Verify parameters were updated
	if cfg.GARCH.Omega.Value <= 0 {
		t.Errorf("omega not calibrated: %f", cfg.GARCH.Omega.Value)
	}
	if cfg.GARCH.Alpha.Value <= 0 {
		t.Errorf("alpha not calibrated: %f", cfg.GARCH.Alpha.Value)
	}
	if cfg.GARCH.Beta.Value <= 0 {
		t.Errorf("beta not calibrated: %f", cfg.GARCH.Beta.Value)
	}
}

func TestInferenceEngineCalibrateVaR(t *testing.T) {
	// Generate volatile returns
	returns := make([]float64, 100)
	for i := range returns {
		returns[i] = randNormal() * 0.03 // 3% daily vol
	}

	cfg := DefaultParametersConfig()
	ie := NewInferenceEngine(cfg)

	err := ie.CalibrateVaR(returns)
	if err != nil {
		t.Fatalf("CalibrateVaR failed: %v", err)
	}

	// Verify parameters were updated
	if cfg.Sizing.MaxDrawdownLimit.Value <= 0 {
		t.Errorf("max drawdown limit not calibrated: %f", cfg.Sizing.MaxDrawdownLimit.Value)
	}
	if cfg.Sizing.TargetVolatility.Value <= 0 {
		t.Errorf("target volatility not calibrated: %f", cfg.Sizing.TargetVolatility.Value)
	}
}

// generateSyntheticGARCHReturns generates synthetic returns from a GARCH(1,1) process.
func generateSyntheticGARCHReturns(n int, omega, alpha, beta float64) []float64 {
	returns := make([]float64, n)
	variance := omega / (1.0 - alpha - beta)

	for i := range returns {
		z := randNormal()
		returns[i] = z * math.Sqrt(variance)
		variance = omega + alpha*returns[i]*returns[i] + beta*variance
	}

	return returns
}

var normalSequence = []float64{
	0.5, -0.3, 0.8, -0.6, 0.2, -0.9, 0.4, -0.1, 0.7, -0.5,
	0.3, -0.8, 0.1, -0.4, 0.6, -0.2, 0.9, -0.7, 0.5, -0.3,
}
var normalIndex int

func randNormal() float64 {
	v := normalSequence[normalIndex%len(normalSequence)]
	normalIndex++
	return v
}
