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

// --- SetParameter / GetParameter / ListParameters round-trip tests ---

func TestInferenceEngineSetGetParameterRoundTrip(t *testing.T) {
	cfg := DefaultParametersConfig()
	ie := NewInferenceEngine(cfg)

	tests := []struct {
		name      string
		paramName string
		newValue  float64
		expected  float64
	}{
		{"darwinian_weight_max", "darwinian_weight_max", 3.5, 3.5},
		{"factor_momentum_lookback_days", "factor_momentum_lookback_days", 45, 45},
		{"optimizer_max_position_pct", "optimizer_max_position_pct", 0.15, 0.15},
		{"sizing_kelly_fraction", "sizing_kelly_fraction", 0.35, 0.35},
		{"health_mute_threshold", "health_mute_threshold", 3, 3},
		{"garch_omega", "garch_omega", 0.000002, 0.000002},
		{"experiment_improvement_threshold", "experiment_improvement_threshold", 0.08, 0.08},
		{"baseline_starting_cash", "baseline_starting_cash", 1500000, 1500000},
		{"orchestrator_cro_zscore_threshold", "orchestrator_cro_zscore_threshold", 2.2, 2.2},
		{"risk_max_drawdown_pct", "risk_max_drawdown_pct", 0.12, 0.12},
		{"realtime_volatility_threshold", "realtime_volatility_threshold", 0.035, 0.035},
		{"janus_short_window_days", "janus_short_window_days", 15, 15},
		{"narrative_min_confidence", "narrative_min_confidence", 0.75, 0.75},
		{"marketdata_twse_api_timeout_sec", "marketdata_twse_api_timeout_sec", 45, 45},
		{"strategy_switch_threshold", "strategy_switch_threshold", 0.65, 0.65},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ie.SetParameter(tt.paramName, tt.newValue); err != nil {
				t.Fatalf("SetParameter(%q, %v) failed: %v", tt.paramName, tt.newValue, err)
			}
			got, ok := ie.GetParameter(tt.paramName)
			if !ok {
				t.Fatalf("GetParameter(%q) returned not found after Set", tt.paramName)
			}
			if got != tt.expected {
				t.Errorf("GetParameter(%q) = %v, want %v", tt.paramName, got, tt.expected)
			}
		})
	}
}

func TestInferenceEngineGetParameterNotFound(t *testing.T) {
	ie := NewInferenceEngine(nil)
	_, ok := ie.GetParameter("nonexistent_parameter")
	if ok {
		t.Error("expected GetParameter to return false for unknown parameter")
	}
}

func TestInferenceEngineSetParameterUnknown(t *testing.T) {
	ie := NewInferenceEngine(nil)
	err := ie.SetParameter("nonexistent_parameter", 1.0)
	if err == nil {
		t.Fatal("expected error for unknown parameter")
	}
}

func TestInferenceEngineListParameters(t *testing.T) {
	ie := NewInferenceEngine(nil)
	params := ie.ListParameters()
	if len(params) == 0 {
		t.Fatal("ListParameters returned empty slice")
	}

	expected := []string{
		"darwinian_weight_max",
		"factor_momentum_lookback_days",
		"optimizer_max_position_pct",
		"sizing_kelly_fraction",
		"garch_omega",
		"risk_max_drawdown_pct",
	}
	paramSet := make(map[string]bool, len(params))
	for _, p := range params {
		paramSet[p] = true
	}
	for _, e := range expected {
		if !paramSet[e] {
			t.Errorf("ListParameters missing expected parameter: %q", e)
		}
	}
}

func TestInferenceEngineMapParameterRoundTrip(t *testing.T) {
	cfg := DefaultParametersConfig()
	ie := NewInferenceEngine(cfg)

	tests := []struct {
		name      string
		paramName string
		newValue  float64
	}{
		{"factor_institutional_sentiment_weights_foreign", "factor_institutional_sentiment_weights_foreign", 0.35},
		{"optimizer_factor_weights_momentum", "optimizer_factor_weights_momentum", 0.25},
		{"risk_sector_constraints_risk_off_semiconductor", "risk_sector_constraints_risk_off_semiconductor", 0.10},
		{"narrative_event_ttl_multiplier_bull", "narrative_event_ttl_multiplier_bull", 1.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ie.SetParameter(tt.paramName, tt.newValue); err != nil {
				t.Fatalf("SetParameter(%q, %v) failed: %v", tt.paramName, tt.newValue, err)
			}
			got, ok := ie.GetParameter(tt.paramName)
			if !ok {
				t.Fatalf("GetParameter(%q) returned not found after Set", tt.paramName)
			}
			if got != tt.newValue {
				t.Errorf("GetParameter(%q) = %v, want %v", tt.paramName, got, tt.newValue)
			}
		})
	}
}

func TestInferenceEngineMapParameterGetNotFound(t *testing.T) {
	ie := NewInferenceEngine(nil)
	_, ok := ie.GetParameter("factor_institutional_sentiment_weights_nonexistent_key")
	if ok {
		t.Error("expected GetParameter to return false for missing map key")
	}
}

func TestInferenceEngineIntParameterConversion(t *testing.T) {
	cfg := DefaultParametersConfig()
	ie := NewInferenceEngine(cfg)

	err := ie.SetParameter("darwinian_lookback_days", 42.7)
	if err != nil {
		t.Fatalf("SetParameter failed: %v", err)
	}
	got, ok := ie.GetParameter("darwinian_lookback_days")
	if !ok {
		t.Fatal("GetParameter returned not found")
	}
	if got != 42 {
		t.Errorf("int parameter conversion: got %v, want 42", got)
	}
}

func TestInferenceEngineSweepParameterWithSetParameter(t *testing.T) {
	cfg := DefaultParametersConfig()
	ie := NewInferenceEngine(cfg)

	result, err := ie.SweepParameter(
		"sizing_kelly_fraction",
		0.25,
		[]float64{0.2, 0.3, 0.4},
		func(params *ParametersConfig) (float64, error) {
			return params.Sizing.KellyFraction.Value, nil
		},
	)
	if err != nil {
		t.Fatalf("SweepParameter failed: %v", err)
	}
	if result.BestValue != 0.4 {
		t.Errorf("expected best value 0.4, got %f", result.BestValue)
	}
	if result.BestScore != 0.4 {
		t.Errorf("expected best score 0.4, got %f", result.BestScore)
	}
}
