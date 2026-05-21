package portfolio

import (
	"math"
	"testing"

	"github.com/kaecer68/atlas-go/internal/risk"
)

const floatEpsilon = 1e-9

func floatEqual(a, b float64) bool {
	return math.Abs(a-b) < floatEpsilon
}

func TestPortfolioRiskAdjuster_None(t *testing.T) {
	a := NewPortfolioRiskAdjuster(nil)
	weights := map[string]float64{
		"2330": 0.20,
		"2317": 0.15,
		"2454": 0.25,
	}

	adjusted, reasons := a.AdjustWeights(weights, risk.DrawdownNone)

	if len(reasons) == 0 {
		t.Fatal("expected at least one reason for none severity")
	}

	for k, v := range weights {
		if !floatEqual(adjusted[k], v) {
			t.Errorf("none: %s weight changed from %.4f to %.4f", k, v, adjusted[k])
		}
	}

	if len(adjusted) != len(weights) {
		t.Errorf("none: expected %d entries, got %d", len(weights), len(adjusted))
	}
}

func TestPortfolioRiskAdjuster_Light(t *testing.T) {
	a := NewPortfolioRiskAdjuster(nil)
	weights := map[string]float64{
		"2330": 0.10,
		"2317": 0.18,
		"2454": 0.20,
		"3008": 0.05,
	}

	adjusted, reasons := a.AdjustWeights(weights, risk.DrawdownLight)

	if !floatEqual(adjusted["2330"], 0.10) {
		t.Errorf("light: 2330 under 15%% should be unchanged, got %.4f", adjusted["2330"])
	}
	if !floatEqual(adjusted["2317"], 0.15) {
		t.Errorf("light: 2317 over 15%% should be capped to 0.15, got %.4f", adjusted["2317"])
	}
	if !floatEqual(adjusted["2454"], 0.15) {
		t.Errorf("light: 2454 over 15%% should be capped to 0.15, got %.4f", adjusted["2454"])
	}
	if !floatEqual(adjusted["3008"], 0.05) {
		t.Errorf("light: 3008 under 15%% should be unchanged, got %.4f", adjusted["3008"])
	}

	if weights["2317"] != 0.18 {
		t.Errorf("light: original weights were mutated, 2317 changed from 0.18 to %.4f", weights["2317"])
	}

	if len(reasons) < 3 {
		t.Errorf("light: expected at least 3 reasons, got %d: %v", len(reasons), reasons)
	}
}

func TestPortfolioRiskAdjuster_Light_NoCappingNeeded(t *testing.T) {
	a := NewPortfolioRiskAdjuster(nil)
	weights := map[string]float64{
		"2330": 0.10,
		"2317": 0.12,
	}

	adjusted, _ := a.AdjustWeights(weights, risk.DrawdownLight)

	if !floatEqual(adjusted["2330"], 0.10) || !floatEqual(adjusted["2317"], 0.12) {
		t.Errorf("light: weights under cap should be unchanged, got %v", adjusted)
	}
}

func TestPortfolioRiskAdjuster_Moderate(t *testing.T) {
	a := NewPortfolioRiskAdjuster(nil)
	a.SetCyclicalSymbols([]string{"2330", "2454"})

	weights := map[string]float64{
		"2330": 0.12,
		"2317": 0.08,
		"2454": 0.15,
		"3008": 0.09,
	}

	adjusted, reasons := a.AdjustWeights(weights, risk.DrawdownModerate)

	expected2330 := 0.10 * 0.80 // capped to 0.10 then reduced by 20%

	if !floatEqual(adjusted["2330"], expected2330) {
		t.Errorf("moderate: 2330 (cyclical) should be 0.10*0.80=%.4f, got %.4f", expected2330, adjusted["2330"])
	}
	if !floatEqual(adjusted["2317"], 0.08) {
		t.Errorf("moderate: 2317 should be unchanged 0.08, got %.4f", adjusted["2317"])
	}

	expected2454 := 0.10 * 0.80

	if !floatEqual(adjusted["2454"], expected2454) {
		t.Errorf("moderate: 2454 (cyclical) should be 0.10*0.80=%.4f, got %.4f", expected2454, adjusted["2454"])
	}
	if !floatEqual(adjusted["3008"], 0.09) {
		t.Errorf("moderate: 3008 should be unchanged 0.09, got %.4f", adjusted["3008"])
	}

	if len(reasons) < 4 {
		t.Errorf("moderate: expected at least 4 reasons, got %d: %v", len(reasons), reasons)
	}
}

