package portfolio

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestOptimizerMomentumScore(t *testing.T) {
	o := NewOptimizer()
	quotes := map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Open: 500, Last: 550, IsTradable: true},
		"2317.TW": {Symbol: "2317.TW", Open: 200, Last: 180, IsTradable: true},
	}

	upScore := o.factorEngine.CalculateMomentumScore("2330.TW", quotes)
	if upScore <= 0 {
		t.Errorf("expected positive momentum for up-day, got %f", upScore)
	}

	downScore := o.factorEngine.CalculateMomentumScore("2317.TW", quotes)
	if downScore >= 0 {
		t.Errorf("expected negative momentum for down-day, got %f", downScore)
	}

	if upScore == downScore {
		t.Errorf("expected different scores for different price actions, got up=%f down=%f", upScore, downScore)
	}
}

func TestOptimizerValueAndQualityScoresAreNonZero(t *testing.T) {
	o := NewOptimizer()
	quotes := map[string]domain.Quote{}

	v := o.factorEngine.CalculateValueScore("2330.TW", quotes)
	if v == 0 {
		t.Error("expected non-zero mock value score")
	}

	q := o.factorEngine.CalculateQualityScore("2330.TW", quotes)
	if q == 0 {
		t.Error("expected non-zero mock quality score")
	}
}

func TestOptimizeProducesDifferentWeightsBasedOnMomentum(t *testing.T) {
	o := NewOptimizer()
	c := DefaultConstraints()
	c.MaxPositionPct = 1.0 // disable per-position cap so scores flow through
	o.SetConstraints(c)
	o.SetFactorWeights(map[FactorType]float64{
		FactorMomentum: 0.8,
		FactorValue:    0.1,
		FactorQuality:  0.1,
		FactorAgent:    0.0,
	})

	quotes := map[string]domain.Quote{
		"UP.TW":   {Symbol: "UP.TW", Open: 100, Last: 110, IsTradable: true},
		"DOWN.TW": {Symbol: "DOWN.TW", Open: 100, Last: 90, IsTradable: true},
	}

	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "UP.TW", Side: domain.SideBuy, Conviction: 50},
		{Agent: "a", Symbol: "DOWN.TW", Side: domain.SideBuy, Conviction: 50},
	}

	positions, err := o.Optimize(context.Background(), recs, quotes, 1_000_000)
	if err != nil {
		t.Fatalf("optimize failed: %v", err)
	}
	if len(positions) == 0 {
		t.Fatal("expected positions from optimize")
	}

	var upWeight, downWeight float64
	for _, p := range positions {
		if p.Symbol == "UP.TW" {
			upWeight = p.TargetWeight
		}
		if p.Symbol == "DOWN.TW" {
			downWeight = p.TargetWeight
		}
	}

	if upWeight <= downWeight {
		t.Errorf("expected UP.TW to have higher weight than DOWN.TW, got up=%f down=%f", upWeight, downWeight)
	}
}

func TestOptimizeEmptyRecommendations(t *testing.T) {
	o := NewOptimizer()
	positions, err := o.Optimize(context.Background(), nil, map[string]domain.Quote{}, 1_000_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if positions != nil {
		t.Error("expected nil positions for empty recommendations")
	}
}

func TestOptimizeMaxPositionPctConstraint(t *testing.T) {
	o := NewOptimizer()
	c := DefaultConstraints()
	c.MaxPositionPct = 0.10
	c.CashReserve = 0.0
	o.SetConstraints(c)

	quotes := map[string]domain.Quote{
		"A.TW": {Symbol: "A.TW", Open: 100, Last: 110, IsTradable: true},
		"B.TW": {Symbol: "B.TW", Open: 100, Last: 110, IsTradable: true},
		"C.TW": {Symbol: "C.TW", Open: 100, Last: 110, IsTradable: true},
		"D.TW": {Symbol: "D.TW", Open: 100, Last: 110, IsTradable: true},
		"E.TW": {Symbol: "E.TW", Open: 100, Last: 110, IsTradable: true},
		"F.TW": {Symbol: "F.TW", Open: 100, Last: 110, IsTradable: true},
		"G.TW": {Symbol: "G.TW", Open: 100, Last: 110, IsTradable: true},
		"H.TW": {Symbol: "H.TW", Open: 100, Last: 110, IsTradable: true},
		"I.TW": {Symbol: "I.TW", Open: 100, Last: 110, IsTradable: true},
		"J.TW": {Symbol: "J.TW", Open: 100, Last: 110, IsTradable: true},
	}

	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "A.TW", Side: domain.SideBuy, Conviction: 80},
		{Agent: "b", Symbol: "B.TW", Side: domain.SideBuy, Conviction: 80},
		{Agent: "c", Symbol: "C.TW", Side: domain.SideBuy, Conviction: 80},
		{Agent: "d", Symbol: "D.TW", Side: domain.SideBuy, Conviction: 80},
		{Agent: "e", Symbol: "E.TW", Side: domain.SideBuy, Conviction: 80},
		{Agent: "f", Symbol: "F.TW", Side: domain.SideBuy, Conviction: 80},
		{Agent: "g", Symbol: "G.TW", Side: domain.SideBuy, Conviction: 80},
		{Agent: "h", Symbol: "H.TW", Side: domain.SideBuy, Conviction: 80},
		{Agent: "i", Symbol: "I.TW", Side: domain.SideBuy, Conviction: 80},
		{Agent: "j", Symbol: "J.TW", Side: domain.SideBuy, Conviction: 80},
	}

	positions, err := o.Optimize(context.Background(), recs, quotes, 1_000_000)
	if err != nil {
		t.Fatalf("optimize failed: %v", err)
	}
	if len(positions) == 0 {
		t.Fatal("expected positions from optimize")
	}

	for _, p := range positions {
		positionValue := p.TargetValue
		positionPct := positionValue / 1_000_000
		if positionPct > c.MaxPositionPct+0.001 {
			t.Errorf("position %s exceeds max pct: %f > %f", p.Symbol, positionPct, c.MaxPositionPct)
		}
	}
}

