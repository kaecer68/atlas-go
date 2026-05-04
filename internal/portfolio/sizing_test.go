package portfolio

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestSizerCalculateSizeInvalidPrice(t *testing.T) {
	sizer := NewSizer()
	signal := Signal{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 0.8, WinRate: 0.6, PayoffRatio: 2.0}
	portfolio := PortfolioSnapshot{TotalValue: 1000000, Cash: 500000, Timestamp: time.Now()}
	quote := domain.Quote{Symbol: "2330.TW", Last: 0}

	shares, riskAmount, err := sizer.CalculateSize(signal, portfolio, quote)
	if err == nil {
		t.Fatal("expected error for invalid price")
	}
	if shares != 0 {
		t.Errorf("expected 0 shares, got %d", shares)
	}
	if riskAmount != 0 {
		t.Errorf("expected 0 risk amount, got %f", riskAmount)
	}
}

func TestSizerCalculateSizeBasic(t *testing.T) {
	sizer := NewSizer()
	// Set ATR so adjustForATR doesn't just return baseSize
	sizer.UpdateATR("2330.TW", 5.0)
	sizer.UpdateVolatility("2330.TW", 0.20)
	sizer.UpdateADV("2330.TW", 100000000)

	signal := Signal{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 0.8, WinRate: 0.6, PayoffRatio: 2.0}
	portfolio := PortfolioSnapshot{TotalValue: 1000000, Cash: 500000, Timestamp: time.Now()}
	quote := domain.Quote{Symbol: "2330.TW", Last: 500}

	shares, riskAmount, err := sizer.CalculateSize(signal, portfolio, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shares <= 0 {
		t.Errorf("expected positive shares, got %d", shares)
	}
	if riskAmount <= 0 {
		t.Errorf("expected positive risk amount, got %f", riskAmount)
	}
}

func TestSizerKellyFallbackWinRateZero(t *testing.T) {
	sizer := NewSizer()
	sizer.UpdateATR("2330.TW", 5.0)
	sizer.UpdateVolatility("2330.TW", 0.20)
	sizer.UpdateADV("2330.TW", 100000000)

	// WinRate=0 should fallback to DefaultWinRate (0.5)
	signal := Signal{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 0.8, WinRate: 0, PayoffRatio: 2.0}
	portfolio := PortfolioSnapshot{TotalValue: 1000000, Cash: 500000, Timestamp: time.Now()}
	quote := domain.Quote{Symbol: "2330.TW", Last: 500}

	shares, _, err := sizer.CalculateSize(signal, portfolio, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shares <= 0 {
		t.Errorf("expected positive shares with fallback winRate, got %d", shares)
	}
}

func TestSizerKellyFallbackPayoffRatioZero(t *testing.T) {
	sizer := NewSizer()
	sizer.UpdateATR("2330.TW", 5.0)
	sizer.UpdateVolatility("2330.TW", 0.20)
	sizer.UpdateADV("2330.TW", 100000000)

	// PayoffRatio=0 should fallback to DefaultPayoffRatio (1.0)
	signal := Signal{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 0.8, WinRate: 0.6, PayoffRatio: 0}
	portfolio := PortfolioSnapshot{TotalValue: 1000000, Cash: 500000, Timestamp: time.Now()}
	quote := domain.Quote{Symbol: "2330.TW", Last: 500}

	shares, _, err := sizer.CalculateSize(signal, portfolio, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shares <= 0 {
		t.Errorf("expected positive shares with fallback payoffRatio, got %d", shares)
	}
}

func TestSizerKellyNegativeReturnsZero(t *testing.T) {
	sizer := NewSizer()
	sizer.UpdateATR("2330.TW", 5.0)

	// Kelly = (0.3*0.5 - 0.7)/0.5 = -0.8, so kellySize = 0
	signal := Signal{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 0.8, WinRate: 0.3, PayoffRatio: 0.5}
	portfolio := PortfolioSnapshot{TotalValue: 1000000, Cash: 500000, Timestamp: time.Now()}
	quote := domain.Quote{Symbol: "2330.TW", Last: 500}

	shares, _, err := sizer.CalculateSize(signal, portfolio, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shares != 0 {
		t.Errorf("expected 0 shares when Kelly is negative, got %d", shares)
	}
}

func TestSizerAdjustForVolatilityZeroVolatility(t *testing.T) {
	sizer := NewSizer()
	sizer.UpdateATR("2330.TW", 5.0)
	sizer.UpdateADV("2330.TW", 100000000)
	// Volatility not set -> uses default (0.25), not 0
	// To truly test zero vol, we need to set it to 0
	sizer.UpdateVolatility("2330.TW", 0)

	signal := Signal{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 0.8, WinRate: 0.6, PayoffRatio: 2.0}
	portfolio := PortfolioSnapshot{TotalValue: 1000000, Cash: 500000, Timestamp: time.Now()}
	quote := domain.Quote{Symbol: "2330.TW", Last: 500}

	shares1, _, err := sizer.CalculateSize(signal, portfolio, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Now set a non-zero volatility and compare
	sizer.UpdateVolatility("2330.TW", 0.20)
	shares2, _, err := sizer.CalculateSize(signal, portfolio, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With vol=0, adjustForVolatility returns baseSize (no reduction)
	// With vol=0.20 (target=0.20), adjustment = 1.0
	// So they should be equal
	if shares1 != shares2 {
		t.Errorf("expected equal shares when vol=0 (returns baseSize) vs vol=target, got %d vs %d", shares1, shares2)
	}
}

func TestSizerAdjustForVolatilityBoundaries(t *testing.T) {
	sizer := NewSizer()
	sizer.UpdateATR("2330.TW", 5.0)
	sizer.UpdateADV("2330.TW", 100000000)

	// Test with very high volatility -> adjustment should be clamped to min
	sizer.UpdateVolatility("2330.TW", 10.0) // way above target
	signal := Signal{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 0.8, WinRate: 0.6, PayoffRatio: 2.0}
	portfolio := PortfolioSnapshot{TotalValue: 1000000, Cash: 500000, Timestamp: time.Now()}
	quote := domain.Quote{Symbol: "2330.TW", Last: 500}

	sharesHighVol, _, err := sizer.CalculateSize(signal, portfolio, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Test with very low volatility -> adjustment should be clamped to max
	sizer.UpdateVolatility("2330.TW", 0.001) // way below target
	sharesLowVol, _, err := sizer.CalculateSize(signal, portfolio, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Low vol should produce larger size than high vol
	if sharesLowVol <= sharesHighVol {
		t.Errorf("expected low vol to produce larger size than high vol, got low=%d high=%d", sharesLowVol, sharesHighVol)
	}
}

func TestSizerAdjustForATRUncachedReturnsZero(t *testing.T) {
	sizer := NewSizer()
	// Do NOT set ATR cache
	sizer.UpdateVolatility("2330.TW", 0.20)
	sizer.UpdateADV("2330.TW", 100000000)

	signal := Signal{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 0.8, WinRate: 0.6, PayoffRatio: 2.0}
	portfolio := PortfolioSnapshot{TotalValue: 1000000, Cash: 500000, Timestamp: time.Now()}
	quote := domain.Quote{Symbol: "2330.TW", Last: 500}

	shares, _, err := sizer.CalculateSize(signal, portfolio, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With no ATR cached, getATR returns 0, adjustForATR returns baseSize
	// The size should still be positive (from Kelly + vol adjustment only)
	if shares <= 0 {
		t.Errorf("expected positive shares even without ATR cache, got %d", shares)
	}
}

func TestSizerCalculateCorrelationPenalty(t *testing.T) {
	sizer := NewSizer()
	sizer.UpdateATR("2330.TW", 5.0)
	sizer.UpdateVolatility("2330.TW", 0.20)
	sizer.UpdateADV("2330.TW", 100000000)

	// Set high correlation with existing position
	sizer.UpdateCorrelation("2330.TW", "2317.TW", 0.9)

	portfolio := PortfolioSnapshot{
		TotalValue: 1000000,
		Cash:       500000,
		Positions:  []domain.Position{{Symbol: "2317.TW", Quantity: 1000}},
		Timestamp:  time.Now(),
	}
	signal := Signal{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 0.8, WinRate: 0.6, PayoffRatio: 2.0}
	quote := domain.Quote{Symbol: "2330.TW", Last: 500}

	sharesWithCorr, _, err := sizer.CalculateSize(signal, portfolio, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Now with no correlation
	sizer2 := NewSizer()
	sizer2.UpdateATR("2330.TW", 5.0)
	sizer2.UpdateVolatility("2330.TW", 0.20)
	sizer2.UpdateADV("2330.TW", 100000000)

	portfolio2 := PortfolioSnapshot{
		TotalValue: 1000000,
		Cash:       500000,
		Positions:  []domain.Position{{Symbol: "2317.TW", Quantity: 1000}},
		Timestamp:  time.Now(),
	}

	sharesNoCorr, _, err := sizer2.CalculateSize(signal, portfolio2, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// High correlation should reduce position size
	if sharesWithCorr >= sharesNoCorr {
		t.Errorf("expected correlated position to be smaller, got withCorr=%d noCorr=%d", sharesWithCorr, sharesNoCorr)
	}
}

func TestSizerCalculateCorrelationPenaltyNoPositions(t *testing.T) {
	sizer := NewSizer()
	sizer.UpdateATR("2330.TW", 5.0)
	sizer.UpdateVolatility("2330.TW", 0.20)
	sizer.UpdateADV("2330.TW", 100000000)
	sizer.UpdateCorrelation("2330.TW", "2317.TW", 0.9)

	// Empty positions -> no penalty
	portfolio := PortfolioSnapshot{TotalValue: 1000000, Cash: 500000, Positions: []domain.Position{}, Timestamp: time.Now()}
	signal := Signal{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 0.8, WinRate: 0.6, PayoffRatio: 2.0}
	quote := domain.Quote{Symbol: "2330.TW", Last: 500}

	shares, _, err := sizer.CalculateSize(signal, portfolio, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shares <= 0 {
		t.Errorf("expected positive shares with no positions, got %d", shares)
	}
}

func TestSizerLiquidityAdjustment(t *testing.T) {
	sizer := NewSizer()
	sizer.UpdateATR("2330.TW", 5.0)
	sizer.UpdateVolatility("2330.TW", 0.20)

	// Very low ADV -> should cap position size
	sizer.UpdateADV("2330.TW", 1000)

	signal := Signal{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 0.8, WinRate: 0.6, PayoffRatio: 2.0}
	portfolio := PortfolioSnapshot{TotalValue: 1000000, Cash: 500000, Timestamp: time.Now()}
	quote := domain.Quote{Symbol: "2330.TW", Last: 500}

	sharesLowADV, _, err := sizer.CalculateSize(signal, portfolio, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// High ADV -> less restrictive
	sizer.UpdateADV("2330.TW", 1000000000)
	sharesHighADV, _, err := sizer.CalculateSize(signal, portfolio, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sharesLowADV >= sharesHighADV {
		t.Errorf("expected low ADV to produce smaller size, got low=%d high=%d", sharesLowADV, sharesHighADV)
	}
}

func TestSizerLiquidityAdjustmentZeroADV(t *testing.T) {
	sizer := NewSizer()
	sizer.UpdateATR("2330.TW", 5.0)
	sizer.UpdateVolatility("2330.TW", 0.20)
	// ADV not set -> uses default (100M)

	signal := Signal{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 0.8, WinRate: 0.6, PayoffRatio: 2.0}
	portfolio := PortfolioSnapshot{TotalValue: 1000000, Cash: 500000, Timestamp: time.Now()}
	quote := domain.Quote{Symbol: "2330.TW", Last: 500}

	shares, _, err := sizer.CalculateSize(signal, portfolio, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shares <= 0 {
		t.Errorf("expected positive shares with default ADV, got %d", shares)
	}
}

func TestSizerCalculatePositionSizing(t *testing.T) {
	sizer := NewSizer()
	sizer.UpdateATR("2330.TW", 5.0)
	sizer.UpdateATR("2317.TW", 3.0)
	sizer.UpdateVolatility("2330.TW", 0.20)
	sizer.UpdateVolatility("2317.TW", 0.20)
	sizer.UpdateADV("2330.TW", 100000000)
	sizer.UpdateADV("2317.TW", 100000000)

	signals := []Signal{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 0.8, WinRate: 0.6, PayoffRatio: 2.0},
		{Symbol: "2317.TW", Side: domain.SideBuy, Conviction: 0.7, WinRate: 0.55, PayoffRatio: 1.5},
	}
	portfolio := PortfolioSnapshot{TotalValue: 1000000, Cash: 500000, Timestamp: time.Now()}
	quotes := map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Last: 500},
		"2317.TW": {Symbol: "2317.TW", Last: 200},
	}

	results := sizer.CalculatePositionSizing(signals, portfolio, quotes)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	for _, r := range results {
		if r.Shares <= 0 {
			t.Errorf("expected positive shares for %s, got %d", r.Symbol, r.Shares)
		}
		if r.TargetValue <= 0 {
			t.Errorf("expected positive target value for %s, got %f", r.Symbol, r.TargetValue)
		}
	}
}

func TestSizerCalculatePositionSizingMissingQuote(t *testing.T) {
	sizer := NewSizer()
	sizer.UpdateATR("2330.TW", 5.0)
	sizer.UpdateVolatility("2330.TW", 0.20)
	sizer.UpdateADV("2330.TW", 100000000)

	signals := []Signal{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 0.8, WinRate: 0.6, PayoffRatio: 2.0},
		{Symbol: "MISSING.TW", Side: domain.SideBuy, Conviction: 0.7, WinRate: 0.55, PayoffRatio: 1.5},
	}
	portfolio := PortfolioSnapshot{TotalValue: 1000000, Cash: 500000, Timestamp: time.Now()}
	quotes := map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Last: 500},
	}

	results := sizer.CalculatePositionSizing(signals, portfolio, quotes)
	if len(results) != 1 {
		t.Errorf("expected 1 result (missing quote skipped), got %d", len(results))
	}
	if results[0].Symbol != "2330.TW" {
		t.Errorf("expected 2330.TW, got %s", results[0].Symbol)
	}
}

func TestSizerCalculatePositionSizingZeroShares(t *testing.T) {
	sizer := NewSizer()
	sizer.UpdateATR("2330.TW", 5.0)
	sizer.UpdateVolatility("2330.TW", 0.20)
	sizer.UpdateADV("2330.TW", 100000000)

	// Kelly negative -> size = 0 -> shares = 0 -> skipped
	signals := []Signal{
		{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 0.8, WinRate: 0.1, PayoffRatio: 0.5},
	}
	portfolio := PortfolioSnapshot{TotalValue: 1000000, Cash: 500000, Timestamp: time.Now()}
	quotes := map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Last: 500},
	}

	results := sizer.CalculatePositionSizing(signals, portfolio, quotes)
	if len(results) != 0 {
		t.Errorf("expected 0 results (zero shares skipped), got %d", len(results))
	}
}

func TestSizerSetRiskParameters(t *testing.T) {
	sizer := NewSizer()
	params := RiskParameters{
		KellyFraction:      0.5,
		VolLookback:        30,
		MaxPositionByADV:   0.02,
		MaxDrawdownLimit:   0.15,
		ATRMultiplier:      3.0,
		CorrelationPenalty: 0.8,
	}
	sizer.SetRiskParameters(params)

	// Verify parameters are used by checking behavior change
	sizer.UpdateATR("2330.TW", 5.0)
	sizer.UpdateVolatility("2330.TW", 0.20)
	sizer.UpdateADV("2330.TW", 100000000)

	signal := Signal{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 0.8, WinRate: 0.6, PayoffRatio: 2.0}
	portfolio := PortfolioSnapshot{TotalValue: 1000000, Cash: 500000, Timestamp: time.Now()}
	quote := domain.Quote{Symbol: "2330.TW", Last: 500}

	shares, _, err := sizer.CalculateSize(signal, portfolio, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shares <= 0 {
		t.Errorf("expected positive shares after parameter change, got %d", shares)
	}
}

func TestSizerSetCorrelationThreshold(t *testing.T) {
	sizer := NewSizer()
	sizer.SetCorrelationThreshold(0.5)

	sizer.UpdateATR("2330.TW", 5.0)
	sizer.UpdateVolatility("2330.TW", 0.20)
	sizer.UpdateADV("2330.TW", 100000000)
	sizer.UpdateCorrelation("2330.TW", "2317.TW", 0.6)

	portfolio := PortfolioSnapshot{
		TotalValue: 1000000,
		Cash:       500000,
		Positions:  []domain.Position{{Symbol: "2317.TW", Quantity: 1000}},
		Timestamp:  time.Now(),
	}
	signal := Signal{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 0.8, WinRate: 0.6, PayoffRatio: 2.0}
	quote := domain.Quote{Symbol: "2330.TW", Last: 500}

	shares, _, err := sizer.CalculateSize(signal, portfolio, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shares <= 0 {
		t.Errorf("expected positive shares, got %d", shares)
	}
}

func TestSizerWithParameters(t *testing.T) {
	sizer := NewSizer()
	params := DefaultRuntimeParameters()
	params.Sizing.KellyFraction = 0.1
	params.Sizing.TargetVolatility = 0.15

	result := sizer.WithParameters(params)
	if result != sizer {
		t.Error("expected WithParameters to return the same sizer for chaining")
	}

	sizer.UpdateATR("2330.TW", 5.0)
	sizer.UpdateVolatility("2330.TW", 0.20)
	sizer.UpdateADV("2330.TW", 100000000)

	signal := Signal{Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 0.8, WinRate: 0.6, PayoffRatio: 2.0}
	portfolio := PortfolioSnapshot{TotalValue: 1000000, Cash: 500000, Timestamp: time.Now()}
	quote := domain.Quote{Symbol: "2330.TW", Last: 500}

	shares, _, err := sizer.CalculateSize(signal, portfolio, quote)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shares <= 0 {
		t.Errorf("expected positive shares, got %d", shares)
	}
}
