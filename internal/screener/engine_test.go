package screener

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

//go:fix inline
func ptrFloat64(f float64) *float64 {
	return new(f)
}

//go:fix inline
func ptrInt64(i int64) *int64 {
	return new(i)
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
		PB: &domain.RangeFilter{Max: new(1.5)},
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
		DividendYield: &domain.RangeFilter{Min: new(2.0)},
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
		Momentum20Day: &domain.RangeFilter{Min: new(0.0)},
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
		MinTotalFactorScore: new(0.2),
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

func TestScreenDetailedVolumeFail(t *testing.T) {
	fe := portfolio.NewFactorEngine()
	fp := portfolio.NewFundamentalProvider()
	e := NewEngine(fe, fp)
	minVol := int64(1000000)
	criteria := domain.ScreeningCriteria{
		VolumeIntraday: &domain.MinFilter{Min: &minVol},
	}
	quotes := map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Volume: 500000, IsTradable: true},
	}
	res, err := e.ScreenDetailed(context.Background(), "2330.TW", criteria, quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected fail")
	}
	if res.Criterion == "" {
		t.Fatal("expected criterion to be set")
	}
	if res.Criterion != "volume_intraday_min" {
		t.Errorf("expected criterion 'volume_intraday_min', got %q", res.Criterion)
	}
	if res.Label == "" {
		t.Fatal("expected label to be set")
	}
	if res.Threshold == "" {
		t.Fatal("expected threshold to be set")
	}
	if res.Actual == "" {
		t.Fatal("expected actual to be set")
	}
}

func TestScreenDetailedPEMinFail(t *testing.T) {
	fp := loadTestFundamentals(t, map[string]portfolio.FundamentalData{
		"LOW_PE.TW": {PE: 5},
	})
	e := NewEngine(nil, fp)
	quotes := map[string]domain.Quote{}
	criteria := domain.ScreeningCriteria{
		PE: &domain.RangeFilter{Min: ptrFloat64(10)},
	}
	res, err := e.ScreenDetailed(context.Background(), "LOW_PE.TW", criteria, quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected fail")
	}
	if res.Criterion != "pe_min" {
		t.Errorf("expected criterion 'pe_min', got %q", res.Criterion)
	}
	if res.Label != "P/E" {
		t.Errorf("expected label 'P/E', got %q", res.Label)
	}
	if res.Threshold == "" {
		t.Fatal("expected threshold to be set")
	}
	if res.Actual == "" {
		t.Fatal("expected actual to be set")
	}
}

func TestScreenDetailedPEMaxFail(t *testing.T) {
	fp := loadTestFundamentals(t, map[string]portfolio.FundamentalData{
		"HIGH_PE.TW": {PE: 30},
	})
	e := NewEngine(nil, fp)
	quotes := map[string]domain.Quote{}
	criteria := domain.ScreeningCriteria{
		PE: &domain.RangeFilter{Max: ptrFloat64(20)},
	}
	res, err := e.ScreenDetailed(context.Background(), "HIGH_PE.TW", criteria, quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected fail")
	}
	if res.Criterion != "pe_max" {
		t.Errorf("expected criterion 'pe_max', got %q", res.Criterion)
	}
	if res.Label != "P/E" {
		t.Errorf("expected label 'P/E', got %q", res.Label)
	}
	if res.Threshold == "" {
		t.Fatal("expected threshold to be set")
	}
	if res.Actual == "" {
		t.Fatal("expected actual to be set")
	}
}

func TestScreenDetailedPBMissingFail(t *testing.T) {
	fp := loadTestFundamentals(t, map[string]portfolio.FundamentalData{
		"OTHER.TW": {PE: 15},
	})
	e := NewEngine(nil, fp)
	quotes := map[string]domain.Quote{}
	criteria := domain.ScreeningCriteria{
		PB: &domain.RangeFilter{Max: new(2.0)},
	}
	res, err := e.ScreenDetailed(context.Background(), "NO_DATA.TW", criteria, quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected fail for symbol not in fundamentals")
	}
	if res.Criterion != "pb_missing" {
		t.Errorf("expected criterion 'pb_missing', got %q", res.Criterion)
	}
	if res.Label != "P/B" {
		t.Errorf("expected label 'P/B', got %q", res.Label)
	}
	if res.Threshold != "required" {
		t.Errorf("expected threshold 'required', got %q", res.Threshold)
	}
	if res.Actual != "missing data" {
		t.Errorf("expected actual 'missing data', got %q", res.Actual)
	}
}

