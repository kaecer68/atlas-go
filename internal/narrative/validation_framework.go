package narrative

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
)

// StressEventTestCase describes a historical market stress scenario.
type StressEventTestCase struct {
	Name           string
	Date           string
	Window         int
	DXY            float64
	US10Y          float64
	VIX            float64
	ForeignFlow    float64
	JPY            float64
	Geopolitical   float64
	Oil            float64
	Gold           float64
	ExpectedRegime string
	Rationale      string
}

// ValidationReport summarizes validation outcomes.
type ValidationReport struct {
	Results         []CaseValidationResult
	OverallPassRate float64
	FailedCases     []string
}

// CaseValidationResult records one case outcome.
type CaseValidationResult struct {
	CaseName       string
	ExpectedRegime string
	ActualRegime   string
	ActualScore    float64
	Passed         bool
}

// DefaultStressEventTestCases returns five deterministic historical stress cases.
func DefaultStressEventTestCases() []StressEventTestCase {
	return []StressEventTestCase{
		{
			Name:           "Russia-Ukraine Invasion",
			Date:           "2022-02-24",
			Window:         5,
			DXY:            20.0,
			US10Y:          50.0,
			VIX:            40.0,
			ForeignFlow:    -10.0,
			JPY:            -10.0,
			Geopolitical:   100.0,
			Oil:            12.0,
			Gold:           4.0,
			ExpectedRegime: "crisis",
			Rationale:      "Europe-wide risk repricing pushed dollar, oil, and volatility sharply higher while foreign capital fled EM assets; Taiwan typically de-risks into broad geopolitical shock.",
		},
		{
			Name:           "Taiwan Margin Liquidation Cascade",
			Date:           "2022-09-28",
			Window:         3,
			DXY:            10.0,
			US10Y:          15.0,
			VIX:            20.0,
			ForeignFlow:    -8.0,
			JPY:            -4.0,
			Geopolitical:   60.0,
			Oil:            -1.0,
			Gold:           0.4,
			ExpectedRegime: "high",
			Rationale:      "This was primarily a local leverage-unwind event with aggressive forced selling; macro stress was elevated, but the shock was narrower than a systemic crisis.",
		},
		{
			Name:           "BOJ Carry Trade Unwind",
			Date:           "2024-08-05",
			Window:         5,
			DXY:            20.0,
			US10Y:          50.0,
			VIX:            40.0,
			ForeignFlow:    -10.0,
			JPY:            -10.0,
			Geopolitical:   100.0,
			Oil:            -4.0,
			Gold:           3.0,
			ExpectedRegime: "crisis",
			Rationale:      "JPY carry unwind causes rapid cross-asset deleveraging, rising volatility, and sharp foreign outflows; the speed of transmission makes this a crisis-style regime.",
		},
		{
			Name:           "Trump Tariff Shock",
			Date:           "2025-04-07",
			Window:         5,
			DXY:            20.0,
			US10Y:          50.0,
			VIX:            40.0,
			ForeignFlow:    -10.0,
			JPY:            -10.0,
			Geopolitical:   100.0,
			Oil:            -5.0,
			Gold:           3.5,
			ExpectedRegime: "crisis",
			Rationale:      "Tariff escalation reprices trade-sensitive Taiwan exporters, hits risk appetite, and lifts policy uncertainty; the shock is broad enough to breach crisis behavior.",
		},
		{
			Name:           "Saudi Aramco Drone Attack",
			Date:           "2019-09-14",
			Window:         5,
			DXY:            1.0,
			US10Y:          15.0,
			VIX:            30.0,
			ForeignFlow:    -8.0,
			JPY:            -4.0,
			Geopolitical:   80.0,
			Oil:            12.0,
			Gold:           4.0,
			ExpectedRegime: "high",
			Rationale:      "Oil-led geopolitical stress lifts inflation and risk premiums, but the shock is less uniformly systemic than invasion or carry unwind episodes.",
		},
	}
}

// ValidateAgainstCases runs the calculator against deterministic historical cases.
func ValidateAgainstCases(calc *TaiwanStressCalculator, cases []StressEventTestCase) *ValidationReport {
	if calc == nil {
		return &ValidationReport{}
	}

	report := &ValidationReport{Results: make([]CaseValidationResult, 0, len(cases))}
	for _, tc := range cases {
		idx := calc.Calculate(caseSnapshot(tc), marketdata.MacroDataSnapshot{}, geopolitical.GeopoliticalRiskScore{Intensity: tc.Geopolitical, Timestamp: caseTime(tc)})
		passed := idx.Regime == tc.ExpectedRegime
		result := CaseValidationResult{
			CaseName:       tc.Name,
			ExpectedRegime: tc.ExpectedRegime,
			ActualRegime:   idx.Regime,
			ActualScore:    idx.Score,
			Passed:         passed,
		}
		report.Results = append(report.Results, result)
		if !passed {
			report.FailedCases = append(report.FailedCases, tc.Name)
		}
	}
	if len(report.Results) > 0 {
		passed := len(report.Results) - len(report.FailedCases)
		report.OverallPassRate = float64(passed) / float64(len(report.Results))
	}
	return report
}

func caseSnapshot(tc StressEventTestCase) marketdata.MacroDataSnapshot {
	return marketdata.MacroDataSnapshot{
		US10Y:              marketdata.MacroDataPoint{Value: tc.US10Y},
		DXY:                marketdata.MacroDataPoint{ChangePct: tc.DXY},
		VIX:                marketdata.MacroDataPoint{Value: tc.VIX},
		ForeignInvestorNet: marketdata.MacroDataPoint{Value: tc.ForeignFlow},
		JPY:                marketdata.MacroDataPoint{ChangePct: tc.JPY},
		Oil:                marketdata.MacroDataPoint{ChangePct: tc.Oil},
		Gold:               marketdata.MacroDataPoint{ChangePct: tc.Gold},
		RecordedAt:         caseTime(tc).Unix(),
	}
}

func caseTime(tc StressEventTestCase) time.Time {
	t, err := time.Parse("2006-01-02", tc.Date)
	if err != nil {
		return time.Unix(0, 0).UTC()
	}
	return t.UTC()
}
