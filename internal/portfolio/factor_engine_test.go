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
	if score != 0.1 {
		t.Errorf("expected fallback value score 0.1, got %f", score)
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