func TestOptimizeCashReserveConstraint(t *testing.T) {
	o := NewOptimizer()
	c := DefaultConstraints()
	c.CashReserve = 0.20
	o.SetConstraints(c)

	quotes := map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Open: 500, Last: 550, IsTradable: true},
	}

	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 80},
	}

	positions, err := o.Optimize(context.Background(), recs, quotes, 1_000_000)
	if err != nil {
		t.Fatalf("optimize failed: %v", err)
	}
	if len(positions) == 0 {
		t.Fatal("expected positions from optimize")
	}

	investable := 1_000_000 * (1 - c.CashReserve)
	var totalValue float64
	for _, p := range positions {
		totalValue += p.TargetValue
	}
	if totalValue > investable {
		t.Errorf("total value %f exceeds investable %f after cash reserve", totalValue, investable)
	}
}

func TestOptimizeBuildPositionsMissingQuote(t *testing.T) {
	o := NewOptimizer()
	c := DefaultConstraints()
	c.MaxPositionPct = 1.0
	c.CashReserve = 0.0
	o.SetConstraints(c)
	o.SetFactorWeights(map[FactorType]float64{
		FactorMomentum: 0.0,
		FactorValue:    0.0,
		FactorQuality:  0.0,
		FactorAgent:    1.0,
	})

	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "HAS_QUOTE.TW", Side: domain.SideBuy, Conviction: 80},
		{Agent: "a", Symbol: "NO_QUOTE.TW", Side: domain.SideBuy, Conviction: 80},
	}

	quotes := map[string]domain.Quote{
		"HAS_QUOTE.TW": {Symbol: "HAS_QUOTE.TW", Open: 100, Last: 110, IsTradable: true},
	}

	positions, err := o.Optimize(context.Background(), recs, quotes, 1_000_000)
	if err != nil {
		t.Fatalf("optimize failed: %v", err)
	}
	if len(positions) != 1 {
		t.Fatalf("expected 1 position (missing quote skipped), got %d", len(positions))
	}
	if positions[0].Symbol != "HAS_QUOTE.TW" {
		t.Errorf("expected HAS_QUOTE.TW, got %s", positions[0].Symbol)
	}
}

func TestOptimizeBuildPositionsZeroQuotePrice(t *testing.T) {
	o := NewOptimizer()
	c := DefaultConstraints()
	c.MaxPositionPct = 1.0
	c.CashReserve = 0.0
	o.SetConstraints(c)
	o.SetFactorWeights(map[FactorType]float64{
		FactorMomentum: 0.0,
		FactorValue:    0.0,
		FactorQuality:  0.0,
		FactorAgent:    1.0,
	})

	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "ZERO_PRICE.TW", Side: domain.SideBuy, Conviction: 80},
	}

	quotes := map[string]domain.Quote{
		"ZERO_PRICE.TW": {Symbol: "ZERO_PRICE.TW", Open: 100, Last: 0, IsTradable: true},
	}

	positions, err := o.Optimize(context.Background(), recs, quotes, 1_000_000)
	if err != nil {
		t.Fatalf("optimize failed: %v", err)
	}
	if len(positions) != 0 {
		t.Fatalf("expected 0 positions (zero price skipped), got %d", len(positions))
	}
}

func TestOptimizeAggregateRecommendationsSameSymbolDifferentSides(t *testing.T) {
	o := NewOptimizer()

	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 80},
		{Agent: "b", Symbol: "2330.TW", Side: domain.SideSell, Conviction: 60},
	}

	positions, err := o.Optimize(context.Background(), recs, map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Open: 500, Last: 550, IsTradable: true},
	}, 1_000_000)
	if err != nil {
		t.Fatalf("optimize failed: %v", err)
	}

	buyCount := 0
	sellCount := 0
	for _, p := range positions {
		if p.Symbol == "2330.TW" {
			if p.Side == domain.SideBuy {
				buyCount++
			}
			if p.Side == domain.SideSell {
				sellCount++
			}
		}
	}

	if buyCount == 0 {
		t.Error("expected at least one buy position for 2330.TW")
	}
	if sellCount == 0 {
		t.Error("expected at least one sell position for 2330.TW")
	}
}

func TestOptimizeAggregateRecommendationsSameSymbolSameSide(t *testing.T) {
	o := NewOptimizer()
	c := DefaultConstraints()
	c.MaxPositionPct = 1.0
	c.CashReserve = 0.0
	o.SetConstraints(c)
	o.SetFactorWeights(map[FactorType]float64{
		FactorMomentum: 0.0,
		FactorValue:    0.0,
		FactorQuality:  0.0,
		FactorAgent:    1.0,
	})

	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 80},
		{Agent: "b", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 60},
	}

	positions, err := o.Optimize(context.Background(), recs, map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Open: 500, Last: 550, IsTradable: true},
	}, 1_000_000)
	if err != nil {
		t.Fatalf("optimize failed: %v", err)
	}

	count := 0
	for _, p := range positions {
		if p.Symbol == "2330.TW" {
			count++
		}
	}

	if count != 1 {
		t.Errorf("expected 1 aggregated position for same symbol+side, got %d", count)
	}
}

