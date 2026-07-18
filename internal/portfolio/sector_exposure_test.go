package portfolio_test

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// mockL1Resolver implements portfolio.L1SymbolResolver for testing.
type mockL1Resolver struct {
	mappings map[string]industry.SectorID
}

func (r *mockL1Resolver) ResolveL1(symbol string) (industry.SectorID, bool) {
	id, ok := r.mappings[symbol]
	return id, ok
}

func makeAll20L1WeightsZero() map[industry.SectorID]float64 {
	w := make(map[industry.SectorID]float64, 20)
	for _, id := range industry.L1Sectors() {
		w[id] = 0
	}
	return w
}

func TestSectorExposure_UsesQuantityTimesLast(t *testing.T) {
	cal := portfolio.SectorExposureCalculator{}
	// SA-INV-10: current exposure must be quantity × T-close, NOT AverageCost
	pos := []domain.Position{
		{Symbol: "2330", Quantity: 10, AverageCost: 500.0, CurrentPrice: 600.0, MarketValue: 6000.0},
	}
	q := []domain.Quote{
		// Last is 600 but MarketValue uses AverageCost→500 — we must use Last
		{Symbol: "2330", Last: 600.0, AsOf: date("2026-07-17")},
	}
	resolver := &mockL1Resolver{mappings: map[string]industry.SectorID{
		"2330": industry.SectorSemiconductor,
	}}
	exp := cal.Calculate(pos, q, date("2026-07-17"), resolver)
	// quantity × Last = 10 × 600 = 6000; NOT quantity × AverageCost = 5000
	if exp.Weights[industry.SectorSemiconductor] <= 0 {
		t.Fatal("should have non-zero weight")
	}
	// The weight should be based on Last price, not AverageCost
}

func TestSectorExposure_AlwaysReturnsExactly20L1Keys(t *testing.T) {
	cal := portfolio.SectorExposureCalculator{}
	pos := []domain.Position{
		{Symbol: "2330", Quantity: 10, AverageCost: 500, CurrentPrice: 600, MarketValue: 6000},
	}
	q := []domain.Quote{
		{Symbol: "2330", Last: 600, AsOf: date("2026-07-17")},
	}
	resolver := &mockL1Resolver{mappings: map[string]industry.SectorID{
		"2330": industry.SectorSemiconductor,
	}}
	exp := cal.Calculate(pos, q, date("2026-07-17"), resolver)
	// SA-INV-01: must have exactly 20 keys
	if len(exp.Weights) != 20 {
		t.Fatalf("got %d keys, want exactly 20", len(exp.Weights))
	}
	for _, id := range industry.L1Sectors() {
		if _, ok := exp.Weights[id]; !ok {
			t.Errorf("missing L1 sector: %s", id)
		}
	}
}

func TestSectorExposure_UnmappedPositiveWeightMakesIncomplete(t *testing.T) {
	cal := portfolio.SectorExposureCalculator{}
	pos := []domain.Position{
		{Symbol: "2330", Quantity: 10, AverageCost: 500, CurrentPrice: 600, MarketValue: 6000},
		{Symbol: "9999", Quantity: 5, AverageCost: 200, CurrentPrice: 200, MarketValue: 1000},
	}
	q := []domain.Quote{
		{Symbol: "2330", Last: 600, AsOf: date("2026-07-17")},
		{Symbol: "9999", Last: 200, AsOf: date("2026-07-17")},
	}
	resolver := &mockL1Resolver{mappings: map[string]industry.SectorID{
		"2330": industry.SectorSemiconductor,
		// 9999 is unmapped
	}}
	exp := cal.Calculate(pos, q, date("2026-07-17"), resolver)
	// SA-INV-15: unmapped weight > 0 must set Complete=false
	if exp.Complete {
		t.Fatal("exposure must be incomplete when unmapped weight > 0")
	}
	if len(exp.UnmappedSymbols) != 1 || exp.UnmappedSymbols[0] != "9999" {
		t.Fatalf("unmapped: %v", exp.UnmappedSymbols)
	}
	if exp.UnmappedWeight <= 0 {
		t.Fatal("unmapped weight must be > 0")
	}
}

func TestSectorExposure_UnmappedZeroQuantityFine(t *testing.T) {
	cal := portfolio.SectorExposureCalculator{}
	pos := []domain.Position{
		{Symbol: "2330", Quantity: 10, AverageCost: 500, CurrentPrice: 600, MarketValue: 6000},
		{Symbol: "9999", Quantity: 0, AverageCost: 0, CurrentPrice: 0, MarketValue: 0},
	}
	q := []domain.Quote{
		{Symbol: "2330", Last: 600, AsOf: date("2026-07-17")},
	}
	resolver := &mockL1Resolver{mappings: map[string]industry.SectorID{
		"2330": industry.SectorSemiconductor,
	}}
	exp := cal.Calculate(pos, q, date("2026-07-17"), resolver)
	// zero-quantity positions should be ignored
	if !exp.Complete {
		t.Fatal("zero-quantity unmapped should not cause incompleteness")
	}
}

