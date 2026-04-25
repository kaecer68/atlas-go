package orchestrator

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestGenerateForwardReturn_ValidQuotePositiveIntraday(t *testing.T) {
	fallback := DefaultFallbackParams()
	quote := domain.Quote{
		Symbol: "2330",
		Open:   100.0,
		Last:   102.0,
	}

	fr := GenerateForwardReturn("2330", quote, domain.RegimeRiskOn, fallback)

	expected := (102.0 - 100.0) / 100.0 * 0.9
	if math.Abs(fr-expected) > 0.0001 {
		t.Errorf("expected %f, got %f", expected, fr)
	}
}

func TestGenerateForwardReturn_ValidQuoteNegativeIntraday(t *testing.T) {
	fallback := DefaultFallbackParams()
	quote := domain.Quote{
		Symbol: "2330",
		Open:   100.0,
		Last:   98.0,
	}

	fr := GenerateForwardReturn("2330", quote, domain.RegimeRiskOn, fallback)

	expected := (98.0 - 100.0) / 100.0 * 0.9
	if math.Abs(fr-expected) > 0.0001 {
		t.Errorf("expected %f, got %f", expected, fr)
	}
}

func TestGenerateForwardReturn_FlatDayUsesDistributionFallback(t *testing.T) {
	fallback := DefaultFallbackParams()
	quote := domain.Quote{
		Symbol: "TESTFLAT",
		Open:   100.0,
		Last:   100.0005,
	}

	fr := GenerateForwardReturn("TESTFLAT", quote, domain.RegimeRiskOn, fallback)

	intraday := (quote.Last - quote.Open) / quote.Open
	rawFr := intraday * 0.9

	if math.Abs(fr-rawFr) < 0.00001 {
		t.Errorf("flat day should use distribution, not raw intraday. got %f, raw would be %f", fr, rawFr)
	}
}

func TestGenerateForwardReturn_NoQuoteUsesDistributionFallback(t *testing.T) {
	fallback := DefaultFallbackParams()
	quote := domain.Quote{
		Symbol: "2330",
		Open:   0,
		Last:   0,
	}

	fr := GenerateForwardReturn("2330", quote, domain.RegimeRiskOn, fallback)

	if fr == 0 {
		t.Errorf("no quote should use distribution fallback, got %f", fr)
	}
}

func TestGenerateForwardReturn_RiskOnVsRiskOffDiffers(t *testing.T) {
	fallback := DefaultFallbackParams()
	quote := domain.Quote{
		Symbol: "2330",
		Open:   0,
		Last:   0,
	}

	frRiskOn := GenerateForwardReturn("2330", quote, domain.RegimeRiskOn, fallback)
	frRiskOff := GenerateForwardReturn("2330", quote, domain.RegimeRiskOff, fallback)

	if frRiskOn == frRiskOff {
		t.Errorf("RiskOn and RiskOff should produce different results, got both %f", frRiskOn)
	}
}

func TestGenerateForwardReturn_SymbolHashDeterminism(t *testing.T) {
	fallback := DefaultFallbackParams()
	quote := domain.Quote{
		Symbol: "2330",
		Open:   0,
		Last:   0,
	}

	results := make(map[float64]bool)
	for i := 0; i < 10; i++ {
		fr := GenerateForwardReturn("2330", quote, domain.RegimeRiskOn, fallback)
		results[fr] = true
	}

	if len(results) != 1 {
		t.Errorf("same symbol should produce deterministic result, got %d unique values", len(results))
	}
}

func TestGenerateForwardReturn_QuoteWithNoOpenFallsBack(t *testing.T) {
	fallback := DefaultFallbackParams()
	quote := domain.Quote{
		Symbol: "2330",
		Open:   0,
		Last:   100.0,
	}

	fr := GenerateForwardReturn("2330", quote, domain.RegimeRiskOn, fallback)

	if fr == 0 {
		t.Errorf("no open price should use distribution fallback, got %f", fr)
	}
}

func TestDefaultFallbackParams_Values(t *testing.T) {
	fallback := DefaultFallbackParams()

	if fallback.RiskOnParams.Mean <= 0 {
		t.Errorf("RiskOn Mean should be positive, got %f", fallback.RiskOnParams.Mean)
	}
	if fallback.RiskOnParams.StdDev <= 0 {
		t.Errorf("RiskOn StdDev should be positive, got %f", fallback.RiskOnParams.StdDev)
	}
	if fallback.RiskOnParams.MinReturn >= fallback.RiskOnParams.MaxReturn {
		t.Errorf("RiskOn MinReturn should be less than MaxReturn")
	}
	if fallback.RiskOffParams.Mean <= 0 {
		t.Errorf("RiskOff Mean should be positive, got %f", fallback.RiskOffParams.Mean)
	}
	if fallback.RiskOffParams.StdDev <= 0 {
		t.Errorf("RiskOff StdDev should be positive, got %f", fallback.RiskOffParams.StdDev)
	}
	if fallback.RiskOffParams.MinReturn >= fallback.RiskOffParams.MaxReturn {
		t.Errorf("RiskOff MinReturn should be less than MaxReturn")
	}
}

func TestGenerateForwardReturn_ClipToMax(t *testing.T) {
	fallback := DefaultFallbackParams()
	quote := domain.Quote{
		Symbol: "2330",
		Open:   100.0,
		Last:   110.0,
	}

	fr := GenerateForwardReturn("2330", quote, domain.RegimeRiskOn, fallback)

	if fr > 0.05 {
		t.Errorf("forward return should be clipped to 0.05, got %f", fr)
	}
}

func TestGenerateForwardReturn_ClipToMin(t *testing.T) {
	fallback := DefaultFallbackParams()
	quote := domain.Quote{
		Symbol: "2330",
		Open:   100.0,
		Last:   90.0,
	}

	fr := GenerateForwardReturn("2330", quote, domain.RegimeRiskOn, fallback)

	if fr < -0.05 {
		t.Errorf("forward return should be clipped to -0.05, got %f", fr)
	}
}

func TestGenerateForwardReturn_EmptySymbol(t *testing.T) {
	fallback := DefaultFallbackParams()
	quote := domain.Quote{
		Symbol: "",
		Open:   0,
		Last:   0,
	}

	fr := GenerateForwardReturn("", quote, domain.RegimeRiskOn, fallback)

	if fr == 0 {
		t.Errorf("empty symbol should still produce distribution value, got %f", fr)
	}
}

func TestHashString_Deterministic(t *testing.T) {
	h1 := hashString("test")
	h2 := hashString("test")

	if h1 != h2 {
		t.Errorf("hashString should be deterministic, got %d and %d", h1, h2)
	}
}

func TestHashString_DifferentSymbols(t *testing.T) {
	h1 := hashString("2330")
	h2 := hashString("2317")

	if h1 == h2 {
		t.Errorf("different symbols should produce different hashes, got both %d", h1)
	}
}

func TestGenerateForwardReturn_NeutralRegime(t *testing.T) {
	fallback := DefaultFallbackParams()
	quote := domain.Quote{
		Symbol: "2330",
		Open:   0,
		Last:   0,
	}

	fr := GenerateForwardReturn("2330", quote, domain.RegimeNeutral, fallback)

	if fr == 0 {
		t.Errorf("neutral regime should use RiskOn params (default), got %f", fr)
	}
}
