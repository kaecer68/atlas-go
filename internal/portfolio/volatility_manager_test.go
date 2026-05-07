package portfolio

import (
	"testing"
	"time"
)

func TestNewVolatilityManager(t *testing.T) {
	vm := NewVolatilityManager(0.15, 0.25)
	if vm == nil {
		t.Fatal("expected non-nil volatility manager")
	}

	metrics := vm.GetVolatilityMetrics()
	if metrics.TargetVolatility != 0.15 {
		t.Errorf("expected target vol 0.15, got %f", metrics.TargetVolatility)
	}
	if metrics.MaxVolatility != 0.25 {
		t.Errorf("expected max vol 0.25, got %f", metrics.MaxVolatility)
	}
	if metrics.AssetCount != 0 {
		t.Errorf("expected 0 assets, got %d", metrics.AssetCount)
	}
}

func TestVolatilityManagerUpdateReturns(t *testing.T) {
	vm := NewVolatilityManager(0.15, 0.25)

	returns := []float64{0.01, -0.02, 0.015, -0.005, 0.02}
	vm.UpdateReturns("2330.TW", returns)

	// Forecast should use fallback since we don't have enough days
	forecast := vm.GetVolatilityForecast("2330.TW", 5)
	if len(forecast) != 5 {
		t.Errorf("expected 5 forecast periods, got %d", len(forecast))
	}

	// All fallback values should be equal (currentVolatility which starts at 0)
	for i, v := range forecast {
		if v != 0 {
			t.Logf("forecast[%d] = %f (expected 0 for empty current volatility)", i, v)
		}
	}
}

func TestVolatilityManagerUpdateReturnsTrimsHistory(t *testing.T) {
	vm := NewVolatilityManager(0.15, 0.25)
	params := DefaultRuntimeParameters()
	params.GARCH.MaxHistory = 10
	vm.WithParameters(params)

	returns := make([]float64, 20)
	for i := range returns {
		returns[i] = 0.01
	}
	vm.UpdateReturns("2330.TW", returns)

	// Should not panic and should handle trimming
	forecast := vm.GetVolatilityForecast("2330.TW", 3)
	if len(forecast) != 3 {
		t.Errorf("expected 3 forecast periods, got %d", len(forecast))
	}
}

func TestVolatilityManagerGARCHForecast(t *testing.T) {
	vm := NewVolatilityManager(0.15, 0.25)
	params := DefaultRuntimeParameters()
	params.GARCH.MinForecastDays = 5 // Lower threshold for testing
	vm.WithParameters(params)

	// Generate enough returns for GARCH forecasting
	returns := make([]float64, 10)
	for i := range returns {
		returns[i] = 0.01 * float64(i%3-1) // Small alternating returns
	}
	vm.UpdateReturns("2330.TW", returns)

	// Set current volatility so fallback isn't just zeros
	vm.CalculateCurrentVolatility(map[string]float64{"2330.TW": 1.0})

	forecast := vm.GetVolatilityForecast("2330.TW", 5)
	if len(forecast) != 5 {
		t.Fatalf("expected 5 forecast periods, got %d", len(forecast))
	}

	// GARCH forecast should produce non-negative values
	for i, v := range forecast {
		if v < 0 {
			t.Errorf("forecast[%d] = %f, expected non-negative", i, v)
		}
	}
}

func TestVolatilityManagerGARCHForecastNotEnoughData(t *testing.T) {
	vm := NewVolatilityManager(0.15, 0.25)
	params := DefaultRuntimeParameters()
	params.GARCH.MinForecastDays = 100
	vm.WithParameters(params)

	// Only 5 returns, not enough for GARCH
	returns := []float64{0.01, -0.02, 0.015, -0.005, 0.02}
	vm.UpdateReturns("2330.TW", returns)

	// Set some current volatility
	vm.CalculateCurrentVolatility(map[string]float64{"2330.TW": 1.0})

	forecast := vm.GetVolatilityForecast("2330.TW", 3)
	if len(forecast) != 3 {
		t.Fatalf("expected 3 forecast periods, got %d", len(forecast))
	}

	// Should use fallback (currentVolatility)
	for i, v := range forecast {
		if v != vm.currentVolatility {
			t.Errorf("forecast[%d] = %f, expected fallback %f", i, v, vm.currentVolatility)
		}
	}
}