func TestOptimizeToOrders(t *testing.T) {
	o := NewOptimizer()
	c := DefaultConstraints()
	c.MaxPositionPct = 1.0
	c.CashReserve = 0.0
	o.SetConstraints(c)
	o.SetFactorWeights(map[FactorType]float64{
		FactorMomentum: 0.0,
		FactorValue:    0.0,
		FactorQuality:  0.0,
		FactorAgent:    1.0,
	})

	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 80},
	}

	quotes := map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Open: 500, Last: 550, IsTradable: true},
	}

	orders, err := o.OptimizeToOrders(context.Background(), recs, quotes, 1_000_000)
	if err != nil {
		t.Fatalf("optimize to orders failed: %v", err)
	}
	if len(orders) == 0 {
		t.Fatal("expected orders from optimize")
	}

	if orders[0].Symbol != "2330.TW" {
		t.Errorf("expected symbol 2330.TW, got %s", orders[0].Symbol)
	}
	if orders[0].Side != domain.SideBuy {
		t.Errorf("expected side buy, got %s", orders[0].Side)
	}
	if orders[0].Quantity <= 0 {
		t.Errorf("expected positive quantity, got %d", orders[0].Quantity)
	}
	if orders[0].Price != 550 {
		t.Errorf("expected price 550, got %f", orders[0].Price)
	}
}

func TestOptimizeToOrdersMissingQuote(t *testing.T) {
	o := NewOptimizer()
	c := DefaultConstraints()
	c.MaxPositionPct = 1.0
	c.CashReserve = 0.0
	o.SetConstraints(c)

	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "MISSING.TW", Side: domain.SideBuy, Conviction: 80},
	}

	orders, err := o.OptimizeToOrders(context.Background(), recs, map[string]domain.Quote{}, 1_000_000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(orders) != 0 {
		t.Errorf("expected 0 orders (missing quote), got %d", len(orders))
	}
}

func TestOptimizerWithFactorEngine(t *testing.T) {
	o := NewOptimizer()
	fe := NewFactorEngine()
	result := o.WithFactorEngine(fe)
	if result != o {
		t.Error("expected WithFactorEngine to return the same optimizer")
	}
	if o.factorEngine != fe {
		t.Error("expected factor engine to be attached")
	}
}

func TestOptimizerWithHistoricalPrices(t *testing.T) {
	o := NewOptimizer()
	hp := NewHistoricalPrices()
	result := o.WithHistoricalPrices(hp)
	if result != o {
		t.Error("expected WithHistoricalPrices to return the same optimizer")
	}
	if o.history != hp {
		t.Error("expected historical prices to be attached")
	}
	if o.factorEngine.history != hp {
		t.Error("expected factor engine to also receive historical prices")
	}
}

func TestOptimizerWithFundamentalProvider(t *testing.T) {
	o := NewOptimizer()
	fp := NewFundamentalProvider()
	result := o.WithFundamentalProvider(fp)
	if result != o {
		t.Error("expected WithFundamentalProvider to return the same optimizer")
	}
	if o.fundamentals != fp {
		t.Error("expected fundamental provider to be attached")
	}
	if o.factorEngine.fundamentals != fp {
		t.Error("expected factor engine to also receive fundamental provider")
	}
}

func TestOptimizerSetAgentWeights(t *testing.T) {
	o := NewOptimizer()
	weights := map[string]float64{
		"agent-a": 1.5,
		"agent-b": 0.8,
	}
	o.SetAgentWeights(weights)
	if o.agentWeights["agent-a"] != 1.5 {
		t.Errorf("expected agent-a weight 1.5, got %f", o.agentWeights["agent-a"])
	}
	if o.agentWeights["agent-b"] != 0.8 {
		t.Errorf("expected agent-b weight 0.8, got %f", o.agentWeights["agent-b"])
	}
}

func TestOptimizerSetStyleWeights(t *testing.T) {
	o := NewOptimizer()
	weights := map[string]float64{
		"growth": 1.2,
		"value":  0.9,
	}
	o.SetStyleWeights(weights)
	if o.styleWeights["growth"] != 1.2 {
		t.Errorf("expected growth weight 1.2, got %f", o.styleWeights["growth"])
	}
	if o.styleWeights["value"] != 0.9 {
		t.Errorf("expected value weight 0.9, got %f", o.styleWeights["value"])
	}
}

func TestOptimizerGetEfficientFrontier(t *testing.T) {
	o := NewOptimizer()
	frontier := o.GetEfficientFrontier()
	if len(frontier) == 0 {
		t.Skip("no historical prices attached — frontier requires real return data")
	}
	for i, point := range frontier {
		if point.Risk < 0 {
			t.Errorf("point %d: expected non-negative risk, got %f", i, point.Risk)
		}
	}
}

