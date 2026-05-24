package portfolio

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestFactorEngineCalculateMomentumScore(t *testing.T) {
	fe := NewFactorEngine()
	quotes := map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Open: 500, Last: 550, IsTradable: true},
		"2317.TW": {Symbol: "2317.TW", Open: 200, Last: 180, IsTradable: true},
		"FLAT.TW": {Symbol: "FLAT.TW", Open: 100, Last: 100, IsTradable: true},
	}

	upScore := fe.CalculateMomentumScore("2330.TW", quotes)
	if upScore <= 0 {
		t.Errorf("expected positive momentum for up-day, got %f", upScore)
	}

	downScore := fe.CalculateMomentumScore("2317.TW", quotes)
	if downScore >= 0 {
		t.Errorf("expected negative momentum for down-day, got %f", downScore)
	}

	flatScore := fe.CalculateMomentumScore("FLAT.TW", quotes)
	if flatScore != 0 {
		t.Errorf("expected zero momentum for flat day, got %f", flatScore)
	}
}

func TestFactorEngineCalculateValueScore(t *testing.T) {
	fe := NewFactorEngine()
	quotes := map[string]domain.Quote{}

	score := fe.CalculateValueScore("2330.TW", quotes)
	if score != 0.05 {
		t.Errorf("expected fallback value score 0.05, got %f", score)
	}
}

func TestFactorEngineCalculateValueScoreWithFundamentals(t *testing.T) {
	fe := NewFactorEngine()
	fp := NewFundamentalProvider()
	fp.data = map[string]FundamentalData{
		"CHEAP.TW":     {PE: 5, PB: 0.5},
		"EXPENSIVE.TW": {PE: 50, PB: 5.0},
	}
	fe.WithFundamentalProvider(fp)

	cheapScore := fe.CalculateValueScore("CHEAP.TW", nil)
	if cheapScore <= 0.5 {
		t.Errorf("expected high value score for cheap stock, got %f", cheapScore)
	}

	expensiveScore := fe.CalculateValueScore("EXPENSIVE.TW", nil)
	if expensiveScore > 0.1 {
		t.Errorf("expected low or neutral value score for expensive stock, got %f", expensiveScore)
	}
}

func TestFactorEngineCalculateQualityScore(t *testing.T) {
	fe := NewFactorEngine()
	quotes := map[string]domain.Quote{}

	score := fe.CalculateQualityScore("2330.TW", quotes)
	if score != 0.05 {
		t.Errorf("expected fallback quality score 0.05, got %f", score)
	}
}

func TestFactorEngineCalculateQualityScoreWithDividendYield(t *testing.T) {
	fe := NewFactorEngine()
	fp := NewFundamentalProvider()
	fp.data = map[string]FundamentalData{
		"HIGH_YIELD.TW": {DividendYield: 5.0},
		"NO_YIELD.TW":   {DividendYield: 0},
	}
	fe.WithFundamentalProvider(fp)

	highScore := fe.CalculateQualityScore("HIGH_YIELD.TW", nil)
	if highScore <= 0.5 {
		t.Errorf("expected high quality score for high yield, got %f", highScore)
	}

	lowScore := fe.CalculateQualityScore("NO_YIELD.TW", nil)
	if lowScore != 0.05 {
		t.Errorf("expected fallback quality score for no yield, got %f", lowScore)
	}
}

func TestFactorEngineCalculateAllScores(t *testing.T) {
	fe := NewFactorEngine()
	quotes := map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Open: 500, Last: 550, IsTradable: true},
	}
	agentRecs := []domain.Recommendation{
		{Agent: "test-agent", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 80},
	}
	agentWeights := map[string]float64{}
	factorWeights := map[FactorType]float64{
		FactorMomentum: 0.3,
		FactorValue:    0.25,
		FactorQuality:  0.25,
		FactorAgent:    0.2,
	}

	scores := fe.CalculateAllScores("2330.TW", quotes, agentRecs, agentWeights, factorWeights)

	if _, ok := scores[FactorMomentum]; !ok {
		t.Error("expected momentum score")
	}
	if _, ok := scores[FactorValue]; !ok {
		t.Error("expected value score")
	}
	if _, ok := scores[FactorQuality]; !ok {
		t.Error("expected quality score")
	}
	if _, ok := scores[FactorAgent]; !ok {
		t.Error("expected agent score")
	}
	if total, ok := scores["total"]; !ok || total == 0 {
		t.Errorf("expected non-zero total score, got %f", total)
	}
}

