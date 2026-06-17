package sim

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestNewEngine_HasDefaultTaxCalculator(t *testing.T) {
	engine := NewEngine(domain.SimulationConstraints{
		StartingCash: 1000000,
	})

	if engine.taxCalc == nil {
		t.Fatalf("NewEngine should default-initialize taxCalc with Taiwan statutory rates")
	}
}
