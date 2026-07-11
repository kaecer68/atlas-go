package narrative

import "math"

type SeasonalExpectation struct {
	HistoricalAvgReturn float64  `json:"historical_avg_return"`
	CurrentReturn       *float64 `json:"current_return"`
	ExpectationGap      float64  `json:"expectation_gap"`
	AlreadyPricedIn     bool     `json:"already_priced_in"`
	SurprisePotential   float64  `json:"surprise_potential"`
	Confidence          float64  `json:"confidence"`
}

type SeasonalAnalyzer struct {
	history map[string][]float64
}

func NewSeasonalAnalyzer() *SeasonalAnalyzer {
	return &SeasonalAnalyzer{
		history: make(map[string][]float64),
	}
}

func (sa *SeasonalAnalyzer) CalculateExpectationGap(historicalAvg, currentReturn float64) SeasonalExpectation {
	gap := historicalAvg - currentReturn

	surprisePotential := 0.0
	if historicalAvg != 0 {
		surprisePotential = math.Max(0, -gap) / math.Abs(historicalAvg)
		surprisePotential = math.Min(1.0, surprisePotential)
	}

	return SeasonalExpectation{
		HistoricalAvgReturn: historicalAvg,
		CurrentReturn:       &currentReturn,
		ExpectationGap:      gap,
		AlreadyPricedIn:     currentReturn > historicalAvg,
		SurprisePotential:   surprisePotential,
		Confidence:          0.7,
	}
}

func (sa *SeasonalAnalyzer) CalculateWeightedHitRate(baseHitRate float64, expectation SeasonalExpectation) float64 {
	if expectation.AlreadyPricedIn {
		return baseHitRate * 0.5
	}

	reliability := 1.0 - expectation.SurprisePotential
	return baseHitRate * (0.7 + 0.3*reliability)
}