func TestFactorEngineCalculateAllScoresWithoutTotal(t *testing.T) {
	fe := NewFactorEngine()
	quotes := map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Open: 500, Last: 550, IsTradable: true},
	}
	agentRecs := []domain.Recommendation{}
	agentWeights := map[string]float64{}

	scores := fe.CalculateAllScores("2330.TW", quotes, agentRecs, agentWeights, nil)

	if _, ok := scores["total"]; ok {
		t.Error("did not expect total score when factorWeights is nil")
	}
}

func TestFactorEngineCalculateMomentumScoreMissingQuote(t *testing.T) {
	fe := NewFactorEngine()
	quotes := map[string]domain.Quote{}

	score := fe.CalculateMomentumScore("UNKNOWN.TW", quotes)
	if score != 0.0 {
		t.Errorf("expected zero momentum for missing quote, got %f", score)
	}
}

func TestFactorEngineWithHistoricalPricesAndFundamentals(t *testing.T) {
	fe := NewFactorEngine()
	hp := NewHistoricalPrices()
	fp := NewFundamentalProvider()

	fe.WithHistoricalPrices(hp).WithFundamentalProvider(fp)

	if fe.history != hp {
		t.Error("expected historical prices to be set")
	}
	if fe.fundamentals != fp {
		t.Error("expected fundamentals to be set")
	}
}

// TestMomentumFallbackProducesLowerScore tests SCOR-01:
// Momentum fallback (intraday) should produce lower score than 20-day return for same input
func TestMomentumFallbackProducesLowerScore(t *testing.T) {
	fe := NewFactorEngine()

	// Test with intraday return that would clamp to 1.0 with old formula
	// With old formula: 0.10 / 0.10 = 1.0 (clamped)
	// With new formula: 0.10 / 0.10 * 0.5 = 0.5
	quotes := map[string]domain.Quote{
		"TEST.TW": {Symbol: "TEST.TW", Open: 100, Last: 110, IsTradable: true},
	}

	momFallback := fe.calculateMomentumDetail("TEST.TW", quotes)

	// Verify it's a fallback
	if !momFallback.IsFallback {
		t.Error("expected fallback momentum when no historical data")
	}

	// Verify the score is 0.5 (not 1.0 as it would be with old formula)
	// intraday return = (110-100)/100 = 0.10
	// old: 0.10 / 0.10 = 1.0
	// new: 0.10 / 0.10 * 0.5 = 0.5
	if momFallback.Score != 0.5 {
		t.Errorf("expected fallback score 0.5, got %f", momFallback.Score)
	}

	// Verify formula is updated
	if momFallback.Formula != "clamp(intraday / 0.10 * 0.5, -1, 1)" {
		t.Errorf("expected formula 'clamp(intraday / 0.10 * 0.5, -1, 1)', got '%s'", momFallback.Formula)
	}
}

// TestMomentumFallbackZeroQuote tests edge case:
// When quote is missing or has zero open, fallback should return zero score
func TestMomentumFallbackZeroQuote(t *testing.T) {
	fe := NewFactorEngine()

	// Empty quotes - should return fallback with zero score
	quotes := map[string]domain.Quote{}

	momDetail := fe.calculateMomentumDetail("UNKNOWN.TW", quotes)

	if !momDetail.IsFallback {
		t.Error("expected fallback when quote is missing")
	}
	if momDetail.Score != 0.0 {
		t.Errorf("expected zero score for missing quote, got %f", momDetail.Score)
	}
}

// TestValueFallbackWeight tests SCOR-01:
func TestValueFallbackFlag(t *testing.T) {
	fe := NewFactorEngine()

	valDetail := fe.calculateValueDetail("TEST.TW", nil)

	if !valDetail.IsFallback {
		t.Error("expected value to be fallback")
	}
}

func TestQualityFallbackFlag(t *testing.T) {
	fe := NewFactorEngine()

	qlyDetail := fe.calculateQualityDetail("TEST.TW")

	if !qlyDetail.IsFallback {
		t.Error("expected quality to be fallback")
	}
}