func TestOptimizeWithAgentWeights(t *testing.T) {
	o := NewOptimizer()
	c := DefaultConstraints()
	c.MaxPositionPct = 1.0
	c.CashReserve = 0.0
	o.SetConstraints(c)
	o.SetFactorWeights(map[FactorType]float64{
		FactorMomentum: 0.0,
		FactorValue:    0.0,
		FactorQuality:  0.0,
		FactorAgent:    1.0,
	})
	o.SetAgentWeights(map[string]float64{
		"strong-agent": 2.0,
		"weak-agent":   0.5,
	})

	recs := []domain.Recommendation{
		{Agent: "strong-agent", Symbol: "A.TW", Side: domain.SideBuy, Conviction: 80},
		{Agent: "weak-agent", Symbol: "B.TW", Side: domain.SideBuy, Conviction: 20},
	}

	quotes := map[string]domain.Quote{
		"A.TW": {Symbol: "A.TW", Open: 100, Last: 100, IsTradable: true},
		"B.TW": {Symbol: "B.TW", Open: 100, Last: 100, IsTradable: true},
	}

	positions, err := o.Optimize(context.Background(), recs, quotes, 1_000_000)
	if err != nil {
		t.Fatalf("optimize failed: %v", err)
	}
	if len(positions) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(positions))
	}

	var aWeight, bWeight float64
	for _, p := range positions {
		if p.Symbol == "A.TW" {
			aWeight = p.TargetWeight
		}
		if p.Symbol == "B.TW" {
			bWeight = p.TargetWeight
		}
	}
	if aWeight <= bWeight {
		t.Errorf("expected A.TW (strong-agent) to have higher weight than B.TW (weak-agent), got a=%f b=%f", aWeight, bWeight)
	}
}

func TestOptimizeWithNarrativeAndIndustryCycleFactors(t *testing.T) {
	o := NewOptimizer()
	c := DefaultConstraints()
	c.MaxPositionPct = 1.0
	c.CashReserve = 0.0
	o.SetConstraints(c)
	o.SetFactorWeights(map[FactorType]float64{
		FactorMomentum:      0.0,
		FactorValue:         0.0,
		FactorQuality:       0.0,
		FactorAgent:         0.0,
		FactorNarrative:     0.5,
		FactorIndustryCycle: 0.5,
	})

	fe := NewFactorEngine()
	fe.WithNarrativeProvider(func(symbol string) *domain.NarrativeFactorScore {
		if symbol == "A.TW" {
			return &domain.NarrativeFactorScore{Score: 0.80, Theme: "AI_capex_surge", HitRate: 0.81, Confidence: 0.90}
		}
		if symbol == "B.TW" {
			return &domain.NarrativeFactorScore{Score: 0.20, Theme: "oil_price_shock", HitRate: 0.58, Confidence: 0.60}
		}
		return nil
	})
	fe.WithIndustryCycleProvider(func(symbol string) *domain.IndustryCycleFactorScore {
		if symbol == "A.TW" {
			return &domain.IndustryCycleFactorScore{Score: 0.70, Phase: "expansion", PhaseScore: 0.80, Confidence: 0.85}
		}
		if symbol == "B.TW" {
			return &domain.IndustryCycleFactorScore{Score: 0.30, Phase: "recession", PhaseScore: -0.60, Confidence: 0.70}
		}
		return nil
	})
	o.WithFactorEngine(fe)

	recs := []domain.Recommendation{
		{Agent: "test-agent", Symbol: "A.TW", Side: domain.SideBuy, Conviction: 50},
		{Agent: "test-agent", Symbol: "B.TW", Side: domain.SideBuy, Conviction: 50},
	}

	quotes := map[string]domain.Quote{
		"A.TW": {Symbol: "A.TW", Open: 100, Last: 100, IsTradable: true},
		"B.TW": {Symbol: "B.TW", Open: 100, Last: 100, IsTradable: true},
	}

	positions, err := o.Optimize(context.Background(), recs, quotes, 1_000_000)
	if err != nil {
		t.Fatalf("optimize failed: %v", err)
	}
	if len(positions) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(positions))
	}

	var aWeight, bWeight float64
	for _, p := range positions {
		if p.Symbol == "A.TW" {
			aWeight = p.TargetWeight
		}
		if p.Symbol == "B.TW" {
			bWeight = p.TargetWeight
		}
		if p.Symbol == "A.TW" || p.Symbol == "B.TW" {
			if _, ok := p.Factors[FactorNarrative]; !ok {
				t.Errorf("expected %s to have narrative factor", p.Symbol)
			}
			if _, ok := p.Factors[FactorIndustryCycle]; !ok {
				t.Errorf("expected %s to have industry cycle factor", p.Symbol)
			}
		}
	}
	if aWeight <= bWeight {
		t.Errorf("expected A.TW (high narrative + cycle) to have higher weight than B.TW (low), got a=%f b=%f", aWeight, bWeight)
	}
}

// ── Covariance Optimization Tests ──

func makeTestHistory(syms []string, nDays int, gen func(sym string, day int) float64) *HistoricalPrices {
	hp := NewHistoricalPrices()
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, sym := range syms {
		pts := make([]pricePoint, nDays)
		for d := range nDays {
			pts[d] = pricePoint{
				Date:  base.AddDate(0, 0, d),
				Close: gen(sym, d),
			}
		}
		hp.prices[sym] = pts
	}
	return hp
}