func TestPortfolioRiskAdjuster_Moderate_NoCyclicals(t *testing.T) {
	a := NewPortfolioRiskAdjuster(nil)

	weights := map[string]float64{
		"2330": 0.12,
		"2317": 0.08,
	}

	adjusted, reasons := a.AdjustWeights(weights, risk.DrawdownModerate)

	if !floatEqual(adjusted["2330"], 0.10) {
		t.Errorf("moderate: 2330 should be capped to 0.10 without cyclical reduction, got %.4f", adjusted["2330"])
	}
	if !floatEqual(adjusted["2317"], 0.08) {
		t.Errorf("moderate: 2317 under cap should be unchanged, got %.4f", adjusted["2317"])
	}

	if len(reasons) < 2 {
		t.Errorf("moderate: expected at least 2 reasons, got %d: %v", len(reasons), reasons)
	}
}

func TestPortfolioRiskAdjuster_Severe(t *testing.T) {
	a := NewPortfolioRiskAdjuster(nil)
	a.SetCyclicalSymbols([]string{"2330", "2454"})
	a.SetDefensiveSymbols([]string{"4904"})

	weights := map[string]float64{
		"2330": 0.08,
		"2317": 0.06,
		"2454": 0.12,
		"3008": 0.04,
		"4904": 0.04,
	}

	adjusted, reasons := a.AdjustWeights(weights, risk.DrawdownSevere)

	expectedCyclical := 0.05 * 0.50

	if !floatEqual(adjusted["2330"], expectedCyclical) {
		t.Errorf("severe: 2330 (cyclical) should be 0.05*0.50=%.4f, got %.4f", expectedCyclical, adjusted["2330"])
	}
	if !floatEqual(adjusted["2317"], 0.05) {
		t.Errorf("severe: 2317 should be capped to 0.05, got %.4f", adjusted["2317"])
	}
	if !floatEqual(adjusted["2454"], expectedCyclical) {
		t.Errorf("severe: 2454 (cyclical) should be 0.05*0.50=%.4f, got %.4f", expectedCyclical, adjusted["2454"])
	}
	if !floatEqual(adjusted["3008"], 0.04) {
		t.Errorf("severe: 3008 should be unchanged 0.04, got %.4f", adjusted["3008"])
	}

	expectedDefensive := 0.04 * 1.20

	if !floatEqual(adjusted["4904"], expectedDefensive) {
		t.Errorf("severe: 4904 (defensive) should be 0.04*1.20=%.4f, got %.4f", expectedDefensive, adjusted["4904"])
	}

	if len(reasons) < 5 {
		t.Errorf("severe: expected at least 5 reasons, got %d: %v", len(reasons), reasons)
	}
}

func TestPortfolioRiskAdjuster_Severe_DefensiveBelowCap(t *testing.T) {
	a := NewPortfolioRiskAdjuster(nil)
	a.SetDefensiveSymbols([]string{"4904", "2412"})

	weights := map[string]float64{
		"4904": 0.03,
		"2412": 0.04,
		"2330": 0.07,
	}

	adjusted, _ := a.AdjustWeights(weights, risk.DrawdownSevere)

	if !floatEqual(adjusted["4904"], 0.03*1.20) {
		t.Errorf("severe: 4904 defensive should be 0.036, got %.4f", adjusted["4904"])
	}
	if !floatEqual(adjusted["2412"], 0.04*1.20) {
		t.Errorf("severe: 2412 defensive should be 0.048, got %.4f", adjusted["2412"])
	}
	if !floatEqual(adjusted["2330"], 0.05) {
		t.Errorf("severe: 2330 should be capped to 0.05, got %.4f", adjusted["2330"])
	}
}