// TestFallbackFactorsGetReducedWeight tests SCOR-04:
// Fallback factors should get reduced weight (50%) in total calculation
func TestFallbackFactorsGetReducedWeight(t *testing.T) {
	fe := NewFactorEngine()

	quotes := map[string]domain.Quote{
		"TEST.TW": {Symbol: "TEST.TW", Open: 100, Last: 110, IsTradable: true},
	}

	agentRecs := []domain.Recommendation{
		{Agent: "test-agent", Symbol: "TEST.TW", Side: domain.SideBuy, Conviction: 80},
	}

	factorWeights := map[FactorType]float64{
		FactorMomentum: 0.30,
		FactorValue:    0.25,
		FactorQuality:  0.25,
		FactorAgent:    0.20,
	}

	breakdown, scores := fe.CalculateAllScoresWithBreakdown("TEST.TW", quotes, agentRecs, nil, factorWeights)

	// Momentum should be fallback (no historical data)
	if !breakdown.Momentum.IsFallback {
		t.Error("expected momentum to be fallback")
	}

	// Value and Quality should be fallback (no fundamentals)
	if !breakdown.Value.IsFallback {
		t.Error("expected value to be fallback")
	}
	if !breakdown.Quality.IsFallback {
		t.Error("expected quality to be fallback")
	}

	// Calculate expected total with 50% weight reduction for fallback factors
	// Momentum: 0.5 * 0.15 (0.30 * 0.5) = 0.075
	// Value: 0.05 * 0.125 (0.25 * 0.5) = 0.00625
	// Quality: 0.05 * 0.125 (0.25 * 0.5) = 0.00625
	// Agent: 0.8 * 0.20 = 0.16 (agent is not fallback)
	expectedTotal := 0.5*0.15 + 0.05*0.125 + 0.05*0.125 + 0.8*0.20

	if abs(scores["total"]-expectedTotal) > 0.001 {
		t.Errorf("expected total %f, got %f", expectedTotal, scores["total"])
	}

	// Verify the formula mentions fallback weight reduction
	if breakdown.Total.Formula == "" {
		t.Error("expected total formula to be set")
	}
	if breakdown.Total.Formula != "sum(factor_score * effective_weight)" {
		t.Errorf("expected formula 'sum(factor_score * effective_weight)', got '%s'", breakdown.Total.Formula)
	}
}

// TestNonFallbackFactorsKeepNormalWeight tests SCOR-04:
// Non-fallback factors should keep normal weight in total calculation
// This test verifies the CalculateAllScoresWithBreakdown still works when no fallbacks
func TestNonFallbackFactorsKeepNormalWeight(t *testing.T) {
	fe := NewFactorEngine()
	fp := NewFundamentalProvider()

	// Set up fundamentals for value and quality (but no historical prices for momentum)
	fp.data = map[string]FundamentalData{
		"TEST.TW": {PE: 10, PB: 1.0, DividendYield: 2.0},
	}

	fe.WithFundamentalProvider(fp)

	quotes := map[string]domain.Quote{
		"TEST.TW": {Symbol: "TEST.TW", Open: 100, Last: 110, IsTradable: true},
	}

	agentRecs := []domain.Recommendation{
		{Agent: "test-agent", Symbol: "TEST.TW", Side: domain.SideBuy, Conviction: 80},
	}

	factorWeights := map[FactorType]float64{
		FactorMomentum: 0.30,
		FactorValue:    0.25,
		FactorQuality:  0.25,
		FactorAgent:    0.20,
	}

	breakdown, scores := fe.CalculateAllScoresWithBreakdown("TEST.TW", quotes, agentRecs, nil, factorWeights)

	// Momentum should be fallback (no historical data)
	if !breakdown.Momentum.IsFallback {
		t.Error("expected momentum to be fallback (no historical prices)")
	}

	// Value and Quality should NOT be fallback (have fundamentals)
	if breakdown.Value.IsFallback {
		t.Error("expected value to be non-fallback (has fundamentals)")
	}
	if breakdown.Quality.IsFallback {
		t.Error("expected quality to be non-fallback (has fundamentals)")
	}

	// Calculate expected total with normal weights for non-fallback factors
	// Momentum: 0.5 * 0.15 (0.30 * 0.5) = 0.075 (fallback)
	// Value: uses full weight 0.25
	// Quality: uses full weight 0.25
	// Agent: 0.8 * 0.20 = 0.16 (not fallback)
	expectedTotal := 0.5*0.15 + breakdown.Value.Score*0.25 + breakdown.Quality.Score*0.25 + 0.8*0.20

	if abs(scores["total"]-expectedTotal) > 0.001 {
		t.Errorf("expected total %f, got %f", expectedTotal, scores["total"])
	}
}