func TestCovarianceOptimizer_N2EdgeCase(t *testing.T) {
	hp := makeTestHistory([]string{"STABLE.TW", "VOL.TW"}, 65, func(sym string, day int) float64 {
		base := 100.0
		if sym == "STABLE.TW" {
			return base + 0.1*math.Sin(float64(day)*0.3)
		}
		return base + 5.0*math.Sin(float64(day)*1.7)
	})

	o := NewOptimizer()
	o.WithHistoricalPrices(hp)
	o.SetConstraints(Constraints{
		MaxPositionPct:   0.6,
		CashReserve:      0.0,
		MaxSectorPct:     1.0,
		MaxTurnoverDaily: 1.0,
	})

	quotes := map[string]domain.Quote{
		"STABLE.TW": {Symbol: "STABLE.TW", Open: 100, Last: 101, IsTradable: true},
		"VOL.TW":    {Symbol: "VOL.TW", Open: 100, Last: 101, IsTradable: true},
	}

	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "STABLE.TW", Side: domain.SideBuy, Conviction: 50},
		{Agent: "a", Symbol: "VOL.TW", Side: domain.SideBuy, Conviction: 50},
	}

	positions, err := o.Optimize(context.Background(), recs, quotes, 1_000_000)
	if err != nil {
		t.Fatalf("optimize failed: %v", err)
	}

	if len(positions) == 0 {
		t.Fatal("expected positions from optimize")
	}

	var sumW float64
	for _, p := range positions {
		sumW += p.TargetWeight
	}
	if math.Abs(sumW-1.0) > 0.001 {
		t.Errorf("Scenario A fail: Σw = %f, expected 1.0 ± 0.001", sumW)
	}

	for _, p := range positions {
		if p.TargetWeight <= 0 {
			t.Errorf("Scenario A fail: %s weight = %f, expected > 0", p.Symbol, p.TargetWeight)
		}
	}

	var stableW, volW float64
	for _, p := range positions {
		if p.Symbol == "STABLE.TW" {
			stableW = p.TargetWeight
		}
		if p.Symbol == "VOL.TW" {
			volW = p.TargetWeight
		}
	}
	if stableW <= volW {
		t.Errorf("Scenario A fail: STABLE (low vol) weight %f ≤ VOL (high vol) weight %f", stableW, volW)
	}
}

func TestCovarianceOptimizer_CorrectnessCheck(t *testing.T) {
	hp := makeTestHistory([]string{"A.TW", "B.TW", "C.TW", "D.TW"}, 65, func(sym string, day int) float64 {
		seeds := map[string]float64{"A.TW": 0.3, "B.TW": 1.0, "C.TW": 2.0, "D.TW": 0.7}
		base := 100.0
		return base * (1 + 0.02*math.Sin(float64(day)*seeds[sym]))
	})

	o := NewOptimizer()
	o.WithHistoricalPrices(hp)
	wMax := 0.3
	o.SetConstraints(Constraints{
		MaxPositionPct:   wMax,
		CashReserve:      0.0,
		MaxSectorPct:     1.0,
		MaxTurnoverDaily: 1.0,
	})
	o.SetFactorWeights(map[FactorType]float64{
		FactorMomentum: 0.0, FactorValue: 0.0, FactorQuality: 0.0, FactorAgent: 1.0,
	})

	quotes := map[string]domain.Quote{
		"A.TW": {Symbol: "A.TW", Open: 100, Last: 100, IsTradable: true},
		"B.TW": {Symbol: "B.TW", Open: 100, Last: 100, IsTradable: true},
		"C.TW": {Symbol: "C.TW", Open: 100, Last: 100, IsTradable: true},
		"D.TW": {Symbol: "D.TW", Open: 100, Last: 100, IsTradable: true},
	}

	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "A.TW", Side: domain.SideBuy, Conviction: 50},
		{Agent: "a", Symbol: "B.TW", Side: domain.SideBuy, Conviction: 50},
		{Agent: "a", Symbol: "C.TW", Side: domain.SideBuy, Conviction: 50},
		{Agent: "a", Symbol: "D.TW", Side: domain.SideBuy, Conviction: 50},
	}

	positions, err := o.Optimize(context.Background(), recs, quotes, 1_000_000)
	if err != nil {
		t.Fatalf("optimize failed: %v", err)
	}

	if len(positions) == 0 {
		t.Fatal("expected positions from optimize")
	}

	var sumW float64
	for _, p := range positions {
		if p.TargetWeight < 0 {
			t.Errorf("Scenario C fail: %s weight = %f < 0", p.Symbol, p.TargetWeight)
		}
		if p.TargetWeight > wMax+0.001 {
			t.Errorf("Scenario C fail: %s weight = %f > w_max = %f", p.Symbol, p.TargetWeight, wMax)
		}
		sumW += p.TargetWeight
	}

	if math.Abs(sumW-1.0) > 0.001 {
		t.Errorf("Scenario C fail: Σw = %f, expected 1.0 ± 0.001", sumW)
	}
}

func TestCovarianceOptimizer_FallbackToLinearWhenNoHistory(t *testing.T) {
	o := NewOptimizer()
	c := DefaultConstraints()
	c.MaxPositionPct = 1.0
	c.CashReserve = 0.0
	o.SetConstraints(c)
	o.SetFactorWeights(map[FactorType]float64{
		FactorMomentum: 0.8, FactorValue: 0.1, FactorQuality: 0.1, FactorAgent: 0.0,
	})

	quotes := map[string]domain.Quote{
		"UP.TW":   {Symbol: "UP.TW", Open: 100, Last: 110, IsTradable: true},
		"DOWN.TW": {Symbol: "DOWN.TW", Open: 100, Last: 90, IsTradable: true},
	}

	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "UP.TW", Side: domain.SideBuy, Conviction: 50},
		{Agent: "a", Symbol: "DOWN.TW", Side: domain.SideBuy, Conviction: 50},
	}

	positions, err := o.Optimize(context.Background(), recs, quotes, 1_000_000)
	if err != nil {
		t.Fatalf("optimize failed: %v", err)
	}

	if len(positions) == 0 {
		t.Fatal("expected positions from optimize")
	}

	var upWeight, downWeight float64
	for _, p := range positions {
		if p.Symbol == "UP.TW" {
			upWeight = p.TargetWeight
		}
		if p.Symbol == "DOWN.TW" {
			downWeight = p.TargetWeight
		}
	}

	if upWeight <= downWeight {
		t.Errorf("fallback fail: expected UP.TW to have higher weight than DOWN.TW, got up=%f down=%f", upWeight, downWeight)
	}
}

