package experiment

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestScoreSimulationResult_UsesAfterTaxPnL(t *testing.T) {
	result := domain.SimulationResult{
		EndingCash:    1100000,
		BeforeTaxPnL:  100000,
		AfterTaxPnL:   95000,
		TotalTaxPaid:  5000,
		Positions:     []domain.Position{},
	}
	startingCash := 1000000

	score := scoreSimulationResult(result, nil, float64(startingCash))

	expectedRate := 95000.0 / 1000000.0
	if math.Abs(score-expectedRate) > 0.0001 {
		t.Errorf("expected after-tax return rate %f, got %f", expectedRate, score)
	}
}

func TestScoreSimulationResult_NoTaxReturnsZero(t *testing.T) {
	result := domain.SimulationResult{
		EndingCash: 1000000,
	}
	score := scoreSimulationResult(result, nil, 0.0)
	if score != 0 {
		t.Errorf("expected 0 for zero starting cash, got %f", score)
	}
}