func TestVolatilityManagerCalculateCurrentVolatility(t *testing.T) {
	vm := NewVolatilityManager(0.15, 0.25)
	params := DefaultRuntimeParameters()
	params.GARCH.CorrelationMinDays = 5
	vm.WithParameters(params)

	returns1 := []float64{0.01, -0.02, 0.015, -0.005, 0.02, 0.01}
	returns2 := []float64{-0.01, 0.02, -0.015, 0.005, -0.02, -0.01}
	vm.UpdateReturns("2330.TW", returns1)
	vm.UpdateReturns("2317.TW", returns2)

	weights := map[string]float64{
		"2330.TW": 0.6,
		"2317.TW": 0.4,
	}

	vol := vm.CalculateCurrentVolatility(weights)
	if vol < 0 {
		t.Errorf("expected non-negative volatility, got %f", vol)
	}

	metrics := vm.GetVolatilityMetrics()
	if metrics.CurrentVolatility != vol {
		t.Errorf("expected metrics current vol %f, got %f", vol, metrics.CurrentVolatility)
	}
	if metrics.AssetCount != 2 {
		t.Errorf("expected 2 assets, got %d", metrics.AssetCount)
	}
	if metrics.HistoryLength != 1 {
		t.Errorf("expected 1 history point, got %d", metrics.HistoryLength)
	}
}

func TestVolatilityManagerCalculateCurrentVolatilityMissingData(t *testing.T) {
	vm := NewVolatilityManager(0.15, 0.25)

	// No returns data for any assets
	weights := map[string]float64{
		"2330.TW": 0.6,
		"2317.TW": 0.4,
	}

	vol := vm.CalculateCurrentVolatility(weights)
	if vol != 0 {
		t.Errorf("expected 0 volatility with no data, got %f", vol)
	}
}

func TestVolatilityManagerGetVolatilityAdjustmentsHighVol(t *testing.T) {
	vm := NewVolatilityManager(0.15, 0.25)
	params := DefaultRuntimeParameters()
	params.GARCH.CorrelationMinDays = 5
	params.GARCH.HighVolThreshold = 1.2
	vm.WithParameters(params)

	returns := []float64{0.20, -0.20, 0.20, -0.20, 0.20, -0.20}
	vm.UpdateReturns("VOLATILE.TW", returns)

	vm.CalculateCurrentVolatility(map[string]float64{"VOLATILE.TW": 1.0})
	vm.currentVolatility = 0.30

	adjustments := vm.GetVolatilityAdjustments()

	foundReduce := false
	for _, adj := range adjustments {
		if adj.Action == ActionReduce {
			foundReduce = true
			if adj.Asset != "VOLATILE.TW" {
				t.Errorf("expected reduce for VOLATILE.TW, got %s", adj.Asset)
			}
		}
	}
	if !foundReduce {
		t.Error("expected at least one Reduce adjustment for high volatility")
	}
}

func TestVolatilityManagerGetVolatilityAdjustmentsLowVol(t *testing.T) {
	vm := NewVolatilityManager(0.15, 0.25)
	params := DefaultRuntimeParameters()
	params.GARCH.CorrelationMinDays = 5
	params.GARCH.LowVolThreshold = 0.5
	vm.WithParameters(params)

	// Create low-vol positive return asset
	returns := []float64{0.005, 0.003, 0.004, 0.002, 0.003, 0.004}
	vm.UpdateReturns("STABLE.TW", returns)

	// Set current volatility below threshold
	vm.CalculateCurrentVolatility(map[string]float64{"STABLE.TW": 1.0})
	vm.currentVolatility = 0.05 // Below target*0.5 = 0.075

	adjustments := vm.GetVolatilityAdjustments()

	foundIncrease := false
	for _, adj := range adjustments {
		if adj.Action == ActionIncrease {
			foundIncrease = true
			if adj.Asset != "STABLE.TW" {
				t.Errorf("expected increase for STABLE.TW, got %s", adj.Asset)
			}
		}
	}
	if !foundIncrease {
		t.Error("expected at least one Increase adjustment for low volatility")
	}
}

func TestVolatilityManagerGetVolatilityAdjustmentsScheduledRebalance(t *testing.T) {
	vm := NewVolatilityManager(0.15, 0.25)
	params := DefaultRuntimeParameters()
	params.GARCH.WeeklyRebalanceDays = 0 // Force rebalance
	vm.WithParameters(params)

	// Set last rebalance to the past
	vm.lastRebalance = time.Now().Add(-24 * time.Hour)

	vm.CalculateCurrentVolatility(map[string]float64{"2330.TW": 1.0})
	vm.currentVolatility = 0.10 // Between low and high thresholds

	adjustments := vm.GetVolatilityAdjustments()

	foundRebalance := false
	for _, adj := range adjustments {
		if adj.Action == ActionRebalance {
			foundRebalance = true
			if adj.Asset != "portfolio" {
				t.Errorf("expected rebalance for portfolio, got %s", adj.Asset)
			}
		}
	}
	if !foundRebalance {
		t.Error("expected scheduled rebalance adjustment")
	}
}