func TestCovarianceOptimizer_EfficientFrontierWithData(t *testing.T) {
	hp := makeTestHistory([]string{"2330.TW", "2317.TW", "2454.TW"}, 65, func(sym string, day int) float64 {
		seeds := map[string]float64{"2330.TW": 0.4, "2317.TW": 1.2, "2454.TW": 0.8}
		base := 100.0
		return base * (1 + 0.015*math.Sin(float64(day)*seeds[sym]))
	})

	o := NewOptimizer()
	o.WithHistoricalPrices(hp)
	o.SetConstraints(DefaultConstraints())

	frontier := o.GetEfficientFrontier()
	if len(frontier) == 0 {
		t.Skip("no valid returns — frontier requires real data from the 10 default symbols")
	}
	if len(frontier) != 20 {
		t.Errorf("expected 20 frontier points, got %d", len(frontier))
	}

	for i, point := range frontier {
		if point.Risk < 0 {
			t.Errorf("point %d: risk = %f < 0", i, point.Risk)
		}
	}
}

// TestCovarianceOptimizer_HighVolatilityStress verifies that Ledoit-Wolf
// shrinkage remains stable under extreme daily amplitudes (±20%).
// Falsification: if shrinkage produces NaN, Inf, or concentrates >80% weight
// into a single asset, the model is wrong.
func TestCovarianceOptimizer_HighVolatilityStress(t *testing.T) {
	assets := []string{"HV1.TW", "HV2.TW", "HV3.TW", "HV4.TW", "HV5.TW"}
	hp := makeTestHistory(assets, 65, func(sym string, day int) float64 {
		base := 100.0
		amp := 0.05
		switch sym {
		case "HV1.TW":
			amp = 0.18
		case "HV2.TW":
			amp = 0.22
		case "HV3.TW":
			amp = 0.08
		case "HV4.TW":
			amp = 0.15
		case "HV5.TW":
			amp = 0.20
		}
		return base * (1 + amp*math.Sin(float64(day)*float64(len(sym))))
	})

	o := NewOptimizer()
	o.WithHistoricalPrices(hp)
	o.SetConstraints(Constraints{
		MaxPositionPct:   0.4,
		CashReserve:      0.0,
		MaxSectorPct:     1.0,
		MaxTurnoverDaily: 1.0,
	})
	o.SetFactorWeights(map[FactorType]float64{
		FactorMomentum: 0.0, FactorValue: 0.0, FactorQuality: 0.0, FactorAgent: 1.0,
	})

	quotes := make(map[string]domain.Quote)
	recs := make([]domain.Recommendation, 0, len(assets))
	for _, sym := range assets {
		quotes[sym] = domain.Quote{Symbol: sym, Open: 100, Last: 100, IsTradable: true}
		recs = append(recs, domain.Recommendation{Agent: "a", Symbol: sym, Side: domain.SideBuy, Conviction: 50})
	}

	positions, err := o.Optimize(context.Background(), recs, quotes, 1_000_000)
	if err != nil {
		t.Fatalf("optimize failed under stress: %v", err)
	}
	if len(positions) == 0 {
		t.Fatal("expected positions under stress")
	}

	var sumW, hhi float64
	maxW := 0.0
	for _, p := range positions {
		if p.TargetWeight < 0 {
			t.Errorf("stress fail: %s weight = %f < 0", p.Symbol, p.TargetWeight)
		}
		if p.TargetWeight > maxW {
			maxW = p.TargetWeight
		}
		if math.IsNaN(p.TargetWeight) {
			t.Errorf("stress fail: %s weight is NaN", p.Symbol)
		}
		if math.IsInf(p.TargetWeight, 0) {
			t.Errorf("stress fail: %s weight is Inf", p.Symbol)
		}
		sumW += p.TargetWeight
		hhi += p.TargetWeight * p.TargetWeight
	}

	if math.Abs(sumW-1.0) > 0.001 {
		t.Errorf("stress fail: Σw = %f, expected 1.0 ± 0.001", sumW)
	}
	if maxW > 0.8 {
		t.Errorf("stress fail: max weight = %f > 0.8 — over-concentrated", maxW)
	}
	if hhi > 0.5 {
		t.Errorf("stress fail: HHI = %f > 0.5 — insufficient diversification", hhi)
	}
}