func TestFactorEngineWithNarrativeAndIndustryCycleProviders(t *testing.T) {
	fe := NewFactorEngine()
	quotes := map[string]domain.Quote{
		"TEST.TW": {Symbol: "TEST.TW", Open: 100, Last: 110, IsTradable: true},
	}
	agentRecs := []domain.Recommendation{
		{Agent: "test-agent", Symbol: "TEST.TW", Side: domain.SideBuy, Conviction: 80},
	}
	factorWeights := map[FactorType]float64{
		FactorMomentum:      0.25,
		FactorValue:         0.20,
		FactorQuality:       0.20,
		FactorAgent:         0.15,
		FactorNarrative:     0.10,
		FactorIndustryCycle: 0.10,
	}

	fe.WithNarrativeProvider(func(symbol string) *domain.NarrativeFactorScore {
		if symbol == "TEST.TW" {
			return &domain.NarrativeFactorScore{Score: 0.75, Theme: "AI_capex_surge", HitRate: 0.81, Confidence: 0.90}
		}
		return nil
	})
	fe.WithIndustryCycleProvider(func(symbol string) *domain.IndustryCycleFactorScore {
		if symbol == "TEST.TW" {
			return &domain.IndustryCycleFactorScore{Score: 0.60, Phase: "expansion", PhaseScore: 0.80, Confidence: 0.85}
		}
		return nil
	})

	breakdown, scores := fe.CalculateAllScoresWithBreakdown("TEST.TW", quotes, agentRecs, nil, factorWeights)

	if breakdown.Narrative.Score != 0.75 {
		t.Errorf("expected narrative score 0.75, got %f", breakdown.Narrative.Score)
	}
	if breakdown.IndustryCycle.Score != 0.60 {
		t.Errorf("expected industry cycle score 0.60, got %f", breakdown.IndustryCycle.Score)
	}
	if scores[FactorNarrative] != 0.75 {
		t.Errorf("expected narrative score in map 0.75, got %f", scores[FactorNarrative])
	}
	if scores[FactorIndustryCycle] != 0.60 {
		t.Errorf("expected industry cycle score in map 0.60, got %f", scores[FactorIndustryCycle])
	}

	expectedTotal := 0.5*0.25*0.5 + 0.05*0.20*0.5 + 0.05*0.20*0.5 + 0.8*0.15 + 0.75*0.10 + 0.60*0.10
	if abs(scores["total"]-expectedTotal) > 0.001 {
		t.Errorf("expected total %f, got %f", expectedTotal, scores["total"])
	}
}

func TestFactorEngineWithoutProvidersOmitsNewFactors(t *testing.T) {
	fe := NewFactorEngine()
	quotes := map[string]domain.Quote{
		"TEST.TW": {Symbol: "TEST.TW", Open: 100, Last: 110, IsTradable: true},
	}
	agentRecs := []domain.Recommendation{
		{Agent: "test-agent", Symbol: "TEST.TW", Side: domain.SideBuy, Conviction: 80},
	}
	factorWeights := map[FactorType]float64{
		FactorMomentum:      0.25,
		FactorValue:         0.20,
		FactorQuality:       0.20,
		FactorAgent:         0.15,
		FactorNarrative:     0.10,
		FactorIndustryCycle: 0.10,
	}

	breakdown, scores := fe.CalculateAllScoresWithBreakdown("TEST.TW", quotes, agentRecs, nil, factorWeights)

	if breakdown.Narrative.Score != 0 {
		t.Errorf("expected narrative score 0 when no provider, got %f", breakdown.Narrative.Score)
	}
	if breakdown.IndustryCycle.Score != 0 {
		t.Errorf("expected industry cycle score 0 when no provider, got %f", breakdown.IndustryCycle.Score)
	}
	if _, ok := scores[FactorNarrative]; ok {
		t.Error("expected narrative score not in map when no provider")
	}
	if _, ok := scores[FactorIndustryCycle]; ok {
		t.Error("expected industry cycle score not in map when no provider")
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