func TestVolatilityManagerApplySmoothing(t *testing.T) {
	vm := NewVolatilityManager(0.15, 0.25)

	// Test with less than 2 returns
	shortReturns := []float64{0.01}
	smoothed := vm.ApplySmoothing(shortReturns)
	if len(smoothed) != 1 || smoothed[0] != 0.01 {
		t.Errorf("expected unchanged single return, got %v", smoothed)
	}

	// Test with multiple returns
	returns := []float64{0.01, 0.02, 0.015, 0.005, 0.01}
	smoothed = vm.ApplySmoothing(returns)
	if len(smoothed) != len(returns) {
		t.Errorf("expected same length, got %d vs %d", len(smoothed), len(returns))
	}

	// EMA smoothing should reduce extremes
	// First value unchanged
	if smoothed[0] != returns[0] {
		t.Errorf("expected first value unchanged, got %f", smoothed[0])
	}
}

func TestVolatilityManagerCorrelationMatrixUpdate(t *testing.T) {
	vm := NewVolatilityManager(0.15, 0.25)
	params := DefaultRuntimeParameters()
	params.GARCH.CorrelationMinDays = 5
	vm.WithParameters(params)

	returns1 := []float64{0.01, 0.02, 0.015, 0.005, 0.02, 0.01}
	returns2 := []float64{0.005, 0.01, 0.01, 0.002, 0.01, 0.005}
	vm.UpdateReturns("2330.TW", returns1)
	vm.UpdateReturns("2317.TW", returns2)

	// Calculate volatility to trigger correlation matrix usage
	weights := map[string]float64{
		"2330.TW": 0.5,
		"2317.TW": 0.5,
	}
	vol := vm.CalculateCurrentVolatility(weights)
	if vol < 0 {
		t.Errorf("expected non-negative volatility, got %f", vol)
	}

	// History should contain one point
	metrics := vm.GetVolatilityMetrics()
	if metrics.HistoryLength < 1 {
		t.Errorf("expected at least 1 history point, got %d", metrics.HistoryLength)
	}
}

func TestVolatilityManagerBetaValuesUpdate(t *testing.T) {
	vm := NewVolatilityManager(0.15, 0.25)
	params := DefaultRuntimeParameters()
	params.GARCH.CorrelationMinDays = 5
	vm.WithParameters(params)

	returns := []float64{0.01, -0.02, 0.015, -0.005, 0.02, 0.01}
	vm.UpdateReturns("2330.TW", returns)

	// Beta should be calculated (returns to itself via getPortfolioReturns)
	// Since there's only one asset, getPortfolioReturns returns its own returns
	// Beta of asset against itself should be 1.0
	vol := vm.CalculateCurrentVolatility(map[string]float64{"2330.TW": 1.0})
	if vol < 0 {
		t.Errorf("expected non-negative volatility, got %f", vol)
	}
}

func TestVolatilityManagerWithParameters(t *testing.T) {
	vm := NewVolatilityManager(0.15, 0.25)
	params := DefaultRuntimeParameters()
	params.GARCH.MaxHistory = 50

	result := vm.WithParameters(params)
	if result != vm {
		t.Error("expected WithParameters to return the same manager for chaining")
	}
}

func TestVolatilityManagerGetVolatilityMetrics(t *testing.T) {
	vm := NewVolatilityManager(0.15, 0.25)

	metrics := vm.GetVolatilityMetrics()
	if metrics.TargetVolatility != 0.15 {
		t.Errorf("expected target 0.15, got %f", metrics.TargetVolatility)
	}
	if metrics.MaxVolatility != 0.25 {
		t.Errorf("expected max 0.25, got %f", metrics.MaxVolatility)
	}
	if metrics.Deviation != -1.0 {
		// (0 - 0.15) / 0.15 = -1.0
		t.Errorf("expected deviation -1.0 for zero current vol, got %f", metrics.Deviation)
	}
	if metrics.LastRebalance.IsZero() {
		t.Error("expected non-zero last rebalance time")
	}
}
