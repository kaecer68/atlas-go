package portfolio

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestFactorEngine_CalculateLiquidityScore(t *testing.T) {
	fe := NewFactorEngine()

	micro := marketdata.MicrostructureSnapshot{
		LiquidityScore:    85.0,
		TradeabilityScore: 80.0,
		AbnormalVolume:    false,
	}

	score := fe.CalculateLiquidityScore("2330.TW", micro)
	if score <= 0 {
		t.Errorf("expected positive score for liquid stock, got %f", score)
	}

	micro2 := marketdata.MicrostructureSnapshot{
		LiquidityScore:    10.0,
		TradeabilityScore: 5.0,
		AbnormalVolume:    true,
	}

	score2 := fe.CalculateLiquidityScore("2330.TW", micro2)
	if score2 > -3 {
		t.Errorf("expected negative score for illiquid stock, got %f", score2)
	}
}