// TestCrossAssetIntegration verifies the full pipeline (FactorEngine → Optimizer)
// handles a mixed portfolio of stocks, gold, and silver correctly.
// Each asset class takes a different computation path:
//
//	2330.TW: stock factors (Momentum, Value, Quality) + no PM
//	00635U: stock Value/Quality→0, IndustryCycle skipped, PM active
//	SLV:    same as gold + IndustrialDemand/GoldSilver in silver formula
//
// Falsification: if a stock gets PM>0 or gold gets Value>0, the model is wrong.
func TestCrossAssetIntegration(t *testing.T) {
	symbols := []string{"2330.TW", "00635U", "SLV"}
	hp := makeTestHistory(symbols, 65, func(sym string, day int) float64 {
		base := 100.0
		return base * (1 + 0.01*math.Sin(float64(day)*0.5))
	})

	o := NewOptimizer()
	o.WithHistoricalPrices(hp)
	o.SetConstraints(Constraints{
		MaxPositionPct:   0.4,
		CashReserve:      0.0,
		MaxSectorPct:     1.0,
		MaxTurnoverDaily: 1.0,
	})
	o.SetFactorWeights(map[FactorType]float64{
		FactorMomentum: 0.0, FactorValue: 0.0, FactorQuality: 0.0,
		FactorAgent: 1.0, FactorPreciousMetals: 1.0,
	})

	o.factorEngine.WithPreciousMetalsProvider(func(symbol string) *PreciousMetalsContext {
		return &PreciousMetalsContext{
			RealRate: 0.01,
			VIX:      18,
			DXY:      100,
			CPIYoY:   0.02,
		}
	})

	quotes := map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Open: 100, Last: 101, IsTradable: true},
		"00635U":  {Symbol: "00635U", Open: 30, Last: 30.3, IsTradable: true},
		"SLV":     {Symbol: "SLV", Open: 25, Last: 25.5, IsTradable: true},
	}

	recs := []domain.Recommendation{
		{Agent: "a", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 50},
		{Agent: "a", Symbol: "00635U", Side: domain.SideBuy, Conviction: 50},
		{Agent: "a", Symbol: "SLV", Side: domain.SideBuy, Conviction: 50},
	}

	positions, err := o.Optimize(context.Background(), recs, quotes, 1_000_000)
	if err != nil {
		t.Fatalf("cross-asset optimize failed: %v", err)
	}
	if len(positions) != 3 {
		t.Fatalf("expected 3 positions, got %d", len(positions))
	}

	var sumW float64
	factorBySymbol := make(map[string]map[FactorType]float64)
	for _, p := range positions {
		sumW += p.TargetWeight
		factorBySymbol[p.Symbol] = p.Factors
	}

	if math.Abs(sumW-1.0) > 0.001 {
		t.Errorf("cross-asset fail: Σw = %f, expected 1.0", sumW)
	}

	// 2330.TW is a stock: must NOT have PreciousMetals factor.
	f2330 := factorBySymbol["2330.TW"]
	if _, hasPM := f2330[FactorPreciousMetals]; hasPM {
		t.Errorf("cross-asset fail: 2330.TW (stock) has PreciousMetals = %f", f2330[FactorPreciousMetals])
	}
	// Stock should retain its Value/Quality path (0 when no fundamental data).
	if f2330[FactorValue] != 0 {
		t.Logf("2330.TW Value = %f (non-zero — good, fundamental data available)", f2330[FactorValue])
	}
	if f2330[FactorQuality] != 0 {
		t.Logf("2330.TW Quality = %f (non-zero — good, fundamental data available)", f2330[FactorQuality])
	}

	// 00635U is gold: PreciousMetals active, stock Value/Quality must be zero.
	fGold := factorBySymbol["00635U"]
	if fGold[FactorValue] != 0 {
		t.Errorf("cross-asset fail: 00635U (gold) Value = %f, expected 0", fGold[FactorValue])
	}
	if fGold[FactorQuality] != 0 {
		t.Errorf("cross-asset fail: 00635U (gold) Quality = %f, expected 0", fGold[FactorQuality])
	}
	if _, hasPM := fGold[FactorPreciousMetals]; !hasPM {
		t.Errorf("cross-asset fail: 00635U (gold) missing PreciousMetals factor")
	}

	// SLV is silver: should have PreciousMetals.
	fSilver := factorBySymbol["SLV"]
	if _, hasPM := fSilver[FactorPreciousMetals]; !hasPM {
		t.Errorf("cross-asset fail: SLV (silver) missing PreciousMetals factor")
	}
}

// ── Multi-Day Drawdown Simulation Tests (P1, Gate 3.2) ──

func TestDrawdownSimulation_COVIDCrash(t *testing.T) {
	hp := makeTestHistory([]string{"A.TW", "B.TW", "C.TW", "D.TW"}, 65, func(sym string, day int) float64 {
		return 100 * (1 + 0.01*math.Sin(float64(day)*0.5))
	})

	o := NewOptimizer()
	o.WithHistoricalPrices(hp)

	weights := []weightInfo{
		{Symbol: "A.TW", Weight: 0.25},
		{Symbol: "B.TW", Weight: 0.25},
		{Symbol: "C.TW", Weight: 0.25},
		{Symbol: "D.TW", Weight: 0.25},
	}

	result := o.SimulateDrawdown(weights, 4.0, 21, 200)

	if result.MaxDrawdown < 0.15 {
		t.Errorf("COVID crash: max dd = %.2f%%, expected > 15%% (VIX=80, scale=4x)",
			result.MaxDrawdown*100)
	}
	if result.VaR95 < 0.10 {
		t.Errorf("COVID crash: VaR95 = %.2f%%, expected > 10%%",
			result.VaR95*100)
	}
	if len(result.WorstPath) != 22 {
		t.Errorf("expected 22 path points (21 days + start), got %d", len(result.WorstPath))
	}
}