func TestPortfolioRiskAdjuster_Emergency(t *testing.T) {
	a := NewPortfolioRiskAdjuster(nil)
	a.SetCyclicalSymbols([]string{"2330"})
	a.SetDefensiveSymbols([]string{"4904"})

	weights := map[string]float64{
		"2330": 0.12,
		"2317": 0.08,
		"4904": 0.05,
	}

	adjusted, reasons := a.AdjustWeights(weights, risk.DrawdownEmergency)

	for symbol, w := range adjusted {
		if w != 0 {
			t.Errorf("emergency: %s should be 0, got %.4f", symbol, w)
		}
	}

	if len(adjusted) != len(weights) {
		t.Errorf("emergency: expected %d entries, got %d", len(weights), len(adjusted))
	}

	if weights["2330"] != 0.12 {
		t.Errorf("emergency: original weights were mutated")
	}

	if len(reasons) < 2 {
		t.Errorf("emergency: expected at least 2 reasons, got %d: %v", len(reasons), reasons)
	}
}

func TestPortfolioRiskAdjuster_Emergency_EmptyWeights(t *testing.T) {
	a := NewPortfolioRiskAdjuster(nil)
	weights := map[string]float64{}

	adjusted, reasons := a.AdjustWeights(weights, risk.DrawdownEmergency)

	if len(adjusted) != 0 {
		t.Errorf("emergency: empty weights should produce empty result, got %d entries", len(adjusted))
	}
	if len(reasons) == 0 {
		t.Errorf("emergency: should still produce reasons for empty weights")
	}
}

func TestPortfolioRiskAdjuster_UnknownSeverity(t *testing.T) {
	a := NewPortfolioRiskAdjuster(nil)
	weights := map[string]float64{
		"2330": 0.20,
		"2317": 0.15,
	}

	const unknownSeverity risk.DrawdownAction = 999
	adjusted, reasons := a.AdjustWeights(weights, unknownSeverity)

	for k, v := range weights {
		if !floatEqual(adjusted[k], v) {
			t.Errorf("unknown: %s should be unchanged, expected %.4f got %.4f", k, v, adjusted[k])
		}
	}

	if len(reasons) == 0 {
		t.Errorf("unknown: expected reasons for unknown severity")
	}
}

func TestPortfolioRiskAdjuster_SetSymbolsOverwrite(t *testing.T) {
	a := NewPortfolioRiskAdjuster(nil)

	a.SetCyclicalSymbols([]string{"2330", "2454"})
	a.SetDefensiveSymbols([]string{"4904"})

	a.SetCyclicalSymbols([]string{"3008"})
	a.SetDefensiveSymbols([]string{"2412"})

	weights := map[string]float64{
		"2330": 0.20,
		"3008": 0.20,
	}

	adjusted, _ := a.AdjustWeights(weights, risk.DrawdownSevere)

	if !floatEqual(adjusted["2330"], 0.05) {
		t.Errorf("2330 (overwritten, not cyclical) should only be capped to 0.05, got %.4f", adjusted["2330"])
	}
	if !floatEqual(adjusted["3008"], 0.05*0.50) {
		t.Errorf("3008 (now cyclical) should be 0.05*0.50=0.025, got %.4f", adjusted["3008"])
	}
}

func TestPortfolioRiskAdjuster_ZeroWeightPositions(t *testing.T) {
	a := NewPortfolioRiskAdjuster(nil)
	a.SetCyclicalSymbols([]string{"2330"})

	weights := map[string]float64{
		"2330": 0.0,
		"2317": 0.0,
	}

	adjusted, _ := a.AdjustWeights(weights, risk.DrawdownSevere)

	if adjusted["2330"] != 0.0 {
		t.Errorf("zero-weight cyclical should stay 0, got %.4f", adjusted["2330"])
	}
	if adjusted["2317"] != 0.0 {
		t.Errorf("zero-weight non-cyclical should stay 0, got %.4f", adjusted["2317"])
	}
}
