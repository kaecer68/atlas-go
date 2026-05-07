package marketdata

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestCalculateSpreadEstimate(t *testing.T) {
	quotes := map[string]domain.Quote{
		"2330.TW": {
			Symbol: "2330.TW",
			Last:   500.0,
			High:   510.0,
			Low:    495.0,
			Volume: 100000,
		},
	}

	avgVolume := map[string]int64{"2330.TW": 50000}

	p := &MicrostructureProvider{}
	spread := p.calculateSpreadEstimate(quotes["2330.TW"], avgVolume["2330.TW"])

	if spread <= 0 {
		t.Errorf("expected positive spread, got %f", spread)
	}
	if spread > 0.05 {
		t.Errorf("expected spread < 0.05 for liquid stock, got %f", spread)
	}
}

func TestCalculateLiquidityScore(t *testing.T) {
	p := &MicrostructureProvider{}

	score1 := p.calculateLiquidityScore(100000, 50000, 0.001)
	if score1 < 80 {
		t.Errorf("expected high liquidity score, got %f", score1)
	}

	score2 := p.calculateLiquidityScore(5000, 50000, 0.05)
	if score2 > 30 {
		t.Errorf("expected low liquidity score, got %f", score2)
	}
}

func TestMicrostructureProvider_Calculate(t *testing.T) {
	p := NewMicrostructureProvider(func(symbol string) int64 {
		return 50000
	})

	quote := domain.Quote{
		Symbol: "2330.TW",
		Last:   500.0,
		High:   510.0,
		Low:    495.0,
		Volume: 100000,
	}

	snap := p.Calculate("2330.TW", quote)

	if snap.Symbol != "2330.TW" {
		t.Errorf("expected symbol 2330.TW, got %s", snap.Symbol)
	}
	if snap.LiquidityScore <= 0 {
		t.Errorf("expected positive liquidity score, got %f", snap.LiquidityScore)
	}
	if snap.AbnormalVolume {
		t.Error("unexpected abnormal volume for 2x avg")
	}
}