func TestDrawdownSimulation_FedHiking(t *testing.T) {
	hp := makeTestHistory([]string{"A.TW", "B.TW", "C.TW", "D.TW"}, 65, func(sym string, day int) float64 {
		return 100 * (1 + 0.01*math.Sin(float64(day)*0.5))
	})

	o := NewOptimizer()
	o.WithHistoricalPrices(hp)

	weights := []weightInfo{
		{Symbol: "A.TW", Weight: 0.25},
		{Symbol: "B.TW", Weight: 0.25},
		{Symbol: "C.TW", Weight: 0.25},
		{Symbol: "D.TW", Weight: 0.25},
	}

	result := o.SimulateDrawdown(weights, 1.6, 21, 200)

	if result.MaxDrawdown < 0.05 {
		t.Errorf("Fed hikes: max dd = %.2f%%, expected > 5%% (VIX=32, scale=1.6x)",
			result.MaxDrawdown*100)
	}
	if result.MaxDrawdown > 0.40 {
		t.Errorf("Fed hikes: max dd = %.2f%%, expected < 40%% (VIX=32 not as extreme as VIX=80)",
			result.MaxDrawdown*100)
	}
}

func TestDrawdownSimulation_NormalMarket(t *testing.T) {
	hp := makeTestHistory([]string{"A.TW", "B.TW", "C.TW", "D.TW"}, 65, func(sym string, day int) float64 {
		return 100 * (1 + 0.01*math.Sin(float64(day)*0.5))
	})

	o := NewOptimizer()
	o.WithHistoricalPrices(hp)

	weights := []weightInfo{
		{Symbol: "A.TW", Weight: 0.25},
		{Symbol: "B.TW", Weight: 0.25},
		{Symbol: "C.TW", Weight: 0.25},
		{Symbol: "D.TW", Weight: 0.25},
	}

	result := o.SimulateDrawdown(weights, 1.0, 21, 200)

	if result.MaxDrawdown > 0.15 {
		t.Errorf("Normal market: max dd = %.2f%%, expected < 15%% (VIX~20, scale=1x)",
			result.MaxDrawdown*100)
	}
	if result.VaR95 < 0 {
		t.Errorf("Normal market: VaR95 should be non-negative, got %.2f%%",
			result.VaR95*100)
	}
}

func TestDrawdownSimulation_NotEnoughAssets(t *testing.T) {
	o := NewOptimizer()
	result := o.SimulateDrawdown([]weightInfo{{Symbol: "ONLY.TW", Weight: 1.0}}, 2.0, 10, 100)

	if result.MaxDrawdown != 0 {
		t.Error("expected zero drawdown for single-asset portfolio (no covariance)")
	}
}

const testTSMCProviderScore = 0.75

func TestOptimizer_TSMCConsistency(t *testing.T) {
	fe := NewFactorEngine()
	fe.WithTSMCProvider(func(symbol string) *domain.FactorScoreItem {
		return &domain.FactorScoreItem{
			Score:      testTSMCProviderScore,
			Formula:    "tsmc_factor(adv_ratio=0.42, premium=1.23)",
			RawInputs:  map[string]float64{"adv_ratio": 0.42, "premium": 1.23},
			IsFallback: false,
		}
	})

	o := NewOptimizer()
	o.WithFactorEngine(fe)
	c := DefaultConstraints()
	c.MaxPositionPct = 1.0
	c.CashReserve = 0.0
	o.SetConstraints(c)
	o.SetFactorWeights(map[FactorType]float64{
		FactorMomentum: 0.0,
		FactorValue:    0.0,
		FactorQuality:  0.0,
		FactorAgent:    0.0,
		FactorTSMC:     1.0,
	})

	quotes := map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Open: 500, Last: 550, IsTradable: true},
	}
	recs := []domain.Recommendation{
		{Agent: "test-agent", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 80},
	}

	breakdown, _ := fe.CalculateAllScoresWithBreakdown("2330.TW", quotes, recs, nil, map[FactorType]float64{
		FactorMomentum: 0.25,
		FactorValue:    0.20,
		FactorQuality:  0.20,
		FactorAgent:    0.15,
		FactorTSMC:     0.20,
	})

	if breakdown.TSMC.Score != testTSMCProviderScore {
		t.Errorf("FactorEngine path: expected TSMC score %f, got %f", testTSMCProviderScore, breakdown.TSMC.Score)
	}
	if breakdown.TSMC.Formula == "" {
		t.Error("FactorEngine path: expected non-empty TSMC.Formula")
	}

	aggregated := o.aggregateRecommendations(recs)
	internalScores := o.calculateMultiFactorScores(aggregated, quotes, map[FactorType]float64{
		FactorMomentum: 0.0,
		FactorValue:    0.0,
		FactorQuality:  0.0,
		FactorAgent:    0.0,
		FactorTSMC:     1.0,
	})

	key := "2330.TW_BUY"
	internalScore, ok := internalScores[key]
	if !ok {
		t.Fatal("expected symbol score for 2330.TW in optimizer path")
	}
	if internalScore.TSMC != testTSMCProviderScore {
		t.Errorf("Optimizer path: expected TSMC score %f, got %f", testTSMCProviderScore, internalScore.TSMC)
	}

	if breakdown.TSMC.Score != internalScore.TSMC {
		t.Errorf("TSMC score mismatch between paths: FactorEngine=%f, Optimizer=%f",
			breakdown.TSMC.Score, internalScore.TSMC)
	}
}
