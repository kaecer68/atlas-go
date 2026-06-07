package stress

import (
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// Scenario defines a historical stress test scenario with specific market conditions.
type Scenario struct {
	ID          string
	Name        string
	Description string
	Date        time.Time
	Quotes      []domain.Quote
	Regime      domain.Regime
	WindowDays  int // trading days to simulate (after scenario.Date)
}

// Built-in historical scenarios for Taiwan equity stress testing.
var (
	ScenarioCOVIDCrash = Scenario{
		ID:          "covid_crash_2020",
		Name:        "COVID-19 Market Crash",
		Description: "March 2020: VIX spike to 80+, global market crash, liquidity freeze",
		Date:        time.Date(2020, 3, 16, 0, 0, 0, 0, time.UTC),
		Regime:      domain.RegimeRiskOff,
		WindowDays:  40,
		Quotes: []domain.Quote{
			{Symbol: "VIX", Last: 82.7, IsTradable: true},
			{Symbol: "DXY", Last: 99.0, Open: 96.0, IsTradable: true},
			{Symbol: "US10Y", Last: 0.73, IsTradable: true},
			{Symbol: "OIL", Last: 20.0, Open: 45.0, IsTradable: true},
		},
	}

	ScenarioFedRateHikes = Scenario{
		ID:          "fed_hikes_2022",
		Name:        "Fed Aggressive Rate Hikes",
		Description: "June 2022: US10Y > 3%, tech selloff, QT begins",
		Date:        time.Date(2022, 6, 15, 0, 0, 0, 0, time.UTC),
		Regime:      domain.RegimeRiskOff,
		WindowDays:  30,
		Quotes: []domain.Quote{
			{Symbol: "VIX", Last: 32.0, IsTradable: true},
			{Symbol: "DXY", Last: 105.0, Open: 102.0, IsTradable: true},
			{Symbol: "US10Y", Last: 3.4, IsTradable: true},
			{Symbol: "OIL", Last: 115.0, Open: 120.0, IsTradable: true},
		},
	}

	ScenarioAIBubble = Scenario{
		ID:          "ai_bubble_2024",
		Name:        "AI Semiconductor Bubble",
		Description: "March 2024: AI capex surge, semiconductor euphoria, VIX complacent",
		Date:        time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		Regime:      domain.RegimeRiskOn,
		WindowDays:  20,
		Quotes: []domain.Quote{
			{Symbol: "VIX", Last: 14.0, IsTradable: true},
			{Symbol: "DXY", Last: 104.0, Open: 103.5, IsTradable: true},
			{Symbol: "US10Y", Last: 4.2, IsTradable: true},
			{Symbol: "OIL", Last: 78.0, Open: 76.0, IsTradable: true},
		},
	}

	ScenarioTaiwanTension = Scenario{
		ID:          "taiwan_tension_2022",
		Name:        "Taiwan Geopolitical Tension",
		Description: "August 2022: Pelosi visit, cross-strait tension, regional risk premium spike",
		Date:        time.Date(2022, 8, 2, 0, 0, 0, 0, time.UTC),
		Regime:      domain.RegimeRiskOff,
		WindowDays:  15,
		Quotes: []domain.Quote{
			{Symbol: "VIX", Last: 26.0, IsTradable: true},
			{Symbol: "DXY", Last: 106.0, Open: 105.0, IsTradable: true},
			{Symbol: "US10Y", Last: 2.6, IsTradable: true},
			{Symbol: "OIL", Last: 94.0, Open: 98.0, IsTradable: true},
		},
	}

	ScenarioNormalMarket = Scenario{
		ID:          "normal_market_2024",
		Name:        "Normal Market Conditions",
		Description: "January 2024: Balanced regime, VIX moderate, no extreme events",
		Date:        time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		Regime:      domain.RegimeNeutral,
		WindowDays:  20,
		Quotes: []domain.Quote{
			{Symbol: "VIX", Last: 18.0, IsTradable: true},
			{Symbol: "DXY", Last: 103.0, Open: 102.8, IsTradable: true},
			{Symbol: "US10Y", Last: 4.0, IsTradable: true},
			{Symbol: "OIL", Last: 72.0, Open: 71.5, IsTradable: true},
		},
	}

	ScenarioStagflation = Scenario{
		ID:          "stagflation_2023",
		Name:        "Stagflation",
		Description: "2023: High inflation + high rates + stagnant growth",
		Regime:      domain.RegimeRiskOff,
		WindowDays:  20,
		Quotes: []domain.Quote{
			{Symbol: "VIX", Last: 22.0, IsTradable: true},
			{Symbol: "DXY", Last: 106.5, Open: 106.0, IsTradable: true},
			{Symbol: "US10Y", Last: 5.1, IsTradable: true},
			{Symbol: "OIL", Last: 95.0, Open: 92.0, IsTradable: true},
		},
	}

	ScenarioEMContagion = Scenario{
		ID:         "em_contagion_2018",
		Regime:     domain.RegimeRiskOff,
		WindowDays: 20,
		Quotes: []domain.Quote{
			{Symbol: "VIX", Last: 24.0, IsTradable: true},
			{Symbol: "DXY", Last: 96.5, Open: 95.0, IsTradable: true},
			{Symbol: "US10Y", Last: 2.9, IsTradable: true},
			{Symbol: "OIL", Last: 67.0, Open: 70.0, IsTradable: true},
		},
	}

	ScenarioLiquidityCrunch = Scenario{
		ID:         "liquidity_crunch_2008",
		Regime:     domain.RegimeRiskOff,
		WindowDays: 20,
		Quotes: []domain.Quote{
			{Symbol: "VIX", Last: 69.3, IsTradable: true},
			{Symbol: "DXY", Last: 83.0, Open: 80.0, IsTradable: true},
			{Symbol: "US10Y", Last: 3.8, IsTradable: true},
			{Symbol: "OIL", Last: 36.0, Open: 90.0, IsTradable: true},
		},
	}
)

// AllScenarios returns all built-in stress test scenarios.
func AllScenarios() []Scenario {
	return []Scenario{
		ScenarioCOVIDCrash,
		ScenarioFedRateHikes,
		ScenarioAIBubble,
		ScenarioTaiwanTension,
		ScenarioNormalMarket,
		ScenarioStagflation,
		ScenarioEMContagion,
		ScenarioLiquidityCrunch,
	}
}

// GetScenarioByID returns a scenario by its ID.
func GetScenarioByID(id string) (Scenario, error) {
	for _, s := range AllScenarios() {
		if s.ID == id {
			return s, nil
		}
	}
	return Scenario{}, fmt.Errorf("scenario not found: %s", id)
}

// MergeQuotes merges scenario macro quotes with individual stock quotes.
func (s Scenario) MergeQuotes(stockQuotes []domain.Quote) []domain.Quote {
	merged := make([]domain.Quote, 0, len(s.Quotes)+len(stockQuotes))
	merged = append(merged, s.Quotes...)
	merged = append(merged, stockQuotes...)
	return merged
}
