package narrative

// SeasonalBridge implements industry.NarrativeSeasonalProvider, bridging
// the macro-narrative event system into seasonal pattern adjustment calculations.
// It maps active narrative themes to industry-specific seasonal multipliers.
type SeasonalBridge struct {
	engine       *NarrativeEngine
	activeEvents []NarrativeEvent
}

// NewSeasonalBridge creates a bridge from a NarrativeEngine.
func NewSeasonalBridge(engine *NarrativeEngine) *SeasonalBridge {
	return &SeasonalBridge{engine: engine}
}

// SetActiveEvents updates the active event list. Call this before computing
// seasonal adjustments to reflect the latest narrative state.
func (sb *SeasonalBridge) SetActiveEvents(events []NarrativeEvent) {
	sb.activeEvents = events
}

// ActiveThemes returns all active narrative theme identifiers.
func (sb *SeasonalBridge) ActiveThemes() []string {
	seen := make(map[string]bool)
	var themes []string
	for _, e := range sb.activeEvents {
		if !seen[e.Theme] {
			seen[e.Theme] = true
			themes = append(themes, e.Theme)
		}
	}
	return themes
}

// SeasonalMultiplier returns the seasonal adjustment multiplier for a given
// narrative theme and industry. Themes are mapped based on known macro→sector
// causal relationships from the narrative knowledge base:
//
//	oil_price_shock → amplifies energy (+), dampens industrial/shipping (−)
//	AI_capex_surge → amplifies semiconductor and ai_supply_chain
//	US_rates_up     → dampens dividend-themed sectors (financials, high_dividend)
//	JPY_carry_unwind → dampens risk-on sectors (ai_supply_chain, growth)
//	geopolitical_risk_spike → amplifies defensive (consumer, financials), dampens export
func (sb *SeasonalBridge) SeasonalMultiplier(theme string, industryID string) float64 {
	switch theme {
	case "oil_price_shock":
		switch industryID {
		case "energy":
			return 1.12
		case "shipping", "industrial":
			return 0.92
		default:
			return 1.0
		}
	case "AI_capex_surge":
		switch industryID {
		case "semiconductor", "ai_supply_chain", "electronics":
			return 1.15
		default:
			return 1.0
		}
	case "US_rates_up":
		switch industryID {
		case "financials":
			return 1.08
		case "consumer", "electronics", "industrial":
			return 0.95
		default:
			return 1.0
		}
	case "JPY_carry_unwind":
		switch industryID {
		case "semiconductor", "ai_supply_chain", "shipping":
			return 0.90
		case "financials", "consumer":
			return 1.05
		default:
			return 1.0
		}
	case "geopolitical_risk_spike":
		switch industryID {
		case "consumer", "financials":
			return 1.06
		case "semiconductor", "ai_supply_chain", "shipping":
			return 0.94
		default:
			return 1.0
		}
	default:
		return 1.0
	}
}
