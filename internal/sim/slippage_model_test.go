package sim

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestDefaultSlippageModelTiers(t *testing.T) {
	model := DefaultSlippageModel()
	if len(model.TierBPS) != 3 {
		t.Fatalf("expected 3 tiers, got %d", len(model.TierBPS))
	}

	quotes := map[string]domain.Quote{
		"A": {Symbol: "A", Volume: 1000000}, // top tier
		"B": {Symbol: "B", Volume: 500000},  // middle
		"C": {Symbol: "C", Volume: 10000},   // bottom tier
		"D": {Symbol: "D", Volume: 400000},  // middle
		"E": {Symbol: "E", Volume: 800000},  // middle (not top 20% with 5 items)
	}

	tests := []struct {
		symbol   string
		expected float64
	}{
		{"A", 5},  // top 20% = 0.05%
		{"E", 15}, // middle 60% = 0.15% (with 5 items, only 1 is top 20%)
		{"B", 15}, // middle 60% = 0.15%
		{"D", 15}, // middle 60% = 0.15%
		{"C", 50}, // bottom 20% = 0.50%
	}

	for _, tt := range tests {
		got := model.CalculateSlippageBPS(tt.symbol, quotes, nil)
		if got != tt.expected {
			t.Errorf("CalculateSlippageBPS(%s) = %v, want %v", tt.symbol, got, tt.expected)
		}
	}
}

func TestSlippageModelMissingQuote(t *testing.T) {
	model := DefaultSlippageModel()
	quotes := map[string]domain.Quote{
		"A": {Symbol: "A", Volume: 1000000},
	}

	// Symbol not in quotes should return most conservative (bottom tier)
	got := model.CalculateSlippageBPS("Z", quotes, nil)
	if got != 50 {
		t.Errorf("missing symbol slippage = %v, want 50", got)
	}
}

func TestSlippageModelZeroVolume(t *testing.T) {
	model := DefaultSlippageModel()
	quotes := map[string]domain.Quote{
		"A": {Symbol: "A", Volume: 0},
	}

	got := model.CalculateSlippageBPS("A", quotes, nil)
	if got != 50 {
		t.Errorf("zero volume slippage = %v, want 50", got)
	}
}

func TestSlippageModelNil(t *testing.T) {
	var model *SlippageModel
	quotes := map[string]domain.Quote{
		"A": {Symbol: "A", Volume: 1000000},
	}

	got := model.CalculateSlippageBPS("A", quotes, nil)
	if got != 15 {
		t.Errorf("nil model slippage = %v, want 15", got)
	}
}

func TestEngineUsesDynamicSlippage(t *testing.T) {
	model := DefaultSlippageModel()
	engine := NewEngine(domain.SimulationConstraints{
		StartingCash:                1000000,
		MaxPositionWeight:           0.25,
		MaxOpenPositions:            2,
		MinTradableVolume:           1000,
		MinRecommendationConviction: 0,
		TransactionCostBPS:          0,
		SlippageBPS:                 1, // fixed fallback
		ReserveCashFraction:         0.1,
	}).WithSlippageModel(model)

	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 800, Volume: 10000000, IsTradable: true}, // top tier -> 5 bps
		{Symbol: "2317.TW", Last: 160, Volume: 10000, IsTradable: true},    // bottom tier -> 50 bps
		{Symbol: "A", Last: 100, Volume: 5000000, IsTradable: true},
		{Symbol: "B", Last: 100, Volume: 3000000, IsTradable: true},
		{Symbol: "C", Last: 100, Volume: 2000000, IsTradable: true},
		{Symbol: "D", Last: 100, Volume: 1000000, IsTradable: true},
		{Symbol: "E", Last: 100, Volume: 500000, IsTradable: true},
		{Symbol: "F", Last: 100, Volume: 200000, IsTradable: true},
		{Symbol: "G", Last: 100, Volume: 100000, IsTradable: true},
		{Symbol: "H", Last: 100, Volume: 50000, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 90, Reason: "test"},
		{Symbol: "2317.TW", Side: domain.SideBuy, Conviction: 80, Reason: "test"},
	}

	result := engine.Run(domain.RegimeRiskOn, quotes, recs)

	// Find orders and verify prices reflect dynamic slippage
	for _, o := range result.Orders {
		if o.Side == domain.SideBuy {
			expectedSlippageBPS := 5.0
			if o.Symbol == "2317.TW" {
				expectedSlippageBPS = 50.0
			}
			expectedPrice := 800.0 * (1 + expectedSlippageBPS/10000.0)
			if o.Symbol == "2317.TW" {
				expectedPrice = 160.0 * (1 + expectedSlippageBPS/10000.0)
			}
			if math.Abs(o.Price-expectedPrice) > 0.01 {
				t.Errorf("%s buy price = %.4f, want %.4f (slippage %.0f bps)", o.Symbol, o.Price, expectedPrice, expectedSlippageBPS)
			}
		}
	}
}