func TestScreenDetailedMomentum20DMaxFail(t *testing.T) {
	fe := portfolio.NewFactorEngine()
	e := NewEngine(fe, nil)
	quotes := map[string]domain.Quote{
		"SKY_HIGH.TW": {Symbol: "SKY_HIGH.TW", Open: 100, Last: 200, IsTradable: true},
	}
	criteria := domain.ScreeningCriteria{
		Momentum20Day: &domain.RangeFilter{Max: new(0.5)},
	}
	res, err := e.ScreenDetailed(context.Background(), "SKY_HIGH.TW", criteria, quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected fail")
	}
	if res.Criterion != "momentum_20d_max" {
		t.Errorf("expected criterion 'momentum_20d_max', got %q", res.Criterion)
	}
	if res.Label != "20-day momentum" {
		t.Errorf("expected label '20-day momentum', got %q", res.Label)
	}
	if res.Threshold == "" {
		t.Fatal("expected threshold to be set")
	}
	if res.Actual == "" {
		t.Fatal("expected actual to be set")
	}
}

func TestScreenDetailedMinTotalFactorScoreMissing(t *testing.T) {
	fe := portfolio.NewFactorEngine()
	e := NewEngine(fe, nil)
	quotes := map[string]domain.Quote{
		"NO_SCORE.TW": {Symbol: "NO_SCORE.TW", Open: 100, Last: 100, IsTradable: true},
	}
	criteria := domain.ScreeningCriteria{
		MinTotalFactorScore: new(0.5),
	}
	res, err := e.ScreenDetailed(context.Background(), "NO_SCORE.TW", criteria, quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected fail for missing total score")
	}
	if res.Criterion != "min_total_factor_score" {
		t.Errorf("expected criterion 'min_total_factor_score', got %q", res.Criterion)
	}
	if res.Label != "Total factor score" {
		t.Errorf("expected label 'Total factor score', got %q", res.Label)
	}
	if res.Threshold == "" {
		t.Fatal("expected threshold to be set")
	}
	if res.Actual == "" {
		t.Fatal("expected actual to be set")
	}
}

// mockTraceWriter records trace calls for test assertions.
type mockTraceWriter struct {
	records []traceRecord
}

type traceRecord struct {
	step   int
	layer  string
	status string
	meta   map[string]any
}

func (m *mockTraceWriter) Record(step int, layer, status string, meta map[string]any) {
	m.records = append(m.records, traceRecord{step: step, layer: layer, status: status, meta: meta})
}

func TestEngine_WithTraceWriter(t *testing.T) {
	engine := NewEngine(nil, nil)
	if engine.traceWriter != nil {
		t.Error("traceWriter should be nil by default")
	}
	m := &mockTraceWriter{}
	got := engine.WithTraceWriter(m)
	if got != engine {
		t.Error("WithTraceWriter should return the same Engine for chaining")
	}
	if engine.traceWriter != m {
		t.Error("WithTraceWriter should set the traceWriter")
	}
}

