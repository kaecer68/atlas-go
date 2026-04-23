package portfolio

import (
	"context"
	"testing"

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