func TestEngineFallbackToFixedSlippage(t *testing.T) {
	engine := NewEngine(domain.SimulationConstraints{
		StartingCash:                1000000,
		MaxPositionWeight:           0.25,
		MaxOpenPositions:            1,
		MinTradableVolume:           1000,
		MinRecommendationConviction: 0,
		TransactionCostBPS:          0,
		SlippageBPS:                 10, // fixed 0.1%
		ReserveCashFraction:         0.1,
	})
	// No WithSlippageModel call - should use fixed SlippageBPS

	quotes := []domain.Quote{
		{Symbol: "2330.TW", Last: 800, Volume: 1000000, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 90, Reason: "test"},
	}

	result := engine.Run(domain.RegimeRiskOn, quotes, recs)

	expectedPrice := 800.0 * (1 + 10.0/10000.0)
	for _, o := range result.Orders {
		if o.Side == domain.SideBuy {
			if math.Abs(o.Price-expectedPrice) > 0.01 {
				t.Errorf("fallback price = %.4f, want %.4f", o.Price, expectedPrice)
			}
		}
	}
}

func TestAdjustPriceForSlippage(t *testing.T) {
	tests := []struct {
		price    float64
		bps      float64
		side     domain.Side
		expected float64
	}{
		{100, 50, domain.SideBuy, 100.5},
		{100, 50, domain.SideSell, 99.5},
		{200, 10, domain.SideBuy, 200.2},
		{200, 10, domain.SideSell, 199.8},
	}

	for _, tt := range tests {
		got := AdjustPriceForSlippage(tt.price, tt.bps, tt.side)
		if math.Abs(got-tt.expected) > 0.001 {
			t.Errorf("AdjustPriceForSlippage(%.2f, %.2f, %s) = %.4f, want %.4f",
				tt.price, tt.bps, tt.side, got, tt.expected)
		}
	}
}

func TestSlippageModelPrecompute(t *testing.T) {
	model := DefaultSlippageModel()
	quotes := map[string]domain.Quote{
		"A": {Symbol: "A", Volume: 1000000},
		"B": {Symbol: "B", Volume: 500000},
		"C": {Symbol: "C", Volume: 10000},
		"D": {Symbol: "D", Volume: 400000},
		"E": {Symbol: "E", Volume: 800000},
	}

	model.Precompute(quotes)

	if model.precomputedLen != len(quotes) {
		t.Errorf("precomputedLen = %d, want %d", model.precomputedLen, len(quotes))
	}

	if len(model.sortedVolumes) != 5 {
		t.Errorf("sortedVolumes len = %d, want 5", len(model.sortedVolumes))
	}

	for i := 1; i < len(model.sortedVolumes); i++ {
		if model.sortedVolumes[i-1] > model.sortedVolumes[i] {
			t.Errorf("sortedVolumes not sorted: %v", model.sortedVolumes)
		}
	}
}

func TestSlippageModelPrecomputeEmptyQuotes(t *testing.T) {
	model := DefaultSlippageModel()
	model.Precompute(map[string]domain.Quote{})

	if model.precomputedLen != 0 {
		t.Errorf("precomputedLen = %d, want 0", model.precomputedLen)
	}
}