func TestScreenUniverse_AllRejected_EmitsTrace(t *testing.T) {
	fp := loadTestFundamentals(t, map[string]portfolio.FundamentalData{
		"A.TW": {PE: 10},
	})
	engine := NewEngine(nil, fp)
	m := &mockTraceWriter{}
	engine.WithTraceWriter(m)
	quotes := map[string]domain.Quote{}
	criteria := domain.ScreeningCriteria{
		PE: &domain.RangeFilter{Max: ptrFloat64(15)},
	}

	passed, err := engine.ScreenUniverse(context.Background(), []string{"A.TW"}, criteria, quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(passed) != 1 {
		t.Fatal("A.TW should pass PE filter")
	}
	if len(m.records) != 0 {
		t.Error("no trace should be emitted when some symbols pass")
	}

	m2 := &mockTraceWriter{}
	engine.WithTraceWriter(m2)
	criteria2 := domain.ScreeningCriteria{
		PE: &domain.RangeFilter{Max: ptrFloat64(5)},
	}
	passed2, err2 := engine.ScreenUniverse(context.Background(), []string{"A.TW", "B.TW"}, criteria2, quotes)
	if err2 != nil {
		t.Fatalf("unexpected error: %v", err2)
	}
	if len(passed2) != 0 {
		t.Fatal("expected all symbols to be rejected")
	}
	if len(m2.records) != 1 {
		t.Fatalf("expected 1 trace record, got %d", len(m2.records))
	}
	tr := m2.records[0]
	if tr.status != "WARN" {
		t.Errorf("trace status = %q, want WARN", tr.status)
	}
	if tr.layer != "screener" {
		t.Errorf("trace layer = %q, want screener", tr.layer)
	}
	if tr.meta["rejected"].(int) != 2 {
		t.Errorf("expected 2 rejected in trace, got %v", tr.meta["rejected"])
	}
}

func TestScreenDetailed_DividendYieldMinFail(t *testing.T) {
	fp := loadTestFundamentals(t, map[string]portfolio.FundamentalData{
		"LOW_DIV.TW": {DividendYield: 1.0},
	})
	e := NewEngine(nil, fp)
	quotes := map[string]domain.Quote{}
	criteria := domain.ScreeningCriteria{
		DividendYield: &domain.RangeFilter{Min: new(3.0)},
	}
	res, err := e.ScreenDetailed(context.Background(), "LOW_DIV.TW", criteria, quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected fail")
	}
	if res.Criterion != "dividend_yield_min" {
		t.Errorf("expected criterion 'dividend_yield_min', got %q", res.Criterion)
	}
	if res.Label != "Dividend yield" {
		t.Errorf("expected label 'Dividend yield', got %q", res.Label)
	}
	if res.Threshold == "" {
		t.Fatal("expected threshold to be set")
	}
	if res.Actual == "" {
		t.Fatal("expected actual to be set")
	}
}

func TestScreenDetailed_DividendYieldMaxFail(t *testing.T) {
	fp := loadTestFundamentals(t, map[string]portfolio.FundamentalData{
		"HIGH_DIV.TW": {DividendYield: 8.0},
	})
	e := NewEngine(nil, fp)
	quotes := map[string]domain.Quote{}
	criteria := domain.ScreeningCriteria{
		DividendYield: &domain.RangeFilter{Max: new(5.0)},
	}
	res, err := e.ScreenDetailed(context.Background(), "HIGH_DIV.TW", criteria, quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected fail")
	}
	if res.Criterion != "dividend_yield_max" {
		t.Errorf("expected criterion 'dividend_yield_max', got %q", res.Criterion)
	}
	if res.Label != "Dividend yield" {
		t.Errorf("expected label 'Dividend yield', got %q", res.Label)
	}
}

func TestScreenDetailed_DividendYieldMissing(t *testing.T) {
	fp := loadTestFundamentals(t, map[string]portfolio.FundamentalData{
		"OTHER.TW": {PE: 15},
	})
	e := NewEngine(nil, fp)
	quotes := map[string]domain.Quote{}
	criteria := domain.ScreeningCriteria{
		DividendYield: &domain.RangeFilter{Min: new(2.0)},
	}
	res, err := e.ScreenDetailed(context.Background(), "NO_DATA.TW", criteria, quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected fail for missing dividend yield")
	}
	if res.Criterion != "dividend_yield_missing" {
		t.Errorf("expected criterion 'dividend_yield_missing', got %q", res.Criterion)
	}
	if res.Label != "Dividend yield" {
		t.Errorf("expected label 'Dividend yield', got %q", res.Label)
	}
	if res.Threshold != "required" {
		t.Errorf("expected threshold 'required', got %q", res.Threshold)
	}
	if res.Actual != "missing data" {
		t.Errorf("expected actual 'missing data', got %q", res.Actual)
	}
}

func TestScreenDetailed_PEWithMissingFundamentals(t *testing.T) {
	e := NewEngine(nil, nil)
	quotes := map[string]domain.Quote{}
	criteria := domain.ScreeningCriteria{
		PE: &domain.RangeFilter{Max: ptrFloat64(15)},
	}
	res, err := e.ScreenDetailed(context.Background(), "ANY.TW", criteria, quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Passed {
		t.Fatal("expected pass: nil fundamentals skips PE filter")
	}
}

func TestScreenDetailed_VolumeMissingQuote(t *testing.T) {
	engine := NewEngine(nil, nil)
	quotes := map[string]domain.Quote{}
	minVol := int64(1000000)
	criteria := domain.ScreeningCriteria{
		VolumeIntraday: &domain.MinFilter{Min: &minVol},
	}
	res, err := engine.ScreenDetailed(context.Background(), "MISSING.TW", criteria, quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected fail for missing quote")
	}
	if res.Criterion != "volume_intraday_min" {
		t.Errorf("expected criterion 'volume_intraday_min', got %q", res.Criterion)
	}
	if res.Actual != "missing quote" {
		t.Errorf("expected actual 'missing quote', got %q", res.Actual)
	}
}

func TestScreenDetailed_PBMinFail(t *testing.T) {
	fp := loadTestFundamentals(t, map[string]portfolio.FundamentalData{
		"LOW_PB.TW": {PB: 0.5},
	})
	e := NewEngine(nil, fp)
	quotes := map[string]domain.Quote{}
	criteria := domain.ScreeningCriteria{
		PB: &domain.RangeFilter{Min: new(1.0)},
	}
	res, err := e.ScreenDetailed(context.Background(), "LOW_PB.TW", criteria, quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected fail")
	}
	if res.Criterion != "pb_min" {
		t.Errorf("expected criterion 'pb_min', got %q", res.Criterion)
	}
	if res.Label != "P/B" {
		t.Errorf("expected label 'P/B', got %q", res.Label)
	}
}

func TestScreenDetailed_PBMaxFail(t *testing.T) {
	fp := loadTestFundamentals(t, map[string]portfolio.FundamentalData{
		"HIGH_PB.TW": {PB: 5.0},
	})
	e := NewEngine(nil, fp)
	quotes := map[string]domain.Quote{}
	criteria := domain.ScreeningCriteria{
		PB: &domain.RangeFilter{Max: new(3.0)},
	}
	res, err := e.ScreenDetailed(context.Background(), "HIGH_PB.TW", criteria, quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected fail")
	}
	if res.Criterion != "pb_max" {
		t.Errorf("expected criterion 'pb_max', got %q", res.Criterion)
	}
	if res.Label != "P/B" {
		t.Errorf("expected label 'P/B', got %q", res.Label)
	}
}

func TestScreenDetailed_MinTotalFactorScoreBelowThreshold(t *testing.T) {
	fe := portfolio.NewFactorEngine()
	e := NewEngine(fe, nil)
	quotes := map[string]domain.Quote{
		"FLAT.TW": {Symbol: "FLAT.TW", Open: 100, Last: 100, IsTradable: true},
	}
	criteria := domain.ScreeningCriteria{
		MinTotalFactorScore: new(0.9),
	}
	res, err := e.ScreenDetailed(context.Background(), "FLAT.TW", criteria, quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected fail for score below threshold")
	}
	if res.Criterion != "min_total_factor_score" {
		t.Errorf("expected criterion 'min_total_factor_score', got %q", res.Criterion)
	}
	if res.Actual == "" || res.Actual == "missing" {
		t.Error("expected numeric actual value, not missing")
	}
}

func TestScreenDetailed_Momentum20DMinFail(t *testing.T) {
	fe := portfolio.NewFactorEngine()
	e := NewEngine(fe, nil)
	quotes := map[string]domain.Quote{
		"DOWN.TW": {Symbol: "DOWN.TW", Open: 100, Last: 90, IsTradable: true},
	}
	criteria := domain.ScreeningCriteria{
		Momentum20Day: &domain.RangeFilter{Min: new(0.05)},
	}
	res, err := e.ScreenDetailed(context.Background(), "DOWN.TW", criteria, quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected fail for momentum below min")
	}
	if res.Criterion != "momentum_20d_min" {
		t.Errorf("expected criterion 'momentum_20d_min', got %q", res.Criterion)
	}
	if res.Label != "20-day momentum" {
		t.Errorf("expected label '20-day momentum', got %q", res.Label)
	}
}