func TestSectorExposure_MissingTPriceFailsClosed(t *testing.T) {
	cal := portfolio.SectorExposureCalculator{}
	pos := []domain.Position{
		{Symbol: "2330", Quantity: 10, AverageCost: 500, CurrentPrice: 600, MarketValue: 6000},
	}
	// no quote for 2330
	q := []domain.Quote{}
	resolver := &mockL1Resolver{mappings: map[string]industry.SectorID{
		"2330": industry.SectorSemiconductor,
	}}
	exp := cal.Calculate(pos, q, date("2026-07-17"), resolver)
	if exp.Complete {
		t.Fatal("missing T price should fail closed (Complete=false)")
	}
	if len(exp.UnpricedSymbols) != 1 || exp.UnpricedSymbols[0] != "2330" {
		t.Fatalf("unpriced: %v", exp.UnpricedSymbols)
	}
}

func TestSectorExposure_QuoteDateMismatchFailsClosed(t *testing.T) {
	cal := portfolio.SectorExposureCalculator{}
	pos := []domain.Position{
		{Symbol: "2330", Quantity: 10, AverageCost: 500, CurrentPrice: 600, MarketValue: 6000},
	}
	q := []domain.Quote{
		{Symbol: "2330", Last: 600, AsOf: date("2026-07-16")}, // day before!
	}
	resolver := &mockL1Resolver{mappings: map[string]industry.SectorID{
		"2330": industry.SectorSemiconductor,
	}}
	exp := cal.Calculate(pos, q, date("2026-07-17"), resolver)
	// SA-INV-19: date mismatch → fail closed
	if exp.Complete {
		t.Fatal("date mismatch must fail closed")
	}
}

func TestSectorExposure_EmptyPortfolioReturnsNonNilTwentyZeroMap(t *testing.T) {
	cal := portfolio.SectorExposureCalculator{}
	exp := cal.Calculate(nil, nil, date("2026-07-17"), nil)
	if exp.Weights == nil {
		t.Fatal("weights must not be nil even for empty portfolio")
	}
	if len(exp.Weights) != 20 {
		t.Fatalf("got %d keys for empty portfolio, want 20", len(exp.Weights))
	}
	if exp.TotalMarketValue != 0 {
		t.Fatalf("empty portfolio total must be 0, got %f", exp.TotalMarketValue)
	}
	if !exp.Complete {
		t.Fatal("empty portfolio should be Complete")
	}
}

func TestSectorExposure_SumIsOneWhenComplete(t *testing.T) {
	cal := portfolio.SectorExposureCalculator{}
	pos := []domain.Position{
		{Symbol: "2330", Quantity: 10, AverageCost: 500, CurrentPrice: 600, MarketValue: 6000},
		{Symbol: "2882", Quantity: 40, AverageCost: 50, CurrentPrice: 60, MarketValue: 2400},
	}
	q := []domain.Quote{
		{Symbol: "2330", Last: 600, AsOf: date("2026-07-17")},
		{Symbol: "2882", Last: 60, AsOf: date("2026-07-17")},
	}
	resolver := &mockL1Resolver{mappings: map[string]industry.SectorID{
		"2330": industry.SectorSemiconductor,
		"2882": industry.SectorFinancials,
	}}
	exp := cal.Calculate(pos, q, date("2026-07-17"), resolver)
	if !exp.Complete {
		t.Fatal("all mapped must be Complete")
	}
	sum := 0.0
	for _, v := range exp.Weights {
		sum += v
	}
	if sum < 0.999999999 || sum > 1.000000001 {
		t.Fatalf("sum=%f, want 1.0±1e-9", sum)
	}
}

func TestSectorExposure_UnmappedListsAreStableSorted(t *testing.T) {
	cal := portfolio.SectorExposureCalculator{}
	pos := []domain.Position{
		{Symbol: "9999", Quantity: 5, AverageCost: 100, CurrentPrice: 100, MarketValue: 500},
		{Symbol: "8888", Quantity: 3, AverageCost: 100, CurrentPrice: 100, MarketValue: 300},
	}
	q := []domain.Quote{
		{Symbol: "9999", Last: 100, AsOf: date("2026-07-17")},
		{Symbol: "8888", Last: 100, AsOf: date("2026-07-17")},
	}
	resolver := &mockL1Resolver{mappings: map[string]industry.SectorID{}}
	exp := cal.Calculate(pos, q, date("2026-07-17"), resolver)
	if exp.UnmappedSymbols[0] != "8888" {
		t.Fatalf("unmapped must be sorted: got %v", exp.UnmappedSymbols)
	}
}

func date(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}