func TestSlippageModelPrecomputeNil(t *testing.T) {
	var model *SlippageModel
	model.Precompute(map[string]domain.Quote{
		"A": {Symbol: "A", Volume: 1000},
	})
	// Should not panic
}

func TestSlippageModelUsesPrecomputed(t *testing.T) {
	model := DefaultSlippageModel()
	quotes := map[string]domain.Quote{
		"A": {Symbol: "A", Volume: 1000000},
		"B": {Symbol: "B", Volume: 500000},
		"C": {Symbol: "C", Volume: 10000},
		"D": {Symbol: "D", Volume: 400000},
		"E": {Symbol: "E", Volume: 800000},
	}

	model.Precompute(quotes)

	tests := []struct {
		symbol   string
		expected float64
	}{
		{"A", 5},  // top 20%
		{"E", 15}, // middle 60%
		{"B", 15}, // middle 60%
		{"D", 15}, // middle 60%
		{"C", 50}, // bottom 20%
	}

	for _, tt := range tests {
		got := model.CalculateSlippageBPS(tt.symbol, quotes, nil)
		if got != tt.expected {
			t.Errorf("with precompute: CalculateSlippageBPS(%s) = %v, want %v", tt.symbol, got, tt.expected)
		}
	}
}

func TestSlippageModelFallbackWithoutPrecompute(t *testing.T) {
	model := DefaultSlippageModel()
	quotes := map[string]domain.Quote{
		"A": {Symbol: "A", Volume: 1000000},
		"B": {Symbol: "B", Volume: 500000},
		"C": {Symbol: "C", Volume: 10000},
		"D": {Symbol: "D", Volume: 400000},
		"E": {Symbol: "E", Volume: 800000},
	}

	tests := []struct {
		symbol   string
		expected float64
	}{
		{"A", 5},  // top 20%
		{"E", 15}, // middle 60%
		{"B", 15}, // middle 60%
		{"D", 15}, // middle 60%
		{"C", 50}, // bottom 20%
	}

	for _, tt := range tests {
		got := model.CalculateSlippageBPS(tt.symbol, quotes, nil)
		if got != tt.expected {
			t.Errorf("without precompute: CalculateSlippageBPS(%s) = %v, want %v", tt.symbol, got, tt.expected)
		}
	}
}

func TestSlippageModelConsistencyWithAndWithoutPrecompute(t *testing.T) {
	model := DefaultSlippageModel()
	quotes := map[string]domain.Quote{
		"A": {Symbol: "A", Volume: 1000000},
		"B": {Symbol: "B", Volume: 500000},
		"C": {Symbol: "C", Volume: 10000},
		"D": {Symbol: "D", Volume: 400000},
		"E": {Symbol: "E", Volume: 800000},
	}

	for symbol := range quotes {
		withoutPrecompute := model.CalculateSlippageBPS(symbol, quotes, nil)

		model.Precompute(quotes)
		withPrecompute := model.CalculateSlippageBPS(symbol, quotes, nil)

		if withPrecompute != withoutPrecompute {
			t.Errorf("inconsistent results for %s: precompute=%v, fallback=%v",
				symbol, withPrecompute, withoutPrecompute)
		}
	}
}

func TestSlippageModelStaleCache(t *testing.T) {
	model := DefaultSlippageModel()
	quotes5 := map[string]domain.Quote{
		"A": {Symbol: "A", Volume: 1000000},
		"B": {Symbol: "B", Volume: 500000},
		"C": {Symbol: "C", Volume: 10000},
		"D": {Symbol: "D", Volume: 400000},
		"E": {Symbol: "E", Volume: 800000},
	}

	quotes3 := map[string]domain.Quote{
		"A": {Symbol: "A", Volume: 1000000},
		"B": {Symbol: "B", Volume: 500000},
		"C": {Symbol: "C", Volume: 10000},
	}

	model.Precompute(quotes5)

	result := model.CalculateSlippageBPS("A", quotes3, nil)

	// With 3-item fallback, A's percentile = 2/3 ≈ 0.667 (middle tier = 15)
	if result != 15 {
		t.Errorf("expected fallback to 15 for mismatched cache, got %v", result)
	}
}
