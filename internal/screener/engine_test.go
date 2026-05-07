package screener

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

func ptrFloat64(f float64) *float64 {
	return &f
}

func ptrInt64(i int64) *int64 {
	return &i
}

func loadTestFundamentals(t *testing.T, data map[string]portfolio.FundamentalData) *portfolio.FundamentalProvider {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "fundamentals-*.json")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(data); err != nil {
		t.Fatalf("encode fundamentals: %v", err)
	}
	fp := portfolio.NewFundamentalProvider()
	if err := fp.LoadFromJSON(f.Name()); err != nil {
		t.Fatalf("load fundamentals: %v", err)
	}
	return fp
}

func TestScreenPassesWithNilCriteria(t *testing.T) {
	engine := NewEngine(nil, nil)
	quotes := map[string]domain.Quote{}
	passed, err := engine.Screen(context.Background(), "2330.TW", domain.ScreeningCriteria{}, quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !passed {
		t.Error("expected pass when criteria has no filters")
	}
}

func TestScreenFiltersByPE(t *testing.T) {
	fp := loadTestFundamentals(t, map[string]portfolio.FundamentalData{
		"CHEAP.TW":     {PE: 8, PB: 1.0},
		"EXPENSIVE.TW": {PE: 25, PB: 2.0},
	})

	engine := NewEngine(nil, fp)
	quotes := map[string]domain.Quote{}
	criteria := domain.ScreeningCriteria{
		PE: &domain.RangeFilter{Max: ptrFloat64(15)},
	}

	cheapPassed, _ := engine.Screen(context.Background(), "CHEAP.TW", criteria, quotes)
	expensivePassed, _ := engine.Screen(context.Background(), "EXPENSIVE.TW", criteria, quotes)

	if !cheapPassed {
		t.Error("expected CHEAP.TW to pass PE screen")
	}
	if expensivePassed {
		t.Error("expected EXPENSIVE.TW to fail PE screen")
	}
}

func TestScreenFiltersByPB(t *testing.T) {
	fp := loadTestFundamentals(t, map[string]portfolio.FundamentalData{
		"LOW_PB.TW":  {PB: 0.8},
		"HIGH_PB.TW": {PB: 3.5},
	})

	engine := NewEngine(nil, fp)
	quotes := map[string]domain.Quote{}
	criteria := domain.ScreeningCriteria{
		PB: &domain.RangeFilter{Max: ptrFloat64(1.5)},
	}

	lowPassed, _ := engine.Screen(context.Background(), "LOW_PB.TW", criteria, quotes)
	highPassed, _ := engine.Screen(context.Background(), "HIGH_PB.TW", criteria, quotes)

	if !lowPassed {
		t.Error("expected LOW_PB.TW to pass PB screen")
	}
	if highPassed {
		t.Error("expected HIGH_PB.TW to fail PB screen")
	}
}

func TestScreenFiltersByDividendYield(t *testing.T) {
	fp := loadTestFundamentals(t, map[string]portfolio.FundamentalData{
		"HIGH_DIV.TW": {DividendYield: 4.0},
		"LOW_DIV.TW":  {DividendYield: 0.5},
	})

	engine := NewEngine(nil, fp)
	quotes := map[string]domain.Quote{}
	criteria := domain.ScreeningCriteria{
		DividendYield: &domain.RangeFilter{Min: ptrFloat64(2.0)},
	}

	highPassed, _ := engine.Screen(context.Background(), "HIGH_DIV.TW", criteria, quotes)
	lowPassed, _ := engine.Screen(context.Background(), "LOW_DIV.TW", criteria, quotes)

	if !highPassed {
		t.Error("expected HIGH_DIV.TW to pass dividend yield screen")
	}
	if lowPassed {
		t.Error("expected LOW_DIV.TW to fail dividend yield screen")
	}
}

func TestScreenFiltersByVolume(t *testing.T) {
	engine := NewEngine(nil, nil)
	quotes := map[string]domain.Quote{
		"LIQUID.TW":   {Symbol: "LIQUID.TW", Volume: 2000000, IsTradable: true},
		"ILLIQUID.TW": {Symbol: "ILLIQUID.TW", Volume: 500000, IsTradable: true},
	}
	criteria := domain.ScreeningCriteria{
		VolumeIntraday: &domain.MinFilter{Min: ptrInt64(1000000)},
	}

	liquidPassed, _ := engine.Screen(context.Background(), "LIQUID.TW", criteria, quotes)
	illiquidPassed, _ := engine.Screen(context.Background(), "ILLIQUID.TW", criteria, quotes)

	if !liquidPassed {
		t.Error("expected LIQUID.TW to pass volume screen")
	}
	if illiquidPassed {
		t.Error("expected ILLIQUID.TW to fail volume screen")
	}
}

func TestScreenFiltersByMomentum(t *testing.T) {
	fe := portfolio.NewFactorEngine()
	engine := NewEngine(fe, nil)
	quotes := map[string]domain.Quote{
		"UP.TW":   {Symbol: "UP.TW", Open: 100, Last: 110, IsTradable: true},
		"DOWN.TW": {Symbol: "DOWN.TW", Open: 100, Last: 90, IsTradable: true},
	}
	criteria := domain.ScreeningCriteria{
		Momentum20Day: &domain.RangeFilter{Min: ptrFloat64(0.0)},
	}

	upPassed, _ := engine.Screen(context.Background(), "UP.TW", criteria, quotes)
	downPassed, _ := engine.Screen(context.Background(), "DOWN.TW", criteria, quotes)

	if !upPassed {
		t.Error("expected UP.TW to pass momentum screen")
	}
	if downPassed {
		t.Error("expected DOWN.TW to fail momentum screen")
	}
}

func TestScreenFiltersByMinTotalFactorScore(t *testing.T) {
	fe := portfolio.NewFactorEngine()
	engine := NewEngine(fe, nil)
	quotes := map[string]domain.Quote{
		"STRONG.TW": {Symbol: "STRONG.TW", Open: 100, Last: 130, IsTradable: true},
	}
	criteria := domain.ScreeningCriteria{
		MinTotalFactorScore: ptrFloat64(0.2),
	}

	passed, err := engine.Screen(context.Background(), "STRONG.TW", criteria, quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !passed {
		t.Error("expected STRONG.TW to pass min total factor score screen")
	}
}

func TestScreenUniverseFiltersList(t *testing.T) {
	fp := loadTestFundamentals(t, map[string]portfolio.FundamentalData{
		"A.TW": {PE: 10},
		"B.TW": {PE: 30},
		"C.TW": {PE: 8},
	})

	engine := NewEngine(nil, fp)
	quotes := map[string]domain.Quote{}
	criteria := domain.ScreeningCriteria{
		PE: &domain.RangeFilter{Max: ptrFloat64(15)},
	}

	passed, err := engine.ScreenUniverse(context.Background(), []string{"A.TW", "B.TW", "C.TW"}, criteria, quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(passed) != 2 {
		t.Errorf("expected 2 symbols to pass, got %d: %v", len(passed), passed)
	}
}

func TestScreenMissingFundamentalRejectsWhenFilterRequired(t *testing.T) {
	fp := loadTestFundamentals(t, map[string]portfolio.FundamentalData{
		"OTHER.TW": {PE: 10, PB: 1.0},
	})
	engine := NewEngine(nil, fp)
	quotes := map[string]domain.Quote{}
	criteria := domain.ScreeningCriteria{
		PE: &domain.RangeFilter{Max: ptrFloat64(15)},
	}

	passed, _ := engine.Screen(context.Background(), "NO_DATA.TW", criteria, quotes)
	if passed {
		t.Error("expected rejection when symbol is missing from loaded fundamentals and PE filter is required")
	}
}
