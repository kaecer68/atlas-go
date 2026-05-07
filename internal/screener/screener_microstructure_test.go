package screener

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

func TestEngine_ApplyMicrostructureFilter(t *testing.T) {
	fe := portfolio.NewFactorEngine()
	fp := portfolio.NewFundamentalProvider()
	engine := NewEngine(fe, fp)

	minLiq := 30.0
	criteria := domain.ScreeningCriteria{
		MinLiquidityScore: &minLiq,
	}

	microData := map[string]marketdata.MicrostructureSnapshot{
		"2330.TW": {
			Symbol:         "2330.TW",
			LiquidityScore: 85.0,
			SpreadEstimate: 0.005,
		},
	}

	passed, reason := engine.ApplyMicrostructureFilter("2330.TW", criteria, microData)
	if !passed {
		t.Errorf("expected pass for liquid stock, got: %s", reason)
	}

	microData["2330.TW"] = marketdata.MicrostructureSnapshot{
		Symbol:         "2330.TW",
		LiquidityScore: 15.0,
		SpreadEstimate: 0.02,
	}

	passed, _ = engine.ApplyMicrostructureFilter("2330.TW", criteria, microData)
	if passed {
		t.Error("expected fail for illiquid stock")
	}
}
